package onboardcloud

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGitHubRepo(t *testing.T) {
	tests := []struct {
		name      string
		remoteURL string
		wantOwner string
		wantRepo  string
		wantIsGH  bool
	}{
		{
			name:      "HTTPS URL with .git",
			remoteURL: "https://github.com/owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantIsGH:  true,
		},
		{
			name:      "HTTPS URL without .git",
			remoteURL: "https://github.com/owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantIsGH:  true,
		},
		{
			name:      "SSH URL with .git",
			remoteURL: "git@github.com:owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantIsGH:  true,
		},
		{
			name:      "SSH URL without .git",
			remoteURL: "git@github.com:owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantIsGH:  true,
		},
		{
			name:      "Non-GitHub URL",
			remoteURL: "https://gitlab.com/owner/repo.git",
			wantOwner: "",
			wantRepo:  "",
			wantIsGH:  false,
		},
		{
			name:      "Invalid GitHub URL - single path component",
			remoteURL: "https://github.com/owner",
			wantOwner: "",
			wantRepo:  "",
			wantIsGH:  false,
		},
		{
			name:      "GitHub URL with nested path",
			remoteURL: "https://github.com/owner/repo/extra/path",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantIsGH:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOwner, gotRepo, gotIsGH := parseGitHubRepo(tt.remoteURL)
			assert.Equal(t, tt.wantOwner, gotOwner, "owner mismatch")
			assert.Equal(t, tt.wantRepo, gotRepo, "repo mismatch")
			assert.Equal(t, tt.wantIsGH, gotIsGH, "isGitHub mismatch")
		})
	}
}

func TestParseGitLabRepo(t *testing.T) {
	tests := []struct {
		name      string
		remoteURL string
		wantOwner string
		wantRepo  string
		wantIsGL  bool
	}{
		{
			name:      "HTTPS URL with .git",
			remoteURL: "https://gitlab.com/owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantIsGL:  true,
		},
		{
			name:      "HTTPS URL without .git",
			remoteURL: "https://gitlab.com/owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantIsGL:  true,
		},
		{
			name:      "SSH URL with .git",
			remoteURL: "git@gitlab.com:owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantIsGL:  true,
		},
		{
			name:      "SSH URL without .git",
			remoteURL: "git@gitlab.com:owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantIsGL:  true,
		},
		{
			name:      "Non-GitLab URL",
			remoteURL: "https://github.com/owner/repo.git",
			wantOwner: "",
			wantRepo:  "",
			wantIsGL:  false,
		},
		{
			name:      "Invalid GitLab URL - single path component",
			remoteURL: "https://gitlab.com/owner",
			wantOwner: "",
			wantRepo:  "",
			wantIsGL:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOwner, gotRepo, gotIsGL := parseGitLabRepo(tt.remoteURL)
			assert.Equal(t, tt.wantOwner, gotOwner, "owner mismatch")
			assert.Equal(t, tt.wantRepo, gotRepo, "repo mismatch")
			assert.Equal(t, tt.wantIsGL, gotIsGL, "isGitLab mismatch")
		})
	}
}

func TestParseGenericGitURL(t *testing.T) {
	tests := []struct {
		name      string
		remoteURL string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			name:      "HTTPS URL with .git",
			remoteURL: "https://git.example.com/owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantErr:   false,
		},
		{
			name:      "HTTPS URL without .git",
			remoteURL: "https://git.example.com/owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantErr:   false,
		},
		{
			name:      "SSH URL with .git",
			remoteURL: "git@git.example.com:owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantErr:   false,
		},
		{
			name:      "SSH URL without .git",
			remoteURL: "git@git.example.com:owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantErr:   false,
		},
		{
			name:      "Invalid URL - no path",
			remoteURL: "https://git.example.com",
			wantOwner: "",
			wantRepo:  "",
			wantErr:   true,
		},
		{
			name:      "Invalid URL - only owner",
			remoteURL: "https://git.example.com/owner",
			wantOwner: "",
			wantRepo:  "",
			wantErr:   true,
		},
		{
			name:      "SSH URL with nested path (GitLab subgroups)",
			remoteURL: "git@git.example.com:group/subgroup/repo.git",
			wantOwner: "group",
			wantRepo:  "subgroup",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOwner, gotRepo, err := parseGenericGitURL(tt.remoteURL)
			if tt.wantErr {
				assert.Error(t, err, "expected an error")
			} else {
				assert.NoError(t, err, "unexpected error")
			}
			assert.Equal(t, tt.wantOwner, gotOwner, "owner mismatch")
			assert.Equal(t, tt.wantRepo, gotRepo, "repo mismatch")
		})
	}
}

