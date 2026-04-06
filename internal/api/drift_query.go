package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Use-Tusk/tusk-cli/internal/driftquery"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const DriftQueryAPIPath = "/api/drift/query"

func (c *TuskClient) QueryDriftSpans(ctx context.Context, input *driftquery.QuerySpansInput, auth AuthOptions) (json.RawMessage, error) {
	var out json.RawMessage
	if err := c.makeProtoJSONRequestWithBody(ctx, http.MethodPost, DriftQueryAPIPath+"/spans", input, &out, auth); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *TuskClient) GetDriftSchema(ctx context.Context, input *driftquery.GetSchemaInput, auth AuthOptions) (json.RawMessage, error) {
	var out json.RawMessage
	if err := c.makeProtoJSONRequestWithBody(ctx, http.MethodPost, DriftQueryAPIPath+"/schema", input, &out, auth); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *TuskClient) ListDriftDistinctValues(ctx context.Context, input *driftquery.ListDistinctValuesInput, auth AuthOptions) (json.RawMessage, error) {
	var out json.RawMessage
	if err := c.makeProtoJSONRequestWithBody(ctx, http.MethodPost, DriftQueryAPIPath+"/distinct", input, &out, auth); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *TuskClient) AggregateDriftSpans(ctx context.Context, input *driftquery.AggregateSpansInput, auth AuthOptions) (json.RawMessage, error) {
	var out json.RawMessage
	if err := c.makeProtoJSONRequestWithBody(ctx, http.MethodPost, DriftQueryAPIPath+"/aggregate", input, &out, auth); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *TuskClient) GetDriftTrace(ctx context.Context, input *driftquery.GetTraceInput, auth AuthOptions) (json.RawMessage, error) {
	var out json.RawMessage
	if err := c.makeProtoJSONRequestWithBody(ctx, http.MethodPost, DriftQueryAPIPath+"/trace", input, &out, auth); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *TuskClient) GetDriftSpansByIds(ctx context.Context, input *driftquery.GetSpansByIdsInput, auth AuthOptions) (json.RawMessage, error) {
	var out json.RawMessage
	if err := c.makeProtoJSONRequestWithBody(ctx, http.MethodPost, DriftQueryAPIPath+"/spans-by-id", input, &out, auth); err != nil {
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

func (c *TuskClient) makeProtoJSONRequestWithBody(ctx context.Context, method string, path string, payload proto.Message, out any, auth AuthOptions) error {
	body, err := protojson.MarshalOptions{
		UseProtoNames:   false,
		UseEnumNumbers:  true,
		EmitUnpopulated: false,
	}.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode proto json: %w", err)
	}

	httpReq, err := buildAuthenticatedRequest(ctx, method, c.baseURL+path, bytes.NewReader(body), auth)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Tusk-Source", "cli")

	respBody, httpResp, err := c.executeRequest(httpReq)
	if err != nil {
		return err
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return newApiError(httpResp.StatusCode, respBody)
	}

	if out == nil || len(respBody) == 0 {
		return nil
	}

	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}

	return nil
}
