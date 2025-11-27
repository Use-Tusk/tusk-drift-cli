package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Use-Tusk/tusk-drift-cli/internal/api"
	"github.com/Use-Tusk/tusk-drift-cli/internal/auth"
	"github.com/Use-Tusk/tusk-drift-cli/internal/cliconfig"
	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
)

const defaultAPIURL = "https://api.usetusk.ai"

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with Tusk Cloud",
	Long:  `Authenticate with Tusk Cloud using Auth0 SSO device authorization flow.`,
	RunE:  login,
}

func init() {
	authCmd.AddCommand(loginCmd)
}

func login(cmd *cobra.Command, args []string) error {
	fmt.Println("🔐 Tusk CLI Authentication")

	authenticator, err := auth.NewAuthenticator()
	if err != nil {
		return err
	}

	err = authenticator.Login(context.Background())
	if err != nil {
		return fmt.Errorf("Failed to login: %w", err)
	}

	fmt.Printf("✅ Authenticated as %s\n", authenticator.Email)
	fmt.Print("Fetching organization info...")

	// Fetch auth info from backend and cache it
	if err := cacheAuthInfo(authenticator.AccessToken); err != nil {
		// Log but don't fail - auth succeeded, caching is optional
		fmt.Printf("⚠️  Could not cache user info: %v\n", err)
	}

	return nil
}

// cacheAuthInfo fetches user/client info from the backend and caches it locally
func cacheAuthInfo(bearerToken string) error {
	client := api.NewClient(defaultAPIURL, "")
	authOpts := api.AuthOptions{
		BearerToken: bearerToken,
	}

	resp, err := client.GetAuthInfo(context.Background(), &backend.GetAuthInfoRequest{}, authOpts)
	if err != nil {
		return fmt.Errorf("Failed to get auth info: %w", err)
	}

	// Load or create CLI config
	cfg, err := cliconfig.Load()
	if err != nil {
		return fmt.Errorf("Failed to load CLI config: %w", err)
	}

	// Extract user info
	userID := resp.User.GetId()
	userName := resp.User.GetName()
	userEmail := ""
	if resp.User.Email != nil {
		userEmail = *resp.User.Email
	}

	// Handle client selection
	var selectedClientID, selectedClientName string
	switch len(resp.Clients) {
	case 1:
		selectedClientID = resp.Clients[0].Id
		if resp.Clients[0].Name != nil {
			selectedClientName = *resp.Clients[0].Name
		}
		fmt.Printf(" done\n📋 Organization: %s\n", selectedClientName)
	case 0:
		fmt.Println(" done")
	default:
		fmt.Println(" done")
		// Check if previously selected client is still valid
		if cfg.SelectedClientID != "" {
			for _, c := range resp.Clients {
				if c.Id == cfg.SelectedClientID {
					selectedClientID = c.Id
					if c.Name != nil {
						selectedClientName = *c.Name
					}
					fmt.Printf("📋 Organization: %s (remembered from last session)\n", selectedClientName)
					break
				}
			}
		}
		// If no valid previous selection, prompt
		if selectedClientID == "" {
			selectedClientID, selectedClientName = promptClientSelection(resp.Clients)
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

// promptClientSelection prompts the user to select from multiple clients
func promptClientSelection(clients []*backend.AuthInfoClient) (string, string) {
	fmt.Println("\nYou belong to multiple organizations:")
	for i, c := range clients {
		name := "Unnamed"
		if c.Name != nil {
			name = *c.Name
		}
		fmt.Printf("  %d. %s\n", i+1, name)
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("Select organization [1-%d]: ", len(clients))
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		// Default to first option if empty
		if input == "" {
			input = "1"
		}

		choice, err := strconv.Atoi(input)
		if err != nil || choice < 1 || choice > len(clients) {
			fmt.Printf("Please enter a number between 1 and %d\n", len(clients))
			continue
		}

		selected := clients[choice-1]
		name := "Unnamed"
		if selected.Name != nil {
			name = *selected.Name
		}
		fmt.Printf("📋 Selected organization: %s\n", name)
		return selected.Id, name
	}
}
