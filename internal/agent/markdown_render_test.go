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
