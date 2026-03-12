package cmd

import (
	"context"

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

		return printJSON(result)
	},
}

func init() {
	unitCmd.AddCommand(unitGetRunCmd)
}
