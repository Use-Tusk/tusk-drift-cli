package api

import (
	"context"
	"fmt"

	"github.com/Use-Tusk/tusk-drift-cli/internal/auth"
	"github.com/Use-Tusk/tusk-drift-cli/internal/cliconfig"
	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
)

func SetupCloud(ctx context.Context, requireServiceID bool) (*TuskClient, AuthOptions, *config.Config, error) {
	cfg, cfgErr := config.Get()
	if cfgErr != nil {
		return nil, AuthOptions{}, nil, fmt.Errorf("failed to load config: %w", cfgErr)
	}
	if requireServiceID && cfg.Service.ID == "" {
		if !config.HasConfigFile() {
			return nil, AuthOptions{}, nil, fmt.Errorf("no config file found. Run `tusk drift setup` to get started.")
		}
		return nil, AuthOptions{}, nil, fmt.Errorf("service.id in '.tusk/config.yaml' is required. Run `tusk drift setup` to get started.")
	}

	authenticator, aerr := auth.NewAuthenticator()
	if aerr != nil {
		return nil, AuthOptions{}, nil, fmt.Errorf("auth init failed: %w", aerr)
	}

	// Try to get credentials
	var bearer, apiKey string
	var hasJWT bool
	if err := authenticator.TryExistingAuth(ctx); err == nil {
		hasJWT = true
	}

	// Determine which auth method to use
	methodEnv, effectiveMethod := cliconfig.GetAuthMethod(hasJWT)

	switch effectiveMethod {
	case cliconfig.AuthMethodJWT:
		bearer = authenticator.AccessToken
	case cliconfig.AuthMethodAPIKey:
		apiKey = cliconfig.GetAPIKey()
	case cliconfig.AuthMethodNone:
		switch methodEnv {
		case "jwt":
			return nil, AuthOptions{}, nil, fmt.Errorf("not authenticated. No valid JWT found.\nRun `tusk auth login` or set TUSK_API_KEY.\nGet started: %s", DocsSetupURL)
		case "api_key":
			return nil, AuthOptions{}, nil, fmt.Errorf("not authenticated. TUSK_API_KEY is not set.\nRun `tusk auth login` or set TUSK_API_KEY.\nGet started: %s", DocsSetupURL)
		case "auto":
			return nil, AuthOptions{}, nil, fmt.Errorf("not authenticated. Run `tusk auth login` or set TUSK_API_KEY.\nGet started: %s", DocsSetupURL)
		default:
			return nil, AuthOptions{}, nil, fmt.Errorf("invalid TUSK_AUTH_METHOD '%s'. Valid values: auth0|jwt, api_key, auto", methodEnv)
		}
	}

	// Get client ID (env var takes precedence, then selected client from login)
	tuskClientID := cliconfig.CLIConfig.GetClientID()

	client := NewClient(cfg.TuskAPI.URL, apiKey)
	authOptions := AuthOptions{
		APIKey:       apiKey,
		BearerToken:  bearer,
		TuskClientID: tuskClientID,
	}
	return client, authOptions, cfg, nil
}
