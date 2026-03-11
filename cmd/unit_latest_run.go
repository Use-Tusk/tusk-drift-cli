package cmd

import (
	"context"

	"github.com/Use-Tusk/tusk-drift-cli/internal/api"
	"github.com/spf13/cobra"
)

var (
	unitLatestRunRepo   string
	unitLatestRunBranch string
)

var unitLatestRunCmd = &cobra.Command{
	Use:   "latest-run",
	Short: "Get the latest unit test run for a repo branch",
	RunE: func(cmd *cobra.Command, args []string) error {
		repo, branch, err := resolveLatestRunInput(unitLatestRunRepo, unitLatestRunBranch)
		if err != nil {
			return err
		}

		client, authOptions, _, err := api.SetupCloud(context.Background(), false)
		if err != nil {
			return err
		}

		result, err := client.GetLatestUnitTestRun(context.Background(), repo, branch, authOptions)
		if err != nil {
			return err
		}

		return printJSON(result)
	},
}

func init() {
	unitCmd.AddCommand(unitLatestRunCmd)

	unitLatestRunCmd.Flags().StringVar(&unitLatestRunRepo, "repo", "", "Repository in owner/name format (defaults to git origin remote)")
	unitLatestRunCmd.Flags().StringVar(&unitLatestRunBranch, "branch", "", "Branch name (defaults to current git branch)")
}
