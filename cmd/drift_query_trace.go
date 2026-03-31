package cmd

import (
	"context"

	"github.com/Use-Tusk/tusk-cli/internal/driftquery"
	"github.com/spf13/cobra"
)

var (
	queryTraceServiceID      string
	queryTraceIncludePayloads bool
	queryTraceMaxPayload     int
)

var driftQueryTraceCmd = &cobra.Command{
	Use:          "trace <trace-id>",
	Short:        "Get all spans in a trace as a hierarchical tree",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, authOptions, serviceID, err := setupDriftQueryCloud(queryTraceServiceID)
		if err != nil {
			return formatApiError(err)
		}

		input := &driftquery.GetTraceInput{
			ObservableServiceID: serviceID,
			TraceID:             args[0],
			IncludePayloads:     queryTraceIncludePayloads,
			MaxPayloadLength:    queryTraceMaxPayload,
		}

		result, err := client.GetDriftTrace(context.Background(), input, authOptions)
		if err != nil {
			return formatApiError(err)
		}

		return printJSON(result)
	},
}

func init() {
	driftQueryCmd.AddCommand(driftQueryTraceCmd)

	f := driftQueryTraceCmd.Flags()
	f.StringVar(&queryTraceServiceID, "service-id", "", "Observable service ID")
	f.BoolVar(&queryTraceIncludePayloads, "include-payloads", false, "Include inputValue/outputValue")
	f.IntVar(&queryTraceMaxPayload, "max-payload-length", 500, "Truncate payload strings to this length")
}
