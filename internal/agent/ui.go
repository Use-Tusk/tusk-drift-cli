package agent

import "context"

// AgentUI abstracts the user interface for the agent, allowing both TUI and headless modes
// to share the same core agent logic. All methods are blocking and return results directly.
type AgentUI interface {
	// Lifecycle
	Start() error // Initialize the UI (e.g., start TUI program)
	Stop()        // Clean up the UI

	// ShowIntro displays the intro screen with animation and description.
	// Returns true if user wants to continue, false if they quit.
	// isProxyMode determines whether to show the proxy mode note.
	// skipToCloud determines whether to show the cloud-only phases description.
	ShowIntro(isProxyMode, skipToCloud bool) (bool, error)

	// Phase updates
	PhaseChange(name, desc string, phaseNum, totalPhases int)
	UpdatePhaseList(phaseNames []string)

	// Agent output
	AgentText(text string, streaming bool)
	Thinking(thinking bool)

	// Tool execution feedback
	ToolStart(name, input string)
	ToolComplete(name string, success bool, output string)

	// Status updates
	SidebarUpdate(key, value string)
	Error(err error)
	FatalError(err error)
	Completed(workDir string)
	EligibilityCompleted(workDir string)
	Aborted(reason string)

	// Interactive prompts - all blocking, return results directly
	// Returns (response, cancelled)
	PromptUserInput(question string) (string, bool)
	// Returns (selectedID, selectedLabel, cancelled)
	PromptUserSelect(question string, options []SelectOption) (string, string, bool)
	// Returns "approve", "approve_all", "deny", or "deny:<alternative>"
	PromptPermission(toolName, preview string) string
	// Returns true if user wants to kill the process
	PromptKillPort(port int) bool
	// Returns (rerun, cancelled)
	PromptRerun() (bool, bool)
	// Returns (continueWithCloud, cancelled)
	PromptCloudSetup() (bool, bool)

	// GetFinalOutput returns any final output to display after UI stops (TUI only)
	GetFinalOutput() string
}

// NewAgentUI creates the appropriate UI implementation based on the mode
func NewAgentUI(ctx context.Context, cancel context.CancelFunc, headless bool, phaseNames []string, hideProgressBar bool) AgentUI {
	if headless {
		return NewHeadlessUI()
	}
	return NewTUIUI(ctx, cancel, phaseNames, hideProgressBar)
}
