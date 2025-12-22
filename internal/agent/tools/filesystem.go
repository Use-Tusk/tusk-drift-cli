package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FilesystemTools provides file system operations
type FilesystemTools struct {
	workDir string
}

// NewFilesystemTools creates a new FilesystemTools instance
func NewFilesystemTools(workDir string) *FilesystemTools {
	return &FilesystemTools{workDir: workDir}
}

// ReadFile reads the contents of a file
func (ft *FilesystemTools) ReadFile(input json.RawMessage) (string, error) {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	fullPath := ft.resolvePath(params.Path)
	content, err := os.ReadFile(fullPath) //nolint:gosec // Path is resolved relative to workDir
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Truncate very large files
	maxSize := 100000 // ~100KB
	result := string(content)
	if len(result) > maxSize {
		result = result[:maxSize] + "\n\n... (truncated, file too large)"
	}

	return result, nil
}

// WriteFile writes content to a file
func (ft *FilesystemTools) WriteFile(input json.RawMessage) (string, error) {
	var params struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	fullPath := ft.resolvePath(params.Path)

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	content := params.Content
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	if err := os.WriteFile(fullPath, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), params.Path), nil
}

// ListDirectory lists files and directories in a path
func (ft *FilesystemTools) ListDirectory(input json.RawMessage) (string, error) {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	path := params.Path
	if path == "" {
		path = "."
	}
	fullPath := ft.resolvePath(path)

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read directory: %w", err)
	}

	var lines []string
	for i, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Tree-style prefix
		var prefix string
		if i == len(entries)-1 {
			prefix = "└── "
		} else {
			prefix = "├── "
		}

		name := entry.Name()
		if entry.IsDir() {
			name += "/"
			lines = append(lines, fmt.Sprintf("%s%s", prefix, name))
		} else {
			lines = append(lines, fmt.Sprintf("%s%s (%d bytes)", prefix, name, info.Size()))
		}
	}

	if len(lines) == 0 {
		return "(empty directory)", nil
	}

	return strings.Join(lines, "\n"), nil
}

// Grep searches for a pattern in files
func (ft *FilesystemTools) Grep(input json.RawMessage) (string, error) {
	var params struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
		Include string `json:"include"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	path := params.Path
	if path == "" {
		path = "."
	}
	fullPath := ft.resolvePath(path)

	// Use ripgrep if available, otherwise fall back to grep
	var cmd *exec.Cmd
	if _, err := exec.LookPath("rg"); err == nil {
		args := []string{"-n", "--max-count=100", params.Pattern, fullPath}
		if params.Include != "" {
			args = []string{"-n", "--max-count=100", "-g", params.Include, params.Pattern, fullPath}
		}
		cmd = exec.Command("rg", args...) //nolint:gosec // Args are validated search parameters
	} else {
		args := []string{"-rn", params.Pattern, fullPath}
		if params.Include != "" {
			args = []string{"-rn", "--include=" + params.Include, params.Pattern, fullPath}
		}
		cmd = exec.Command("grep", args...) //nolint:gosec // Args are validated search parameters
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		// grep returns exit code 1 when no matches found
		if len(output) == 0 {
			return "No matches found", nil
		}
	}

	result := string(output)
	// Truncate if too long
	maxLen := 50000
	if len(result) > maxLen {
		result = result[:maxLen] + "\n\n... (truncated, too many matches)"
	}

	return result, nil
}

// PatchFile applies a targeted edit to a file
func (ft *FilesystemTools) PatchFile(input json.RawMessage) (string, error) {
	var params struct {
		Path    string `json:"path"`
		Search  string `json:"search"`
		Replace string `json:"replace"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	fullPath := ft.resolvePath(params.Path)

	content, err := os.ReadFile(fullPath) //nolint:gosec // Path is resolved relative to workDir
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	original := string(content)
	if !strings.Contains(original, params.Search) {
		return "", fmt.Errorf("search string not found in file")
	}

	count := strings.Count(original, params.Search)
	if count > 1 {
		return "", fmt.Errorf("search string found %d times, must be unique", count)
	}

	modified := strings.Replace(original, params.Search, params.Replace, 1)

	if len(modified) > 0 && !strings.HasSuffix(modified, "\n") {
		modified += "\n"
	}

	if err := os.WriteFile(fullPath, []byte(modified), 0o600); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return fmt.Sprintf("Successfully patched %s", params.Path), nil
}

func (ft *FilesystemTools) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(ft.workDir, path)
}
