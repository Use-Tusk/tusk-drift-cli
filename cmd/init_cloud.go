package cmd

import (
	onboardcloud "github.com/Use-Tusk/tusk-drift-cli/internal/tui/onboard-cloud"
	"github.com/spf13/cobra"
)

var initCloudCmd = &cobra.Command{
	Use:   "init-cloud",
	Short: "Initialize Tusk Drift Cloud for this service",
	Long:  `Interactive wizard to set up Tusk Drift Cloud integration including authentication, repo connection, and CI configuration.`,
	RunE:  initCloud,
}

func init() {
	rootCmd.AddCommand(initCloudCmd)
}

func initCloud(cmd *cobra.Command, args []string) error {
	err := onboardcloud.Run()
	if err != nil {
		cmd.SilenceUsage = true
	}
	return err
}
