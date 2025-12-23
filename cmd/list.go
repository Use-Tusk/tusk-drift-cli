package cmd

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"time"

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

var listJSON bool

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
	listCmd.Flags().BoolVar(&enableServiceLogs, "enable-service-logs", false, "Send logs from your service to a file in .tusk/logs if you start a test. Logs from the SDK will be present.")
	listCmd.Flags().BoolVar(&listJSON, "json", false, "Output trace list as JSON (non-interactive)")

	listCmd.Flags().SortFlags = false
}

func listTests(cmd *cobra.Command, args []string) error {
	_ = config.Load(cfgFile)
	cfg, getConfigErr := config.Get()

	executor := runner.NewExecutor()
	executor.SetEnableServiceLogs(enableServiceLogs || debug)

	if getConfigErr == nil && cfg.TestExecution.Concurrency > 0 {
		executor.SetConcurrency(cfg.TestExecution.Concurrency)
	}
	if getConfigErr == nil && cfg.TestExecution.Timeout != "" {
		if d, err := time.ParseDuration(cfg.TestExecution.Timeout); err == nil {
			executor.SetTestTimeout(d)
		}
	}

	var tests []runner.Test
	var err error
	var client *api.TuskClient
	var authOptions api.AuthOptions

	if cloud {
		client, authOptions, cfg, err = api.SetupCloud(context.Background(), true)
		if err != nil {
			return err
		}

		tracker := utils.NewProgressTracker("Fetching traces from Tusk Drift Cloud", false, false)

		var (
			all      []*backend.TraceTest
			cur      string
			totalSet bool
		)

		for {
			req := &backend.GetAllTraceTestsRequest{
				ObservableServiceId: cfg.Service.ID,
				PageSize:            25,
			}
			if cur != "" {
				req.PaginationCursor = &cur
			}

			resp, err := client.GetAllTraceTests(context.Background(), req, authOptions)
			if err != nil {
				tracker.Stop()
				return fmt.Errorf("failed to fetch trace tests from backend: %w", err)
			}

			all = append(all, resp.TraceTests...)

			if !totalSet && resp.TotalCount > 0 {
				tracker.SetTotal(int(resp.TotalCount))
				totalSet = true
			}

			tracker.Update(len(all))

			if next := resp.GetNextCursor(); next != "" {
				cur = next
				continue
			}
			break
		}

		tracker.Finish("")
		tests = runner.ConvertTraceTestsToRunnerTests(all)
	} else {
		_ = config.Load("")
		cfg, getConfigErr := config.Get()

		selected := traceDir

		if selected == "" && getConfigErr == nil && cfg.Traces.Dir != "" {
			selected = cfg.Traces.Dir
		}

		// Default to standard traces directory if nothing specified
		if selected == "" {
			selected = utils.GetTracesDir()
		} else if traceDir != "" {
			// Resolve --trace-dir flag relative to tusk root if it's a relative path
			selected = utils.ResolveTuskPath(selected)
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
		// TODO: support filters over backend query so we don't need to fetch all tests first
		if tests, err = runner.FilterTests(tests, filter); err != nil {
			return fmt.Errorf("invalid filter: %w", err)
		}
	}

	if listJSON {
		return outputTestsAsJSON(tests)
	}

	suiteOpts := runner.SuiteSpanOptions{
		IsCloudMode: cloud,
		Client:      client,
		AuthOptions: authOptions,
		Interactive: true,
	}
	if cfg != nil {
		suiteOpts.ServiceID = cfg.Service.ID
	}

	return tui.ShowTestListWithExecutor(tests, executor, suiteOpts)
}

func outputTestsAsJSON(tests []runner.Test) error {
	type testOutput struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Type        string `json:"type,omitempty"`
		Method      string `json:"method,omitempty"`
		Path        string `json:"path,omitempty"`
		Status      string `json:"status,omitempty"`
		DurationMs  int    `json:"duration_ms,omitempty"`
		Environment string `json:"environment,omitempty"`
		FileName    string `json:"file_name,omitempty"`
	}

	output := struct {
		Count int          `json:"count"`
		Tests []testOutput `json:"tests"`
	}{
		Count: len(tests),
		Tests: make([]testOutput, 0, len(tests)),
	}

	for _, t := range tests {
		output.Tests = append(output.Tests, testOutput{
			ID:          t.TraceID,
			Type:        t.DisplayType,
			Name:        t.DisplayName,
			Method:      t.Method,
			Path:        t.Path,
			Status:      t.Status,
			DurationMs:  t.Duration,
			Environment: t.Environment,
			FileName:    t.FileName,
		})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}
