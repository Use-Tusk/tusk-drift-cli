package tui

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/Use-Tusk/tusk-drift-cli/internal/log"
	"github.com/Use-Tusk/tusk-drift-cli/internal/runner"
	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/components"
	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
)

type executionState int

const (
	stateRunning executionState = iota
	stateCompleted
)

type testExecutorModel struct {
	tests    []runner.Test
	executor *runner.Executor
	state    executionState

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

	// Environment grouping
	environmentGroups      []*runner.EnvironmentGroup
	currentGroupIndex      int
	groupCleanup           func()
	totalTestsAcrossEnvs   int
	testToEnvIndex         map[int]int // Maps global test index to environment group index
	currentEnvTestIndices  []int       // Global indices of tests in current environment
	currentEnvTestsStarted int         // Number of tests started in current environment

	// Retry tracking for crashed tests
	testsToRetry []int // Global indices of tests to retry after crash
	inRetryPhase bool  // Whether we're currently in retry phase

	sizeWarning *components.TerminalSizeWarning

	opts *InteractiveOpts

	// Program reference for sending refresh messages from goroutines
	program *tea.Program
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

type environmentGroupCompleteMsg struct{}

// refreshTickMsg triggers periodic UI refresh for live log updates
type refreshTickMsg struct{}

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
	log.SetTUILogger(m)

	// In CI mode, don't use alt screen or mouse (they require a real TTY)
	var p *tea.Program
	if utils.TUICIMode() {
		p = tea.NewProgram(m)
	} else {
		p = tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	}
	m.program = p
	if _, err := p.Run(); err != nil {
		log.SetTUILogger(nil)
		return nil, err
	}

	log.SetTUILogger(nil)
	return m.results, nil
}

func RunTestsInteractiveWithOpts(tests []runner.Test, executor *runner.Executor, opts *InteractiveOpts) ([]runner.TestResult, error) {
	m := newTestExecutorModel(tests, executor, opts)

	// Register this model as the global test logger
	log.SetTUILogger(m)

	// Prepend initial service logs
	if opts != nil && len(opts.InitialServiceLogs) > 0 {
		for _, line := range opts.InitialServiceLogs {
			m.addServiceLog(line)
		}
	}

	// In CI mode, don't use alt screen or mouse (they require a real TTY)
	var p *tea.Program
	if utils.TUICIMode() {
		p = tea.NewProgram(m)
	} else {
		p = tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	}
	m.program = p
	if _, err := p.Run(); err != nil {
		log.SetTUILogger(nil)
		return nil, err
	}

	log.SetTUILogger(nil)
	return m.results, nil
}

