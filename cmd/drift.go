package cmd

import "github.com/spf13/cobra"

var driftCmd = &cobra.Command{
	Use:   "drift",
	Short: "Tusk Drift commands",
}

func init() {
	rootCmd.AddCommand(driftCmd)
}
