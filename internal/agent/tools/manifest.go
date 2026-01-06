package tools

import "github.com/Use-Tusk/tusk-drift-cli/internal/sdk"

// Re-export SDK manifest constants and functions from the shared sdk package
// for backward compatibility with existing code.

const (
	NodeSDKManifestURL   = sdk.NodeSDKManifestURL
	PythonSDKManifestURL = sdk.PythonSDKManifestURL
)

// GetManifestURLForProjectType returns the SDK manifest URL for a given project type.
// Returns empty string if no manifest URL is available for the project type.
func GetManifestURLForProjectType(projectType string) string {
	return sdk.GetManifestURLForProjectType(projectType)
}

// FetchManifestFromURL fetches an SDK manifest from a URL
// Returns the raw JSON string
func FetchManifestFromURL(url string) (string, error) {
	return sdk.FetchManifestFromURL(url)
}
