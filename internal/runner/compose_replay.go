package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Use-Tusk/tusk-drift-cli/internal/log"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
	shellquote "github.com/kballard/go-shellquote"
	"gopkg.in/yaml.v3"
)

type replayComposeOverride struct {
	Services map[string]replayComposeService `yaml:"services"`
}

type replayComposeService struct {
	Environment map[string]string `yaml:"environment,omitempty"`
}

const replayComposeServiceSourceFile = "docker-compose.tusk-override.yml"

func (e *Executor) setReplayComposeOverride(path string) {
	e.replayComposeOverride = path
}

func (e *Executor) getReplayComposeOverride() string {
	return e.replayComposeOverride
}

// createReplayComposeOverrideFile builds a temporary compose override that injects
// recorded env vars into every discovered compose service for the current group.
// Service discovery is intentionally scoped to docker-compose.tusk-override.yml.
// If that file is absent or has no services, replay env override injection is skipped.
func createReplayComposeOverrideFile(command string, envVars map[string]string, groupName string) (string, error) {
	if len(envVars) == 0 {
		return "", nil
	}

	serviceNames, err := extractComposeServiceNames(command)
	if err != nil {
		return "", err
	}
	if len(serviceNames) == 0 {
		return "", nil
	}

	safeGroup := sanitizePathComponent(groupName)
	if safeGroup == "" {
		safeGroup = "default"
	}
	tempFile, err := os.CreateTemp("", fmt.Sprintf("tusk-replay-env-override-%s-*.yml", safeGroup))
	if err != nil {
		return "", fmt.Errorf("failed to create temporary replay compose override file: %w", err)
	}
	overridePath := tempFile.Name()
	if closeErr := tempFile.Close(); closeErr != nil {
		os.Remove(overridePath)
		return "", fmt.Errorf("failed to close temporary replay compose override file: %w", closeErr)
	}

	override := replayComposeOverride{
		Services: make(map[string]replayComposeService, len(serviceNames)),
	}

	for _, serviceName := range serviceNames {
		environment := make(map[string]string, len(envVars))
		for key, value := range envVars {
			environment[key] = value
		}
		override.Services[serviceName] = replayComposeService{Environment: environment}
	}

	content, err := yaml.Marshal(override)
	if err != nil {
		os.Remove(overridePath)
		return "", fmt.Errorf("failed to marshal replay compose override: %w", err)
	}

	if err := os.WriteFile(overridePath, content, 0o600); err != nil {
		os.Remove(overridePath)
		return "", fmt.Errorf("failed to write replay compose override file: %w", err)
	}

	return overridePath, nil
}

