package runner

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Note: this test file uses the following specific
// env vars for the sole purpose of unit testing:
// - TUSK_TEST_DEFAULT_WAIT: default wait time for readiness check (to make tests run faster)
// - TUSK_TEST_LOGS_DIR: directory to store logs (store it in temp dir to avoid modifying the
// user's .tusk/logs directory)

// noopCleanup returns an empty cleanup function
func noopCleanup() func() {
	return func() {}
}

// createTestConfig is a helper to create a test config file with optional default timeout override
func createTestConfig(t *testing.T, port int, startCmd, readinessCmd string) string {
	return createTestConfigWithTimeout(t, port, startCmd, readinessCmd, "2s")
}

func createTestConfigWithTimeout(t *testing.T, port int, startCmd, readinessCmd, timeout string) string {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "tusk.yaml")

	configContent := fmt.Sprintf(`
service:
  port: %d
  start:
    command: "%s"
  readiness_check:
    command: "%s"
    timeout: "%s"
    interval: "200ms"
`, port, startCmd, readinessCmd, timeout)

	err := os.WriteFile(configPath, []byte(configContent), 0o600)
	require.NoError(t, err)

	return configPath
}

func TestStartService(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(t *testing.T) (*Executor, string, func())
		expectError bool
		errorMsg    string
	}{
		{
			name: "successful_start_without_readiness_check",
			setupFunc: func(t *testing.T) (*Executor, string, func()) {
				e := NewExecutor()
				// Use environment variable to speed up tests
				origVal := os.Getenv("TUSK_TEST_DEFAULT_WAIT")
				_ = os.Setenv("TUSK_TEST_DEFAULT_WAIT", "100ms")
				configPath := createTestConfig(t, 13001, getSimpleSleepCommand(), "")
				return e, configPath, func() {
					if origVal != "" {
						_ = os.Setenv("TUSK_TEST_DEFAULT_WAIT", origVal)
					} else {
						_ = os.Unsetenv("TUSK_TEST_DEFAULT_WAIT")
					}
				}
			},
			expectError: false,
		},
		{
			name: "successful_start_with_readiness_check",
			setupFunc: func(t *testing.T) (*Executor, string, func()) {
				e := NewExecutor()
				configPath := createTestConfig(t, 13002, getSimpleSleepCommand(), "true")
				return e, configPath, func() {}
			},
			expectError: false,
		},
		{
			name: "fail_when_no_start_command",
			setupFunc: func(t *testing.T) (*Executor, string, func()) {
				e := NewExecutor()
				configPath := createTestConfig(t, 13003, "", "")
				return e, configPath, func() {}
			},
			expectError: true,
			errorMsg:    "no start command defined in config",
		},
		{
			name: "fail_when_port_already_in_use",
			setupFunc: func(t *testing.T) (*Executor, string, func()) {
				e := NewExecutor()
				// Start a listener on the port to simulate it being in use
				listener, err := net.Listen("tcp", ":13004") // #nosec G102
				require.NoError(t, err)
				configPath := createTestConfig(t, 13004, "sleep 0.1", "")
				return e, configPath, func() { _ = listener.Close() }
			},
			expectError: true,
			errorMsg:    "port 13004 is already in use",
		},
		{
			name: "start_with_server_socket",
			setupFunc: func(t *testing.T) (*Executor, string, func()) {
				e := NewExecutor()
				// Speed up test
				origVal := os.Getenv("TUSK_TEST_DEFAULT_WAIT")
				_ = os.Setenv("TUSK_TEST_DEFAULT_WAIT", "100ms")

				testServiceConfig := &config.ServiceConfig{
					ID:   "test-service",
					Port: 13005,
					Start: config.StartConfig{
						Command: "sleep 0.1",
					},
					Communication: config.CommunicationConfig{
						Type:    "unix", // Force Unix mode for this test
						TCPPort: 9001,
					},
				}

				server, err := NewServer("test-service", testServiceConfig)
				require.NoError(t, err)

				// Start the server so socketPath is set
				err = server.Start()
				require.NoError(t, err)

				// Now get the socket path (it's been set by Start())
				socketPath := server.GetSocketPath()
				require.NotEmpty(t, socketPath, "socket path should not be empty")

				e.server = server

				configPath := createTestConfig(t, 13005, "sleep 0.1", "")
				return e, configPath, func() {
					_ = server.Stop() // Clean up the server
					if origVal != "" {
						_ = os.Setenv("TUSK_TEST_DEFAULT_WAIT", origVal)
					} else {
						_ = os.Unsetenv("TUSK_TEST_DEFAULT_WAIT")
					}
				}
			},
			expectError: false,
		},
		{
			name: "fail_when_socket_doesnt_exist",
			setupFunc: func(t *testing.T) (*Executor, string, func()) {
				e := NewExecutor()
				// Create a server but don't create the socket file
				testServiceConfig := &config.ServiceConfig{
					ID:   "test-service",
					Port: 13006,
					Start: config.StartConfig{
						Command: "sleep 0.1",
					},
					Communication: config.CommunicationConfig{
						Type:    "auto",
						TCPPort: 9001,
					},
				}
				server, err := NewServer("test-service", testServiceConfig)
				require.NoError(t, err)

				// Remove the socket if it was created
				_ = os.Remove(server.GetSocketPath())

				e.server = server

				configPath := createTestConfig(t, 13006, "sleep 0.1", "")
				return e, configPath, func() {}
			},
			expectError: true,
			errorMsg:    "socket file does not exist",
		},
		{
			name: "fail_when_readiness_check_times_out",
			setupFunc: func(t *testing.T) (*Executor, string, func()) {
				e := NewExecutor()
				configPath := createTestConfig(t, 13007, "sleep 2", "false")
				return e, configPath, func() {}
			},
			expectError: true,
			errorMsg:    "service readiness check failed",
		},
		{
			name: "service_logging_enabled",
			setupFunc: func(t *testing.T) (*Executor, string, func()) {
				e := NewExecutor()
				e.SetEnableServiceLogs(true)
				// Speed up test and use temp directory for logs
				origWait := os.Getenv("TUSK_TEST_DEFAULT_WAIT")
				_ = os.Setenv("TUSK_TEST_DEFAULT_WAIT", "100ms")

				tempDir := t.TempDir()
				logsDir := filepath.Join(tempDir, "logs")
				origLogsDir := os.Getenv("TUSK_TEST_LOGS_DIR")
				_ = os.Setenv("TUSK_TEST_LOGS_DIR", logsDir)

				configPath := createTestConfig(t, 13008, "sleep 0.1", "")
				return e, configPath, func() {
					if origWait != "" {
						_ = os.Setenv("TUSK_TEST_DEFAULT_WAIT", origWait)
					} else {
						_ = os.Unsetenv("TUSK_TEST_DEFAULT_WAIT")
					}
					if origLogsDir != "" {
						_ = os.Setenv("TUSK_TEST_LOGS_DIR", origLogsDir)
					} else {
						_ = os.Unsetenv("TUSK_TEST_LOGS_DIR")
					}
				}
			},
			expectError: false,
		},
		{
			name: "service_logging_disabled",
			setupFunc: func(t *testing.T) (*Executor, string, func()) {
				e := NewExecutor()
				e.SetEnableServiceLogs(false)
				// Speed up test
				origVal := os.Getenv("TUSK_TEST_DEFAULT_WAIT")
				_ = os.Setenv("TUSK_TEST_DEFAULT_WAIT", "100ms")
				configPath := createTestConfig(t, 13009, "sleep 0.1", "")
				return e, configPath, func() {
					if origVal != "" {
						_ = os.Setenv("TUSK_TEST_DEFAULT_WAIT", origVal)
					} else {
						_ = os.Unsetenv("TUSK_TEST_DEFAULT_WAIT")
					}
				}
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.ResetForTesting()

			// Setup
			executor, configPath, cleanup := tt.setupFunc(t)
			defer cleanup()

			// Load config from file
			err := config.Load(configPath)
			require.NoError(t, err)

			// Execute
			err = executor.StartService()

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.Greater(t, executor.servicePort, 0)
				assert.Contains(t, executor.serviceURL, "http://localhost:")

				// Clean up the service if started successfully
				if executor.serviceCmd != nil && executor.serviceCmd.Process != nil {
					_ = executor.StopService()
				}
			}

			// Clean up log files
			if executor.serviceLogFile != nil {
				_ = executor.serviceLogFile.Close()
				_ = os.Remove(executor.serviceLogFile.Name())
			}
		})
	}
}

