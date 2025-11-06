package runner

import (
	"testing"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestIsVersionCompatible(t *testing.T) {
	tcs := []struct {
		name     string
		actual   string
		required string
		want     bool
	}{
		{"equal", "1.2.3", "1.2.3", true},
		{"patchHigher", "1.2.4", "1.2.3", true},
		{"minorHigher", "1.3.0", "1.2.9", true},
		{"majorHigher", "2.0.0", "1.9.9", true},
		{"devWins", "dev", "9.9.9", true},
		{"patchLower", "1.2.3", "1.2.4", false},
		{"minorLower", "1.2.3", "1.3.0", false},
		{"majorLower", "1.9.9", "2.0.0", false},
		{"actualInvalid", "foo", "1.0.0", false},
		{"requiredInvalid", "1.2.3", "bar", false},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			got := isVersionCompatible(tc.actual, tc.required)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseVersion(t *testing.T) {
	tcs := []struct {
		name  string
		input string
		want  SemVer
	}{
		{"plain", "1.2.3", SemVer{1, 2, 3}},
		{"prefixed", "v2.0.1", SemVer{2, 0, 1}},
		{"withSuffix", "1.2.3-beta", SemVer{1, 2, 3}},
		{"dev", "dev", SemVer{999, 999, 999}},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseVersion(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseVersionInvalid(t *testing.T) {
	_, err := parseVersion("1.2")
	require.Error(t, err)
}

func TestServerTCPMode(t *testing.T) {
	config.Invalidate()

	testServiceConfig := &config.ServiceConfig{
		ID:   "test-tcp-service",
		Port: 3000,
		Start: config.StartConfig{
			Command: "docker compose up",
		},
		Communication: config.CommunicationConfig{
			Type:    "tcp",
			TCPPort: 9002,
		},
	}

	server, err := NewServer("test-tcp-service", testServiceConfig)
	require.NoError(t, err)
	defer func() { _ = server.Stop() }()

	// Verify it's TCP mode
	assert.Equal(t, CommunicationTCP, server.GetCommunicationType())

	// Start the server
	err = server.Start()
	require.NoError(t, err)

	// Verify connection info
	socketPath, tcpPort := server.GetConnectionInfo()
	assert.Empty(t, socketPath, "TCP mode should have empty socket path")
	assert.Equal(t, 9002, tcpPort)
}

func TestServerUnixMode(t *testing.T) {
	config.Invalidate()

	testServiceConfig := &config.ServiceConfig{
		ID:   "test-unix-service",
		Port: 3000,
		Start: config.StartConfig{
			Command: "npm run dev",
		},
		Communication: config.CommunicationConfig{
			Type:    "unix",
			TCPPort: 9001,
		},
	}

	server, err := NewServer("test-unix-service", testServiceConfig)
	require.NoError(t, err)
	defer func() { _ = server.Stop() }()

	// Verify it's Unix mode
	assert.Equal(t, CommunicationUnix, server.GetCommunicationType())

	// Start the server
	err = server.Start()
	require.NoError(t, err)

	// Verify connection info
	socketPath, tcpPort := server.GetConnectionInfo()
	assert.NotEmpty(t, socketPath, "Unix mode should have socket path")
	assert.Equal(t, 0, tcpPort, "Unix mode should have zero TCP port")
	assert.Contains(t, socketPath, "tusk-connect.sock")
}

func TestDetermineCommunicationType(t *testing.T) {
	tests := []struct {
		name       string
		startCmd   string
		configType string
		expected   CommunicationType
	}{
		{
			name:       "auto_detects_docker_compose",
			startCmd:   "docker compose up",
			configType: "auto",
			expected:   CommunicationTCP,
		},
		{
			name:       "auto_detects_docker_run",
			startCmd:   "docker run my-image",
			configType: "auto",
			expected:   CommunicationTCP,
		},
		{
			name:       "auto_detects_docker-compose",
			startCmd:   "docker-compose up -d",
			configType: "auto",
			expected:   CommunicationTCP,
		},
		{
			name:       "auto_defaults_to_unix_for_npm",
			startCmd:   "npm run start",
			configType: "auto",
			expected:   CommunicationUnix,
		},
		{
			name:       "auto_defaults_to_unix_for_node",
			startCmd:   "node server.js",
			configType: "auto",
			expected:   CommunicationUnix,
		},
		{
			name:       "explicit_tcp_overrides",
			startCmd:   "npm run start",
			configType: "tcp",
			expected:   CommunicationTCP,
		},
		{
			name:       "explicit_unix_overrides",
			startCmd:   "docker compose up",
			configType: "unix",
			expected:   CommunicationUnix,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.ServiceConfig{
				Start: config.StartConfig{
					Command: tt.startCmd,
				},
				Communication: config.CommunicationConfig{
					Type: tt.configType,
				},
			}

			result := determineCommunicationType(cfg)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsDockerCommand(t *testing.T) {
	tests := []struct {
		command  string
		expected bool
	}{
		// Docker commands
		{"docker", true},
		{"docker-compose", true},
		{"docker compose up", true},
		{"docker-compose up", true},
		{"docker run myimage", true},
		{"ENV=test docker compose up", true},

		// Non-Docker commands
		{"npm run start", false},
		{"node server.js", false},
		{"python app.py", false},

		// This is likely a docker-related script, but we don't make further assumptions.
		// Users can explicitly set the communication type in the config.
		{"./start-docker.sh", false},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			result := isDockerCommand(tt.command)
			assert.Equal(t, tt.expected, result, "Command: %s", tt.command)
		})
	}
}

func TestSpanToMockInteractionPopulatesRequestAndResponse(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.Stop() })

	inputValue, err := structpb.NewStruct(map[string]any{
		"method": "POST",
		"target": "/api/items",
		"headers": map[string]any{
			"X-Test": "value",
		},
		"body": map[string]any{
			"foo": "bar",
		},
	})
	require.NoError(t, err)

	outputValue, err := structpb.NewStruct(map[string]any{
		"statusCode": float64(201),
		"headers": map[string]any{
			"Content-Type": "application/json",
		},
		"body": map[string]any{
			"result": "ok",
		},
	})
	require.NoError(t, err)

	ts := timestamppb.New(time.Unix(1_730_000_000, 0))
	span := &core.Span{
		PackageName:   "http",
		SubmoduleName: "ListItems",
		InputValue:    inputValue,
		OutputValue:   outputValue,
		Timestamp:     ts,
	}

	mock := server.spanToMockInteraction(span)

	assert.Equal(t, "http", mock.Service)
	assert.Equal(t, 1, mock.Order)
	assert.True(t, mock.Timestamp.Equal(ts.AsTime()))

	assert.Equal(t, "/api/items", mock.Request.Path)
	assert.Equal(t, []string{"value"}, mock.Request.Headers["X-Test"])
	body, ok := mock.Request.Body.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "POST", body["method"])
	headersAny, ok := body["headers"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "value", headersAny["X-Test"])

	assert.Equal(t, 201, mock.Response.Status)
	assert.Equal(t, []string{"application/json"}, mock.Response.Headers["Content-Type"])
	respBody, ok := mock.Response.Body.(map[string]any)
	require.True(t, ok)
	nestedBody, ok := respBody["body"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "ok", nestedBody["result"])
}

func TestSpanToMockInteractionFallbacksWhenValuesMissing(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.Stop() })

	span := &core.Span{
		PackageName:   "service",
		SubmoduleName: "FallbackMethod",
	}

	mock := server.spanToMockInteraction(span)

	assert.Equal(t, "service", mock.Service)
	assert.Equal(t, "FallbackMethod", mock.Request.Method)
	assert.Empty(t, mock.Request.Path)
	assert.Nil(t, mock.Request.Headers)
	assert.Nil(t, mock.Request.Body)

	assert.Equal(t, 200, mock.Response.Status)
	assert.Nil(t, mock.Response.Headers)
	assert.Nil(t, mock.Response.Body)

	assert.True(t, mock.Timestamp.IsZero())
}

func TestRecordMatchEventReturnsCopy(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.Stop() })

	traceID := "trace-match"
	server.recordMatchEvent(traceID, MatchEvent{SpanID: "span-1"})

	events := server.GetMatchEvents(traceID)
	require.Len(t, events, 1)
	events[0].SpanID = "modified"
	events = append(events, MatchEvent{SpanID: "span-2"})
	require.Len(t, events, 2)
	assert.Equal(t, "span-2", events[1].SpanID)

	fresh := server.GetMatchEvents(traceID)
	require.Len(t, fresh, 1)
	assert.Equal(t, "span-1", fresh[0].SpanID)
}

