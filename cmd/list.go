package cmd

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Use-Tusk/tusk-drift-cli/internal/api"
	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
	"github.com/Use-Tusk/tusk-drift-cli/internal/runner"
	"github.com/Use-Tusk/tusk-drift-cli/internal/tui"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
)

//go:embed short_docs/list.md
var listContent string

//go:embed short_docs/filter.md
var filterContent string

var listCmd = &cobra.Command{
	Use:          "list",
	Short:        "List available traces for replay",
	Long:         utils.RenderMarkdown(listContent + "\n\n" + filterContent),
	RunE:         listTests,
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(listCmd)

	listCmd.Flags().StringVar(&traceDir, "trace-dir", "", "Path to local folder containing recorded trace files")
	listCmd.Flags().StringVarP(&filter, "filter", "f", "", "Filter tests (see above help)")
	listCmd.Flags().BoolVarP(&cloud, "cloud", "c", false, "List trace tests from Tusk Drift Cloud")
}

func listTests(cmd *cobra.Command, args []string) error {
	_ = config.Load(cfgFile)

	executor := runner.NewExecutor()

	var tests []runner.Test
	var err error

	if cloud {
		client, authOptions, cfg, err := api.SetupCloud(context.Background(), true)
		if err != nil {
			return err
		}

		var (
			all []*backend.TraceTest
			cur string
		)
		for {
			req := &backend.GetAllTraceTestsRequest{
				ObservableServiceId: cfg.Service.ID,
				PageSize:            100,
			}
			if cur != "" {
				req.PaginationCursor = &cur
			}
			resp, err := client.GetAllTraceTests(context.Background(), req, authOptions)
			if err != nil {
				return fmt.Errorf("failed to fetch trace tests from backend: %w", err)
			}
			all = append(all, resp.TraceTests...)
			if next := resp.GetNextCursor(); next != "" {
				cur = next
				continue
			}
			break
		}
		tests = runner.ConvertTraceTestsToRunnerTests(all)
	} else {
		_ = config.Load("")
		cfg, _ := config.Get()

		selected := traceDir

		if selected == "" {
			selected = cfg.Traces.Dir
		}
		if selected != "" {
			utils.SetTracesDirOverride(selected)
		}

		tests, err = executor.LoadTestsFromFolder(selected)
		if err != nil {
			return fmt.Errorf("failed to load traces: %w", err)
		}
	}

	if len(tests) == 0 {
		if cloud {
			fmt.Println("No trace tests found in Tusk Drift Cloud for this service.")
			return nil
		}

		fmt.Println(`No traces found.

1. Install the Tusk Drift SDK in your service:
   Reference: https://docs.usetusk.ai/

2. Start your service in record mode:
   TUSK_DRIFT_MODE=RECORD <your-start-command>

3. Send requests to your service after startup. Traces will be recorded and saved to .tusk/traces.`)
		return nil
	}

	if filter != "" {
		if tests, err = runner.FilterTests(tests, filter); err != nil {
			return fmt.Errorf("invalid filter: %w", err)
		}
	}

	return tui.ShowTestListWithExecutor(tests, executor)
}
