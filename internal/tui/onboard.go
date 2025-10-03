package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/components"
	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"
)

/**
This utility only sets up the config.yaml file, for local runs.

TODO:
- Add service ID for Tusk Drift Cloud setup (perhaps this will be a separate wizard)
*/

type onboardStep int

const (
	stepIntro onboardStep = iota
	stepServiceName
	stepServicePort
	stepStartCommand
	stepReadinessCommand
	stepReadinessTimeout
	stepReadinessInterval
	stepConfirm
	stepDone
)

const (
	configDir  = ".tusk"
	configFile = "config.yaml"
)

type onboardModel struct {
	step              onboardStep
	inputs            []textinput.Model
	serviceName       string
	servicePort       string
	startCmd          string
	readinessCmd      string
	readinessTimeout  string
	readinessInterval string
	width             int
	height            int
	err               error
	validationErr     error
}

type Config struct {
	Service       Service       `yaml:"service"`
	Traces        Traces        `yaml:"traces"`
	TestExecution TestExecution `yaml:"test_execution"`
	Recording     Recording     `yaml:"recording"`
	TuskAPI       *TuskAPI      `yaml:"tusk_api,omitempty"`
}

type Service struct {
	ID        string    `yaml:"id,omitempty"`
	Name      string    `yaml:"name"`
	Port      int       `yaml:"port"`
	Start     Start     `yaml:"start"`
	Readiness Readiness `yaml:"readiness_check"`
}

type Start struct {
	Command string `yaml:"command"`
}

type Readiness struct {
	Command  string `yaml:"command"`
	Timeout  string `yaml:"timeout"`
	Interval string `yaml:"interval"`
}

type TestExecution struct {
	Timeout string `yaml:"timeout"`
}

type Recording struct {
	SamplingRate float64 `yaml:"sampling_rate"`
}

type Traces struct {
	Dir string `yaml:"dir"`
}

type TuskAPI struct {
	URL string `yaml:"url"`
}

type stepDef struct {
	inputIdx    int
	question    string
	description string
	inputLabel  string
	defaultVal  string
	apply       func(*onboardModel, string)
}

var stepOrder = []onboardStep{
	stepServiceName,
	stepServicePort,
	stepStartCommand,
	stepReadinessCommand,
	stepReadinessTimeout,
	stepReadinessInterval,
}

var stepDefs = map[onboardStep]stepDef{
	stepServiceName: {
		inputIdx:    0,
		question:    "What's the name of your service?",
		description: "e.g., \"acme-backend\"",
		inputLabel:  "",
		defaultVal:  "my-service",
		apply: func(m *onboardModel, v string) {
			m.serviceName = v
		},
	},
	stepServicePort: {
		inputIdx:    1,
		question:    "What port does your service run on?",
		description: "e.g., 3000",
		inputLabel:  "",
		defaultVal:  "3000",
		apply: func(m *onboardModel, v string) {
			m.servicePort = v
		},
	},
	stepStartCommand: {
		inputIdx:    2,
		question:    "How do you start your service?",
		description: "e.g., npm run start",
		inputLabel:  "Command: ",
		defaultVal:  "npm run start",
		apply: func(m *onboardModel, v string) {
			m.startCmd = v
		},
	},
	stepReadinessCommand: {
		inputIdx:    3,
		question:    "How should we check if your service is ready?",
		description: "e.g., curl -fsS http://localhost:3000/health",
		inputLabel:  "Readiness command: ",
		defaultVal:  "curl -fsS http://localhost:3000/health",
		apply: func(m *onboardModel, v string) {
			m.readinessCmd = v
		},
	},
	stepReadinessTimeout: {
		inputIdx:    4,
		question:    "Maximum time to wait for the service to be ready upon startup:",
		description: "e.g., 30s, 1m",
		inputLabel:  "Timeout: ",
		defaultVal:  "30s",
		apply: func(m *onboardModel, v string) {
			m.readinessTimeout = v
		},
	},
	stepReadinessInterval: {
		inputIdx:    5,
		question:    "How often to check readiness upon startup:",
		description: "e.g., 1s, 500ms",
		inputLabel:  "Interval: ",
		defaultVal:  "1s",
		apply: func(m *onboardModel, v string) {
			m.readinessInterval = v
		},
	},
}

// validateNodeRepo ensures we're in a Node.js project by requiring package.json in CWD.
func validateNodeRepo() error {
	path := "package.json"
	if fi, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			wd, _ := os.Getwd()
			return fmt.Errorf("package.json not found in %s. Run `tusk init` from your Node.js service root.", wd)
		}
		return fmt.Errorf("failed to access package.json: %w", err)
	} else if fi.IsDir() {
		wd, _ := os.Getwd()
		return fmt.Errorf("expected a file at %s/package.json, found a directory", wd)
	}
	return nil
}

