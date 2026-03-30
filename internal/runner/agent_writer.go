package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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

// NewAgentWriter creates a new AgentWriter that writes to a timestamped subdirectory.
func NewAgentWriter(baseDir string) (*AgentWriter, error) {
	if baseDir == "" {
		baseDir = utils.ResolveTuskPath(".tusk/logs")
	} else {
		baseDir = utils.ResolveTuskPath(baseDir)
	}

	if err := os.MkdirAll(baseDir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	timestamp := time.Now().Format("20060102-150405")
	dir := filepath.Join(baseDir, fmt.Sprintf("agent-run-%s", timestamp))

	// Atomic create-or-fail: os.Mkdir returns EEXIST if another process won
	// the race, avoiding the TOCTOU window that os.Stat+os.MkdirAll has.
	if err := os.Mkdir(dir, 0o750); err != nil {
		if !os.IsExist(err) {
			return nil, fmt.Errorf("failed to create agent output directory: %w", err)
		}
		for i := 2; ; i++ {
			candidate := fmt.Sprintf("%s-%d", dir, i)
			if err := os.Mkdir(candidate, 0o750); err == nil {
				dir = candidate
				break
			} else if !os.IsExist(err) {
				return nil, fmt.Errorf("failed to create agent output directory %s: %w", candidate, err)
			}
		}
	}

	return &AgentWriter{
		outputDir: dir,
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
	if err := os.WriteFile(filePath, []byte(fm+body), 0o600); err != nil {
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
	sb.WriteString(fmt.Sprintf("Run: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("Tests: %d total, %d passed, %d failed\n", totalTests, passedTests, failedTests))
	if w.baseBranch != "" {
		sb.WriteString(fmt.Sprintf("Base Branch: %s\n", w.baseBranch))
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
			sb.WriteString(fmt.Sprintf("| %d | %s | %s %s | %s | %s |\n",
				i+1, d.testID, d.method, d.path, d.failureType, d.fileName))
		}
	}

	if len(passed) > 0 {
		sb.WriteString("\n## Passing Tests\n")
		for _, p := range passed {
			sb.WriteString(fmt.Sprintf("- %s: %s %s\n", p.testID, p.method, p.path))
		}
	}

	filePath := filepath.Join(w.outputDir, "index.md")
	return os.WriteFile(filePath, []byte(sb.String()), 0o600)
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
	for _, d := range result.Deviations {
		if d.Field == "response.status" {
			statusActual = anyToInt(d.Actual, statusExpected)
			break
		}
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("deviation_id: %s\n", result.TestID))
	sb.WriteString(fmt.Sprintf("endpoint: %s %s\n", test.Method, test.Path))
	sb.WriteString(fmt.Sprintf("method: %s\n", test.Method))
	sb.WriteString(fmt.Sprintf("path: %s\n", test.Path))
	sb.WriteString(fmt.Sprintf("failure_type: %s\n", failureType))
	sb.WriteString(fmt.Sprintf("status_expected: %d\n", statusExpected))
	sb.WriteString(fmt.Sprintf("status_actual: %d\n", statusActual))
	sb.WriteString(fmt.Sprintf("has_mock_not_found: %t\n", hasMockNotFound))
	sb.WriteString(fmt.Sprintf("duration_ms: %d\n", result.Duration))
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
	sb.WriteString(fmt.Sprintf("%s %s\n", test.Request.Method, test.Request.Path))
	if len(test.Request.Headers) > 0 {
		sb.WriteString("Headers:\n")
		for k, v := range test.Request.Headers {
			if strings.EqualFold(k, "authorization") {
				v = "***"
			}
			sb.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
		}
	}
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
			sb.WriteString(fmt.Sprintf("Status: %d -> %d (CHANGED)\n", statusExpected, statusActual))
		} else {
			sb.WriteString(fmt.Sprintf("Status: %d (OK)\n", statusExpected))
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
				sb.WriteString(fmt.Sprintf("| %d | %s | %s | %s | |\n", idx, opName, quality, scope))
				idx++
			}
			for _, ev := range mockNotFoundEvents {
				opName := mockNotFoundOperationName(ev)
				sb.WriteString(fmt.Sprintf("| %d | %s | MOCK NOT FOUND | — | No matching recording |\n", idx, opName))
				idx++
			}
			sb.WriteString("\n")
		}

		// Mock Not Found Events detail section
		if len(mockNotFoundEvents) > 0 {
			sb.WriteString("## Mock Not Found Events\n")
			for _, ev := range mockNotFoundEvents {
				opName := mockNotFoundOperationName(ev)
				sb.WriteString(fmt.Sprintf("- %s\n", opName))
				if ev.SpanName != "" {
					sb.WriteString(fmt.Sprintf("  Request: %s\n", ev.SpanName))
				}
				if ev.StackTrace != "" {
					sb.WriteString(fmt.Sprintf("  Stack: %s\n", ev.StackTrace))
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
		// Try to parse and pretty-print JSON
		var parsed any
		if err := json.Unmarshal([]byte(v), &parsed); err == nil {
			b, err := json.MarshalIndent(parsed, "", "  ")
			if err == nil {
				return string(b)
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
	sb.WriteString(fmt.Sprintf("Body diff too large to display (%dKB expected, %dKB actual).\n\n", len(eBytes)/1024, len(aBytes)/1024))
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
