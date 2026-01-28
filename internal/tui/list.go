package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"google.golang.org/protobuf/types/known/structpb"

	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"

	"github.com/Use-Tusk/tusk-drift-cli/internal/log"
	"github.com/Use-Tusk/tusk-drift-cli/internal/runner"
	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/components"
	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
)

// spanTreeNode represents a span in the tree hierarchy
type spanTreeNode struct {
	span     *core.Span
	children []*spanTreeNode
}

// rowInfo tracks what each visible row represents
type rowInfo struct {
	testIndex int    // Index in the tests slice
	spanID    string // Empty for the root trace row, span ID for child spans
	isTrace   bool   // True if this is the main trace row (not a child span)
}

type viewState int

const (
	listView viewState = iota
	testExecutionView
)

// tableRow holds the data for a single row
type tableRow struct {
	number  string
	traceID string
	rowType string
	name    string
}

// detailsUpdateInterval is the minimum time between details panel updates.
// Because details is usually a lot of content it causes some lag when scrolling
// rapidly, so we have to "debounce" it a little.
const detailsUpdateInterval = 100 * time.Millisecond

// detailsTickMsg is sent to trigger a pending details update
type detailsTickMsg struct{}

type listModel struct {
	viewport       viewport.Model
	tests          []runner.Test
	executor       *runner.Executor
	width          int
	height         int
	state          viewState
	testExecutor   *testExecutorModel
	selectedTest   *runner.Test
	suiteOpts      runner.SuiteSpanOptions
	err            error
	sizeWarning    *components.TerminalSizeWarning
	expandedTraces map[string]bool // map of trace id to whether it's expanded
	rowInfos       []rowInfo
	rows           []tableRow // Pre-built row data
	cursor         int        // Currently selected row
	detailsPanel   *components.DetailsPanel
	lastCursor     int
	detailsCache   map[string]string

	// Debouncing for details panel updates
	lastDetailsUpdate    time.Time
	pendingDetailsUpdate bool

	// Pre-rendered row cache (invalidated when width or rows change)
	renderedRowsNormal   []string // Each row rendered with normal style
	renderedRowsSelected []string // Each row rendered with selected style
	lastRenderedWidth    int      // Width used for last render
}

func ShowTestList(tests []runner.Test) error {
	return ShowTestListWithExecutor(tests, nil, runner.SuiteSpanOptions{})
}

func ShowTestListWithExecutor(tests []runner.Test, executor *runner.Executor, suiteOpts runner.SuiteSpanOptions) error {
	vp := viewport.New(50, 20)
	vp.Style = lipgloss.NewStyle()

	m := &listModel{
		viewport:       vp,
		tests:          tests,
		executor:       executor,
		state:          listView,
		suiteOpts:      suiteOpts,
		sizeWarning:    components.NewListViewSizeWarning(),
		expandedTraces: make(map[string]bool),
		detailsPanel:   components.NewDetailsPanel(),
		cursor:         0,
		lastCursor:     -1,
		detailsCache:   make(map[string]string),
	}

	m.rebuildRows()
	m.updateDetailsContent()

	if _, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run(); err != nil {
		return err
	}

	return nil
}

func (m *listModel) Init() tea.Cmd {
	return nil
}

