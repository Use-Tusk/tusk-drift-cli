package cmd

import (
	"context"
	"fmt"

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

		metricNames := splitComma(queryAggMetrics)
		if len(metricNames) == 0 {
			return fmt.Errorf("--metrics is required (e.g. count,avgDuration,p95Duration)")
		}
		metrics, err := parseAggregateMetrics(metricNames)
		if err != nil {
			return err
		}
		limit, err := driftquery.Int32Ptr("--limit", queryAggLimit)
		if err != nil {
			return err
		}

		input := &driftquery.AggregateSpansInput{
			ObservableServiceId: serviceID,
			Metrics:             metrics,
			Limit:               limit,
		}

		if queryAggGroupBy != "" {
			groupBy, err := parseAggregateGroupFields(splitComma(queryAggGroupBy))
			if err != nil {
				return err
			}
			input.GroupBy = groupBy
		}

		if queryAggTimeBucket != "" {
			timeBucket, err := parseTimeBucket(queryAggTimeBucket)
			if err != nil {
				return err
			}
			input.TimeBucket = timeBucket
		}

		if queryAggOrderBy != "" {
			orderBy, err := parseMetricOrderBy(queryAggOrderBy)
			if err != nil {
				return err
			}
			input.OrderBy = orderBy
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
