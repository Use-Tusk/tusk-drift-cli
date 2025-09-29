package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Use-Tusk/tusk-drift-cli/internal/auth"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with Tusk",
	Long:  `Authenticate with Tusk using WorkOS SSO device authorization flow.`,
	RunE:  login,
}

func init() {
	rootCmd.AddCommand(loginCmd)
}

func login(cmd *cobra.Command, args []string) error {
	fmt.Println("🔐 Tusk CLI Authentication")

	authenticator, err := auth.NewAuthenticator()
	if err != nil {
		return err
	}

	err = authenticator.Login(context.Background())
	if err != nil {
		return fmt.Errorf("failed to login: %w", err)
	}

	fmt.Printf("✅ Authenticated as %s\n", authenticator.Email)
	return nil
}
