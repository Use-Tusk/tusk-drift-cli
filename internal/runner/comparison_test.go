package runner

import (
	"testing"

	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
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

func TestCompareAndGenerateResult_PlainTextDecodedType(t *testing.T) {
	executor := &Executor{}

	// When decodedType=PLAIN_TEXT, the body should be treated as a raw string
	// Even if it contains valid JSON syntax, it should NOT be parsed
	expectedBody := `"Pull request reminders sent successfully"`

	test := Test{
		TraceID: "t-plain-text-decoded-type",
		Spans: []*core.Span{
			makeSpanWithOutputSchema(core.DecodedType_DECODED_TYPE_PLAIN_TEXT),
		},
		Response: Response{
			Status:  200,
			Headers: map[string]string{},
			Body:    expectedBody,
		},
	}

	// Actual response contains a JSON-encoded string, but should be treated as plain text
	actualResponseBody := `"Pull request reminders sent successfully"`
	resp := makeResponse(200, nil, actualResponseBody)

	res, err := executor.compareAndGenerateResult(test, resp, 45)
	require.NoError(t, err)
	require.True(t, res.Passed, "PLAIN_TEXT decodedType should treat body as raw string")
	require.Empty(t, res.Deviations)
}

func TestCompareAndGenerateResult_PlainTextDecodedType_JSONLikeContent(t *testing.T) {
	executor := &Executor{}

	// When decodedType=PLAIN_TEXT, even JSON-like content should be treated as string
	expectedBody := `{"key": "value"}`

	test := Test{
		TraceID: "t-plain-text-json-like",
		Spans: []*core.Span{
			makeSpanWithOutputSchema(core.DecodedType_DECODED_TYPE_PLAIN_TEXT),
		},
		Response: Response{
			Status:  200,
			Headers: map[string]string{},
			Body:    expectedBody,
		},
	}

	actualResponseBody := `{"key": "value"}`
	resp := makeResponse(200, nil, actualResponseBody)

	res, err := executor.compareAndGenerateResult(test, resp, 10)
	require.NoError(t, err)
	require.True(t, res.Passed, "PLAIN_TEXT decodedType should not parse JSON")
	require.Empty(t, res.Deviations)
}

func TestCompareAndGenerateResult_JSONDecodedType(t *testing.T) {
	executor := &Executor{}

	// When decodedType=JSON, body should be parsed as JSON
	expectedBody := map[string]any{"status": "success"}

	test := Test{
		TraceID: "t-json-decoded-type",
		Spans: []*core.Span{
			makeSpanWithOutputSchema(core.DecodedType_DECODED_TYPE_JSON),
		},
		Response: Response{
			Status:  200,
			Headers: map[string]string{},
			Body:    expectedBody,
		},
	}

	actualResponseBody := `{"status": "success"}`
	resp := makeResponse(200, nil, actualResponseBody)

	res, err := executor.compareAndGenerateResult(test, resp, 20)
	require.NoError(t, err)
	require.True(t, res.Passed, "JSON decodedType should parse as JSON")
	require.Empty(t, res.Deviations)
}

func TestCompareAndGenerateResult_UnspecifiedDecodedType(t *testing.T) {
	executor := &Executor{}

	// When decodedType=UNSPECIFIED, tries JSON parsing first, falls back to string
	// Valid JSON should be parsed
	expectedBody := map[string]any{"ok": true}

	test := Test{
		TraceID: "t-unspecified-decoded-type",
		Spans: []*core.Span{
			makeSpanWithOutputSchema(core.DecodedType_DECODED_TYPE_UNSPECIFIED),
		},
		Response: Response{
			Status:  200,
			Headers: map[string]string{},
			Body:    expectedBody,
		},
	}

	actualResponseBody := `{"ok": true}`
	resp := makeResponse(200, nil, actualResponseBody)

	res, err := executor.compareAndGenerateResult(test, resp, 15)
	require.NoError(t, err)
	require.True(t, res.Passed, "UNSPECIFIED decodedType should try JSON parsing")
	require.Empty(t, res.Deviations)
}

func TestCompareAndGenerateResult_UnspecifiedDecodedType_FallbackToString(t *testing.T) {
	executor := &Executor{}

	// When decodedType=UNSPECIFIED and content is not valid JSON, should fall back to string
	expectedBody := "not valid json"

	test := Test{
		TraceID: "t-unspecified-fallback",
		Spans: []*core.Span{
			makeSpanWithOutputSchema(core.DecodedType_DECODED_TYPE_UNSPECIFIED),
		},
		Response: Response{
			Status:  200,
			Headers: map[string]string{},
			Body:    expectedBody,
		},
	}

	actualResponseBody := "not valid json"
	resp := makeResponse(200, nil, actualResponseBody)

	res, err := executor.compareAndGenerateResult(test, resp, 12)
	require.NoError(t, err)
	require.True(t, res.Passed, "UNSPECIFIED decodedType should fall back to string for non-JSON")
	require.Empty(t, res.Deviations)
}

func TestCompareAndGenerateResult_NoSpans_DefaultsToUnspecified(t *testing.T) {
	executor := &Executor{}

	// When test.Spans is empty, should default to UNSPECIFIED behavior
	expectedBody := map[string]any{"data": "test"}

	test := Test{
		TraceID: "t-no-spans",
		Spans:   []*core.Span{}, // Empty spans
		Response: Response{
			Status:  200,
			Headers: map[string]string{},
			Body:    expectedBody,
		},
	}

	actualResponseBody := `{"data": "test"}`
	resp := makeResponse(200, nil, actualResponseBody)

	res, err := executor.compareAndGenerateResult(test, resp, 8)
	require.NoError(t, err)
	require.True(t, res.Passed, "Empty spans should default to UNSPECIFIED (tries JSON)")
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