func TestStopService(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func() *Executor
	}{
		{
			name: "stop_running_service_gracefully",
			setupFunc: func() *Executor {
				e := NewExecutor()
				// Create a mock process using platform-specific helper
				ctx := context.Background()
				cmd := createTestCommand(ctx, "10")
				_ = cmd.Start()
				e.serviceCmd = cmd
				e.servicePort = 3000
				return e
			},
		},
		{
			name:      "stop_when_no_service_running",
			setupFunc: NewExecutor,
		},
		{
			name: "stop_with_force_kill_after_timeout",
			setupFunc: func() *Executor {
				e := NewExecutor()
				// Create a process that's harder to kill
				ctx := context.Background()
				cmd := createUnkillableTestCommand(ctx, "10")
				_ = cmd.Start()
				e.serviceCmd = cmd
				e.servicePort = 3000
				return e
			},
		},
		{
			name: "stop_with_log_file_cleanup",
			setupFunc: func() *Executor {
				e := NewExecutor()
				// Create a temporary log file
				tempFile, _ := os.CreateTemp("", "test-service-*.log")
				e.serviceLogFile = tempFile

				// Create a mock process
				ctx := context.Background()
				cmd := createTestCommand(ctx, "1")
				_ = cmd.Start()
				e.serviceCmd = cmd
				return e
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.ResetForTesting()

			// Setup
			executor := tt.setupFunc()

			// Execute
			err := executor.StopService()

			// Assert
			assert.NoError(t, err)
			assert.Nil(t, executor.serviceCmd)
			assert.Nil(t, executor.serviceLogFile)
		})
	}
}

