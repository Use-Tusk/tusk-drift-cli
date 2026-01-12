package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"

	agenttools "github.com/Use-Tusk/tusk-drift-cli/internal/agent/tools"
	"github.com/Use-Tusk/tusk-drift-cli/internal/analytics"
)

const progressFileName = "PROGRESS.md"

// Timeouts
const (
	PhaseTimeout         = 15 * time.Minute // Max time per phase
	APITimeout           = 5 * time.Minute  // Max time for a single API call
	ToolTimeout          = 3 * time.Minute  // Max time for a single tool execution
	DefaultMaxIterations = 50               // Default max iterations if phase doesn't specify
	MaxTotalTokens       = 500000           // Max total tokens before warning
	MaxAPIRetries        = 3                // Max retries for API errors
)

// Error messages
const (
	ErrMaxIterations = "exceeded maximum iterations without completing phase"
	ErrMaxTokens     = "exceeded maximum token usage"
)

// RecoveryGuidance returns a message explaining how to resume after a failure
func RecoveryGuidance() string {
	return `Progress has been saved to .tusk/PROGRESS.md
Run 'tusk exp setup' again to continue where you left off.

If issues persist, contact support@usetusk.ai`
}

// Agent orchestrates the AI-powered setup process
type Agent struct {
	client         *ClaudeClient
	allTools       []Tool
	executors      map[string]ToolExecutor
	phaseManager   *PhaseManager
	processManager *ProcessManager
	workDir        string
	totalTokensIn  int
	totalTokensOut int

	skipPermissions bool
	disableProgress bool
	skipToCloud     bool
	printMode       bool
	eligibilityOnly bool

	// UI abstraction (TUI / headless)
	ui     AgentUI
	ctx    context.Context
	cancel context.CancelFunc

	// Logging
	logger      *AgentLogger
	finalStatus CloseStatus
	finalError  error

	// Analytics
	tracker   *analytics.Tracker
	startTime time.Time
	sessionID string
}

// New creates a new Agent
func New(cfg Config) (*Agent, error) {
	client, err := NewClaudeClient(cfg.APIKey, cfg.Model)
	if err != nil {
		return nil, err
	}

	pm := NewProcessManager(cfg.WorkDir)
	phaseMgr := NewPhaseManager()

	// If skipping to cloud, start with cloud phases only
	if cfg.SkipToCloud {
		phaseMgr.SetCloudOnlyMode()
	}

	// If eligibility-only mode, use only the eligibility check phase
	if cfg.EligibilityOnly {
		phaseMgr.SetEligibilityOnlyMode()
	}

	tools, executors := RegisterTools(cfg.WorkDir, pm, phaseMgr)

	a := &Agent{
		client:          client,
		allTools:        tools,
		executors:       executors,
		phaseManager:    phaseMgr,
		processManager:  pm,
		workDir:         cfg.WorkDir,
		skipPermissions: cfg.SkipPermissions,
		disableProgress: cfg.DisableProgress,
		skipToCloud:     cfg.SkipToCloud,
		printMode:       cfg.PrintMode,
		eligibilityOnly: cfg.EligibilityOnly,
	}

	if cfg.OutputLogs {
		mode := "TUI"
		if cfg.PrintMode {
			mode = "Headless"
		}
		logger, err := NewAgentLogger(cfg.WorkDir, mode)
		if err != nil {
			return nil, fmt.Errorf("failed to create agent logger: %w", err)
		}
		a.logger = logger
	}

	return a, nil
}

// SetTracker sets the analytics tracker for the agent
func (a *Agent) SetTracker(tracker *analytics.Tracker) {
	a.tracker = tracker
}

// trackEvent sends an analytics event if tracking is enabled
func (a *Agent) trackEvent(event string, props map[string]any) {
	if a.tracker == nil {
		return
	}
	if props == nil {
		props = make(map[string]any)
	}
	props["session_id"] = a.sessionID
	a.tracker.Track(event, props)
}

// trackInterrupted tracks an interruption event with standard context
func (a *Agent) trackInterrupted(phaseName string, phasesCompleted int) {
	a.trackEvent("drift_cli:setup_agent:interrupted", map[string]any{
		"phase":                  phaseName,
		"phases_completed":       phasesCompleted,
		"duration_ms":            time.Since(a.startTime).Milliseconds(),
		"project_type":           a.phaseManager.state.ProjectType,
		"package_manager":        a.phaseManager.state.PackageManager,
		"has_docker":             a.phaseManager.state.DockerType != "" && a.phaseManager.state.DockerType != "none",
		"compatibility_warnings": a.phaseManager.state.CompatibilityWarnings,
	})
}

// Run executes the agent with TUI or in headless mode
func (a *Agent) Run(parentCtx context.Context) error {
	// Create cancellable context
	a.ctx, a.cancel = context.WithCancel(parentCtx)
	defer a.cancel()

	if a.logger != nil {
		fmt.Printf("Logs will be written to: %s\n\n", a.logger.FilePath())
	}

	// Create UI based on mode
	a.ui = NewAgentUI(a.ctx, a.cancel, a.printMode, a.phaseManager.GetPhaseNames(), a.eligibilityOnly)

	if a.printMode {
		// Headless mode: set up signal handling and run directly
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigCh
			fmt.Println("\n\nInterrupted. Cleaning up...")
			a.cancel()
		}()

		if err := a.ui.Start(); err != nil {
			return err
		}
		return a.runAgent()
	}

	// TUI mode: suppress slog output to prevent corrupting the display
	originalHandler := slog.Default().Handler()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer slog.SetDefault(slog.New(originalHandler))

	// Start TUI
	tuiUI := a.ui.(*TUIUI)
	if err := tuiUI.Start(); err != nil {
		return err
	}

	// Run agent in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- a.runAgent()
	}()

	// Run TUI (blocks until quit)
	_, err := tuiUI.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	// Print final output so user can see what happened
	if output := a.ui.GetFinalOutput(); output != "" {
		fmt.Println(output)
	}

	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

