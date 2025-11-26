package main

import (
	"os"

	"github.com/Use-Tusk/tusk-drift-cli/cmd"
)

func main() {
	err := cmd.Execute()

	// Track command result and close tracker
	// Must happen after Execute but before exit
	if tracker := cmd.GetTracker(); tracker != nil {
		tracker.TrackResult(err)
		tracker.Close()
	}

	if err != nil {
		os.Exit(1)
	}
}
