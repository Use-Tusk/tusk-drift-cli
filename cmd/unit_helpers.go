package cmd

import (
	"fmt"
	"net/url"
	"os/exec"
	"strings"
)

func getCurrentGitBranch() (string, error) {
	out, err := exec.Command("git", "branch", "--show-current").Output() //nolint:gosec
	if err != nil {
		return "", fmt.Errorf("failed to detect current git branch: %w", err)
	}

	branch := strings.TrimSpace(string(out))
	if branch == "" {
		return "", fmt.Errorf("failed to detect current git branch")
	}

	return branch, nil
}

func getOriginRepoSlug() (string, error) {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output() //nolint:gosec
	if err != nil {
		return "", fmt.Errorf("failed to detect git remote `origin`: %w", err)
	}

	repoSlug, err := parseRepoSlugFromRemote(strings.TrimSpace(string(out)))
	if err != nil {
		return "", err
	}

	return repoSlug, nil
}

func parseRepoSlugFromRemote(remoteURL string) (string, error) {
	if remoteURL == "" {
		return "", fmt.Errorf("empty remote URL")
	}

	var path string

	switch {
	case strings.HasPrefix(remoteURL, "git@"):
		parts := strings.SplitN(remoteURL, ":", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("failed to parse git remote URL: %s", remoteURL)
		}
		path = parts[1]
	case strings.Contains(remoteURL, "://"):
		parsed, err := url.Parse(remoteURL)
		if err != nil {
			return "", fmt.Errorf("failed to parse git remote URL: %w", err)
		}
		path = strings.TrimPrefix(parsed.Path, "/")
	default:
		return "", fmt.Errorf("unsupported git remote URL format: %s", remoteURL)
	}

	path = strings.TrimSuffix(path, ".git")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("failed to infer owner/repo from remote URL: %s", remoteURL)
	}

	return parts[len(parts)-2] + "/" + parts[len(parts)-1], nil
}

func resolveLatestRunInput(repo string, branch string) (string, string, error) {
	var err error

	if repo == "" {
		repo, err = getOriginRepoSlug()
		if err != nil {
			return "", "", fmt.Errorf("repo is required; pass --repo or run inside a git repo with an origin remote: %w", err)
		}
	}

	if branch == "" {
		branch, err = getCurrentGitBranch()
		if err != nil {
			return "", "", fmt.Errorf("branch is required; pass --branch or run inside a git repo on a named branch: %w", err)
		}
	}

	return repo, branch, nil
}
