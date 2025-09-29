package version

import "fmt"

// Build-time variables (set via ldflags during CI/CD builds)
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

// Minimum SDK version this CLI supports
const MinSDKVersion = "1.0.0"

func PrintVersion() {
	fmt.Printf("Tusk CLI (version: %s)\n", Version)
	if BuildTime != "unknown" {
		fmt.Printf("Build Time: %s\n", BuildTime)
	}
	if GitCommit != "unknown" {
		fmt.Printf("Git Commit: %s\n", GitCommit)
	}
}
