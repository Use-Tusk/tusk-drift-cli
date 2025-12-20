package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Trusted CDN hosts for SDK manifests
var trustedManifestHosts = []string{
	"unpkg.com",
	"cdn.jsdelivr.net",
	"registry.npmjs.org",
}

// FetchSDKManifest fetches an SDK instrumentation manifest from a trusted CDN
// This is a non-confirmable tool since it only fetches from whitelisted URLs
func FetchSDKManifest(input json.RawMessage) (string, error) {
	var params struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	if params.URL == "" {
		return "", fmt.Errorf("url is required")
	}

	// Validate URL is from a trusted host
	isTrusted := false
	for _, host := range trustedManifestHosts {
		if strings.Contains(params.URL, host) {
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

	resp, err := client.Get(params.URL)
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

	// Validate it's valid JSON
	var manifest map[string]interface{}
	if err := json.Unmarshal(body, &manifest); err != nil {
		return "", fmt.Errorf("invalid manifest JSON: %w", err)
	}

	return string(body), nil
}
