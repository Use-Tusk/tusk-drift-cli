package runner

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
	"github.com/Use-Tusk/tusk-drift-cli/internal/log"
)

// ReplayTestsByEnvironment orchestrates environment-based test replay
// For each environment group:
//  1. Configure replay environment variables for the service subprocess
//  2. Start environment (server + service)
//  3. Run tests for that environment
//  4. Stop environment
//  5. Clear replay environment variable configuration
func ReplayTestsByEnvironment(
	ctx context.Context,
	executor *Executor,
	groups []*EnvironmentGroup,
) ([]TestResult, error) {
	allResults := make([]TestResult, 0)

	for i, group := range groups {
		envStart := time.Now()

		log.Debug("Starting replay for environment group",
			"environment", group.Name,
			"test_count", len(group.Tests),
			"env_var_count", len(group.EnvVars),
			"group_index", i+1,
			"total_groups", len(groups))

		log.ServiceLog(fmt.Sprintf("Running %d tests for environment: %s", len(group.Tests), group.Name))

		// 1. Configure replay env vars and prepare compose replay override (if needed)
		cleanup, err := PrepareReplayEnvironmentGroup(executor, group)
		if err != nil {
			return allResults, fmt.Errorf("failed to set env vars for %s: %w", group.Name, err)
		}

		// 2. Start environment (server + service)
		if err := executor.StartEnvironment(); err != nil {
			// Dump startup logs before returning so the caller's help message makes sense
			startupLogs := executor.GetStartupLogs()
			if startupLogs != "" {
				log.ServiceLog("📋 Service startup logs:")
				for _, line := range strings.Split(strings.TrimRight(startupLogs, "\n"), "\n") {
					log.ServiceLog(line)
				}
			}
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
			"results_count", len(results),
			"duration_seconds", time.Since(envStart).Seconds())
	}

	log.Debug("Completed all environment group replays",
		"total_groups", len(groups),
		"total_results", len(allResults))

	return allResults, nil
}

// PrepareReplayEnvironmentGroup sets recorded env vars on the process and, if applicable,
// prepares a Docker Compose override file so env vars are injected into containers.
// The returned cleanup always clears executor replay env vars, clears executor override
// state, and removes the temporary override file when present.
func PrepareReplayEnvironmentGroup(executor *Executor, group *EnvironmentGroup) (cleanup func(), err error) {
	cfg, err := config.Get()
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}

	filteredProcessEnvVars, skippedProcessEnvVars := filterReplayEnvVarsForProcess(group.EnvVars)
	executor.SetReplayEnvVars(filteredProcessEnvVars)
	cleanup = chainCleanup(func() {
		executor.SetReplayEnvVars(nil)
	}, func() {
		executor.setReplayComposeOverride("")
	})

	isComposeStart := isComposeBasedStartCommand(cfg.Service.Start.Command)
	log.Debug("Replay env diagnostics",
		"environment", group.Name,
		"recorded_env_vars", len(group.EnvVars),
		"process_env_vars", len(filteredProcessEnvVars),
		"skipped_process_env_vars", len(skippedProcessEnvVars),
		"compose_start_command", isComposeStart)
	if len(skippedProcessEnvVars) > 0 {
		log.Debug("Ignoring host-specific replay env vars for local process startup",
			"environment", group.Name,
			"skipped_env_vars", skippedProcessEnvVars)
	}

	if len(group.EnvVars) == 0 {
		log.Debug("No recorded env vars found; skipping replay env override",
			"environment", group.Name)
		return cleanup, nil
	}

	if !isComposeStart {
		log.Debug("Replay env vars applied to process only after filtering (start command is not Docker Compose)",
			"environment", group.Name)
		return cleanup, nil
	}

	filteredEnvVars, skippedEnvVars := filterReplayEnvVarsForCompose(group.EnvVars)
	if len(skippedEnvVars) > 0 {
		log.Debug("Excluding host-specific and internal runtime env vars from Docker Compose override",
			"environment", group.Name,
			"skipped_env_vars", skippedEnvVars)
	}
	if len(filteredEnvVars) == 0 {
		log.Debug("All recorded env vars are internal-only; skipping Docker Compose override",
			"environment", group.Name)
		return cleanup, nil
	}

	overridePath, createErr := createReplayComposeOverrideFile(filteredEnvVars, group.Name)
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
