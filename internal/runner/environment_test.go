package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartEnvironment(t *testing.T) {
	tests := []struct {
		name          string
		setupFunc     func(t *testing.T) (*Executor, func())
		expectError   bool
		errorContains string
		verifyFunc    func(t *testing.T, e *Executor)
	}{
		{
			name: "successful_start_all_components_with_mock_sdk",
			setupFunc: func(t *testing.T) (*Executor, func()) {
				e := NewExecutor()
				// Create a test config
				tempDir := t.TempDir()
				configPath := filepath.Join(tempDir, "tusk.yaml")
				configContent := `
service:
  id: test-service
  port: 14001
  start:
    command: "` + getMediumSleepCommand() + `"
`
				err := os.WriteFile(configPath, []byte(configContent), 0o600)
				require.NoError(t, err)

				err = config.Load(configPath)
				require.NoError(t, err)

				// Speed up test
				origVal := os.Getenv("TUSK_TEST_DEFAULT_WAIT")
				_ = os.Setenv("TUSK_TEST_DEFAULT_WAIT", "100ms")

				// Start a goroutine to simulate SDK connection
				go func() {
					// Wait for server to be created
					for range 50 {
						if e.server != nil {
							time.Sleep(100 * time.Millisecond)
							// Simulate SDK connection
							e.server.mu.Lock()
							if !e.server.sdkConnected {
								e.server.sdkConnected = true
								close(e.server.sdkConnectedChan)
							}
							e.server.mu.Unlock()
							break
						}
						time.Sleep(100 * time.Millisecond)
					}
				}()

				return e, func() {
					if e.serviceCmd != nil && e.serviceCmd.Process != nil {
						_ = e.StopService()
					}
					if e.server != nil {
						_ = e.StopServer()
					}
					if origVal != "" {
						_ = os.Setenv("TUSK_TEST_DEFAULT_WAIT", origVal)
					} else {
						_ = os.Unsetenv("TUSK_TEST_DEFAULT_WAIT")
					}
				}
			},
			expectError: false,
			verifyFunc: func(t *testing.T, e *Executor) {
				assert.NotNil(t, e.server)
				assert.NotNil(t, e.serviceCmd)
			},
		},
		{
			name: "failure_service_start_cleans_up_server",
			setupFunc: func(t *testing.T) (*Executor, func()) {
				e := NewExecutor()
				// Create config with invalid command
				tempDir := t.TempDir()
				configPath := filepath.Join(tempDir, "tusk.yaml")
				configContent := `
service:
  id: test-service
  port: 14002
  start:
    command: ""
`
				err := os.WriteFile(configPath, []byte(configContent), 0o600)
				require.NoError(t, err)

				err = config.Load(configPath)
				require.NoError(t, err)

				return e, func() {
					if e.server != nil {
						_ = e.StopServer()
					}
				}
			},
			expectError:   true,
			errorContains: "start service",
			verifyFunc: func(t *testing.T, e *Executor) {
				// Server should be created but stopped after service fails
				assert.NotNil(t, e.server)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.ResetForTesting()

			executor, cleanup := tt.setupFunc(t)
			defer cleanup()

			err := executor.StartEnvironment()

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			if tt.verifyFunc != nil {
				tt.verifyFunc(t, executor)
			}
		})
	}
}

func TestStopEnvironment(t *testing.T) {
	tests := []struct {
		name          string
		setupFunc     func() *Executor
		expectError   bool
		errorContains string
	}{
		{
			name: "successful_stop_both_components",
			setupFunc: func() *Executor {
				e := NewExecutor()
				// Create mock server
				server, _ := NewServer("test")
				_ = server.Start()
				e.server = server

				// Create mock service command using platform-specific helper
				ctx := context.Background()
				cmd := createTestCommand(ctx, "1")
				_ = cmd.Start()
				e.serviceCmd = cmd

				return e
			},
			expectError: false,
		},
		{
			name:        "no_components_running",
			setupFunc:   NewExecutor,
			expectError: false,
		},
		{
			name: "service_stop_fails_but_server_stops",
			setupFunc: func() *Executor {
				e := NewExecutor()
				// Create mock server
				server, _ := NewServer("test")
				_ = server.Start()
				e.server = server

				// Service command is nil, so StopService will succeed but we simulate an error scenario
				// by having a process that already exited
				ctx := context.Background()
				cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "exit 0")
				_ = cmd.Start()
				_ = cmd.Wait() // Process already exited
				e.serviceCmd = cmd

				return e
			},
			expectError: false, // StopService handles already-exited processes gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.ResetForTesting()

			executor := tt.setupFunc()

			err := executor.StopEnvironment()

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			// Clean up any remaining resources
			if executor.server != nil {
				_ = executor.server.Stop()
			}
			if executor.serviceCmd != nil && executor.serviceCmd.Process != nil {
				_ = executor.serviceCmd.Process.Kill()
			}
		})
	}
}

