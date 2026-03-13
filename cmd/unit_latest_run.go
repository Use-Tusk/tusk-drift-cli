package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var (
	unitLatestRunRepo   string
	unitLatestRunBranch string
)

var unitLatestRunCmd = &cobra.Command{
	Use:          "latest-run",
	Short:        "Get the latest unit test run for a repo branch",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		repo, branch, err := resolveLatestRunInput(unitLatestRunRepo, unitLatestRunBranch)
		if err != nil {
			return err
		}

		client, authOptions, err := setupUnitCloud()
		if err != nil {
			return err
		}

		result, err := client.GetLatestUnitTestRun(context.Background(), repo, branch, authOptions)
		if err != nil {
			return err
		}

		result["next_steps"] = buildLatestRunNextSteps(result)
		return printJSON(result)
	},
}

func buildLatestRunNextSteps(result map[string]any) []string {
	latest, _ := result["latest"].(map[string]any)
	if latest == nil {
		return nil
	}

	status, _ := latest["status"].(string)
	runID, _ := latest["run_id"].(string)

	var steps []string
	steps = append(steps, fmt.Sprintf("Get full run details: `tusk unit get-run %s`", runID))

	switch status {
	case "in_progress":
		steps = append(steps, "Run is still in progress. Check back shortly or refer to an earlier run if available.")
	case "completed":
		steps = append(steps, fmt.Sprintf("Apply diffs: `tusk unit get-diffs %s | jq -r '.files[].diff' | git apply`", runID))
	case "error", "cancelled", "skipped":
		steps = append(steps, "Check status_detail in the latest run for more info.")
	}

	return steps
}

func init() {
	unitCmd.AddCommand(unitLatestRunCmd)

	unitLatestRunCmd.Flags().StringVar(&unitLatestRunRepo, "repo", "", "Repository in owner/name format (defaults to git origin remote)")
	unitLatestRunCmd.Flags().StringVar(&unitLatestRunBranch, "branch", "", "Branch name (defaults to current git branch)")
}
