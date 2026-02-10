package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
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

// UserEmail extracts the best email/username from an AuthInfoUser.
// Prefers CodeHostingUsername (GitHub/GitLab handle) over email.
func UserEmail(user *backend.UserAuthInfo) string {
	if user == nil {
		return ""
	}
	if user.CodeHostingUsername != nil {
		return *user.CodeHostingUsername
	}
	if user.Email != nil {
		return *user.Email
	}
	return ""
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

const (
	// DefaultBaseURL is the default Tusk Cloud API URL
	DefaultBaseURL = "https://api.usetusk.ai"

	TestRunServiceAPIPath    = "/api/drift/test_run_service"
	ClientServiceAPIPath     = "/api/drift/client_service"
	SpanExportServiceAPIPath = "/api/drift/tusk.drift.backend.v1.SpanExportService"
)

// GetBaseURL returns the API base URL with the following priority:
// 1. TUSK_API_URL environment variable
// 2. tusk_api.url from .tusk/config.yaml
// 3. Default URL (https://api.usetusk.ai)
func GetBaseURL() string {
	if envURL := os.Getenv("TUSK_API_URL"); envURL != "" {
		return envURL
	}

	if cfg, err := config.Get(); err == nil && cfg.TuskAPI.URL != "" {
		return cfg.TuskAPI.URL
	}

	return DefaultBaseURL
}

func NewClient(baseURL, apiKey string) *TuskClient {
	// https://app.usetusk.ai/api/vi/drift/test_run_service -> https://app.usetusk.ai
	u, _ := url.Parse(baseURL)
	host := u.Scheme + "://" + u.Host

	return &TuskClient{
		baseURL: host,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// Helper method to make protobuf requests
// If overrideBaseURL is provided, it's used instead of c.baseURL
func (c *TuskClient) makeProtoRequest(ctx context.Context, serviceAPIPath string, endpoint string, req proto.Message, resp proto.Message, auth AuthOptions) error {
	fullURL := fmt.Sprintf("%s/%s", serviceAPIPath, endpoint)

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

func (c *TuskClient) makeProtoRequestWithRetryConfig(ctx context.Context, serviceAPIPath string, endpoint string, req proto.Message, resp proto.Message, auth AuthOptions, config RetryConfig) error {
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

		err := c.makeProtoRequest(ctx, serviceAPIPath, endpoint, req, resp, auth)
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

func (c *TuskClient) makeTestRunServiceRequest(ctx context.Context, endpoint string, req proto.Message, resp proto.Message, auth AuthOptions, config RetryConfig) error {
	fullServiceAPIPath := c.baseURL + TestRunServiceAPIPath
	return c.makeProtoRequestWithRetryConfig(ctx, fullServiceAPIPath, endpoint, req, resp, auth, config)
}

func (c *TuskClient) makeClientServiceRequest(ctx context.Context, endpoint string, req proto.Message, resp proto.Message, auth AuthOptions) error {
	fullServiceAPIPath := c.baseURL + ClientServiceAPIPath

	// Client service requests are typically simple requests that don't need retries
	return c.makeProtoRequestWithRetryConfig(ctx, fullServiceAPIPath, endpoint, req, resp, auth, DefaultRetryConfig(0))
}

// NoSeatError is returned when the PR creator doesn't have a Tusk Cloud seat
type NoSeatError struct {
	Message string
}

func (e *NoSeatError) Error() string {
	return e.Message
}

// IsNoSeatError checks if an error is a NoSeatError
func IsNoSeatError(err error) bool {
	var noSeatErr *NoSeatError
	return errors.As(err, &noSeatErr)
}

// PausedByLabelError is returned when the PR has the "Tusk - Pause For Current PR" label
type PausedByLabelError struct {
	Message string
}

func (e *PausedByLabelError) Error() string {
	return e.Message
}

// IsPausedByLabelError checks if an error is a PausedByLabelError
func IsPausedByLabelError(err error) bool {
	var pausedErr *PausedByLabelError
	return errors.As(err, &pausedErr)
}

func (c *TuskClient) CreateDriftRun(ctx context.Context, in *backend.CreateDriftRunRequest, auth AuthOptions) (string, error) {
	var out backend.CreateDriftRunResponse
	if err := c.makeTestRunServiceRequest(ctx, "create_drift_run", in, &out, auth, DefaultRetryConfig(3)); err != nil {
		return "", err
	}

	if s := out.GetSuccess(); s != nil {
		return s.DriftRunId, nil
	}
	if e := out.GetError(); e != nil {
		if e.Code == "NO_SEAT" {
			return "", &NoSeatError{Message: e.Message}
		}
		if e.Code == "PAUSED_BY_LABEL" {
			return "", &PausedByLabelError{Message: e.Message}
		}
		return "", fmt.Errorf("%s: %s", e.Code, e.Message)
	}
	return "", fmt.Errorf("invalid response")
}

func (c *TuskClient) GetGlobalSpans(ctx context.Context, in *backend.GetGlobalSpansRequest, auth AuthOptions) (*backend.GetGlobalSpansResponseSuccess, error) {
	var out backend.GetGlobalSpansResponse
	if err := c.makeTestRunServiceRequest(ctx, "get_global_spans", in, &out, auth, DefaultRetryConfig(3)); err != nil {
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
	if err := c.makeTestRunServiceRequest(ctx, "get_pre_app_start_spans", in, &out, auth, DefaultRetryConfig(3)); err != nil {
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
	if err := c.makeTestRunServiceRequest(ctx, "get_drift_run_trace_tests", in, &out, auth, DefaultRetryConfig(3)); err != nil {
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
	if err := c.makeTestRunServiceRequest(ctx, "get_all_trace_tests", in, &out, auth, DefaultRetryConfig(3)); err != nil {
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
	if err := c.makeTestRunServiceRequest(ctx, "get_trace_test", in, &out, auth, DefaultRetryConfig(3)); err != nil {
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
	if err := c.makeTestRunServiceRequest(ctx, "upload_trace_test_results", in, &out, auth, DefaultRetryConfig(3)); err != nil {
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
	if err := c.makeTestRunServiceRequest(ctx, "update_drift_run_ci_status", in, &out, auth, DefaultRetryConfig(3)); err != nil {
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

	if err := c.makeClientServiceRequest(ctx, "get_auth_info", in, &out, auth); err != nil {
		return nil, err
	}

	return &out, nil
}

func (c *TuskClient) CreateObservableService(ctx context.Context, in *backend.CreateObservableServiceRequest, auth AuthOptions) (*backend.CreateObservableServiceResponse, error) {
	var out backend.CreateObservableServiceResponse
	if err := c.makeClientServiceRequest(ctx, "create_observable_service", in, &out, auth); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *TuskClient) VerifyRepoAccess(ctx context.Context, in *backend.VerifyRepoAccessRequest, auth AuthOptions) (*backend.VerifyRepoAccessResponse, error) {
	var out backend.VerifyRepoAccessResponse

	if err := c.makeClientServiceRequest(ctx, "verify_repo_access", in, &out, auth); err != nil {
		return nil, err
	}

	return &out, nil
}

func (c *TuskClient) CreateApiKey(ctx context.Context, in *backend.CreateApiKeyRequest, auth AuthOptions) (*backend.CreateApiKeyResponse, error) {
	var out backend.CreateApiKeyResponse

	if err := c.makeClientServiceRequest(ctx, "create_api_key", in, &out, auth); err != nil {
		return nil, err
	}

	return &out, nil
}

// GetObservableServiceInfo fetches observable service info including default branch
func (c *TuskClient) GetObservableServiceInfo(ctx context.Context, in *backend.GetObservableServiceInfoRequest, auth AuthOptions) (*backend.GetObservableServiceInfoResponseSuccess, error) {
	var out backend.GetObservableServiceInfoResponse
	if err := c.makeClientServiceRequest(ctx, "get_observable_service_info", in, &out, auth); err != nil {
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

// GetValidationTraceTests fetches traces for validation (both DRAFT and IN_SUITE)
func (c *TuskClient) GetValidationTraceTests(ctx context.Context, in *backend.GetValidationTraceTestsRequest, auth AuthOptions) (*backend.GetValidationTraceTestsResponseSuccess, error) {
	var out backend.GetValidationTraceTestsResponse
	if err := c.makeTestRunServiceRequest(ctx, "get_validation_trace_tests", in, &out, auth, DefaultRetryConfig(3)); err != nil {
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

// GetAllTraceTestIds fetches all trace test IDs for a service (lightweight, no pagination).
func (c *TuskClient) GetAllTraceTestIds(ctx context.Context, in *backend.GetAllTraceTestIdsRequest, auth AuthOptions) (*backend.GetAllTraceTestIdsResponseSuccess, error) {
	var out backend.GetAllTraceTestIdsResponse
	if err := c.makeTestRunServiceRequest(ctx, "get_all_trace_test_ids", in, &out, auth, DefaultRetryConfig(3)); err != nil {
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

// GetTraceTestsByIds fetches trace tests by their IDs (batch fetch).
func (c *TuskClient) GetTraceTestsByIds(ctx context.Context, in *backend.GetTraceTestsByIdsRequest, auth AuthOptions) (*backend.GetTraceTestsByIdsResponseSuccess, error) {
	var out backend.GetTraceTestsByIdsResponse
	if err := c.makeTestRunServiceRequest(ctx, "get_trace_tests_by_ids", in, &out, auth, DefaultRetryConfig(3)); err != nil {
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

// GetAllPreAppStartSpanIds fetches all pre-app-start span IDs for a service (lightweight, no pagination).
func (c *TuskClient) GetAllPreAppStartSpanIds(ctx context.Context, in *backend.GetAllPreAppStartSpanIdsRequest, auth AuthOptions) (*backend.GetAllPreAppStartSpanIdsResponseSuccess, error) {
	var out backend.GetAllPreAppStartSpanIdsResponse
	if err := c.makeTestRunServiceRequest(ctx, "get_all_pre_app_start_span_ids", in, &out, auth, DefaultRetryConfig(3)); err != nil {
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

// GetPreAppStartSpansByIds fetches pre-app-start spans by their IDs (batch fetch).
func (c *TuskClient) GetPreAppStartSpansByIds(ctx context.Context, in *backend.GetPreAppStartSpansByIdsRequest, auth AuthOptions) (*backend.GetPreAppStartSpansByIdsResponseSuccess, error) {
	var out backend.GetPreAppStartSpansByIdsResponse
	if err := c.makeTestRunServiceRequest(ctx, "get_pre_app_start_spans_by_ids", in, &out, auth, DefaultRetryConfig(3)); err != nil {
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

// GetAllGlobalSpanIds fetches all global span IDs for a service (lightweight, no pagination).
func (c *TuskClient) GetAllGlobalSpanIds(ctx context.Context, in *backend.GetAllGlobalSpanIdsRequest, auth AuthOptions) (*backend.GetAllGlobalSpanIdsResponseSuccess, error) {
	var out backend.GetAllGlobalSpanIdsResponse
	if err := c.makeTestRunServiceRequest(ctx, "get_all_global_span_ids", in, &out, auth, DefaultRetryConfig(3)); err != nil {
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

// GetGlobalSpansByIds fetches global spans by their IDs (batch fetch).
func (c *TuskClient) GetGlobalSpansByIds(ctx context.Context, in *backend.GetGlobalSpansByIdsRequest, auth AuthOptions) (*backend.GetGlobalSpansByIdsResponseSuccess, error) {
	var out backend.GetGlobalSpansByIdsResponse
	if err := c.makeTestRunServiceRequest(ctx, "get_global_spans_by_ids", in, &out, auth, DefaultRetryConfig(3)); err != nil {
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

// ExportSpans uploads spans to Tusk Cloud
func (c *TuskClient) ExportSpans(ctx context.Context, in *backend.ExportSpansRequest, auth AuthOptions) (*backend.ExportSpansResponse, error) {
	var out backend.ExportSpansResponse
	fullServiceAPIPath := c.baseURL + SpanExportServiceAPIPath
	if err := c.makeProtoRequestWithRetryConfig(ctx, fullServiceAPIPath, "ExportSpans", in, &out, auth, DefaultRetryConfig(3)); err != nil {
		return nil, err
	}
	return &out, nil
}
