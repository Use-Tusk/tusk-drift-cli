package runner

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Use-Tusk/tusk-cli/internal/config"
	"github.com/Use-Tusk/tusk-cli/internal/log"
	"github.com/Use-Tusk/tusk-cli/internal/utils"
)

func (e *Executor) StartService() error {
	e.lastServiceSandboxed = false

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
		log.Debug("Failed to check for existing processes on port", "port", cfg.Service.Port, "error", err)
	} else if processExists {
		return fmt.Errorf("port %d is already in use, if your service is already running you should stop it first", cfg.Service.Port)
	}

	log.Debug("Starting service", "command", cfg.Service.Start.Command)

	command := cfg.Service.Start.Command

	// Coverage: nothing to set here, env vars injected below after sandbox wrapping

	// Wrap command with fence sandboxing (if supported and enabled)
	replayOverridePath := e.getReplayComposeOverride()
	if replayOverridePath != "" && isComposeBasedStartCommand(command) {
		commandWithReplayOverride, injected, injectErr := injectComposeOverrideFile(command, replayOverridePath)
		if injectErr != nil {
			return fmt.Errorf("failed to inject replay compose env override: %w", injectErr)
		}
		command = commandWithReplayOverride
		if injected {
			log.ServiceLog(fmt.Sprintf("✅ Replay env override injected into Docker Compose command: %s", replayOverridePath))
		} else {
			log.ServiceLog("❌ Replay env override was prepared but not injected (unsupported Docker Compose command shape)")
		}
	}
	effectiveSandboxMode := e.GetEffectiveSandboxMode()
	if effectiveSandboxMode == SandboxModeOff || e.sandboxBypass {
		log.ServiceLog("⚠️  Replay sandbox disabled (real outbound connections allowed)")
	}

	requireSandbox := effectiveSandboxMode == SandboxModeStrict
	if effectiveSandboxMode != SandboxModeOff && !e.sandboxBypass {
		if !isSandboxSupported() {
			if requireSandbox {
				return fmt.Errorf("strict replay sandbox unavailable: sandbox not supported on this platform")
			}
			log.UserWarn("⚠️  Sandbox unavailable: sandbox not supported on this platform")
			log.UserWarn("   Tests will run without network isolation (real connections allowed)\n")
		} else {
			sandboxConfigPath := cfg.Replay.Sandbox.ConfigPath
			if e.getReplaySandboxConfigPath() != "" {
				sandboxConfigPath = e.getReplaySandboxConfigPath()
			}
			if sandboxConfigPath != "" {
				log.ServiceLog(fmt.Sprintf("🔧 Merged custom Fence config into replay sandbox: %s", utils.ResolveTuskPath(sandboxConfigPath)))
			}

			// Build the list of host paths that must be visible inside
			// the sandbox. The replay compose env-override (when present)
			// is the main case: it lives under /tmp on the host, which
			// fence tmpfs-overmounts — without this exposure the sandboxed
			// docker client can't see the file passed to `-f`.
			var exposedHostPaths []exposedHostPath
			if replayOverridePath != "" {
				exposedHostPaths = append(exposedHostPaths, exposedHostPath{
					Path:     replayOverridePath,
					Writable: false,
				})
			}

			// For docker / docker-compose / podman commands, the daemon
			// binds the host port outside the sandbox netns, so the
			// sandbox must NOT set up a reverse bridge on that port (it
			// would collide with the daemon's own bind). For everything
			// else, the sandbox proxies inbound traffic into its netns as
			// usual.
			sbx, sbxErr := newReplaySandboxManager(replaySandboxOptions{
				UserConfigPath:   sandboxConfigPath,
				Debug:            e.debug,
				ExposedPort:      cfg.Service.Port,
				BindsOnHost:      serviceDelegatesToHostDaemon(cfg.Service.Start.Command),
				ExposedHostPaths: exposedHostPaths,
			})
			if sbxErr != nil {
				if requireSandbox {
					return fmt.Errorf("strict replay sandbox unavailable: %s", friendlySandboxError(sbxErr))
				}
				log.UserWarn(fmt.Sprintf("⚠️  Sandbox unavailable: %s", friendlySandboxError(sbxErr)))
				log.UserWarn("   Tests will run without network isolation (real connections allowed)\n")
			} else {
				wrappedCmd, wrapErr := sbx.WrapCommand(command)
				if wrapErr != nil {
					sbx.Cleanup()
					if requireSandbox {
						return fmt.Errorf("strict replay sandbox unavailable: %s", friendlySandboxError(wrapErr))
					}
					log.UserWarn(fmt.Sprintf("⚠️  Sandbox unavailable: %s", friendlySandboxError(wrapErr)))
					log.UserWarn("   Tests will run without network isolation (real connections allowed)\n")
				} else {
					e.sandbox = sbx
					command = wrappedCmd
					e.lastServiceSandboxed = true
					log.ServiceLog("🔒 Service sandboxed (localhost outbound blocked for replay isolation)")
				}
			}
		}
	}

	ctx := context.Background()
	e.serviceCmd = createServiceCommand(ctx, command)

	// Set up process group so we can kill all child processes
	setupProcessGroup(e.serviceCmd)

	env := e.buildCommandEnv()

	if e.server != nil {
		socketPath, tcpPort := e.server.GetConnectionInfo()

		if e.server.GetCommunicationType() == CommunicationTCP {
			// TCP mode - set host and port
			env = append(env, fmt.Sprintf("TUSK_MOCK_PORT=%d", tcpPort))
			env = append(env, "TUSK_MOCK_HOST=host.docker.internal") // Mac/Windows

			log.Debug("Setting TCP environment variables",
				"TUSK_MOCK_PORT", tcpPort,
				"TUSK_MOCK_HOST", "host.docker.internal")
		} else {
			// Unix socket mode
			env = append(env, fmt.Sprintf("TUSK_MOCK_SOCKET=%s", socketPath))
			log.Debug("Setting socket environment variable", "TUSK_MOCK_SOCKET", socketPath)

			if _, err := os.Stat(socketPath); err != nil {
				return fmt.Errorf("socket file does not exist before starting service: %w", err)
			}
		}
	}

	env = append(env, "TUSK_DRIFT_MODE=REPLAY")

	// Coverage: inject env vars that SDK coverage servers listen for.
	// NODE_V8_COVERAGE is required by the Node SDK to enable V8 coverage collection.
	// Coverage env vars:
	// TUSK_COVERAGE=true - language-agnostic signal for both Node and Python SDKs
	// NODE_V8_COVERAGE=<dir> - Node-specific: tells V8 to collect coverage data
	// TS_NODE_EMIT=true - Node-specific: forces ts-node to write compiled JS to disk
	if e.coverageEnabled {
		env = append(env, "TUSK_COVERAGE=true")
		// Node.js: V8 coverage needs a directory to write JSON files
		v8CoverageDir, err := os.MkdirTemp("", "tusk-v8-coverage-*")
		if err != nil {
			return fmt.Errorf("failed to create temp dir for V8 coverage: %w", err)
		}
		e.coverageTempDir = v8CoverageDir
		env = append(env, fmt.Sprintf("NODE_V8_COVERAGE=%s", v8CoverageDir))
		env = append(env, "TS_NODE_EMIT=true")
		log.Debug("Coverage enabled", "v8_dir", v8CoverageDir)
	}

	e.serviceCmd.Env = env

	// Always capture service logs during startup.
	// When --enable-service-logs is set, logs go to a file.
	// Otherwise, logs go to an in-memory buffer that is shown on failure and discarded on success.
	if err := e.setupServiceLogging(); err != nil {
		log.Debug("Failed to setup service logging, suppressing output", "error", err)
		e.serviceCmd.Stdout = nil
		e.serviceCmd.Stderr = nil
	} else if e.enableServiceLogs {
		e.serviceCmd.Stdout = e.serviceLogFile
		e.serviceCmd.Stderr = e.serviceLogFile
	} else {
		e.serviceCmd.Stdout = e.startupLogBuffer
		e.serviceCmd.Stderr = e.startupLogBuffer
	}

	if err := e.serviceCmd.Start(); err != nil {
		if e.sandbox != nil {
			e.sandbox.Cleanup()
			e.sandbox = nil
		}
		return fmt.Errorf("failed to start service: %w", err)
	}

	// Monitor process for early exit so waitForReadiness can fail fast.
	// Capture the channel locally so that if a sandbox retry creates a new channel,
	// this goroutine still sends to the original one (not the new one).
	e.processExitCh = make(chan error, 1)
	exitCh := e.processExitCh
	cmd := e.serviceCmd
	go func() {
		exitCh <- cmd.Wait()
	}()

	if err := e.waitForReadiness(cfg); err != nil {
		_ = e.StopService()
		return fmt.Errorf("service readiness check failed: %w", err)
	}

	log.Debug("Service is ready", "url", e.serviceURL)

	return nil
}

