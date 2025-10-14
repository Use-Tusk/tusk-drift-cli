package onboard

import (
	"fmt"
	"strings"

	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/components"
	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"
)

func (m *Model) View() string {
	header := components.Title(m.width, "TUSK DRIFT SETUP")

	var body strings.Builder
	switch m.flow.Current(m.stepIdx).ID() {
	case stepValidateRepo:
		if hasPackageJSON() {
			return ""
		}
		wd, _ := getwdSafe()
		body.WriteString(styles.ErrorStyle.Render("❌ Unable to initialize Tusk Drift in this directory") + "\n\n")
		switch {
		case hasJavaScriptFiles():
			body.WriteString("It looks like you have JavaScript/TypeScript files here, but no package.json.\n")
			body.WriteString("You may be in a subdirectory of your project.\n\n")
			body.WriteString(styles.WarningStyle.Render("→ Please run `tusk init` from your Node.js service root directory.") + "\n")
			body.WriteString(styles.DimStyle.Render(fmt.Sprintf("  Current directory: %s", wd)) + "\n\n")
			body.WriteString("Look for the directory containing your package.json file.\n")
		case isEmptyDirectory():
			body.WriteString("This directory appears to be empty.\n\n")
			body.WriteString("Please either:\n")
			body.WriteString("  • Navigate to your Node.js service root directory, or\n")
			body.WriteString("  • Initialize a new Node.js project with " + styles.SuccessStyle.Render("npm init") + "\n")
		default:
			body.WriteString("This doesn't appear to be a Node.js project.\n\n")
			body.WriteString("Tusk Drift currently supports Node.js services only." + "\n")
			body.WriteString(getSupportMessage() + "\n")
		}

	case stepIntro, stepRecordingIntro, stepReplayIntro:
		s := m.flow.Current(m.stepIdx)
		body.WriteString(styles.SuccessStyle.Render(s.Question(m)) + "\n\n")
		body.WriteString(s.Description(m) + "\n")

	case stepSDKCompatibility:
		s := m.flow.Current(m.stepIdx)
		body.WriteString(s.Question(m) + "\n\n")
		body.WriteString(s.Description(m) + "\n")

	case stepDockerSetup:
		s := m.flow.Current(m.stepIdx)
		body.WriteString(s.Question(m) + "\n")
		body.WriteString(styles.DimStyle.Render(s.Description(m)) + "\n\n")
		if hasDockerCompose() {
			body.WriteString(styles.SuccessStyle.Render("✓ Docker Compose file detected") + "\n")
		}
		if hasDockerfile() {
			body.WriteString(styles.SuccessStyle.Render("✓ Dockerfile detected") + "\n")
		}
		if hasDockerCompose() || hasDockerfile() {
			body.WriteString("\n")
		}

	case stepDockerType:
		s := m.flow.Current(m.stepIdx)
		body.WriteString("Both Docker Compose and Dockerfile detected.\n")
		body.WriteString(s.Question(m) + "\n")
		body.WriteString(styles.DimStyle.Render(s.Description(m)) + "\n\n")
		body.WriteString(styles.DimStyle.Render("Docker Compose: Use if you normally run `docker compose up`\n"))
		body.WriteString(styles.DimStyle.Render("Dockerfile: Use if you normally run `docker run`\n"))

	case stepConfirm:
		cfg := m.getCurrentConfig()
		var buf strings.Builder
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		_ = enc.Encode(cfg)
		_ = enc.Close()
		formatted := formatYAMLWithBlankLines([]byte(buf.String()))

		// Initialize viewport content if not already set or if config changed
		if !m.viewportReady {
			m.updateViewportSize()
		}

		// Set viewport content
		m.viewport.SetContent(styles.BoxStyle.Render(string(formatted)))

		body.WriteString("Some configurations are pre-filled.\n")
		body.WriteString("You may adjust them in the config file later if necesssary.\n")
		body.WriteString("Refer to the documentation for more details:\n")
		body.WriteString(styles.LinkStyle.Render("https://github.com/Use-Tusk/tusk-drift-cli/blob/main/docs/configuration.md") + "\n\n")
		body.WriteString(m.viewport.View() + "\n\n")
		body.WriteString(fmt.Sprintf("Save this configuration to %s/%s? (y/n)\n", configDir, configFile))

	case stepDone:
		if m.Err != nil {
			body.WriteString(m.Err.Error() + "\n")
		} else {
			body.WriteString(styles.SuccessStyle.Render(fmt.Sprintf("✅ Configuration saved to %s/%s", configDir, configFile)) + "\n\n")
			next := fmt.Sprintf(`Next steps:
1. Follow the instructions to get started with the SDK in your service:
   %s
   • Install the SDK
   • Initialize the SDK on your service
   • Record your first traces`, styles.LinkStyle.Render("https://github.com/Use-Tusk/drift-node-sdk#installation"))
			switch m.DockerType {
			case dockerTypeCompose:
				next += fmt.Sprintf(`
2. A docker-compose.tusk-override.yml file has been created for you.
   Review it and uncomment extra_hosts if you're on Linux.
   See: %s`, styles.LinkStyle.Render("https://github.com/Use-Tusk/tusk-drift-cli/blob/main/docs/configuration.md#docker-support"))
			case dockerTypeFile:
				next += `
2. Review the Docker run command in your config.
   Adjust the image name and ports if needed.`
			}
			next += fmt.Sprintf(`
3. [Optional] Obtain an API key from Tusk Drift Cloud: %s
   • Set your API key: export TUSK_API_KEY=your-key
4. Run tests: tusk run (see --help for more options)
`, styles.LinkStyle.Render("https://app.usetusk.ai/"))
			body.WriteString(next)
		}

	default:
		body.WriteString(m.summary())
		s := m.flow.Current(m.stepIdx)

		question := s.Question(m)
		description := s.Description(m)

		body.WriteString(question + "\n")
		if description != "" {
			body.WriteString(styles.DimStyle.Render(description) + "\n\n")
		} else {
			body.WriteString("\n")
		}

		def := s.Default(m)

		if len(def) > 60 || strings.Contains(def, "\n") || strings.Contains(def, "\\") {
			body.WriteString(styles.DimStyle.Render("Default value:") + "\n")
			body.WriteString(styles.BoxStyle.Render(def) + "\n\n")
			body.WriteString(styles.DimStyle.Render("Press enter to use default, or type your custom command:") + "\n")
		} else {
			body.WriteString(styles.DimStyle.Render("Default value: "+def) + "\n")
		}

		if idx := s.InputIndex(); idx >= 0 {
			body.WriteString(inputLabelForStep(s.ID()))
			body.WriteString(m.inputs[idx].View() + "\n")
		}
		if m.ValidationErr != nil {
			body.WriteString(styles.ErrorStyle.Render(m.ValidationErr.Error()) + "\n")
		}
	}

	helpText := m.flow.Current(m.stepIdx).Help(m)

	// Prepend back button if history exists and step allows going back
	// (Don't add back button to intro, validate, or done steps)
	cur := m.flow.Current(m.stepIdx).ID()
	if len(m.history) > 0 && cur != stepIntro && cur != stepValidateRepo && cur != stepDone {
		helpText = "ctrl+b/←: back • " + helpText
	}

	help := components.Footer(m.width, helpText)

	if m.width > 0 && m.height > 0 {
		top := header + "\n\n"
		bodyStr := body.String()
		want := max(m.height-lipgloss.Height(top)-lipgloss.Height(help), 0)
		have := lipgloss.Height(bodyStr)

		// For confirm step, viewport handles its own height, so don't pad
		if m.flow.Current(m.stepIdx).ID() != stepConfirm {
			if have < want {
				bodyStr += strings.Repeat("\n", want-have+2)
			}
		}
		return top + bodyStr + help
	}

	return header + "\n\n" + body.String() + "\n\n" + help
}

