package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestValidateConfigFile_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	tuskDir := filepath.Join(tmpDir, ".tusk")
	_ = os.MkdirAll(tuskDir, 0o750)

	validConfig := `service:
  name: test-service
  port: 3000
  start:
    command: npm start
`
	configPath := filepath.Join(tuskDir, "config.yaml")
	_ = os.WriteFile(configPath, []byte(validConfig), 0o600)

	result := ValidateConfigFile(configPath)

	if !result.Valid {
		t.Errorf("Expected valid config, got errors: %v", result.Errors)
	}
	if len(result.UnknownKeys) > 0 {
		t.Errorf("Expected no unknown keys, got: %v", result.UnknownKeys)
	}
}

func TestValidateConfigFile_UnknownKey(t *testing.T) {
	tmpDir := t.TempDir()
	tuskDir := filepath.Join(tmpDir, ".tusk")
	_ = os.MkdirAll(tuskDir, 0o750)
	Invalidate()

	// start_command is a common mistake - should be service.start.command
	badConfig := `service:
  name: test-service
  port: 3000
  start_command: npm start
`
	configPath := filepath.Join(tuskDir, "config.yaml")
	_ = os.WriteFile(configPath, []byte(badConfig), 0o600)

	result := ValidateConfigFile(configPath)

	if len(result.UnknownKeys) == 0 {
		t.Error("Expected unknown keys to be detected")
	}

	foundStartCommand := false
	for _, key := range result.UnknownKeys {
		if key == "service.start_command" {
			foundStartCommand = true
			break
		}
	}
	if !foundStartCommand {
		t.Errorf("Expected 'service.start_command' in unknown keys, got: %v", result.UnknownKeys)
	}

	// Should have a warning suggesting the correct key
	if len(result.Warnings) == 0 {
		t.Error("Expected warnings about unknown keys")
	}
}

func TestValidateConfigFile_MissingRequired(t *testing.T) {
	tmpDir := t.TempDir()
	tuskDir := filepath.Join(tmpDir, ".tusk")
	_ = os.MkdirAll(tuskDir, 0o750)
	Invalidate()

	// Missing start.command
	missingConfig := `service:
  name: test-service
  port: 3000
`
	configPath := filepath.Join(tuskDir, "config.yaml")
	_ = os.WriteFile(configPath, []byte(missingConfig), 0o600)

	result := ValidateConfigFile(configPath)

	if result.Valid {
		t.Error("Expected invalid config due to missing start.command")
	}

	foundMissing := false
	for _, key := range result.MissingKeys {
		if key == "service.start.command" {
			foundMissing = true
			break
		}
	}
	if !foundMissing {
		t.Errorf("Expected 'service.start.command' in missing keys, got: %v", result.MissingKeys)
	}
}

func TestValidateConfigFile_NotFound(t *testing.T) {
	Invalidate()
	result := ValidateConfigFile("/nonexistent/path/config.yaml")

	if result.Valid {
		t.Error("Expected invalid result for nonexistent file")
	}
	if len(result.Errors) == 0 {
		t.Error("Expected error message for nonexistent file")
	}
}

func TestCheckUnknownKeys(t *testing.T) {
	// Test that our valid key extraction works
	validKeys := getValidKeys(reflect.TypeOf(Config{}), "")

	// Should include nested keys
	expectedKeys := []string{
		"service",
		"service.name",
		"service.port",
		"service.start",
		"service.start.command",
		"service.readiness_check",
		"service.readiness_check.command",
	}

	keySet := make(map[string]bool)
	for _, k := range validKeys {
		keySet[k] = true
	}

	for _, expected := range expectedKeys {
		if !keySet[expected] {
			t.Errorf("Expected valid key %q not found in: %v", expected, validKeys)
		}
	}
}
