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

	targetService := m.DockerComposeServiceName
	if targetService == "" {
		// Fallback: try to find service by Tusk service name or use first service
		serviceNames, err := getDockerComposeServiceNames()
		if err != nil {
			return err
		}
		if len(serviceNames) > 0 {
			targetService = serviceNames[0]
		} else {
			return fmt.Errorf("no services found in docker-compose.yml")
		}
	}

	var buf strings.Builder
	buf.WriteString("# Tusk Drift override file for Docker Compose\n")
	buf.WriteString(fmt.Sprintf("# Environment variables for service: %s\n", targetService))
	buf.WriteString("# Please double check that this is the correct service.\n\n")
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
