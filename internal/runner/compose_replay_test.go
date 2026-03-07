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
	assert.True(t, isComposeBasedStartCommand("docker compose -f docker-compose.tusk-override.yml up"))
	assert.True(t, isComposeBasedStartCommand("ENV=test docker-compose -f docker-compose.tusk-override.yml up -d"))
	assert.True(t, isComposeBasedStartCommand("./.tusk/bin/tusk-compose -f docker-compose.tusk-override.yml up"))

	assert.False(t, isComposeBasedStartCommand("docker compose up"))
	assert.False(t, isComposeBasedStartCommand("docker run --rm alpine echo hello"))
	assert.False(t, isComposeBasedStartCommand("go run ./cmd/server"))
}

func TestInjectComposeOverrideFile(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		override string
		want     string
		injected bool
	}{
		{
			name:     "appends_override_after_source_file_flag",
			command:  "./.tusk/bin/tusk-compose -f docker-compose.local.yaml -f docker-compose.tusk-override.yml up",
			override: "/tmp/tusk-replay-env-override-staging-us-east-123456.yml",
			want:     "./.tusk/bin/tusk-compose -f docker-compose.local.yaml -f docker-compose.tusk-override.yml -f /tmp/tusk-replay-env-override-staging-us-east-123456.yml up",
			injected: true,
		},
		{
			name:     "supports_double_quoted_file_arg",
			command:  `docker compose --file "${COMPOSE_DIR}/docker-compose.tusk-override.yml" up`,
			override: "/tmp/tusk-replay-env-override-staging-us-east-123456.yml",
			want:     `docker compose --file "${COMPOSE_DIR}/docker-compose.tusk-override.yml" --file "/tmp/tusk-replay-env-override-staging-us-east-123456.yml" up`,
			injected: true,
		},
		{
			name:     "escapes_dollar_and_backtick_when_original_file_arg_is_double_quoted",
			command:  "docker compose --file \"${COMPOSE_DIR}/docker-compose.tusk-override.yml\" up",
			override: "/tmp/replay-$FOO-`bar`.yml",
			want:     "docker compose --file \"${COMPOSE_DIR}/docker-compose.tusk-override.yml\" --file \"/tmp/replay-\\$FOO-\\`bar\\`.yml\" up",
			injected: true,
		},
		{
			name:     "non_compose_command_unchanged",
			command:  "go test ./...",
			override: "/tmp/tusk-replay-env-override-staging-us-east-123456.yml",
			want:     "go test ./...",
			injected: false,
		},
		{
			name:     "supports_compound_commands",
			command:  "cd /app && docker compose -f docker-compose.tusk-override.yml up",
			override: "/tmp/tusk-replay-env-override-staging-us-east-123456.yml",
			want:     "cd /app && docker compose -f docker-compose.tusk-override.yml -f /tmp/tusk-replay-env-override-staging-us-east-123456.yml up",
			injected: true,
		},
		{
			name:     "supports_bash_lc_with_literal_compose_file_arg",
			command:  "bash -lc 'docker compose -f docker-compose.tusk-override.yml up'",
			override: "/tmp/tusk-replay-env-override-staging-us-east-123456.yml",
			want:     "bash -lc 'docker compose -f docker-compose.tusk-override.yml -f /tmp/tusk-replay-env-override-staging-us-east-123456.yml up'",
			injected: true,
		},
		{
			name:     "equal_form_file_flag_replaced",
			command:  "docker compose --file=./docker-compose.tusk-override.yml up",
			override: "/tmp/tusk-replay-env-override-staging-us-east-123456.yml",
			want:     "docker compose --file=./docker-compose.tusk-override.yml --file=/tmp/tusk-replay-env-override-staging-us-east-123456.yml up",
			injected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, injected, err := injectComposeOverrideFile(tt.command, tt.override)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.injected, injected)
		})
	}
}

func TestInjectComposeOverrideFile_PreservesShellExpansion(t *testing.T) {
	command := "docker compose -f $COMPOSE_DIR/docker-compose.tusk-override.yml up"
	override := "/tmp/tusk-replay-env-override-staging-us-east-123456.yml"

	got, injected, err := injectComposeOverrideFile(command, override)
	require.NoError(t, err)
	assert.True(t, injected)
	assert.Equal(t, "docker compose -f $COMPOSE_DIR/docker-compose.tusk-override.yml -f /tmp/tusk-replay-env-override-staging-us-east-123456.yml up", got)
	assert.Contains(t, got, "$COMPOSE_DIR/docker-compose.tusk-override.yml")
}

