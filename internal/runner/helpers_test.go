package runner

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
)

// makeStruct creates a structpb.Struct from a map, failing the test if there's an error.
func makeStruct(t *testing.T, m map[string]any) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(m)
	require.NoError(t, err)
	return s
}

// makeResponse creates an HTTP response for testing.
func makeResponse(status int, headers map[string]string, body string) *http.Response {
	h := http.Header{}
	for k, v := range headers {
		h.Set(k, v)
	}
	return &http.Response{
		StatusCode: status,
		Header:     h,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}

// jsonAny parses a JSON string into an any type, failing the test if parsing fails.
func jsonAny(t *testing.T, s string) any {
	t.Helper()
	var v any
	require.NoError(t, json.Unmarshal([]byte(s), &v))
	return v
}

// writeTempConfig writes a YAML config to a temporary directory and returns the path.
func writeTempConfig(t *testing.T, yaml string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "tusk.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))
	return path
}
