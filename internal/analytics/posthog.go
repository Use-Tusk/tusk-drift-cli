package analytics

import (
	"context"
	"log/slog"
	"os"
	"sync"

	"github.com/Use-Tusk/tusk-drift-cli/internal/api"
	"github.com/Use-Tusk/tusk-drift-cli/internal/auth"
	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
	"github.com/posthog/posthog-go"
)

// PostHogClient wraps the PostHog client for analytics tracking
type PostHogClient struct {
	client          posthog.Client
	userEmail       string
	tuskClient      *api.TuskClient
	apiKey          string
	bearerToken     string
	clientID        string
	authInfo        *backend.GetAuthInfoResponse
	authInfoOnce    sync.Once
	enableTelemetry bool
}

// NewPostHogClient creates a new PostHog client
func NewPostHogClient(apiBaseURL, apiKey, bearerToken, clientID string, enableTelemetry bool) *PostHogClient {
	// #nosec G101 -- This is a public PostHog API key, safe to hardcode
	posthogAPIKey := "phc_mUFon9ykhVY9tga0zS6TPQ7FQloQNO91PQRtXdAREqz"

	client, err := posthog.NewWithConfig(posthogAPIKey, posthog.Config{
		Endpoint: "https://us.i.posthog.com",
	})
	if err != nil {
		slog.Error("Failed to initialize PostHog client", "error", err)
		return nil
	}

	// Try to load user identity from auth
	userEmail := getUserEmail()

	slog.Debug("PostHog client initialized", "userEmail", userEmail, "telemetryEnabled", enableTelemetry)

	var tuskClient *api.TuskClient
	if apiBaseURL != "" {
		tuskClient = api.NewClient(apiBaseURL, apiKey)
	}

	return &PostHogClient{
		client:          client,
		userEmail:       userEmail,
		tuskClient:      tuskClient,
		apiKey:          apiKey,
		bearerToken:     bearerToken,
		clientID:        clientID,
		enableTelemetry: enableTelemetry,
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

// fetchAuthInfo fetches auth info from the Tusk API using protobuf
// This is called lazily and cached to avoid repeated API calls
func (p *PostHogClient) fetchAuthInfo(ctx context.Context) {
	p.authInfoOnce.Do(func() {
		// If we don't have a Tusk client or credentials, skip
		if p.tuskClient == nil || (p.apiKey == "" && p.bearerToken == "") {
			slog.Debug("Skipping auth info fetch - missing credentials or client")
			return
		}

		// Create the protobuf request
		req := &backend.GetAuthInfoRequest{}

		// Build auth options
		authOpts := api.AuthOptions{
			APIKey:       p.apiKey,
			BearerToken:  p.bearerToken,
			TuskClientID: p.clientID,
		}

		// Call the TuskClient method
		resp, err := p.tuskClient.GetAuthInfo(ctx, req, authOpts)
		if err != nil {
			slog.Warn("Failed to fetch auth info", "error", err)
			return
		}

		p.authInfo = resp
		slog.Debug("Successfully fetched auth info",
			"userId", resp.User.GetId(),
			"clientCount", len(resp.Clients))
	})
}

// getDistinctID returns the distinct ID for PostHog events
// Uses user_id from auth info if available, otherwise falls back to email or anonymous ID
func (p *PostHogClient) getDistinctID() string {
	// First try to use user_id from auth info
	if p.authInfo != nil && p.authInfo.User != nil && p.authInfo.User.Id != "" {
		return p.authInfo.User.Id
	}

	// Fall back to email
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
	sdkVersion string,
) {
	if p == nil || p.client == nil {
		return
	}

	// Check if telemetry is enabled
	if !p.enableTelemetry {
		slog.Debug("Telemetry disabled, skipping version mismatch event")
		return
	}

	// Fetch auth info lazily (only once)
	ctx := context.Background()
	p.fetchAuthInfo(ctx)

	properties := posthog.NewProperties().
		Set("module_name", moduleName).
		Set("requested_version", requestedVersion).
		Set("supported_versions", supportedVersions).
		Set("sdk_version", sdkVersion)

	// Add auth info properties if available
	if p.authInfo != nil {
		if p.authInfo.User != nil {
			if p.authInfo.User.Id != "" {
				properties.Set("user_id", p.authInfo.User.Id)
			}
			if p.authInfo.User.Name != "" {
				properties.Set("user_name", p.authInfo.User.Name)
			}
		}

		if len(p.authInfo.Clients) > 0 {
			// Collect client IDs and names
			clientIDs := make([]string, 0, len(p.authInfo.Clients))
			clientNames := make([]string, 0, len(p.authInfo.Clients))
			for _, client := range p.authInfo.Clients {
				if client.Id != "" {
					clientIDs = append(clientIDs, client.Id)
				}
				if client.Name != nil && *client.Name != "" {
					clientNames = append(clientNames, *client.Name)
				}
			}
			if len(clientIDs) > 0 {
				properties.Set("client_ids", clientIDs)
			}
			if len(clientNames) > 0 {
				properties.Set("client_names", clientNames)
			}
		}
	}

	capture := posthog.Capture{
		DistinctId: p.getDistinctID(),
		Event:      "instrumentation_version_mismatch",
		Properties: properties,
	}

	// Add group identification if we have a client ID
	if p.authInfo != nil && len(p.authInfo.Clients) > 0 && p.authInfo.Clients[0].Id != "" {
		capture.Groups = posthog.NewGroups().
			Set("client", p.authInfo.Clients[0].Id)
	}

	err := p.client.Enqueue(capture)

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
	sdkVersion string,
) {
	if p == nil || p.client == nil {
		return
	}

	// Check if telemetry is enabled
	if !p.enableTelemetry {
		slog.Debug("Telemetry disabled, skipping unpatched dependency event")
		return
	}

	// Fetch auth info lazily (only once)
	ctx := context.Background()
	p.fetchAuthInfo(ctx)

	properties := posthog.NewProperties().
		Set("trace_test_server_span_id", traceTestServerSpanId).
		Set("stack_trace", stackTrace).
		Set("sdk_version", sdkVersion)

	// Add auth info properties if available
	if p.authInfo != nil {
		if p.authInfo.User != nil {
			if p.authInfo.User.Id != "" {
				properties.Set("user_id", p.authInfo.User.Id)
			}
			if p.authInfo.User.Name != "" {
				properties.Set("user_name", p.authInfo.User.Name)
			}
		}

		if len(p.authInfo.Clients) > 0 {
			// Collect client IDs and names
			clientIDs := make([]string, 0, len(p.authInfo.Clients))
			clientNames := make([]string, 0, len(p.authInfo.Clients))
			for _, client := range p.authInfo.Clients {
				if client.Id != "" {
					clientIDs = append(clientIDs, client.Id)
				}
				if client.Name != nil && *client.Name != "" {
					clientNames = append(clientNames, *client.Name)
				}
			}
			if len(clientIDs) > 0 {
				properties.Set("client_ids", clientIDs)
			}
			if len(clientNames) > 0 {
				properties.Set("client_names", clientNames)
			}
		}
	}

	capture := posthog.Capture{
		DistinctId: p.getDistinctID(),
		Event:      "unpatched_dependency",
		Properties: properties,
	}

	// Add group identification if we have a client ID
	if p.authInfo != nil && len(p.authInfo.Clients) > 0 && p.authInfo.Clients[0].Id != "" {
		capture.Groups = posthog.NewGroups().
			Set("client", p.authInfo.Clients[0].Id)
	}

	err := p.client.Enqueue(capture)

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
