package components

import (
	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
	"github.com/charmbracelet/huh"
)

// SelectorOption represents a selectable item
type SelectorOption struct {
	ID    string
	Label string
}

// RunSelector runs an interactive selector and returns the selected option
func RunSelector(prompt string, options []SelectorOption, currentID string) (*SelectorOption, error) {
	if len(options) == 0 {
		return nil, nil
	}

	// Build huh options
	huhOptions := make([]huh.Option[string], len(options))
	for i, opt := range options {
		var label string
		if opt.ID == currentID {
			label = opt.Label + " (" + opt.ID + " - current)"
		} else {
			label = opt.Label + " (" + opt.ID + ")"
		}
		huhOptions[i] = huh.NewOption(label, opt.ID)
	}

	// Default to current selection or first option
	selected := options[0].ID
	if currentID != "" {
		for _, opt := range options {
			if opt.ID == currentID {
				selected = currentID
				break
			}
		}
	}

	err := huh.NewSelect[string]().
		Title(prompt).
		Options(huhOptions...).
		Value(&selected).
		WithTheme(styles.HuhTheme()).
		Run()
	if err != nil {
		return nil, err
	}

	// Find and return the selected option
	for _, opt := range options {
		if opt.ID == selected {
			return &opt, nil
		}
	}

	return nil, nil
}
