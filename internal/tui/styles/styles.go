//nolint:unused
package styles

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

var (
	HasDarkBackground = lipgloss.HasDarkBackground()

	PrimaryColor = func() string {
		if HasDarkBackground {
			return "213"
		}
		return "53"
	}()

	SecondaryColor = "55"
)

var (
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(PrimaryColor)).
			MarginBottom(1)

	SuccessStyle = func() lipgloss.Style {
		color := "34"
		if HasDarkBackground {
			color = "42"
		}
		return lipgloss.NewStyle().Foreground(lipgloss.Color(color))
	}()

	ErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	WarningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))

	DimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	HelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	SelectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")).
			Bold(true)

	TableRowSelectedStyle = func() lipgloss.Style {
		foreground := "231"
		if HasDarkBackground {
			foreground = "229"
		}
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(foreground)).
			Background(lipgloss.Color(SecondaryColor)).
			Bold(false)
	}()

	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(PrimaryColor)).
			Padding(1, 2)

	InputStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color(PrimaryColor))

	FocusedInputStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("205"))

	TableHeaderStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("240")).
				BorderBottom(true).
				Bold(true).
				PaddingLeft(1).
				PaddingRight(1)

	TableCellStyle = lipgloss.NewStyle().
			PaddingLeft(1).
			PaddingRight(1)

	LinkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("32")).Bold(true).Underline(true)
)

func NoColor() bool {
	return termenv.EnvNoColor()
}
