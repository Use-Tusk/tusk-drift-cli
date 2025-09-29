package utils

import (
	"encoding/json"
	"fmt"

	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
	"github.com/charmbracelet/glamour"
)

// createRenderer creates a new glamour renderer with the appropriate style overrides.
// Reference: https://github.com/charmbracelet/glamour/tree/master/styles
func createRenderer() (*glamour.TermRenderer, error) {
	hasDarkBackground := styles.HasDarkBackground
	baseStyle := "dark"
	if !hasDarkBackground {
		baseStyle = "light"
	}

	styleOverrides := make(map[string]any)
	styleOverrides["document"] = map[string]any{
		"margin": 0,
	}
	styleOverrides["code_block"] = map[string]any{
		"margin": 0,
	}

	if hasDarkBackground {
		styleOverrides["document"].(map[string]any)["color"] = "255"
		styleOverrides["heading"] = map[string]any{
			"color": styles.PrimaryColor,
		}
		styleOverrides["h1"] = map[string]any{
			"color":            "255",
			"background_color": styles.SecondaryColor,
		}
	} else {
		styleOverrides["heading"] = map[string]any{
			"color": styles.SecondaryColor,
		}
		styleOverrides["h1"] = map[string]any{
			"color":            "255",
			"background_color": styles.SecondaryColor,
		}
	}

	overridesJSON, err := json.Marshal(styleOverrides)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal style overrides: %w", err)
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(baseStyle),
		glamour.WithWordWrap(90),
		glamour.WithStylesFromJSONBytes(overridesJSON),
	)
	if err != nil {
		return nil, err
	}
	return renderer, nil
}

func RenderMarkdown(markdown string) string {
	renderer, err := createRenderer()
	if err != nil {
		return markdown
	}
	rendered, err := renderer.Render(markdown)
	if err != nil {
		return markdown
	}
	return rendered
}
