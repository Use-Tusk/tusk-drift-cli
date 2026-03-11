package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var unitCmd = &cobra.Command{
	Use:   "unit",
	Short: "Commands for Tusk unit test workflows",
	Long:  "Retrieve Tusk unit test generation runs and scenarios for local review and agent workflows.",
}

func init() {
	rootCmd.AddCommand(unitCmd)
}

func printJSON(value any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}
