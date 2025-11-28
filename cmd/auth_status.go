package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/api"
	"github.com/Use-Tusk/tusk-drift-cli/internal/auth"
	"github.com/Use-Tusk/tusk-drift-cli/internal/cliconfig"
	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show authentication and connection status",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Auth0/JWT status
		auth0LoggedIn := false
		var auth0ExpiresAt time.Time
		var bearerToken string

		a, aerr := auth.NewAuthenticator()
		if aerr == nil {
			if err := a.TryExistingAuth(ctx); err == nil {
				auth0LoggedIn = true
				auth0ExpiresAt = a.ExpiresAt
				bearerToken = a.AccessToken
			}
		}

		// Get effective auth method
		apiKey := cliconfig.GetAPIKey()
		_, effectiveMethod := cliconfig.GetAuthMethod(auth0LoggedIn)
		apiKeyPresent := apiKey != ""

		// Get client ID from cliconfig (for JWT auth)
		var localClientID string
		var clientSource cliconfig.ClientIDSource
		if cfg, err := cliconfig.Load(); err == nil {
			localClientID, clientSource = cfg.GetClientIDWithSource()
		}

		// Check cloud connection (if we have credentials)
		var cloudResp *backend.GetAuthInfoResponse
		var cloudErr error
		if effectiveMethod != cliconfig.AuthMethodNone {
			authAPIKey := ""
			if effectiveMethod == cliconfig.AuthMethodAPIKey {
				authAPIKey = apiKey
			}

			client := api.NewClient(defaultAPIURL, authAPIKey)
			authOpts := api.AuthOptions{
				BearerToken:  bearerToken,
				APIKey:       authAPIKey,
				TuskClientID: localClientID,
			}

			cloudResp, cloudErr = client.GetAuthInfo(ctx, &backend.GetAuthInfoRequest{}, authOpts)
		}

		// Print status
		fmt.Printf("⚙️ Tusk CLI status\n\n")

		// User and Organization (from cloud response)
		if effectiveMethod == cliconfig.AuthMethodNone {
			fmt.Printf("User: (not authenticated)\n")
			fmt.Printf("Organization: (not authenticated)\n")
		} else if cloudErr != nil {
			fmt.Printf("User: (unknown - connection failed)\n")
			fmt.Printf("Organization: (unknown - connection failed)\n")
		} else {
			// User info
			if effectiveMethod == cliconfig.AuthMethodAPIKey {
				fmt.Printf("User: (API key)\n")
			} else if cloudResp.User != nil && cloudResp.User.GetName() != "" {
				fmt.Printf("User: %s\n", cloudResp.User.GetName())
			} else {
				fmt.Printf("User: (unknown)\n")
			}

			// Organization info - find active client
			var activeClientID, activeClientName string
			if effectiveMethod == cliconfig.AuthMethodJWT && localClientID != "" {
				// For JWT, use locally selected client ID
				activeClientID = localClientID
				for _, c := range cloudResp.Clients {
					if c.Id == localClientID && c.Name != nil {
						activeClientName = *c.Name
						break
					}
				}
			} else if len(cloudResp.Clients) > 0 {
				// For API key, use client from response (backend derives it)
				activeClientID = cloudResp.Clients[0].Id
				if cloudResp.Clients[0].Name != nil {
					activeClientName = *cloudResp.Clients[0].Name
				}
			}

			if activeClientName != "" {
				if effectiveMethod == cliconfig.AuthMethodJWT && clientSource != cliconfig.ClientIDSourceNone {
					fmt.Printf("Organization: %s (%s, %s)\n", activeClientName, activeClientID, clientSource)
				} else {
					fmt.Printf("Organization: %s (%s)\n", activeClientName, activeClientID)
				}
			} else if activeClientID != "" {
				fmt.Printf("Organization: %s\n", activeClientID)
			} else {
				fmt.Printf("Organization: (none)\n")
			}
		}

		// Auth method details
		fmt.Printf("\nAuth method: %s\n", effectiveMethod)
		if auth0LoggedIn {
			fmt.Printf("Auth0 logged in: true (expires %s)\n", auth0ExpiresAt.Format(time.RFC3339))
		} else {
			fmt.Printf("Auth0 logged in: false\n")
		}
		fmt.Printf("API key present: %v (set via TUSK_API_KEY env var)\n", apiKeyPresent)

		// Cloud connection status
		if effectiveMethod == cliconfig.AuthMethodNone {
			fmt.Printf("\nTusk Cloud connection: ⚠️  Not authenticated\n")
		} else if cloudErr != nil {
			fmt.Printf("\nTusk Cloud connection: ❌ Failed (%v)\n", cloudErr)
		} else {
			fmt.Printf("\nTusk Cloud connection: ✅ Success\n")
		}

		return nil
	},
}

func init() {
	authCmd.AddCommand(statusCmd)
}
