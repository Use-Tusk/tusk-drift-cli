package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Use-Tusk/tusk-drift-cli/internal/analytics"
	"github.com/Use-Tusk/tusk-drift-cli/internal/cliconfig"
)

var analyticsCmd = &cobra.Command{
	Use:   "analytics",
	Short: "Manage analytics settings",
	Long:  `View and manage usage analytics settings for Tusk CLI.`,
}

var analyticsStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current analytics settings",
	Run: func(cmd *cobra.Command, args []string) {
		showAnalyticsStatus()
	},
}

var analyticsEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable usage analytics",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := cliconfig.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		cfg.AnalyticsEnabled = true
		cfg.IsTuskDeveloper = false

		if err := cfg.Save(); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Println("Analytics enabled. Thank you for helping improve Tusk!")
		return nil
	},
}

var analyticsDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable usage analytics",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := cliconfig.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		cfg.AnalyticsEnabled = false

		if err := cfg.Save(); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Println("Analytics disabled.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(analyticsCmd)
	analyticsCmd.AddCommand(analyticsStatusCmd)
	analyticsCmd.AddCommand(analyticsEnableCmd)
	analyticsCmd.AddCommand(analyticsDisableCmd)
}

func showAnalyticsStatus() {
	cfg, err := cliconfig.Load()
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		return
	}

	fmt.Println("Analytics Settings")
	fmt.Println("------------------")

	// Check disable reasons in same order as IsAnalyticsEnabled()
	// (env var > CI > developer mode > user preference)
	switch {
	case cliconfig.IsAnalyticsDisabledByEnv():
		fmt.Println("Status:      Disabled (TUSK_ANALYTICS_DISABLED set)")
	case cliconfig.IsCI():
		fmt.Println("Status:      Disabled (CI environment)")
	case cfg.IsTuskDeveloper:
		fmt.Println("Status:      Disabled (Tusk developer)")
	case !cfg.AnalyticsEnabled:
		fmt.Println("Status:      Disabled (user preference)")
	default:
		fmt.Println("Status:      Enabled")
	}

	fmt.Printf("Device ID:   %s\n", cfg.AnonymousID)

	if cfg.UserID != "" {
		fmt.Printf("Account:     %s (analytics linked to your account)\n", cfg.UserID)
	} else {
		fmt.Println("Account:     Not logged in (analytics are anonymous)")
	}

	fmt.Println()
	fmt.Println(analytics.NoticeText)
}
