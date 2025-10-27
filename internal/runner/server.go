package runner

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/api"
	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
	"github.com/Use-Tusk/tusk-drift-cli/internal/logging"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
	"github.com/Use-Tusk/tusk-drift-cli/internal/version"
	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

type CommunicationType string

const (
	CommunicationUnix CommunicationType = "unix"
	CommunicationTCP  CommunicationType = "tcp"
)

// Server handles Unix socket communication with the SDK
type Server struct {
	socketPath string
	listener   net.Listener

	// Hashes for fast lookup
	spans                        map[string][]*core.Span
	spanUsage                    map[string]map[string]bool         // traceId -> spanId -> isUsed
	spansByPackage               map[string]map[string][]*core.Span // traceId -> packageName -> spans
	suiteSpansByPackage          map[string][]*core.Span            // packageName -> spans (for suite spans)
	spansByReducedValueHash      map[string]map[string][]*core.Span // traceId -> reducedValueHash -> spans
	suiteSpansByReducedValueHash map[string][]*core.Span            // reducedValueHash -> spans (for suite)
	spansByValueHash             map[string]map[string][]*core.Span // traceId -> valueHash -> spans
	suiteSpansByValueHash        map[string][]*core.Span

	currentTestID      atomic.Value
	ctx                context.Context
	cancel             context.CancelFunc
	wg                 sync.WaitGroup
	mu                 sync.RWMutex
	connWriteMutex     sync.Mutex
	sdkVersion         string
	sdkConnected       bool
	sdkConnectedChan   chan struct{}
	suiteSpans         []*core.Span
	matchEvents        map[string][]MatchEvent
	replayInbound      map[string]*core.Span
	mockNotFoundEvents map[string][]MockNotFoundEvent

	// For TCP communication (docker environments)
	communicationType CommunicationType
	tcpListener       net.Listener
	tcpPort           int
}

// MessageType represents the type of message sent by the SDK
type MessageType string

const (
	MessageTypeSDKConnect  MessageType = "sdk_connect"
	MessageTypeMockRequest MessageType = "mock_request"
)

type MatchEvent struct {
	SpanID     string              `json:"spanId"`
	MatchLevel *backend.MatchLevel `json:"matchLevel"`
	StackTrace string              `json:"stackTrace"`
	InputData  map[string]any      `json:"inputData,omitempty"`
	Timestamp  time.Time           `json:"timestamp"`
	ReplaySpan *core.Span          `json:"replaySpan,omitempty"`
}

type MockNotFoundEvent struct {
	PackageName string     `json:"packageName"`
	SpanName    string     `json:"spanName"`   // e.g., "GET /api/users" or "pg.query"
	Operation   string     `json:"operation"`  // "GET", "POST", "query", etc.
	StackTrace  string     `json:"stackTrace"` // Code location that made the call
	Timestamp   time.Time  `json:"timestamp"`
	Error       string     `json:"error"`      // Full error message
	ReplaySpan  *core.Span `json:"replaySpan"` // The outbound span that failed to find a mock
}

func isDockerCommand(cmd string) bool {
	cmd = strings.ToLower(cmd)
	cmd = strings.Join(strings.Fields(cmd), " ")

	return strings.Contains(cmd, "docker ") ||
		strings.Contains(cmd, "docker-compose ") ||
		cmd == "docker" ||
		cmd == "docker-compose"
}

func determineCommunicationType(cfg *config.ServiceConfig) CommunicationType {
	commType := cfg.Communication.Type

	// Auto-detect based on start command
	if commType == "auto" {
		if isDockerCommand(cfg.Start.Command) {
			slog.Debug("Auto-detected Docker command, using TCP communication")
			return CommunicationTCP
		}
		return CommunicationUnix
	}

	if commType == "tcp" {
		return CommunicationTCP
	}
	return CommunicationUnix
}

// NewServer creates a new server instance
func NewServer(serviceID string, cfg *config.ServiceConfig) (*Server, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Determine communication type
	commType := determineCommunicationType(cfg)

	server := &Server{
		spans:                        make(map[string][]*core.Span),
		spanUsage:                    make(map[string]map[string]bool),
		spansByPackage:               make(map[string]map[string][]*core.Span),
		suiteSpansByPackage:          make(map[string][]*core.Span),
		spansByReducedValueHash:      make(map[string]map[string][]*core.Span),
		suiteSpansByReducedValueHash: make(map[string][]*core.Span),
		spansByValueHash:             make(map[string]map[string][]*core.Span),
		suiteSpansByValueHash:        make(map[string][]*core.Span),

		ctx:                ctx,
		cancel:             cancel,
		sdkConnected:       false,
		sdkConnectedChan:   make(chan struct{}),
		matchEvents:        make(map[string][]MatchEvent),
		replayInbound:      make(map[string]*core.Span),
		mockNotFoundEvents: make(map[string][]MockNotFoundEvent),
		communicationType:  commType,
		tcpPort:            cfg.Communication.TCPPort,
	}

	return server, nil
}

