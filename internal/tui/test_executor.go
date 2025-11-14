package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/Use-Tusk/tusk-drift-cli/internal/logging"
	"github.com/Use-Tusk/tusk-drift-cli/internal/runner"
	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/components"
	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
)

type (
	executionState int
	viewMode       int
)

const (
	stateRunning executionState = iota
	stateCompleted
)

const (
	tableNavigation viewMode = iota
	logNavigation
	logCopyMode
)

type testExecutorModel struct {
	tests    []runner.Test
	executor *runner.Executor
	state    executionState
	viewMode viewMode

	// Progress tracking
	activeTests       map[int]bool
	completedCount    int
	results           []runner.TestResult
	errors            []error
	currentTestTraces map[string]bool
	pendingTests      []int
	nextTestIndex     int

	// Components
	testTable *components.TestTableComponent
	logPanel  *components.LogPanelComponent
	header    *components.TestExecutionHeaderComponent

	// UI dimensions
	width  int
	height int

	// Control flags
	serverStarted  bool
	serviceStarted bool

	sizeWarning *components.TerminalSizeWarning

	copyModeViewport viewport.Model
	copyNotice       bool

	opts *InteractiveOpts
}

type testsLoadedMsg struct {
	tests []runner.Test
}

type testsLoadFailedMsg struct {
	err error
}

type testStartedMsg struct {
	index int
	test  runner.Test
}

type testCompletedMsg struct {
	index   int
	result  runner.TestResult
	err     error
	logPath string
}

type executionCompleteMsg struct{}

type executionFailedMsg struct {
	reason string
}

type hideCopyNoticeMsg struct{}

// TUI log writer to capture slog output
type tuiLogWriter struct {
	model *testExecutorModel
}

type InteractiveOpts struct {
	OnTestCompleted    func(res runner.TestResult, test runner.Test, executor *runner.Executor)
	OnAllCompleted     func(results []runner.TestResult, tests []runner.Test, executor *runner.Executor)
	InitialServiceLogs []string
	IsCloudMode        bool

	// A callback that TUI invokes async to prepare the list of runner.Test items.
	LoadTests func(ctx context.Context) ([]runner.Test, error)
	// If true, TUI waits for LoadTestsToComplete before starting the environment and executing tests.
	StartAfterTestsLoaded bool
	// A hook called after tests are available (at the very beginning of test execution),
	// right before starting the environment.
	OnBeforeEnvironmentStart func(executor *runner.Executor, tests []runner.Test) error
}

func (m *testExecutorModel) LogToCurrentTest(testID, message string) {
	// Always log to the test if testID is provided; fallback to service logs only if empty
	if testID != "" {
		m.addTestLog(testID, message)
	} else {
		m.addServiceLog(message)
	}
}

// formatDriftCloudCTA returns the CTA text for Tusk Drift Cloud as a boxed message
func formatDriftCloudCTA() []string {
	lines := []string{
		"💡 Want root cause analysis?",
		"   Sign up for Tusk Drift Cloud: https://docs.usetusk.ai/api-tests/cloud",
	}
	padding := 2

	if len(lines) == 0 {
		return []string{}
	}

	// Calculate max display width (not byte length) of content
	contentMaxWidth := 0
	for _, line := range lines {
		width := runewidth.StringWidth(line)
		if width > contentMaxWidth {
			contentMaxWidth = width
		}
	}

	boxWidth := contentMaxWidth + 2*padding

	result := []string{""}

	result = append(result, utils.MarkNonWrappable("╭"+strings.Repeat("─", boxWidth)+"╮"))

	for _, line := range lines {
		leftPad := strings.Repeat(" ", padding)
		lineWidth := runewidth.StringWidth(line)
		rightPad := strings.Repeat(" ", boxWidth-lineWidth-padding)
		result = append(result, utils.MarkNonWrappable("│"+leftPad+line+rightPad+"│"))
	}

	result = append(result, utils.MarkNonWrappable("╰"+strings.Repeat("─", boxWidth)+"╯"))

	return result
}

func (m *testExecutorModel) LogToService(message string) {
	m.addServiceLog(message)
}

func (w *tuiLogWriter) Write(p []byte) (n int, err error) {
	line := strings.TrimSpace(string(p))
	if line != "" {
		w.model.routeLog(line)
	}
	return len(p), nil
}

