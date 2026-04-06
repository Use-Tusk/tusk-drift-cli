package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Use-Tusk/fence/pkg/fence"
	"github.com/Use-Tusk/tusk-cli/internal/config"
	"github.com/Use-Tusk/tusk-cli/internal/log"
	"github.com/Use-Tusk/tusk-cli/internal/utils"
	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
)

const (
	requireInboundReplaySpanEnvVar = "TUSK_REQUIRE_INBOUND_REPLAY_SPAN"
	inboundSpanCheckTimeout        = 3 * time.Second
	inboundSpanDeviationField      = "replay.inbound_span"
)

const (
	SandboxModeAuto   = "auto"
	SandboxModeStrict = "strict"
	SandboxModeOff    = "off"
)

// syncBuffer is a thread-safe buffer for capturing service stdout/stderr concurrently.
type syncBuffer struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	discard bool
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.discard {
		return len(p), nil
	}
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// Discard makes future writes no-ops and frees the buffer memory.
func (s *syncBuffer) Discard() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.discard = true
	s.buf.Reset()
}

type Executor struct {
	serviceURL              string
	parallel                int
	testTimeout             time.Duration
	serviceCmd              *exec.Cmd
	server                  *Server
	serviceLogFile          *os.File
	serviceLogPath          string      // persists across StopService so GetStartupLogs can read it back
	startupLogBuffer        *syncBuffer // in-memory buffer when --enable-service-logs is off
	processExitCh           chan error  // signals early process exit
	enableServiceLogs       bool
	servicePort             int
	resultsDir              string
	ResultsFile             string // Will be set by the run command if --save-results is true
	OnTestCompleted         func(TestResult, Test)
	suiteSpans              []*core.Span
	globalSpans             []*core.Span // Explicitly marked global spans for cross-trace matching
	allowSuiteWideMatching  bool         // When true, allows cross-trace matching from any suite span
	cancelTests             context.CancelFunc
	sandboxBypass           bool // Internal runtime bypass used by auto-mode fallback retry
	sandboxMode             string
	lastServiceSandboxed    bool
	debug                   bool
	fenceManager            *fence.Manager
	requireInboundReplay    bool
	replayComposeOverride   string
	replayEnvVars           map[string]string
	replaySandboxConfigPath string

	// Coverage
	coverageEnabled         bool
	coverageShowOutput      bool
	coverageOutputPath      string
	coverageTempDir         string
	coverageIncludePatterns []string
	coverageExcludePatterns []string
	coverageStripPrefix     string
	coveragePerTest      map[string]map[string]CoverageFileDiff
	coveragePerTestMu    sync.Mutex
	coverageBaseline     CoverageSnapshot
	coverageBaselineMu   sync.Mutex
	coverageRecords      []CoverageTestRecord
	coverageRecordsMu    sync.Mutex
}

func NewExecutor() *Executor {
	return &Executor{
		serviceURL:           "http://localhost:3000",
		parallel:             5,
		testTimeout:          30 * time.Second,
		requireInboundReplay: isTruthyEnv(os.Getenv(requireInboundReplaySpanEnvVar)),
	}
}

// SetSandboxMode configures replay sandbox behavior.
// Supported values: auto, strict, off.
func (e *Executor) SetSandboxMode(mode string) error {
	switch mode {
	case SandboxModeAuto, SandboxModeStrict, SandboxModeOff:
		e.sandboxMode = mode
		return nil
	default:
		return fmt.Errorf("invalid sandbox mode %q (expected one of: auto, strict, off)", mode)
	}
}

// GetSandboxMode returns the configured replay sandbox mode.
// An empty string means the user did not explicitly configure a mode.
func (e *Executor) GetSandboxMode() string {
	return e.sandboxMode
}

// GetEffectiveSandboxMode returns the runtime sandbox mode after applying the
// platform-aware default for unset configs/flags.
func (e *Executor) GetEffectiveSandboxMode() string {
	if e.sandboxMode != "" {
		return e.sandboxMode
	}
	if fence.IsSupported() {
		return SandboxModeStrict
	}
	return SandboxModeAuto
}

// SetDebug enables debug mode for fence sandbox
func (e *Executor) SetDebug(debug bool) {
	e.debug = debug
}

