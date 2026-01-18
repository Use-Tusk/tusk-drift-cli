package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Use-Tusk/tusk-drift-cli/internal/auth"
	"github.com/Use-Tusk/tusk-drift-cli/internal/cliconfig"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out from Tusk Cloud",
	Long:  `Log out from Tusk Cloud by removing stored authentication credentials.`,
	RunE:  logout,
}

func init() {
	authCmd.AddCommand(logoutCmd)
}

func logout(cmd *cobra.Command, args []string) error {
	fmt.Println("🔓 Logging out from Tusk...")

	authenticator, err := auth.NewAuthenticator()
	if err != nil {
		return err
	}

	err = authenticator.Logout()
	if err != nil {
		return fmt.Errorf("failed to logout: %w", err)
	}

	// Clear cached auth info from CLI config
	cfg := cliconfig.CLIConfig
	cfg.ClearAuthInfo()
	_ = cfg.Save() // Best effort, don't fail logout if this fails

	fmt.Println("✓ Successfully logged out")
	return nil
}