func (m *testExecutorModel) routeLog(line string) {
	// Check if this is a test-specific log
	testID := m.extractTestIDFromLog(line)
	if testID != "" && m.isTestActive(testID) {
		// Route to specific test
		m.addTestLog(testID, line)
	} else {
		// Route to service logs
		m.addServiceLog(line)
	}
}

func (m *testExecutorModel) extractTestIDFromLog(line string) string {
	// Look for patterns that indicate test-specific logs
	if strings.Contains(line, "Finding best match for request") {
		// This is test-specific - we need to determine which test
		// We can use the fact that tests run one at a time per trace
		for testID := range m.currentTestTraces {
			return testID
		}
	}

	// Add other patterns as needed
	// if strings.Contains(line, "other test pattern") { ... }

	return ""
}

func (m *testExecutorModel) isTestActive(testID string) bool {
	return m.currentTestTraces[testID]
}

func RunTestsInteractive(tests []runner.Test, executor *runner.Executor) ([]runner.TestResult, error) {
	m := newTestExecutorModel(tests, executor, nil)

	// Register this model as the global test logger
	logging.SetTestLogger(m)

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		logging.SetTestLogger(nil)
		return nil, err
	}

	logging.SetTestLogger(nil)
	return m.results, nil
}

func RunTestsInteractiveWithOpts(tests []runner.Test, executor *runner.Executor, opts *InteractiveOpts) ([]runner.TestResult, error) {
	m := newTestExecutorModel(tests, executor, opts)

	// Register this model as the global test logger
	logging.SetTestLogger(m)

	// Prepend initial service logs
	if opts != nil && len(opts.InitialServiceLogs) > 0 {
		for _, line := range opts.InitialServiceLogs {
			m.addServiceLog(line)
		}
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		logging.SetTestLogger(nil)
		return nil, err
	}

	logging.SetTestLogger(nil)
	return m.results, nil
}

func newTestExecutorModel(tests []runner.Test, executor *runner.Executor, opts *InteractiveOpts) *testExecutorModel {
	model := &testExecutorModel{
		tests:             tests,
		executor:          executor,
		state:             stateRunning,
		viewMode:          tableNavigation,
		activeTests:       make(map[int]bool),
		currentTestTraces: make(map[string]bool),
		completedCount:    0,
		results:           make([]runner.TestResult, len(tests)),
		errors:            make([]error, len(tests)),
		pendingTests:      make([]int, 0),
		nextTestIndex:     0,
		testTable:         components.NewTestTableComponent(tests),
		logPanel:          components.NewLogPanelComponent(),
		header:            components.NewTestExecutionHeaderComponent(len(tests)),
		width:             120, // Default width
		height:            30,  // Default height
		sizeWarning:       components.NewTestViewSizeWarning(),
		opts:              opts,
	}

	// Setup TUI logging to capture slog output
	model.setupTUILogging()

	return model
}

func (m *testExecutorModel) setupTUILogging() {
	tuiWriter := &tuiLogWriter{model: m}

	// Use debug level if debug logging was enabled
	level := slog.LevelInfo
	if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		level = slog.LevelDebug
	}

	handler := slog.NewTextHandler(tuiWriter, &slog.HandlerOptions{
		Level: level,
	})
	slog.SetDefault(slog.New(handler))
}

func (m *testExecutorModel) loadTestsIfNeeded() tea.Cmd {
	if m.opts == nil || m.opts.LoadTests == nil {
		return nil
	}
	return func() tea.Msg {
		tests, err := m.opts.LoadTests(context.Background())
		if err != nil {
			return testsLoadFailedMsg{err: err}
		}
		return testsLoadedMsg{tests: tests}
	}
}

func (m *testExecutorModel) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, m.header.Update(nil))

	if m.opts == nil || !m.opts.StartAfterTestsLoaded {
		cmds = append(cmds, m.header.SetInitialProgress())
		cmds = append(cmds, m.startExecution())
	}
	// Otherwise, startExecution will be called later (after tests are loaded)

	if cmd := m.loadTestsIfNeeded(); cmd != nil {
		cmds = append(cmds, cmd)
	}

	return tea.Batch(cmds...)
}

