package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Use-Tusk/tusk-drift-cli/internal/api"
	"github.com/Use-Tusk/tusk-drift-cli/internal/auth"
	"github.com/Use-Tusk/tusk-drift-cli/internal/cliconfig"
	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
)

var selectClientCmd = &cobra.Command{
	Use:   "select-client",
	Short: "Select a different organization",
	Long:  `Select a different organization if you belong to multiple.`,
	RunE:  selectClient,
}

func init() {
	loginCmd.AddCommand(selectClientCmd)
}

func selectClient(cmd *cobra.Command, args []string) error {
	// Get existing auth
	authenticator, err := auth.NewAuthenticator()
	if err != nil {
		return err
	}

	if err := authenticator.TryExistingAuth(context.Background()); err != nil {
		return fmt.Errorf("not logged in. Please run 'tusk login' first")
	}

	// Fetch available clients
	client := api.NewClient(defaultAPIURL, "")
	authOpts := api.AuthOptions{
		BearerToken: authenticator.AccessToken,
	}

	resp, err := client.GetAuthInfo(context.Background(), &backend.GetAuthInfoRequest{}, authOpts)
	if err != nil {
		return fmt.Errorf("failed to get auth info: %w", err)
	}

	if len(resp.Clients) == 0 {
		return fmt.Errorf("no organizations found for your account")
	}

	if len(resp.Clients) == 1 {
		name := "Unnamed"
		if resp.Clients[0].Name != nil {
			name = *resp.Clients[0].Name
		}
		fmt.Printf("You only belong to one organization: %s\n", name)
		return nil
	}

	// Show current selection
	cfg, err := cliconfig.Load()
	if err != nil {
		return fmt.Errorf("failed to load CLI config: %w", err)
	}

	if cfg.SelectedClientName != "" {
		fmt.Printf("Current organization: %s\n", cfg.SelectedClientName)
	}

	// Prompt for new selection
	selectedID, selectedName := promptClientSelection(resp.Clients)

	// Update config
	cfg.SelectedClientID = selectedID
	cfg.SelectedClientName = selectedName

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save selection: %w", err)
	}

	return nil
}
