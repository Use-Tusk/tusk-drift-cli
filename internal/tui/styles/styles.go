//nolint:unused
package styles

import (
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/Use-Tusk/tusk-drift-cli/internal/cliconfig"
)

var (
	HasDarkBackground = initDarkBackground()

	PrimaryColor = func() string {
		if HasDarkBackground {
			return "213"
		}
		return "53"
	}()

	SecondaryColor = "55"

	WarningColor = "214"
)

var (
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(PrimaryColor)).
			MarginBottom(1)

	HeadingStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(PrimaryColor))

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
			Foreground(lipgloss.Color(WarningColor))

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
			Padding(1, 2)

	InfoBoxStyle = BoxStyle.
			BorderForeground(lipgloss.Color(PrimaryColor))

	WarningBoxStyle = BoxStyle.
			BorderForeground(lipgloss.Color(WarningColor))

	TextCenterStyle = lipgloss.NewStyle().Align(lipgloss.Center)

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

// HuhTheme returns a huh theme using our style system
func HuhTheme() *huh.Theme {
	t := huh.ThemeBase()

	primary := lipgloss.Color(PrimaryColor)

	// Title styling - bold and underlined, default color
	t.Focused.Title = lipgloss.NewStyle().Bold(true).Underline(true)

	// Remove the vertical line on the left (base border)
	t.Focused.Base = lipgloss.NewStyle().PaddingLeft(0)
	t.Blurred.Base = lipgloss.NewStyle().PaddingLeft(0)

	// Selection styling - ">" indicator in primary color
	t.Focused.SelectSelector = lipgloss.NewStyle().Foreground(primary).SetString("> ")
	t.Focused.SelectedOption = lipgloss.NewStyle().Foreground(primary)

	return t
}

// initDarkBackground determines if dark background should be used from config.
func initDarkBackground() bool {
	cfg := cliconfig.CLIConfig
	if cfg.DarkMode == nil {
		return lipgloss.HasDarkBackground()
	}
	return *cfg.DarkMode
}
