package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/logging"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
)

type Executor struct {
	serviceURL        string
	parallel          int
	testTimeout       time.Duration
	serviceCmd        *exec.Cmd
	server            *Server
	serviceLogFile    *os.File
	enableServiceLogs bool
	servicePort       int
	resultsDir        string
	ResultsFile       string // Will be set by the run command if --save-results is true
	onTestCompleted   func(TestResult, Test)
	suiteSpans        []*core.Span
	cancelTests       context.CancelFunc
}

func NewExecutor() *Executor {
	return &Executor{
		serviceURL:  "http://localhost:3000",
		parallel:    5,
		testTimeout: 30 * time.Second,
	}
}

func (e *Executor) SetResultsOutput(dir string) {
	e.resultsDir = dir

	timestamp := time.Now().Format("20060102-150405")
	e.ResultsFile = filepath.Join(dir, fmt.Sprintf("results-%s.json", timestamp))
}

func (e *Executor) RunTests(tests []Test) ([]TestResult, error) {
	return e.runTestsWithResilience(tests)
}

// runTestsWithResilience executes tests in batches with crash detection and recovery
func (e *Executor) runTestsWithResilience(tests []Test) ([]TestResult, error) {
	if len(tests) == 0 {
		return []TestResult{}, nil
	}

	batchSize := e.parallel
	allResults := make([]TestResult, 0, len(tests))

	for i := 0; i < len(tests); i += batchSize {
		end := i + batchSize
		if end > len(tests) {
			end = len(tests)
		}
		batch := tests[i:end]

		slog.Debug("Processing batch", "start", i, "end", end, "size", len(batch))

		// Try batch concurrently first
		results, serverCrashed := e.RunBatchWithCrashDetection(batch, batchSize)

		if !serverCrashed {
			// No crash detected, add results and continue
			slog.Debug("Batch completed successfully, no crash detected", "batch_size", len(batch))
			allResults = append(allResults, results...)
			continue
		}

		// Server crashed during batch - restart and retry sequentially
		logging.LogToService(fmt.Sprintf("⚠️  Server crashed during batch execution. Restarting and retrying %d tests sequentially...", len(batch)))

		if err := e.RestartServerWithRetry(0); err != nil {
			// Can't restart - mark all remaining tests as failed
			logging.LogToService(fmt.Sprintf("❌ Failed to restart server: %v", err))
			logging.LogToService("Marking all remaining tests as failed")

			for j := i; j < len(tests); j++ {
				allResults = append(allResults, TestResult{
					TestID: tests[j].TraceID,
					Passed: false,
					Error:  fmt.Sprintf("Server repeatedly crashed, cannot continue: %v", err),
				})
			}
			return allResults, fmt.Errorf("server repeatedly crashed and failed to restart: %w", err)
		}

		// Re-run batch sequentially to identify problematic tests
		sequentialResults := e.RunBatchSequentialWithCrashHandling(batch)
		allResults = append(allResults, sequentialResults...)
	}

	return allResults, nil
}