// Start begins listening (Unix socket or TCP)
func (ms *Server) Start() error {
	if ms.communicationType == CommunicationTCP {
		return ms.startTCP()
	}
	return ms.startUnix()
}

func (ms *Server) startUnix() error {
	ms.socketPath = filepath.Join(os.TempDir(), "tusk-connect.sock")
	_ = os.Remove(ms.socketPath)

	listener, err := net.Listen("unix", ms.socketPath)
	if err != nil {
		return fmt.Errorf("failed to create Unix socket listener: %w", err)
	}

	ms.listener = listener
	slog.Debug("Mock server started with Unix socket", "socket", ms.socketPath)

	// Verify the socket file exists and is accessible
	if _, err := os.Stat(ms.socketPath); err != nil {
		_ = ms.listener.Close()
		return fmt.Errorf("socket file not accessible after creation: %w", err)
	}

	ms.wg.Add(1)
	go ms.acceptConnections()

	// Give the goroutine a moment to start accepting connections
	time.Sleep(100 * time.Millisecond)

	slog.Debug("Mock server ready to accept connections", "socket", ms.socketPath)

	return nil
}

func (ms *Server) startTCP() error {
	addr := fmt.Sprintf("127.0.0.1:%d", ms.tcpPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to create TCP listener: %w", err)
	}

	ms.tcpListener = listener
	ms.listener = listener
	slog.Debug("Mock server started with TCP", "address", addr, "port", ms.tcpPort)

	ms.wg.Add(1)
	go ms.acceptConnections()
	time.Sleep(100 * time.Millisecond)

	return nil
}

// Stop shuts down the mock server
func (ms *Server) Stop() error {
	ms.cancel()

	if ms.listener != nil {
		_ = ms.listener.Close()
	}

	// Clean up socket file only for Unix sockets
	if ms.communicationType == CommunicationUnix {
		_ = os.Remove(ms.socketPath)
	}

	ms.wg.Wait()
	slog.Debug("Mock server stopped")
	return nil
}

func (ms *Server) GetConnectionInfo() (string, int) {
	if ms.communicationType == CommunicationTCP {
		return "", ms.tcpPort
	}
	return ms.socketPath, 0
}

func (ms *Server) GetCommunicationType() CommunicationType {
	return ms.communicationType
}

func (ms *Server) GetSocketPath() string {
	return ms.socketPath
}

func (ms *Server) GetSDKVersion() string {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return ms.sdkVersion
}

func (ms *Server) SetCurrentTestID(id string) {
	ms.currentTestID.Store(id)
}

func (ms *Server) WaitForSDKConnection(timeout time.Duration) error {
	slog.Debug("Waiting for SDK to connect and acknowledge...", "timeout", timeout)

	select {
	case <-ms.sdkConnectedChan:
		slog.Debug("SDK connection acknowledged")
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("timeout waiting for SDK acknowledgement after %v", timeout)
	case <-ms.ctx.Done():
		return fmt.Errorf("server context cancelled while waiting for SDK acknowledgement")
	}
}

