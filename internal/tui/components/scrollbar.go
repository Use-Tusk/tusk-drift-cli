package components

import (
	"strings"

	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
)

// RenderScrollbar renders a vertical scrollbar given the visible height,
// total content lines, and current scroll offset.
func RenderScrollbar(height, totalLines, scrollOffset int) string {
	// If content fits, show empty space (no scrollbar needed)
	if totalLines <= height {
		var sb strings.Builder
		for i := range height {
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(" ")
		}
		return sb.String()
	}

	thumbSize := max(1, height*height/totalLines)
	scrollableLines := totalLines - height

	// Clamp scroll offset
	if scrollOffset > scrollableLines {
		scrollOffset = scrollableLines
	}
	if scrollOffset < 0 {
		scrollOffset = 0
	}

	scrollProgress := float64(scrollOffset) / float64(scrollableLines)
	thumbStart := int(scrollProgress * float64(height-thumbSize))

	if thumbStart+thumbSize > height {
		thumbStart = height - thumbSize
	}

	var sb strings.Builder
	for i := range height {
		if i > 0 {
			sb.WriteString("\n")
		}
		if i >= thumbStart && i < thumbStart+thumbSize {
			sb.WriteString(styles.ScrollbarThumbStyle.Render("┃"))
		} else {
			sb.WriteString(styles.ScrollbarTrackStyle.Render("│"))
		}
	}

	return sb.String()
}
