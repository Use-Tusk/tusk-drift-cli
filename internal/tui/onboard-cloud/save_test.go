package onboardcloud

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestUpdateField_ExistingScalar(t *testing.T) {
	input := `service:
  id: old-id
  name: my-service`

	result := updateYAMLString(t, input, []string{"service", "id"}, "new-id")

	assert.Contains(t, result, "id: new-id")
	assert.Contains(t, result, "name: my-service")
}

func TestUpdateField_ExistingBool(t *testing.T) {
	input := `recording:
  export_spans: false`

	result := updateYAMLString(t, input, []string{"recording", "export_spans"}, true)

	assert.Contains(t, result, "export_spans: true")
}

func TestUpdateField_ExistingFloat(t *testing.T) {
	input := `recording:
  sampling_rate: 0.5`

	result := updateYAMLString(t, input, []string{"recording", "sampling_rate"}, 1.0)

	assert.Contains(t, result, "sampling_rate: 1")
	// Should NOT contain explicit type tag
	assert.NotContains(t, result, "!!float")
}

func TestUpdateField_ClearsExplicitTags(t *testing.T) {
	// Simulate a file that was previously saved with explicit tags
	input := `recording:
  sampling_rate: !!float 1
  export_spans: !!bool true`

	result := updateYAMLString(t, input, []string{"recording", "sampling_rate"}, 0.5)

	// The updated field should not have explicit tag
	assert.Contains(t, result, "sampling_rate: 0.5")
	assert.NotContains(t, result, "!!float")
}

func TestUpdateField_CreatesNewField(t *testing.T) {
	input := `service:
  name: my-service`

	result := updateYAMLString(t, input, []string{"service", "id"}, "new-id")

	assert.Contains(t, result, "id: new-id")
	assert.Contains(t, result, "name: my-service")
}

func TestUpdateField_CreatesNestedStructure(t *testing.T) {
	input := `service:
  name: my-service`

	result := updateYAMLString(t, input, []string{"custom_section", "custom_key"}, "custom_value")

	assert.Contains(t, result, "custom_section:")
	assert.Contains(t, result, "custom_key: custom_value")
}

func TestUpdateField_PreservesComments(t *testing.T) {
	input := `recording:
  # This is a comment about sampling rate
  sampling_rate: 0.5 # inline comment
  export_spans: true`

	result := updateYAMLString(t, input, []string{"recording", "sampling_rate"}, 1.0)

	// Comments should be preserved
	assert.Contains(t, result, "# This is a comment about sampling rate")
	assert.Contains(t, result, "sampling_rate: 1")
}

func TestUpdateField_PreservesOtherFields(t *testing.T) {
	input := `service:
  id: svc-123
  name: my-service
  port: 8080
recording:
  sampling_rate: 0.5`

	result := updateYAMLString(t, input, []string{"recording", "sampling_rate"}, 1.0)

	// Other fields should be unchanged
	assert.Contains(t, result, "id: svc-123")
	assert.Contains(t, result, "name: my-service")
	assert.Contains(t, result, "port: 8080")
	assert.Contains(t, result, "sampling_rate: 1")
}

func TestAddBlankLinesBetweenSections(t *testing.T) {
	input := `service:
  name: test
recording:
  sampling_rate: 1`

	result := addBlankLinesBetweenSections([]byte(input))

	lines := strings.Split(string(result), "\n")
	// Find the line index of "recording:"
	recordingIdx := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "recording:") {
			recordingIdx = i
			break
		}
	}

	require.NotEqual(t, -1, recordingIdx, "recording: not found")
	require.Greater(t, recordingIdx, 0, "recording: should not be first line")

	// The line before "recording:" should be blank
	assert.Equal(t, "", strings.TrimSpace(lines[recordingIdx-1]))
}

func TestSaveRecordingConfig_Integration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tusk-test-*")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(tmpDir)
		config.Invalidate()
	})

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(originalWd) })

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	// Create .tusk directory and config file
	tuskDir := filepath.Join(tmpDir, ".tusk")
	err = os.MkdirAll(tuskDir, 0o750)
	require.NoError(t, err)

	initialConfig := `service:
  name: test-service

recording:
  sampling_rate: 0.5
  export_spans: false
  enable_env_var_recording: false
`
	err = os.WriteFile(filepath.Join(tuskDir, "config.yaml"), []byte(initialConfig), 0o600)
	require.NoError(t, err)

	// Save new recording config
	err = saveRecordingConfig(1.0, true, true)
	require.NoError(t, err)

	// Read the file back
	data, err := os.ReadFile(filepath.Join(tuskDir, "config.yaml")) // #nosec G304
	require.NoError(t, err)

	result := string(data)
	assert.Contains(t, result, "sampling_rate: 1")
	assert.Contains(t, result, "export_spans: true")
	assert.Contains(t, result, "enable_env_var_recording: true")
	assert.NotContains(t, result, "!!float")
	assert.NotContains(t, result, "!!bool")
}

// Helper function to test updateField without file I/O
func updateYAMLString(t *testing.T, input string, path []string, value any) string {
	t.Helper()

	var node yaml.Node
	err := yaml.Unmarshal([]byte(input), &node)
	require.NoError(t, err)

	err = updateYAMLNode(&node, []configUpdate{{path: path, value: value}})
	require.NoError(t, err)

	var buf strings.Builder
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	err = encoder.Encode(&node)
	require.NoError(t, err)
	_ = encoder.Close()

	return buf.String()
}
