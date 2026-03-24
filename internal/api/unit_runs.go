package api

import (
	"bytes"
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
	return c.makeJSONRequestWithBody(ctx, method, path, query, nil, out, auth)
}

func (c *TuskClient) makeJSONRequestWithBody(ctx context.Context, method string, path string, query url.Values, payload any, out any, auth AuthOptions) error {
	fullURL := c.baseURL + path
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}

	var bodyReader *bytes.Reader
	if payload == nil {
		bodyReader = bytes.NewReader(nil)
	} else {
		body, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode json: %w", err)
		}
		bodyReader = bytes.NewReader(body)
	}

	httpReq, err := buildAuthenticatedRequest(ctx, method, fullURL, bodyReader, auth)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("X-Tusk-Source", "cli")
	if payload != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

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

type UnitTestFeedbackResult map[string]any

func (c *TuskClient) SubmitUnitTestFeedback(ctx context.Context, runID string, payload any, auth AuthOptions) (UnitTestFeedbackResult, error) {
	var out UnitTestFeedbackResult
	path := fmt.Sprintf("/api/v1/unit_test_run/%s/feedback", url.PathEscape(runID))
	if err := c.makeJSONRequestWithBody(ctx, http.MethodPost, path, nil, payload, &out, auth); err != nil {
		return nil, err
	}
	return out, nil
}

type UnitTestRetryResult map[string]any

func (c *TuskClient) RetryUnitTestRun(ctx context.Context, runID string, payload any, auth AuthOptions) (UnitTestRetryResult, error) {
	var out UnitTestRetryResult
	path := fmt.Sprintf("/api/v1/unit_test_run/%s/retry", url.PathEscape(runID))
	if err := c.makeJSONRequestWithBody(ctx, http.MethodPost, path, nil, payload, &out, auth); err != nil {
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
