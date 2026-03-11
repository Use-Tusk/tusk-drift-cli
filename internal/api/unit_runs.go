package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type UnitTestRunSummary map[string]any
type UnitTestRunDetails map[string]any
type UnitTestScenarioDetails map[string]any

func (c *TuskClient) makeJSONRequest(ctx context.Context, method string, path string, query url.Values, out any, auth AuthOptions) error {
	fullURL := c.baseURL + path
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, fullURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Accept", "application/json")

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
		return fmt.Errorf("read json response body: %w", err)
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return fmt.Errorf("http %d: %s", httpResp.StatusCode, strings.TrimSpace(string(body)))
	}

	if out == nil || len(body) == 0 {
		return nil
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}

	return nil
}

func (c *TuskClient) GetLatestUnitTestRun(ctx context.Context, repo string, branch string, auth AuthOptions) (UnitTestRunSummary, error) {
	query := url.Values{}
	query.Set("repo", repo)
	query.Set("branch", branch)

	var out UnitTestRunSummary
	if err := c.makeJSONRequest(ctx, http.MethodGet, "/api/v1/unit_test_run/latest", query, &out, auth); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *TuskClient) GetUnitTestRun(ctx context.Context, runID string, auth AuthOptions) (UnitTestRunDetails, error) {
	var out UnitTestRunDetails
	if err := c.makeJSONRequest(ctx, http.MethodGet, "/api/v1/unit_test_run/"+url.PathEscape(runID), nil, &out, auth); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *TuskClient) GetUnitTestScenario(ctx context.Context, runID string, scenarioID string, auth AuthOptions) (UnitTestScenarioDetails, error) {
	var out UnitTestScenarioDetails
	path := fmt.Sprintf(
		"/api/v1/unit_test_run/%s/test_scenario/%s",
		url.PathEscape(runID),
		url.PathEscape(scenarioID),
	)
	if err := c.makeJSONRequest(ctx, http.MethodGet, path, nil, &out, auth); err != nil {
		return nil, err
	}
	return out, nil
}
