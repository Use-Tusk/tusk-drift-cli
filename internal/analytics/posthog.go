package analytics

import (
	"context"
	"log/slog"
	"os"

	"github.com/Use-Tusk/tusk-drift-cli/internal/auth"
	"github.com/posthog/posthog-go"
)

// PostHogClient wraps the PostHog client for analytics tracking
type PostHogClient struct {
	client    posthog.Client
	userEmail string
}

// NewPostHogClient creates a new PostHog client
func NewPostHogClient() *PostHogClient {
	apiKey := "phc_mUFon9ykhVY9tga0zS6TPQ7FQloQNO91PQRtXdAREqz"

	client, err := posthog.NewWithConfig(apiKey, posthog.Config{
		Endpoint: "https://us.i.posthog.com",
	})
	if err != nil {
		slog.Error("Failed to initialize PostHog client", "error", err)
		return nil
	}

	// Try to load user identity from auth
	userEmail := getUserEmail()

	slog.Debug("PostHog client initialized", "userEmail", userEmail)

	return &PostHogClient{
		client:    client,
		userEmail: userEmail,
	}
}

// getUserEmail attempts to load user email from auth file
// Returns empty string if not logged in
func getUserEmail() string {
	authenticator, err := auth.NewAuthenticator()
	if err != nil {
		return ""
	}

	ctx := context.Background()
	err = authenticator.TryExistingAuth(ctx)
	if err != nil {
		return ""
	}

	return authenticator.Email
}

// getDistinctID returns the distinct ID for PostHog events
// Uses email if available, otherwise generates anonymous ID
func (p *PostHogClient) getDistinctID() string {
	if p.userEmail != "" {
		return p.userEmail
	}
	// For anonymous users, use a machine-specific identifier
	hostname, _ := os.Hostname()
	if hostname != "" {
		return "anonymous-" + hostname
	}
	return "anonymous-unknown"
}

// CaptureInstrumentationVersionMismatch sends a version mismatch event to PostHog
func (p *PostHogClient) CaptureInstrumentationVersionMismatch(
	moduleName string,
	requestedVersion string,
	supportedVersions []string,
) {
	if p == nil || p.client == nil {
		return
	}

	properties := posthog.NewProperties().
		Set("module_name", moduleName).
		Set("requested_version", requestedVersion).
		Set("supported_versions", supportedVersions).
		Set("user_email", p.userEmail)

	err := p.client.Enqueue(posthog.Capture{
		DistinctId: p.getDistinctID(),
		Event:      "instrumentation_version_mismatch",
		Properties: properties,
	})

	if err != nil {
		slog.Error("Failed to send version mismatch event to PostHog", "error", err)
	} else {
		slog.Debug("Sent version mismatch event to PostHog", "module", moduleName)
	}
}

// CaptureUnpatchedDependency sends an unpatched dependency event to PostHog
func (p *PostHogClient) CaptureUnpatchedDependency(
	traceTestServerSpanId string,
	stackTrace string,
) {
	if p == nil || p.client == nil {
		return
	}

	properties := posthog.NewProperties().
		Set("trace_test_server_span_id", traceTestServerSpanId).
		Set("stack_trace", stackTrace).
		Set("user_email", p.userEmail)

	err := p.client.Enqueue(posthog.Capture{
		DistinctId: p.getDistinctID(),
		Event:      "unpatched_dependency",
		Properties: properties,
	})

	if err != nil {
		slog.Error("Failed to send unpatched dependency event to PostHog", "error", err)
	} else {
		slog.Debug("Sent unpatched dependency event to PostHog")
	}
}

// Close flushes and closes the PostHog client
func (p *PostHogClient) Close() error {
	if p == nil || p.client == nil {
		return nil
	}
	return p.client.Close()
}
