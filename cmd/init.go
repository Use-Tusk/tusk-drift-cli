package cmd

import (
	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/onboard"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Set up a new service with Tusk",
	Long: `Interactive wizard to configure a new service for Tusk replay. 
This will create a .tusk/config.yaml file in the current directory.`,
	RunE: initService,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func initService(cmd *cobra.Command, args []string) error {
	err := onboard.RunOnboardingWizard()
	if err != nil {
		cmd.SilenceUsage = true
	}
	return err
}
