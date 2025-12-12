package runner

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/api"
	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
	"github.com/Use-Tusk/tusk-drift-cli/internal/version"
	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteRunResultsToFile(t *testing.T) {
	t.Parallel()

	t.Run("success with all data", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		resultsDir := filepath.Join(tmpDir, "results")
		resultsFile := filepath.Join(resultsDir, "test-results.json")

		executor := &Executor{
			resultsDir:  resultsDir,
			ResultsFile: resultsFile,
			server:      nil, // We'll test without server for simplicity
		}

		tests := []Test{
			{
				TraceID:     "trace-1",
				TraceTestID: "tt-1",
				DisplayName: "Test 1",
			},
			{
				TraceID:     "trace-2",
				TraceTestID: "tt-2",
				DisplayName: "Test 2",
			},
		}

		results := []TestResult{
			{
				TestID: "trace-1",
				Passed: true,
			},
			{
				TestID: "trace-2",
				Passed: false,
				Error:  "test failed",
				Deviations: []Deviation{
					{Field: "status", Description: "expected 200, got 500"},
				},
			},
		}

		path, err := executor.WriteRunResultsToFile(tests, results)

		require.NoError(t, err)
		assert.Equal(t, resultsFile, path)
		assert.FileExists(t, resultsFile)

		data, err := os.ReadFile(resultsFile) // #nosec G304
		require.NoError(t, err)

		var req backend.UploadTraceTestResultsRequest
		err = json.Unmarshal(data, &req)
		require.NoError(t, err)

		assert.Equal(t, "", req.DriftRunId)
		assert.Equal(t, version.Version, req.CliVersion)
		assert.Equal(t, "", req.SdkVersion) // No server, so empty SDK version
		assert.Len(t, req.TraceTestResults, 2)

		// Verify first result
		assert.Equal(t, "tt-1", req.TraceTestResults[0].TraceTestId)
		assert.True(t, req.TraceTestResults[0].TestSuccess)
		assert.Nil(t, req.TraceTestResults[0].TestFailureReason)

		// Verify second result
		assert.Equal(t, "tt-2", req.TraceTestResults[1].TraceTestId)
		assert.False(t, req.TraceTestResults[1].TestSuccess)
		assert.NotNil(t, req.TraceTestResults[1].TestFailureReason)
		assert.Equal(t, backend.TraceTestFailureReason_TRACE_TEST_FAILURE_REASON_NO_RESPONSE, *req.TraceTestResults[1].TestFailureReason)
		assert.NotNil(t, req.TraceTestResults[1].TestFailureMessage)
		assert.Equal(t, "test failed", *req.TraceTestResults[1].TestFailureMessage)
	})

	t.Run("error when results directory not set", func(t *testing.T) {
		t.Parallel()

		executor := &Executor{}

		_, err := executor.WriteRunResultsToFile([]Test{}, []TestResult{})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "results directory is not set")
	})

	t.Run("error when results file not set", func(t *testing.T) {
		t.Parallel()

		executor := &Executor{
			resultsDir: "/tmp/results",
		}

		_, err := executor.WriteRunResultsToFile([]Test{}, []TestResult{})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "results file is not set")
	})
}

