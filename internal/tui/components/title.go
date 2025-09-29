package components

import (
	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
	"github.com/charmbracelet/lipgloss"
)

func Title(width int, text string) string {
	styledText := "• " + text + " •"
	if width > 0 {
		return styles.TitleStyle.
			Width(width).
			Align(lipgloss.Center).
			Render(styledText)
	}
	return styles.TitleStyle.Render(styledText)
}
