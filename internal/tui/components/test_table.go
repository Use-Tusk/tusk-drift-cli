package components

import (
	"fmt"
	"strings"

	"github.com/Use-Tusk/tusk-drift-cli/internal/runner"
	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type TestTableComponent struct {
	viewport viewport.Model
	tests    []runner.Test
	results  []runner.TestResult
	errors   []error

	// Column widths
	numWidth      int
	testWidth     int
	statusWidth   int
	durationWidth int

	completed []bool

	cursor     int
	lastCursor int // Track for change detection
}

func NewTestTableComponent(tests []runner.Test) *TestTableComponent {
	vp := viewport.New(50, 10)
	vp.Style = lipgloss.NewStyle()

	return &TestTableComponent{
		viewport:      vp,
		tests:         tests,
		results:       make([]runner.TestResult, len(tests)),
		errors:        make([]error, len(tests)),
		completed:     make([]bool, len(tests)),
		cursor:        0,
		lastCursor:    -1,
		numWidth:      4,
		testWidth:     35,
		statusWidth:   17,
		durationWidth: 8,
	}
}

func (tt *TestTableComponent) Update(msg tea.Msg) tea.Cmd {
	return nil
}

func (tt *TestTableComponent) View(width, height int) string {
	// Safety checks for dimensions
	if width <= 0 {
		width = 50
	}
	if height <= 0 {
		height = 10
	}

	// title (1) + margin (1) + header (1) = 3
	viewportHeight := max(height-3, 3)
	tt.viewport.Width = width
	tt.viewport.Height = viewportHeight

	fixedWidth := tt.numWidth + tt.statusWidth + tt.durationWidth + 6 // 6 for spacing
	tt.testWidth = max(width-fixedWidth, 10)

	// Build and set content
	tt.updateViewportContent()

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(styles.PrimaryColor)).
		MarginBottom(1)

	headerLine := fmt.Sprintf(" %-*s %-*s %-*s %-*s",
		tt.numWidth, "#",
		tt.testWidth, "Test",
		tt.statusWidth, "Status",
		tt.durationWidth, "Duration",
	)
	header := styles.TableHeaderStyle.Render(headerLine)

	return lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("Tests"),
		header,
		tt.viewport.View(),
	)
}

func (tt *TestTableComponent) updateViewportContent() {
	var sb strings.Builder

	totalRows := len(tt.tests) + 1 // +1 for service logs row

	for i := 0; i < totalRows; i++ {
		if i > 0 {
			sb.WriteString("\n")
		}

		var line string
		if i == 0 {
			serviceLabel := "(service logs)"
			if styles.NoColor() && tt.cursor == 0 {
				serviceLabel = "▶ " + serviceLabel
			}
			line = fmt.Sprintf(" %-*s %-*s %-*s %-*s",
				tt.numWidth, "",
				tt.testWidth, serviceLabel,
				tt.statusWidth, "",
				tt.durationWidth, "",
			)
		} else {
			testIdx := i - 1
			test := tt.tests[testIdx]

			status := "⏳ Pending"
			duration := "-"

			if tt.completed[testIdx] {
				result := tt.results[testIdx]
				err := tt.errors[testIdx]
				switch {
				case result.CrashedServer:
					status = "❌ Server crashed"
				case err != nil:
					status = "❌ Error"
				case result.Passed:
					status = "✅ No deviation"
				default:
					status = "🟠 Deviation"
				}
				duration = fmt.Sprintf("%dms", result.Duration)
			}

			testDescription := fmt.Sprintf("%s %s", test.DisplayType, test.DisplayName)
			if styles.NoColor() && tt.cursor == i {
				testDescription = "▶ " + testDescription
			}

			line = fmt.Sprintf(" %-*s %-*s %-*s %-*s",
				tt.numWidth, fmt.Sprintf("%d", testIdx+1),
				tt.testWidth, utils.TruncateWithEllipsis(testDescription, tt.testWidth),
				tt.statusWidth, status,
				tt.durationWidth, duration,
			)
		}

		if i == tt.cursor {
			sb.WriteString(styles.TableRowSelectedStyle.Render(line))
		} else {
			sb.WriteString(styles.TableCellStyle.Render(line))
		}
	}

	tt.viewport.SetContent(sb.String())
}

