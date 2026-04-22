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
//
// Spinner animation and backend polling are intentionally decoupled: the
// spinner ticks on its own ~100ms schedule for smooth visual feedback, while
// the backend is hit only every Interval seconds. A stale spinner frame
// remains smooth to the eye; a stale backend poll costs money.
type PollOptions struct {
	// Interval between backend status polls. Default: 5s.
	//
	// Must stay well under the backend's heartbeat-abandonment window
	// (currently 5 minutes) so live runs don't get reaped.
	Interval time.Duration
	// SpinnerInterval controls how often the TTY spinner redraws. Ignored
	// when stderr is not a TTY or when Quiet is set. Default: 100ms.
	SpinnerInterval time.Duration
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
// TTY mode: animated spinner redraws every ~100ms using the most recent
// display_message; backend is hit every Interval.
// Non-TTY: one line per message change (avoids spamming CI logs).
// Quiet: nothing to stderr; polling still happens so the backend heartbeat
// is kept fresh.
func Poll(ctx context.Context, client *api.TuskClient, auth api.AuthOptions, runId string, opts PollOptions) (*backend.GetCodeReviewRunStatusResponseSuccess, error) {
	if opts.Interval <= 0 {
		opts.Interval = 5 * time.Second
	}
	if opts.SpinnerInterval <= 0 {
		opts.SpinnerInterval = 100 * time.Millisecond
	}

	// TTY detection must track whichever file descriptor we're actually
	// writing to — checking os.Stderr while writing to an overridden
	// opts.Stderr (e.g. a test pipe) would emit spinner escape sequences
	// into the wrong destination.
	stderrFile := os.Stderr
	if opts.Stderr != nil {
		stderrFile = opts.Stderr
	}
	var stderr io.Writer = stderrFile
	stderrIsTTY := !opts.Quiet && isatty.IsTerminal(stderrFile.Fd())

	// Cached state shared between the poll ticker (writer) and the spinner
	// ticker (reader). Both live on the same goroutine so no mutex needed.
	var (
		latestMsg     string // most recent display_message from the backend
		lastLoggedMsg string // last message we actually printed in non-TTY mode (dedup)
		frame         int
		renderedTTY   bool
	)

	clearSpinner := func() {
		if renderedTTY {
			_, _ = fmt.Fprint(stderr, "\r\033[K")
		}
	}

	fetchOnce := func() (*backend.GetCodeReviewRunStatusResponseSuccess, error) {
		return client.GetCodeReviewRunStatus(ctx,
			&backend.GetCodeReviewRunStatusRequest{RunId: runId}, auth)
	}

	// Initial poll so the very first spinner frame shows a real message
	// rather than an empty string for the entire first Interval.
	resp, err := fetchOnce()
	if err != nil {
		return nil, err
	}
	if isTerminal(resp.GetStatus()) {
		return resp, nil
	}
	latestMsg = resp.GetDisplayMessage()

	// Paint the first frame (TTY) or first line (non-TTY) immediately so the
	// user sees something before the first spinner/poll tick fires.
	if !opts.Quiet {
		if stderrIsTTY {
			_, _ = fmt.Fprintf(stderr, "\r\033[K%c %s",
				spinnerFrames[frame%len(spinnerFrames)], latestMsg)
			renderedTTY = true
		} else if latestMsg != "" && latestMsg != lastLoggedMsg {
			_, _ = fmt.Fprintln(stderr, latestMsg)
			lastLoggedMsg = latestMsg
		}
	}

	pollTicker := time.NewTicker(opts.Interval)
	defer pollTicker.Stop()

	// Spinner channel only wired up when we're actually going to animate —
	// a nil channel blocks forever in select, which is what we want for
	// non-TTY / quiet mode.
	var spinnerC <-chan time.Time
	if stderrIsTTY {
		spinnerTicker := time.NewTicker(opts.SpinnerInterval)
		defer spinnerTicker.Stop()
		spinnerC = spinnerTicker.C
	}

	for {
		select {
		case <-ctx.Done():
			clearSpinner()
			return nil, ctx.Err()

		case <-spinnerC:
			// Redraw with the cached latest message. Cheap local tick —
			// no backend call. This is why we can afford 100ms cadence.
			frame++
			_, _ = fmt.Fprintf(stderr, "\r\033[K%c %s",
				spinnerFrames[frame%len(spinnerFrames)], latestMsg)
			renderedTTY = true

		case <-pollTicker.C:
			resp, err := fetchOnce()
			if err != nil {
				clearSpinner()
				return nil, err
			}

			// Terminal-status check before any render, same reasoning as the
			// initial poll: on SUCCESS/FAILED/CANCELLED the backend packs the
			// full output into display_message, which the caller writes to
			// stdout. Rendering it to stderr would duplicate it.
			if isTerminal(resp.GetStatus()) {
				clearSpinner()
				return resp, nil
			}

			latestMsg = resp.GetDisplayMessage()

			// Non-TTY mode prints only on change — there's no spinner ticker
			// to pick up message updates. TTY mode just updates the cache
			// and lets the spinner ticker redraw at its own cadence.
			if !opts.Quiet && !stderrIsTTY {
				if latestMsg != "" && latestMsg != lastLoggedMsg {
					_, _ = fmt.Fprintln(stderr, latestMsg)
					lastLoggedMsg = latestMsg
				}
			}
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
