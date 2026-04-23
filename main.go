package main

import (
	"os"

	"github.com/Use-Tusk/tusk-cli/cmd"
	"github.com/Use-Tusk/tusk-cli/internal/analytics"
)

func main() {
	err := cmd.Execute()

	// Track command result and close tracker
	// Must happen after Execute but before exit
	analytics.GlobalTracker.TrackResult(err)
	analytics.GlobalTracker.Close()

	if err != nil {
		os.Exit(cmd.ExitCodeOf(err))
	}
}
