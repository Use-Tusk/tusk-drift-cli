package cmd

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Use-Tusk/tusk-cli/internal/api"
	"github.com/Use-Tusk/tusk-cli/internal/log"
	"github.com/Use-Tusk/tusk-cli/internal/review"
	"github.com/Use-Tusk/tusk-cli/internal/utils"
	"github.com/Use-Tusk/tusk-cli/internal/version"
	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
)

//go:embed short_docs/review/overview.md
var reviewOverviewContent string

var (
	reviewRepo        string
	reviewBase        string
	reviewMinSeverity string
	reviewExcludes    []string
	reviewIncludes    []string
	reviewJSON        bool
	reviewOutput      string
	reviewQuiet       bool
	// Used by the status subcommand only.
	reviewStatusWatch bool
)

var reviewCmd = &cobra.Command{
	Use:          "review",
	Short:        "Run Tusk code review on your local working tree",
	Long:         utils.RenderMarkdown(reviewOverviewContent),
	SilenceUsage: true,
	RunE:         runReview,
}

func init() {
	rootCmd.AddCommand(reviewCmd)
	bindReviewFlags(reviewCmd)
}

func bindReviewFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&reviewRepo, "repo", "", "Repository in owner/name format (defaults to git origin remote)")
	cmd.Flags().StringVar(&reviewBase, "base", "", "Base ref or SHA to diff against (defaults to merge-base with origin/HEAD)")
	cmd.Flags().StringVar(&reviewMinSeverity, "min-severity", "", "Minimum severity to surface: low|medium|high|critical")
	cmd.Flags().StringArrayVar(&reviewExcludes, "exclude", nil, "Extra path glob(s) to exclude from the patch (repeatable)")
	cmd.Flags().StringArrayVar(&reviewIncludes, "include", nil, "Cancel a default skip for matching files (repeatable)")
	cmd.Flags().BoolVar(&reviewJSON, "json", false, "Write the result as JSON (to stdout or --output)")
	cmd.Flags().StringVar(&reviewOutput, "output", "", "Write the result to a file instead of stdout")
	cmd.Flags().BoolVar(&reviewQuiet, "quiet", false, "Suppress stderr progress output")
	cmd.Flags().SortFlags = false
}

// setupReviewCloud resolves auth (JWT or API key) and returns a client.
// Mirrors setupUnitCloud.
func setupReviewCloud() (*api.TuskClient, api.AuthOptions, error) {
	client, authOptions, _, err := api.SetupCloud(context.Background(), false)
	if err != nil {
		return nil, api.AuthOptions{}, err
	}
	return client, authOptions, nil
}

// resolveReviewRepo returns (owner, name). If repoFlag is set, it's parsed
// as "owner/name"; otherwise the origin remote is used.
func resolveReviewRepo(repoFlag string) (string, string, error) {
	slug := repoFlag
	if slug == "" {
		detected, err := getOriginRepoSlug()
		if err != nil {
			return "", "", err
		}
		slug = detected
	}
	parts := strings.Split(slug, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repo %q; expected owner/name", slug)
	}
	return parts[0], parts[1], nil
}

