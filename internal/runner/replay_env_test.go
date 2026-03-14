package runner

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilterReplayEnvVarsForCompose(t *testing.T) {
	input := map[string]string{
		"API_KEY":         "abc",
		"HOME":            "/usr/src/app",
		"NVM_BIN":         "/Users/test/.nvm/versions/node/v22/bin",
		"Path":            "/windows/system32",
		"PATH":            "/usr/local/bin",
		"TUSK_DRIFT_MODE": "RECORD",
		"Tusk_Mock_Host":  "localhost",
		"TUSK_MOCK_PORT":  "9999",
	}

	filtered, skipped := filterReplayEnvVarsForCompose(input)

	assert.Equal(t, map[string]string{
		"API_KEY": "abc",
	}, filtered)
	assert.Equal(t, []string{"HOME", "NVM_BIN", "PATH", "Path", "TUSK_DRIFT_MODE", "TUSK_MOCK_PORT", "Tusk_Mock_Host"}, skipped)
}

func TestFilterReplayEnvVarsForProcess(t *testing.T) {
	input := map[string]string{
		"API_KEY":        "abc",
		"HOME":           "/usr/src/app",
		"nvm_bin":        "/Users/test/.nvm/versions/node/v22/bin",
		"NVM_BIN":        "/Users/test/.nvm/versions/node/v22/bin",
		"Path":           "/windows/system32",
		"PATH":           "/usr/local/bin",
		"TUSK_CUSTOM_ID": "local-project-value",
	}

	filtered, skipped := filterReplayEnvVarsForProcess(input)

	assert.Equal(t, map[string]string{
		"API_KEY":        "abc",
		"TUSK_CUSTOM_ID": "local-project-value",
	}, filtered)
	assert.Equal(t, []string{"HOME", "NVM_BIN", "PATH", "Path", "nvm_bin"}, skipped)
}
