package onboard

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
)

func getSupportMessage() string {
	return "We're actively working on adding support for more packages and languages.\n\n" +
		"Help us prioritize and bring Tusk Drift to your service!\n" +
		"Fill up this form to request support: " + styles.LinkStyle.Render("https://tally.so/r/w456Xo") + "\n\n" +
		"You may also:\n" +
		"  • create an issue at " + styles.LinkStyle.Render("https://github.com/Use-Tusk/drift-node-sdk/issues") + "\n" +
		"  • contact " + styles.LinkStyle.Render("support@usetusk.ai")
}

func inputLabelForStep(s onboardStep) string {
	switch s {
	case stepStartCommand:
		return "Command: "
	case stepStopCommand:
		return "Stop command: "
	case stepReadinessCommand:
		return "Readiness command: "
	case stepReadinessTimeout:
		return "Timeout: "
	case stepReadinessInterval:
		return "Interval: "
	default:
		return ""
	}
}

func (m *Model) serviceNameDefault() string {
	if (m.DockerType == dockerTypeFile || m.DockerType == dockerTypeCompose) && strings.TrimSpace(m.DockerAppName) != "" {
		return m.DockerAppName
	}
	return inferServiceNameFromDir()
}

func (m *Model) servicePortDefault() string {
	if m.DockerType == dockerTypeFile {
		if inferred := inferDockerPort(); inferred != "" && inferred != "3000" {
			return fmt.Sprintf("%s (detected from Dockerfile)", inferred)
		}
	}
	if m.DockerType == dockerTypeCompose {
		if inferred := inferDockerComposePort(); inferred != "" && inferred != "3000" {
			return fmt.Sprintf("%s (detected from Docker Compose)", inferred)
		}
	}
	return "3000"
}

func (m *Model) startCommandQuestion() string {
	switch m.DockerType {
	case dockerTypeCompose:
		return "What's your Docker Compose start command?"
	case dockerTypeFile:
		return "What's your Docker run command?"
	default:
		return "How do you start your service?"
	}
}

func (m *Model) startCommandDescription() string {
	switch m.DockerType {
	case dockerTypeCompose:
		return "Default includes the Tusk override file to add the required environment variables and host mapping"
	case dockerTypeFile:
		return "Default includes required environment variables and host mapping"
	default:
		return "e.g., npm run start"
	}
}

func (m *Model) startCommandDefault() string {
	switch m.DockerType {
	case dockerTypeCompose:
		return "docker compose -f docker-compose.yml -f docker-compose.tusk-override.yml up"
	case dockerTypeFile:
		port := strings.TrimSpace(m.ServicePort)
		if port == "" {
			port = "3000"
		}
		appName := m.DockerAppName
		if strings.TrimSpace(appName) == "" {
			appName = "my-app"
		}
		imageName := m.DockerImageName
		if strings.TrimSpace(imageName) == "" {
			imageName = "my-app-image:latest"
		}
		return fmt.Sprintf(`docker run -d \
        --name %s \
        --add-host=host.docker.internal:host-gateway \
        -p %s:%s \
        -e TUSK_MOCK_HOST=host.docker.internal \
        -e TUSK_MOCK_PORT=9001 \
        -e TUSK_DRIFT_MODE=REPLAY \
        %s`, appName, trimFirstToken(port), trimFirstToken(port), imageName)
	default:
		return "npm run start"
	}
}

func (m *Model) stopCommandDefault() string {
	switch m.DockerType {
	case dockerTypeCompose:
		return "docker compose down"
	case dockerTypeFile:
		appName := m.DockerAppName
		if strings.TrimSpace(appName) == "" {
			appName = "my-app"
		}
		return fmt.Sprintf("docker stop %s && docker rm %s", appName, appName)
	default:
		return ""
	}
}

func (m *Model) readinessCommandDefault() string {
	port := strings.TrimSpace(m.ServicePort)
	if port == "" {
		port = "3000"
	}
	return fmt.Sprintf("curl -fsS http://localhost:%s/health", trimFirstToken(port))
}

func trimFirstToken(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.Index(s, " "); idx > 0 {
		return s[:idx]
	}
	return s
}

func parseFloatSafe(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

func parseFloatInRange(s string, lo, hi float64, msg string) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || v < lo || v > hi {
		return 0, fmt.Errorf("%s", msg)
	}
	return v, nil
}

func errInvalidPort() error {
	return fmt.Errorf("Invalid port: must be an integer")
}

// clearValuesFromStep clears the target step and all steps after it
func (m *Model) clearValuesFromStep(targetStep onboardStep) {
	targetIdx := -1
	for i, step := range m.flow.steps {
		if step.ID() == targetStep {
			targetIdx = i
			break
		}
	}
	if targetIdx == -1 {
		return
	}

	for i := targetIdx; i < len(m.flow.steps); i++ {
		m.flow.steps[i].Clear(m)
	}
}

func (m *Model) commitCurrent() bool {
	s := m.flow.Current(m.stepIdx)
	idx := s.InputIndex()
	if idx < 0 {
		return true
	}

	def := s.Default(m)
	val := valueOrDefault(m.inputs[idx].Value(), def)

	if err := s.Validate(m, val); err != nil {
		m.ValidationErr = err
		return false
	}
	s.Apply(m, val)
	m.ValidationErr = nil
	return true
}

func valueOrDefault(v, def string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return def
}
