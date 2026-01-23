package components

import (
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/ansi"

	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
)

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

// DetailsPanel is a scrollable panel for displaying details with text selection support
type DetailsPanel struct {
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
	yOffset int // header offset within panel

	// Content storage for selection
	rawContent   string
	contentLines []string

	// Copied indicator
	showCopied bool

	// Auto-scroll state
	panelID       int             // Unique ID to identify this panel's messages
	autoScrollDir scrollDirection // Current auto-scroll direction
	lastMouseX    int             // Last mouse X during selection (viewport-relative)
	lastMouseY    int             // Last mouse Y during selection (viewport-relative)
}

// panelIDCounter is used to generate unique panel IDs
var panelIDCounter int

// NewDetailsPanel creates a new details panel
func NewDetailsPanel() *DetailsPanel {
	vp := viewport.New(50, 20)
	vp.Style = lipgloss.NewStyle()
	vp.MouseWheelEnabled = false

	panelIDCounter++
	return &DetailsPanel{
		viewport: vp,
		title:    "Details",
		yOffset:  1, // title only (border wraps everything now)
		selStart: SelectionPos{-1, -1},
		selEnd:   SelectionPos{-1, -1},
		panelID:  panelIDCounter,
	}
}

// Update handles input messages
func (dp *DetailsPanel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		switch msg.String() {
		case "g":
			dp.viewport.GotoTop()
			return nil
		case "G":
			dp.viewport.GotoBottom()
			return nil
		case "J":
			dp.viewport.ScrollDown(1)
			return nil
		case "K":
			dp.viewport.ScrollUp(1)
			return nil
		case "D", "ctrl+d":
			dp.viewport.HalfPageDown()
			return nil
		case "U", "ctrl+u":
			dp.viewport.HalfPageUp()
			return nil
		}

	case tea.MouseMsg:
		return dp.handleMouse(msg)

	case autoScrollMsg:
		// Only handle if this message is for this panel
		if msg.panelID != dp.panelID {
			return nil
		}
		return dp.handleAutoScroll()
	}

	var cmd tea.Cmd
	dp.viewport, cmd = dp.viewport.Update(msg)
	return cmd
}

const (
	autoScrollInterval = 50 * time.Millisecond
	scrollZone         = 2 // lines from edge to trigger scroll
)

// handleMouse processes mouse events for text selection
func (dp *DetailsPanel) handleMouse(msg tea.MouseMsg) tea.Cmd {
	x := msg.X - dp.xOffset
	y := msg.Y - dp.yOffset

	if x < 0 || x >= dp.viewport.Width || y < 0 {
		return nil
	}

	contentLine := y + dp.viewport.YOffset
	contentCol := x

	switch msg.Button {
	case tea.MouseButtonLeft:
		switch msg.Action {
		case tea.MouseActionPress:
			dp.selecting = true
			dp.selStart = SelectionPos{Line: contentLine, Col: contentCol}
			dp.selEnd = dp.selStart
			dp.hasSelection = false
			dp.autoScrollDir = scrollNone
			dp.updateViewportContent()

		case tea.MouseActionMotion:
			if !dp.selecting {
				return nil
			}

			dp.lastMouseX = x
			dp.lastMouseY = y

			prevDir := dp.autoScrollDir
			dp.autoScrollDir = scrollNone

			if y < scrollZone && dp.viewport.YOffset > 0 {
				dp.autoScrollDir = scrollUp
				dp.viewport.ScrollUp(1)
			} else if y >= dp.viewport.Height-scrollZone &&
				dp.viewport.YOffset < dp.viewport.TotalLineCount()-dp.viewport.Height {
				dp.autoScrollDir = scrollDown
				dp.viewport.ScrollDown(1)
			}

			contentLine = y + dp.viewport.YOffset
			dp.selEnd = SelectionPos{Line: contentLine, Col: contentCol}
			dp.hasSelection = true
			dp.updateViewportContent()

			if dp.autoScrollDir != scrollNone && prevDir == scrollNone {
				return dp.scheduleAutoScroll()
			}

		case tea.MouseActionRelease:
			if !dp.selecting {
				return nil
			}

			dp.selEnd = SelectionPos{Line: contentLine, Col: contentCol}
			hasSelection := dp.selStart != dp.selEnd

			var textToCopy string
			if hasSelection {
				dp.hasSelection = true
				textToCopy = dp.GetSelectedText()
			}

			dp.selecting = false
			dp.hasSelection = false
			dp.autoScrollDir = scrollNone
			dp.selStart = SelectionPos{-1, -1}
			dp.selEnd = SelectionPos{-1, -1}
			dp.copyToClipboard(textToCopy)
			dp.updateViewportContent()

			return tea.Tick(time.Second, func(t time.Time) tea.Msg {
				dp.showCopied = false
				return struct{}{}
			})
		}

	case tea.MouseButtonWheelUp:
		dp.viewport.ScrollUp(3)
		return nil

	case tea.MouseButtonWheelDown:
		dp.viewport.ScrollDown(3)
		return nil
	}

	return nil
}

