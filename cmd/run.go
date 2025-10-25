package cmd

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
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
)

var (
	traceDir          string
	traceFile         string
	traceID           string
	print             bool
	outputFormat      string
	filter            string
	quiet             bool
	verbose           bool
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
	runCmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Quiet output, only show deviations (only works with --print and --output-format text)")
	runCmd.Flags().BoolVarP(&verbose, "verbose", "", false, "Verbose output, show detailed deviation information (only works with --print)")
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
			ciMetadata := CIMetadata{
				CommitSha:          commitSha,
				PRNumber:           prNumber,
				BranchName:         branchName,
				ExternalCheckRunID: externalCheckRunID,
			}

			ciMetadata, err = validateCIMetadata(ciMetadata)
			if err != nil {
				cmd.SilenceUsage = true
				return err
			}

			commitSha = ciMetadata.CommitSha
			prNumber = ciMetadata.PRNumber
			branchName = ciMetadata.BranchName
			externalCheckRunID = ciMetadata.ExternalCheckRunID

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
				// TODO: make this more user-friendly, this is probably a server side issue, but could be wrong url set.
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
	if !interactive {
		executor.SetOnTestCompleted(func(res runner.TestResult, test runner.Test) {
			runner.OutputSingleResult(res, test, outputFormat, quiet, verbose)
		})
	}

	// Aggregation for results upload logs
	var mu sync.Mutex
	uploadedCount := 0
	attemptedCount := 0
	var lastUploadErr error

	// Per-test cloud upload while TUI is active (and also in headless)
	if cloud && client != nil && ci {
		// Save existing callback if print mode is enabled
		existingCallback := func(res runner.TestResult, test runner.Test) {}
		if !interactive {
			existingCallback = func(res runner.TestResult, test runner.Test) {
				runner.OutputSingleResult(res, test, outputFormat, quiet, verbose)
			}
		}

		executor.SetOnTestCompleted(func(res runner.TestResult, test runner.Test) {
			if !interactive {
				existingCallback(res, test)
			}

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
			false,
			quiet,
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

	if !interactive && !cloud {
		fmt.Fprintf(os.Stderr, "\n➤ Loaded %d tests from local traces\n", len(tests))
	}

	// Provide suite spans before starting environment so SDK can mock pre-app calls
	if !deferLoadTests {
		if err := runner.PrepareAndSetSuiteSpans(
			context.Background(),
			executor,
			runner.SuiteSpanOptions{
				IsCloudMode: cloud,
				Client:      client,
				AuthOptions: authOptions,
				ServiceID:   cfg.Service.ID,
				TraceTestID: traceTestID,
				AllTests:    tests,
				Interactive: false,
				Quiet:       quiet,
			},
			tests,
		); err != nil {
			slog.Warn("Failed to prepare suite spans", "error", err)
		}
	}

	RegisterCleanup(func() {
		slog.Debug("Cleanup: Cancelling running tests")
		executor.CancelTests()

		slog.Debug("Cleanup: Stopping services from signal handler")
		if err := executor.StopEnvironment(); err != nil {
			slog.Debug("Cleanup: Failed to stop environment", "error", err)
		}

		if cloud && client != nil {
			statusReq := &backend.UpdateDriftRunCIStatusRequest{
				DriftRunId:      driftRunID,
				CiStatus:        backend.DriftRunCIStatus_DRIFT_RUN_CI_STATUS_FAILURE,
				CiStatusMessage: stringPtr("Test execution interrupted"),
			}
			if err := client.UpdateDriftRunCIStatus(context.Background(), statusReq, authOptions); err != nil {
				slog.Debug("Failed to update CI status to FAILURE", "error", err)
			}
		}
	})

	if interactive {
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
			IsCloudMode:           cloud,
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
				true,
				quiet,
			),
			OnBeforeEnvironmentStart: func(exec *runner.Executor, tests []runner.Test) error {
				return runner.PrepareAndSetSuiteSpans(
					context.Background(),
					exec,
					runner.SuiteSpanOptions{
						IsCloudMode: cloud,
						Client:      client,
						AuthOptions: authOptions,
						ServiceID:   cfg.Service.ID,
						TraceTestID: traceTestID,
						AllTests:    tests,
						Interactive: true,
					},
					tests,
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

	if !interactive && !quiet {
		fmt.Fprintf(os.Stderr, "➤ Starting environment...\n")
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

		fmt.Fprint(os.Stderr, executor.GetStartupFailureHelpMessage())
		return fmt.Errorf("failed to start environment: %w", err)
	}
	defer func() {
		if stopErr := executor.StopEnvironment(); stopErr != nil {
			slog.Warn("Failed to stop environment", "error", stopErr)
		}
	}()

	if !interactive && !quiet {
		fmt.Fprintf(os.Stderr, "  ✓ Environment ready\n")
		fmt.Fprintf(os.Stderr, "➤ Running %d tests (concurrency: %d)...\n\n", len(tests), executor.GetConcurrency())
	}

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

	_ = os.Stdout.Sync()
	time.Sleep(1 * time.Millisecond)

	var outputErr error
	if !interactive {
		// Results already streamed, just print summary
		outputErr = runner.OutputResultsSummary(results, outputFormat, quiet)
	}

	// Step 5: Upload results to backend if in cloud mode
	// Do this before returning any error so CI status is always updated
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

	if outputErr != nil {
		cmd.SilenceUsage = true
		return outputErr
	}

	return nil
}

func loadCloudTests(ctx context.Context, client *api.TuskClient, auth api.AuthOptions, serviceID, driftRunID, traceTestID string, allCloud bool, interactive bool, quiet bool) ([]runner.Test, error) {
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

	var progress *utils.ProgressBar
	if !interactive && !quiet {
		progress = utils.NewProgressBar("Fetching tests")
		progress.Start()
		defer progress.Stop()
	}

	var (
		all []*backend.TraceTest
		cur string
	)

	if allCloud {
		for {
			req := &backend.GetAllTraceTestsRequest{
				ObservableServiceId: serviceID,
				PageSize:            25,
			}
			if cur != "" {
				req.PaginationCursor = &cur
			}
			resp, err := client.GetAllTraceTests(ctx, req, auth)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch trace tests from backend: %w", err)
			}

			// Set total on first request
			if progress != nil && cur == "" {
				progress.SetTotal(int(resp.TotalCount))
			}

			all = append(all, resp.TraceTests...)

			if progress != nil {
				progress.SetCurrent(len(all))
			}

			if next := resp.GetNextCursor(); next != "" {
				cur = next
				continue
			}
			break
		}
	} else {
		for {
			req := &backend.GetDriftRunTraceTestsRequest{
				DriftRunId: driftRunID,
				PageSize:   25,
			}
			if cur != "" {
				req.PaginationCursor = &cur
			}
			resp, err := client.GetDriftRunTraceTests(ctx, req, auth)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch trace tests from backend: %w", err)
			}

			// Set total on first request
			if progress != nil && cur == "" {
				progress.SetTotal(int(resp.TotalCount))
			}

			all = append(all, resp.TraceTests...)

			if progress != nil {
				progress.SetCurrent(len(all))
			}

			if next := resp.GetNextCursor(); next != "" {
				cur = next
				continue
			}
			break
		}
	}

	if progress != nil {
		progress.Finish(fmt.Sprintf("➤ Fetched %d trace tests from Tusk Drift Cloud", len(all)))
	} else {
		logging.LogToService(fmt.Sprintf("Fetched %d trace tests from Tusk Drift Cloud", len(all)))
	}
	return runner.ConvertTraceTestsToRunnerTests(all), nil
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
	interactive bool,
	quiet bool,
) func(ctx context.Context) ([]runner.Test, error) {
	return func(ctx context.Context) ([]runner.Test, error) {
		var tests []runner.Test
		var err error

		if client != nil {
			if traceID != "" && traceTestID == "" {
				return nil, fmt.Errorf("specify --trace-test-id to run against a single trace test in Tusk Drift Cloud")
			}
			tests, err = loadCloudTests(ctx, client, auth, serviceID, driftRunID, traceTestID, allCloud, interactive, quiet)
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

type CIMetadata struct {
	CommitSha          string
	PRNumber           string
	BranchName         string
	ExternalCheckRunID string
}

// validateCIMetadata validates and populates CI metadata from environment variables
// Only attempts to populate from environment variables when running in a recognized CI environment.
func validateCIMetadata(metadata CIMetadata) (CIMetadata, error) {
	// Note: we only detect GitHub/GitLab CI environments for now since they are the most common.
	// Other CI providers can use flags to provide this metadata.
	isGitHub := os.Getenv("GITHUB_ACTIONS") == "true"
	isGitLab := os.Getenv("GITLAB_CI") == "true"
	inCI := isGitHub || isGitLab

	// Only populate from environment variables if in CI
	if inCI {
		if metadata.CommitSha == "" {
			if isGitHub {
				metadata.CommitSha = os.Getenv("GITHUB_SHA")
			} else if isGitLab {
				metadata.CommitSha = os.Getenv("CI_COMMIT_SHA")
			}
		}

		if metadata.PRNumber == "" {
			if isGitHub {
				if ref := os.Getenv("GITHUB_REF"); ref != "" {
					// Only for pull request events
					// Example: refs/pull/123/merge -> 123
					parts := strings.Split(ref, "/")
					if len(parts) > 2 {
						metadata.PRNumber = parts[2]
					}
				}
			} else if isGitLab {
				metadata.PRNumber = os.Getenv("CI_MERGE_REQUEST_IID")
			}
		}

		if metadata.BranchName == "" {
			if isGitHub {
				metadata.BranchName = os.Getenv("GITHUB_REF_NAME")
			} else if isGitLab {
				// Prefer merge request source branch name when available
				metadata.BranchName = os.Getenv("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME")
				if metadata.BranchName == "" {
					metadata.BranchName = os.Getenv("CI_COMMIT_REF_NAME")
				}
			}
		}

		if metadata.ExternalCheckRunID == "" {
			if isGitHub {
				metadata.ExternalCheckRunID = os.Getenv("GITHUB_CHECK_RUN_ID")
			} else if isGitLab {
				// GitLab doesn't have an exact equivalent to check runs
				// Use pipeline ID as the external identifier
				metadata.ExternalCheckRunID = os.Getenv("CI_PIPELINE_ID")
				if metadata.ExternalCheckRunID == "" {
					metadata.ExternalCheckRunID = os.Getenv("CI_JOB_ID")
				}
			}
		}
	}

	// Validate required fields (whether in CI or not)
	if metadata.CommitSha == "" {
		return metadata, fmt.Errorf("commit SHA is required. Provide via --commit-sha flag if not running in a CI environment.")
	}
	if metadata.PRNumber == "" {
		return metadata, fmt.Errorf("pull/merge request number is required. Provide via --pr-number flag if not running in a CI environment.")
	}
	if _, err := strconv.Atoi(metadata.PRNumber); err != nil {
		return metadata, fmt.Errorf("pull/merge request number must be an integer. You provided: '%s'.", metadata.PRNumber)
	}
	if metadata.BranchName == "" {
		return metadata, fmt.Errorf("branch name is required. Provide via --branch flag if not running in a CI environment.")
	}

	// ExternalCheckRunID is optional - no validation needed

	return metadata, nil
}

func stringPtr(s string) *string {
	return &s
}