func (m *testExecutorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		oldWidth := m.width
		oldHeight := m.height
		m.width = msg.Width
		m.height = msg.Height

		// Reset dismissed flag if window becomes large enough, then small again
		wasLargeEnough := !m.sizeWarning.IsTooSmall(oldWidth, oldHeight)
		isNowTooSmall := m.sizeWarning.IsTooSmall(m.width, m.height)

		if wasLargeEnough && isNowTooSmall {
			m.sizeWarning.Reset()
		}

		return m, nil

	case tea.KeyMsg:
		if m.sizeWarning.ShouldShow(m.width, m.height) {
			switch msg.String() {
			case "enter", "d", "D":
				m.sizeWarning.Dismiss()
				return m, nil
			case "q", "ctrl+c", "esc":
				m.cleanup()
				return m, tea.Quit
			}
			return m, nil
		}

		switch m.viewMode {
		case tableNavigation:
			return m.handleTableNavigation(msg)
		case logNavigation:
			return m.handleLogNavigation(msg)
		case logCopyMode:
			return m.handleLogCopyMode(msg)
		}

	case testsLoadedMsg:
		// Inject tests into model and start execution
		m.tests = msg.tests
		m.results = make([]runner.TestResult, len(m.tests))
		m.errors = make([]error, len(m.tests))
		m.testTable = components.NewTestTableComponent(m.tests)
		m.header = components.NewTestExecutionHeaderComponent(len(m.tests))
		if m.opts != nil && m.opts.StartAfterTestsLoaded {
			if len(m.tests) == 0 {
				return m, tea.Batch(m.updateStats(), func() tea.Msg { return executionFailedMsg{reason: "No tests to run"} })
			}
			return m, tea.Batch(m.updateStats(), m.header.SetInitialProgress(), m.startExecution())

		}
		return m, m.updateStats()

	case testsLoadFailedMsg:
		logging.LogToService(fmt.Sprintf("❌ Failed to load tests: %v", msg.err))
		return m, func() tea.Msg { return executionFailedMsg{reason: fmt.Sprintf("Failed to load tests: %v", msg.err)} }

	case testStartedMsg:
		m.activeTests[msg.index] = true
		m.currentTestTraces[msg.test.TraceID] = true
		m.addTestLog(msg.test.TraceID, fmt.Sprintf("🧪 Started: %s %s", msg.test.Method, msg.test.Path))
		cmds = append(cmds, m.updateStats())
		cmds = append(cmds, m.executeTest(msg.index))

	case testCompletedMsg:
		delete(m.activeTests, msg.index)
		delete(m.currentTestTraces, m.tests[msg.index].TraceID)
		m.results[msg.index] = msg.result
		m.errors[msg.index] = msg.err
		m.completedCount++

		m.testTable.UpdateTestResult(msg.index, msg.result, msg.err)

		test := m.tests[msg.index]
		switch {
		case msg.result.CrashedServer:
			m.addTestLog(test.TraceID, fmt.Sprintf("❌ %s %s - SERVER CRASHED (%dms)", test.Method, test.Path, msg.result.Duration))
			if msg.err != nil {
				m.addTestLog(test.TraceID, fmt.Sprintf("  Error: %v", msg.err))
			}
		case msg.err != nil:
			m.addTestLog(test.TraceID, fmt.Sprintf("❌ %s %s - ERROR: %v", test.Method, test.Path, msg.err))
		case msg.result.Passed:
			m.addTestLog(test.TraceID, fmt.Sprintf("✅ %s %s - NO DEVIATION (%dms)", test.Method, test.Path, msg.result.Duration))
		default:
			m.addTestLog(test.TraceID, fmt.Sprintf("🟠 %s %s - DEVIATION DETECTED (%dms)", test.Method, test.Path, msg.result.Duration))

			// Check for mock-not-found events first
			if m.executor != nil && m.executor.GetServer() != nil && m.executor.GetServer().HasMockNotFoundEvents(test.TraceID) {
				mockNotFoundEvents := m.executor.GetServer().GetMockNotFoundEvents(test.TraceID)
				for _, ev := range mockNotFoundEvents {
					m.addTestLog(test.TraceID, fmt.Sprintf("  🔴 Mock not found: %s %s", ev.PackageName, ev.Operation))
					if ev.SpanName != "" {
						m.addTestLog(test.TraceID, fmt.Sprintf("    Request: %s", ev.SpanName))
					}
					if ev.StackTrace != "" {
						m.addTestLog(test.TraceID, fmt.Sprintf("    Stack trace:\n%s", ev.StackTrace))
					}
				}
			} else if len(msg.result.Deviations) > 0 {
				for _, dev := range msg.result.Deviations {
					m.addTestLog(test.TraceID, fmt.Sprintf("  Deviation: %s", dev.Description))

					// For JSON response body mismatches, use git-style diff formatting
					if dev.Field == "response.body" {
						m.addTestLog(test.TraceID, utils.FormatJSONDiff(dev.Expected, dev.Actual))
					} else {
						m.addTestLog(test.TraceID, fmt.Sprintf("    Expected: %v", dev.Expected))
						m.addTestLog(test.TraceID, fmt.Sprintf("    Actual: %v", dev.Actual))
					}
				}
			}

			if m.opts == nil || !m.opts.IsCloudMode {
				for _, line := range formatDriftCloudCTA() {
					m.addTestLog(test.TraceID, line)
				}
			}
		}

		// Per-test cloud upload (non-blocking)
		if m.opts != nil && m.opts.OnTestCompleted != nil {
			res := msg.result
			go m.opts.OnTestCompleted(res, test, m.executor)
		}

		cmds = append(cmds, m.updateStats())

		if m.nextTestIndex < len(m.tests) {
			nextIndex := m.nextTestIndex
			m.nextTestIndex++
			cmds = append(cmds, func() tea.Msg {
				return testStartedMsg{index: nextIndex, test: m.tests[nextIndex]}
			})
		}

		if m.completedCount >= len(m.tests) {
			if m.opts != nil && m.opts.OnAllCompleted != nil {
				m.opts.OnAllCompleted(m.results, m.tests, m.executor)
			}
			cmds = append(cmds, m.completeExecution())
		}

		return m, tea.Batch(cmds...)

	case executionCompleteMsg:
		m.state = stateCompleted
		m.header.SetCompleted()
		m.addServiceLog("\n" + strings.Repeat("=", 60))
		m.addServiceLog("🏁 All tests completed!")

		// All-tests completed upload (non-blocking)
		if m.opts != nil && m.opts.OnAllCompleted != nil {
			results := make([]runner.TestResult, len(m.results))
			copy(results, m.results)
			tests := make([]runner.Test, len(m.tests))
			copy(tests, m.tests)
			go m.opts.OnAllCompleted(results, tests, m.executor)
		}

		if m.executor.ResultsFile != "" {
			if path, err := m.executor.WriteRunResultsToFile(m.tests, m.results); err != nil {
				m.addServiceLog(fmt.Sprintf("❌ Failed to write results to file: %v", err))
			} else {
				m.addServiceLog(fmt.Sprintf("📝 Results written to %s", path))
			}
		}

		m.cleanup()

	case executionFailedMsg:
		m.state = stateCompleted
		m.header.SetCompleted()
		m.addServiceLog("\n" + strings.Repeat("=", 60))
		m.addServiceLog("❌ Execution failed - no tests were run")
		m.cleanup()

	case hideCopyNoticeMsg:
		m.copyNotice = false
	}

	// Update components
	if cmd := m.testTable.Update(msg); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if cmd := m.logPanel.Update(msg); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if cmd := m.header.Update(msg); cmd != nil {
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *testExecutorModel) handleTableNavigation(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "down":
		cmd := m.testTable.Update(msg)

		// Update log panel based on selection
		if m.testTable.IsServiceLogsSelected() {
			m.logPanel.SetCurrentTest("") // Show service logs
		} else if selectedTest := m.testTable.GetSelectedTest(); selectedTest != nil {
			m.logPanel.SetCurrentTest(selectedTest.TraceID)
		}

		return m, cmd

	case "right":
		m.viewMode = logNavigation
		m.testTable.SetFocused(false)
		m.logPanel.SetFocused(true)
		return m, nil

	case "g":
		m.testTable.GotoTop()

	case "G":
		m.testTable.GotoBottom()

	case "q", "ctrl+c", "esc":
		m.cleanup()
		return m, tea.Quit
	}

	return m, nil
}

