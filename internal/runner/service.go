package runner

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
	"github.com/Use-Tusk/tusk-drift-cli/internal/logging"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
)

func (e *Executor) StartService() error {
	if err := config.Load(""); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	cfg, err := config.Get()
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	e.servicePort = cfg.Service.Port
	e.serviceURL = fmt.Sprintf("http://localhost:%d", cfg.Service.Port)

	if cfg.Service.Start.Command == "" {
		return fmt.Errorf("no start command defined in config")
	}

	processExists, err := e.checkProcessOnPort(cfg.Service.Port)
	if err != nil {
		slog.Debug("Failed to check for existing processes on port", "port", cfg.Service.Port, "error", err)
	} else if processExists {
		return fmt.Errorf("port %d is already in use, if your service is already running you should stop it first", cfg.Service.Port)
	}

	slog.Debug("Starting service", "command", cfg.Service.Start.Command)

	ctx := context.Background()
	e.serviceCmd = createServiceCommand(ctx, cfg.Service.Start.Command)

	// Set up process group so we can kill all child processes
	setupProcessGroup(e.serviceCmd)

	env := os.Environ()

	if e.server != nil {
		socketPath, tcpPort := e.server.GetConnectionInfo()

		if e.server.GetCommunicationType() == CommunicationTCP {
			// TCP mode - set host and port
			env = append(env, fmt.Sprintf("TUSK_MOCK_PORT=%d", tcpPort))
			env = append(env, "TUSK_MOCK_HOST=host.docker.internal") // Mac/Windows

			slog.Debug("Setting TCP environment variables",
				"TUSK_MOCK_PORT", tcpPort,
				"TUSK_MOCK_HOST", "host.docker.internal")
		} else {
			// Unix socket mode
			env = append(env, fmt.Sprintf("TUSK_MOCK_SOCKET=%s", socketPath))
			slog.Debug("Setting socket environment variable", "TUSK_MOCK_SOCKET", socketPath)

			if _, err := os.Stat(socketPath); err != nil {
				return fmt.Errorf("socket file does not exist before starting service: %w", err)
			}
		}
	}

	env = append(env, "TUSK_DRIFT_MODE=REPLAY")
	e.serviceCmd.Env = env

	// Dump service logs to file in .tusk/logs instead of suppressing
	// TODO: provide option whether to store these logs
	if err := e.setupServiceLogging(); err != nil {
		slog.Debug("Failed to setup service logging, suppressing output", "error", err)
		e.serviceCmd.Stdout = nil
		e.serviceCmd.Stderr = nil
	} else {
		e.serviceCmd.Stdout = e.serviceLogFile
		e.serviceCmd.Stderr = e.serviceLogFile
	}

	if err := e.serviceCmd.Start(); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	if err := e.waitForReadiness(cfg); err != nil {
		_ = e.StopService()
		return fmt.Errorf("service readiness check failed: %w", err)
	}

	slog.Debug("Service is ready", "url", e.serviceURL)

	return nil
}

func (e *Executor) StopService() error {
	cfg, _ := config.Get()

	defer func() {
		e.cleanupLogFiles()
		logging.LogToService("Service stopped")
	}()

	// Use custom stop command if provided
	if cfg != nil && cfg.Service.Stop.Command != "" {
		slog.Debug("Using custom stop command", "command", cfg.Service.Stop.Command)

		stopCmd := createServiceCommand(context.Background(), cfg.Service.Stop.Command)
		if err := stopCmd.Run(); err != nil {
			slog.Warn("Stop command failed", "error", err)
			// Continue to fallback method
		} else {
			return nil
		}
	}

	// Default: kill process group
	if e.serviceCmd != nil && e.serviceCmd.Process != nil {
		// Use platform-specific process group killing with 3 second timeout
		if err := killProcessGroup(e.serviceCmd, 3*time.Second); err != nil {
			slog.Debug("Process group kill completed with error", "error", err)
		}
		e.serviceCmd = nil
	}

	return nil
}