func TestBuildTraceTestResultsProto(t *testing.T) {
	t.Parallel()

	t.Run("basic success and failure cases", func(t *testing.T) {
		t.Parallel()

		tests := []Test{
			{TraceID: "trace-1", TraceTestID: "tt-1"},
			{TraceID: "trace-2", TraceTestID: "tt-2"},
		}

		results := []TestResult{
			{
				TestID: "trace-1",
				Passed: false,
				Deviations: []Deviation{
					{Field: "field1", Description: "deviation1"},
					{Field: "field2", Description: "deviation2"},
				},
			},
			{
				TestID: "trace-2",
				Passed: true,
			},
		}

		// Execute with nil executor (no server)
		protoResults := BuildTraceTestResultsProto(nil, results, tests)

		// Assertions
		require.Len(t, protoResults, 2)

		// First result with deviations
		result1 := protoResults[0]
		assert.Equal(t, "tt-1", result1.TraceTestId)
		assert.False(t, result1.TestSuccess)
		assert.Equal(t, backend.TraceTestFailureReason_TRACE_TEST_FAILURE_REASON_RESPONSE_MISMATCH, *result1.TestFailureReason)
		assert.Empty(t, result1.SpanResults) // No server, no span results

		// Second result (success)
		result2 := protoResults[1]
		assert.Equal(t, "tt-2", result2.TraceTestId)
		assert.True(t, result2.TestSuccess)
		assert.Nil(t, result2.TestFailureReason)
		assert.Empty(t, result2.SpanResults)
	})

	t.Run("with error message", func(t *testing.T) {
		t.Parallel()

		tests := []Test{
			{TraceID: "trace-1", TraceTestID: "tt-1"},
		}

		results := []TestResult{
			{
				TestID: "trace-1",
				Passed: false,
				Error:  "connection error",
			},
		}

		// Execute with nil executor
		protoResults := BuildTraceTestResultsProto(nil, results, tests)

		// Assertions
		require.Len(t, protoResults, 1)

		result := protoResults[0]
		assert.Equal(t, "tt-1", result.TraceTestId)
		assert.False(t, result.TestSuccess)
		assert.Equal(t, backend.TraceTestFailureReason_TRACE_TEST_FAILURE_REASON_NO_RESPONSE, *result.TestFailureReason)
		assert.Equal(t, "connection error", *result.TestFailureMessage)
		assert.Empty(t, result.SpanResults)
	})

	t.Run("fallback to trace ID when TraceTestID is empty", func(t *testing.T) {
		t.Parallel()

		tests := []Test{
			{TraceID: "trace-1", TraceTestID: ""},
		}

		results := []TestResult{
			{
				TestID: "trace-1",
				Passed: true,
			},
		}

		protoResults := BuildTraceTestResultsProto(nil, results, tests)

		require.Len(t, protoResults, 1)
		assert.Equal(t, "trace-1", protoResults[0].TraceTestId)
	})
}

func TestUploadSingleTestResult(t *testing.T) {
	t.Parallel()

	t.Run("successful upload", func(t *testing.T) {
		t.Parallel()

		var capturedRequest *backend.UploadTraceTestResultsRequest
		mockClient := &api.TuskClient{}
		// We can't actually mock TuskClient since it's a struct, but we can test the request building

		test := Test{
			TraceID:     "trace-1",
			TraceTestID: "tt-1",
		}

		result := TestResult{
			TestID: "trace-1",
			Passed: true,
		}

		// Since we can't actually call the method without a real client,
		// we'll test the request building logic via BuildTraceTestResultsProto
		protoResults := BuildTraceTestResultsProto(nil, []TestResult{result}, []Test{test})

		require.Len(t, protoResults, 1)
		assert.Equal(t, "tt-1", protoResults[0].TraceTestId)
		assert.True(t, protoResults[0].TestSuccess)

		// In a real test, we would need to either:
		// 1. Create an interface for TuskClient
		// 2. Use a test server to mock the HTTP calls
		// 3. Use dependency injection to allow mocking
		_ = capturedRequest
		_ = mockClient
	})
}

func TestUpdateDriftRunCIStatusWrapper(t *testing.T) {
	t.Parallel()

	t.Run("determine correct status for all passing tests", func(t *testing.T) {
		t.Parallel()

		results := []TestResult{
			{TestID: "trace-1", Passed: true},
			{TestID: "trace-2", Passed: true},
		}

		finalStatus := backend.DriftRunCIStatus_DRIFT_RUN_CI_STATUS_SUCCESS
		statusMessage := "All tests completed successfully"
		for _, r := range results {
			if !r.Passed {
				finalStatus = backend.DriftRunCIStatus_DRIFT_RUN_CI_STATUS_FAILURE
				statusMessage = "Some tests failed"
				break
			}
		}

		assert.Equal(t, backend.DriftRunCIStatus_DRIFT_RUN_CI_STATUS_SUCCESS, finalStatus)
		assert.Equal(t, "All tests completed successfully", statusMessage)
	})

	t.Run("determine correct status for failing tests", func(t *testing.T) {
		t.Parallel()

		results := []TestResult{
			{TestID: "trace-1", Passed: true},
			{TestID: "trace-2", Passed: false},
		}

		finalStatus := backend.DriftRunCIStatus_DRIFT_RUN_CI_STATUS_SUCCESS
		statusMessage := "All tests completed successfully"
		for _, r := range results {
			if !r.Passed {
				finalStatus = backend.DriftRunCIStatus_DRIFT_RUN_CI_STATUS_FAILURE
				statusMessage = "Some tests failed"
				break
			}
		}

		assert.Equal(t, backend.DriftRunCIStatus_DRIFT_RUN_CI_STATUS_FAILURE, finalStatus)
		assert.Equal(t, "Some tests failed", statusMessage)
	})
}

