package cmd

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/Use-Tusk/tusk-drift-cli/internal/api"
	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
	"github.com/Use-Tusk/tusk-drift-cli/internal/log"
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
	verbose           bool
	concurrency       int
	enableServiceLogs bool
	saveResults       bool
	resultsDir        string
	disableSandbox    bool

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

	// Validation mode
	validateSuiteIfDefaultBranch bool
	validateSuite                bool
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
	runCmd.Flags().IntVar(&concurrency, "concurrency", 1, "Maximum number of concurrent tests. If set, overrides the concurrency setting in the config file.")
	runCmd.Flags().BoolVar(&enableServiceLogs, "enable-service-logs", false, "Send logs from your service to a file in .tusk/logs. Logs from the SDK will be present.")
	runCmd.Flags().BoolVar(&saveResults, "save-results", false, "Save replay results to a file")
	runCmd.Flags().StringVar(&resultsDir, "results-dir", "", "Path to directory to save results (only works with --save-results). Default is '.tusk/results'")
	runCmd.Flags().BoolVar(&disableSandbox, "disable-sandbox", false, "Disable replaying service in a sandbox (allows real connections)")

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

	// Validation mode flags
	runCmd.Flags().BoolVar(&validateSuiteIfDefaultBranch, "validate-suite-if-default-branch", false, "[Cloud] Validate traces on default branch before adding to suite")
	runCmd.Flags().BoolVar(&validateSuite, "validate-suite", false, "[Cloud] Force validation mode regardless of branch")

	_ = runCmd.Flags().MarkHidden("client-id")
	runCmd.Flags().SortFlags = false
}

