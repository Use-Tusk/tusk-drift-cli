package api

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Use-Tusk/tusk-drift-cli/internal/auth"
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

	bearer := ""
	apiKey := ""

	switch strings.ToLower(os.Getenv("TUSK_AUTH_METHOD")) {
	case "", "auto":
		// Default: prefer JWT if available, otherwise API key
		if err := authenticator.TryExistingAuth(ctx); err == nil {
			bearer = authenticator.AccessToken
		} else {
			apiKey = config.GetAPIKey()
			if apiKey == "" {
				return nil, AuthOptions{}, nil, fmt.Errorf("not authenticated. Either run `tusk login` or set TUSK_API_KEY")
			}
		}
	case "auth0", "jwt":
		if err := authenticator.TryExistingAuth(ctx); err != nil {
			return nil, AuthOptions{}, nil, fmt.Errorf("auth method 'auth0' selected, but no valid JWT found. Run `tusk login`")
		}
		bearer = authenticator.AccessToken
	case "api_key", "api-key", "apikey":
		apiKey = config.GetAPIKey()
		if apiKey == "" {
			return nil, AuthOptions{}, nil, fmt.Errorf("auth method 'api_key' selected, but TUSK_API_KEY is not set")
		}
	default:
		return nil, AuthOptions{}, nil, fmt.Errorf("invalid TUSK_AUTH_METHOD. Valid values: auth0|jwt, api_key")
	}

	client := NewClient(cfg.TuskAPI.URL, apiKey)
	authOptions := AuthOptions{
		APIKey:       apiKey,
		BearerToken:  bearer,
		TuskClientID: os.Getenv("TUSK_CLIENT_ID"),
	}
	return client, authOptions, cfg, nil
}
