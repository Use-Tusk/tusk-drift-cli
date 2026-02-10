package agent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// sseSuccessBody returns a minimal valid SSE stream that produces a complete APIResponse.
func sseSuccessBody() string {
	return `event: message_start
data: {"type":"message_start","message":{"id":"msg_test","model":"claude-test","role":"assistant"}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}

event: message_stop
data: {"type":"message_stop"}

`
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

// newTestClient creates a ClaudeClient pointing at the given test server URL
// with fast retry config.
func newTestClient(url string) *ClaudeClient {
	return &ClaudeClient{
		mode:        APIModeProxy,
		model:       "claude-test",
		baseURL:     url,
		bearerToken: "test-token",
		httpClient:  &http.Client{Timeout: 5 * time.Second},
		retryConfig: fastLLMRetryConfig(),
	}
}

func TestCreateMessageStreaming_SuccessFirstAttempt(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, sseSuccessBody())
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	resp, err := client.CreateMessageStreaming(context.Background(), "system", nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "msg_test" {
		t.Errorf("expected ID msg_test, got %s", resp.ID)
	}
	if attempts.Load() != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts.Load())
	}
}

func TestCreateMessageStreaming_RetryOn502(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = fmt.Fprint(w, "bad gateway")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, sseSuccessBody())
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	resp, err := client.CreateMessageStreaming(context.Background(), "system", nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "msg_test" {
		t.Errorf("expected ID msg_test, got %s", resp.ID)
	}
	if attempts.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts.Load())
	}
}

func TestCreateMessageStreaming_RetryOn503(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprint(w, "service unavailable")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, sseSuccessBody())
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	resp, err := client.CreateMessageStreaming(context.Background(), "system", nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "msg_test" {
		t.Errorf("expected ID msg_test, got %s", resp.ID)
	}
	if attempts.Load() != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts.Load())
	}
}

func TestCreateMessageStreaming_RetryOn504(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= 1 {
			w.WriteHeader(http.StatusGatewayTimeout)
			_, _ = fmt.Fprint(w, "gateway timeout")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, sseSuccessBody())
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	resp, err := client.CreateMessageStreaming(context.Background(), "system", nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "msg_test" {
		t.Errorf("expected ID msg_test, got %s", resp.ID)
	}
	if attempts.Load() != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts.Load())
	}
}

func TestCreateMessageStreaming_RetryOn429(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = fmt.Fprint(w, `{"error":{"type":"rate_limit","message":"too many requests"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, sseSuccessBody())
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	resp, err := client.CreateMessageStreaming(context.Background(), "system", nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "msg_test" {
		t.Errorf("expected ID msg_test, got %s", resp.ID)
	}
	if attempts.Load() != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts.Load())
	}
}

func TestCreateMessageStreaming_RetryOn529(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= 1 {
			w.WriteHeader(529)
			_, _ = fmt.Fprint(w, `{"error":{"type":"overloaded","message":"overloaded"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, sseSuccessBody())
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	resp, err := client.CreateMessageStreaming(context.Background(), "system", nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "msg_test" {
		t.Errorf("expected ID msg_test, got %s", resp.ID)
	}
	if attempts.Load() != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts.Load())
	}
}

func TestCreateMessageStreaming_NoRetryOn400(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"error":{"type":"invalid_request_error","message":"bad request"}}`)
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	_, err := client.CreateMessageStreaming(context.Background(), "system", nil, nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *LLMAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *LLMAPIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected status 400, got %d", apiErr.StatusCode)
	}
	if attempts.Load() != 1 {
		t.Errorf("expected 1 attempt (no retry), got %d", attempts.Load())
	}
}

func TestCreateMessageStreaming_NoRetryOn401(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, "unauthorized")
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	_, err := client.CreateMessageStreaming(context.Background(), "system", nil, nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if attempts.Load() != 1 {
		t.Errorf("expected 1 attempt (no retry), got %d", attempts.Load())
	}
}

func TestCreateMessageStreaming_MaxRetriesExhausted(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = fmt.Fprint(w, "bad gateway")
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	_, err := client.CreateMessageStreaming(context.Background(), "system", nil, nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var retryErr *LLMRetryExhaustedError
	if !errors.As(err, &retryErr) {
		t.Fatalf("expected *LLMRetryExhaustedError, got %T: %v", err, err)
	}
	if retryErr.Attempts != 4 {
		t.Errorf("expected 4 attempts, got %d", retryErr.Attempts)
	}
	if attempts.Load() != 4 {
		t.Errorf("expected 4 server hits, got %d", attempts.Load())
	}
}

func TestCreateMessageStreaming_ContextCancellation(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = fmt.Fprint(w, "bad gateway")
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after a short delay to allow the first attempt
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	client := newTestClient(srv.URL)
	_, err := client.CreateMessageStreaming(ctx, "system", nil, nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Should get context cancelled, not exhaust all retries
	if attempts.Load() > 3 {
		t.Errorf("expected fewer than 4 attempts due to cancellation, got %d", attempts.Load())
	}
}

func TestCreateMessageStreaming_TransportErrorRetry(t *testing.T) {
	// Create a server, get its URL, then close it immediately to cause transport errors
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	client := newTestClient(url)
	_, err := client.CreateMessageStreaming(context.Background(), "system", nil, nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var retryErr *LLMRetryExhaustedError
	if !errors.As(err, &retryErr) {
		t.Fatalf("expected *LLMRetryExhaustedError for transport errors, got %T: %v", err, err)
	}
	if retryErr.Attempts != 4 {
		t.Errorf("expected 4 attempts, got %d", retryErr.Attempts)
	}
}

func TestCreateMessageStreaming_StructuredErrorParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"error":{"type":"invalid_request_error","message":"Field required: messages"}}`)
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	_, err := client.CreateMessageStreaming(context.Background(), "system", nil, nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *LLMAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *LLMAPIError, got %T: %v", err, err)
	}
	if apiErr.APIErrorType != "invalid_request_error" {
		t.Errorf("expected APIErrorType 'invalid_request_error', got %q", apiErr.APIErrorType)
	}
	if apiErr.Message != "Field required: messages" {
		t.Errorf("expected message 'Field required: messages', got %q", apiErr.Message)
	}
}

