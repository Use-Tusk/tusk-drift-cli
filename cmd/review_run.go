package cmd

import (
	_ "embed"

	"github.com/spf13/cobra"

	"github.com/Use-Tusk/tusk-cli/internal/utils"
)

//go:embed short_docs/review/run.md
var reviewRunContent string

var reviewRunCmd = &cobra.Command{
	Use:          "run",
	Short:        "Submit a code review for your local working tree",
	Long:         utils.RenderMarkdown(reviewRunContent),
	SilenceUsage: true,
	RunE:         runReview,
}

func init() {
	reviewCmd.AddCommand(reviewRunCmd)
	bindReviewFlags(reviewRunCmd)
}
