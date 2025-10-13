package onboard

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type DockerCompose struct {
	Services map[string]any `yaml:"services"`
}

func getDockerComposeServiceNames() ([]string, error) {
	composePath := getDockerComposeFilePath()
	if composePath == "" {
		return nil, fmt.Errorf("docker-compose file not found")
	}
	data, err := os.ReadFile(composePath) // #nosec G304
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", composePath, err)
	}
	var compose DockerCompose
	if err := yaml.Unmarshal(data, &compose); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", composePath, err)
	}
	if len(compose.Services) == 0 {
		return nil, fmt.Errorf("no services found in %s", composePath)
	}
	serviceNames := make([]string, 0, len(compose.Services))
	for name := range compose.Services {
		serviceNames = append(serviceNames, name)
	}
	return serviceNames, nil
}

func (m *Model) createDockerComposeOverrideFile() error {
	overridePath := "docker-compose.tusk-override.yml"
	if _, err := os.Stat(overridePath); err == nil {
		return nil
	}

	serviceNames, err := getDockerComposeServiceNames()
	if err != nil {
		return err
	}

	targetService := m.ServiceName

	found := false
	for _, name := range serviceNames {
		if name == targetService {
			found = true
			break
		}
	}
	if !found && len(serviceNames) > 0 {
		fmt.Fprintf(os.Stderr, "Warning: Service '%s' not found in docker-compose.yml\n", targetService)
		fmt.Fprintf(os.Stderr, "Available services: %s\n", strings.Join(serviceNames, ", "))
		fmt.Fprintf(os.Stderr, "Creating override file with '%s' - you may need to edit it manually.\n\n", targetService)
	}

	var buf strings.Builder
	buf.WriteString("# Tusk Drift override file for Docker Compose\n")
	buf.WriteString(fmt.Sprintf("# Environment variables for service: %s\n\n", targetService))
	buf.WriteString("services:\n")
	buf.WriteString(fmt.Sprintf("  %s:\n", targetService))
	buf.WriteString("    environment:\n")
	buf.WriteString("      TUSK_DRIFT_MODE: ${TUSK_DRIFT_MODE:-REPLAY}\n")
	buf.WriteString("      TUSK_MOCK_HOST: ${TUSK_MOCK_HOST:-host.docker.internal}\n")
	buf.WriteString("      TUSK_MOCK_PORT: ${TUSK_MOCK_PORT:-9001}\n")
	buf.WriteString("\n")
	buf.WriteString("    # Uncomment this if you are running on Linux\n")
	buf.WriteString("    # extra_hosts:\n")
	buf.WriteString("    #   - \"host.docker.internal:host-gateway\"\n")

	if err := os.WriteFile(overridePath, []byte(buf.String()), 0o600); err != nil {
		return fmt.Errorf("failed to write %s: %w", overridePath, err)
	}
	return nil
}
