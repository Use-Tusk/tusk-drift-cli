package utils

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
	"github.com/charmbracelet/glamour"
)

var (
	cachedRenderer    *glamour.TermRenderer
	rendererOnce      sync.Once
	cachedStyleConfig *styleConfig
	styleConfigOnce   sync.Once
)

// styleConfig holds the cached style configuration
type styleConfig struct {
	baseStyle     string
	overridesJSON []byte
}

// getStyleConfig returns a cached style configuration, creating it on first call.
// Reference: https://github.com/charmbracelet/glamour/tree/master/styles
func getStyleConfig() (*styleConfig, error) {
	var initErr error
	styleConfigOnce.Do(func() {
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
			initErr = fmt.Errorf("failed to marshal style overrides: %w", err)
			return
		}

		cachedStyleConfig = &styleConfig{
			baseStyle:     baseStyle,
			overridesJSON: overridesJSON,
		}
	})
	return cachedStyleConfig, initErr
}

// getRenderer returns a cached glamour renderer, creating it on first call.
// Uses width of 90 for the default renderer.
func getRenderer() (*glamour.TermRenderer, error) {
	var initErr error
	rendererOnce.Do(func() {
		cfg, err := getStyleConfig()
		if err != nil {
			initErr = err
			return
		}

		cachedRenderer, initErr = glamour.NewTermRenderer(
			glamour.WithStandardStyle(cfg.baseStyle),
			glamour.WithWordWrap(90),
			glamour.WithStylesFromJSONBytes(cfg.overridesJSON),
		)
	})
	return cachedRenderer, initErr
}

func RenderMarkdown(markdown string) string {
	if styles.NoColor() || !IsTerminal() {
		return markdown
	}

	renderer, err := getRenderer()
	if err != nil {
		return markdown
	}

	rendered, err := renderer.Render(markdown)
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

	cfg, err := getStyleConfig()
	if err != nil {
		return markdown
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(cfg.baseStyle),
		glamour.WithWordWrap(width),
		glamour.WithStylesFromJSONBytes(cfg.overridesJSON),
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
