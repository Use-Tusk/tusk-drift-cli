package utils

import (
	"fmt"
	"os"
	"os/exec"
)

// CIWarning emits a warning annotation visible in the CI provider's UI.
// On providers without annotation support, this is a no-op — the caller
// should log the message separately via log.Stderrln or similar.
//
// Supported providers:
//   - GitHub Actions: ::warning:: annotation (shows in workflow summary)
//   - Azure Pipelines: ##vso[task.logissue] (shows in pipeline summary)
//   - Buildkite: buildkite-agent annotate --style warning (shows on build page)
//   - GitLab CI: ANSI yellow text (visible in job log, no annotation UI)
func CIWarning(message string) {
	switch {
	case os.Getenv("GITHUB_ACTIONS") == "true":
		// https://docs.github.com/en/actions/reference/workflow-commands#setting-a-warning-message
		fmt.Fprintf(os.Stderr, "::warning::%s\n", message)

	case os.Getenv("TF_BUILD") == "True":
		// https://learn.microsoft.com/en-us/azure/devops/pipelines/scripts/logging-commands
		fmt.Fprintf(os.Stderr, "##vso[task.logissue type=warning]%s\n", message)

	case os.Getenv("BUILDKITE") == "true":
		// https://buildkite.com/docs/agent/v3/cli-annotate
		cmd := exec.Command("buildkite-agent", "annotate", message, "--style", "warning")
		_ = cmd.Run() // best-effort, ignore errors

	case os.Getenv("GITLAB_CI") == "true":
		// GitLab has no annotation API — use ANSI yellow to stand out in job logs
		fmt.Fprintf(os.Stderr, "\033[33mWarning: %s\033[0m\n", message)
	}
}
