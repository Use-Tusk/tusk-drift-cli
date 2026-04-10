package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildNextSteps(t *testing.T) {
	tests := []struct {
		name     string
		run      map[string]any
		expected []string
	}{
		{
			name: "in_progress without webappURL",
			run: map[string]any{
				"status": "in_progress",
				"run_id": "run-123",
			},
			expected: []string{
				"Run is still in progress. Poll again with `tusk unit get-run run-123`.",
			},
		},
		{
			name: "in_progress with webappURL",
			run: map[string]any{
				"status":     "in_progress",
				"run_id":     "run-456",
				"webapp_url": "https://app.usetusk.ai/run/456",
			},
			expected: []string{
				"Run is still in progress. Poll again with `tusk unit get-run run-456`.",
				"Or monitor in the webapp: https://app.usetusk.ai/run/456",
			},
		},
		{
			name: "completed with test scenarios",
			run: map[string]any{
				"status": "completed",
				"run_id": "run-789",
				"test_scenarios": []any{
					map[string]any{"scenario_id": "s1"},
					map[string]any{"scenario_id": "s2"},
				},
			},
			expected: []string{
				"Review a test scenario: `tusk unit get-scenario --run-id run-789 --scenario-id <scenario_id>`",
				"Apply all diffs: `tusk unit get-diffs run-789 | jq -r '.files[].diff' | git apply`",
				"If the tests are mostly correct, prefer small local edits instead of a full retry.",
				"If the run used the wrong mocks, symbols, or overall approach, submit feedback and retry: `tusk unit feedback --run-id run-789 --file feedback.json --retry`",
				"Or trigger an explicit retry with run-level guidance: `tusk unit retry --run-id run-789 --comment \"Wrong mocks for this run\"`",
			},
		},
		{
			name: "completed without test scenarios",
			run: map[string]any{
				"status":         "completed",
				"run_id":         "run-999",
				"test_scenarios": []any{},
			},
			expected: []string{
				"Run completed but no test scenarios were generated.",
			},
		},
		{
			name: "completed with nil test_scenarios",
			run: map[string]any{
				"status": "completed",
				"run_id": "run-000",
			},
			expected: []string{
				"Run completed but no test scenarios were generated.",
			},
		},
		{
			name: "error without webappURL",
			run: map[string]any{
				"status": "error",
				"run_id": "run-err",
			},
			expected: []string{
				"Run encountered an error. Check status_detail for more info.",
			},
		},
		{
			name: "error with webappURL",
			run: map[string]any{
				"status":     "error",
				"run_id":     "run-err2",
				"webapp_url": "https://app.usetusk.ai/run/err2",
			},
			expected: []string{
				"Run encountered an error. Check status_detail for more info.",
				"View in the webapp: https://app.usetusk.ai/run/err2",
			},
		},
		{
			name: "cancelled",
			run: map[string]any{
				"status": "cancelled",
				"run_id": "run-cancel",
			},
			expected: []string{
				"Run was cancelled. Check status_detail for the reason.",
			},
		},
		{
			name: "skipped",
			run: map[string]any{
				"status": "skipped",
				"run_id": "run-skip",
			},
			expected: []string{
				"Run was skipped. Check status_detail for the reason.",
			},
		},
		{
			name: "unknown status returns nil",
			run: map[string]any{
				"status": "unknown_status",
				"run_id": "run-unk",
			},
			expected: nil,
		},
		{
			name:     "empty map returns nil",
			run:      map[string]any{},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildNextSteps(tt.run)
			require.Equal(t, tt.expected, got)
		})
	}
}
