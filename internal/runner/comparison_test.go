package runner

import (
	"testing"

	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
	"github.com/stretchr/testify/require"
)

func TestCompareAndGenerateResult_PassesWithIgnoredDynamicFields(t *testing.T) {
	executor := &Executor{}

	expected := jsonAny(t, `{
		"user": {
			"id": "00000000-0000-0000-0000-000000000000",
			"name": "Alice",
			"createdAt": "2023-01-01T00:00:00Z"
		}
	}`)

	actualBody := `{
		"user": {
			"id": "11111111-1111-1111-1111-111111111111",
			"name": "Alice",
			"createdAt": "2024-02-02T12:34:56Z"
		}
	}`

	test := Test{
		TraceID: "t-1",
		Response: Response{
			Status:  200,
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    expected,
		},
	}

	resp := makeResponse(200, map[string]string{"Content-Type": "application/json"}, actualBody)

	res, err := executor.compareAndGenerateResult(test, resp, 123)
	require.NoError(t, err)
	require.True(t, res.Passed)
	require.Empty(t, res.Deviations)
	require.Equal(t, "t-1", res.TestID)
	require.Equal(t, 123, res.Duration)
}

func TestCompareAndGenerateResult_StatusMismatch(t *testing.T) {
	executor := &Executor{}

	test := Test{
		TraceID: "t-2",
		Response: Response{
			Status:  200,
			Headers: map[string]string{},
			Body:    jsonAny(t, `{"ok": true}`),
		},
	}
	resp := makeResponse(500, nil, `{"ok": true}`)

	res, err := executor.compareAndGenerateResult(test, resp, 10)
	require.NoError(t, err)
	require.False(t, res.Passed)
	require.Len(t, res.Deviations, 1)
	require.Equal(t, "response.status", res.Deviations[0].Field)
	require.Equal(t, 200, res.Deviations[0].Expected)
	require.Equal(t, 500, res.Deviations[0].Actual)
}

func TestCompareAndGenerateResult_HeaderMismatch(t *testing.T) {
	executor := &Executor{}

	test := Test{
		TraceID: "t-3",
		Response: Response{
			Status:  200,
			Headers: map[string]string{"X-RateLimit-Remaining": "43"},
			Body:    jsonAny(t, `{"ok": true}`),
		},
	}
	resp := makeResponse(200, map[string]string{"X-RateLimit-Remaining": "42"}, `{"ok": true}`)

	res, err := executor.compareAndGenerateResult(test, resp, 5)
	require.NoError(t, err)
	require.False(t, res.Passed)
	require.Len(t, res.Deviations, 1)
	require.Equal(t, "response.headers.x-ratelimit-remaining", res.Deviations[0].Field)
	require.Equal(t, "43", res.Deviations[0].Expected)
	require.Equal(t, "42", res.Deviations[0].Actual)
}

func TestCompareAndGenerateResult_BodyMismatchDueToExtraActualKey(t *testing.T) {
	config.ResetForTesting()

	// Ensure ignore_fields is empty so extra keys are not ignored.
	cfgPath := writeTempConfig(t, `
comparison:
  ignore_fields: []
  ignore_uuids: true
  ignore_timestamps: true
  ignore_dates: true
`)
	require.NoError(t, config.Load(cfgPath))

	executor := &Executor{}
	test := Test{
		TraceID: "t-4",
		Response: Response{
			Status:  200,
			Headers: map[string]string{},
			Body:    map[string]any{}, // Expect empty object
		},
	}
	// Actual contains an extra field not present in expected.
	resp := makeResponse(200, nil, `{"traceId":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}`)

	res, err := executor.compareAndGenerateResult(test, resp, 1)
	require.NoError(t, err)
	require.False(t, res.Passed)
	require.Len(t, res.Deviations, 1)
	require.Equal(t, "response.body", res.Deviations[0].Field)
}

func TestCompareAndGenerateResult_TextPlainWithJSONString(t *testing.T) {
	executor := &Executor{}

	// Expected body: when decoded from span with decodedType=PLAIN_TEXT,
	// it returns the raw string including the quotes
	// (This is what DecodeValueBySchema returns for PLAIN_TEXT)
	expectedBody := `"Pull request reminders sent successfully"`

	test := Test{
		TraceID: "t-text-plain",
		Response: Response{
			Status:  200,
			Headers: map[string]string{"Content-Type": "text/plain; charset=utf-8"},
			Body:    expectedBody,
		},
	}

	// Actual response: Express returns a JSON-encoded string with text/plain content-type
	actualResponseBody := `"Pull request reminders sent successfully"`
	resp := makeResponse(200, map[string]string{"Content-Type": "text/plain; charset=utf-8"}, actualResponseBody)

	res, err := executor.compareAndGenerateResult(test, resp, 45)
	require.NoError(t, err)
	require.True(t, res.Passed, "text/plain response with JSON-encoded string should match")
	require.Empty(t, res.Deviations)
	require.Equal(t, "t-text-plain", res.TestID)
	require.Equal(t, 45, res.Duration)
}

