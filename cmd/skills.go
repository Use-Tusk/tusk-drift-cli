package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var skillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "Install Tusk agent skills",
	Long:  `Install Tusk agent skills using the skills package manager. Requires Node.js (npx) to be installed.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		npxPath, err := exec.LookPath("npx")
		if err != nil {
			return fmt.Errorf("npx not found: please install Node.js (https://nodejs.org/) to use this command")
		}

		c := exec.Command(npxPath, "skills", "add", "Use-Tusk/tusk-skills")
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	},
}

func init() {
	rootCmd.AddCommand(skillsCmd)
}
