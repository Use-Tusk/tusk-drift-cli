package cmd

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/Use-Tusk/tusk-drift-cli/internal/api"
	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
	"github.com/Use-Tusk/tusk-drift-cli/internal/logging"
	"github.com/Use-Tusk/tusk-drift-cli/internal/runner"
	"github.com/Use-Tusk/tusk-drift-cli/internal/tui"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
	"github.com/Use-Tusk/tusk-drift-cli/internal/version"
	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
)

var (
	traceDir          string
	traceFile         string
	traceID           string
	print             bool
	outputFormat      string
	filter            string
	quiet             bool
	concurrency       int
	enableServiceLogs bool
	saveResults       bool
	resultsDir        string

	// Cloud mode
	cloud              bool
	ci                 bool
	allCloudTraceTests bool
	commitSha          string
	prNumber           string
	branchName         string
	externalCheckRunID string
	traceTestID        string
	clientID           string
)

//go:embed short_docs/run.md
var runContent string

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run API tests",
	Long:  utils.RenderMarkdown(runContent + "\n\n" + filterContent),
	RunE:  runTests,
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().StringVar(&traceDir, "trace-dir", "", "Path to local recordings folder")
	runCmd.Flags().StringVar(&traceFile, "trace-file", "", "Path to a single test file")
	runCmd.Flags().StringVar(&traceID, "trace-id", "", "Database ID of a single test")
	runCmd.Flags().BoolVarP(&print, "print", "p", false, "Print response and exit (useful for pipes)")
	runCmd.Flags().StringVar(&outputFormat, "output-format", "text", `Output format (only works with --print): "text" (default) or "json" (single result) (choices: "text", "json")"`)
	runCmd.Flags().StringVarP(&filter, "filter", "f", "", "Filter tests (see above help)")
	runCmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Quiet output, only show failures (only works with --print and --output-format text)")
	runCmd.Flags().IntVar(&concurrency, "concurrency", 5, "Maximum number of concurrent tests. If set, overrides the concurrency setting in the config file.")
	runCmd.Flags().BoolVar(&enableServiceLogs, "enable-service-logs", false, "Send logs from your service to a file in .tusk/logs. Logs from the SDK will be present.")
	runCmd.Flags().BoolVar(&saveResults, "save-results", false, "Save replay results to a file")
	runCmd.Flags().StringVar(&resultsDir, "results-dir", "", "Path to directory to save results (only works with --save-results). Default is '.tusk/results'")

	// Cloud mode
	runCmd.Flags().BoolVarP(&cloud, "cloud", "c", false, "[Cloud] Use Tusk Drift Cloud backend for orchestration/reporting")
	runCmd.Flags().BoolVar(&ci, "ci", false, "[Cloud] Create a Tusk Drift run and upload results to Tusk Drift Cloud")
	runCmd.Flags().BoolVarP(&allCloudTraceTests, "all-cloud-trace-tests", "a", false, "[Cloud] Run against all trace tests from Tusk Drift Cloud for this run (not just the current suite)")
	runCmd.Flags().StringVar(&commitSha, "commit-sha", "", "[Cloud] Commit SHA for this run (only works with --ci)")
	runCmd.Flags().StringVar(&prNumber, "pr-number", "", "[Cloud] Pull request number (only works with --ci)")
	runCmd.Flags().StringVar(&branchName, "branch", "", "[Cloud] Branch name for this run (only works with --ci)")
	runCmd.Flags().StringVar(&externalCheckRunID, "external-check-run-id", "", "[Cloud] External check run ID (only works with --ci)")
	runCmd.Flags().StringVar(&traceTestID, "trace-test-id", "", "[Cloud] Run against a single trace test")
	runCmd.Flags().StringVar(&clientID, "client-id", "", "[Cloud] Client ID for JWT auth (optional; ignored when using API key)") // Tusk client ID. Not used right now, but could be useful for auth

	_ = runCmd.Flags().MarkHidden("client-id")
	runCmd.Flags().SortFlags = false
}

