package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/api"
	"github.com/Use-Tusk/tusk-drift-cli/internal/auth"
	"github.com/Use-Tusk/tusk-drift-cli/internal/cliconfig"
	"github.com/Use-Tusk/tusk-drift-cli/internal/log"
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

		apiKey := cliconfig.GetAPIKey()
		_, effectiveMethod := cliconfig.GetAuthMethod(auth0LoggedIn)
		apiKeyPresent := apiKey != ""

		cfg := cliconfig.CLIConfig
		localClientID, clientSource := cfg.GetClientIDWithSource()

		// Check cloud connection (if we have credentials)
		var cloudResp *backend.GetAuthInfoResponse
		var cloudErr error
		if effectiveMethod != cliconfig.AuthMethodNone {
			authAPIKey := ""
			if effectiveMethod == cliconfig.AuthMethodAPIKey {
				authAPIKey = apiKey
			}

			client := api.NewClient(api.GetBaseURL(), authAPIKey)
			authOpts := api.AuthOptions{
				BearerToken:  bearerToken,
				APIKey:       authAPIKey,
				TuskClientID: localClientID,
			}

			cloudResp, cloudErr = client.GetAuthInfo(ctx, &backend.GetAuthInfoRequest{}, authOpts)
		}

		// Print status
		log.Println("⚙️ Tusk CLI status\n")

		// User and Organization (from cloud response)
		if effectiveMethod == cliconfig.AuthMethodNone {
			log.Println("User: (not authenticated)")
			log.Println("Organization: (not authenticated)")
		} else if cloudErr != nil {
			log.Println("User: (unknown - connection failed)")
			log.Println("Organization: (unknown - connection failed)")
		} else {
			// User info
			if effectiveMethod == cliconfig.AuthMethodAPIKey {
				log.Println("User: (API key)")
			} else if cloudResp.User != nil && cloudResp.User.GetName() != "" {
				log.Println(fmt.Sprintf("User: %s", cloudResp.User.GetName()))
			} else {
				log.Println("User: (unknown)")
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
					log.Println(fmt.Sprintf("Organization: %s (%s, %s)", activeClientName, activeClientID, clientSource))
				} else {
					log.Println(fmt.Sprintf("Organization: %s (%s)", activeClientName, activeClientID))
				}
			} else if activeClientID != "" {
				log.Println(fmt.Sprintf("Organization: %s", activeClientID))
			} else {
				log.Println("Organization: (none)")
			}
		}

		// Auth method details
		log.Println(fmt.Sprintf("\nAuth method: %s", effectiveMethod))
		if auth0LoggedIn {
			log.Println(fmt.Sprintf("Auth0 logged in: true (expires %s)", auth0ExpiresAt.Format(time.RFC3339)))
		} else {
			log.Println("Auth0 logged in: false")
		}
		log.Println(fmt.Sprintf("API key present: %v (set via TUSK_API_KEY env var)", apiKeyPresent))

		// Cloud connection status
		if effectiveMethod == cliconfig.AuthMethodNone {
			log.Println("\nTusk Cloud connection: ⚠️  Not authenticated")
		} else if cloudErr != nil {
			log.Println(fmt.Sprintf("\nTusk Cloud connection: ❌ Failed (%v)", cloudErr))
		} else {
			log.Println("\nTusk Cloud connection: ✅ Success")
		}

		return nil
	},
}

func init() {
	authCmd.AddCommand(statusCmd)
}
