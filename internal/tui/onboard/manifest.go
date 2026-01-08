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
func FetchSDKPackagesDescription(projectType string) string {
	manifestURL := sdk.GetManifestURLForProjectType(projectType)
	if manifestURL == "" {
		return ""
	}

	manifestJSON, err := sdk.FetchManifestFromURL(manifestURL)
	if err != nil {
		return fallbackSDKPackagesDescription(projectType)
	}

	var manifest Manifest
	if err := json.Unmarshal([]byte(manifestJSON), &manifest); err != nil {
		return fallbackSDKPackagesDescription(projectType)
	}

	return formatManifestForDisplay(&manifest, projectType)
}

// formatManifestForDisplay converts the manifest into a human-readable bullet list
func formatManifestForDisplay(manifest *Manifest, projectType string) string {
	if len(manifest.Instrumentations) == 0 {
		return fallbackSDKPackagesDescription(projectType)
	}

	var sb strings.Builder
	sdkName := "Node"
	if projectType == "python" {
		sdkName = "Python"
	}
	sb.WriteString(fmt.Sprintf("Tusk Drift %s SDK (v%s) currently supports:\n", sdkName, manifest.SDKVersion))

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
func fallbackSDKPackagesDescription(projectType string) string {
	if projectType == "python" {
		return `Tusk Drift Python SDK currently supports:
  • HTTP/HTTPS: All versions (Python built-in urllib, requests, httpx, aiohttp)
  • Flask: flask@2.x-3.x
  • FastAPI: fastapi@0.100+
  • psycopg2: psycopg2@2.9+
  • asyncpg: asyncpg@0.27+
  • SQLAlchemy: sqlalchemy@2.x
  • PyMongo: pymongo@4.x
  • Redis: redis@4.x-5.x
`
	}
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