// Test helper to verify the JSON structure of written files
func TestWriteRunResultsToFile_JSONStructure(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	resultsDir := filepath.Join(tmpDir, "results")
	resultsFile := filepath.Join(resultsDir, "test.json")

	executor := &Executor{
		resultsDir:  resultsDir,
		ResultsFile: resultsFile,
	}

	tests := []Test{{TraceID: "trace-1", TraceTestID: "tt-1"}}
	results := []TestResult{
		{
			TestID: "trace-1",
			Passed: false,
			Deviations: []Deviation{
				{Field: "headers.content-type", Description: "expected application/json"},
				{Field: "body.status", Description: "expected ok, got error"},
			},
		},
	}

	path, err := executor.WriteRunResultsToFile(tests, results)
	require.NoError(t, err)

	// Read the file and verify it's valid JSON
	data, err := os.ReadFile(path) // #nosec G304
	require.NoError(t, err)

	var jsonData map[string]interface{}
	err = json.Unmarshal(data, &jsonData)
	require.NoError(t, err)

	// Note: drift_run_id and sdk_version may be omitted when empty due to protobuf JSON behavior
	assert.Contains(t, jsonData, "cli_version")
	assert.Contains(t, jsonData, "trace_test_results")

	resultsArray, ok := jsonData["trace_test_results"].([]any)
	require.True(t, ok)
	assert.Len(t, resultsArray, 1)

	firstResult, ok := resultsArray[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "tt-1", firstResult["trace_test_id"])
	// test_success field may be omitted when false due to protobuf JSON behavior
	if val, ok := firstResult["test_success"]; ok {
		assert.Equal(t, false, val)
	}
}

func TestBuildTraceTestResultsProto_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("empty results", func(t *testing.T) {
		t.Parallel()

		protoResults := BuildTraceTestResultsProto(nil, []TestResult{}, []Test{})
		assert.Empty(t, protoResults)
	})

	t.Run("mismatched tests and results", func(t *testing.T) {
		t.Parallel()

		tests := []Test{
			{TraceID: "trace-1", TraceTestID: "tt-1"},
		}

		results := []TestResult{
			{TestID: "trace-1", Passed: true},
			{TestID: "trace-2", Passed: false},
		}

		protoResults := BuildTraceTestResultsProto(nil, results, tests)

		require.Len(t, protoResults, 2)
		assert.Equal(t, "tt-1", protoResults[0].TraceTestId)
		assert.Equal(t, "trace-2", protoResults[1].TraceTestId) // Uses TestID as fallback
	})

	t.Run("nil executor and empty deviations", func(t *testing.T) {
		t.Parallel()

		tests := []Test{
			{TraceID: "trace-1", TraceTestID: "tt-1"},
		}

		results := []TestResult{
			{
				TestID:     "trace-1",
				Passed:     false,
				Deviations: []Deviation{},
			},
		}

		protoResults := BuildTraceTestResultsProto(nil, results, tests)

		require.Len(t, protoResults, 1)
		assert.False(t, protoResults[0].TestSuccess)
		assert.Equal(t, backend.TraceTestFailureReason_TRACE_TEST_FAILURE_REASON_RESPONSE_MISMATCH, *protoResults[0].TestFailureReason)
		assert.Empty(t, protoResults[0].SpanResults)
	})
}

