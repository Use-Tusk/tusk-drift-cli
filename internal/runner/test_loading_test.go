package runner

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTraceFile(t *testing.T, dir, name string, spans ...map[string]any) string {
	t.Helper()

	path := filepath.Join(dir, name)
	var data []byte
	for _, span := range spans {
		line, err := json.Marshal(span)
		require.NoError(t, err)
		data = append(data, line...)
		data = append(data, '\n')
	}

	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func TestExecutorLoadTestsFromFolderReadsJSONLTraces(t *testing.T) {
	executor := &Executor{}

	dir := t.TempDir()
	nestedDir := filepath.Join(dir, "nested")
	require.NoError(t, os.MkdirAll(nestedDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("ignore"), 0o600))

	responseJSON, err := json.Marshal(map[string]any{"ok": true})
	require.NoError(t, err)
	encodedResponse := base64.StdEncoding.EncodeToString(responseJSON)

	timestamp := time.Unix(1710000000, 500000000).UTC()

	traceOnePreApp := map[string]any{
		"traceId":       "trace-1",
		"spanId":        "span-pre",
		"name":          "bootstrap",
		"isPreAppStart": true,
	}

	traceOneNonRoot := map[string]any{
		"traceId":        "trace-1",
		"spanId":         "span-non-root",
		"name":           "handler",
		"packageName":    "http",
		"submodule_name": "PUT",
	}

	traceOneRoot := map[string]any{
		"traceId":        "trace-1",
		"spanId":         "span-root",
		"name":           "root-op",
		"packageName":    "http",
		"submodule_name": "POST",
		"isRootSpan":     true,
		"timestamp": map[string]any{
			"seconds": timestamp.Unix(),
			"nanos":   int32(timestamp.Nanosecond()), // #nosec G115
		},
		"duration": map[string]any{
			"seconds": 1,
			"nanos":   500000000,
		},
		"packageType": int(core.PackageType_PACKAGE_TYPE_HTTP),
		"status": map[string]any{
			"code": int(core.StatusCode_STATUS_CODE_ERROR),
		},
		"inputValue": map[string]any{
			"method": "PATCH",
			"target": "/teams/42",
			"headers": map[string]any{
				"accept": "application/json",
				"trace":  "xyz",
			},
			"body": map[string]any{
				"payload": "value",
			},
		},
		"outputValue": map[string]any{
			"statusCode": 201,
			"body":       encodedResponse,
		},
		"metadata": map[string]any{
			"service": "billing",
		},
	}

	traceTwoRoot := map[string]any{
		"traceId":        "trace-2",
		"spanId":         "span-2",
		"name":           "second-trace-root",
		"packageName":    "grpc",
		"submodule_name": "GetUser",
		"isRootSpan":     true,
		"inputValue": map[string]any{
			"target": "/grpc/users",
		},
	}

	writeTraceFile(t, dir, "trace-one.jsonl", traceOnePreApp, traceOneNonRoot, traceOneRoot)
	writeTraceFile(t, nestedDir, "trace-two.jsonl", traceTwoRoot)

	tests, err := executor.LoadTestsFromFolder(dir)
	require.NoError(t, err)
	require.Len(t, tests, 2)

	testsByTrace := map[string]Test{}
	for _, testCase := range tests {
		testsByTrace[testCase.TraceID] = testCase
	}

	traceOneTest, ok := testsByTrace["trace-1"]
	require.True(t, ok)

	assert.Equal(t, "trace-1", traceOneTest.TraceID)
	assert.Equal(t, "trace-one.jsonl", traceOneTest.FileName)
	assert.Equal(t, "http", traceOneTest.Type)
	assert.Equal(t, "HTTP", traceOneTest.DisplayType)
	assert.Equal(t, "PATCH", traceOneTest.Method)
	assert.Equal(t, "/teams/42", traceOneTest.Path)
	assert.Equal(t, "root-op", traceOneTest.DisplayName)
	assert.Equal(t, "error", traceOneTest.Status)
	assert.Equal(t, 1500, traceOneTest.Duration)
	assert.Equal(t, timestamp.Format(time.RFC3339), traceOneTest.Timestamp)
	assert.Equal(t, 201, traceOneTest.Response.Status)
	assert.Equal(t, "billing", traceOneTest.Metadata["service"])

	responseBody, ok := traceOneTest.Response.Body.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, responseBody["ok"])

	assert.Equal(t, map[string]string{
		"accept": "application/json",
		"trace":  "xyz",
	}, traceOneTest.Request.Headers)

	assert.Equal(t, map[string]any{
		"payload": "value",
	}, traceOneTest.Request.Body)

	require.Len(t, traceOneTest.Spans, 3)
	foundPreApp := false
	spanIDs := map[string]struct{}{}
	for _, span := range traceOneTest.Spans {
		spanIDs[span.SpanId] = struct{}{}
		if span.IsPreAppStart {
			foundPreApp = true
		}
	}
	assert.Contains(t, spanIDs, "span-root")
	assert.True(t, foundPreApp)

	traceTwoTest, ok := testsByTrace["trace-2"]
	require.True(t, ok)

	assert.Equal(t, "trace-2", traceTwoTest.TraceID)
	assert.Equal(t, "trace-two.jsonl", traceTwoTest.FileName)
	assert.Equal(t, "grpc", traceTwoTest.Type)
	assert.Equal(t, "grpc", traceTwoTest.DisplayType)
	assert.Equal(t, "GetUser", traceTwoTest.Method)
	assert.Equal(t, "/grpc/users", traceTwoTest.Path)
	assert.Equal(t, "success", traceTwoTest.Status)
	assert.Equal(t, 200, traceTwoTest.Response.Status)
	assert.Empty(t, traceTwoTest.Request.Headers)
	assert.Nil(t, traceTwoTest.Request.Body)
	require.Len(t, traceTwoTest.Spans, 1)
}

