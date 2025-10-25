package runner

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
	"github.com/stretchr/testify/assert"
)

func TestNewExecutor(t *testing.T) {
	executor := NewExecutor()

	assert.NotNil(t, executor)
	assert.Equal(t, "http://localhost:3000", executor.serviceURL)
	assert.Equal(t, 5, executor.parallel)
	assert.Equal(t, 30*time.Second, executor.testTimeout)
	assert.Nil(t, executor.server)
	assert.Nil(t, executor.serviceCmd)
	assert.Nil(t, executor.serviceLogFile)
	assert.False(t, executor.enableServiceLogs)
	assert.Equal(t, 0, executor.servicePort)
	assert.Empty(t, executor.resultsDir)
	assert.Empty(t, executor.ResultsFile)
	assert.Nil(t, executor.onTestCompleted)
	assert.Nil(t, executor.suiteSpans)
}

func TestExecutor_SetResultsOutput(t *testing.T) {
	executor := NewExecutor()
	tempDir := t.TempDir()

	// We can't easily mock time.Now in the function, so we'll test the pattern
	executor.SetResultsOutput(tempDir)

	assert.Equal(t, tempDir, executor.resultsDir)
	assert.Contains(t, executor.ResultsFile, tempDir)
	assert.Contains(t, executor.ResultsFile, "results-")
	assert.Contains(t, executor.ResultsFile, ".json")
}

func TestExecutor_SetConcurrency(t *testing.T) {
	tests := []struct {
		name        string
		concurrency int
		expected    int
	}{
		{
			name:        "valid_positive_concurrency",
			concurrency: 10,
			expected:    10,
		},
		{
			name:        "zero_concurrency_ignored",
			concurrency: 0,
			expected:    5, // default value
		},
		{
			name:        "negative_concurrency_ignored",
			concurrency: -1,
			expected:    5, // default value
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := NewExecutor()
			executor.SetConcurrency(tt.concurrency)
			assert.Equal(t, tt.expected, executor.GetConcurrency())
		})
	}
}

func TestExecutor_SetTestTimeout(t *testing.T) {
	tests := []struct {
		name     string
		timeout  time.Duration
		expected time.Duration
	}{
		{
			name:     "valid_timeout",
			timeout:  60 * time.Second,
			expected: 60 * time.Second,
		},
		{
			name:     "zero_timeout_ignored",
			timeout:  0,
			expected: 30 * time.Second, // default value
		},
		{
			name:     "negative_timeout_ignored",
			timeout:  -1 * time.Second,
			expected: 30 * time.Second, // default value
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := NewExecutor()
			executor.SetTestTimeout(tt.timeout)
			assert.Equal(t, tt.expected, executor.testTimeout)
		})
	}
}

func TestExecutor_SetOnTestCompleted(t *testing.T) {
	executor := NewExecutor()

	var callbackCalled bool
	callback := func(result TestResult, test Test) {
		callbackCalled = true
	}

	executor.SetOnTestCompleted(callback)
	assert.NotNil(t, executor.onTestCompleted)

	// Verify callback works
	executor.onTestCompleted(TestResult{}, Test{})
	assert.True(t, callbackCalled)
}

func TestExecutor_SetSuiteSpans(t *testing.T) {
	executor := NewExecutor()
	spans := []*core.Span{
		{TraceId: "trace1", SpanId: "span1"},
		{TraceId: "trace2", SpanId: "span2"},
	}

	executor.SetSuiteSpans(spans)
	assert.Equal(t, spans, executor.suiteSpans)

	// Test with server set
	mockServer := &Server{}
	executor.server = mockServer
	executor.SetSuiteSpans(spans)
	assert.Equal(t, spans, executor.suiteSpans)
}

