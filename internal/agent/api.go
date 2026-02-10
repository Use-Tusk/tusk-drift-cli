package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/version"
)

// APIMode represents how the client connects to the LLM
type APIMode string

const (
	// APIModeDirect connects directly to Anthropic API (BYOK)
	APIModeDirect APIMode = "direct"
	// APIModeProxy connects through Tusk backend proxy
	APIModeProxy APIMode = "proxy"
)

// ClaudeClientConfig holds configuration for creating a ClaudeClient
type ClaudeClientConfig struct {
	Mode        APIMode
	APIKey      string // For direct mode
	BearerToken string // For proxy mode
	Model       string
	BaseURL     string // Custom base URL (for proxy mode)
}

// llmRetryConfig controls HTTP-level retry behaviour for LLM API calls.
type llmRetryConfig struct {
	MaxRetries  int // number of retries (total attempts = MaxRetries + 1)
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
	JitterMin   float64 // lower multiplier bound (inclusive)
	JitterMax   float64 // upper multiplier bound (exclusive)
}

func defaultLLMRetryConfig() llmRetryConfig {
	return llmRetryConfig{
		MaxRetries:  3,
		BaseBackoff: 2 * time.Second,
		MaxBackoff:  15 * time.Second,
		JitterMin:   1.0,
		JitterMax:   2.0,
	}
}

func fastLLMRetryConfig() llmRetryConfig {
	return llmRetryConfig{
		MaxRetries:  3,
		BaseBackoff: 10 * time.Millisecond,
		MaxBackoff:  50 * time.Millisecond,
		JitterMin:   1.0,
		JitterMax:   1.0, // no jitter in tests
	}
}

// ClaudeClient handles communication with the Claude API
type ClaudeClient struct {
	mode        APIMode
	apiKey      string // For direct mode
	bearerToken string // For proxy mode
	model       string
	httpClient  *http.Client
	baseURL     string
	sessionID   string
	retryConfig llmRetryConfig
}

// SetSessionID sets the session ID for request correlation
func (c *ClaudeClient) SetSessionID(sessionID string) {
	c.sessionID = sessionID
}

// NewClaudeClient creates a new Claude API client (legacy constructor for BYOK)
func NewClaudeClient(apiKey, model string) (*ClaudeClient, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	if model == "" {
		model = "claude-sonnet-4-5-20250929"
	}
	return &ClaudeClient{
		mode:   APIModeDirect,
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: 10 * time.Minute, // Long timeout for complex tool use
		},
		baseURL:     "https://api.anthropic.com/v1",
		retryConfig: defaultLLMRetryConfig(),
	}, nil
}

// NewClaudeClientWithConfig creates a new Claude API client with the given configuration
func NewClaudeClientWithConfig(cfg ClaudeClientConfig) (*ClaudeClient, error) {
	if cfg.Mode == "" {
		cfg.Mode = APIModeDirect
	}

	switch cfg.Mode {
	case APIModeDirect:
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("API key is required for direct mode")
		}
		if cfg.BaseURL == "" {
			cfg.BaseURL = "https://api.anthropic.com/v1"
		}
	case APIModeProxy:
		if cfg.BearerToken == "" {
			return nil, fmt.Errorf("bearer token is required for proxy mode")
		}
		if cfg.BaseURL == "" {
			return nil, fmt.Errorf("base URL is required for proxy mode")
		}
	default:
		return nil, fmt.Errorf("unsupported API mode: %s", cfg.Mode)
	}

	if cfg.Model == "" {
		cfg.Model = "claude-sonnet-4-5-20250929"
	}

	return &ClaudeClient{
		mode:        cfg.Mode,
		apiKey:      cfg.APIKey,
		bearerToken: cfg.BearerToken,
		model:       cfg.Model,
		httpClient: &http.Client{
			Timeout: 10 * time.Minute,
		},
		baseURL:     cfg.BaseURL,
		retryConfig: defaultLLMRetryConfig(),
	}, nil
}

// getEndpoint returns the appropriate API endpoint URL based on the client mode
func (c *ClaudeClient) getEndpoint() string {
	if c.mode == APIModeProxy {
		return c.baseURL
	}
	return c.baseURL + "/messages"
}