// extractComposeServiceNames reads service names exclusively from
// docker-compose.tusk-override.yml in the tusk root.
func extractComposeServiceNames(command string) ([]string, error) {
	_ = command
	composeFile := filepath.Join(utils.GetTuskRoot(), replayComposeServiceSourceFile)
	data, err := os.ReadFile(composeFile) // #nosec G304
	if err != nil {
		if os.IsNotExist(err) {
			log.Debug("Replay compose service source file not found; skipping replay env override",
				"path", composeFile)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read compose service source file %s: %w", composeFile, err)
	}

	var parsed struct {
		Services map[string]any `yaml:"services"`
	}
	if unmarshalErr := yaml.Unmarshal(data, &parsed); unmarshalErr != nil {
		return nil, fmt.Errorf("failed to parse compose service source file %s: %w", composeFile, unmarshalErr)
	}

	if len(parsed.Services) == 0 {
		log.Debug("Compose service source file has no services; skipping replay env override",
			"path", composeFile)
		return nil, nil
	}

	serviceNames := make([]string, 0, len(parsed.Services))
	for serviceName := range parsed.Services {
		serviceNames = append(serviceNames, serviceName)
	}
	sort.Strings(serviceNames)
	return serviceNames, nil
}

func isComposeBasedStartCommand(command string) bool {
	return strings.Contains(command, replayComposeServiceSourceFile)
}

// injectComposeOverrideFile inserts an extra "-f <override>" before the compose
// subcommand (up/run/etc.) so compose treats it as a merged config file.
// This preserves existing flags and subcommand arguments.
func injectComposeOverrideFile(command, overridePath string) (string, error) {
	// shellquote.Split/Join are used instead of strings.Fields/Join to correctly
	// handle quoted arguments and paths with spaces (e.g. -f "my compose.yml").
	tokens, err := shellquote.Split(command)
	if err != nil {
		return "", fmt.Errorf("failed to parse compose command: %w", err)
	}
	_, composeArgsStart, ok := findComposeInvocation(tokens)
	if !ok {
		return command, nil
	}

	subcommandIdx, _, err := findComposeSubcommandAndFiles(tokens, composeArgsStart)
	if err != nil {
		return "", err
	}
	if subcommandIdx == -1 {
		subcommandIdx = len(tokens)
	}

	injected := make([]string, 0, len(tokens)+2)
	injected = append(injected, tokens[:subcommandIdx]...)
	injected = append(injected, "-f", overridePath)
	injected = append(injected, tokens[subcommandIdx:]...)

	return shellquote.Join(injected...), nil
}

// findComposeInvocation identifies where compose starts in a shell command.
// It supports:
//   - "docker compose ..."
//   - "docker-compose ..."
//   - "tusk-compose ..." wrappers
//
// The returned composeArgsStart index points to the first token after the
// compose executable/verb where global compose flags are expected.
func findComposeInvocation(tokens []string) (commandIdx int, composeArgsStart int, ok bool) {
	for i := 0; i < len(tokens); i++ {
		token := strings.ToLower(tokens[i])
		base := filepath.Base(token)

		if token == "docker" && i+1 < len(tokens) && strings.ToLower(tokens[i+1]) == "compose" {
			return i, i + 2, true
		}
		if base == "docker-compose" || base == "tusk-compose" {
			return i, i + 1, true
		}
	}
	return -1, -1, false
}

// findComposeSubcommandAndFiles walks compose-global args until it finds the
// subcommand token (e.g. "up", "run") and collects any compose file paths from
// -f/--file flags encountered before that point.
func findComposeSubcommandAndFiles(tokens []string, composeArgsStart int) (subcommandIdx int, composeFiles []string, err error) {
	expectingValue := ""

	for i := composeArgsStart; i < len(tokens); i++ {
		token := tokens[i]

		if expectingValue != "" {
			if expectingValue == "file" {
				composeFiles = append(composeFiles, token)
			}
			expectingValue = ""
			continue
		}

		switch {
		case token == "-f" || token == "--file":
			expectingValue = "file"
			continue
		case strings.HasPrefix(token, "-f="):
			composeFiles = append(composeFiles, strings.TrimPrefix(token, "-f="))
			continue
		case strings.HasPrefix(token, "--file="):
			composeFiles = append(composeFiles, strings.TrimPrefix(token, "--file="))
			continue
		case strings.HasPrefix(token, "-"):
			if composeFlagNeedsValue(token) {
				expectingValue = "flag"
			}
			continue
		default:
			subcommandIdx = i
			return subcommandIdx, composeFiles, nil
		}
	}

	if expectingValue == "file" {
		return -1, nil, fmt.Errorf("compose command has -f/--file without a value")
	}

	return -1, composeFiles, nil
}

// composeFlagNeedsValue marks compose-global flags that consume the next token.
// This keeps argument scanning aligned when identifying the subcommand.
func composeFlagNeedsValue(flag string) bool {
	switch flag {
	case "-p", "--project-name", "--project-directory", "--env-file", "--profile", "--parallel", "--progress", "--ansi":
		return true
	default:
		return false
	}
}

func sanitizePathComponent(input string) string {
	if input == "" {
		return ""
	}

	var b strings.Builder
	for _, r := range input {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteRune('-')
	}
	return strings.Trim(b.String(), "-")
}

// filterReplayEnvVarsForCompose removes internal runtime keys that should be
// controlled by replay startup logic (not by recorded env snapshots).
func filterReplayEnvVarsForCompose(envVars map[string]string) (map[string]string, []string) {
	filtered := make(map[string]string, len(envVars))
	skipped := make([]string, 0)

	for key, value := range envVars {
		if strings.HasPrefix(key, "TUSK_") {
			skipped = append(skipped, key)
			continue
		}
		filtered[key] = value
	}

	sort.Strings(skipped)
	return filtered, skipped
}