func newTestExecutorModel(tests []runner.Test, executor *runner.Executor, opts *InteractiveOpts) *testExecutorModel {
	model := &testExecutorModel{
		tests:             tests,
		executor:          executor,
		state:             stateRunning,
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
	case refreshTickMsg:
		// Just triggers a re-render, no action needed
		return m, nil

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

		return m.handleTableNavigation(msg)

	case tea.MouseMsg:
		leftWidth := m.width / 2
		headerHeight := 4 // header takes 4 lines

		if msg.X < leftWidth {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				// Scroll viewport only, clamp cursor to visible bounds
				m.testTable.ScrollUp(3)
				// Update log panel based on selection
				if m.testTable.IsServiceLogsSelected() {
					m.logPanel.SetCurrentTest("")
				} else if selectedTest := m.testTable.GetSelectedTest(); selectedTest != nil {
					m.logPanel.SetCurrentTest(selectedTest.TraceID)
				}
				return m, nil
			case tea.MouseButtonWheelDown:
				// Scroll viewport only, clamp cursor to visible bounds
				m.testTable.ScrollDown(3)
				// Update log panel based on selection
				if m.testTable.IsServiceLogsSelected() {
					m.logPanel.SetCurrentTest("")
				} else if selectedTest := m.testTable.GetSelectedTest(); selectedTest != nil {
					m.logPanel.SetCurrentTest(selectedTest.TraceID)
				}
				return m, nil
			}
		} else {
			// - X: leftWidth + border(1) + padding(1) = leftWidth + 2
			// - Y: headerHeight + title(1) + empty line(1) = headerHeight + 2
			m.logPanel.SetOffset(leftWidth+2, headerHeight+2)
			if cmd := m.logPanel.Update(msg); cmd != nil {
				return m, cmd
			}
			return m, nil
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
		log.ServiceLog(fmt.Sprintf("❌ Failed to load tests: %v", msg.err))
		return m, func() tea.Msg { return executionFailedMsg{reason: fmt.Sprintf("Failed to load tests: %v", msg.err)} }

	case testStartedMsg:
		m.activeTests[msg.index] = true
		m.currentTestTraces[msg.test.TraceID] = true
		if m.inRetryPhase {
			m.addTestLog(msg.test.TraceID, fmt.Sprintf("🔄 Retrying: %s %s", msg.test.Method, msg.test.Path))
		} else {
			m.addTestLog(msg.test.TraceID, fmt.Sprintf("🧪 Started: %s %s", msg.test.Method, msg.test.Path))
		}
		cmds = append(cmds, m.updateStats())
		cmds = append(cmds, m.executeTest(msg.index))

	case testCompletedMsg:
		delete(m.activeTests, msg.index)
		delete(m.currentTestTraces, m.tests[msg.index].TraceID)
		m.results[msg.index] = msg.result
		m.errors[msg.index] = msg.err
		m.completedCount++

		// Check if this test is pending retry - if so, don't update the UI yet
		isPendingRetry := slices.Contains(m.testsToRetry, msg.index)

		// Only update the test table if this is the final result (not pending retry)
		if !isPendingRetry {
			m.testTable.UpdateTestResult(msg.index, msg.result, msg.err)
		}

		test := m.tests[msg.index]
		switch {
		case msg.result.CrashedServer:
			m.addTestLog(test.TraceID, fmt.Sprintf("❌ %s %s - SERVER CRASHED (%dms)", test.Method, test.Path, msg.result.Duration))
			if msg.err != nil {
				m.addTestLog(test.TraceID, fmt.Sprintf("  Error: %v", msg.err))
			}
		case isPendingRetry:
			// Don't log final status for tests pending retry - they'll be retried
			m.addTestLog(test.TraceID, fmt.Sprintf("⏳ %s %s - will retry (%dms)", test.Method, test.Path, msg.result.Duration))
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

		// Invoke callbacks only for final results (not tests pending retry).
		// There are two separate callbacks:
		// - executor.OnTestCompleted: Set in cmd/run.go, handles cloud upload via UploadSingleTestResult
		//   and trace span cleanup. This is the primary upload mechanism in CI/cloud mode.
		// - opts.OnTestCompleted: TUI-specific callback for additional per-test processing.
		if !isPendingRetry {
			if m.executor.OnTestCompleted != nil {
				m.executor.OnTestCompleted(msg.result, test)
			}
			if m.opts != nil && m.opts.OnTestCompleted != nil {
				res := msg.result
				go m.opts.OnTestCompleted(res, test, m.executor)
			}
		}

		cmds = append(cmds, m.updateStats())

		// When using environment groups, check if there are more tests in the CURRENT environment
		if len(m.environmentGroups) > 0 && len(m.currentEnvTestIndices) > 0 {
			// Check if we need to start the next test in current environment
			if m.currentEnvTestsStarted < len(m.currentEnvTestIndices) {
				// In retry phase, run tests sequentially (wait for current test to complete)
				// In normal phase, run tests concurrently
				if !m.inRetryPhase || len(m.activeTests) == 0 {
					nextGlobalIndex := m.currentEnvTestIndices[m.currentEnvTestsStarted]
					m.currentEnvTestsStarted++
					cmds = append(cmds, func() tea.Msg {
						return testStartedMsg{index: nextGlobalIndex, test: m.tests[nextGlobalIndex]}
					})
				}
			}

			// Count how many tests from current environment have completed
			completedInCurrentEnv := 0
			for _, globalIdx := range m.currentEnvTestIndices {
				if m.results[globalIdx].TestID != "" || m.errors[globalIdx] != nil {
					completedInCurrentEnv++
				}
			}

			// Check if current environment is complete
			if completedInCurrentEnv >= len(m.currentEnvTestIndices) {
				switch {
				case !m.inRetryPhase && len(m.testsToRetry) > 0:
					// Need retry phase first (only if not already in retry phase)
					cmds = append(cmds, m.startRetryPhase())
				case m.currentGroupIndex < len(m.environmentGroups):
					// More groups to process - trigger environment group completion
					cmds = append(cmds, func() tea.Msg {
						return environmentGroupCompleteMsg{}
					})
				default:
					cmds = append(cmds, m.completeExecution())
				}
			}
		} else {
			// Legacy path: no environment groups
			if m.nextTestIndex < len(m.tests) {
				nextIndex := m.nextTestIndex
				m.nextTestIndex++
				cmds = append(cmds, func() tea.Msg {
					return testStartedMsg{index: nextIndex, test: m.tests[nextIndex]}
				})
			}

			if m.completedCount >= len(m.tests) {
				cmds = append(cmds, m.completeExecution())
			}
		}

		return m, tea.Batch(cmds...)

	case environmentGroupCompleteMsg:
		// Reset retry state for next environment
		m.inRetryPhase = false
		m.testsToRetry = nil

		// Stop current environment
		m.addServiceLog("Stopping environment...")
		if m.serviceStarted {
			if err := m.executor.StopEnvironment(); err != nil {
				m.addServiceLog(fmt.Sprintf("⚠️  Warning: Failed to stop environment: %v", err))
			}
			m.serviceStarted = false
			m.serverStarted = false
		}

		// Cleanup environment variables
		if m.groupCleanup != nil {
			m.groupCleanup()
			m.groupCleanup = nil
		}

		// Start next environment group
		cmds = append(cmds, m.startNextEnvironmentGroup())

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
			go m.opts.OnAllCompleted(results, m.tests, m.executor)
		}

		if m.executor.ResultsFile != "" {
			if path, err := m.executor.WriteRunResultsToFile(m.tests, m.results); err != nil {
				m.addServiceLog(fmt.Sprintf("❌ Failed to write results to file: %v", err))
			} else {
				m.addServiceLog(fmt.Sprintf("📝 Results written to %s", path))
			}
		}

		m.cleanup()

		// Auto-exit in CI/forced TUI mode (no user to press 'q')
		if utils.TUICIMode() {
			return m, tea.Quit
		}

	case executionFailedMsg:
		m.state = stateCompleted
		m.header.SetCompleted()
		m.addServiceLog("\n" + strings.Repeat("=", 60))
		m.addServiceLog("❌ Execution failed - no tests were run")
		m.cleanup()

		// Auto-exit in CI/forced TUI mode (no user to press 'q')
		if utils.TUICIMode() {
			return m, tea.Quit
		}
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
	case "up", "k":
		// Move selection up (updates log panel)
		m.testTable.SelectUp(1)
		m.updateLogPanelFromSelection()
		return m, nil

	case "down", "j":
		// Move selection down (updates log panel)
		m.testTable.SelectDown(1)
		m.updateLogPanelFromSelection()
		return m, nil

	case "u":
		// Scroll viewport up (no selection change, clamp cursor to visible)
		m.testTable.HalfPageUp()
		m.updateLogPanelFromSelection()
		return m, nil

	case "d":
		// Scroll viewport down (no selection change, clamp cursor to visible)
		m.testTable.HalfPageDown()
		m.updateLogPanelFromSelection()
		return m, nil

	case "J":
		// Scroll right side (log panel) down by 1
		m.logPanel.ScrollDown(1)
		return m, nil

	case "K":
		// Scroll right side (log panel) up by 1
		m.logPanel.ScrollUp(1)
		return m, nil

	case "U":
		// Page up on right side (log panel)
		m.logPanel.HalfPageUp()
		return m, nil

	case "D":
		// Page down on right side (log panel)
		m.logPanel.HalfPageDown()
		return m, nil

	case "g":
		m.testTable.GotoTop()
		m.updateLogPanelFromSelection()
		return m, nil

	case "G":
		m.testTable.GotoBottom()
		m.updateLogPanelFromSelection()
		return m, nil

	case "q", "ctrl+c", "esc":
		m.cleanup()
		return m, tea.Quit
	}

	return m, nil
}