func TestExecutorLoadTestsFromFolderMissing(t *testing.T) {
	executor := &Executor{}
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	tests, err := executor.LoadTestsFromFolder(missing)
	require.Error(t, err)
	assert.Empty(t, tests)
	assert.Contains(t, err.Error(), "traces folder not found")
}

func TestExecutorLoadTestsFromFolderPropagatesParseErrors(t *testing.T) {
	executor := &Executor{}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.jsonl"), []byte("{bad"), 0o600))

	tests, err := executor.LoadTestsFromFolder(dir)
	require.Error(t, err)
	assert.Nil(t, tests)
}

func TestExecutorLoadTestFromTraceFileReturnsErrorOnMalformed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.jsonl")
	require.NoError(t, os.WriteFile(path, []byte("{bad"), 0o600))

	_, err := (&Executor{}).LoadTestFromTraceFile(path)
	require.Error(t, err)
}

func TestExecutorLoadSpansForTraceFiltersByTraceID(t *testing.T) {
	executor := &Executor{}
	dir := t.TempDir()
	utils.SetTracesDirOverride(dir)
	t.Cleanup(func() { utils.SetTracesDirOverride("") })

	file := writeTraceFile(t, dir, "combined.jsonl",
		map[string]any{"traceId": "keep", "spanId": "s1", "name": "first"},
		map[string]any{"traceId": "other", "spanId": "s2", "name": "second"},
	)

	spans, err := executor.LoadSpansForTrace("keep", filepath.Base(file))
	require.NoError(t, err)
	require.Len(t, spans, 1)
	assert.Equal(t, "keep", spans[0].TraceId)
}

func TestExecutorLoadSpansForTraceFindsByTraceIDWhenFilenameEmpty(t *testing.T) {
	executor := &Executor{}
	dir := t.TempDir()
	utils.SetTracesDirOverride(dir)
	t.Cleanup(func() { utils.SetTracesDirOverride("") })

	writeTraceFile(t, dir, "2025-01-01_trace-keep.jsonl",
		map[string]any{"traceId": "trace-keep", "spanId": "s1", "name": "root"},
	)

	spans, err := executor.LoadSpansForTrace("trace-keep", "")
	require.NoError(t, err)
	require.Len(t, spans, 1)
	assert.Equal(t, "trace-keep", spans[0].TraceId)
}

func TestExecutorLoadSpansForTracePropagatesError(t *testing.T) {
	executor := &Executor{}
	dir := t.TempDir()
	utils.SetTracesDirOverride(dir)
	t.Cleanup(func() { utils.SetTracesDirOverride("") })

	_, err := executor.LoadSpansForTrace("missing", "")
	require.Error(t, err)
}

func TestPackageTypeToString(t *testing.T) {
	cases := map[core.PackageType]string{
		core.PackageType_PACKAGE_TYPE_HTTP:        "HTTP",
		core.PackageType_PACKAGE_TYPE_GRAPHQL:     "GRAPHQL",
		core.PackageType_PACKAGE_TYPE_GRPC:        "GRPC",
		core.PackageType_PACKAGE_TYPE_PG:          "PG",
		core.PackageType_PACKAGE_TYPE_MYSQL:       "MYSQL",
		core.PackageType_PACKAGE_TYPE_MONGODB:     "MONGODB",
		core.PackageType_PACKAGE_TYPE_REDIS:       "REDIS",
		core.PackageType_PACKAGE_TYPE_KAFKA:       "KAFKA",
		core.PackageType_PACKAGE_TYPE_RABBITMQ:    "RABBITMQ",
		core.PackageType_PACKAGE_TYPE_UNSPECIFIED: "",
		core.PackageType(999):                     "",
	}

	for pkgType, expected := range cases {
		assert.Equal(t, expected, packageTypeToString(pkgType))
	}
}