func (tt *TestTableComponent) GetSelectedTest() *runner.Test {
	// cursor 0 is service logs, tests start at cursor 1
	if tt.cursor == 0 {
		return nil
	}
	testIdx := tt.cursor - 1
	if testIdx >= 0 && testIdx < len(tt.tests) {
		return &tt.tests[testIdx]
	}
	return nil
}

func (tt *TestTableComponent) IsServiceLogsSelected() bool {
	return tt.cursor == 0
}

func (tt *TestTableComponent) UpdateTestResult(index int, result runner.TestResult, err error) {
	if index >= 0 && index < len(tt.results) {
		tt.results[index] = result
		tt.errors[index] = err
		tt.completed[index] = true
	}
}

func (tt *TestTableComponent) GotoTop() {
	tt.cursor = 0
	tt.viewport.GotoTop()
	tt.updateViewportContent()
}

func (tt *TestTableComponent) GotoBottom() {
	tt.cursor = len(tt.tests) // last row (tests + service logs - 1)
	tt.viewport.GotoBottom()
	tt.updateViewportContent()
}

func (tt *TestTableComponent) Height() int {
	return tt.viewport.Height
}

func (tt *TestTableComponent) TotalRows() int {
	return len(tt.tests) + 1 // +1 for service logs
}

func (tt *TestTableComponent) Cursor() int {
	return tt.cursor
}

// SelectUp moves the selection up by n rows (updates details panel)
func (tt *TestTableComponent) SelectUp(n int) {
	tt.cursor = max(tt.cursor-n, 0)
	tt.ensureCursorVisible()
	tt.updateViewportContent()
}

// SelectDown moves the selection down by n rows (updates details panel)
func (tt *TestTableComponent) SelectDown(n int) {
	maxCursor := len(tt.tests) // last valid cursor position
	tt.cursor = min(tt.cursor+n, maxCursor)
	tt.ensureCursorVisible()
	tt.updateViewportContent()
}

// ScrollUp scrolls the viewport up by n rows, clamping cursor to visible bounds
func (tt *TestTableComponent) ScrollUp(n int) {
	tt.viewport.ScrollUp(n)
	tt.clampCursorToViewport()
	tt.updateViewportContent()
}

// ScrollDown scrolls the viewport down by n rows, clamping cursor to visible bounds
func (tt *TestTableComponent) ScrollDown(n int) {
	tt.viewport.ScrollDown(n)
	tt.clampCursorToViewport()
	tt.updateViewportContent()
}

// MoveUp is an alias for SelectUp (for backwards compatibility)
func (tt *TestTableComponent) MoveUp(n int) {
	tt.SelectUp(n)
}

// MoveDown is an alias for SelectDown (for backwards compatibility)
func (tt *TestTableComponent) MoveDown(n int) {
	tt.SelectDown(n)
}

// HalfPageUp scrolls the viewport up by half a page, clamping cursor
func (tt *TestTableComponent) HalfPageUp() {
	tt.viewport.HalfPageUp()
	tt.clampCursorToViewport()
	tt.updateViewportContent()
}

// HalfPageDown scrolls the viewport down by half a page, clamping cursor
func (tt *TestTableComponent) HalfPageDown() {
	tt.viewport.HalfPageDown()
	tt.clampCursorToViewport()
	tt.updateViewportContent()
}

// ensureCursorVisible scrolls the viewport to keep the cursor visible
func (tt *TestTableComponent) ensureCursorVisible() {
	if tt.cursor < tt.viewport.YOffset {
		tt.viewport.SetYOffset(tt.cursor)
	} else if tt.cursor >= tt.viewport.YOffset+tt.viewport.Height {
		tt.viewport.SetYOffset(tt.cursor - tt.viewport.Height + 1)
	}
}

// clampCursorToViewport keeps the cursor within the visible viewport bounds
func (tt *TestTableComponent) clampCursorToViewport() {
	firstVisible := tt.viewport.YOffset
	lastVisible := tt.viewport.YOffset + tt.viewport.Height - 1
	maxCursor := len(tt.tests) // last valid cursor position
	if lastVisible > maxCursor {
		lastVisible = maxCursor
	}

	if tt.cursor < firstVisible {
		tt.cursor = firstVisible
	} else if tt.cursor > lastVisible {
		tt.cursor = lastVisible
	}
}

// ViewportYOffset returns the current viewport scroll offset
func (tt *TestTableComponent) ViewportYOffset() int {
	return tt.viewport.YOffset
}
