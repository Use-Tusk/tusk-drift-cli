package tools

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// UserTools provides user interaction operations
type UserTools struct{}

// NewUserTools creates a new UserTools instance
func NewUserTools() *UserTools {
	return &UserTools{}
}

var (
	questionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212"))

	inputPromptStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("86"))
)

// Ask prompts the user with a question and returns their response
func (ut *UserTools) Ask(input json.RawMessage) (string, error) {
	var params struct {
		Question string `json:"question"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	fmt.Println()
	fmt.Println(questionStyle.Render("🤖 Agent needs your input:"))
	fmt.Println()
	fmt.Println(params.Question)
	fmt.Print(inputPromptStyle.Render("\n> "))

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}

	return strings.TrimSpace(response), nil
}
