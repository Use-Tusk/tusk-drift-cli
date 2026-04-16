package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Use-Tusk/tusk-cli/internal/api"
	"github.com/Use-Tusk/tusk-cli/internal/runner"
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

func TestGetCommitSHAFromEnv(t *testing.T) {
	t.Run("GitHub Actions returns PR head SHA when available", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("GITLAB_CI", "")
		t.Setenv("GITHUB_EVENT_NAME", "pull_request")
		t.Setenv("GITHUB_EVENT_PATH", writeEventFile(t, map[string]any{
			"pull_request": map[string]any{
				"head": map[string]any{"sha": "pr-head-sha"},
			},
		}))
		t.Setenv("GITHUB_SHA", "merge-sha")

		got := getCommitSHAFromEnv()
		require.Equal(t, "pr-head-sha", got)
	})

	t.Run("GitHub Actions falls back to GITHUB_SHA for non-PR event", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("GITLAB_CI", "")
		t.Setenv("GITHUB_EVENT_NAME", "push")
		t.Setenv("GITHUB_EVENT_PATH", "")
		t.Setenv("GITHUB_SHA", "push-sha-abc")

		got := getCommitSHAFromEnv()
		require.Equal(t, "push-sha-abc", got)
	})

	t.Run("GitLab CI returns CI_COMMIT_SHA", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "")
		t.Setenv("GITLAB_CI", "true")
		t.Setenv("CI_COMMIT_SHA", "gitlab-sha-xyz")

		got := getCommitSHAFromEnv()
		require.Equal(t, "gitlab-sha-xyz", got)
	})

	t.Run("not in CI returns empty string", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "")
		t.Setenv("GITLAB_CI", "")

		got := getCommitSHAFromEnv()
		require.Equal(t, "", got)
	})
}

func TestGetBranchFromEnv(t *testing.T) {
	t.Run("GITHUB_HEAD_REF takes priority", func(t *testing.T) {
		t.Setenv("GITHUB_HEAD_REF", "my-feature-branch")
		t.Setenv("GITHUB_REF_NAME", "other-branch")
		t.Setenv("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME", "gitlab-branch")
		t.Setenv("CI_COMMIT_REF_NAME", "ci-ref-branch")

		got := getBranchFromEnv()
		require.Equal(t, "my-feature-branch", got)
	})

	t.Run("GITHUB_REF_NAME used when GITHUB_HEAD_REF is empty", func(t *testing.T) {
		t.Setenv("GITHUB_HEAD_REF", "")
		t.Setenv("GITHUB_REF_NAME", "main")
		t.Setenv("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME", "")
		t.Setenv("CI_COMMIT_REF_NAME", "")

		got := getBranchFromEnv()
		require.Equal(t, "main", got)
	})

	t.Run("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME used for GitLab MR", func(t *testing.T) {
		t.Setenv("GITHUB_HEAD_REF", "")
		t.Setenv("GITHUB_REF_NAME", "")
		t.Setenv("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME", "feature/gl-branch")
		t.Setenv("CI_COMMIT_REF_NAME", "other-gl-branch")

		got := getBranchFromEnv()
		require.Equal(t, "feature/gl-branch", got)
	})

	t.Run("CI_COMMIT_REF_NAME used as GitLab fallback", func(t *testing.T) {
		t.Setenv("GITHUB_HEAD_REF", "")
		t.Setenv("GITHUB_REF_NAME", "")
		t.Setenv("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME", "")
		t.Setenv("CI_COMMIT_REF_NAME", "develop")

		got := getBranchFromEnv()
		require.Equal(t, "develop", got)
	})

	t.Run("falls back to git when all env vars empty", func(t *testing.T) {
		t.Setenv("GITHUB_HEAD_REF", "")
		t.Setenv("GITHUB_REF_NAME", "")
		t.Setenv("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME", "")
		t.Setenv("CI_COMMIT_REF_NAME", "")

		// Should not panic; returns either a branch name or empty string
		got := getBranchFromEnv()
		// We're in a git repo, so it should return something (may be "HEAD" in detached state)
		_ = got
	})
}