func runTests(cmd *cobra.Command, args []string) error {
	setupSignalHandling()

	slog.Debug("Starting test execution",
		"trace-dir", traceDir,
		"trace-file", traceFile,
		"trace-id", traceID,
		"print", print,
		"output-format", outputFormat,
		"filter", filter,
		"quiet", quiet,
		"concurrency", concurrency,
		"enable-service-logs", enableServiceLogs,
		"save-results", saveResults,
		"results-dir", resultsDir,
		"cloud", cloud,
		"ci", ci,
		"commitSha", commitSha,
		"prNumber", prNumber,
		"branchName", branchName,
		"externalCheckRunID", externalCheckRunID,
		"clientID", clientID,
	)

	executor := runner.NewExecutor()

	_ = config.Load(cfgFile)
	cfg, getConfigErr := config.Get()
	if getConfigErr == nil && cfg.TestExecution.Concurrency > 0 {
		executor.SetConcurrency(cfg.TestExecution.Concurrency)
	}
	if getConfigErr == nil && cfg.TestExecution.Timeout != "" {
		if d, err := time.ParseDuration(cfg.TestExecution.Timeout); err == nil {
			executor.SetTestTimeout(d)
		} else {
			slog.Warn("Invalid test_execution.timeout; using default", "value", cfg.TestExecution.Timeout, "error", err)
		}
	}

	if traceDir != "" {
		utils.SetTracesDirOverride(traceDir)
	} else if getConfigErr == nil && cfg.Traces.Dir != "" {
		utils.SetTracesDirOverride(cfg.Traces.Dir)
	}

	interactive := !print && utils.IsTerminal()

	var driftRunID string
	var client *api.TuskClient
	var authOptions api.AuthOptions

	if cloud {
		var err error
		client, authOptions, cfg, err = api.SetupCloud(context.Background(), true)
		if err != nil {
			cmd.SilenceUsage = true
			return err
		}

		if ci {

			// Validate required CI metadata
			// TODO: this is GitHub-specific; add support for other CI providers
			// TODO: we probably don't need all of these to be required
			verifyCIMetadata := false
			if verifyCIMetadata {
				if commitSha == "" {
					if v := os.Getenv("GITHUB_SHA"); v != "" {
						commitSha = v
					} else {
						cmd.SilenceUsage = true
						return fmt.Errorf("commit SHA is required in cloud mode. Set --commit-sha or GITHUB_SHA.")
					}
				}
				if prNumber == "" {
					if ref := os.Getenv("GITHUB_REF"); ref != "" {
						// Only for pull request events
						// Example: refs/pull/123/merge -> 123
						prNumber = strings.Split(ref, "/")[2]
					} else {
						cmd.SilenceUsage = true
						return fmt.Errorf("pull request number is required in cloud mode. Set --pr-number or GITHUB_PR_NUMBER.")
					}
				}
				if _, err := strconv.Atoi(prNumber); err != nil {
					cmd.SilenceUsage = true
					return fmt.Errorf("pull request number must be a number: %w", err)
				}
				if branchName == "" {
					if v := os.Getenv("GITHUB_REF_NAME"); v != "" {
						branchName = v
					} else {
						cmd.SilenceUsage = true
						return fmt.Errorf("branch name is required in cloud mode. Set --branch or GITHUB_REF_NAME.")
					}
				}
				if externalCheckRunID == "" {
					if v := os.Getenv("GITHUB_CHECK_RUN_ID"); v != "" {
						externalCheckRunID = v
					} else {
						cmd.SilenceUsage = true
						return fmt.Errorf("external check run ID is required in cloud mode. Set --external-check-run-id or GITHUB_CHECK_RUN_ID.")
					}
				}
			}

			req := &backend.CreateDriftRunRequest{
				ObservableServiceId: cfg.Service.ID,
				CliVersion:          version.Version,
				CommitSha:           commitSha,
				PrNumber:            prNumber,
				BranchName:          branchName,
				ExternalCheckRunId:  externalCheckRunID,
			}

			id, err := client.CreateDriftRun(context.Background(), req, authOptions)
			if err != nil {
				return fmt.Errorf("failed to create drift run: %w", err)
			}

			driftRunID = id
			if !interactive {
				fmt.Fprintf(os.Stderr, "Tusk Drift run ID: %s\n", driftRunID)
			}

			statusReq := &backend.UpdateDriftRunCIStatusRequest{
				DriftRunId: driftRunID,
				CiStatus:   backend.DriftRunCIStatus_DRIFT_RUN_CI_STATUS_RUNNING,
			}
			if err := client.UpdateDriftRunCIStatus(context.Background(), statusReq, authOptions); err != nil {
				slog.Warn("Failed to update CI status to RUNNING", "error", err)
			}
		}
	}

	if cmd.Flags().Changed("concurrency") {
		executor.SetConcurrency(concurrency)
	}

	executor.SetEnableServiceLogs(enableServiceLogs || debug)
	if saveResults {
		if resultsDir == "" {
			if getConfigErr == nil && cfg.Results.Dir != "" {
				resultsDir = cfg.Results.Dir
			} else {
				resultsDir = utils.ResolveTuskPath(".tusk/results")
			}
		} else {
			resultsDir = utils.ResolveTuskPath(resultsDir)
		}
		executor.SetResultsOutput(resultsDir)
	}

	// Aggregation for results upload logs
	var mu sync.Mutex
	uploadedCount := 0
	attemptedCount := 0
	var lastUploadErr error

	// Per-test cloud upload while TUI is active (and also in headless)
	if cloud && client != nil && ci {
		executor.SetOnTestCompleted(func(res runner.TestResult, test runner.Test) {
			err := runner.UploadSingleTestResult(
				context.Background(),
				client,
				driftRunID,
				authOptions,
				executor,
				res,
				test,
			)

			mu.Lock()
			attemptedCount++
			if err != nil {
				lastUploadErr = err
				if interactive {
					logging.LogToCurrentTest(test.TraceID, fmt.Sprintf("\n🟠 Failed to upload test results: %v\n", err))
				}
			} else {
				uploadedCount++
				if interactive {
					logging.LogToCurrentTest(test.TraceID, "\n📝 Test result successfully uploaded\n")
				}
			}
			mu.Unlock()
		})
	}

	var tests []runner.Test
	var err error

	// Step 3: Load tests - in cloud mode, fetch from backend; otherwise use local files
	deferLoadTests := interactive
	if deferLoadTests {
		// Defer loading to the TUI (async)
	} else {
		loadTests := makeLoadTestsFunc(
			executor,
			client,
			authOptions,
			cfg.Service.ID,
			driftRunID,
			traceID,
			traceTestID,
			allCloudTraceTests || !ci,
			filter,
		)
		tests, err = loadTests(context.Background())
		if err != nil {
			cmd.SilenceUsage = true
			if cloud && client != nil {
				return fmt.Errorf("failed to load cloud tests: %w", err)
			}
			return fmt.Errorf("failed to load tests: %w", err)
		}
	}

	if !deferLoadTests && len(tests) == 0 {
		if print && outputFormat == "json" {
			fmt.Println("[]")
			fmt.Fprintln(os.Stderr, "No tests found")
		} else {
			fmt.Println("No tests found")
		}

		if cloud && client != nil && ci {
			statusReq := &backend.UpdateDriftRunCIStatusRequest{
				DriftRunId:      driftRunID,
				CiStatus:        backend.DriftRunCIStatus_DRIFT_RUN_CI_STATUS_SUCCESS,
				CiStatusMessage: stringPtr("No tests found"),
			}
			if err := client.UpdateDriftRunCIStatus(context.Background(), statusReq, authOptions); err != nil {
				slog.Warn("Failed to update CI status to SUCCESS", "error", err)
			}
		}

		return nil
	}

	// Provide suite spans before starting environment so SDK can mock pre-app calls
	if !deferLoadTests {
		if err := prepareAndSetSuiteSpans(
			context.Background(),
			executor,
			client,
			authOptions,
			cfg.Service.ID,
			tests,
			traceTestID,
			false, // interactive
		); err != nil {
			slog.Warn("Failed to prepare suite spans", "error", err)
		}
	}

	if interactive {
		RegisterCleanup(func() {
			slog.Info("Cleanup: Stopping services from signal handler")
			if err := executor.StopEnvironment(); err != nil {
				slog.Warn("Cleanup: Failed to stop environment", "error", err)
			}

			if cloud && client != nil {
				statusReq := &backend.UpdateDriftRunCIStatusRequest{
					DriftRunId:      driftRunID,
					CiStatus:        backend.DriftRunCIStatus_DRIFT_RUN_CI_STATUS_FAILURE,
					CiStatusMessage: stringPtr("Test execution interrupted"),
				}
				if err := client.UpdateDriftRunCIStatus(context.Background(), statusReq, authOptions); err != nil {
					slog.Warn("Failed to update CI status to FAILURE", "error", err)
				}
			}
		})

		initialLogs := []string{}
		if driftRunID != "" {
			initialLogs = append(initialLogs, fmt.Sprintf("Created Tusk Drift run: %s", driftRunID))
		}
		if cloud && client != nil {
			initialLogs = append(initialLogs, "📡 Fetching tests from Tusk Drift Cloud...")
		} else {
			initialLogs = append(initialLogs, "📁 Loading tests from local traces...")
		}

		_, err := tui.RunTestsInteractiveWithOpts(nil, executor, &tui.InteractiveOpts{
			InitialServiceLogs:    initialLogs,
			StartAfterTestsLoaded: true,
			LoadTests: makeLoadTestsFunc(
				executor,
				client,
				authOptions,
				cfg.Service.ID,
				driftRunID,
				traceID,
				traceTestID,
				allCloudTraceTests || !ci,
				filter,
			),
			OnBeforeEnvironmentStart: func(exec *runner.Executor, tests []runner.Test) error {
				return prepareAndSetSuiteSpans(
					context.Background(),
					exec,
					client,
					authOptions,
					cfg.Service.ID,
					tests,
					traceTestID,
					true, // interactive
				)
			},
			OnAllCompleted: func(results []runner.TestResult, tests []runner.Test, exec *runner.Executor) {
				if cloud && client != nil && ci {
					if err := runner.UploadResultsAndFinalize(context.Background(), client, driftRunID, authOptions, exec, results, tests, true); err != nil {
						slog.Warn("Interactive: cloud finalize failed", "error", err)
					}
					mu.Lock()
					summary := fmt.Sprintf("Upload summary: %d/%d results uploaded", uploadedCount, attemptedCount)
					mu.Unlock()
					logging.LogToService(summary)
				}
			},
		})
		if err != nil {
			if cloud && client != nil && ci {
				statusReq := &backend.UpdateDriftRunCIStatusRequest{
					DriftRunId:      driftRunID,
					CiStatus:        backend.DriftRunCIStatus_DRIFT_RUN_CI_STATUS_FAILURE,
					CiStatusMessage: stringPtr(fmt.Sprintf("Interactive run failed: %v", err)),
				}
				_ = client.UpdateDriftRunCIStatus(context.Background(), statusReq, authOptions)
			}
			return err
		}
		return nil
	}

	// Beyond this point, we're running tests without the UI
	if err = executor.StartEnvironment(); err != nil {
		cmd.SilenceUsage = true

		if cloud && client != nil && ci {
			statusReq := &backend.UpdateDriftRunCIStatusRequest{
				DriftRunId:      driftRunID,
				CiStatus:        backend.DriftRunCIStatus_DRIFT_RUN_CI_STATUS_FAILURE,
				CiStatusMessage: stringPtr(fmt.Sprintf("Failed to start environment: %v", err)),
			}
			if updateErr := client.UpdateDriftRunCIStatus(context.Background(), statusReq, authOptions); updateErr != nil {
				slog.Warn("Failed to update CI status to FAILURE", "error", updateErr)
			}
		}

		return fmt.Errorf("failed to start environment: %w", err)
	}
	defer func() {
		if stopErr := executor.StopEnvironment(); stopErr != nil {
			slog.Warn("Failed to stop environment", "error", stopErr)
		}
	}()

	// Step 4: Run tests
	results, err := executor.RunTests(tests)
	if err != nil {
		cmd.SilenceUsage = true

		// Update CI status to FAILURE if in cloud mode
		if cloud && client != nil && ci {
			if err := runner.UploadResultsAndFinalize(context.Background(), client, driftRunID, authOptions, executor, results, tests, true); err != nil {
				slog.Warn("Headless: cloud finalize failed", "error", err)
			}
			mu.Lock()
			fmt.Fprintf(os.Stderr, "Successfully uploaded %d/%d test results", uploadedCount, attemptedCount)
			if attemptedCount > uploadedCount && lastUploadErr != nil {
				fmt.Fprintf(os.Stderr, ". Last error: %v", lastUploadErr)
			}
			fmt.Fprintln(os.Stderr)
			mu.Unlock()
		}

		return fmt.Errorf("test execution failed: %w", err)
	}

	err = runner.OutputResults(results, tests, outputFormat, quiet)
	if err != nil {
		cmd.SilenceUsage = true
		return err
	}

	// Step 5: Upload results to backend if in cloud mode
	if cloud && client != nil && ci {
		if err := runner.UploadResultsAndFinalize(context.Background(), client, driftRunID, authOptions, executor, results, tests, true); err != nil {
			slog.Warn("Headless: cloud finalize failed", "error", err)
		}
		mu.Lock()
		fmt.Fprintf(os.Stderr, "\nSuccessfully uploaded %d/%d test results", uploadedCount, attemptedCount)
		if attemptedCount > uploadedCount && lastUploadErr != nil {
			fmt.Fprintf(os.Stderr, ". Last error: %v", lastUploadErr)
		}
		fmt.Fprintln(os.Stderr)
		mu.Unlock()
	}

	return nil
}

