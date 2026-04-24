package runner

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Use-Tusk/tusk-cli/internal/log"
	"github.com/Use-Tusk/tusk-cli/internal/utils"
	"gopkg.in/yaml.v3"
	"mvdan.cc/sh/v3/syntax"
)

type replayComposeOverride struct {
	Services map[string]replayComposeService `yaml:"services"`
}

type replayComposeService struct {
	Environment map[string]string `yaml:"environment,omitempty"`
}

const replayComposeServiceSourceFile = "docker-compose.tusk-override.yml"

var composeFileFlagPattern = regexp.MustCompile(`(?i)(-f|--file)(=|\s+)('([^']*)'|"([^"]*)"|([^\s;&|]+))`)

func (e *Executor) setReplayComposeOverride(path string) {
	e.replayComposeOverride = path
}

func (e *Executor) getReplayComposeOverride() string {
	return e.replayComposeOverride
}

func removeOverrideFile(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Debug("Failed to remove replay compose override file",
			"path", path,
			"error", err)
	}
}

// createReplayComposeOverrideFile builds a temporary compose override that uses
// ${VAR} interpolation references to inject recorded env vars into every discovered
// compose service for the current group. The actual values are read from the
// docker compose command environment (injected when the service subprocess is
// started), so the override file never contains secret values directly.
// Service discovery is intentionally scoped to docker-compose.tusk-override.yml.
// If that file is absent or has no services, replay env override injection is skipped.
func createReplayComposeOverrideFile(envVars map[string]string, groupName string) (string, error) {
	if len(envVars) == 0 {
		return "", nil
	}

	serviceNames, err := extractComposeServiceNames()
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
	// The override file lives in the OS temp dir (/tmp on Linux). Fence
	// tmpfs-overmounts /tmp inside its Linux sandbox, so a naive `docker
	// compose -f /tmp/...` inside the sandbox can't see this file. Callers
	// that pass this path into a sandboxed command must register it via
	// fence.Manager.ExposeHostPath before launching the sandbox — see
	// StartService in service.go, which does this automatically.
	tempFile, err := os.CreateTemp("", fmt.Sprintf("tusk-replay-env-override-%s-*.yml", safeGroup))
	if err != nil {
		return "", fmt.Errorf("failed to create temporary replay compose override file: %w", err)
	}
	overridePath := tempFile.Name()
	if closeErr := tempFile.Close(); closeErr != nil {
		removeOverrideFile(overridePath)
		return "", fmt.Errorf("failed to close temporary replay compose override file: %w", closeErr)
	}

	override := replayComposeOverride{
		Services: make(map[string]replayComposeService, len(serviceNames)),
	}

	for _, serviceName := range serviceNames {
		environment := make(map[string]string, len(envVars))
		for key := range envVars {
			environment[key] = fmt.Sprintf("${%s}", key)
		}
		override.Services[serviceName] = replayComposeService{Environment: environment}
	}

	content, err := yaml.Marshal(override)
	if err != nil {
		removeOverrideFile(overridePath)
		return "", fmt.Errorf("failed to marshal replay compose override: %w", err)
	}

	if err := os.WriteFile(overridePath, content, 0o600); err != nil {
		removeOverrideFile(overridePath)
		return "", fmt.Errorf("failed to write replay compose override file: %w", err)
	}

	return overridePath, nil
}

// resolveComposeServiceSourceFile resolves docker-compose.tusk-override.yml,
// preferring the tusk root path and falling back to a recursive search under
// the tusk root when the root-level file is absent.
func resolveComposeServiceSourceFile() (string, error) {
	tuskRoot := utils.GetTuskRoot()
	rootComposeFile := filepath.Join(tuskRoot, replayComposeServiceSourceFile)

	if _, err := os.Stat(rootComposeFile); err == nil {
		return rootComposeFile, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("failed to stat compose service source file %s: %w", rootComposeFile, err)
	}

	matches := make([]string, 0, 1)
	walkErr := filepath.WalkDir(tuskRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == replayComposeServiceSourceFile {
			matches = append(matches, path)
		}
		return nil
	})
	if walkErr != nil {
		return "", fmt.Errorf("failed to recursively search for compose service source file under %s: %w", tuskRoot, walkErr)
	}

	if len(matches) == 0 {
		return "", nil
	}
	sort.Strings(matches)
	if len(matches) > 1 {
		return "", fmt.Errorf("multiple %s files found under %s; unable to choose one: %s",
			replayComposeServiceSourceFile,
			tuskRoot,
			strings.Join(matches, ", "))
	}
	return matches[0], nil
}