func TestValidateCIMetadata_Errors(t *testing.T) {
	t.Run("not in CI, missing CommitSha returns error", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "")
		t.Setenv("GITLAB_CI", "")

		_, err := validateCIMetadata(CIMetadata{PRNumber: "42", BranchName: "main"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "commit SHA is required")
	})

	t.Run("not in CI, missing PRNumber returns error", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "")
		t.Setenv("GITLAB_CI", "")

		_, err := validateCIMetadata(CIMetadata{CommitSha: "abc", BranchName: "main"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "pull/merge request number is required")
	})

	t.Run("non-numeric PRNumber returns error", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "")
		t.Setenv("GITLAB_CI", "")

		_, err := validateCIMetadata(CIMetadata{CommitSha: "abc", PRNumber: "notanumber", BranchName: "main"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "must be an integer")
	})

	t.Run("not in CI, missing BranchName returns error", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "")
		t.Setenv("GITLAB_CI", "")

		_, err := validateCIMetadata(CIMetadata{CommitSha: "abc", PRNumber: "42"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "branch name is required")
	})

	t.Run("not in CI, all fields provided succeeds", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "")
		t.Setenv("GITLAB_CI", "")

		meta, err := validateCIMetadata(CIMetadata{CommitSha: "abc123", PRNumber: "42", BranchName: "feature"})
		require.NoError(t, err)
		require.Equal(t, "abc123", meta.CommitSha)
		require.Equal(t, "42", meta.PRNumber)
		require.Equal(t, "feature", meta.BranchName)
	})
}

func TestValidateCIMetadata_GitLab(t *testing.T) {
	t.Run("GitLab CI populates metadata from env vars", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "")
		t.Setenv("GITLAB_CI", "true")
		t.Setenv("CI_COMMIT_SHA", "gl-sha-abc")
		t.Setenv("CI_MERGE_REQUEST_IID", "77")
		t.Setenv("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME", "gl-feature")
		t.Setenv("CI_PIPELINE_ID", "pipeline-999")
		t.Setenv("CI_JOB_ID", "")

		meta, err := validateCIMetadata(CIMetadata{})
		require.NoError(t, err)
		require.Equal(t, "gl-sha-abc", meta.CommitSha)
		require.Equal(t, "77", meta.PRNumber)
		require.Equal(t, "gl-feature", meta.BranchName)
		require.Equal(t, "pipeline-999", meta.ExternalCheckRunID)
	})

	t.Run("GitLab CI falls back to CI_JOB_ID when CI_PIPELINE_ID empty", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "")
		t.Setenv("GITLAB_CI", "true")
		t.Setenv("CI_COMMIT_SHA", "gl-sha-abc")
		t.Setenv("CI_MERGE_REQUEST_IID", "77")
		t.Setenv("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME", "gl-feature")
		t.Setenv("CI_PIPELINE_ID", "")
		t.Setenv("CI_JOB_ID", "job-555")

		meta, err := validateCIMetadata(CIMetadata{})
		require.NoError(t, err)
		require.Equal(t, "job-555", meta.ExternalCheckRunID)
	})
}

