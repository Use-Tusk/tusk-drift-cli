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
		"PATH":            "/usr/local/bin",
		"TUSK_DRIFT_MODE": "RECORD",
		"TUSK_MOCK_PORT":  "9999",
	}

	filtered, skipped := filterReplayEnvVarsForCompose(input)

	assert.Equal(t, map[string]string{
		"API_KEY": "abc",
	}, filtered)
	assert.Equal(t, []string{"HOME", "NVM_BIN", "PATH", "TUSK_DRIFT_MODE", "TUSK_MOCK_PORT"}, skipped)
}

func TestFilterReplayEnvVarsForProcess(t *testing.T) {
	input := map[string]string{
		"API_KEY":        "abc",
		"HOME":           "/usr/src/app",
		"NVM_BIN":        "/Users/test/.nvm/versions/node/v22/bin",
		"PATH":           "/usr/local/bin",
		"TUSK_CUSTOM_ID": "local-project-value",
	}

	filtered, skipped := filterReplayEnvVarsForProcess(input)

	assert.Equal(t, map[string]string{
		"API_KEY":        "abc",
		"TUSK_CUSTOM_ID": "local-project-value",
	}, filtered)
	assert.Equal(t, []string{"HOME", "NVM_BIN", "PATH"}, skipped)
}