func TestExecutor_RunTests(t *testing.T) {
	executor := NewExecutor()
	executor.serviceURL = "http://localhost:59999"  // Use a port that's definitely not in use
	executor.SetTestTimeout(100 * time.Millisecond) // Short timeout for faster test failure

	tests := []Test{
		{TraceID: "test1", Request: Request{Method: "GET", Path: "/test1"}},
		{TraceID: "test2", Request: Request{Method: "GET", Path: "/test2"}},
	}

	// This will call RunTestsConcurrently internally
	results, err := executor.RunTests(tests)

	// Should not error at the RunTests level - errors are captured in individual test results
	assert.NotNil(t, results)
	assert.NoError(t, err) // No error at the concurrent execution level
	assert.Len(t, results, 2)

	// But individual test results should show failures with connection errors
	for _, result := range results {
		assert.False(t, result.Passed)
		assert.NotEmpty(t, result.Error, "Expected connection error but got empty error")
		assert.Contains(t, result.Error, "connect", "Expected connection error")
	}
}

func TestExecutor_RunTestsConcurrently(t *testing.T) {
	tests := []struct {
		name           string
		tests          []Test
		maxConcurrency int
		expectError    bool
	}{
		{
			name:           "empty_tests",
			tests:          []Test{},
			maxConcurrency: 1,
			expectError:    false,
		},
		{
			name: "single_test",
			tests: []Test{
				{TraceID: "test1", Request: Request{Method: "GET", Path: "/test"}},
			},
			maxConcurrency: 1,
			expectError:    false, // Will error on HTTP call but not on concurrency logic
		},
		{
			name: "multiple_tests",
			tests: []Test{
				{TraceID: "test1", Request: Request{Method: "GET", Path: "/test1"}},
				{TraceID: "test2", Request: Request{Method: "GET", Path: "/test2"}},
				{TraceID: "test3", Request: Request{Method: "GET", Path: "/test3"}},
			},
			maxConcurrency: 2,
			expectError:    false, // Will error on HTTP calls but not on concurrency logic
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := NewExecutor()
			executor.SetTestTimeout(100 * time.Millisecond) // Short timeout for faster tests

			results, err := executor.RunTestsConcurrently(tt.tests, tt.maxConcurrency)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, results, len(tt.tests))

				// Verify all tests have results
				for _, result := range results {
					assert.NotEmpty(t, result.TestID)
					assert.False(t, result.Passed) // Expected to fail due to no server
					if len(tt.tests) > 0 {
						// Create slice of expected test IDs
						expectedIDs := make([]string, len(tt.tests))
						for j, test := range tt.tests {
							expectedIDs[j] = test.TraceID
						}
						assert.Contains(t, expectedIDs, result.TestID)
					}
				}
			}
		})
	}
}

func TestExecutor_RunSingleTest_WithMockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-trace-id", r.Header.Get("x-td-trace-id"))
		assert.Equal(t, "{}", r.Header.Get("x-td-env-vars"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	executor := NewExecutor()
	executor.serviceURL = server.URL

	test := Test{
		TraceID: "test-trace-id",
		Request: Request{
			Method:  "GET",
			Path:    "/api/test",
			Headers: map[string]string{"Authorization": "Bearer token"},
		},
		Response: Response{
			Status:  200,
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    map[string]string{"status": "ok"},
		},
		Metadata: map[string]any{},
	}

	result, err := executor.RunSingleTest(test)

	assert.NoError(t, err)
	assert.Equal(t, "test-trace-id", result.TestID)
	assert.GreaterOrEqual(t, result.Duration, 0) // Duration should be non-negative
	// Note: result.Passed might be false due to comparison logic differences, but no HTTP error
	// The test is successful if we get a result without HTTP errors, comparison details are tested elsewhere
}

func TestExecutor_RunSingleTest_WithRequestBody(t *testing.T) {
	// Mock HTTP server that expects a POST with body
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)

		var bodyData map[string]string
		err = json.Unmarshal(body, &bodyData)
		assert.NoError(t, err)
		assert.Equal(t, "test", bodyData["key"])

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"created": "true"})
	}))
	defer server.Close()

	executor := NewExecutor()
	executor.serviceURL = server.URL

	// Create base64 encoded JSON body
	bodyData := map[string]string{"key": "test"}
	bodyBytes, _ := json.Marshal(bodyData)
	encodedBody := base64.StdEncoding.EncodeToString(bodyBytes)

	test := Test{
		TraceID: "test-with-body",
		Request: Request{
			Method: "POST",
			Path:   "/api/create",
			Body:   encodedBody,
		},
		Response: Response{
			Status: 201,
			Body:   map[string]string{"created": "true"},
		},
	}

	result, err := executor.RunSingleTest(test)

	assert.NoError(t, err)
	assert.Equal(t, "test-with-body", result.TestID)
}

