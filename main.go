package main

import (
	"fmt"
	"os"

	"github.com/Use-Tusk/tusk-drift-cli/cmd"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil && !os.IsNotExist(err) {
		fmt.Printf("Error loading .env file: %v\n", err)
		os.Exit(1)
	}

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
