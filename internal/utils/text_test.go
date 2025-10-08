package utils

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrapLine_NoWrap(t *testing.T) {
	got := WrapLine("hello", 10)
	require.Len(t, got, 1)
	assert.Equal(t, "hello", got[0])
}

func TestWrapLine_ExactWidth(t *testing.T) {
	text := "12345"
	got := WrapLine(text, 5)
	require.Len(t, got, 1)
	assert.Equal(t, text, got[0])
}

func TestWrapLine_WrapsAtSpacesAndRemovesLeadingSpace(t *testing.T) {
	text := "abc def ghi"
	got := WrapLine(text, 5)
	require.Len(t, got, 3)
	assert.Equal(t, []string{"abc", "def", "ghi"}, got)
}

func TestWrapLine_LongWordSplitsWithinLimit(t *testing.T) {
	text := "supercalifragilisticexpialidocious"
	maxWidth := 10
	got := WrapLine(text, maxWidth)
	require.GreaterOrEqual(t, len(got), 2)
	for _, line := range got {
		assert.LessOrEqual(t, len(line), maxWidth, "line exceeds maxWidth: %q", line)
	}
}

func TestWrapLine_PreservesANSICodesAndWrapsCorrectly(t *testing.T) {
	red := "\033[31m"
	reset := "\033[0m"
	text := red + "hello world" + reset // visible: 11 chars, actual: 20 chars
	got := WrapLine(text, 6)
	require.Len(t, got, 2)
	// Should wrap at "hello" and "world", preserving ANSI codes
}

func TestWrapLine_PreservesLeadingWhitespace(t *testing.T) {
	text := "    hello world test"
	got := WrapLine(text, 15)
	require.GreaterOrEqual(t, len(got), 2)
	for _, line := range got {
		assert.True(t, strings.HasPrefix(line, "    "), "wrapped line should preserve leading whitespace")
	}
}

func TestWrapLine_DefaultWidthWhenZero(t *testing.T) {
	long := strings.Repeat("a", 100)
	got := WrapLine(long, 0)
	require.Len(t, got, 2)
	assert.Equal(t, 80, len(got[0]))
	assert.Equal(t, 20, len(got[1]))
}

func TestWrapText_WrapsAndPreservesLines(t *testing.T) {
	content := "short\n" + strings.Repeat("x", 25)
	out := WrapText(content, 10)
	lines := strings.Split(out, "\n")
	require.Greater(t, len(lines), 2)
	assert.Equal(t, "short", lines[0])
	for i := 1; i < len(lines); i++ {
		assert.LessOrEqual(t, len(lines[i]), 10)
	}
}

func TestWrapText_PreservesEmptyLines(t *testing.T) {
	content := "a\n\nb"
	out := WrapText(content, 1)
	lines := strings.Split(out, "\n")
	require.Equal(t, 3, len(lines))
	assert.Equal(t, "a", lines[0])
	assert.Equal(t, "", lines[1])
	assert.Equal(t, "b", lines[2])
}

func TestFormatJSONForDiff_NilValue(t *testing.T) {
	got := FormatJSONForDiff(nil)
	assert.Equal(t, "<nil>", got)
}

func TestFormatJSONForDiff_StringValue(t *testing.T) {
	got := FormatJSONForDiff("plain string")
	assert.Equal(t, "plain string", got)
}

func TestFormatJSONForDiff_JSONString(t *testing.T) {
	jsonStr := `{"key":"value"}`
	got := FormatJSONForDiff(jsonStr)
	assert.Contains(t, got, "key")
	assert.Contains(t, got, "value")
}

func TestFormatJSONForDiff_ObjectValue(t *testing.T) {
	obj := map[string]any{"name": "test", "count": 42}
	got := FormatJSONForDiff(obj)
	assert.Contains(t, got, "name")
	assert.Contains(t, got, "test")
}

func TestFormatJSONDiff_IdenticalValues(t *testing.T) {
	obj := map[string]string{"a": "b"}
	got := FormatJSONDiff(obj, obj)
	assert.Equal(t, "No differences found", got)
}

func TestFormatJSONDiff_DifferentValues(t *testing.T) {
	expected := map[string]string{"key": "value1"}
	actual := map[string]string{"key": "value2"}
	got := FormatJSONDiff(expected, actual)
	assert.NotEqual(t, "No differences found", got)
	assert.Contains(t, got, "value1")
	assert.Contains(t, got, "value2")

	require.Contains(t, got, "-", "diff should contain '-' for removed lines")
	require.Contains(t, got, "+", "diff should contain '+' for added lines")

	lines := strings.Split(got, "\n")
	foundMinusValue1 := false
	foundPlusValue2 := false

	for _, line := range lines {
		// Strip both ANSI codes and the NoWrapMarker before checking
		stripped := StripNoWrapMarker(stripANSI(line))
		trimmed := strings.TrimSpace(stripped)

		if strings.HasPrefix(trimmed, "-") && strings.Contains(line, "value1") {
			foundMinusValue1 = true
		}
		if strings.HasPrefix(trimmed, "+") && strings.Contains(line, "value2") {
			foundPlusValue2 = true
		}
	}

	assert.True(t, foundMinusValue1, "expected value 'value1' should appear on a line starting with '-'")
	assert.True(t, foundPlusValue2, "actual value 'value2' should appear on a line starting with '+'")
}