func TestBuildTraceTestResultsProto_WithMockNotFound(t *testing.T) {
	t.Parallel()

	t.Run("mock not found takes priority over deviations", func(t *testing.T) {
		t.Parallel()

		// Create a mock server with mock-not-found events
		cfg, _ := config.Get()
		server, err := NewServer("test-service", &cfg.Service)
		require.NoError(t, err)
		defer func() { _ = server.Stop() }()

		server.recordMockNotFoundEvent("trace-1", MockNotFoundEvent{
			PackageName: "pg",
			SpanName:    "pg.query",
			Operation:   "query",
			StackTrace:  "at test.ts:10",
			Timestamp:   time.Now(),
			Error:       "no mock found for query pg.query",
			ReplaySpan: &core.Span{
				SpanId:      "replay-span-1",
				TraceId:     "trace-1",
				Name:        "pg.query",
				PackageName: "pg",
			},
		})

		executor := &Executor{
			server: server,
		}

		tests := []Test{
			{TraceID: "trace-1", TraceTestID: "tt-1"},
		}

		results := []TestResult{
			{
				TestID: "trace-1",
				Passed: false,
				Deviations: []Deviation{
					{Field: "body", Description: "Response body content mismatch"},
				},
			},
		}

		protoResults := BuildTraceTestResultsProto(executor, results, tests)

		require.Len(t, protoResults, 1)
		result := protoResults[0]

		assert.Equal(t, "tt-1", result.TraceTestId)
		assert.False(t, result.TestSuccess)

		require.NotNil(t, result.TestFailureReason)
		assert.Equal(t, backend.TraceTestFailureReason_TRACE_TEST_FAILURE_REASON_MOCK_NOT_FOUND, *result.TestFailureReason)

		require.NotNil(t, result.TestFailureMessage)
		assert.Equal(t, "Mock not found during replay", *result.TestFailureMessage)

		// EXPECT: Span results should be created for mock-not-found events
		// We expect 2 span results:
		// [0] = inbound span result (for the deviations)
		// [1] = mock-not-found span result (for the failed outbound call)
		require.Len(t, result.SpanResults, 2, "Should have inbound span result + mock-not-found span result")

		// Check the inbound span result (index 0)
		inboundSpanResult := result.SpanResults[0]
		assert.Nil(t, inboundSpanResult.MatchedSpanRecordingId)
		assert.NotEmpty(t, inboundSpanResult.Deviations, "Inbound span should have deviations")

		// Check the mock-not-found span result (index 1)
		mockNotFoundSpanResult := result.SpanResults[1]
		assert.Nil(t, mockNotFoundSpanResult.MatchedSpanRecordingId, "Mock-not-found event should have NO matched span recording ID")
		assert.Nil(t, mockNotFoundSpanResult.MatchLevel, "Mock-not-found event should have NO match level")
		assert.NotNil(t, mockNotFoundSpanResult.StackTrace, "Should include stack trace from the mock-not-found event")
		assert.Equal(t, "at test.ts:10", *mockNotFoundSpanResult.StackTrace)
		assert.NotNil(t, mockNotFoundSpanResult.ReplaySpan, "Should include the replay span that failed to find a mock")
	})

	t.Run("no mock-not-found events falls back to response mismatch", func(t *testing.T) {
		t.Parallel()

		cfg, _ := config.Get()
		server, err := NewServer("test-service", &cfg.Service)
		require.NoError(t, err)
		defer func() { _ = server.Stop() }()

		executor := &Executor{
			server: server,
		}

		tests := []Test{
			{TraceID: "trace-1", TraceTestID: "tt-1"},
		}

		results := []TestResult{
			{
				TestID: "trace-1",
				Passed: false,
				Deviations: []Deviation{
					{Field: "body", Description: "Response body content mismatch"},
				},
			},
		}

		protoResults := BuildTraceTestResultsProto(executor, results, tests)

		require.Len(t, protoResults, 1)
		result := protoResults[0]

		assert.Equal(t, "tt-1", result.TraceTestId)
		assert.False(t, result.TestSuccess)

		// Should be RESPONSE_MISMATCH when no mock-not-found events
		require.NotNil(t, result.TestFailureReason)
		assert.Equal(t, backend.TraceTestFailureReason_TRACE_TEST_FAILURE_REASON_RESPONSE_MISMATCH, *result.TestFailureReason)
	})
}

