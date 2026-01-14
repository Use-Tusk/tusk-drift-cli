package cmd

import (
	"github.com/spf13/cobra"
)

var expCmd = &cobra.Command{
	Use:    "exp",
	Short:  "Experimental commands",
	Long:   "Experimental commands that are under development.",
	Hidden: true,
}

func init() {
	rootCmd.AddCommand(expCmd)
}