func TestWaitForSpanDataReturnsOnceDataAvailable(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.Stop() })

	traceID := "trace-wait"
	done := make(chan struct{})

	go func() {
		time.Sleep(25 * time.Millisecond)
		server.recordMatchEvent(traceID, MatchEvent{SpanID: "span"})
		close(done)
	}()

	start := time.Now()
	server.WaitForSpanData(traceID, 500*time.Millisecond)
	duration := time.Since(start)

	if duration >= 500*time.Millisecond {
		t.Fatalf("expected WaitForSpanData to return before deadline, took %v", duration)
	}

	<-done
	events := server.GetMatchEvents(traceID)
	require.Len(t, events, 1)
	assert.Equal(t, "span", events[0].SpanID)
}

func TestWaitForSpanDataTimesOut(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.Stop() })

	traceID := "trace-timeout"
	timeout := 70 * time.Millisecond

	start := time.Now()
	server.WaitForSpanData(traceID, timeout)
	duration := time.Since(start)

	if duration < timeout {
		t.Fatalf("wait returned too early: %v (timeout %v)", duration, timeout)
	}

	assert.Empty(t, server.GetMatchEvents(traceID))
}

func TestWaitForSDKConnectionSignals(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.Stop() })

	go func() {
		time.Sleep(25 * time.Millisecond)
		server.mu.Lock()
		if !server.sdkConnected {
			server.sdkConnected = true
			close(server.sdkConnectedChan)
		}
		server.mu.Unlock()
	}()

	err = server.WaitForSDKConnection(300 * time.Millisecond)
	require.NoError(t, err)
}

