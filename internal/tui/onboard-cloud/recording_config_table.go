package onboardcloud

import (
	"fmt"
	"strconv"

	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type RecordingConfigTable struct {
	table                 table.Model
	samplingRate          string
	exportSpans           bool
	enableEnvVarRecording bool
	focused               bool
	EditMode              bool // For editing sampling rate
	cursor                int
}

func NewRecordingConfigTable(samplingRate string, exportSpans, enableEnvVarRecording bool) *RecordingConfigTable {
	columns := []table.Column{
		{Title: "Setting", Width: 35},
		{Title: "Value", Width: 25},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(5),
	)

	s := table.DefaultStyles()
	s.Header = lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(styles.PrimaryColor)).
		BorderBottom(true).
		Bold(true).
		Padding(0, 1)
	s.Selected = lipgloss.NewStyle().
		Foreground(lipgloss.Color(styles.PrimaryColor)).
		Background(lipgloss.Color(styles.SubtleBgColor)).
		Bold(true).
		Padding(0, 1)
	s.Cell = lipgloss.NewStyle().Padding(0, 1)

	t.SetStyles(s)

	rct := &RecordingConfigTable{
		table:                 t,
		samplingRate:          samplingRate,
		exportSpans:           true, // Required for cloud onboarding
		enableEnvVarRecording: enableEnvVarRecording,
		focused:               true,
		cursor:                0,
	}

	rct.updateRows()
	return rct
}

func (rct *RecordingConfigTable) updateRows() {
	rate, _ := strconv.ParseFloat(rct.samplingRate, 64)
	rateDisplay := fmt.Sprintf("%.2f (%.0f%%)", rate, rate*100)
	if rct.EditMode && rct.cursor == 0 {
		rateDisplay = "→ " + rct.samplingRate + "_"
	}

	formatBool := func(b bool) string {
		if b {
			return "✓ true"
		}
		return "✗ false"
	}

	rows := []table.Row{
		{"Sampling Rate", rateDisplay},
		{"Export Spans", formatBool(rct.exportSpans)},
		{"Record Environment Variables", formatBool(rct.enableEnvVarRecording)},
	}

	rct.table.SetRows(rows)
}

func (rct *RecordingConfigTable) Update(msg tea.Msg) (*RecordingConfigTable, tea.Cmd) {
	if !rct.focused {
		return rct, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// If in edit mode (typing sampling rate)
		if rct.EditMode && rct.cursor == 0 {
			switch msg.String() {
			case "tab", "esc":
				rct.EditMode = false
				rct.updateRows()
				return rct, nil
			case "backspace":
				if len(rct.samplingRate) > 0 {
					rct.samplingRate = rct.samplingRate[:len(rct.samplingRate)-1]
					rct.updateRows()
				}
				return rct, nil
			default:
				if len(msg.String()) == 1 {
					char := msg.String()
					if (char >= "0" && char <= "9") || char == "." {
						rct.samplingRate += char
						rct.updateRows()
					}
				}
				return rct, nil
			}
		}

		// Normal navigation mode - ONLY handle keys we care about
		switch msg.String() {
		case "up", "k":
			if rct.cursor > 0 {
				rct.cursor--
				rct.table.MoveUp(1)
			}
			rct.updateRows()
			return rct, nil

		case "down", "j":
			if rct.cursor < 2 {
				rct.cursor++
				rct.table.MoveDown(1)
			}
			rct.updateRows()
			return rct, nil

		case "tab", " ":
			switch rct.cursor {
			case 1:
				rct.exportSpans = !rct.exportSpans
			case 2:
				rct.enableEnvVarRecording = !rct.enableEnvVarRecording
			case 0:
				rct.EditMode = true
			}
			rct.updateRows()
			return rct, nil

		case "e":
			if rct.cursor == 0 {
				rct.EditMode = true
				rct.updateRows()
				return rct, nil
			}
		}

		// Don't consume any other key (left, ctrl+c, esc, enter, etc.),
		// let it pass through to the parent
	}

	return rct, nil
}

func (rct *RecordingConfigTable) View() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		"",
		rct.table.View(),
		"",
	)
}

func (rct *RecordingConfigTable) GetValues() (samplingRate float64, exportSpans, enableEnvVarRecording bool) {
	rate, _ := strconv.ParseFloat(rct.samplingRate, 64)
	return rate, rct.exportSpans, rct.enableEnvVarRecording
}

func (rct *RecordingConfigTable) SetFocused(focused bool) {
	rct.focused = focused
}
