package review

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/mattn/go-isatty"

	"github.com/Use-Tusk/tusk-cli/internal/api"
	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
)

// PollOptions tunes the status-polling loop.
type PollOptions struct {
	// Interval between polls. Default: 2s.
	Interval time.Duration
	// Quiet suppresses all stderr progress output entirely.
	Quiet bool
	// Stderr is where progress lines are written. Defaults to os.Stderr.
	// Primarily for testing.
	Stderr *os.File
}

// spinnerFrames is cycled for TTY spinner animation.
var spinnerFrames = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

// Poll blocks, polling the backend for the given runId, rendering progress
// to stderr as it goes, until the run reaches a terminal status (SUCCESS,
// FAILED, CANCELLED).
//
// TTY mode: animated spinner, message repainted on each tick with \r.
// Non-TTY: one line per change (avoids spamming CI logs).
// Quiet: nothing to stderr; polling still happens so the backend heartbeat
// is kept fresh.
func Poll(ctx context.Context, client *api.TuskClient, auth api.AuthOptions, runId string, opts PollOptions) (*backend.GetCodeReviewRunStatusResponseSuccess, error) {
	if opts.Interval <= 0 {
		opts.Interval = 2 * time.Second
	}
	var stderr io.Writer = os.Stderr
	if opts.Stderr != nil {
		stderr = opts.Stderr
	}
	stderrIsTTY := !opts.Quiet && isatty.IsTerminal(os.Stderr.Fd())

	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()

	var lastMsg string
	frame := 0
	renderedTTY := false

	for {
		resp, err := client.GetCodeReviewRunStatus(ctx,
			&backend.GetCodeReviewRunStatusRequest{RunId: runId}, auth)
		if err != nil {
			// Clear the spinner line before bubbling up so the caller's error
			// text starts on a clean line.
			if renderedTTY {
				_, _ = fmt.Fprint(stderr, "\r\033[K")
			}
			return nil, err
		}

		status := resp.GetStatus()
		msg := resp.GetDisplayMessage()

		// Terminal-status check runs BEFORE rendering: on SUCCESS/FAILED/
		// CANCELLED the backend packs the full final output into
		// display_message, and the caller writes that to stdout via
		// writeResult. Rendering it here as well would duplicate the output
		// on stderr (and \r\033[K can only clear one row, so a multi-line
		// final message leaves visible leftovers).
		if isTerminal(status) {
			if renderedTTY {
				_, _ = fmt.Fprint(stderr, "\r\033[K")
			}
			return resp, nil
		}

		if !opts.Quiet {
			if stderrIsTTY {
				char := spinnerFrames[frame%len(spinnerFrames)]
				frame++
				_, _ = fmt.Fprintf(stderr, "\r\033[K%c %s", char, msg)
				renderedTTY = true
			} else if msg != "" && msg != lastMsg {
				_, _ = fmt.Fprintln(stderr, msg)
				lastMsg = msg
			}
		}

		select {
		case <-ctx.Done():
			if renderedTTY {
				_, _ = fmt.Fprint(stderr, "\r\033[K")
			}
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// isTerminal reports whether the given status value indicates a terminal
// state. UNSPECIFIED is treated as still-running (defensive — server should
// never send it).
func isTerminal(s backend.CodeReviewRunStatus) bool {
	switch s {
	case backend.CodeReviewRunStatus_CODE_REVIEW_RUN_STATUS_SUCCESS,
		backend.CodeReviewRunStatus_CODE_REVIEW_RUN_STATUS_FAILED,
		backend.CodeReviewRunStatus_CODE_REVIEW_RUN_STATUS_CANCELLED:
		return true
	default:
		return false
	}
}
