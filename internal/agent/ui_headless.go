package agent

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Styles for headless output
var (
	headlessPhaseStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("212"))

	headlessToolStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("81"))

	headlessSuccessStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("82"))

	headlessErrorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196"))

	headlessDimStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245"))

	headlessQuestionStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("212"))

	headlessInputStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("86"))
)

// HeadlessUI implements AgentUI for terminal output without TUI
type HeadlessUI struct {
	reader        *bufio.Reader
	isThinking    bool
	currentPhase  string
	phasesTotal   int
	phasesCurrent int
}

// NewHeadlessUI creates a new headless UI
func NewHeadlessUI() *HeadlessUI {
	return &HeadlessUI{
		reader: bufio.NewReader(os.Stdin),
	}
}

// Start displays the headless mode header
func (u *HeadlessUI) Start() error {
	fmt.Println(headlessPhaseStyle.Render("• TUSK DRIFT AUTO SETUP (Headless Mode) •"))
	fmt.Println()
	return nil
}

// ShowIntro displays the intro screen for headless mode (no confirmation for scripts)
func (u *HeadlessUI) ShowIntro() (bool, error) {
	PrintIntroHeadless()
	return true, nil
}

// Stop is a no-op for headless mode
func (u *HeadlessUI) Stop() {}

// PhaseChange displays a phase change header
func (u *HeadlessUI) PhaseChange(name, desc string, phaseNum, totalPhases int) {
	u.currentPhase = name
	u.phasesCurrent = phaseNum
	u.phasesTotal = totalPhases

	fmt.Println()
	fmt.Println(headlessPhaseStyle.Render(fmt.Sprintf("━━━ Phase %d/%d: %s ━━━", phaseNum, totalPhases, name)))
	fmt.Println(headlessDimStyle.Render(desc))
	fmt.Println()
}

// UpdatePhaseList is a no-op for headless mode (no visual phase list)
func (u *HeadlessUI) UpdatePhaseList(phaseNames []string) {
	u.phasesTotal = len(phaseNames)
}

// AgentText displays agent output text
func (u *HeadlessUI) AgentText(text string, streaming bool) {
	// In headless mode, skip streaming updates - only print final text
	if streaming {
		return
	}

	// Clear thinking indicator if showing
	if u.isThinking {
		fmt.Print("\r                    \r")
		u.isThinking = false
	}
	if strings.TrimSpace(text) != "" {
		fmt.Println(text)
		fmt.Println()
	}
}

// Thinking shows/hides the thinking indicator
func (u *HeadlessUI) Thinking(thinking bool) {
	if thinking && !u.isThinking {
		fmt.Print(headlessDimStyle.Render("○ Thinking..."))
		u.isThinking = true
	} else if !thinking && u.isThinking {
		fmt.Print("\r                    \r")
		u.isThinking = false
	}
}

// ToolStart displays tool start notification
func (u *HeadlessUI) ToolStart(name, input string) {
	// Clear thinking indicator if showing
	if u.isThinking {
		fmt.Print("\r                    \r")
		u.isThinking = false
	}

	// Skip internal tools
	if name == "transition_phase" {
		return
	}

	displayName := getToolDisplayName(name)
	fmt.Println(headlessToolStyle.Render(fmt.Sprintf("🔧 %s", displayName)))
}

// ToolComplete displays tool completion status
func (u *HeadlessUI) ToolComplete(name string, success bool, output string) {
	// Skip internal tools
	if name == "transition_phase" {
		return
	}

	displayName := getToolDisplayName(name)
	if success {
		fmt.Println(headlessSuccessStyle.Render(fmt.Sprintf("   ✓ %s", displayName)))
	} else {
		fmt.Println(headlessErrorStyle.Render(fmt.Sprintf("   ✗ %s", output)))
	}
}

// SidebarUpdate is a no-op for headless mode (no sidebar)
func (u *HeadlessUI) SidebarUpdate(key, value string) {}

// Error displays a non-fatal error
func (u *HeadlessUI) Error(err error) {
	fmt.Println(headlessErrorStyle.Render(fmt.Sprintf("⚠️ Warning: %s", err.Error())))
}

// FatalError displays a fatal error
func (u *HeadlessUI) FatalError(err error) {
	fmt.Println(headlessErrorStyle.Render(fmt.Sprintf("❌ Error: %s", err.Error())))
	fmt.Println()
	fmt.Println(RecoveryGuidance())
}

// Completed displays completion message
func (u *HeadlessUI) Completed(workDir string) {
	fmt.Println()
	fmt.Println(headlessSuccessStyle.Render("🎉 Setup complete!"))
	fmt.Println(headlessDimStyle.Render("   Check .tusk/SETUP_REPORT.md for details."))
}