func loadCloudTests(ctx context.Context, client *api.TuskClient, auth api.AuthOptions, serviceID, driftRunID, traceTestID string, allCloud bool) ([]runner.Test, error) {
	if allCloud {
		var (
			all []*backend.TraceTest
			cur string
		)
		for {
			req := &backend.GetAllTraceTestsRequest{
				ObservableServiceId: serviceID,
				PageSize:            100,
			}
			if cur != "" {
				req.PaginationCursor = &cur
			}
			resp, err := client.GetAllTraceTests(ctx, req, auth)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch trace tests from backend: %w", err)
			}
			all = append(all, resp.TraceTests...)
			if next := resp.GetNextCursor(); next != "" {
				cur = next
				continue
			}
			break
		}
		logging.LogToService(fmt.Sprintf("Fetched %d trace tests from backend", len(all)))
		return runner.ConvertTraceTestsToRunnerTests(all), nil
	}

	if traceTestID != "" {
		req := &backend.GetTraceTestRequest{
			ObservableServiceId: serviceID,
			TraceTestId:         traceTestID,
		}
		resp, err := client.GetTraceTest(ctx, req, auth)
		if err != nil {
			return nil, err
		}
		return runner.ConvertTraceTestsToRunnerTests([]*backend.TraceTest{resp.TraceTest}), nil
	}

	var (
		all []*backend.TraceTest
		cur string
	)
	for {
		req := &backend.GetDriftRunTraceTestsRequest{
			DriftRunId: driftRunID,
			PageSize:   100,
		}
		if cur != "" {
			req.PaginationCursor = &cur
		}
		resp, err := client.GetDriftRunTraceTests(ctx, req, auth)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch trace tests from backend: %w", err)
		}
		all = append(all, resp.TraceTests...)
		if next := resp.GetNextCursor(); next != "" {
			cur = next
			continue
		}
		break
	}
	logging.LogToService(fmt.Sprintf("Fetched %d trace tests from backend", len(all)))
	return runner.ConvertTraceTestsToRunnerTests(all), nil
}

