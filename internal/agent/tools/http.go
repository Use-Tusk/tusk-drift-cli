package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPTools provides HTTP request operations
type HTTPTools struct {
	client *http.Client
}

// NewHTTPTools creates a new HTTPTools instance
func NewHTTPTools() *HTTPTools {
	return &HTTPTools{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Request makes an HTTP request and returns the response
func (ht *HTTPTools) Request(input json.RawMessage) (string, error) {
	var params struct {
		Method  string            `json:"method"`
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
		Body    string            `json:"body"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	if params.Method == "" {
		params.Method = "GET"
	}

	var bodyReader io.Reader
	if params.Body != "" {
		bodyReader = strings.NewReader(params.Body)
	}

	req, err := http.NewRequest(params.Method, params.URL, bodyReader)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range params.Headers {
		req.Header.Set(k, v)
	}

	resp, err := ht.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var headerLines []string
	for k, v := range resp.Header {
		headerLines = append(headerLines, fmt.Sprintf("%s: %s", k, strings.Join(v, ", ")))
	}

	// Truncate body if too long
	bodyStr := string(body)
	if len(bodyStr) > 10000 {
		bodyStr = bodyStr[:10000] + "\n\n... (response truncated)"
	}

	return fmt.Sprintf("Status: %s\n\nHeaders:\n%s\n\nBody:\n%s",
		resp.Status,
		strings.Join(headerLines, "\n"),
		bodyStr,
	), nil
}