func TestGetServiceLogPath(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *Executor
		expected string
	}{
		{
			name: "returns_path_when_log_file_exists",
			setup: func() *Executor {
				e := NewExecutor()
				tempFile, _ := os.CreateTemp("", "test-*.log")
				e.serviceLogFile = tempFile
				return e
			},
			expected: "test-",
		},
		{
			name:     "returns_empty_when_no_log_file",
			setup:    NewExecutor,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.ResetForTesting()

			executor := tt.setup()
			path := executor.GetServiceLogPath()

			if tt.expected == "" {
				assert.Empty(t, path)
			} else {
				assert.Contains(t, path, tt.expected)
			}

			// Cleanup
			if executor.serviceLogFile != nil {
				_ = executor.serviceLogFile.Close()
				_ = os.Remove(executor.serviceLogFile.Name())
			}
		})
	}
}

func TestCheckProcessOnPort(t *testing.T) {
	// Note: This test is challenging because it depends on the lsof command
	// We'll test the logic but mock the actual command execution

	tests := []struct {
		name       string
		port       int
		lsofOutput string
		lsofError  error
		wantExists bool
		wantError  bool
	}{
		{
			name:       "port_in_use",
			port:       3000,
			lsofOutput: "12345\n67890\n",
			lsofError:  nil,
			wantExists: true,
			wantError:  false,
		},
		{
			name:       "port_free",
			port:       3000,
			lsofOutput: "",
			lsofError:  fmt.Errorf("exit status 1"),
			wantExists: false,
			wantError:  false,
		},
		{
			name:       "empty_output",
			port:       3000,
			lsofOutput: "   \n   \n",
			lsofError:  nil,
			wantExists: false,
			wantError:  false,
		},
		{
			name:       "single_process",
			port:       3000,
			lsofOutput: "54321",
			lsofError:  nil,
			wantExists: true,
			wantError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.ResetForTesting()

			e := NewExecutor()

			// We can't easily mock exec.Command, so we'll test the actual method
			// on a port that's unlikely to be in use
			if tt.name == "port_free" {
				// Use a high random port that's unlikely to be in use
				exists, err := e.checkProcessOnPort(65432)
				assert.NoError(t, err)
				assert.False(t, exists)
			}

			// For other cases, we'd need to actually have processes on ports
			// which is not practical in unit tests
		})
	}
}