func (a *Agent) runAgent() error {
	defer a.cleanup()
	a.startTime = time.Now()
	a.sessionID = generateSessionID()
	a.trackEvent("drift_cli:setup_agent:started", nil)

	// Track completed phases for progress file
	var completedPhases []string

	// Check for existing progress and skip to the appropriate phase (unless disabled)
	existingProgress := ""
	if !a.disableProgress {
		existingProgress = a.readProgress()
	}
	if existingProgress != "" {
		a.phaseManager.SetPreviousProgress(existingProgress)

		discoveredInfo := parseDiscoveredInfo(existingProgress)
		if len(discoveredInfo) > 0 {
			a.phaseManager.RestoreDiscoveredInfo(discoveredInfo)
		}
		setupProgress := parseSetupProgress(existingProgress)
		if len(setupProgress) > 0 {
			a.phaseManager.RestoreSetupProgress(setupProgress)
		}

		a.updateSidebarFromState()

		// Parse completed phases and skip ahead
		previouslyCompleted := parseCompletedPhases(existingProgress)
		if len(previouslyCompleted) > 0 {
			completedPhases = previouslyCompleted

			// Check if setup is already complete (Summary phase completed)
			allPhases := a.phaseManager.GetPhaseNames()
			lastPhase := allPhases[len(allPhases)-1]
			for _, p := range previouslyCompleted {
				if p == lastPhase {
					// Setup is already complete - ask user if they want to rerun
					rerun, cancelled := a.ui.PromptRerun()
					if cancelled {
						return a.setCancelled()
					}
					if rerun {
						// Start fresh - delete progress and report files
						a.deleteProgress()
						_ = os.Remove(filepath.Join(a.workDir, ".tusk", "SETUP_REPORT.md"))
						completedPhases = nil
						a.phaseManager = NewPhaseManager()
						a.ui.AgentText("Starting fresh setup...\n", false)
					} else {
						return a.setCompleted()
					}
					break
				}
			}

			// If we haven't reset, check for next phase to resume
			if len(completedPhases) > 0 {
				nextPhase := a.findNextPhaseToRun(completedPhases)
				if nextPhase != "" {
					if a.phaseManager.SkipToPhase(nextPhase) {
						a.ui.AgentText(fmt.Sprintf("Resuming from phase: %s (skipping %d completed phases)\n", nextPhase, len(completedPhases)), false)
					}
				}
			}
		}
	}

	// Run through phases
	for !a.phaseManager.IsComplete() {
		select {
		case <-a.ctx.Done():
			// Save progress before exiting
			phase := a.phaseManager.CurrentPhase()
			phaseName := ""
			if phase != nil {
				phaseName = phase.Name
			}
			_ = a.saveProgress(completedPhases, phaseName, "Agent was interrupted.")
			a.trackInterrupted(phaseName, len(completedPhases))
			return a.setCancelled()
		default:
		}

		phase := a.phaseManager.CurrentPhase()
		if phase == nil {
			break
		}

		// Notify UI of phase change
		phaseIdx := a.phaseManager.currentIdx + 1
		a.ui.PhaseChange(phase.Name, phase.Description, phaseIdx, len(a.phaseManager.phases))
		if a.logger != nil {
			a.logger.LogPhaseStart(phase.Name, phase.Description, phaseIdx, len(a.phaseManager.phases))
		}

		// Run phase with timeout
		phaseCtx, phaseCancel := context.WithTimeout(a.ctx, PhaseTimeout)
		err := a.runPhase(phaseCtx, phase)
		phaseCancel()

		if err != nil {
			if a.ctx.Err() != nil {
				_ = a.saveProgress(completedPhases, phase.Name, fmt.Sprintf("Agent was interrupted during %s phase.", phase.Name))
				a.trackInterrupted(phase.Name, len(completedPhases))
				return a.setCancelled()
			}

			// Special handling for abort_setup - graceful exit (not an error)
			if errors.Is(err, agenttools.ErrSetupAborted) {
				// Extract abort reason and project type from error
				abortReason := "unknown"
				projectType := a.phaseManager.state.ProjectType
				var abortErr *agenttools.AbortError
				if errors.As(err, &abortErr) {
					abortReason = abortErr.Reason
					// Use project type from abort if state doesn't have one
					if projectType == "" && abortErr.ProjectType != "" {
						projectType = abortErr.ProjectType
					}
				}

				a.trackEvent("drift_cli:setup_agent:aborted", map[string]any{
					"phase":                  phase.Name,
					"reason":                 abortReason,
					"phases_completed":       len(completedPhases),
					"duration_ms":            time.Since(a.startTime).Milliseconds(),
					"project_type":           projectType,
					"package_manager":        a.phaseManager.state.PackageManager,
					"has_docker":             a.phaseManager.state.DockerType != "" && a.phaseManager.state.DockerType != "none",
					"compatibility_warnings": a.phaseManager.state.CompatibilityWarnings,
				})
				a.ui.AgentText("\n\n🟠 Setup aborted. See message above for details.\n", false)
				a.ui.Aborted("")
				time.Sleep(500 * time.Millisecond)
				return a.setCompleted() // Not an error - graceful exit
			}

			if phase.Required {
				_ = a.saveProgress(completedPhases, phase.Name, fmt.Sprintf("Phase failed with error: %v", err))

				a.trackEvent("drift_cli:setup_agent:phase_failed", map[string]any{
					"phase":                  phase.Name,
					"error":                  err.Error(),
					"phases_completed":       len(completedPhases),
					"duration_ms":            time.Since(a.startTime).Milliseconds(),
					"project_type":           a.phaseManager.state.ProjectType,
					"package_manager":        a.phaseManager.state.PackageManager,
					"has_docker":             a.phaseManager.state.DockerType != "" && a.phaseManager.state.DockerType != "none",
					"compatibility_warnings": a.phaseManager.state.CompatibilityWarnings,
				})

				// Fatal error for required phases - will auto-quit
				a.ui.FatalError(err)
				time.Sleep(100 * time.Millisecond)
				return a.setFailed(fmt.Errorf("required phase %s failed: %w", phase.Name, err))
			}

			// Non-fatal error for optional phases
			a.ui.Error(err)

			// Skip to next phase for optional phases
			_, _ = a.phaseManager.AdvancePhase()
		} else {
			a.trackEvent("drift_cli:setup_agent:phase_completed", map[string]any{
				"phase": phase.Name,
			})

			completedPhases = append(completedPhases, phase.Name)
			nextPhase := a.phaseManager.CurrentPhase()
			nextPhaseName := ""
			if nextPhase != nil {
				nextPhaseName = nextPhase.Name
			}
			_ = a.saveProgress(completedPhases, nextPhaseName, "")
		}
	}

	// Check if eligibility mode - write report and exit
	if a.phaseManager.GetState().EligibilityReport != "" {
		report, err := ParseEligibilityReport(a.phaseManager.GetState().EligibilityReport)
		if err != nil {
			// This shouldn't happen as we validated earlier, but handle gracefully
			return a.setFailed(fmt.Errorf("failed to parse eligibility report: %w", err))
		}

		if err := WriteEligibilityReport(a.workDir, report); err != nil {
			return a.setFailed(fmt.Errorf("failed to write eligibility report: %w", err))
		}

		a.trackEvent("drift_cli:setup_agent:eligibility_completed", map[string]any{
			"total_services":       report.Summary.TotalServices,
			"compatible":           report.Summary.Compatible,
			"partially_compatible": report.Summary.PartiallyCompatible,
			"not_compatible":       report.Summary.NotCompatible,
			"duration_ms":          time.Since(a.startTime).Milliseconds(),
		})

		// Show completion message specific to eligibility mode
		a.ui.AgentText(fmt.Sprintf("\n\nEligibility report saved to .tusk/eligibility-report.json\n\nSummary:\n- Total services: %d\n- Compatible: %d\n- Partially compatible: %d\n- Not compatible: %d\n",
			report.Summary.TotalServices,
			report.Summary.Compatible,
			report.Summary.PartiallyCompatible,
			report.Summary.NotCompatible), false)
		a.ui.EligibilityCompleted(a.workDir)
		time.Sleep(500 * time.Millisecond)
		return a.setCompleted()
	}

	if a.skipToCloud {
		// Cloud loop already completed, just send the completed message
		_ = a.saveProgress(completedPhases, "", "Cloud setup completed successfully.")
		a.trackEvent("drift_cli:setup_agent:cloud_completed", map[string]any{
			"phases_completed": len(completedPhases),
			"duration_ms":      time.Since(a.startTime).Milliseconds(),
			"skip_to_cloud":    true,
		})
		a.ui.Completed(a.workDir)
		time.Sleep(500 * time.Millisecond)
		return a.setCompleted()
	}

	_ = a.saveProgress(completedPhases, "", "Local setup completed successfully.")

	a.trackEvent("drift_cli:setup_agent:local_completed", map[string]any{
		"phases_completed":       len(completedPhases),
		"duration_ms":            time.Since(a.startTime).Milliseconds(),
		"project_type":           a.phaseManager.state.ProjectType,
		"package_manager":        a.phaseManager.state.PackageManager,
		"has_docker":             a.phaseManager.state.DockerType != "" && a.phaseManager.state.DockerType != "none",
		"compatibility_warnings": a.phaseManager.state.CompatibilityWarnings,
	})

	// Check if we should prompt for cloud setup
	// Only prompt if we haven't already done cloud setup
	if !a.phaseManager.HasCloudPhases() {
		// Prompt user to continue with cloud setup
		continueCloud, cancelled := a.ui.PromptCloudSetup()
		if cancelled {
			return a.setCancelled()
		}

		if continueCloud {
			// Add cloud phases and continue
			a.phaseManager.AddCloudPhases()
			a.ui.UpdatePhaseList(a.phaseManager.GetPhaseNames())

			a.trackEvent("drift_cli:setup_agent:cloud_started", nil)

			for !a.phaseManager.IsComplete() {
				select {
				case <-a.ctx.Done():
					phase := a.phaseManager.CurrentPhase()
					phaseName := ""
					if phase != nil {
						phaseName = phase.Name
					}
					_ = a.saveProgress(completedPhases, phaseName, "Agent was interrupted during cloud setup.")
					a.trackInterrupted(phaseName, len(completedPhases))
					return a.setCancelled()
				default:
				}

				phase := a.phaseManager.CurrentPhase()
				if phase == nil {
					break
				}

				// Notify UI of phase change
				phaseIdx := a.phaseManager.currentIdx + 1
				a.ui.PhaseChange(phase.Name, phase.Description, phaseIdx, len(a.phaseManager.phases))
				if a.logger != nil {
					a.logger.LogPhaseStart(phase.Name, phase.Description, phaseIdx, len(a.phaseManager.phases))
				}

				phaseCtx, phaseCancel := context.WithTimeout(a.ctx, PhaseTimeout)
				err := a.runPhase(phaseCtx, phase)
				phaseCancel()

				if err != nil {
					if a.ctx.Err() != nil {
						_ = a.saveProgress(completedPhases, phase.Name, fmt.Sprintf("Agent was interrupted during %s phase.", phase.Name))
						a.trackInterrupted(phase.Name, len(completedPhases))
						return a.setCancelled()
					}

					// Special handling for abort_setup
					if errors.Is(err, agenttools.ErrSetupAborted) {
						var abortErr *agenttools.AbortError
						if errors.As(err, &abortErr) {
							a.trackEvent("drift_cli:setup_agent:cloud_aborted", map[string]any{
								"phase":  phase.Name,
								"reason": abortErr.Reason,
							})
						}
						a.ui.AgentText("\n\n🟠 Cloud setup aborted. See message above for details.\n", false)
						a.ui.Aborted("")
						time.Sleep(500 * time.Millisecond)
						return a.setCompleted()
					}

					if phase.Required {
						_ = a.saveProgress(completedPhases, phase.Name, fmt.Sprintf("Cloud phase failed with error: %v", err))
						a.trackEvent("drift_cli:setup_agent:cloud_phase_failed", map[string]any{
							"phase": phase.Name,
							"error": err.Error(),
						})
						a.ui.FatalError(err)
						time.Sleep(100 * time.Millisecond)
						return a.setFailed(fmt.Errorf("required cloud phase %s failed: %w", phase.Name, err))
					}

					a.ui.Error(err)
					_, _ = a.phaseManager.AdvancePhase()
				} else {
					a.trackEvent("drift_cli:setup_agent:cloud_phase_completed", map[string]any{
						"phase": phase.Name,
					})
					completedPhases = append(completedPhases, phase.Name)
					nextPhase := a.phaseManager.CurrentPhase()
					nextPhaseName := ""
					if nextPhase != nil {
						nextPhaseName = nextPhase.Name
					}
					_ = a.saveProgress(completedPhases, nextPhaseName, "")
				}
			}

			_ = a.saveProgress(completedPhases, "", "Cloud setup completed successfully.")
			a.trackEvent("drift_cli:setup_agent:cloud_completed", map[string]any{
				"phases_completed": len(completedPhases),
				"duration_ms":      time.Since(a.startTime).Milliseconds(),
			})
		} else {
			a.trackEvent("drift_cli:setup_agent:cloud_skipped", nil)
		}
	}

	a.ui.Completed(a.workDir)
	time.Sleep(500 * time.Millisecond)

	return a.setCompleted()
}

