package components

import (
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/ansi"
)

// logPanelAutoScrollMsg is sent to trigger continuous scrolling during selection
type logPanelAutoScrollMsg struct {
	panelID int
}

// logScrollDirection indicates the direction of auto-scroll
type logScrollDirection int

const (
	logScrollNone logScrollDirection = iota
	logScrollUp
	logScrollDown
)

// logPanelIDCounter is used to generate unique panel IDs
var logPanelIDCounter int

type LogPanelComponent struct {
	viewport      viewport.Model
	serviceLogs   []string
	testLogs      map[string][]string
	currentTestID string
	logMutex      sync.RWMutex

	// Selection state
	selecting    bool
	selStart     SelectionPos
	selEnd       SelectionPos
	hasSelection bool

	// Screen position (set by parent)
	xOffset int
	yOffset int

	// Content lines for selection (after wrapping)
	contentLines []string

	// Copied indicator
	showCopied bool

	// Auto-scroll state
	panelID       int
	autoScrollDir logScrollDirection
	lastMouseX    int
	lastMouseY    int
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
	vp.MouseWheelEnabled = false

	logPanelIDCounter++
	return &LogPanelComponent{
		viewport:    vp,
		serviceLogs: []string{},
		testLogs:    make(map[string][]string),
		selStart:    SelectionPos{-1, -1},
		selEnd:      SelectionPos{-1, -1},
		panelID:     logPanelIDCounter,
		yOffset:     2, // title + empty line
	}
}

func (lp *LogPanelComponent) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.MouseMsg:
		return lp.handleMouse(msg)

	case logPanelAutoScrollMsg:
		if msg.panelID != lp.panelID {
			return nil
		}
		return lp.handleAutoScroll()
	}

	var cmd tea.Cmd
	lp.viewport, cmd = lp.viewport.Update(msg)
	return cmd
}

const (
	logAutoScrollInterval = 50 * time.Millisecond
	logScrollZone         = 2
)

func (lp *LogPanelComponent) handleMouse(msg tea.MouseMsg) tea.Cmd {
	x := msg.X - lp.xOffset
	y := msg.Y - lp.yOffset

	if x < 0 || x >= lp.viewport.Width || y < 0 {
		return nil
	}

	contentLine := y + lp.viewport.YOffset
	contentCol := x

	switch msg.Button {
	case tea.MouseButtonLeft:
		switch msg.Action {
		case tea.MouseActionPress:
			lp.selecting = true
			lp.selStart = SelectionPos{Line: contentLine, Col: contentCol}
			lp.selEnd = lp.selStart
			lp.hasSelection = false
			lp.autoScrollDir = logScrollNone
			lp.updateViewportWithSelection()

		case tea.MouseActionMotion:
			if !lp.selecting {
				return nil
			}

			lp.lastMouseX = x
			lp.lastMouseY = y

			prevDir := lp.autoScrollDir
			lp.autoScrollDir = logScrollNone

			if y < logScrollZone && lp.viewport.YOffset > 0 {
				lp.autoScrollDir = logScrollUp
				lp.viewport.ScrollUp(1)
			} else if y >= lp.viewport.Height-logScrollZone &&
				lp.viewport.YOffset < lp.viewport.TotalLineCount()-lp.viewport.Height {
				lp.autoScrollDir = logScrollDown
				lp.viewport.ScrollDown(1)
			}

			contentLine = y + lp.viewport.YOffset
			lp.selEnd = SelectionPos{Line: contentLine, Col: contentCol}
			lp.hasSelection = true
			lp.updateViewportWithSelection()

			if lp.autoScrollDir != logScrollNone && prevDir == logScrollNone {
				return lp.scheduleAutoScroll()
			}

		case tea.MouseActionRelease:
			if !lp.selecting {
				return nil
			}

			lp.selEnd = SelectionPos{Line: contentLine, Col: contentCol}
			hasSelection := lp.selStart != lp.selEnd

			var textToCopy string
			if hasSelection {
				lp.hasSelection = true
				textToCopy = lp.getSelectedText()
			}

			lp.selecting = false
			lp.hasSelection = false
			lp.autoScrollDir = logScrollNone
			lp.selStart = SelectionPos{-1, -1}
			lp.selEnd = SelectionPos{-1, -1}
			lp.copyToClipboard(textToCopy)
			lp.updateViewportWithSelection()

			if textToCopy != "" {
				lp.showCopied = true
				return tea.Tick(time.Second, func(t time.Time) tea.Msg {
					lp.showCopied = false
					return struct{}{}
				})
			}
		}

	case tea.MouseButtonWheelUp:
		lp.viewport.ScrollUp(3)
		return nil

	case tea.MouseButtonWheelDown:
		lp.viewport.ScrollDown(3)
		return nil
	}

	return nil
}

