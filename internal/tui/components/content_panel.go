package components

import (
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/ansi"

	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
)

// isSoftLineBreak checks if line starts with soft line break marker
func isSoftLineBreak(line string) bool {
	return utils.IsSoftLineBreak(line)
}

// SelectionPos represents a position in the content
type SelectionPos struct {
	Line int
	Col  int
}

// autoScrollMsg is sent to trigger continuous scrolling during selection
type autoScrollMsg struct {
	panelID int
}

// scrollDirection indicates the direction of auto-scroll
type scrollDirection int

const (
	scrollNone scrollDirection = iota
	scrollUp
	scrollDown
)

const (
	autoScrollInterval = 50 * time.Millisecond
	scrollZone         = 2 // lines from edge to trigger scroll
)

// panelIDCounter is used to generate unique panel IDs
var panelIDCounter int

// ContentPanel is a scrollable panel with text selection support
type ContentPanel struct {
	viewport viewport.Model
	title    string
	width    int
	height   int

	// Selection state
	selecting    bool
	selStart     SelectionPos
	selEnd       SelectionPos
	hasSelection bool

	// Screen position (set by parent)
	xOffset int
	yOffset int

	// Content storage for selection (may contain SoftLineBreak markers)
	contentLines []string

	// Copied indicator
	showCopied bool

	// Auto-scroll state
	panelID       int             // Unique ID to identify this panel's messages
	autoScrollDir scrollDirection // Current auto-scroll direction
	lastMouseX    int             // Last mouse X during selection (viewport-relative)
	lastMouseY    int             // Last mouse Y during selection (viewport-relative)

	// Configuration
	EmptyLineAfterTitle bool
}

// NewContentPanel creates a new content panel
func NewContentPanel() *ContentPanel {
	vp := viewport.New(50, 20)
	vp.Style = lipgloss.NewStyle()
	vp.MouseWheelEnabled = false

	panelIDCounter++
	return &ContentPanel{
		viewport: vp,
		title:    "Content",
		yOffset:  1, // title only
		selStart: SelectionPos{-1, -1},
		selEnd:   SelectionPos{-1, -1},
		panelID:  panelIDCounter,
	}
}

// Update handles input messages (mouse and auto-scroll only)
func (cp *ContentPanel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.MouseMsg:
		return cp.handleMouse(msg)

	case autoScrollMsg:
		if msg.panelID != cp.panelID {
			return nil
		}
		return cp.handleAutoScroll()
	}

	var cmd tea.Cmd
	cp.viewport, cmd = cp.viewport.Update(msg)
	return cmd
}

// handleMouse processes mouse events for text selection
func (cp *ContentPanel) handleMouse(msg tea.MouseMsg) tea.Cmd {
	x := msg.X - cp.xOffset
	y := msg.Y - cp.yOffset

	if y < 0 {
		return nil
	}

	// error margin for clicks near the left border
	if x < 0 && x >= -3 {
		x = 0
	}

	// Reject clicks too far outside horizontal bounds
	if x < 0 || x >= cp.viewport.Width {
		return nil
	}

	contentLine := y + cp.viewport.YOffset
	contentCol := x

	switch msg.Button {
	case tea.MouseButtonLeft:
		switch msg.Action {
		case tea.MouseActionPress:
			cp.selecting = true
			cp.selStart = SelectionPos{Line: contentLine, Col: contentCol}
			cp.selEnd = cp.selStart
			cp.hasSelection = false
			cp.autoScrollDir = scrollNone
			cp.updateViewportContent()

		case tea.MouseActionMotion:
			if !cp.selecting {
				return nil
			}

			cp.lastMouseX = x
			cp.lastMouseY = y

			prevDir := cp.autoScrollDir
			cp.autoScrollDir = scrollNone

			if y < scrollZone && cp.viewport.YOffset > 0 {
				cp.autoScrollDir = scrollUp
				cp.viewport.ScrollUp(1)
			} else if y >= cp.viewport.Height-scrollZone &&
				cp.viewport.YOffset < cp.viewport.TotalLineCount()-cp.viewport.Height {
				cp.autoScrollDir = scrollDown
				cp.viewport.ScrollDown(1)
			}

			contentLine = y + cp.viewport.YOffset
			cp.selEnd = SelectionPos{Line: contentLine, Col: contentCol}
			cp.hasSelection = true
			cp.updateViewportContent()

			if cp.autoScrollDir != scrollNone && prevDir == scrollNone {
				return cp.scheduleAutoScroll()
			}

		case tea.MouseActionRelease:
			if !cp.selecting {
				return nil
			}

			cp.selEnd = SelectionPos{Line: contentLine, Col: contentCol}
			hasSelection := cp.selStart != cp.selEnd

			var textToCopy string
			if hasSelection {
				cp.hasSelection = true
				textToCopy = cp.GetSelectedText()
			}

			cp.selecting = false
			cp.hasSelection = false
			cp.autoScrollDir = scrollNone
			cp.selStart = SelectionPos{-1, -1}
			cp.selEnd = SelectionPos{-1, -1}
			cp.copyToClipboard(textToCopy)
			cp.updateViewportContent()

			return tea.Tick(time.Second, func(t time.Time) tea.Msg {
				cp.showCopied = false
				return struct{}{}
			})
		}

	case tea.MouseButtonWheelUp:
		cp.viewport.ScrollUp(3)
		return nil

	case tea.MouseButtonWheelDown:
		cp.viewport.ScrollDown(3)
		return nil
	}

	return nil
}

