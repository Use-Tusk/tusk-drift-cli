package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// evalSymlinks is a helper that resolves symlinks for path comparison
// On macOS, /var is a symlink to /private/var which causes test failures
func evalSymlinks(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		// If we can't resolve, just return the original
		return path
	}
	return resolved
}

func TestNestedRecordingSamplingConfig(t *testing.T) {
	defer Invalidate()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
recording:
  sampling:
    mode: adaptive
    base_rate: 0.25
    min_rate: 0.05
`), 0o600))

	require.NoError(t, Load(configPath))

	cfg, err := Get()
	require.NoError(t, err)
	assert.Equal(t, "adaptive", cfg.Recording.Sampling.Mode)
	require.NotNil(t, cfg.Recording.Sampling.BaseRate)
	assert.Equal(t, 0.25, *cfg.Recording.Sampling.BaseRate)
	require.NotNil(t, cfg.Recording.Sampling.MinRate)
	assert.Equal(t, 0.05, *cfg.Recording.Sampling.MinRate)
	assert.Equal(t, 0.25, cfg.Recording.SamplingRate)
}

func TestLegacyRecordingSamplingRateBackfillsNestedSamplingConfig(t *testing.T) {
	defer Invalidate()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
recording:
  sampling_rate: 0.25
`), 0o600))

	require.NoError(t, Load(configPath))

	cfg, err := Get()
	require.NoError(t, err)
	assert.Equal(t, 0.25, cfg.Recording.SamplingRate)
	assert.Equal(t, "fixed", cfg.Recording.Sampling.Mode)
	require.NotNil(t, cfg.Recording.Sampling.BaseRate)
	assert.Equal(t, 0.25, *cfg.Recording.Sampling.BaseRate)
	assert.Nil(t, cfg.Recording.Sampling.MinRate)
}

