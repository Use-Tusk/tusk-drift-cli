package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var (
	unitRetryRunID   string
	unitRetryComment string
)

var unitRetryCmd = &cobra.Command{
	Use:   "retry",
	Short: "Trigger a retry for a unit test run",
	Long: `Trigger a retry for a unit test run.

Use --comment for broad retry guidance when the generated mocks, symbols, or
overall test direction were wrong for the run.

For agents: prefer small local edits when the generated tests are mostly correct. Use this
command when the user has asked to regenerate the run, or when the required changes are too
broad to fix locally.

Example usage:
tusk unit retry --run-id <run-id>
tusk unit retry --run-id <run-id> --comment "The run targeted the right files, but the mocks do not match the real service contracts and several scenarios assert on implementation details. Use simpler setup assumptions and focus on externally observable behavior."
`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(unitRetryRunID) == "" {
			return fmt.Errorf("--run-id must be non-empty")
		}

		payload := map[string]any{}
		if trimmedComment := strings.TrimSpace(unitRetryComment); trimmedComment != "" {
			payload["comment"] = trimmedComment
		}

		client, authOptions, err := setupUnitCloud()
		if err != nil {
			return err
		}

		result, err := client.RetryUnitTestRun(context.Background(), unitRetryRunID, payload, authOptions)
		if err != nil {
			return formatApiError(err)
		}

		return printJSON(result)
	},
}

func init() {
	unitCmd.AddCommand(unitRetryCmd)

	unitRetryCmd.Flags().StringVar(&unitRetryRunID, "run-id", "", "Unit test run ID")
	unitRetryCmd.Flags().StringVar(&unitRetryComment, "comment", "", "Optional run-level guidance to save before retrying")

	_ = unitRetryCmd.MarkFlagRequired("run-id")
}