func (m *listModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case detailsTickMsg:
		if m.pendingDetailsUpdate && m.cursor != m.lastCursor {
			m.lastCursor = m.cursor
			m.lastDetailsUpdate = time.Now()
			m.pendingDetailsUpdate = false
			m.updateDetailsContent()
		}
		return m, nil

	case testsLoadFailedMsg:
		m.err = msg.err
		return m, tea.Quit

	case tea.KeyMsg:
		if m.state == listView && m.sizeWarning.ShouldShow(m.width, m.height) {
			switch msg.String() {
			case "enter", "d", "D":
				m.sizeWarning.Dismiss()
				return m, nil
			case "q", "ctrl+c", "esc":
				return m, tea.Quit
			}
			return m, nil
		}

		switch m.state {
		case listView:
			switch msg.String() {
			case "q", "ctrl+c", "esc":
				return m, tea.Quit
			case "enter":
				if m.cursor >= 0 &&
					m.cursor < len(m.rowInfos) &&
					m.executor != nil {
					info := m.rowInfos[m.cursor]
					test := m.tests[info.testIndex]
					opts := &InteractiveOpts{
						IsCloudMode:              m.suiteOpts.IsCloudMode,
						OnBeforeEnvironmentStart: m.createSuiteSpanPreparation(),
					}
					executor := newTestExecutorModel([]runner.Test{test}, m.executor, opts)

					log.SetTUILogger(executor)

					if m.width > 0 && m.height > 0 {
						sizeMsg := tea.WindowSizeMsg{Width: m.width, Height: m.height}
						updatedModel, _ := executor.Update(sizeMsg)
						executor = updatedModel.(*testExecutorModel)
					}

					m.testExecutor = executor
					m.selectedTest = &test
					m.state = testExecutionView
					return m, m.testExecutor.Init()
				}
			case "j", "down":
				if m.cursor < len(m.rows)-1 {
					m.cursor++
					m.ensureCursorVisible()
					m.updateViewportContent()
				}
				return m, m.scheduleDetailsUpdate()
			case "k", "up":
				if m.cursor > 0 {
					m.cursor--
					m.ensureCursorVisible()
					m.updateViewportContent()
				}
				return m, m.scheduleDetailsUpdate()
			case "u", "ctrl+u":
				m.viewport.HalfPageUp()
				m.clampCursorToViewport()
				m.updateViewportContent()
				return m, m.scheduleDetailsUpdate()
			case "d", "ctrl+d":
				m.viewport.HalfPageDown()
				m.clampCursorToViewport()
				m.updateViewportContent()
				return m, m.scheduleDetailsUpdate()
			case "J":
				m.detailsPanel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'J'}})
				return m, nil
			case "K":
				m.detailsPanel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'K'}})
				return m, nil
			case "U":
				m.detailsPanel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
				return m, nil
			case "D":
				m.detailsPanel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
				return m, nil
			case "g":
				m.cursor = 0
				m.viewport.GotoTop()
				m.updateViewportContent()
				return m, m.scheduleDetailsUpdate()
			case "G":
				if len(m.rows) > 0 {
					m.cursor = len(m.rows) - 1
					m.viewport.GotoBottom()
					m.updateViewportContent()
				}
				return m, m.scheduleDetailsUpdate()
			case "left", "h":
				if m.cursor >= 0 && m.cursor < len(m.rowInfos) {
					info := m.rowInfos[m.cursor]
					traceID := m.tests[info.testIndex].TraceID
					if m.expandedTraces[traceID] {
						m.expandedTraces[traceID] = false
						m.rebuildRows()
						m.lastCursor = -1
						m.updateDetailsContent()
					}
				}
				return m, nil
			case "right", "l":
				if m.cursor >= 0 && m.cursor < len(m.rowInfos) {
					info := m.rowInfos[m.cursor]
					test := m.tests[info.testIndex]
					if len(test.Spans) > 0 && !m.expandedTraces[test.TraceID] {
						m.expandedTraces[test.TraceID] = true
						m.rebuildRows()
						m.lastCursor = -1
						m.updateDetailsContent()
					}
				}
				return m, nil
			}
		case testExecutionView:
			if m.testExecutor != nil && m.testExecutor.state == stateCompleted {
				switch msg.String() {
				case "q", "ctrl+c", "esc", "enter", " ":
					// Clean up and return to list
					m.testExecutor.cleanup()
					log.SetTUILogger(nil)

					m.state = listView
					m.testExecutor = nil
					m.selectedTest = nil
					return m, nil
				}
			}
			// Otherwise, forward to test executor
			if m.testExecutor != nil {
				var executorCmd tea.Cmd
				updatedExecutor, executorCmd := m.testExecutor.Update(msg)
				if exec, ok := updatedExecutor.(*testExecutorModel); ok {
					m.testExecutor = exec
				}
				return m, executorCmd
			}
		}

	case tea.MouseMsg:
		if m.state == listView {
			tableWidth := m.width / 2

			// Check if mouse is over the table (left side)
			if msg.X < tableWidth {
				switch msg.Button {
				case tea.MouseButtonWheelUp:
					oldCursor := m.cursor
					m.viewport.ScrollUp(3)
					m.clampCursorToViewport()
					// Only rebuild content if cursor changed (needs new highlighting)
					if m.cursor != oldCursor {
						m.updateViewportContent()
					}
					return m, m.scheduleDetailsUpdate()
				case tea.MouseButtonWheelDown:
					oldCursor := m.cursor
					m.viewport.ScrollDown(3)
					m.clampCursorToViewport()
					// Only rebuild content if cursor changed (needs new highlighting)
					if m.cursor != oldCursor {
						m.updateViewportContent()
					}
					return m, m.scheduleDetailsUpdate()
				}
			} else {
				// Mouse is over details panel (right side)
				cmd := m.detailsPanel.Update(msg)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
				return m, tea.Batch(cmds...)
			}
		}

	case tea.WindowSizeMsg:
		oldWidth := m.width
		oldHeight := m.height
		m.width = msg.Width
		m.height = msg.Height

		wasLargeEnough := !m.sizeWarning.IsTooSmall(oldWidth, oldHeight)
		isNowTooSmall := m.sizeWarning.IsTooSmall(m.width, m.height)

		if wasLargeEnough && isNowTooSmall {
			m.sizeWarning.Reset()
		}

		if m.state == listView {
			tableWidth := msg.Width/2 - 1 // -1 for scrollbar
			// header (1) + empty line (1) + "Tests" title (1) + table header (2) + margin (1) + footer (1) = 7
			tableHeight := msg.Height - 7
			if tableHeight < 1 {
				tableHeight = 1
			}
			m.viewport.Width = tableWidth
			m.viewport.Height = tableHeight
			m.updateViewportContent()
		} else if m.state == testExecutionView && m.testExecutor != nil {
			// Forward window size to test executor
			updatedExecutor, cmd := m.testExecutor.Update(msg)
			if exec, ok := updatedExecutor.(*testExecutorModel); ok {
				m.testExecutor = exec
			}
			cmds = append(cmds, cmd)
		}

	default:
		// Forward all other messages to the appropriate view
		if m.state == listView {
			// Forward to details panel (for auto-scroll messages, etc.)
			cmd := m.detailsPanel.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		} else if m.state == testExecutionView && m.testExecutor != nil {
			updatedExecutor, cmd := m.testExecutor.Update(msg)
			if exec, ok := updatedExecutor.(*testExecutorModel); ok {
				m.testExecutor = exec
			}
			return m, cmd
		}
	}

	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// ensureCursorVisible scrolls the viewport to keep the cursor visible
