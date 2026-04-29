package runner

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Use-Tusk/tusk-cli/internal/utils"
	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
)

const maxDiffBodySize = 100 * 1024 // 100KB

// AgentWriter handles writing deviation files for agent consumption.
type AgentWriter struct {
	outputDir  string
	baseBranch string
	results    []agentTestEntry
	mu         sync.Mutex
}

type agentTestEntry struct {
	testID      string
	method      string
	path        string
	passed      bool
	failureType string // "RESPONSE_MISMATCH", "MOCK_NOT_FOUND", "NO_RESPONSE", ""
	fileName    string // "deviation-{testID}.md" or ""
}

// NewAgentWriter creates a new AgentWriter that writes to the given directory.
// The directory must already exist (created by the caller).
func NewAgentWriter(outputDir string) (*AgentWriter, error) {
	return &AgentWriter{
		outputDir: outputDir,
	}, nil
}

// OutputDir returns the full output directory path.
func (w *AgentWriter) OutputDir() string {
	return w.outputDir
}

// SetBaseBranch sets the base branch name to include in the index file.
func (w *AgentWriter) SetBaseBranch(branch string) {
	w.baseBranch = branch
}

// WriteDeviation writes a single deviation file for a failed test.
func (w *AgentWriter) WriteDeviation(test Test, result TestResult, server *Server) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	fileName := fmt.Sprintf("deviation-%s.md", sanitizeFileName(result.TestID))
	failureType := determineFailureType(result, server)

	fm := buildFrontmatter(test, result, server, failureType)
	body := buildDeviationBody(test, result, server)

	filePath := filepath.Join(w.outputDir, fileName)
	content := RedactSecrets(fm + body)
	if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
		return err
	}

	w.results = append(w.results, agentTestEntry{
		testID:      result.TestID,
		method:      test.Method,
		path:        test.Path,
		passed:      false,
		failureType: failureType,
		fileName:    fileName,
	})

	return nil
}

// RecordPassedTest records a passing test for the index.
func (w *AgentWriter) RecordPassedTest(test Test) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.results = append(w.results, agentTestEntry{
		testID: test.TraceID,
		method: test.Method,
		path:   test.Path,
		passed: true,
	})
}

// WriteIndex writes the index.md summary file.
func (w *AgentWriter) WriteIndex(totalTests int, passedTests int) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	failedTests := totalTests - passedTests

	var sb strings.Builder
	sb.WriteString("# Tusk Drift Agent Deviation Report\n\n")
	fmt.Fprintf(&sb, "Run: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&sb, "Tests: %d total, %d passed, %d failed\n", totalTests, passedTests, failedTests)
	if w.baseBranch != "" {
		fmt.Fprintf(&sb, "Base Branch: %s\n", w.baseBranch)
	}

	// Deviations table
	var deviations []agentTestEntry
	var passed []agentTestEntry
	for _, e := range w.results {
		if e.passed {
			passed = append(passed, e)
		} else {
			deviations = append(deviations, e)
		}
	}

	if len(deviations) > 0 {
		sb.WriteString("\n## Deviations\n\n")
		sb.WriteString("| # | Test ID | Endpoint | Failure Reason | File |\n")
		sb.WriteString("|---|---------|----------|----------------|------|\n")
		for i, d := range deviations {
			fmt.Fprintf(&sb, "| %d | %s | %s %s | %s | %s |\n",
				i+1, d.testID, d.method, d.path, d.failureType, d.fileName)
		}
	}

	if len(passed) > 0 {
		sb.WriteString("\n## Passing Tests\n")
		for _, p := range passed {
			fmt.Fprintf(&sb, "- %s: %s %s\n", p.testID, p.method, p.path)
		}
	}

	filePath := filepath.Join(w.outputDir, "index.md")
	indexContent := RedactSecrets(sb.String())
	return os.WriteFile(filePath, []byte(indexContent), 0o600)
}

func determineFailureType(result TestResult, server *Server) string {
	if result.CrashedServer {
		return "NO_RESPONSE"
	}
	if server != nil && server.HasMockNotFoundEvents(result.TestID) {
		return "MOCK_NOT_FOUND"
	}
	if result.Error != "" {
		return "NO_RESPONSE"
	}
	return "RESPONSE_MISMATCH"
}

