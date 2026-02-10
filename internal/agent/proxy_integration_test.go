package agent

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/api"
	"github.com/Use-Tusk/tusk-drift-cli/internal/version"
)

// TestSystemPromptAcceptedByBackend verifies that the current system prompt
// is accepted by the Tusk backend proxy. This test ensures that when the CLI's
// system prompt changes, a corresponding update to the backend's PROMPT_VERSION_RANGES
// has been deployed.
//
// Required environment variables:
//   - TUSK_PROXY_TEST_API_KEY: Tusk API key for authentication (does not expire)
//
// Optional environment variables:
//   - TUSK_API_URL: Override the backend URL (defaults to https://api.usetusk.ai)
//
// This test uses validate-only mode (x-tusk-validate-only header) to avoid
// Anthropic API calls and associated costs. It only validates that the system
// prompt is accepted by the backend.
//
// This test is skipped if TUSK_PROXY_TEST_API_KEY is not set.
func TestSystemPromptAcceptedByBackend(t *testing.T) {
	apiKey := os.Getenv("TUSK_PROXY_TEST_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping integration test: TUSK_PROXY_TEST_API_KEY must be set")
	}

	baseURL := api.GetBaseURL()

	requestBody := createMessageRequest{
		Model:     "claude-sonnet-4-5-20250929",
		MaxTokens: 1,
		System:    SystemPrompt,
		Messages:  []Message{{Role: "user", Content: []Content{{Type: "text", Text: "test"}}}},
		Stream:    false,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	endpoint := baseURL + "/api/drift/setup-agent"
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("x-tusk-cli-version", version.Version)
	req.Header.Set("x-tusk-validate-only", "true")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	// Check for prompt mismatch error (403)
	if resp.StatusCode == http.StatusForbidden {
		var errorResp struct {
			Error string `json:"error"`
			Code  string `json:"code"`
		}
		if json.Unmarshal(respBody, &errorResp) == nil && errorResp.Code == "PROMPT_MISMATCH" {
			t.Fatalf(`
================================================================================
SYSTEM PROMPT MISMATCH DETECTED
================================================================================
The backend rejected the current system prompt. This means the CLI's system.md
has changed but the backend's PROMPT_VERSION_RANGES has not been updated.

To fix this:
1. Update PROMPT_VERSION_RANGES in tusk/backend/src/api/drift/config/setupAgentPrompts.ts
2. Deploy the backend changes before releasing this CLI version

Error: %s
Code: %s
CLI Version: %s
================================================================================
`, errorResp.Error, errorResp.Code, version.Version)
		}

		// Other 403 errors (e.g., auth issues)
		t.Fatalf("Request returned 403 Forbidden: %s", string(respBody))
	}

	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		t.Fatalf("Request returned client error %d: %s", resp.StatusCode, string(respBody))
	}

	if resp.StatusCode >= 500 {
		t.Fatalf("Request returned server error %d: %s", resp.StatusCode, string(respBody))
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Unexpected status code %d: %s", resp.StatusCode, string(respBody))
	}

	t.Logf("System prompt accepted by backend (status: %d, version: %s)", resp.StatusCode, version.Version)
}