func (m *listModel) ensureCursorVisible() {
	if m.cursor < m.viewport.YOffset {
		m.viewport.SetYOffset(m.cursor)
	} else if m.cursor >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.SetYOffset(m.cursor - m.viewport.Height + 1)
	}
}

// clampCursorToViewport keeps the cursor within the visible viewport bounds
func (m *listModel) clampCursorToViewport() {
	firstVisible := m.viewport.YOffset
	lastVisible := m.viewport.YOffset + m.viewport.Height - 1
	if lastVisible >= len(m.rows) {
		lastVisible = len(m.rows) - 1
	}

	if m.cursor < firstVisible {
		m.cursor = firstVisible
	} else if m.cursor > lastVisible {
		m.cursor = lastVisible
	}
}

// renderRowCache pre-renders all rows in both normal and selected styles.
// This is called when width changes or rows are rebuilt.
func (m *listModel) renderRowCache() {
	tableWidth := m.viewport.Width
	numWidth := 4
	traceWidth := 32
	typeWidth := 10
	nameWidth := max(tableWidth-numWidth-traceWidth-typeWidth-6, 10)

	m.renderedRowsNormal = make([]string, len(m.rows))
	m.renderedRowsSelected = make([]string, len(m.rows))
	m.lastRenderedWidth = tableWidth

	for i, row := range m.rows {
		line := fmt.Sprintf(" %-*s %-*s %-*s %-*s",
			numWidth, utils.TruncateWithEllipsis(row.number, numWidth),
			traceWidth, utils.TruncateWithEllipsis(row.traceID, traceWidth),
			typeWidth, utils.TruncateWithEllipsis(row.rowType, typeWidth),
			nameWidth, utils.TruncateWithEllipsis(row.name, nameWidth),
		)
		m.renderedRowsNormal[i] = styles.TableCellStyle.Render(line)
		m.renderedRowsSelected[i] = styles.TableRowSelectedStyle.Render(line)
	}
}