// RunTestsConcurrently executes tests in parallel with the specified concurrency limit
// This is now used internally by the resilience logic
func (e *Executor) RunTestsConcurrently(tests []Test, maxConcurrency int) ([]TestResult, error) {
	if len(tests) == 0 {
		return []TestResult{}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Store cancel function so signal handler can call it
	e.cancelTests = cancel

	testChan := make(chan Test, len(tests))
	resultChan := make(chan TestResult, len(tests))

	for workerID := range maxConcurrency {
		go func(workerID int) {
			for test := range testChan {
				slog.Debug("Worker starting test", "workerID", workerID, "testID", test.TraceID)
				select {
				case <-ctx.Done():
					// Context cancelled - mark as cancelled, not deviation
					resultChan <- TestResult{
						TestID:    test.TraceID,
						Passed:    false,
						Cancelled: true,
						Error:     "Test execution interrupted",
					}
					return
				default:
					result, err := e.RunSingleTest(test)
					if err != nil {
						result = TestResult{
							TestID: test.TraceID,
							Passed: false,
							Error:  err.Error(),
						}
						slog.Debug("Worker test failed", "workerID", workerID, "testID", test.TraceID, "error", err)
					} else {
						slog.Debug("Worker test completed", "workerID", workerID, "testID", test.TraceID, "passed", result.Passed)
					}
					resultChan <- result
				}
			}
		}(workerID)
	}

	for _, test := range tests {
		testChan <- test
	}
	close(testChan)

	results := make([]TestResult, 0, len(tests))
	for i := 0; i < len(tests); i++ {
		select {
		case result := <-resultChan:
			results = append(results, result)
		case <-ctx.Done():
			// Interrupted - mark remaining tests as cancelled
			for j := i; j < len(tests); j++ {
				results = append(results, TestResult{
					TestID:    tests[j].TraceID,
					Passed:    false,
					Cancelled: true,
					Error:     "Test execution interrupted",
				})
			}
			return results, nil
		}
	}

	slog.Debug("Completed concurrent test execution",
		"totalTests", len(tests),
		"maxConcurrency", maxConcurrency,
		"passed", countPassedTests(results),
		"failed", len(results)-countPassedTests(results))

	return results, nil
}

// RunBatchWithCrashDetection runs a batch of tests and detects if the server crashed
func (e *Executor) RunBatchWithCrashDetection(batch []Test, concurrency int) ([]TestResult, bool) {
	results, err := e.RunTestsConcurrently(batch, concurrency)

	// Check if context was cancelled (e.g., Ctrl+C)
	if err != nil {
		return results, false
	}

	// Detect if server crashed based on error patterns in results
	serverCrashed := e.DetectServerCrashFromResults(results)

	if serverCrashed {
		slog.Debug("Server crash detected from results", "batch_size", len(batch))
	}

	return results, serverCrashed
}

// RunBatchSequentialWithCrashHandling runs a batch of tests sequentially, restarting after each crash
func (e *Executor) RunBatchSequentialWithCrashHandling(batch []Test) []TestResult {
	results := make([]TestResult, 0, len(batch))
	consecutiveRestartAttempt := 0

	for idx, test := range batch {
		slog.Debug("Running test sequentially", "index", idx+1, "total", len(batch), "testID", test.TraceID)
		logging.LogToService(fmt.Sprintf("Running test %d/%d sequentially: %s", idx+1, len(batch), test.TraceID))

		result, err := e.RunSingleTest(test)

		// Check if this test crashed the server
		if err != nil && !e.CheckServerHealth() {
			slog.Warn("Test crashed the server", "testID", test.TraceID, "error", err)
			logging.LogToService(fmt.Sprintf("⚠️  Test %s crashed the server", test.TraceID))

			result.CrashedServer = true
			result.RetriedAfterCrash = true

			// Try to restart for next test
			if idx < len(batch)-1 { // Only restart if there are more tests
				logging.LogToService("Restarting server for next test...")
				if restartErr := e.RestartServerWithRetry(consecutiveRestartAttempt); restartErr != nil {
					consecutiveRestartAttempt++
					// If multiple tests in a row crash the server, we need to mark the remaining tests as failed
					if consecutiveRestartAttempt >= MaxServerRestartAttempts {
						// Mark remaining tests as failed
						logging.LogToService(fmt.Sprintf("❌ Exceeded maximum restart attempts. Marking remaining %d tests as failed.", len(batch)-idx-1))
						results = append(results, result)
						for j := idx + 1; j < len(batch); j++ {
							results = append(results, TestResult{
								TestID: batch[j].TraceID,
								Passed: false,
								Error:  "Server repeatedly crashed, cannot continue testing",
							})
						}
						break
					}
				} else {
					consecutiveRestartAttempt = 0 // Reset on successful restart
				}
			}
		} else {
			// Test succeeded or failed normally (server still running)
			consecutiveRestartAttempt = 0 // Reset counter on successful test
			if err == nil {
				result.RetriedAfterCrash = true // Mark as retried since it was in crashed batch
			}
		}

		results = append(results, result)
	}

	return results
}

// GetConcurrency returns the current concurrency setting
func (e *Executor) GetConcurrency() int {
	return e.parallel
}

// GetServer returns the server instance
func (e *Executor) GetServer() *Server {
	return e.server
}

// WaitForSpanData blocks briefly until inbound or match events are recorded for a test
func (e *Executor) WaitForSpanData(traceID string, timeout time.Duration) {
	if e.server != nil {
		e.server.WaitForSpanData(traceID, timeout)
	}
}

// SetConcurrency sets the maximum number of concurrent tests
func (e *Executor) SetConcurrency(concurrency int) {
	if concurrency > 0 {
		e.parallel = concurrency
	}
}

func (e *Executor) SetTestTimeout(timeout time.Duration) {
	if timeout > 0 {
		e.testTimeout = timeout
	}
}

func (e *Executor) SetOnTestCompleted(callback func(TestResult, Test)) {
	e.onTestCompleted = callback
}

func (e *Executor) SetSuiteSpans(spans []*core.Span) {
	e.suiteSpans = spans
	if e.server != nil && len(spans) > 0 {
		e.server.SetSuiteSpans(spans)
	}
}

func (e *Executor) CancelTests() {
	if e.cancelTests != nil {
		e.cancelTests()
	}
}

func (e *Executor) IsServiceLogsEnabled() bool {
	return e.enableServiceLogs
}

// CheckServerHealth performs a quick health check to see if the service is responsive
func (e *Executor) CheckServerHealth() bool {
	client := &http.Client{Timeout: 2 * time.Second}

	// Try a simple HEAD request to the service URL
	req, err := http.NewRequest("HEAD", e.serviceURL, nil)
	if err != nil {
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		slog.Debug("Health check failed", "error", err)
		return false
	}
	defer resp.Body.Close()

	// Any response (even error status codes) means the server is alive
	return true
}

// DetectServerCrashFromResults analyzes test results to determine if the server crashed
func (e *Executor) DetectServerCrashFromResults(results []TestResult) bool {
	// Look for patterns that indicate server crash:
	// - Connection errors (refused, reset, etc.)
	// - EOF errors (server closed connection)
	// - Network errors
	connectionErrors := 0
	errorSamples := []string{}

	for _, result := range results {
		if result.Error != "" {
			errLower := strings.ToLower(result.Error)
			// Check for various crash indicators
			if strings.Contains(errLower, "connection refused") ||
				strings.Contains(errLower, ": eof") || // Match ": EOF" pattern
				strings.Contains(errLower, "connection reset") ||
				strings.Contains(errLower, "broken pipe") ||
				strings.Contains(errLower, "no such host") ||
				strings.Contains(errLower, "connection closed") {
				connectionErrors++
				if len(errorSamples) < 3 {
					errorSamples = append(errorSamples, result.Error)
				}
			}
		}
	}

	slog.Debug("Checking batch for crashes", "total_results", len(results), "connection_errors", connectionErrors)

	// If we have connection errors, verify server is actually down
	if connectionErrors > 0 {
		slog.Debug("Detected connection errors in batch", "count", connectionErrors, "samples", errorSamples)
		serverHealthy := e.CheckServerHealth()
		slog.Debug("Server health check result", "healthy", serverHealthy)
		return !serverHealthy
	}

	return false
}

func countPassedTests(results []TestResult) int {
	count := 0
	for _, result := range results {
		if result.Passed {
			count++
		}
	}
	return count
}

// GetStartupFailureHelpMessage returns a user-friendly help message when the service fails to start.
// It explains how to use --enable-service-logs and shows where logs will be saved.
func (e *Executor) GetStartupFailureHelpMessage() string {
	var msg strings.Builder

	msg.WriteString("\n")
	msg.WriteString("💡 Tip: Use --enable-service-logs to see detailed service logs and diagnose startup issues.\n")
	msg.WriteString("   Service logs show the stdout/stderr output from your service, which can help identify why the service failed to start.\n")

	// Always show where logs would be saved
	logsDir := utils.GetLogsDir()
	if e.enableServiceLogs && e.serviceLogFile != nil {
		msg.WriteString(fmt.Sprintf("📄 Service logs are available at: %s\n", e.serviceLogFile.Name()))
	} else {
		msg.WriteString(fmt.Sprintf("📁 Service logs will be saved to: %s/\n", logsDir))
	}

	return msg.String()
}

// RunSingleTest replays a single trace on the service under test.
func (e *Executor) RunSingleTest(test Test) (TestResult, error) {
	// Load all spans for this trace into the server for sophisticated matching
	if e.server != nil {
		if len(test.Spans) > 0 {
			e.server.LoadSpansForTrace(test.TraceID, test.Spans)
		} else {
			spans, err := e.LoadSpansForTrace(test.TraceID, test.FileName)
			if err != nil {
				slog.Warn("Failed to load spans for trace", "traceID", test.TraceID, "error", err)
			} else {
				e.server.LoadSpansForTrace(test.TraceID, spans)
			}
		}

		// Help attribute outbound events to this test when SDK omits testId
		e.server.SetCurrentTestID(test.TraceID)
		defer e.server.SetCurrentTestID("")
		defer e.server.CleanupTraceSpans(test.TraceID)
	}

	var reqBody io.Reader
	if test.Request.Body != nil {
		// Extract body schema from input schema
		var bodySchema *core.JsonSchema
		if len(test.Spans) > 0 {
			// Root/server span has the request data
			for _, span := range test.Spans {
				if span.IsRootSpan && span.InputSchema != nil && span.InputSchema.Properties != nil {
					bodySchema = span.InputSchema.Properties["body"]
					break
				}
			}
		}

		// Decode body using schema (returns both bytes and parsed value)
		decodedBytes, _, err := DecodeValueBySchema(test.Request.Body, bodySchema)
		if err != nil {
			return TestResult{}, fmt.Errorf("failed to decode request body: %w", err)
		}

		reqBody = bytes.NewReader(decodedBytes)
	}

	urlStr := test.Request.Path
	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		urlStr = e.serviceURL + urlStr
	}
	req, err := http.NewRequest(test.Request.Method, urlStr, reqBody)
	if err != nil {
		return TestResult{}, err
	}

	req.Header.Set("x-td-trace-id", test.TraceID)

	// Only set fetch header if there are actually env vars to fetch
	if _, hasEnvVars := test.Metadata["ENV_VARS"]; hasEnvVars {
		req.Header.Set("x-td-fetch-env-vars", "true")
	}

	if test.Request.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	for k, v := range test.Request.Headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: e.testTimeout}

	startTime := time.Now()
	resp, err := client.Do(req)
	duration := int(time.Since(startTime).Milliseconds())

	if err != nil {
		result := TestResult{
			TestID:   test.TraceID,
			Passed:   false,
			Error:    err.Error(),
			Duration: duration,
		}
		if e.onTestCompleted != nil {
			e.onTestCompleted(result, test)
		}
		return result, err
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("Failed to close response body", "error", err)
		}
	}()

	result, _ := e.compareAndGenerateResult(test, resp, duration)
	if e.onTestCompleted != nil {
		r := result
		t := test
		e.onTestCompleted(r, t)
	}

	return result, nil
}