func fetchPreAppStartSpans(ctx context.Context, client *api.TuskClient, auth api.AuthOptions, serviceID string) ([]*core.Span, error) {
	var all []*core.Span
	cur := ""
	for {
		req := &backend.GetPreAppStartSpansRequest{
			ObservableServiceId: serviceID,
			PageSize:            200,
		}
		if cur != "" {
			req.PaginationCursor = &cur
		}

		resp, err := client.GetPreAppStartSpans(ctx, req, auth)
		if err != nil {
			return nil, fmt.Errorf("get pre-app-start spans: %w", err)
		}
		all = append(all, resp.Spans...)
		if next := resp.GetNextCursor(); next != "" {
			cur = next
			continue
		}
		break
	}
	return all, nil
}

func fetchLocalPreAppStartSpans(interactive bool) ([]*core.Span, error) {
	var out []*core.Span
	seen := map[string]struct{}{}

	for _, dir := range utils.GetPossibleTraceDirs() {
		matches, err := filepath.Glob(filepath.Join(dir, "*trace*.jsonl"))
		if err != nil {
			continue
		}
		for _, f := range matches {
			spans, err := utils.ParseSpansFromFile(f, func(s *core.Span) bool { return s.IsPreAppStart })
			if err != nil {
				if interactive {
					logging.LogToService(fmt.Sprintf("❌ Failed to parse spans from %s: %v", f, err))
				} else {
					fmt.Fprintf(os.Stderr, "Failed to parse spans from %s: %v\n", f, err)
				}
				continue
			}
			for _, s := range spans {
				key := s.TraceId + "|" + s.SpanId
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				out = append(out, s)
			}
		}
	}
	return out, nil
}

