package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeEventFile(t *testing.T, payload any) string {
	t.Helper()
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "event.json")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func TestGetGitHubPRHeadSHA(t *testing.T) {
	tests := []struct {
		name      string
		eventName string
		eventFile string // raw content; if empty, writeEventFile is used with payload
		payload   any
		expected  string
	}{
		{
			name:      "pull_request event with valid JSON",
			eventName: "pull_request",
			payload: map[string]any{
				"pull_request": map[string]any{
					"head": map[string]any{
						"sha": "abc123def456",
					},
				},
			},
			expected: "abc123def456",
		},
		{
			name:      "pull_request_target event",
			eventName: "pull_request_target",
			payload: map[string]any{
				"pull_request": map[string]any{
					"head": map[string]any{
						"sha": "target789",
					},
				},
			},
			expected: "target789",
		},
		{
			name:      "non-PR event returns empty",
			eventName: "push",
			expected:  "",
		},
		{
			name:      "no event name returns empty",
			eventName: "",
			expected:  "",
		},
		{
			name:      "malformed JSON returns empty",
			eventName: "pull_request",
			eventFile: "{invalid json",
			expected:  "",
		},
		{
			name:      "missing sha field returns empty",
			eventName: "pull_request",
			payload: map[string]any{
				"pull_request": map[string]any{
					"head": map[string]any{},
				},
			},
			expected: "",
		},
		{
			name:      "missing event file returns empty",
			eventName: "pull_request",
			eventFile: "__nonexistent__",
			expected:  "",
		},
		{
			name:      "empty event path returns empty",
			eventName: "pull_request",
			eventFile: "",
			expected:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GITHUB_EVENT_NAME", tt.eventName)

			switch {
			case tt.eventFile == "__nonexistent__":
				t.Setenv("GITHUB_EVENT_PATH", "/nonexistent/path/event.json")
			case tt.eventFile != "":
				path := filepath.Join(t.TempDir(), "event.json")
				require.NoError(t, os.WriteFile(path, []byte(tt.eventFile), 0o600))
				t.Setenv("GITHUB_EVENT_PATH", path)
			case tt.payload != nil:
				t.Setenv("GITHUB_EVENT_PATH", writeEventFile(t, tt.payload))
			default:
				t.Setenv("GITHUB_EVENT_PATH", "")
			}

			got := getGitHubPRHeadSHA()
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestValidateCIMetadata_GitHubPRSHA(t *testing.T) {
	t.Run("uses PR head SHA over GITHUB_SHA", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("GITHUB_SHA", "merge-commit-sha")
		t.Setenv("GITHUB_EVENT_NAME", "pull_request")
		t.Setenv("GITHUB_EVENT_PATH", writeEventFile(t, map[string]any{
			"pull_request": map[string]any{
				"head": map[string]any{
					"sha": "real-head-sha",
				},
			},
		}))
		t.Setenv("GITHUB_REF", "refs/pull/42/merge")
		t.Setenv("GITHUB_HEAD_REF", "feature-branch")

		meta, err := validateCIMetadata(CIMetadata{})
		require.NoError(t, err)
		require.Equal(t, "real-head-sha", meta.CommitSha)
	})

	t.Run("falls back to GITHUB_SHA on non-PR event", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("GITHUB_SHA", "push-sha")
		t.Setenv("GITHUB_EVENT_NAME", "push")
		t.Setenv("GITHUB_REF", "refs/pull/42/merge")
		t.Setenv("GITHUB_HEAD_REF", "feature-branch")

		meta, err := validateCIMetadata(CIMetadata{})
		require.NoError(t, err)
		require.Equal(t, "push-sha", meta.CommitSha)
	})

	t.Run("explicit flag takes precedence", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("GITHUB_SHA", "merge-commit-sha")
		t.Setenv("GITHUB_EVENT_NAME", "pull_request")
		t.Setenv("GITHUB_EVENT_PATH", writeEventFile(t, map[string]any{
			"pull_request": map[string]any{
				"head": map[string]any{
					"sha": "real-head-sha",
				},
			},
		}))
		t.Setenv("GITHUB_REF", "refs/pull/42/merge")
		t.Setenv("GITHUB_HEAD_REF", "feature-branch")

		meta, err := validateCIMetadata(CIMetadata{CommitSha: "flag-sha"})
		require.NoError(t, err)
		require.Equal(t, "flag-sha", meta.CommitSha)
	})
}
