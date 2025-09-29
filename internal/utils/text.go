package utils

import "strings"

// WrapText wraps a single line of text to the specified width, trying to break at word boundaries
func WrapLine(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		maxWidth = 80
	}

	if len(text) <= maxWidth {
		return []string{text}
	}

	var lines []string
	for len(text) > maxWidth {
		breakPoint := maxWidth
		// Try to break at a space to avoid breaking words
		for i := maxWidth - 1; i >= maxWidth/2; i-- {
			if text[i] == ' ' {
				breakPoint = i
				break
			}
		}

		lines = append(lines, text[:breakPoint])
		text = text[breakPoint:]
		// Remove leading space from the next line
		if len(text) > 0 && text[0] == ' ' {
			text = text[1:]
		}
	}

	if len(text) > 0 {
		lines = append(lines, text)
	}

	return lines
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