func OutputSingleResult(result TestResult, test Test, format string, quiet bool, verbose bool) {
	switch format {
	case "json":
		outputSingleJSON(result)
	default:
		outputSingleText(result, test, quiet, verbose)
	}
}

func outputSingleJSON(result TestResult) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(result)
}

func outputSingleText(result TestResult, test Test, quiet bool, verbose bool) {
	if result.Cancelled {
		return
	}

	green := ""
	orange := ""
	yellow := ""
	red := ""
	reset := ""

	if utils.IsTerminal() && os.Getenv("NO_COLOR") == "" {
		green = "\033[32m"
		orange = "\033[38;5;208m"
		yellow = "\033[33m"
		red = "\033[31m"
		reset = "\033[0m"
	}

	// Handle server crash scenario
	if result.CrashedServer {
		fmt.Printf("%s❌ SERVER CRASHED - %s (%dms)%s", red, result.TestID, result.Duration, reset)
		if result.RetriedAfterCrash {
			fmt.Printf(" %s[retried]%s\n", yellow, reset)
		} else {
			fmt.Println()
		}
		if result.Error != "" {
			fmt.Printf("  Error: %s\n", result.Error)
		}
		return
	}

	if result.Passed {
		if !quiet {
			suffix := ""
			if result.RetriedAfterCrash {
				suffix = fmt.Sprintf(" %s[retried after crash]%s", yellow, reset)
			}
			fmt.Printf("%s✓ NO DEVIATION - %s (%dms)%s%s\n", green, result.TestID, result.Duration, reset, suffix)
		}
	} else {
		suffix := ""
		if result.RetriedAfterCrash {
			suffix = fmt.Sprintf(" %s[retried after crash]%s", yellow, reset)
		}
		fmt.Printf("%s● DEVIATION - %s (%dms)%s%s\n", orange, result.TestID, result.Duration, reset, suffix)

		if verbose && !quiet && len(result.Deviations) > 0 {
			fmt.Printf("  Request: %s %s\n", test.Request.Method, test.Request.Path)
			if len(test.Request.Headers) > 0 {
				fmt.Printf("  Headers:\n")
				for key, value := range test.Request.Headers {
					fmt.Printf("    %s: %s\n", key, value)
				}
			}
			if test.Request.Body != nil {
				fmt.Printf("  Body: %v\n", test.Request.Body)
			}
			fmt.Println()

			for _, dev := range result.Deviations {
				fmt.Printf("  %sDeviation: %s%s\n", yellow, dev.Description, reset)
				fmt.Printf("    Expected: %v\n", dev.Expected)
				fmt.Printf("    Actual: %v\n", dev.Actual)
			}
		}

		if result.Error != "" {
			fmt.Printf("  Error: %s\n", result.Error)
		}
	}
}