// updateLogPanelFromSelection updates the log panel based on current table selection
func (m *testExecutorModel) updateLogPanelFromSelection() {
	if m.testTable.IsServiceLogsSelected() {
		m.logPanel.SetCurrentTest("")
	} else if selectedTest := m.testTable.GetSelectedTest(); selectedTest != nil {
		m.logPanel.SetCurrentTest(selectedTest.TraceID)
	}
}

func (m *testExecutorModel) getFooterText() string {
	testCount := fmt.Sprintf("%d TESTS ", len(m.tests))
	return testCount + "• j/k: select • u/d: scroll • g/G: top/bottom • J/K/U/D: scroll logs • q: quit"
}

func (m *testExecutorModel) View() string {
	if m.sizeWarning.ShouldShow(m.width, m.height) {
		return m.sizeWarning.View(m.width, m.height)
	}

	if m.width < 100 {
		return m.verticalLayout()
	}
	return m.horizontalLayout()
}

func (m *testExecutorModel) horizontalLayout() string {
	// header (4) + footer (1) = 5
	contentHeight := m.height - 5

	leftWidth := m.width / 2
	rightWidth := m.width - leftWidth

	header := m.header.View(m.width)

	// Build footer with test count prefix like list view
	footer := components.Footer(m.width, utils.TruncateWithEllipsis(m.getFooterText(), m.width))

	// Render table with scrollbar (-1 for scrollbar width)
	tableView := m.testTable.View(leftWidth-1, contentHeight)
	tableWithScrollbar := lipgloss.JoinHorizontal(
		lipgloss.Top,
		tableView,
		m.renderTableScrollbar(contentHeight),
	)

	logView := m.logPanel.View(rightWidth, contentHeight)

	mainContent := lipgloss.JoinHorizontal(
		lipgloss.Top,
		tableWithScrollbar,
		logView,
	)

	return lipgloss.JoinVertical(lipgloss.Left, header, mainContent, footer)
}

