package cmd

import "github.com/spf13/cobra"

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authentication and organization management",
	Long:  `Manage authentication with Tusk Cloud and organization selection.`,
}

func init() {
	rootCmd.AddCommand(authCmd)
}
