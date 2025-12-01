package analytics

import (
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/cliconfig"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Tracker handles analytics for a single command execution
type Tracker struct {
	client    *Client
	startTime time.Time
	command   string
	flags     []string
}

// NewTracker creates a tracker for the given command
// Returns nil if analytics is disabled
func NewTracker(cmd *cobra.Command) *Tracker {
	if !cliconfig.IsAnalyticsEnabled() {
		return nil
	}

	// Check if we need to show first-run notice (before creating client)
	firstRun := ShowFirstRunNotice()

	client := NewClient()
	if client == nil {
		return nil
	}

	// Track first run event if notice was just shown
	if firstRun {
		client.Track("drift_cli:first_run", nil)
	}

	return &Tracker{
		client:    client,
		startTime: time.Now(),
		command:   getCommandPath(cmd),
		flags:     getUsedFlags(cmd),
	}
}

// TrackResult records the command result (call after Execute returns)
func (t *Tracker) TrackResult(err error) {
	if t == nil || t.client == nil {
		return
	}

	duration := time.Since(t.startTime)
	exitCode := 0
	var errorType string

	if err != nil {
		exitCode = 1
		errorType = CategorizeError(err)
	}

	props := map[string]any{
		"command":     t.command,
		"flags":       t.flags,
		"success":     err == nil,
		"exit_code":   exitCode,
		"duration_ms": duration.Milliseconds(),
	}

	if errorType != "" {
		props["error_type"] = errorType
	}

	t.client.Track("drift_cli:command_executed", props)
}

// Track sends a custom event (delegates to client)
func (t *Tracker) Track(event string, props map[string]any) {
	if t == nil || t.client == nil {
		return
	}
	t.client.Track(event, props)
}

// Alias connects anonymous ID to user ID (call after login)
func (t *Tracker) Alias(userID string) {
	if t == nil || t.client == nil {
		return
	}
	t.client.Alias(userID)
}

// Close flushes and closes the analytics client
func (t *Tracker) Close() {
	if t == nil || t.client == nil {
		return
	}
	_ = t.client.Close() // Errors ignored - analytics should never break the CLI
}

// getCommandPath returns the full command path (e.g., "login select-client")
func getCommandPath(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}

	// Build path from root to current command
	path := cmd.Name()
	for p := cmd.Parent(); p != nil && p.Name() != "tusk"; p = p.Parent() {
		path = p.Name() + " " + path
	}
	return path
}

// getUsedFlags returns a list of flag names that were set (not values)
func getUsedFlags(cmd *cobra.Command) []string {
	if cmd == nil {
		return nil
	}

	var flags []string
	cmd.Flags().Visit(func(f *pflag.Flag) {
		flags = append(flags, "--"+f.Name)
	})
	return flags
}