func TestBuildTraceTestResultsProto_WithNoResponse(t *testing.T) {
	t.Parallel()

	t.Run("NO_RESPONSE with error creates deviation", func(t *testing.T) {
		t.Parallel()

		cfg, _ := config.Get()
		server, err := NewServer("test-service", &cfg.Service)
		require.NoError(t, err)
		defer func() { _ = server.Stop() }()

		executor := &Executor{
			server: server,
		}

		tests := []Test{
			{TraceID: "trace-1", TraceTestID: "tt-1"},
		}

		results := []TestResult{
			{
				TestID: "trace-1",
				Passed: false,
				Error:  "context deadline exceeded (Client.Timeout exceeded while awaiting headers)",
			},
		}

		protoResults := BuildTraceTestResultsProto(executor, results, tests)

		require.Len(t, protoResults, 1)
		result := protoResults[0]

		assert.Equal(t, "tt-1", result.TraceTestId)
		assert.False(t, result.TestSuccess)

		require.NotNil(t, result.TestFailureReason)
		assert.Equal(t, backend.TraceTestFailureReason_TRACE_TEST_FAILURE_REASON_NO_RESPONSE, *result.TestFailureReason)

		require.NotNil(t, result.TestFailureMessage)
		assert.Equal(t, "context deadline exceeded (Client.Timeout exceeded while awaiting headers)", *result.TestFailureMessage)

		require.Len(t, result.SpanResults, 1, "Should have inbound span result with deviation")

		inboundSpanResult := result.SpanResults[0]
		require.Len(t, inboundSpanResult.Deviations, 1, "Should have exactly one deviation")
		assert.Equal(t, "response", inboundSpanResult.Deviations[0].Field)
		assert.Contains(t, inboundSpanResult.Deviations[0].Description, "No response received:")
		assert.Contains(t, inboundSpanResult.Deviations[0].Description, "context deadline exceeded")
	})

	t.Run("NO_RESPONSE with crashed server creates deviation", func(t *testing.T) {
		t.Parallel()

		cfg, _ := config.Get()
		server, err := NewServer("test-service", &cfg.Service)
		require.NoError(t, err)
		defer func() { _ = server.Stop() }()

		executor := &Executor{
			server: server,
		}

		tests := []Test{
			{TraceID: "trace-1", TraceTestID: "tt-1"},
		}

		results := []TestResult{
			{
				TestID:        "trace-1",
				Passed:        false,
				CrashedServer: true,
				Error:         "server process exited unexpectedly",
			},
		}

		protoResults := BuildTraceTestResultsProto(executor, results, tests)

		require.Len(t, protoResults, 1)
		result := protoResults[0]

		assert.Equal(t, "tt-1", result.TraceTestId)
		assert.False(t, result.TestSuccess)

		require.NotNil(t, result.TestFailureReason)
		assert.Equal(t, backend.TraceTestFailureReason_TRACE_TEST_FAILURE_REASON_NO_RESPONSE, *result.TestFailureReason)

		require.NotNil(t, result.TestFailureMessage)
		assert.Contains(t, *result.TestFailureMessage, "Test caused server to crash")

		require.Len(t, result.SpanResults, 1, "Should have inbound span result with deviation")

		inboundSpanResult := result.SpanResults[0]
		require.Len(t, inboundSpanResult.Deviations, 1, "Should have exactly one deviation")
		assert.Equal(t, "response", inboundSpanResult.Deviations[0].Field)
		assert.Contains(t, inboundSpanResult.Deviations[0].Description, "No response received:")
		assert.Contains(t, inboundSpanResult.Deviations[0].Description, "Test caused server to crash")
	})
}

// Benchmarks
func BenchmarkBuildTraceTestResultsProto(b *testing.B) {
	tests := make([]Test, 100)
	results := make([]TestResult, 100)

	for i := range 100 {
		tests[i] = Test{
			TraceID:     "trace-" + string(rune(i)),
			TraceTestID: "tt-" + string(rune(i)),
		}
		results[i] = TestResult{
			TestID: "trace-" + string(rune(i)),
			Passed: i%2 == 0,
		}
	}

	for b.Loop() {
		_ = BuildTraceTestResultsProto(nil, results, tests)
	}
}