// scheduleAutoScroll returns a command that will trigger auto-scroll after a delay
func (cp *ContentPanel) scheduleAutoScroll() tea.Cmd {
	id := cp.panelID
	return tea.Tick(autoScrollInterval, func(t time.Time) tea.Msg {
		return autoScrollMsg{panelID: id}
	})
}

// handleAutoScroll processes auto-scroll ticks during selection
func (cp *ContentPanel) handleAutoScroll() tea.Cmd {
	if !cp.selecting || cp.autoScrollDir == scrollNone {
		return nil
	}

	canScrollUp := cp.viewport.YOffset > 0
	canScrollDown := cp.viewport.YOffset < cp.viewport.TotalLineCount()-cp.viewport.Height

	switch cp.autoScrollDir {
	case scrollUp:
		if !canScrollUp {
			cp.autoScrollDir = scrollNone
			return nil
		}
		cp.viewport.ScrollUp(1)
	case scrollDown:
		if !canScrollDown {
			cp.autoScrollDir = scrollNone
			return nil
		}
		cp.viewport.ScrollDown(1)
	}

	contentLine := cp.lastMouseY + cp.viewport.YOffset
	cp.selEnd = SelectionPos{Line: contentLine, Col: cp.lastMouseX}
	cp.updateViewportContent()

	return cp.scheduleAutoScroll()
}

// View renders the panel with the given dimensions
func (cp *ContentPanel) View(width, height int) string {
	if width <= 0 {
		width = 50
	}
	if height <= 0 {
		height = 10
	}

	cp.width = width
	cp.height = height

	// Calculate title height locally - don't overwrite yOffset which is set by parent for mouse coords
	titleHeight := 1
	if cp.EmptyLineAfterTitle {
		titleHeight = 2 // title + empty line
	}

	// Account for: left border (1), left padding (1), scrollbar (1) = 3
	viewportWidth := width - 3
	// Account for: title (1) + optional empty line
	viewportHeight := height - titleHeight

	if viewportWidth < 10 {
		viewportWidth = 10
	}
	if viewportHeight < 3 {
		viewportHeight = 3
	}

	cp.viewport.Width = viewportWidth
	cp.viewport.Height = viewportHeight

	title := cp.title
	copiedIndicator := ""
	if cp.showCopied {
		copiedIndicator = styles.SuccessStyle.Render(" [copied]")
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(styles.PrimaryColor))
	titleText := title + copiedIndicator

	cp.viewport.Style = lipgloss.NewStyle()

	scrollbar := cp.renderScrollbar()

	viewportWithScrollbar := lipgloss.JoinHorizontal(
		lipgloss.Top,
		cp.viewport.View(),
		scrollbar,
	)

	var content string
	if cp.EmptyLineAfterTitle {
		content = lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render(titleText),
			"",
			viewportWithScrollbar,
		)
	} else {
		content = lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render(titleText),
			viewportWithScrollbar,
		)
	}

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