func (lp *LogPanelComponent) scheduleAutoScroll() tea.Cmd {
	id := lp.panelID
	return tea.Tick(logAutoScrollInterval, func(t time.Time) tea.Msg {
		return logPanelAutoScrollMsg{panelID: id}
	})
}

func (lp *LogPanelComponent) handleAutoScroll() tea.Cmd {
	if !lp.selecting || lp.autoScrollDir == logScrollNone {
		return nil
	}

	canScrollUp := lp.viewport.YOffset > 0
	canScrollDown := lp.viewport.YOffset < lp.viewport.TotalLineCount()-lp.viewport.Height

	switch lp.autoScrollDir {
	case logScrollUp:
		if !canScrollUp {
			lp.autoScrollDir = logScrollNone
			return nil
		}
		lp.viewport.ScrollUp(1)
	case logScrollDown:
		if !canScrollDown {
			lp.autoScrollDir = logScrollNone
			return nil
		}
		lp.viewport.ScrollDown(1)
	}

	contentLine := lp.lastMouseY + lp.viewport.YOffset
	lp.selEnd = SelectionPos{Line: contentLine, Col: lp.lastMouseX}
	lp.updateViewportWithSelection()

	return lp.scheduleAutoScroll()
}

func (lp *LogPanelComponent) View(width, height int) string {
	lp.logMutex.Lock()
	defer lp.logMutex.Unlock()

	if width <= 0 {
		width = 50
	}
	if height <= 0 {
		height = 10
	}

	lp.updateViewport(false)

	// Account for left border (1) + padding (1) + scrollbar (1) = 3
	// Account for title (1) + empty line (1) = 2 in height
	viewportWidth := width - 3
	viewportHeight := height - 2

	if viewportWidth < 10 {
		viewportWidth = 10
	}
	if viewportHeight < 3 {
		viewportHeight = 3
	}

	lp.viewport.Width = viewportWidth
	lp.viewport.Height = viewportHeight

	lp.viewport.Style = lipgloss.NewStyle()

	title := "Logs"
	if lp.currentTestID != "" {
		title = "Test Logs"
		if len(lp.currentTestID) > 35 {
			title += ": " + lp.currentTestID[:35] + "..."
		} else {
			title += ": " + lp.currentTestID
		}
	}

	if lp.showCopied {
		title += styles.SuccessStyle.Render(" [copied]")
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(styles.PrimaryColor))

	scrollbar := RenderScrollbar(lp.viewport.Height, lp.viewport.TotalLineCount(), lp.viewport.YOffset)

	viewportWithScrollbar := lipgloss.JoinHorizontal(
		lipgloss.Top,
		lp.viewport.View(),
		scrollbar,
	)

	content := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render(title),
		"",
		viewportWithScrollbar,
	)

	containerStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(styles.BorderColor)).
		BorderTop(false).
		BorderBottom(false).
		BorderRight(false).
		BorderLeft(true).
		PaddingLeft(1)

	return containerStyle.Render(content)
}

func (lp *LogPanelComponent) AddServiceLog(line string) {
	lp.logMutex.Lock()
	defer lp.logMutex.Unlock()

	lp.serviceLogs = append(lp.serviceLogs, line)

	if len(lp.serviceLogs) > 1000 {
		lp.serviceLogs = lp.serviceLogs[len(lp.serviceLogs)-1000:]
	}

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

	if len(lp.testLogs[testID]) > 500 {
		lp.testLogs[testID] = lp.testLogs[testID][len(lp.testLogs[testID])-500:]
	}

	if lp.currentTestID == testID {
		lp.updateViewport(true)
	}
}

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

func (lp *LogPanelComponent) SetOffset(x, y int) {
	lp.xOffset = x
	lp.yOffset = y
}

