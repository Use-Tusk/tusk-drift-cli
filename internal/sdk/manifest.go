package sdk

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SDK manifest URLs
const (
	NodeSDKManifestURL   = "https://unpkg.com/@use-tusk/drift-node-sdk@latest/dist/instrumentation-manifest.json"
	PythonSDKManifestURL = "https://use-tusk.github.io/drift-python-sdk/instrumentation-manifest.json"
)

// Trusted CDN hosts for SDK manifests
var trustedManifestHosts = []string{
	"unpkg.com",
	"cdn.jsdelivr.net",
	"registry.npmjs.org",
	"raw.githubusercontent.com",
	"pypi.org",
	"use-tusk.github.io",
}

// GetManifestURLForProjectType returns the SDK manifest URL for a given project type.
// Returns empty string if no manifest URL is available for the project type.
func GetManifestURLForProjectType(projectType string) string {
	switch projectType {
	case "nodejs":
		return NodeSDKManifestURL
	case "python":
		return PythonSDKManifestURL
	default:
		return ""
	}
}

// FetchManifestFromURL fetches an SDK manifest from a URL
// Returns the raw JSON string
func FetchManifestFromURL(manifestURL string) (string, error) {
	parsedURL, err := url.Parse(manifestURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	isTrusted := false
	host := strings.ToLower(parsedURL.Hostname())
	for _, trustedHost := range trustedManifestHosts {
		if host == trustedHost || strings.HasSuffix(host, "."+trustedHost) {
			isTrusted = true
			break
		}
	}
	if !isTrusted {
		return "", fmt.Errorf("URL must be from a trusted CDN (%s)", strings.Join(trustedManifestHosts, ", "))
	}

	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	resp, err := client.Get(manifestURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch manifest: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("manifest fetch failed with status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest map[string]any
	if err := json.Unmarshal(body, &manifest); err != nil {
		return "", fmt.Errorf("invalid manifest JSON: %w", err)
	}

	return string(body), nil
}