func TestWaitForReadiness(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		expectError bool
		setupFunc   func() func()
	}{
		{
			name: "no_readiness_check_waits_default",
			config: &config.Config{
				Service: config.ServiceConfig{
					Readiness: config.ReadinessConfig{
						Command: "",
					},
				},
			},
			expectError: false,
			setupFunc: func() func() {
				// Speed up test by setting a short default wait
				origVal := os.Getenv("TUSK_TEST_DEFAULT_WAIT")
				_ = os.Setenv("TUSK_TEST_DEFAULT_WAIT", "100ms")
				return func() {
					if origVal != "" {
						_ = os.Setenv("TUSK_TEST_DEFAULT_WAIT", origVal)
					} else {
						_ = os.Unsetenv("TUSK_TEST_DEFAULT_WAIT")
					}
				}
			},
		},
		{
			name: "readiness_check_succeeds_immediately",
			config: &config.Config{
				Service: config.ServiceConfig{
					Readiness: config.ReadinessConfig{
						Command:  "true",
						Timeout:  "5s",
						Interval: "1s",
					},
				},
			},
			expectError: false,
			setupFunc:   noopCleanup,
		},
		{
			name: "readiness_check_fails_with_timeout",
			config: &config.Config{
				Service: config.ServiceConfig{
					Readiness: config.ReadinessConfig{
						Command:  "false",
						Timeout:  "1s",
						Interval: "200ms",
					},
				},
			},
			expectError: true,
			setupFunc:   noopCleanup,
		},
		{
			name: "invalid_timeout_uses_default",
			config: &config.Config{
				Service: config.ServiceConfig{
					Readiness: config.ReadinessConfig{
						Command:  "true",
						Timeout:  "invalid",
						Interval: "1s",
					},
				},
			},
			expectError: false,
			setupFunc:   noopCleanup,
		},
		{
			name: "invalid_interval_uses_default",
			config: &config.Config{
				Service: config.ServiceConfig{
					Readiness: config.ReadinessConfig{
						Command:  "true",
						Timeout:  "5s",
						Interval: "invalid",
					},
				},
			},
			expectError: false,
			setupFunc:   noopCleanup,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.ResetForTesting()

			cleanup := tt.setupFunc()
			defer cleanup()

			e := NewExecutor()
			err := e.waitForReadiness(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "service failed to become ready")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSetEnableServiceLogs(t *testing.T) {
	e := NewExecutor()

	e.SetEnableServiceLogs(true)
	assert.True(t, e.enableServiceLogs)

	e.SetEnableServiceLogs(false)
	assert.False(t, e.enableServiceLogs)
}

func TestSetupServiceLogging(t *testing.T) {
	tests := []struct {
		name              string
		enableServiceLogs bool
		setupFunc         func() func()
		expectError       bool
	}{
		{
			name:              "logging_disabled",
			enableServiceLogs: false,
			setupFunc:         noopCleanup,
			expectError:       true,
		},
		{
			name:              "logging_enabled_creates_file",
			enableServiceLogs: true,
			setupFunc: func() func() {
				// Use temp directory for logs
				tempDir := t.TempDir()
				logsDir := filepath.Join(tempDir, "logs")
				origVal := os.Getenv("TUSK_TEST_LOGS_DIR")
				_ = os.Setenv("TUSK_TEST_LOGS_DIR", logsDir)
				return func() {
					if origVal != "" {
						_ = os.Setenv("TUSK_TEST_LOGS_DIR", origVal)
					} else {
						_ = os.Unsetenv("TUSK_TEST_LOGS_DIR")
					}
				}
			},
			expectError: false,
		},
		{
			name:              "logging_enabled_existing_directory",
			enableServiceLogs: true,
			setupFunc: func() func() {
				// Use temp directory for logs
				tempDir := t.TempDir()
				logsDir := filepath.Join(tempDir, "logs")
				_ = os.MkdirAll(logsDir, 0o750)
				origVal := os.Getenv("TUSK_TEST_LOGS_DIR")
				_ = os.Setenv("TUSK_TEST_LOGS_DIR", logsDir)
				return func() {
					if origVal != "" {
						_ = os.Setenv("TUSK_TEST_LOGS_DIR", origVal)
					} else {
						_ = os.Unsetenv("TUSK_TEST_LOGS_DIR")
					}
				}
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.ResetForTesting()

			cleanup := tt.setupFunc()
			defer cleanup()

			e := NewExecutor()
			e.enableServiceLogs = tt.enableServiceLogs

			err := e.setupServiceLogging()

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, e.serviceLogFile)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, e.serviceLogFile)

				// Verify file was created
				assert.FileExists(t, e.serviceLogFile.Name())
				assert.Contains(t, e.serviceLogFile.Name(), "tusk-replay-")
				assert.Contains(t, e.serviceLogFile.Name(), ".log")

				_ = e.serviceLogFile.Close()
				_ = os.Remove(e.serviceLogFile.Name())
			}
		})
	}
}