// renderScrollbar renders a vertical scrollbar
func (cp *ContentPanel) renderScrollbar() string {
	return RenderScrollbar(cp.viewport.Height, cp.viewport.TotalLineCount(), cp.viewport.YOffset)
}

// updateViewportContent re-renders content with selection highlighting
func (cp *ContentPanel) updateViewportContent() {
	if len(cp.contentLines) == 0 {
		cp.viewport.SetContent("")
		return
	}

	var highlighted strings.Builder
	selStyle := styles.SelectedStyle.Background(lipgloss.Color(styles.SecondaryColor))

	start, end := cp.normalizeSelection()

	for i, line := range cp.contentLines {
		if i > 0 {
			highlighted.WriteString("\n")
		}

		// Strip markers for display (markers are kept in contentLines for copy logic)
		displayLine := utils.StripAllMarkers(line)

		if !cp.hasSelection && !cp.selecting {
			highlighted.WriteString(displayLine)
			continue
		}

		lineWidth := ansi.PrintableRuneWidth(displayLine)

		if i < start.Line || i > end.Line {
			highlighted.WriteString(displayLine)
			continue
		}

		highlighted.WriteString(cp.highlightLine(
			displayLine,
			lineWidth,
			i,
			start,
			end,
			selStyle,
		))
	}

	cp.viewport.SetContent(highlighted.String())
}

// highlightLine applies selection highlighting to a single line
func (cp *ContentPanel) highlightLine(line string, lineWidth, lineNum int, start, end SelectionPos, selStyle lipgloss.Style) string {
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

	return cp.applyHighlight(line, selStartCol, selEndCol, selStyle)
}