// updateViewportContent assembles the viewport content from pre-rendered rows.
// Only re-renders rows if width changed.
func (m *listModel) updateViewportContent() {
	tableWidth := m.viewport.Width

	// Re-render cache if width changed or cache is empty
	if tableWidth != m.lastRenderedWidth || len(m.renderedRowsNormal) != len(m.rows) {
		m.renderRowCache()
	}

	// Fast path: just assemble from pre-rendered strings
	var sb strings.Builder
	for i := range m.rows {
		if i > 0 {
			sb.WriteString("\n")
		}
		if i == m.cursor {
			sb.WriteString(m.renderedRowsSelected[i])
		} else {
			sb.WriteString(m.renderedRowsNormal[i])
		}
	}

	m.viewport.SetContent(sb.String())
}

// scheduleDetailsUpdate schedules a debounced update to the details panel.
// Updates are ALWAYS deferred via tea.Tick to avoid blocking the main update loop.
func (m *listModel) scheduleDetailsUpdate() tea.Cmd {
	if m.cursor == m.lastCursor {
		return nil
	}

	// Always schedule via tick with full debounce interval
	if !m.pendingDetailsUpdate {
		m.pendingDetailsUpdate = true
		return tea.Tick(detailsUpdateInterval, func(t time.Time) tea.Msg {
			return detailsTickMsg{}
		})
	}
	return nil
}

func (m *listModel) View() string {
	switch m.state {
	case listView:
		if m.sizeWarning.ShouldShow(m.width, m.height) {
			return m.sizeWarning.View(m.width, m.height)
		}

		header := components.Title(m.width, "AVAILABLE TESTS")
		testCount := fmt.Sprintf("%d TESTS ", len(m.tests))

		usedWidth := lipgloss.Width(testCount)
		availableWidthForHelp := m.width - usedWidth
		availableWidthForHelp = max(availableWidthForHelp, 20)

		footer := utils.TruncateWithEllipsis(
			testCount+"• j/k: select • u/d: scroll • g/G: top/bottom • J/K/U/D: scroll details • ←/→: expand • enter: run • q: quit",
			availableWidthForHelp,
		)

		help := components.Footer(m.width, footer)

		tableWidth := m.width / 2
		detailsWidth := m.width - tableWidth

		m.detailsPanel.SetSize(detailsWidth, m.height-3)
		m.detailsPanel.SetXOffset(tableWidth + 2) // panel position + border (1) + padding (1)
		m.detailsPanel.SetYOffset(3)              // header (1) + empty line (1) + title (1)

		testsSectionTitle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(styles.PrimaryColor)).
			MarginBottom(1).
			Render("Tests")

		// Render table header - use same widths as updateViewportContent
		numWidth := 4
		traceWidth := 32
		typeWidth := 10
		nameWidth := tableWidth - numWidth - traceWidth - typeWidth - 6 // 6 for spacing
		if nameWidth < 10 {
			nameWidth = 10
		}
		headerLine := fmt.Sprintf(" %-*s %-*s %-*s %-*s", numWidth, "#", traceWidth, "Trace ID", typeWidth, "Type", nameWidth, "Name")
		tableHeader := styles.TableHeaderStyle.Render(headerLine)

		// Render table with scrollbar
		tableWithScrollbar := lipgloss.JoinHorizontal(
			lipgloss.Top,
			m.viewport.View(),
			m.renderTableScrollbar(),
		)

		leftStyle := lipgloss.NewStyle().MaxWidth(tableWidth)
		rightStyle := lipgloss.NewStyle().MaxWidth(detailsWidth)

		leftSide := leftStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
			testsSectionTitle,
			tableHeader,
			tableWithScrollbar,
		))

		mainContent := lipgloss.JoinHorizontal(
			lipgloss.Top,
			leftSide,
			rightStyle.Render(m.detailsPanel.View()),
		)

		return lipgloss.JoinVertical(lipgloss.Left, header, "", mainContent, help)

	case testExecutionView:
		if m.testExecutor != nil {
			return m.testExecutor.View()
		}
		return "Loading test executor..."
	default:
		return "Unknown state"
	}
}

