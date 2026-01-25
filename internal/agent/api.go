package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// ClaudeClient handles communication with the Claude API
type ClaudeClient struct {
	mode        APIMode
	apiKey      string // For direct mode
	bearerToken string // For proxy mode
	model       string
	httpClient  *http.Client
	baseURL     string
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
		baseURL: "https://api.anthropic.com/v1",
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
		baseURL: cfg.BaseURL,
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
		req.Header.Set("X-Tusk-CLI-Version", version.Version)
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

// CreateMessageStreaming sends a message to Claude and streams the response
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

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		var apiErr struct {
			Error APIError `json:"error"`
		}
		if json.Unmarshal(body, &apiErr) == nil && apiErr.Error.Message != "" {
			return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, apiErr.Error.Message)
		}
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	// Parse SSE stream
	return c.parseStreamResponse(resp.Body, callback)
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
