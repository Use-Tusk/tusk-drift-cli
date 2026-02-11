package analytics

import (
	"context"
	"maps"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/api"
	"github.com/Use-Tusk/tusk-drift-cli/internal/auth"
	"github.com/Use-Tusk/tusk-drift-cli/internal/cliconfig"
	"github.com/Use-Tusk/tusk-drift-cli/internal/log"
	"github.com/Use-Tusk/tusk-drift-cli/internal/version"
	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
	"github.com/posthog/posthog-go"
)

const (
	// #nosec G101 -- This is a public PostHog API key, safe to hardcode
	posthogAPIKey   = "phc_mUFon9ykhVY9tga0zS6TPQ7FQloQNO91PQRtXdAREqz"
	posthogEndpoint = "https://us.i.posthog.com"
	groupName       = "company"
)

// Client wraps the PostHog client for analytics
type Client struct {
	posthog posthog.Client
	config  *cliconfig.Config

	// For API key auth: backend fetch capability to resolve user/client identity
	tuskClient *api.TuskClient
	apiKey     string

	// Cached identity from backend fetch (API key auth only)
	authInfo     *backend.GetAuthInfoResponse
	authInfoOnce sync.Once

	// JWT backfill: ensures at most one network call per process
	jwtBackfillOnce sync.Once
}

// NewClient creates a new analytics client.
// Returns nil if analytics is disabled.
func NewClient() *Client {
	if !cliconfig.IsAnalyticsEnabled() {
		log.Debug("Analytics disabled, not creating client")
		return nil
	}

	cfg := cliconfig.CLIConfig

	phClient, err := posthog.NewWithConfig(posthogAPIKey, posthog.Config{
		Endpoint: posthogEndpoint,
	})
	if err != nil {
		log.Debug("Failed to create PostHog client", "error", err)
		return nil
	}

	// Set up backend fetch capability for API key auth
	apiKey := cliconfig.GetAPIKey()
	var tuskClient *api.TuskClient
	if apiKey != "" {
		tuskClient = api.NewClient(api.GetBaseURL(), apiKey)
	}

	return &Client{
		posthog:    phClient,
		config:     cfg,
		tuskClient: tuskClient,
		apiKey:     apiKey,
	}
}

// Track sends an event to PostHog with the provided properties.
func (c *Client) Track(event string, properties map[string]any) {
	if c == nil || c.posthog == nil {
		return
	}

	c.resolveIdentity()

	props := c.baseProperties()
	maps.Copy(props, properties)

	capture := posthog.Capture{
		DistinctId: c.getDistinctID(),
		Event:      event,
		Properties: props,
	}

	if clientID := c.getClientID(); clientID != "" {
		capture.Groups = posthog.NewGroups().Set(groupName, clientID)
	}

	if err := c.posthog.Enqueue(capture); err != nil {
		log.Debug("Failed to enqueue analytics event", "event", event, "error", err)
	}
}

// Alias connects the anonymous ID to a user ID (call after login)
func (c *Client) Alias(userID string) {
	if c == nil || c.posthog == nil || c.config == nil || userID == "" {
		return
	}

	// Only alias if we haven't already aliased to this user
	if !c.config.NeedsAlias(userID) {
		return
	}

	err := c.posthog.Enqueue(posthog.Alias{
		DistinctId: userID,
		Alias:      c.config.AnonymousID,
	})
	if err != nil {
		log.Debug("Failed to alias user", "error", err)
		return
	}

	// Mark as aliased and save
	c.config.MarkAliased(userID)
	if err := c.config.Save(); err != nil {
		log.Debug("Failed to save config after alias", "error", err)
	}
}

// Close flushes pending events and closes the client
func (c *Client) Close() error {
	if c == nil || c.posthog == nil {
		return nil
	}
	return c.posthog.Close()
}

// resolveIdentity fetches auth info from backend if needed.
func (c *Client) resolveIdentity() {
	if c.getAuthType() == string(cliconfig.AuthMethodAPIKey) {
		c.fetchAuthInfo()
		return
	}
	// Temporary backfill: resolve identity for JWT users whose cli.json was
	// never populated. See backfillJWTIdentity comment for details.
	c.backfillJWTIdentity()
}

// fetchAuthInfo fetches auth info from the backend (called once, cached)
func (c *Client) fetchAuthInfo() {
	c.authInfoOnce.Do(func() {
		if c.tuskClient == nil || c.apiKey == "" {
			return
		}

		resp, err := c.tuskClient.GetAuthInfo(
			context.Background(),
			&backend.GetAuthInfoRequest{},
			api.AuthOptions{APIKey: c.apiKey},
		)
		if err != nil {
			log.Warn("Failed to fetch auth info for analytics", "error", err)
			return
		}

		c.authInfo = resp
		log.Debug("Successfully fetched auth info for analytics",
			"userId", resp.User.GetId(),
			"clientCount", len(resp.Clients))
	})
}

