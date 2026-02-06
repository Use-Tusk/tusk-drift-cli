package utils

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	osc52 "github.com/aymanbagabas/go-osc52/v2"
	"github.com/mattn/go-isatty"
)

func IsTerminal() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
}

// TUICIMode returns true if TUSK_TUI_CI_MODE=1 is set.
// This enables CI-friendly TUI mode: forces TUI without a TTY,
// skips terminal size warnings, and auto-exits on completion.
func TUICIMode() bool {
	return os.Getenv("TUSK_TUI_CI_MODE") == "1"
}

// CopyToClipboard copies text to the system clipboard.
// It tries OSC52 first (for remote terminals), then falls back to OS clipboard.
func CopyToClipboard(text string) error {
	if IsTerminal() {
		_, err := osc52.New(text).WriteTo(os.Stdout)
		return err
	}
	return WriteToSystemClipboard(text)
}

// WriteToSystemClipboard writes text to the OS clipboard with proper UTF-8 encoding.
// On macOS, it sets __CF_USER_TEXT_ENCODING to force pbcopy to interpret input as UTF-8
// (the default is Mac OS Roman, which garbles multi-byte Unicode characters).
func WriteToSystemClipboard(text string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("__CF_USER_TEXT_ENCODING=0x%X:0:0x08000100", os.Getuid()),
		)
	case "linux":
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return fmt.Errorf("no clipboard utility found (install xclip or xsel)")
		}
	default:
		return fmt.Errorf("clipboard not supported on %s", runtime.GOOS)
	}

	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// PromptUserChoice displays a question with numbered options and returns the index of the selected option.
// Returns 0 if input is invalid or empty (defaults to first option).
func PromptUserChoice(question string, options []string) int {
	fmt.Println(question)
	for i, opt := range options {
		fmt.Printf("  [%d] %s\n", i+1, opt)
	}
	fmt.Print("Enter choice (1-", len(options), "): ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return 0
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return 0
	}

	choice, err := strconv.Atoi(input)
	if err != nil || choice < 1 || choice > len(options) {
		return 0
	}

	return choice - 1
}
