package components

import (
	"fmt"
	"strings"

	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
	"github.com/charmbracelet/lipgloss"
)

const (
	// Slightly wider than the minimum width for
	// horizontal layout for test execution view
	TestViewMinRecommendedWidth  = 150
	TestViewMinRecommendedHeight = 40

	// Below this, show warning overlay
	TestViewAbsoluteMinWidth  = 60
	TestViewAbsoluteMinHeight = 25
)

const (
	ListViewMinRecommendedWidth  = 140
	ListViewMinRecommendedHeight = 20

	// Below this, show warning overlay
	ListViewAbsoluteMinWidth  = 55
	ListViewAbsoluteMinHeight = 15
)

// TerminalSizeWarning handles the terminal size warning overlay
type TerminalSizeWarning struct {
	dismissed         bool
	minWidth          int
	minHeight         int
	recommendedWidth  int
	recommendedHeight int
}

func NewTerminalSizeWarning(minWidth, minHeight, recommendedWidth, recommendedHeight int) *TerminalSizeWarning {
	return &TerminalSizeWarning{
		dismissed:         false,
		minWidth:          minWidth,
		minHeight:         minHeight,
		recommendedWidth:  recommendedWidth,
		recommendedHeight: recommendedHeight,
	}
}

// NewTestExecutorSizeWarning creates a warning for test view
func NewTestViewSizeWarning() *TerminalSizeWarning {
	return NewTerminalSizeWarning(TestViewAbsoluteMinWidth, TestViewAbsoluteMinHeight, TestViewMinRecommendedWidth, TestViewMinRecommendedHeight)
}

// NewListViewSizeWarning creates a warning for list view
func NewListViewSizeWarning() *TerminalSizeWarning {
	return NewTerminalSizeWarning(ListViewAbsoluteMinWidth, ListViewAbsoluteMinHeight, ListViewMinRecommendedWidth, ListViewMinRecommendedHeight)
}

// IsTooSmall checks if the terminal size is below minimum
func (w *TerminalSizeWarning) IsTooSmall(width, height int) bool {
	return width < w.minWidth || height < w.minHeight
}

// IsDismissed returns whether the warning has been dismissed
func (w *TerminalSizeWarning) IsDismissed() bool {
	return w.dismissed
}

// Dismiss marks the warning as dismissed
func (w *TerminalSizeWarning) Dismiss() {
	w.dismissed = true
}

// Reset resets the dismissed state (useful when window becomes large then small again)
func (w *TerminalSizeWarning) Reset() {
	w.dismissed = false
}

// ShouldShow returns true if the warning should be displayed.
// Always returns false if TUSK_TUI_CI_MODE=1 to support CI testing.
func (w *TerminalSizeWarning) ShouldShow(width, height int) bool {
	if utils.TUICIMode() {
		return false
	}
	return w.IsTooSmall(width, height) && !w.dismissed
}

// View renders the warning overlay
func (w *TerminalSizeWarning) View(width, height int) string {
	contentWidth := width - 8 // 4 for padding (2 on each side) + 4 for borders and margins
	contentWidth = max(contentWidth, 40)

	warningBox := styles.WarningBoxStyle.Align(lipgloss.Center)
	warningText := styles.WarningStyle.Bold(true).Render("WARNING: Window too small")

	center := styles.TextCenterStyle.Width(contentWidth)

	content := []string{
		center.Render(warningText),
		"",
		center.Render("Please resize your terminal window to view the contents properly."),
		"",
		center.Render(fmt.Sprintf("Current size: %d × %d", width, height)),
		center.Render(fmt.Sprintf("Recommended: %d × %d or larger", w.recommendedWidth, w.recommendedHeight)),
		"",
		center.Render(styles.DimStyle.Render("Press Enter or d to dismiss and show anyway")),
		center.Render(styles.DimStyle.Render("Press q to quit")),
	}

	boxContent := lipgloss.JoinVertical(lipgloss.Left, content...)
	box := warningBox.Render(boxContent)

	verticalPadding := (height - lipgloss.Height(box)) / 2
	if verticalPadding > 0 {
		padding := strings.Repeat("\n", verticalPadding)
		return padding + box
	}

	return box
}
