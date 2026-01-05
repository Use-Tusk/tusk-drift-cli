package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	agenttools "github.com/Use-Tusk/tusk-drift-cli/internal/agent/tools"
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

// runHeadless executes the agent in headless mode (no TUI)
func (a *Agent) runHeadless() error {
	defer a.cleanup()
	a.startTime = time.Now()
	a.sessionID = generateSessionID()
	a.trackEvent("drift_cli:setup_agent:started", map[string]any{
		"headless": true,
	})

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\n\nInterrupted. Cleaning up...")
		a.cancel()
	}()

	fmt.Println(headlessPhaseStyle.Render("• TUSK DRIFT AUTO SETUP (Headless Mode) •"))
	fmt.Println()

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
					fmt.Println(headlessSuccessStyle.Render("✅ Tusk Drift setup is already complete!"))
					fmt.Println()
					fmt.Print("Would you like to rerun the setup from scratch? [y/N]: ")

					reader := bufio.NewReader(os.Stdin)
					response, _ := reader.ReadString('\n')
					response = strings.TrimSpace(strings.ToLower(response))

					if response == "y" || response == "yes" {
						// Start fresh - delete progress and report files
						a.deleteProgress()
						_ = os.Remove(a.workDir + "/.tusk/SETUP_REPORT.md")
						completedPhases = nil
						a.phaseManager = NewPhaseManager()
						fmt.Println("Starting fresh setup...")
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
						fmt.Printf("Resuming from phase: %s (skipping %d completed phases)\n\n", nextPhase, len(completedPhases))
					}
				}
			}
		}
	}

	for !a.phaseManager.IsComplete() {
		select {
		case <-a.ctx.Done():
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

		phaseIdx := a.phaseManager.currentIdx + 1
		a.printPhaseChange(phase.Name, phase.Description, phaseIdx, len(a.phaseManager.phases))

		phaseCtx, phaseCancel := context.WithTimeout(a.ctx, PhaseTimeout)
		err := a.runPhaseHeadless(phaseCtx, phase)
		phaseCancel()

		if err != nil {
			if a.ctx.Err() != nil {
				_ = a.saveProgress(completedPhases, phase.Name, fmt.Sprintf("Agent was interrupted during %s phase.", phase.Name))
				a.trackInterrupted(phase.Name, len(completedPhases))
				return a.setCancelled()
			}

			if errors.Is(err, agenttools.ErrSetupAborted) {
				abortReason := "unknown"
				projectType := a.phaseManager.state.ProjectType
				var abortErr *agenttools.AbortError
				if errors.As(err, &abortErr) {
					abortReason = abortErr.Reason
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
				fmt.Println()
				fmt.Println(headlessErrorStyle.Render("🟠 Setup aborted. See message above for details."))
				return a.setCompleted()
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

				fmt.Println(headlessErrorStyle.Render(fmt.Sprintf("❌ Error: %s", err.Error())))
				fmt.Println()
				fmt.Println(RecoveryGuidance())
				return a.setFailed(fmt.Errorf("required phase %s failed: %w", phase.Name, err))
			}

			fmt.Println(headlessErrorStyle.Render(fmt.Sprintf("⚠️ Warning: %s", err.Error())))
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

	if a.skipToCloud {
		_ = a.saveProgress(completedPhases, "", "Cloud setup completed successfully.")
		a.trackEvent("drift_cli:setup_agent:cloud_completed", map[string]any{
			"phases_completed": len(completedPhases),
			"duration_ms":      time.Since(a.startTime).Milliseconds(),
			"skip_to_cloud":    true,
		})
		fmt.Println()
		fmt.Println(headlessSuccessStyle.Render("🎉 Cloud setup complete!"))
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

	if !a.phaseManager.HasCloudPhases() {
		fmt.Println()
		fmt.Println("───────────────────────────────────────────────────────────")
		fmt.Println()
		fmt.Println(headlessSuccessStyle.Render("✅ Local setup complete!"))
		fmt.Println()
		fmt.Println("Would you like to continue with Tusk Drift Cloud setup?")
		fmt.Println("This will connect your repository and enable cloud features.")
		fmt.Println()
		fmt.Print("Continue with cloud setup? [y/N]: ")

		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response == "y" || response == "yes" {
			a.phaseManager.AddCloudPhases()
			a.trackEvent("drift_cli:setup_agent:cloud_started", nil)

			// Run cloud phases
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

				phaseIdx := a.phaseManager.currentIdx + 1
				a.printPhaseChange(phase.Name, phase.Description, phaseIdx, len(a.phaseManager.phases))

				phaseCtx, phaseCancel := context.WithTimeout(a.ctx, PhaseTimeout)
				err := a.runPhaseHeadless(phaseCtx, phase)
				phaseCancel()

				if err != nil {
					if a.ctx.Err() != nil {
						_ = a.saveProgress(completedPhases, phase.Name, fmt.Sprintf("Agent was interrupted during %s phase.", phase.Name))
						a.trackInterrupted(phase.Name, len(completedPhases))
						return a.setCancelled()
					}

					if errors.Is(err, agenttools.ErrSetupAborted) {
						var abortErr *agenttools.AbortError
						if errors.As(err, &abortErr) {
							a.trackEvent("drift_cli:setup_agent:cloud_aborted", map[string]any{
								"phase":  phase.Name,
								"reason": abortErr.Reason,
							})
						}
						fmt.Println()
						fmt.Println(headlessErrorStyle.Render("🟠 Cloud setup aborted. See message above for details."))
						return a.setCompleted()
					}

					if phase.Required {
						_ = a.saveProgress(completedPhases, phase.Name, fmt.Sprintf("Cloud phase failed with error: %v", err))
						a.trackEvent("drift_cli:setup_agent:cloud_phase_failed", map[string]any{
							"phase": phase.Name,
							"error": err.Error(),
						})
						fmt.Println(headlessErrorStyle.Render(fmt.Sprintf("❌ Error: %s", err.Error())))
						return a.setFailed(fmt.Errorf("required cloud phase %s failed: %w", phase.Name, err))
					}

					fmt.Println(headlessErrorStyle.Render(fmt.Sprintf("⚠️ Warning: %s", err.Error())))
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

	fmt.Println()
	fmt.Println(headlessSuccessStyle.Render("🎉 Setup complete!"))
	fmt.Println(headlessDimStyle.Render("   Check .tusk/SETUP_REPORT.md for details."))

	return a.setCompleted()
}

// printPhaseChange prints a phase change header
func (a *Agent) printPhaseChange(name, desc string, phaseNum, totalPhases int) {
	if a.logger != nil {
		a.logger.LogPhaseStart(name, desc, phaseNum, totalPhases)
	}
	fmt.Println()
	fmt.Println(headlessPhaseStyle.Render(fmt.Sprintf("━━━ Phase %d/%d: %s ━━━", phaseNum, totalPhases, name)))
	fmt.Println(headlessDimStyle.Render(desc))
	fmt.Println()
}

// runPhaseHeadless runs a single phase in headless mode
func (a *Agent) runPhaseHeadless(ctx context.Context, phase *Phase) error {
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

		if a.logger != nil {
			a.logger.LogThinking(true)
		}
		fmt.Print(headlessDimStyle.Render("○ Thinking..."))

		var streamedText strings.Builder

		apiCtx, apiCancel := context.WithTimeout(ctx, APITimeout)
		resp, err := a.client.CreateMessageStreaming(apiCtx, systemPrompt, messages, tools, func(event StreamEvent) {
			if event.Type == "text" {
				streamedText.WriteString(event.Text)
			}
		})
		apiCancel()

		// Clear the "Thinking..." line
		fmt.Print("\r                    \r")

		if err != nil {
			apiErrorCount++
			errMsg := err.Error()

			if apiErrorCount < MaxAPIRetries && isRecoverableAPIError(errMsg) {
				fmt.Println(headlessErrorStyle.Render(fmt.Sprintf("API error (retrying): %s", errMsg)))
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

		apiErrorCount = 0
		a.totalTokensIn += resp.Usage.InputTokens
		a.totalTokensOut += resp.Usage.OutputTokens

		cleanedContent := cleanupContent(resp.Content)

		messages = append(messages, Message{
			Role:    "assistant",
			Content: cleanedContent,
		})

		for _, content := range cleanedContent {
			if content.Type == "text" && strings.TrimSpace(content.Text) != "" {
				if a.logger != nil {
					a.logger.LogMessage(content.Text)
				}
				fmt.Println(content.Text)
				fmt.Println()
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
			toolResults, err := a.executeToolCallsHeadless(ctx, cleanedContent)
			if err != nil {
				if errors.Is(err, agenttools.ErrSetupAborted) {
					return err
				}

				if a.logger != nil {
					a.logger.LogError(err)
				}
				fmt.Println(headlessErrorStyle.Render(fmt.Sprintf("Tool error: %s", err.Error())))
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

// executeToolCallsHeadless executes tool calls in headless mode
func (a *Agent) executeToolCallsHeadless(ctx context.Context, content []Content) ([]Content, error) {
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

		inputStr := string(c.Input)

		if a.logger != nil {
			a.logger.LogToolStart(c.Name, inputStr)
		}

		if c.Name != "transition_phase" {
			displayName := getToolDisplayName(c.Name)
			fmt.Println(headlessToolStyle.Render(fmt.Sprintf("🔧 %s", displayName)))
		}

		if c.Name == "ask_user" {
			result := a.handleAskUserHeadless(c)
			results = append(results, result)
			continue
		}

		if c.Name == "ask_user_select" {
			result := a.handleAskUserSelectHeadless(c)
			results = append(results, result)
			continue
		}

		// Check for port conflicts
		if c.Name == "start_background_process" || c.Name == "run_command" {
			if err := a.checkPortConflictsHeadless(c.Input); err != nil {
				if a.logger != nil {
					a.logger.LogToolComplete(c.Name, false, err.Error())
				}
				fmt.Println(headlessErrorStyle.Render(fmt.Sprintf("   ✗ %s", err.Error())))
				results = append(results, Content{
					Type:      "tool_result",
					ToolUseID: c.ID,
					Content:   fmt.Sprintf("Error: %s", err.Error()),
					IsError:   true,
				})
				continue
			}
		}

		// Check permissions
		if !a.skipPermissions {
			if toolDef := GetRegistry().Get(ToolName(c.Name)); toolDef != nil && toolDef.RequiresConfirmation {
				preview := formatToolPreview(c.Name, inputStr)
				if preview != "" {
					fmt.Println(headlessDimStyle.Render(fmt.Sprintf("   %s", preview)))
				}
				fmt.Print(headlessQuestionStyle.Render("   Allow? [y/n/a(ll)]: "))

				reader := bufio.NewReader(os.Stdin)
				response, _ := reader.ReadString('\n')
				response = strings.TrimSpace(strings.ToLower(response))

				switch response {
				case "y", "yes":
					// Continue
				case "a", "all":
					a.skipPermissions = true
				default:
					if a.logger != nil {
						a.logger.LogToolComplete(c.Name, false, "User denied permission")
					}
					fmt.Println(headlessDimStyle.Render("   ✗ Denied"))
					results = append(results, Content{
						Type:      "tool_result",
						ToolUseID: c.ID,
						Content:   "Error: User denied permission for this action. Please try a different approach.",
						IsError:   true,
					})
					continue
				}
			}
		}

		executor, ok := a.executors[c.Name]
		if !ok {
			if a.logger != nil {
				a.logger.LogToolComplete(c.Name, false, "unknown tool")
			}
			fmt.Println(headlessErrorStyle.Render(fmt.Sprintf("   ✗ Unknown tool: %s", c.Name)))
			results = append(results, Content{
				Type:      "tool_result",
				ToolUseID: c.ID,
				Content:   fmt.Sprintf("Unknown tool: %s", c.Name),
				IsError:   true,
			})
			continue
		}

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
			if a.logger != nil {
				a.logger.LogToolComplete(c.Name, false, "timeout")
			}
			fmt.Println(headlessErrorStyle.Render(fmt.Sprintf("   ✗ Timeout after %v", ToolTimeout)))
			results = append(results, Content{
				Type:      "tool_result",
				ToolUseID: c.ID,
				Content:   fmt.Sprintf("Error: tool execution timed out after %v", ToolTimeout),
				IsError:   true,
			})
		case res := <-resultCh:
			toolCancel()
			if res.err != nil {
				if errors.Is(res.err, agenttools.ErrSetupAborted) {
					if a.logger != nil {
						a.logger.LogToolComplete(c.Name, true, res.result)
					}
					return nil, res.err
				}

				if a.logger != nil {
					a.logger.LogToolComplete(c.Name, false, res.err.Error())
				}
				if c.Name != "transition_phase" {
					fmt.Println(headlessErrorStyle.Render(fmt.Sprintf("   ✗ %s", res.err.Error())))
				}
				results = append(results, Content{
					Type:      "tool_result",
					ToolUseID: c.ID,
					Content:   fmt.Sprintf("Error: %s", res.err.Error()),
					IsError:   true,
				})
			} else {
				if a.logger != nil {
					a.logger.LogToolComplete(c.Name, true, res.result)
				}
				if c.Name != "transition_phase" {
					displayName := getToolDisplayName(c.Name)
					fmt.Println(headlessSuccessStyle.Render(fmt.Sprintf("   ✓ %s", displayName)))
				}
				results = append(results, Content{
					Type:      "tool_result",
					ToolUseID: c.ID,
					Content:   res.result,
				})
			}
		}
	}

	return results, nil
}

// handleAskUserHeadless handles ask_user in headless mode
func (a *Agent) handleAskUserHeadless(c Content) Content {
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

	fmt.Println()
	fmt.Println(headlessQuestionStyle.Render("🤖 Agent needs your input:"))
	fmt.Println()
	fmt.Println(params.Question)
	fmt.Print(headlessInputStyle.Render("\n> "))

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return Content{
			Type:      "tool_result",
			ToolUseID: c.ID,
			Content:   fmt.Sprintf("Error: failed to read input: %v", err),
			IsError:   true,
		}
	}

	response = strings.TrimSpace(response)
	if response == "" {
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

	fmt.Println()
	return Content{
		Type:      "tool_result",
		ToolUseID: c.ID,
		Content:   response,
	}
}

// handleAskUserSelectHeadless handles ask_user_select in headless mode
func (a *Agent) handleAskUserSelectHeadless(c Content) Content {
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

	fmt.Println()
	fmt.Println(headlessQuestionStyle.Render("🤖 Agent needs your selection:"))
	fmt.Println()
	fmt.Println(params.Question)
	fmt.Println()
	for i, opt := range params.Options {
		fmt.Printf("  [%d] %s\n", i+1, opt.Label)
	}
	fmt.Print(headlessInputStyle.Render(fmt.Sprintf("\nEnter number (1-%d): ", len(params.Options))))

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return Content{
			Type:      "tool_result",
			ToolUseID: c.ID,
			Content:   fmt.Sprintf("Error: failed to read input: %v", err),
			IsError:   true,
		}
	}

	response = strings.TrimSpace(response)
	var selection int
	if _, err := fmt.Sscanf(response, "%d", &selection); err != nil || selection < 1 || selection > len(params.Options) {
		return Content{
			Type:      "tool_result",
			ToolUseID: c.ID,
			Content:   "User cancelled selection",
			IsError:   true,
		}
	}

	selected := params.Options[selection-1]

	if a.logger != nil {
		a.logger.LogUserSelect(params.Question, selected.ID, selected.Label)
	}

	fmt.Println(headlessDimStyle.Render(fmt.Sprintf("   Selected: %s", selected.Label)))
	fmt.Println()

	result := map[string]string{
		"selected_id":    selected.ID,
		"selected_label": selected.Label,
	}
	resultJSON, _ := json.Marshal(result)
	return Content{
		Type:      "tool_result",
		ToolUseID: c.ID,
		Content:   string(resultJSON),
	}
}

// checkPortConflictsHeadless checks for port conflicts in headless mode
func (a *Agent) checkPortConflictsHeadless(input json.RawMessage) error {
	var params struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil
	}

	ports := extractPortsFromCommand(params.Command)

	for _, port := range ports {
		if isPortInUse(port) {
			fmt.Println(headlessErrorStyle.Render(fmt.Sprintf("⚠️  Port %d is already in use", port)))
			fmt.Print("Kill process on port? [y/N]: ")

			reader := bufio.NewReader(os.Stdin)
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(strings.ToLower(response))

			if response == "y" || response == "yes" {
				if err := killProcessOnPort(port); err != nil {
					return fmt.Errorf("failed to kill process on port %d: %w", port, err)
				}
				fmt.Println(headlessDimStyle.Render("   Killed process."))
			} else {
				return fmt.Errorf("port %d is in use and user declined to kill process", port)
			}
		}
	}

	return nil
}
