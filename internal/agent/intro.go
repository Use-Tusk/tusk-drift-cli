package agent

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const tuskLogo = `████████╗██╗   ██╗███████╗██╗  ██╗
╚══██╔══╝██║   ██║██╔════╝██║ ██╔╝
   ██║   ██║   ██║███████╗█████╔╝ 
   ██║   ██║   ██║╚════██║██╔═██╗ 
   ██║   ╚██████╔╝███████║██║  ██╗
   ╚═╝    ╚═════╝ ╚══════╝╚═╝  ╚═╝

██████╗ ██████╗ ██╗███████╗████████╗
██╔══██╗██╔══██╗██║██╔════╝╚══██╔══╝
██║  ██║██████╔╝██║█████╗     ██║   
██║  ██║██╔══██╗██║██╔══╝     ██║   
██████╔╝██║  ██║██║██║        ██║   
╚═════╝ ╚═╝  ╚═╝╚═╝╚═╝        ╚═╝   `

const introDescription = `Welcome to Tusk Drift Setup!

This AI-powered agent will automatically configure your project 
for API testing by analyzing your codebase, installing the Drift
SDK, and setting up the necessary configuration.

The agent will guide you through the following phases:
  • Discovery    - Analyze your project structure and dependencies
  • Validation   - Verify your application can start successfully  
  • Installation - Install the Drift SDK and instrument your code
  • Config       - Generate configuration files for test recording
  • Test         - Run sample tests to verify the setup works

You may be prompted for input during the setup process.`

// Color gradient for the wave effect
var waveColors = []string{
	"213", // bright magenta
	"177", // light purple
	"141", // medium purple
	"105", // purple
	"69",  // blue-purple
	"33",  // cyan-blue
	"39",  // cyan
	"45",  // light cyan
	"51",  // bright cyan
	"45",  // light cyan
	"39",  // cyan
	"33",  // cyan-blue
	"69",  // blue-purple
	"105", // purple
	"141", // medium purple
	"177", // light purple
}

// IntroModel is the bubbletea model for the intro screen
type IntroModel struct {
	width      int
	height     int
	quitting   bool
	continuing bool
	tick       int

	// Mouse tracking
	mouseX, mouseY int

	// Logo data
	logoLines  []string
	logoWidth  int
	logoHeight int
}

type introTickMsg time.Time

// NewIntroModel creates a new intro screen model
func NewIntroModel() *IntroModel {
	lines := strings.Split(tuskLogo, "\n")

	maxWidth := 0
	for _, line := range lines {
		runeLen := len([]rune(line))
		if runeLen > maxWidth {
			maxWidth = runeLen
		}
	}

	return &IntroModel{
		tick:       0,
		logoLines:  lines,
		logoWidth:  maxWidth,
		logoHeight: len(lines),
		mouseX:     -1,
		mouseY:     -1,
	}
}

func (m *IntroModel) Init() tea.Cmd {
	return m.tickAnimation()
}

func (m *IntroModel) tickAnimation() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
		return introTickMsg(t)
	})
}

func (m *IntroModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.MouseMsg:
		m.mouseX = msg.X
		m.mouseY = msg.Y
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.quitting = true
			return m, tea.Quit
		default:
			m.continuing = true
			return m, tea.Quit
		}

	case introTickMsg:
		m.tick++
		return m, m.tickAnimation()
	}

	return m, nil
}

