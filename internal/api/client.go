package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"time"

	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
	"google.golang.org/protobuf/proto"
)

type TuskClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

type AuthOptions struct {
	APIKey       string
	BearerToken  string
	TuskClientID string
}

type RetryConfig struct {
	MaxRetries  int
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
	JitterMin   float64
	JitterMax   float64
}

// DefaultRetryConfig returns normal retry configuration
func DefaultRetryConfig(maxRetries int) RetryConfig {
	return RetryConfig{
		MaxRetries:  maxRetries,
		BaseBackoff: 2 * time.Second,
		MaxBackoff:  15 * time.Second,
		JitterMin:   1.0,
		JitterMax:   2.0,
	}
}

// FastRetryConfig returns retry configuration for testing
func FastRetryConfig(maxRetries int) RetryConfig {
	return RetryConfig{
		MaxRetries:  maxRetries,
		BaseBackoff: 10 * time.Millisecond,
		MaxBackoff:  50 * time.Millisecond,
		JitterMin:   1.0,
		JitterMax:   2.0,
	}
}

const TestRunServiceAPIPath = "/api/drift/test_run_service"

func NewClient(baseURL, apiKey string) *TuskClient {
	// https://app.usetusk.ai/api/vi/drift/test_run_service -> https://app.usetusk.ai
	u, _ := url.Parse(baseURL)
	host := u.Scheme + "://" + u.Host

	return &TuskClient{
		baseURL: host + TestRunServiceAPIPath,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// Helper method to make protobuf requests
// If overrideBaseURL is provided, it's used instead of c.baseURL
func (c *TuskClient) makeProtoRequest(ctx context.Context, endpoint string, req proto.Message, resp proto.Message, auth AuthOptions, overrideBaseURL ...string) error {
	baseURL := c.baseURL
	if len(overrideBaseURL) > 0 && overrideBaseURL[0] != "" {
		baseURL = overrideBaseURL[0]
	}
	fullURL := fmt.Sprintf("%s/%s", baseURL, endpoint)

	bin, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal proto: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(bin))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/protobuf")
	httpReq.Header.Set("Accept", "application/protobuf")

	// Prefer API key if provided, otherwise JWT bearer
	switch {
	case auth.APIKey != "":
		httpReq.Header.Set("x-api-key", auth.APIKey)
	case auth.BearerToken != "":
		httpReq.Header.Set("Authorization", "Bearer "+auth.BearerToken)
	default:
		return fmt.Errorf("no auth provided")
	}

	if auth.BearerToken != "" && auth.TuskClientID != "" {
		httpReq.Header.Set("selected-client-id", auth.TuskClientID)
	}

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("http error: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return fmt.Errorf("read proto response body: %w", err)
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return fmt.Errorf("http %d: %s", httpResp.StatusCode, string(body))
	}
	if err := proto.Unmarshal(body, resp); err != nil {
		ct := httpResp.Header.Get("Content-Type")
		first := string(body[:min(120, len(body))])
		return fmt.Errorf("decode proto: %w (status=%d content-type=%s first=%q...)", err, httpResp.StatusCode, ct, first)
	}

	return nil
}

func (c *TuskClient) makeProtoRequestWithRetryConfig(ctx context.Context, endpoint string, req proto.Message, resp proto.Message, auth AuthOptions, config RetryConfig, overrideBaseURL ...string) error {
	var lastErr error
	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			baseExpBackoff := config.BaseBackoff * time.Duration(math.Pow(2, float64(attempt-1)))
			jitterRange := config.JitterMax - config.JitterMin
			jitter := config.JitterMin + rand.Float64()*jitterRange // #nosec G404
			backoff := min(time.Duration(float64(baseExpBackoff)*jitter), config.MaxBackoff)

			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		err := c.makeProtoRequest(ctx, endpoint, req, resp, auth, overrideBaseURL...)
		if err == nil {
			return nil
		}

		// Check if error is retryable (502, 503, 504, network errors)
		if strings.Contains(err.Error(), "http 502") ||
			strings.Contains(err.Error(), "http 503") ||
			strings.Contains(err.Error(), "http 504") {
			lastErr = err
			continue
		}

		// Non-retryable error
		return err
	}
	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

func (c *TuskClient) CreateDriftRun(ctx context.Context, in *backend.CreateDriftRunRequest, auth AuthOptions) (string, error) {
	var out backend.CreateDriftRunResponse
	if err := c.makeProtoRequestWithRetryConfig(ctx, "create_drift_run", in, &out, auth, DefaultRetryConfig(3)); err != nil {
		return "", err
	}

	if s := out.GetSuccess(); s != nil {
		return s.DriftRunId, nil
	}
	if e := out.GetError(); e != nil {
		return "", fmt.Errorf("%s: %s", e.Code, e.Message)
	}
	return "", fmt.Errorf("invalid response")
}

func (c *TuskClient) GetGlobalSpans(ctx context.Context, in *backend.GetGlobalSpansRequest, auth AuthOptions) (*backend.GetGlobalSpansResponseSuccess, error) {
	var out backend.GetGlobalSpansResponse
	if err := c.makeProtoRequestWithRetryConfig(ctx, "get_global_spans", in, &out, auth, DefaultRetryConfig(3)); err != nil {
		return nil, err
	}

	if s := out.GetSuccess(); s != nil {
		return s, nil
	}
	if e := out.GetError(); e != nil {
		return nil, fmt.Errorf("%s: %s", e.Code, e.Message)
	}
	return nil, fmt.Errorf("invalid response")
}

func (c *TuskClient) GetPreAppStartSpans(ctx context.Context, in *backend.GetPreAppStartSpansRequest, auth AuthOptions) (*backend.GetPreAppStartSpansResponseSuccess, error) {
	var out backend.GetPreAppStartSpansResponse
	if err := c.makeProtoRequestWithRetryConfig(ctx, "get_pre_app_start_spans", in, &out, auth, DefaultRetryConfig(3)); err != nil {
		return nil, err
	}

	if s := out.GetSuccess(); s != nil {
		return s, nil
	}
	if e := out.GetError(); e != nil {
		return nil, fmt.Errorf("%s: %s", e.Code, e.Message)
	}
	return nil, fmt.Errorf("invalid response")
}

func (c *TuskClient) GetDriftRunTraceTests(ctx context.Context, in *backend.GetDriftRunTraceTestsRequest, auth AuthOptions) (*backend.GetDriftRunTraceTestsResponseSuccess, error) {
	var out backend.GetDriftRunTraceTestsResponse
	if err := c.makeProtoRequestWithRetryConfig(ctx, "get_drift_run_trace_tests", in, &out, auth, DefaultRetryConfig(3)); err != nil {
		return nil, err
	}

	if s := out.GetSuccess(); s != nil {
		return s, nil
	}
	if e := out.GetError(); e != nil {
		return nil, fmt.Errorf("%s: %s", e.Code, e.Message)
	}
	return nil, fmt.Errorf("invalid response")
}

func (c *TuskClient) GetAllTraceTests(ctx context.Context, in *backend.GetAllTraceTestsRequest, auth AuthOptions) (*backend.GetAllTraceTestsResponseSuccess, error) {
	var out backend.GetAllTraceTestsResponse
	if err := c.makeProtoRequestWithRetryConfig(ctx, "get_all_trace_tests", in, &out, auth, DefaultRetryConfig(3)); err != nil {
		return nil, err
	}

	if s := out.GetSuccess(); s != nil {
		return s, nil
	}
	if e := out.GetError(); e != nil {
		return nil, fmt.Errorf("%s: %s", e.Code, e.Message)
	}
	return nil, fmt.Errorf("invalid response")
}

func (c *TuskClient) GetTraceTest(ctx context.Context, in *backend.GetTraceTestRequest, auth AuthOptions) (*backend.GetTraceTestResponseSuccess, error) {
	var out backend.GetTraceTestResponse
	if err := c.makeProtoRequestWithRetryConfig(ctx, "get_trace_test", in, &out, auth, DefaultRetryConfig(3)); err != nil {
		return nil, err
	}

	if s := out.GetSuccess(); s != nil {
		return s, nil
	}
	if e := out.GetError(); e != nil {
		if strings.Contains(e.Message, "EntityNotFoundError") {
			return nil, fmt.Errorf("Unable to find trace test with ID: %s", in.TraceTestId)
		}

		return nil, fmt.Errorf("%s: %s", e.Code, e.Message)
	}
	return nil, fmt.Errorf("invalid response")
}

func (c *TuskClient) UploadTraceTestResults(ctx context.Context, in *backend.UploadTraceTestResultsRequest, auth AuthOptions) error {
	var out backend.UploadTraceTestResultsResponse
	if err := c.makeProtoRequestWithRetryConfig(ctx, "upload_trace_test_results", in, &out, auth, DefaultRetryConfig(3)); err != nil {
		return err
	}

	if s := out.GetSuccess(); s != nil {
		return nil
	}
	if e := out.GetError(); e != nil {
		return fmt.Errorf("%s: %s", e.Code, e.Message)
	}
	return fmt.Errorf("invalid response")
}

func (c *TuskClient) UpdateDriftRunCIStatus(ctx context.Context, in *backend.UpdateDriftRunCIStatusRequest, auth AuthOptions) error {
	var out backend.UpdateDriftRunCIStatusResponse
	if err := c.makeProtoRequestWithRetryConfig(ctx, "update_drift_run_ci_status", in, &out, auth, DefaultRetryConfig(3)); err != nil {
		return err
	}

	if s := out.GetSuccess(); s != nil {
		return nil
	}
	if e := out.GetError(); e != nil {
		return fmt.Errorf("%s: %s", e.Code, e.Message)
	}
	return fmt.Errorf("invalid response")
}

func (c *TuskClient) GetAuthInfo(ctx context.Context, in *backend.GetAuthInfoRequest, auth AuthOptions) (*backend.GetAuthInfoResponse, error) {
	var out backend.GetAuthInfoResponse

	// Build the base URL for client_service instead of test_run_service
	baseURL := strings.TrimSuffix(c.baseURL, TestRunServiceAPIPath)
	clientServiceBaseURL := baseURL + "/api/drift/client_service"

	// Use the common makeProtoRequest helper with overridden base URL
	if err := c.makeProtoRequest(ctx, "get_auth_info", in, &out, auth, clientServiceBaseURL); err != nil {
		return nil, err
	}

	return &out, nil
}