func TestConfigExists(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(originalWd) })

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	assert.False(t, configExists(), "configExists() should return false when config doesn't exist")

	// Create .tusk directory and config file
	err = os.MkdirAll(".tusk", 0o750)
	require.NoError(t, err)
	err = os.WriteFile(".tusk/config.yaml", []byte("test config"), 0o600)
	require.NoError(t, err)

	assert.True(t, configExists(), "configExists() should return true when config exists")
}

func TestIsGitRepo(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantGit bool
	}{
		{
			name: "Not a git repo",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				return tmpDir
			},
			wantGit: false,
		},
		{
			name: "Is a git repo",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				cmd := exec.Command("git", "init")
				cmd.Dir = tmpDir
				err := cmd.Run()
				require.NoError(t, err)
				return tmpDir
			},
			wantGit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := tt.setup(t)
			originalWd, err := os.Getwd()
			require.NoError(t, err)
			t.Cleanup(func() { _ = os.Chdir(originalWd) })

			err = os.Chdir(tmpDir)
			require.NoError(t, err)

			got := isGitRepo()
			assert.Equal(t, tt.wantGit, got)
		})
	}
}

func TestGetGithubAuthURL(t *testing.T) {
	tests := []struct {
		name          string
		model         *Model
		githubAppName string
		want          string
		wantContains  []string
	}{
		{
			name: "Valid model with default app name",
			model: &Model{
				SelectedClient: &ClientInfo{ID: "client-123"},
				UserId:         "user-456",
			},
			githubAppName: "use-tusk",
			wantContains: []string{
				"https://github.com/apps/use-tusk/installations/new",
				"client-123",
				"user-456",
				"cli-init-cloud",
			},
		},
		{
			name: "Valid model with custom app name",
			model: &Model{
				SelectedClient: &ClientInfo{ID: "client-789"},
				UserId:         "user-abc",
			},
			githubAppName: "custom-tusk-app",
			wantContains: []string{
				"https://github.com/apps/custom-tusk-app/installations/new",
				"client-789",
				"user-abc",
			},
		},
		{
			name: "Nil client",
			model: &Model{
				SelectedClient: nil,
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.githubAppName != "" {
				t.Setenv("GITHUB_APP_NAME", tt.githubAppName)
			}

			got := tt.model.getGithubAuthURL()

			if tt.model.SelectedClient == nil {
				assert.Empty(t, got, "should return empty string for nil client")
				return
			}

			if tt.want != "" {
				assert.Equal(t, tt.want, got)
			}

			for _, wantContains := range tt.wantContains {
				assert.Contains(t, got, wantContains)
			}
		})
	}
}

func TestGetGitlabAuthURL(t *testing.T) {
	m := &Model{}
	got := m.getGitlabAuthURL()
	assert.Equal(t, "https://app.usetusk.ai/app/settings/connect-gitlab", got)
}

func TestBuildGithubAuthMessage(t *testing.T) {
	tests := []struct {
		name         string
		model        *Model
		wantContains []string
	}{
		{
			name: "Valid model",
			model: &Model{
				SelectedClient: &ClientInfo{ID: "client-123"},
				UserId:         "user-456",
				GitRepoOwner:   "testowner",
				GitRepoName:    "testrepo",
			},
			wantContains: []string{
				"testowner/testrepo",
				"GitHub App Installation URL",
				"Press Enter to continue",
				"Grant access",
			},
		},
		{
			name: "Nil client",
			model: &Model{
				SelectedClient: nil,
			},
			wantContains: []string{
				"Error: No client selected",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.model.buildGithubAuthMessage()
			for _, want := range tt.wantContains {
				assert.Contains(t, got, want)
			}
		})
	}
}

func TestBuildGitlabAuthMessage(t *testing.T) {
	m := &Model{
		GitRepoOwner: "testowner",
		GitRepoName:  "testrepo",
	}

	got := m.buildGitlabAuthMessage()

	wantContains := []string{
		"testowner/testrepo",
		"connect your GitLab account",
		"Press Enter to open browser",
		"personal access token",
	}

	for _, want := range wantContains {
		assert.Contains(t, got, want)
	}
}

