package components

import (
	"strings"
	"sync"

	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type LogPanelComponent struct {
	viewport      viewport.Model
	serviceLogs   []string
	testLogs      map[string][]string
	currentTestID string
	focused       bool
	logMutex      sync.RWMutex
}

func NewLogPanelComponent() *LogPanelComponent {
	vp := viewport.New(50, 20)
	vp.Style = lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(styles.BorderColor)).
		BorderTop(false).
		BorderBottom(false).
		BorderRight(false).
		BorderLeft(true).
		PaddingLeft(1)

	return &LogPanelComponent{
		viewport:    vp,
		serviceLogs: []string{},
		testLogs:    make(map[string][]string),
		focused:     false,
	}
}

func (lp *LogPanelComponent) Update(msg tea.Msg) tea.Cmd {
	if lp.focused {

		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "g":
				lp.viewport.GotoTop()
				return nil
			case "G":
				lp.viewport.GotoBottom()
				return nil
			case "j":
				lp.viewport.ScrollDown(1)
				return nil
			case "k":
				lp.viewport.ScrollUp(1)
				return nil
			}
		}

		var cmd tea.Cmd
		lp.viewport, cmd = lp.viewport.Update(msg)
		return cmd
	}
	return nil
}

func (lp *LogPanelComponent) View(width, height int) string {
	lp.logMutex.Lock()
	defer lp.logMutex.Unlock()

	// Safety checks for dimensions
	if width <= 0 {
		width = 50
	}
	if height <= 0 {
		height = 10
	}

	lp.updateViewport(false)

	// Account for left border (1) + padding (1) + scrollbar (1) = 3
	viewportWidth := width - 3
	// Title only (no top/bottom borders)
	viewportHeight := height - 1

	// Ensure minimum viewport dimensions
	if viewportWidth < 10 {
		viewportWidth = 10
	}
	if viewportHeight < 3 {
		viewportHeight = 3
	}

	lp.viewport.Width = viewportWidth
	lp.viewport.Height = viewportHeight

	// Update border color based on focus
	borderColor := lipgloss.Color(styles.BorderColor)
	if lp.focused {
		borderColor = lipgloss.Color(styles.PrimaryColor)
	}

	lp.viewport.Style = lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(borderColor).
		BorderTop(false).
		BorderBottom(false).
		BorderRight(false).
		BorderLeft(true).
		PaddingLeft(1).
		MaxWidth(width - 1) // -1 for scrollbar

	// Determine title and content
	title := "Logs"
	if lp.currentTestID != "" {
		title = "Test Logs"
		if len(lp.currentTestID) > 35 {
			title += ": " + lp.currentTestID[:35] + "..."
		} else {
			title += ": " + lp.currentTestID
		}
	}

	if lp.focused {
		title = "► " + title
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(styles.PrimaryColor)).
		MarginBottom(1)

	// Add scrollbar next to viewport content
	scrollbar := RenderScrollbar(lp.viewport.Height, lp.viewport.TotalLineCount(), lp.viewport.YOffset)

	viewportWithScrollbar := lipgloss.JoinHorizontal(
		lipgloss.Top,
		lp.viewport.View(),
		scrollbar,
	)

	return lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render(title),
		viewportWithScrollbar,
	)
}

func (lp *LogPanelComponent) AddServiceLog(line string) {
	lp.logMutex.Lock()
	defer lp.logMutex.Unlock()

	lp.serviceLogs = append(lp.serviceLogs, line)

	// Keep only last 1000 lines
	if len(lp.serviceLogs) > 1000 {
		lp.serviceLogs = lp.serviceLogs[len(lp.serviceLogs)-1000:]
	}

	// Update viewport if showing service logs
	if lp.currentTestID == "" {
		lp.updateViewport(true)
	}
}

func (lp *LogPanelComponent) AddTestLog(testID, line string) {
	lp.logMutex.Lock()
	defer lp.logMutex.Unlock()

	if lp.testLogs[testID] == nil {
		lp.testLogs[testID] = []string{}
	}

	lp.testLogs[testID] = append(lp.testLogs[testID], line)

	// Keep only last 500 lines per test
	if len(lp.testLogs[testID]) > 500 {
		lp.testLogs[testID] = lp.testLogs[testID][len(lp.testLogs[testID])-500:]
	}

	// Update viewport if showing this test's logs
	if lp.currentTestID == testID {
		lp.updateViewport(true)
	}
}

// GetRawLogs returns unwrapped logs for copy mode
func (lp *LogPanelComponent) GetRawLogs() string {
	lp.logMutex.RLock()
	defer lp.logMutex.RUnlock()

	var raw string
	if lp.currentTestID == "" {
		raw = strings.Join(lp.serviceLogs, "\n")
	} else {
		if logs, exists := lp.testLogs[lp.currentTestID]; exists {
			raw = strings.Join(logs, "\n")
		} else {
			return "No logs available for this test yet..."
		}
	}

	return utils.StripANSI(utils.StripNoWrapMarker(raw))
}

func (lp *LogPanelComponent) SetCurrentTest(testID string) {
	lp.logMutex.Lock()
	defer lp.logMutex.Unlock()

	lp.currentTestID = testID
	lp.updateViewport(true)
}

func (lp *LogPanelComponent) SetFocused(focused bool) {
	lp.focused = focused
	// Note: viewport doesn't have Focus/Blur methods, so we just track the state
}

func (lp *LogPanelComponent) IsFocused() bool {
	return lp.focused
}

func (lp *LogPanelComponent) updateViewport(gotoBottom bool) {
	var content string

	// Subtract left border (1) and padding (1) = 2
	wrapWidth := lp.viewport.Width - 2
	if wrapWidth <= 0 {
		wrapWidth = 70 // Conservative fallback
	}

	if lp.currentTestID == "" {
		// Wrap service logs at display time
		var wrappedLines []string

		for _, line := range lp.serviceLogs {
			subLines := strings.SplitSeq(line, "\n")
			for subLine := range subLines {
				wrapped := utils.WrapLine(subLine, wrapWidth)
				wrappedLines = append(wrappedLines, wrapped...)
			}
		}
		content = strings.Join(wrappedLines, "\n")
	} else {
		if logs, exists := lp.testLogs[lp.currentTestID]; exists {
			var wrappedLines []string

			for _, line := range logs {
				subLines := strings.SplitSeq(line, "\n")
				for subLine := range subLines {
					wrapped := utils.WrapLine(subLine, wrapWidth)
					wrappedLines = append(wrappedLines, wrapped...)
				}
			}
			content = strings.Join(wrappedLines, "\n")
		} else {
			content = "No logs available for this test yet..."
		}
	}

	lp.viewport.SetContent(utils.StripNoWrapMarker(content))
	if gotoBottom {
		lp.viewport.GotoBottom()
	}
}

func (lp *LogPanelComponent) ScrollUp(n int) {
	lp.viewport.ScrollUp(n)
}

func (lp *LogPanelComponent) ScrollDown(n int) {
	lp.viewport.ScrollDown(n)
}

func (lp *LogPanelComponent) HalfPageUp() {
	halfPage := max(lp.viewport.Height/2, 1)
	lp.viewport.ScrollUp(halfPage)
}

func (lp *LogPanelComponent) HalfPageDown() {
	halfPage := max(lp.viewport.Height/2, 1)
	lp.viewport.ScrollDown(halfPage)
}
