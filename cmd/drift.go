package cmd

import "github.com/spf13/cobra"

const configFlagUsage = "config file (default is .tusk/config.yaml)"

var driftCmd = &cobra.Command{
	Use:   "drift",
	Short: "Tusk Drift commands",
}

func init() {
	rootCmd.AddCommand(driftCmd)
	driftCmd.PersistentFlags().StringVar(&cfgFile, "config", "", configFlagUsage)
}

func bindLegacyDriftAliasConfigFlag(cmd *cobra.Command) {
	cmd.Flags().StringVar(&cfgFile, "config", "", configFlagUsage)
}