func (a *Agent) runPhase(ctx context.Context, phase *Phase) error {
	systemPrompt := a.buildSystemPrompt(phase)
	tools := FilterToolsForPhase(a.allTools, phase)

	messages := []Message{
		{
			Role: "user",
			Content: []Content{{
				Type: "text",
				Text: fmt.Sprintf("Please proceed with the %s phase. The working directory is: %s\n\nCurrent state:\n%s",
					phase.Name, a.workDir, a.phaseManager.StateAsContext()),
			}},
		},
	}

	a.phaseManager.ResetTransitionFlag()
	apiErrorCount := 0

	maxIterations := phase.MaxIterations
	if maxIterations <= 0 {
		maxIterations = DefaultMaxIterations
	}

	for iteration := 0; iteration < maxIterations; iteration++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		a.ui.Thinking(true)
		if a.logger != nil {
			a.logger.LogThinking(true)
		}

		var streamedText strings.Builder

		apiCtx, apiCancel := context.WithTimeout(ctx, APITimeout)
		resp, err := a.client.CreateMessageStreaming(apiCtx, systemPrompt, messages, tools, func(event StreamEvent) {
			switch event.Type {
			case "text":
				streamedText.WriteString(event.Text)
				a.ui.AgentText(streamedText.String(), true)
			case "tool_use_start":
				// Don't show tool start here - wait for full input
			}
		})
		apiCancel()

		a.ui.Thinking(false)

		if err != nil {
			apiErrorCount++
			errMsg := err.Error()

			// If error is recoverable, retry with backoff
			// Log the error and let the agent try again
			if apiErrorCount < MaxAPIRetries && isRecoverableAPIError(errMsg) {
				a.ui.Error(fmt.Errorf("API error (retrying): %s", errMsg))
				messages = append(messages, Message{
					Role: "user",
					Content: []Content{{
						Type: "text",
						Text: fmt.Sprintf("There was an API error: %s. Please try again with a simpler approach.", errMsg),
					}},
				})

				time.Sleep(time.Duration(apiErrorCount) * time.Second)
				continue
			}

			return fmt.Errorf("API error: %w", err)
		}

		apiErrorCount = 0 // Reset on success
		a.totalTokensIn += resp.Usage.InputTokens
		a.totalTokensOut += resp.Usage.OutputTokens

		// Clean up content - ensure all tool_use have valid Input
		cleanedContent := cleanupContent(resp.Content)

		messages = append(messages, Message{
			Role:    "assistant",
			Content: cleanedContent,
		})

		for _, content := range cleanedContent {
			if content.Type == "text" && strings.TrimSpace(content.Text) != "" {
				a.ui.AgentText(content.Text, false)
				if a.logger != nil {
					a.logger.LogMessage(content.Text)
				}
			}
		}

		if a.phaseManager.HasTransitioned() {
			return nil
		}

		if resp.StopReason == "end_turn" {
			messages = append(messages, Message{
				Role: "user",
				Content: []Content{{
					Type: "text",
					Text: "Please continue with the current phase, or if you've completed the objectives, call transition_phase to move to the next phase.",
				}},
			})
			continue
		}

		if resp.StopReason == "tool_use" {
			toolResults, err := a.executeToolCalls(ctx, cleanedContent)
			if err != nil {
				// Special handling for abort_setup - graceful exit
				if errors.Is(err, agenttools.ErrSetupAborted) {
					return err
				}

				// Don't return error - let agent handle it
				a.ui.Error(err)
				messages = append(messages, Message{
					Role: "user",
					Content: []Content{{
						Type: "text",
						Text: fmt.Sprintf("Tool execution error: %s. Please try a different approach.", err.Error()),
					}},
				})
				continue
			}

			if a.phaseManager.HasTransitioned() {
				return nil
			}

			messages = append(messages, Message{
				Role:    "user",
				Content: toolResults,
			})
		}

		if a.totalTokensIn+a.totalTokensOut > MaxTotalTokens {
			return fmt.Errorf("%s (%d tokens)", ErrMaxTokens, MaxTotalTokens)
		}
	}

	return fmt.Errorf("%s", ErrMaxIterations)
}