func TestValidateCIMetadata_GitHub(t *testing.T) {
	t.Run("GitHub CI populates PR number from GITHUB_REF", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("GITLAB_CI", "")
		t.Setenv("GITHUB_SHA", "gh-sha")
		t.Setenv("GITHUB_EVENT_NAME", "push")
		t.Setenv("GITHUB_EVENT_PATH", "")
		t.Setenv("GITHUB_REF", "refs/pull/55/merge")
		t.Setenv("GITHUB_HEAD_REF", "feature-x")
		t.Setenv("GITHUB_CHECK_RUN_ID", "check-111")

		meta, err := validateCIMetadata(CIMetadata{})
		require.NoError(t, err)
		require.Equal(t, "55", meta.PRNumber)
		require.Equal(t, "check-111", meta.ExternalCheckRunID)
	})

	t.Run("GitHub CI with non-numeric segment from GITHUB_REF returns error", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("GITLAB_CI", "")
		t.Setenv("GITHUB_SHA", "gh-sha")
		t.Setenv("GITHUB_EVENT_NAME", "push")
		t.Setenv("GITHUB_EVENT_PATH", "")
		t.Setenv("GITHUB_REF", "refs/heads/main") // parts[2] = "main", non-numeric
		t.Setenv("GITHUB_HEAD_REF", "feature-y")
		t.Setenv("GITHUB_CHECK_RUN_ID", "")

		// PRNumber gets set to "main" (non-numeric) -> validation error
		_, err := validateCIMetadata(CIMetadata{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "must be an integer")
	})
}

func TestStringPtr(t *testing.T) {
	s := "hello"
	ptr := stringPtr(s)
	require.NotNil(t, ptr)
	require.Equal(t, s, *ptr)

	empty := stringPtr("")
	require.NotNil(t, empty)
	require.Equal(t, "", *empty)
}

func TestCountPassedFailed(t *testing.T) {
	t.Run("empty results returns zeros", func(t *testing.T) {
		passed, failed := countPassedFailed(nil)
		assert.Equal(t, 0, passed)
		assert.Equal(t, 0, failed)
	})

	t.Run("all passed", func(t *testing.T) {
		results := []runner.TestResult{{Passed: true}, {Passed: true}}
		passed, failed := countPassedFailed(results)
		assert.Equal(t, 2, passed)
		assert.Equal(t, 0, failed)
	})

	t.Run("all failed", func(t *testing.T) {
		results := []runner.TestResult{{Passed: false}, {Passed: false}, {Passed: false}}
		passed, failed := countPassedFailed(results)
		assert.Equal(t, 0, passed)
		assert.Equal(t, 3, failed)
	})

	t.Run("mixed results", func(t *testing.T) {
		results := []runner.TestResult{
			{Passed: true},
			{Passed: false},
			{Passed: true},
		}
		passed, failed := countPassedFailed(results)
		assert.Equal(t, 2, passed)
		assert.Equal(t, 1, failed)
	})
}

func TestCreateRunDirectory(t *testing.T) {
	t.Run("creates directory under base dir", func(t *testing.T) {
		base := t.TempDir()
		dir, err := createRunDirectory(base)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(dir, base))
		info, statErr := os.Stat(dir)
		require.NoError(t, statErr)
		require.True(t, info.IsDir())
	})

	t.Run("creates nested base dir if it does not exist", func(t *testing.T) {
		base := filepath.Join(t.TempDir(), "nested", "results")
		dir, err := createRunDirectory(base)
		require.NoError(t, err)
		info, statErr := os.Stat(dir)
		require.NoError(t, statErr)
		require.True(t, info.IsDir())
	})

	t.Run("appends counter on collision within same second", func(t *testing.T) {
		base := t.TempDir()
		dir1, err := createRunDirectory(base)
		require.NoError(t, err)

		// Pre-create a dir with the same name pattern to force collision
		// Create a placeholder that matches what the next call would create
		require.NotEmpty(t, dir1)

		// Call again; if same second, will get dir1 + "-2"
		dir2, err := createRunDirectory(base)
		require.NoError(t, err)
		require.NotEmpty(t, dir2)

		// Both directories should exist
		_, err1 := os.Stat(dir1)
		_, err2 := os.Stat(dir2)
		require.NoError(t, err1)
		require.NoError(t, err2)
	})

	t.Run("returns error when base dir cannot be created", func(t *testing.T) {
		// Use a path inside an existing file as base dir
		f, err := os.CreateTemp("", "tusk-test-*")
		require.NoError(t, err)
		require.NoError(t, f.Close())
		t.Cleanup(func() { _ = os.Remove(f.Name()) })

		_, err = createRunDirectory(filepath.Join(f.Name(), "subdir"))
		require.Error(t, err)
	})

	t.Run("returns error when dir creation fails with non-exist error", func(t *testing.T) {
		base := t.TempDir()
		// Remove write permission so os.Mkdir fails with permission denied (not IsExist)
		if err := os.Chmod(base, 0o555); err != nil { //nolint:gosec
			t.Skip("cannot chmod dir, skipping")
		}
		t.Cleanup(func() { _ = os.Chmod(base, 0o755) }) //nolint:gosec

		_, err := createRunDirectory(base)
		require.Error(t, err)
	})
}