func buildFrontmatter(test Test, result TestResult, server *Server, failureType string) string {
	hasMockNotFound := false
	if server != nil {
		hasMockNotFound = server.HasMockNotFoundEvents(result.TestID)
	}

	statusExpected := test.Response.Status
	statusActual := statusExpected
	if failureType == "NO_RESPONSE" {
		statusActual = 0
	} else {
		for _, d := range result.Deviations {
			if d.Field == "response.status" {
				statusActual = anyToInt(d.Actual, statusExpected)
				break
			}
		}
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "deviation_id: %s\n", result.TestID)
	fmt.Fprintf(&sb, "endpoint: %s %s\n", test.Method, test.Path)
	fmt.Fprintf(&sb, "method: %s\n", test.Method)
	fmt.Fprintf(&sb, "path: %s\n", test.Path)
	fmt.Fprintf(&sb, "failure_type: %s\n", failureType)
	fmt.Fprintf(&sb, "status_expected: %d\n", statusExpected)
	fmt.Fprintf(&sb, "status_actual: %d\n", statusActual)
	fmt.Fprintf(&sb, "has_mock_not_found: %t\n", hasMockNotFound)
	fmt.Fprintf(&sb, "duration_ms: %d\n", result.Duration)
	sb.WriteString("---\n\n")

	return sb.String()
}

