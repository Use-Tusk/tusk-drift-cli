package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Use-Tusk/tusk-drift-cli/internal/cliconfig"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Get and set CLI configuration options",
	Long: `Get and set CLI configuration options.

Configuration is stored in ~/.config/tusk/cli.json

Available configuration keys:
  analytics    Enable or disable usage analytics (true/false)
  darkMode     Dark mode for terminal output (true/false)

Examples:
  tusk config get analytics        # Show current analytics setting
  tusk config set analytics false  # Disable analytics
  tusk config get darkMode         # Show current dark mode setting
  tusk config set darkMode true    # Enable dark mode
  tusk config set darkMode false   # Disable dark mode`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Long: `Get the current value of a configuration key.

Available keys:
  analytics    Usage analytics setting
  darkMode     Dark mode setting`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		cfg := cliconfig.CLIConfig

		switch strings.ToLower(key) {
		case "analytics":
			fmt.Println(cfg.AnalyticsEnabled)
		case "darkmode":
			if cfg.DarkMode != nil {
				fmt.Println(*cfg.DarkMode)
			} else {
				fmt.Println("false")
			}
		default:
			return fmt.Errorf("unknown config key: %s\n\nAvailable keys: analytics, darkMode", key)
		}

		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set the value of a configuration key.

Available keys and values:
  analytics    true/false    Enable or disable usage analytics
  darkMode     true/false    Dark mode for terminal output

Examples:
  tusk config set analytics false
  tusk config set darkMode true`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		value := args[1]
		cfg := cliconfig.CLIConfig

		switch strings.ToLower(key) {
		case "analytics":
			boolVal, err := parseBool(value)
			if err != nil {
				return fmt.Errorf("invalid value for analytics: %s (expected true/false)", value)
			}
			cfg.AnalyticsEnabled = boolVal
			if boolVal {
				// Clear developer mode when enabling analytics
				cfg.IsTuskDeveloper = false
			}
		case "darkmode":
			boolVal, err := parseBool(value)
			if err != nil {
				return fmt.Errorf("invalid value for darkMode: %s (expected true/false)", value)
			}
			cfg.DarkMode = &boolVal
		default:
			return fmt.Errorf("unknown config key: %s\n\nAvailable keys: analytics, darkMode", key)
		}

		if err := cfg.Save(); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Printf("%s = %s\n", key, value)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
}

// parseBool parses a boolean string value
func parseBool(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "true", "1", "yes", "on":
		return true, nil
	case "false", "0", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean value: %s", s)
	}
}
