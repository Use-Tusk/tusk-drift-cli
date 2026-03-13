package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var unitGetRunCmd = &cobra.Command{
	Use:          "get-run <run-id>",
	Short:        "Get details for a unit test run",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, authOptions, err := setupUnitCloud()
		if err != nil {
			return err
		}

		result, err := client.GetUnitTestRun(context.Background(), args[0], authOptions)
		if err != nil {
			return err
		}

		result["next_steps"] = buildNextSteps(result)
		return printJSON(result)
	},
}

func buildNextSteps(run map[string]any) []string {
	status, _ := run["status"].(string)
	runID, _ := run["run_id"].(string)
	webappURL, _ := run["webapp_url"].(string)

	scenarios, _ := run["test_scenarios"].([]any)
	hasScenarios := len(scenarios) > 0

	var steps []string

	switch status {
	case "in_progress":
		steps = append(steps, fmt.Sprintf("Run is still in progress. Poll again with `tusk unit get-run %s`.", runID))
		if webappURL != "" {
			steps = append(steps, fmt.Sprintf("Or monitor in the webapp: %s", webappURL))
		}
	case "completed":
		if hasScenarios {
			steps = append(steps, fmt.Sprintf("Review a test scenario: `tusk unit get-scenario --run-id %s --scenario-id <scenario_id>`", runID))
			steps = append(steps, fmt.Sprintf("Apply all diffs: `tusk unit get-diffs %s | jq -r '.files[].diff' | git apply`", runID))
		} else {
			steps = append(steps, "Run completed but no test scenarios were generated.")
		}
	case "error":
		steps = append(steps, "Run encountered an error. Check status_detail for more info.")
		if webappURL != "" {
			steps = append(steps, fmt.Sprintf("View in the webapp: %s", webappURL))
		}
	case "cancelled":
		steps = append(steps, "Run was cancelled. Check status_detail for the reason.")
	case "skipped":
		steps = append(steps, "Run was skipped. Check status_detail for the reason.")
	}

	return steps
}

func init() {
	unitCmd.AddCommand(unitGetRunCmd)
}
