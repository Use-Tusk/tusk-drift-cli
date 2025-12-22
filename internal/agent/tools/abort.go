package tools

import (
	"encoding/json"
	"errors"
)

// ErrSetupAborted is a sentinel error for checking abort status
var ErrSetupAborted = errors.New("setup aborted")

// AbortError wraps ErrSetupAborted with the reason and detected project type
type AbortError struct {
	Reason      string
	ProjectType string
}

func (e *AbortError) Error() string {
	if e.Reason != "" {
		return e.Reason
	}
	return "setup aborted"
}

func (e *AbortError) Is(target error) bool {
	return target == ErrSetupAborted
}

// AbortSetup handles graceful exit when setup cannot proceed
// This is used when the agent detects an unsupported project type
func AbortSetup(input json.RawMessage) (string, error) {
	var params struct {
		Reason      string `json:"reason"`
		ProjectType string `json:"project_type"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", err
	}

	reason := params.Reason
	if reason == "" {
		reason = "Setup aborted by agent"
	}

	return "Setup aborted.", &AbortError{Reason: reason, ProjectType: params.ProjectType}
}