func TestStartServer(t *testing.T) {
	tests := []struct {
		name          string
		setupFunc     func(t *testing.T) (*Executor, func())
		expectError   bool
		errorContains string
	}{
		{
			name: "successful_start_with_suite_spans",
			setupFunc: func(t *testing.T) (*Executor, func()) {
				e := NewExecutor()
				// Add suite spans
				e.suiteSpans = []*core.Span{
					{PackageName: "test-pkg", SubmoduleName: "test-module"},
				}

				// Create valid config
				tempDir := t.TempDir()
				configPath := filepath.Join(tempDir, "tusk.yaml")
				configContent := `
service:
  id: test-service-with-spans
`
				err := os.WriteFile(configPath, []byte(configContent), 0o600)
				require.NoError(t, err)

				err = config.Load(configPath)
				require.NoError(t, err)

				return e, func() {
					if e.server != nil {
						_ = e.server.Stop()
					}
				}
			},
			expectError: false,
		},
		{
			name: "successful_start_without_suite_spans",
			setupFunc: func(t *testing.T) (*Executor, func()) {
				e := NewExecutor()

				// Create valid config
				tempDir := t.TempDir()
				configPath := filepath.Join(tempDir, "tusk.yaml")
				configContent := `
service:
  id: test-service-no-spans
`
				err := os.WriteFile(configPath, []byte(configContent), 0o600)
				require.NoError(t, err)

				err = config.Load(configPath)
				require.NoError(t, err)

				return e, func() {
					if e.server != nil {
						_ = e.server.Stop()
					}
				}
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.ResetForTesting()

			executor, cleanup := tt.setupFunc(t)
			defer cleanup()

			err := executor.StartServer()

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, executor.server)

				// Verify suite spans were applied if they existed
				// This would require accessing private fields of the server
				// In a real test, we'd verify through behavior
			}
		})
	}
}

func TestStopServer(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func() *Executor
		expectError bool
	}{
		{
			name: "stop_running_server",
			setupFunc: func() *Executor {
				e := NewExecutor()
				server, _ := NewServer("test")
				_ = server.Start()
				e.server = server
				return e
			},
			expectError: false,
		},
		{
			name:        "stop_when_server_is_nil",
			setupFunc:   NewExecutor,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.ResetForTesting()

			executor := tt.setupFunc()

			err := executor.StopServer()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWaitForSDKAcknowledgement(t *testing.T) {
	tests := []struct {
		name          string
		setupFunc     func() *Executor
		expectError   bool
		errorContains string
	}{
		{
			name: "successful_acknowledgement",
			setupFunc: func() *Executor {
				e := NewExecutor()
				server, _ := NewServer("test")
				_ = server.Start()
				e.server = server

				// Simulate SDK connection
				go func() {
					time.Sleep(50 * time.Millisecond)
					server.mu.Lock()
					if !server.sdkConnected {
						server.sdkConnected = true
						close(server.sdkConnectedChan)
					}
					server.mu.Unlock()
				}()

				return e
			},
			expectError: false,
		},
		{
			name:          "failure_server_not_started",
			setupFunc:     NewExecutor,
			expectError:   true,
			errorContains: "mock server not started",
		},
		{
			name: "failure_timeout",
			setupFunc: func() *Executor {
				e := NewExecutor()
				server, _ := NewServer("test")
				_ = server.Start()
				e.server = server
				// Don't simulate SDK connection, let it timeout
				return e
			},
			expectError:   true,
			errorContains: "timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.ResetForTesting()

			// Set short timeout for timeout test
			var origVal string
			if tt.name == "failure_timeout" {
				origVal = os.Getenv("TUSK_TEST_DEFAULT_WAIT")
				_ = os.Setenv("TUSK_TEST_DEFAULT_WAIT", "100ms")
				defer func() {
					if origVal != "" {
						_ = os.Setenv("TUSK_TEST_DEFAULT_WAIT", origVal)
					} else {
						_ = os.Unsetenv("TUSK_TEST_DEFAULT_WAIT")
					}
				}()
			}

			executor := tt.setupFunc()
			defer func() {
				if executor.server != nil {
					_ = executor.server.Stop()
				}
			}()

			err := executor.WaitForSDKAcknowledgement()

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestStartServerWithSuiteSpans(t *testing.T) {
	config.ResetForTesting()

	e := NewExecutor()

	// Add suite spans before starting server
	testSpans := []*core.Span{
		{PackageName: "pkg1", SubmoduleName: "mod1"},
		{PackageName: "pkg2", SubmoduleName: "mod2"},
	}
	e.suiteSpans = testSpans

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "tusk.yaml")
	configContent := `
service:
  id: test-spans-service
`
	err := os.WriteFile(configPath, []byte(configContent), 0o600)
	require.NoError(t, err)

	err = config.Load(configPath)
	require.NoError(t, err)

	err = e.StartServer()
	require.NoError(t, err)
	defer func() { _ = e.StopServer() }()

	// Verify server was created and started
	assert.NotNil(t, e.server)

	// In a real scenario, we'd verify the spans were actually set on the server
	// but that would require exposing internal server state or testing through behavior
}

func TestEnvironmentCleanupOnFailure(t *testing.T) {
	config.ResetForTesting()

	e := NewExecutor()

	// Config that will fail at service start (no start command)
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "tusk.yaml")
	configContent := `
service:
  id: cleanup-test-service
  port: 14005
  start:
    command: ""
`
	err := os.WriteFile(configPath, []byte(configContent), 0o600)
	require.NoError(t, err)

	err = config.Load(configPath)
	require.NoError(t, err)

	err = e.StartEnvironment()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "start service")

	// Server should be created but the environment start should have cleaned it up
	// We can verify by checking if we can create a new server with the same service ID
	// (if the old one wasn't cleaned up properly, this might fail)
	newServer, err := NewServer("cleanup-test-service")
	require.NoError(t, err)
	defer func() { _ = newServer.Stop() }()

	err = newServer.Start()
	assert.NoError(t, err)
}
