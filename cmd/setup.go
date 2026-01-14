package cmd

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Use-Tusk/tusk-drift-cli/internal/agent"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
	"github.com/spf13/cobra"
)

//go:embed short_docs/setup.md
var setupContent string

var (
	setupAPIKey          string
	setupModel           string
	setupSkipPermissions bool
	setupDisableProgress bool
	setupSkipToCloud     bool
	setupPrintMode       bool
	setupOutputLogs      bool
	setupEligibilityOnly bool
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "AI-powered setup wizard for Tusk Drift",
	Long:  utils.RenderMarkdown(setupContent),
	RunE:  runSetup,
}

func init() {
	rootCmd.AddCommand(setupCmd)

	setupCmd.Flags().StringVar(&setupAPIKey, "api-key", "", "Anthropic API key (or set ANTHROPIC_API_KEY env var)")
	setupCmd.Flags().StringVar(&setupModel, "model", "claude-sonnet-4-20250514", "Claude model to use")
	setupCmd.Flags().BoolVar(&setupSkipPermissions, "skip-permissions", false, "Skip permission prompts for consequential actions (commands, file writes, etc.)")
	setupCmd.Flags().BoolVar(&setupDisableProgress, "disable-progress-state", false, "Disable progress state (saving to a PROGRESS.md file) or resuming from it")
	setupCmd.Flags().BoolVar(&setupSkipToCloud, "skip-to-cloud", false, "Skip local setup and go directly to cloud setup (for testing)")
	setupCmd.Flags().BoolVar(&setupPrintMode, "print", false, "Headless mode - no TUI, stream output to stdout")
	setupCmd.Flags().BoolVar(&setupOutputLogs, "output-logs", false, "Output all logs (tool calls, messages) to .tusk/logs/setup-<datetime>.log")
	setupCmd.Flags().BoolVar(&setupEligibilityOnly, "eligibility-only", false, "Only check eligibility for SDK setup across all services in the directory tree, output JSON report and exit")
}

func runSetup(cmd *cobra.Command, args []string) error {
	apiKey := setupAPIKey
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
	if setupSkipToCloud && !setupEligibilityOnly {
		configPath := filepath.Join(workDir, ".tusk", "config.yaml")
		if _, err := os.Stat(configPath); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("--skip-to-cloud requires local setup to be complete first.\n\nNo .tusk/config.yaml found. Please run 'tusk setup' (without --skip-to-cloud) first to complete local setup, or create .tusk/config.yaml manually")
			}
			return fmt.Errorf("failed to check .tusk/config.yaml: %w", err)
		}
		fmt.Println("🔧 Skipping to cloud setup (--skip-to-cloud mode)")
		fmt.Println()
	}

	cfg := agent.Config{
		APIKey:          apiKey,
		Model:           setupModel,
		WorkDir:         workDir,
		SkipPermissions: setupSkipPermissions,
		DisableProgress: setupDisableProgress,
		SkipToCloud:     setupSkipToCloud,
		PrintMode:       setupPrintMode,
		OutputLogs:      setupOutputLogs,
		EligibilityOnly: setupEligibilityOnly,
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