func TestCompareAndGenerateResult_TextPlainVsJSON(t *testing.T) {
	executor := &Executor{}

	// Test that text/plain is not parsed as JSON
	expectedBody := `{"key": "value"}`

	test := Test{
		TraceID: "t-text-plain-json",
		Response: Response{
			Status:  200,
			Headers: map[string]string{"Content-Type": "text/plain"},
			Body:    expectedBody,
		},
	}

	actualResponseBody := `{"key": "value"}`
	resp := makeResponse(200, map[string]string{"Content-Type": "text/plain"}, actualResponseBody)

	res, err := executor.compareAndGenerateResult(test, resp, 10)
	require.NoError(t, err)
	require.True(t, res.Passed, "text/plain with JSON-like content should be treated as string")
	require.Empty(t, res.Deviations)
}

func TestCompareAndGenerateResult_MissingContentType(t *testing.T) {
	executor := &Executor{}

	// When content-type is missing, SDK sets decodedType=UNSPECIFIED
	// which tries JSON parsing first, then falls back to string
	// So our expected value here is the parsed JSON
	expectedBody := map[string]any{"ok": true}

	test := Test{
		TraceID: "t-no-content-type",
		Response: Response{
			Status:  200,
			Headers: map[string]string{}, // No content-type
			Body:    expectedBody,
		},
	}

	actualResponseBody := `{"ok": true}`
	resp := makeResponse(200, nil, actualResponseBody)

	res, err := executor.compareAndGenerateResult(test, resp, 15)
	require.NoError(t, err)
	require.True(t, res.Passed, "missing content-type should default to JSON parsing")
	require.Empty(t, res.Deviations)
}

func TestCompareAndGenerateResult_JSONContentType(t *testing.T) {
	executor := &Executor{}

	// With application/json content-type, body should be parsed as JSON
	expectedBody := map[string]any{"status": "success"}

	test := Test{
		TraceID: "t-json-content-type",
		Response: Response{
			Status:  200,
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    expectedBody,
		},
	}

	actualResponseBody := `{"status": "success"}`
	resp := makeResponse(200, map[string]string{"Content-Type": "application/json"}, actualResponseBody)

	res, err := executor.compareAndGenerateResult(test, resp, 20)
	require.NoError(t, err)
	require.True(t, res.Passed, "application/json should parse as JSON")
	require.Empty(t, res.Deviations)
}

func TestCompareResponseBodies_IgnoreExtraActualKeyViaConfigIgnoreFields(t *testing.T) {
	config.ResetForTesting()

	// Configure to ignore an extra field by name.
	cfgPath := writeTempConfig(t, `
comparison:
  ignore_fields:
    - traceId
  ignore_uuids: true
  ignore_timestamps: true
  ignore_dates: true
`)
	require.NoError(t, config.Load(cfgPath))

	executor := &Executor{}

	expected := map[string]any{} // no fields expected
	actual := map[string]any{
		"traceId": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
	}

	ok := executor.compareResponseBodies(expected, actual, "t-5")
	require.True(t, ok, "extra field 'traceId' should be ignored via config.ignore_fields")
}

func TestCompareJSONValues_TypeMismatch(t *testing.T) {
	executor := &Executor{}
	m := NewDynamicFieldMatcher() // Default patterns

	// expected: object, actual: string
	require.False(t, executor.compareJSONValues("root", map[string]any{"a": 1.0}, "not-an-object", m, "t-6"))

	// expected: number, actual: string
	require.False(t, executor.compareJSONValues("n", 1.0, "1", m, "t-6"))
}

func TestCompareSlices_OrderAndLengthMatters(t *testing.T) {
	executor := &Executor{}
	m := NewDynamicFieldMatcher()

	// Same length, different order
	require.False(t, executor.compareSlices("arr", []any{1.0, 2.0}, []any{2.0, 1.0}, m, "t-7"))
	// Different length
	require.False(t, executor.compareSlices("arr", []any{1.0, 2.0}, []any{1.0}, m, "t-7"))
	// Equal
	require.True(t, executor.compareSlices("arr", []any{1.0, 2.0}, []any{1.0, 2.0}, m, "t-7"))
}

func TestGetFieldName(t *testing.T) {
	require.Equal(t, "", getFieldName(""))
	require.Equal(t, "name", getFieldName("user.profile.name"))
	require.Equal(t, "c", getFieldName("a.b[3].c"))
}

func TestSafeEqual(t *testing.T) {
	// Primitives
	eq, ok := safeEqual("x", "x")
	require.True(t, ok)
	require.True(t, eq)

	eq, ok = safeEqual(1.0, 1.0) // JSON numbers are float64
	require.True(t, ok)
	require.True(t, eq)

	eq, ok = safeEqual(true, false)
	require.True(t, ok)
	require.False(t, eq)

	// Different types
	eq, ok = safeEqual(1.0, "1")
	require.True(t, ok)
	require.False(t, eq)

	// Complex types: cannot compare directly
	_, ok = safeEqual([]any{1.0}, []any{1.0})
	require.False(t, ok)
	_, ok = safeEqual(map[string]any{"a": 1.0}, map[string]any{"a": 1.0})
	require.False(t, ok)
}