// renderTableScrollbar renders a vertical scrollbar for the test table
func (m *testExecutorModel) renderTableScrollbar(contentHeight int) string {
	totalRows := m.testTable.TotalRows()
	scrollOffset := m.testTable.ViewportYOffset()

	return components.RenderScrollbar(contentHeight, totalRows, scrollOffset)
}

func (m *testExecutorModel) verticalLayout() string {
	// header (4) + info (2) + footer (1) = 7
	contentHeight := m.height - 7

	header := m.header.View(m.width)

	footer := components.Footer(m.width, utils.TruncateWithEllipsis(m.getFooterText(), m.width))

	infoMsg := "Vertical layout enabled for narrow terminal. Seeing weird formatting? Make this window wider for horizontal layout."
	wrappedInfo := utils.WrapText(infoMsg, m.width)
	infoStyle := styles.DimStyle.Italic(true)
	styledInfo := infoStyle.Render(wrappedInfo)

	// Split content height between table and log
	tableHeight := contentHeight / 2
	logHeight := contentHeight - tableHeight

	tableView := m.testTable.View(m.width, tableHeight)
	logView := m.logPanel.View(m.width, logHeight)

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		tableView,
		logView,
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
	for i := 0; i < len(m.results); i++ {
		// Skip tests that haven't completed yet
		if m.results[i].TestID == "" && m.errors[i] == nil {
			continue
		}

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

		// Get suite spans from executor (which includes pre-app-start spans)
		suiteSpans := m.executor.GetSuiteSpans()

		// Group tests by environment
		groupResult, err := runner.GroupTestsByEnvironment(m.tests, suiteSpans)
		if err != nil {
			m.addServiceLog(fmt.Sprintf("⚠️  Warning: Failed to group by environment: %v", err))
			// Fall through to single-environment mode
			groupResult = &runner.EnvironmentExtractionResult{
				Groups: []*runner.EnvironmentGroup{
					{Name: "default", Tests: m.tests, EnvVars: make(map[string]string)},
				},
			}
		}

		// Log any warnings
		for _, warn := range groupResult.Warnings {
			m.addServiceLog(fmt.Sprintf("⚠️  %s", warn))
		}

		// Store groups for sequential processing
		m.environmentGroups = groupResult.Groups
		m.currentGroupIndex = 0
		m.totalTestsAcrossEnvs = len(m.tests)

		// Build mapping from global test index to environment group index
		m.testToEnvIndex = make(map[int]int)
		for envIdx, group := range groupResult.Groups {
			for _, groupTest := range group.Tests {
				// Find this test in the global m.tests array
				for globalIdx, test := range m.tests {
					if test.TraceID == groupTest.TraceID {
						m.testToEnvIndex[globalIdx] = envIdx
						break
					}
				}
			}
		}

		// Initialize results and errors arrays for ALL tests
		m.results = make([]runner.TestResult, len(m.tests))
		m.errors = make([]error, len(m.tests))

		// Start first environment group
		return m.startNextEnvironmentGroup()()
	}
}