// cleanupContent ensures all content blocks are valid for the API
func cleanupContent(content []Content) []Content {
	var cleaned []Content
	for _, c := range content {
		if c.Type == "tool_use" {
			// Ensure Input is never nil
			if c.Input == nil {
				c.Input = json.RawMessage("{}")
			}
		}
		cleaned = append(cleaned, c)
	}
	return cleaned
}

// isRecoverableAPIError checks if an API error might be recoverable
func isRecoverableAPIError(errMsg string) bool {
	recoverablePatterns := []string{
		"Field required",
		"invalid_request_error",
		"malformed",
		"rate_limit",
		"overloaded",
	}
	for _, pattern := range recoverablePatterns {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}
	return false
}

func (a *Agent) buildSystemPrompt(phase *Phase) string {
	// Build instructions, including any dynamic content from OnEnter
	instructions := phase.Instructions
	if phase.OnEnter != nil {
		if extra := phase.OnEnter(a.phaseManager.GetState()); extra != "" {
			instructions = instructions + "\n\n" + extra
		}
	}

	return fmt.Sprintf(`%s

## Current Phase: %s

%s

## Available Phases
%s

## Important Notes
- Always call transition_phase when you complete a phase's objectives
- Include all relevant results when transitioning
- If you're stuck, use ask_user to get help from the user
- Be thorough but efficient - don't repeat work unnecessarily
- If a tool call fails, try a different approach rather than repeating the same call
`,
		SystemPrompt,
		phase.Name,
		instructions,
		PhasesSummary(),
	)
}

