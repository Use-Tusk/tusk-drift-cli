package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeFileName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"trace-abc123", "trace-abc123"},
		{"trace/abc:123", "trace_abc_123"},
		{"trace\\abc 123", "trace_abc_123"},
		{"simple", "simple"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, sanitizeFileName(tt.input))
	}
}

func TestDetermineFailureType(t *testing.T) {
	t.Run("CrashedServer", func(t *testing.T) {
		result := TestResult{CrashedServer: true}
		assert.Equal(t, "NO_RESPONSE", determineFailureType(result, nil))
	})

	t.Run("ErrorNoServer", func(t *testing.T) {
		result := TestResult{Error: "connection refused"}
		assert.Equal(t, "NO_RESPONSE", determineFailureType(result, nil))
	})

	t.Run("ResponseMismatch", func(t *testing.T) {
		result := TestResult{
			Deviations: []Deviation{{Field: "response.status", Expected: 200, Actual: 201}},
		}
		assert.Equal(t, "RESPONSE_MISMATCH", determineFailureType(result, nil))
	})
}

func TestBuildFrontmatter_ResponseMismatch(t *testing.T) {
	test := Test{
		Method: "POST",
		Path:   "/api/v1/users",
		Response: Response{
			Status: 200,
		},
	}
	result := TestResult{
		TestID:   "trace-abc123",
		Duration: 245,
		Deviations: []Deviation{
			{Field: "response.status", Expected: float64(200), Actual: float64(201)},
		},
	}

	fm := buildFrontmatter(test, result, nil, "RESPONSE_MISMATCH")

	assert.Contains(t, fm, "deviation_id: trace-abc123")
	assert.Contains(t, fm, "endpoint: POST /api/v1/users")
	assert.Contains(t, fm, "method: POST")
	assert.Contains(t, fm, "path: /api/v1/users")
	assert.Contains(t, fm, "failure_type: RESPONSE_MISMATCH")
	assert.Contains(t, fm, "status_expected: 200")
	assert.Contains(t, fm, "status_actual: 201")
	assert.Contains(t, fm, "has_mock_not_found: false")
	assert.Contains(t, fm, "duration_ms: 245")
}

func TestBuildFrontmatter_ServerCrash(t *testing.T) {
	test := Test{
		Method:   "GET",
		Path:     "/api/health",
		Response: Response{Status: 200},
	}
	result := TestResult{
		TestID:        "trace-crash",
		CrashedServer: true,
		Duration:      50,
	}

	fm := buildFrontmatter(test, result, nil, "NO_RESPONSE")

	assert.Contains(t, fm, "failure_type: NO_RESPONSE")
	assert.Contains(t, fm, "status_expected: 200")
	assert.Contains(t, fm, "status_actual: 200")
}

func TestBuildDeviationBody_StatusAndBodyDiff(t *testing.T) {
	test := Test{
		Method: "POST",
		Path:   "/api/v1/users",
		Request: Request{
			Method:  "POST",
			Path:    "/api/v1/users",
			Headers: map[string]string{"Content-Type": "application/json", "Authorization": "Bearer secret"},
			Body:    map[string]any{"name": "test user"},
		},
		Response: Response{Status: 200},
	}
	result := TestResult{
		TestID: "trace-abc",
		Deviations: []Deviation{
			{Field: "response.status", Expected: float64(200), Actual: float64(201), Description: "HTTP status code mismatch"},
			{Field: "response.body", Expected: map[string]any{"status": "active"}, Actual: map[string]any{"status": "pending"}, Description: "Response body content mismatch"},
		},
	}

	body := buildDeviationBody(test, result, nil)

	assert.Contains(t, body, "## Request")
	assert.Contains(t, body, "POST /api/v1/users")
	assert.Contains(t, body, "Authorization: ***")
	assert.Contains(t, body, "## Response Diff")
	assert.Contains(t, body, "Status: 200 -> 201 (CHANGED)")
	assert.Contains(t, body, "```diff")
}

func TestBuildDeviationBody_EmptyBody(t *testing.T) {
	test := Test{
		Request: Request{
			Method: "GET",
			Path:   "/api/health",
			Body:   nil,
		},
		Response: Response{Status: 200},
	}
	result := TestResult{
		Deviations: []Deviation{
			{Field: "response.status", Expected: float64(200), Actual: float64(500)},
		},
	}

	body := buildDeviationBody(test, result, nil)

	assert.Contains(t, body, "(empty)")
}