func (m *testExecutorModel) startNextEnvironmentGroup() tea.Cmd {
	return func() tea.Msg {
		if m.currentGroupIndex >= len(m.environmentGroups) {
			return executionCompleteMsg{}
		}

		group := m.environmentGroups[m.currentGroupIndex]
		m.currentGroupIndex++

		m.addServiceLog(fmt.Sprintf("Starting environment: %s (%d tests)", group.Name, len(group.Tests)))

		// Set environment variables with cleanup
		var err error
		m.groupCleanup, err = runner.SetEnvironmentVariables(group.EnvVars)
		if err != nil {
			m.addServiceLog(fmt.Sprintf("❌ Failed to set env vars: %v", err))
			return executionFailedMsg{reason: fmt.Sprintf("Failed to set env vars: %v", err)}
		}

		// Start environment
		if err := m.executor.StartEnvironment(); err != nil {
			m.groupCleanup()
			m.addServiceLog(fmt.Sprintf("❌ Failed to start environment for %s: %v", group.Name, err))

			helpMsg := m.executor.GetStartupFailureHelpMessage()
			for _, line := range strings.Split(strings.TrimSpace(helpMsg), "\n") {
				m.addServiceLog(line)
			}

			return executionFailedMsg{reason: fmt.Sprintf("Failed to start environment: %v", err)}
		}

		m.serverStarted = true
		m.serviceStarted = true
		m.addServiceLog("✅ Environment ready")

		// Build list of global test indices for this environment
		envIdx := m.currentGroupIndex - 1 // We already incremented it above
		m.currentEnvTestIndices = make([]int, 0, len(group.Tests))
		for globalIdx, groupIdx := range m.testToEnvIndex {
			if groupIdx == envIdx {
				m.currentEnvTestIndices = append(m.currentEnvTestIndices, globalIdx)
			}
		}

		// Reset environment-specific counters
		m.currentEnvTestsStarted = 0
		m.activeTests = make(map[int]bool)

		// DON'T replace m.tests - keep the full test list
		// DON'T recreate test table - keep showing all tests

		return m.startConcurrentTests()()
	}
}

func (m *testExecutorModel) startConcurrentTests() tea.Cmd {
	return func() tea.Msg {
		if len(m.currentEnvTestIndices) == 0 {
			return executionCompleteMsg{}
		}
		concurrency := m.executor.GetConcurrency()
		m.addServiceLog(fmt.Sprintf("🚀 Starting %d tests with max %d concurrency...\n", len(m.currentEnvTestIndices), concurrency))

		var cmds []tea.Cmd
		for i := 0; i < concurrency && i < len(m.currentEnvTestIndices); i++ {
			globalIndex := m.currentEnvTestIndices[i]
			cmds = append(cmds, func() tea.Msg {
				return testStartedMsg{index: globalIndex, test: m.tests[globalIndex]}
			})
		}

		m.currentEnvTestsStarted = min(concurrency, len(m.currentEnvTestIndices))

		return tea.Batch(cmds...)()
	}
}