func (a *Agent) executeToolCalls(ctx context.Context, content []Content) ([]Content, error) {
	var results []Content

	for _, c := range content {
		if c.Type != "tool_use" {
			continue
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Format input for display
		inputStr := string(c.Input)
		a.ui.ToolStart(c.Name, inputStr)
		if a.logger != nil {
			a.logger.LogToolStart(c.Name, inputStr)
		}

		// Extract sidebar info from tool inputs
		a.extractSidebarInfo(c.Name, inputStr)

		// Special handling for ask_user and ask_user_select
		if c.Name == "ask_user" {
			result := a.handleAskUser(c)
			results = append(results, result)
			continue
		}
		if c.Name == "ask_user_select" {
			result := a.handleAskUserSelect(c)
			results = append(results, result)
			continue
		}

		// Check for port conflicts
		if c.Name == "start_background_process" || c.Name == "run_command" {
			if err := a.checkPortConflicts(c.Input); err != nil {
				a.ui.ToolComplete(c.Name, false, err.Error())
				results = append(results, Content{
					Type:      "tool_result",
					ToolUseID: c.ID,
					Content:   fmt.Sprintf("Error: %s", err.Error()),
					IsError:   true,
				})
				continue
			}
		}

		// Check if tool requires permission
		if !a.skipPermissions {
			if toolDef := GetRegistry().Get(ToolName(c.Name)); toolDef != nil && toolDef.RequiresConfirmation {
				preview := formatToolPreview(c.Name, inputStr)
				response := a.ui.PromptPermission(c.Name, preview)

				switch {
				case response == "approve":
					// Continue with execution
				case response == "approve_all":
					a.skipPermissions = true
					// Continue with execution
				case response == "deny":
					a.ui.ToolComplete(c.Name, false, "User denied permission")
					results = append(results, Content{
						Type:      "tool_result",
						ToolUseID: c.ID,
						Content:   "Error: User denied permission for this action. Please try a different approach.",
						IsError:   true,
					})
					continue
				case strings.HasPrefix(response, "deny:"):
					alternative := strings.TrimPrefix(response, "deny:")
					a.ui.ToolComplete(c.Name, false, "User suggested alternative")
					results = append(results, Content{
						Type:      "tool_result",
						ToolUseID: c.ID,
						Content:   fmt.Sprintf("User denied this action and suggested: %s\n\nPlease follow the user's suggestion instead.", alternative),
						IsError:   true,
					})
					continue
				}
			}
		}

		executor, ok := a.executors[c.Name]
		if !ok {
			a.ui.ToolComplete(c.Name, false, "unknown tool")
			if a.logger != nil {
				a.logger.LogToolComplete(c.Name, false, "unknown tool")
			}
			results = append(results, Content{
				Type:      "tool_result",
				ToolUseID: c.ID,
				Content:   fmt.Sprintf("Unknown tool: %s", c.Name),
				IsError:   true,
			})
			continue
		}

		// Execute with timeout
		toolCtx, toolCancel := context.WithTimeout(ctx, ToolTimeout)
		resultCh := make(chan struct {
			result string
			err    error
		}, 1)

		go func() {
			result, err := executor(c.Input)
			resultCh <- struct {
				result string
				err    error
			}{result, err}
		}()

		select {
		case <-toolCtx.Done():
			toolCancel()
			a.ui.ToolComplete(c.Name, false, "timeout")
			if a.logger != nil {
				a.logger.LogToolComplete(c.Name, false, "timeout")
			}
			results = append(results, Content{
				Type:      "tool_result",
				ToolUseID: c.ID,
				Content:   fmt.Sprintf("Error: tool execution timed out after %v", ToolTimeout),
				IsError:   true,
			})
		case res := <-resultCh:
			toolCancel()
			if res.err != nil {
				// Special handling for abort_setup - return the error to stop the agent
				if errors.Is(res.err, agenttools.ErrSetupAborted) {
					a.ui.ToolComplete(c.Name, true, res.result)
					if a.logger != nil {
						a.logger.LogToolComplete(c.Name, true, res.result)
					}
					return nil, res.err
				}

				a.ui.ToolComplete(c.Name, false, res.err.Error())
				if a.logger != nil {
					a.logger.LogToolComplete(c.Name, false, res.err.Error())
				}
				results = append(results, Content{
					Type:      "tool_result",
					ToolUseID: c.ID,
					Content:   fmt.Sprintf("Error: %s", res.err.Error()),
					IsError:   true,
				})
			} else {
				a.ui.ToolComplete(c.Name, true, res.result)
				if a.logger != nil {
					a.logger.LogToolComplete(c.Name, true, res.result)
				}
				results = append(results, Content{
					Type:      "tool_result",
					ToolUseID: c.ID,
					Content:   res.result,
				})

				// After successful transition_phase, update sidebar from confirmed state
				if c.Name == "transition_phase" {
					a.updateSidebarFromState()
				}
			}
		}
	}

	return results, nil
}

func (a *Agent) handleAskUser(c Content) Content {
	var params struct {
		Question string `json:"question"`
	}
	if err := json.Unmarshal(c.Input, &params); err != nil {
		return Content{
			Type:      "tool_result",
			ToolUseID: c.ID,
			Content:   fmt.Sprintf("Error: invalid input: %v", err),
			IsError:   true,
		}
	}

	response, cancelled := a.ui.PromptUserInput(params.Question)

	if cancelled {
		if a.logger != nil {
			a.logger.LogUserInput(params.Question, "(cancelled)")
		}
		return Content{
			Type:      "tool_result",
			ToolUseID: c.ID,
			Content:   "User cancelled input",
			IsError:   true,
		}
	}

	if a.logger != nil {
		a.logger.LogUserInput(params.Question, response)
	}
	a.ui.ToolComplete(c.Name, true, response)
	return Content{
		Type:      "tool_result",
		ToolUseID: c.ID,
		Content:   response,
	}
}

func (a *Agent) handleAskUserSelect(c Content) Content {
	var params struct {
		Question string         `json:"question"`
		Options  []SelectOption `json:"options"`
	}
	if err := json.Unmarshal(c.Input, &params); err != nil {
		return Content{
			Type:      "tool_result",
			ToolUseID: c.ID,
			Content:   fmt.Sprintf("Error: invalid input: %v", err),
			IsError:   true,
		}
	}

	if len(params.Options) == 0 {
		return Content{
			Type:      "tool_result",
			ToolUseID: c.ID,
			Content:   "Error: no options provided",
			IsError:   true,
		}
	}

	selectedID, selectedLabel, cancelled := a.ui.PromptUserSelect(params.Question, params.Options)

	if cancelled {
		if a.logger != nil {
			a.logger.LogUserSelect(params.Question, "", "(cancelled)")
		}
		return Content{
			Type:      "tool_result",
			ToolUseID: c.ID,
			Content:   "User cancelled selection",
			IsError:   true,
		}
	}

	if a.logger != nil {
		a.logger.LogUserSelect(params.Question, selectedID, selectedLabel)
	}
	a.ui.ToolComplete(c.Name, true, selectedLabel)

	result := map[string]string{
		"selected_id":    selectedID,
		"selected_label": selectedLabel,
	}
	resultJSON, _ := json.Marshal(result)
	return Content{
		Type:      "tool_result",
		ToolUseID: c.ID,
		Content:   string(resultJSON),
	}
}

func (a *Agent) checkPortConflicts(input json.RawMessage) error {
	var params struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil
	}

	ports := extractPortsFromCommand(params.Command)

	for _, port := range ports {
		if isPortInUse(port) {
			if a.ui.PromptKillPort(port) {
				if err := killProcessOnPort(port); err != nil {
					return fmt.Errorf("failed to kill process on port %d: %w", port, err)
				}
			} else {
				return fmt.Errorf("port %d is in use and user declined to kill process", port)
			}
		}
	}

	return nil
}