func TestCleanupLogFiles(t *testing.T) {
	e := NewExecutor()

	// Test with no log file
	e.cleanupLogFiles()
	assert.Nil(t, e.serviceLogFile)

	// Test with log file
	tempFile, err := os.CreateTemp("", "test-cleanup-*.log")
	require.NoError(t, err)
	e.serviceLogFile = tempFile

	e.cleanupLogFiles()
	assert.Nil(t, e.serviceLogFile)

	// Verify file is closed (writing should fail)
	_, err = tempFile.Write([]byte("test"))
	assert.Error(t, err)

	_ = os.Remove(tempFile.Name())
}

func TestIntegrationServiceLifecycle(t *testing.T) {
	config.ResetForTesting()

	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup with temp directory for logs
	tempDir := t.TempDir()
	logsDir := filepath.Join(tempDir, "logs")
	origLogsDir := os.Getenv("TUSK_TEST_LOGS_DIR")
	_ = os.Setenv("TUSK_TEST_LOGS_DIR", logsDir)
	defer func() {
		if origLogsDir != "" {
			_ = os.Setenv("TUSK_TEST_LOGS_DIR", origLogsDir)
		} else {
			_ = os.Unsetenv("TUSK_TEST_LOGS_DIR")
		}
	}()

	// Setup
	e := NewExecutor()
	e.SetEnableServiceLogs(true)

	configPath := createTestConfig(t, 19876, getLongRunningCommand(), "true")

	err := config.Load(configPath)
	require.NoError(t, err)

	err = e.StartService()
	require.NoError(t, err)

	// Verify service is running
	assert.NotNil(t, e.serviceCmd)
	assert.NotNil(t, e.serviceCmd.Process)
	assert.Equal(t, 19876, e.servicePort)
	assert.Equal(t, "http://localhost:19876", e.serviceURL)

	// Verify log file was created
	logPath := e.GetServiceLogPath()
	assert.NotEmpty(t, logPath)
	assert.FileExists(t, logPath)
	assert.Contains(t, logPath, logsDir) // Verify it's in the temp directory

	err = e.StopService()
	assert.NoError(t, err)

	// Verify service is stopped
	assert.Nil(t, e.serviceCmd)
	assert.Nil(t, e.serviceLogFile)
}

func TestConcurrentServiceOperations(t *testing.T) {
	config.ResetForTesting()

	e := NewExecutor()
	e.SetEnableServiceLogs(false)

	// Speed up test by setting a short default wait
	origVal := os.Getenv("TUSK_TEST_DEFAULT_WAIT")
	_ = os.Setenv("TUSK_TEST_DEFAULT_WAIT", "100ms")
	defer func() {
		if origVal != "" {
			_ = os.Setenv("TUSK_TEST_DEFAULT_WAIT", origVal)
		} else {
			_ = os.Unsetenv("TUSK_TEST_DEFAULT_WAIT")
		}
	}()

	configPath := createTestConfig(t, 13010, "sleep 0.5", "")

	err := config.Load(configPath)
	require.NoError(t, err)

	err = e.StartService()
	require.NoError(t, err)

	// Concurrent operations
	var wg sync.WaitGroup
	errors := make([]error, 3)

	wg.Add(3)
	go func() {
		defer wg.Done()
		errors[0] = e.StopService()
	}()

	go func() {
		defer wg.Done()
		_ = e.GetServiceLogPath()
	}()

	go func() {
		defer wg.Done()
		e.SetEnableServiceLogs(true)
	}()

	wg.Wait()

	// At least the stop should succeed
	assert.NoError(t, errors[0])
}