// setAuthHeaders sets the appropriate authentication headers based on the client mode
func (c *ClaudeClient) setAuthHeaders(req *http.Request) {
	if c.mode == APIModeProxy {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
		req.Header.Set("x-tusk-cli-version", version.Version)
		if c.sessionID != "" {
			req.Header.Set("x-tusk-session-id", c.sessionID)
		}
	} else {
		req.Header.Set("x-api-key", c.apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	}
}

type createMessageRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []Message `json:"messages"`
	Tools     []Tool    `json:"tools,omitempty"`
	Stream    bool      `json:"stream,omitempty"`
}

// StreamCallback is called with streaming updates
type StreamCallback func(event StreamEvent)

// StreamEvent represents a streaming event
type StreamEvent struct {
	Type       string // "text", "tool_use_start", "tool_use_input", "done"
	Text       string
	ToolName   string
	ToolID     string
	ToolInput  string
	StopReason string
}

// isRetryableError returns true if the error represents a transient failure
// that should be retried (network errors, 429, 502, 503, 504).
func isRetryableError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	var apiErr *LLMAPIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusTooManyRequests,
			http.StatusInternalServerError,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout:
			return true
		}
	}

	return false
}

// llmRetryBackoff computes the backoff duration for a given retry attempt.
func llmRetryBackoff(attempt int, cfg llmRetryConfig) time.Duration {
	backoff := float64(cfg.BaseBackoff) * math.Pow(2, float64(attempt))
	jitter := cfg.JitterMin
	if cfg.JitterMax > cfg.JitterMin {
		jitter = cfg.JitterMin + rand.Float64()*(cfg.JitterMax-cfg.JitterMin)
	}
	backoff *= jitter
	if backoff > float64(cfg.MaxBackoff) {
		backoff = float64(cfg.MaxBackoff)
	}
	return time.Duration(backoff)
}

// doStreamingRequest performs a single HTTP request and returns either
// a successful *http.Response (status 200, body still open) or a typed error.
// The caller is responsible for closing the response body on success.
func (c *ClaudeClient) doStreamingRequest(ctx context.Context, bodyBytes []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		c.getEndpoint(),
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	c.setAuthHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err // transport error — caller checks retryability
	}

	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)

		llmErr := &LLMAPIError{
			StatusCode: resp.StatusCode,
			Message:    string(body),
		}

		// Try to parse structured error from JSON body.
		// Anthropic format: {"error": {"type": "...", "message": "..."}}
		var anthropicErr struct {
			Error APIError `json:"error"`
		}
		if json.Unmarshal(body, &anthropicErr) == nil && anthropicErr.Error.Message != "" {
			llmErr.APIErrorType = anthropicErr.Error.Type
			llmErr.Message = anthropicErr.Error.Message
		} else {
			// Proxy format: {"error": "string", "code": "string"}
			var proxyErr struct {
				Error string `json:"error"`
				Code  string `json:"code"`
			}
			if json.Unmarshal(body, &proxyErr) == nil && proxyErr.Error != "" {
				llmErr.APIErrorType = proxyErr.Code
				llmErr.Message = proxyErr.Error
			}
		}

		return nil, llmErr
	}

	return resp, nil
}

// CreateMessageStreaming sends a message to Claude and streams the response.
// Transient HTTP errors (network failures, 429, 5xx) are retried with exponential backoff.
// Mid-stream errors are NOT retried — they bubble up directly.
func (c *ClaudeClient) CreateMessageStreaming(
	ctx context.Context,
	system string,
	messages []Message,
	tools []Tool,
	callback StreamCallback,
) (*APIResponse, error) {
	reqBody := createMessageRequest{
		Model:     c.model,
		MaxTokens: 8192,
		System:    system,
		Messages:  messages,
		Tools:     tools,
		Stream:    true,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	cfg := c.retryConfig
	var lastErr error

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		// Backoff before retries (not before the first attempt)
		if attempt > 0 {
			backoff := llmRetryBackoff(attempt-1, cfg)
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}

		resp, err := c.doStreamingRequest(ctx, bodyBytes)
		if err != nil {
			lastErr = err
			if isRetryableError(err) && attempt < cfg.MaxRetries {
				continue
			}
			// Non-retryable or out of retries
			if attempt > 0 {
				return nil, &LLMRetryExhaustedError{Attempts: attempt + 1, Err: lastErr}
			}
			return nil, err
		}

		// Success — parse SSE stream (mid-stream errors are not retried).
		// Close body after parsing; not deferred inside the loop to avoid
		// accumulating deferred closers across retry iterations.
		apiResp, parseErr := c.parseStreamResponse(resp.Body, callback)
		_ = resp.Body.Close()
		return apiResp, parseErr
	}

	// Should not be reached, but handle just in case
	return nil, &LLMRetryExhaustedError{Attempts: cfg.MaxRetries + 1, Err: lastErr}
}