func TestBuildDeviationBody_ServerCrash(t *testing.T) {
	test := Test{
		Request: Request{Method: "POST", Path: "/api/users"},
	}
	result := TestResult{
		CrashedServer: true,
		Error:         "segfault",
	}

	body := buildDeviationBody(test, result, nil)

	assert.Contains(t, body, "## Error")
	assert.Contains(t, body, "Server crashed during test execution.")
	assert.Contains(t, body, "segfault")
	assert.NotContains(t, body, "## Response Diff")
}

func TestBuildDeviationBody_NoServer(t *testing.T) {
	test := Test{
		Request:  Request{Method: "GET", Path: "/api/health"},
		Response: Response{Status: 200},
	}
	result := TestResult{
		Deviations: []Deviation{
			{Field: "response.status", Expected: float64(200), Actual: float64(500)},
		},
	}

	body := buildDeviationBody(test, result, nil)

	assert.NotContains(t, body, "## Outbound Call Context")
	assert.NotContains(t, body, "## Mock Not Found Events")
}

func TestBuildDeviationBody_RetriedAfterCrash(t *testing.T) {
	test := Test{
		Request:  Request{Method: "GET", Path: "/api/health"},
		Response: Response{Status: 200},
	}
	result := TestResult{
		RetriedAfterCrash: true,
		Deviations: []Deviation{
			{Field: "response.status", Expected: float64(200), Actual: float64(500)},
		},
	}

	body := buildDeviationBody(test, result, nil)

	assert.Contains(t, body, "> Note: This test was retried after a server crash")
}

func TestBuildDeviationBody_LargeBody(t *testing.T) {
	// Create a body larger than 100KB
	largeMap := make(map[string]string)
	for i := 0; i < 5000; i++ {
		largeMap[strings.Repeat("k", 20)+fmt.Sprintf("%d", i)] = strings.Repeat("v", 20)
	}

	test := Test{
		Request:  Request{Method: "GET", Path: "/api/data"},
		Response: Response{Status: 200},
	}
	result := TestResult{
		Deviations: []Deviation{
			{Field: "response.body", Expected: largeMap, Actual: largeMap},
		},
	}

	body := buildDeviationBody(test, result, nil)

	assert.Contains(t, body, "Body diff too large to display")
	assert.Contains(t, body, "### Expected (truncated)")
	assert.Contains(t, body, "### Actual (truncated)")
}

func TestMatchLevelToStrings(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		q, s := matchLevelToStrings(nil)
		assert.Equal(t, "UNKNOWN", q)
		assert.Equal(t, "UNKNOWN", s)
	})

	t.Run("value hash trace", func(t *testing.T) {
		ml := &core.MatchLevel{
			MatchType:  core.MatchType_MATCH_TYPE_INPUT_VALUE_HASH,
			MatchScope: core.MatchScope_MATCH_SCOPE_TRACE,
		}
		q, s := matchLevelToStrings(ml)
		assert.Equal(t, "INPUT_VALUE_HASH", q)
		assert.Equal(t, "TRACE", s)
	})

	t.Run("schema hash global", func(t *testing.T) {
		ml := &core.MatchLevel{
			MatchType:  core.MatchType_MATCH_TYPE_INPUT_SCHEMA_HASH,
			MatchScope: core.MatchScope_MATCH_SCOPE_GLOBAL,
		}
		q, s := matchLevelToStrings(ml)
		assert.Equal(t, "INPUT_SCHEMA_HASH", q)
		assert.Equal(t, "GLOBAL", s)
	})
}

func TestMockNotFoundOperationName(t *testing.T) {
	assert.Equal(t, "pg: SELECT * FROM users",
		mockNotFoundOperationName(MockNotFoundEvent{PackageName: "pg", SpanName: "SELECT * FROM users"}))

	assert.Equal(t, "http: GET",
		mockNotFoundOperationName(MockNotFoundEvent{PackageName: "http", Operation: "GET"}))

	assert.Equal(t, "unknown",
		mockNotFoundOperationName(MockNotFoundEvent{}))
}

func TestNewAgentWriter_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	w, err := NewAgentWriter(tmpDir)
	require.NoError(t, err)
	require.NotEmpty(t, w.OutputDir())

	// Directory should exist
	info, err := os.Stat(w.OutputDir())
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// Should contain "agent-run-" in the path
	assert.Contains(t, filepath.Base(w.OutputDir()), "agent-run-")
}

