package runner

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/Use-Tusk/tusk-drift-cli/internal/logging"
)

// ReplayTestsByEnvironment orchestrates environment-based test replay
// For each environment group:
//  1. Set environment variables
//  2. Start environment (server + service)
//  3. Run tests for that environment
//  4. Stop environment
//  5. Restore environment variables
func ReplayTestsByEnvironment(
	ctx context.Context,
	executor *Executor,
	groups []*EnvironmentGroup,
) ([]TestResult, error) {
	allResults := make([]TestResult, 0)

	for i, group := range groups {
		slog.Debug("Starting replay for environment group",
			"environment", group.Name,
			"test_count", len(group.Tests),
			"env_var_count", len(group.EnvVars),
			"group_index", i+1,
			"total_groups", len(groups))

		logging.LogToService(fmt.Sprintf("Running %d tests for environment: %s", len(group.Tests), group.Name))

		// 1. Set environment variables
		cleanup, err := SetEnvironmentVariables(group.EnvVars)
		if err != nil {
			return allResults, fmt.Errorf("failed to set env vars for %s: %w", group.Name, err)
		}

		// 2. Start environment (server + service)
		if err := executor.StartEnvironment(); err != nil {
			cleanup() // Restore env vars before returning
			return allResults, fmt.Errorf("failed to start environment for %s: %w", group.Name, err)
		}

		// 3. Run tests for this environment
		results, err := executor.RunTests(group.Tests)
		if err != nil {
			// Attempt cleanup even on error
			_ = executor.StopEnvironment()
			cleanup()
			return allResults, fmt.Errorf("failed to run tests for %s: %w", group.Name, err)
		}

		// 4. Collect results
		allResults = append(allResults, results...)

		// 5. Stop environment
		if err := executor.StopEnvironment(); err != nil {
			slog.Warn("Failed to stop environment cleanly",
				"environment", group.Name,
				"error", err)
			logging.LogToService(fmt.Sprintf("⚠️  Warning: failed to stop environment for %s: %v", group.Name, err))
		}

		// 6. Restore environment variables
		cleanup()

		slog.Debug("Completed replay for environment group",
			"environment", group.Name,
			"results_count", len(results))
	}

	slog.Debug("Completed all environment group replays",
		"total_groups", len(groups),
		"total_results", len(allResults))

	return allResults, nil
}

// SetEnvironmentVariables applies env vars to current process
// Returns cleanup function to restore original values
func SetEnvironmentVariables(envVars map[string]string) (cleanup func(), err error) {
	if len(envVars) == 0 {
		// No env vars to set, return no-op cleanup
		return func() {}, nil
	}

	// Store original values for cleanup
	originalVars := make(map[string]string)
	keysToUnset := make([]string, 0)

	for key := range envVars {
		if val, exists := os.LookupEnv(key); exists {
			originalVars[key] = val
		} else {
			keysToUnset = append(keysToUnset, key)
		}
	}

	// Set new values
	for key, val := range envVars {
		if err := os.Setenv(key, val); err != nil {
			// If setting fails, attempt to restore what we've already changed
			restoreEnvironmentVariables(originalVars, keysToUnset)
			return nil, fmt.Errorf("failed to set env var %s: %w", key, err)
		}
	}

	slog.Debug("Set environment variables", "count", len(envVars))

	// Return cleanup function
	cleanup = func() {
		restoreEnvironmentVariables(originalVars, keysToUnset)
	}

	return cleanup, nil
}

// restoreEnvironmentVariables restores original env var values
func restoreEnvironmentVariables(originalVars map[string]string, keysToUnset []string) {
	// Restore original values
	for key, val := range originalVars {
		if err := os.Setenv(key, val); err != nil {
			slog.Warn("Failed to restore env var", "key", key, "error", err)
		}
	}

	// Unset keys that didn't exist before
	for _, key := range keysToUnset {
		if err := os.Unsetenv(key); err != nil {
			slog.Warn("Failed to unset env var", "key", key, "error", err)
		}
	}

	slog.Debug("Restored environment variables",
		"restored_count", len(originalVars),
		"unset_count", len(keysToUnset))
}
