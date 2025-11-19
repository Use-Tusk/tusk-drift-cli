# Design Doc: Environment-Based Test Grouping and Replay

## Overview

Currently, Tusk Drift fetches all pre-app-start tests and replays them in a single server session. This design proposes grouping tests by environment and running each environment group in isolated server sessions with environment-specific configuration.

## Problem Statement

Tests may have been recorded in different environments (e.g., staging, production, feature branches) with different environment variable configurations. Replaying all tests in a single environment can cause:

- False positives/negatives due to environment mismatch
- Inability to reproduce environment-specific behavior
- Confusion about which environment configuration to use

## Goals

1. Group pre-app-start tests by environment
2. Replay each environment group with its specific environment variables
3. Restart server between environment groups with fresh configuration
4. Maintain existing TUI functionality with minimal changes
5. Keep code modular by reusing existing server start/replay/stop logic

## Non-Goals

- Changing the core test replay or comparison logic
- Major TUI redesign (simple sorting/grouping is acceptable)
- Supporting multiple environments simultaneously

## Proposed Solution

### High-Level Flow

```
┌─────────────────────────────────────┐
│ 1. Fetch all pre-app-start tests    │
└──────────────┬──────────────────────┘
               │
               ▼
┌─────────────────────────────────────┐
│ 2. Group tests by environment       │
│    - Extract from span metadata     │
│    - Create environment groups      │
└──────────────┬──────────────────────┘
               │
               ▼
┌─────────────────────────────────────┐
│ 3. For each environment group:      │
│    ┌──────────────────────────────┐ │
│    │ a. Extract env vars          │ │
│    │ b. Set process env vars      │ │
│    │ c. Start server              │ │
│    │ d. Replay tests              │ │
│    │ e. Collect results           │ │
│    │ f. Stop server               │ │
│    └──────────────────────────────┘ │
└──────────────┬──────────────────────┘
               │
               ▼
┌─────────────────────────────────────┐
│ 4. Upload/return all results        │
└─────────────────────────────────────┘
```

### Data Structures

#### Environment Group
```go
type EnvironmentGroup struct {
    Name         string              // Environment name (e.g., "production", "staging")
    Tests        []*runner.Test      // Tests for this environment
    EnvVars      map[string]string   // Environment variables for this group
    EnvVarsSpan  *core.Span          // The span containing ENV_VARS (for provenance)
}
```

#### Environment Extraction Result
```go
type EnvironmentExtractionResult struct {
    Groups           []*EnvironmentGroup
    UngroupedTests   []*runner.Test      // Tests without environment metadata
    Errors           []error             // Non-fatal extraction errors
}
```

### Component Design

#### 1. Test Grouping Module (`internal/runner/environment_grouping.go`)

**Purpose**: Extract environment information and group tests

```go
// GroupTestsByEnvironment analyzes pre-app-start tests and groups them by environment
func GroupTestsByEnvironment(tests []*runner.Test) (*EnvironmentExtractionResult, error)

// extractEnvironmentFromTest looks for metadata.environment field in test spans
func extractEnvironmentFromTest(test *runner.Test) string

// extractEnvVarsForEnvironment finds the ENV_VARS span for a given environment
// Filters for: SpanKind.Internal AND packageName="process.env" AND IsPreAppStart=true
// If multiple exist, selects the most recent one
func extractEnvVarsForEnvironment(tests []*runner.Test, environment string) (map[string]string, *core.Span, error)

// findMostRecentEnvVarsSpan selects the latest ENV_VARS span based on timestamp
func findMostRecentEnvVarsSpan(spans []*core.Span) *core.Span
```

**Implementation Details**:

- Iterate through all tests, extract `metadata.environment` from test spans
- For each unique environment value, collect all tests with that environment
- For each environment, find all `SpanKind.Internal` + `packageName="process.env"` spans
- Select most recent span (by timestamp) and extract `ENV_VARS` from metadata
- Return structured groups with tests, env vars, and source span

**Edge Cases**:

- Tests without environment metadata → grouped as "default" or ungrouped
- No ENV_VARS span found → use empty map (warn user)
- Multiple ENV_VARS spans → use timestamp to pick latest
- Conflicting ENV_VARS within same environment → error or warn

#### 2. Environment Orchestration Module (`internal/runner/environment_replay.go`)

**Purpose**: Coordinate server lifecycle across multiple environments

```go
// ReplayTestsByEnvironment orchestrates the full environment-based replay flow
func ReplayTestsByEnvironment(
    ctx context.Context,
    groups []*EnvironmentGroup,
    config *RunConfig,
    callbacks *runner.Callbacks,
) ([]*runner.TestResult, error)

// setEnvironmentVariables applies env vars to the current process
func setEnvironmentVariables(envVars map[string]string) (cleanup func(), err error)

// replayEnvironmentGroup runs a single environment group
func replayEnvironmentGroup(
    ctx context.Context,
    group *EnvironmentGroup,
    config *RunConfig,
    callbacks *runner.Callbacks,
) ([]*runner.TestResult, error)
```

**Implementation Flow**:
```go
func ReplayTestsByEnvironment(...) {
    allResults := []*runner.TestResult{}

    for _, group := range groups {
        // 1. Set environment variables
        cleanup, err := setEnvironmentVariables(group.EnvVars)
        if err != nil {
            return nil, fmt.Errorf("failed to set env vars for %s: %w", group.Name, err)
        }
        defer cleanup()

        // 2. Start environment (server + service)
        env, err := runner.StartEnvironment(ctx, config)
        if err != nil {
            return nil, fmt.Errorf("failed to start environment %s: %w", group.Name, err)
        }

        // 3. Replay tests for this environment
        results, err := runner.RunTests(ctx, env, group.Tests, config, callbacks)
        if err != nil {
            env.StopEnvironment()
            return nil, fmt.Errorf("failed to run tests for %s: %w", group.Name, err)
        }

        // 4. Collect results
        allResults = append(allResults, results...)

        // 5. Stop environment
        env.StopEnvironment()

        // 6. Restore environment variables
        cleanup()
    }

    return allResults, nil
}
```

