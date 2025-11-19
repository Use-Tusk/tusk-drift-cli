package runner

import (
	"fmt"
	"log/slog"

	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
)

// EnvironmentGroup represents tests grouped by environment
type EnvironmentGroup struct {
	Name        string            // Environment name (e.g., "production", "staging", "default")
	Tests       []Test            // Tests for this environment
	EnvVars     map[string]string // Environment variables extracted from ENV_VARS span
	EnvVarsSpan *core.Span        // Source span for provenance/debugging (can be nil)
}

// EnvironmentExtractionResult contains the result of grouping tests by environment
type EnvironmentExtractionResult struct {
	Groups   []*EnvironmentGroup // Grouped tests by environment
	Warnings []string            // Non-fatal warnings (e.g., missing ENV_VARS)
}

// GroupTestsByEnvironment analyzes tests and groups them by environment
// preAppStartSpans should contain all pre-app-start spans (including ENV_VARS spans)
// Returns grouped tests and any warnings encountered
func GroupTestsByEnvironment(tests []Test, preAppStartSpans []*core.Span) (*EnvironmentExtractionResult, error) {
	result := &EnvironmentExtractionResult{
		Groups:   []*EnvironmentGroup{},
		Warnings: []string{},
	}

	// Group tests by environment name
	envToTests := make(map[string][]Test)
	for _, test := range tests {
		env := extractEnvironmentFromTest(&test)
		if env == "" {
			env = "default"
		}
		envToTests[env] = append(envToTests[env], test)
	}

	// For each environment, extract env vars and create group
	for envName, envTests := range envToTests {
		envVars, envVarsSpan, err := extractEnvVarsForEnvironment(preAppStartSpans, envName)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to extract env vars for %s: %v", envName, err))
			envVars = make(map[string]string) // Use empty map on error
		}
		if envVarsSpan == nil && envName != "default" {
			result.Warnings = append(result.Warnings, fmt.Sprintf("No ENV_VARS span found for environment: %s", envName))
		}

		result.Groups = append(result.Groups, &EnvironmentGroup{
			Name:        envName,
			Tests:       envTests,
			EnvVars:     envVars,
			EnvVarsSpan: envVarsSpan,
		})
	}

	return result, nil
}

// extractEnvironmentFromTest looks for metadata.environment field in test spans
// Priority:
//  1. Check Test.Metadata["environment"] (already loaded from spans)
//  2. Iterate through Test.Spans and look for metadata.environment in any span
//  3. Return empty string if not found
func extractEnvironmentFromTest(test *Test) string {
	// Check test-level metadata first
	if env, ok := test.Metadata["environment"]; ok {
		if envStr, ok := env.(string); ok && envStr != "" {
			return envStr
		}
	}

	// Check spans for environment metadata
	for _, span := range test.Spans {
		if span.Metadata != nil {
			metadataMap := span.Metadata.AsMap()
			if env, ok := metadataMap["environment"]; ok {
				if envStr, ok := env.(string); ok && envStr != "" {
					return envStr
				}
			}
		}
	}

	return ""
}

// extractEnvVarsForEnvironment finds the ENV_VARS span for a given environment
// Searches through preAppStartSpans for process.env spans
// If multiple exist for same environment, selects most recent by timestamp
func extractEnvVarsForEnvironment(preAppStartSpans []*core.Span, environment string) (map[string]string, *core.Span, error) {
	var candidateSpans []*core.Span

	// Search through pre-app-start spans for ENV_VARS spans
	for _, span := range preAppStartSpans {
		// Filter for process.env spans that are pre-app-start
		if span.PackageName == "process.env" && span.IsPreAppStart {
			candidateSpans = append(candidateSpans, span)
			slog.Debug("Found ENV_VARS span candidate",
				"environment", environment,
				"spanId", span.SpanId,
				"packageName", span.PackageName,
				"isPreAppStart", span.IsPreAppStart)
		}
	}

	if len(candidateSpans) == 0 {
		slog.Debug("No ENV_VARS spans found in preAppStartSpans",
			"environment", environment,
			"preAppStartSpan_count", len(preAppStartSpans))
		return make(map[string]string), nil, nil
	}

	slog.Debug("Found ENV_VARS span candidates",
		"environment", environment,
		"candidate_count", len(candidateSpans))

	// Select most recent span if multiple
	selectedSpan := findMostRecentEnvVarsSpan(candidateSpans)
	if selectedSpan == nil {
		return make(map[string]string), nil, nil
	}

	// Extract ENV_VARS from the selected span's metadata
	envVars, err := parseEnvVarsFromMetadata(selectedSpan)
	if err != nil {
		slog.Debug("Failed to parse ENV_VARS from span metadata",
			"environment", environment,
			"spanId", selectedSpan.SpanId,
			"error", err)
		return make(map[string]string), selectedSpan, err
	}

	slog.Debug("Successfully extracted ENV_VARS",
		"environment", environment,
		"env_var_count", len(envVars))

	return envVars, selectedSpan, nil
}

// findMostRecentEnvVarsSpan selects the latest ENV_VARS span based on timestamp
func findMostRecentEnvVarsSpan(spans []*core.Span) *core.Span {
	if len(spans) == 0 {
		return nil
	}

	mostRecent := spans[0]
	for _, span := range spans[1:] {
		if span.Timestamp != nil && mostRecent.Timestamp != nil {
			if span.Timestamp.AsTime().After(mostRecent.Timestamp.AsTime()) {
				mostRecent = span
			}
		}
	}

	return mostRecent
}

// parseEnvVarsFromMetadata extracts ENV_VARS map from span metadata
// ENV_VARS is expected to be a nested object in metadata
func parseEnvVarsFromMetadata(span *core.Span) (map[string]string, error) {
	if span == nil || span.Metadata == nil {
		slog.Debug("parseEnvVarsFromMetadata: span or metadata is nil",
			"span_nil", span == nil,
			"metadata_nil", span == nil || span.Metadata == nil)
		return make(map[string]string), nil
	}

	metadataMap := span.Metadata.AsMap()

	// Get metadata keys for debugging
	keys := make([]string, 0, len(metadataMap))
	for k := range metadataMap {
		keys = append(keys, k)
	}
	slog.Debug("parseEnvVarsFromMetadata: checking metadata",
		"spanId", span.SpanId,
		"metadata_keys", keys)

	envVarsRaw, ok := metadataMap["ENV_VARS"]
	if !ok {
		slog.Debug("parseEnvVarsFromMetadata: ENV_VARS key not found in metadata",
			"spanId", span.SpanId)
		return make(map[string]string), nil
	}

	// Convert to map[string]string
	envVars := make(map[string]string)

	switch v := envVarsRaw.(type) {
	case map[string]interface{}:
		for key, val := range v {
			if strVal, ok := val.(string); ok {
				envVars[key] = strVal
			} else if val != nil {
				envVars[key] = fmt.Sprintf("%v", val)
			}
		}
	case map[string]string:
		envVars = v
	default:
		slog.Warn("ENV_VARS metadata has unexpected type", "type", fmt.Sprintf("%T", v))
		return make(map[string]string), fmt.Errorf("ENV_VARS has unexpected type: %T", v)
	}

	return envVars, nil
}
