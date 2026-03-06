package runner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestIsComposeBasedStartCommand(t *testing.T) {
	assert.True(t, isComposeBasedStartCommand("docker compose up"))
	assert.True(t, isComposeBasedStartCommand("ENV=test docker-compose up -d"))
	assert.True(t, isComposeBasedStartCommand("./.tusk/bin/tusk-compose -f docker-compose.yml up"))

	assert.False(t, isComposeBasedStartCommand("docker run --rm alpine echo hello"))
	assert.False(t, isComposeBasedStartCommand("go run ./cmd/server"))
}

func TestInjectComposeOverrideFile(t *testing.T) {
	tests := []struct {
		name       string
		command    string
		override   string
		want       string
		shouldFail bool
	}{
		{
			name:     "docker_compose",
			command:  "docker compose -f docker-compose.yml up",
			override: "/tmp/replay-env.yml",
			want:     "docker compose -f docker-compose.yml -f /tmp/replay-env.yml up",
		},
		{
			name:     "docker_compose_with_env_prefix",
			command:  "ENV=test docker-compose --project-name demo up -d",
			override: "/tmp/replay-env.yml",
			want:     "ENV=test docker-compose --project-name demo -f /tmp/replay-env.yml up -d",
		},
		{
			name:     "tusk_compose_wrapper",
			command:  "./.tusk/bin/tusk-compose -f docker-compose.local.yaml -f docker-compose.tusk-override.yml up",
			override: "/tmp/replay-env.yml",
			want:     "./.tusk/bin/tusk-compose -f docker-compose.local.yaml -f docker-compose.tusk-override.yml -f /tmp/replay-env.yml up",
		},
		{
			name:     "non_compose_command_unchanged",
			command:  "go test ./...",
			override: "/tmp/replay-env.yml",
			want:     "go test ./...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := injectComposeOverrideFile(tt.command, tt.override)
			if tt.shouldFail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCreateReplayComposeOverrideFile(t *testing.T) {
	originalWD, err := os.Getwd()
	require.NoError(t, err)

	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	defer func() {
		_ = os.Chdir(originalWD)
	}()

	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".tusk"), 0o750))

	composeContent := `
services:
  django:
    image: test
  worker:
    image: test
`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, replayComposeServiceSourceFile), []byte(composeContent), 0o600))

	envVars := map[string]string{
		"API_KEY":         `abc=123`,
		"QUOTED_VALUE":    `value "with quotes"`,
		"MULTILINE":       "line1\nline2",
		"TUSK_DRIFT_MODE": "RECORD",
		"TUSK_MOCK_PORT":  "9999",
	}

	overridePath, err := createReplayComposeOverrideFile("docker compose -f docker-compose.yml up", envVars, "staging/us-east")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Remove(overridePath)
	})
	assert.FileExists(t, overridePath)
	assert.Contains(t, filepath.Base(overridePath), "tusk-replay-env-override-staging-us-east-")
	assert.Contains(t, filepath.Base(overridePath), ".yml")
	assert.Contains(t, overridePath, os.TempDir())

	data, err := os.ReadFile(overridePath) // #nosec G304
	require.NoError(t, err)

	var parsed struct {
		Services map[string]struct {
			Environment map[string]string `yaml:"environment"`
		} `yaml:"services"`
	}
	require.NoError(t, yaml.Unmarshal(data, &parsed))
	require.Len(t, parsed.Services, 2)
	assert.Equal(t, map[string]string{
		"API_KEY":      `abc=123`,
		"QUOTED_VALUE": `value "with quotes"`,
		"MULTILINE":    "line1\nline2",
	}, parsed.Services["django"].Environment)
	assert.Equal(t, map[string]string{
		"API_KEY":      `abc=123`,
		"QUOTED_VALUE": `value "with quotes"`,
		"MULTILINE":    "line1\nline2",
	}, parsed.Services["worker"].Environment)
	assert.NotContains(t, parsed.Services["django"].Environment, "TUSK_DRIFT_MODE")
	assert.NotContains(t, parsed.Services["django"].Environment, "TUSK_MOCK_PORT")
}

func TestCreateReplayComposeOverrideFile_SkipsWhenSourceFileMissing(t *testing.T) {
	originalWD, err := os.Getwd()
	require.NoError(t, err)

	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	defer func() {
		_ = os.Chdir(originalWD)
	}()

	overridePath, err := createReplayComposeOverrideFile(
		"docker compose -f docker-compose.yml up",
		map[string]string{"API_KEY": "abc"},
		"development",
	)
	require.NoError(t, err)
	assert.Empty(t, overridePath)
}
