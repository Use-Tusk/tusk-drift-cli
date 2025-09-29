package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Use-Tusk/tusk-drift-cli/internal/auth"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out from Tusk",
	Long:  `Log out from Tusk by removing stored authentication credentials.`,
	RunE:  logout,
}

func init() {
	rootCmd.AddCommand(logoutCmd)
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

	fmt.Println("✓ Successfully logged out")
	return nil
}
