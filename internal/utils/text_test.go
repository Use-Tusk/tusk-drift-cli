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