func runReview(cmd *cobra.Command, args []string) error {
	setupSignalHandling()

	ctx := context.Background()

	log.Debug("Starting tusk review",
		"repo", reviewRepo,
		"base", reviewBase,
		"min-severity", reviewMinSeverity,
		"json", reviewJSON,
		"output", reviewOutput,
		"quiet", reviewQuiet,
	)

	repoRoot, err := review.RepoRoot()
	if err != nil {
		return &ExitCodeError{Code: 2, Err: err}
	}

	if err := review.Preflight(repoRoot); err != nil {
		if review.IsPreflightError(err) {
			return &ExitCodeError{Code: 2, Err: err}
		}
		return err
	}

	owner, name, err := resolveReviewRepo(reviewRepo)
	if err != nil {
		return &ExitCodeError{Code: 2, Err: err}
	}

	patch, err := review.BuildPatch(ctx, review.PatchOptions{
		RepoRoot:        repoRoot,
		Base:            reviewBase,
		ExtraExcludes:   reviewExcludes,
		Includes:        reviewIncludes,
		RegisterCleanup: RegisterCleanup,
	})
	if err != nil {
		if errors.Is(err, review.ErrEmptyPatch) {
			log.Println("Nothing to review: all changed files were filtered out (lockfiles, build artifacts, etc.).\nPass --include '<glob>' to override the default skip list.")
			return nil
		}
		return mapPatchError(err)
	}

	client, authOptions, err := setupReviewCloud()
	if err != nil {
		return err
	}

	// Quick stderr header for non-TTY callers so they know something's happening
	// even before the first progress poll. The backend will replace this with
	// richer phase text once it starts rendering.
	if !reviewQuiet {
		baseLabel := patch.BaseRef
		if baseLabel == "" {
			baseLabel = patch.BaseSha
		}
		shortSha := patch.BaseSha
		if len(shortSha) > 7 {
			shortSha = shortSha[:7]
		}
		log.Stderrln(fmt.Sprintf("Reviewing %d lines across %d files (base: %s @ %s)",
			patch.ChangedLines, patch.FileCount, baseLabel, shortSha))
	}

	createReq := &backend.CreateLocalCodeReviewRunRequest{
		OwnerName:  owner,
		RepoName:   name,
		BaseSha:    patch.BaseSha,
		Patch:      patch.Patch,
		CliVersion: fmt.Sprintf("tusk-cli/%s", version.Version),
	}
	if reviewMinSeverity != "" {
		s := reviewMinSeverity
		createReq.MinSeverity = &s
	}

	runID, err := client.CreateLocalCodeReviewRun(ctx, createReq, authOptions)
	if err != nil {
		if api.IsRateLimitError(err) {
			return &ExitCodeError{Code: 2, Err: err}
		}
		if api.IsRepoNotFoundError(err) {
			return &ExitCodeError{Code: 2, Err: fmt.Errorf(
				"repo %s/%s is not connected to Tusk under your current org.\n\n"+
					"If this is the repo you meant to review:\n"+
					"  • Connect it at https://app.usetusk.ai/repos\n"+
					"    (installs the GitHub/GitLab app and grants access)\n"+
					"  • Or, if you belong to multiple Tusk orgs, switch:\n"+
					"      tusk auth select-org\n\n"+
					"If this is not the repo you meant:\n"+
					"  • Pass --repo owner/name to target a different connected repo\n"+
					"  • Check whether origin is a fork (git remote -v); you likely\n"+
					"    want the upstream, not the fork",
				owner, name)}
		}
		if api.IsPatchInvalidError(err) {
			return &ExitCodeError{Code: 2, Err: err}
		}
		return formatApiError(err)
	}

	// Cancellation cleanup MUST be registered before the poll loop so that
	// Ctrl+C fires a backend cancel. Keep the timeout short — if the cancel
	// RPC itself hangs, we don't want to block process exit.
	RegisterCleanup(func() {
		cancelCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := client.CancelCodeReviewRun(cancelCtx, &backend.CancelCodeReviewRunRequest{RunId: runID}, authOptions); err != nil {
			log.Debug("Failed to cancel code review run", "runId", runID, "error", err)
			return
		}
		if !reviewQuiet {
			log.Stderrln(fmt.Sprintf("Cancelled run %s.", runID))
		}
	})

	final, err := review.Poll(ctx, client, authOptions, runID, review.PollOptions{
		Quiet: reviewQuiet,
	})
	if err != nil {
		return formatApiError(err)
	}

	if err := writeResult(final, reviewJSON, reviewOutput); err != nil {
		return err
	}

	switch final.GetStatus() {
	case backend.CodeReviewRunStatus_CODE_REVIEW_RUN_STATUS_FAILED:
		// Backend already rendered the failure reason into display_message/
		// display_json which we just wrote to stdout. Bubble up a sentinel
		// error (no duplicate stderr) purely so the process exits with code 1.
		// SilenceErrors prevents Cobra from printing a stray "Error: \n" line
		// for errSilentFail (whose Error() returns ""); normal errors from
		// earlier returns are still printed by Cobra.
		cmd.SilenceErrors = true
		return errSilentFail
	case backend.CodeReviewRunStatus_CODE_REVIEW_RUN_STATUS_CANCELLED:
		return nil
	default:
		return nil
	}
}

// errSilentFail lets runReview signal "exit code 1 but don't print anything"
// — the failure text is already on stdout/stderr via the result writer.
var errSilentFail = &silentErr{}

