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

func TestParseSpansFromFile_MapsOTelSpanKindsToProto(t *testing.T) {
	// Test that OTel SpanKind values (SERVER=1, CLIENT=2) are mapped to Proto values (SERVER=2, CLIENT=3)
	// when the file contains isRootSpan=true with kind=1 (OTel SERVER).
	tmp := t.TempDir()
	filename := filepath.Join(tmp, "trace.jsonl")

	// OTel values: SERVER=1, CLIENT=2
	serverSpan := map[string]any{
		"traceId":    "t1",
		"spanId":     "s1",
		"name":       "server",
		"isRootSpan": true,
		"kind":       1, // OTel SERVER
	}
	clientSpan := map[string]any{
		"traceId":    "t1",
		"spanId":     "s2",
		"name":       "client",
		"isRootSpan": false,
		"kind":       2, // OTel CLIENT
	}

	serverBytes, err := json.Marshal(serverSpan)
	require.NoError(t, err)
	clientBytes, err := json.Marshal(clientSpan)
	require.NoError(t, err)

	var data []byte
	data = append(data, serverBytes...)
	data = append(data, '\n')
	data = append(data, clientBytes...)
	data = append(data, '\n')
	require.NoError(t, os.WriteFile(filename, data, 0o644))

	spans, err := ParseSpansFromFile(filename, nil)
	require.NoError(t, err)
	require.Len(t, spans, 2)

	// After mapping: OTel SERVER (1) → Proto SERVER (2), OTel CLIENT (2) → Proto CLIENT (3)
	var server, client *core.Span
	for _, s := range spans {
		if s.Name == "server" {
			server = s
		} else if s.Name == "client" {
			client = s
		}
	}

	require.NotNil(t, server)
	require.NotNil(t, client)
	assert.Equal(t, core.SpanKind_SPAN_KIND_SERVER, server.Kind, "OTel SERVER (1) should map to Proto SERVER (2)")
	assert.Equal(t, core.SpanKind_SPAN_KIND_CLIENT, client.Kind, "OTel CLIENT (2) should map to Proto CLIENT (3)")
}

func TestParseSpansFromFile_DoesNotMapProtoSpanKinds(t *testing.T) {
	// Test that Proto SpanKind values are NOT remapped when the file already uses Proto values.
	// Detection: isRootSpan=true with kind=2 (Proto SERVER) means no mapping needed.
	tmp := t.TempDir()
	filename := filepath.Join(tmp, "trace.jsonl")

	// Proto values: SERVER=2, CLIENT=3
	serverSpan := map[string]any{
		"traceId":    "t1",
		"spanId":     "s1",
		"name":       "server",
		"isRootSpan": true,
		"kind":       2, // Proto SERVER
	}
	clientSpan := map[string]any{
		"traceId":    "t1",
		"spanId":     "s2",
		"name":       "client",
		"isRootSpan": false,
		"kind":       3, // Proto CLIENT
	}

	serverBytes, err := json.Marshal(serverSpan)
	require.NoError(t, err)
	clientBytes, err := json.Marshal(clientSpan)
	require.NoError(t, err)

	var data []byte
	data = append(data, serverBytes...)
	data = append(data, '\n')
	data = append(data, clientBytes...)
	data = append(data, '\n')
	require.NoError(t, os.WriteFile(filename, data, 0o644))

	spans, err := ParseSpansFromFile(filename, nil)
	require.NoError(t, err)
	require.Len(t, spans, 2)

	// Values should remain unchanged since they're already Proto format
	var server, client *core.Span
	for _, s := range spans {
		if s.Name == "server" {
			server = s
		} else if s.Name == "client" {
			client = s
		}
	}

	require.NotNil(t, server)
	require.NotNil(t, client)
	assert.Equal(t, core.SpanKind_SPAN_KIND_SERVER, server.Kind, "Proto SERVER (2) should stay as Proto SERVER (2)")
	assert.Equal(t, core.SpanKind_SPAN_KIND_CLIENT, client.Kind, "Proto CLIENT (3) should stay as Proto CLIENT (3)")
}

