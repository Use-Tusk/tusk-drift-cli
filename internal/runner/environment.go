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
		logging.LogToService(fmt.Sprintf("❌ Failed to start service: %v", err))
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
