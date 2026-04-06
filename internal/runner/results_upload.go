package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Use-Tusk/tusk-cli/internal/api"
	"github.com/Use-Tusk/tusk-cli/internal/config"
	"github.com/Use-Tusk/tusk-cli/internal/version"
	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
)

func (e *Executor) WriteRunResultsToFile(tests []Test, results []TestResult) (string, error) {
	// Resolve output path
	if err := config.Load(""); err != nil {
		return "", fmt.Errorf("failed to load config: %w", err)
	}

	dir := e.resultsDir

	if dir == "" {
		// This should be set by the run command if --save-results is true
		return "", fmt.Errorf("results directory is not set")
	}

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("failed to create results dir: %w", err)
	}

	outPath := e.ResultsFile
	if outPath == "" {
		return "", fmt.Errorf("results file is not set")
	}

	// Build quick lookup for tests
	testByID := make(map[string]Test, len(tests))
	for _, t := range tests {
		testByID[t.TraceID] = t
	}

	sdkVersion := ""
	if e.server != nil {
		sdkVersion = e.server.GetSDKVersion()
	}

	req := &backend.UploadTraceTestResultsRequest{
		DriftRunId:       "", // Optional/unknown in local runs
		CliVersion:       version.Version,
		SdkVersion:       sdkVersion,
		TraceTestResults: BuildTraceTestResultsProto(e, results, tests),
	}

	f, err := os.Create(outPath) // #nosec G304
	if err != nil {
		return "", fmt.Errorf("failed to create results file: %w", err)
	}
	defer func() { _ = f.Close() }()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(req); err != nil {
		return "", fmt.Errorf("failed to write results: %w", err)
	}

	return outPath, nil
}

func UploadSingleTestResult(
	ctx context.Context,
	client *api.TuskClient,
	driftRunID string,
	auth api.AuthOptions,
	e *Executor,
	res TestResult,
	test Test,
) error {
	waitForSpanDataTimeout := 3000 * time.Millisecond

	if e != nil && waitForSpanDataTimeout > 0 {
		e.WaitForSpanData(test.TraceID, waitForSpanDataTimeout)
	}

	if e != nil && e.server != nil {
		e.server.WaitForInboundSpan(test.TraceID, waitForSpanDataTimeout)
	}

	sdkVersion := "unknown"
	if e != nil && e.server != nil {
		if v := e.server.GetSDKVersion(); v != "" {
			sdkVersion = v
		}
	}

	req := &backend.UploadTraceTestResultsRequest{
		DriftRunId:       driftRunID,
		CliVersion:       version.Version,
		SdkVersion:       sdkVersion,
		TraceTestResults: BuildTraceTestResultsProto(e, []TestResult{res}, []Test{test}),
	}
	return client.UploadTraceTestResults(ctx, req, auth)
}

func ReportDriftRunSuccess(
	ctx context.Context,
	client *api.TuskClient,
	driftRunID string,
	authOptions api.AuthOptions,
	results []TestResult,
	coverageBaseline CoverageSnapshot,
	commitSha string,
	statusMessageOverride ...string,
) error {
	// Note: We always report SUCCESS status here unless there was an error executing tests.
	// Individual test failures (assertions, deviations, etc.) are not considered CI failures.
	// The CI run succeeded if all tests were executed, regardless of test outcomes.
	finalStatus := backend.DriftRunCIStatus_DRIFT_RUN_CI_STATUS_SUCCESS
	statusMessage := fmt.Sprintf("Completed %d tests", len(results))
	if len(statusMessageOverride) > 0 && statusMessageOverride[0] != "" {
		statusMessage = statusMessageOverride[0]
	}
	statusReq := &backend.UpdateDriftRunCIStatusRequest{
		DriftRunId:      driftRunID,
		CiStatus:        finalStatus,
		CiStatusMessage: &statusMessage,
	}

	// Attach coverage baseline if available
	if coverageBaseline != nil {
		statusReq.CoverageBaseline = buildCoverageBaselineProto(coverageBaseline, commitSha)
	}

	return client.UpdateDriftRunCIStatus(ctx, statusReq, authOptions)
}

