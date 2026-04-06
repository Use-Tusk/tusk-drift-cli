package cmd

import (
	"context"

	"github.com/Use-Tusk/tusk-cli/internal/driftquery"
	"github.com/spf13/cobra"
)

var (
	queryDistinctServiceID string
	queryDistinctField     string
	queryDistinctLimit     int
	queryDistinctWhere     string
)

var driftQueryDistinctCmd = &cobra.Command{
	Use:          "distinct",
	Short:        "List unique values for a field, ordered by frequency",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, authOptions, serviceID, err := setupDriftQueryCloud(queryDistinctServiceID)
		if err != nil {
			return formatApiError(err)
		}
		limit, err := driftquery.Int32Ptr("--limit", queryDistinctLimit)
		if err != nil {
			return err
		}

		input := &driftquery.ListDistinctValuesInput{
			ObservableServiceId: serviceID,
			Field:               queryDistinctField,
			Limit:               limit,
		}

		if queryDistinctWhere != "" {
			where, err := parseWhereJSON(queryDistinctWhere)
			if err != nil {
				return err
			}
			input.Where = where
		}

		result, err := client.ListDriftDistinctValues(context.Background(), input, authOptions)
		if err != nil {
			return formatApiError(err)
		}

		return printJSON(result)
	},
}

func init() {
	driftQueryCmd.AddCommand(driftQueryDistinctCmd)

	f := driftQueryDistinctCmd.Flags()
	f.StringVar(&queryDistinctServiceID, "service-id", "", "Observable service ID")
	f.StringVar(&queryDistinctField, "field", "", "Field to get distinct values for (e.g. name, packageName, outputValue.statusCode)")
	f.IntVar(&queryDistinctLimit, "limit", 50, "Max distinct values to return (1-100)")
	f.StringVar(&queryDistinctWhere, "where", "", "WhereClause JSON to scope distinct values")

	_ = driftQueryDistinctCmd.MarkFlagRequired("field")
}
