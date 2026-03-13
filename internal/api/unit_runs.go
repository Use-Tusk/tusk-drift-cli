package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type (
	UnitTestRunSummary      map[string]any
	UnitTestRunDetails      map[string]any
	UnitTestScenarioDetails map[string]any
)

func (c *TuskClient) makeJSONRequest(ctx context.Context, method string, path string, query url.Values, out any, auth AuthOptions) error {
	fullURL := c.baseURL + path
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}

	httpReq, err := buildAuthenticatedRequest(ctx, method, fullURL, nil, auth)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("X-Tusk-Source", "cli")

	body, httpResp, err := c.executeRequest(httpReq)
	if err != nil {
		return err
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return newApiError(httpResp.StatusCode, body)
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

type UnitTestRunFiles map[string]any

func (c *TuskClient) GetUnitTestRunFiles(ctx context.Context, runID string, auth AuthOptions) (UnitTestRunFiles, error) {
	var out UnitTestRunFiles
	path := fmt.Sprintf("/api/v1/unit_test_run/%s/diffs", url.PathEscape(runID))
	if err := c.makeJSONRequest(ctx, http.MethodGet, path, nil, &out, auth); err != nil {
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