// backfillJWTIdentity is a temporary backfill for users who logged in before
// the forward-fixes were added. Those login paths (setup.go, cloud.go,
// onboard-cloud/run.go) saved the auth token to auth.json but never wrote
// user_id to cli.json. The sync.Once block fetches identity from the API and
// persists it. Safe to remove once all active users have updated past this
// version — the forward-fixes in those files now handle both SetAuthInfo and
// Alias directly.
func (c *Client) backfillJWTIdentity() {
	if c.config == nil || c.config.UserID != "" {
		return // already resolved or no config to backfill
	}

	c.jwtBackfillOnce.Do(func() {
		authenticator, err := auth.NewAuthenticator()
		if err != nil {
			log.Debug("JWT backfill: failed to create authenticator", "error", err)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := authenticator.TryExistingAuth(ctx); err != nil {
			log.Debug("JWT backfill: no valid JWT on disk", "error", err)
			return
		}

		client := api.NewClient(api.GetBaseURL(), "")
		authOpts := api.AuthOptions{BearerToken: authenticator.AccessToken}

		resp, err := client.GetAuthInfo(ctx, &backend.GetAuthInfoRequest{}, authOpts)
		if err != nil {
			log.Debug("JWT backfill: failed to fetch auth info", "error", err)
			return
		}

		userID := resp.User.GetId()
		if userID == "" {
			log.Debug("JWT backfill: empty user ID in response")
			return
		}

		userName := resp.User.GetName()
		userEmail := api.UserEmail(resp.User)

		cfg := cliconfig.CLIConfig
		cfg.SetAuthInfo(userID, userName, userEmail, cfg.SelectedClientID, cfg.SelectedClientName)
		if err := cfg.Save(); err != nil {
			log.Debug("JWT backfill: failed to save config", "error", err)
			return
		}

		c.Alias(userID)
		log.Debug("JWT backfill: resolved identity", "userId", userID)
	})
}

// getDistinctID returns the distinct ID for PostHog events
func (c *Client) getDistinctID() string {
	// First try user ID from fetched auth info (API key auth)
	if c.authInfo != nil && c.authInfo.User != nil && c.authInfo.User.Id != "" {
		return c.authInfo.User.Id
	}

	// Fall back to cliconfig (JWT auth or anonymous)
	if c.config != nil {
		return c.config.GetDistinctID()
	}

	// Last resort: anonymous hostname-based ID
	hostname, _ := os.Hostname()
	if hostname != "" {
		return "anonymous-" + hostname
	}
	return "anonymous-unknown"
}

// getClientID returns the client ID for group identification
func (c *Client) getClientID() string {
	// First try from fetched auth info (API key auth)
	if c.authInfo != nil && len(c.authInfo.Clients) > 0 && c.authInfo.Clients[0].Id != "" {
		return c.authInfo.Clients[0].Id
	}

	// Fall back to cliconfig (JWT auth)
	if c.config != nil {
		return c.config.GetClientID()
	}

	return ""
}

// baseProperties returns common properties for all events
func (c *Client) baseProperties() map[string]any {
	// Check if actually authenticated:
	// - JWT: config.UserID is set (cached from login)
	// - API key: authInfo was successfully fetched
	authenticated := false
	if c.config != nil && c.config.UserID != "" {
		authenticated = true
	} else if c.authInfo != nil {
		authenticated = true
	}

	props := map[string]any{
		"cli_version":   version.Version,
		"os":            runtime.GOOS,
		"arch":          runtime.GOARCH,
		"authenticated": authenticated,
		"auth_type":     c.getAuthType(),
	}

	if os.Getenv("CODESPACES") == "true" {
		props["codespace"] = true
		if user := os.Getenv("GITHUB_USER"); user != "" {
			props["codespace_user"] = user
		}
		if repo := os.Getenv("GITHUB_REPOSITORY"); repo != "" {
			props["codespace_repo"] = repo
		}
		if name := os.Getenv("CODESPACE_NAME"); name != "" {
			props["codespace_name"] = name
		}
	}

	return props
}

// getAuthType returns the authentication type for analytics.
// Note: This uses cached UserID to determine JWT status, not actual JWT validity.
func (c *Client) getAuthType() string {
	hasJWT := c.config != nil && c.config.UserID != ""
	_, effective := cliconfig.GetAuthMethod(hasJWT)
	return string(effective)
}
