package cmd

import (
	"context"

	"github.com/Use-Tusk/tusk-cli/internal/api"
	"github.com/spf13/cobra"
)

var driftQueryServicesCmd = &cobra.Command{
	Use:          "services",
	Short:        "List available observable services",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, authOptions, _, err := api.SetupCloud(context.Background(), false)
		if err != nil {
			return formatApiError(err)
		}

		result, err := client.ListDriftServices(context.Background(), authOptions)
		if err != nil {
			return formatApiError(err)
		}

		return printJSON(result)
	},
}

func init() {
	driftQueryCmd.AddCommand(driftQueryServicesCmd)
}