func (m *IntroModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	// Content heights
	logoHeight := m.logoHeight
	spacingAfterLogo := 2
	descBoxHeight := 17 // Approx height of description box with borders/padding
	footerHeight := 1

	totalContentHeight := logoHeight + spacingAfterLogo + descBoxHeight + footerHeight

	topPadding := (m.height - totalContentHeight) / 2
	topPadding = max(topPadding, 1)

	logoStartX := (m.width - m.logoWidth) / 2
	logoStartY := topPadding

	var sb strings.Builder

	for i := 0; i < topPadding; i++ {
		sb.WriteRune('\n')
	}

	logoContent := m.renderLogo(logoStartX, logoStartY)

	centeredLogo := lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		Render(logoContent)

	sb.WriteString(centeredLogo)

	// Spacing after logo
	for range spacingAfterLogo {
		sb.WriteRune('\n')
	}

	// Description box (text color based on background)
	descTextColor := "252"
	if !styles.HasDarkBackground {
		descTextColor = "238"
	}

	descStyle := lipgloss.NewStyle().
		Width(min(70, m.width-4)).
		Align(lipgloss.Left).
		Foreground(lipgloss.Color(descTextColor))

	// Box border (adjust for background)
	boxBorderColor := "238"
	if !styles.HasDarkBackground {
		boxBorderColor = "245"
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(boxBorderColor)).
		Padding(1, 2)

	descBox := boxStyle.Render(descStyle.Render(introDescription))

	centeredDesc := lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		Render(descBox)

	sb.WriteString(centeredDesc)

	spacingAfterDesc := 2
	for range spacingAfterDesc {
		sb.WriteRune('\n')
	}

	// Footer - simple centered text without background
	footerText := "Press any key to start • q/Esc/Ctrl+C to quit"
	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(styles.PrimaryColor)).
		Width(m.width).
		Align(lipgloss.Center)

	sb.WriteString(footerStyle.Render(footerText))

	return sb.String()
}

func (m *IntroModel) renderLogo(logoStartX, logoStartY int) string {
	var sb strings.Builder

	var glowColors []string
	if styles.HasDarkBackground {
		// Light glow for dark backgrounds
		glowColors = []string{"231", "255", "253", "251"}
	} else {
		// Dark glow for light backgrounds
		glowColors = []string{"55", "56", "57", "93"}
	}

	for lineIdx, line := range m.logoLines {
		runes := []rune(line)
		for charIdx, char := range runes {
			if char == ' ' {
				sb.WriteRune(' ')
				continue
			}

			// Calculate wave color based on position and time
			wavePos := float64(m.tick)*0.3 + float64(charIdx)*0.15 + float64(lineIdx)*0.2
			colorIdx := int(wavePos) % len(waveColors)
			if colorIdx < 0 {
				colorIdx += len(waveColors)
			}
			color := waveColors[colorIdx]

			// Check for mouse glow effect - smaller radius, more diffuse
			charScreenX := logoStartX + charIdx
			charScreenY := logoStartY + lineIdx

			if m.mouseX >= 0 && m.mouseY >= 0 {
				dx := float64(charScreenX - m.mouseX)
				dy := float64(charScreenY-m.mouseY) * 2.0 // Scale Y for terminal char aspect ratio (chars are ~2x tall)
				dist := math.Sqrt(dx*dx + dy*dy)

				// Glow radius with smoother gradient
				if dist < 9 {
					glowIdx := int((dist / 9.0) * float64(len(glowColors)))
					if glowIdx >= len(glowColors) {
						glowIdx = len(glowColors) - 1
					}
					color = glowColors[glowIdx]
				}
			}

			style := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true)
			sb.WriteString(style.Render(string(char)))
		}
		if lineIdx < len(m.logoLines)-1 {
			sb.WriteRune('\n')
		}
	}

	return sb.String()
}

// ShouldContinue returns true if the user pressed a key to continue (not quit)
func (m *IntroModel) ShouldContinue() bool {
	return m.continuing
}

// RunIntroScreen runs the intro screen and returns true if user wants to continue
func RunIntroScreen() (bool, error) {
	model := NewIntroModel()
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseAllMotion())

	finalModel, err := p.Run()

	// Small delay to let terminal properly reset mouse mode
	time.Sleep(50 * time.Millisecond)

	// Explicitly disable mouse tracking by printing the disable sequences
	fmt.Print("\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l")

	if err != nil {
		return false, err
	}

	if m, ok := finalModel.(*IntroModel); ok {
		return m.ShouldContinue(), nil
	}

	return false, nil
}

// PrintIntroHeadless prints a simple intro for headless mode (no confirmation needed for scripts)
func PrintIntroHeadless() {
	primaryStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(styles.PrimaryColor)).
		Bold(true)

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))

	fmt.Println()
	fmt.Println(primaryStyle.Render(tuskLogo))
	fmt.Println()
	fmt.Println(introDescription)
	fmt.Println()
	fmt.Println(dimStyle.Render("────────────────────────────────────────────────────────"))
	fmt.Println()
}