func TestBuildCodeHostingAuthMessage(t *testing.T) {
	tests := []struct {
		name         string
		resourceType CodeHostingResourceType
		wantContains string
	}{
		{
			name:         "GitHub",
			resourceType: CodeHostingResourceTypeGitHub,
			wantContains: "GitHub App Installation URL",
		},
		{
			name:         "GitLab",
			resourceType: CodeHostingResourceTypeGitLab,
			wantContains: "connect your GitLab account",
		},
		{
			name:         "Unknown type",
			resourceType: CodeHostingResourceType(999),
			wantContains: "Unknown code hosting resource type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Model{
				CodeHostingResourceType: tt.resourceType,
				GitRepoOwner:            "owner",
				GitRepoName:             "repo",
				SelectedClient:          &ClientInfo{ID: "client-123"},
				UserId:                  "user-456",
			}

			got := m.buildCodeHostingAuthMessage()
			assert.Contains(t, got, tt.wantContains)
		})
	}
}

func TestDetectShellConfig(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	tests := []struct {
		name          string
		shell         string
		possiblePaths []string
	}{
		{
			name:  "zsh shell",
			shell: "/bin/zsh",
			possiblePaths: []string{
				filepath.Join(homeDir, ".zshrc"),
				filepath.Join(homeDir, ".profile"),
			},
		},
		{
			name:  "bash shell",
			shell: "/bin/bash",
			possiblePaths: []string{
				filepath.Join(homeDir, ".bashrc"),
				filepath.Join(homeDir, ".bash_profile"),
				filepath.Join(homeDir, ".profile"),
			},
		},
		{
			name:  "unknown shell",
			shell: "/bin/sh",
			possiblePaths: []string{
				filepath.Join(homeDir, ".profile"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SHELL", tt.shell)
			got := detectShellConfig()

			// Check if result is one of the expected paths
			found := slices.Contains(tt.possiblePaths, got)
			assert.True(t, found, "detectShellConfig() = %v, want one of %v", got, tt.possiblePaths)
		})
	}
}

func TestGetAppDir(t *testing.T) {
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(originalWd) })

	t.Run("At repo root", func(t *testing.T) {
		_ = setupGitRepoWithRemote(t, "https://github.com/test/repo.git")
		t.Cleanup(func() { _ = os.Chdir(originalWd) })

		appDir, err := getAppDir()
		assert.NoError(t, err)

		// At repo root, should return empty string or "."
		// On Windows CI with nested repos, git may resolve differently
		normalized := filepath.Clean(appDir)
		assert.True(t,
			appDir == "" || normalized == ".",
			"should return empty or '.' at repo root, got: %s", appDir)
	})

	t.Run("In subdirectory", func(t *testing.T) {
		tmpDir := setupGitRepoWithRemote(t, "https://github.com/test/repo.git")

		subDir := filepath.Join(tmpDir, "app", "src")
		err := os.MkdirAll(subDir, 0o750)
		require.NoError(t, err)

		err = os.Chdir(subDir)
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.Chdir(originalWd) })

		appDir, err := getAppDir()
		assert.NoError(t, err)
		assert.True(t,
			strings.HasSuffix(filepath.ToSlash(appDir), "app/src"),
			"path should end with app/src, got: %s", appDir)
	})

	t.Run("Not in git repo", func(t *testing.T) {
		nonGitDir := t.TempDir()
		err := os.Chdir(nonGitDir)
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.Chdir(originalWd) })

		_, err = getAppDir()
		assert.Error(t, err, "should return error when not in git repo")
	})
}

func TestDetectGitHubIndicators(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(originalWd) })

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	err = cmd.Run()
	require.NoError(t, err)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	t.Run("Without .github directory", func(t *testing.T) {
		assert.False(t, detectGitHubIndicators())
	})

	t.Run("With .github directory", func(t *testing.T) {
		err := os.MkdirAll(".github", 0o750)
		require.NoError(t, err)

		assert.True(t, detectGitHubIndicators())
	})
}

func TestDetectGitLabIndicators(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(originalWd) })

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	err = cmd.Run()
	require.NoError(t, err)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	t.Run("Without .gitlab-ci.yml", func(t *testing.T) {
		assert.False(t, detectGitLabIndicators())
	})

	t.Run("With .gitlab-ci.yml", func(t *testing.T) {
		err := os.WriteFile(".gitlab-ci.yml", []byte("test: true"), 0o600)
		require.NoError(t, err)

		assert.True(t, detectGitLabIndicators())
	})
}