// SetReplayEnvVars configures environment variables to inject into the replay
// service subprocess. This does not mutate the CLI process environment.
func (e *Executor) SetReplayEnvVars(envVars map[string]string) {
	if len(envVars) == 0 {
		e.replayEnvVars = nil
		return
	}

	copied := make(map[string]string, len(envVars))
	for key, value := range envVars {
		copied[key] = value
	}
	e.replayEnvVars = copied
}

func (e *Executor) getReplayEnvVars() map[string]string {
	if len(e.replayEnvVars) == 0 {
		return nil
	}

	copied := make(map[string]string, len(e.replayEnvVars))
	for key, value := range e.replayEnvVars {
		copied[key] = value
	}
	return copied
}

// SetReplaySandboxConfigPath configures an optional user-provided sandbox config
// to merge with the built-in replay sandbox policy.
func (e *Executor) SetReplaySandboxConfigPath(path string) {
	e.replaySandboxConfigPath = path
}

func (e *Executor) getReplaySandboxConfigPath() string {
	return e.replaySandboxConfigPath
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

		log.Debug("Processing batch", "start", i, "end", end, "size", len(batch))

		results, serverCrashed := e.RunBatchWithCrashDetection(batch, batchSize)

		if !serverCrashed {
			// No crash detected - invoke callbacks manually for all results
			log.Debug("Batch completed successfully, no crash detected", "batch_size", len(batch))
			if e.OnTestCompleted != nil {
				// Create a map of tests by TraceID for matching
				testsByID := make(map[string]Test, len(batch))
				for _, test := range batch {
					testsByID[test.TraceID] = test
				}
				// Invoke callbacks with correct test for each result
				for _, result := range results {
					if test, found := testsByID[result.TestID]; found {
						e.OnTestCompleted(result, test)
					}
				}
			}
			allResults = append(allResults, results...)
			continue
		}

		// Server crashed during batch - discard results, restart, and retry sequentially
		// Callbacks will fire during sequential execution from each test
		log.ServiceLog(fmt.Sprintf("❌  Server crashed during batch execution. Restarting and retrying %d tests sequentially...", len(batch)))

		if err := e.RestartServerWithRetry(0); err != nil {
			// Can't restart - mark all remaining tests as failed
			log.ServiceLog(fmt.Sprintf("❌ Failed to restart server: %v", err))
			log.ServiceLog("Marking all remaining tests as failed")

			for j := i; j < len(tests); j++ {
				// TODO: should this be a specific error type or at least message?
				// We weren't able to restart the server, hence not able to run the remaining tests
				result := TestResult{
					TestID: tests[j].TraceID,
					Passed: false,
					Error:  fmt.Sprintf("Server repeatedly crashed, cannot continue: %v", err),
				}
				allResults = append(allResults, result)
				// Invoke callback for these failed results
				if e.OnTestCompleted != nil {
					e.OnTestCompleted(result, tests[j])
				}
			}
			return allResults, nil
		}

		// Re-run batch sequentially (callbacks fire normally)
		hasMoreTests := end < len(tests) // Are there more tests after this batch?
		sequentialResults := e.RunBatchSequentialWithCrashHandling(batch, hasMoreTests)
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
				log.Debug("Worker starting test", "workerID", workerID, "testID", test.TraceID)
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
						log.Debug("Worker test failed", "workerID", workerID, "testID", test.TraceID, "error", err)
					} else {
						log.Debug("Worker test completed", "workerID", workerID, "testID", test.TraceID, "passed", result.Passed)
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

	log.Debug("Completed concurrent test execution",
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

	// Check if any result has an error
	hasErrors := false
	for _, result := range results {
		if result.Error != "" {
			hasErrors = true
			break
		}
	}

	// If errors found, check if server is still healthy
	serverCrashed := false
	if hasErrors {
		serverCrashed = !e.CheckServerHealth()
		if serverCrashed {
			log.Debug("Server crash detected via health check", "batch_size", len(batch))
		}
	}

	return results, serverCrashed
}

// RunBatchSequentialWithCrashHandling runs a batch of tests sequentially, restarting after each crash
// hasMoreTestsAfterBatch indicates if there are more tests to run after this batch completes
func (e *Executor) RunBatchSequentialWithCrashHandling(batch []Test, hasMoreTestsAfterBatch bool) []TestResult {
	results := make([]TestResult, 0, len(batch))
	consecutiveRestartAttempt := 0

	for idx, test := range batch {
		log.Debug("Running test sequentially", "index", idx+1, "total", len(batch), "testID", test.TraceID)
		log.ServiceLog(fmt.Sprintf("Running test %d/%d sequentially: %s", idx+1, len(batch), test.TraceID))

		result, err := e.RunSingleTest(test)
		result.RetriedAfterCrash = true

		// Check if this test crashed the server
		if err != nil && !e.CheckServerHealth() {
			log.Warn("Test crashed the server", "testID", test.TraceID, "error", err)
			log.ServiceLog(fmt.Sprintf("⚠️  Test %s crashed the server", test.TraceID))

			result.CrashedServer = true

			// Try to restart for next test (either in this batch or subsequent batches)
			shouldRestart := (idx < len(batch)-1) || hasMoreTestsAfterBatch
			if shouldRestart {
				log.ServiceLog("Restarting server for next test...")
				if restartErr := e.RestartServerWithRetry(consecutiveRestartAttempt); restartErr != nil {
					consecutiveRestartAttempt++
					// If multiple tests in a row crash the server, we need to mark the remaining tests as failed
					if consecutiveRestartAttempt >= MaxServerRestartAttempts {
						// Mark remaining tests in batch as failed
						log.ServiceLog(fmt.Sprintf("❌ Exceeded maximum restart attempts. Marking remaining %d tests as failed.", len(batch)-idx-1))
						results = append(results, result)
						for j := idx + 1; j < len(batch); j++ {
							failedResult := TestResult{
								TestID: batch[j].TraceID,
								Passed: false,
								Error:  "Server repeatedly crashed, cannot continue testing",
							}
							results = append(results, failedResult)
							// Invoke callback for these failed results
							if e.OnTestCompleted != nil {
								e.OnTestCompleted(failedResult, batch[j])
							}
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
		}

		results = append(results, result)

		// Invoke callback for this test result
		if e.OnTestCompleted != nil {
			e.OnTestCompleted(result, test)
		}
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

// GetSuiteSpans returns the suite spans (includes pre-app-start spans)
func (e *Executor) GetSuiteSpans() []*core.Span {
	return e.suiteSpans
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
	e.OnTestCompleted = callback
}

func (e *Executor) SetCoverageEnabled(enabled bool) {
	e.coverageEnabled = enabled
}

func (e *Executor) IsCoverageEnabled() bool {
	return e.coverageEnabled
}

func (e *Executor) SetShowCoverage(show bool) {
	e.coverageShowOutput = show
}

func (e *Executor) IsCoverageShowOutput() bool {
	return e.coverageShowOutput
}

func (e *Executor) GetCoverageOutputPath() string {
	return e.coverageOutputPath
}

// GetCoverageBaselineForUpload computes the full baseline by merging the raw baseline
// with all per-test records. This ensures the denominator includes lines discovered
// during test execution that weren't in the initial baseline snapshot.
func (e *Executor) GetCoverageBaselineForUpload() CoverageSnapshot {
	e.coverageBaselineMu.Lock()
	baseline := e.coverageBaseline
	e.coverageBaselineMu.Unlock()

	records := e.GetCoverageRecords()
	if baseline == nil && len(records) == 0 {
		return nil
	}

	// Merge baseline with ALL per-test records (not filtered by suite status)
	// to get the complete set of coverable lines for the denominator
	aggregate := mergeWithBaseline(baseline, records)

	// Apply include/exclude patterns
	aggregate = filterCoverageByPatterns(aggregate, e.coverageIncludePatterns, e.coverageExcludePatterns)

	return aggregate
}

func (e *Executor) SetCoverageOutputPath(path string) {
	e.coverageOutputPath = path
}

func (e *Executor) SetCoverageIncludePatterns(patterns []string) {
	e.coverageIncludePatterns = patterns
}

func (e *Executor) SetCoverageExcludePatterns(patterns []string) {
	e.coverageExcludePatterns = patterns
}

func (e *Executor) SetCoverageStripPrefix(prefix string) {
	e.coverageStripPrefix = prefix
}

// SetCoverageBaseline merges new baseline data into the existing baseline.
// Called per environment group - accumulates across service restarts.
func (e *Executor) SetCoverageBaseline(baseline CoverageSnapshot) {
	e.coverageBaselineMu.Lock()
	defer e.coverageBaselineMu.Unlock()
	if e.coverageBaseline == nil {
		e.coverageBaseline = make(CoverageSnapshot)
	}
	for filePath, fileData := range baseline {
		existing, ok := e.coverageBaseline[filePath]
		if !ok {
			existing = FileCoverageData{
				Lines:    make(map[string]int),
				Branches: make(map[string]BranchInfo),
			}
		}
		for line, count := range fileData.Lines {
			if existingCount, ok := existing.Lines[line]; !ok || existingCount == 0 {
				existing.Lines[line] = count
			}
		}
		// Merge branch data (keep max per line)
		for line, branchInfo := range fileData.Branches {
			if existing.Branches == nil {
				existing.Branches = make(map[string]BranchInfo)
			}
			if eb, ok := existing.Branches[line]; !ok || branchInfo.Total > eb.Total {
				existing.Branches[line] = branchInfo
			}
		}
		// Recompute file-level totals from merged per-line data
		totalB, covB := 0, 0
		for _, b := range existing.Branches {
			totalB += b.Total
			covB += b.Covered
		}
		existing.TotalBranches = totalB
		existing.CoveredBranches = covB
		e.coverageBaseline[filePath] = existing
	}
}

// SetTestCoverageDetail stores per-test coverage diff for display in TUI/print.
func (e *Executor) SetTestCoverageDetail(testID string, detail map[string]CoverageFileDiff) {
	e.coveragePerTestMu.Lock()
	defer e.coveragePerTestMu.Unlock()
	if e.coveragePerTest == nil {
		e.coveragePerTest = make(map[string]map[string]CoverageFileDiff)
	}
	e.coveragePerTest[testID] = detail
}

// GetTestCoverageDetail returns a copy of per-test coverage diff for a given test.
func (e *Executor) GetTestCoverageDetail(testID string) map[string]CoverageFileDiff {
	e.coveragePerTestMu.Lock()
	defer e.coveragePerTestMu.Unlock()
	if e.coveragePerTest == nil {
		return nil
	}
	original := e.coveragePerTest[testID]
	if original == nil {
		return nil
	}
	// Return a copy to avoid concurrent map access from TUI goroutines
	copied := make(map[string]CoverageFileDiff, len(original))
	for k, v := range original {
		copied[k] = v
	}
	return copied
}

// AddCoverageRecord stores a per-test coverage record.
func (e *Executor) AddCoverageRecord(record CoverageTestRecord) {
	e.coverageRecordsMu.Lock()
	defer e.coverageRecordsMu.Unlock()
	e.coverageRecords = append(e.coverageRecords, record)
}

// GetCoverageRecords returns a copy of all coverage records.
func (e *Executor) GetCoverageRecords() []CoverageTestRecord {
	e.coverageRecordsMu.Lock()
	defer e.coverageRecordsMu.Unlock()
	records := make([]CoverageTestRecord, len(e.coverageRecords))
	copy(records, e.coverageRecords)
	return records
}

func (e *Executor) SetSuiteSpans(spans []*core.Span) {
	e.suiteSpans = spans
	if e.server != nil && len(spans) > 0 {
		e.server.SetSuiteSpans(spans)
	}
}

func (e *Executor) SetGlobalSpans(spans []*core.Span) {
	e.globalSpans = spans
	if e.server != nil && len(spans) > 0 {
		e.server.SetGlobalSpans(spans)
	}
}

func (e *Executor) SetAllowSuiteWideMatching(enabled bool) {
	e.allowSuiteWideMatching = enabled
	if e.server != nil {
		e.server.SetAllowSuiteWideMatching(enabled)
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
	cfg, err := config.Get()
	if err != nil {
		log.Debug("Failed to get config for health check", "error", err)
		return false
	}

	// Use readiness command if configured
	if cfg.Service.Readiness.Command != "" {
		cmd := createReadinessCommand(cfg.Service.Readiness.Command)
		cmd.Env = e.buildCommandEnv()
		if err := cmd.Run(); err == nil {
			return true
		}
		log.Debug("Readiness command failed", "command", cfg.Service.Readiness.Command)
		return false
	}

	// Fallback to simple HTTP HEAD request if no readiness command configured
	client := &http.Client{Timeout: 2 * time.Second}

	req, err := http.NewRequest("HEAD", e.serviceURL, nil)
	if err != nil {
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Debug("Health check failed", "error", err)
		return false
	}
	defer func() { _ = resp.Body.Close() }()

	// Any response (even error status codes) means the server is alive
	return true
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

// GetStartupLogs returns the captured service startup logs.
// When --enable-service-logs is set, it reads back from the log file.
// Otherwise, it returns the contents of the in-memory startup buffer.
func (e *Executor) GetStartupLogs() string {
	if e.enableServiceLogs && e.serviceLogPath != "" {
		data, err := os.ReadFile(e.serviceLogPath)
		if err != nil {
			return ""
		}
		return string(data)
	}
	if e.startupLogBuffer != nil {
		return e.startupLogBuffer.String()
	}
	return ""
}

// DiscardStartupBuffer makes the in-memory startup log buffer discard future writes
// and frees its memory. This is called after the service starts successfully to avoid
// unbounded memory growth during the test run. Has no effect when --enable-service-logs
// is set (file-based logging persists for the full run).
//
// Note: we can't swap cmd.Stdout after Start() because Go's exec package captures the
// writer by reference in an internal goroutine at Start() time. Instead, we set a flag
// on the buffer itself to make Write a no-op.
func (e *Executor) DiscardStartupBuffer() {
	if !e.enableServiceLogs && e.startupLogBuffer != nil {
		e.startupLogBuffer.Discard()
		e.startupLogBuffer = nil
	}
}

// GetStartupFailureHelpMessage returns a user-friendly help message when the service fails to start.
func (e *Executor) GetStartupFailureHelpMessage() string {
	if e.enableServiceLogs && e.serviceLogPath != "" {
		return fmt.Sprintf("\n📄 Service logs are available at: %s\n", e.serviceLogPath)
	}
	return ""
}

// RunSingleTest replays a single trace on the service under test.
// NOTE: this does not invoke the OnTestCompleted callback. It is the responsibility of the caller to invoke it.
func (e *Executor) RunSingleTest(test Test) (TestResult, error) {
	// Load all spans for this trace into the server for sophisticated matching
	if e.server != nil {
		if len(test.Spans) > 0 {
			e.server.LoadSpansForTrace(test.TraceID, test.Spans)
		} else {
			spans, err := e.LoadSpansForTrace(test.TraceID, test.FileName)
			if err != nil {
				log.Warn("Failed to load spans for trace", "traceID", test.TraceID, "error", err)
			} else {
				e.server.LoadSpansForTrace(test.TraceID, spans)
				test.Spans = spans // Ensure spans are available for time-travel and schema extraction
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

	if test.Request.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	for k, v := range test.Request.Headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: e.testTimeout}

	// Send time travel request to Python SDK before making HTTP request
	// This ensures auth checks at the inbound request level use the recorded time
	if e.server != nil && e.server.GetSDKRuntime() == core.Runtime_RUNTIME_PYTHON {
		timestamp, source := GetFirstSpanTimestamp(test.Spans)
		if timestamp > 0 {
			if err := e.server.SendSetTimeTravel(timestamp, test.TraceID, source); err != nil {
				// Log warning but don't fail the test - time travel is best-effort
				log.Warn("Failed to set time travel", "error", err, "traceID", test.TraceID)
			} else {
				log.Debug("Time travel set", "timestamp", timestamp, "source", source, "traceID", test.TraceID)
			}
		}
	}

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
		return result, err
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Warn("Failed to close response body", "error", err)
		}
	}()

	result, _ := e.compareAndGenerateResult(test, resp, duration)
	e.enforceInboundReplaySpanIfRequired(test.TraceID, &result)

	return result, nil
}

func (e *Executor) enforceInboundReplaySpanIfRequired(traceID string, result *TestResult) {
	if !e.requireInboundReplay || e.server == nil || result == nil {
		return
	}

	e.server.WaitForInboundSpan(traceID, inboundSpanCheckTimeout)
	if e.server.GetInboundReplaySpan(traceID) != nil {
		return
	}

	description := "Inbound replay span missing; SDK did not send replay span to CLI"
	log.Warn(description, "traceID", traceID)

	result.Passed = false
	for _, deviation := range result.Deviations {
		if deviation.Field == inboundSpanDeviationField {
			return
		}
	}

	result.Deviations = append(result.Deviations, Deviation{
		Field:       inboundSpanDeviationField,
		Description: description,
	})
}

func isTruthyEnv(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
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

	// Handle server crash scenario
	if result.CrashedServer {
		msg := fmt.Sprintf("SERVER CRASHED - %s (%dms)", result.TestID, result.Duration)
		if result.RetriedAfterCrash {
			log.UserError(msg + " [retried]")
		} else {
			log.UserError(msg)
		}
		if result.Error != "" {
			log.Println(fmt.Sprintf("  Error: %s", result.Error))
		}
		return
	}

	if result.Passed {
		if !quiet {
			msg := fmt.Sprintf("NO DEVIATION - %s (%dms)", result.TestID, result.Duration)
			if result.RetriedAfterCrash {
				log.UserSuccess(msg + " [retried after crash]")
			} else {
				log.UserSuccess(msg)
			}
		}
	} else {
		msg := fmt.Sprintf("DEVIATION - %s (%dms)", result.TestID, result.Duration)
		if result.RetriedAfterCrash {
			log.UserDeviation(msg + " [retried after crash]")
		} else {
			log.UserDeviation(msg)
		}

		if verbose && !quiet && len(result.Deviations) > 0 {
			log.Println(fmt.Sprintf("  Request: %s %s", test.Request.Method, test.Request.Path))
			if len(test.Request.Headers) > 0 {
				log.Println("  Headers:")
				for key, value := range test.Request.Headers {
					log.Println(fmt.Sprintf("    %s: %s", key, value))
				}
			}
			if test.Request.Body != nil {
				log.Println(fmt.Sprintf("  Body: %v", test.Request.Body))
			}
			log.Println("")

			for _, dev := range result.Deviations {
				log.UserWarn(fmt.Sprintf("  Deviation: %s", dev.Description))
				log.Println(fmt.Sprintf("    Expected: %v", dev.Expected))
				log.Println(fmt.Sprintf("    Actual: %v", dev.Actual))
			}
		}

		if result.Error != "" {
			log.Println(fmt.Sprintf("  Error: %s", result.Error))
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
		switch {
		case crashed > 0 && failed > 0:
			return fmt.Errorf("%d tests with deviations, %d crashed server", failed, crashed)
		case crashed > 0:
			return fmt.Errorf("%d tests crashed server", crashed)
		default:
			return fmt.Errorf("%d tests with deviations", failed)
		}
	}

	return nil
}

// GetFirstSpanTimestamp returns the timestamp to use for time travel.
// Priority: server/root span > earliest non-server span.
// Root span is preferred for inbound-level determinism (e.g., caching keys derived from time).
func GetFirstSpanTimestamp(spans []*core.Span) (float64, string) {
	var firstTimestamp float64 = math.MaxFloat64
	var serverSpanTimestamp float64 = 0
	var foundNonServerSpan bool

	for _, span := range spans {
		if span == nil || span.Timestamp == nil {
			continue
		}

		spanTimestamp := float64(span.Timestamp.Seconds) + float64(span.Timestamp.Nanos)/1e9

		// Track server span
		if span.IsRootSpan {
			serverSpanTimestamp = spanTimestamp
			continue
		}

		// Track earliest non-server span
		if spanTimestamp < firstTimestamp {
			firstTimestamp = spanTimestamp
			foundNonServerSpan = true
		}
	}

	// Prefer root/server span for inbound-level determinism (e.g., caching keys derived from time).
	// Fall back to the earliest recorded non-server span if no root span exists.
	if serverSpanTimestamp > 0 {
		return serverSpanTimestamp, "server_span"
	}

	if foundNonServerSpan {
		return firstTimestamp, "first_span"
	}

	// No timestamp available
	return 0, "none"
}
