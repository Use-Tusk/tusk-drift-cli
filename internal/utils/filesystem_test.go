package utils

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// evalSymlinks is a helper that resolves symlinks for path comparison.
// On macOS, /var is a symlink to /private/var which causes test failures.
func evalSymlinks(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		// If we can't resolve, just return the original
		return path
	}
	return resolved
}

func TestFindTuskRoot_CurrentDirectory(t *testing.T) {
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()

	tmp := evalSymlinks(t.TempDir())
	require.NoError(t, os.Chdir(tmp))
	require.NoError(t, os.MkdirAll(".tusk", 0o750))

	got := FindTuskRoot()
	assert.Equal(t, tmp, got)
}

func TestFindTuskRoot_ParentDirectory(t *testing.T) {
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()

	tmp := evalSymlinks(t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, ".tusk"), 0o750))

	// Create nested subdirectories
	subdir := filepath.Join(tmp, "a", "b", "c")
	require.NoError(t, os.MkdirAll(subdir, 0o750))
	require.NoError(t, os.Chdir(subdir))

	got := FindTuskRoot()
	assert.Equal(t, tmp, got)
}

func TestFindTuskRoot_MultipleNestedLevels(t *testing.T) {
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()

	tmp := evalSymlinks(t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, ".tusk"), 0o750))

	// Create deeply nested structure
	deepDir := filepath.Join(tmp, "one", "two", "three", "four", "five")
	require.NoError(t, os.MkdirAll(deepDir, 0o750))
	require.NoError(t, os.Chdir(deepDir))

	got := FindTuskRoot()
	assert.Equal(t, tmp, got)
}

func TestFindTuskRoot_NotFound(t *testing.T) {
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()

	tmp := evalSymlinks(t.TempDir())
	require.NoError(t, os.Chdir(tmp)) // No .tusk directory

	got := FindTuskRoot()
	assert.Equal(t, "", got)
}

func TestFindTuskRoot_ClosestWins(t *testing.T) {
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()

	// Create structure: tmp/.tusk and tmp/nested/.tusk
	tmp := evalSymlinks(t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, ".tusk"), 0o750))

	nestedRoot := filepath.Join(tmp, "nested")
	require.NoError(t, os.MkdirAll(filepath.Join(nestedRoot, ".tusk"), 0o750))

	subdir := filepath.Join(nestedRoot, "sub")
	require.NoError(t, os.MkdirAll(subdir, 0o750))
	require.NoError(t, os.Chdir(subdir))

	// Should find the closest .tusk (in nested/, not tmp/)
	got := FindTuskRoot()
	assert.Equal(t, nestedRoot, got)
}

func TestGetTuskDir_WithParentTraversal(t *testing.T) {
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()

	tmp := evalSymlinks(t.TempDir())
	tuskDir := filepath.Join(tmp, ".tusk")
	require.NoError(t, os.MkdirAll(tuskDir, 0o750))

	// Work from a subdirectory
	subdir := filepath.Join(tmp, "src", "components")
	require.NoError(t, os.MkdirAll(subdir, 0o750))
	require.NoError(t, os.Chdir(subdir))

	got := GetTuskDir()
	assert.Equal(t, tuskDir, got)
}

func TestGetTuskRoot_ReturnsRootDirectory(t *testing.T) {
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()

	tmp := evalSymlinks(t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, ".tusk"), 0o750))

	subdir := filepath.Join(tmp, "nested")
	require.NoError(t, os.MkdirAll(subdir, 0o750))
	require.NoError(t, os.Chdir(subdir))

	got := GetTuskRoot()
	assert.Equal(t, tmp, got)
}

func TestGetTuskRoot_FallsBackToCurrentDir(t *testing.T) {
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()

	tmp := evalSymlinks(t.TempDir())
	require.NoError(t, os.Chdir(tmp)) // No .tusk here

	got := GetTuskRoot()
	assert.Equal(t, tmp, got)
}

func TestResolveTuskPath_AbsolutePath(t *testing.T) {
	absPath := "/absolute/path/to/something"
	got := ResolveTuskPath(absPath)
	assert.Equal(t, absPath, got)
}

func TestResolveTuskPath_RelativePath(t *testing.T) {
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()

	tmp := evalSymlinks(t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, ".tusk"), 0o750))

	subdir := filepath.Join(tmp, "src", "handlers")
	require.NoError(t, os.MkdirAll(subdir, 0o750))
	require.NoError(t, os.Chdir(subdir))

	got := ResolveTuskPath(".tusk/results")
	expected := filepath.Join(tmp, ".tusk/results")
	assert.Equal(t, expected, got)
}

func TestResolveTuskPath_EmptyString(t *testing.T) {
	got := ResolveTuskPath("")
	assert.Equal(t, "", got)
}

