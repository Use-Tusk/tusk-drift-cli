package analytics

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestGetCommandPath(t *testing.T) {
	// Create a command hierarchy: tusk -> login -> select-client
	rootCmd := &cobra.Command{Use: "tusk"}
	loginCmd := &cobra.Command{Use: "login"}
	selectClientCmd := &cobra.Command{Use: "select-client"}

	rootCmd.AddCommand(loginCmd)
	loginCmd.AddCommand(selectClientCmd)

	tests := []struct {
		name     string
		cmd      *cobra.Command
		expected string
	}{
		{
			name:     "root command",
			cmd:      rootCmd,
			expected: "tusk",
		},
		{
			name:     "subcommand",
			cmd:      loginCmd,
			expected: "login",
		},
		{
			name:     "nested subcommand",
			cmd:      selectClientCmd,
			expected: "login select-client",
		},
		{
			name:     "nil command",
			cmd:      nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getCommandPath(tt.cmd)
			if result != tt.expected {
				t.Errorf("getCommandPath() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetUsedFlags(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool("debug", false, "debug mode")
	cmd.Flags().String("config", "", "config file")
	cmd.Flags().Int("count", 0, "count")

	// Set some flags (simulating user input)
	_ = cmd.Flags().Set("debug", "true")
	_ = cmd.Flags().Set("config", "/path/to/config")
	// count is not set

	flags := getUsedFlags(cmd)

	// Should only include flags that were set
	if len(flags) != 2 {
		t.Errorf("expected 2 flags, got %d: %v", len(flags), flags)
	}

	// Check flag names (not values)
	hasDebug := false
	hasConfig := false
	for _, f := range flags {
		if f == "--debug" {
			hasDebug = true
		}
		if f == "--config" {
			hasConfig = true
		}
		// Should never contain values
		if f == "true" || f == "/path/to/config" {
			t.Errorf("flag list should not contain values: %v", flags)
		}
	}

	if !hasDebug {
		t.Error("expected --debug in flags")
	}
	if !hasConfig {
		t.Error("expected --config in flags")
	}
}

func TestGetUsedFlagsNil(t *testing.T) {
	flags := getUsedFlags(nil)
	if flags != nil {
		t.Errorf("expected nil for nil command, got %v", flags)
	}
}

func TestTrackerNilSafety(t *testing.T) {
	// All methods should be safe to call on nil tracker
	var tracker *Tracker

	// These should not panic
	tracker.TrackResult(nil)
	tracker.Track("test", nil)
	tracker.Alias("user123")
	tracker.Close()
}

func TestNewTrackerWhenDisabled(t *testing.T) {
	// Disable analytics via env var
	t.Setenv("TUSK_ANALYTICS_DISABLED", "1")

	cmd := &cobra.Command{Use: "test"}
	tracker := NewTracker(cmd)

	if tracker != nil {
		t.Error("NewTracker() should return nil when analytics is disabled")
		tracker.Close()
	}
}
