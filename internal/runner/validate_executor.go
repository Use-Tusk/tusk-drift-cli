package runner

import (
	"context"
	"fmt"
	"time"
)

// ValidationResult represents the result of validating a trace on main
// Note: GlobalSpanIDs is no longer tracked here - backend handles global span detection
// via TraceTestSpanResult.matchedDbSpanRecordingId
type ValidationResult struct {
	TraceID       string
	TraceTestID   string
	Passed        bool
	FailureReason string
	Duration      time.Duration
}

// ValidateExecutor wraps Executor with validation-specific behavior
// Note: With the new validation flow, most logic is handled by the regular Executor
// and the backend processes results to curate the test suite
type ValidateExecutor struct {
	*Executor
}

// NewValidateExecutor creates a new ValidateExecutor wrapping the given Executor
func NewValidateExecutor(base *Executor) *ValidateExecutor {
	return &ValidateExecutor{Executor: base}
}

// ValidateDraftTraces runs validation for all draft traces
// Returns partial results if context is cancelled (workflow timeout)
// Note: This is retained for backwards compatibility but the new validation flow
// uses executor.RunTests directly with result uploads
func (ve *ValidateExecutor) ValidateDraftTraces(ctx context.Context, tests []Test) ([]ValidationResult, error) {
	var results []ValidationResult

	for i, test := range tests {
		select {
		case <-ctx.Done():
			// Workflow timeout - return partial results
			fmt.Printf("Context cancelled after %d/%d traces, saving progress\n", i, len(tests))
			return results, nil
		default:
		}

		result := ve.validateSingleTrace(ctx, &test)
		results = append(results, result)

		status := "PASSED"
		if !result.Passed {
			status = "FAILED"
		}
		fmt.Printf("[%d/%d] %s: %s (%s)\n", i+1, len(tests), test.TraceID, status, result.Duration.Truncate(time.Millisecond))
	}

	return results, nil
}

func (ve *ValidateExecutor) validateSingleTrace(ctx context.Context, test *Test) ValidationResult {
	start := time.Now()

	// Run the test using existing executor logic
	testResult, runErr := ve.Executor.RunSingleTest(*test)

	result := ValidationResult{
		TraceID:     test.TraceID,
		TraceTestID: test.TraceTestID,
		Passed:      testResult.Passed && !testResult.CrashedServer && runErr == nil,
		Duration:    time.Since(start),
	}

	if !result.Passed {
		switch {
		case runErr != nil:
			result.FailureReason = fmt.Sprintf("run_error: %v", runErr)
		case testResult.CrashedServer:
			result.FailureReason = "server_crashed"
		case len(testResult.Deviations) > 0:
			result.FailureReason = fmt.Sprintf("deviations: %d", len(testResult.Deviations))
		case testResult.Error != "":
			result.FailureReason = testResult.Error
		}
	}

	return result
}
