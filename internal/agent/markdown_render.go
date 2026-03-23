package agent

import (
	"regexp"
	"strings"

	"github.com/Use-Tusk/tusk-cli/internal/utils"
)

var (
	gitScpLikeRemotePattern   = regexp.MustCompile(`\b[A-Za-z0-9._-]+@[A-Za-z0-9._-]+:[^\s)>\]]+`)
	emailPatternWithDelimiter = regexp.MustCompile(`\b([A-Za-z0-9._%+\-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,})(\s|$|[),>.\]!?\]])`)
)

// renderAgentMessage renders agent output as markdown with terminal styling.
// Falls back to wrapped plain text when markdown rendering isn't available.
func renderAgentMessage(text string, width int) string {
	if width <= 0 {
		width = 80
	}

	originalText := text

	// Prevent markdown autolink/email parsers from converting scp-style Git remotes
	// (e.g., git@github.com:org/repo.git) into broken mailto links.
	text = gitScpLikeRemotePattern.ReplaceAllStringFunc(text, func(match string) string {
		return "`" + match + "`"
	})
	text = emailPatternWithDelimiter.ReplaceAllString(text, "`$1`$2")

	rendered := utils.RenderMarkdownWithWidth(text, width)
	rendered = strings.TrimRight(rendered, "\n")

	// RenderMarkdownWithWidth returns raw text in non-terminal/no-color mode.
	// Keep output readable by wrapping plain text to the viewport width.
	if rendered == text {
		return utils.WrapText(originalText, width)
	}

	return rendered
}
