package runner

import (
	"sort"
	"strings"
)

var replayProcessEnvVarKeysToSkip = map[string]struct{}{
	"HOME":    {},
	"LOGNAME": {},
	"OLDPWD":  {},
	"PATH":    {},
	"PWD":     {},
	"SHELL":   {},
	"SHLVL":   {},
	"TEMP":    {},
	"TERM":    {},
	"TMP":     {},
	"TMPDIR":  {},
	"USER":    {},
}

var replayProcessEnvVarPrefixesToSkip = []string{
	"ASDF_",
	"FNM_",
	"NVM_",
	"PYENV_",
	"RBENV_",
	"SDKMAN_",
	"VOLTA_",
	"UV_",
}

func shouldSkipReplayEnvVarForProcess(key string) bool {
	normalizedKey := strings.ToUpper(key)
	if _, ok := replayProcessEnvVarKeysToSkip[normalizedKey]; ok {
		return true
	}
	for _, prefix := range replayProcessEnvVarPrefixesToSkip {
		if strings.HasPrefix(normalizedKey, prefix) {
			return true
		}
	}
	return false
}

func filterReplayEnvVarsForProcess(envVars map[string]string) (map[string]string, []string) {
	filtered := make(map[string]string, len(envVars))
	skipped := make([]string, 0)

	for key, value := range envVars {
		if shouldSkipReplayEnvVarForProcess(key) {
			skipped = append(skipped, key)
			continue
		}
		filtered[key] = value
	}

	sort.Strings(skipped)
	return filtered, skipped
}

// filterReplayEnvVarsForCompose removes host-specific and internal runtime keys
// that should be controlled by replay startup logic (not by recorded env snapshots).
func filterReplayEnvVarsForCompose(envVars map[string]string) (map[string]string, []string) {
	filtered := make(map[string]string, len(envVars))
	skipped := make([]string, 0)

	for key, value := range envVars {
		if strings.HasPrefix(strings.ToUpper(key), "TUSK_") || shouldSkipReplayEnvVarForProcess(key) {
			skipped = append(skipped, key)
			continue
		}
		filtered[key] = value
	}

	sort.Strings(skipped)
	return filtered, skipped
}