**Key Design Decisions**:
- Reuse existing `StartEnvironment()`, `RunTests()`, `StopEnvironment()` functions
- Clean environment between groups (stop server fully, restore env vars)
- Sequential execution (no parallel environments)
- Fail fast on environment setup errors
- Accumulate results across all environments

#### 3. Integration Point (`cmd/run.go`)

**Current Flow** (simplified):
```go
func runTests(cmd *cobra.Command, args []string) {
    // Load tests
    tests, err := loadTestsFunc()

    // Prepare suite spans
    PrepareAndSetSuiteSpans(tests)

    // Start environment
    env, err := StartEnvironment(ctx, config)

    // Run tests
    results, err := RunTests(ctx, env, tests, config, callbacks)

    // Stop environment
    env.StopEnvironment()

    // Upload results
    uploadResults(results)
}
```

**Proposed Flow**:
```go
func runTests(cmd *cobra.Command, args []string) {
    // Load tests
    tests, err := loadTestsFunc()

    // NEW: Check if environment-based replay is enabled
    if shouldUseEnvironmentBasedReplay(tests) {
        // NEW: Group tests by environment
        groupResult, err := GroupTestsByEnvironment(tests)

        // NEW: Prepare suite spans per environment group
        for _, group := range groupResult.Groups {
            PrepareAndSetSuiteSpans(group.Tests)
        }

        // NEW: Replay by environment
        results, err := ReplayTestsByEnvironment(ctx, groupResult.Groups, config, callbacks)

        // Upload results
        uploadResults(results)
    } else {
        // EXISTING: Original flow (backward compatible)
        PrepareAndSetSuiteSpans(tests)
        env, err := StartEnvironment(ctx, config)
        results, err := RunTests(ctx, env, tests, config, callbacks)
        env.StopEnvironment()
        uploadResults(results)
    }
}

// shouldUseEnvironmentBasedReplay determines if environment grouping should be used
func shouldUseEnvironmentBasedReplay(tests []*runner.Test) bool {
    // Check if any test has environment metadata
    // Or check config flag
    return hasEnvironmentMetadata(tests) || config.EnableEnvironmentReplay
}
```

### TUI Changes

#### Minimal Changes Required

1. **Test Sorting in CLI Output** ([test_executor.go](internal/tui/test_executor.go))
   - Sort tests by environment before displaying

2. **No Structural Changes**
   - Keep existing test table component
   - Keep existing status/duration columns
   - Keep existing service logs panel

## Implementation Plan

### Phase 1: Core Grouping Logic

1. Create `environment_grouping.go` with test grouping functions
2. Implement environment extraction from span metadata
3. Implement ENV_VARS extraction from process.env spans
4. Add unit tests for grouping logic

### Phase 2: Orchestration Layer

1. Create `environment_replay.go` with replay orchestration
2. Implement environment variable setting/cleanup
3. Integrate with existing `StartEnvironment()` / `StopEnvironment()`
4. Add error handling and logging

### Phase 3: Integration

1. Update `cmd/run.go` to conditionally use environment-based replay
2. Add configuration flag for enabling/disabling feature
3. Update suite spans preparation to work per-environment
4. Add integration tests

### Phase 4: TUI Updates

1. Add environment sorting to test table
2. Add environment transition messages
3. Update test display names with environment labels
4. Test interactive mode with multiple environments

### Phase 5: Documentation & Rollout

1. Update user documentation
2. Add migration guide for users
3. Add logging for environment transitions
4. Gradual rollout with feature flag

## Testing Strategy

### Unit Tests

- `GroupTestsByEnvironment()` with various test configurations
- `extractEnvVarsForEnvironment()` with multiple/missing ENV_VARS spans
- Environment variable setting/cleanup
- Most recent span selection logic

### Integration Tests

- End-to-end replay with 2+ environment groups
- Server restart between environments
- ENV_VARS correctly applied per environment
- Results correctly accumulated

### Edge Case Tests

- Tests without environment metadata
- Missing ENV_VARS spans
- Conflicting environment variables
- Server crash during environment group replay

## Open Questions

1. **What if a test has environment metadata but no ENV_VARS span?**
   - Proposed: Run with empty env vars, log warning

2. **Should we support environment variable merging (base + override)?**
   - Proposed: Not in initial version, each environment is independent

3. **How to handle environment groups with only 1 test?**
   - Proposed: Still restart server, maintain consistency

4. **Should suite spans be environment-aware?**
   - Proposed: Yes, filter suite spans by environment to avoid cross-environment mocking conflicts

5. **What's the expected format of `metadata.environment` field?**
   - Proposed: Simple string (e.g., "production", "staging", "feature-xyz")

6. **Should we parallelize environment groups?**
   - Proposed: No, sequential execution is simpler and safer (one server at a time)

## Success Metrics

- [ ] Tests correctly grouped by environment
- [ ] Server restarts between environments
- [ ] ENV_VARS correctly applied per environment
- [ ] No regressions in existing test replay
- [ ] TUI remains responsive and clear
- [ ] Feature can be disabled via config flag