func fetchAllSuiteSpans(ctx context.Context, client *api.TuskClient, auth api.AuthOptions, serviceID string) ([]*core.Span, error) {
	var spans []*core.Span
	cur := ""
	for {
		req := &backend.GetAllTraceTestsRequest{
			ObservableServiceId: serviceID,
			PageSize:            100,
		}
		if cur != "" {
			req.PaginationCursor = &cur
		}
		resp, err := client.GetAllTraceTests(ctx, req, auth)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch trace tests from backend: %w", err)
		}
		for _, tt := range resp.TraceTests {
			if len(tt.Spans) > 0 {
				spans = append(spans, tt.Spans...)
			}
		}
		if next := resp.GetNextCursor(); next != "" {
			cur = next
			continue
		}
		break
	}
	return spans, nil
}

// buildSuiteSpansForRun builds the suite spans for the run.
// If running a single cloud trace test, eager-fetch all suite spans to enable cross-suite matching.
// Returns the suite spans, the number of pre-app-start spans, and the number of unique traces.
func buildSuiteSpansForRun(
	ctx context.Context,
	client *api.TuskClient,
	auth api.AuthOptions,
	serviceID string,
	tests []runner.Test,
	traceTestID string,
	interactive bool,
) ([]*core.Span, int, int, error) {
	var suiteSpans []*core.Span

	if client != nil && traceTestID != "" {
		all, err := fetchAllSuiteSpans(ctx, client, auth, serviceID)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("fetch all suite spans: %w", err)
		}
		if len(all) > 0 {
			suiteSpans = append(suiteSpans, all...)
		}
	}

	// Fallback: use spans from the loaded tests
	if len(suiteSpans) == 0 {
		for _, t := range tests {
			if len(t.Spans) > 0 {
				suiteSpans = append(suiteSpans, t.Spans...)
			}
		}
	}

	// Layer on pre-app-start spans if available
	// Prepend these spans so they get considered first
	if client != nil {
		preAppStartSpans, err := fetchPreAppStartSpans(ctx, client, auth, serviceID)
		if err == nil && len(preAppStartSpans) > 0 {
			suiteSpans = append(preAppStartSpans, suiteSpans...)
		}
	} else {
		if localPreAppStartSpans, err := fetchLocalPreAppStartSpans(interactive); err == nil && len(localPreAppStartSpans) > 0 {
			suiteSpans = append(localPreAppStartSpans, suiteSpans...)
		}
	}

	suiteSpans = dedupeSpans(suiteSpans)

	preAppCount := 0
	uniq := make(map[string]struct{})
	for _, s := range suiteSpans {
		if s == nil {
			continue
		}
		if s.IsPreAppStart {
			preAppCount++
		}
		if s.TraceId != "" {
			uniq[s.TraceId] = struct{}{}
		}
	}

	return suiteSpans, preAppCount, len(uniq), nil
}