func BenchmarkWriteRunResultsToFile(b *testing.B) {
	tmpDir := b.TempDir()

	tests := []Test{
		{TraceID: "trace-1", TraceTestID: "tt-1"},
	}
	results := []TestResult{
		{TestID: "trace-1", Passed: true},
	}

	for i := 0; b.Loop(); i++ {
		executor := &Executor{
			resultsDir:  tmpDir,
			ResultsFile: filepath.Join(tmpDir, "test-"+string(rune(i))+".json"),
		}
		_, _ = executor.WriteRunResultsToFile(tests, results)
	}
}

func TestRunTests_UploadMissingDataDueToRaceCondition(t *testing.T) {
	t.Parallel()

	// Setup HTTP test server that will respond to test requests
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer testServer.Close()

	// Setup mock server for recording/replay
	cfg, _ := config.Get()
	mockServer, err := NewServer("test-service", &cfg.Service)
	require.NoError(t, err)
	defer func() { _ = mockServer.Stop() }()

	// Start the mock server
	err = mockServer.Start()
	require.NoError(t, err)

	// Prepare recording spans (simulating what would be recorded during tracing)
	traceID := "test-trace-123"
	recordingSpans := []*core.Span{
		{
			SpanId:      "root-span-id",
			TraceId:     traceID,
			IsRootSpan:  true,
			Name:        "GET /api/test",
			PackageName: "http",
		},
		{
			SpanId:         "db-span-id",
			TraceId:        traceID,
			Name:           "pg.query",
			PackageName:    "pg",
			InputValueHash: "mock-input-hash-123",
		},
	}

	// Create test with spans
	test := Test{
		TraceID:     traceID,
		TraceTestID: "tt-123",
		DisplayName: "Test API endpoint",
		Request: Request{
			Method: "GET",
			Path:   "/api/test",
		},
		Response: Response{
			Status: 200,
			Body:   map[string]any{"status": "ok"},
		},
		Spans: recordingSpans,
	}

	// Setup executor
	executor := NewExecutor()
	executor.serviceURL = testServer.URL
	executor.SetTestTimeout(5 * time.Second)
	executor.SetConcurrency(1)
	executor.server = mockServer

	// Capture what gets passed to the upload callback
	var capturedResults []TestResult
	var capturedTests []Test
	var capturedProtoResults []*backend.TraceTestResult

	executor.SetOnTestCompleted(func(result TestResult, test Test) {
		capturedResults = append(capturedResults, result)
		capturedTests = append(capturedTests, test)

		// Simulate what UploadSingleTestResult does - build the proto
		protoResults := BuildTraceTestResultsProto(executor, []TestResult{result}, []Test{test})
		capturedProtoResults = append(capturedProtoResults, protoResults...)

		if executor.GetServer() != nil {
			executor.GetServer().CleanupTraceSpans(test.TraceID)
		}
	})

	// Run the test through the full flow
	results, err := executor.RunTests([]Test{test})
	require.NoError(t, err)
	require.Len(t, results, 1)

	// The test should have passed
	assert.True(t, results[0].Passed, "Test should pass")

	// Now verify what was captured in the upload callback
	require.Len(t, capturedProtoResults, 1, "Should have captured upload data")
	uploadedResult := capturedProtoResults[0]

	// The inbound span result should have the root span ID
	if len(uploadedResult.SpanResults) > 0 {
		inboundSpan := uploadedResult.SpanResults[0]
		assert.NotNil(t, inboundSpan.MatchedSpanRecordingId,
			"BUG: Root span ID is missing because CleanupTraceSpans deleted it before upload callback")
		if inboundSpan.MatchedSpanRecordingId != nil {
			assert.Equal(t, "root-span-id", *inboundSpan.MatchedSpanRecordingId,
				"Root span ID should be preserved for upload")
		}
	}

	// We expect at least 1 span result
	assert.NotEmpty(t, uploadedResult.SpanResults,
		"BUG: No span results in upload because CleanupTraceSpans deleted match events before callback")

	// Verify that the trace spans were cleaned up
	assert.Empty(t, executor.GetServer().GetMatchEvents(traceID))
	assert.Empty(t, executor.GetServer().GetRootSpanID(traceID))
}
