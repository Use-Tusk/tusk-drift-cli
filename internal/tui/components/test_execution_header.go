package components

import (
	"fmt"

	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

type TestExecutionHeaderComponent struct {
	spinner   spinner.Model
	progress  progress.Model
	testCount int
	completed int
	passed    int
	failed    int
	running   int
	state     string // "running" or "completed"
}

func NewTestExecutionHeaderComponent(testCount int) *TestExecutionHeaderComponent {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(styles.AccentColor))

	opts := []progress.Option{}
	if styles.NoColor() {
		opts = append(opts, progress.WithColorProfile(termenv.Ascii))
	} else {
		opts = append(opts, progress.WithDefaultGradient())
	}
	p := progress.New(opts...)

	return &TestExecutionHeaderComponent{
		spinner:   s,
		progress:  p,
		testCount: testCount,
		state:     "running",
	}
}

func (h *TestExecutionHeaderComponent) Update(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd

	var scmd tea.Cmd
	h.spinner, scmd = h.spinner.Update(msg)
	if scmd != nil {
		cmds = append(cmds, scmd)
	}

	// Drive progress animation
	var pm tea.Model
	var pcmd tea.Cmd
	pm, pcmd = h.progress.Update(msg)
	if p, ok := pm.(progress.Model); ok {
		h.progress = p
	}
	if pcmd != nil {
		cmds = append(cmds, pcmd)
	}

	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (h *TestExecutionHeaderComponent) View(width int) string {
	title := Title(width, "TEST EXECUTION")

	// For narrow terminals, use compact format
	if width < 80 {
		h.progress.Width = max(width-30, 10) // Reserve 30 chars for stats

		progressLine := fmt.Sprintf("%s %d/%d (%d run, %d✓, %d✗)",
			h.progress.View(),
			h.completed,
			h.testCount,
			h.running,
			h.passed,
			h.failed,
		)

		return lipgloss.JoinVertical(lipgloss.Left,
			title,
			progressLine,
			"", // Empty line for spacing
		)
	} else {
		statsText := fmt.Sprintf("%d/%d completed | 🏃 %d running | ✅ %d passed | 🟠 %d deviations",
			h.completed, h.testCount, h.running, h.passed, h.failed,
		)
		statsWidth := lipgloss.Width(statsText) + 1 // Space between bar and stats

		var percent float64
		if h.testCount > 0 {
			percent = 100 * float64(h.completed) / float64(h.testCount)
		}
		percentLabelWidth := lipgloss.Width(fmt.Sprintf(" %3.0f%%", percent))

		h.progress.Width = max(width-statsWidth-percentLabelWidth, 15)

		progressLine := fmt.Sprintf("%s %s", h.progress.View(), statsText)

		return lipgloss.JoinVertical(lipgloss.Left,
			title,
			progressLine,
			"", // Empty line for spacing
		)
	}
}

func (h *TestExecutionHeaderComponent) UpdateStats(completed, passed, failed, running int) tea.Cmd {
	h.completed = completed
	h.passed = passed
	h.failed = failed
	h.running = running

	var percent float64
	if h.testCount > 0 {
		percent = float64(h.completed) / float64(h.testCount)
	}
	return h.progress.SetPercent(percent)
}

func (h *TestExecutionHeaderComponent) SetCompleted() {
	h.state = "completed"
}

func (h *TestExecutionHeaderComponent) SetInitialProgress() tea.Cmd {
	if h.testCount > 0 {
		percent := 0.5 / float64(h.testCount)
		return h.progress.SetPercent(percent)
	}

	// Fallback to 0% if we don't know test count yet
	return h.progress.SetPercent(0.0)
}
