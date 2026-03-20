package runner

import (
	"testing"

	"github.com/Use-Tusk/fence/pkg/fence"
)

// newExecutorForServiceLifecycleTests keeps generic lifecycle tests focused on
// service startup/shutdown behavior rather than sandbox availability.
func newExecutorForServiceLifecycleTests() *Executor {
	e := NewExecutor()
	_ = e.SetSandboxMode(SandboxModeOff)
	return e
}

func TestGetEffectiveSandboxMode(t *testing.T) {
	e := NewExecutor()
	if fence.IsSupported() {
		if got := e.GetEffectiveSandboxMode(); got != SandboxModeStrict {
			t.Fatalf("expected default sandbox mode %q on supported platform, got %q", SandboxModeStrict, got)
		}
	} else {
		if got := e.GetEffectiveSandboxMode(); got != SandboxModeAuto {
			t.Fatalf("expected default sandbox mode %q on unsupported platform, got %q", SandboxModeAuto, got)
		}
	}

	if err := e.SetSandboxMode(SandboxModeStrict); err != nil {
		t.Fatalf("set sandbox mode strict: %v", err)
	}
	if got := e.GetEffectiveSandboxMode(); got != SandboxModeStrict {
		t.Fatalf("expected explicit sandbox mode %q, got %q", SandboxModeStrict, got)
	}
}