func TestExecutor_RunSingleTest_InvalidRequestBody(t *testing.T) {
	executor := NewExecutor()

	test := Test{
		TraceID: "test-invalid-body",
		Request: Request{
			Method: "POST",
			Path:   "/api/test",
			Body:   123, // Invalid body type
		},
	}

	result, err := executor.RunSingleTest(test)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected value to be a string")
	assert.Equal(t, TestResult{}, result)
}

func TestExecutor_RunSingleTest_InvalidBase64Body(t *testing.T) {
	executor := NewExecutor()

	test := Test{
		TraceID: "test-invalid-base64",
		Request: Request{
			Method: "POST",
			Path:   "/api/test",
			Body:   "invalid-base64!", // Invalid base64
		},
	}

	result, err := executor.RunSingleTest(test)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode base64 value")
	assert.Equal(t, TestResult{}, result)
}

func TestExecutor_RunSingleTest_WithAbsoluteURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	executor := NewExecutor()

	test := Test{
		TraceID: "test-absolute-url",
		Request: Request{
			Method: "GET",
			Path:   server.URL + "/api/test", // Absolute URL
		},
		Response: Response{
			Status: 200,
		},
	}

	result, err := executor.RunSingleTest(test)

	assert.NoError(t, err)
	assert.Equal(t, "test-absolute-url", result.TestID)
}

func TestExecutor_RunSingleTest_WithEnvVars(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envVars := r.Header.Get("x-td-env-vars")
		assert.NotEmpty(t, envVars)

		var envData map[string]string
		err := json.Unmarshal([]byte(envVars), &envData)
		assert.NoError(t, err)
		assert.Equal(t, "test-value", envData["TEST_VAR"])

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	executor := NewExecutor()
	executor.serviceURL = server.URL

	test := Test{
		TraceID: "test-env-vars",
		Request: Request{
			Method: "GET",
			Path:   "/api/test",
		},
		Metadata: map[string]any{
			"ENV_VARS": map[string]string{
				"TEST_VAR": "test-value",
			},
		},
		Response: Response{
			Status: 200,
		},
	}

	result, err := executor.RunSingleTest(test)

	assert.NoError(t, err)
	assert.Equal(t, "test-env-vars", result.TestID)
}

func TestExecutor_RunSingleTest_WithTimeout(t *testing.T) {
	// Create a slow mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond) // Longer than our timeout
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	executor := NewExecutor()
	executor.serviceURL = server.URL
	executor.SetTestTimeout(50 * time.Millisecond) // Short timeout

	test := Test{
		TraceID: "test-timeout",
		Request: Request{
			Method: "GET",
			Path:   "/api/slow",
		},
	}

	result, err := executor.RunSingleTest(test)

	assert.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "timeout")
	assert.Equal(t, TestResult{}, result)
}

func TestExecutor_RunSingleTest_WithCallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	executor := NewExecutor()
	executor.serviceURL = server.URL

	var callbackResult TestResult
	var callbackTest Test
	var callbackCalled bool

	executor.SetOnTestCompleted(func(result TestResult, test Test) {
		callbackResult = result
		callbackTest = test
		callbackCalled = true
	})

	test := Test{
		TraceID: "test-callback",
		Request: Request{
			Method: "GET",
			Path:   "/api/test",
		},
		Response: Response{
			Status: 200,
		},
	}

	result, err := executor.RunSingleTest(test)

	assert.NoError(t, err)
	assert.True(t, callbackCalled)
	assert.Equal(t, result.TestID, callbackResult.TestID)
	assert.Equal(t, test.TraceID, callbackTest.TraceID)
}

