package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Use-Tusk/tusk-cli/internal/api"
	"github.com/Use-Tusk/tusk-cli/internal/config"
	"github.com/Use-Tusk/tusk-cli/internal/driftquery"
	queryv1 "github.com/Use-Tusk/tusk-drift-schemas/generated/go/query"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
)

var driftQueryCmd = &cobra.Command{
	Use:   "query",
	Short: "Query recorded API traffic and span data",
	Long:  "Query and analyze recorded API traffic spans from Tusk Drift Cloud.",
}

func init() {
	driftCmd.AddCommand(driftQueryCmd)
}

// setupDriftQueryCloud sets up the API client and resolves the service ID.
func setupDriftQueryCloud(serviceIDFlag string) (*api.TuskClient, api.AuthOptions, string, error) {
	client, authOptions, cfg, err := api.SetupCloud(context.Background(), false)
	if err != nil {
		return nil, api.AuthOptions{}, "", err
	}

	serviceID, err := resolveQueryServiceID(serviceIDFlag, cfg)
	if err != nil {
		return nil, api.AuthOptions{}, "", err
	}

	return client, authOptions, serviceID, nil
}

// resolveQueryServiceID resolves the service ID with priority:
// 1. --service-id flag
// 2. TUSK_DRIFT_SERVICE_ID env var
// 3. service.id from .tusk/config.yaml
func resolveQueryServiceID(flagValue string, cfg *config.Config) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if envID := os.Getenv("TUSK_DRIFT_SERVICE_ID"); envID != "" {
		return envID, nil
	}
	if cfg != nil && cfg.Service.ID != "" {
		return cfg.Service.ID, nil
	}
	return "", fmt.Errorf("no service ID found. Provide --service-id, set TUSK_DRIFT_SERVICE_ID, or ensure service.id is set in .tusk/config.yaml")
}

// buildWhereFromFlags constructs a SpanWhereClause from convenience flags.
// Returns nil if no flags were set.
func buildWhereFromFlags(name, packageName, traceID, environment string, minDuration int, rootSpansOnly bool) *driftquery.SpanWhereClause {
	fields := map[string]*driftquery.FieldPredicate{}

	if name != "" {
		fields["name"] = &driftquery.FieldPredicate{Eq: driftquery.StringValue(name)}
	}
	if packageName != "" {
		fields["packageName"] = &driftquery.FieldPredicate{Eq: driftquery.StringValue(packageName)}
	}
	if traceID != "" {
		fields["traceId"] = &driftquery.FieldPredicate{Eq: driftquery.StringValue(traceID)}
	}
	if environment != "" {
		fields["environment"] = &driftquery.FieldPredicate{Eq: driftquery.StringValue(environment)}
	}
	if minDuration > 0 {
		fields["duration"] = &driftquery.FieldPredicate{Gte: driftquery.NumberValue(float64(minDuration))}
	}
	if rootSpansOnly {
		fields["isRootSpan"] = &driftquery.FieldPredicate{Eq: driftquery.BoolValue(true)}
	}

	if len(fields) == 0 {
		return nil
	}
	return &driftquery.SpanWhereClause{Fields: fields}
}

// parseOrderBy parses "field:direction" into an OrderByField.
func parseOrderBy(s string) (*driftquery.OrderByField, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid --order-by format %q, expected field:direction (e.g. timestamp:DESC)", s)
	}
	field, ok := spanSortFieldByName[strings.TrimSpace(parts[0])]
	if !ok {
		return nil, fmt.Errorf("invalid field %q, expected one of: timestamp, createdAt, updatedAt, duration, name, traceId", parts[0])
	}
	direction, err := parseSortDirection(parts[1])
	if err != nil {
		return nil, err
	}
	return &driftquery.OrderByField{Field: field, Direction: direction}, nil
}

// parseWhereJSON parses a JSON string into a SpanWhereClause.
func parseWhereJSON(s string) (*driftquery.SpanWhereClause, error) {
	normalizedJSON, err := normalizeWhereJSONEnums([]byte(s))
	if err != nil {
		return nil, fmt.Errorf("invalid --where JSON: %w", err)
	}

	var where driftquery.SpanWhereClause
	if err := protojson.Unmarshal(normalizedJSON, &where); err != nil {
		return nil, fmt.Errorf("invalid --where JSON: %w", err)
	}
	return &where, nil
}

