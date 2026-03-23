package cmd

import (
	_ "embed"

	"github.com/Use-Tusk/tusk-cli/internal/utils"
	"github.com/spf13/cobra"
)

const configFlagUsage = "config file (default is .tusk/config.yaml)"

//go:embed short_docs/drift/drift_overview.md
var driftOverviewContent string

var driftCmd = &cobra.Command{
	Use:   "drift",
	Short: "Tusk Drift commands",
	Long:  utils.RenderMarkdown(driftOverviewContent),
}

func init() {
	rootCmd.AddCommand(driftCmd)
	driftCmd.PersistentFlags().StringVar(&cfgFile, "config", "", configFlagUsage)
}

func bindLegacyDriftAliasConfigFlag(cmd *cobra.Command) {
	cmd.Flags().StringVar(&cfgFile, "config", "", configFlagUsage)
}
