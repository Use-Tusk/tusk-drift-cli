package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
	"github.com/Use-Tusk/tusk-drift-cli/internal/runner"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
)

// Note: We currently use the internal `runner` package instead of direct Tusk CLI commands `tusk list` and `tusk run`.
// Short term benefits:
// - no dependency on finding the tusk binary path (for dev testing)
// - faster execution (no process spawn)
// - better error handling with native Go errors
// But this also causes tighter coupling to internal APIs, more complex code (e.g., loading of tests, fetching pre-app start spans, etc).
// We may want to switch if this gets more complex in the future.

// TuskTools provides Tusk CLI operations using internal runner package
type TuskTools struct {
	workDir string
}

// NewTuskTools creates a new TuskTools instance
func NewTuskTools(workDir string) *TuskTools {
	return &TuskTools{workDir: workDir}
}

// ValidateConfig validates the .tusk/config.yaml file and returns detailed results.
// This should be called after creating or modifying the config to catch errors early.
func (tt *TuskTools) ValidateConfig(input json.RawMessage) (string, error) {
	configPath := filepath.Join(tt.workDir, ".tusk", "config.yaml")

	result := config.ValidateConfigFile(configPath)

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal validation result: %w", err)
	}

	return string(jsonBytes), nil
}

// List loads and lists traces from the .tusk/traces directory
func (tt *TuskTools) List(input json.RawMessage) (string, error) {
	_ = config.Load(filepath.Join(tt.workDir, ".tusk", "config.yaml"))

	tracesDir := filepath.Join(tt.workDir, ".tusk", "traces")
	if cfg, err := config.Get(); err == nil && cfg.Traces.Dir != "" {
		if filepath.IsAbs(cfg.Traces.Dir) {
			tracesDir = cfg.Traces.Dir
		} else {
			tracesDir = filepath.Join(tt.workDir, cfg.Traces.Dir)
		}
	}

	utils.SetTracesDirOverride(tracesDir)

	executor := runner.NewExecutor()
	tests, err := executor.LoadTestsFromFolder(tracesDir)
	if err != nil {
		if strings.Contains(err.Error(), "traces folder not found") {
			return "No traces found. The .tusk/traces directory doesn't exist or is empty.", nil
		}
		return "", fmt.Errorf("failed to load traces: %w", err)
	}

	if len(tests) == 0 {
		return "No traces found in " + tracesDir, nil
	}

	type testOutput struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Method   string `json:"method,omitempty"`
		Path     string `json:"path,omitempty"`
		FileName string `json:"file_name,omitempty"`
	}

	output := struct {
		Count int          `json:"count"`
		Tests []testOutput `json:"tests"`
	}{
		Count: len(tests),
		Tests: make([]testOutput, 0, len(tests)),
	}

	for _, t := range tests {
		output.Tests = append(output.Tests, testOutput{
			ID:       t.TraceID,
			Name:     t.DisplayName,
			Method:   t.Method,
			Path:     t.Path,
			FileName: t.FileName,
		})
	}

	jsonBytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal output: %w", err)
	}

	return string(jsonBytes), nil
}