func OutputResultsSummary(results []TestResult, format string, quiet bool) error {
	passed := 0
	failed := 0
	cancelled := 0
	crashed := 0

	for _, result := range results {
		switch {
		case result.Cancelled:
			cancelled++
		case result.CrashedServer:
			crashed++
		case result.Passed:
			passed++
		default:
			failed++
		}
	}

	if format == "json" {
		if crashed > 0 {
			fmt.Fprintf(os.Stderr, "\nTests: %d total, %d passed, %d failed, %d crashed server\n",
				len(results), passed, failed, crashed)
		} else {
			fmt.Fprintf(os.Stderr, "\nTests: %d total, %d passed, %d failed\n",
				len(results), passed, failed)
		}

		if failed > 0 || crashed > 0 {
			return fmt.Errorf("%d tests with deviations, %d crashed server", failed, crashed)
		}
		return nil
	}

	green := ""
	orange := ""
	red := ""
	reset := ""
	gray := ""

	if utils.IsTerminal() && os.Getenv("NO_COLOR") == "" {
		green = "\033[32m"
		orange = "\033[38;5;208m"
		red = "\033[31m"
		gray = "\033[90m"
		reset = "\033[0m"
	}

	// Build summary string based on what we have
	summaryParts := []string{
		fmt.Sprintf("%d total", len(results)),
		fmt.Sprintf("%s%d passed%s", green, passed, reset),
	}

	if failed > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("%s%d deviations%s", orange, failed, reset))
	}

	if crashed > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("%s%d crashed server%s", red, crashed, reset))
	}

	if cancelled > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("%s%d cancelled%s", gray, cancelled, reset))
	}

	fmt.Printf("\nTests: %s\n\n", strings.Join(summaryParts, ", "))

	if failed > 0 || crashed > 0 {
		if crashed > 0 && failed > 0 {
			return fmt.Errorf("%d tests with deviations, %d crashed server", failed, crashed)
		} else if crashed > 0 {
			return fmt.Errorf("%d tests crashed server", crashed)
		} else {
			return fmt.Errorf("%d tests with deviations", failed)
		}
	}

	return nil
}