// scheduleAutoScroll returns a command that will trigger auto-scroll after a delay
func (dp *DetailsPanel) scheduleAutoScroll() tea.Cmd {
	id := dp.panelID
	return tea.Tick(autoScrollInterval, func(t time.Time) tea.Msg {
		return autoScrollMsg{panelID: id}
	})
}

// handleAutoScroll processes auto-scroll ticks during selection
func (dp *DetailsPanel) handleAutoScroll() tea.Cmd {
	// Stop if no longer selecting or no scroll direction
	if !dp.selecting || dp.autoScrollDir == scrollNone {
		return nil
	}

	// Check if we can still scroll in the current direction
	canScrollUp := dp.viewport.YOffset > 0
	canScrollDown := dp.viewport.YOffset < dp.viewport.TotalLineCount()-dp.viewport.Height

	switch dp.autoScrollDir {
	case scrollUp:
		if !canScrollUp {
			dp.autoScrollDir = scrollNone
			return nil
		}
		dp.viewport.ScrollUp(1)
	case scrollDown:
		if !canScrollDown {
			dp.autoScrollDir = scrollNone
			return nil
		}
		dp.viewport.ScrollDown(1)
	}

	// Update selection end position based on scroll
	contentLine := dp.lastMouseY + dp.viewport.YOffset
	dp.selEnd = SelectionPos{Line: contentLine, Col: dp.lastMouseX}
	dp.updateViewportContent()

	// Schedule next auto-scroll tick
	return dp.scheduleAutoScroll()
}

// View renders the panel
func (dp *DetailsPanel) View() string {
	title := dp.title
	copiedIndicator := ""
	if dp.showCopied {
		copiedIndicator = styles.SuccessStyle.Render(" [copied]")
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(styles.PrimaryColor))
	titleText := title + copiedIndicator

	// Remove border from viewport - we'll add it to the container
	dp.viewport.Style = lipgloss.NewStyle()

	scrollbar := dp.renderScrollbar()

	viewportWithScrollbar := lipgloss.JoinHorizontal(
		lipgloss.Top,
		dp.viewport.View(),
		scrollbar,
	)

	// Combine title and viewport, then wrap with border
	content := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render(titleText),
		viewportWithScrollbar,
	)

	// Container with left border that spans the full height
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
func (dp *DetailsPanel) renderScrollbar() string {
	return RenderScrollbar(dp.viewport.Height, dp.viewport.TotalLineCount(), dp.viewport.YOffset)
}