func normalizeWhereJSONEnums(input []byte) ([]byte, error) {
	var whereJSON any
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.UseNumber()
	if err := decoder.Decode(&whereJSON); err != nil {
		return nil, err
	}

	normalizeWhereClauseJSON(whereJSON)

	normalized, err := json.Marshal(whereJSON)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

func normalizeWhereClauseJSON(value any) {
	clause, ok := value.(map[string]any)
	if !ok {
		return
	}

	if fields, ok := clause["fields"].(map[string]any); ok {
		for _, predicate := range fields {
			normalizeFieldPredicateJSON(predicate)
		}
	}

	if andClauses, ok := clause["and"].([]any); ok {
		for _, nested := range andClauses {
			normalizeWhereClauseJSON(nested)
		}
	}

	if orClauses, ok := clause["or"].([]any); ok {
		for _, nested := range orClauses {
			normalizeWhereClauseJSON(nested)
		}
	}

	if notClause, ok := clause["not"]; ok {
		normalizeWhereClauseJSON(notClause)
	}
}

func normalizeFieldPredicateJSON(value any) {
	predicate, ok := value.(map[string]any)
	if !ok {
		return
	}

	access, ok := predicate["access"].(map[string]any)
	if !ok {
		return
	}

	if castAs, ok := access["castAs"].(string); ok {
		if enumValue, ok := castTypeByName[strings.ToLower(strings.TrimSpace(castAs))]; ok {
			access["castAs"] = enumValue
		}
	}

	if decode, ok := access["decode"].(string); ok {
		if enumValue, ok := decodeStrategyByName[strings.ToLower(strings.TrimSpace(decode))]; ok {
			access["decode"] = enumValue
		}
	}
}

func parseSortDirection(direction string) (driftquery.SortDirection, error) {
	switch strings.ToUpper(strings.TrimSpace(direction)) {
	case "ASC":
		return queryv1.SortDirection_SORT_DIRECTION_ASC, nil
	case "DESC":
		return queryv1.SortDirection_SORT_DIRECTION_DESC, nil
	default:
		return queryv1.SortDirection_SORT_DIRECTION_UNSPECIFIED, fmt.Errorf("invalid direction %q, expected ASC or DESC", direction)
	}
}

func parseAggregateMetric(name string) (driftquery.AggregateMetric, error) {
	metric, ok := aggregateMetricByName[strings.TrimSpace(name)]
	if !ok {
		return queryv1.AggregateMetric_AGGREGATE_METRIC_UNSPECIFIED, fmt.Errorf("invalid metric %q", name)
	}
	return metric, nil
}

func parseAggregateMetrics(names []string) ([]driftquery.AggregateMetric, error) {
	metrics := make([]driftquery.AggregateMetric, 0, len(names))
	for _, name := range names {
		metric, err := parseAggregateMetric(name)
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, metric)
	}
	return metrics, nil
}

func parseAggregateGroupFields(names []string) ([]driftquery.AggregateGroupField, error) {
	fields := make([]driftquery.AggregateGroupField, 0, len(names))
	for _, name := range names {
		field, ok := aggregateGroupFieldByName[strings.TrimSpace(name)]
		if !ok {
			return nil, fmt.Errorf("invalid group-by field %q", name)
		}
		fields = append(fields, field)
	}
	return fields, nil
}

func parseMetricOrderBy(s string) (*driftquery.MetricOrderBy, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid --order-by format %q, expected metric:direction (e.g. count:DESC)", s)
	}
	metric, err := parseAggregateMetric(parts[0])
	if err != nil {
		return nil, err
	}
	direction, err := parseSortDirection(parts[1])
	if err != nil {
		return nil, err
	}
	return &driftquery.MetricOrderBy{Metric: metric, Direction: direction}, nil
}

func parseTimeBucket(s string) (driftquery.TimeBucket, error) {
	bucket, ok := timeBucketByName[strings.TrimSpace(s)]
	if !ok {
		return queryv1.TimeBucket_TIME_BUCKET_UNSPECIFIED, fmt.Errorf("invalid time bucket %q, expected hour, day, or week", s)
	}
	return bucket, nil
}

func parseSelectableFields(s string) ([]driftquery.SelectableSpanField, error) {
	names := splitComma(s)
	fields := make([]driftquery.SelectableSpanField, 0, len(names))
	for _, name := range names {
		field, ok := selectableFieldByName[name]
		if !ok {
			return nil, fmt.Errorf("invalid field %q", name)
		}
		fields = append(fields, field)
	}
	return fields, nil
}