func buildDeviationBody(test Test, result TestResult, server *Server) string {
	var sb strings.Builder

	// Retry note
	if result.RetriedAfterCrash {
		sb.WriteString("> Note: This test was retried after a server crash in a previous batch.\n\n")
	}

	// Request section
	sb.WriteString("## Request\n")
	fmt.Fprintf(&sb, "%s %s\n", test.Request.Method, test.Request.Path)
	sb.WriteString("Body:\n")
	sb.WriteString(formatBodyForAgent(test.Request.Body))
	sb.WriteString("\n\n")

	switch {
	case result.CrashedServer:
		sb.WriteString("## Error\n")
		sb.WriteString("Server crashed during test execution.\n")
		if result.Error != "" {
			sb.WriteString(result.Error)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	case result.Error != "" && len(result.Deviations) == 0:
		sb.WriteString("## Error\n")
		sb.WriteString(result.Error)
		sb.WriteString("\n\n")
	default:
		// Response Diff section
		sb.WriteString("## Response Diff\n")

		statusExpected := test.Response.Status
		statusActual := statusExpected
		for _, d := range result.Deviations {
			if d.Field == "response.status" {
				statusActual = anyToInt(d.Actual, statusExpected)
				break
			}
		}

		if statusExpected != statusActual {
			fmt.Fprintf(&sb, "Status: %d -> %d (CHANGED)\n", statusExpected, statusActual)
		} else {
			fmt.Fprintf(&sb, "Status: %d (OK)\n", statusExpected)
		}

		// Body diff
		for _, d := range result.Deviations {
			if d.Field == "response.body" {
				sb.WriteString("\nBody:\n")
				if shouldTruncateDiff(d.Expected, d.Actual) {
					sb.WriteString(formatTruncatedDiff(d.Expected, d.Actual))
				} else {
					diff := utils.FormatJSONDiffPlain(d.Expected, d.Actual)
					if diff != "" {
						sb.WriteString("```diff\n")
						sb.WriteString(diff)
						sb.WriteString("```\n")
					}
				}
				break
			}
		}
		sb.WriteString("\n")
	}

	// Outbound Call Context (only if server is available)
	if server != nil {
		matchEvents := server.GetMatchEvents(result.TestID)
		mockNotFoundEvents := server.GetMockNotFoundEvents(result.TestID)

		if len(matchEvents) > 0 || len(mockNotFoundEvents) > 0 {
			sb.WriteString("## Outbound Call Context\n")
			sb.WriteString("| # | Operation | Match Level | Match Scope | Notes |\n")
			sb.WriteString("|---|-----------|-------------|-------------|-------|\n")

			idx := 1
			for _, ev := range matchEvents {
				opName := matchEventOperationName(ev)
				quality, scope := matchLevelToStrings(ev.MatchLevel)
				fmt.Fprintf(&sb, "| %d | %s | %s | %s | |\n", idx, opName, quality, scope)
				idx++
			}
			for _, ev := range mockNotFoundEvents {
				opName := mockNotFoundOperationName(ev)
				fmt.Fprintf(&sb, "| %d | %s | MOCK NOT FOUND | — | No matching recording |\n", idx, opName)
				idx++
			}
			sb.WriteString("\n")
		}

		// Mock Not Found Events detail section
		if len(mockNotFoundEvents) > 0 {
			sb.WriteString("## Mock Not Found Events\n")
			for _, ev := range mockNotFoundEvents {
				opName := mockNotFoundOperationName(ev)
				fmt.Fprintf(&sb, "- %s\n", opName)
				if ev.SpanName != "" {
					fmt.Fprintf(&sb, "  Request: %s\n", ev.SpanName)
				}
				if ev.StackTrace != "" {
					fmt.Fprintf(&sb, "  Stack: %s\n", ev.StackTrace)
				}
				sb.WriteString("  This outbound call had no matching recording.\n")
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// sanitizeFileName replaces path-unsafe characters in a test ID for use as a filename.
func sanitizeFileName(testID string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_")
	return replacer.Replace(testID)
}

// anyToInt converts an any value to int, returning the fallback if conversion fails.
func anyToInt(v any, fallback int) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case json.Number:
		if i, err := val.Int64(); err == nil {
			return int(i)
		}
	}
	return fallback
}

// formatBodyForAgent pretty-prints a body for agent output, or returns "(empty)" if nil.
func formatBodyForAgent(body any) string {
	if body == nil {
		return "(empty)"
	}

	switch v := body.(type) {
	case string:
		if v == "" {
			return "(empty)"
		}
		// Try to parse as JSON first
		var parsed any
		if err := json.Unmarshal([]byte(v), &parsed); err == nil {
			b, err := json.MarshalIndent(parsed, "", "  ")
			if err == nil {
				return string(b)
			}
		}
		// Try base64 decoding
		if decoded, err := base64.StdEncoding.DecodeString(v); err == nil {
			// Try to parse decoded bytes as JSON
			var parsed any
			if err := json.Unmarshal(decoded, &parsed); err == nil {
				b, err := json.MarshalIndent(parsed, "", "  ")
				if err == nil {
					return string(b)
				}
			}
			if utf8.Valid(decoded) {
				return string(decoded)
			}
		}
		return v
	default:
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

func shouldTruncateDiff(expected, actual any) bool {
	e, _ := json.Marshal(expected)
	a, _ := json.Marshal(actual)
	return len(e) > maxDiffBodySize || len(a) > maxDiffBodySize
}

func formatTruncatedDiff(expected, actual any) string {
	eBytes, _ := json.MarshalIndent(expected, "", "  ")
	aBytes, _ := json.MarshalIndent(actual, "", "  ")

	eStr := string(eBytes)
	aStr := string(aBytes)

	var sb strings.Builder
	fmt.Fprintf(&sb, "Body diff too large to display (%dKB expected, %dKB actual).\n\n", len(eBytes)/1024, len(aBytes)/1024)
	sb.WriteString("### Expected (truncated)\n")
	if len(eStr) > 1000 {
		sb.WriteString(eStr[:1000])
		sb.WriteString("...\n")
	} else {
		sb.WriteString(eStr)
		sb.WriteString("\n")
	}
	sb.WriteString("\n### Actual (truncated)\n")
	if len(aStr) > 1000 {
		sb.WriteString(aStr[:1000])
		sb.WriteString("...\n")
	} else {
		sb.WriteString(aStr)
		sb.WriteString("\n")
	}
	return sb.String()
}

func matchLevelToStrings(ml *core.MatchLevel) (string, string) {
	if ml == nil {
		return "UNKNOWN", "UNKNOWN"
	}
	quality := ml.MatchType.String()
	scope := ml.MatchScope.String()

	// Clean up proto enum prefixes for readability
	quality = strings.TrimPrefix(quality, "MATCH_TYPE_")
	scope = strings.TrimPrefix(scope, "MATCH_SCOPE_")

	return quality, scope
}

func matchEventOperationName(ev MatchEvent) string {
	if ev.ReplaySpan != nil {
		name := ev.ReplaySpan.Name
		pkg := ev.ReplaySpan.PackageName
		if pkg != "" && name != "" {
			return fmt.Sprintf("%s: %s", pkg, name)
		}
		if name != "" {
			return name
		}
	}
	return ev.SpanID
}

func mockNotFoundOperationName(ev MockNotFoundEvent) string {
	if ev.SpanName != "" {
		if ev.PackageName != "" {
			return fmt.Sprintf("%s: %s", ev.PackageName, ev.SpanName)
		}
		return ev.SpanName
	}
	if ev.PackageName != "" && ev.Operation != "" {
		return fmt.Sprintf("%s: %s", ev.PackageName, ev.Operation)
	}
	if ev.PackageName != "" {
		return ev.PackageName
	}
	return "unknown"
}
