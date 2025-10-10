package runner

import (
	"encoding/base64"
	"testing"
	"time"

	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestConvertTraceTestToRunnerTest_GraphQLDisplaySpan(t *testing.T) {
	t.Parallel()

	ts := time.Date(2024, time.June, 1, 12, 34, 56, 0, time.UTC)
	encodedBody := base64.StdEncoding.EncodeToString([]byte(`{"status":"ok"}`))

	serverSpan := &core.Span{
		Kind:        core.SpanKind_SPAN_KIND_SERVER,
		PackageName: "http",
		InputValue: makeStruct(t, map[string]any{
			"method": "POST",
			"target": "/graphql?op=GetUser",
			"headers": map[string]any{
				"Content-Type": "application/json",
				"X-Test":       "true",
			},
			"body": map[string]any{
				"query": "query GetUser { user { id } }",
			},
		}),
		OutputValue: makeStruct(t, map[string]any{
			"statusCode": 200,
			"body":       encodedBody,
		}),
		OutputSchema: &core.JsonSchema{
			Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
			Properties: map[string]*core.JsonSchema{
				"statusCode": {
					Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_NUMBER,
				},
				"body": {
					Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
					Encoding:    core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
					DecodedType: core.DecodedType_DECODED_TYPE_JSON.Enum(),
				},
			},
		},
		Metadata: makeStruct(t, map[string]any{
			"env":         "prod",
			"http.method": "POST",
			"http.target": "/graphql?op=GetUser",
			"http.url":    "https://api.example.com/graphql?op=GetUser",
			"customField": "value",
		}),
	}

	displaySpan := &core.Span{
		Name:        "query GetUser",
		PackageType: core.PackageType_PACKAGE_TYPE_GRAPHQL,
		Status: &core.SpanStatus{
			Code: core.StatusCode_STATUS_CODE_OK,
		},
		Duration:  durationpb.New(1500 * time.Millisecond),
		Timestamp: timestamppb.New(ts),
	}

	traceTest := &backend.TraceTest{
		Id:      "tt-123",
		TraceId: "trace-123",
		Spans:   []*core.Span{displaySpan, serverSpan},
	}

	got := ConvertTraceTestToRunnerTest(traceTest)

	require.Equal(t, "trace_trace-123.json", got.FileName)
	require.Equal(t, "trace-123", got.TraceID)
	require.Equal(t, "tt-123", got.TraceTestID)
	require.Equal(t, traceTest.Spans, got.Spans)
	require.Equal(t, "http", got.Type)
	require.Equal(t, "GRAPHQL", got.DisplayType)
	require.Equal(t, "query GetUser", got.DisplayName)
	require.Equal(t, "success", got.Status)
	require.Equal(t, 1500, got.Duration)
	require.Equal(t, ts.Format(time.RFC3339), got.Timestamp)
	require.Equal(t, "POST", got.Method)
	require.Equal(t, "POST", got.Request.Method)
	require.Equal(t, "/graphql?op=GetUser", got.Path)
	require.Equal(t, "/graphql?op=GetUser", got.Request.Path)
	require.Equal(t, map[string]string{
		"Content-Type": "application/json",
		"X-Test":       "true",
	}, got.Request.Headers)
	require.Equal(t, map[string]any{
		"query": "query GetUser { user { id } }",
	}, got.Request.Body)
	require.Equal(t, 200, got.Response.Status)
	require.Equal(t, map[string]any{
		"status": "ok",
	}, got.Response.Body)
	require.Equal(t, map[string]any{
		"env":         "prod",
		"http.method": "POST",
		"http.target": "/graphql?op=GetUser",
		"http.url":    "https://api.example.com/graphql?op=GetUser",
		"customField": "value",
	}, got.Metadata)
}

func TestConvertTraceTestToRunnerTest_MetadataFallback(t *testing.T) {
	t.Parallel()

	metadata := makeStruct(t, map[string]any{
		"http.method": "POST",
		"http.url":    "https://api.example.com/foo?bar=baz",
	})

	serverSpan := &core.Span{
		Kind:        core.SpanKind_SPAN_KIND_SERVER,
		PackageName: "fetch",
		Metadata:    metadata,
		Status: &core.SpanStatus{
			Code: core.StatusCode_STATUS_CODE_ERROR,
		},
	}

	traceTest := &backend.TraceTest{
		Id:      "tt-456",
		TraceId: "trace-456",
		Spans:   []*core.Span{serverSpan},
	}

	got := ConvertTraceTestToRunnerTest(traceTest)

	require.Equal(t, "trace_trace-456.json", got.FileName)
	require.Equal(t, "trace-456", got.TraceID)
	require.Equal(t, "tt-456", got.TraceTestID)
	require.Equal(t, traceTest.Spans, got.Spans)
	require.Equal(t, "http", got.Type)
	require.Equal(t, "FETCH", got.DisplayType)
	require.Equal(t, "error", got.Status)
	require.Equal(t, 0, got.Duration)
	require.Equal(t, "", got.Timestamp)

	expectedPath := "/foo?bar=baz"
	require.Equal(t, "POST", got.Method)
	require.Equal(t, "POST", got.Request.Method)
	require.Equal(t, expectedPath, got.Path)
	require.Equal(t, expectedPath, got.Request.Path)
	require.Empty(t, got.Request.Headers)
	require.Nil(t, got.Request.Body)
	require.Equal(t, 0, got.Response.Status)
	require.Nil(t, got.Response.Body)
	require.Equal(t, "POST /foo?bar=baz", got.DisplayName)
	require.Equal(t, map[string]any{
		"http.method": "POST",
		"http.url":    "https://api.example.com/foo?bar=baz",
	}, got.Metadata)
}

func TestConvertTraceTestsToRunnerTests(t *testing.T) {
	t.Parallel()

	tt1 := &backend.TraceTest{Id: "tt-1", TraceId: "trace-1"}
	tt2 := &backend.TraceTest{Id: "tt-2", TraceId: "trace-2"}

	got := ConvertTraceTestsToRunnerTests([]*backend.TraceTest{tt1, tt2})

	require.Len(t, got, 2)
	require.Equal(t, ConvertTraceTestToRunnerTest(tt1), got[0])
	require.Equal(t, ConvertTraceTestToRunnerTest(tt2), got[1])
}

func TestConvertRunnerResultToTraceTestResult(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		result := TestResult{TestID: "tt-1", Passed: true}
		output := ConvertRunnerResultToTraceTestResult(result, Test{TraceTestID: "tt-1"})

		require.Equal(t, "tt-1", output.GetTraceTestId())
		require.True(t, output.GetTestSuccess())
		require.Nil(t, output.TestFailureReason)
		require.Nil(t, output.TestFailureMessage)
		require.Empty(t, output.SpanResults)
	})

	t.Run("failureWithError", func(t *testing.T) {
		t.Parallel()

		result := TestResult{
			TestID: "tt-2",
			Passed: false,
			Error:  "request timeout",
			Deviations: []Deviation{
				{Field: "body", Description: "mismatch"},
			},
		}

		output := ConvertRunnerResultToTraceTestResult(result, Test{TraceTestID: "tt-2"})

		require.Equal(t, "tt-2", output.GetTraceTestId())
		require.False(t, output.GetTestSuccess())
		require.NotNil(t, output.TestFailureReason)
		require.Equal(t, backend.TraceTestFailureReason_TRACE_TEST_FAILURE_REASON_NO_RESPONSE, output.GetTestFailureReason())
		require.NotNil(t, output.TestFailureMessage)
		require.Equal(t, "request timeout", output.GetTestFailureMessage())
		require.Len(t, output.SpanResults, 1)
		require.Len(t, output.SpanResults[0].Deviations, 1)
		require.Equal(t, "body", output.SpanResults[0].Deviations[0].GetField())
		require.Equal(t, "mismatch", output.SpanResults[0].Deviations[0].GetDescription())
	})

	t.Run("failureWithDeviations", func(t *testing.T) {
		t.Parallel()

		result := TestResult{
			TestID: "tt-3",
			Passed: false,
			Deviations: []Deviation{
				{Field: "status", Description: "expected 200"},
				{Field: "body", Description: "missing field"},
			},
		}

		output := ConvertRunnerResultToTraceTestResult(result, Test{TraceTestID: "tt-3"})

		require.Equal(t, "tt-3", output.GetTraceTestId())
		require.False(t, output.GetTestSuccess())
		require.NotNil(t, output.TestFailureReason)
		require.Equal(t, backend.TraceTestFailureReason_TRACE_TEST_FAILURE_REASON_RESPONSE_MISMATCH, output.GetTestFailureReason())
		require.NotNil(t, output.TestFailureMessage)
		require.Equal(t, "Found 2 deviations", output.GetTestFailureMessage())
		require.Len(t, output.SpanResults, 1)
		require.Len(t, output.SpanResults[0].Deviations, 2)
		require.Equal(t, "status", output.SpanResults[0].Deviations[0].GetField())
		require.Equal(t, "expected 200", output.SpanResults[0].Deviations[0].GetDescription())
		require.Equal(t, "body", output.SpanResults[0].Deviations[1].GetField())
		require.Equal(t, "missing field", output.SpanResults[0].Deviations[1].GetDescription())
	})
}

func TestConvertRunnerResultsToTraceTestResults(t *testing.T) {
	t.Parallel()

	results := []TestResult{
		{TestID: "tt-1", Passed: true},
		{TestID: "tt-2", Passed: false, Error: "boom"},
	}
	tests := []Test{
		{TraceTestID: "tt-1"},
		{TraceTestID: "tt-2"},
	}

	got := ConvertRunnerResultsToTraceTestResults(results, tests)

	require.Len(t, got, 2)

	require.Equal(t, "tt-1", got[0].GetTraceTestId())
	require.True(t, got[0].GetTestSuccess())
	require.Nil(t, got[0].TestFailureReason)
	require.Equal(t, "", got[0].GetTestFailureMessage())
	require.Empty(t, got[0].SpanResults)

	require.Equal(t, "tt-2", got[1].GetTraceTestId())
	require.False(t, got[1].GetTestSuccess())
	require.NotNil(t, got[1].TestFailureReason)
	require.Equal(t, backend.TraceTestFailureReason_TRACE_TEST_FAILURE_REASON_NO_RESPONSE, got[1].GetTestFailureReason())
	require.Equal(t, "boom", got[1].GetTestFailureMessage())
	require.Empty(t, got[1].SpanResults)
}
