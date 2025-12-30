package runner

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
	"github.com/Use-Tusk/tusk-drift-cli/internal/logging"
)

// StartEnvironment starts the mock server and service, then waits for the SDK ack.
// It performs best-effort cleanup on failure.
func (e *Executor) StartEnvironment() error {
	logging.LogToService("Starting mock server...")
	if err := e.StartServer(); err != nil {
		logging.LogToService(fmt.Sprintf("❌ Failed to start mock server: %v", err))
		return fmt.Errorf("start mock server: %w", err)
	}
	logging.LogToService("✅ Mock server started")

	logging.LogToService("Starting service...")
	if err := e.StartService(); err != nil {
		_ = e.StopServer()
		return fmt.Errorf("start service: %w", err)
	}
	logging.LogToService("✅ Service started")

	logging.LogToService("Waiting for SDK acknowledgement...")
	if err := e.WaitForSDKAcknowledgement(); err != nil {
		logging.LogToService(fmt.Sprintf("❌ Failed to get SDK acknowledgement: %v", err))
		_ = e.StopService()
		_ = e.StopServer()
		return fmt.Errorf("sdk acknowledgement: %w", err)
	}
	logging.LogToService("✅ SDK acknowledged")

	slog.Debug("Environment is ready")
	return nil
}

// StopEnvironment stops the service and mock server (best effort).
func (e *Executor) StopEnvironment() error {
	var firstErr error
	if err := e.StopService(); err != nil {
		firstErr = err
	}
	if err := e.StopServer(); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

// forceStopEnvironment aggressively stops the service and mock server
// Used when the server has crashed and we need to ensure clean slate
func (e *Executor) ForceStopEnvironment() error {
	slog.Debug("Force stopping environment")

	// Force kill the service if it's running
	if e.serviceCmd != nil && e.serviceCmd.Process != nil {
		if err := e.serviceCmd.Process.Kill(); err != nil {
			slog.Debug("Failed to kill service process", "error", err)
		}
		e.serviceCmd = nil
	}

	// Stop the server
	if err := e.StopServer(); err != nil {
		slog.Debug("Failed to stop server during force stop", "error", err)
	}

	// Close service log file if open
	if e.serviceLogFile != nil {
		_ = e.serviceLogFile.Close()
		e.serviceLogFile = nil
	}

	return nil
}

// StartServer initializes and starts the mock server
func (e *Executor) StartServer() error {
	if err := config.Load(""); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	cfg, err := config.Get()
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	server, err := NewServer(cfg.Service.ID, &cfg.Service)
	if err != nil {
		return fmt.Errorf("failed to create mock server: %w", err)
	}

	// Check if TCP port is available before starting
	if server.GetCommunicationType() == CommunicationTCP {
		_, tcpPort := server.GetConnectionInfo()
		if portInUse, err := checkTCPPortAvailable(tcpPort); err == nil && portInUse {
			return fmt.Errorf("TCP mock port %d is already in use. Please choose a different port in config.yaml (communication.tcp_port)", tcpPort)
		}
	}

	if err := server.Start(); err != nil {
		return fmt.Errorf("failed to start mock server: %w", err)
	}

	e.server = server

	// Apply suite spans immediately so pre-app-start mocks work
	if len(e.suiteSpans) > 0 {
		server.SetSuiteSpans(e.suiteSpans)
	}

	// Apply global spans for cross-trace matching
	if len(e.globalSpans) > 0 {
		server.SetGlobalSpans(e.globalSpans)
	}

	// Apply suite-wide matching setting (for local runs or validation mode)
	if e.allowSuiteWideMatching {
		server.SetAllowSuiteWideMatching(true)
	}

	if server.GetCommunicationType() == CommunicationTCP {
		_, port := server.GetConnectionInfo()
		slog.Debug("Mock server ready", "type", "TCP", "port", port)
	} else {
		socketPath, _ := server.GetConnectionInfo()
		slog.Debug("Mock server ready", "type", "Unix", "socket", socketPath)
	}

	return nil
}

func (e *Executor) StopServer() error {
	if e.server != nil {
		return e.server.Stop()
	}
	return nil
}

// WaitForSDKAcknowledgement waits for the SDK to acknowledge the connection.
func (e *Executor) WaitForSDKAcknowledgement() error {
	if e.server == nil {
		return fmt.Errorf("mock server not started")
	}

	timeout := 10 * time.Second
	// Allow tests to override the default wait time
	if testWait := os.Getenv("TUSK_TEST_DEFAULT_WAIT"); testWait != "" {
		if parsed, err := time.ParseDuration(testWait); err == nil {
			timeout = parsed
		}
	}

	slog.Debug(fmt.Sprintf("Waiting for SDK acknowledgement from the service (timeout: %v)...", timeout))
	err := e.server.WaitForSDKConnection(timeout)
	if err != nil {
		return err
	}
	return nil
}

// Restart constants
const (
	MaxServerRestartAttempts = 1
	RestartBackoffBase       = 2 * time.Second
)

// RestartServerWithRetry attempts to restart the server with exponential backoff
func (e *Executor) RestartServerWithRetry(attempt int) error {
	if attempt >= MaxServerRestartAttempts {
		return fmt.Errorf("exceeded maximum restart attempts (%d)", MaxServerRestartAttempts)
	}

	attemptNum := attempt + 1
	logging.LogToService(fmt.Sprintf("🔄 Attempting to restart server (attempt %d/%d)...", attemptNum, MaxServerRestartAttempts))

	// 1. Force stop existing server/service
	if err := e.ForceStopEnvironment(); err != nil {
		slog.Warn("Force stop failed during restart", "error", err)
	}

	// 2. Wait with exponential backoff
	shift := attempt
	if shift > 10 {
		shift = 10
	}
	backoff := RestartBackoffBase * time.Duration(1<<shift)
	slog.Debug("Waiting before restart", "backoff", backoff, "attempt", attemptNum)
	time.Sleep(backoff)

	// 3. Restart environment
	if err := e.StartEnvironment(); err != nil {
		logging.LogToService(fmt.Sprintf("❌ Restart attempt %d failed: %v", attemptNum, err))
		// Try again with next attempt
		if attempt+1 < MaxServerRestartAttempts {
			return e.RestartServerWithRetry(attempt + 1)
		}
		return fmt.Errorf("restart attempt %d failed: %w", attemptNum, err)
	}

	logging.LogToService(fmt.Sprintf("✅ Server restarted successfully (attempt %d)", attemptNum))
	return nil
}

func checkTCPPortAvailable(port int) (bool, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		// Port is in use
		return true, nil
	}
	_ = ln.Close()
	return false, nil
}
