package runner

import (
	"fmt"
	"regexp"
	"strings"
)

func FilterTests(tests []Test, pattern string) ([]Test, error) {
	// Fielded filter syntax: key=regex[,key=regex...]
	// AND semantics across keys. Values are Go regexes; quotes supported.
	if !strings.Contains(pattern, "=") {
		return nil, fmt.Errorf("non-fielded filters are no longer supported; use key=regex (e.g., type=GRAPHQL,op=^GetUser$)")
	}

	matchers, err := parseFieldedFilter(pattern)
	if err != nil {
		return nil, err
	}
	var out []Test
	for _, t := range tests {
		ok := true
		for _, m := range matchers {
			val := getFieldValueForFilter(t, m.field)
			if !m.re.MatchString(val) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, t)
		}
	}
	return out, nil
}

type fieldMatcher struct {
	field string
	re    *regexp.Regexp
}

func parseFieldedFilter(q string) ([]fieldMatcher, error) {
	tokens := splitCommaAware(q)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("invalid filter: %q", q)
	}

	var out []fieldMatcher
	for _, tok := range tokens {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		idx := strings.Index(tok, "=")
		if idx <= 0 {
			return nil, fmt.Errorf("invalid filter token: %q (expected key=value)", tok)
		}
		key := strings.TrimSpace(strings.ToLower(tok[:idx]))
		val := strings.TrimSpace(tok[idx+1:])
		// Trim surrounding quotes if present
		if len(val) >= 2 && ((val[0] == '\'' && val[len(val)-1] == '\'') || (val[0] == '"' && val[len(val)-1] == '"')) {
			val = val[1 : len(val)-1]
		}
		re, err := regexp.Compile(val)
		if err != nil {
			return nil, fmt.Errorf("invalid regex for %s: %w", key, err)
		}
		field := normalizeFilterFieldKey(key)
		if field == "" {
			return nil, fmt.Errorf("unknown filter field: %s", key)
		}
		out = append(out, fieldMatcher{field: field, re: re})
	}
	return out, nil
}

func splitCommaAware(s string) []string {
	var toks []string
	var cur strings.Builder
	var inQuote byte
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inQuote == 0 && (ch == '\'' || ch == '"') {
			inQuote = ch
			cur.WriteByte(ch)
			continue
		}
		if inQuote != 0 {
			cur.WriteByte(ch)
			if ch == inQuote {
				inQuote = 0
			}
			continue
		}
		if ch == ',' {
			toks = append(toks, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(ch)
	}
	if cur.Len() > 0 {
		toks = append(toks, cur.String())
	}
	return toks
}

func normalizeFilterFieldKey(k string) string {
	switch k {
	case "path", "p":
		return "path"
	case "name", "display", "display_name", "n":
		return "name"
	case "op", "operation", "operation_name", "graphql_op": // GraphQL-only
		return "op"
	case "type", "t":
		return "type"
	case "method", "m":
		return "method"
	case "status", "s":
		return "status"
	case "id", "trace", "trace_id":
		return "id"
	case "file", "filename", "f":
		return "file"
	default:
		return ""
	}
}

func getFieldValueForFilter(t Test, field string) string {
	switch field {
	case "path":
		return t.Path
	case "name":
		if t.DisplayName != "" {
			return t.DisplayName
		}
		return ""
	case "op":
		if t.DisplayType == "GRAPHQL" {
			return extractGraphQLOperationName(t.DisplayName)
		}
		return ""
	case "type":
		if t.DisplayType != "" {
			return t.DisplayType
		}
		return t.Type
	case "method":
		return t.Method
	case "status":
		return t.Status
	case "id":
		return t.TraceID
	case "file":
		return t.FileName
	default:
		return ""
	}
}

func extractGraphQLOperationName(displayName string) string {
	// e.g. "query GetUser", "mutation UpdateUser", "subscription OnEvent"
	parts := strings.Fields(displayName)
	if len(parts) >= 2 {
		switch parts[0] {
		case "query", "mutation", "subscription":
			return parts[1]
		}
	}
	return displayName
}