// updateDetailsContent updates the details panel content based on current selection
func (m *listModel) updateDetailsContent() {
	if m.cursor < 0 || m.cursor >= len(m.rowInfos) {
		m.detailsPanel.SetContent("No selection")
		m.detailsPanel.SetTitle("Details")
		return
	}

	info := m.rowInfos[m.cursor]

	// Set title based on selection type
	if info.isTrace {
		m.detailsPanel.SetTitle("Trace Details")
	} else {
		m.detailsPanel.SetTitle("Span Details")
	}

	cacheKey := fmt.Sprintf("%d:%s", info.testIndex, info.spanID)

	if cached, ok := m.detailsCache[cacheKey]; ok {
		m.detailsPanel.SetContent(cached)
		m.detailsPanel.GotoTop()
		return
	}

	lines := m.generateDetailsContent()
	content := utils.RenderMarkdown(strings.Join(lines, "\n"))
	m.detailsCache[cacheKey] = content
	m.detailsPanel.SetContent(content)
	m.detailsPanel.GotoTop()
}

// lineBuilder helps build lines for the details panel efficiently
type lineBuilder struct {
	lines []string
}

func newLineBuilder(capacity int) *lineBuilder {
	return &lineBuilder{lines: make([]string, 0, capacity)}
}

func (b *lineBuilder) blank() {
	b.lines = append(b.lines, "")
}

func (b *lineBuilder) header(title string) {
	b.lines = append(b.lines, fmt.Sprintf("# %s", title))
}

func (b *lineBuilder) field(key string, value any) {
	b.lines = append(b.lines, fmt.Sprintf("- %s: %v", key, value))
}

func (b *lineBuilder) addSortedMap(header string, m map[string]string) {
	if len(m) == 0 {
		return
	}
	b.lines = append(b.lines, fmt.Sprintf("- %s:", header))
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.lines = append(b.lines, fmt.Sprintf("  - %s: %s", k, m[k]))
	}
}

func (b *lineBuilder) addJSON(label string, indent string, data any) {
	if data == nil {
		return
	}
	if label != "" {
		b.lines = append(b.lines, fmt.Sprintf("- %s:", label))
	}
	b.lines = append(b.lines, "```json")
	jsonBytes, _ := json.MarshalIndent(data, "", "  ")
	for line := range strings.SplitSeq(string(jsonBytes), "\n") {
		b.lines = append(b.lines, indent+line)
	}
	b.lines = append(b.lines, "```")
}

func (b *lineBuilder) build() []string {
	return b.lines
}

