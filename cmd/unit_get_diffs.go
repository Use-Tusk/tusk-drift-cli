package cmd

import (
	"context"

	"github.com/spf13/cobra"
)

var unitGetDiffsCmd = &cobra.Command{
	Use:          "get-diffs <run-id>",
	Short:        "Get file diffs for a unit test run",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, authOptions, err := setupUnitCloud()
		if err != nil {
			return err
		}

		result, err := client.GetUnitTestRunFiles(context.Background(), args[0], authOptions)
		if err != nil {
			return formatApiError(err)
		}

		return printJSON(result)
	},
}

func init() {
	unitCmd.AddCommand(unitGetDiffsCmd)
}
