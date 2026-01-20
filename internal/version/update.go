package version

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

const (
	latestVersionURL = "https://use-tusk.github.io/tusk-drift-cli/latest.json"
	releaseURLFormat = "https://github.com/Use-Tusk/tusk-drift-cli/releases/download/%s/tusk-drift-cli_%s_%s_%s.%s"
)

// LatestRelease represents the response from the version check endpoint.
type LatestRelease struct {
	Version     string `json:"version"`
	PublishedAt string `json:"published_at"`
	URL         string `json:"url"`
}

// CheckForUpdate checks if a newer version is available.
// Returns the latest release info if an update is available, nil otherwise.
func CheckForUpdate(ctx context.Context) (*LatestRelease, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestVersionURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var release LatestRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, err
	}

	currentV := Version
	if !strings.HasPrefix(currentV, "v") {
		currentV = "v" + currentV
	}
	latestV := release.Version
	if !strings.HasPrefix(latestV, "v") {
		latestV = "v" + latestV
	}

	if Version == "dev" || semver.Compare(currentV, latestV) >= 0 {
		return nil, nil
	}

	return &release, nil
}

// PromptAndUpdate prompts the user to update and performs the update if confirmed.
// Returns true if an update was performed.
func PromptAndUpdate(release *LatestRelease) bool {
	if release == nil {
		return false
	}

	fmt.Printf("\nA new version of Tusk CLI is available: %s (current: %s)\n", release.Version, Version)
	fmt.Printf("Release notes: %s\n", release.URL)
	fmt.Print("\nWould you like to update now? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response != "y" && response != "yes" {
		fmt.Println("Update skipped.")
		return false
	}

	return performUpdate(release)
}

// AutoUpdate automatically updates to the specified release without prompting.
// Returns true if an update was performed.
func AutoUpdate(release *LatestRelease) bool {
	if release == nil {
		return false
	}

	fmt.Printf("\nA new version of Tusk CLI is available: %s (current: %s)\n", release.Version, Version)
	fmt.Printf("Release notes: %s\n", release.URL)
	fmt.Println("\nAuto-updating...")

	return performUpdate(release)
}

// performUpdate downloads and installs the update, printing status messages.
func performUpdate(release *LatestRelease) bool {
	fmt.Printf("Downloading %s...\n", release.Version)
	if err := SelfUpdate(release); err != nil {
		fmt.Printf("Update failed: %v\n", err)
		fmt.Printf("You can download the latest release from: %s\n", release.URL)
		return false
	}

	fmt.Printf("\nSuccessfully updated to %s!\n", release.Version)
	fmt.Println("Please restart tusk to use the new version.")
	return true
}

// SelfUpdate downloads and installs the specified release.
func SelfUpdate(release *LatestRelease) error {
	// Get the current executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	downloadURL := getDownloadURL(release.Version)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}

	archiveData, err := io.ReadAll(resp.Body)
	if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("failed to read download: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	var newBinary []byte
	if runtime.GOOS == "windows" {
		newBinary, err = extractFromZip(archiveData, "tusk.exe")
	} else {
		newBinary, err = extractFromTarGz(archiveData, "tusk")
	}
	if err != nil {
		return fmt.Errorf("failed to extract binary: %w", err)
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(execPath), ".tusk-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(newBinary); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Chmod(tmpPath, 0o755); err != nil { //#nosec G302 -- executable binary needs 0755
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	if err := os.Rename(tmpPath, execPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to replace binary: %w", err)
	}

	return nil
}

// getDownloadURL builds the download URL for the current platform.
func getDownloadURL(version string) string {
	// Strip 'v' prefix for the filename (goreleaser uses version without 'v' in filename)
	ver := strings.TrimPrefix(version, "v")

	// Map Go OS names to goreleaser names
	osName := runtime.GOOS
	switch osName {
	case "darwin":
		osName = "Darwin"
	case "linux":
		osName = "Linux"
	case "windows":
		osName = "Windows"
	}

	// Map Go arch names to goreleaser names
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x86_64"
	}

	// Extension based on OS
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}

	return fmt.Sprintf(releaseURLFormat, version, ver, osName, arch, ext)
}

// extractFromTarGz extracts a specific file from a tar.gz archive.
func extractFromTarGz(archiveData []byte, filename string) ([]byte, error) {
	gzr, err := gzip.NewReader(bytes.NewReader(archiveData))
	if err != nil {
		return nil, err
	}

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			if closeErr := gzr.Close(); closeErr != nil {
				return nil, closeErr
			}
			return nil, fmt.Errorf("file %s not found in archive", filename)
		}
		if err != nil {
			_ = gzr.Close()
			return nil, err
		}

		if header.Typeflag == tar.TypeReg && filepath.Base(header.Name) == filename {
			data, err := io.ReadAll(tr)
			if closeErr := gzr.Close(); closeErr != nil && err == nil {
				return nil, closeErr
			}
			return data, err
		}
	}
}

// extractFromZip extracts a specific file from a zip archive.
func extractFromZip(archiveData []byte, filename string) ([]byte, error) {
	zipReader, err := zip.NewReader(bytes.NewReader(archiveData), int64(len(archiveData)))
	if err != nil {
		return nil, err
	}

	for _, f := range zipReader.File {
		if filepath.Base(f.Name) == filename {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			fileData, readErr := io.ReadAll(rc)
			closeErr := rc.Close()
			if readErr != nil {
				return nil, readErr
			}
			if closeErr != nil {
				return nil, closeErr
			}
			return fileData, nil
		}
	}

	return nil, fmt.Errorf("file %s not found in archive", filename)
}
