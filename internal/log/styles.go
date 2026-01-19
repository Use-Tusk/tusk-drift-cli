package log

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
)

// DeviationStyle is orange for deviation messages
var DeviationStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))

// renderError applies error styling to a message
func renderError(msg string) string {
	if styles.NoColor() {
		return msg
	}
	return styles.ErrorStyle.Render(msg)
}

// renderWarning applies warning styling to a message
func renderWarning(msg string) string {
	if styles.NoColor() {
		return msg
	}
	return styles.WarningStyle.Render(msg)
}

// renderSuccess applies success styling to a message
func renderSuccess(msg string) string {
	if styles.NoColor() {
		return msg
	}
	return styles.SuccessStyle.Render(msg)
}

// renderDim applies dim/progress styling to a message
func renderDim(msg string) string {
	if styles.NoColor() {
		return msg
	}
	return styles.DimStyle.Render(msg)
}

// renderDeviation applies deviation/orange styling to a message
func renderDeviation(msg string) string {
	if styles.NoColor() {
		return msg
	}
	return DeviationStyle.Render(msg)
}
