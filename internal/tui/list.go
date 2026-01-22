package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"google.golang.org/protobuf/types/known/structpb"

	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"

	"github.com/Use-Tusk/tusk-drift-cli/internal/logging"
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

type listModel struct {
	table          table.Model
	tests          []runner.Test
	executor       *runner.Executor
	width          int
	height         int
	state          viewState
	testExecutor   *testExecutorModel
	selectedTest   *runner.Test
	columns        []table.Column
	suiteOpts      runner.SuiteSpanOptions
	err            error
	sizeWarning    *components.TerminalSizeWarning
	expandedTraces map[string]bool // map of trace id to whether it's expanded
	rowInfos       []rowInfo
	detailsPanel   *components.DetailsPanel
	lastCursor     int // Track cursor to detect selection changes
}

func ShowTestList(tests []runner.Test) error {
	return ShowTestListWithExecutor(tests, nil, runner.SuiteSpanOptions{})
}

func ShowTestListWithExecutor(tests []runner.Test, executor *runner.Executor, suiteOpts runner.SuiteSpanOptions) error {
	columns := []table.Column{
		{Title: "#", Width: 4},
		{Title: "Trace ID", Width: 32},
		{Title: "Type", Width: 10},
		{Title: "Name", Width: 30},
	}

	m := &listModel{
		tests:          tests,
		executor:       executor,
		state:          listView,
		columns:        columns,
		suiteOpts:      suiteOpts,
		sizeWarning:    components.NewListViewSizeWarning(),
		expandedTraces: make(map[string]bool),
		detailsPanel:   components.NewDetailsPanel(),
		lastCursor:     -1,
	}

	rows, rowMapping := m.buildRows()
	m.rowInfos = rowMapping

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(20),
	)

	s := table.DefaultStyles()
	s.Header = styles.TableHeaderStyle
	s.Cell = styles.TableCellStyle
	s.Selected = styles.TableRowSelectedStyle
	t.SetStyles(s)

	m.table = t

	if _, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run(); err != nil {
		return err
	}

	return nil
}

func (m *listModel) Init() tea.Cmd {
	return nil
}