func (m *testExecutorModel) startRetryPhase() tea.Cmd {
	return func() tea.Msg {
		// Deduplicate testsToRetry (multiple concurrent tests may have added the same indices)
		seen := make(map[int]bool)
		dedupedRetries := make([]int, 0, len(m.testsToRetry))
		for _, idx := range m.testsToRetry {
			if !seen[idx] {
				seen[idx] = true
				dedupedRetries = append(dedupedRetries, idx)
			}
		}

		m.addServiceLog(fmt.Sprintf("\n🔄 Starting retry phase for %d tests that failed during crash...", len(dedupedRetries)))

		m.inRetryPhase = true

		// Clear results for tests that need retry so completion check doesn't count them as done.
		// Without this, old results in m.results cause premature completion detection.
		for _, idx := range dedupedRetries {
			m.results[idx] = runner.TestResult{}
			m.errors[idx] = nil
			m.completedCount--
		}

		// Reset state for retry execution
		m.currentEnvTestIndices = dedupedRetries
		m.testsToRetry = nil // Clear the retry queue
		m.currentEnvTestsStarted = 0
		m.activeTests = make(map[int]bool)

		// Run retries sequentially (concurrency = 1)
		if len(m.currentEnvTestIndices) > 0 {
			firstIdx := m.currentEnvTestIndices[0]
			m.currentEnvTestsStarted = 1
			return testStartedMsg{index: firstIdx, test: m.tests[firstIdx]}
		}

		return environmentGroupCompleteMsg{}
	}
}

func (m *testExecutorModel) executeTest(index int) tea.Cmd {
	return func() tea.Msg {
		test := m.tests[index]

		logPath := m.executor.GetServiceLogPath()

		result, err := m.executor.RunSingleTest(test)

		// Set RetriedAfterCrash if in retry phase
		if m.inRetryPhase {
			result.RetriedAfterCrash = true
		}

		// Check if this test crashed the server
		if err != nil && !m.executor.CheckServerHealth() {
			log.Warn("Test crashed the server in interactive mode", "testID", test.TraceID, "error", err)

			if m.inRetryPhase {
				// Second crash during retry - mark as definitively crashed
				result.CrashedServer = true
				m.addServiceLog(fmt.Sprintf("❌ Test %s crashed server again on retry", test.TraceID))
			} else {
				// First crash - queue this test and all active tests for retry
				m.addServiceLog(fmt.Sprintf("⚠️  Server crash detected during test %s - will retry failed tests later", test.TraceID))

				// Queue this test for retry
				m.testsToRetry = append(m.testsToRetry, index)

				// Also queue all other active tests for retry (they failed due to the crash)
				for activeIdx := range m.activeTests {
					if activeIdx != index {
						m.testsToRetry = append(m.testsToRetry, activeIdx)
					}
				}
			}

			// Determine if there are more tests to run
			hasMoreTests := false
			if m.inRetryPhase {
				// In retry phase, check if there are more tests in the retry queue
				hasMoreTests = m.currentEnvTestsStarted < len(m.currentEnvTestIndices)
			} else {
				// In normal phase, check if there are more tests overall
				hasMoreTests = index < len(m.tests)-1
			}

			// Attempt to restart the server for next test
			if hasMoreTests {
				m.addServiceLog("🔄 Restarting server...")
				if restartErr := m.executor.RestartServerWithRetry(0); restartErr != nil {
					m.addServiceLog(fmt.Sprintf("❌ Failed to restart server: %v", restartErr))
				} else {
					m.addServiceLog("✅ Server restarted successfully")
				}
			}
		} else {
			log.Debug("Test completed", "testID", test.TraceID, "result", result)
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