func TestDetectGitRepo(t *testing.T) {
	t.Run("Not a git repo", func(t *testing.T) {
		tmpDir := t.TempDir()
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.Chdir(originalWd) })

		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		m := &Model{}
		err = detectGitRepo(m)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not a git repository")
	})

	t.Run("GitHub repo - HTTPS", func(t *testing.T) {
		tmpDir := setupGitRepoWithRemote(t, "https://github.com/testowner/testrepo.git")

		m := &Model{}
		err := detectGitRepo(m)
		assert.NoError(t, err)
		assert.Equal(t, "testowner", m.GitRepoOwner)
		assert.Equal(t, "testrepo", m.GitRepoName)
		assert.Equal(t, CodeHostingResourceTypeGitHub, m.CodeHostingResourceType)

		_ = os.Chdir(tmpDir) // Keep test in tmpDir for cleanup
	})

	t.Run("GitHub repo - SSH", func(t *testing.T) {
		tmpDir := setupGitRepoWithRemote(t, "git@github.com:testowner/testrepo.git")

		m := &Model{}
		err := detectGitRepo(m)
		assert.NoError(t, err)
		assert.Equal(t, "testowner", m.GitRepoOwner)
		assert.Equal(t, "testrepo", m.GitRepoName)
		assert.Equal(t, CodeHostingResourceTypeGitHub, m.CodeHostingResourceType)

		_ = os.Chdir(tmpDir)
	})

	t.Run("GitLab repo - HTTPS", func(t *testing.T) {
		tmpDir := setupGitRepoWithRemote(t, "https://gitlab.com/testowner/testrepo.git")

		m := &Model{}
		err := detectGitRepo(m)
		assert.NoError(t, err)
		assert.Equal(t, "testowner", m.GitRepoOwner)
		assert.Equal(t, "testrepo", m.GitRepoName)
		assert.Equal(t, CodeHostingResourceTypeGitLab, m.CodeHostingResourceType)

		_ = os.Chdir(tmpDir)
	})

	t.Run("Non-GitHub/GitLab repo", func(t *testing.T) {
		tmpDir := setupGitRepoWithRemote(t, "https://bitbucket.org/testowner/testrepo.git")

		m := &Model{}
		err := detectGitRepo(m)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be hosted on GitHub or GitLab")

		_ = os.Chdir(tmpDir)
	})

	t.Run("Self-hosted GitHub with .github directory", func(t *testing.T) {
		tmpDir := setupGitRepoWithRemote(t, "https://git.company.com/testowner/testrepo.git")

		// Create .github directory to indicate GitHub
		err := os.MkdirAll(".github", 0o750)
		require.NoError(t, err)

		m := &Model{}
		err = detectGitRepo(m)
		assert.NoError(t, err)
		assert.Equal(t, "testowner", m.GitRepoOwner)
		assert.Equal(t, "testrepo", m.GitRepoName)
		assert.Equal(t, CodeHostingResourceTypeGitHub, m.CodeHostingResourceType)

		_ = os.Chdir(tmpDir)
	})

	t.Run("Self-hosted GitLab with .gitlab-ci.yml", func(t *testing.T) {
		tmpDir := setupGitRepoWithRemote(t, "https://git.company.com/testowner/testrepo.git")

		// Create .gitlab-ci.yml to indicate GitLab
		err := os.WriteFile(".gitlab-ci.yml", []byte("test: true"), 0o600)
		require.NoError(t, err)

		m := &Model{}
		err = detectGitRepo(m)
		assert.NoError(t, err)
		assert.Equal(t, "testowner", m.GitRepoOwner)
		assert.Equal(t, "testrepo", m.GitRepoName)
		assert.Equal(t, CodeHostingResourceTypeGitLab, m.CodeHostingResourceType)

		_ = os.Chdir(tmpDir)
	})
}

// setupGitRepoWithRemote creates a temporary git repo with a remote configured
func setupGitRepoWithRemote(t *testing.T, remoteURL string) string {
	t.Helper()

	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(originalWd) })

	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	err = cmd.Run()
	require.NoError(t, err)

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	_ = cmd.Run()

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	_ = cmd.Run()

	cmd = exec.Command("git", "remote", "add", "origin", remoteURL)
	cmd.Dir = tmpDir
	err = cmd.Run()
	require.NoError(t, err)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	return tmpDir
}
