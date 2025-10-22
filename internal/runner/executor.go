package runner

import (
	"bytes"
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
	return e.RunTestsConcurrently(tests, e.parallel)
}

// RunTestsConcurrently executes tests in parallel with the specified concurrency limit
func (e *Executor) RunTestsConcurrently(tests []Test, maxConcurrency int) ([]TestResult, error) {
	if len(tests) == 0 {
		return []TestResult{}, nil
	}

	testChan := make(chan Test, len(tests))
	resultChan := make(chan TestResult, len(tests))

	for workerID := range maxConcurrency {
		go func(workerID int) {
			for test := range testChan {
				slog.Debug("Worker starting test", "workerID", workerID, "testID", test.TraceID)

				result, err := e.RunSingleTest(test)
				if err != nil {
					result = TestResult{
						TestID: test.TraceID,
						Passed: false,
						Error:  err.Error(),
					}
					slog.Error("Worker test failed", "workerID", workerID, "testID", test.TraceID, "error", err)
				} else {
					slog.Debug("Worker test completed", "workerID", workerID, "testID", test.TraceID, "passed", result.Passed)
				}

				resultChan <- result
			}
		}(workerID)
	}

	for _, test := range tests {
		testChan <- test
	}
	close(testChan)

	results := make([]TestResult, 0, len(tests))
	for range len(tests) {
		result := <-resultChan
		results = append(results, result)
	}

	slog.Debug("Completed concurrent test execution",
		"totalTests", len(tests),
		"maxConcurrency", maxConcurrency,
		"passed", countPassedTests(results),
		"failed", len(results)-countPassedTests(results))

	return results, nil
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

func (e *Executor) IsServiceLogsEnabled() bool {
	return e.enableServiceLogs
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

	// Extract ENV_VARS from metadata, default to empty object if not present
	envVars := "{}"
	if envVarsValue, exists := test.Metadata["ENV_VARS"]; exists {
		if envVarsBytes, err := json.Marshal(envVarsValue); err == nil {
			envVars = string(envVarsBytes)
		}
	}

	slog.Debug("Setting env vars", "envVars", envVars)
	req.Header.Set("x-td-env-vars", envVars)

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
		return TestResult{}, err
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

func OutputResults(results []TestResult, tests []Test, format string, quiet bool) error {
	switch format {
	case "json":
		return outputJSON(results)
	default:
		return outputText(results, tests, quiet)
	}
}

func outputJSON(results []TestResult) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(results)
}

func outputText(results []TestResult, tests []Test, quiet bool) error {
	passed := 0
	failed := 0

	for _, result := range results {
		if result.Passed {
			passed++
		} else {
			failed++
		}
	}

	green := ""
	orange := ""
	yellow := ""
	reset := ""

	if utils.IsTerminal() && os.Getenv("NO_COLOR") == "" {
		green = "\033[32m"
		// This is a 256-color code for orange, previously we were using 16-color
		// Some really old terminals may not support 256-color codes, but since we offer NO_COLOR it's ok to use it
		orange = "\033[38;5;208m"
		yellow = "\033[33m"
		reset = "\033[0m"
	}

	fmt.Println()

	// For quick test lookup by TraceID
	testMap := make(map[string]Test)
	for _, test := range tests {
		testMap[test.TraceID] = test
	}

	for _, result := range results {
		if result.Passed {
			if !quiet {
				fmt.Printf("%s✓ NO DEVIATION - %s (%dms)%s\n", green, result.TestID, result.Duration, reset)
			}
		} else {
			fmt.Printf("%s● DEVIATION - %s (%dms)%s\n", orange, result.TestID, result.Duration, reset)

			if len(result.Deviations) > 0 {
				if test, exists := testMap[result.TestID]; exists {
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
				}

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

	if quiet && failed > 0 {
		fmt.Printf("\nTests: %d total, %s%d deviations%s\n", len(results), orange, failed, reset)
	} else if !quiet {
		fmt.Printf("\nTests: %d total, %s%d passed%s, %s%d deviations%s\n\n", len(results), green, passed, reset, orange, failed, reset)
	}

	if failed > 0 {
		return fmt.Errorf("%d tests with deviations", failed)
	}

	return nil
}
