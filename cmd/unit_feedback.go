package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	unitFeedbackRunID string
	unitFeedbackFile  string
	unitFeedbackRetry bool
)

var unitFeedbackCmd = &cobra.Command{
	Use:   "feedback",
	Short: "Submit feedback for one or more unit test scenarios",
	Long: `Submit feedback for one or more unit test scenarios.

The feedback payload must be JSON, provided via --file <path> or --file - for stdin.
It must include at least one run_feedback.comment or one scenario entry.
Use run_feedback.comment when the user wants Tusk to retry the overall run with different guidance, or when the required fixes are too broad to make locally.

Example usage:
tusk unit feedback --run-id <run-id> --file feedback.json
tusk unit feedback --run-id <run-id> --file feedback.json --retry
tusk unit feedback --run-id <run-id> --file - <<'EOF'
{
  "scenarios": [
    {
      "scenario_id": "uuid",
      "positive_feedback": ["covers_critical_path"],
      "comment": "Good scenario and likely worth keeping.",
      "applied_locally": true
    }
  ]
}
EOF

Example payload (schema reference):
{
  "run_feedback": {
    "comment": "The run targeted the right files, but the mocks do not match the real service contracts and several scenarios are asserting on implementation details. Use simpler setup assumptions and focus on externally observable behavior."
  },
  "scenarios": [
    {
      "scenario_id": "uuid",
      "positive_feedback": ["covers_critical_path"],
      "comment": "Good scenario and likely worth keeping.",
      "applied_locally": true
    },
    {
      "scenario_id": "uuid",
      "negative_feedback": ["incorrect_assertion"],
      "comment": "The generated assertion does not match the behavior we want to preserve, so we did not keep this test.",
      "applied_locally": false
    }
  ]
}

Notes:
- Prefer local edits by default when the generated tests are mostly correct.
- Use run_feedback.comment mainly for broad retry guidance, such as wrong mocks, wrong symbols, or an overall incorrect test strategy.
- Use either positive_feedback or negative_feedback for a scenario.
- Allowed positive_feedback values: "covers_critical_path", "valid_edge_case", "caught_a_bug", "other"
- Allowed negative_feedback values: "incorrect_business_assumption", "duplicates_existing_test", "no_value", "incorrect_assertion", "poor_coding_practice", "other"
- Add --retry when the user has asked Tusk to regenerate the run, or when the changes are too large to fix locally. This may take a while.

Thank you for your feedback and helping to improve Tusk!
`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(unitFeedbackRunID) == "" {
			return fmt.Errorf("--run-id must be non-empty")
		}
		if strings.TrimSpace(unitFeedbackFile) == "" {
			return fmt.Errorf("--file must be provided")
		}

		payload, err := readUnitFeedbackPayload(unitFeedbackFile)
		if err != nil {
			return err
		}
		if unitFeedbackRetry {
			obj, ok := payload.(map[string]any)
			if !ok {
				return fmt.Errorf("feedback payload must be a JSON object when using --retry")
			}
			obj["retry"] = true
		}

		client, authOptions, err := setupUnitCloud()
		if err != nil {
			return err
		}

		result, err := client.SubmitUnitTestFeedback(context.Background(), unitFeedbackRunID, payload, authOptions)
		if err != nil {
			return formatApiError(err)
		}

		return printJSON(result)
	},
}

func readUnitFeedbackPayload(path string) (any, error) {
	var raw []byte
	var err error

	if path == "-" {
		raw, err = io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
	} else {
		raw, err = os.ReadFile(path) //nolint:gosec
		if err != nil {
			return nil, fmt.Errorf("read feedback file: %w", err)
		}
	}

	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, fmt.Errorf("feedback payload is empty")
	}

	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("parse feedback json: %w", err)
	}

	if scenarios, ok := payload.([]any); ok {
		return map[string]any{"scenarios": scenarios}, nil
	}

	obj, ok := payload.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("feedback payload must be a JSON object or an array of scenario entries")
	}

	return obj, nil
}

func init() {
	unitCmd.AddCommand(unitFeedbackCmd)

	unitFeedbackCmd.Flags().StringVar(&unitFeedbackRunID, "run-id", "", "Unit test run ID")
	unitFeedbackCmd.Flags().StringVar(&unitFeedbackFile, "file", "", "Path to feedback JSON file, or `-` to read from stdin")
	unitFeedbackCmd.Flags().BoolVar(&unitFeedbackRetry, "retry", false, "Trigger a retry after saving feedback")

	_ = unitFeedbackCmd.MarkFlagRequired("run-id")
	_ = unitFeedbackCmd.MarkFlagRequired("file")
}
