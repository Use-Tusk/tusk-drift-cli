//nolint:gosec
package utils

import (
	"os"
	"path/filepath"
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
	t.Setenv("HOME", home)

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
	require.NoError(t, os.MkdirAll(filepath.Join(".tusk", "traces"), 0o750))

	name := "foo.jsonl"
	full := filepath.Join(".tusk", "traces", name)
	require.NoError(t, os.WriteFile(full, []byte("{}\n"), 0o644))

	got, err := FindTraceFile("does-not-matter", name)
	require.NoError(t, err)
	assert.Equal(t, full, got)
}

func TestFindTraceFile_ExplicitFilenameAbsolute(t *testing.T) {
	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })

	tmp := t.TempDir()
	require.NoError(t, os.Chdir(tmp))
	require.NoError(t, os.MkdirAll(filepath.Join(".tusk", "traces"), 0o750))

	full := filepath.Join(".tusk", "traces", "bar.jsonl")
	require.NoError(t, os.WriteFile(full, []byte("{}\n"), 0o644))

	got, err := FindTraceFile("irrelevant", full)
	require.NoError(t, err)
	assert.Equal(t, full, got)
}

func TestFindTraceFile_FindsByTraceIDNested(t *testing.T) {
	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })

	tmp := t.TempDir()
	require.NoError(t, os.Chdir(tmp))
	require.NoError(t, os.MkdirAll(filepath.Join(".tusk", "traces", "nested", "x"), 0o750))

	traceID := "abc123"
	target := filepath.Join(".tusk", "traces", "nested", "x", "2025-01-01_"+traceID+".jsonl")
	require.NoError(t, os.WriteFile(target, []byte("{}\n"), 0o644))

	got, err := FindTraceFile(traceID, "")
	require.NoError(t, err)
	assert.Equal(t, target, got)
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
