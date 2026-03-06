package runner

import (
	"context"
	"fmt"
	"os"

	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
	"github.com/Use-Tusk/tusk-drift-cli/internal/log"
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
		log.Debug("Starting replay for environment group",
			"environment", group.Name,
			"test_count", len(group.Tests),
			"env_var_count", len(group.EnvVars),
			"group_index", i+1,
			"total_groups", len(groups))

		log.ServiceLog(fmt.Sprintf("Running %d tests for environment: %s", len(group.Tests), group.Name))

		// 1. Set environment variables and prepare compose replay override (if needed)
		cleanup, err := PrepareReplayEnvironmentGroup(executor, group)
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
			log.Warn("Failed to stop environment cleanly",
				"environment", group.Name,
				"error", err)
			log.ServiceLog(fmt.Sprintf("⚠️  Warning: failed to stop environment for %s: %v", group.Name, err))
		}

		// 6. Restore environment variables
		cleanup()

		log.Debug("Completed replay for environment group",
			"environment", group.Name,
			"results_count", len(results))
	}

	log.Debug("Completed all environment group replays",
		"total_groups", len(groups),
		"total_results", len(allResults))

	return allResults, nil
}

// PrepareReplayEnvironmentGroup sets recorded env vars on the process and, if applicable,
// prepares a Docker Compose override file so env vars are injected into containers.
// The returned cleanup always restores process env vars, clears executor override state,
// and removes the temporary override file when present.
func PrepareReplayEnvironmentGroup(executor *Executor, group *EnvironmentGroup) (cleanup func(), err error) {
	cfg, err := config.Get()
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}

	cleanup, err = SetEnvironmentVariables(group.EnvVars)
	if err != nil {
		return nil, err
	}

	cleanup = chainCleanup(cleanup, func() {
		executor.setReplayComposeOverride("")
	})

	isComposeStart := isComposeBasedStartCommand(cfg.Service.Start.Command)
	log.Debug("Replay env diagnostics",
		"environment", group.Name,
		"recorded_env_vars", len(group.EnvVars),
		"compose_start_command", isComposeStart)

	if len(group.EnvVars) == 0 {
		log.Debug("No recorded env vars found; skipping replay env override",
			"environment", group.Name)
		return cleanup, nil
	}

	if !isComposeStart {
		log.Debug("Replay env vars applied to process only (start command is not Docker Compose)",
			"environment", group.Name)
		return cleanup, nil
	}

	filteredEnvVars, skippedEnvVars := filterReplayEnvVarsForCompose(group.EnvVars)
	if len(skippedEnvVars) > 0 {
		log.Debug("Excluding internal TUSK_* env vars from Docker Compose override",
			"environment", group.Name,
			"skipped_env_vars", skippedEnvVars)
	}
	if len(filteredEnvVars) == 0 {
		log.Debug("All recorded env vars are internal-only; skipping Docker Compose override",
			"environment", group.Name)
		return cleanup, nil
	}

	overridePath, createErr := createReplayComposeOverrideFile(cfg.Service.Start.Command, filteredEnvVars, group.Name)
	if createErr != nil {
		cleanup()
		return nil, fmt.Errorf("failed to prepare replay compose override for %s: %w", group.Name, createErr)
	}
	if overridePath == "" {
		log.Debug("No replay compose override file created; skipping injection",
			"environment", group.Name,
			"source_file", replayComposeServiceSourceFile)
		return cleanup, nil
	}

	executor.setReplayComposeOverride(overridePath)
	log.ServiceLog(fmt.Sprintf("✅ Replay env override prepared for %s (%d vars after filtering): %s", group.Name, len(filteredEnvVars), overridePath))
	log.Debug("Prepared replay compose env override file",
		"environment", group.Name,
		"path", overridePath,
		"env_var_count", len(filteredEnvVars))

	cleanup = chainCleanup(cleanup, func() {
		if removeErr := os.Remove(overridePath); removeErr != nil && !os.IsNotExist(removeErr) {
			log.Warn("Failed to remove replay compose override file",
				"path", overridePath,
				"error", removeErr)
		}
	})

	return cleanup, nil
}

func chainCleanup(cleanups ...func()) func() {
	return func() {
		for _, cleanup := range cleanups {
			if cleanup != nil {
				cleanup()
			}
		}
	}
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

	log.Debug("Set environment variables", "count", len(envVars))

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
			log.Warn("Failed to restore env var", "key", key, "error", err)
		}
	}

	// Unset keys that didn't exist before
	for _, key := range keysToUnset {
		if err := os.Unsetenv(key); err != nil {
			log.Warn("Failed to unset env var", "key", key, "error", err)
		}
	}

	log.Debug("Restored environment variables",
		"restored_count", len(originalVars),
		"unset_count", len(keysToUnset))
}
