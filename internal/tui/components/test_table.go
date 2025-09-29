package components

import (
	"fmt"

	"github.com/Use-Tusk/tusk-drift-cli/internal/runner"
	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type TestTableComponent struct {
	table   table.Model
	tests   []runner.Test
	results []runner.TestResult
	errors  []error
	focused bool
	// Fixed baseline (original titles and widths) defined at construction
	baseColumns []table.Column
	// Mutable, resized copy applied to the table for current terminal width.
	// Recalculated from baseColumns on each View/resize.
	columns []table.Column

	completed []bool
}

func NewTestTableComponent(tests []runner.Test) *TestTableComponent {
	columns := []table.Column{
		{Title: "#", Width: 4},
		{Title: "Test", Width: 35},
		{Title: "Status", Width: 10},
		{Title: "Duration", Width: 8},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true), // Sets initial focus
		table.WithHeight(10),
	)

	return &TestTableComponent{
		table:       t,
		tests:       tests,
		results:     make([]runner.TestResult, len(tests)),
		errors:      make([]error, len(tests)),
		focused:     true, // Track our internal focus state
		baseColumns: columns,
		columns:     columns,
		completed:   make([]bool, len(tests)),
	}
}

func (tt *TestTableComponent) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	tt.table, cmd = tt.table.Update(msg)
	return cmd
}

func (tt *TestTableComponent) View(width, height int) string {
	// Safety checks for dimensions
	if width <= 0 {
		width = 50
	}
	if height <= 0 {
		height = 10
	}

	tableWidth := max(width, 0)

	// Resize columns to fill available width; grow "Test" column
	if len(tt.baseColumns) > 0 {
		padPerCol := styles.TableCellStyle.GetPaddingLeft() + styles.TableCellStyle.GetPaddingRight()
		contentWidth := max(tableWidth-padPerCol*len(tt.baseColumns), 0)
		sum := 0
		for _, c := range tt.baseColumns {
			sum += c.Width
		}
		cols := make([]table.Column, len(tt.baseColumns))
		copy(cols, tt.baseColumns)
		if contentWidth > sum {
			cols[1].Width += contentWidth - sum // expand "Test"
		}
		tt.columns = cols
		tt.table.SetColumns(cols)
	}

	tt.table.SetWidth(tableWidth)
	tableHeight := max(height-3, 3)
	tt.table.SetHeight(tableHeight)

	// Update styles based on focus
	style := table.DefaultStyles()
	borderColor := lipgloss.Color("240")
	if tt.focused {
		borderColor = lipgloss.Color(styles.PrimaryColor)
	}

	style.Header = styles.TableHeaderStyle
	style.Cell = styles.TableCellStyle
	style.Selected = styles.TableRowSelectedStyle

	tt.table.SetStyles(style)

	// Build rows
	tt.buildRows()

	title := "Tests"
	if tt.focused {
		title = "► Tests"
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(borderColor).
		MarginBottom(1)

	return lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render(title),
		tt.table.View(),
	)
}

func (tt *TestTableComponent) buildRows() {
	rows := []table.Row{}

	// Know which row is selected so we can add a pointer in no-color mode
	cursor := tt.table.Cursor()

	// Add service logs row as first row
	serviceLabel := "(service logs)"
	if styles.NoColor() && cursor == 0 {
		serviceLabel = "▶ " + serviceLabel
	}
	serviceRow := table.Row{
		"",           // No index number
		serviceLabel, // Test field
		"",           // No status
		"",           // No duration
	}
	rows = append(rows, serviceRow)

	// Add actual test rows
	for i, test := range tt.tests {
		status := "⏳ Pending"
		duration := "-"

		if tt.completed[i] {
			result := tt.results[i]
			err := tt.errors[i]
			switch {
			case err != nil:
				status = "❌ Error"
			case result.Passed:
				status = "✅ Passed"
			default:
				status = "❌ Failed"
			}
			duration = fmt.Sprintf("%dms", result.Duration)
		}

		testDescription := fmt.Sprintf("%s %s", test.DisplayType, test.DisplayName)
		if styles.NoColor() && cursor == i+1 {
			testDescription = "▶ " + testDescription
		}

		maxTestLen := 33
		if len(tt.columns) > 1 {
			if w := tt.columns[1].Width - 2; w > 3 {
				maxTestLen = w
			} else {
				maxTestLen = 3
			}
		}

		row := table.Row{
			fmt.Sprintf("%d", i+1),
			tt.truncate(testDescription, maxTestLen),
			status,
			duration,
		}
		rows = append(rows, row)
	}

	tt.table.SetRows(rows)
}

func (tt *TestTableComponent) truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func (tt *TestTableComponent) SetFocused(focused bool) {
	tt.focused = focused
	if focused {
		tt.table.Focus()
	} else {
		tt.table.Blur()
	}
}

func (tt *TestTableComponent) IsFocused() bool {
	return tt.table.Focused()
}

func (tt *TestTableComponent) GetSelectedTest() *runner.Test {
	selectedRow := tt.table.SelectedRow()
	if len(selectedRow) == 0 {
		return nil
	}

	if selectedRow[0] == "" {
		// Empty index, service logs selected
		return nil
	}

	// Find test by matching the row data (adjust for service logs row offset)
	for i, test := range tt.tests {
		if fmt.Sprintf("%d", i+1) == selectedRow[0] {
			return &test
		}
	}
	return nil
}

func (tt *TestTableComponent) IsServiceLogsSelected() bool {
	selectedRow := tt.table.SelectedRow()
	if len(selectedRow) == 0 {
		return true
	}
	return selectedRow[0] == "" // Service logs row has empty index
}

func (tt *TestTableComponent) UpdateTestResult(index int, result runner.TestResult, err error) {
	if index >= 0 && index < len(tt.results) {
		tt.results[index] = result
		tt.errors[index] = err
		tt.completed[index] = true
	}
}

func (tt *TestTableComponent) GotoTop() {
	tt.table.GotoTop()
}

func (tt *TestTableComponent) GotoBottom() {
	tt.table.GotoBottom()
}
