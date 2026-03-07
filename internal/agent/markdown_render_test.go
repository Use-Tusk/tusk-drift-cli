package agent

import (
	"strings"
	"testing"
)

func TestRenderAgentMessage_DoesNotCreateMailtoForGitRemote(t *testing.T) {
	input := "Remote: origin (git@github.com:Use-Tusk/tusk-drift-cli.git)"

	out := renderAgentMessage(input, 100)

	if strings.Contains(out, "mailto:") {
		t.Fatalf("expected no mailto link in rendered output, got: %q", out)
	}
}

func TestRenderAgentMessage_DoesNotCreateMailtoForPlainEmail(t *testing.T) {
	input := "Authenticated as jy@usetusk.ai. Continuing setup..."

	out := renderAgentMessage(input, 100)

	if strings.Contains(out, "mailto:") {
		t.Fatalf("expected no mailto link in rendered output, got: %q", out)
	}
}

func TestRenderAgentMessage_FallbackDoesNotLeakSanitizerBackticks(t *testing.T) {
	input := "Remote: git@github.com:Use-Tusk/tusk-drift-cli.git | Email: jy@usetusk.ai"

	out := renderAgentMessage(input, 100)

	if strings.Contains(out, "`git@github.com:Use-Tusk/tusk-drift-cli.git`") ||
		strings.Contains(out, "`jy@usetusk.ai`") {
		t.Fatalf("expected fallback/rendered output to not contain sanitizer backticks, got: %q", out)
	}
}
