package onboardcloud

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
)

func getGitRemoteURL() (string, error) {
	cmd := exec.Command("git", "config", "--get", "remote.origin.url")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get git remote: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func parseGitHubRepo(remoteURL string) (owner, repo string, isGitHub bool) {
	// Handle both HTTPS and SSH formats
	// https://github.com/owner/repo.git
	// git@github.com:owner/repo.git

	if !strings.Contains(remoteURL, "github.com") {
		return "", "", false
	}

	var path string
	if after, ok := strings.CutPrefix(remoteURL, "git@github.com:"); ok {
		path = after
	} else if strings.Contains(remoteURL, "github.com/") {
		parts := strings.Split(remoteURL, "github.com/")
		if len(parts) > 1 {
			path = parts[1]
		}
	}

	path = strings.TrimSuffix(path, ".git")
	parts := strings.Split(path, "/")
	if len(parts) >= 2 {
		return parts[0], parts[1], true
	}

	return "", "", false
}

func isGitRepo() bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	return cmd.Run() == nil
}

func configExists() bool {
	// TODO: consider whether we want to use `findConfigFile` here
	_, err := os.Stat(".tusk/config.yaml")
	return err == nil
}

func loadExistingConfig(m *Model) error {
	cfg, err := config.Get()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	m.ServiceID = cfg.Service.ID
	m.SamplingRate = fmt.Sprintf("%.2f", cfg.Recording.SamplingRate)

	if cfg.Recording.ExportSpans != nil {
		m.ExportSpans = *cfg.Recording.ExportSpans
	} else {
		m.ExportSpans = false
	}

	if cfg.Recording.EnableEnvVarRecording != nil {
		m.EnableEnvVarRecording = *cfg.Recording.EnableEnvVarRecording
	} else {
		m.EnableEnvVarRecording = false
	}

	m.HasApiKey = config.GetAPIKey() != ""

	return nil
}

func detectGitRepo(m *Model) error {
	if !isGitRepo() {
		return fmt.Errorf(`not a git repository.

This directory must be a git repository with a remote configured.
Run these commands to initialize:
  git init
  git remote add origin <your-repo-url>`)
	}

	remoteURL, err := getGitRemoteURL()
	if err != nil {
		return fmt.Errorf("failed to get git remote URL: %w", err)
	}

	owner, repo, isGitHub := parseGitHubRepo(remoteURL)
	if isGitHub {
		m.GitRepoOwner = owner
		m.GitRepoName = repo
		m.IsGitHubRepo = true
		return nil
	}

	// TODO: Add GitLab parsing support
	return fmt.Errorf(`repository is not hosted on GitHub.

Remote URL: %s

Tusk Drift Cloud currently supports GitHub repositories only.
GitLab support is coming soon.`, remoteURL)
}

func (m *Model) buildGithubAuthMessage() string {
	if m.SelectedClient == nil {
		return "Error: No client selected"
	}

	authURL := m.getGithubAuthURL()

	return fmt.Sprintf(`Tusk cannot access %s/%s yet.

Please install the Tusk GitHub app and grant access to this repository.

GitHub App Installation URL:
%s

This will open in your browser when you press Enter.

After installation:
  1. Grant access to %s/%s
  2. Press Enter again to retry verification

Press Enter to continue...`,
		m.GitRepoOwner, m.GitRepoName,
		authURL,
		m.GitRepoOwner, m.GitRepoName)
}

func (m *Model) getGithubAuthURL() string {
	if m.SelectedClient == nil {
		return ""
	}

	clientID := m.SelectedClient.ID
	state := fmt.Sprintf(`{"clientId":"%s","userId":""}`, clientID)
	encodedState := url.QueryEscape(state)

	githubAppName := utils.EnvDefault("GITHUB_APP_NAME", "use-tusk")

	return fmt.Sprintf("https://github.com/apps/%s/installations/new?state=%s",
		githubAppName, encodedState)
}

func openGithubAuthBrowser(m *Model) {
	authURL := m.getGithubAuthURL()
	if authURL == "" {
		return
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", authURL) // #nosec G204
	case "linux":
		cmd = exec.Command("xdg-open", authURL) // #nosec G204
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", authURL) // #nosec G204
	}

	if cmd != nil {
		_ = cmd.Start()
	}
}

// getAppDir returns the relative path from git repo root to current directory
// Returns empty string if current directory is the repo root
func getAppDir() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get git repo root: %w", err)
	}
	repoRoot := strings.TrimSpace(string(output))

	currentDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}

	// Calculate relative path from repo root to current directory
	relPath, err := filepath.Rel(repoRoot, currentDir)
	if err != nil {
		return "", fmt.Errorf("failed to calculate relative path: %w", err)
	}

	// If relative path is ".", we're at repo root
	if relPath == "." {
		return "", nil
	}

	return relPath, nil
}

// detectShellConfig returns the path to the user's shell config file
func detectShellConfig() string {
	shell := os.Getenv("SHELL")
	homeDir, _ := os.UserHomeDir()

	if strings.Contains(shell, "zsh") {
		zshrc := filepath.Join(homeDir, ".zshrc")
		if _, err := os.Stat(zshrc); err == nil {
			return zshrc
		}
	}

	if strings.Contains(shell, "bash") {
		bashrc := filepath.Join(homeDir, ".bashrc")
		if _, err := os.Stat(bashrc); err == nil {
			return bashrc
		}
		bashProfile := filepath.Join(homeDir, ".bash_profile")
		if _, err := os.Stat(bashProfile); err == nil {
			return bashProfile
		}
	}

	// Fallback to .profile
	return filepath.Join(homeDir, ".profile")
}
