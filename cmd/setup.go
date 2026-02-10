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
	setupAPIKey            string
	setupModel             string
	setupSkipPermissions   bool
	setupNoSkipPermissions bool
	setupDisableProgress   bool
	setupSkipToCloud       bool
	setupPrintMode         bool
	setupOutputLogs        bool
	setupEligibilityOnly   bool
	setupVerifyMode        bool
	setupGuidance          string
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "AI-powered setup wizard for Tusk Drift",
	Long:  utils.RenderMarkdown(setupContent),
	RunE:  runSetup,
}

func init() {
	rootCmd.AddCommand(setupCmd)

	setupCmd.Flags().StringVar(&setupAPIKey, "api-key", "", "Your Anthropic API key (requests go directly to Anthropic). If not provided, uses Tusk's secure proxy")
	setupCmd.Flags().StringVar(&setupModel, "model", "claude-sonnet-4-5-20250929", "Claude model to use")
	setupCmd.Flags().BoolVar(&setupSkipPermissions, "skip-permissions", false, "Skip permission prompts for consequential actions (commands, file writes, etc.)")
	setupCmd.Flags().BoolVar(&setupNoSkipPermissions, "no-skip-permissions", false, "In headless mode (--print), still prompt for permissions instead of auto-approving")
	setupCmd.Flags().BoolVar(&setupDisableProgress, "disable-progress-state", false, "Disable progress state (saving to .tusk/setup/PROGRESS.md) or resuming from it")
	setupCmd.Flags().BoolVar(&setupSkipToCloud, "skip-to-cloud", false, "Skip local setup and go directly to cloud setup (for testing)")
	setupCmd.Flags().BoolVar(&setupPrintMode, "print", false, "Headless mode - no TUI, stream output to stdout (auto-approves permissions unless --no-skip-permissions)")
	setupCmd.Flags().BoolVar(&setupOutputLogs, "output-logs", false, "Output all logs (tool calls, messages) to .tusk/logs/setup-<datetime>.log")
	setupCmd.Flags().BoolVar(&setupEligibilityOnly, "eligibility-only", false, "Only check eligibility for SDK setup across all services in the directory tree, output JSON report and exit")
	setupCmd.Flags().BoolVar(&setupVerifyMode, "verify", false, "Verify that an existing Tusk Drift setup is working correctly by re-recording and replaying traces")
	setupCmd.Flags().StringVar(&setupGuidance, "guidance", "", "Additional guidance for the eligibility check agent (used with --eligibility-only)")
	_ = setupCmd.Flags().MarkHidden("guidance") // Hidden - primarily for backend use
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
	// Validate mutually exclusive flags
	modeFlags := 0
	if setupSkipToCloud {
		modeFlags++
	}
	if setupEligibilityOnly {
		modeFlags++
	}
	if setupVerifyMode {
		modeFlags++
	}
	if modeFlags > 1 {
		return fmt.Errorf("--verify, --skip-to-cloud, and --eligibility-only are mutually exclusive")
	}

	apiConfig, err := getAnthropicAPIConfig()
	if err != nil {
		return err
	}

	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Verify mode requires existing .tusk/ directory and config
	if setupVerifyMode {
		tuskDir := filepath.Join(workDir, ".tusk")
		if _, err := os.Stat(tuskDir); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("--verify requires a completed setup.\n\nNo .tusk/ directory found. Please run 'tusk setup' first")
			}
			return fmt.Errorf("failed to check .tusk/ directory: %w", err)
		}
		configPath := filepath.Join(tuskDir, "config.yaml")
		if _, err := os.Stat(configPath); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("--verify requires a completed setup.\n\nNo .tusk/config.yaml found. Please run 'tusk setup' first")
			}
			return fmt.Errorf("failed to check .tusk/config.yaml: %w", err)
		}
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

	// Headless mode (--print) implies skip permissions unless --no-skip-permissions is set
	skipPerms := setupSkipPermissions || (setupPrintMode && !setupNoSkipPermissions)

	cfg := agent.Config{
		APIMode:         apiConfig.Mode,
		APIKey:          apiConfig.APIKey,
		BearerToken:     apiConfig.BearerToken,
		ProxyURL:        apiConfig.URL,
		Model:           setupModel,
		WorkDir:         workDir,
		SkipPermissions: skipPerms,
		DisableProgress: setupDisableProgress,
		SkipToCloud:     setupSkipToCloud,
		PrintMode:       setupPrintMode,
		OutputLogs:      setupOutputLogs,
		EligibilityOnly: setupEligibilityOnly,
		VerifyMode:      setupVerifyMode,
		UserGuidance:    setupGuidance,
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
