package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	TuskDirName    = ".tusk"
	TracesSubDir   = "traces"
	LogsSubDir     = "logs"
	ConfigFileName = "config.yaml"
)

// Optional override for local traces directory (set by config or CLI flag)
var tracesDirOverride string

// List of directories to search for trace files
var PossibleTraceDirs = []string{
	".tusk/traces",
	"traces",
	"tmp",
	".",
}

// FindTuskRoot traverses up the directory tree looking for a .tusk directory
// Returns the directory containing .tusk, or empty string if not found
func FindTuskRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}

	// Start from current directory and traverse up
	currentDir := wd
	for {
		tuskPath := filepath.Join(currentDir, TuskDirName)
		if info, err := os.Stat(tuskPath); err == nil && info.IsDir() {
			return currentDir
		}

		// Get parent directory
		parent := filepath.Dir(currentDir)

		// If we've reached the root or can't go up further, stop
		if parent == currentDir || parent == "." {
			break
		}

		currentDir = parent
	}

	return ""
}

// GetTuskDir returns the .tusk directory path (either local or in home directory)
func GetTuskDir() string {
	if root := FindTuskRoot(); root != "" {
		return filepath.Join(root, TuskDirName)
	}

	if homeDir, err := os.UserHomeDir(); err == nil {
		homeTuskPath := filepath.Join(homeDir, TuskDirName)
		if _, err := os.Stat(homeTuskPath); err == nil {
			return homeTuskPath
		}
	}

	// Last resort: return relative .tusk path
	return TuskDirName
}

// GetTuskRoot returns the root directory containing .tusk, or current directory if not found
func GetTuskRoot() string {
	if root := FindTuskRoot(); root != "" {
		return root
	}

	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

// ResolveTuskPath resolves a path relative to the tusk root if it's relative,
// or returns it as-is if it's already absolute
func ResolveTuskPath(path string) string {
	if path == "" {
		return path
	}

	if filepath.IsAbs(path) {
		return path
	}

	root := GetTuskRoot()
	return filepath.Join(root, path)
}

// GetTracesDir returns the traces directory path
func GetTracesDir() string {
	if tracesDirOverride != "" {
		return tracesDirOverride
	}
	return filepath.Join(GetTuskDir(), TracesSubDir)
}

// SetTracesDirOverride sets an explicit traces directory to use.
func SetTracesDirOverride(dir string) {
	tracesDirOverride = dir
}

// GetPossibleTraceDirs returns the list of directories to search for trace files, preferring override first.
func GetPossibleTraceDirs() []string {
	if tracesDirOverride == "" {
		return PossibleTraceDirs
	}
	out := []string{tracesDirOverride}
	seen := map[string]struct{}{tracesDirOverride: {}}
	for _, d := range PossibleTraceDirs {
		if _, ok := seen[d]; !ok {
			out = append(out, d)
		}
	}
	return out
}

// GetLogsDir returns the logs directory path
func GetLogsDir() string {
	return filepath.Join(GetTuskDir(), LogsSubDir)
}

// EnsureDir creates a directory if it doesn't exist
func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0o750)
}

// FindTraceFile searches for a JSONL trace file containing the given trace ID.
// If filename is provided, it tries that first before searching
func FindTraceFile(traceID string, filename string) (string, error) {
	tracesDir := GetTracesDir()

	if _, err := os.Stat(tracesDir); os.IsNotExist(err) {
		return "", fmt.Errorf("traces directory not found: %s", tracesDir)
	}

	if filename != "" {
		var fullPath string

		switch {
		case filepath.IsAbs(filename):
			fullPath = filename
		case strings.Contains(filename, tracesDir):
			fullPath = filename
		default:
			fullPath = filepath.Join(tracesDir, filename)
		}

		if _, err := os.Stat(fullPath); err == nil {
			return fullPath, nil
		}
	}

	var foundFile string
	err := filepath.Walk(tracesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !strings.HasSuffix(path, ".jsonl") {
			return nil
		}

		filename := filepath.Base(path)
		if strings.Contains(filename, traceID) {
			foundFile = path
			return filepath.SkipDir
		}

		return nil
	})
	if err != nil {
		return "", fmt.Errorf("error searching for trace file: %w", err)
	}

	if foundFile == "" {
		return "", fmt.Errorf("no trace file found for trace ID: %s", traceID)
	}

	return foundFile, nil
}