func (m *testExecutorModel) handleLogNavigation(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "down", "pgup", "pgdown", "g", "G":
		return m, m.logPanel.Update(msg)

	case "y":
		return m, m.copyLogsToClipboard()

	case "c":
		m.viewMode = logCopyMode
		m.copyModeViewport = viewport.Model{}

		return m, nil

	case "left", "esc":
		m.viewMode = tableNavigation
		m.testTable.SetFocused(true)
		m.logPanel.SetFocused(false)
		return m, nil

	case "q", "ctrl+c":
		m.cleanup()
		return m, tea.Quit
	}

	return m, nil
}

func (m *testExecutorModel) handleLogCopyMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "down", "pgup", "pgdown":
		var cmd tea.Cmd
		m.copyModeViewport, cmd = m.copyModeViewport.Update(msg)
		return m, cmd

	case "g":
		m.copyModeViewport.GotoTop()

	case "G":
		m.copyModeViewport.GotoBottom()

	case "y":
		return m, m.copyLogsToClipboard()

	case "esc", "c":
		m.viewMode = logNavigation
		return m, nil

	case "q", "ctrl+c":
		m.cleanup()
		return m, tea.Quit
	}

	return m, nil
}

func (m *testExecutorModel) getFooterText() string {
	switch m.viewMode {
	case tableNavigation:
		return "↑/↓: navigate • g: go to top • G: go to bottom • →: view logs • q: quit"
	case logNavigation:
		return "↑/↓: scroll logs • g: go to top • G: go to bottom • y: copy • c: full screen mode • ←/Esc: back to table • q: quit"
	case logCopyMode:
		return "FULL SCREEN MODE • g: go to top • G: go to bottom • y: copy • c/Esc: exit full screen • q: quit"
	default:
		return ""
	}
}

