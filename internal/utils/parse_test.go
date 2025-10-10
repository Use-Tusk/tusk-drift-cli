//nolint:gosec
package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
)

func TestParseProtobufSpanFromJSON_Basic(t *testing.T) {
	input := map[string]any{
		"traceId":             "t1",
		"spanId":              "s1",
		"parentSpanId":        "p1",
		"name":                "span-name",
		"packageName":         "http",
		"instrumentationName": "lib",
		"submodule_name":      "sub",
		"inputValue": map[string]any{
			"a": 1,
			"headers": map[string]any{
				"accept": "application/json",
			},
		},
		"outputValue": map[string]any{
			"b": "x",
		},
		"inputSchema": map[string]any{
			"type": 6, // JSON_SCHEMA_TYPE_OBJECT
		},
		"outputSchema": map[string]any{
			"type": 6, // JSON_SCHEMA_TYPE_OBJECT
			"properties": map[string]any{
				"x": map[string]any{
					"type": 1, // JSON_SCHEMA_TYPE_NUMBER
				},
			},
		},
		"inputSchemaHash":  "ih",
		"outputSchemaHash": "oh",
		"inputValueHash":   "ivh",
		"outputValueHash":  "ovh",
		"kind":             2, // arbitrary enum numeric
		"status": map[string]any{
			"code":    2, // arbitrary enum numeric
			"message": "ok",
		},
		"timestamp": map[string]any{
			"seconds": 123,
			"nanos":   456,
		},
		"duration": map[string]any{
			"seconds": 5,
			"nanos":   6,
		},
		"isRootSpan":  true,
		"metadata":    map[string]any{"m": "v"},
		"packageType": 1, // arbitrary enum numeric
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	span, err := ParseProtobufSpanFromJSON(data)
	require.NoError(t, err)
	require.NotNil(t, span)

	assert.Equal(t, "t1", span.TraceId)
	assert.Equal(t, "s1", span.SpanId)
	assert.Equal(t, "p1", span.ParentSpanId)
	assert.Equal(t, "span-name", span.Name)
	assert.Equal(t, "http", span.PackageName)
	assert.Equal(t, "lib", span.InstrumentationName)
	assert.Equal(t, "sub", span.SubmoduleName)

	// Struct fields -> AsMap checks
	require.NotNil(t, span.InputValue)
	inMap := span.InputValue.AsMap()
	require.NotNil(t, inMap)
	assert.Equal(t, float64(1), inMap["a"])
	if hdrs, ok := inMap["headers"].(map[string]any); ok {
		assert.Equal(t, "application/json", hdrs["accept"])
	} else {
		t.Fatalf("expected headers map, got %T", inMap["headers"])
	}

	require.NotNil(t, span.OutputValue)
	outMap := span.OutputValue.AsMap()
	require.NotNil(t, outMap)
	assert.Equal(t, "x", outMap["b"])

	require.NotNil(t, span.InputSchema)
	assert.Equal(t, core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT, span.InputSchema.Type)

	require.NotNil(t, span.OutputSchema)
	assert.NotNil(t, span.OutputSchema.Properties)
	assert.Contains(t, span.OutputSchema.Properties, "x")

	// Hashes/strings
	assert.Equal(t, "ih", span.InputSchemaHash)
	assert.Equal(t, "oh", span.OutputSchemaHash)
	assert.Equal(t, "ivh", span.InputValueHash)
	assert.Equal(t, "ovh", span.OutputValueHash)

	// Enums and flags
	assert.Equal(t, core.SpanKind(2), span.Kind)
	require.NotNil(t, span.Status)
	assert.Equal(t, core.StatusCode(2), span.Status.Code)
	assert.Equal(t, "ok", span.Status.Message)
	assert.True(t, span.IsRootSpan)
	assert.Equal(t, core.PackageType(1), span.PackageType)

	// Timestamp / Duration
	require.NotNil(t, span.Timestamp)
	assert.Equal(t, int64(123), span.Timestamp.Seconds)
	assert.Equal(t, int32(456), span.Timestamp.Nanos)

	require.NotNil(t, span.Duration)
	assert.Equal(t, int64(5), span.Duration.Seconds)
	assert.Equal(t, int32(6), span.Duration.Nanos)

	// Metadata present
	require.NotNil(t, span.Metadata)
	md := span.Metadata.AsMap()
	assert.Equal(t, "v", md["m"])
}

func TestParseSpansFromFile_ReturnsErrorOnMalformed(t *testing.T) {
	tmp := t.TempDir()
	filename := filepath.Join(tmp, "trace.jsonl")

	// Build lines: empty, valid "keep", malformed, valid "drop"
	keep := map[string]any{"traceId": "t-1", "spanId": "s-1", "name": "keep"}
	drop := map[string]any{"traceId": "t-2", "spanId": "s-2", "name": "drop"}

	keepBytes, err := json.Marshal(keep)
	require.NoError(t, err)
	dropBytes, err := json.Marshal(drop)
	require.NoError(t, err)

	content := []byte("\n")
	content = append(content, keepBytes...)
	content = append(content, byte('\n'))
	content = append(content, []byte("{malformed")...) // malformed line should cause an error
	content = append(content, byte('\n'))
	content = append(content, dropBytes...)
	content = append(content, byte('\n'))

	require.NoError(t, os.WriteFile(filename, content, 0o644))

	// Filter: only keep name == "keep"
	filter := func(s *core.Span) bool { return s.GetName() == "keep" }
	spans, err := ParseSpansFromFile(filename, filter)
	require.Error(t, err)
	assert.Nil(t, spans)
}

func TestParseSpansFromFile_NoFilterReturnsAllValid(t *testing.T) {
	tmp := t.TempDir()
	filename := filepath.Join(tmp, "trace.jsonl")

	a := map[string]any{"traceId": "t-a", "spanId": "s-a", "name": "A"}
	b := map[string]any{"traceId": "t-b", "spanId": "s-b", "name": "B"}

	aBytes, err := json.Marshal(a)
	require.NoError(t, err)
	bBytes, err := json.Marshal(b)
	require.NoError(t, err)

	lines := [][]byte{
		aBytes,
		bBytes,
	}
	var data []byte
	for _, ln := range lines {
		data = append(data, ln...)
		data = append(data, '\n')
	}
	require.NoError(t, os.WriteFile(filename, data, 0o644))

	spans, err := ParseSpansFromFile(filename, nil)
	require.NoError(t, err)
	require.Len(t, spans, 2)
	names := []string{spans[0].Name, spans[1].Name}
	assert.ElementsMatch(t, []string{"A", "B"}, names)
}