// extractComposeServiceNames reads service names from the resolved
// docker-compose.tusk-override.yml location.
func extractComposeServiceNames() ([]string, error) {
	composeFile, pathErr := resolveComposeServiceSourceFile()
	if pathErr != nil {
		return nil, pathErr
	}
	if composeFile == "" {
		log.Debug("Replay compose service source file not found; skipping replay env override",
			"filename", replayComposeServiceSourceFile,
			"search_root", utils.GetTuskRoot())
		return nil, nil
	}

	data, err := os.ReadFile(composeFile) // #nosec G304
	if err != nil {
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

// injectComposeOverrideFile appends the replay override file as an additional
// "-f/--file" flag immediately after an existing
// "docker-compose.tusk-override.yml" compose file flag.
//
// The returned injected flag indicates whether override args were actually
// appended. A nil error with injected=false means no matching -f/--file value
// for docker-compose.tusk-override.yml was found.
func injectComposeOverrideFile(command, overridePath string) (string, bool, error) {
	if strings.TrimSpace(command) == "" {
		return command, false, nil
	}

	matches := composeFileFlagPattern.FindAllStringSubmatchIndex(command, -1)
	if len(matches) == 0 {
		return command, false, nil
	}

	var out bytes.Buffer
	last := 0
	injected := false

	for _, m := range matches {
		if len(m) < 14 {
			continue
		}

		matchStart, matchEnd := m[0], m[1]
		flagStart, flagEnd := m[2], m[3]
		delimStart, delimEnd := m[4], m[5]
		tokenStart, tokenEnd := m[6], m[7]
		singleInnerStart, singleInnerEnd := m[8], m[9]
		doubleInnerStart, doubleInnerEnd := m[10], m[11]
		bareStart, bareEnd := m[12], m[13]

		if tokenStart < 0 || tokenEnd < 0 {
			continue
		}

		quoteChar := byte(0)
		value := ""
		switch {
		case singleInnerStart >= 0 && singleInnerEnd >= 0:
			quoteChar = '\''
			value = command[singleInnerStart:singleInnerEnd]
		case doubleInnerStart >= 0 && doubleInnerEnd >= 0:
			quoteChar = '"'
			value = command[doubleInnerStart:doubleInnerEnd]
		case bareStart >= 0 && bareEnd >= 0:
			value = command[bareStart:bareEnd]
		default:
			continue
		}

		if !isReplayComposeSourcePath(value) {
			continue
		}

		out.WriteString(command[last:matchStart])
		out.WriteString(command[matchStart:matchEnd])
		out.WriteString(" ")
		flag := command[flagStart:flagEnd]
		delim := command[delimStart:delimEnd]
		out.WriteString(flag)
		out.WriteString(delim)
		out.WriteString(quoteForShell(overridePath, quoteChar))
		last = matchEnd
		injected = true
	}

	if !injected {
		return command, false, nil
	}
	out.WriteString(command[last:])
	return out.String(), true, nil
}

func isReplayComposeSourcePath(path string) bool {
	if path == replayComposeServiceSourceFile {
		return true
	}
	return strings.HasSuffix(path, "/"+replayComposeServiceSourceFile)
}

func quoteForShell(value string, quoteChar byte) string {
	switch quoteChar {
	case '\'':
		return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
	case '"':
		escaped := strings.ReplaceAll(value, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		escaped = strings.ReplaceAll(escaped, `$`, `\$`)
		escaped = strings.ReplaceAll(escaped, "`", "\\`")
		return `"` + escaped + `"`
	default:
		quoted, err := syntax.Quote(value, syntax.LangBash)
		if err != nil {
			return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
		}
		return quoted
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