// applyHighlight applies highlighting to a portion of a line, handling ANSI codes
func (cp *ContentPanel) applyHighlight(line string, startCol, endCol int, selStyle lipgloss.Style) string {
	var result strings.Builder
	var currentCol int
	var inAnsi bool
	var ansiSeq strings.Builder

	runes := []rune(line)
	for i := range runes {
		r := runes[i]

		// Handle ANSI escape sequences
		if r == '\x1b' && i+1 < len(runes) && runes[i+1] == '[' {
			inAnsi = true
			ansiSeq.Reset()
			ansiSeq.WriteRune(r)
			continue
		}

		if inAnsi {
			ansiSeq.WriteRune(r)
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				// End of ANSI sequence
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

// normalizeSelection returns start and end with start always before end
func (cp *ContentPanel) normalizeSelection() (SelectionPos, SelectionPos) {
	start, end := cp.selStart, cp.selEnd

	if start.Line > end.Line || (start.Line == end.Line && start.Col > end.Col) {
		start, end = end, start
	}

	return start, end
}

// GetSelectedText returns the currently selected text
func (cp *ContentPanel) GetSelectedText() string {
	if !cp.hasSelection || len(cp.contentLines) == 0 {
		return ""
	}

	start, end := cp.normalizeSelection()

	var result strings.Builder
	for i := start.Line; i <= end.Line && i < len(cp.contentLines); i++ {
		if i < 0 {
			continue
		}

		line := cp.contentLines[i]
		isContinuation := isSoftLineBreak(line)

		plainLine := utils.StripAllMarkers(stripAnsi(line))
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

		if i > start.Line && !isContinuation {
			result.WriteString("\n")
		}

		if startCol < endCol {
			result.WriteString(string(runes[startCol:endCol]))
		}
	}

	return result.String()
}

// ClearSelection clears the current selection
func (cp *ContentPanel) ClearSelection() {
	cp.hasSelection = false
	cp.selecting = false
	cp.selStart = SelectionPos{-1, -1}
	cp.selEnd = SelectionPos{-1, -1}
	cp.updateViewportContent()
}

// SetContent sets the content to display
func (cp *ContentPanel) SetContent(content string) {
	cp.contentLines = strings.Split(content, "\n")
	cp.ClearSelection()
	cp.viewport.SetContent(content)
}

// SetContentWithWrapping sets content and wraps it to the given width
// Uses SoftLineBreak markers so copying preserves original line structure
func (cp *ContentPanel) SetContentWithWrapping(content string, wrapWidth int) {
	if wrapWidth <= 0 {
		wrapWidth = 70
	}

	lines := strings.Split(content, "\n")
	var wrappedLines []string

	for _, line := range lines {
		wrapped := utils.WrapLine(line, wrapWidth)
		wrappedLines = append(wrappedLines, wrapped...)
	}

	cp.contentLines = wrappedLines
	cp.ClearSelection()
	cp.viewport.SetContent(utils.StripAllMarkers(strings.Join(wrappedLines, "\n")))
}

// SetContentLines sets the content lines directly (useful for pre-processed content)
func (cp *ContentPanel) SetContentLines(lines []string) {
	cp.contentLines = lines
	cp.ClearSelection()
	cp.viewport.SetContent(utils.StripAllMarkers(strings.Join(lines, "\n")))
}

// SetTitle sets the panel title
func (cp *ContentPanel) SetTitle(title string) {
	cp.title = title
}

// SetOffset sets the panel's position on screen (for mouse coordinate translation)
func (cp *ContentPanel) SetOffset(x, y int) {
	cp.xOffset = x
	cp.yOffset = y
}

// SetXOffset sets the panel's X position on screen (for mouse coordinate translation)
func (cp *ContentPanel) SetXOffset(offset int) {
	cp.xOffset = offset
}

// SetYOffset sets the panel's Y position on screen (for mouse coordinate translation)
func (cp *ContentPanel) SetYOffset(offset int) {
	cp.yOffset = offset
}

// ScrollUp scrolls up by n lines
func (cp *ContentPanel) ScrollUp(n int) {
	cp.viewport.ScrollUp(n)
}

// ScrollDown scrolls down by n lines
func (cp *ContentPanel) ScrollDown(n int) {
	cp.viewport.ScrollDown(n)
}

// HalfPageUp scrolls up by half a page
func (cp *ContentPanel) HalfPageUp() {
	halfPage := max(cp.viewport.Height/2, 1)
	cp.viewport.ScrollUp(halfPage)
}

// HalfPageDown scrolls down by half a page
func (cp *ContentPanel) HalfPageDown() {
	halfPage := max(cp.viewport.Height/2, 1)
	cp.viewport.ScrollDown(halfPage)
}

// GotoTop scrolls to the top
func (cp *ContentPanel) GotoTop() {
	cp.viewport.GotoTop()
}

// GotoBottom scrolls to the bottom
func (cp *ContentPanel) GotoBottom() {
	cp.viewport.GotoBottom()
}

// GetViewportWidth returns the viewport width
func (cp *ContentPanel) GetViewportWidth() int {
	return cp.viewport.Width
}

// GetRawContent returns the raw content without ANSI codes
func (cp *ContentPanel) GetRawContent() string {
	var lines []string
	for _, line := range cp.contentLines {
		lines = append(lines, stripAnsi(line))
	}
	return strings.Join(lines, "\n")
}

// UpdateContentLines updates content lines without clearing selection
// Use this when content changes but you want to preserve any active selection
// Lines may contain SoftLineBreak markers which will be stripped for display
// but preserved for copy operations
func (cp *ContentPanel) UpdateContentLines(lines []string) {
	cp.contentLines = lines
	cp.updateViewportContent()
}

// IsSelecting returns true if the user is currently selecting text
func (cp *ContentPanel) IsSelecting() bool {
	return cp.selecting || cp.hasSelection
}

// stripAnsi removes ANSI escape codes from a string
func stripAnsi(s string) string {
	var result strings.Builder
	var inAnsi bool

	for _, r := range s {
		if r == '\x1b' {
			inAnsi = true
			continue
		}
		if inAnsi {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inAnsi = false
			}
			continue
		}
		result.WriteRune(r)
	}

	return result.String()
}

// copyToClipboard copies text to system clipboard
func (cp *ContentPanel) copyToClipboard(text string) {
	if text == "" {
		return
	}

	if err := clipboard.WriteAll(text); err == nil {
		cp.showCopied = true
	}
}

// CopyText copies the given text to clipboard and shows the "[copied]" indicator.
// Returns a tea.Cmd to reset the indicator after a timeout.
func (cp *ContentPanel) CopyText(text string) tea.Cmd {
	cp.copyToClipboard(text)
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		cp.showCopied = false
		return struct{}{}
	})
}
