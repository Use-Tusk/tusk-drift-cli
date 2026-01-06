package onboard

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func RunOnboardingWizard() error {
	steps := stepsList()
	maxInputIdx := -1
	for _, step := range steps {
		if idx := step.InputIndex(); idx > maxInputIdx {
			maxInputIdx = idx
		}
	}

	numInputs := maxInputIdx + 1
	inputs := make([]textinput.Model, numInputs)

	for i := range inputs {
		in := textinput.New()
		if i == 0 {
			in.Focus()
		}
		inputs[i] = in
	}

	projectType := detectProjectType()

	// Only fetch SDK manifest for Node.js projects (Python SDK not yet ready)
	var sdkDescription string
	if projectType == "nodejs" { // TODO-PYTHON: Add case for Python
		sdkDescription = FetchSDKPackagesDescription()
	}

	m := &Model{
		stepIdx:                0, // stepValidateRepo
		inputs:                 inputs,
		SamplingRate:           "1.0",
		ReadinessTimeout:       "30s",
		ReadinessInterval:      "1s",
		SDKPackagesDescription: sdkDescription,
		ProjectType:            projectType,
	}
	m.flow = NewFlow(stepsList())

	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		return err
	}
	return nil
}