func (e *Executor) StopService() error {
	cfg, _ := config.Get()

	defer func() {
		e.cleanupLogFiles()
		if e.sandbox != nil {
			e.sandbox.Cleanup()
			e.sandbox = nil
		}
		// Clean up V8 coverage temp directory
		if e.coverageTempDir != "" {
			_ = os.RemoveAll(e.coverageTempDir)
			e.coverageTempDir = ""
		}
		log.ServiceLog("Service stopped")
	}()

	// Use custom stop command if provided
	if cfg != nil && cfg.Service.Stop.Command != "" {
		log.Debug("Using custom stop command", "command", cfg.Service.Stop.Command)

		stopCmd := createServiceCommand(context.Background(), cfg.Service.Stop.Command)
		stopCmd.Env = e.buildCommandEnv()
		if err := stopCmd.Run(); err != nil {
			log.Warn("Stop command failed", "error", err)
			// Continue to fallback method
		} else {
			return nil
		}
	}

	// Default: kill process group
	if e.serviceCmd != nil && e.serviceCmd.Process != nil {
		// Use platform-specific process group killing with 3 second timeout
		if err := killProcessGroup(e.serviceCmd, 3*time.Second); err != nil {
			log.Debug("Process group kill completed with error", "error", err)
		}
		e.serviceCmd = nil
	}

	return nil
}