func RunOnboardingWizard() error {
	// TODO: make this generic for other languages
	if err := validateNodeRepo(); err != nil {
		return err
	}

	inputs := make([]textinput.Model, len(stepOrder))
	for i := range stepOrder {
		in := textinput.New()
		if i == 0 {
			in.Focus()
		}
		inputs[i] = in
	}

	m := onboardModel{
		step:   stepIntro,
		inputs: inputs,
	}

	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		return err
	}

	return nil
}

func (m onboardModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m onboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit

		case "enter":
			if m.step == stepDone {
				return m, tea.Quit
			}

			switch m.step {
			case stepConfirm:
				if err := m.saveConfig(); err != nil {
					m.err = err
				}
				m.step = stepDone

			default:
				if m.commitCurrent() {
					m.advance()
				}
			}

		case "y":
			if m.step == stepConfirm {
				if err := m.saveConfig(); err != nil {
					m.err = err
				}
				m.step = stepDone
			}

		case "n":
			if m.step == stepConfirm {
				return m, tea.Quit
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	cmd := m.updateInputs(msg)
	return m, cmd
}

func (m *onboardModel) updateInputs(msg tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, len(m.inputs))
	for i := range m.inputs {
		m.inputs[i], cmds[i] = m.inputs[i].Update(msg)
	}
	return tea.Batch(cmds...)
}

func (m onboardModel) View() string {
	header := components.Title(m.width, "TUSK DRIFT SETUP")

	var body strings.Builder
	switch m.step {
	case stepIntro:
		body.WriteString("This wizard will help you configure Tusk Drift for your service (the current directory).\n")
		body.WriteString(fmt.Sprintf("A config file, %s/%s, will be created.\n\n", configDir, configFile))
		body.WriteString("Press [enter] to continue.\n")

	case stepConfirm:
		config := m.getCurrentConfig()
		raw, _ := yaml.Marshal(config)
		formatted := formatYAMLWithBlankLines(raw)
		body.WriteString("Some configurations are pre-filled.\n")
		body.WriteString("You may adjust them in the config file later if necesssary.\n")
		body.WriteString("Refer to the documentation for more details.\n\n")
		body.WriteString(styles.BoxStyle.Render(string(formatted)) + "\n\n")
		body.WriteString(fmt.Sprintf("Save this configuration to %s/%s? (y/n)\n", configDir, configFile))

	case stepDone:
		if m.err != nil {
			body.WriteString(styles.ErrorStyle.Render("❌ Error: "+m.err.Error()) + "\n")
		} else {
			body.WriteString(styles.SuccessStyle.Render(fmt.Sprintf("✅ Configuration saved to %s/%s", configDir, configFile)) + "\n\n")

			// TODO: update this to point to site docs
			nextSteps := `Next steps:
1. Follow the instructions to get started with the SDK in your service: https://github.com/Use-Tusk/drift-node-sdk#installation
   - Install the SDK
   - Initialize the SDK on your service
   - Record your first traces
2. [Optional] Obtain an API key from Tusk Drift Cloud: https://app.usetusk.ai/
   - Set your API key: export TUSK_API_KEY=your-key
3. Run tests: tusk run (see --help for more options)
`
			body.WriteString(nextSteps)
		}

	default:
		body.WriteString(m.summary())
		def := stepDefs[m.step]
		body.WriteString(def.question + "\n")
		if def.description != "" {
			body.WriteString(styles.DimStyle.Render(def.description) + "\n\n")
		} else {
			body.WriteString("\n")
		}
		if def.inputLabel != "" {
			body.WriteString(def.inputLabel)
		}
		body.WriteString(m.inputs[def.inputIdx].View() + "\n")
		if m.validationErr != nil {
			body.WriteString(styles.ErrorStyle.Render(m.validationErr.Error()) + "\n")
		}
	}

	var helpText string
	switch m.step {
	case stepIntro:
		helpText = "enter: continue • esc: quit"
	case stepConfirm:
		helpText = "y: yes • n: no • esc: quit"
	case stepDone:
		helpText = "enter/esc: quit"
	default:
		def := stepDefs[m.step]
		helpText = fmt.Sprintf("enter: next / accept default (%s) • esc: quit", def.defaultVal)
	}
	help := components.Footer(m.width, helpText)

	if m.width > 0 && m.height > 0 {
		top := header + "\n\n"
		bodyStr := body.String()
		want := max(m.height-lipgloss.Height(top)-lipgloss.Height(help), 0)
		have := lipgloss.Height(bodyStr)
		if have < want {
			bodyStr += strings.Repeat("\n", want-have+2)
		}
		return top + bodyStr + help
	}

	// Fallback when size unknown
	return header + "\n\n" + body.String() + "\n\n" + help
}

