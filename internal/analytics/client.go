package analytics

import (
	"log/slog"
	"runtime"

	"github.com/Use-Tusk/tusk-drift-cli/internal/cliconfig"
	"github.com/Use-Tusk/tusk-drift-cli/internal/version"
	"github.com/posthog/posthog-go"
)

// Client wraps the PostHog client for CLI analytics
type Client struct {
	posthog posthog.Client
	config  *cliconfig.Config
}

// NewClient creates a new analytics client
// Returns nil if analytics is disabled
func NewClient() *Client {
	if !cliconfig.IsAnalyticsEnabled() {
		slog.Debug("Analytics disabled, not creating client")
		return nil
	}

	cfg, err := cliconfig.Load()
	if err != nil {
		slog.Debug("Failed to load CLI config for analytics", "error", err)
		return nil
	}

	// #nosec G101 -- This is a public PostHog API key, safe to hardcode
	posthogAPIKey := "phc_mUFon9ykhVY9tga0zS6TPQ7FQloQNO91PQRtXdAREqz"

	client, err := posthog.NewWithConfig(posthogAPIKey, posthog.Config{
		Endpoint: "https://us.i.posthog.com",
	})
	if err != nil {
		slog.Debug("Failed to create PostHog client", "error", err)
		return nil
	}

	return &Client{
		posthog: client,
		config:  cfg,
	}
}

// TrackEvent sends a generic event to PostHog
func (c *Client) TrackEvent(event string, properties map[string]any) {
	if c == nil || c.posthog == nil {
		return
	}

	props := c.baseProperties()
	for k, v := range properties {
		props[k] = v
	}

	capture := posthog.Capture{
		DistinctId: c.config.GetDistinctID(),
		Event:      event,
		Properties: props,
	}

	// Add group if we have a client ID and are using JWT auth
	// (For API key auth, we don't know the actual client - backend derives it from the key)
	//
	// TODO-CLI-ANALYTICS: Unlike SDK analytics (internal/sdkanalytics/posthog.go), CLI analytics
	// does not fetch auth info from the backend. SDK analytics calls fetchAuthInfo() which retrieves
	// the correct client ID for both JWT and API key auth methods. For CLI analytics, we skip this
	// to avoid adding latency to every CLI command. As a result, we only send company group for JWT
	// auth where we have the client ID cached locally from login. For API key users, the backend
	// can derive the client from the key itself when processing events.
	// Can consider fetching auth info (and caching with a hash of API key?) and then using that to send company group.
	authType := c.getAuthType()
	if authType == string(cliconfig.AuthMethodJWT) {
		if clientID := c.config.GetClientID(); clientID != "" {
			capture.Groups = posthog.NewGroups().Set("company", clientID)
		}
	}

	if err := c.posthog.Enqueue(capture); err != nil {
		slog.Debug("Failed to enqueue analytics event", "event", event, "error", err)
	}
}

// Alias connects the anonymous ID to a user ID (call after login)
func (c *Client) Alias(userID string) {
	if c == nil || c.posthog == nil || userID == "" {
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
		slog.Debug("Failed to alias user", "error", err)
		return
	}

	// Mark as aliased and save
	c.config.MarkAliased(userID)
	if err := c.config.Save(); err != nil {
		slog.Debug("Failed to save config after alias", "error", err)
	}
}

// Close flushes pending events and closes the client
func (c *Client) Close() error {
	if c == nil || c.posthog == nil {
		return nil
	}
	return c.posthog.Close()
}

// baseProperties returns common properties for all events
func (c *Client) baseProperties() map[string]any {
	return map[string]any{
		"cli_version":   version.Version,
		"os":            runtime.GOOS,
		"arch":          runtime.GOARCH,
		"authenticated": c.config.UserID != "" || cliconfig.GetAPIKey() != "",
		"auth_type":     c.getAuthType(),
	}
}

// getAuthType returns the authentication type for analytics.
func (c *Client) getAuthType() string {
	hasJWT := c.config.UserID != ""
	_, effective := cliconfig.GetAuthMethod(hasJWT)
	return string(effective)
}