func extractPortsFromCommand(cmd string) []int {
	var ports []int

	patterns := []string{
		"PORT=", "port=", "-p ", "--port ", ":3000", ":8000", ":8080", ":5000", ":4000",
	}

	for _, p := range patterns {
		if strings.Contains(cmd, p) {
			if strings.HasSuffix(p, "=") || strings.HasSuffix(p, " ") {
				idx := strings.Index(cmd, p)
				if idx >= 0 {
					rest := cmd[idx+len(p):]
					var portStr string
					for _, c := range rest {
						if c >= '0' && c <= '9' {
							portStr += string(c)
						} else {
							break
						}
					}
					if port, err := strconv.Atoi(portStr); err == nil && port > 0 {
						ports = append(ports, port)
					}
				}
			} else if strings.HasPrefix(p, ":") {
				portStr := p[1:]
				if port, err := strconv.Atoi(portStr); err == nil && port > 0 {
					ports = append(ports, port)
				}
			}
		}
	}

	seen := make(map[int]bool)
	var unique []int
	for _, p := range ports {
		if !seen[p] {
			seen[p] = true
			unique = append(unique, p)
		}
	}

	return unique
}

func isPortInUse(port int) bool {
	addr := fmt.Sprintf(":%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return true
	}
	_ = ln.Close()
	return false
}

func killProcessOnPort(port int) error {
	cmd := exec.Command("sh", "-c", fmt.Sprintf("lsof -ti:%d | xargs kill -9 2>/dev/null || true", port)) //nolint:gosec // Port is an integer, safe to interpolate
	return cmd.Run()
}

// formatToolPreview creates a human-readable preview of a tool's input for permission prompts
func formatToolPreview(toolName, input string) string {
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return input
	}

	switch toolName {
	case "run_command", "start_background_process":
		if cmd, ok := params["command"].(string); ok {
			return fmt.Sprintf("> %s", cmd)
		}
	case "write_file":
		if path, ok := params["path"].(string); ok {
			if content, ok := params["content"].(string); ok {
				lines := strings.Count(content, "\n") + 1
				return fmt.Sprintf("Write to %s (%d lines)", path, lines)
			}
			return fmt.Sprintf("Write to %s", path)
		}
	case "patch_file":
		if path, ok := params["path"].(string); ok {
			return fmt.Sprintf("Edit %s", path)
		}
	case "http_request":
		method := "GET"
		if m, ok := params["method"].(string); ok {
			method = m
		}
		if url, ok := params["url"].(string); ok {
			return fmt.Sprintf("%s %s", method, url)
		}
	case "tusk_run":
		return "Run trace tests"
	}

	return ""
}

// extractSidebarInfo parses tool inputs to extract useful info for the sidebar
// Note: Most sidebar updates now happen via transition_phase results to ensure
// only confirmed information is displayed. This function only handles basic
// runtime detection during the Discovery phase.
func (a *Agent) extractSidebarInfo(toolName, input string) {
	// Extract info based on tool and input patterns
	// Only detect runtime/language from file reads - these are safe to show early
	switch toolName {
	case "read_file":
		// Check for common files to detect runtime
		switch {
		case strings.Contains(input, "package.json"):
			a.ui.SidebarUpdate("Runtime", "Node.js")
		case strings.Contains(input, "tsconfig.json"):
			a.ui.SidebarUpdate("Language", "TypeScript")
		case strings.Contains(input, "go.mod"):
			a.ui.SidebarUpdate("Runtime", "Go")
		case strings.Contains(input, "requirements.txt"), strings.Contains(input, "pyproject.toml"):
			a.ui.SidebarUpdate("Runtime", "Python")
		}
	}
	// Note: Port, Health, Start command, and Mode are now updated via transition_phase
	// to avoid showing unconfirmed/stale values from incidental tool calls
}

