package utils

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/Use-Tusk/tusk-drift-cli/internal/logging"
)

const (
	progressBarWidth = 50
)

// ProgressBar shows a progress bar based on current/total counts
type ProgressBar struct {
	writer  io.Writer
	message string
	current int
	total   int
	mu      sync.Mutex
	started bool
}

// NewProgressBar creates a new progress bar that outputs to stderr
func NewProgressBar(message string) *ProgressBar {
	return &ProgressBar{
		writer:  os.Stderr,
		message: message,
	}
}

// Start initializes the progress bar (shows empty bar)
func (p *ProgressBar) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.started {
		return
	}
	p.started = true
	p.render()
}

// SetTotal sets the total count for the progress bar
func (p *ProgressBar) SetTotal(total int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.total = total
	p.render()
}

// Add increments the current count by the given amount
func (p *ProgressBar) Add(count int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.current += count
	p.render()
}

// SetCurrent sets the current count directly
func (p *ProgressBar) SetCurrent(current int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.current = current
	p.render()
}

// render draws the progress bar
func (p *ProgressBar) render() {
	if !p.started {
		return
	}

	var percentage float64
	if p.total > 0 {
		percentage = float64(p.current) / float64(p.total)
		if percentage > 1.0 {
			percentage = 1.0
		}
	}

	filledWidth := int(percentage * float64(progressBarWidth))

	bar := make([]rune, progressBarWidth)
	for i := range progressBarWidth {
		switch {
		case i < filledWidth-1:
			bar[i] = '='
		case i == filledWidth-1 && filledWidth > 0:
			bar[i] = '>'
		default:
			bar[i] = '.'
		}
	}

	// Show count if we have a total
	countStr := ""
	if p.total > 0 {
		countStr = fmt.Sprintf(" %d/%d", p.current, p.total)
	} else if p.current > 0 {
		countStr = fmt.Sprintf(" %d", p.current)
	}

	_, _ = fmt.Fprintf(p.writer, "\r%s [%s]%s", p.message, string(bar), countStr)
}

// Finish stops the progress bar and shows a completion message
func (p *ProgressBar) Finish(finalMessage string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.started {
		return
	}

	// Clear the line
	clearWidth := len(p.message) + progressBarWidth + 20 // Extra space for count
	_, _ = fmt.Fprintf(p.writer, "\r%s\r", strings.Repeat(" ", clearWidth))

	if finalMessage != "" {
		_, _ = fmt.Fprintln(p.writer, finalMessage)
	}
}

// Stop clears the progress bar without a final message
func (p *ProgressBar) Stop() {
	p.Finish("")
}

// ProgressTracker handles progress reporting for paginated fetches
type ProgressTracker struct {
	progress            *ProgressBar
	interactive         bool
	message             string
	lastLoggedMilestone int
	total               int
}

func NewProgressTracker(message string, interactive, quiet bool) *ProgressTracker {
	pt := &ProgressTracker{
		interactive: interactive,
		message:     message,
	}

	if !interactive && !quiet {
		pt.progress = NewProgressBar(message)
		pt.progress.Start()
	}

	return pt
}

func (pt *ProgressTracker) SetTotal(total int) {
	pt.total = total
	if pt.progress != nil {
		pt.progress.SetTotal(total)
	} else if pt.interactive && total > 0 {
		logging.LogToService(fmt.Sprintf("%s (0/%d)...", pt.message, total))
	}
}

func (pt *ProgressTracker) Update(current int) {
	if pt.progress != nil {
		pt.progress.SetCurrent(current)
	} else if pt.interactive && pt.total > 0 {
		// Only log when crossing 25% milestones
		percentage := float64(current) / float64(pt.total) * 100
		currentMilestone := int(percentage/25) * 25

		if currentMilestone > pt.lastLoggedMilestone {
			logging.LogToService(fmt.Sprintf("%s (%d/%d, %d%%)...", pt.message, current, pt.total, currentMilestone))
			pt.lastLoggedMilestone = currentMilestone
		}
	}
}

func (pt *ProgressTracker) Finish(finalMessage string) {
	if pt.progress != nil {
		if finalMessage != "" {
			pt.progress.Finish(finalMessage)
		} else {
			pt.progress.Stop()
		}
	} else if pt.interactive && finalMessage != "" {
		logging.LogToService(finalMessage)
	}
}

func (pt *ProgressTracker) Stop() {
	if pt.progress != nil {
		pt.progress.Stop()
	}
}