// EligibilityCompleted displays eligibility check completion message
func (u *HeadlessUI) EligibilityCompleted(workDir string) {
	fmt.Println()
	fmt.Println(headlessSuccessStyle.Render("✅ Eligibility check complete!"))
	fmt.Println(headlessDimStyle.Render("   Check .tusk/eligibility-report.json for details."))
}

// Aborted displays abort message
func (u *HeadlessUI) Aborted(reason string) {
	fmt.Println()
	fmt.Println(headlessErrorStyle.Render("🟠 Setup aborted. See message above for details."))
}

// PromptUserInput prompts the user for text input
func (u *HeadlessUI) PromptUserInput(question string) (string, bool) {
	fmt.Println()
	fmt.Println(headlessQuestionStyle.Render("🤖 Agent needs your input:"))
	fmt.Println()
	fmt.Println(question)
	fmt.Print(headlessInputStyle.Render("\n> "))

	response, err := u.reader.ReadString('\n')
	if err != nil {
		return "", true
	}

	response = strings.TrimSpace(response)
	if response == "" {
		return "", true
	}

	fmt.Println()
	return response, false
}

// PromptUserSelect prompts the user to select from options
func (u *HeadlessUI) PromptUserSelect(question string, options []SelectOption) (string, string, bool) {
	fmt.Println()
	fmt.Println(headlessQuestionStyle.Render("🤖 Agent needs your selection:"))
	fmt.Println()
	fmt.Println(question)
	fmt.Println()
	for i, opt := range options {
		fmt.Printf("  [%d] %s\n", i+1, opt.Label)
	}
	fmt.Print(headlessInputStyle.Render(fmt.Sprintf("\nEnter number (1-%d): ", len(options))))

	response, err := u.reader.ReadString('\n')
	if err != nil {
		return "", "", true
	}

	response = strings.TrimSpace(response)
	var selection int
	if _, err := fmt.Sscanf(response, "%d", &selection); err != nil || selection < 1 || selection > len(options) {
		return "", "", true
	}

	selected := options[selection-1]
	fmt.Println(headlessDimStyle.Render(fmt.Sprintf("   Selected: %s", selected.Label)))
	fmt.Println()

	return selected.ID, selected.Label, false
}

// PromptPermission asks the user for permission to execute a tool
func (u *HeadlessUI) PromptPermission(toolName, preview string) string {
	if preview != "" {
		fmt.Println(headlessDimStyle.Render(fmt.Sprintf("   %s", preview)))
	}
	fmt.Print(headlessQuestionStyle.Render("   Allow? [y/n/a(ll)]: "))

	response, err := u.reader.ReadString('\n')
	if err != nil {
		return "deny"
	}

	response = strings.TrimSpace(strings.ToLower(response))

	switch response {
	case "y", "yes":
		return "approve"
	case "a", "all":
		return "approve_all"
	default:
		fmt.Println(headlessDimStyle.Render("   ✗ Denied"))
		return "deny"
	}
}

// PromptKillPort asks the user if they want to kill a process on a port
func (u *HeadlessUI) PromptKillPort(port int) bool {
	fmt.Println(headlessErrorStyle.Render(fmt.Sprintf("⚠️  Port %d is already in use", port)))
	fmt.Print("Kill process on port? [y/N]: ")

	response, err := u.reader.ReadString('\n')
	if err != nil {
		return false
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response == "y" || response == "yes" {
		fmt.Println(headlessDimStyle.Render("   Killing process..."))
		return true
	}
	return false
}

// PromptRerun asks the user if they want to rerun setup
func (u *HeadlessUI) PromptRerun() (bool, bool) {
	fmt.Println(headlessSuccessStyle.Render("✅ Tusk Drift setup is already complete!"))
	fmt.Println()
	fmt.Print("Would you like to rerun the setup from scratch? [y/N]: ")

	response, err := u.reader.ReadString('\n')
	if err != nil {
		return false, true
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes", false
}

// PromptCloudSetup asks the user if they want to continue with cloud setup
func (u *HeadlessUI) PromptCloudSetup() (bool, bool) {
	fmt.Println()
	fmt.Println("───────────────────────────────────────────────────────────")
	fmt.Println()
	fmt.Println(headlessSuccessStyle.Render("✅ Local setup complete!"))
	fmt.Println()
	fmt.Println("Would you like to continue with Tusk Drift Cloud setup?")
	fmt.Println("This will connect your repository and enable cloud features.")
	fmt.Println()
	fmt.Print("Continue with cloud setup? [y/N]: ")

	response, err := u.reader.ReadString('\n')
	if err != nil {
		return false, true
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes", false
}

// GetFinalOutput returns empty string for headless mode
func (u *HeadlessUI) GetFinalOutput() string {
	return ""
}
