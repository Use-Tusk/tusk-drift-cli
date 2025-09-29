package cmd

import (
	"github.com/spf13/cobra"

	"github.com/Use-Tusk/tusk-drift-cli/internal/version"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show the version of Tusk CLI",
	Run: func(cmd *cobra.Command, args []string) {
		version.PrintVersion()
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