// dedupeSpans deduplicates spans by (trace_id, span_id) while preserving order
func dedupeSpans(spans []*core.Span) []*core.Span {
	if len(spans) <= 1 {
		return spans
	}
	seen := make(map[string]struct{}, len(spans))
	out := make([]*core.Span, 0, len(spans))

	for _, s := range spans {
		if s == nil {
			continue
		}
		if s.TraceId != "" && s.SpanId != "" {
			key := s.TraceId + "|" + s.SpanId
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
		}
		out = append(out, s)
	}

	slog.Debug("Deduplicated suite spans", "inCount", len(spans), "outCount", len(out))
	return out
}

func makeLoadTestsFunc(
	executor *runner.Executor,
	client *api.TuskClient,
	auth api.AuthOptions,
	serviceID string,
	driftRunID string,
	traceID string,
	traceTestID string,
	allCloud bool,
	filter string,
) func(ctx context.Context) ([]runner.Test, error) {
	return func(ctx context.Context) ([]runner.Test, error) {
		var tests []runner.Test
		var err error

		if client != nil {
			if traceID != "" && traceTestID == "" {
				return nil, fmt.Errorf("specify --trace-test-id to run against a single trace test in Tusk Drift Cloud")
			}
			tests, err = loadCloudTests(ctx, client, auth, serviceID, driftRunID, traceTestID, allCloud)
			if err != nil {
				return nil, err
			}
		} else {
			switch {
			case traceDir != "":
				tests, err = executor.LoadTestsFromFolder(traceDir)
			case traceFile != "":
				tests, err = executor.LoadTestsFromTraceFile(traceFile)
			case traceID != "":
				var traceFilePath string
				traceFilePath, err = utils.FindTraceFile(traceID, "")
				if err == nil {
					tests, err = executor.LoadTestsFromTraceFile(traceFilePath)
				}
			default:
				tests, err = executor.LoadTestsFromFolder(utils.GetTracesDir())
			}
			if err != nil {
				return nil, err
			}
		}

		if filter != "" {
			return runner.FilterTests(tests, filter)
		}
		return tests, nil
	}
}

func prepareAndSetSuiteSpans(
	ctx context.Context,
	exec *runner.Executor,
	client *api.TuskClient,
	auth api.AuthOptions,
	serviceID string,
	tests []runner.Test,
	traceTestID string,
	interactive bool,
) error {
	suiteSpans, preAppCount, uniqueTraceCount, err := buildSuiteSpansForRun(
		ctx, client, auth, serviceID, tests, traceTestID, interactive,
	)
	if interactive {
		logging.LogToService(fmt.Sprintf(
			"Loading %d suite spans for matching (%d unique traces, %d pre-app-start)",
			len(suiteSpans), uniqueTraceCount, preAppCount,
		))
	}
	slog.Debug("Prepared suite spans for matching",
		"count", len(suiteSpans),
		"uniqueTraces", uniqueTraceCount,
		"preAppSpans", preAppCount,
		"interactive", interactive,
		"traceTestID", traceTestID,
	)
	exec.SetSuiteSpans(suiteSpans)
	return err
}

func stringPtr(s string) *string {
	return &s
}