func (m *Model) summary() string {
	var b strings.Builder
	if strings.TrimSpace(m.ServiceName) != "" {
		b.WriteString(fmt.Sprintf("Service: %s\n", styles.SuccessStyle.Render(m.ServiceName)))
	}
	if strings.TrimSpace(m.ServicePort) != "" {
		b.WriteString(fmt.Sprintf("Port: %s\n", styles.SuccessStyle.Render(m.ServicePort)))
	}
	if strings.TrimSpace(m.StartCmd) != "" {
		b.WriteString(fmt.Sprintf("Start: %s\n", styles.SuccessStyle.Render(m.StartCmd)))
	}
	if strings.TrimSpace(m.StopCmd) != "" {
		b.WriteString(fmt.Sprintf("Stop: %s\n", styles.SuccessStyle.Render(m.StopCmd)))
	}
	if strings.TrimSpace(m.ReadinessCmd) != "" {
		b.WriteString(fmt.Sprintf("Readiness command: %s\n", styles.SuccessStyle.Render(m.ReadinessCmd)))
	}
	if strings.TrimSpace(m.ReadinessTimeout) != "" {
		b.WriteString(fmt.Sprintf("Timeout: %s\n", styles.SuccessStyle.Render(m.ReadinessTimeout)))
	}
	if strings.TrimSpace(m.ReadinessInterval) != "" {
		b.WriteString(fmt.Sprintf("Interval: %s\n", styles.SuccessStyle.Render(m.ReadinessInterval)))
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	return b.String()
}