func TestWaitForSDKConnectionTimeout(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.Stop() })

	timeout := 50 * time.Millisecond
	start := time.Now()
	err = server.WaitForSDKConnection(timeout)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")

	if elapsed := time.Since(start); elapsed < timeout {
		t.Fatalf("wait returned too early: %v (timeout %v)", elapsed, timeout)
	}
}

func TestWaitForSDKConnectionContextCancelled(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.Stop() })

	server.cancel()

	err = server.WaitForSDKConnection(200 * time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context cancelled")
}

func TestMockNotFoundEvents(t *testing.T) {
	t.Parallel()

	cfg, _ := config.Get()
	server, err := NewServer("test-service", &cfg.Service)
	require.NoError(t, err)
	defer func() { _ = server.Stop() }()

	traceID := "test-trace-1"

	// Initially no events
	assert.False(t, server.HasMockNotFoundEvents(traceID))
	assert.Empty(t, server.GetMockNotFoundEvents(traceID))

	// Record an event
	event1 := MockNotFoundEvent{
		PackageName: "pg",
		SpanName:    "pg.query",
		Operation:   "query",
		StackTrace:  "at test.ts:10",
		Timestamp:   time.Now(),
		Error:       "no mock found",
	}
	server.recordMockNotFoundEvent(traceID, event1)

	// Should now have events
	assert.True(t, server.HasMockNotFoundEvents(traceID))
	events := server.GetMockNotFoundEvents(traceID)
	require.Len(t, events, 1)
	assert.Equal(t, "pg", events[0].PackageName)
	assert.Equal(t, "pg.query", events[0].SpanName)

	// Record another
	event2 := MockNotFoundEvent{
		PackageName: "http",
		SpanName:    "GET /api/users",
		Operation:   "GET",
		StackTrace:  "at test.ts:20",
		Timestamp:   time.Now(),
		Error:       "no mock found",
	}
	server.recordMockNotFoundEvent(traceID, event2)

	// Should have 2 events
	events = server.GetMockNotFoundEvents(traceID)
	require.Len(t, events, 2)

	// Different trace should have no events
	assert.False(t, server.HasMockNotFoundEvents("other-trace"))
}
