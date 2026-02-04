package agent

import (
	"strings"
	"testing"
)

func TestEligibilityCheckPhase_UserContext(t *testing.T) {
	tests := []struct {
		name         string
		userContext  string
		wantContains string
		wantMissing  string
	}{
		{
			name:        "no context",
			userContext: "",
			wantMissing: "User Guidance",
		},
		{
			name:         "with context",
			userContext:  "Focus on the api/ folder",
			wantContains: "Focus on the api/ folder",
		},
		{
			name:         "context includes guidance header",
			userContext:  "The legacy/ folder is deprecated",
			wantContains: "## User Guidance",
		},
		{
			name:         "context includes importance note",
			userContext:  "Only check the backend service",
			wantContains: "extremely important to take this into account",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			phase := eligibilityCheckPhase()
			state := &State{UserContext: tt.userContext}
			result := phase.OnEnter(state)

			if tt.wantContains != "" && !strings.Contains(result, tt.wantContains) {
				t.Errorf("expected result to contain %q, got %q", tt.wantContains, result)
			}
			if tt.wantMissing != "" && strings.Contains(result, tt.wantMissing) {
				t.Errorf("expected result to NOT contain %q, got %q", tt.wantMissing, result)
			}
		})
	}
}

func TestEligibilityCheckPhase_ManifestsStillPresent(t *testing.T) {
	phase := eligibilityCheckPhase()
	state := &State{UserContext: "Some user context"}
	result := phase.OnEnter(state)

	// Verify manifests section is still present when user context is provided
	if !strings.Contains(result, "### SDK Manifests") {
		t.Error("expected SDK Manifests section to be present")
	}

	// Verify user context comes after manifests
	manifestIdx := strings.Index(result, "### SDK Manifests")
	guidanceIdx := strings.Index(result, "## User Guidance")
	if guidanceIdx < manifestIdx {
		t.Error("expected User Guidance to come after SDK Manifests")
	}
}