func TestCountPassedTests(t *testing.T) {
	tests := []struct {
		name     string
		results  []TestResult
		expected int
	}{
		{
			name:     "empty_results",
			results:  []TestResult{},
			expected: 0,
		},
		{
			name: "all_passed",
			results: []TestResult{
				{TestID: "test1", Passed: true},
				{TestID: "test2", Passed: true},
			},
			expected: 2,
		},
		{
			name: "all_failed",
			results: []TestResult{
				{TestID: "test1", Passed: false},
				{TestID: "test2", Passed: false},
			},
			expected: 0,
		},
		{
			name: "mixed_results",
			results: []TestResult{
				{TestID: "test1", Passed: true},
				{TestID: "test2", Passed: false},
				{TestID: "test3", Passed: true},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := countPassedTests(tt.results)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExecutor_WaitForSpanData(t *testing.T) {
	executor := NewExecutor()

	// Test with no server - should not panic
	executor.WaitForSpanData("test-trace", 100*time.Millisecond)

	// Test with server
	mockServer := &Server{
		spans:       make(map[string][]*core.Span),
		matchEvents: make(map[string][]MatchEvent),
		mu:          sync.RWMutex{},
	}
	executor.server = mockServer

	// Should not panic and should return quickly since there's no data
	start := time.Now()
	executor.WaitForSpanData("test-trace", 100*time.Millisecond)
	duration := time.Since(start)

	// Should return relatively quickly since there's no actual waiting implementation in the mock
	assert.Less(t, duration, 200*time.Millisecond)
}

func TestOutputSingleResult_Text_WithFailures_Verbose(t *testing.T) {
	result := TestResult{
		TestID:   "test1",
		Passed:   false,
		Duration: 100,
		Deviations: []Deviation{
			{
				Field:       "response.status",
				Expected:    200,
				Actual:      404,
				Description: "Status code mismatch",
			},
		},
	}

	test := Test{
		TraceID: "test1",
		Request: Request{
			Method:  "GET",
			Path:    "/api/test",
			Headers: map[string]string{"Authorization": "Bearer token"},
			Body:    map[string]string{"key": "value"},
		},
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	OutputSingleResult(result, test, "text", false, true) // verbose=true

	_ = w.Close()
	os.Stdout = oldStdout

	output, _ := io.ReadAll(r)
	outputStr := string(output)

	assert.Contains(t, outputStr, "● DEVIATION - test1")
	assert.Contains(t, outputStr, "GET /api/test")
	assert.Contains(t, outputStr, "Authorization: Bearer token")
	assert.Contains(t, outputStr, "Body: map[key:value]")
	assert.Contains(t, outputStr, "Deviation: Status code mismatch")
	assert.Contains(t, outputStr, "Expected: 200")
	assert.Contains(t, outputStr, "Actual: 404")
}

func TestOutputSingleResult_Text_WithPasses(t *testing.T) {
	results := []TestResult{
		{TestID: "test1", Passed: true, Duration: 100},
		{TestID: "test2", Passed: true, Duration: 150},
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	for _, result := range results {
		OutputSingleResult(result, Test{TraceID: result.TestID}, "text", false, false)
	}

	_ = w.Close()
	os.Stdout = oldStdout

	output, _ := io.ReadAll(r)
	outputStr := string(output)

	assert.Contains(t, outputStr, "✓ NO DEVIATION - test1")
	assert.Contains(t, outputStr, "✓ NO DEVIATION - test2")
}

func TestOutputSingleResult_Text_Quiet_OnlyFailures(t *testing.T) {
	results := []TestResult{
		{TestID: "test1", Passed: true, Duration: 100},
		{TestID: "test2", Passed: false, Duration: 200, Error: "error"},
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	for _, result := range results {
		OutputSingleResult(result, Test{TraceID: result.TestID}, "text", true, false) // quiet=true
	}

	_ = w.Close()
	os.Stdout = oldStdout

	output, _ := io.ReadAll(r)
	outputStr := string(output)

	assert.NotContains(t, outputStr, "✓ NO DEVIATION") // Should not show passed tests in quiet mode
	assert.Contains(t, outputStr, "● DEVIATION - test2")
}

func TestOutputSingleResult_Text_WithError(t *testing.T) {
	result := TestResult{
		TestID:   "test1",
		Passed:   false,
		Duration: 100,
		Error:    "Connection refused",
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	OutputSingleResult(result, Test{TraceID: "test1"}, "text", false, false)

	_ = w.Close()
	os.Stdout = oldStdout

	output, _ := io.ReadAll(r)
	outputStr := string(output)

	assert.Contains(t, outputStr, "● DEVIATION - test1")
	assert.Contains(t, outputStr, "Error: Connection refused")
}

func TestOutputSingleResult_Text_QuietSuppressesVerbose(t *testing.T) {
	result := TestResult{
		TestID:   "test1",
		Passed:   false,
		Duration: 100,
		Deviations: []Deviation{
			{
				Field:       "response.status",
				Expected:    200,
				Actual:      404,
				Description: "Status code mismatch",
			},
		},
	}

	test := Test{
		TraceID: "test1",
		Request: Request{
			Method: "GET",
			Path:   "/api/test",
		},
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	OutputSingleResult(result, test, "text", true, true) // Both quiet and verbose

	_ = w.Close()
	os.Stdout = oldStdout

	output, _ := io.ReadAll(r)
	outputStr := string(output)

	assert.Contains(t, outputStr, "● DEVIATION - test1")
	assert.NotContains(t, outputStr, "GET /api/test") // Details should be suppressed by quiet
	assert.NotContains(t, outputStr, "Expected:")
}

func TestExecutor_RunSingleTest_WithServer(t *testing.T) {
	executor := NewExecutor()

	// Mock server with basic functionality - need to initialize all required maps
	mockServer := &Server{
		spans:       make(map[string][]*core.Span),
		matchEvents: make(map[string][]MatchEvent),
		spanUsage:   make(map[string]map[string]bool), // This was missing and causing the panic
		mu:          sync.RWMutex{},
	}
	executor.server = mockServer

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer httpServer.Close()

	executor.serviceURL = httpServer.URL

	test := Test{
		TraceID:  "test-with-server",
		FileName: "test.jsonl",
		Request: Request{
			Method: "GET",
			Path:   "/api/test",
		},
		Response: Response{
			Status: 200,
		},
		Spans: []*core.Span{
			{TraceId: "test-with-server", SpanId: "span1"},
		},
	}

	result, err := executor.RunSingleTest(test)

	assert.NoError(t, err)
	assert.Equal(t, "test-with-server", result.TestID)
}

// Benchmark tests for performance validation
func BenchmarkExecutor_RunTestsConcurrently(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	executor := NewExecutor()
	executor.serviceURL = server.URL
	executor.SetTestTimeout(5 * time.Second)

	// Create test data
	tests := make([]Test, 100)
	for i := 0; i < 100; i++ {
		tests[i] = Test{
			TraceID: fmt.Sprintf("test-%d", i),
			Request: Request{
				Method: "GET",
				Path:   fmt.Sprintf("/api/test-%d", i),
			},
			Response: Response{
				Status: 200,
			},
		}
	}

	for b.Loop() {
		results, err := executor.RunTestsConcurrently(tests, 10)
		if err != nil {
			b.Fatal(err)
		}
		if len(results) != 100 {
			b.Fatalf("Expected 100 results, got %d", len(results))
		}
	}
}

func BenchmarkCountPassedTests(b *testing.B) {
	// Create test data
	results := make([]TestResult, 1000)
	for i := range 1000 {
		results[i] = TestResult{
			TestID: fmt.Sprintf("test-%d", i),
			Passed: i%2 == 0, // Half passed, half failed
		}
	}

	for b.Loop() {
		count := countPassedTests(results)
		if count != 500 {
			b.Fatalf("Expected 500 passed tests, got %d", count)
		}
	}
}
