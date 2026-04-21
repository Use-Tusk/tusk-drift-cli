package cmd

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/Use-Tusk/tusk-cli/internal/log"
	"github.com/Use-Tusk/tusk-cli/internal/review"
	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
)

var reviewStatusCmd = &cobra.Command{
	Use:          "status <run-id>",
	Short:        "Show status of a previously-started code review run",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE:         runReviewStatus,
}

func init() {
	reviewCmd.AddCommand(reviewStatusCmd)

	// Reuse the main review command's --json, --output, --quiet globals so
	// `tusk review status ... --json` behaves identically to `tusk review --json`.
	reviewStatusCmd.Flags().BoolVar(&reviewJSON, "json", false, "Write the result as JSON (to stdout or --output)")
	reviewStatusCmd.Flags().StringVar(&reviewOutput, "output", "", "Write the result to a file instead of stdout")
	reviewStatusCmd.Flags().BoolVar(&reviewQuiet, "quiet", false, "Suppress stderr progress output")
	reviewStatusCmd.Flags().BoolVar(&reviewStatusWatch, "watch", false, "Block on running runs until they reach a terminal state")
	reviewStatusCmd.Flags().SortFlags = false
}

func runReviewStatus(cmd *cobra.Command, args []string) error {
	setupSignalHandling()
	ctx := context.Background()
	runID := args[0]

	log.Debug("tusk review status", "runId", runID, "watch", reviewStatusWatch, "json", reviewJSON)

	client, authOptions, err := setupReviewCloud()
	if err != nil {
		return err
	}

	if reviewStatusWatch {
		// Cancellation cleanup mirrors the main command: if the user hits
		// Ctrl+C while --watch is blocking, cancel the run server-side.
		RegisterCleanup(func() {
			cancelCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := client.CancelCodeReviewRun(cancelCtx, &backend.CancelCodeReviewRunRequest{RunId: runID}, authOptions); err != nil {
				log.Debug("Failed to cancel code review run", "runId", runID, "error", err)
			}
		})

		final, err := review.Poll(ctx, client, authOptions, runID, review.PollOptions{Quiet: reviewQuiet})
		if err != nil {
			return formatApiError(err)
		}
		if err := writeResult(final, reviewJSON, reviewOutput); err != nil {
			return err
		}
		if final.GetStatus() == backend.CodeReviewRunStatus_CODE_REVIEW_RUN_STATUS_FAILED {
			return errSilentFail
		}
		return nil
	}

	// Single snapshot (default). Non-terminal runs exit 0 — the user asked for
	// a snapshot, not a verdict. Pair with --watch to block.
	resp, err := client.GetCodeReviewRunStatus(ctx, &backend.GetCodeReviewRunStatusRequest{RunId: runID}, authOptions)
	if err != nil {
		return formatApiError(err)
	}
	if err := writeResult(resp, reviewJSON, reviewOutput); err != nil {
		return err
	}
	if resp.GetStatus() == backend.CodeReviewRunStatus_CODE_REVIEW_RUN_STATUS_FAILED {
		return errSilentFail
	}
	return nil
}
