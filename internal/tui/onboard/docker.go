package onboard

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
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
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
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