func (m *testExecutorModel) View() string {
	if m.sizeWarning.ShouldShow(m.width, m.height) {
		return m.sizeWarning.View(m.width, m.height)
	}

	if m.viewMode == logCopyMode {
		return m.fullScreenLogView()
	}

	if m.width < 100 {
		return m.verticalLayout()
	}
	return m.horizontalLayout()
}

func (m *testExecutorModel) displayCopyText() string {
	return styles.SuccessStyle.Render("Copied ✓")
}

func (m *testExecutorModel) fullScreenLogView() string {
	footerHeight := 1
	contentHeight := m.height - footerHeight

	// Initialize copy mode viewport if needed
	if m.copyModeViewport.Width == 0 {
		m.copyModeViewport = viewport.New(m.width, contentHeight)
		// Remove all borders and styling for true full screen
		m.copyModeViewport.Style = lipgloss.NewStyle()

		// Get raw content and wrap it to terminal width
		rawContent := m.logPanel.GetRawLogs()
		wrappedContent := utils.WrapText(rawContent, m.width)
		m.copyModeViewport.SetContent(wrappedContent)
		m.copyModeViewport.GotoBottom() // Start at bottom like normal mode
	}

	m.copyModeViewport.Width = m.width
	m.copyModeViewport.Height = contentHeight

	left := components.Footer(m.width, m.getFooterText())
	footer := left
	if m.copyNotice {
		right := m.displayCopyText()
		space := max(m.width-lipgloss.Width(left)-lipgloss.Width(right), 1)
		footer = left + strings.Repeat(" ", space) + right
	}

	return m.copyModeViewport.View() + "\n" + footer
}

func (m *testExecutorModel) horizontalLayout() string {
	headerHeight := 4
	footerHeight := 1
	contentHeight := m.height - headerHeight - footerHeight

	leftWidth := m.width / 2
	rightWidth := m.width - leftWidth - 1 // Account for separator

	header := m.header.View(m.width)

	// Truncate help text if necessary
	helpText := utils.TruncateWithEllipsis(m.getFooterText(), m.width)
	left := components.Footer(m.width, helpText)
	footer := left
	if m.copyNotice {
		right := m.displayCopyText()
		availableWidth := m.width - lipgloss.Width(right) - 1
		helpText = utils.TruncateWithEllipsis(m.getFooterText(), availableWidth)
		left = components.Footer(availableWidth, helpText)
		space := max(m.width-lipgloss.Width(left)-lipgloss.Width(right), 1)
		footer = left + strings.Repeat(" ", space) + right
	}

	tableView := m.testTable.View(leftWidth, contentHeight)
	logView := m.logPanel.View(rightWidth, contentHeight)

	separator := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render(" ")

	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		tableView,
		separator,
		logView,
	)

	// Calculate spacing to push footer to the very last line
	totalUsedLines := headerHeight + contentHeight + footerHeight
	remainingLines := m.height - totalUsedLines

	var spacing string
	if remainingLines > 0 {
		spacing = strings.Repeat("\n", remainingLines)
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, content, spacing, footer)
}

