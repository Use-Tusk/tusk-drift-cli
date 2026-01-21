package components

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
)

// DetailsPanel is a scrollable panel for displaying details with mouse and keyboard support
type DetailsPanel struct {
	viewport viewport.Model
	title    string
	focused  bool
	width    int
	height   int
}

// NewDetailsPanel creates a new details panel
func NewDetailsPanel() *DetailsPanel {
	vp := viewport.New(50, 20)
	vp.Style = lipgloss.NewStyle()
	vp.MouseWheelEnabled = true

	return &DetailsPanel{
		viewport: vp,
		title:    "Details",
		focused:  false,
	}
}

// Update handles input messages
func (dp *DetailsPanel) Update(msg tea.Msg) tea.Cmd {
	if !dp.focused {
		return nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "g":
			dp.viewport.GotoTop()
			return nil
		case "G":
			dp.viewport.GotoBottom()
			return nil
		case "j", "down":
			dp.viewport.ScrollDown(1)
			return nil
		case "k", "up":
			dp.viewport.ScrollUp(1)
			return nil
		case "d", "ctrl+d":
			dp.viewport.HalfPageDown()
			return nil
		case "u", "ctrl+u":
			dp.viewport.HalfPageUp()
			return nil
		case "f", "ctrl+f", "pgdown":
			dp.viewport.PageDown()
			return nil
		case "b", "ctrl+b", "pgup":
			dp.viewport.PageUp()
			return nil
		}
	case tea.MouseMsg:
		var cmd tea.Cmd
		dp.viewport, cmd = dp.viewport.Update(msg)
		return cmd
	}

	var cmd tea.Cmd
	dp.viewport, cmd = dp.viewport.Update(msg)
	return cmd
}

// View renders the panel
func (dp *DetailsPanel) View() string {
	borderColor := lipgloss.Color("240")
	if dp.focused {
		borderColor = lipgloss.Color(styles.PrimaryColor)
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(borderColor)

	title := dp.title
	if dp.focused {
		title = "► " + title
	}

	scrollInfo := ""
	if dp.viewport.TotalLineCount() > dp.viewport.Height {
		scrollPct := dp.viewport.ScrollPercent() * 100
		scrollInfo = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render(formatScrollPercent(scrollPct))
	}

	headerWidth := dp.width - 2 // Account for border
	header := lipgloss.NewStyle().
		Width(headerWidth).
		Render(lipgloss.JoinHorizontal(
			lipgloss.Top,
			titleStyle.Render(title),
			lipgloss.NewStyle().Width(headerWidth-lipgloss.Width(title)-lipgloss.Width(scrollInfo)).Render(""),
			scrollInfo,
		))

	panelStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(borderColor).
		Width(dp.width).
		Height(dp.height)

	content := lipgloss.JoinVertical(lipgloss.Left,
		header,
		dp.viewport.View(),
	)

	return panelStyle.Render(content)
}

// SetContent sets the content to display
func (dp *DetailsPanel) SetContent(content string) {
	dp.viewport.SetContent(content)
}

// SetTitle sets the panel title
func (dp *DetailsPanel) SetTitle(title string) {
	dp.title = title
}

// SetSize sets the panel dimensions
func (dp *DetailsPanel) SetSize(width, height int) {
	dp.width = width
	dp.height = height

	// Account for border (1) and header (1)
	viewportWidth := width - 2
	viewportHeight := height - 2

	if viewportWidth < 10 {
		viewportWidth = 10
	}
	if viewportHeight < 3 {
		viewportHeight = 3
	}

	dp.viewport.Width = viewportWidth
	dp.viewport.Height = viewportHeight
}

// SetFocused sets the focus state
func (dp *DetailsPanel) SetFocused(focused bool) {
	dp.focused = focused
}

// IsFocused returns whether the panel is focused
func (dp *DetailsPanel) IsFocused() bool {
	return dp.focused
}

// GotoTop scrolls to the top
func (dp *DetailsPanel) GotoTop() {
	dp.viewport.GotoTop()
}

// GotoBottom scrolls to the bottom
func (dp *DetailsPanel) GotoBottom() {
	dp.viewport.GotoBottom()
}

// formatScrollPercent formats the scroll percentage for display
func formatScrollPercent(pct float64) string {
	if pct <= 0 {
		return "Top"
	}
	if pct >= 100 {
		return "Bot"
	}
	return lipgloss.NewStyle().Render(
		lipgloss.NewStyle().Render(
			formatPct(pct),
		),
	)
}

func formatPct(pct float64) string {
	if pct < 10 {
		return " " + formatInt(int(pct)) + "%"
	}
	return formatInt(int(pct)) + "%"
}

func formatInt(n int) string {
	if n < 10 {
		return string(rune('0'+n)) + ""
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}