func (e *Executor) GetServiceLogPath() string {
	if e.serviceLogFile != nil {
		return e.serviceLogFile.Name()
	}
	return ""
}

// checkProcessOnPort checks if any process is listening on the specified port
// Returns true if a process is found, false otherwise
// This uses a connection-based approach to check if the port is already in use.
func (e *Executor) checkProcessOnPort(port int) (bool, error) {
	slog.Debug("Checking for existing processes on port", "port", port)

	// Try to connect to the port
	addr := fmt.Sprintf("localhost:%d", port)
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err == nil {
		// Successfully connected - port is in use
		_ = conn.Close()
		slog.Debug("Port is already in use", "port", port)
		return true, nil
	}

	// Check if it's a connection refused (means port is available)
	// vs other errors (network issues, etc.)
	slog.Debug("Port appears to be available", "port", port, "error", err)
	return false, nil
}

// waitForReadiness polls the specified readiness check command until it succeeds or the timeout is reached.
// This is necessary because replaying traces requires the service to be properly instrumented and ready to handle requests.
// If no readiness check command is configured, it will simply wait for 10 seconds.
func (e *Executor) waitForReadiness(cfg *config.Config) error {
	if cfg.Service.Readiness.Command == "" {
		// Allow tests to override the default wait time
		waitTime := 10 * time.Second
		if testWait := os.Getenv("TUSK_TEST_DEFAULT_WAIT"); testWait != "" {
			if parsed, err := time.ParseDuration(testWait); err == nil {
				waitTime = parsed
			}
		}
		time.Sleep(waitTime)
		return nil
	}

	timeout := 10 * time.Second
	if cfg.Service.Readiness.Timeout != "" {
		if parsedTimeout, err := time.ParseDuration(cfg.Service.Readiness.Timeout); err == nil {
			timeout = parsedTimeout
		}
	}

	interval := 2 * time.Second
	if cfg.Service.Readiness.Interval != "" {
		if parsedInterval, err := time.ParseDuration(cfg.Service.Readiness.Interval); err == nil {
			interval = parsedInterval
		}
	}

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		cmd := createReadinessCommand(cfg.Service.Readiness.Command)
		if err := cmd.Run(); err == nil {
			return nil
		}

		time.Sleep(interval)
	}

	return fmt.Errorf("service failed to become ready within %v. You can increase the timeout in .tusk/config.yaml under service.readiness.timeout", timeout)
}

// SetDisableServiceLogs sets whether service logging should be disabled
func (e *Executor) SetEnableServiceLogs(enable bool) {
	e.enableServiceLogs = enable
}

// setupServiceLogging creates a log file for service stdout and stderr if service logging is not disabled.
func (e *Executor) setupServiceLogging() error {
	if !e.enableServiceLogs {
		return fmt.Errorf("service logging disabled")
	}

	// Allow tests to override the logs directory
	logsDir := utils.GetLogsDir()
	if testLogsDir := os.Getenv("TUSK_TEST_LOGS_DIR"); testLogsDir != "" {
		logsDir = testLogsDir
	}

	if err := os.MkdirAll(logsDir, 0o750); err != nil {
		return fmt.Errorf("failed to create logs directory: %w", err)
	}

	timestamp := time.Now().Format("20060102-150405")

	logPath := filepath.Join(logsDir, fmt.Sprintf("tusk-replay-%s.log", timestamp))
	logFile, err := os.Create(logPath) // #nosec G304
	if err != nil {
		return fmt.Errorf("failed to create service log file: %w", err)
	}

	e.serviceLogFile = logFile

	logging.LogToService(fmt.Sprintf("Service logs will be written to: %s", logPath))

	return nil
}

// cleanupLogFiles closes the log file
func (e *Executor) cleanupLogFiles() {
	if e.serviceLogFile != nil {
		_ = e.serviceLogFile.Close()
		e.serviceLogFile = nil
	}
}