// LoadSpansForTrace loads all spans for matching
func (ms *Server) LoadSpansForTrace(traceID string, spans []*core.Span) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	// Initialize usage tracking for this trace
	if ms.spanUsage[traceID] == nil {
		ms.spanUsage[traceID] = make(map[string]bool)
	}

	// Reset usage state for all spans
	for i := range spans {
		ms.spanUsage[traceID][spans[i].SpanId] = false
	}

	ms.spans[traceID] = spans
	ms.matchEvents[traceID] = nil

	// Build package name index
	ms.spansByPackage[traceID] = make(map[string][]*core.Span)
	ms.spansByReducedValueHash[traceID] = make(map[string][]*core.Span)
	ms.spansByValueHash[traceID] = make(map[string][]*core.Span)

	for _, span := range spans {
		// Package index
		pkgName := span.PackageName
		ms.spansByPackage[traceID][pkgName] = append(ms.spansByPackage[traceID][pkgName], span)

		// Value hash index (already computed by SDK)
		if span.InputValueHash != "" {
			ms.spansByValueHash[traceID][span.InputValueHash] = append(ms.spansByValueHash[traceID][span.InputValueHash], span)
		}

		// Reduced value hash index (compute once here)
		reducedHash := reducedInputValueHash(span)
		if reducedHash != "" {
			ms.spansByReducedValueHash[traceID][reducedHash] = append(ms.spansByReducedValueHash[traceID][reducedHash], span)
		}
	}

	// Sort all indexed spans by timestamp (oldest first)
	sortSpansByTimestamp := func(spans []*core.Span) {
		sort.Slice(spans, func(i, j int) bool {
			if spans[i].Timestamp == nil && spans[j].Timestamp == nil {
				return spans[i].SpanId < spans[j].SpanId
			}
			if spans[i].Timestamp == nil {
				return true
			}
			if spans[j].Timestamp == nil {
				return false
			}
			return spans[i].Timestamp.AsTime().Before(spans[j].Timestamp.AsTime())
		})
	}

	for pkg := range ms.spansByPackage[traceID] {
		sortSpansByTimestamp(ms.spansByPackage[traceID][pkg])
	}

	for hash := range ms.spansByValueHash[traceID] {
		sortSpansByTimestamp(ms.spansByValueHash[traceID][hash])
	}

	for hash := range ms.spansByReducedValueHash[traceID] {
		sortSpansByTimestamp(ms.spansByReducedValueHash[traceID][hash])
	}

	slog.Debug("Loaded spans for trace", "traceID", traceID, "count", len(spans))
}

func (ms *Server) SetSuiteSpans(spans []*core.Span) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.suiteSpans = spans

	// Build package name index
	ms.suiteSpansByPackage = make(map[string][]*core.Span)
	ms.suiteSpansByReducedValueHash = make(map[string][]*core.Span)
	ms.suiteSpansByValueHash = make(map[string][]*core.Span)

	for _, span := range spans {
		// Package index
		pkgName := span.PackageName
		ms.suiteSpansByPackage[pkgName] = append(ms.suiteSpansByPackage[pkgName], span)

		// Value hash index (already computed by SDK)
		if span.InputValueHash != "" {
			ms.suiteSpansByValueHash[span.InputValueHash] = append(ms.suiteSpansByValueHash[span.InputValueHash], span)
		}

		// Reduced value hash index (compute once here)
		reducedHash := reducedInputValueHash(span)
		if reducedHash != "" {
			ms.suiteSpansByReducedValueHash[reducedHash] = append(ms.suiteSpansByReducedValueHash[reducedHash], span)
		}
	}
}

func (ms *Server) GetSuiteSpans() []*core.Span {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return ms.suiteSpans
}

func (ms *Server) GetSpansByPackageForTrace(traceID string, packageName string) []*core.Span {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if pkgMap, exists := ms.spansByPackage[traceID]; exists {
		return pkgMap[packageName]
	}
	return nil
}

func (ms *Server) GetSuiteSpansByPackage(packageName string) []*core.Span {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return ms.suiteSpansByPackage[packageName]
}

func (ms *Server) GetSpansByValueHashForTrace(traceID string, valueHash string) []*core.Span {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if hashMap, exists := ms.spansByValueHash[traceID]; exists {
		return hashMap[valueHash]
	}
	return nil
}

func (ms *Server) GetSpansByReducedValueHashForTrace(traceID string, reducedHash string) []*core.Span {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if hashMap, exists := ms.spansByReducedValueHash[traceID]; exists {
		return hashMap[reducedHash]
	}
	return nil
}

func (ms *Server) GetSuiteSpansByValueHash(valueHash string) []*core.Span {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return ms.suiteSpansByValueHash[valueHash]
}

func (ms *Server) GetSuiteSpansByReducedValueHash(reducedHash string) []*core.Span {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return ms.suiteSpansByReducedValueHash[reducedHash]
}

func (ms *Server) CleanupTraceSpans(traceID string) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	delete(ms.spans, traceID)
	delete(ms.spanUsage, traceID)
	delete(ms.matchEvents, traceID)
	delete(ms.mockNotFoundEvents, traceID)
	delete(ms.spansByPackage, traceID)
	delete(ms.spansByValueHash, traceID)
	delete(ms.spansByReducedValueHash, traceID)

	slog.Debug("Cleaned up spans for trace", "traceID", traceID)
}

// acceptConnections handles incoming socket connections
func (ms *Server) acceptConnections() {
	defer ms.wg.Done()

	for {
		select {
		case <-ms.ctx.Done():
			return
		default:
			conn, err := ms.listener.Accept()
			if err != nil {
				if ms.ctx.Err() != nil {
					return // Context cancelled, shutting down
				}
				slog.Error("Failed to accept connection", "error", err)
				continue
			}

			ms.wg.Add(1)
			go ms.handleConnection(conn)
		}
	}
}

