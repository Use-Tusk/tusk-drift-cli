package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Use-Tusk/tusk-drift-cli/internal/logging"
	"github.com/Use-Tusk/tusk-drift-cli/internal/runner"
	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/components"
	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
)

type viewState int

const (
	listView viewState = iota
	testExecutionView
)

type listModel struct {
	table        table.Model
	tests        []runner.Test
	executor     *runner.Executor
	width        int
	height       int
	state        viewState
	testExecutor *testExecutorModel
	selectedTest *runner.Test
	columns      []table.Column
	suiteOpts    runner.SuiteSpanOptions
}

func ShowTestList(tests []runner.Test) error {
	return ShowTestListWithExecutor(tests, nil, runner.SuiteSpanOptions{})
}

func ShowTestListWithExecutor(tests []runner.Test, executor *runner.Executor, suiteOpts runner.SuiteSpanOptions) error {
	columns := []table.Column{
		{Title: "#", Width: 5},
		{Title: "Trace ID", Width: 32},
		{Title: "Type", Width: 12},
		{Title: "Path", Width: 32},
		{Title: "Status", Width: 10},
		{Title: "Duration", Width: 10},
		{Title: "Recorded At", Width: 32},
	}

	rows := []table.Row{}
	for i, test := range tests {
		timestamp := test.Timestamp
		// Parse and format timestamp with local timezone
		if t, err := time.Parse(time.RFC3339, timestamp); err == nil {
			timestamp = t.Local().Format("2006-01-02 15:04:05 MST")
		} else if len(timestamp) >= 19 {
			// Fallback to old format if parsing fails
			timestamp = timestamp[:10] + " " + timestamp[11:19]
		}

		// Use display fields for better GraphQL representation
		displayType := test.DisplayType
		if displayType == "" {
			displayType = test.Type
		}
		displayPath := test.DisplayName
		if displayPath == "" {
			displayPath = test.Path
		}

		rows = append(rows, table.Row{
			fmt.Sprintf("%d", i+1),
			test.TraceID,
			displayType,
			displayPath,
			test.Status,
			fmt.Sprintf("%dms", test.Duration),
			timestamp,
		})
	}

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

	m := &listModel{
		table:     t,
		tests:     tests,
		executor:  executor,
		state:     listView,
		columns:   columns,
		suiteOpts: suiteOpts,
	}

	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
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
	case tea.KeyMsg:
		switch m.state {
		case listView:
			switch msg.String() {
			case "q", "ctrl+c", "esc":
				return m, tea.Quit
			case "enter":
				selectedRow := m.table.SelectedRow()
				if len(selectedRow) > 0 && m.executor != nil {
					traceID := selectedRow[1]
					for _, test := range m.tests {
						if test.TraceID == traceID {
							opts := &InteractiveOpts{
								OnBeforeEnvironmentStart: m.createSuiteSpanPreparation(),
							}
							executor := newTestExecutorModel([]runner.Test{test}, m.executor, opts)

							logging.SetTestLogger(executor)

							// Set the window size if we have it
							if m.width > 0 && m.height > 0 {
								sizeMsg := tea.WindowSizeMsg{Width: m.width, Height: m.height}
								updatedModel, _ := executor.Update(sizeMsg)
								executor = updatedModel.(*testExecutorModel)
							}

							m.testExecutor = executor
							m.selectedTest = &test
							m.state = testExecutionView
							// Initialize the test executor
							return m, m.testExecutor.Init()
						}
					}
				}
			case "g":
				m.table.GotoTop()
			case "G":
				m.table.GotoBottom()
			}
		case testExecutionView:
			// Handle return from test execution
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

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

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
		if m.state == testExecutionView && m.testExecutor != nil {
			updatedExecutor, cmd := m.testExecutor.Update(msg)
			if exec, ok := updatedExecutor.(*testExecutorModel); ok {
				m.testExecutor = exec
			}
			return m, cmd
		}
	}

	// Update the table only in list view
	if m.state == listView {
		m.table, cmd = m.table.Update(msg)
		cmds = append(cmds, cmd)
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

	// Match padding
	padPerCol := styles.TableCellStyle.GetPaddingLeft() + styles.TableCellStyle.GetPaddingRight()
	contentWidth := max(totalWidth-padPerCol*len(m.columns), 0)

	sum := 0
	for _, c := range m.columns {
		sum += c.Width
	}

	cols := make([]table.Column, len(m.columns))
	copy(cols, m.columns)

	if contentWidth > sum {
		cols[3].Width += contentWidth - sum // grow Path column
	}

	m.table.SetColumns(cols)
	m.table.SetWidth(totalWidth)
}

func (m *listModel) View() string {
	switch m.state {
	case listView:
		header := components.Title(m.width, "AVAILABLE TESTS")
		help := components.Footer(m.width, "↑/↓: navigate • g: go to top • G: go to bottom • enter: run test (with default run options) • q: quit")
		return fmt.Sprintf("%s\n\n%s\n\n%s", header, m.table.View(), help)
	case testExecutionView:
		if m.testExecutor != nil {
			return m.testExecutor.View()
		}
		return "Loading test executor..."
	default:
		return "Unknown state"
	}
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
