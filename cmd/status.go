package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/auth"
	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
	"github.com/spf13/cobra"
)

// TODO: make this a connection/health check by actually hitting a Tusk BE endpoint

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show authentication status",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		methodEnv := strings.ToLower(os.Getenv("TUSK_AUTH_METHOD"))
		if methodEnv == "" {
			methodEnv = "auto"
		}

		// Auth0/JWT status
		auth0LoggedIn := false
		var auth0Email string
		var auth0ExpiresAt time.Time

		a, aerr := auth.NewAuthenticator()
		if aerr == nil {
			if err := a.TryExistingAuth(ctx); err == nil {
				auth0LoggedIn = true
				auth0Email = a.Email
				auth0ExpiresAt = a.ExpiresAt
			}
		}

		// API key status
		apiKey := config.GetAPIKey()
		apiKeyPresent := apiKey != ""

		// Effective method (what would be used)
		effective := "none"
		switch methodEnv {
		case "auto":
			if auth0LoggedIn {
				effective = "auth0"
			} else if apiKeyPresent {
				effective = "api_key"
			}
		case "auth0", "jwt":
			if auth0LoggedIn {
				effective = "auth0"
			} else {
				effective = "invalid (auth0 selected but not logged in)"
			}
		case "api_key", "api-key", "apikey":
			if apiKeyPresent {
				effective = "api_key"
			} else {
				effective = "invalid (api_key selected but TUSK_API_KEY not set)"
			}
		default:
			effective = "invalid (unknown TUSK_AUTH_METHOD)"
		}

		clientID := os.Getenv("TUSK_CLIENT_ID")
		clientStatus := clientID
		if clientStatus == "" {
			clientStatus = "(unset)"
		}

		fmt.Printf("⚙️ Tusk CLI status\n\n")
		fmt.Printf("Preferred auth method: %s\n", methodEnv)
		fmt.Printf("Effective method: %s\n", effective)

		fmt.Printf("Auth0 logged in: %v\n", auth0LoggedIn)
		if auth0LoggedIn {
			if auth0Email != "" {
				fmt.Printf("    Email: %s\n", auth0Email)
			}
			if !auth0ExpiresAt.IsZero() {
				fmt.Printf("    Token expires at: %s\n", auth0ExpiresAt.Format(time.RFC3339))
			}
		}

		fmt.Printf("API key present (TUSK_API_KEY): %v\n", apiKeyPresent)
		fmt.Printf("Active Tusk client ID (TUSK_CLIENT_ID): %s\n", clientStatus)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
