package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseRepoSlugFromRemote(t *testing.T) {
	t.Parallel()

	successTests := []struct {
		name      string
		remoteURL string
		want      string
	}{
		{
			name:      "github https",
			remoteURL: "https://github.com/use-tusk/tusk.git",
			want:      "use-tusk/tusk",
		},
		{
			name:      "github ssh",
			remoteURL: "git@github.com:use-tusk/tusk.git",
			want:      "use-tusk/tusk",
		},
		{
			name:      "prefixed https path",
			remoteURL: "https://git.example.com/scm/team/use-tusk/tusk.git",
			want:      "use-tusk/tusk",
		},
		{
			name:      "prefixed ssh path",
			remoteURL: "git@git.example.com:scm/team/use-tusk/tusk.git",
			want:      "use-tusk/tusk",
		},
		{
			name:      "https without .git suffix",
			remoteURL: "https://github.com/owner/repo",
			want:      "owner/repo",
		},
	}

	for _, tt := range successTests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseRepoSlugFromRemote(tt.remoteURL)
			if err != nil {
				t.Fatalf("parseRepoSlugFromRemote returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseRepoSlugFromRemote = %q, want %q", got, tt.want)
			}
		})
	}

	errorTests := []struct {
		name        string
		remoteURL   string
		errContains string
	}{
		{
			name:        "empty URL returns error",
			remoteURL:   "",
			errContains: "empty remote URL",
		},
		{
			name:        "unsupported format returns error",
			remoteURL:   "localpath/no-scheme",
			errContains: "unsupported git remote URL format",
		},
		{
			name:        "ssh URL with only one path segment",
			remoteURL:   "git@github.com:singlerepo.git",
			errContains: "failed to infer owner/repo",
		},
		{
			name:        "https URL with only one path segment",
			remoteURL:   "https://github.com/singlerepo",
			errContains: "failed to infer owner/repo",
		},
		{
			name:        "git@ URL without colon",
			remoteURL:   "git@nocolon",
			errContains: "failed to parse git remote URL",
		},
		{
			name:        "URL with invalid scheme causes parse error",
			remoteURL:   "://",
			errContains: "failed to parse git remote URL",
		},
	}

	for _, tt := range errorTests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := parseRepoSlugFromRemote(tt.remoteURL)
			require.Error(t, err)
			require.True(t, strings.Contains(err.Error(), tt.errContains),
				"expected error containing %q, got: %v", tt.errContains, err)
		})
	}
}

func TestResolveLatestRunInput(t *testing.T) {
	t.Run("returns provided repo and branch unchanged", func(t *testing.T) {
		repo, branch, err := resolveLatestRunInput("owner/repo", "main")
		require.NoError(t, err)
		require.Equal(t, "owner/repo", repo)
		require.Equal(t, "main", branch)
	})

	t.Run("fills in repo from git remote when empty", func(t *testing.T) {
		// This test runs inside the tusk-cli repo which has an origin remote.
		repo, branch, err := resolveLatestRunInput("", "main")
		// If git remote origin is available, it succeeds; otherwise expect an error
		if err != nil {
			require.Contains(t, err.Error(), "repo is required")
		} else {
			require.Contains(t, repo, "/")
			require.Equal(t, "main", branch)
		}
	})

	t.Run("fills in branch from git when empty", func(t *testing.T) {
		repo, branch, err := resolveLatestRunInput("owner/repo", "")
		// Git branch detection might succeed or fail depending on environment
		if err != nil {
			require.Contains(t, err.Error(), "branch is required")
		} else {
			require.Equal(t, "owner/repo", repo)
			require.NotEmpty(t, branch)
		}
	})
}
