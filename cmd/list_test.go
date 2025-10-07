package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
	"github.com/Use-Tusk/tusk-drift-cli/internal/runner"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// evalSymlinks resolves symlinks for path comparison
// On macOS, /var is a symlink to /private/var which causes test failures
func evalSymlinks(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}
	return resolved
}

func TestListTests_LoadsFromConfigTracesDir_WhenNoFlagProvided(t *testing.T) {
	// Save original state and restore after test
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(origWd)
		config.ResetForTesting()
		utils.SetTracesDirOverride("")
		traceDir = "" // Reset package-level flag variable
	}()

	// Create temp directory structure
	tmp := evalSymlinks(t.TempDir())

	// Create .tusk directory
	tuskDir := filepath.Join(tmp, ".tusk")
	require.NoError(t, os.MkdirAll(tuskDir, 0o750))

	// Create custom traces directory
	customTracesDir := filepath.Join(tmp, "custom-traces")
	require.NoError(t, os.MkdirAll(customTracesDir, 0o750))

	// Create config file with custom traces.dir
	configPath := filepath.Join(tuskDir, "config.yaml")
	configContent := `service:
  name: test-service
  port: 8080
traces:
  dir: custom-traces
`
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o600))

	// Create a trace file in the custom directory
	traceFilePath := filepath.Join(customTracesDir, "test-trace.jsonl")
	traceSpan := map[string]any{
		"traceId":       "test-trace-123",
		"spanId":        "span-001",
		"name":          "test-operation",
		"packageName":   "http",
		"submoduleName": "GET",
		"isRootSpan":    true,
		"inputValue": map[string]any{
			"method": "GET",
			"target": "/api/test",
		},
		"outputValue": map[string]any{
			"statusCode": 200,
		},
		"timestamp": map[string]any{
			"seconds": 1710000000,
			"nanos":   0,
		},
		"duration": map[string]any{
			"seconds": 0,
			"nanos":   100000000,
		},
	}

	traceData, err := json.Marshal(traceSpan)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(traceFilePath, append(traceData, '\n'), 0o600))

	// Change to temp directory so config can be found
	require.NoError(t, os.Chdir(tmp))

	// Reset config state and flags
	config.ResetForTesting()
	traceDir = "" // Ensure no --trace-dir flag is set

	// Test the directory selection logic from listTests
	// This replicates the logic from lines 78-98 of list.go

	// Load config (this should find our config file)
	err = config.Load("")
	require.NoError(t, err)

	cfg, getConfigErr := config.Get()
	require.NoError(t, getConfigErr)
	require.NotNil(t, cfg)

	// Determine which directory should be selected
	selected := traceDir

	if selected == "" && getConfigErr == nil && cfg.Traces.Dir != "" {
		selected = cfg.Traces.Dir
	}

	// Default to standard traces directory if nothing specified
	if selected == "" {
		selected = utils.GetTracesDir()
	} else if traceDir != "" {
		// Resolve --trace-dir flag relative to tusk root if it's a relative path
		selected = utils.ResolveTuskPath(selected)
	}

	// Verify the selected directory is our custom traces directory
	expectedPath := filepath.Join(tmp, "custom-traces")
	assert.Equal(t, expectedPath, selected, "Should select cfg.Traces.Dir when no --trace-dir flag is provided")

	// Now verify tests can actually be loaded from this directory
	if selected != "" {
		utils.SetTracesDirOverride(selected)
	}

	executor := runner.NewExecutor()
	tests, err := executor.LoadTestsFromFolder(selected)
	require.NoError(t, err)
	require.Len(t, tests, 1, "Should load exactly one test from custom traces directory")

	// Verify the loaded test has correct properties
	assert.Equal(t, "test-trace-123", tests[0].TraceID)
	assert.Equal(t, "test-trace.jsonl", tests[0].FileName)
	assert.Equal(t, "http", tests[0].Type)
	assert.Equal(t, "GET", tests[0].Method)
	assert.Equal(t, "/api/test", tests[0].Path)
	assert.Equal(t, 200, tests[0].Response.Status)
}

