package main

import (
	"os"

	"github.com/Use-Tusk/tusk-drift-cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