func buildCoverageBaselineProto(snapshot CoverageSnapshot, commitSha string) *backend.CoverageBaseline {
	baseline := &backend.CoverageBaseline{
		CommitSha:                  commitSha,
		CoverableLinesByFile:      make(map[string]*backend.FileLineRanges),
		StartupCoveredLinesByFile: make(map[string]*backend.FileLineRanges),
	}
	totalCoverable := int32(0)
	for filePath, fileData := range snapshot {
		totalCoverable += int32(len(fileData.Lines))

		var allLines []int32
		var coveredLines []int32
		for lineStr, count := range fileData.Lines {
			if n, err := strconv.Atoi(lineStr); err == nil {
				allLines = append(allLines, int32(n))
				if count > 0 {
					coveredLines = append(coveredLines, int32(n))
				}
			}
		}

		sort.Slice(allLines, func(i, j int) bool { return allLines[i] < allLines[j] })
		baseline.CoverableLinesByFile[filePath] = toLineRangesProto(allLines)

		if len(coveredLines) > 0 {
			sort.Slice(coveredLines, func(i, j int) bool { return coveredLines[i] < coveredLines[j] })
			baseline.StartupCoveredLinesByFile[filePath] = toLineRangesProto(coveredLines)
		}
	}
	baseline.TotalCoverableLines = totalCoverable
	return baseline
}

// toLineRangesProto compresses sorted int32s into LineRange protos.
// [1,2,3,5,6,10] -> [{1,3},{5,6},{10,10}]
func toLineRangesProto(sorted []int32) *backend.FileLineRanges {
	if len(sorted) == 0 {
		return &backend.FileLineRanges{}
	}
	var ranges []*backend.LineRange
	start, end := sorted[0], sorted[0]
	for i := 1; i < len(sorted); i++ {
		if sorted[i] == end+1 {
			end = sorted[i]
		} else {
			ranges = append(ranges, &backend.LineRange{Start: start, End: end})
			start = sorted[i]
			end = sorted[i]
		}
	}
	ranges = append(ranges, &backend.LineRange{Start: start, End: end})
	return &backend.FileLineRanges{Ranges: ranges}
}

