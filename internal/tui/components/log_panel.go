package components

import (
	"strings"
	"sync"

	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
	tea "github.com/charmbracelet/bubbletea"
)

// LogPanelComponent wraps ContentPanel and adds log management
type LogPanelComponent struct {
	*ContentPanel
	serviceLogs   []string
	testLogs      map[string][]string
	currentTestID string
	logMutex      sync.RWMutex
}

// NewLogPanelComponent creates a new log panel
func NewLogPanelComponent() *LogPanelComponent {
	panel := NewContentPanel()
	panel.SetTitle("Logs")
	panel.EmptyLineAfterTitle = true
	return &LogPanelComponent{
		ContentPanel: panel,
		serviceLogs:  []string{},
		testLogs:     make(map[string][]string),
	}
}

// Update handles input messages
func (lp *LogPanelComponent) Update(msg tea.Msg) tea.Cmd {
	return lp.ContentPanel.Update(msg)
}

// View renders the panel with the given dimensions
func (lp *LogPanelComponent) View(width, height int) string {
	lp.logMutex.Lock()
	defer lp.logMutex.Unlock()

	lp.rebuildContent(false)

	return lp.ContentPanel.View(width, height)
}

// AddServiceLog adds a log line to service logs
func (lp *LogPanelComponent) AddServiceLog(line string) {
	lp.logMutex.Lock()
	defer lp.logMutex.Unlock()

	lp.serviceLogs = append(lp.serviceLogs, line)

	if len(lp.serviceLogs) > 1000 {
		lp.serviceLogs = lp.serviceLogs[len(lp.serviceLogs)-1000:]
	}

	if lp.currentTestID == "" {
		lp.rebuildContent(true)
	}
}

// AddTestLog adds a log line to a specific test's logs
func (lp *LogPanelComponent) AddTestLog(testID, line string) {
	lp.logMutex.Lock()
	defer lp.logMutex.Unlock()

	if lp.testLogs[testID] == nil {
		lp.testLogs[testID] = []string{}
	}

	lp.testLogs[testID] = append(lp.testLogs[testID], line)

	if len(lp.testLogs[testID]) > 500 {
		lp.testLogs[testID] = lp.testLogs[testID][len(lp.testLogs[testID])-500:]
	}

	if lp.currentTestID == testID {
		lp.rebuildContent(true)
	}
}

// GetRawLogs returns the raw log content without ANSI codes
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

// SetCurrentTest sets the current test to display logs for
func (lp *LogPanelComponent) SetCurrentTest(testID string) {
	lp.logMutex.Lock()
	defer lp.logMutex.Unlock()

	lp.currentTestID = testID
	lp.updateTitle()
	lp.rebuildContent(true)
}

// SetOffset sets the panel's position on screen (for mouse coordinate translation)
func (lp *LogPanelComponent) SetOffset(x, y int) {
	lp.ContentPanel.SetOffset(x, y)
}

// rebuildContent rebuilds the viewport content from logs
func (lp *LogPanelComponent) rebuildContent(gotoBottom bool) {
	wrapWidth := lp.ContentPanel.GetViewportWidth() - 2
	if wrapWidth <= 0 {
		wrapWidth = 70
	}

	var wrappedLines []string

	if lp.currentTestID == "" {
		for _, line := range lp.serviceLogs {
			subLines := strings.Split(line, "\n")
			for _, subLine := range subLines {
				wrapped := utils.WrapLine(subLine, wrapWidth)
				wrappedLines = append(wrappedLines, wrapped...)
			}
		}
	} else {
		if logs, exists := lp.testLogs[lp.currentTestID]; exists {
			for _, line := range logs {
				subLines := strings.Split(line, "\n")
				for _, subLine := range subLines {
					wrapped := utils.WrapLine(subLine, wrapWidth)
					wrappedLines = append(wrappedLines, wrapped...)
				}
			}
		} else {
			wrappedLines = []string{"No logs available for this test yet..."}
		}
	}

	for i, line := range wrappedLines {
		wrappedLines[i] = utils.StripNoWrapMarker(line)
	}

	lp.ContentPanel.UpdateContentLines(wrappedLines)

	if gotoBottom {
		lp.ContentPanel.GotoBottom()
	}
}

// updateTitle updates the panel title based on current state
func (lp *LogPanelComponent) updateTitle() {
	if lp.currentTestID == "" {
		lp.ContentPanel.SetTitle("Logs")
	} else {
		title := "Test Logs"
		if len(lp.currentTestID) > 35 {
			title += ": " + lp.currentTestID[:35] + "..."
		} else {
			title += ": " + lp.currentTestID
		}
		lp.ContentPanel.SetTitle(title)
	}
}