func TestCreateMessageStreaming_ProxyErrorFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, `{"error":"CLI version not supported. Please upgrade or use your own API key.","code":"VERSION_NOT_SUPPORTED"}`)
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	_, err := client.CreateMessageStreaming(context.Background(), "system", nil, nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *LLMAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *LLMAPIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 403 {
		t.Errorf("expected status 403, got %d", apiErr.StatusCode)
	}
	if apiErr.APIErrorType != "VERSION_NOT_SUPPORTED" {
		t.Errorf("expected APIErrorType 'VERSION_NOT_SUPPORTED', got %q", apiErr.APIErrorType)
	}
	if apiErr.Message != "CLI version not supported. Please upgrade or use your own API key." {
		t.Errorf("expected proxy error message, got %q", apiErr.Message)
	}
}

func TestCreateMessageStreaming_MidStreamErrorNotRetried(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Write partial SSE then close connection abruptly (no message_stop)
		_, _ = fmt.Fprint(w, `event: message_start
data: {"type":"message_start","message":{"id":"msg_partial","model":"claude-test","role":"assistant"}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}

`)
		// Flush and close — scanner will see EOF without message_stop
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	resp, err := client.CreateMessageStreaming(context.Background(), "system", nil, nil, nil)
	// Mid-stream truncation with clean EOF produces a partial response, not an error.
	// The important thing is that it's NOT retried.
	if attempts.Load() != 1 {
		t.Errorf("expected 1 attempt (no mid-stream retry), got %d", attempts.Load())
	}
	// We should get a response (partial) with no error since scanner sees clean EOF
	if err != nil {
		t.Logf("got error (acceptable for truncated stream): %v", err)
	} else if resp != nil && resp.ID != "msg_partial" {
		t.Errorf("expected partial response with ID msg_partial, got %s", resp.ID)
	}
}

