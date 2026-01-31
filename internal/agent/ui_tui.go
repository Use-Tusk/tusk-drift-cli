package agent

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

// TUIUI implements AgentUI using the bubbletea TUI framework
type TUIUI struct {
	ctx     context.Context
	cancel  context.CancelFunc
	model   *TUIModel
	program *tea.Program
}

// NewTUIUI creates a new TUI-based UI
func NewTUIUI(ctx context.Context, cancel context.CancelFunc, phaseNames []string, hideProgressBar bool) *TUIUI {
	model := NewTUIModel(ctx, cancel, phaseNames, hideProgressBar)
	return &TUIUI{
		ctx:    ctx,
		cancel: cancel,
		model:  model,
	}
}

// Start initializes and starts the TUI program
func (u *TUIUI) Start() error {
	u.program = tea.NewProgram(u.model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	return nil
}

// ShowIntro displays the intro screen with animation
func (u *TUIUI) ShowIntro(isProxyMode, skipToCloud, verifyMode bool) (bool, error) {
	return RunIntroScreen(isProxyMode, skipToCloud, verifyMode)
}

// Stop is a no-op for TUI - cleanup happens via the program
func (u *TUIUI) Stop() {
	// TUI cleanup is handled by the program's quit mechanism
}

// Run runs the TUI program and blocks until it exits
// Returns the final model and any error
func (u *TUIUI) Run() (tea.Model, error) {
	return u.program.Run()
}

// PhaseChange notifies the UI of a phase change
func (u *TUIUI) PhaseChange(name, desc string, phaseNum, totalPhases int) {
	u.model.SendPhaseChange(u.program, name, desc, phaseNum, totalPhases)
}

// UpdatePhaseList updates the list of phases (used when cloud phases are added)
func (u *TUIUI) UpdatePhaseList(phaseNames []string) {
	u.model.UpdateTodoItems(u.program, phaseNames)
}

// AgentText displays agent output text
func (u *TUIUI) AgentText(text string, streaming bool) {
	u.model.SendAgentText(u.program, text, streaming)
}

// Thinking shows/hides the thinking indicator
func (u *TUIUI) Thinking(thinking bool) {
	u.model.SendThinking(u.program, thinking)
}

// ToolStart notifies the UI that a tool is starting
func (u *TUIUI) ToolStart(name, input string) {
	u.model.SendToolStart(u.program, name, input)
}

// ToolComplete notifies the UI that a tool has completed
func (u *TUIUI) ToolComplete(name string, success bool, output string) {
	u.model.SendToolComplete(u.program, name, success, output)
}

// SidebarUpdate updates a sidebar info item
func (u *TUIUI) SidebarUpdate(key, value string) {
	u.model.SendSidebarUpdate(u.program, key, value)
}

// Error displays a non-fatal error
func (u *TUIUI) Error(err error) {
	u.model.SendError(u.program, err)
}

// FatalError displays a fatal error and prepares for exit
func (u *TUIUI) FatalError(err error) {
	u.model.SendFatalError(u.program, err)
}

// Completed notifies the UI that setup is complete
func (u *TUIUI) Completed(workDir string) {
	u.model.SendCompleted(u.program, workDir)
}

// EligibilityCompleted notifies the UI that eligibility check is complete
func (u *TUIUI) EligibilityCompleted(workDir string) {
	u.model.SendEligibilityCompleted(u.program, workDir)
}

// Aborted notifies the UI that setup was aborted
func (u *TUIUI) Aborted(reason string) {
	u.model.SendAborted(u.program, reason)
}

// PromptUserInput prompts the user for text input
func (u *TUIUI) PromptUserInput(question string) (string, bool) {
	response := u.model.RequestUserInput(u.program, question)
	return response, response == ""
}

// PromptUserSelect prompts the user to select from options
func (u *TUIUI) PromptUserSelect(question string, options []SelectOption) (string, string, bool) {
	selectedID := u.model.RequestUserSelect(u.program, question, options)
	if selectedID == "" {
		return "", "", true
	}

	// Find the label for the selected option
	var selectedLabel string
	for _, opt := range options {
		if opt.ID == selectedID {
			selectedLabel = opt.Label
			break
		}
	}
	return selectedID, selectedLabel, false
}

// PromptPermission asks the user for permission to execute a tool
func (u *TUIUI) PromptPermission(toolName, preview string) string {
	return u.model.RequestPermission(u.program, toolName, preview)
}

// PromptKillPort asks the user if they want to kill a process on a port
func (u *TUIUI) PromptKillPort(port int) bool {
	return u.model.RequestPortConflict(u.program, port)
}

// PromptRerun asks the user if they want to rerun setup
func (u *TUIUI) PromptRerun() (bool, bool) {
	responseCh := make(chan bool)
	u.model.SendRerunConfirm(u.program, responseCh)

	select {
	case rerun := <-responseCh:
		return rerun, false
	case <-u.ctx.Done():
		return false, true
	}
}

// PromptCloudSetup asks the user if they want to continue with cloud setup
func (u *TUIUI) PromptCloudSetup() (bool, bool) {
	responseCh := make(chan bool)
	u.model.SendCloudSetupPrompt(u.program, responseCh)

	select {
	case continueCloud := <-responseCh:
		return continueCloud, false
	case <-u.ctx.Done():
		return false, true
	}
}

// GetFinalOutput returns the final output to display after the TUI exits
func (u *TUIUI) GetFinalOutput() string {
	return u.model.GetFinalOutput()
}
