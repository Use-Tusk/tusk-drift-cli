package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Use-Tusk/tusk-drift-cli/internal/agent"
	"github.com/spf13/cobra"
)

var (
	expAPIKey          string
	expModel           string
	expSkipPermissions bool
	expDisableProgress bool
	expSkipToCloud     bool
	expPrintMode       bool
	expOutputLogs      bool
	expEligibilityOnly bool
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
	expSetupCmd.Flags().BoolVar(&expDisableProgress, "disable-progress-state", false, "Disable progress state (saving to a PROGRESS.md file) or resuming from it")
	expSetupCmd.Flags().BoolVar(&expSkipToCloud, "skip-to-cloud", false, "Skip local setup and go directly to cloud setup (for testing)")
	expSetupCmd.Flags().BoolVar(&expPrintMode, "print", false, "Headless mode - no TUI, stream output to stdout")
	expSetupCmd.Flags().BoolVar(&expOutputLogs, "output-logs", false, "Output all logs (tool calls, messages) to .tusk/logs/setup-<datetime>.log")
	expSetupCmd.Flags().BoolVar(&expEligibilityOnly, "eligibility-only", false, "Only check eligibility for SDK setup across all services in the directory tree, output JSON report and exit")
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

	// When skipping to cloud, verify that local setup has been completed
	// Skip this validation if eligibility-only mode is set (it doesn't need prior setup)
	if expSkipToCloud && !expEligibilityOnly {
		configPath := filepath.Join(workDir, ".tusk", "config.yaml")
		if _, err := os.Stat(configPath); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("--skip-to-cloud requires local setup to be complete first.\n\nNo .tusk/config.yaml found. Please run 'tusk exp setup' (without --skip-to-cloud) first to complete local setup, or create .tusk/config.yaml manually")
			}
			return fmt.Errorf("failed to check .tusk/config.yaml: %w", err)
		}
		fmt.Println("🔧 Skipping to cloud setup (--skip-to-cloud mode)")
		fmt.Println()
	}

	cfg := agent.Config{
		APIKey:          apiKey,
		Model:           expModel,
		WorkDir:         workDir,
		SkipPermissions: expSkipPermissions,
		DisableProgress: expDisableProgress,
		SkipToCloud:     expSkipToCloud,
		PrintMode:       expPrintMode,
		OutputLogs:      expOutputLogs,
		EligibilityOnly: expEligibilityOnly,
	}

	a, err := agent.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	a.SetTracker(tracker)

	ctx := context.Background()
	if err := a.Run(ctx); err != nil {
		cmd.SilenceUsage = true
		return err
	}

	return nil
}
