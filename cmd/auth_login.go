package cmd

import (
	"context"
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/Use-Tusk/tusk-drift-cli/internal/api"
	"github.com/Use-Tusk/tusk-drift-cli/internal/auth"
	"github.com/Use-Tusk/tusk-drift-cli/internal/cliconfig"
	"github.com/Use-Tusk/tusk-drift-cli/internal/log"
	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/components"
	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
)

var loginCmd = &cobra.Command{
	Use:          "login",
	Short:        "Authenticate with Tusk Cloud",
	Long:         `Authenticate with Tusk Cloud using Auth0 SSO device authorization flow.`,
	RunE:         login,
	SilenceUsage: true,
}

func init() {
	authCmd.AddCommand(loginCmd)
}

func login(cmd *cobra.Command, args []string) error {
	log.Println("🔐 Tusk CLI Authentication\n")

	authenticator, err := auth.NewAuthenticator()
	if err != nil {
		return err
	}

	err = authenticator.Login(context.Background())
	if err != nil {
		return fmt.Errorf("❌ Login failed: %w", err)
	}

	log.Println(fmt.Sprintf("✅ Authenticated as %s", authenticator.Email))
	log.Print("Fetching organization info...")

	// Fetch auth info from backend and cache it
	if err := cacheAuthInfo(authenticator.AccessToken); err != nil {
		// Log but don't fail - auth succeeded, caching is optional
		log.Println(fmt.Sprintf("⚠️  Could not cache user info: %v", err))
	}

	return nil
}

// cacheAuthInfo fetches user/client info from the backend and caches it locally
func cacheAuthInfo(bearerToken string) error {
	client := api.NewClient(api.GetBaseURL(), "")
	authOpts := api.AuthOptions{
		BearerToken: bearerToken,
	}

	resp, err := client.GetAuthInfo(context.Background(), &backend.GetAuthInfoRequest{}, authOpts)
	if err != nil {
		return fmt.Errorf("Failed to get auth info: %w", err)
	}

	cfg := cliconfig.CLIConfig

	// Extract user info
	userID := resp.User.GetId()
	userName := resp.User.GetName()
	userEmail := ""
	if resp.User != nil {
		if resp.User.CodeHostingUsername != nil {
			userEmail = *resp.User.CodeHostingUsername
		} else if resp.User.Email != nil {
			userEmail = *resp.User.Email
		}
	}

	// Handle client selection
	var selectedClientID, selectedClientName string
	switch len(resp.Clients) {
	case 1:
		selectedClientID = resp.Clients[0].Id
		selectedClientName = "Unnamed"
		if resp.Clients[0].Name != nil {
			selectedClientName = *resp.Clients[0].Name
		}
		log.Println(fmt.Sprintf(" done\n\n📋 Organization: %s (%s)", selectedClientName, selectedClientID))
	case 0:
		log.Println(" done\n")
	default:
		log.Println(" done\n")
		// Check if previously selected client is still valid
		if cfg.SelectedClientID != "" {
			for _, c := range resp.Clients {
				if c.Id == cfg.SelectedClientID {
					selectedClientID = c.Id
					selectedClientName = "Unnamed"
					if c.Name != nil {
						selectedClientName = *c.Name
					}
					boldStyle := lipgloss.NewStyle().Bold(true)
					log.Println(fmt.Sprintf("📋 Organization: %s (%s) - remembered from last session, use %s to change", selectedClientName, selectedClientID, boldStyle.Render("tusk auth select-org")))
					break
				}
			}
		}
		// If no valid previous selection, prompt
		if selectedClientID == "" {
			selectedClientID, selectedClientName = promptClientSelection(resp.Clients, "")
		}
	}

	// Cache the auth info
	cfg.SetAuthInfo(userID, userName, userEmail, selectedClientID, selectedClientName)

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("Failed to save CLI config: %w", err)
	}

	// Alias anonymous ID to user ID in PostHog (only happens once per user)
	if tracker := GetTracker(); tracker != nil && userID != "" {
		tracker.Alias(userID)
	}

	return nil
}

// promptClientSelection prompts the user to select from multiple clients using an interactive selector
func promptClientSelection(clients []*backend.AuthInfoClient, currentID string) (string, string) {
	// Build options for the selector
	options := make([]components.SelectorOption, len(clients))
	for i, c := range clients {
		name := "Unnamed"
		if c.Name != nil {
			name = *c.Name
		}
		options[i] = components.SelectorOption{
			ID:    c.Id,
			Label: name,
		}
	}

	selected, err := components.RunSelector("Select organization", options, currentID)
	if err != nil || selected == nil {
		// User cancelled or error - default to first
		name := "Unnamed"
		if clients[0].Name != nil {
			name = *clients[0].Name
		}
		return clients[0].Id, name
	}

	log.Println(fmt.Sprintf("📋 Selected organization: %s (%s)", selected.Label, selected.ID))
	return selected.ID, selected.Label
}
