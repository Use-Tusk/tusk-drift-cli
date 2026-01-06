package onboard

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) Init() tea.Cmd {
	// Auto-advance past validation if in valid directory
	if m.stepIdx == 0 && m.flow.Current(m.stepIdx).ID() == stepValidateRepo {
		if hasPackageJSON() { // TODO-PYTHON: Add `|| hasPythonProject()`
			m.advance()
		}
	}
	return textinput.Blink
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.flow.Current(m.stepIdx).ID() == stepConfirm && m.viewportReady {
			switch msg.String() {
			case "up", "down", "pgup", "pgdown", "home", "end":
				var cmd tea.Cmd
				m.viewport, cmd = m.viewport.Update(msg)
				return m, cmd
			}
		}

		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit

		case "ctrl+b", "left":
			cur := m.flow.Current(m.stepIdx).ID()
			if len(m.history) > 0 && cur != stepIntro && cur != stepDone {
				// Blur current input before navigating
				if idx := m.flow.Current(m.stepIdx).InputIndex(); idx >= 0 {
					m.inputs[idx].Blur()
				}

				targetIdx, newHist, ok := m.flow.PrevIndex(m.history)
				if !ok {
					return m, nil
				}
				m.history = newHist
				m.stepIdx = targetIdx

				m.clearValuesFromStep(m.flow.Current(targetIdx).ID())

				// Clear and focus the target step's input
				if idx := m.flow.Current(m.stepIdx).InputIndex(); idx >= 0 {
					m.inputs[idx].SetValue("")
					m.inputs[idx].Focus()
				}
				return m, nil
			}

		case "enter":
			switch m.flow.Current(m.stepIdx).ID() {
			case stepDone:
				return m, tea.Quit

			case stepValidateRepo:
				if hasPackageJSON() { // TODO-PYTHON: Add `|| hasPythonProject()`
					m.advance()
					return m, nil
				}
				return m, tea.Quit

			case stepIntro, stepRecordingIntro, stepReplayIntro:
				m.advance()
				return m, nil

			case stepSDKCompatibility, stepDockerSetup, stepDockerType:
				// Wait for specific keys
				return m, nil

			case stepConfirm:
				if err := m.saveConfig(); err != nil {
					m.Err = err
				}
				m.stepIdx = m.flow.index[stepDone]
				return m, nil

			default:
				if m.commitCurrent() {
					m.advance()
				}
				return m, nil
			}

		case "y":
			switch m.flow.Current(m.stepIdx).ID() {
			case stepSDKCompatibility:
				m.SDKCompatible = true
				m.advance()
				return m, nil

			case stepDockerSetup:
				m.UseDocker = true
				hasCompose := hasDockerCompose()
				hasDockerfile := hasDockerfile()
				switch {
				case hasCompose && !hasDockerfile:
					// Only compose exists - set type and skip the question
					m.DockerType = dockerTypeCompose
					m.DockerImageName = inferDockerImageName()
					m.DockerAppName = strings.Split(m.DockerImageName, ":")[0]
					m.DockerComposeServiceName = getFirstDockerComposeServiceName()
					m.advance() // to dockerType (adds dockerSetup to history)
					// Manually skip dockerType without adding to history
					m.stepIdx = m.flow.NextIndex(m.stepIdx, m)
					m.focusActiveInput()
				case hasDockerfile && !hasCompose:
					// Only dockerfile exists - set type and skip the question
					m.DockerType = dockerTypeFile
					m.DockerImageName = inferDockerImageName()
					m.DockerAppName = strings.Split(m.DockerImageName, ":")[0]
					m.advance() // to dockerType (adds dockerSetup to history)
					// Manually skip dockerType without adding to history
					m.stepIdx = m.flow.NextIndex(m.stepIdx, m)
					m.focusActiveInput()
				case hasCompose && hasDockerfile:
					// Both exist - ask which to use
					m.advance()
				default:
					// Neither exists (unusual) - ask anyway
					m.advance()
				}
				return m, nil

			case stepConfirm:
				if err := m.saveConfig(); err != nil {
					m.Err = err
				}
				m.stepIdx = m.flow.index[stepDone]
				return m, nil
			}

		case "n":
			switch m.flow.Current(m.stepIdx).ID() {
			case stepSDKCompatibility:
				errMsg := "❌ Your service may not be compatible for Tusk Drift at the moment.\n" +
					getSupportMessage()
				m.Err = fmt.Errorf("%s", errMsg)
				m.stepIdx = m.flow.index[stepDone]
				return m, nil

			case stepDockerSetup:
				m.UseDocker = false
				m.DockerType = dockerTypeNone
				m.advance()
				return m, nil

			case stepConfirm:
				return m, tea.Quit
			}

		case "c":
			if m.flow.Current(m.stepIdx).ID() == stepDockerType {
				m.DockerType = dockerTypeCompose
				m.DockerImageName = inferDockerImageName()
				m.DockerAppName = strings.Split(m.DockerImageName, ":")[0]
				m.DockerComposeServiceName = getFirstDockerComposeServiceName() // NEW
				m.advance()
				return m, nil
			}

		case "d":
			if m.flow.Current(m.stepIdx).ID() == stepDockerType {
				m.DockerType = dockerTypeFile
				m.DockerImageName = inferDockerImageName()
				m.DockerAppName = strings.Split(m.DockerImageName, ":")[0]
				m.advance()
				return m, nil
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	}

	idx := m.flow.Current(m.stepIdx).InputIndex()
	if idx >= 0 {
		return m, m.updateInputs(msg)
	}
	return m, nil
}

func (m *Model) updateInputs(msg tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, len(m.inputs))
	for i := range m.inputs {
		m.inputs[i], cmds[i] = m.inputs[i].Update(msg)
	}
	return tea.Batch(cmds...)
}

func (m *Model) advance() {
	cur := m.flow.Current(m.stepIdx).ID()
	// Don't add confirm/done to history, but DO add intro
	// ValidateRepo auto-advances so it shouldn't be in history for back navigation
	if cur != stepValidateRepo && cur != stepConfirm && cur != stepDone {
		m.history = append(m.history, m.stepIdx)
	}

	prev := m.flow.Current(m.stepIdx).InputIndex()
	if prev >= 0 {
		m.inputs[prev].Blur()
	}

	m.stepIdx = m.flow.NextIndex(m.stepIdx, m)

	// Skip dockerType when not using docker; stop when not using docker
	if m.flow.Current(m.stepIdx).ID() == stepDockerType && !m.UseDocker {
		m.stepIdx = m.flow.NextIndex(m.stepIdx, m)
	}
	if m.flow.Current(m.stepIdx).ID() == stepStopCommand && !m.UseDocker {
		m.stepIdx = m.flow.NextIndex(m.stepIdx, m)
	}

	m.focusActiveInput()
}

func (m *Model) focusActiveInput() {
	if idx := m.flow.Current(m.stepIdx).InputIndex(); idx >= 0 {
		m.inputs[idx].Focus()
	}
}
