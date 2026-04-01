package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Use-Tusk/tusk-cli/internal/log"
	"github.com/Use-Tusk/tusk-cli/internal/utils"
)

const coverageSnapshotTimeout = 5 * time.Second

// TakeCoverageSnapshot calls the SDK's coverage snapshot endpoint.
// Returns per-file line counts for this test only (counters auto-reset).
func (e *Executor) TakeCoverageSnapshot() (map[string]map[string]int, error) {
	return e.callCoverageEndpoint(false)
}

// TakeCoverageBaseline calls the SDK's coverage snapshot endpoint with ?baseline=true.
// Returns ALL coverable lines (including uncovered at count=0) for the aggregate denominator.
// Also resets counters so the first real test gets clean data.
// Retries with backoff since the coverage server may not be ready immediately after service start.
func (e *Executor) TakeCoverageBaseline() (map[string]map[string]int, error) {
	var lastErr error
	for attempt := 0; attempt < 15; attempt++ {
		result, err := e.callCoverageEndpoint(true)
		if err == nil {
			return result, nil
		}
		lastErr = err
		time.Sleep(200 * time.Millisecond)
	}
	return nil, fmt.Errorf("coverage baseline failed after retries: %w", lastErr)
}

func (e *Executor) callCoverageEndpoint(baseline bool) (map[string]map[string]int, error) {
	if !e.coverageEnabled || e.coveragePort == 0 {
		return nil, nil
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/snapshot", e.coveragePort)
	if baseline {
		url += "?baseline=true"
	}
	httpClient := &http.Client{Timeout: coverageSnapshotTimeout}

	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("coverage snapshot failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read coverage response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("coverage snapshot status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		OK       bool                      `json:"ok"`
		Coverage map[string]map[string]int `json:"coverage"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse coverage response: %w", err)
	}

	if !result.OK {
		return nil, fmt.Errorf("coverage snapshot returned ok=false")
	}

	// Normalize absolute paths to repo-relative paths
	return normalizeFilePaths(result.Coverage), nil
}

// CoverageTestRecord holds per-test coverage data.
type CoverageTestRecord struct {
	TestID     string
	TestName   string
	LineCounts map[string]map[string]int // filePath -> lineNumber -> hitCount (per-test, not cumulative)
}

// CoverageFileDiff represents per-test coverage for a single file.
type CoverageFileDiff struct {
	CoveredLines   []int `json:"covered_lines"`
	CoverableLines int   `json:"coverable_lines"`
	CoveredCount   int   `json:"covered_count"`
}

// LinecountsToCoverageDetail converts raw line counts to CoverageFileDiff format.
func LinecountsToCoverageDetail(lineCounts map[string]map[string]int) map[string]CoverageFileDiff {
	result := make(map[string]CoverageFileDiff)
	for filePath, lines := range lineCounts {
		var covered []int
		for lineStr, count := range lines {
			if count > 0 {
				line, err := strconv.Atoi(lineStr)
				if err != nil || line <= 0 {
					continue
				}
				covered = append(covered, line)
			}
		}
		if len(covered) > 0 {
			sort.Ints(covered)
			covered = dedup(covered)
			result[filePath] = CoverageFileDiff{
				CoveredLines:   covered,
				CoverableLines: len(lines),
				CoveredCount:   len(covered),
			}
		}
	}
	return result
}

// ProcessCoverage writes per-test coverage files and prints the summary.
func (e *Executor) ProcessCoverage(records []CoverageTestRecord) error {
	if !e.coverageEnabled || len(records) == 0 {
		return nil
	}

	log.Stderrln("\n➤ Processing coverage data...")

	// Write per-test coverage files
	for _, record := range records {
		detail := e.GetTestCoverageDetail(record.TestID)
		if detail == nil {
			continue
		}
		testDir := filepath.Join(e.coverageOutputDir, sanitizeFileName(record.TestID))
		if err := os.MkdirAll(testDir, 0o750); err != nil {
			return err
		}
		data, _ := json.MarshalIndent(detail, "", "  ")
		if err := os.WriteFile(filepath.Join(testDir, "coverage.json"), data, 0o644); err != nil {
			return err
		}
	}

	// Compute aggregate: start with baseline (all coverable lines including count=0),
	// then merge in per-test coverage. This gives accurate denominator.
	aggregate := mergeWithBaseline(e.coverageBaseline, records)

	// Print and write summary
	return e.printCoverageSummary(records, aggregate)
}

// mergeWithBaseline creates aggregate coverage starting from the baseline
// (which has ALL coverable lines including count=0), then merging in per-test data.
// If no baseline is available, falls back to merging per-test data only.
func mergeWithBaseline(baseline map[string]map[string]int, records []CoverageTestRecord) map[string]map[string]int {
	merged := make(map[string]map[string]int)

	// Start with baseline (all coverable lines, count=0 for uncovered)
	if baseline != nil {
		for filePath, lines := range baseline {
			merged[filePath] = make(map[string]int)
			for line, count := range lines {
				merged[filePath][line] = count
			}
		}
	}

	// Merge in per-test coverage (add counts)
	for _, record := range records {
		for filePath, lines := range record.LineCounts {
			if merged[filePath] == nil {
				merged[filePath] = make(map[string]int)
			}
			for line, count := range lines {
				merged[filePath][line] += count
			}
		}
	}

	return merged
}

// --- Summary output ---

type CoverageSummary struct {
	Timestamp string                         `json:"timestamp"`
	Aggregate CoverageAggregate              `json:"aggregate"`
	PerFile   map[string]CoverageFileSummary `json:"per_file"`
	PerTest   []CoverageTestSummary          `json:"per_test"`
}

type CoverageAggregate struct {
	TotalCoverableLines int     `json:"total_coverable_lines"`
	TotalCoveredLines   int     `json:"total_covered_lines"`
	CoveragePct         float64 `json:"coverage_pct"`
	TotalFiles          int     `json:"total_files"`
	CoveredFiles        int     `json:"covered_files"`
}

type CoverageFileSummary struct {
	CoveredLines   int     `json:"covered_lines"`
	CoverableLines int     `json:"coverable_lines"`
	CoveragePct    float64 `json:"coverage_pct"`
}

type CoverageTestSummary struct {
	TestID       string `json:"test_id"`
	TestName     string `json:"test_name"`
	CoveredLines int    `json:"covered_lines"`
	FilesTouched int    `json:"files_touched"`
}

func (e *Executor) printCoverageSummary(records []CoverageTestRecord, aggregate map[string]map[string]int) error {
	summary := CoverageSummary{
		Timestamp: time.Now().Format(time.RFC3339),
		PerFile:   make(map[string]CoverageFileSummary),
	}

	totalCoverable := 0
	totalCovered := 0
	coveredFiles := 0

	for filePath, lines := range aggregate {
		coverable := len(lines)
		covered := 0
		for _, count := range lines {
			if count > 0 {
				covered++
			}
		}
		totalCoverable += coverable
		totalCovered += covered
		if covered > 0 {
			coveredFiles++
		}
		pct := 0.0
		if coverable > 0 {
			pct = float64(covered) / float64(coverable) * 100
		}
		summary.PerFile[filePath] = CoverageFileSummary{
			CoveredLines: covered, CoverableLines: coverable, CoveragePct: pct,
		}
	}

	aggPct := 0.0
	if totalCoverable > 0 {
		aggPct = float64(totalCovered) / float64(totalCoverable) * 100
	}

	summary.Aggregate = CoverageAggregate{
		TotalCoverableLines: totalCoverable,
		TotalCoveredLines:   totalCovered,
		CoveragePct:         aggPct,
		TotalFiles:          len(aggregate),
		CoveredFiles:        coveredFiles,
	}

	// Per-test summaries from stored detail
	for _, record := range records {
		ts := CoverageTestSummary{TestID: record.TestID, TestName: record.TestName}
		if detail := e.GetTestCoverageDetail(record.TestID); detail != nil {
			for _, fd := range detail {
				ts.CoveredLines += fd.CoveredCount
			}
			ts.FilesTouched = len(detail)
		}
		summary.PerTest = append(summary.PerTest, ts)
	}

	// Write summary.json
	summaryPath := filepath.Join(e.coverageOutputDir, "summary.json")
	summaryData, _ := json.MarshalIndent(summary, "", "  ")
	if err := os.WriteFile(summaryPath, summaryData, 0o644); err != nil {
		return err
	}

	// Console output
	log.Stderrln(fmt.Sprintf("\n📊 Coverage: %.1f%% (%d/%d coverable lines across %d files)",
		aggPct, totalCovered, totalCoverable, len(aggregate)))

	type fileStat struct {
		path    string
		pct     float64
		cov, tot int
	}
	var stats []fileStat
	for fp, fs := range summary.PerFile {
		if fs.CoverableLines > 0 {
			stats = append(stats, fileStat{fp, fs.CoveragePct, fs.CoveredLines, fs.CoverableLines})
		}
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].pct > stats[j].pct })

	log.Stderrln("\n  Per-file:")
	for _, s := range stats {
		shortPath := s.path
		if cwd, err := os.Getwd(); err == nil {
			if rel, err := filepath.Rel(cwd, s.path); err == nil {
				shortPath = rel
			}
		}
		log.Stderrln(fmt.Sprintf("    %-40s %5.1f%% (%d/%d)", shortPath, s.pct, s.cov, s.tot))
	}

	log.Stderrln("\n  Per-test:")
	for _, ts := range summary.PerTest {
		name := ts.TestName
		if name == "" {
			name = ts.TestID
		}
		log.Stderrln(fmt.Sprintf("    %-40s %d lines across %d files", name, ts.CoveredLines, ts.FilesTouched))
	}

	log.Stderrln(fmt.Sprintf("\n  Full report: %s", summaryPath))
	return nil
}

// --- Helpers ---

func dedup(sorted []int) []int {
	if len(sorted) == 0 {
		return sorted
	}
	result := []int{sorted[0]}
	for i := 1; i < len(sorted); i++ {
		if sorted[i] != sorted[i-1] {
			result = append(result, sorted[i])
		}
	}
	return result
}

func sanitizeFileName(name string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_")
	return replacer.Replace(name)
}

// normalizeFilePaths converts absolute file paths to repo-relative paths.
// Uses git root as the base (consistent across machines, handles monorepo
// files outside the service directory like ../shared/utils.js).
// Falls back to cwd if not in a git repo.
func normalizeFilePaths(lineCounts map[string]map[string]int) map[string]map[string]int {
	base := getPathNormalizationBase()
	if base == "" {
		return lineCounts
	}

	normalized := make(map[string]map[string]int, len(lineCounts))
	for absPath, lines := range lineCounts {
		relPath, err := filepath.Rel(base, absPath)
		if err != nil || strings.HasPrefix(relPath, "..") {
			// Outside the base - keep as-is rather than producing ../../... paths
			relPath = absPath
		}
		normalized[relPath] = lines
	}
	return normalized
}

// getPathNormalizationBase returns the git root, falling back to cwd.
func getPathNormalizationBase() string {
	if root, err := utils.GetGitRootDir(); err == nil {
		return root
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return ""
}
