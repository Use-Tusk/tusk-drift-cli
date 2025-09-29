package utils

import (
	"os"

	"github.com/atotto/clipboard"
	osc52 "github.com/aymanbagabas/go-osc52/v2"
	"github.com/mattn/go-isatty"
)

func IsTerminal() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
}

// CopyToClipboard copies text to the system clipboard.
// It tries OSC52 first (for remote terminals), then falls back to OS clipboard.
func CopyToClipboard(text string) error {
	if IsTerminal() {
		_, err := osc52.New(text).WriteTo(os.Stdout)
		return err
	}
	return clipboard.WriteAll(text)
}