// --- Unit tests for isRetryableError ---

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "generic error",
			err:      fmt.Errorf("something went wrong"),
			expected: false,
		},
		{
			name:     "LLMAPIError 400",
			err:      &LLMAPIError{StatusCode: 400, Message: "bad request"},
			expected: false,
		},
		{
			name:     "LLMAPIError 401",
			err:      &LLMAPIError{StatusCode: 401, Message: "unauthorized"},
			expected: false,
		},
		{
			name:     "LLMAPIError 429",
			err:      &LLMAPIError{StatusCode: 429, Message: "rate limited"},
			expected: true,
		},
		{
			name:     "LLMAPIError 502",
			err:      &LLMAPIError{StatusCode: 502, Message: "bad gateway"},
			expected: true,
		},
		{
			name:     "LLMAPIError 503",
			err:      &LLMAPIError{StatusCode: 503, Message: "service unavailable"},
			expected: true,
		},
		{
			name:     "LLMAPIError 504",
			err:      &LLMAPIError{StatusCode: 504, Message: "gateway timeout"},
			expected: true,
		},
		{
			name:     "LLMAPIError 500",
			err:      &LLMAPIError{StatusCode: 500, Message: "internal server error"},
			expected: true,
		},
		{
			name:     "LLMAPIError 529 (Anthropic overloaded)",
			err:      &LLMAPIError{StatusCode: 529, Message: "overloaded"},
			expected: true,
		},
		{
			name: "net.Error (timeout)",
			err: &net.OpError{
				Op:  "dial",
				Net: "tcp",
				Err: &net.DNSError{Err: "i/o timeout", Name: "api.usetusk.ai", IsTimeout: true},
			},
			expected: true,
		},
		{
			name: "wrapped net.Error",
			err: fmt.Errorf("request failed: %w", &net.OpError{
				Op:  "dial",
				Net: "tcp",
				Err: &net.DNSError{Err: "no such host", Name: "api.usetusk.ai"},
			}),
			expected: true,
		},
		{
			name:     "wrapped LLMAPIError 502",
			err:      fmt.Errorf("wrapped: %w", &LLMAPIError{StatusCode: 502, Message: "bad gateway"}),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryableError(tt.err)
			if got != tt.expected {
				t.Errorf("isRetryableError(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}

// --- Unit tests for isRecoverableAPIError ---

func TestIsRecoverableAPIError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "non-API error",
			err:      fmt.Errorf("some random error"),
			expected: false,
		},
		{
			name:     "LLMAPIError with Field required",
			err:      &LLMAPIError{StatusCode: 400, APIErrorType: "invalid_request_error", Message: "Field required: messages"},
			expected: true,
		},
		{
			name:     "LLMAPIError with invalid_request_error type",
			err:      &LLMAPIError{StatusCode: 400, APIErrorType: "invalid_request_error", Message: "something else"},
			expected: true,
		},
		{
			name:     "LLMAPIError with malformed",
			err:      &LLMAPIError{StatusCode: 400, Message: "malformed JSON input"},
			expected: true,
		},
		{
			name:     "LLMAPIError rate_limit (not recoverable at agent level)",
			err:      &LLMAPIError{StatusCode: 429, APIErrorType: "rate_limit", Message: "rate limited"},
			expected: false,
		},
		{
			name:     "LLMAPIError 502 (not recoverable at agent level)",
			err:      &LLMAPIError{StatusCode: 502, Message: "bad gateway"},
			expected: false,
		},
		{
			name:     "wrapped LLMAPIError",
			err:      fmt.Errorf("wrapped: %w", &LLMAPIError{StatusCode: 400, APIErrorType: "invalid_request_error", Message: "Field required"}),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRecoverableAPIError(tt.err)
			if got != tt.expected {
				t.Errorf("isRecoverableAPIError(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}

// --- Unit tests for UserFacingMessage ---

func TestUserFacingMessage(t *testing.T) {
	tests := []struct {
		name     string
		err      *LLMRetryExhaustedError
		contains string
	}{
		{
			name:     "rate limited (429)",
			err:      &LLMRetryExhaustedError{Attempts: 4, Err: &LLMAPIError{StatusCode: 429, Message: "rate limited"}},
			contains: "rate-limited",
		},
		{
			name:     "server error (502)",
			err:      &LLMRetryExhaustedError{Attempts: 4, Err: &LLMAPIError{StatusCode: 502, Message: "bad gateway"}},
			contains: "temporarily unavailable",
		},
		{
			name:     "server error (503)",
			err:      &LLMRetryExhaustedError{Attempts: 4, Err: &LLMAPIError{StatusCode: 503, Message: "service unavailable"}},
			contains: "temporarily unavailable",
		},
		{
			name: "network error",
			err: &LLMRetryExhaustedError{
				Attempts: 4,
				Err: &net.OpError{
					Op:  "dial",
					Net: "tcp",
					Err: &net.DNSError{Err: "i/o timeout", Name: "api.usetusk.ai"},
				},
			},
			contains: "network connection",
		},
		{
			name:     "generic error",
			err:      &LLMRetryExhaustedError{Attempts: 4, Err: fmt.Errorf("something unknown")},
			contains: "network connection",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.UserFacingMessage()
			if !strings.Contains(msg, tt.contains) {
				t.Errorf("UserFacingMessage() = %q, want it to contain %q", msg, tt.contains)
			}
		})
	}
}

// --- Unit test for backoff calculation ---

func TestLLMRetryBackoff(t *testing.T) {
	cfg := llmRetryConfig{
		MaxRetries:  3,
		BaseBackoff: 1 * time.Second,
		MaxBackoff:  10 * time.Second,
		JitterMin:   1.0,
		JitterMax:   1.0, // no jitter for deterministic testing
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 1 * time.Second},  // 1s * 2^0 = 1s
		{1, 2 * time.Second},  // 1s * 2^1 = 2s
		{2, 4 * time.Second},  // 1s * 2^2 = 4s
		{3, 8 * time.Second},  // 1s * 2^3 = 8s
		{4, 10 * time.Second}, // 1s * 2^4 = 16s, capped at 10s
		{5, 10 * time.Second}, // capped
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			got := llmRetryBackoff(tt.attempt, cfg)
			if got != tt.expected {
				t.Errorf("llmRetryBackoff(%d) = %v, want %v", tt.attempt, got, tt.expected)
			}
		})
	}
}

func TestLLMRetryBackoff_WithJitter(t *testing.T) {
	cfg := llmRetryConfig{
		MaxRetries:  3,
		BaseBackoff: 1 * time.Second,
		MaxBackoff:  30 * time.Second,
		JitterMin:   1.0,
		JitterMax:   2.0,
	}

	// With jitter between 1.0 and 2.0, attempt 0 should be between 1s and 2s
	for i := 0; i < 20; i++ {
		got := llmRetryBackoff(0, cfg)
		if got < 1*time.Second || got >= 2*time.Second {
			t.Errorf("llmRetryBackoff(0) = %v, want between 1s and 2s", got)
		}
	}
}