func TestInjectComposeOverrideFile_KnownLimitations(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{
			name:    "does_not_inject_when_file_arg_is_indirect_variable",
			command: `docker compose -f "$COMPOSE_DIR/$OVERRIDE_FILE" up`,
		},
		{
			name:    "does_not_inject_when_compose_file_is_set_via_env",
			command: "COMPOSE_FILE=docker-compose.tusk-override.yml docker compose up",
		},
		{
			name:    "does_not_inject_when_file_arg_uses_command_substitution",
			command: `docker compose -f "$(pwd)/$(echo docker-compose.tusk-override.yml)" up`,
		},
		{
			name:    "does_not_inject_when_file_arg_uses_process_substitution",
			command: `docker compose -f <(echo docker-compose.tusk-override.yml) up`,
		},
		{
			name:    "does_not_inject_when_compose_file_is_only_selected_from_env_list",
			command: `COMPOSE_FILE=docker-compose.tusk-override.yml:docker-compose.dev.yml docker compose up`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, injected, err := injectComposeOverrideFile(tt.command, "/tmp/tusk-replay-env-override-staging-us-east-123456.yml")
			require.NoError(t, err)
			assert.False(t, injected)
			assert.Equal(t, tt.command, got)
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
	filteredEnvVars, _ := filterReplayEnvVarsForCompose(envVars)

	overridePath, err := createReplayComposeOverrideFile(filteredEnvVars, "staging/us-east")
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
		"API_KEY":      "${API_KEY}",
		"QUOTED_VALUE": "${QUOTED_VALUE}",
		"MULTILINE":    "${MULTILINE}",
	}, parsed.Services["django"].Environment)
	assert.Equal(t, map[string]string{
		"API_KEY":      "${API_KEY}",
		"QUOTED_VALUE": "${QUOTED_VALUE}",
		"MULTILINE":    "${MULTILINE}",
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
		map[string]string{"API_KEY": "abc"},
		"development",
	)
	require.NoError(t, err)
	assert.Empty(t, overridePath)
}

func TestExtractComposeServiceNames_FindsNestedSourceFile(t *testing.T) {
	originalWD, err := os.Getwd()
	require.NoError(t, err)

	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	defer func() {
		_ = os.Chdir(originalWD)
	}()

	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".tusk"), 0o750))
	nestedDir := filepath.Join(tempDir, "services", "api")
	require.NoError(t, os.MkdirAll(nestedDir, 0o750))

	composeContent := `
services:
  web:
    image: web
  worker:
    image: worker
`
	require.NoError(t, os.WriteFile(filepath.Join(nestedDir, replayComposeServiceSourceFile), []byte(composeContent), 0o600))

	serviceNames, extractErr := extractComposeServiceNames()
	require.NoError(t, extractErr)
	assert.Equal(t, []string{"web", "worker"}, serviceNames)
}

func TestExtractComposeServiceNames_ErrorsWhenMultipleNestedSourcesExist(t *testing.T) {
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
  api:
    image: api
`
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "a"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "b"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "a", replayComposeServiceSourceFile), []byte(composeContent), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "b", replayComposeServiceSourceFile), []byte(composeContent), 0o600))

	_, extractErr := extractComposeServiceNames()
	require.Error(t, extractErr)
	assert.Contains(t, extractErr.Error(), "multiple "+replayComposeServiceSourceFile+" files found")
}

func TestFilterReplayEnvVarsForCompose(t *testing.T) {
	input := map[string]string{
		"API_KEY":         "abc",
		"TUSK_DRIFT_MODE": "RECORD",
		"TUSK_MOCK_PORT":  "9999",
		"PATH":            "/usr/local/bin",
	}

	filtered, skipped := filterReplayEnvVarsForCompose(input)

	assert.Equal(t, map[string]string{
		"API_KEY": "abc",
		"PATH":    "/usr/local/bin",
	}, filtered)
	assert.Equal(t, []string{"TUSK_DRIFT_MODE", "TUSK_MOCK_PORT"}, skipped)
}