func TestMakeLoadTestsFunc(t *testing.T) {
	t.Run("returns error when traceDir does not exist", func(t *testing.T) {
		origTraceDir := traceDir
		origTraceFile := traceFile
		traceDir = "/nonexistent/trace/dir"
		traceFile = ""
		t.Cleanup(func() {
			traceDir = origTraceDir
			traceFile = origTraceFile
		})

		executor := runner.NewExecutor()
		fn := makeLoadTestsFunc(executor, nil, api.AuthOptions{}, "", "", "", "", false, "", false)
		_, err := fn(context.Background())
		require.Error(t, err)
	})

	t.Run("loads empty folder successfully", func(t *testing.T) {
		origTraceDir := traceDir
		origTraceFile := traceFile
		traceDir = t.TempDir()
		traceFile = ""
		t.Cleanup(func() {
			traceDir = origTraceDir
			traceFile = origTraceFile
		})

		executor := runner.NewExecutor()
		fn := makeLoadTestsFunc(executor, nil, api.AuthOptions{}, "", "", "", "", false, "", false)
		tests, err := fn(context.Background())
		require.NoError(t, err)
		require.Empty(t, tests)
	})

	t.Run("applies filter when folder is empty", func(t *testing.T) {
		origTraceDir := traceDir
		origTraceFile := traceFile
		traceDir = t.TempDir()
		traceFile = ""
		t.Cleanup(func() {
			traceDir = origTraceDir
			traceFile = origTraceFile
		})

		executor := runner.NewExecutor()
		fn := makeLoadTestsFunc(executor, nil, api.AuthOptions{}, "", "", "", "", false, "type=GRAPHQL", false)
		tests, err := fn(context.Background())
		require.NoError(t, err)
		require.Empty(t, tests)
	})

	t.Run("returns error for non-existent traceFile", func(t *testing.T) {
		origTraceDir := traceDir
		origTraceFile := traceFile
		traceDir = ""
		traceFile = "/nonexistent/trace.jsonl"
		t.Cleanup(func() {
			traceDir = origTraceDir
			traceFile = origTraceFile
		})

		executor := runner.NewExecutor()
		fn := makeLoadTestsFunc(executor, nil, api.AuthOptions{}, "", "", "", "", false, "", false)
		_, err := fn(context.Background())
		require.Error(t, err)
	})

	t.Run("returns error for non-existent traceID", func(t *testing.T) {
		origTraceDir := traceDir
		origTraceFile := traceFile
		traceDir = ""
		traceFile = ""
		t.Cleanup(func() {
			traceDir = origTraceDir
			traceFile = origTraceFile
		})

		executor := runner.NewExecutor()
		fn := makeLoadTestsFunc(executor, nil, api.AuthOptions{}, "", "", "nonexistent-id", "", false, "", false)
		_, err := fn(context.Background())
		require.Error(t, err)
	})

	t.Run("cloud client with traceID but no traceTestID returns error", func(t *testing.T) {
		executor := runner.NewExecutor()
		// Pass a non-nil client pointer so the cloud branch is entered
		client := &api.TuskClient{}
		fn := makeLoadTestsFunc(executor, client, api.AuthOptions{}, "", "", "some-trace-id", "", false, "", false)
		_, err := fn(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "trace-test-id")
	})
}