// Run runs tests using the internal runner
func (tt *TuskTools) Run(input json.RawMessage) (string, error) {
	var params struct {
		Filter string `json:"filter"`
		Debug  bool   `json:"debug"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	_ = config.Load(filepath.Join(tt.workDir, ".tusk", "config.yaml"))

	tracesDir := filepath.Join(tt.workDir, ".tusk", "traces")
	if cfg, err := config.Get(); err == nil && cfg.Traces.Dir != "" {
		if filepath.IsAbs(cfg.Traces.Dir) {
			tracesDir = cfg.Traces.Dir
		} else {
			tracesDir = filepath.Join(tt.workDir, cfg.Traces.Dir)
		}
	}

	utils.SetTracesDirOverride(tracesDir)

	executor := runner.NewExecutor()
	executor.SetEnableServiceLogs(params.Debug)

	if cfg, err := config.Get(); err == nil && cfg.TestExecution.Concurrency > 0 {
		executor.SetConcurrency(cfg.TestExecution.Concurrency)
	}
	if cfg, err := config.Get(); err == nil && cfg.TestExecution.Timeout != "" {
		if d, err := time.ParseDuration(cfg.TestExecution.Timeout); err == nil {
			executor.SetTestTimeout(d)
		}
	}

	// Load tests
	tests, err := executor.LoadTestsFromFolder(tracesDir)
	if err != nil {
		if strings.Contains(err.Error(), "traces folder not found") {
			return "No tests found. The .tusk/traces directory doesn't exist.", nil
		}
		return "", fmt.Errorf("failed to load tests: %w", err)
	}

	// Apply filter if specified
	if params.Filter != "" {
		tests, err = runner.FilterTests(tests, params.Filter)
		if err != nil {
			return "", fmt.Errorf("invalid filter: %w", err)
		}
	}

	if len(tests) == 0 {
		return "No tests found", nil
	}

	var results []string
	executor.SetOnTestCompleted(func(res runner.TestResult, test runner.Test) {
		status := "✓ PASS"
		if res.Error != "" {
			status = "⚠ ERROR"
		} else if !res.Passed {
			status = "✗ FAIL"
		}
		results = append(results, fmt.Sprintf("%s %s %s", status, test.Method, test.Path))
	})

	preAppStartSpans, _ := runner.FetchLocalPreAppStartSpans(true)
	groupResult, err := runner.GroupTestsByEnvironment(tests, preAppStartSpans)
	if err != nil {
		return "", fmt.Errorf("failed to group tests: %w", err)
	}

	// Non-fatal error, continue even if this fails
	_ = runner.PrepareAndSetSuiteSpans(
		context.Background(),
		executor,
		runner.SuiteSpanOptions{
			Interactive: false,
			Quiet:       true,
		},
		tests,
	)

	var testResults []runner.TestResult
	if len(groupResult.Groups) > 0 {
		testResults, err = runner.ReplayTestsByEnvironment(context.Background(), executor, groupResult.Groups)
	} else {
		// Start environment and run tests
		if err := executor.StartEnvironment(); err != nil {
			return "", fmt.Errorf("failed to start environment: %w\n%s", err, executor.GetStartupFailureHelpMessage())
		}
		defer func() { _ = executor.StopEnvironment() }()
		testResults, err = executor.RunTests(tests)
	}

	if err != nil {
		return fmt.Sprintf("Test execution failed: %v\n\nResults so far:\n%s", err, strings.Join(results, "\n")), nil
	}

	passed := 0
	failed := 0
	errors := 0
	for _, r := range testResults {
		switch {
		case r.Error != "":
			errors++
		case r.Passed:
			passed++
		default:
			failed++
		}
	}

	summary := fmt.Sprintf("\n\nSummary: %d passed, %d failed, %d errors out of %d tests", passed, failed, errors, len(testResults))
	return strings.Join(results, "\n") + summary, nil
}

// RunValidation runs 'tusk run --cloud --validate-suite --print' and returns the results
func (tt *TuskTools) RunValidation(input json.RawMessage) (string, error) {
	// Use a timeout to prevent hanging indefinitely
	timeout := 120 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Execute tusk run --cloud --validate-suite --print
	cmd := exec.CommandContext(ctx, "tusk", "run", "--cloud", "--validate-suite", "--print")
	cmd.Dir = tt.workDir

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	// Check for timeout
	if ctx.Err() == context.DeadlineExceeded {
		return tt.parseValidationOutput(outputStr, fmt.Errorf("validation timed out after %v", timeout))
	}

	// Parse output even on error - validation may have failed but we want the results
	return tt.parseValidationOutput(outputStr, err)
}

// parseValidationOutput parses the output of validation run
func (tt *TuskTools) parseValidationOutput(output string, runErr error) (string, error) {
	// Count passed/failed tests
	lines := strings.Split(output, "\n")
	passed := 0
	failed := 0

	for _, line := range lines {
		// tusk run --print outputs "NO DEVIATION" for passed tests and "DEVIATION" for failed
		// Must check "NO DEVIATION" first since "DEVIATION" is a substring of it
		if strings.Contains(line, "NO DEVIATION") {
			passed++
		} else if strings.Contains(line, "DEVIATION") {
			failed++
		}
	}

	// Passed tests become part of suite
	testsInSuite := passed

	success := passed > 0
	errorMsg := ""
	if runErr != nil {
		errorMsg = runErr.Error()
		// If there was an error and no tests passed, it's not successful
		if passed == 0 {
			success = false
		}
	}

	result := map[string]interface{}{
		"success":        success,
		"tests_passed":   passed,
		"tests_failed":   failed,
		"tests_in_suite": testsInSuite,
		"output":         output,
	}

	if errorMsg != "" {
		result["error"] = errorMsg
	}

	data, _ := json.Marshal(result)
	return string(data), nil
}