func (m *testExecutorModel) verticalLayout() string {
	headerHeight := 4
	footerHeight := 1
	contentHeight := m.height - headerHeight - footerHeight

	header := m.header.View(m.width)

	helpText := utils.TruncateWithEllipsis(m.getFooterText(), m.width)
	left := components.Footer(m.width, helpText)
	footer := left
	if m.copyNotice {
		right := m.displayCopyText()
		availableWidth := m.width - lipgloss.Width(right) - 1
		helpText = utils.TruncateWithEllipsis(m.getFooterText(), availableWidth)
		left = components.Footer(availableWidth, helpText)
		space := max(m.width-lipgloss.Width(left)-lipgloss.Width(right), 1)
		footer = left + strings.Repeat(" ", space) + right
	}

	infoMsg := "Vertical layout enabled for narrow terminal. Seeing weird formatting? Make this window wider for horizontal layout."
	wrappedInfo := utils.WrapText(infoMsg, m.width)
	infoStyle := styles.DimStyle.Italic(true)
	styledInfo := infoStyle.Render(wrappedInfo)

	// Split content height between table and log
	tableHeight := contentHeight / 2
	logHeight := contentHeight - tableHeight

	tableView := m.testTable.View(m.width, tableHeight)
	logView := m.logPanel.View(m.width, logHeight)

	// Calculate actual heights of rendered components
	actualHeaderHeight := lipgloss.Height(header)
	actualTableHeight := lipgloss.Height(tableView)
	actualLogHeight := lipgloss.Height(logView)
	actualInfoHeight := lipgloss.Height(styledInfo)
	actualFooterHeight := lipgloss.Height(footer)

	// Calculate spacing needed to push footer section to bottom
	totalUsedLines := actualHeaderHeight + actualTableHeight + actualLogHeight + actualInfoHeight + actualFooterHeight
	remainingLines := m.height - totalUsedLines

	var spacing string
	if remainingLines > 0 {
		spacing = strings.Repeat("\n", remainingLines)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		tableView,
		"", // Empty line for visual separation
		logView,
		spacing,
		styledInfo,
		footer,
	)
}

func (m *testExecutorModel) addServiceLog(line string) {
	m.logPanel.AddServiceLog(line)
}

func (m *testExecutorModel) addTestLog(testID, line string) {
	m.logPanel.AddTestLog(testID, line)
}

func (m *testExecutorModel) updateStats() tea.Cmd {
	passed := 0
	failed := 0
	for i := 0; i < m.completedCount; i++ {
		switch {
		case m.results[i].CrashedServer:
			failed++ // Count crashed servers as failures
		case m.errors[i] == nil && m.results[i].Passed:
			passed++
		default:
			failed++
		}
	}
	return m.header.UpdateStats(m.completedCount, passed, failed, len(m.activeTests))
}

func (m *testExecutorModel) startExecution() tea.Cmd {
	return func() tea.Msg {
		// If no tests, skip environment and finish immediately
		if len(m.tests) == 0 {
			m.addServiceLog("No tests to run. Skipping environment start.")
			return executionFailedMsg{reason: "No tests to run"}
		}

		// Pre-start hook from caller (e.g., suite span prep/logging)
		if m.opts != nil && m.opts.OnBeforeEnvironmentStart != nil {
			if err := m.opts.OnBeforeEnvironmentStart(m.executor, m.tests); err != nil {
				m.addServiceLog(fmt.Sprintf("❌ Pre-start setup failed: %v", err))
				return executionFailedMsg{reason: fmt.Sprintf("Pre-start setup failed: %v", err)}
			}
		}

		m.addServiceLog("Starting environment...")
		if err := m.executor.StartEnvironment(); err != nil {
			m.addServiceLog(fmt.Sprintf("❌ Failed to start environment: %v", err))

			helpMsg := m.executor.GetStartupFailureHelpMessage()
			for _, line := range strings.Split(strings.TrimSpace(helpMsg), "\n") {
				m.addServiceLog(line)
			}

			return executionFailedMsg{reason: fmt.Sprintf("Failed to start environment: %v", err)}
		}
		m.serverStarted = true
		m.serviceStarted = true
		m.addServiceLog("✅ Environment ready")

		return m.startConcurrentTests()()
	}
}

