package cmd

import (
	"context"

	"github.com/Use-Tusk/tusk-cli/internal/driftquery"
	"github.com/spf13/cobra"
)

var (
	querySpansServiceID       string
	querySpansName            string
	querySpansPackageName     string
	querySpansTraceID         string
	querySpansEnvironment     string
	querySpansMinDuration     int
	querySpansRootSpansOnly   bool
	querySpansLimit           int
	querySpansOffset          int
	querySpansIncludePayloads bool
	querySpansMaxPayload      int
	querySpansOrderBy         string
	querySpansWhere           string
	querySpansJsonbFilters    string
)

var driftQuerySpansCmd = &cobra.Command{
	Use:          "spans",
	Short:        "Search and filter span recordings",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, authOptions, serviceID, err := setupDriftQueryCloud(querySpansServiceID)
		if err != nil {
			return formatApiError(err)
		}

		input := &driftquery.QuerySpansInput{
			ObservableServiceID: serviceID,
			Limit:               querySpansLimit,
			Offset:              querySpansOffset,
			IncludeInputOutput:  querySpansIncludePayloads,
			MaxPayloadLength:    querySpansMaxPayload,
		}

		// Build where clause: --where JSON takes precedence over convenience flags
		if querySpansWhere != "" {
			where, err := parseWhereJSON(querySpansWhere)
			if err != nil {
				return err
			}
			input.Where = where
		} else {
			input.Where = buildWhereFromFlags(
				querySpansName, querySpansPackageName, querySpansTraceID,
				querySpansEnvironment, querySpansMinDuration, querySpansRootSpansOnly,
			)
		}

		if querySpansJsonbFilters != "" {
			filters, err := parseJsonbFiltersJSON(querySpansJsonbFilters)
			if err != nil {
				return err
			}
			input.JsonbFilters = filters
		}

		if querySpansOrderBy != "" {
			ob, err := parseOrderBy(querySpansOrderBy)
			if err != nil {
				return err
			}
			input.OrderBy = []driftquery.OrderByField{*ob}
		}

		result, err := client.QueryDriftSpans(context.Background(), input, authOptions)
		if err != nil {
			return formatApiError(err)
		}

		return printJSON(result)
	},
}

func init() {
	driftQueryCmd.AddCommand(driftQuerySpansCmd)

	f := driftQuerySpansCmd.Flags()
	f.StringVar(&querySpansServiceID, "service-id", "", "Observable service ID")
	f.StringVar(&querySpansName, "name", "", "Filter by span name (exact match)")
	f.StringVar(&querySpansPackageName, "package-name", "", "Filter by package (http, pg, fetch, grpc, etc.)")
	f.StringVar(&querySpansTraceID, "trace-id", "", "Filter by trace ID")
	f.StringVar(&querySpansEnvironment, "environment", "", "Filter by environment")
	f.IntVar(&querySpansMinDuration, "min-duration", 0, "Minimum duration in milliseconds")
	f.BoolVar(&querySpansRootSpansOnly, "root-spans-only", false, "Only return root spans")
	f.IntVar(&querySpansLimit, "limit", 20, "Max results to return (1-100)")
	f.IntVar(&querySpansOffset, "offset", 0, "Pagination offset")
	f.BoolVar(&querySpansIncludePayloads, "include-payloads", false, "Include full inputValue/outputValue")
	f.IntVar(&querySpansMaxPayload, "max-payload-length", 500, "Truncate payload strings to this length")
	f.StringVar(&querySpansOrderBy, "order-by", "", "Sort results (e.g. timestamp:DESC, duration:ASC)")
	f.StringVar(&querySpansWhere, "where", "", "Full SpanWhereClause as JSON (overrides convenience flags)")
	f.StringVar(&querySpansJsonbFilters, "jsonb-filters", "", "JSONB filters as JSON array")
}