func (c *ClaudeClient) parseStreamResponse(body io.Reader, callback StreamCallback) (*APIResponse, error) {
	scanner := bufio.NewScanner(body)
	// Increase buffer size for large responses
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	response := &APIResponse{
		Content: []Content{},
	}

	var currentTextContent *Content
	var currentToolUse *Content
	var currentToolInput strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		// Parse SSE event
		if strings.HasPrefix(line, "event: ") {
			// Event type - we'll handle in the data
			continue
		}

		if after, ok := strings.CutPrefix(line, "data: "); ok {
			data := after

			var event map[string]any
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			eventType, _ := event["type"].(string)

			switch eventType {
			case "message_start":
				if msg, ok := event["message"].(map[string]any); ok {
					response.ID, _ = msg["id"].(string)
					response.Model, _ = msg["model"].(string)
					response.Role, _ = msg["role"].(string)
				}

			case "content_block_start":
				if block, ok := event["content_block"].(map[string]any); ok {
					blockType, _ := block["type"].(string)
					switch blockType {
					case "text":
						currentTextContent = &Content{Type: "text"}
					case "tool_use":
						toolID, _ := block["id"].(string)
						toolName, _ := block["name"].(string)
						currentToolUse = &Content{
							Type: "tool_use",
							ID:   toolID,
							Name: toolName,
						}
						currentToolInput.Reset()
						if callback != nil {
							callback(StreamEvent{
								Type:     "tool_use_start",
								ToolName: toolName,
								ToolID:   toolID,
							})
						}
					}
				}

			case "content_block_delta":
				if delta, ok := event["delta"].(map[string]any); ok {
					deltaType, _ := delta["type"].(string)
					switch deltaType {
					case "text_delta":
						text, _ := delta["text"].(string)
						if currentTextContent != nil {
							currentTextContent.Text += text
							if callback != nil {
								callback(StreamEvent{
									Type: "text",
									Text: text,
								})
							}
						}
					case "input_json_delta":
						partialJSON, _ := delta["partial_json"].(string)
						currentToolInput.WriteString(partialJSON)
						if callback != nil {
							callback(StreamEvent{
								Type:      "tool_use_input",
								ToolInput: partialJSON,
							})
						}
					}
				}

			case "content_block_stop":
				if currentTextContent != nil {
					response.Content = append(response.Content, *currentTextContent)
					currentTextContent = nil
				}
				if currentToolUse != nil {
					inputStr := currentToolInput.String()
					// Ensure we have valid JSON - empty input becomes {}
					if inputStr == "" || inputStr == "null" {
						inputStr = "{}"
					}
					currentToolUse.Input = json.RawMessage(inputStr)
					response.Content = append(response.Content, *currentToolUse)
					currentToolUse = nil
				}

			case "message_delta":
				if delta, ok := event["delta"].(map[string]any); ok {
					if stopReason, ok := delta["stop_reason"].(string); ok {
						response.StopReason = stopReason
						if callback != nil {
							callback(StreamEvent{
								Type:       "done",
								StopReason: stopReason,
							})
						}
					}
				}
				if usage, ok := event["usage"].(map[string]any); ok {
					if outputTokens, ok := usage["output_tokens"].(float64); ok {
						response.Usage.OutputTokens = int(outputTokens)
					}
				}

			case "message_stop":
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading stream: %w", err)
	}

	return response, nil
}

// CreateMessage sends a message to Claude and returns the response (non-streaming)
func (c *ClaudeClient) CreateMessage(
	ctx context.Context,
	system string,
	messages []Message,
	tools []Tool,
) (*APIResponse, error) {
	reqBody := createMessageRequest{
		Model:     c.model,
		MaxTokens: 8192,
		System:    system,
		Messages:  messages,
		Tools:     tools,
		Stream:    false,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		c.getEndpoint(),
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	c.setAuthHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var apiErr struct {
			Error APIError `json:"error"`
		}
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Error.Message != "" {
			return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, apiErr.Error.Message)
		}
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var apiResp APIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &apiResp, nil
}
