package utils

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/pmezard/go-difflib/difflib"
)

const NoWrapMarker = "\x00NOWRAP\x00"

// MarkNonWrappable adds an invisible marker to indicate text should not be wrapped
func MarkNonWrappable(text string) string {
	return NoWrapMarker + text
}

// StripNoWrapMarker removes the non-wrappable marker from text before display
func StripNoWrapMarker(text string) string {
	return strings.ReplaceAll(text, NoWrapMarker, "")
}

// ANSI color code regex pattern
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// stripANSI removes ANSI escape sequences from a string
func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// visibleLen returns the visible length of a string (excluding ANSI codes)
func visibleLen(s string) int {
	return len(stripANSI(s))
}

// WrapLine wraps a single line of text to the specified width, trying to break at word boundaries
// This function is ANSI-aware and preserves color codes
// It avoids wrapping lines that appear to be formatted output (like diffs)
func WrapLine(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		maxWidth = 80
	}

	// Don't wrap lines marked as non-wrappable (used for diffs and structured output)
	// Strip the marker immediately after checking
	if strings.Contains(text, NoWrapMarker) {
		return []string{strings.ReplaceAll(text, NoWrapMarker, "")}
	}

	// Check visible length, not actual length (excluding ANSI codes)
	if visibleLen(text) <= maxWidth {
		return []string{text}
	}

	// Preserve leading whitespace
	leadingSpaces := len(text) - len(strings.TrimLeft(text, " "))
	leadingWhitespace := text[:leadingSpaces]
	trimmedText := text[leadingSpaces:]

	var lines []string
	currentLine := leadingWhitespace
	currentVisibleLen := leadingSpaces

	// Split by spaces to wrap at word boundaries
	words := strings.Fields(trimmedText)

	// If there are no spaces (single long word) and it's too long, split it
	if len(words) == 1 && visibleLen(words[0]) > maxWidth-leadingSpaces {
		chunks := splitLongWord(words[0], maxWidth-leadingSpaces)
		for _, chunk := range chunks {
			lines = append(lines, leadingWhitespace+chunk)
		}
		return lines
	}

	for i, word := range words {
		wordVisibleLen := visibleLen(word)
		spaceLen := 1 // Space between words

		// If this individual word is too long to fit even on its own line, split it
		if wordVisibleLen > maxWidth-leadingSpaces {
			// Save current line if it has content
			if currentLine != leadingWhitespace {
				lines = append(lines, currentLine)
			}

			// Split the long word
			chunks := splitLongWord(word, maxWidth-leadingSpaces)
			for j, chunk := range chunks {
				if j < len(chunks)-1 {
					// All chunks except the last get their own line
					lines = append(lines, leadingWhitespace+chunk)
				} else {
					// Last chunk starts the next line
					currentLine = leadingWhitespace + chunk
					currentVisibleLen = leadingSpaces + visibleLen(chunk)
				}
			}
			continue
		}

		switch {
		case i == 0:
			// First word, no leading space
			currentLine += word
			currentVisibleLen += wordVisibleLen
		case currentVisibleLen+spaceLen+wordVisibleLen <= maxWidth:
			// Word fits on current line
			currentLine += " " + word
			currentVisibleLen += spaceLen + wordVisibleLen
		default:
			// Word doesn't fit, start new line
			lines = append(lines, currentLine)
			currentLine = leadingWhitespace + word
			currentVisibleLen = leadingSpaces + wordVisibleLen
		}
	}

	// Add last line
	if currentLine != "" && currentLine != leadingWhitespace {
		lines = append(lines, currentLine)
	}

	if len(lines) == 0 {
		return []string{text}
	}

	return lines
}

// splitLongWord splits a word that's longer than maxWidth into chunks
// This is ANSI-aware and tries to preserve ANSI codes properly
func splitLongWord(word string, maxWidth int) []string {
	if maxWidth <= 0 {
		maxWidth = 80
	}

	// For simplicity with ANSI codes, we'll strip them, split, then try to preserve
	// This is a basic implementation that works for most cases
	stripped := stripANSI(word)
	if len(stripped) <= maxWidth {
		return []string{word}
	}

	// If there are no ANSI codes, simple split
	if word == stripped {
		var chunks []string
		for len(stripped) > 0 {
			if len(stripped) <= maxWidth {
				chunks = append(chunks, stripped)
				break
			}
			chunks = append(chunks, stripped[:maxWidth])
			stripped = stripped[maxWidth:]
		}
		return chunks
	}

	// TODO: Handle ANSI codes more carefully if needed
	// For now, just do a basic split ignoring ANSI codes
	var chunks []string
	for len(stripped) > 0 {
		if len(stripped) <= maxWidth {
			chunks = append(chunks, stripped)
			break
		}
		chunks = append(chunks, stripped[:maxWidth])
		stripped = stripped[maxWidth:]
	}
	return chunks
}

