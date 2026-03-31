package cmd

import (
	"context"

	"github.com/Use-Tusk/tusk-cli/internal/driftquery"
	"github.com/spf13/cobra"
)

var (
	querySchemaServiceID           string
	querySchemaName                string
	querySchemaPackageName         string
	querySchemaInstrumentationName string
	querySchemaShowExample         bool
	querySchemaMaxPayload          int
)

var driftQuerySchemaCmd = &cobra.Command{
	Use:          "schema",
	Short:        "Get schema information for span recordings",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, authOptions, serviceID, err := setupDriftQueryCloud(querySchemaServiceID)
		if err != nil {
			return formatApiError(err)
		}

		input := &driftquery.GetSchemaInput{
			ObservableServiceID: serviceID,
			ShowExample:         querySchemaShowExample,
			MaxPayloadLength:    querySchemaMaxPayload,
		}

		if querySchemaName != "" {
			input.Name = &querySchemaName
		}
		if querySchemaPackageName != "" {
			input.PackageName = &querySchemaPackageName
		}
		if querySchemaInstrumentationName != "" {
			input.InstrumentationName = &querySchemaInstrumentationName
		}

		result, err := client.GetDriftSchema(context.Background(), input, authOptions)
		if err != nil {
			return formatApiError(err)
		}

		return printJSON(result)
	},
}

func init() {
	driftQueryCmd.AddCommand(driftQuerySchemaCmd)

	f := driftQuerySchemaCmd.Flags()
	f.StringVar(&querySchemaServiceID, "service-id", "", "Observable service ID")
	f.StringVar(&querySchemaName, "name", "", "Span name to get schema for (e.g. /api/users)")
	f.StringVar(&querySchemaPackageName, "package-name", "", "Package name (e.g. http, pg, fetch)")
	f.StringVar(&querySchemaInstrumentationName, "instrumentation-name", "", "Instrumentation name")
	f.BoolVar(&querySchemaShowExample, "show-example", true, "Include an example span")
	f.IntVar(&querySchemaMaxPayload, "max-payload-length", 500, "Truncate example payload strings")
}