func (m onboardModel) summary() string {
	var b strings.Builder
	if m.serviceName != "" {
		b.WriteString(fmt.Sprintf("Service: %s\n", styles.SuccessStyle.Render(m.serviceName)))
	}
	if m.servicePort != "" {
		b.WriteString(fmt.Sprintf("Port: %s\n", styles.SuccessStyle.Render(m.servicePort)))
	}
	if m.startCmd != "" {
		b.WriteString(fmt.Sprintf("Start: %s\n", styles.SuccessStyle.Render(m.startCmd)))
	}
	if m.readinessCmd != "" {
		b.WriteString(fmt.Sprintf("Readiness command: %s\n", styles.SuccessStyle.Render(m.readinessCmd)))
	}
	if m.readinessTimeout != "" {
		b.WriteString(fmt.Sprintf("Timeout: %s\n", styles.SuccessStyle.Render(m.readinessTimeout)))
	}
	if m.readinessInterval != "" {
		b.WriteString(fmt.Sprintf("Interval: %s\n", styles.SuccessStyle.Render(m.readinessInterval)))
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	return b.String()
}

func (m *onboardModel) commitCurrent() bool {
	def, ok := stepDefs[m.step]
	if !ok || def.apply == nil {
		return true
	}
	val := m.valueOrDefault(def.inputIdx, def.defaultVal)
	if m.step == stepServicePort {
		if _, err := strconv.Atoi(strings.TrimSpace(val)); err != nil {
			m.validationErr = fmt.Errorf("Invalid port: must be an integer")
			return false
		}
	}
	def.apply(m, val)
	m.validationErr = nil
	return true
}

func (m *onboardModel) advance() {
	prev := inputIdxForStep(m.step)
	if prev >= 0 {
		m.inputs[prev].Blur()
	}
	m.step = nextStep(m.step)
	if idx := inputIdxForStep(m.step); idx >= 0 {
		m.inputs[idx].Focus()
	}
}

func inputIdxForStep(s onboardStep) int {
	if def, ok := stepDefs[s]; ok {
		return def.inputIdx
	}
	return -1
}

func nextStep(s onboardStep) onboardStep {
	switch s {
	case stepIntro:
		return stepServiceName
	case stepServiceName:
		return stepServicePort
	case stepServicePort:
		return stepStartCommand
	case stepStartCommand:
		return stepReadinessCommand
	case stepReadinessCommand:
		return stepReadinessTimeout
	case stepReadinessTimeout:
		return stepReadinessInterval
	case stepReadinessInterval:
		return stepConfirm
	case stepConfirm:
		return stepDone
	default:
		return s
	}
}

func (m onboardModel) getCurrentConfig() Config {
	timeout := m.readinessTimeout
	if strings.TrimSpace(timeout) == "" {
		timeout = "30s"
	}
	interval := m.readinessInterval
	if strings.TrimSpace(interval) == "" {
		interval = "1s"
	}

	port := 3000
	if p, err := strconv.Atoi(strings.TrimSpace(m.servicePort)); err == nil {
		port = p
	}

	return Config{
		Service: Service{
			Name: m.serviceName,
			Port: port,
			Start: Start{
				Command: m.startCmd,
			},
			Readiness: Readiness{
				Command:  m.readinessCmd,
				Timeout:  timeout,
				Interval: interval,
			},
		},
		Traces: Traces{
			Dir: ".tusk/traces",
		},
		TuskAPI: &TuskAPI{
			URL: "https://api.usetusk.ai",
		},
		TestExecution: TestExecution{
			Timeout: "30s",
		},
		Recording: Recording{
			SamplingRate: 1.0,
		},
	}
}

func (m onboardModel) saveConfig() error {
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, configFile)

	config := m.getCurrentConfig()

	raw, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	data := formatYAMLWithBlankLines(raw)

	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func (m onboardModel) valueOrDefault(idx int, def string) string {
	if idx < 0 || idx >= len(m.inputs) {
		return def
	}
	if v := m.inputs[idx].Value(); strings.TrimSpace(v) != "" {
		return v
	}
	return def
}

// formatYAMLWithBlankLines inserts a blank line before each top-level key (after the first)
// to improve readability of the YAML output.
func formatYAMLWithBlankLines(data []byte) []byte {
	s := string(data)
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines)+8)
	firstTop := true

	for _, line := range lines {
		if isTopLevelKey(line) && !firstTop {
			if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
				out = append(out, "")
			}
		}
		out = append(out, line)
		if isTopLevelKey(line) && firstTop {
			firstTop = false
		}
	}
	return []byte(strings.Join(out, "\n"))
}

func isTopLevelKey(line string) bool {
	if line == "" {
		return false
	}

	// Ignore document markers and comments
	if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "...") || strings.HasPrefix(line, "#") {
		return false
	}

	// Must be unindented (top-level), not an array item, and contain a colon
	if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "-") {
		return false
	}

	return strings.Contains(line, ":")
}
