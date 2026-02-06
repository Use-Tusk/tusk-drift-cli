package components

// PanelWidths holds calculated widths for a two-panel layout
type PanelWidths struct {
	Left  int
	Right int
}

const (
	MinLeftPanelWidth  = 45
	MinRightPanelWidth = 40
)

// CalculatePanelWidths computes optimal widths for horizontal layout.
// Uses 50/50 split when both halves can meet their minimums.
// Otherwise shrinks proportionally while respecting minimums.
func CalculatePanelWidths(totalWidth, minLeft, minRight int) PanelWidths {
	halfWidth := totalWidth / 2
	otherHalf := totalWidth - halfWidth

	if halfWidth >= minLeft && otherHalf >= minRight {
		return PanelWidths{Left: halfWidth, Right: otherHalf}
	}

	if totalWidth >= minLeft+minRight {
		return PanelWidths{Left: minLeft, Right: totalWidth - minLeft}
	}

	if totalWidth >= minLeft {
		return PanelWidths{Left: minLeft, Right: totalWidth - minLeft}
	}

	return PanelWidths{Left: totalWidth, Right: 0}
}
