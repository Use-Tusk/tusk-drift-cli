package onboard

import "strings"

// formatYAMLWithBlankLines inserts a blank line before each top-level key (after the first)
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
	if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "...") || strings.HasPrefix(line, "#") {
		return false
	}
	if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "-") {
		return false
	}
	return strings.Contains(line, ":")
}
