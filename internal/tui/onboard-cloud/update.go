package onboardcloud

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type stepCompleteMsg struct{}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case verifyRepoAccessSuccessMsg:
		m.RepoAccessVerified = true
		m.RepoID = msg.repoID
		m.Err = nil

		step := m.flow.Current(m.stepIdx)
		if step != nil && step.ID() == stepVerifyRepoAccess {
			return m, func() tea.Msg { return stepCompleteMsg{} }
		}
		return m, nil

	case verifyRepoAccessErrorMsg:
		m.Err = msg.err
		m.RepoAccessVerified = false

		if strings.Contains(msg.err.Error(), "no GitHub/GitLab connection") {
			m.NeedsCodeHostingAuth = true
		}

		return m, nil

	case createObservableServiceSuccessMsg:
		m.ServiceID = msg.serviceID
		m.ServiceCreated = true
		m.Err = nil
		return m, func() tea.Msg { return stepCompleteMsg{} }

	case createObservableServiceErrorMsg:
		m.Err = msg.err
		m.ServiceCreated = false
		return m, nil

	case createApiKeySuccessMsg:
		m.ApiKeyID = msg.apiKeyID
		m.ApiKey = msg.apiKey
		m.Err = nil
		// Don't auto-advance yet - user needs to see and copy the key
		return m, nil

	case createApiKeyErrorMsg:
		m.Err = msg.err
		return m, nil

	case tea.KeyMsg:
		step := m.flow.Current(m.stepIdx)

		// Check if RecordingConfigTable is in edit mode - let it handle esc first
		if step != nil && step.ID() == stepRecordingConfig && m.RecordingConfigTable != nil && m.RecordingConfigTable.EditMode {
			if msg.String() == "esc" || msg.String() == "tab" {
				_, cmd := m.RecordingConfigTable.Update(msg)
				return m, cmd
			}
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c", "esc"))):
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+b", "left"))):
			// Don't go back if table is in edit mode
			if step != nil && step.ID() == stepRecordingConfig && m.RecordingConfigTable != nil && m.RecordingConfigTable.EditMode {
				if msg.String() == "left" {
					return m, nil
				}
			}

			// Go back to the previous step
			if m.stepIdx > 0 && len(m.history) > 0 {
				currentStep := m.flow.Current(m.stepIdx)

				// Pop from history
				prevIdx := m.history[len(m.history)-1]
				m.history = m.history[:len(m.history)-1]
				m.stepIdx = prevIdx

				// Clear current step's state
				if currentStep != nil {
					currentStep.Clear(&m)
				}

				// Clear errors
				m.Err = nil
				m.ValidationErr = nil

				step := m.flow.Current(m.stepIdx)
				if step != nil {
					if step.ID() == stepRecordingConfig {
						// Initialize RecordingConfigTable when going BACK to that step
						m.RecordingConfigTable = NewRecordingConfigTable(
							m.SamplingRate,
							m.ExportSpans,
							m.EnableEnvVarRecording,
						)
					}

					inputIdx := step.InputIndex()
					if inputIdx >= 0 && inputIdx < len(m.inputs) {
						m.inputs[inputIdx].Focus()
					}
				}
			}
			return m, nil
		}

		if step != nil && step.ID() == stepRecordingConfig {
			if m.RecordingConfigTable != nil {
				_, cmd := m.RecordingConfigTable.Update(msg)

				// Only handle Enter for saving if not in edit mode
				if msg.String() == "enter" && !m.RecordingConfigTable.EditMode {
					samplingRate, exportSpans, enableEnvVarRecording := m.RecordingConfigTable.GetValues()
					m.SamplingRate = fmt.Sprintf("%.2f", samplingRate)
					m.ExportSpans = exportSpans
					m.EnableEnvVarRecording = enableEnvVarRecording

					if err := saveRecordingConfig(samplingRate, exportSpans, enableEnvVarRecording); err != nil {
						m.Err = fmt.Errorf("failed to save: %w", err)
					} else {
						return m, func() tea.Msg { return stepCompleteMsg{} }
					}
				}

				return m, cmd
			}
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			return m.processStep()
		}

		if step != nil {
			inputIdx := step.InputIndex()
			if inputIdx >= 0 && inputIdx < len(m.inputs) {
				var cmd tea.Cmd
				m.inputs[inputIdx], cmd = m.inputs[inputIdx].Update(msg)
				return m, cmd
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		if m.width > 20 {
			// Leave some padding
			m.progress.Width = m.width - 10
		}

	case stepCompleteMsg:
		// Move to next step
		m.history = append(m.history, m.stepIdx)
		nextIdx := m.flow.NextIndex(m.stepIdx, &m)
		if nextIdx == m.stepIdx {
			return m, tea.Quit
		}
		m.stepIdx = nextIdx

		m.Err = nil
		m.ValidationErr = nil

		// Focus next input if needed
		step := m.flow.Current(m.stepIdx)
		if step != nil {
			// Initialize RecordingConfigTable when entering the step,
			// always recreate the table
			if step.ID() == stepRecordingConfig {
				m.RecordingConfigTable = NewRecordingConfigTable(
					m.SamplingRate,
					m.ExportSpans,
					m.EnableEnvVarRecording,
				)
			}

			inputIdx := step.InputIndex()
			if inputIdx >= 0 && inputIdx < len(m.inputs) {
				m.inputs[inputIdx].Focus()
				m.inputs[inputIdx].SetValue("")
				m.inputs[inputIdx].Placeholder = step.Default(&m)
			}

			if step.ShouldAutoProcess(&m) {
				return m.processStep()
			}
		}
	}

	return m, nil
}

func (m Model) processStep() (tea.Model, tea.Cmd) {
	step := m.flow.Current(m.stepIdx)
	if step == nil {
		return m, tea.Quit
	}

	inputValue := ""
	inputIdx := step.InputIndex()
	if inputIdx >= 0 && inputIdx < len(m.inputs) {
		inputValue = m.inputs[inputIdx].Value()
		if inputValue == "" {
			inputValue = step.Default(&m)
		}
	}

	if err := step.Validate(&m, inputValue); err != nil {
		m.ValidationErr = err
		return m, nil
	}

	step.Apply(&m, inputValue)

	// Execute any custom logic (after Apply, so state is updated)
	if cmd := step.Execute(&m); cmd != nil {
		return m, cmd
	}

	// Don't advance if there's an error after Execute()
	if m.Err != nil {
		return m, nil
	}

	return m, func() tea.Msg { return stepCompleteMsg{} }
}
