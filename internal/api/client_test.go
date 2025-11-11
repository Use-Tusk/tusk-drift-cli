package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
)

func TestMakeProtoRequestWithRetry_SuccessFirstAttempt(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		w.Header().Set("Content-Type", "application/protobuf")

		resp := &backend.CreateDriftRunResponse{
			Response: &backend.CreateDriftRunResponse_Success{
				Success: &backend.CreateDriftRunResponseSuccess{
					DriftRunId: "test-run-id",
				},
			},
		}
		bin, _ := proto.Marshal(resp)
		_, _ = w.Write(bin)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")

	req := &backend.CreateDriftRunRequest{}
	resp := &backend.CreateDriftRunResponse{}
	auth := AuthOptions{APIKey: "test-key"}

	err := client.makeProtoRequestWithRetryConfig(context.Background(), server.URL, "test_endpoint", req, resp, auth, FastRetryConfig(3))

	assert.NoError(t, err)
	assert.Equal(t, 1, attemptCount, "Should succeed on first attempt")
}

func TestMakeProtoRequestWithRetry_RetryOn502(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 3 {
			// First 2 attempts: return 502
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("Bad Gateway"))
			return
		}
		// Third attempt: success
		w.Header().Set("Content-Type", "application/protobuf")
		resp := &backend.CreateDriftRunResponse{
			Response: &backend.CreateDriftRunResponse_Success{
				Success: &backend.CreateDriftRunResponseSuccess{
					DriftRunId: "test-run-id",
				},
			},
		}
		bin, _ := proto.Marshal(resp)
		_, _ = w.Write(bin)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")

	req := &backend.CreateDriftRunRequest{}
	resp := &backend.CreateDriftRunResponse{}
	auth := AuthOptions{APIKey: "test-key"}

	startTime := time.Now()
	err := client.makeProtoRequestWithRetryConfig(context.Background(), server.URL, "test_endpoint", req, resp, auth, FastRetryConfig(3))
	duration := time.Since(startTime)

	assert.NoError(t, err)
	assert.Equal(t, 3, attemptCount, "Should retry and succeed on 3rd attempt")
	// With fast config: 10ms + 20ms = ~30ms total (vs 2s+ in production)
	assert.Greater(t, duration, 20*time.Millisecond, "Should have backoff delays")
	assert.Less(t, duration, 200*time.Millisecond, "Should complete quickly in tests")
}

func TestMakeProtoRequestWithRetry_MaxRetriesExceeded(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("Bad Gateway"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")

	req := &backend.CreateDriftRunRequest{}
	resp := &backend.CreateDriftRunResponse{}
	auth := AuthOptions{APIKey: "test-key"}

	err := client.makeProtoRequestWithRetryConfig(context.Background(), server.URL, "test_endpoint", req, resp, auth, FastRetryConfig(3))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max retries exceeded")
	assert.Equal(t, 4, attemptCount, "Should try initial + 3 retries = 4 total")
}

func TestMakeProtoRequestWithRetry_NoRetryOn400(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Bad Request"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")

	req := &backend.CreateDriftRunRequest{}
	resp := &backend.CreateDriftRunResponse{}
	auth := AuthOptions{APIKey: "test-key"}

	err := client.makeProtoRequestWithRetryConfig(context.Background(), server.URL, "test_endpoint", req, resp, auth, FastRetryConfig(3))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "http 400")
	assert.Equal(t, 1, attemptCount, "Should not retry on 400")
}

func TestMakeProtoRequestWithRetry_ContextCancellation(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("Bad Gateway"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req := &backend.CreateDriftRunRequest{}
	resp := &backend.CreateDriftRunResponse{}
	auth := AuthOptions{APIKey: "test-key"}

	err := client.makeProtoRequestWithRetryConfig(ctx, server.URL, "test_endpoint", req, resp, auth, FastRetryConfig(10))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
	assert.Less(t, attemptCount, 10, "Should stop retrying on context cancellation")
}

func TestMakeProtoRequestWithRetry_503And504(t *testing.T) {
	testCases := []struct {
		name       string
		statusCode int
	}{
		{"503 Service Unavailable", http.StatusServiceUnavailable},
		{"504 Gateway Timeout", http.StatusGatewayTimeout},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			attemptCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attemptCount++
				if attemptCount < 2 {
					w.WriteHeader(tc.statusCode)
					_, _ = w.Write([]byte(tc.name))
					return
				}
				w.Header().Set("Content-Type", "application/protobuf")
				resp := &backend.CreateDriftRunResponse{
					Response: &backend.CreateDriftRunResponse_Success{
						Success: &backend.CreateDriftRunResponseSuccess{
							DriftRunId: "test-run-id",
						},
					},
				}
				bin, _ := proto.Marshal(resp)
				_, _ = w.Write(bin)
			}))
			defer server.Close()

			client := NewClient(server.URL, "test-key")

			req := &backend.CreateDriftRunRequest{}
			resp := &backend.CreateDriftRunResponse{}
			auth := AuthOptions{APIKey: "test-key"}

			err := client.makeProtoRequestWithRetryConfig(context.Background(), server.URL, "test_endpoint", req, resp, auth, FastRetryConfig(3))

			assert.NoError(t, err)
			assert.Equal(t, 2, attemptCount, "Should retry on "+tc.name)
		})
	}
}

func TestMakeProtoRequestWithRetry_BackoffCap(t *testing.T) {
	// Test that backoff is capped at MaxBackoff
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount <= 5 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("Bad Gateway"))
			return
		}
		w.Header().Set("Content-Type", "application/protobuf")
		resp := &backend.CreateDriftRunResponse{
			Response: &backend.CreateDriftRunResponse_Success{
				Success: &backend.CreateDriftRunResponseSuccess{
					DriftRunId: "test-run-id",
				},
			},
		}
		bin, _ := proto.Marshal(resp)
		_, _ = w.Write(bin)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")

	req := &backend.CreateDriftRunRequest{}
	resp := &backend.CreateDriftRunResponse{}
	auth := AuthOptions{APIKey: "test-key"}

	startTime := time.Now()
	err := client.makeProtoRequestWithRetryConfig(context.Background(), server.URL, "test_endpoint", req, resp, auth, FastRetryConfig(5))
	duration := time.Since(startTime)

	assert.NoError(t, err)
	// With fast config (50ms cap):
	// Attempt 1: 0ms
	// Attempt 2: 10ms * jitter = 10-20ms
	// Attempt 3: 20ms * jitter = 20-40ms
	// Attempt 4: 40ms * jitter = 40-80ms (capped at 50ms)
	// Attempt 5: 80ms * jitter (capped at 50ms)
	// Attempt 6: success
	// Total worst case: 20ms + 40ms + 50ms + 50ms = 160ms max
	assert.Less(t, duration, 300*time.Millisecond, "Should complete quickly with fast config")
	assert.Greater(t, duration, 50*time.Millisecond, "Should have some backoff delays")
}
