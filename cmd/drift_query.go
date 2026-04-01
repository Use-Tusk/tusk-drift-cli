package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Use-Tusk/tusk-cli/internal/api"
	"github.com/Use-Tusk/tusk-cli/internal/config"
	"github.com/Use-Tusk/tusk-cli/internal/driftquery"
	"github.com/spf13/cobra"
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
	w := &driftquery.SpanWhereClause{}
	empty := true

	if name != "" {
		w.Name = &driftquery.StringFilter{Eq: &name}
		empty = false
	}
	if packageName != "" {
		w.PackageName = &driftquery.StringFilter{Eq: &packageName}
		empty = false
	}
	if traceID != "" {
		w.TraceID = &driftquery.StringFilter{Eq: &traceID}
		empty = false
	}
	if environment != "" {
		w.Environment = &driftquery.StringFilter{Eq: &environment}
		empty = false
	}
	if minDuration > 0 {
		d := float64(minDuration)
		w.Duration = &driftquery.NumberFilter{Gte: &d}
		empty = false
	}
	if rootSpansOnly {
		w.IsRootSpan = &driftquery.BooleanFilter{Eq: true}
		empty = false
	}

	if empty {
		return nil
	}
	return w
}

// parseOrderBy parses "field:direction" into an OrderByField.
func parseOrderBy(s string) (*driftquery.OrderByField, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid --order-by format %q, expected field:direction (e.g. timestamp:DESC)", s)
	}
	field := parts[0]
	direction := strings.ToUpper(parts[1])
	if direction != "ASC" && direction != "DESC" {
		return nil, fmt.Errorf("invalid direction %q, expected ASC or DESC", parts[1])
	}
	return &driftquery.OrderByField{Field: field, Direction: direction}, nil
}

// parseWhereJSON parses a JSON string into a SpanWhereClause.
func parseWhereJSON(s string) (*driftquery.SpanWhereClause, error) {
	var where driftquery.SpanWhereClause
	if err := json.Unmarshal([]byte(s), &where); err != nil {
		return nil, fmt.Errorf("invalid --where JSON: %w", err)
	}
	return &where, nil
}

// parseJsonbFiltersJSON parses a JSON string into a slice of JsonbFilter.
func parseJsonbFiltersJSON(s string) ([]driftquery.JsonbFilter, error) {
	var filters []driftquery.JsonbFilter
	if err := json.Unmarshal([]byte(s), &filters); err != nil {
		return nil, fmt.Errorf("invalid --jsonb-filters JSON: %w", err)
	}
	return filters, nil
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
