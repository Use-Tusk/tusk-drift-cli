package tools

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

// ResetPhaseProgress removes a specific phase from the progress file so it will run again on next setup.
// If no phase_name is provided, removes all cloud phases.
func ResetPhaseProgress(workDir string) func(json.RawMessage) (string, error) {
	return func(input json.RawMessage) (string, error) {
		var params struct {
			PhaseName string `json:"phase_name"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return "", err
		}

		progressPath := filepath.Clean(filepath.Join(workDir, ".tusk", "PROGRESS.md"))

		content, err := os.ReadFile(progressPath)
		if err != nil {
			if os.IsNotExist(err) {
				return "No progress file found, nothing to reset.", nil
			}
			return "", err
		}

		// Determine which phases to remove
		var phasesToRemove []string
		if params.PhaseName != "" {
			phasesToRemove = []string{params.PhaseName}
		} else {
			// If no specific phase, remove all cloud phases
			phasesToRemove = []string{
				"Cloud Auth",
				"Detect Repository",
				"Verify Access",
				"Create Service",
				"Create API Key",
				"Configure Recording",
				"Upload Traces",
				"Validate Suite",
				"Cloud Summary",
			}
		}

		lines := strings.Split(string(content), "\n")
		var newLines []string

		for _, line := range lines {
			shouldKeep := true
			for _, phase := range phasesToRemove {
				if strings.Contains(line, "- ✓ "+phase) {
					shouldKeep = false
					break
				}
			}
			if shouldKeep {
				newLines = append(newLines, line)
			}
		}

		newContent := strings.Join(newLines, "\n")
		if err := os.WriteFile(progressPath, []byte(newContent), 0o600); err != nil {
			return "", err
		}

		if params.PhaseName != "" {
			return "Phase '" + params.PhaseName + "' removed from progress. It will run again on next setup.", nil
		}
		return "All cloud phases removed from progress. They will run again on next setup.", nil
	}
}