// updateViewportContent re-renders content with selection highlighting
func (dp *DetailsPanel) updateViewportContent() {
	if len(dp.contentLines) == 0 {
		return
	}

	var highlighted strings.Builder
	selStyle := styles.SelectedStyle.Background(lipgloss.Color(styles.SecondaryColor))

	start, end := dp.normalizeSelection()

	for i, line := range dp.contentLines {
		if i > 0 {
			highlighted.WriteString("\n")
		}

		if !dp.hasSelection && !dp.selecting {
			highlighted.WriteString(line)
			continue
		}

		lineWidth := ansi.PrintableRuneWidth(line)

		if i < start.Line || i > end.Line {
			highlighted.WriteString(line)
			continue
		}

		highlighted.WriteString(dp.highlightLine(
			line,
			lineWidth,
			i,
			start,
			end,
			selStyle,
		))
	}

	dp.viewport.SetContent(highlighted.String())
}

// highlightLine applies selection highlighting to a single line
func (dp *DetailsPanel) highlightLine(line string, lineWidth, lineNum int, start, end SelectionPos, selStyle lipgloss.Style) string {
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

	return dp.applyHighlight(line, selStartCol, selEndCol, selStyle)
}

// applyHighlight applies highlighting to a portion of a line, handling ANSI codes
func (dp *DetailsPanel) applyHighlight(line string, startCol, endCol int, selStyle lipgloss.Style) string {
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
func (dp *DetailsPanel) normalizeSelection() (SelectionPos, SelectionPos) {
	start, end := dp.selStart, dp.selEnd

	if start.Line > end.Line || (start.Line == end.Line && start.Col > end.Col) {
		start, end = end, start
	}

	return start, end
}

// GetSelectedText returns the currently selected text
func (dp *DetailsPanel) GetSelectedText() string {
	if !dp.hasSelection || len(dp.contentLines) == 0 {
		return ""
	}

	start, end := dp.normalizeSelection()

	var result strings.Builder

	for i := start.Line; i <= end.Line && i < len(dp.contentLines); i++ {
		if i < 0 {
			continue
		}

		line := dp.contentLines[i]
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

		// Clamp
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

// ClearSelection clears the current selection
func (dp *DetailsPanel) ClearSelection() {
	dp.hasSelection = false
	dp.selecting = false
	dp.selStart = SelectionPos{-1, -1}
	dp.selEnd = SelectionPos{-1, -1}
	dp.updateViewportContent()
}

// SetContent sets the content to display
func (dp *DetailsPanel) SetContent(content string) {
	dp.rawContent = content
	dp.contentLines = strings.Split(content, "\n")
	dp.ClearSelection()
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

	// Account for: left border (1), left padding (1), scrollbar (1) = 3
	viewportWidth := width - 3
	// Account for: title (1) - no top/bottom borders
	viewportHeight := height - 1

	if viewportWidth < 10 {
		viewportWidth = 10
	}
	if viewportHeight < 3 {
		viewportHeight = 3
	}

	dp.viewport.Width = viewportWidth
	dp.viewport.Height = viewportHeight
}

// SetXOffset sets the panel's X position on screen (for mouse coordinate translation)
func (dp *DetailsPanel) SetXOffset(offset int) {
	dp.xOffset = offset
}

// SetYOffset sets the panel's Y position on screen (for mouse coordinate translation)
func (dp *DetailsPanel) SetYOffset(offset int) {
	dp.yOffset = offset
}

// GotoTop scrolls to the top
func (dp *DetailsPanel) GotoTop() {
	dp.viewport.GotoTop()
}

// GotoBottom scrolls to the bottom
func (dp *DetailsPanel) GotoBottom() {
	dp.viewport.GotoBottom()
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

// copyToClipboard returns a command to copy text to clipboard
func (dp *DetailsPanel) copyToClipboard(text string) tea.Cmd {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		// Try xclip first, fall back to xsel
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return nil
		}
	default:
		return nil
	}

	cmd.Stdin = strings.NewReader(text)
	_ = cmd.Run()
	dp.showCopied = true
	return nil
}
