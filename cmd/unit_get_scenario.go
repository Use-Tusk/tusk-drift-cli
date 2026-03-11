package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/Use-Tusk/tusk-drift-cli/internal/api"
	"github.com/spf13/cobra"
)

var (
	unitGetScenarioRunID      string
	unitGetScenarioScenarioID string
)

var unitGetScenarioCmd = &cobra.Command{
	Use:   "get-scenario",
	Short: "Get details for a unit test scenario",
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(unitGetScenarioRunID) == "" || strings.TrimSpace(unitGetScenarioScenarioID) == "" {
			return fmt.Errorf("--run-id and --scenario-id must be non-empty")
		}

		client, authOptions, _, err := api.SetupCloud(context.Background(), false)
		if err != nil {
			return err
		}

		result, err := client.GetUnitTestScenario(
			context.Background(),
			unitGetScenarioRunID,
			unitGetScenarioScenarioID,
			authOptions,
		)
		if err != nil {
			return err
		}

		return printJSON(result)
	},
}

func init() {
	unitCmd.AddCommand(unitGetScenarioCmd)

	unitGetScenarioCmd.Flags().StringVar(&unitGetScenarioRunID, "run-id", "", "Unit test run ID")
	unitGetScenarioCmd.Flags().StringVar(&unitGetScenarioScenarioID, "scenario-id", "", "Scenario ID")

	_ = unitGetScenarioCmd.MarkFlagRequired("run-id")
	_ = unitGetScenarioCmd.MarkFlagRequired("scenario-id")
}