// handleConnection processes a single SDK connection
func (ms *Server) handleConnection(conn net.Conn) {
	defer ms.wg.Done()
	defer func() { _ = conn.Close() }()

	for {
		// Read message length (4 bytes)
		lengthBytes := make([]byte, 4)
		if _, err := io.ReadFull(conn, lengthBytes); err != nil {
			if err == io.EOF {
				slog.Debug("SDK connection closed")
				return
			}
			slog.Error("Failed to read message length", "error", err)
			return
		}

		// Parse message length
		messageLength := binary.BigEndian.Uint32(lengthBytes)
		if messageLength > 10*1024*1024 { // 10MB limit
			slog.Warn("Message too large, skipping", "length", messageLength)
			discardBuf := make([]byte, messageLength)
			if _, err := io.ReadFull(conn, discardBuf); err != nil {
				slog.Error("Failed to discard oversized message", "error", err)
				return
			}
			continue // Skip this message but keep connection alive
		}

		// Read message data
		messageData := make([]byte, messageLength)
		if _, err := io.ReadFull(conn, messageData); err != nil {
			slog.Debug("Failed to read message data", "error", err)
			return
		}

		// Parse protobuf message
		var sdkMsg core.SDKMessage
		if err := proto.Unmarshal(messageData, &sdkMsg); err != nil {
			slog.Debug("Failed to parse protobuf message", "error", err)
			continue
		}

		// Handle message based on type
		switch sdkMsg.Type {
		case core.MessageType_MESSAGE_TYPE_SDK_CONNECT:
			ms.handleSDKConnectProtobuf(&sdkMsg, conn)
		case core.MessageType_MESSAGE_TYPE_MOCK_REQUEST:
			// Handle mock requests concurrently to avoid blocking on expensive searches
			ms.wg.Add(1)
			go func(msg *core.SDKMessage) {
				defer ms.wg.Done()
				ms.handleMockRequestProtobuf(msg, conn)
			}(&sdkMsg)
		case core.MessageType_MESSAGE_TYPE_INBOUND_SPAN:
			ms.handleInboundReplaySpanProtobuf(&sdkMsg, conn)
		default:
			slog.Debug("Unknown message type", "type", sdkMsg.Type)
		}
	}
}

// Helper function to send protobuf response
func (ms *Server) sendProtobufResponse(conn net.Conn, msg proto.Message) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	// Check for potential integer overflow before casting to uint32
	dataLen := len(data)
	if dataLen > math.MaxUint32 {
		return fmt.Errorf("message too large: %d bytes exceeds maximum of %d bytes", dataLen, math.MaxUint32)
	}

	// Send length prefix
	lengthBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBytes, uint32(dataLen))

	// Lock to ensure atomic write of length + data
	ms.connWriteMutex.Lock()
	defer ms.connWriteMutex.Unlock()

	if _, err := conn.Write(lengthBytes); err != nil {
		return fmt.Errorf("failed to write length: %w", err)
	}

	// Send message data
	if _, err := conn.Write(data); err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	return nil
}

func isVersionCompatible(actualVersion, minRequiredVersion string) bool {
	actual, err := parseVersion(actualVersion)
	if err != nil {
		slog.Warn("Failed to parse actual version", "version", actualVersion, "error", err)
		return false
	}

	required, err := parseVersion(minRequiredVersion)
	if err != nil {
		slog.Warn("Failed to parse required version", "version", minRequiredVersion, "error", err)
		return false
	}

	// Compare major.minor.patch
	if actual.major > required.major {
		return true
	}
	if actual.major < required.major {
		return false
	}

	// Major versions equal, check minor
	if actual.minor > required.minor {
		return true
	}
	if actual.minor < required.minor {
		return false
	}

	// Major and minor equal, check patch
	return actual.patch >= required.patch
}

type SemVer struct {
	major, minor, patch int
}

