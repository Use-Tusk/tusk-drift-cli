package onboardcloud

import (
	"fmt"
	"strings"

	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/components"
	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	if m.flow == nil {
		return "Initializing..."
	}

	step := m.flow.Current(m.stepIdx)
	if step == nil {
		return "Done"
	}

	header := components.Title(m.width, "TUSK DRIFT CLOUD SETUP")

	var body strings.Builder

	progressPercent := float64(m.stepIdx) / float64(len(m.flow.steps)-1)
	stepText := fmt.Sprintf("Step %d/%d", m.stepIdx+1, len(m.flow.steps))

	// Calculate progress bar width to fill the remaining terminal width
	spacing := 3                                      // Space between step text and bar
	barWidth := m.width - len(stepText) - spacing - 2 // -2 for margins
	barWidth = max(barWidth, 20)

	// Temporarily update progress bar width to fill available space
	originalWidth := m.progress.Width
	m.progress.Width = barWidth
	progressBar := m.progress.ViewAs(progressPercent)
	m.progress.Width = originalWidth

	progressLine := lipgloss.JoinHorizontal(
		lipgloss.Left,
		styles.DimStyle.Render(stepText),
		strings.Repeat(" ", spacing),
		progressBar,
	)
	body.WriteString(progressLine + "\n\n")

	switch step.ID() {
	case stepDone:
		if m.Err != nil {
			body.WriteString(m.Err.Error() + "\n")
		} else {
			body.WriteString(styles.SuccessStyle.Render(step.Heading(&m)) + "\n\n")
			body.WriteString(step.Description(&m) + "\n")
		}

	default:
		body.WriteString(styles.HeadingStyle.Render(step.Heading(&m)) + "\n\n")

		if step.ShouldSkip(&m) {
			skipReason := step.SkipReason(&m)
			if skipReason == "" {
				skipReason = "This step was skipped based on your current configuration."
			}
			body.WriteString(styles.SuccessStyle.Render("✓ Step skipped") + "\n\n")
			body.WriteString(skipReason + "\n")
		} else {

			desc := step.Description(&m)
			if desc != "" {
				body.WriteString(desc + "\n\n")
			}

			if step.ID() == stepRecordingConfig && m.RecordingConfigTable != nil {
				body.WriteString(m.RecordingConfigTable.View() + "\n")
			} else {

				inputIdx := step.InputIndex()
				if inputIdx >= 0 && inputIdx < len(m.inputs) {

					shouldShowInput := true
					// Hide input for CreateApiKeyStep when showing the created key
					if step.ID() == stepCreateApiKey && m.ApiKey != "" {
						shouldShowInput = false
					}

					if shouldShowInput {
						def := step.Default(&m)
						if def != "" {
							body.WriteString(styles.DimStyle.Render("Default value: "+def) + "\n")
						}
						body.WriteString(m.inputs[inputIdx].View() + "\n")
					}
				}
			}

			// Validation error
			if m.ValidationErr != nil {
				body.WriteString(styles.ErrorStyle.Render(fmt.Sprintf("✗ %s", m.ValidationErr.Error())) + "\n")
			}

			// General error
			if m.Err != nil {
				shouldShowError := true
				if step.ID() == stepVerifyRepoAccess && m.NeedsGithubAuth {
					// Skip rendering error for VerifyRepoAccessStep when showing GitHub auth instructions
					// (the Description already contains all the info)
					shouldShowError = false
				}

				if shouldShowError {
					body.WriteString(styles.ErrorStyle.Render(fmt.Sprintf("Error: %s", m.Err.Error())) + "\n")
				}
			}
		}
	}

	helpText := step.Help(&m)
	if m.stepIdx > 0 {
		helpText = "ctrl+b/←: back • " + helpText
	}

	footer := components.Footer(m.width, helpText)

	if m.width > 0 && m.height > 0 {
		top := header + "\n\n"
		bodyStr := body.String()
		footerStr := footer

		page := top + bodyStr + footerStr
		gap := m.height - lipgloss.Height(page)
		if gap > 0 {
			return top + bodyStr + strings.Repeat("\n", gap) + footerStr
		}
		return page
	}

	return header + "\n\n" + body.String() + "\n\n" + footer
}
