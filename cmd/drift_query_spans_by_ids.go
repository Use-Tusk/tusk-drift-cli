package cmd

import (
	"context"
	"fmt"

	"github.com/Use-Tusk/tusk-cli/internal/driftquery"
	"github.com/spf13/cobra"
)

var (
	queryByIdsServiceID       string
	queryByIdsIDs             string
	queryByIdsIncludePayloads bool
	queryByIdsMaxPayload      int
)

var driftQuerySpansByIdsCmd = &cobra.Command{
	Use:          "spans-by-ids",
	Short:        "Fetch specific span recordings by their IDs",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, authOptions, serviceID, err := setupDriftQueryCloud(queryByIdsServiceID)
		if err != nil {
			return formatApiError(err)
		}

		ids := splitComma(queryByIdsIDs)
		if len(ids) == 0 {
			return fmt.Errorf("--ids is required (comma-separated span recording IDs)")
		}

		input := &driftquery.GetSpansByIdsInput{
			ObservableServiceID: serviceID,
			IDs:                 ids,
			IncludePayloads:     queryByIdsIncludePayloads,
			MaxPayloadLength:    queryByIdsMaxPayload,
		}

		result, err := client.GetDriftSpansByIds(context.Background(), input, authOptions)
		if err != nil {
			return formatApiError(err)
		}

		return printJSON(result)
	},
}

func init() {
	driftQueryCmd.AddCommand(driftQuerySpansByIdsCmd)

	f := driftQuerySpansByIdsCmd.Flags()
	f.StringVar(&queryByIdsServiceID, "service-id", "", "Observable service ID")
	f.StringVar(&queryByIdsIDs, "ids", "", "Span recording IDs, comma-separated (max 20)")
	f.BoolVar(&queryByIdsIncludePayloads, "include-payloads", true, "Include inputValue/outputValue")
	f.IntVar(&queryByIdsMaxPayload, "max-payload-length", 500, "Truncate payload strings to this length")

	_ = driftQuerySpansByIdsCmd.MarkFlagRequired("ids")
}
