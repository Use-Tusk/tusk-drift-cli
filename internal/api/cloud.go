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
	if cfg.TuskAPI.URL == "" {
		return nil, AuthOptions{}, nil, fmt.Errorf("Tusk API base URL must be provided in '.tusk/config.yaml' (tusk_api.url)")
	}
	if requireServiceID && cfg.Service.ID == "" {
		return nil, AuthOptions{}, nil, fmt.Errorf("Service ID is required. Set config.service.id in '.tusk/config.yaml'")
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
		// Provide context-specific error messages
		switch methodEnv {
		case "jwt":
			return nil, AuthOptions{}, nil, fmt.Errorf("auth method 'jwt' selected, but no valid JWT found. Run `tusk auth login`")
		case "api_key":
			return nil, AuthOptions{}, nil, fmt.Errorf("auth method 'api_key' selected, but TUSK_API_KEY is not set")
		case "auto":
			return nil, AuthOptions{}, nil, fmt.Errorf("not authenticated. Either run `tusk auth login` or set TUSK_API_KEY")
		default:
			return nil, AuthOptions{}, nil, fmt.Errorf("invalid TUSK_AUTH_METHOD '%s'. Valid values: auth0|jwt, api_key, auto", methodEnv)
		}
	}

	// Get client ID (env var takes precedence, then selected client from login)
	var tuskClientID string
	if cliCfg, err := cliconfig.Load(); err == nil {
		tuskClientID = cliCfg.GetClientID()
	}

	client := NewClient(cfg.TuskAPI.URL, apiKey)
	authOptions := AuthOptions{
		APIKey:       apiKey,
		BearerToken:  bearer,
		TuskClientID: tuskClientID,
	}
	return client, authOptions, cfg, nil
}