func (lp *LogPanelComponent) updateViewport(gotoBottom bool) {
	wrapWidth := lp.viewport.Width - 2
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

	lp.contentLines = wrappedLines

	if lp.selecting || lp.hasSelection {
		lp.applySelectionHighlighting()
	} else {
		content := strings.Join(wrappedLines, "\n")
		lp.viewport.SetContent(utils.StripNoWrapMarker(content))
	}

	if gotoBottom {
		lp.viewport.GotoBottom()
	}
}

// applySelectionHighlighting applies selection highlighting to the viewport content
// This is separate from updateViewportWithSelection to avoid recursion
func (lp *LogPanelComponent) applySelectionHighlighting() {
	if len(lp.contentLines) == 0 {
		return
	}

	var highlighted strings.Builder
	selStyle := styles.SelectedStyle.Background(lipgloss.Color(styles.SecondaryColor))

	start, end := lp.normalizeSelection()

	for i, line := range lp.contentLines {
		if i > 0 {
			highlighted.WriteString("\n")
		}

		lineWidth := ansi.PrintableRuneWidth(line)

		if i < start.Line || i > end.Line {
			highlighted.WriteString(line)
			continue
		}

		highlighted.WriteString(lp.highlightLine(line, lineWidth, i, start, end, selStyle))
	}

	lp.viewport.SetContent(utils.StripNoWrapMarker(highlighted.String()))
}

func (lp *LogPanelComponent) updateViewportWithSelection() {
	lp.applySelectionHighlighting()
}

func (lp *LogPanelComponent) highlightLine(line string, lineWidth, lineNum int, start, end SelectionPos, selStyle lipgloss.Style) string {
	selStartCol := 0
	selEndCol := lineWidth

	if lineNum == start.Line {
		selStartCol = start.Col
	}
	if lineNum == end.Line {
		selEndCol = end.Col
	}

	if selStartCol < 0 {
		selStartCol = 0
	}
	if selEndCol > lineWidth {
		selEndCol = lineWidth
	}
	if selStartCol > lineWidth {
		selStartCol = lineWidth
	}

	if selStartCol >= selEndCol {
		return line
	}

	return lp.applyHighlight(line, selStartCol, selEndCol, selStyle)
}

func (lp *LogPanelComponent) applyHighlight(line string, startCol, endCol int, selStyle lipgloss.Style) string {
	var result strings.Builder
	var currentCol int
	var inAnsi bool
	var ansiSeq strings.Builder

	runes := []rune(line)
	for i := range runes {
		r := runes[i]

		if r == '\x1b' && i+1 < len(runes) && runes[i+1] == '[' {
			inAnsi = true
			ansiSeq.Reset()
			ansiSeq.WriteRune(r)
			continue
		}

		if inAnsi {
			ansiSeq.WriteRune(r)
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inAnsi = false
				result.WriteString(ansiSeq.String())
			}
			continue
		}

		if currentCol >= startCol && currentCol < endCol {
			result.WriteString(selStyle.Render(string(r)))
		} else {
			result.WriteRune(r)
		}
		currentCol++
	}

	return result.String()
}

func (lp *LogPanelComponent) normalizeSelection() (SelectionPos, SelectionPos) {
	start, end := lp.selStart, lp.selEnd

	if start.Line > end.Line || (start.Line == end.Line && start.Col > end.Col) {
		start, end = end, start
	}

	return start, end
}

func (lp *LogPanelComponent) getSelectedText() string {
	if !lp.hasSelection || len(lp.contentLines) == 0 {
		return ""
	}

	start, end := lp.normalizeSelection()

	var result strings.Builder

	for i := start.Line; i <= end.Line && i < len(lp.contentLines); i++ {
		if i < 0 {
			continue
		}

		line := lp.contentLines[i]
		plainLine := stripAnsi(line)
		runes := []rune(plainLine)

		startCol := 0
		endCol := len(runes)

		if i == start.Line {
			startCol = start.Col
		}
		if i == end.Line {
			endCol = end.Col
		}

		if startCol < 0 {
			startCol = 0
		}
		if endCol > len(runes) {
			endCol = len(runes)
		}
		if startCol > len(runes) {
			startCol = len(runes)
		}

		if startCol < endCol {
			result.WriteString(string(runes[startCol:endCol]))
		}

		if i < end.Line {
			result.WriteString("\n")
		}
	}

	return result.String()
}

func (lp *LogPanelComponent) copyToClipboard(text string) {
	if text == "" {
		return
	}

	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return
		}
	default:
		return
	}

	cmd.Stdin = strings.NewReader(text)
	_ = cmd.Run()
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
