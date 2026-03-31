package cmd

import (
	"context"

	"github.com/Use-Tusk/tusk-cli/internal/driftquery"
	"github.com/spf13/cobra"
)

var (
	queryDistinctServiceID    string
	queryDistinctField        string
	queryDistinctLimit        int
	queryDistinctWhere        string
	queryDistinctJsonbFilters string
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

		input := &driftquery.ListDistinctValuesInput{
			ObservableServiceID: serviceID,
			Field:               queryDistinctField,
			Limit:               queryDistinctLimit,
		}

		if queryDistinctWhere != "" {
			where, err := parseWhereJSON(queryDistinctWhere)
			if err != nil {
				return err
			}
			input.Where = where
		}

		if queryDistinctJsonbFilters != "" {
			filters, err := parseJsonbFiltersJSON(queryDistinctJsonbFilters)
			if err != nil {
				return err
			}
			input.JsonbFilters = filters
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
	f.StringVar(&queryDistinctWhere, "where", "", "Filter conditions as JSON")
	f.StringVar(&queryDistinctJsonbFilters, "jsonb-filters", "", "JSONB filters as JSON array")

	_ = driftQueryDistinctCmd.MarkFlagRequired("field")
}