// generateDetailsContent generates the details content for the currently selected item
func (m *listModel) generateDetailsContent() []string {
	if m.cursor < 0 || m.cursor >= len(m.rowInfos) {
		return []string{"No selection"}
	}

	info := m.rowInfos[m.cursor]
	test := m.tests[info.testIndex]

	b := newLineBuilder(64) // Pre-allocate reasonable capacity

	if info.isTrace {
		b.header("TRACE DETAILS")
		b.blank()
		b.field("Trace ID", test.TraceID)
		b.field("Type", test.DisplayType)
		b.field("Method", test.Method)
		b.field("Path", test.Path)
		b.field("Status", test.Status)
		b.field("Duration", fmt.Sprintf("%dms", test.Duration))
		b.field("Environment", test.Environment)
		b.field("Timestamp", test.Timestamp)
		b.field("File", test.FileName)
		b.field("Spans", len(test.Spans))

		var rootSpan *core.Span
		for _, span := range test.Spans {
			if span.IsRootSpan {
				rootSpan = span
				break
			}
		}

		b.blank()
		b.header("REQUEST")
		b.blank()
		b.field("Method", test.Request.Method)
		b.field("Path", test.Request.Path)
		b.addSortedMap("Headers", test.Request.Headers)

		reqBody := test.Request.Body
		if reqBody != nil && rootSpan != nil && rootSpan.InputSchema != nil {
			if bodySchema := rootSpan.InputSchema.Properties["body"]; bodySchema != nil {
				if _, decoded, err := runner.DecodeValueBySchema(reqBody, bodySchema); err == nil {
					reqBody = decoded
				}
			}
		}
		b.addJSON("Body", "  ", reqBody)

		b.blank()
		b.header("RESPONSE")
		b.blank()
		b.field("Status", test.Response.Status)
		b.addSortedMap("Headers", test.Response.Headers)

		// Decode response body using schema
		respBody := test.Response.Body
		if respBody != nil && rootSpan != nil && rootSpan.OutputSchema != nil {
			if bodySchema := rootSpan.OutputSchema.Properties["body"]; bodySchema != nil {
				if _, decoded, err := runner.DecodeValueBySchema(respBody, bodySchema); err == nil {
					respBody = decoded
				}
			}
		}
		b.addJSON("Body", "  ", respBody)
	} else {
		span := m.findSpan(test.Spans, info.spanID)
		if span == nil {
			return []string{"Span not found"}
		}

		b.header("SPAN DETAILS")
		b.blank()
		b.field("Span ID", span.SpanId)
		b.field("Parent ID", span.ParentSpanId)
		b.field("Name", span.Name)
		b.field("Package", span.PackageName)
		b.field("Instrumentation", span.InstrumentationName)
		b.field("Submodule", span.SubmoduleName)
		b.field("Kind", span.Kind.String())
		b.field("Is Root", span.IsRootSpan)

		if span.Duration != nil {
			b.field("Duration", fmt.Sprintf("%dms", span.Duration.AsDuration().Milliseconds()))
		}
		if span.Timestamp != nil {
			b.field("Timestamp", span.Timestamp.AsTime().Format(time.RFC3339))
		}
		if span.Status != nil {
			b.field("Status Code", span.Status.Code.String())
			if span.Status.Message != "" {
				b.field("Status Message", span.Status.Message)
			}
		}

		if span.InputValue != nil {
			b.blank()
			b.header("INPUT")
			decodedInput := decodeStructValue(span.InputValue, span.InputSchema)
			b.addJSON("", "", decodedInput)
		}

		if span.OutputValue != nil {
			b.blank()
			b.header("OUTPUT")
			decodedOutput := decodeStructValue(span.OutputValue, span.OutputSchema)
			b.addJSON("", "", decodedOutput)
		}

		if span.Metadata != nil {
			b.blank()
			b.header("METADATA")
			b.addJSON("", "", span.Metadata.AsMap())
		}
	}

	return b.build()
}

// decodeStructValue decodes a protobuf struct value using its schema.
// For each field in the struct, it attempts to decode using the corresponding schema property.
// This handles base64-encoded values that need decoding.
func decodeStructValue(value *structpb.Struct, schema *core.JsonSchema) map[string]any {
	if value == nil {
		return nil
	}

	result := make(map[string]any)
	for key, field := range value.Fields {
		fieldValue := field.AsInterface()

		var fieldSchema *core.JsonSchema
		if schema != nil && schema.Properties != nil {
			fieldSchema = schema.Properties[key]
		}

		if fieldValue != nil {
			_, decoded, err := runner.DecodeValueBySchema(fieldValue, fieldSchema)
			if err == nil && decoded != nil {
				result[key] = decoded
			} else {
				result[key] = fieldValue
			}
		} else {
			result[key] = fieldValue
		}
	}
	return result
}

// findSpan finds a span by ID in the test's spans
func (m *listModel) findSpan(spans []*core.Span, spanID string) *core.Span {
	for _, span := range spans {
		if span.SpanId == spanID {
			return span
		}
	}
	return nil
}

// createSuiteSpanPreparation creates the OnBeforeEnvironmentStart hook for preparing suite spans
func (m *listModel) createSuiteSpanPreparation() func(*runner.Executor, []runner.Test) error {
	return func(exec *runner.Executor, tests []runner.Test) error {
		return runner.PrepareAndSetSuiteSpans(
			context.Background(),
			exec,
			m.suiteOpts,
			tests,
		)
	}
}

