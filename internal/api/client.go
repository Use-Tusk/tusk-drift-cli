package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
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

const TestRunServiceAPIPath = "/api/drift/test_run_service"

func NewClient(baseURL, apiKey string) *TuskClient {
	// https://app.usetusk.ai/api/vi/drift/test_run_service -> https://app.usetusk.ai
	u, _ := url.Parse(baseURL)
	host := u.Scheme + "://" + u.Host

	return &TuskClient{
		baseURL: host + TestRunServiceAPIPath,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Helper method to make protobuf requests
func (c *TuskClient) makeProtoRequest(ctx context.Context, endpoint string, req proto.Message, resp proto.Message, auth AuthOptions) error {
	fullURL := fmt.Sprintf("%s/%s", c.baseURL, endpoint)

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

	body, _ := io.ReadAll(httpResp.Body)
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return fmt.Errorf("http %d: %s", httpResp.StatusCode, string(body))
	}

	if err := proto.Unmarshal(body, resp); err != nil {
		return fmt.Errorf("decode proto: %w", err)
	}

	return nil
}

func (c *TuskClient) CreateDriftRun(ctx context.Context, in *backend.CreateDriftRunRequest, auth AuthOptions) (string, error) {
	var out backend.CreateDriftRunResponse
	if err := c.makeProtoRequest(ctx, "create_drift_run", in, &out, auth); err != nil {
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
	if err := c.makeProtoRequest(ctx, "get_global_spans", in, &out, auth); err != nil {
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
	if err := c.makeProtoRequest(ctx, "get_pre_app_start_spans", in, &out, auth); err != nil {
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
	if err := c.makeProtoRequest(ctx, "get_drift_run_trace_tests", in, &out, auth); err != nil {
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
	if err := c.makeProtoRequest(ctx, "get_all_trace_tests", in, &out, auth); err != nil {
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
	if err := c.makeProtoRequest(ctx, "get_trace_test", in, &out, auth); err != nil {
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
	if err := c.makeProtoRequest(ctx, "upload_trace_test_results", in, &out, auth); err != nil {
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
	if err := c.makeProtoRequest(ctx, "update_drift_run_ci_status", in, &out, auth); err != nil {
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

// Legacy methods - TODO: Remove these once we've migrated to protobuf everywhere
func (c *TuskClient) GetTests(serviceID string) ([]TestRecording, error) {
	return []TestRecording{
		{
			ID:        "api-test-1",
			ServiceID: serviceID,
			TraceID:   "trace-123",
			Method:    "GET",
			Path:      "/api/users",
			Timestamp: time.Now(),
		},
		{
			ID:        "api-test-2",
			ServiceID: serviceID,
			TraceID:   "trace-456",
			Method:    "POST",
			Path:      "/api/users",
			Timestamp: time.Now(),
		},
	}, nil
}

func (c *TuskClient) GetTest(testID string) (*TestRecording, error) {
	return &TestRecording{
		ID:        testID,
		ServiceID: "service-123",
		TraceID:   "trace-789",
		Method:    "GET",
		Path:      "/api/test",
		Timestamp: time.Now(),
	}, nil
}

func (c *TuskClient) UploadResults(results []TestResult) error {
	fmt.Printf("Uploading %d test results to Tusk API...\n", len(results))
	time.Sleep(500 * time.Millisecond)
	fmt.Println("Results uploaded successfully")
	return nil
}
