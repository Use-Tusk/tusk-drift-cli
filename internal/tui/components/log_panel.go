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
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		PaddingLeft(1).
		PaddingRight(1)

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
			}
		}

		var cmd tea.Cmd
		lp.viewport, cmd = lp.viewport.Update(msg)
		return cmd
	}
	return nil
}

func (lp *LogPanelComponent) View(width, height int) string {
	// Safety checks for dimensions
	if width <= 0 {
		width = 50 // Fallback width
	}
	if height <= 0 {
		height = 20 // Fallback height
	}

	viewportWidth := width - 4   // Account for borders (2) and padding (2)
	viewportHeight := height - 3 // Space for title and borders

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
	borderColor := lipgloss.Color("240") // Dim
	if lp.focused {
		borderColor = lipgloss.Color(styles.PrimaryColor)
	}

	lp.viewport.Style = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		PaddingLeft(1).
		PaddingRight(1).
		Width(width)

	// Determine title and content
	title := "Service Logs"
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
		Foreground(borderColor).
		MarginBottom(1)

	return lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render(title),
		lp.viewport.View(),
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
		lp.updateViewport()
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
		lp.updateViewport()
	}
}

// GetRawLogs returns unwrapped logs for copy mode
func (lp *LogPanelComponent) GetRawLogs() string {
	lp.logMutex.RLock()
	defer lp.logMutex.RUnlock()

	if lp.currentTestID == "" {
		// Return unwrapped service logs
		return strings.Join(lp.serviceLogs, "\n")
	} else {
		// Return unwrapped test logs
		if logs, exists := lp.testLogs[lp.currentTestID]; exists {
			return strings.Join(logs, "\n")
		}
		return "No logs available for this test yet..."
	}
}

func (lp *LogPanelComponent) SetCurrentTest(testID string) {
	lp.logMutex.Lock()
	defer lp.logMutex.Unlock()

	lp.currentTestID = testID
	lp.updateViewport()
}

func (lp *LogPanelComponent) SetFocused(focused bool) {
	lp.focused = focused
	// Note: viewport doesn't have Focus/Blur methods, so we just track the state
}

func (lp *LogPanelComponent) IsFocused() bool {
	return lp.focused
}

func (lp *LogPanelComponent) updateViewport() {
	var content string

	wrapWidth := lp.viewport.Width - 2 // Only subtract left + right padding (1 + 1)
	if wrapWidth <= 0 {
		wrapWidth = 70 // Conservative fallback
	}

	if lp.currentTestID == "" {
		// Wrap service logs at display time
		var wrappedLines []string

		for _, line := range lp.serviceLogs {
			wrapped := utils.WrapLine(line, wrapWidth)
			wrappedLines = append(wrappedLines, wrapped...)
		}
		content = strings.Join(wrappedLines, "\n")
	} else {
		if logs, exists := lp.testLogs[lp.currentTestID]; exists {
			var wrappedLines []string

			for _, line := range logs {
				wrapped := utils.WrapLine(line, wrapWidth)
				wrappedLines = append(wrappedLines, wrapped...)
			}
			content = strings.Join(wrappedLines, "\n")
		} else {
			content = "No logs available for this test yet..."
		}
	}

	lp.viewport.SetContent(content)
	lp.viewport.GotoBottom()
}