func runTests(cmd *cobra.Command, args []string) error {
	setupSignalHandling()

	log.Debug("Starting test execution",
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
		"disable-sandbox", disableSandbox,
		"cloud", cloud,
		"ci", ci,
		"commitSha", commitSha,
		"prNumber", prNumber,
		"branchName", branchName,
		"externalCheckRunID", externalCheckRunID,
		"clientID", clientID,
	)

	executor := runner.NewExecutor()
	executor.SetDisableSandbox(disableSandbox)
	executor.SetDebug(debug)

	_ = config.Load(cfgFile)
	cfg, getConfigErr := config.Get()
	if getConfigErr == nil && cfg.TestExecution.Concurrency > 0 {
		executor.SetConcurrency(cfg.TestExecution.Concurrency)
	}
	if getConfigErr == nil && cfg.TestExecution.Timeout != "" {
		// Already validated for correct duration
		d, _ := time.ParseDuration(cfg.TestExecution.Timeout)
		executor.SetTestTimeout(d)
	}

	if traceDir != "" {
		utils.SetTracesDirOverride(traceDir)
	} else if getConfigErr == nil && cfg.Traces.Dir != "" {
		utils.SetTracesDirOverride(cfg.Traces.Dir)
	}

	interactive := !print && (utils.IsTerminal() || utils.TUICIMode())

	var driftRunID string
	var client *api.TuskClient
	var authOptions api.AuthOptions
	isValidation := false

	if cloud {
		var err error
		client, authOptions, cfg, err = api.SetupCloud(context.Background(), true)
		if err != nil {
			cmd.SilenceUsage = true
			return err
		}

		// Check for validation mode
		// --validate-suite forces validation regardless of branch
		// --validate-suite-if-default-branch only validates on default branch
		if validateSuite && traceTestID == "" {
			log.Debug("Validation mode forced with --validate-suite flag")
			isValidation = true
		} else if validateSuiteIfDefaultBranch && traceTestID == "" {
			// Get default branch from backend
			infoReq := &backend.GetObservableServiceInfoRequest{
				ObservableServiceId: cfg.Service.ID,
			}
			info, err := client.GetObservableServiceInfo(context.Background(), infoReq, authOptions)
			if err != nil {
				return fmt.Errorf("failed to get observable service info: %w", err)
			}

			// Check if we're on the default branch
			currentBranch := getBranchFromEnv()
			if currentBranch == info.DefaultBranch {
				log.Debug("On default branch, running validation run", "currentBranch", currentBranch, "defaultBranch", info.DefaultBranch)
				isValidation = true
			} else {
				log.Debug("Not on default branch, running regular run", "currentBranch", currentBranch, "defaultBranch", info.DefaultBranch)
			}
		}

		if ci || isValidation {
			var req *backend.CreateDriftRunRequest

			if isValidation {
				// Validation mode: no CI metadata needed
				req = &backend.CreateDriftRunRequest{
					ObservableServiceId: cfg.Service.ID,
					CliVersion:          version.Version,
					IsValidationRun:     true,
				}
			} else {
				// Regular CI mode: validate and include CI metadata
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

				req = &backend.CreateDriftRunRequest{
					ObservableServiceId: cfg.Service.ID,
					CliVersion:          version.Version,
					CommitSha:           stringPtr(commitSha),
					PrNumber:            stringPtr(prNumber),
					BranchName:          stringPtr(branchName),
					ExternalCheckRunId:  stringPtr(externalCheckRunID),
					IsValidationRun:     false,
				}
			}

			id, err := client.CreateDriftRun(context.Background(), req, authOptions)
			if err != nil {
				// TODO: make this more user-friendly, this is probably a server side issue, but could be wrong url set.
				return fmt.Errorf("failed to create drift run: %w", err)
			}

			driftRunID = id
			if !interactive {
				log.Stderrln(fmt.Sprintf("Tusk Drift run ID: %s", driftRunID))
			}

			statusReq := &backend.UpdateDriftRunCIStatusRequest{
				DriftRunId: driftRunID,
				CiStatus:   backend.DriftRunCIStatus_DRIFT_RUN_CI_STATUS_RUNNING,
			}
			if err := client.UpdateDriftRunCIStatus(context.Background(), statusReq, authOptions); err != nil {
				log.Warn("Failed to update CI status to RUNNING", "error", err)
			}
		}
	} else if getConfigErr != nil {
		// Non-cloud mode: config is required
		cmd.SilenceUsage = true
		return getConfigErr
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

			// Cleanup trace spans after the test is completed
			if executor.GetServer() != nil {
				executor.GetServer().CleanupTraceSpans(test.TraceID)
			}
		})
	}

	// Aggregation for results upload logs
	var mu sync.Mutex
	uploadedCount := 0
	attemptedCount := 0
	var lastUploadErr error

	// Per-test cloud upload while TUI is active (and also in headless)
	// Cloud mode, overrides the above OnTestCompleted callback
	if cloud && client != nil && (ci || isValidation) {
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
					log.TestLog(test.TraceID, fmt.Sprintf("\n🟠 Failed to upload test results: %v\n", err))
				}
			} else {
				uploadedCount++
				if interactive {
					log.TestLog(test.TraceID, "\n📝 Test result successfully uploaded\n")
				}
			}
			mu.Unlock()

			// Cleanup trace spans after the test is completed
			if executor.GetServer() != nil {
				executor.GetServer().CleanupTraceSpans(test.TraceID)
			}
		})
	}

	// If no OnTestCompleted callback was set, set a default one that just cleans up the trace spans
	if executor.OnTestCompleted == nil {
		executor.SetOnTestCompleted(func(res runner.TestResult, test runner.Test) {
			if executor.GetServer() != nil {
				executor.GetServer().CleanupTraceSpans(test.TraceID)
			}
		})
	}

	var tests []runner.Test
	var err error

	// Step 3: Load tests - in cloud mode, fetch from backend; otherwise use local files
	deferLoadTests := interactive
	if deferLoadTests {
		// Defer loading to the TUI (async)
	} else {
		if isValidation {
			// Validation mode: fetch all validation traces (draft + in_suite)
			tests, err = fetchValidationTraceTests(context.Background(), client, authOptions, cfg.Service.ID)
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
				quiet,
			)
			tests, err = loadTests(context.Background())
		}
		if err != nil {
			cmd.SilenceUsage = true
			if cloud && client != nil {
				if isValidation {
					updateStatusToFailure(context.Background(), client, driftRunID, authOptions, fmt.Sprintf("Failed to fetch tests: %v", err))
				}
				return fmt.Errorf("failed to load cloud tests: %w", err)
			}
			return fmt.Errorf("failed to load tests: %w", err)
		}
	}

	if !deferLoadTests && len(tests) == 0 {
		noTestsMsg := "No tests found"
		if isValidation {
			noTestsMsg = "No traces to validate"
		}

		if print && outputFormat == "json" {
			log.Println("[]")
			log.Stderrln(noTestsMsg)
		} else {
			log.Println(noTestsMsg)
		}

		if cloud && client != nil && (ci || isValidation) {
			statusReq := &backend.UpdateDriftRunCIStatusRequest{
				DriftRunId:      driftRunID,
				CiStatus:        backend.DriftRunCIStatus_DRIFT_RUN_CI_STATUS_SUCCESS,
				CiStatusMessage: stringPtr(noTestsMsg),
			}
			if err := client.UpdateDriftRunCIStatus(context.Background(), statusReq, authOptions); err != nil {
				log.Warn("Failed to update CI status to SUCCESS", "error", err)
			}
		}

		return nil
	}

	if !interactive && !quiet {
		if isValidation {
			log.Stderrln(fmt.Sprintf("\n➤ Found %d traces to validate", len(tests)))
		} else if !cloud {
			log.Stderrln(fmt.Sprintf("\n➤ Loaded %d tests from local traces", len(tests)))
		}
	}

	// Fetch pre-app-start spans before grouping (needed for ENV_VARS extraction)
	var preAppStartSpans []*core.Span
	if !deferLoadTests {
		if cloud && client != nil {
			preAppStartSpans, err = runner.FetchPreAppStartSpansFromCloud(context.Background(), client, authOptions, cfg.Service.ID, false, quiet)
			if err != nil {
				log.Warn("Failed to fetch pre-app-start spans from cloud", "error", err)
			}
		} else {
			preAppStartSpans, err = runner.FetchLocalPreAppStartSpans(false)
			if err != nil {
				log.Debug("Failed to fetch local pre-app-start spans", "error", err)
			}
		}
	}

	// Group tests by environment before starting
	var groupResult *runner.EnvironmentExtractionResult
	if !deferLoadTests {
		// For local traces: build suite spans with ALL tests first (for mock matching),
		// then filter out error responses (status >= 300) for execution only.
		// This ensures spans from error traces are still available for mock matching.
		testsForSuiteSpans := tests
		if !cloud {
			// Build suite spans with ALL tests before filtering
			if err := runner.PrepareAndSetSuiteSpans(
				context.Background(),
				executor,
				runner.SuiteSpanOptions{
					IsCloudMode:            cloud,
					Client:                 client,
					AuthOptions:            authOptions,
					ServiceID:              cfg.Service.ID,
					TraceTestID:            traceTestID,
					Interactive:            false,
					Quiet:                  quiet,
					AllowSuiteWideMatching: isValidation,
				},
				testsForSuiteSpans,
			); err != nil {
				log.Warn("Failed to prepare suite spans", "error", err)
			}

			// Filter out error responses for execution
			var excludedCount int
			tests, excludedCount = runner.FilterLocalTestsForExecution(tests)
			if excludedCount > 0 && !quiet {
				log.Stderrln(fmt.Sprintf("➤ Skipping %d tests with HTTP status >= 300 (spans still available for mocking)", excludedCount))
			}
		} else {
			// Cloud mode: no automatic filtering, prepare suite spans with all tests
			if err := runner.PrepareAndSetSuiteSpans(
				context.Background(),
				executor,
				runner.SuiteSpanOptions{
					IsCloudMode:            cloud,
					Client:                 client,
					AuthOptions:            authOptions,
					ServiceID:              cfg.Service.ID,
					TraceTestID:            traceTestID,
					Interactive:            false,
					Quiet:                  quiet,
					AllowSuiteWideMatching: isValidation,
				},
				tests,
			); err != nil {
				log.Warn("Failed to prepare suite spans", "error", err)
			}
		}

		groupResult, err = runner.GroupTestsByEnvironment(tests, preAppStartSpans)
		if err != nil {
			cmd.SilenceUsage = true
			return fmt.Errorf("failed to group tests by environment: %w", err)
		}

		// Log warnings if any
		for _, warning := range groupResult.Warnings {
			if !quiet {
				log.Stderrln(fmt.Sprintf("⚠️  %s", warning))
			}
		}
	}

	RegisterCleanup(func() {
		log.Debug("Cleanup: Cancelling running tests")
		executor.CancelTests()

		log.Debug("Cleanup: Stopping services from signal handler")
		if err := executor.StopEnvironment(); err != nil {
			log.Debug("Cleanup: Failed to stop environment", "error", err)
		}

		if cloud && client != nil {
			statusReq := &backend.UpdateDriftRunCIStatusRequest{
				DriftRunId:      driftRunID,
				CiStatus:        backend.DriftRunCIStatus_DRIFT_RUN_CI_STATUS_FAILURE,
				CiStatusMessage: stringPtr("Test execution interrupted"),
			}
			if err := client.UpdateDriftRunCIStatus(context.Background(), statusReq, authOptions); err != nil {
				log.Debug("Failed to update CI status to FAILURE", "error", err)
			}
		}
	})

	if interactive {
		initialLogs := []string{}
		if driftRunID != "" {
			initialLogs = append(initialLogs, fmt.Sprintf("Created Tusk Drift run: %s", driftRunID))
		}

		// For cloud mode, load tests upfront (with progress bar) before starting TUI
		// For local mode, defer loading to the TUI
		var allTestsForSuiteSpans []runner.Test
		var preloadedTests []runner.Test
		var loadTestsFn func(ctx context.Context) ([]runner.Test, error)

		if cloud && client != nil {
			// Load tests upfront with progress bar
			var err error
			if isValidation {
				preloadedTests, err = fetchValidationTraceTests(context.Background(), client, authOptions, cfg.Service.ID)
			} else {
				preloadedTests, err = loadCloudTests(context.Background(), client, authOptions, cfg.Service.ID, driftRunID, traceTestID, allCloudTraceTests || !ci, quiet)
			}
			if err != nil {
				return fmt.Errorf("failed to load cloud tests: %w", err)
			}
			if filter != "" {
				preloadedTests, err = runner.FilterTests(preloadedTests, filter)
				if err != nil {
					return fmt.Errorf("invalid filter: %w", err)
				}
			}
			allTestsForSuiteSpans = preloadedTests
		} else {
			initialLogs = append(initialLogs, "📁 Loading tests from local traces...")
			baseLoadTestsFn := makeLoadTestsFunc(
				executor,
				nil, // no client for local mode
				authOptions,
				cfg.Service.ID,
				driftRunID,
				traceID,
				traceTestID,
				false,
				filter,
				quiet,
			)
			loadTestsFn = func(ctx context.Context) ([]runner.Test, error) {
				tests, err := baseLoadTestsFn(ctx)
				if err != nil {
					return nil, err
				}
				// Store all tests for suite span preparation
				allTestsForSuiteSpans = tests

				// For local traces: filter out error responses for execution
				if !cloud {
					var excludedCount int
					tests, excludedCount = runner.FilterLocalTestsForExecution(tests)
					if excludedCount > 0 {
						log.ServiceLog(fmt.Sprintf("Skipping %d tests with HTTP status >= 300 (spans still available for mocking)", excludedCount))
					}
				}
				return tests, nil
			}
		}

		_, err := tui.RunTestsInteractiveWithOpts(preloadedTests, executor, &tui.InteractiveOpts{
			InitialServiceLogs:    initialLogs,
			StartAfterTestsLoaded: len(preloadedTests) == 0, // Only wait for loading if tests aren't preloaded
			IsCloudMode:           cloud,
			LoadTests:             loadTestsFn,
			OnBeforeEnvironmentStart: func(exec *runner.Executor, tests []runner.Test) error {
				// Use allTestsForSuiteSpans (includes error tests) for mock matching
				testsForSpans := allTestsForSuiteSpans
				if len(testsForSpans) == 0 {
					testsForSpans = tests // Fallback to passed tests if not set
				}
				return runner.PrepareAndSetSuiteSpans(
					context.Background(),
					exec,
					runner.SuiteSpanOptions{
						IsCloudMode:            cloud,
						Client:                 client,
						AuthOptions:            authOptions,
						ServiceID:              cfg.Service.ID,
						TraceTestID:            traceTestID,
						Interactive:            true,
						AllowSuiteWideMatching: isValidation,
					},
					testsForSpans,
				)
			},
			OnAllCompleted: func(results []runner.TestResult, tests []runner.Test, exec *runner.Executor) {
				if cloud && client != nil && (ci || isValidation) {
					var statusMessage string
					if isValidation {
						passed, failed := countPassedFailed(results)
						statusMessage = fmt.Sprintf("Validation complete: %d passed, %d failed", passed, failed)
					}
					if err := runner.UpdateDriftRunCIStatusWrapper(context.Background(), client, driftRunID, authOptions, results, statusMessage); err != nil {
						log.Warn("Interactive: cloud finalize failed", "error", err)
					}
					mu.Lock()
					summary := fmt.Sprintf("Upload summary: %d/%d results uploaded", uploadedCount, attemptedCount)
					mu.Unlock()
					log.ServiceLog(summary)
				}
			},
		})
		if err != nil {
			if cloud && client != nil && (ci || isValidation) {
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
		envCount := 1
		if groupResult != nil {
			envCount = len(groupResult.Groups)
		}
		log.Stderrln(fmt.Sprintf("➤ Running %d tests across %d environment(s) (concurrency: %d)...\n", len(tests), envCount, executor.GetConcurrency()))
	}

	// Step 4: Run tests by environment
	var results []runner.TestResult
	if groupResult != nil && len(groupResult.Groups) > 0 {
		// Use environment-based replay
		results, err = runner.ReplayTestsByEnvironment(context.Background(), executor, groupResult.Groups)
		if err != nil {
			cmd.SilenceUsage = true

			// Update CI status to FAILURE if in cloud mode
			if cloud && client != nil && (ci || isValidation) {
				if err := runner.UpdateDriftRunCIStatusWrapper(context.Background(), client, driftRunID, authOptions, results); err != nil {
					log.Warn("Headless: cloud finalize failed", "error", err)
				}
				mu.Lock()
				log.Stderr(fmt.Sprintf("Successfully uploaded %d/%d test results", uploadedCount, attemptedCount))
				if attemptedCount > uploadedCount && lastUploadErr != nil {
					log.Stderr(fmt.Sprintf(". Last error: %v", lastUploadErr))
				}
				log.Stderrln("")
				mu.Unlock()
			}

			return fmt.Errorf("environment-based test execution failed: %w", err)
		}
	} else {
		// Fallback: Original single-environment flow (for interactive mode or edge cases)
		if !interactive && !quiet {
			log.Stderrln("➤ Starting environment...")
		}

		if err = executor.StartEnvironment(); err != nil {
			cmd.SilenceUsage = true

			if cloud && client != nil && (ci || isValidation) {
				statusReq := &backend.UpdateDriftRunCIStatusRequest{
					DriftRunId:      driftRunID,
					CiStatus:        backend.DriftRunCIStatus_DRIFT_RUN_CI_STATUS_FAILURE,
					CiStatusMessage: stringPtr(fmt.Sprintf("Failed to start environment: %v", err)),
				}
				if updateErr := client.UpdateDriftRunCIStatus(context.Background(), statusReq, authOptions); updateErr != nil {
					log.Warn("Failed to update CI status to FAILURE", "error", updateErr)
				}
			}

			log.Stderr(executor.GetStartupFailureHelpMessage())
			return fmt.Errorf("failed to start environment: %w", err)
		}
		defer func() {
			if stopErr := executor.StopEnvironment(); stopErr != nil {
				log.Warn("Failed to stop environment", "error", stopErr)
			}
		}()

		if !interactive && !quiet {
			log.Stderrln("  ✓ Environment ready")
			log.Stderrln(fmt.Sprintf("➤ Running %d tests (concurrency: %d)...\n", len(tests), executor.GetConcurrency()))
		}

		results, err = executor.RunTests(tests)
		if err != nil {
			cmd.SilenceUsage = true

			// Update CI status to FAILURE if in cloud mode
			if cloud && client != nil && (ci || isValidation) {
				if err := runner.UpdateDriftRunCIStatusWrapper(context.Background(), client, driftRunID, authOptions, results); err != nil {
					log.Warn("Headless: cloud finalize failed", "error", err)
				}
				mu.Lock()
				log.Stderr(fmt.Sprintf("Successfully uploaded %d/%d test results", uploadedCount, attemptedCount))
				if attemptedCount > uploadedCount && lastUploadErr != nil {
					log.Stderr(fmt.Sprintf(". Last error: %v", lastUploadErr))
				}
				log.Stderrln("")
				mu.Unlock()
			}

			return fmt.Errorf("test execution failed: %w", err)
		}
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
	if cloud && client != nil && (ci || isValidation) {
		var statusMessage string
		if isValidation {
			passed, failed := countPassedFailed(results)
			statusMessage = fmt.Sprintf("Validation complete: %d passed, %d failed", passed, failed)
		}
		// streamed is always true here so this only updates the CI status
		// Does NOT upload results to the backend as they are already uploaded via UploadSingleTestResult during the callback
		if err := runner.UpdateDriftRunCIStatusWrapper(context.Background(), client, driftRunID, authOptions, results, statusMessage); err != nil {
			log.Warn("Headless: cloud finalize failed", "error", err)
		}
		if isValidation {
			log.Println("\nSuite validation completed - backend will process results and update suite")
		}
		mu.Lock()
		log.Stderr(fmt.Sprintf("\nSuccessfully uploaded %d/%d test results", uploadedCount, attemptedCount))
		if attemptedCount > uploadedCount && lastUploadErr != nil {
			log.Stderr(fmt.Sprintf(". Last error: %v", lastUploadErr))
		}
		log.Stderrln("")
		mu.Unlock()
	}

	if outputErr != nil {
		cmd.SilenceUsage = true
		// In CI mode, don't fail on test deviations - only fail on execution/upload errors
		// As long as all tests ran and results were uploaded successfully, we want the CI workflow to pass.
		if (ci || isValidation) && cloud {
			return nil
		}
		return outputErr
	}

	return nil
}

func loadCloudTests(ctx context.Context, client *api.TuskClient, auth api.AuthOptions, serviceID, driftRunID, traceTestID string, allCloud bool, quiet bool) ([]runner.Test, error) {
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

	var all []*backend.TraceTest
	var err error

	if allCloud {
		all, err = api.FetchAllTraceTests(ctx, client, auth, serviceID, nil)
	} else {
		all, err = api.FetchDriftRunTraceTests(ctx, client, auth, driftRunID, nil)
	}

	if err != nil {
		return nil, err
	}

	if !quiet {
		log.Stderrln(fmt.Sprintf("➤ Fetched %d trace tests from Tusk Drift Cloud", len(all)))
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
	quiet bool,
) func(ctx context.Context) ([]runner.Test, error) {
	return func(ctx context.Context) ([]runner.Test, error) {
		var tests []runner.Test
		var err error

		if client != nil {
			if traceID != "" && traceTestID == "" {
				return nil, fmt.Errorf("specify --trace-test-id to run against a single trace test in Tusk Drift Cloud")
			}
			tests, err = loadCloudTests(ctx, client, auth, serviceID, driftRunID, traceTestID, allCloud, quiet)
			if err != nil {
				return nil, err
			}
		} else {
			switch {
			case traceDir != "":
				tests, err = executor.LoadTestsFromFolder(traceDir)
			case traceFile != "":
				var test *runner.Test
				test, err = executor.LoadTestFromTraceFile(traceFile)
				if test != nil {
					tests = []runner.Test{*test}
				}
			case traceID != "":
				var traceFilePath string
				traceFilePath, err = utils.FindTraceFile(traceID, "")
				if err == nil {
					var test *runner.Test
					test, err = executor.LoadTestFromTraceFile(traceFilePath)
					if test != nil {
						tests = []runner.Test{*test}
					}
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
			metadata.BranchName = getBranchFromEnv()
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

// getBranchFromEnv returns the branch name from CI environment variables
// Returns empty string if no branch can be determined from environment
func getBranchFromEnv() string {
	// GitHub Actions - prefer GITHUB_HEAD_REF for PRs (actual branch name)
	if branch := os.Getenv("GITHUB_HEAD_REF"); branch != "" {
		return branch
	}
	if branch := os.Getenv("GITHUB_REF_NAME"); branch != "" {
		return branch
	}
	// GitLab CI - prefer merge request source branch when available
	if branch := os.Getenv("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME"); branch != "" {
		return branch
	}
	if branch := os.Getenv("CI_COMMIT_REF_NAME"); branch != "" {
		return branch
	}

	// Fallback: git rev-parse
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// fetchValidationTraceTests fetches all traces for validation (draft + in_suite)
func fetchValidationTraceTests(ctx context.Context, client *api.TuskClient, auth api.AuthOptions, serviceID string) ([]runner.Test, error) {
	var allTests []*backend.TraceTest
	var cursor string

	for {
		req := &backend.GetValidationTraceTestsRequest{
			ObservableServiceId: serviceID,
			PageSize:            50,
		}
		if cursor != "" {
			req.PaginationCursor = &cursor
		}

		resp, err := client.GetValidationTraceTests(ctx, req, auth)
		if err != nil {
			return nil, err
		}

		allTests = append(allTests, resp.TraceTests...)

		if resp.GetNextCursor() == "" {
			break
		}
		cursor = resp.GetNextCursor()
	}

	return runner.ConvertTraceTestsToRunnerTests(allTests), nil
}

func updateStatusToFailure(ctx context.Context, client *api.TuskClient, driftRunID string, auth api.AuthOptions, message string) {
	statusReq := &backend.UpdateDriftRunCIStatusRequest{
		DriftRunId:      driftRunID,
		CiStatus:        backend.DriftRunCIStatus_DRIFT_RUN_CI_STATUS_FAILURE,
		CiStatusMessage: stringPtr(message),
	}
	_ = client.UpdateDriftRunCIStatus(ctx, statusReq, auth)
}

// countPassedFailed counts passed and failed tests from results
func countPassedFailed(results []runner.TestResult) (passed, failed int) {
	for _, r := range results {
		if r.Passed {
			passed++
		} else {
			failed++
		}
	}
	return passed, failed
}
