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

// extractEnvironmentFromTest extracts environment from test or its spans
// Priority:
//  1. Check Test.Environment field (populated when test is created)
//  2. Fallback to checking spans directly using span.GetEnvironment()
//  3. Return empty string if not found
func extractEnvironmentFromTest(test *Test) string {
	if test.Environment != "" {
		return test.Environment
	}

	// Fallback: check spans directly for environment
	for _, span := range test.Spans {
		if env := span.GetEnvironment(); env != "" {
			return env
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
			slog.Debug("Found ENV_VARS span candidates",
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

	// Extract ENV_VARS from the selected span's output value
	envVars, err := parseEnvVarsFromOutputValue(selectedSpan)
	if err != nil {
		slog.Debug("Failed to parse ENV_VARS from span output value",
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

// parseEnvVarsFromOutputValue extracts ENV_VARS map from span output value
// ENV_VARS is expected to be a nested object in output value
func parseEnvVarsFromOutputValue(span *core.Span) (map[string]string, error) {
	if span == nil || span.OutputValue == nil {
		slog.Debug("parseEnvVarsFromOutputValue: span or output value is nil",
			"span_nil", span == nil,
			"output_value_nil", span == nil || span.OutputValue == nil)
		return make(map[string]string), nil
	}

	outputValueMap := span.OutputValue.AsMap()

	// Get metadata keys for debugging
	keys := make([]string, 0, len(outputValueMap))
	for k := range outputValueMap {
		keys = append(keys, k)
	}
	slog.Debug("parseEnvVarsFromOutputValue: checking output value",
		"spanId", span.SpanId,
		"metadata_keys", keys)

	envVarsRaw, ok := outputValueMap["ENV_VARS"]
	if !ok {
		slog.Debug("parseEnvVarsFromOutputValue: ENV_VARS key not found in output value",
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
		slog.Warn("ENV_VARS output value has unexpected type", "type", fmt.Sprintf("%T", v))
		return make(map[string]string), fmt.Errorf("ENV_VARS has unexpected type: %T", v)
	}

	return envVars, nil
}
