package components

import (
	tea "github.com/charmbracelet/bubbletea"
)

// DetailsPanel wraps ContentPanel for the list command
type DetailsPanel struct {
	*ContentPanel
}

// NewDetailsPanel creates a new details panel
func NewDetailsPanel() *DetailsPanel {
	panel := NewContentPanel()
	panel.SetTitle("Details")
	panel.EmptyLineAfterTitle = false
	return &DetailsPanel{ContentPanel: panel}
}

// Update handles input messages
func (dp *DetailsPanel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "g":
			dp.GotoTop()
			return nil
		case "G":
			dp.GotoBottom()
			return nil
		case "J":
			dp.ScrollDown(1)
			return nil
		case "K":
			dp.ScrollUp(1)
			return nil
		case "D", "ctrl+d":
			dp.HalfPageDown()
			return nil
		case "U", "ctrl+u":
			dp.HalfPageUp()
			return nil
		}
	}

	return dp.ContentPanel.Update(msg)
}

// SetSize sets the panel dimensions
func (dp *DetailsPanel) SetSize(width, height int) {
	dp.width = width
	dp.height = height

	// Account for: left border (1), left padding (1), scrollbar (1) = 3
	viewportWidth := width - 3
	// Account for: title (1) - no top/bottom borders
	viewportHeight := height - 1

	if viewportWidth < 10 {
		viewportWidth = 10
	}
	if viewportHeight < 3 {
		viewportHeight = 3
	}

	dp.viewport.Width = viewportWidth
	dp.viewport.Height = viewportHeight
}

// View renders the panel (uses stored dimensions from SetSize)
func (dp *DetailsPanel) View() string {
	return dp.ContentPanel.View(dp.width, dp.height)
}