// WrapLogs wraps multiple lines of log content to fit within the specified width
func WrapText(content string, maxWidth int) string {
	if maxWidth <= 0 {
		maxWidth = 80
	}

	lines := strings.Split(content, "\n")
	var wrappedLines []string

	for _, line := range lines {
		wrapped := WrapLine(line, maxWidth)
		wrappedLines = append(wrappedLines, wrapped...)
	}

	return strings.Join(wrappedLines, "\n")
}

// FormatJSONForDiff formats a JSON value with proper indentation for diff display
func FormatJSONForDiff(v any) string {
	if v == nil {
		return "<nil>"
	}

	// If it's already a string, try to parse it as JSON first
	if str, ok := v.(string); ok {
		var parsed any
		if err := json.Unmarshal([]byte(str), &parsed); err == nil {
			v = parsed
		} else {
			// Not valid JSON, return as is
			return str
		}
	}

	// Pretty print the JSON
	b, err := json.MarshalIndent(v, "      ", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}

	return string(b)
}

// FormatJSONDiff creates a git-style unified diff between two JSON values
func FormatJSONDiff(expected, actual any) string {
	expectedJSON := formatJSONForDiff(expected)
	actualJSON := formatJSONForDiff(actual)

	if expectedJSON == actualJSON {
		return "No differences found"
	}

	// Generate unified diff
	diff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(expectedJSON),
		B:        difflib.SplitLines(actualJSON),
		FromFile: "Expected",
		ToFile:   "Actual",
		Context:  5,
	}

	result, err := difflib.GetUnifiedDiffString(diff)
	if err != nil {
		// Fallback to simple side-by-side if diff fails
		return fmt.Sprintf("Expected:\n%s\n\nActual:\n%s", expectedJSON, actualJSON)
	}

	red := "\033[31m"
	green := "\033[32m"
	cyan := "\033[36m"
	gray := "\033[38;5;250m"
	reset := "\033[0m"

	lines := strings.Split(result, "\n")
	var indentedLines []string

	topDelimiter := "  " + gray + "╭─" + strings.Repeat("─", 22) + " Diff " + strings.Repeat("─", 22) + "" + reset
	bottomDelimiter := "  " + gray + "╰─" + strings.Repeat("─", 50) + reset

	indentedLines = append(indentedLines, MarkNonWrappable(topDelimiter))

	for _, line := range lines {
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++"):
			indentedLines = append(indentedLines, MarkNonWrappable("    "+cyan+line+reset))
		case strings.HasPrefix(line, "-"):
			indentedLines = append(indentedLines, MarkNonWrappable("    "+red+line+reset))
		case strings.HasPrefix(line, "+"):
			indentedLines = append(indentedLines, MarkNonWrappable("    "+green+line+reset))
		case strings.HasPrefix(line, "@@"):
			indentedLines = append(indentedLines, MarkNonWrappable("    "+cyan+line+reset))
		default:
			indentedLines = append(indentedLines, MarkNonWrappable("    "+gray+line+reset))
		}
	}

	indentedLines = append(indentedLines, MarkNonWrappable(bottomDelimiter))
	return strings.Join(indentedLines, "\n")
}

// formatJSONForDiff is a helper that formats JSON without the extra indentation prefix
func formatJSONForDiff(v any) string {
	if v == nil {
		return "<nil>"
	}

	if str, ok := v.(string); ok {
		var parsed any
		if err := json.Unmarshal([]byte(str), &parsed); err == nil {
			v = parsed
		} else {
			// Not valid JSON, return as is
			return str
		}
	}

	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}

	return string(b)
}

func TruncateWithEllipsis(text string, maxWidth int) string {
	visibleLength := runewidth.StringWidth(stripANSI(text))

	if visibleLength <= maxWidth {
		return text
	}

	if maxWidth <= 3 {
		return "..."
	}

	targetWidth := maxWidth - 3

	// For ANSI-aware truncation, we preserve escape sequences
	// while only counting visible characters
	var result strings.Builder
	displayWidth := 0
	i := 0

	for i < len(text) && displayWidth < targetWidth {
		// Check for ANSI escape sequence
		if i+1 < len(text) && text[i] == '\x1b' && text[i+1] == '[' {
			// Copy the entire ANSI sequence without counting it
			j := i + 2
			for j < len(text) && text[j] != 'm' {
				j++
			}
			if j < len(text) {
				j++ // Include the 'm'
			}
			result.WriteString(text[i:j])
			i = j
			continue
		}

		// Regular character - decode and count display width
		r, size := utf8.DecodeRuneInString(text[i:])
		if r != utf8.RuneError {
			charWidth := runewidth.RuneWidth(r)
			// Only add the character if it fits
			if displayWidth+charWidth <= targetWidth {
				result.WriteRune(r)
				displayWidth += charWidth
			} else {
				// Character would exceed limit, stop here
				break
			}
		}
		i += size
	}

	return result.String() + "..."
}
