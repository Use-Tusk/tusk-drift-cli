package main

import (
	"os"

	"github.com/Use-Tusk/tusk-drift-cli/cmd"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