func mergeEnvVars(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return base
	}

	merged := make([]string, len(base))
	copy(merged, base)

	indexByKey := make(map[string]int, len(base))
	for i, pair := range merged {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			indexByKey[parts[0]] = i
		}
	}

	for key, value := range overrides {
		pair := fmt.Sprintf("%s=%s", key, value)
		if idx, ok := indexByKey[key]; ok {
			merged[idx] = pair
			continue
		}

		indexByKey[key] = len(merged)
		merged = append(merged, pair)
	}

	return merged
}

func (e *Executor) buildCommandEnv() []string {
	return mergeEnvVars(os.Environ(), e.getReplayEnvVars())
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
	log.Debug("Checking for existing processes on port", "port", port)

	// Try to connect to the port
	addr := fmt.Sprintf("localhost:%d", port)
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err == nil {
		// Successfully connected - port is in use
		_ = conn.Close()
		log.Debug("Port is already in use", "port", port)
		return true, nil
	}

	// Check if it's a connection refused (means port is available)
	// vs other errors (network issues, etc.)
	log.Debug("Port appears to be available", "port", port, "error", err)
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
		// Wait for the specified duration, but fail fast if the process exits
		select {
		case exitErr := <-e.processExitCh:
			return fmt.Errorf("service process exited during startup: %w", exitErr)
		case <-time.After(waitTime):
			return nil
		}
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
		// Check for early process exit before each readiness attempt
		select {
		case exitErr := <-e.processExitCh:
			return fmt.Errorf("service process exited during startup: %w", exitErr)
		default:
		}

		cmd := createReadinessCommand(cfg.Service.Readiness.Command)
		cmd.Env = e.buildCommandEnv()
		if err := cmd.Run(); err == nil {
			return nil
		}

		// Wait for the interval, but fail fast if the process exits
		select {
		case exitErr := <-e.processExitCh:
			return fmt.Errorf("service process exited during startup: %w", exitErr)
		case <-time.After(interval):
		}
	}

	return fmt.Errorf("service failed to become ready within %v. You can increase the timeout in .tusk/config.yaml under service.readiness.timeout", timeout)
}

// SetDisableServiceLogs sets whether service logging should be disabled
func (e *Executor) SetEnableServiceLogs(enable bool) {
	e.enableServiceLogs = enable
}

// setupServiceLogging prepares log capture for the service process.
// When --enable-service-logs is set, logs are written to a file in .tusk/logs/.
// Otherwise, an in-memory buffer captures startup output (shown on failure, discarded on success).
func (e *Executor) setupServiceLogging() error {
	if e.enableServiceLogs {
		// Reuse existing log file across sandbox retries (same as the buffer path guard below).
		// The file was closed by StopService → cleanupLogFiles, but we can reopen it for append.
		if e.serviceLogPath != "" {
			logFile, err := os.OpenFile(e.serviceLogPath, os.O_APPEND|os.O_WRONLY, 0o600) // #nosec G304
			if err != nil {
				return fmt.Errorf("failed to reopen service log file: %w", err)
			}
			e.serviceLogFile = logFile
		} else {
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
			e.serviceLogPath = logPath
			log.ServiceLog(fmt.Sprintf("Service logs will be written to: %s", logPath))
		}
	} else if e.startupLogBuffer == nil {
		// Only create a new buffer if one doesn't already exist.
		// During sandbox retry, the existing buffer preserves the first attempt's logs.
		e.startupLogBuffer = &syncBuffer{}
	}

	return nil
}

// cleanupLogFiles closes the log file
func (e *Executor) cleanupLogFiles() {
	if e.serviceLogFile != nil {
		_ = e.serviceLogFile.Close()
		e.serviceLogFile = nil
	}
}

// friendlySandboxError extracts a user-friendly error message for sandbox initialization failures.
func friendlySandboxError(err error) string {
	errStr := err.Error()

	// Check for common issues and provide actionable messages
	if strings.Contains(errStr, "socat") {
		return "socat not installed (run: sudo apt install socat)"
	}
	if strings.Contains(errStr, "bubblewrap") || strings.Contains(errStr, "bwrap") {
		return "bubblewrap not installed (run: sudo apt install bubblewrap)"
	}

	// Generic fallback - extract the most relevant part
	if strings.Contains(errStr, ": ") {
		// Get the innermost error message
		parts := strings.Split(errStr, ": ")
		return parts[len(parts)-1]
	}
	return errStr
}
