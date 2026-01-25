package cmd

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Use-Tusk/tusk-drift-cli/internal/agent"
	"github.com/Use-Tusk/tusk-drift-cli/internal/api"
	"github.com/Use-Tusk/tusk-drift-cli/internal/auth"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
	"github.com/spf13/cobra"
	"golang.org/x/term"
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

	setupCmd.Flags().StringVar(&setupAPIKey, "api-key", "", "Anthropic API key (or set ANTHROPIC_API_KEY env var) - uses Tusk backend if not provided")
	setupCmd.Flags().StringVar(&setupModel, "model", "claude-sonnet-4-5-20250929", "Claude model to use")
	setupCmd.Flags().BoolVar(&setupSkipPermissions, "skip-permissions", false, "Skip permission prompts for consequential actions (commands, file writes, etc.)")
	setupCmd.Flags().BoolVar(&setupDisableProgress, "disable-progress-state", false, "Disable progress state (saving to a PROGRESS.md file) or resuming from it")
	setupCmd.Flags().BoolVar(&setupSkipToCloud, "skip-to-cloud", false, "Skip local setup and go directly to cloud setup (for testing)")
	setupCmd.Flags().BoolVar(&setupPrintMode, "print", false, "Headless mode - no TUI, stream output to stdout")
	setupCmd.Flags().BoolVar(&setupOutputLogs, "output-logs", false, "Output all logs (tool calls, messages) to .tusk/logs/setup-<datetime>.log")
	setupCmd.Flags().BoolVar(&setupEligibilityOnly, "eligibility-only", false, "Only check eligibility for SDK setup across all services in the directory tree, output JSON report and exit")
}

// APIConfig holds the configuration for connecting to the LLM API
type APIConfig struct {
	Mode        agent.APIMode
	APIKey      string // For direct mode (BYOK)
	BearerToken string // For proxy mode
	URL         string // Base URL for proxy mode
}

// getAnthropicAPIConfig determines how to connect to the LLM API
// Priority:
// 1. --api-key flag: BYOK (direct to Anthropic)
// 2. ANTHROPIC_API_KEY env var: Ask user preference (BYOK or proxy)
// 3. No API key: Use Tusk backend proxy (requires login)
func getAnthropicAPIConfig() (*APIConfig, error) {
	if setupAPIKey != "" {
		return &APIConfig{
			Mode:   agent.APIModeDirect,
			APIKey: setupAPIKey,
		}, nil
	}

	if envKey := os.Getenv("ANTHROPIC_API_KEY"); envKey != "" {
		// In non-interactive mode (CI/scripts), default to BYOK to avoid hanging
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return &APIConfig{
				Mode:   agent.APIModeDirect,
				APIKey: envKey,
			}, nil
		}

		choice := utils.PromptUserChoice(
			"Found ANTHROPIC_API_KEY in environment. How would you like to proceed?",
			[]string{
				"Use my own API key (BYOK)",
				"Use Tusk backend (requires login, no API key needed)",
			},
		)
		if choice == 0 {
			return &APIConfig{
				Mode:   agent.APIModeDirect,
				APIKey: envKey,
			}, nil
		}
		// Fall through to proxy
	}

	authenticator, err := auth.NewAuthenticator()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize authenticator: %w", err)
	}

	ctx := context.Background()
	if err := authenticator.TryExistingAuth(ctx); err != nil {
		fmt.Println("🔐 Login required to use Tusk's AI backend.")
		fmt.Println("   (Or provide your own Anthropic API key with --api-key)")
		fmt.Println()

		if err := authenticator.Login(ctx); err != nil {
			return nil, fmt.Errorf("login failed: %w\n\nAlternatively, provide your own API key with --api-key or ANTHROPIC_API_KEY", err)
		}
	}

	return &APIConfig{
		Mode:        agent.APIModeProxy,
		BearerToken: authenticator.AccessToken,
		URL:         api.GetBaseURL() + "/api/drift/setup-agent",
	}, nil
}

func runSetup(cmd *cobra.Command, args []string) error {
	apiConfig, err := getAnthropicAPIConfig()
	if err != nil {
		return err
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
	}

	cfg := agent.Config{
		APIMode:         apiConfig.Mode,
		APIKey:          apiConfig.APIKey,
		BearerToken:     apiConfig.BearerToken,
		ProxyURL:        apiConfig.URL,
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
