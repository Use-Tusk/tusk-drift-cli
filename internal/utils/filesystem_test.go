package utils

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTuskDir_PrefersLocalDotDir(t *testing.T) {
	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })

	tmp := t.TempDir()
	require.NoError(t, os.Chdir(tmp))
	require.NoError(t, os.MkdirAll(".tusk", 0o750))

	got := GetTuskDir()
	assert.Equal(t, ".tusk", got)
}

func TestGetTuskDir_FallsBackToHome(t *testing.T) {
	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })

	tmp := t.TempDir()
	home := t.TempDir()

	// Set both HOME and USERPROFILE for cross-platform compatibility
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	}

	require.NoError(t, os.Chdir(tmp)) // No .tusk here
	got := GetTuskDir()
	assert.Equal(t, filepath.Join(home, ".tusk"), got)
}

func TestGetTracesAndLogsDir(t *testing.T) {
	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })

	tmp := t.TempDir()
	require.NoError(t, os.Chdir(tmp))
	require.NoError(t, os.MkdirAll(".tusk", 0o750))

	assert.Equal(t, filepath.Join(".tusk", "traces"), GetTracesDir())
	assert.Equal(t, filepath.Join(".tusk", "logs"), GetLogsDir())
}

func TestEnsureDir_CreatesAndIdempotent(t *testing.T) {
	tmp := t.TempDir()
	nested := filepath.Join(tmp, "a", "b", "c")

	require.NoError(t, EnsureDir(nested))

	info, err := os.Stat(nested)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	// Idempotent
	require.NoError(t, EnsureDir(nested))
}

func TestFindTraceFile_TracesDirMissing(t *testing.T) {
	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })

	tmp := t.TempDir()
	require.NoError(t, os.Chdir(tmp)) // no .tusk/traces

	_, err := FindTraceFile("abc123", "")
	require.Error(t, err)
}

func TestFindTraceFile_ExplicitFilenameRelative(t *testing.T) {
	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })

	tmp := t.TempDir()
	require.NoError(t, os.Chdir(tmp))
	tracesDir := filepath.Join(".tusk", "traces")
	require.NoError(t, os.MkdirAll(tracesDir, 0o750))

	name := "foo.jsonl"
	full := filepath.Join(tracesDir, name)
	require.NoError(t, os.WriteFile(full, []byte("{}\n"), 0o600))

	got, err := FindTraceFile("does-not-matter", name)
	require.NoError(t, err)
	assert.Equal(t, full, got)
}

func TestFindTraceFile_ExplicitFilenameAbsolute(t *testing.T) {
	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })

	tmp := t.TempDir()
	require.NoError(t, os.Chdir(tmp))
	tracesDir := filepath.Join(".tusk", "traces")
	require.NoError(t, os.MkdirAll(tracesDir, 0o750))

	full := filepath.Join(tracesDir, "bar.jsonl")
	require.NoError(t, os.WriteFile(full, []byte("{}\n"), 0o600))

	got, err := FindTraceFile("irrelevant", full)
	require.NoError(t, err)
	assert.Equal(t, full, got)
}

func TestFindTraceFile_FindsByTraceIDNested(t *testing.T) {
	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })

	tmp := t.TempDir()
	require.NoError(t, os.Chdir(tmp))
	tracesDir := filepath.Join(".tusk", "traces", "nested", "x")
	require.NoError(t, os.MkdirAll(tracesDir, 0o750))

	traceID := "abc123"
	target := filepath.Join(tracesDir, "2025-01-01_"+traceID+".jsonl")
	require.NoError(t, os.WriteFile(target, []byte("{}\n"), 0o600))

	got, err := FindTraceFile(traceID, "")
	require.NoError(t, err)

	// Use filepath.Join to normalize for comparison (Windows vs Unix slashes)
	expectedBase := filepath.Join(".tusk", "traces", "nested", "x", "2025-01-01_"+traceID+".jsonl")
	assert.Equal(t, expectedBase, got)
}

func TestFindTraceFile_NotFound(t *testing.T) {
	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })

	tmp := t.TempDir()
	require.NoError(t, os.Chdir(tmp))
	require.NoError(t, os.MkdirAll(filepath.Join(".tusk", "traces"), 0o750))

	_, err := FindTraceFile("nope", "")
	require.Error(t, err)
}