type silentErr struct{}

func (*silentErr) Error() string { return "" }

// mapPatchError turns BuildPatch errors into the right user-facing error and
// exit code. Plain errors fall through unchanged.
func mapPatchError(err error) error {
	if review.IsBaseResolutionError(err) {
		return &ExitCodeError{Code: 2, Err: err}
	}
	if review.IsPreflightError(err) {
		return &ExitCodeError{Code: 2, Err: err}
	}
	var tooLarge *review.PatchTooLargeError
	if errors.As(err, &tooLarge) {
		return &ExitCodeError{Code: 2, Err: fmt.Errorf("%s\n\n%s", tooLarge.LimitMessage, formatTopContributors(tooLarge.TopFiles))}
	}
	var submodule *review.SubmoduleError
	if errors.As(err, &submodule) {
		lines := []string{"submodule changes are not supported.\n\nFound submodule update(s) in the generated patch:"}
		for _, p := range submodule.Paths {
			lines = append(lines, "  "+p)
		}
		lines = append(lines, "\nCommit submodule updates separately, or exclude them via:\n  tusk review --exclude '<path>/**'")
		return &ExitCodeError{Code: 2, Err: errors.New(strings.Join(lines, "\n"))}
	}
	return err
}

func formatTopContributors(files []review.FileSummary) string {
	if len(files) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Top contributors:\n")
	for _, f := range files {
		total := f.AddedLines + f.DelLines
		fmt.Fprintf(&sb, "  %s  (+%d/-%d, %d lines)\n", f.Path, f.AddedLines, f.DelLines, total)
	}
	sb.WriteString("\nAdd these to .tuskignore or pass --exclude '<glob>' to skip them.")
	return sb.String()
}

// writeResult writes the backend-rendered final output to the selected sink
// (stdout by default, or --output file). JSON mode writes display_json;
// default mode writes display_message.
//
// If the backend did not set display_json (e.g. FAILED with no JSON renderer),
// a minimal CLI-assembled JSON object is written so callers piping to jq
// never receive empty output.
func writeResult(resp *backend.GetCodeReviewRunStatusResponseSuccess, jsonMode bool, outputPath string) error {
	var out *os.File
	var err error
	if outputPath != "" {
		out, err = os.Create(outputPath) //nolint:gosec // user-specified path
		if err != nil {
			return fmt.Errorf("open --output: %w", err)
		}
		defer func() { _ = out.Close() }()
	} else {
		out = os.Stdout
	}

	if jsonMode {
		if resp.GetDisplayJson() != "" {
			_, err := out.WriteString(resp.GetDisplayJson())
			if err != nil {
				return err
			}
			if !strings.HasSuffix(resp.GetDisplayJson(), "\n") {
				_, _ = out.WriteString("\n")
			}
			return nil
		}
		// Fallback: minimal JSON so downstream scripts never see empty output.
		fallback := map[string]any{
			"run_id":  resp.GetRunId(),
			"status":  protoStatusToString(resp.GetStatus()),
			"message": resp.GetDisplayMessage(),
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(fallback)
	}

	msg := resp.GetDisplayMessage()
	if msg == "" {
		msg = fmt.Sprintf("Run %s completed (status: %s).", resp.GetRunId(), protoStatusToString(resp.GetStatus()))
	}
	if _, err := out.WriteString(msg); err != nil {
		return err
	}
	if !strings.HasSuffix(msg, "\n") {
		_, _ = out.WriteString("\n")
	}
	return nil
}

func protoStatusToString(s backend.CodeReviewRunStatus) string {
	switch s {
	case backend.CodeReviewRunStatus_CODE_REVIEW_RUN_STATUS_PENDING:
		return "PENDING"
	case backend.CodeReviewRunStatus_CODE_REVIEW_RUN_STATUS_RUNNING:
		return "RUNNING"
	case backend.CodeReviewRunStatus_CODE_REVIEW_RUN_STATUS_SUCCESS:
		return "SUCCESS"
	case backend.CodeReviewRunStatus_CODE_REVIEW_RUN_STATUS_FAILED:
		return "FAILED"
	case backend.CodeReviewRunStatus_CODE_REVIEW_RUN_STATUS_CANCELLED:
		return "CANCELLED"
	default:
		return "UNKNOWN"
	}
}
