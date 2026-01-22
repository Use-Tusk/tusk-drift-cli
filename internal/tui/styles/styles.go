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

	// BorderColor is used for borders and dividers
	BorderColor = "240"

	// AccentColor is used for highlights and focus indicators
	AccentColor = "205"

	// SubtleBgColor is used for subtle background highlights
	SubtleBgColor = func() string {
		if HasDarkBackground {
			return "236"
		}
		return "254"
	}()

	// ErrorColor is used for error states
	ErrorColor = "196"

	// SuccessColor is used for success states
	SuccessColor = func() string {
		if HasDarkBackground {
			return "42"
		}
		return "34"
	}()

	// LinkColor is used for hyperlinks
	LinkColor = "32"
)

var (
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(PrimaryColor)).
			MarginBottom(1)

	HeadingStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(PrimaryColor))

	SuccessStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(SuccessColor))

	ErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(ErrorColor))

	WarningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(WarningColor))

	DimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	HelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	// BorderDimStyle is used for borders and panel titles
	BorderDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(BorderColor))

	SelectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(AccentColor)).
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
				BorderForeground(lipgloss.Color(AccentColor))

	TableHeaderStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color(BorderColor)).
				BorderBottom(true).
				Bold(true).
				PaddingLeft(1).
				PaddingRight(1)

	TableCellStyle = lipgloss.NewStyle().
			PaddingLeft(1).
			PaddingRight(1)

	LinkStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(LinkColor)).
			Bold(true).
			Underline(true)

	// ScrollbarThumbStyle is for the scrollbar thumb/handle
	ScrollbarThumbStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(PrimaryColor))

	// ScrollbarTrackStyle is for the scrollbar track
	ScrollbarTrackStyle = DimStyle
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
