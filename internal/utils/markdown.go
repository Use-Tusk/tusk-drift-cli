package utils

import (
	"encoding/json"

	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
	"github.com/charmbracelet/glamour"
)

var (
	markdownRenderer    *glamour.TermRenderer
	markdownStyleConfig *styleConfig
)

// styleConfig holds the cached style configuration
type styleConfig struct {
	baseStyle     string
	overridesJSON []byte
}

func init() {
	// Initialize style configuration
	// Reference: https://github.com/charmbracelet/glamour/tree/master/styles
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
		panic("failed to marshal style overrides: " + err.Error())
	}

	markdownStyleConfig = &styleConfig{
		baseStyle:     baseStyle,
		overridesJSON: overridesJSON,
	}

	// Initialize default renderer with width 90
	markdownRenderer, err = glamour.NewTermRenderer(
		glamour.WithStandardStyle(markdownStyleConfig.baseStyle),
		glamour.WithWordWrap(90),
		glamour.WithStylesFromJSONBytes(markdownStyleConfig.overridesJSON),
	)
	if err != nil {
		panic("failed to create glamour renderer: " + err.Error())
	}
}

func RenderMarkdown(markdown string) string {
	if styles.NoColor() || !IsTerminal() {
		return markdown
	}

	rendered, err := markdownRenderer.Render(markdown)
	if err != nil {
		return markdown
	}

	return rendered
}

// RenderMarkdownWithWidth renders markdown with a specific word wrap width.
// Uses width of 80 as a fallback if width <= 0.
func RenderMarkdownWithWidth(markdown string, width int) string {
	if styles.NoColor() || !IsTerminal() {
		return markdown
	}

	if width <= 0 {
		width = 80
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(markdownStyleConfig.baseStyle),
		glamour.WithWordWrap(width),
		glamour.WithStylesFromJSONBytes(markdownStyleConfig.overridesJSON),
	)
	if err != nil {
		return markdown
	}

	rendered, err := renderer.Render(markdown)
	if err != nil {
		return markdown
	}

	return rendered
}
