package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Use-Tusk/tusk-cli/internal/driftquery"
)

const DriftQueryAPIPath = "/api/drift/query"

func (c *TuskClient) QueryDriftSpans(ctx context.Context, input *driftquery.QuerySpansInput, auth AuthOptions) (json.RawMessage, error) {
	var out json.RawMessage
	if err := c.makeJSONRequestWithBody(ctx, http.MethodPost, DriftQueryAPIPath+"/spans", nil, input, &out, auth); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *TuskClient) GetDriftSchema(ctx context.Context, input *driftquery.GetSchemaInput, auth AuthOptions) (json.RawMessage, error) {
	var out json.RawMessage
	if err := c.makeJSONRequestWithBody(ctx, http.MethodPost, DriftQueryAPIPath+"/schema", nil, input, &out, auth); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *TuskClient) ListDriftDistinctValues(ctx context.Context, input *driftquery.ListDistinctValuesInput, auth AuthOptions) (json.RawMessage, error) {
	var out json.RawMessage
	if err := c.makeJSONRequestWithBody(ctx, http.MethodPost, DriftQueryAPIPath+"/distinct", nil, input, &out, auth); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *TuskClient) AggregateDriftSpans(ctx context.Context, input *driftquery.AggregateSpansInput, auth AuthOptions) (json.RawMessage, error) {
	var out json.RawMessage
	if err := c.makeJSONRequestWithBody(ctx, http.MethodPost, DriftQueryAPIPath+"/aggregate", nil, input, &out, auth); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *TuskClient) GetDriftTrace(ctx context.Context, input *driftquery.GetTraceInput, auth AuthOptions) (json.RawMessage, error) {
	var out json.RawMessage
	if err := c.makeJSONRequestWithBody(ctx, http.MethodPost, DriftQueryAPIPath+"/trace", nil, input, &out, auth); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *TuskClient) GetDriftSpansByIds(ctx context.Context, input *driftquery.GetSpansByIdsInput, auth AuthOptions) (json.RawMessage, error) {
	var out json.RawMessage
	if err := c.makeJSONRequestWithBody(ctx, http.MethodPost, DriftQueryAPIPath+"/spans-by-id", nil, input, &out, auth); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *TuskClient) ListDriftServices(ctx context.Context, auth AuthOptions) (json.RawMessage, error) {
	var out json.RawMessage
	if err := c.makeJSONRequest(ctx, http.MethodGet, DriftQueryAPIPath+"/services", nil, &out, auth); err != nil {
		return nil, err
	}
	return out, nil
}