// splitComma splits a comma-separated string into trimmed non-empty parts.
func splitComma(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

var spanSortFieldByName = map[string]driftquery.SpanSortField{
	"timestamp": queryv1.SpanSortField_SPAN_SORT_FIELD_TIMESTAMP,
	"createdAt": queryv1.SpanSortField_SPAN_SORT_FIELD_CREATED_AT,
	"updatedAt": queryv1.SpanSortField_SPAN_SORT_FIELD_UPDATED_AT,
	"duration":  queryv1.SpanSortField_SPAN_SORT_FIELD_DURATION,
	"name":      queryv1.SpanSortField_SPAN_SORT_FIELD_NAME,
	"traceId":   queryv1.SpanSortField_SPAN_SORT_FIELD_TRACE_ID,
}

var aggregateMetricByName = map[string]driftquery.AggregateMetric{
	"count":       queryv1.AggregateMetric_AGGREGATE_METRIC_COUNT,
	"errorCount":  queryv1.AggregateMetric_AGGREGATE_METRIC_ERROR_COUNT,
	"errorRate":   queryv1.AggregateMetric_AGGREGATE_METRIC_ERROR_RATE,
	"avgDuration": queryv1.AggregateMetric_AGGREGATE_METRIC_AVG_DURATION,
	"minDuration": queryv1.AggregateMetric_AGGREGATE_METRIC_MIN_DURATION,
	"maxDuration": queryv1.AggregateMetric_AGGREGATE_METRIC_MAX_DURATION,
	"p50Duration": queryv1.AggregateMetric_AGGREGATE_METRIC_P50_DURATION,
	"p95Duration": queryv1.AggregateMetric_AGGREGATE_METRIC_P95_DURATION,
	"p99Duration": queryv1.AggregateMetric_AGGREGATE_METRIC_P99_DURATION,
}

var aggregateGroupFieldByName = map[string]driftquery.AggregateGroupField{
	"name":                queryv1.AggregateGroupField_AGGREGATE_GROUP_FIELD_NAME,
	"kind":                queryv1.AggregateGroupField_AGGREGATE_GROUP_FIELD_KIND,
	"packageName":         queryv1.AggregateGroupField_AGGREGATE_GROUP_FIELD_PACKAGE_NAME,
	"instrumentationName": queryv1.AggregateGroupField_AGGREGATE_GROUP_FIELD_INSTRUMENTATION_NAME,
	"environment":         queryv1.AggregateGroupField_AGGREGATE_GROUP_FIELD_ENVIRONMENT,
	"statusCode":          queryv1.AggregateGroupField_AGGREGATE_GROUP_FIELD_STATUS_CODE,
}

var timeBucketByName = map[string]driftquery.TimeBucket{
	"hour": queryv1.TimeBucket_TIME_BUCKET_HOUR,
	"day":  queryv1.TimeBucket_TIME_BUCKET_DAY,
	"week": queryv1.TimeBucket_TIME_BUCKET_WEEK,
}

var selectableFieldByName = map[string]driftquery.SelectableSpanField{
	"id":                  queryv1.SelectableSpanField_SELECTABLE_SPAN_FIELD_ID,
	"spanId":              queryv1.SelectableSpanField_SELECTABLE_SPAN_FIELD_SPAN_ID,
	"traceId":             queryv1.SelectableSpanField_SELECTABLE_SPAN_FIELD_TRACE_ID,
	"parentSpanId":        queryv1.SelectableSpanField_SELECTABLE_SPAN_FIELD_PARENT_SPAN_ID,
	"name":                queryv1.SelectableSpanField_SELECTABLE_SPAN_FIELD_NAME,
	"kind":                queryv1.SelectableSpanField_SELECTABLE_SPAN_FIELD_KIND,
	"status":              queryv1.SelectableSpanField_SELECTABLE_SPAN_FIELD_STATUS,
	"timestamp":           queryv1.SelectableSpanField_SELECTABLE_SPAN_FIELD_TIMESTAMP,
	"duration":            queryv1.SelectableSpanField_SELECTABLE_SPAN_FIELD_DURATION,
	"isRootSpan":          queryv1.SelectableSpanField_SELECTABLE_SPAN_FIELD_IS_ROOT_SPAN,
	"metadata":            queryv1.SelectableSpanField_SELECTABLE_SPAN_FIELD_METADATA,
	"packageName":         queryv1.SelectableSpanField_SELECTABLE_SPAN_FIELD_PACKAGE_NAME,
	"instrumentationName": queryv1.SelectableSpanField_SELECTABLE_SPAN_FIELD_INSTRUMENTATION_NAME,
	"inputValue":          queryv1.SelectableSpanField_SELECTABLE_SPAN_FIELD_INPUT_VALUE,
	"outputValue":         queryv1.SelectableSpanField_SELECTABLE_SPAN_FIELD_OUTPUT_VALUE,
	"inputSchema":         queryv1.SelectableSpanField_SELECTABLE_SPAN_FIELD_INPUT_SCHEMA,
	"outputSchema":        queryv1.SelectableSpanField_SELECTABLE_SPAN_FIELD_OUTPUT_SCHEMA,
	"environment":         queryv1.SelectableSpanField_SELECTABLE_SPAN_FIELD_ENVIRONMENT,
	"createdAt":           queryv1.SelectableSpanField_SELECTABLE_SPAN_FIELD_CREATED_AT,
	"updatedAt":           queryv1.SelectableSpanField_SELECTABLE_SPAN_FIELD_UPDATED_AT,
}

var castTypeByName = map[string]queryv1.CastType{
	"text":    queryv1.CastType_CAST_TYPE_TEXT,
	"int":     queryv1.CastType_CAST_TYPE_INT,
	"float":   queryv1.CastType_CAST_TYPE_FLOAT,
	"boolean": queryv1.CastType_CAST_TYPE_BOOLEAN,
}

var decodeStrategyByName = map[string]queryv1.DecodeStrategy{
	"base64": queryv1.DecodeStrategy_DECODE_STRATEGY_BASE64,
}
