package tools

import (
	"encoding/json"
	"errors"
)

// ErrSetupAborted is returned when the agent decides to abort setup
var ErrSetupAborted = errors.New("setup aborted")

// AbortSetupResult contains the reason for aborting
type AbortSetupResult struct {
	Reason string
}

// AbortSetup handles graceful exit when setup cannot proceed
// This is used when the agent detects an unsupported project type
func AbortSetup(input json.RawMessage) (string, error) {
	var params struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", err
	}

	// Return minimal acknowledgment - the agent already explains the reason in its output
	return "Setup aborted.", ErrSetupAborted
}