func (m *listModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
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
				selectedIdx := m.table.Cursor()
				if selectedIdx >= 0 &&
					selectedIdx < len(m.rowInfos) &&
					m.executor != nil {
					info := m.rowInfos[selectedIdx]
					test := m.tests[info.testIndex]
					opts := &InteractiveOpts{
						IsCloudMode:              m.suiteOpts.IsCloudMode,
						OnBeforeEnvironmentStart: m.createSuiteSpanPreparation(),
					}
					executor := newTestExecutorModel([]runner.Test{test}, m.executor, opts)

					logging.SetTestLogger(executor)

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
			case "u":
				m.detailsPanel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
				return m, nil
			case "d":
				m.detailsPanel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
				return m, nil
			case "left", "h":
				selectedIdx := m.table.Cursor()
				if selectedIdx >= 0 && selectedIdx < len(m.rowInfos) {
					info := m.rowInfos[selectedIdx]
					traceID := m.tests[info.testIndex].TraceID
					if m.expandedTraces[traceID] {
						m.expandedTraces[traceID] = false
						m.rebuildTable()
					}
				}
			case "right", "l":
				selectedIdx := m.table.Cursor()
				if selectedIdx >= 0 && selectedIdx < len(m.rowInfos) {
					info := m.rowInfos[selectedIdx]
					test := m.tests[info.testIndex]
					if len(test.Spans) > 0 && !m.expandedTraces[test.TraceID] {
						m.expandedTraces[test.TraceID] = true
						m.rebuildTable()
					}
				}
			}
		case testExecutionView:
			if m.testExecutor != nil && m.testExecutor.state == stateCompleted {
				switch msg.String() {
				case "q", "ctrl+c", "esc", "enter", " ":
					// Clean up and return to list
					m.testExecutor.cleanup()
					logging.SetTestLogger(nil)

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
			cmd := m.detailsPanel.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
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
			m.resizeColumns(msg.Width)
			m.table.SetHeight(msg.Height - 5)
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

	if m.state == listView {
		prevCursor := m.table.Cursor()
		m.table, cmd = m.table.Update(msg)
		cmds = append(cmds, cmd)
		if m.table.Cursor() != prevCursor {
			m.updateDetailsContent()
		}
	}

	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, cmd
}

func (m *listModel) resizeColumns(totalWidth int) {
	if totalWidth <= 0 || len(m.columns) == 0 {
		return
	}

	tableWidth := totalWidth / 2

	// Match padding
	padPerCol := styles.TableCellStyle.GetPaddingLeft() + styles.TableCellStyle.GetPaddingRight()
	contentWidth := max(tableWidth-padPerCol*len(m.columns), 0)

	sum := 0
	for _, c := range m.columns {
		sum += c.Width
	}

	cols := make([]table.Column, len(m.columns))
	copy(cols, m.columns)

	if contentWidth > sum {
		cols[3].Width += contentWidth - sum // grow Name column
	}

	m.table.SetColumns(cols)
	m.table.SetWidth(tableWidth)
}

func (m *listModel) View() string {
	switch m.state {
	case listView:
		if m.sizeWarning.ShouldShow(m.width, m.height) {
			return m.sizeWarning.View(m.width, m.height)
		}

		header := components.Title(m.width, "AVAILABLE TESTS")
		testCount := fmt.Sprintf("%d TESTS", len(m.tests))
		separator := " • "

		usedWidth := lipgloss.Width(testCount) + lipgloss.Width(separator)
		availableWidthForHelp := m.width - usedWidth
		availableWidthForHelp = max(availableWidthForHelp, 20)

		helpText := utils.TruncateWithEllipsis("• ↑/↓/j/k: navigate • ←/→: collapse/expand • u/d: scroll details • enter: run • q: quit", availableWidthForHelp)

		footer := fmt.Sprintf("%s%s%s", testCount, separator, helpText)
		help := components.Footer(m.width, footer)

		tableWidth := m.width / 2
		detailsWidth := m.width - tableWidth

		m.detailsPanel.SetSize(detailsWidth, m.height-5)
		m.detailsPanel.SetXOffset(tableWidth + 2) // panel position + border (1) + padding (1)
		m.detailsPanel.SetYOffset(5)              // header (1) + empty line (1) + title (1) + margin (1) + border (1)

		if m.table.Cursor() != m.lastCursor {
			m.lastCursor = m.table.Cursor()
			m.updateDetailsContent()
		}

		testsSectionTitle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(styles.PrimaryColor)).
			MarginBottom(1).
			Render("Tests")

		leftStyle := lipgloss.NewStyle().Width(tableWidth).MaxWidth(tableWidth)
		rightStyle := lipgloss.NewStyle().Width(detailsWidth).MaxWidth(detailsWidth)

		leftSide := leftStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
			testsSectionTitle,
			m.table.View(),
		))

		mainContent := lipgloss.JoinHorizontal(
			lipgloss.Top,
			leftSide,
			rightStyle.Render(m.detailsPanel.View()),
		)

		return fmt.Sprintf("%s\n\n%s\n%s", header, mainContent, help)

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
	selectedIdx := m.table.Cursor()
	if selectedIdx < 0 || selectedIdx >= len(m.rowInfos) {
		m.detailsPanel.SetContent("No selection")
		m.detailsPanel.SetTitle("Details")
		return
	}

	info := m.rowInfos[selectedIdx]
	test := m.tests[info.testIndex]

	// Set title based on selection type
	if info.isTrace {
		m.detailsPanel.SetTitle("Trace Details")
	} else {
		m.detailsPanel.SetTitle("Span Details")
	}

	// Generate and render content
	lines := m.generateDetailsContent()
	content := utils.RenderMarkdown(strings.Join(lines, "\n"))
	m.detailsPanel.SetContent(content)
	m.detailsPanel.GotoTop()

	_ = test // Used in generateDetailsContent
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
	selectedIdx := m.table.Cursor()
	if selectedIdx < 0 || selectedIdx >= len(m.rowInfos) {
		return []string{"No selection"}
	}

	info := m.rowInfos[selectedIdx]
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
func (m *listModel) buildRows() ([]table.Row, []rowInfo) {
	rows := []table.Row{}
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

		rows = append(rows, table.Row{
			fmt.Sprintf("%d", i+1),
			expandIndicator + test.TraceID,
			displayType,
			displayPath,
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
func (m *listModel) addSpanRows(rows *[]table.Row, rowMapping *[]rowInfo, nodes []*spanTreeNode, testIndex int, prefix string, isRoot bool) {
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

		*rows = append(*rows, table.Row{
			"", // No row number for child spans
			prefix + branchChar + node.span.SpanId[:min(8, len(node.span.SpanId))], // Truncated span ID with tree
			spanType,
			spanName,
		})
		*rowMapping = append(*rowMapping, rowInfo{testIndex: testIndex, spanID: node.span.SpanId, isTrace: false})

		// Recursively add children
		if len(node.children) > 0 {
			m.addSpanRows(rows, rowMapping, node.children, testIndex, childPrefix, false)
		}
	}
}

// rebuildTable rebuilds the table rows while preserving cursor position
func (m *listModel) rebuildTable() {
	cursor := m.table.Cursor()
	rows, rowMapping := m.buildRows()
	m.rowInfos = rowMapping
	m.table.SetRows(rows)

	// Try to keep cursor at a valid position
	if cursor >= len(rows) {
		cursor = len(rows) - 1
	}
	if cursor < 0 {
		cursor = 0
	}
	m.table.SetCursor(cursor)
}