func TestRecordingSamplingRateEnvOverrideBeatsNestedBaseRate(t *testing.T) {
	defer Invalidate()
	t.Setenv("TUSK_RECORDING_SAMPLING_RATE", "0.5")

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
recording:
  sampling:
    mode: adaptive
    base_rate: 0.25
    min_rate: 0.05
`), 0o600))

	require.NoError(t, Load(configPath))

	cfg, err := Get()
	require.NoError(t, err)
	require.NotNil(t, cfg.Recording.Sampling.BaseRate)
	assert.Equal(t, 0.5, *cfg.Recording.Sampling.BaseRate)
	assert.Equal(t, 0.5, cfg.Recording.SamplingRate)
	assert.Equal(t, "adaptive", cfg.Recording.Sampling.Mode)
	require.NotNil(t, cfg.Recording.Sampling.MinRate)
	assert.Equal(t, 0.05, *cfg.Recording.Sampling.MinRate)
}

func TestValidateRejectsInvalidRecordingSamplingMode(t *testing.T) {
	cfg := &Config{
		Service: ServiceConfig{
			Port: 3000,
			Communication: CommunicationConfig{
				Type:    "auto",
				TCPPort: 9001,
			},
		},
		Recording: RecordingConfig{
			Sampling: RecordingSamplingConfig{
				Mode: "adapttive",
			},
		},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.ErrorContains(t, err, "recording.sampling.mode must be 'fixed' or 'adaptive'")
}

func TestValidateRejectsOutOfRangeRecordingSamplingRates(t *testing.T) {
	baseRate := 5.0
	minRate := -0.1
	cfg := &Config{
		Service: ServiceConfig{
			Port: 3000,
			Communication: CommunicationConfig{
				Type:    "auto",
				TCPPort: 9001,
			},
		},
		Recording: RecordingConfig{
			Sampling: RecordingSamplingConfig{
				Mode:     "adaptive",
				BaseRate: &baseRate,
				MinRate:  &minRate,
			},
		},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.ErrorContains(t, err, "recording.sampling.base_rate must be between 0.0 and 1.0")
	assert.ErrorContains(t, err, "recording.sampling.min_rate must be between 0.0 and 1.0")
}

func TestValidateRejectsRecordingMinRateGreaterThanBaseRate(t *testing.T) {
	baseRate := 0.1
	minRate := 0.9
	cfg := &Config{
		Service: ServiceConfig{
			Port: 3000,
			Communication: CommunicationConfig{
				Type:    "auto",
				TCPPort: 9001,
			},
		},
		Recording: RecordingConfig{
			Sampling: RecordingSamplingConfig{
				Mode:     "adaptive",
				BaseRate: &baseRate,
				MinRate:  &minRate,
			},
		},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.ErrorContains(t, err, "recording.sampling.min_rate must be less than or equal to recording.sampling.base_rate")
}

func TestFindConfigFile_ParentTraversal(t *testing.T) {
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()
	defer Invalidate()

	tmp := evalSymlinks(t.TempDir())
	configPath := filepath.Join(tmp, ".tusk", "config.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o750))
	require.NoError(t, os.WriteFile(configPath, []byte("service:\n  name: test"), 0o600))

	// Work from a subdirectory
	subdir := filepath.Join(tmp, "src", "handlers")
	require.NoError(t, os.MkdirAll(subdir, 0o750))
	require.NoError(t, os.Chdir(subdir))

	found := findConfigFile()
	assert.Equal(t, configPath, found)
}

func TestFindConfigFile_ClosestWins(t *testing.T) {
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()
	defer Invalidate()

	// Create structure: tmp/.tusk/config.yaml and tmp/nested/.tusk/config.yaml
	tmp := evalSymlinks(t.TempDir())
	outerConfig := filepath.Join(tmp, ".tusk", "config.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(outerConfig), 0o750))
	require.NoError(t, os.WriteFile(outerConfig, []byte("service:\n  name: outer"), 0o600))

	nestedRoot := filepath.Join(tmp, "nested")
	innerConfig := filepath.Join(nestedRoot, ".tusk", "config.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(innerConfig), 0o750))
	require.NoError(t, os.WriteFile(innerConfig, []byte("service:\n  name: inner"), 0o600))

	subdir := filepath.Join(nestedRoot, "src")
	require.NoError(t, os.MkdirAll(subdir, 0o750))
	require.NoError(t, os.Chdir(subdir))

	// Should find the closest config (in nested/.tusk/, not tmp/.tusk/)
	found := findConfigFile()
	assert.Equal(t, innerConfig, found)
}

func TestFindConfigFile_RootLevel(t *testing.T) {
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()
	defer Invalidate()

	tmp := evalSymlinks(t.TempDir())
	configPath := filepath.Join(tmp, "tusk.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("service:\n  name: test"), 0o600))

	subdir := filepath.Join(tmp, "src")
	require.NoError(t, os.MkdirAll(subdir, 0o750))
	require.NoError(t, os.Chdir(subdir))

	found := findConfigFile()
	assert.Equal(t, configPath, found)
}

func TestConfigPaths_ResolvedRelativeToTuskRoot(t *testing.T) {
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()
	defer Invalidate()

	tmp := evalSymlinks(t.TempDir())
	tuskDir := filepath.Join(tmp, ".tusk")
	require.NoError(t, os.MkdirAll(tuskDir, 0o750))

	// Write config with relative paths
	configPath := filepath.Join(tuskDir, "config.yaml")
	configContent := `service:
  name: test-service
  port: 8080
results:
  dir: .tusk/results
traces:
  dir: .tusk/traces
`
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o600))

	// Change to a nested directory
	subdir := filepath.Join(tmp, "src", "api")
	require.NoError(t, os.MkdirAll(subdir, 0o750))
	require.NoError(t, os.Chdir(subdir))

	// Load config
	err := Load("")
	require.NoError(t, err)

	cfg, err := Get()
	require.NoError(t, err)

	// Paths should be resolved relative to tusk root (tmp), not current directory (tmp/src/api)
	assert.Equal(t, filepath.Join(tmp, ".tusk/results"), cfg.Results.Dir)
	assert.Equal(t, filepath.Join(tmp, ".tusk/traces"), cfg.Traces.Dir)
}
