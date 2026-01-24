package components

import (
	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
	"github.com/charmbracelet/lipgloss"
)

func Footer(width int, helpText string) string {
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(styles.PrimaryColor)).
		Background(lipgloss.Color(styles.SubtleBgColor)).
		Width(width)
	return helpStyle.Render(helpText)
}
