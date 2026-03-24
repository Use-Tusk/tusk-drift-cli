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
)

var unitFeedbackCmd = &cobra.Command{
	Use:   "feedback",
	Short: "Submit feedback for one or more unit test scenarios",
	Long: `Submit feedback for one or more unit test scenarios.

The feedback payload must be JSON, provided via --file <path> or --file - for stdin.

Example usage:
tusk unit feedback --run-id <run-id> --file feedback.json
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
- Use either positive_feedback or negative_feedback for a scenario.
- Allowed positive_feedback values: "covers_critical_path", "valid_edge_case", "caught_a_bug", "other"
- Allowed negative_feedback values: "incorrect_business_assumption", "duplicates_existing_test", "no_value", "incorrect_assertion", "poor_coding_practice", "other"

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

	_ = unitFeedbackCmd.MarkFlagRequired("run-id")
	_ = unitFeedbackCmd.MarkFlagRequired("file")
}
