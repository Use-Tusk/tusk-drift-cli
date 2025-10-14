package onboard

import (
	"os"
	"path/filepath"
	"strings"
)

func hasPackageJSON() bool {
	if fi, err := os.Stat("package.json"); err == nil && !fi.IsDir() {
		return true
	}
	return false
}

func hasJavaScriptFiles() bool {
	patterns := []string{"*.js", "*.ts", "*.jsx", "*.tsx", "*.mjs", "*.cjs"}

	entries, err := os.ReadDir(".")
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		for _, pattern := range patterns {
			if matched, _ := filepath.Match(pattern, name); matched {
				return true
			}
		}
	}

	commonDirs := []string{"src", "lib", "dist", "build"}
	for _, dir := range commonDirs {
		if entries, err := os.ReadDir(dir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				name := entry.Name()
				for _, pattern := range patterns {
					if matched, _ := filepath.Match(pattern, name); matched {
						return true
					}
				}
			}
		}
	}
	return false
}

func isEmptyDirectory() bool {
	entries, err := os.ReadDir(".")
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".") {
			return false
		}
	}
	return true
}

func inferServiceNameFromDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return "my-service"
	}
	return filepath.Base(wd)
}

func getwdSafe() (string, error) {
	return os.Getwd()
}
