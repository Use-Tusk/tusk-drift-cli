package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/Use-Tusk/tusk-cli/internal/driftquery"
	"github.com/spf13/cobra"
)

var (
	queryAggServiceID  string
	queryAggMetrics    string
	queryAggGroupBy    string
	queryAggTimeBucket string
	queryAggOrderBy    string
	queryAggLimit      int
	queryAggWhere      string
)

var driftQueryAggregateCmd = &cobra.Command{
	Use:          "aggregate",
	Short:        "Calculate aggregated metrics across spans",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, authOptions, serviceID, err := setupDriftQueryCloud(queryAggServiceID)
		if err != nil {
			return formatApiError(err)
		}

		metrics := splitComma(queryAggMetrics)
		if len(metrics) == 0 {
			return fmt.Errorf("--metrics is required (e.g. count,avgDuration,p95Duration)")
		}

		input := &driftquery.AggregateSpansInput{
			ObservableServiceID: serviceID,
			Metrics:             metrics,
			Limit:               queryAggLimit,
		}

		if queryAggGroupBy != "" {
			input.GroupBy = splitComma(queryAggGroupBy)
		}

		if queryAggTimeBucket != "" {
			input.TimeBucket = &queryAggTimeBucket
		}

		if queryAggOrderBy != "" {
			parts := strings.SplitN(queryAggOrderBy, ":", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid --order-by format %q, expected metric:direction (e.g. count:DESC)", queryAggOrderBy)
			}
			direction := strings.ToUpper(parts[1])
			if direction != "ASC" && direction != "DESC" {
				return fmt.Errorf("invalid direction %q, expected ASC or DESC", parts[1])
			}
			input.OrderBy = &driftquery.MetricOrderBy{Metric: parts[0], Direction: direction}
		}

		if queryAggWhere != "" {
			where, err := parseWhereJSON(queryAggWhere)
			if err != nil {
				return err
			}
			input.Where = where
		}

		result, err := client.AggregateDriftSpans(context.Background(), input, authOptions)
		if err != nil {
			return formatApiError(err)
		}

		return printJSON(result)
	},
}

func init() {
	driftQueryCmd.AddCommand(driftQueryAggregateCmd)

	f := driftQueryAggregateCmd.Flags()
	f.StringVar(&queryAggServiceID, "service-id", "", "Observable service ID")
	f.StringVar(&queryAggMetrics, "metrics", "", "Metrics to calculate, comma-separated (count, errorCount, errorRate, avgDuration, minDuration, maxDuration, p50Duration, p95Duration, p99Duration)")
	f.StringVar(&queryAggGroupBy, "group-by", "", "Fields to group by, comma-separated (name, packageName, instrumentationName, environment, statusCode)")
	f.StringVar(&queryAggTimeBucket, "time-bucket", "", "Time bucket for time-series (hour, day, week)")
	f.StringVar(&queryAggOrderBy, "order-by", "", "Order by metric (e.g. count:DESC)")
	f.IntVar(&queryAggLimit, "limit", 20, "Max results (1-100)")
	f.StringVar(&queryAggWhere, "where", "", "Filter conditions as JSON")

	_ = driftQueryAggregateCmd.MarkFlagRequired("metrics")
}