func BuildTraceTestResultsProto(e *Executor, results []TestResult, tests []Test) []*backend.TraceTestResult {
	out := make([]*backend.TraceTestResult, 0, len(results))

	// Build quick lookup from traceId to test (to access TraceTestID)
	testByTrace := make(map[string]Test, len(tests))
	for _, t := range tests {
		testByTrace[t.TraceID] = t
	}

	for _, r := range results {
		traceTestID := r.TestID
		if t, ok := testByTrace[r.TestID]; ok && t.TraceTestID != "" {
			traceTestID = t.TraceTestID
		}
		tr := &backend.TraceTestResult{
			TraceTestId: traceTestID,
			TestSuccess: r.Passed,
		}

		if !r.Passed || r.CrashedServer {
			switch {
			case r.CrashedServer:
				// Test crashed the server
				reason := backend.TraceTestFailureReason_TRACE_TEST_FAILURE_REASON_NO_RESPONSE
				tr.TestFailureReason = &reason
				msg := "Test caused server to crash"
				if r.Error != "" {
					msg = fmt.Sprintf("Test caused server to crash: %s", r.Error)
				}
				tr.TestFailureMessage = &msg
				r.Deviations = append(r.Deviations, Deviation{
					Field:       "response",
					Description: fmt.Sprintf("No response received: %s", msg),
				})
			case e != nil && e.server != nil && e.server.HasMockNotFoundEvents(r.TestID):
				// Check if there were any mock-not-found events during replay
				reason := backend.TraceTestFailureReason_TRACE_TEST_FAILURE_REASON_MOCK_NOT_FOUND
				tr.TestFailureReason = &reason
				msg := "Mock not found during replay"
				tr.TestFailureMessage = &msg

				// Build deviation message with details about which calls failed
				mockEvents := e.server.GetMockNotFoundEvents(r.TestID)
				var failedCalls []string
				for _, ev := range mockEvents {
					failedCalls = append(failedCalls, fmt.Sprintf("%s %s", ev.PackageName, ev.SpanName))
				}
				r.Deviations = append(r.Deviations, Deviation{
					Field:       "response",
					Description: fmt.Sprintf("Test failed: mock not found for outbound call(s): %s", strings.Join(failedCalls, ", ")),
				})
			case r.Error != "":
				// HTTP request failed (network error, timeout, etc.)
				reason := backend.TraceTestFailureReason_TRACE_TEST_FAILURE_REASON_NO_RESPONSE
				tr.TestFailureReason = &reason
				tr.TestFailureMessage = &r.Error
				r.Deviations = append(r.Deviations, Deviation{
					Field:       "response",
					Description: fmt.Sprintf("No response received: %s", r.Error),
				})
			default:
				// Response received but doesn't match expected
				reason := backend.TraceTestFailureReason_TRACE_TEST_FAILURE_REASON_RESPONSE_MISMATCH
				tr.TestFailureReason = &reason
			}
		}

		// Inbound replay + root span id + deviations
		if e != nil && e.server != nil {
			inbound := e.server.GetInboundReplaySpan(r.TestID)
			rootID := e.server.GetRootSpanID(r.TestID)
			if inbound != nil || len(r.Deviations) > 0 || rootID != "" {
				inboundRes := &backend.TraceTestSpanResult{}
				if inbound != nil {
					inboundRes.ReplaySpan = inbound
				}
				if rootID != "" {
					inboundRes.MatchedSpanRecordingId = &rootID
				}
				for _, d := range r.Deviations {
					inboundRes.Deviations = append(inboundRes.Deviations, &backend.Deviation{
						Field:       d.Field,
						Description: d.Description,
					})
				}
				tr.SpanResults = append(tr.SpanResults, inboundRes)
			}

			// Outbound match events
			events := e.server.GetMatchEvents(r.TestID)
			for i := range events {
				ev := events[i]
				spanRes := &backend.TraceTestSpanResult{
					MatchedSpanRecordingId: &ev.SpanID,
					MatchLevel:             ev.MatchLevel,
				}
				if ev.StackTrace != "" {
					spanRes.StackTrace = &ev.StackTrace
				}
				if ev.ReplaySpan != nil {
					spanRes.ReplaySpan = ev.ReplaySpan
				}
				tr.SpanResults = append(tr.SpanResults, spanRes)
			}

			// Mock-not-found events (outbound requests that had no matching recording)
			mockNotFoundEvents := e.server.GetMockNotFoundEvents(r.TestID)
			for i := range mockNotFoundEvents {
				ev := mockNotFoundEvents[i]
				spanRes := &backend.TraceTestSpanResult{
					MatchedSpanRecordingId: nil,
					MatchLevel:             nil,
				}
				if ev.StackTrace != "" {
					spanRes.StackTrace = &ev.StackTrace
				}
				if ev.ReplaySpan != nil {
					spanRes.ReplaySpan = ev.ReplaySpan
				}
				tr.SpanResults = append(tr.SpanResults, spanRes)
			}
		}

		// Per-test coverage data (if coverage is enabled)
		if e != nil && e.IsCoverageEnabled() {
			detail := e.GetTestCoverageDetail(r.TestID)
			if len(detail) > 0 {
				covData := &backend.TraceTestCoverageData{
					CoveredLinesByFile: make(map[string]*backend.FileLineRanges),
				}
				totalCovered := int32(0)
				for filePath, fd := range detail {
					totalCovered += int32(fd.CoveredCount)
					sorted := toInt32Slice(fd.CoveredLines)
					covData.CoveredLinesByFile[filePath] = toLineRangesProto(sorted)
				}
				covData.TotalCoveredLines = totalCovered
				tr.CoverageData = covData
			}
		}

		out = append(out, tr)
	}
	return out
}

func toInt32Slice(ints []int) []int32 {
	result := make([]int32, len(ints))
	for i, v := range ints {
		result[i] = int32(v)
	}
	return result
}