func (m *testExecutorModel) startConcurrentTests() tea.Cmd {
	return func() tea.Msg {
		if len(m.tests) == 0 {
			return executionCompleteMsg{}
		}
		concurrency := m.executor.GetConcurrency()
		m.addServiceLog(fmt.Sprintf("🚀 Starting %d tests with max %d concurrency...\n", len(m.tests), concurrency))

		var cmds []tea.Cmd
		for i := 0; i < concurrency && i < len(m.tests); i++ {
			index := i
			cmds = append(cmds, func() tea.Msg {
				return testStartedMsg{index: index, test: m.tests[index]}
			})
		}

		m.nextTestIndex = min(concurrency, len(m.tests))

		return tea.Batch(cmds...)()
	}
}

func (m *testExecutorModel) executeTest(index int) tea.Cmd {
	return func() tea.Msg {
		test := m.tests[index]

		logPath := m.executor.GetServiceLogPath()

		// Suppress callbacks during execution
		m.executor.SetSuppressCallbacks(true)
		result, err := m.executor.RunSingleTest(test)
		m.executor.SetSuppressCallbacks(false)

		// Check if this test crashed the server
		if err != nil && !m.executor.CheckServerHealth() {
			slog.Warn("Test crashed the server in interactive mode", "testID", test.TraceID, "error", err)
			m.addServiceLog(fmt.Sprintf("⚠️  Test %s crashed the server", test.TraceID))

			result.CrashedServer = true

			// Attempt to restart the server for next test
			if index < len(m.tests)-1 {
				m.addServiceLog("🔄 Restarting server...")
				if restartErr := m.executor.RestartServerWithRetry(0); restartErr != nil {
					m.addServiceLog(fmt.Sprintf("❌ Failed to restart server: %v", restartErr))
				} else {
					m.addServiceLog("✅ Server restarted successfully")
				}
			}
		} else {
			slog.Debug("Test completed", "testID", test.TraceID, "result", result)
		}

		// Manually invoke callback with the updated result
		if m.executor.OnTestCompleted != nil {
			m.executor.OnTestCompleted(result, test)
		}

		return testCompletedMsg{
			index:   index,
			result:  result,
			err:     err,
			logPath: logPath,
		}
	}
}

func (m *testExecutorModel) completeExecution() tea.Cmd {
	return func() tea.Msg {
		time.Sleep(500 * time.Millisecond) // Small delay for visual feedback
		return executionCompleteMsg{}
	}
}

func (m *testExecutorModel) hasAnyDeviations() bool {
	for i := 0; i < len(m.results); i++ {
		if m.errors[i] == nil && !m.results[i].Passed {
			return true
		}
	}
	return false
}

func (m *testExecutorModel) cleanup() {
	if m.serviceStarted || m.serverStarted {
		m.addServiceLog("Stopping environment...")
		if err := m.executor.StopEnvironment(); err != nil {
			m.addServiceLog(fmt.Sprintf("Warning: Failed to stop environment: %v", err))
		}
		m.serviceStarted = false
		m.serverStarted = false

		if (m.opts == nil || !m.opts.IsCloudMode) && m.hasAnyDeviations() {
			for _, line := range formatDriftCloudCTA() {
				m.addServiceLog(line)
			}
		}
	}
}

func (m *testExecutorModel) copyLogsToClipboard() tea.Cmd {
	text := m.logPanel.GetRawLogs()
	_ = utils.CopyToClipboard(text)
	m.copyNotice = true
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg { return hideCopyNoticeMsg{} })
}
