package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/Use-Tusk/tusk-drift-cli/internal/agent"
	"github.com/spf13/cobra"
)

var (
	expAPIKey          string
	expModel           string
	expSkipPermissions bool
)

var expCmd = &cobra.Command{
	Use:    "exp",
	Short:  "Experimental commands",
	Long:   "Experimental commands that are under development.",
	Hidden: true,
}

var expSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "AI-powered setup wizard for Tusk Drift (experimental)",
	Long: `Uses an AI agent to analyze your codebase and set up Tusk Drift automatically.

The agent will:
1. Discover your project structure and dependencies
2. Verify your service starts correctly  
3. Instrument the Tusk Drift SDK
4. Create configuration files
5. Run test recordings and replays

Requires ANTHROPIC_API_KEY environment variable or --api-key flag.`,
	RunE: runExpSetup,
}

func init() {
	rootCmd.AddCommand(expCmd)
	expCmd.AddCommand(expSetupCmd)

	expSetupCmd.Flags().StringVar(&expAPIKey, "api-key", "", "Anthropic API key (or set ANTHROPIC_API_KEY env var)")
	expSetupCmd.Flags().StringVar(&expModel, "model", "claude-sonnet-4-20250514", "Claude model to use")
	expSetupCmd.Flags().BoolVar(&expSkipPermissions, "skip-permissions", false, "Skip permission prompts for consequential actions (commands, file writes, etc.)")
}

func runExpSetup(cmd *cobra.Command, args []string) error {
	apiKey := expAPIKey
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if apiKey == "" {
		return fmt.Errorf("ANTHROPIC_API_KEY environment variable or --api-key flag is required")
	}

	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	cfg := agent.Config{
		APIKey:          apiKey,
		Model:           expModel,
		WorkDir:         workDir,
		SkipPermissions: expSkipPermissions,
	}

	a, err := agent.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	ctx := context.Background()
	if err := a.Run(ctx); err != nil {
		cmd.SilenceUsage = true
		return err
	}

	return nil
}