func TestListTests_LoadsFromTraceDirFlag_WhenFlagProvided(t *testing.T) {
	// Save original state and restore after test
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(origWd)
		config.ResetForTesting()
		utils.SetTracesDirOverride("")
		traceDir = ""
	}()

	// Create temp directory structure
	tmp := evalSymlinks(t.TempDir())

	// Create .tusk directory
	tuskDir := filepath.Join(tmp, ".tusk")
	require.NoError(t, os.MkdirAll(tuskDir, 0o750))

	// Create two different traces directories
	configTracesDir := filepath.Join(tmp, "config-traces")
	flagTracesDir := filepath.Join(tmp, "flag-traces")
	require.NoError(t, os.MkdirAll(configTracesDir, 0o750))
	require.NoError(t, os.MkdirAll(flagTracesDir, 0o750))

	// Create config file pointing to config-traces
	configPath := filepath.Join(tuskDir, "config.yaml")
	configContent := `service:
  name: test-service
  port: 8080
traces:
  dir: config-traces
`
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o600))

	// Create a trace file only in flag-traces directory
	traceFilePath := filepath.Join(flagTracesDir, "flag-trace.jsonl")
	traceSpan := map[string]any{
		"traceId":     "flag-trace-456",
		"spanId":      "span-002",
		"name":        "flag-operation",
		"packageName": "http",
		"isRootSpan":  true,
		"inputValue": map[string]any{
			"target": "/api/flag",
		},
	}

	traceData, err := json.Marshal(traceSpan)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(traceFilePath, append(traceData, '\n'), 0o600))

	// Change to temp directory
	require.NoError(t, os.Chdir(tmp))

	// Reset config state
	config.ResetForTesting()

	// Simulate --trace-dir flag being set
	traceDir = "flag-traces"

	// Load config
	err = config.Load("")
	require.NoError(t, err)

	cfg, getConfigErr := config.Get()
	require.NoError(t, getConfigErr)

	// Replicate directory selection logic from list.go
	selected := traceDir

	if selected == "" && getConfigErr == nil && cfg.Traces.Dir != "" {
		selected = cfg.Traces.Dir
	}

	if selected == "" {
		selected = utils.GetTracesDir()
	} else if traceDir != "" {
		// Resolve --trace-dir flag relative to tusk root if it's a relative path
		selected = utils.ResolveTuskPath(selected)
	}

	// Verify the selected directory is the flag directory, not config directory
	expectedPath := filepath.Join(tmp, "flag-traces")
	assert.Equal(t, expectedPath, selected, "Should prioritize --trace-dir flag over cfg.Traces.Dir")

	// Verify tests can be loaded from the flag directory
	if selected != "" {
		utils.SetTracesDirOverride(selected)
	}

	executor := runner.NewExecutor()
	tests, err := executor.LoadTestsFromFolder(selected)
	require.NoError(t, err)
	require.Len(t, tests, 1, "Should load test from flag-specified directory")
	assert.Equal(t, "flag-trace-456", tests[0].TraceID)
}

func TestListTests_UsesDefaultDir_WhenNoConfigAndNoFlag(t *testing.T) {
	// Save original state and restore after test
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(origWd)
		config.ResetForTesting()
		utils.SetTracesDirOverride("")
		traceDir = ""
	}()

	// Create temp directory without .tusk config
	tmp := evalSymlinks(t.TempDir())
	require.NoError(t, os.Chdir(tmp))

	// Reset state
	config.ResetForTesting()
	traceDir = ""

	// Load config (will fail to find config file, but should not error)
	_ = config.Load("")

	cfg, getConfigErr := config.Get()

	// Replicate directory selection logic
	selected := traceDir

	if selected == "" && getConfigErr == nil && cfg.Traces.Dir != "" {
		selected = cfg.Traces.Dir
	}

	// Default to standard traces directory if nothing specified
	if selected == "" {
		selected = utils.GetTracesDir()
	} else if traceDir != "" {
		selected = utils.ResolveTuskPath(selected)
	}

	// Should fall back to default traces directory
	assert.Contains(t, selected, ".tusk", "Should use default .tusk/traces directory when no config or flag provided")
	assert.Contains(t, selected, "traces", "Should use traces subdirectory by default")
}