func TestResolveTuskPath_FromNestedDirectory(t *testing.T) {
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()

	tmp := evalSymlinks(t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, ".tusk"), 0o750))

	deepDir := filepath.Join(tmp, "a", "b", "c", "d")
	require.NoError(t, os.MkdirAll(deepDir, 0o750))
	require.NoError(t, os.Chdir(deepDir))

	got := ResolveTuskPath("custom/traces")
	expected := filepath.Join(tmp, "custom/traces")
	assert.Equal(t, expected, got)
}

func TestGetTuskDir_PrefersLocalDotDir(t *testing.T) {
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()

	tmp := evalSymlinks(t.TempDir())
	require.NoError(t, os.Chdir(tmp))
	tuskDir := filepath.Join(tmp, ".tusk")
	require.NoError(t, os.MkdirAll(tuskDir, 0o750))

	got := GetTuskDir()
	assert.Equal(t, tuskDir, got)
}

func TestGetTuskDir_FallsBackToHome(t *testing.T) {
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()

	tmp := evalSymlinks(t.TempDir())
	home := evalSymlinks(t.TempDir())

	// Create .tusk in home directory
	homeTuskDir := filepath.Join(home, ".tusk")
	require.NoError(t, os.MkdirAll(homeTuskDir, 0o750))

	// Set both HOME and USERPROFILE for cross-platform compatibility
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	}

	require.NoError(t, os.Chdir(tmp)) // No .tusk here or in parents
	got := GetTuskDir()
	assert.Equal(t, homeTuskDir, got)
}

func TestGetTracesAndLogsDir(t *testing.T) {
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()

	tmp := evalSymlinks(t.TempDir())
	require.NoError(t, os.Chdir(tmp))
	require.NoError(t, os.MkdirAll(".tusk", 0o750))

	expectedTraces := filepath.Join(tmp, ".tusk", "traces")
	expectedLogs := filepath.Join(tmp, ".tusk", "logs")

	assert.Equal(t, expectedTraces, GetTracesDir())
	assert.Equal(t, expectedLogs, GetLogsDir())
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
	defer func() { _ = os.Chdir(wd) }()

	tmp := evalSymlinks(t.TempDir())
	require.NoError(t, os.Chdir(tmp)) // no .tusk/traces

	_, err := FindTraceFile("abc123", "")
	require.Error(t, err)
}

func TestFindTraceFile_ExplicitFilenameRelative(t *testing.T) {
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()

	tmp := evalSymlinks(t.TempDir())
	require.NoError(t, os.Chdir(tmp))
	tracesDir := filepath.Join(".tusk", "traces")
	require.NoError(t, os.MkdirAll(tracesDir, 0o750))

	name := "foo.jsonl"
	full := filepath.Join(tracesDir, name)
	require.NoError(t, os.WriteFile(full, []byte("{}\n"), 0o600))

	got, err := FindTraceFile("does-not-matter", name)
	require.NoError(t, err)

	expectedFull := filepath.Join(tmp, tracesDir, name)
	assert.Equal(t, expectedFull, got)
}

func TestFindTraceFile_ExplicitFilenameAbsolute(t *testing.T) {
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()

	tmp := evalSymlinks(t.TempDir())
	require.NoError(t, os.Chdir(tmp))
	tracesDir := filepath.Join(tmp, ".tusk", "traces")
	require.NoError(t, os.MkdirAll(tracesDir, 0o750))

	full := filepath.Join(tracesDir, "bar.jsonl")
	require.NoError(t, os.WriteFile(full, []byte("{}\n"), 0o600))

	got, err := FindTraceFile("irrelevant", full)
	require.NoError(t, err)
	assert.Equal(t, full, got)
}

func TestFindTraceFile_FindsByTraceIDNested(t *testing.T) {
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()

	tmp := evalSymlinks(t.TempDir())
	require.NoError(t, os.Chdir(tmp))
	tracesDir := filepath.Join(".tusk", "traces", "nested", "x")
	require.NoError(t, os.MkdirAll(tracesDir, 0o750))

	traceID := "abc123"
	target := filepath.Join(tracesDir, "2025-01-01_"+traceID+".jsonl")
	require.NoError(t, os.WriteFile(target, []byte("{}\n"), 0o600))

	got, err := FindTraceFile(traceID, "")
	require.NoError(t, err)

	// Use filepath.Join to normalize for comparison (Windows vs Unix slashes)
	expectedBase := filepath.Join(tmp, ".tusk", "traces", "nested", "x", "2025-01-01_"+traceID+".jsonl")
	assert.Equal(t, expectedBase, got)
}

func TestFindTraceFile_NotFound(t *testing.T) {
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()

	tmp := evalSymlinks(t.TempDir())
	require.NoError(t, os.Chdir(tmp))
	require.NoError(t, os.MkdirAll(filepath.Join(".tusk", "traces"), 0o750))

	_, err := FindTraceFile("nope", "")
	require.Error(t, err)
}
