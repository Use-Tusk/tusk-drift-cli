package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Use-Tusk/tusk-drift-cli/internal/api"
	"github.com/Use-Tusk/tusk-drift-cli/internal/auth"
	"github.com/Use-Tusk/tusk-drift-cli/internal/cliconfig"
	"github.com/Use-Tusk/tusk-drift-cli/internal/log"
	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
)

var selectOrgCmd = &cobra.Command{
	Use:   "select-org",
	Short: "Select a different organization",
	Long:  `Select a different organization if you belong to multiple.`,
	RunE:  selectOrg,
}

func init() {
	authCmd.AddCommand(selectOrgCmd)
}

func selectOrg(cmd *cobra.Command, args []string) error {
	// Get existing auth to check JWT status
	authenticator, err := auth.NewAuthenticator()
	if err != nil {
		return err
	}

	hasJWT := authenticator.TryExistingAuth(context.Background()) == nil

	_, effectiveMethod := cliconfig.GetAuthMethod(hasJWT)

	switch effectiveMethod {
	case cliconfig.AuthMethodAPIKey:
		return fmt.Errorf("Organization selection is not available with API key authentication.\nThe organization is determined by your API key")
	case cliconfig.AuthMethodNone:
		return fmt.Errorf("Not logged in. Please run 'tusk auth login' first")
	}

	// Fetch available clients
	client := api.NewClient(api.GetBaseURL(), "")
	authOpts := api.AuthOptions{
		BearerToken: authenticator.AccessToken,
	}

	resp, err := client.GetAuthInfo(context.Background(), &backend.GetAuthInfoRequest{}, authOpts)
	if err != nil {
		return fmt.Errorf("Failed to get auth info: %w", err)
	}

	if len(resp.Clients) == 0 {
		return fmt.Errorf("No organizations found for your account")
	}

	if len(resp.Clients) == 1 {
		name := "Unnamed"
		if resp.Clients[0].Name != nil {
			name = *resp.Clients[0].Name
		}
		log.UserInfo(fmt.Sprintf("You only belong to one organization: %s (%s)", name, resp.Clients[0].Id))
		return nil
	}

	cfg := cliconfig.CLIConfig

	// Prompt for new selection (selector shows current selection)
	selectedID, selectedName := promptClientSelection(resp.Clients, cfg.SelectedClientID)

	// Update config
	cfg.SelectedClientID = selectedID
	cfg.SelectedClientName = selectedName

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("Failed to save selection: %w", err)
	}

	return nil
}
