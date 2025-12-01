package analytics

import (
	"context"
	"log/slog"
	"os"
	"runtime"
	"sync"

	"github.com/Use-Tusk/tusk-drift-cli/internal/api"
	"github.com/Use-Tusk/tusk-drift-cli/internal/cliconfig"
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
}

// NewClient creates a new analytics client.
// Returns nil if analytics is disabled.
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

	phClient, err := posthog.NewWithConfig(posthogAPIKey, posthog.Config{
		Endpoint: posthogEndpoint,
	})
	if err != nil {
		slog.Debug("Failed to create PostHog client", "error", err)
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

	// Resolve identity for API key auth (no-op for JWT/anonymous)
	c.resolveIdentity()

	props := c.baseProperties()
	for k, v := range properties {
		props[k] = v
	}

	capture := posthog.Capture{
		DistinctId: c.getDistinctID(),
		Event:      event,
		Properties: props,
	}

	if clientID := c.getClientID(); clientID != "" {
		capture.Groups = posthog.NewGroups().Set(groupName, clientID)
	}

	if err := c.posthog.Enqueue(capture); err != nil {
		slog.Debug("Failed to enqueue analytics event", "event", event, "error", err)
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

// resolveIdentity fetches auth info from backend if needed.
// Only fetches for API key auth - JWT auth uses cached cliconfig data.
func (c *Client) resolveIdentity() {
	// Only fetch for API key auth
	if c.getAuthType() != string(cliconfig.AuthMethodAPIKey) {
		return
	}

	c.fetchAuthInfo()
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
			slog.Warn("Failed to fetch auth info for analytics", "error", err)
			return
		}

		c.authInfo = resp
		slog.Debug("Successfully fetched auth info for analytics",
			"userId", resp.User.GetId(),
			"clientCount", len(resp.Clients))
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

	return map[string]any{
		"cli_version":   version.Version,
		"os":            runtime.GOOS,
		"arch":          runtime.GOARCH,
		"authenticated": authenticated,
		"auth_type":     c.getAuthType(),
	}
}

// getAuthType returns the authentication type for analytics.
// Note: This uses cached UserID to determine JWT status, not actual JWT validity.
func (c *Client) getAuthType() string {
	hasJWT := c.config != nil && c.config.UserID != ""
	_, effective := cliconfig.GetAuthMethod(hasJWT)
	return string(effective)
}
