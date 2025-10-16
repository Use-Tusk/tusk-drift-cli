package onboard

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func hasDockerfile() bool {
	files := []string{"Dockerfile", "dockerfile"}
	for _, f := range files {
		if _, err := os.Stat(f); err == nil {
			return true
		}
	}
	return false
}

func hasDockerCompose() bool {
	files := []string{"docker-compose.yml", "docker-compose.yaml"}
	for _, f := range files {
		if _, err := os.Stat(f); err == nil {
			return true
		}
	}
	return false
}

func inferDockerPort() string {
	dockerfiles := []string{"Dockerfile", "dockerfile"}
	for _, df := range dockerfiles {
		if data, err := os.ReadFile(df); err == nil { //nolint:gosec
			lines := strings.SplitSeq(string(data), "\n")
			for line := range lines {
				if strings.HasPrefix(strings.TrimSpace(line), "EXPOSE") {
					parts := strings.Fields(line)
					if len(parts) >= 2 {
						return parts[1]
					}
				}
			}
		}
	}
	return "3000"
}

func inferDockerComposePort() string {
	composePath := getDockerComposeFilePath()
	if composePath == "" {
		return ""
	}

	data, err := os.ReadFile(composePath) //nolint:gosec
	if err != nil {
		return ""
	}

	var root yaml.Node // Preserves order
	if err := yaml.Unmarshal(data, &root); err != nil {
		return ""
	}

	if len(root.Content) == 0 {
		return ""
	}

	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return ""
	}

	var servicesNode *yaml.Node
	for i := 0; i < len(doc.Content); i += 2 {
		keyNode := doc.Content[i]
		if keyNode.Value == "services" && i+1 < len(doc.Content) {
			servicesNode = doc.Content[i+1]
			break
		}
	}

	if servicesNode == nil || servicesNode.Kind != yaml.MappingNode || len(servicesNode.Content) < 2 {
		return ""
	}

	// Get the first service (index 1 is the value node for the first service)
	firstServiceNode := servicesNode.Content[1]
	if firstServiceNode.Kind != yaml.MappingNode {
		return ""
	}

	var portsNode *yaml.Node
	for i := 0; i < len(firstServiceNode.Content); i += 2 {
		keyNode := firstServiceNode.Content[i]
		if keyNode.Value == "ports" && i+1 < len(firstServiceNode.Content) {
			portsNode = firstServiceNode.Content[i+1]
			break
		}
	}

	if portsNode == nil || portsNode.Kind != yaml.SequenceNode || len(portsNode.Content) == 0 {
		return ""
	}

	firstPortNode := portsNode.Content[0]

	// Handle string format: "8089:8080" or similar
	if firstPortNode.Kind == yaml.ScalarNode {
		return extractPublishedPort(firstPortNode.Value)
	}

	// Handle map format: {target: 8080, published: 8089}
	if firstPortNode.Kind == yaml.MappingNode {
		for i := 0; i < len(firstPortNode.Content); i += 2 {
			keyNode := firstPortNode.Content[i]
			if keyNode.Value == "published" && i+1 < len(firstPortNode.Content) {
				valueNode := firstPortNode.Content[i+1]
				return valueNode.Value
			}
		}
	}

	return ""
}

// extractPublishedPort extracts the published (host) port from a port string
// Handles formats like:
// - "8089:8080" -> "8089"
// - "0.0.0.0:8089:8080" -> "8089"
// - "8089" -> "8089"
func extractPublishedPort(portStr string) string {
	parts := strings.Split(portStr, ":")

	if len(parts) == 1 {
		// Just a single port number
		return strings.TrimSpace(parts[0])
	}

	if len(parts) == 2 {
		// Format: "published:target" or "ip:port"
		// If first part is an IP (contains dots), return second part
		// Otherwise return first part
		first := strings.TrimSpace(parts[0])
		if strings.Contains(first, ".") {
			return strings.TrimSpace(parts[1])
		}
		return first
	}

	if len(parts) == 3 {
		// Format: "ip:published:target"
		return strings.TrimSpace(parts[1])
	}

	return ""
}

func inferDockerImageName() string {
	// Try package.json "name"
	if data, err := os.ReadFile("package.json"); err == nil {
		type pkg struct {
			Name string `json:"name"`
		}
		var p pkg
		if json.Unmarshal(data, &p) == nil && strings.TrimSpace(p.Name) != "" {
			return p.Name + ":latest"
		}
		// Fallback: scan line by line for "name"
		sc := bufio.NewScanner(strings.NewReader(string(data)))
		for sc.Scan() {
			line := sc.Text()
			if strings.Contains(line, "\"name\"") {
				parts := strings.Split(line, ":")
				if len(parts) >= 2 {
					name := strings.Trim(parts[1], " \",")
					if name != "" {
						return name + ":latest"
					}
				}
			}
		}
	}
	return "my-app-image:latest"
}

func getDockerComposeFilePath() string {
	files := []string{"docker-compose.yml", "docker-compose.yaml"}
	for _, f := range files {
		if _, err := os.Stat(f); err == nil {
			return f
		}
	}
	return ""
}

// getFirstDockerComposeServiceName returns the name of the first service in docker-compose.yml
// This is typically the main application service
func getFirstDockerComposeServiceName() string {
	composePath := getDockerComposeFilePath()
	if composePath == "" {
		return ""
	}

	data, err := os.ReadFile(composePath) //nolint:gosec
	if err != nil {
		return ""
	}

	var root yaml.Node // Preserves order
	if err := yaml.Unmarshal(data, &root); err != nil {
		return ""
	}

	if len(root.Content) == 0 {
		return ""
	}

	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return ""
	}

	// Find the "services" key in the document, then get first service at index 0
	for i := 0; i < len(doc.Content); i += 2 {
		keyNode := doc.Content[i]
		if keyNode.Value == "services" && i+1 < len(doc.Content) {
			servicesNode := doc.Content[i+1]
			if servicesNode.Kind == yaml.MappingNode && len(servicesNode.Content) >= 2 {
				firstServiceName := servicesNode.Content[0].Value
				return firstServiceName
			}
		}
	}

	return ""
}