// updateSidebarFromState updates the sidebar with confirmed state from PhaseManager
func (a *Agent) updateSidebarFromState() {
	state := a.phaseManager.GetState()
	if state == nil {
		return
	}

	if state.Port != "" {
		a.ui.SidebarUpdate("Port", state.Port)
	}
	if state.HealthEndpoint != "" {
		a.ui.SidebarUpdate("Health", state.HealthEndpoint)
	}
	if state.StartCommand != "" {
		// Shorten start command for display
		cmd := state.StartCommand
		if len(cmd) > 20 {
			cmd = cmd[:17] + "..."
		}
		a.ui.SidebarUpdate("Start", cmd)
	}
	if state.EntryPoint != "" {
		// Shorten entry point for display
		entry := state.EntryPoint
		if len(entry) > 25 {
			entry = "..." + entry[len(entry)-22:]
		}
		a.ui.SidebarUpdate("Entry", entry)
	}
	if state.ModuleSystem != "" {
		a.ui.SidebarUpdate("Module", strings.ToUpper(state.ModuleSystem))
	}
}

func (a *Agent) cleanup() {
	a.processManager.StopAll()
	if a.logger != nil {
		_ = a.logger.Close(a.finalStatus, a.finalError)
	}
}

// setCompleted marks the agent as successfully completed and returns nil
func (a *Agent) setCompleted() error {
	a.finalStatus = StatusCompleted
	a.finalError = nil
	return nil
}

// setCancelled marks the agent as cancelled by user and returns an "interrupted" error
func (a *Agent) setCancelled() error {
	a.finalStatus = StatusCancelled
	a.finalError = nil
	return fmt.Errorf("interrupted")
}

// setFailed marks the agent as failed and returns the error
func (a *Agent) setFailed(err error) error {
	a.finalStatus = StatusFailed
	a.finalError = err
	return err
}

// generateSessionID creates a unique short session ID for this agent run
func generateSessionID() string {
	return uuid.New().String()[:8]
}

// Progress file management

func (a *Agent) progressFilePath() string {
	return filepath.Join(a.workDir, ".tusk", progressFileName)
}

// readProgress reads the existing progress file if it exists
func (a *Agent) readProgress() string {
	content, err := os.ReadFile(a.progressFilePath())
	if err != nil {
		return ""
	}
	return string(content)
}

// parseCompletedPhases extracts completed phase names from the progress file content
func parseCompletedPhases(progressContent string) []string {
	var completed []string

	// Find the "## Completed Phases" section
	lines := strings.Split(progressContent, "\n")
	inCompletedSection := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "## Completed Phases") {
			inCompletedSection = true
			continue
		}

		// Stop at the next section
		if inCompletedSection && strings.HasPrefix(line, "## ") {
			break
		}

		// Parse completed phase lines like "- ✓ Discovery"
		if inCompletedSection && strings.HasPrefix(line, "- ✓ ") {
			phaseName := strings.TrimPrefix(line, "- ✓ ")
			if phaseName != "" {
				completed = append(completed, phaseName)
			}
		}
	}

	return completed
}

// parseDiscoveredInfo extracts discovered information from the progress file content
func parseDiscoveredInfo(progressContent string) map[string]string {
	info := make(map[string]string)

	lines := strings.Split(progressContent, "\n")
	inDiscoveredSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "## Discovered Information") {
			inDiscoveredSection = true
			continue
		}

		// Stop at the next section
		if inDiscoveredSection && strings.HasPrefix(trimmed, "## ") {
			break
		}

		// Parse lines like "- **Service Name**: backend" or "- **Start Command**: `npm run dev`"
		if inDiscoveredSection && strings.HasPrefix(trimmed, "- **") {
			// Find the closing **
			rest := strings.TrimPrefix(trimmed, "- **")
			idx := strings.Index(rest, "**:")
			if idx > 0 {
				key := rest[:idx]
				value := strings.TrimSpace(rest[idx+3:])
				// Remove backticks if present (for Start Command)
				value = strings.Trim(value, "`")
				info[key] = value
			}
		}
	}

	return info
}

// parseSetupProgress extracts setup progress flags from the progress file content
func parseSetupProgress(progressContent string) map[string]bool {
	progress := make(map[string]bool)

	lines := strings.Split(progressContent, "\n")
	inProgressSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "## Setup Progress") {
			inProgressSection = true
			continue
		}

		// Stop at the next section
		if inProgressSection && strings.HasPrefix(trimmed, "## ") {
			break
		}

		// Parse lines like "- ✓ App starts without SDK"
		if inProgressSection && strings.HasPrefix(trimmed, "- ✓ ") {
			item := strings.TrimPrefix(trimmed, "- ✓ ")
			switch {
			case strings.Contains(item, "App starts without SDK"):
				progress["app_starts_without_sdk"] = true
			case strings.Contains(item, "SDK installed"):
				progress["sdk_installed"] = true
			case strings.Contains(item, "SDK instrumented"):
				progress["sdk_instrumented"] = true
			case strings.Contains(item, "Config file created"):
				progress["config_created"] = true
			case strings.Contains(item, "Simple test passed"):
				progress["simple_test_passed"] = true
			case strings.Contains(item, "Complex test passed"):
				progress["complex_test_passed"] = true
			}
		}
	}

	return progress
}

// findNextPhaseToRun determines which phase to start from based on completed phases
func (a *Agent) findNextPhaseToRun(completedPhases []string) string {
	allPhases := a.phaseManager.GetPhaseNames()

	// Create a set of completed phases for quick lookup
	completedSet := make(map[string]bool)
	for _, p := range completedPhases {
		completedSet[p] = true
	}

	// Find the first non-completed phase
	for _, phaseName := range allPhases {
		if !completedSet[phaseName] {
			return phaseName
		}
	}

	// All phases completed
	return ""
}

