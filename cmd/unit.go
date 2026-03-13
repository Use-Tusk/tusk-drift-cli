package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Use-Tusk/tusk-drift-cli/internal/api"
	"github.com/spf13/cobra"
)

var unitCmd = &cobra.Command{
	Use:          "unit",
	Short:        "Commands for Tusk unit test workflows",
	Long:         "Retrieve Tusk unit test generation runs and scenarios for local review and agent workflows.",
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(unitCmd)
}

func printJSON(value any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}

func setupUnitCloud() (*api.TuskClient, api.AuthOptions, error) {
	client, authOptions, _, err := api.SetupCloud(context.Background(), false)
	if err != nil {
		return nil, api.AuthOptions{}, err
	}

	return client, authOptions, nil
}
