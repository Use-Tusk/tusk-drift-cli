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

	// Add group if we have a client ID
	if clientID := c.config.GetClientID(); clientID != "" {
		capture.Groups = posthog.NewGroups().Set("company", clientID)
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
	_ = c.config.Save()
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
		"authenticated": c.config.UserID != "",
	}
}
