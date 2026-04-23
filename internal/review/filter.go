// Package review contains helpers used by the `tusk review` command:
// file-filter lists mirroring the backend, pre-flight git checks, patch
// generation, and status polling.
package review

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

var ExtensionsToSkip = []string{
	// Images
	".png", ".jpg", ".jpeg", ".gif", ".bmp", ".tif", ".tiff", ".ico", ".svg", ".webp", ".heic",
	// Audio
	".mp3", ".wav", ".wma", ".ogg", ".flac", ".m4a", ".aac", ".midi", ".mid",
	// Video
	".mp4", ".avi", ".mkv", ".mov", ".wmv", ".m4v", ".3gp", ".3g2", ".rm", ".swf", ".flv", ".webm", ".mpg", ".mpeg",
	// Fonts
	".otf", ".ttf",
	// Documents
	".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".rtf", ".odt", ".ods", ".odp",
	// Archives
	".iso", ".bin", ".tar", ".zip", ".7z", ".gz", ".rar", ".bz2", ".xz",
	// Minified and source maps
	".min.js", ".min.js.map", ".js.map", ".min.css", ".min.css.map",
	// Data and configuration
	".tfstate", ".tfstate.backup", ".parquet", ".pyc", ".pub", ".pem", ".lock", ".sqlite", ".db", ".env", ".log",
	// Compiled code
	".class", ".dll", ".exe",
	// Design files
	".psd", ".ai", ".sketch",
	// 3D and CAD
	".stl", ".obj", ".dwg",
	// Backup files
	".bak", ".old", ".tmp",
}

var FilesToSkip = []string{
	"pnpm-lock.yaml",
	"package-lock.json",
	".DS_Store",
	".gitignore",
	"bun.lockb",
	"npm-debug.log",
	"yarn-error.log",
	"Thumbs.db",
	"Gemfile.lock",
}

var DirectoriesToSkip = []string{
	// Version control & IDE
	".git", ".vscode", ".idea",
	// JavaScript/Node
	"node_modules", "dist", "build", "out", ".next", ".nuxt", ".turbo",
	".parcel-cache", ".svelte-kit", ".vercel", ".angular", ".nx", "bower_components",
	// Python
	"__pycache__", ".venv", "venv", "env", ".pytest_cache", ".mypy_cache", ".tox",
	// Java/JVM
	"target", ".gradle",
	// Ruby
	".bundle",
	// Go/General vendor
	"vendor",
	// Infrastructure
	".terraform", ".serverless",
	// General
	"assets", "coverage", "tmp", "temp", "logs", "generated", ".cache", ".sass-cache",
}

// BuildPathspecExclusions returns a list of git pathspec strings (each prefixed
// with `:(exclude,glob)`) that can be passed to `git diff` to filter out files
// the backend code-review pipeline would skip anyway, plus any user-supplied
// extras.
//
// includes cancels individual default exclusions: any default pattern that
// doublestar-matches an include glob is dropped. This gives the user an
// escape hatch without needing git pathspec's more awkward include form.
func BuildPathspecExclusions(extraExcludes []string, includes []string) []string {
	var defaults []string
	for _, name := range FilesToSkip {
		defaults = append(defaults, "**/"+name)
	}
	for _, ext := range ExtensionsToSkip {
		defaults = append(defaults, "**/*"+ext)
	}
	for _, dir := range DirectoriesToSkip {
		defaults = append(defaults, "**/"+dir+"/**")
	}

	filtered := defaults[:0]
	for _, pattern := range defaults {
		drop := false
		for _, inc := range includes {
			if match, _ := doublestar.Match(inc, pattern); match {
				drop = true
				break
			}
			// Also drop if any file path matched by the default could be
			// matched by the include. Cheap heuristic: treat include as
			// a path and check if the default would match it.
			if match, _ := doublestar.Match(pattern, inc); match {
				drop = true
				break
			}
		}
		if !drop {
			filtered = append(filtered, pattern)
		}
	}

	all := make([]string, 0, len(filtered)+len(extraExcludes))
	for _, p := range filtered {
		all = append(all, ":(exclude,glob)"+p)
	}
	for _, p := range extraExcludes {
		all = append(all, ":(exclude,glob)"+p)
	}
	return all
}

// ReadTuskignore parses `.tuskignore` at the given repo root if present.
// Returns a list of glob patterns; comments and blanks are ignored.
// Missing file returns (nil, nil).
func ReadTuskignore(repoRoot string) ([]string, error) {
	path := filepath.Join(repoRoot, ".tuskignore")
	f, err := os.Open(path) //nolint:gosec // reading user repo file is intended
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var patterns []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns, scanner.Err()
}
