package runner

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/Use-Tusk/tusk-cli/internal/config"
	"github.com/Use-Tusk/tusk-cli/internal/log"
)

// StartEnvironment starts the mock server and service, then waits for the SDK ack.
// It performs best-effort cleanup on failure.
func (e *Executor) StartEnvironment() error {
	log.ServiceLog("Starting mock server...")
	if err := e.StartServer(); err != nil {
		log.ServiceLog(fmt.Sprintf("❌ Failed to start mock server: %v", err))
		return fmt.Errorf("start mock server: %w", err)
	}
	log.ServiceLog("✅ Mock server started")

	log.ServiceLog("Starting service...")
	if err := e.StartService(); err != nil {
		if e.GetEffectiveSandboxMode() == SandboxModeAuto && e.lastServiceSandboxed {
			log.ServiceLog("⚠️  Service failed to start in sandbox; retrying once without sandbox...")
			_ = e.StopService()

			// Write separator so the user can see where the retry begins.
			// The in-memory buffer survives StopService; the file path persists
			// via serviceLogPath and setupServiceLogging will reopen in append mode.
			if e.enableServiceLogs && e.serviceLogPath != "" {
				if f, err := os.OpenFile(e.serviceLogPath, os.O_APPEND|os.O_WRONLY, 0o600); err == nil { // #nosec G304
					_, _ = f.WriteString("⚠️ Retrying without sandbox...\n")
					_ = f.Close()
				}
			} else if e.startupLogBuffer != nil {
				_, _ = e.startupLogBuffer.Write([]byte("⚠️ Retrying without sandbox...\n"))
			}

			e.sandboxBypass = true
			e.lastServiceSandboxed = false

			if retryErr := e.StartService(); retryErr == nil {
				log.ServiceLog("✅ Service started without sandbox (auto fallback)")
				goto waitForSDK
			} else {
				_ = e.StopServer()
				return fmt.Errorf("start service (sandboxed): %w; retry without sandbox failed: %w", err, retryErr)
			}
		}
		_ = e.StopServer()
		return fmt.Errorf("start service: %w", err)
	}
	log.ServiceLog("✅ Service started")

waitForSDK:
	log.ServiceLog("Waiting for SDK acknowledgement...")
	if err := e.WaitForSDKAcknowledgement(); err != nil {
		log.ServiceLog(fmt.Sprintf("❌ Failed to get SDK acknowledgement: %v", err))
		_ = e.StopService()
		_ = e.StopServer()
		return fmt.Errorf("sdk acknowledgement: %w", err)
	}
	log.ServiceLog("✅ SDK acknowledged")

	log.Debug("Environment is ready")

	// Discard the in-memory startup buffer now that startup succeeded.
	// File-based logging (--enable-service-logs) persists for the full run.
	e.DiscardStartupBuffer()

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
	log.Debug("Force stopping environment")

	// Force kill the service if it's running
	if e.serviceCmd != nil && e.serviceCmd.Process != nil {
		if err := e.serviceCmd.Process.Kill(); err != nil {
			log.Debug("Failed to kill service process", "error", err)
		}
		e.serviceCmd = nil
	}

	// Stop the server
	if err := e.StopServer(); err != nil {
		log.Debug("Failed to stop server during force stop", "error", err)
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
		log.Debug("Mock server ready", "type", "TCP", "port", port)
	} else {
		socketPath, _ := server.GetConnectionInfo()
		log.Debug("Mock server ready", "type", "Unix", "socket", socketPath)
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

	log.Debug(fmt.Sprintf("Waiting for SDK acknowledgement from the service (timeout: %v)...", timeout))
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
	log.ServiceLog(fmt.Sprintf("🔄 Attempting to restart server (attempt %d/%d)...", attemptNum, MaxServerRestartAttempts))

	// 1. Force stop existing server/service
	if err := e.ForceStopEnvironment(); err != nil {
		log.Warn("Force stop failed during restart", "error", err)
	}

	// 2. Wait with exponential backoff
	shift := attempt
	if shift > 10 {
		shift = 10
	}
	backoff := RestartBackoffBase * time.Duration(1<<shift)
	log.Debug("Waiting before restart", "backoff", backoff, "attempt", attemptNum)
	time.Sleep(backoff)

	// 3. Restart environment
	if err := e.StartEnvironment(); err != nil {
		log.ServiceLog(fmt.Sprintf("❌ Restart attempt %d failed: %v", attemptNum, err))
		// Try again with next attempt
		if attempt+1 < MaxServerRestartAttempts {
			return e.RestartServerWithRetry(attempt + 1)
		}
		return fmt.Errorf("restart attempt %d failed: %w", attemptNum, err)
	}

	log.ServiceLog(fmt.Sprintf("✅ Server restarted successfully (attempt %d)", attemptNum))
	return nil
}

// checkTCPPortAvailable probes both 0.0.0.0 and 127.0.0.1 because the mock
// server binds to 0.0.0.0 but another process may hold the port on loopback
// only. Checking both ensures we catch conflicts on either interface.
func checkTCPPortAvailable(port int) (bool, error) {
	for _, host := range []string{"0.0.0.0", "127.0.0.1"} {
		addr := fmt.Sprintf("%s:%d", host, port)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return true, nil
		}
		_ = ln.Close()
	}
	return false, nil
}
