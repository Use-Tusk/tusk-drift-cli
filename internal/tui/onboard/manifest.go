package onboard

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Use-Tusk/tusk-drift-cli/internal/sdk"
)

// Manifest represents the SDK instrumentation manifest structure
type Manifest struct {
	SDKVersion       string            `json:"sdkVersion"`
	Language         string            `json:"language"`
	Instrumentations []Instrumentation `json:"instrumentations"`
}

// Instrumentation represents a single instrumentation entry
type Instrumentation struct {
	PackageName       string   `json:"packageName"`
	SupportedVersions []string `json:"supportedVersions"`
}

// FetchSDKPackagesDescription fetches the SDK manifest and formats it for display
// TODO-PYTHON: Accept project type and handle Python manifest
func FetchSDKPackagesDescription() string {
	manifestJSON, err := sdk.FetchManifestFromURL(sdk.NodeSDKManifestURL)
	if err != nil {
		return fallbackSDKPackagesDescription()
	}

	var manifest Manifest
	if err := json.Unmarshal([]byte(manifestJSON), &manifest); err != nil {
		return fallbackSDKPackagesDescription()
	}

	return formatManifestForDisplay(&manifest)
}

// formatManifestForDisplay converts the manifest into a human-readable bullet list
func formatManifestForDisplay(manifest *Manifest) string {
	if len(manifest.Instrumentations) == 0 {
		return fallbackSDKPackagesDescription()
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Tusk Drift Node SDK (v%s) currently supports:\n", manifest.SDKVersion))

	for _, inst := range manifest.Instrumentations {
		versions := formatVersions(inst.SupportedVersions)
		sb.WriteString(fmt.Sprintf("  • %s: %s\n", inst.PackageName, versions))
	}

	return sb.String()
}

// formatVersions converts version array to a readable string
func formatVersions(versions []string) string {
	if len(versions) == 0 {
		return "all versions"
	}
	if len(versions) == 1 && versions[0] == "*" {
		return "all versions"
	}
	return strings.Join(versions, ", ")
}

// fallbackSDKPackagesDescription returns the hardcoded list if fetching fails
func fallbackSDKPackagesDescription() string {
	return `Tusk Drift Node SDK currently supports:
  • HTTP/HTTPS: All versions (Node.js built-in)
  • GRPC: @grpc/grpc-js@1.x (Outbound requests only)
  • PG: pg@8.x, pg-pool@2.x-3.x
  • Firestore: @google-cloud/firestore@7.x
  • Postgres: postgres@3.x
  • MySQL: mysql2@3.x, mysql@2.x
  • IORedis: ioredis@4.x-5.x
  • Upstash Redis: @upstash/redis@1.x
  • GraphQL: graphql@15.x-16.x
  • JSON Web Tokens: jsonwebtoken@5.x-9.x
  • JWKS RSA: jwks-rsa@1.x-3.x
`
}