// saveProgress saves the current progress to the progress file
func (a *Agent) saveProgress(completedPhases []string, currentPhase string, notes string) error {
	if a.disableProgress || a.eligibilityOnly {
		return nil
	}

	tuskDir := filepath.Join(a.workDir, ".tusk")
	if err := os.MkdirAll(tuskDir, 0o750); err != nil {
		return err
	}

	state := a.phaseManager.GetState()

	var sb strings.Builder
	sb.WriteString("# Tusk Drift Setup Progress\n\n")
	sb.WriteString("This file tracks the progress of the AI setup agent. If the agent was interrupted,\n")
	sb.WriteString("it will read this file on the next run to continue where it left off.\n\n")
	sb.WriteString("**Note**: This file is automatically deleted when setup completes successfully.\n\n")

	sb.WriteString("## Discovered Information\n\n")
	if state != nil && (state.ProjectType != "" || state.PackageManager != "" || state.EntryPoint != "") {
		if state.ServiceName != "" {
			sb.WriteString(fmt.Sprintf("- **Service Name**: %s\n", state.ServiceName))
		}
		if state.ProjectType != "" {
			sb.WriteString(fmt.Sprintf("- **Project Type**: %s\n", state.ProjectType))
		}
		if state.PackageManager != "" {
			sb.WriteString(fmt.Sprintf("- **Package Manager**: %s\n", state.PackageManager))
		}
		if state.ModuleSystem != "" {
			sb.WriteString(fmt.Sprintf("- **Module System**: %s\n", state.ModuleSystem))
		}
		if state.EntryPoint != "" {
			sb.WriteString(fmt.Sprintf("- **Entry Point**: %s\n", state.EntryPoint))
		}
		if state.StartCommand != "" {
			sb.WriteString(fmt.Sprintf("- **Start Command**: `%s`\n", state.StartCommand))
		}
		if state.Port != "" {
			sb.WriteString(fmt.Sprintf("- **Port**: %s\n", state.Port))
		}
		if state.HealthEndpoint != "" {
			sb.WriteString(fmt.Sprintf("- **Health Endpoint**: %s\n", state.HealthEndpoint))
		}
		if state.DockerType != "" && state.DockerType != "none" {
			sb.WriteString(fmt.Sprintf("- **Docker**: %s\n", state.DockerType))
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("No information discovered yet.\n\n")
	}

	if state != nil && len(state.CompatibilityWarnings) > 0 {
		sb.WriteString("## Compatibility Warnings\n\n")
		sb.WriteString("The following packages are used but not instrumented by the SDK.\n")
		sb.WriteString("Recording/replay may not capture these calls:\n\n")
		for _, warning := range state.CompatibilityWarnings {
			sb.WriteString(fmt.Sprintf("- ⚠️ %s\n", warning))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Setup Progress\n\n")
	if state != nil {
		if state.AppStartsWithoutSDK {
			sb.WriteString("- ✓ App starts without SDK\n")
		}
		if state.SDKInstalled {
			sb.WriteString("- ✓ SDK installed\n")
		}
		if state.SDKInstrumented {
			sb.WriteString("- ✓ SDK instrumented in entry point\n")
		}
		if state.ConfigCreated {
			sb.WriteString("- ✓ Config file created (.tusk/config.yaml)\n")
		}
		if state.SimpleTestPassed {
			sb.WriteString("- ✓ Simple test passed (health endpoint)\n")
		}
		if state.ComplexTestPassed {
			sb.WriteString("- ✓ Complex test passed (endpoint with external calls)\n")
		}
		// Cloud setup progress
		if state.IsAuthenticated {
			sb.WriteString("- ✓ Authenticated with Tusk Cloud\n")
		}
		if state.GitRepoOwner != "" && state.GitRepoName != "" {
			sb.WriteString(fmt.Sprintf("- ✓ Repository detected: %s/%s\n", state.GitRepoOwner, state.GitRepoName))
		}
		if state.CloudServiceID != "" {
			sb.WriteString(fmt.Sprintf("- ✓ Cloud service created (ID: %s)\n", state.CloudServiceID))
		}
		if state.ApiKeyCreated {
			sb.WriteString("- ✓ API key created\n")
		}
	}
	sb.WriteString("\n")

	sb.WriteString("## Completed Phases\n\n")
	if len(completedPhases) == 0 {
		sb.WriteString("None yet.\n\n")
	} else {
		for _, phase := range completedPhases {
			sb.WriteString(fmt.Sprintf("- ✓ %s\n", phase))
		}
		sb.WriteString("\n")
	}

	if currentPhase != "" {
		sb.WriteString(fmt.Sprintf("## Current Phase\n\n%s (in progress)\n\n", currentPhase))
	}

	if state != nil && (len(state.Errors) > 0 || len(state.Warnings) > 0) {
		if len(state.Errors) > 0 {
			sb.WriteString("## Errors Encountered\n\n")
			for _, err := range state.Errors {
				if err.Fatal {
					sb.WriteString(fmt.Sprintf("- ❌ [%s] %s (fatal)\n", err.Phase, err.Message))
				} else {
					sb.WriteString(fmt.Sprintf("- ⚠️ [%s] %s\n", err.Phase, err.Message))
				}
			}
			sb.WriteString("\n")
		}
		if len(state.Warnings) > 0 {
			sb.WriteString("## Warnings\n\n")
			for _, w := range state.Warnings {
				sb.WriteString(fmt.Sprintf("- %s\n", w))
			}
			sb.WriteString("\n")
		}
	}

	if notes != "" {
		sb.WriteString("## Notes\n\n")
		sb.WriteString(notes)
		sb.WriteString("\n\n")
	}

	sb.WriteString(fmt.Sprintf("---\nLast updated: %s\n", time.Now().Format(time.RFC3339)))

	return os.WriteFile(a.progressFilePath(), []byte(sb.String()), 0o600)
}

// deleteProgress removes the progress file (called on successful completion)
func (a *Agent) deleteProgress() {
	if a.disableProgress {
		return
	}
	_ = os.Remove(a.progressFilePath())
}