// buildRows generates table rows and row mapping based on current expand state
func (m *listModel) buildRows() ([]tableRow, []rowInfo) {
	rows := []tableRow{}
	rowMapping := []rowInfo{}

	for i, test := range m.tests {
		displayType := test.DisplayType
		if displayType == "" {
			displayType = test.Type
		}
		displayPath := test.DisplayName
		if displayPath == "" {
			displayPath = test.Path
		}

		// Add expand/collapse indicator if test has spans
		expandIndicator := "  "
		if len(test.Spans) > 0 {
			if m.expandedTraces[test.TraceID] {
				expandIndicator = "▼ "
			} else {
				expandIndicator = "▶ "
			}
		}

		rows = append(rows, tableRow{
			number:  fmt.Sprintf("%d", i+1),
			traceID: expandIndicator + test.TraceID,
			rowType: displayType,
			name:    displayPath,
		})
		rowMapping = append(rowMapping, rowInfo{testIndex: i, isTrace: true})

		// If expanded, add child span rows
		if m.expandedTraces[test.TraceID] && len(test.Spans) > 0 {
			tree := buildSpanTree(test.Spans)
			m.addSpanRows(&rows, &rowMapping, tree, i, "", true)
		}
	}

	return rows, rowMapping
}

// buildSpanTree converts a flat list of spans into a tree structure
func buildSpanTree(spans []*core.Span) []*spanTreeNode {
	if len(spans) == 0 {
		return nil
	}

	// Create a map of span ID to node
	nodeMap := make(map[string]*spanTreeNode)
	for _, span := range spans {
		nodeMap[span.SpanId] = &spanTreeNode{span: span}
	}

	// Build tree by linking children to parents
	var roots []*spanTreeNode
	for _, span := range spans {
		node := nodeMap[span.SpanId]
		if span.ParentSpanId == "" || nodeMap[span.ParentSpanId] == nil {
			// This is a root span
			roots = append(roots, node)
		} else {
			// Add as child to parent
			parent := nodeMap[span.ParentSpanId]
			parent.children = append(parent.children, node)
		}
	}

	return roots
}

// addSpanRows recursively adds span rows to the table with tree visualization
func (m *listModel) addSpanRows(rows *[]tableRow, rowMapping *[]rowInfo, nodes []*spanTreeNode, testIndex int, prefix string, isRoot bool) {
	for i, node := range nodes {
		isLast := i == len(nodes)-1

		// Determine tree prefix characters
		var branchChar, childPrefix string
		if isRoot {
			if isLast {
				branchChar = "  └─ "
				childPrefix = prefix + "     "
			} else {
				branchChar = "  ├─ "
				childPrefix = prefix + "  │  "
			}
		} else {
			if isLast {
				branchChar = "└─ "
				childPrefix = prefix + "   "
			} else {
				branchChar = "├─ "
				childPrefix = prefix + "│  "
			}
		}

		// Get span display info
		spanName := node.span.Name
		if spanName == "" {
			spanName = node.span.PackageName
			if node.span.SubmoduleName != "" {
				spanName += "." + node.span.SubmoduleName
			}
		}

		spanType := node.span.PackageName
		if spanType == "" {
			spanType = "span"
		}

		*rows = append(*rows, tableRow{
			number:  "", // No row number for child spans
			traceID: prefix + branchChar + node.span.SpanId[:min(8, len(node.span.SpanId))],
			rowType: spanType,
			name:    spanName,
		})
		*rowMapping = append(*rowMapping, rowInfo{testIndex: testIndex, spanID: node.span.SpanId, isTrace: false})

		// Recursively add children
		if len(node.children) > 0 {
			m.addSpanRows(rows, rowMapping, node.children, testIndex, childPrefix, false)
		}
	}
}

// rebuildRows rebuilds the rows and updates the viewport content
func (m *listModel) rebuildRows() {
	rows, rowMapping := m.buildRows()
	m.rows = rows
	m.rowInfos = rowMapping

	// Keep cursor at a valid position
	if m.cursor >= len(rows) {
		m.cursor = len(rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}

	m.updateViewportContent()
}

// renderTableScrollbar renders a vertical scrollbar for the table
func (m *listModel) renderTableScrollbar() string {
	visibleRows := m.viewport.Height
	totalRows := len(m.rows)
	scrollOffset := m.viewport.YOffset

	scrollbarHeight := visibleRows

	return components.RenderScrollbar(scrollbarHeight, totalRows, scrollOffset)
}