func TestNewAgentWriter_ConcurrentDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	w1, err := NewAgentWriter(tmpDir)
	require.NoError(t, err)

	// Create another writer with same timestamp (simulate concurrent)
	// Force same directory name by creating it
	w2, err := NewAgentWriter(tmpDir)
	require.NoError(t, err)

	// Both should exist and be different
	assert.NotEqual(t, w1.OutputDir(), w2.OutputDir())
}

func TestWriteDeviation(t *testing.T) {
	tmpDir := t.TempDir()
	w, err := NewAgentWriter(tmpDir)
	require.NoError(t, err)

	test := Test{
		Method: "POST",
		Path:   "/api/users",
		Request: Request{
			Method:  "POST",
			Path:    "/api/users",
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    map[string]any{"name": "test"},
		},
		Response: Response{Status: 200},
	}
	result := TestResult{
		TestID:   "trace-abc123",
		Duration: 100,
		Deviations: []Deviation{
			{Field: "response.status", Expected: float64(200), Actual: float64(201), Description: "status mismatch"},
		},
	}

	err = w.WriteDeviation(test, result, nil)
	require.NoError(t, err)

	// File should exist
	filePath := filepath.Join(w.OutputDir(), "deviation-trace-abc123.md")
	content, err := os.ReadFile(filePath)
	require.NoError(t, err)

	s := string(content)
	assert.Contains(t, s, "---")
	assert.Contains(t, s, "deviation_id: trace-abc123")
	assert.Contains(t, s, "## Request")
	assert.Contains(t, s, "## Response Diff")
}

func TestWriteIndex_MixedResults(t *testing.T) {
	tmpDir := t.TempDir()
	w, err := NewAgentWriter(tmpDir)
	require.NoError(t, err)

	// Write a deviation
	test := Test{
		Method:   "POST",
		Path:     "/api/users",
		Request:  Request{Method: "POST", Path: "/api/users"},
		Response: Response{Status: 200},
	}
	result := TestResult{
		TestID:   "trace-fail",
		Duration: 100,
		Deviations: []Deviation{
			{Field: "response.status", Expected: float64(200), Actual: float64(500)},
		},
	}
	err = w.WriteDeviation(test, result, nil)
	require.NoError(t, err)

	// Record a passed test
	passedTest := Test{
		TraceID: "trace-pass",
		Method:  "GET",
		Path:    "/api/health",
	}
	w.RecordPassedTest(passedTest)

	// Write index
	err = w.WriteIndex(2, 1)
	require.NoError(t, err)

	// Read and verify
	content, err := os.ReadFile(filepath.Join(w.OutputDir(), "index.md"))
	require.NoError(t, err)

	s := string(content)
	assert.Contains(t, s, "Tests: 2 total, 1 passed, 1 failed")
	assert.Contains(t, s, "## Deviations")
	assert.Contains(t, s, "trace-fail")
	assert.Contains(t, s, "RESPONSE_MISMATCH")
	assert.Contains(t, s, "## Passing Tests")
	assert.Contains(t, s, "trace-pass")
}

func TestWriteIndex_AllPass(t *testing.T) {
	tmpDir := t.TempDir()
	w, err := NewAgentWriter(tmpDir)
	require.NoError(t, err)

	w.RecordPassedTest(Test{TraceID: "t1", Method: "GET", Path: "/health"})
	w.RecordPassedTest(Test{TraceID: "t2", Method: "GET", Path: "/status"})

	err = w.WriteIndex(2, 2)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(w.OutputDir(), "index.md"))
	require.NoError(t, err)

	s := string(content)
	assert.Contains(t, s, "Tests: 2 total, 2 passed, 0 failed")
	assert.NotContains(t, s, "## Deviations")
	assert.Contains(t, s, "## Passing Tests")
}

func TestAnyToInt(t *testing.T) {
	assert.Equal(t, 200, anyToInt(200, 0))
	assert.Equal(t, 201, anyToInt(float64(201), 0))
	assert.Equal(t, 500, anyToInt(int64(500), 0))
	assert.Equal(t, 42, anyToInt("not a number", 42))
	assert.Equal(t, 0, anyToInt(nil, 0))
}

func TestFormatBodyForAgent(t *testing.T) {
	assert.Equal(t, "(empty)", formatBodyForAgent(nil))
	assert.Equal(t, "(empty)", formatBodyForAgent(""))
	assert.Contains(t, formatBodyForAgent(map[string]any{"key": "value"}), "key")
	assert.Contains(t, formatBodyForAgent(`{"json":"string"}`), "json")
}