func parseVersion(v string) (SemVer, error) {
	// Remove 'v' prefix if present
	v = strings.TrimPrefix(v, "v")

	// Handle special case for "dev"
	if v == "dev" {
		return SemVer{999, 999, 999}, nil // Dev version is considered latest
	}

	// Parse semantic version (major.minor.patch)
	re := regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)`)
	matches := re.FindStringSubmatch(v)
	if len(matches) != 4 {
		return SemVer{}, fmt.Errorf("invalid version format: %s", v)
	}

	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	patch, _ := strconv.Atoi(matches[3])

	return SemVer{major, minor, patch}, nil
}

// handleSDKConnectProtobuf processes SDK connection using protobuf
func (ms *Server) handleSDKConnectProtobuf(msg *core.SDKMessage, conn net.Conn) {
	connectReq := msg.GetConnectRequest()
	if connectReq == nil {
		logging.LogToService("Invalid connect request from SDK- no payload")
		response := &core.CLIMessage{
			Type:      core.MessageType_MESSAGE_TYPE_SDK_CONNECT,
			RequestId: msg.RequestId,
			Payload: &core.CLIMessage_ConnectResponse{
				ConnectResponse: &core.ConnectResponse{
					Success: false,
					Error:   "Invalid connect request format",
				},
			},
		}
		err := ms.sendProtobufResponse(conn, response)
		if err != nil {
			logging.LogToService(fmt.Sprintf("Failed to send connect response: %v", err))
		}
		return
	}

	cliVersion := version.Version

	// Check CLI <> SDK version compatibility. If not compatible, close the server. `WaitForSDKConnection` will return an error.

	// Check if CLI version meets SDK's minimum requirement
	if connectReq.MinCliVersion != "" && !isVersionCompatible(cliVersion, connectReq.MinCliVersion) {
		logging.LogToService(fmt.Sprintf("CLI version %s is incompatible. SDK (%s) requires CLI version %s or higher", cliVersion, connectReq.SdkVersion, connectReq.MinCliVersion))

		response := &core.CLIMessage{
			Type:      core.MessageType_MESSAGE_TYPE_SDK_CONNECT,
			RequestId: msg.RequestId,
			Payload: &core.CLIMessage_ConnectResponse{
				ConnectResponse: &core.ConnectResponse{
					Success: false,
					Error:   fmt.Sprintf("CLI version %s is incompatible. SDK requires CLI version %s or higher", cliVersion, connectReq.MinCliVersion),
				},
			},
		}
		err := ms.sendProtobufResponse(conn, response)
		if err != nil {
			logging.LogToService(fmt.Sprintf("Failed to send connect response: %v", err))
		}

		go func() {
			time.Sleep(100 * time.Millisecond)
			ms.cancel()
		}()
		return
	}

	// Check if SDK version meets CLI's minimum requirement
	if connectReq.SdkVersion != "" && !isVersionCompatible(connectReq.SdkVersion, version.MinSDKVersion) {
		logging.LogToService(fmt.Sprintf("SDK version %s is incompatible. CLI (%s) requires SDK version %s or higher", connectReq.SdkVersion, cliVersion, version.MinSDKVersion))

		response := &core.CLIMessage{
			Type:      core.MessageType_MESSAGE_TYPE_SDK_CONNECT,
			RequestId: msg.RequestId,
			Payload: &core.CLIMessage_ConnectResponse{
				ConnectResponse: &core.ConnectResponse{
					Success: false,
					Error:   fmt.Sprintf("SDK version %s is incompatible. CLI requires SDK version %s or higher", connectReq.SdkVersion, version.MinSDKVersion),
				},
			},
		}
		err := ms.sendProtobufResponse(conn, response)
		if err != nil {
			logging.LogToService(fmt.Sprintf("Failed to send connect response: %v", err))
		}

		go func() {
			time.Sleep(100 * time.Millisecond)
			ms.cancel()
		}()
		return
	}

	logging.LogToService("SDK connected:")
	logging.LogToService(fmt.Sprintf("  - Service ID: %s", connectReq.ServiceId))
	logging.LogToService(fmt.Sprintf("  - SDK version: %s", connectReq.SdkVersion))
	logging.LogToService(fmt.Sprintf("  - CLI version: %s", cliVersion))
	logging.LogToService(fmt.Sprintf("  - Min CLI version: %s", connectReq.MinCliVersion))

	ms.mu.Lock()
	ms.sdkVersion = connectReq.SdkVersion
	if !ms.sdkConnected {
		ms.sdkConnected = true
		close(ms.sdkConnectedChan)
	}
	ms.mu.Unlock()

	response := &core.CLIMessage{
		Type:      core.MessageType_MESSAGE_TYPE_SDK_CONNECT,
		RequestId: msg.RequestId,
		Payload: &core.CLIMessage_ConnectResponse{
			ConnectResponse: &core.ConnectResponse{
				Success: true,
			},
		},
	}

	if err := ms.sendProtobufResponse(conn, response); err != nil {
		slog.Error("Failed to send connect response", "error", err)
	}
}

// handleMockRequestProtobuf processes mock requests using protobuf
func (ms *Server) handleMockRequestProtobuf(msg *core.SDKMessage, conn net.Conn) {
	startTime := time.Now()

	mockReq := msg.GetGetMockRequest()
	if mockReq == nil {
		slog.Error("Invalid mock request - no payload")
		return
	}

	testID := mockReq.TestId
	if testID == "" {
		if stored := ms.currentTestID.Load(); stored != nil {
			testID = stored.(string)
		}
	}

	responseChan := make(chan *core.GetMockResponse, 1)
	go func() {
		response := ms.findMock(mockReq)
		response.RequestId = msg.RequestId
		responseChan <- response
	}()

	var response *core.GetMockResponse
	select {
	case response = <-responseChan:
		// Success
	case <-time.After(15 * time.Second):
		slog.Error("Mock request timeout",
			"requestId", msg.RequestId,
			"testId", testID,
			"package", mockReq.OutboundSpan.PackageName,
			"duration", time.Since(startTime))

		response = &core.GetMockResponse{
			RequestId: msg.RequestId,
			Found:     false,
			Error:     fmt.Sprintf("mock search timed out after 15s for %s", mockReq.OutboundSpan.PackageName),
		}
	}

	cliMsg := &core.CLIMessage{
		Type:      core.MessageType_MESSAGE_TYPE_MOCK_REQUEST,
		RequestId: msg.RequestId,
		Payload: &core.CLIMessage_GetMockResponse{
			GetMockResponse: response,
		},
	}

	if err := ms.sendProtobufResponse(conn, cliMsg); err != nil {
		slog.Debug("Failed to send mock response", "error", err)
	}
}

func (ms *Server) handleInboundReplaySpanProtobuf(msg *core.SDKMessage, conn net.Conn) {
	req := msg.GetSendInboundSpanForReplayRequest()
	if req == nil || req.Span == nil {
		slog.Error("Invalid inbound span request")
		return
	}

	slog.Debug("Received inbound span for replay", "traceID", req.Span.TraceId, "spanID", req.Span.SpanId)

	span := req.Span

	ms.mu.Lock()
	if ms.replayInbound == nil {
		ms.replayInbound = make(map[string]*core.Span)
	}
	ms.replayInbound[span.TraceId] = span
	ms.mu.Unlock()

	_ = ms.sendProtobufResponse(conn, &core.CLIMessage{
		Type:      core.MessageType_MESSAGE_TYPE_INBOUND_SPAN,
		RequestId: msg.RequestId,
		Payload: &core.CLIMessage_SendInboundSpanForReplayResponse{
			SendInboundSpanForReplayResponse: &core.SendInboundSpanForReplayResponse{Success: true},
		},
	})
}

// findMock searches for a matching mock for the given request
func (ms *Server) findMock(req *core.GetMockRequest) *core.GetMockResponse {
	testID := req.TestId
	if testID == "" {
		if stored := ms.currentTestID.Load(); stored != nil {
			testID = stored.(string)
		}
	}

	matcher := NewMockMatcher(ms)
	var span *core.Span
	var matchLevel *backend.MatchLevel
	var err error
	scope := scopeUnknown

	// If we have a test ID, try to find mock in the trace first
	if testID != "" {
		// Check if spans are loaded for this trace, if not, try to load them
		ms.mu.RLock()
		_, spansLoaded := ms.spans[testID]
		ms.mu.RUnlock()

		if !spansLoaded {
			slog.Debug("Spans not loaded for trace, attempting to load", "traceID", testID)
			if err := ms.loadSpansForTraceID(testID); err != nil {
				slog.Debug("Failed to load spans for trace", "traceID", testID, "error", err)
				return &core.GetMockResponse{
					Found: false,
					Error: fmt.Sprintf("failed to load spans for trace %s: %v", testID, err),
				}
			}
		}

		span, matchLevel, err = matcher.FindBestMatchInTrace(req, testID)
		if err == nil {
			scope = scopeTrace
		}
	}

	// If no match found in trace (or no testID), try global fallback
	if span == nil {
		if testID == "" && req.OutboundSpan != nil && !req.OutboundSpan.IsPreAppStart {
			slog.Debug("No test ID and not pre-app-start; skipping suite span search",
				"package", req.OutboundSpan.PackageName,
				"operation", req.Operation)

			return &core.GetMockResponse{
				Found: false,
				Error: fmt.Sprintf("no mock found for background query %s %s (no testID)",
					req.Operation, req.OutboundSpan.Name),
			}
		}

		if testID != "" {
			slog.Debug("No mock found in current trace; attempting global fallback",
				"testID", testID, "package", req.OutboundSpan.PackageName, "operation", req.Operation, "error", err)
		} else {
			slog.Debug("No test ID provided; searching global mocks",
				"package", req.OutboundSpan.PackageName, "operation", req.Operation)
		}

		candidates := ms.GetSuiteSpans()
		if len(candidates) > 0 {
			if globalSpan, globalMatchLevel, globalErr := matcher.FindBestMatchAcrossTraces(req, testID, candidates); globalErr == nil && globalSpan != nil {
				slog.Debug("Found suite mock match",
					"testID", testID,
					"spanName", globalSpan.Name,
					"spanID", globalSpan.SpanId,
					"fromTrace", globalSpan.TraceId,
					"preAppStart", globalSpan.IsPreAppStart,
				)
				span = globalSpan
				matchLevel = globalMatchLevel
				scope = scopeGlobal
			}
		}

		if span == nil {
			slog.Debug("No mock found",
				"testID", testID,
				"packageName", req.OutboundSpan.PackageName,
				"operation", req.Operation,
				"error", err)

			if testID != "" {
				logging.LogToCurrentTest(testID, "🔴 No mock found for request\n")
				// Record that a mock was not found for this test
				ms.recordMockNotFoundEvent(testID, MockNotFoundEvent{
					PackageName: req.OutboundSpan.PackageName,
					SpanName:    req.OutboundSpan.Name,
					Operation:   req.Operation,
					StackTrace:  req.StackTrace,
					Timestamp:   time.Now(),
					Error:       fmt.Sprintf("no mock found for %s %s: %v", req.Operation, req.OutboundSpan.Name, err),
					ReplaySpan:  req.OutboundSpan,
				})
			}

			return &core.GetMockResponse{
				Found: false,
				Error: fmt.Sprintf("no mock found for %s %s: %v", req.Operation, req.OutboundSpan.Name, err),
			}
		}
	}

	switch scope {
	case scopeTrace:
		if testID != "" {
			logging.LogToCurrentTest(testID, "🟢 Found best match for request in trace\n")
		}
	case scopeGlobal:
		if testID != "" {
			msg := "🟢 Found best match for request across traces\n"
			if span != nil && span.IsPreAppStart {
				msg = "🟢 Found best match for request across traces (pre-app-start)\n"
			}
			logging.LogToCurrentTest(testID, msg)
		}
	default:
		slog.Debug("Unknown match scope", "scope", scope)
	}

	// Record match event metadata for results output
	var inputMap map[string]any
	if req.OutboundSpan.InputValue != nil {
		inputMap = req.OutboundSpan.InputValue.AsMap()
	}
	timestamp := time.Now()
	if span.Timestamp != nil {
		timestamp = span.Timestamp.AsTime()
	}
	ms.recordMatchEvent(testID, MatchEvent{
		SpanID:     span.SpanId,
		MatchLevel: matchLevel,
		StackTrace: req.StackTrace,
		InputData:  inputMap,
		Timestamp:  timestamp,
		ReplaySpan: req.OutboundSpan,
	})

	// Convert span to mock response
	mockInteraction := ms.spanToMockInteraction(span)

	// Convert to JSON and back to map[string]any for protobuf compatibility
	mockBytes, err := json.Marshal(mockInteraction)
	if err != nil {
		slog.Error("Failed to marshal mock interaction", "error", err)
		return &core.GetMockResponse{
			Found: false,
			Error: "failed to serialize mock response",
		}
	}

	var mockInteractionMap map[string]any
	if err := json.Unmarshal(mockBytes, &mockInteractionMap); err != nil {
		slog.Error("Failed to unmarshal mock interaction", "error", err)
		return &core.GetMockResponse{
			Found: false,
			Error: "failed to serialize mock response",
		}
	}

	responseData, err := structpb.NewStruct(map[string]any{
		"response": mockInteractionMap,
	})
	if err != nil {
		slog.Error("Failed to convert mock interaction to struct", "error", err)
		return &core.GetMockResponse{
			Found: false,
			Error: "failed to serialize mock response",
		}
	}

	slog.Debug("Found mock match",
		"testID", testID,
		"spanName", span.Name,
		"spanID", span.SpanId)

	return &core.GetMockResponse{
		Found:        true,
		ResponseData: responseData,
	}
}

// Helper to convert Span to MockInteraction
func (ms *Server) spanToMockInteraction(span *core.Span) api.MockInteraction {
	// Extract request data from span's input
	request := api.RecordedRequest{
		Method: span.SubmoduleName,
	}

	// Try to extract more specific request data from InputValue
	if span.InputValue != nil {
		inputMap := span.InputValue.AsMap()
		if method, exists := inputMap["method"]; exists {
			if methodStr, ok := method.(string); ok {
				request.Method = methodStr
			}
		}
		if target, exists := inputMap["target"]; exists {
			if targetStr, ok := target.(string); ok {
				request.Path = targetStr
			}
		}
		if headers, exists := inputMap["headers"]; exists {
			if headersMap, ok := headers.(map[string]any); ok {
				request.Headers = make(map[string][]string)
				for k, v := range headersMap {
					if vStr, ok := v.(string); ok {
						request.Headers[k] = []string{vStr}
					}
				}
			}
		}
		request.Body = inputMap
	}

	// Extract response data from span's output
	response := api.RecordedResponse{
		Status: 200, // Default
	}

	if span.OutputValue != nil {
		outputMap := span.OutputValue.AsMap()
		if statusCode, exists := outputMap["statusCode"]; exists {
			if statusInt, ok := statusCode.(float64); ok {
				response.Status = int(statusInt)
			}
		}
		if headers, exists := outputMap["headers"]; exists {
			if headersMap, ok := headers.(map[string]any); ok {
				response.Headers = make(map[string][]string)
				for k, v := range headersMap {
					if vStr, ok := v.(string); ok {
						response.Headers[k] = []string{vStr}
					}
				}
			}
		}
		response.Body = outputMap
	}

	var timestamp time.Time
	if span.Timestamp != nil {
		timestamp = span.Timestamp.AsTime()
	}

	return api.MockInteraction{
		Service:   span.PackageName,
		Request:   request,
		Response:  response,
		Order:     1, // Could be derived from timestamp if needed
		Timestamp: timestamp,
	}
}

// loadSpansForTraceID attempts to load spans for a given trace ID from disk
func (ms *Server) loadSpansForTraceID(traceID string) error {
	// Scan for trace files that contain this trace ID
	for _, dir := range utils.GetPossibleTraceDirs() {
		matches, err := filepath.Glob(filepath.Join(dir, "*trace*"+traceID+"*.jsonl"))
		if err != nil {
			continue
		}

		for _, traceFile := range matches {
			filter := func(span *core.Span) bool {
				return span.TraceId == traceID
			}
			spans, err := utils.ParseSpansFromFile(traceFile, filter)
			if err != nil {
				slog.Warn("Failed to load spans from file", "file", traceFile, "error", err)
				continue
			}

			if len(spans) > 0 {
				ms.LoadSpansForTrace(traceID, spans)
				slog.Info("Successfully loaded spans for trace", "traceID", traceID, "count", len(spans), "file", traceFile)
				return nil
			}
		}
	}

	return fmt.Errorf("no trace file found for trace ID %s", traceID)
}

func (ms *Server) recordMatchEvent(traceID string, ev MatchEvent) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.matchEvents[traceID] = append(ms.matchEvents[traceID], ev)
}

func (ms *Server) GetMatchEvents(traceID string) []MatchEvent {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	events := ms.matchEvents[traceID]
	out := make([]MatchEvent, len(events))
	copy(out, events)
	return out
}

func (ms *Server) GetInboundReplaySpan(traceID string) *core.Span {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return ms.replayInbound[traceID]
}

func (ms *Server) recordMockNotFoundEvent(traceID string, ev MockNotFoundEvent) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if ms.mockNotFoundEvents == nil {
		ms.mockNotFoundEvents = make(map[string][]MockNotFoundEvent)
	}
	ms.mockNotFoundEvents[traceID] = append(ms.mockNotFoundEvents[traceID], ev)
}

func (ms *Server) GetMockNotFoundEvents(traceID string) []MockNotFoundEvent {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	events := ms.mockNotFoundEvents[traceID]
	out := make([]MockNotFoundEvent, len(events))
	copy(out, events)
	return out
}

func (ms *Server) HasMockNotFoundEvents(traceID string) bool {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return len(ms.mockNotFoundEvents[traceID]) > 0
}

func (ms *Server) GetRootSpanID(traceID string) string {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	for _, s := range ms.spans[traceID] {
		if s.IsRootSpan {
			return s.SpanId
		}
	}
	return ""
}

func (ms *Server) WaitForInboundSpan(traceID string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ms.mu.RLock()
		hasInbound := ms.replayInbound[traceID] != nil
		ms.mu.RUnlock()
		if hasInbound {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (ms *Server) WaitForSpanData(traceID string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ms.mu.RLock()
		hasInbound := ms.replayInbound[traceID] != nil
		hasEvents := len(ms.matchEvents[traceID]) > 0
		ms.mu.RUnlock()
		if hasInbound || hasEvents {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}