func TestParseSpansFromFile_FixesEnvVarsSnapshotSpan(t *testing.T) {
	// Test that ENV_VARS_SNAPSHOT spans get INTERNAL kind
	tmp := t.TempDir()
	filename := filepath.Join(tmp, "trace.jsonl")

	// Mix of spans including ENV_VARS_SNAPSHOT with wrong kind
	serverSpan := map[string]any{
		"traceId":    "t1",
		"spanId":     "s1",
		"name":       "server",
		"isRootSpan": true,
		"kind":       1, // Wrong - triggers fix
	}
	envSpan := map[string]any{
		"traceId":    "t1",
		"spanId":     "s2",
		"name":       "ENV_VARS_SNAPSHOT",
		"isRootSpan": false,
		"kind":       0, // Whatever value - should become INTERNAL
	}

	serverBytes, err := json.Marshal(serverSpan)
	require.NoError(t, err)
	envBytes, err := json.Marshal(envSpan)
	require.NoError(t, err)

	var data []byte
	data = append(data, serverBytes...)
	data = append(data, '\n')
	data = append(data, envBytes...)
	data = append(data, '\n')
	require.NoError(t, os.WriteFile(filename, data, 0o644))

	spans, err := ParseSpansFromFile(filename, nil)
	require.NoError(t, err)
	require.Len(t, spans, 2)

	var server, env *core.Span
	for _, s := range spans {
		if s.Name == "server" {
			server = s
		} else if s.Name == "ENV_VARS_SNAPSHOT" {
			env = s
		}
	}

	require.NotNil(t, server)
	require.NotNil(t, env)
	assert.Equal(t, core.SpanKind_SPAN_KIND_SERVER, server.Kind)
	assert.Equal(t, core.SpanKind_SPAN_KIND_INTERNAL, env.Kind, "ENV_VARS_SNAPSHOT should be INTERNAL")
}

func TestParseSpansFromFile_HandlesMixedOldNewTraces(t *testing.T) {
	// Test that mixed old (OTel) and new (Proto) traces are handled correctly.
	// If ANY root span is wrong, we fix ALL spans using semantic derivation.
	tmp := t.TempDir()
	filename := filepath.Join(tmp, "trace.jsonl")

	// First trace: old format (OTel values)
	oldServerSpan := map[string]any{
		"traceId":    "t1",
		"spanId":     "s1",
		"name":       "old-server",
		"isRootSpan": true,
		"kind":       1, // OTel SERVER - wrong
	}
	oldClientSpan := map[string]any{
		"traceId":    "t1",
		"spanId":     "s2",
		"name":       "old-client",
		"isRootSpan": false,
		"kind":       2, // OTel CLIENT - wrong
	}
	// Second trace: new format (Proto values)
	newServerSpan := map[string]any{
		"traceId":    "t2",
		"spanId":     "s3",
		"name":       "new-server",
		"isRootSpan": true,
		"kind":       2, // Proto SERVER - correct
	}
	newClientSpan := map[string]any{
		"traceId":    "t2",
		"spanId":     "s4",
		"name":       "new-client",
		"isRootSpan": false,
		"kind":       3, // Proto CLIENT - correct
	}

	var data []byte
	for _, span := range []map[string]any{oldServerSpan, oldClientSpan, newServerSpan, newClientSpan} {
		bytes, err := json.Marshal(span)
		require.NoError(t, err)
		data = append(data, bytes...)
		data = append(data, '\n')
	}
	require.NoError(t, os.WriteFile(filename, data, 0o644))

	spans, err := ParseSpansFromFile(filename, nil)
	require.NoError(t, err)
	require.Len(t, spans, 4)

	// All spans should have correct proto kinds (derived from isRootSpan)
	for _, s := range spans {
		if s.IsRootSpan {
			assert.Equal(t, core.SpanKind_SPAN_KIND_SERVER, s.Kind, "Root span %s should be SERVER", s.Name)
		} else {
			assert.Equal(t, core.SpanKind_SPAN_KIND_CLIENT, s.Kind, "Non-root span %s should be CLIENT", s.Name)
		}
	}
}

func TestParseSpansFromFile_FixesEnvVarsSnapshotAloneInFile(t *testing.T) {
	// Reproduces the real bug: ENV_VARS_SNAPSHOT is alone in its own trace file
	// with no root span. The SDK bug (|| vs ??) wrote it as OTel CLIENT (2),
	// which the CLI misinterprets as Proto SERVER (2), creating a bogus trace test.
	tmp := t.TempDir()
	filename := filepath.Join(tmp, "trace.jsonl")

	envSpan := map[string]any{
		"traceId":       "880d74306bf83ac8c15162456d819f8e",
		"spanId":        "s1",
		"name":          "ENV_VARS_SNAPSHOT",
		"isRootSpan":    false,
		"isPreAppStart": true,
		"kind":          2, // OTel CLIENT (due to SDK || bug), but Proto SERVER
		"packageName":   "process.env",
	}

	envBytes, err := json.Marshal(envSpan)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filename, append(envBytes, '\n'), 0o644))

	spans, err := ParseSpansFromFile(filename, nil)
	require.NoError(t, err)
	require.Len(t, spans, 1)

	assert.Equal(t, "ENV_VARS_SNAPSHOT", spans[0].Name)
	assert.Equal(t, core.SpanKind_SPAN_KIND_INTERNAL, spans[0].Kind,
		"ENV_VARS_SNAPSHOT alone in file should be fixed to INTERNAL, not left as SERVER")
}
