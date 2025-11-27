package onboardcloud

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Use-Tusk/tusk-drift-cli/internal/cliconfig"
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

func parseGitLabRepo(remoteURL string) (owner, repo string, isGitLab bool) {
	// Handle both HTTPS and SSH formats
	// https://gitlab.com/owner/repo.git
	// git@gitlab.com:owner/repo.git

	if !strings.Contains(remoteURL, "gitlab.com") {
		return "", "", false
	}

	var path string
	if after, ok := strings.CutPrefix(remoteURL, "git@gitlab.com:"); ok {
		path = after
	} else if strings.Contains(remoteURL, "gitlab.com/") {
		parts := strings.Split(remoteURL, "gitlab.com/")
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

func parseGenericGitURL(remoteURL string) (owner, repo string, err error) {
	// Handle both HTTPS and SSH formats for any Git hosting
	// https://git.example.com/owner/repo.git
	// git@git.example.com:owner/repo.git

	var path string

	// Handle SSH format (git@host:path)
	if strings.Contains(remoteURL, "@") && strings.Contains(remoteURL, ":") {
		parts := strings.Split(remoteURL, ":")
		if len(parts) >= 2 {
			path = parts[len(parts)-1]
		}
	} else if strings.Contains(remoteURL, "://") {
		// Handle HTTPS format
		parts := strings.Split(remoteURL, "://")
		if len(parts) >= 2 {
			// Get everything after the domain
			pathParts := strings.SplitN(parts[1], "/", 2)
			if len(pathParts) >= 2 {
				path = pathParts[1]
			}
		}
	}

	if path == "" {
		return "", "", fmt.Errorf("could not parse repository path from URL: %s", remoteURL)
	}

	path = strings.TrimSuffix(path, ".git")
	pathParts := strings.Split(path, "/")

	if len(pathParts) >= 2 {
		return pathParts[0], pathParts[1], nil
	}

	return "", "", fmt.Errorf("could not extract owner/repo from path: %s", path)
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

	m.HasApiKey = cliconfig.GetAPIKey() != ""

	return nil
}

// saveSelectedClientToCLIConfig persists the selected client to CLI config
func saveSelectedClientToCLIConfig(clientID, clientName string) {
	cfg, err := cliconfig.Load()
	if err != nil {
		return // Silently fail - not critical
	}
	cfg.SelectedClientID = clientID
	cfg.SelectedClientName = clientName
	_ = cfg.Save()
}

func getGitRootDir() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get git root: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func detectGitHubIndicators() bool {
	gitRoot, err := getGitRootDir()
	if err != nil {
		return false
	}

	githubPath := filepath.Join(gitRoot, ".github")
	info, err := os.Stat(githubPath)
	return err == nil && info.IsDir()
}

func detectGitLabIndicators() bool {
	gitRoot, err := getGitRootDir()
	if err != nil {
		return false
	}

	gitlabPath := filepath.Join(gitRoot, ".gitlab-ci.yml")
	_, err = os.Stat(gitlabPath)
	return err == nil
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

	// Try GitHub first
	owner, repo, isGitHub := parseGitHubRepo(remoteURL)
	if isGitHub {
		m.GitRepoOwner = owner
		m.GitRepoName = repo
		m.CodeHostingResourceType = CodeHostingResourceTypeGitHub
		return nil
	}

	// Try GitLab
	owner, repo, isGitLab := parseGitLabRepo(remoteURL)
	if isGitLab {
		m.GitRepoOwner = owner
		m.GitRepoName = repo
		m.CodeHostingResourceType = CodeHostingResourceTypeGitLab
		return nil
	}

	// Fallback: check for platform-specific files/dirs
	// This handles self-hosted GitHub/GitLab instances
	if detectGitHubIndicators() {
		owner, repo, err := parseGenericGitURL(remoteURL)
		if err != nil {
			return fmt.Errorf("detected GitHub repository structure (found .github directory) but failed to parse remote URL: %w", err)
		}
		m.GitRepoOwner = owner
		m.GitRepoName = repo
		m.CodeHostingResourceType = CodeHostingResourceTypeGitHub
		return nil
	}

	if detectGitLabIndicators() {
		owner, repo, err := parseGenericGitURL(remoteURL)
		if err != nil {
			return fmt.Errorf("detected GitLab repository structure (found .gitlab-ci.yml) but failed to parse remote URL: %w", err)
		}
		m.GitRepoOwner = owner
		m.GitRepoName = repo
		m.CodeHostingResourceType = CodeHostingResourceTypeGitLab
		return nil
	}

	return fmt.Errorf(`repository must be hosted on GitHub or GitLab.
	
Remote URL: %s`, remoteURL)
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
	state := fmt.Sprintf(`{"clientId":"%s","userId":"%s","source":"cli-init-cloud"}`, clientID, m.UserId)
	encodedState := url.QueryEscape(state)

	githubAppName := utils.EnvDefault("GITHUB_APP_NAME", "use-tusk")

	return fmt.Sprintf("https://github.com/apps/%s/installations/new?state=%s",
		githubAppName, encodedState)
}

func (m *Model) getGitlabAuthURL() string {
	return "https://app.usetusk.ai/app/settings/connect-gitlab"
}

func openCodeHostingAuthBrowser(m *Model) {
	authURL := ""
	switch m.CodeHostingResourceType {
	case CodeHostingResourceTypeGitHub:
		authURL = m.getGithubAuthURL()
	case CodeHostingResourceTypeGitLab:
		authURL = m.getGitlabAuthURL()
	}

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

func (m *Model) buildGitlabAuthMessage() string {
	return fmt.Sprintf(`Tusk cannot access %s/%s yet.

Please connect your GitLab account and grant access to this repository.

Visit: %s

After setup:
  1. Add your GitLab personal access token
  2. Grant access to %s/%s
  3. Press Enter again to retry verification

Press Enter to open browser...`,
		m.GitRepoOwner, m.GitRepoName,
		m.GitRepoOwner, m.GitRepoName,
		m.getGitlabAuthURL())
}

func (m *Model) buildCodeHostingAuthMessage() string {
	switch m.CodeHostingResourceType {
	case CodeHostingResourceTypeGitHub:
		return m.buildGithubAuthMessage()
	case CodeHostingResourceTypeGitLab:
		return m.buildGitlabAuthMessage()
	default:
		return fmt.Sprintf("Error: Unknown code hosting resource type: %d", m.CodeHostingResourceType)
	}
}

// getAppDir returns the relative path from git repo root to current directory
// Returns empty string if current directory is the repo root
func getAppDir() (string, error) {
	repoRoot, err := getGitRootDir()
	if err != nil {
		return "", err
	}

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
