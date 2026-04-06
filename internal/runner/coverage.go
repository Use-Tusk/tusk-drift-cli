package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Use-Tusk/tusk-cli/internal/log"
	"github.com/Use-Tusk/tusk-cli/internal/utils"
	"github.com/bmatcuk/doublestar/v4"
)

const (
	coverageBaselineMaxRetries   = 15
	coverageBaselineRetryDelay   = 200 * time.Millisecond
	coverageSnapshotTimeout      = 60 * time.Second
)

// TakeCoverageSnapshot calls the SDK's coverage snapshot endpoint.
// Returns per-file coverage data for this test only (counters auto-reset).
func (e *Executor) TakeCoverageSnapshot() (CoverageSnapshot, error) {
	return e.callCoverageEndpoint(false)
}

// TakeCoverageBaseline calls the SDK's coverage snapshot endpoint with ?baseline=true.
// Returns ALL coverable lines (including uncovered at count=0) for the aggregate denominator.
// Retries with backoff since the coverage server may not be ready immediately after service start.
func (e *Executor) TakeCoverageBaseline() (CoverageSnapshot, error) {
	var lastErr error
	for attempt := 0; attempt < coverageBaselineMaxRetries; attempt++ {
		result, err := e.callCoverageEndpoint(true)
		if err == nil {
			return result, nil
		}
		lastErr = err
		time.Sleep(coverageBaselineRetryDelay)
	}
	return nil, fmt.Errorf("coverage baseline failed after retries: %w", lastErr)
}

func (e *Executor) callCoverageEndpoint(baseline bool) (CoverageSnapshot, error) {
	if !e.coverageEnabled || e.server == nil {
		return nil, nil
	}

	resp, err := e.server.SendCoverageSnapshot(baseline)
	if err != nil {
		return nil, fmt.Errorf("coverage snapshot failed: %w", err)
	}

	// Convert protobuf response to our internal format
	snapshot := make(CoverageSnapshot)
	for filePath, fileData := range resp.Coverage {
		branches := make(map[string]BranchInfo)
		for line, branchProto := range fileData.Branches {
			branches[line] = BranchInfo{
				Total:   int(branchProto.Total),
				Covered: int(branchProto.Covered),
			}
		}

		lines := make(map[string]int)
		for line, count := range fileData.Lines {
			lines[line] = int(count)
		}

		snapshot[filePath] = FileCoverageData{
			Lines:           lines,
			TotalBranches:   int(fileData.TotalBranches),
			CoveredBranches: int(fileData.CoveredBranches),
			Branches:        branches,
		}
	}

	return normalizeCoveragePaths(snapshot, e.coverageStripPrefix), nil
}

// BranchInfo tracks branch coverage at a specific line.
type BranchInfo struct {
	Total   int `json:"total"`
	Covered int `json:"covered"`
}

// FileCoverageData is the internal representation of per-file coverage.
type FileCoverageData struct {
	Lines           map[string]int        `json:"lines"`
	TotalBranches   int                   `json:"total_branches"`
	CoveredBranches int                   `json:"covered_branches"`
	Branches        map[string]BranchInfo `json:"branches,omitempty"`
}

// CoverageSnapshot is the full coverage data for a snapshot.
type CoverageSnapshot map[string]FileCoverageData

// CoverageTestRecord holds per-test coverage data.
type CoverageTestRecord struct {
	TestID      string
	TestName    string
	SuiteStatus string // "draft", "in_suite", or "" (local)
	Coverage    CoverageSnapshot
}

// CoverageFileDiff represents per-test coverage for a single file.
type CoverageFileDiff struct {
	CoveredLines    []int                  `json:"covered_lines"`
	CoverableLines  int                    `json:"coverable_lines"`
	CoveredCount    int                    `json:"covered_count"`
	TotalBranches   int                    `json:"total_branches"`
	CoveredBranches int                    `json:"covered_branches"`
	Branches        map[string]BranchInfo  `json:"branches,omitempty"`
}

// SnapshotToCoverageDetail converts a CoverageSnapshot to per-file CoverageFileDiff format.
func SnapshotToCoverageDetail(snapshot CoverageSnapshot) map[string]CoverageFileDiff {
	result := make(map[string]CoverageFileDiff)
	for filePath, fileData := range snapshot {
		var covered []int
		for lineStr, count := range fileData.Lines {
			if count > 0 {
				line, err := strconv.Atoi(lineStr)
				if err != nil || line <= 0 {
					log.Debug("Skipping invalid line number in coverage data", "line", lineStr, "file", filePath)
					continue
				}
				covered = append(covered, line)
			}
		}
		if len(covered) > 0 {
			sort.Ints(covered)
			covered = dedup(covered)
			// Deep-copy branches to avoid shared references
			branchesCopy := make(map[string]BranchInfo, len(fileData.Branches))
			for line, info := range fileData.Branches {
				branchesCopy[line] = info
			}
			result[filePath] = CoverageFileDiff{
				CoveredLines:    covered,
				CoverableLines:  len(fileData.Lines),
				CoveredCount:    len(covered),
				TotalBranches:   fileData.TotalBranches,
				CoveredBranches: fileData.CoveredBranches,
				Branches:        branchesCopy,
			}
		}
	}
	return result
}

// ProcessCoverage computes aggregate coverage, optionally prints summary, writes file, and prepares for upload.
// During validation runs, the aggregate and output files only include IN_SUITE tests (not drafts).
// All per-test data (including drafts) is retained for backend upload — the backend needs draft
// coverage for promotion decisions ("does this draft add unique coverage?").
func (e *Executor) ProcessCoverage(records []CoverageTestRecord) error {
	if !e.coverageEnabled || len(records) == 0 {
		return nil
	}

	// Filter to in-suite tests for the aggregate. If no suite status is set
	// (local run, no cloud), include all tests.
	suiteRecords := filterInSuiteRecords(records)

	// Compute aggregate: start with baseline (all coverable lines including count=0),
	// then merge in per-test coverage. This gives accurate denominator.
	aggregate := mergeWithBaseline(e.coverageBaseline, suiteRecords)

	// Apply include/exclude patterns from config
	aggregate = filterCoverageByPatterns(aggregate, e.coverageIncludePatterns, e.coverageExcludePatterns)

	// Print summary if --show-coverage was passed (not in silent config-driven mode)
	if e.coverageShowOutput {
		log.Stderrln("\n➤ Processing coverage data...")
		e.printCoverageSummary(suiteRecords, aggregate)
	}

	// Write coverage file if requested.
	// During validation runs, aggregate and output only include IN_SUITE tests.
	// Draft coverage is excluded from the file but retained for backend upload.
	if e.coverageOutputPath != "" {
		outPath := e.coverageOutputPath
		if !filepath.IsAbs(outPath) {
			if cwd, err := os.Getwd(); err == nil {
				outPath = filepath.Join(cwd, outPath)
			}
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return fmt.Errorf("failed to create coverage output directory: %w", err)
		}

		if strings.HasSuffix(strings.ToLower(outPath), ".json") {
			if err := WriteCoverageJSON(outPath, aggregate, e.coveragePerTest, suiteRecords); err != nil {
				return fmt.Errorf("failed to write coverage JSON: %w", err)
			}
		} else {
			if err := WriteCoverageLCOV(outPath, aggregate); err != nil {
				return fmt.Errorf("failed to write coverage LCOV: %w", err)
			}
		}
		if e.coverageShowOutput {
			log.Stderrln(fmt.Sprintf("\n📄 Coverage written to %s", e.coverageOutputPath))
		}
	}

	return nil
}

// mergeWithBaseline creates aggregate coverage by starting from the baseline
// (all coverable lines including count=0) and unioning per-test data.
//
// Branch merging uses UNION semantics: if test A covers branch path 1 and test B
// covers branch path 2, the aggregate shows both paths as covered. This is done
// by summing covered counts per line (clamped to total) rather than taking max.
func mergeWithBaseline(baseline CoverageSnapshot, records []CoverageTestRecord) CoverageSnapshot {
	merged := make(CoverageSnapshot)

	// Deep-copy baseline (don't mutate the original).
	// Baseline lines include startup-covered counts (count > 0 for lines executed
	// during module loading). These count toward "covered" in the aggregate,
	// matching industry standard behavior (Istanbul, NYC, coverage.py, etc.).
	if baseline != nil {
		for filePath, fileData := range baseline {
			lines := make(map[string]int, len(fileData.Lines))
			for line, count := range fileData.Lines {
				lines[line] = count
			}
			branches := make(map[string]BranchInfo, len(fileData.Branches))
			for line, info := range fileData.Branches {
				branches[line] = info // BranchInfo is a value type, safe to copy
			}
			merged[filePath] = FileCoverageData{
				Lines:           lines,
				TotalBranches:   fileData.TotalBranches,
				CoveredBranches: fileData.CoveredBranches,
				Branches:        branches,
			}
		}
	}

	// Union per-test coverage into the merged result
	for _, record := range records {
		for filePath, fileData := range record.Coverage {
			existing, ok := merged[filePath]
			if !ok {
				existing = FileCoverageData{
					Lines:    make(map[string]int),
					Branches: make(map[string]BranchInfo),
				}
			}
			// Add line counts
			for line, count := range fileData.Lines {
				existing.Lines[line] += count
			}
			// Union branch data: sum covered counts (clamped to total)
			for line, branchInfo := range fileData.Branches {
				if existing.Branches == nil {
					existing.Branches = make(map[string]BranchInfo)
				}
				eb := existing.Branches[line]
				if branchInfo.Total > eb.Total {
					eb.Total = branchInfo.Total
				}
				newCovered := eb.Covered + branchInfo.Covered
				if newCovered > eb.Total || newCovered < 0 { // Clamp + overflow guard
					eb.Covered = eb.Total
				} else {
					eb.Covered = newCovered
				}
				existing.Branches[line] = eb
			}
			// Recompute file-level branch totals from per-line data
			totalB, covB := 0, 0
			for _, b := range existing.Branches {
				totalB += b.Total
				covB += b.Covered
			}
			existing.TotalBranches = totalB
			existing.CoveredBranches = covB
			merged[filePath] = existing
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
	TotalCoverableLines  int     `json:"total_coverable_lines"`
	TotalCoveredLines    int     `json:"total_covered_lines"`
	CoveragePct          float64 `json:"coverage_pct"`
	TotalFiles           int     `json:"total_files"`
	CoveredFiles         int     `json:"covered_files"`
	TotalBranches        int     `json:"total_branches"`
	CoveredBranches      int     `json:"covered_branches"`
	BranchCoveragePct    float64 `json:"branch_coverage_pct"`
}

type CoverageFileSummary struct {
	CoveredLines    int     `json:"covered_lines"`
	CoverableLines  int     `json:"coverable_lines"`
	CoveragePct     float64 `json:"coverage_pct"`
	TotalBranches   int     `json:"total_branches"`
	CoveredBranches int     `json:"covered_branches"`
}

type CoverageTestSummary struct {
	TestID       string `json:"test_id"`
	TestName     string `json:"test_name"`
	CoveredLines int    `json:"covered_lines"`
	FilesTouched int    `json:"files_touched"`
}

// ComputeCoverageSummary builds a CoverageSummary from aggregate coverage data
// and per-test detail. This is a pure function (no side effects, no I/O).
func ComputeCoverageSummary(
	aggregate CoverageSnapshot,
	perTestDetail map[string]map[string]CoverageFileDiff,
	records []CoverageTestRecord,
) CoverageSummary {
	summary := CoverageSummary{
		Timestamp: time.Now().Format(time.RFC3339),
		PerFile:   make(map[string]CoverageFileSummary),
	}

	totalCoverable := 0
	totalCovered := 0
	totalBranches := 0
	totalCoveredBranches := 0
	coveredFiles := 0

	for filePath, fileData := range aggregate {
		coverable := len(fileData.Lines)
		covered := 0
		for _, count := range fileData.Lines {
			if count > 0 {
				covered++
			}
		}
		totalCoverable += coverable
		totalCovered += covered
		totalBranches += fileData.TotalBranches
		totalCoveredBranches += fileData.CoveredBranches
		if covered > 0 {
			coveredFiles++
		}
		pct := 0.0
		if coverable > 0 {
			pct = float64(covered) / float64(coverable) * 100
		}
		summary.PerFile[filePath] = CoverageFileSummary{
			CoveredLines:    covered,
			CoverableLines:  coverable,
			CoveragePct:     pct,
			TotalBranches:   fileData.TotalBranches,
			CoveredBranches: fileData.CoveredBranches,
		}
	}

	aggPct := 0.0
	if totalCoverable > 0 {
		aggPct = float64(totalCovered) / float64(totalCoverable) * 100
	}
	branchPct := 0.0
	if totalBranches > 0 {
		branchPct = float64(totalCoveredBranches) / float64(totalBranches) * 100
	}

	summary.Aggregate = CoverageAggregate{
		TotalCoverableLines: totalCoverable,
		TotalCoveredLines:   totalCovered,
		CoveragePct:         aggPct,
		TotalFiles:          len(aggregate),
		CoveredFiles:        coveredFiles,
		TotalBranches:       totalBranches,
		CoveredBranches:     totalCoveredBranches,
		BranchCoveragePct:   branchPct,
	}

	for _, record := range records {
		ts := CoverageTestSummary{TestID: record.TestID, TestName: record.TestName}
		if detail, ok := perTestDetail[record.TestID]; ok {
			for _, fd := range detail {
				ts.CoveredLines += fd.CoveredCount
			}
			ts.FilesTouched = len(detail)
		}
		summary.PerTest = append(summary.PerTest, ts)
	}

	return summary
}

// formatCoverageSummary formats coverage summary as lines of text.
func (e *Executor) formatCoverageSummary(summary CoverageSummary) []string {
	var lines []string

	// Aggregate line
	coverageMsg := fmt.Sprintf("📊 Coverage: %.1f%% lines (%d/%d)",
		summary.Aggregate.CoveragePct, summary.Aggregate.TotalCoveredLines, summary.Aggregate.TotalCoverableLines)
	if summary.Aggregate.TotalBranches > 0 {
		coverageMsg += fmt.Sprintf(", %.1f%% branches (%d/%d)",
			summary.Aggregate.BranchCoveragePct, summary.Aggregate.CoveredBranches, summary.Aggregate.TotalBranches)
	}
	coverageMsg += fmt.Sprintf(" across %d files", summary.Aggregate.TotalFiles)
	if e.coverageBaseline == nil {
		coverageMsg += "  ⚠️  baseline failed - denominator may be incomplete"
	}
	lines = append(lines, coverageMsg)

	// Per-file breakdown sorted alphabetically
	type fileStat struct {
		path     string
		pct      float64
		cov, tot int
	}
	var stats []fileStat
	for fp, fs := range summary.PerFile {
		if fs.CoverableLines > 0 {
			stats = append(stats, fileStat{fp, fs.CoveragePct, fs.CoveredLines, fs.CoverableLines})
		}
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].path < stats[j].path })

	lines = append(lines, "")
	lines = append(lines, "  Per-file:")
	for _, s := range stats {
		lines = append(lines, fmt.Sprintf("    %-40s %5.1f%% (%d/%d)", s.path, s.pct, s.cov, s.tot))
	}

	return lines
}

// printCoverageSummary computes and prints the coverage summary to stderr.
func (e *Executor) printCoverageSummary(records []CoverageTestRecord, aggregate CoverageSnapshot) {
	summary := ComputeCoverageSummary(aggregate, e.coveragePerTest, records)
	for _, line := range e.formatCoverageSummary(summary) {
		log.Stderrln(line)
	}

	// Per-test breakdown
	log.Stderrln("\n  Per-test:")
	for _, ts := range summary.PerTest {
		name := ts.TestName
		if name == "" {
			name = ts.TestID
		}
		log.Stderrln(fmt.Sprintf("    %-40s %d lines across %d files", name, ts.CoveredLines, ts.FilesTouched))
	}
}

// FormatCoverageSummaryLines computes the coverage summary and returns formatted
// lines for the TUI service log panel (aggregate + per-file, no per-test).
func (e *Executor) FormatCoverageSummaryLines(records []CoverageTestRecord) []string {
	if !e.coverageEnabled || len(records) == 0 {
		return nil
	}

	records = filterInSuiteRecords(records)
	aggregate := mergeWithBaseline(e.coverageBaseline, records)
	aggregate = filterCoverageByPatterns(aggregate, e.coverageIncludePatterns, e.coverageExcludePatterns)
	summary := ComputeCoverageSummary(aggregate, e.coveragePerTest, records)
	return e.formatCoverageSummary(summary)
}

// filterCoverageByPatterns applies include/exclude glob patterns to a snapshot.
// Include (if set): only keep files matching at least one include pattern.
// Exclude: remove files matching any exclude pattern.
// Include is applied first, then exclude.
// Supports ** for recursive directory matching:
//   - "**/migrations/**"      matches any file in any migrations/ directory
//   - "backend/src/db/**"     matches everything under backend/src/db/
//   - "**/*.test.ts"          matches any .test.ts file
//   - "backend/src/db/migrations/**" matches specific path
func filterCoverageByPatterns(snapshot CoverageSnapshot, include, exclude []string) CoverageSnapshot {
	if len(include) == 0 && len(exclude) == 0 {
		return snapshot
	}
	filtered := make(CoverageSnapshot, len(snapshot))
	for filePath, data := range snapshot {
		// Include filter: if patterns are set, file must match at least one
		if len(include) > 0 && !matchesAnyPattern(filePath, include) {
			continue
		}
		// Exclude filter: file must not match any
		if len(exclude) > 0 && matchesAnyPattern(filePath, exclude) {
			continue
		}
		filtered[filePath] = data
	}
	return filtered
}

// matchesAnyPattern checks if a file path matches any of the glob patterns.
// Uses doublestar for proper ** support.
func matchesAnyPattern(filePath string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, _ := doublestar.Match(pattern, filePath); matched {
			return true
		}
	}
	return false
}

// matchGlob matches a path against a glob pattern supporting **.
// Exported for testing.
func matchGlob(filePath, pattern string) bool {
	matched, _ := doublestar.Match(pattern, filePath)
	return matched
}

// --- Coverage Export ---

// CoverageExport is the top-level JSON export structure.
type CoverageExport struct {
	Summary   CoverageSummary                        `json:"summary"`
	Aggregate CoverageSnapshot                       `json:"aggregate"`
	PerTest   map[string]map[string]CoverageFileDiff `json:"per_test"`
}

// WriteCoverageJSON writes aggregate + per-test coverage as JSON.
func WriteCoverageJSON(path string, aggregate CoverageSnapshot, perTest map[string]map[string]CoverageFileDiff, records []CoverageTestRecord) error {
	summary := ComputeCoverageSummary(aggregate, perTest, records)

	// Build set of allowed test IDs from the filtered in-suite records
	allowedTestIDs := make(map[string]struct{}, len(records))
	for _, r := range records {
		allowedTestIDs[r.TestID] = struct{}{}
	}

	// Filter per-test data to only include in-suite tests and files present in the (filtered) aggregate
	filteredPerTest := make(map[string]map[string]CoverageFileDiff, len(perTest))
	for testID, testDetail := range perTest {
		if _, ok := allowedTestIDs[testID]; !ok {
			continue
		}
		filtered := make(map[string]CoverageFileDiff)
		for fp, fd := range testDetail {
			if _, ok := aggregate[fp]; ok {
				filtered[fp] = fd
			}
		}
		if len(filtered) > 0 {
			filteredPerTest[testID] = filtered
		}
	}

	export := CoverageExport{
		Summary:   summary,
		Aggregate: aggregate,
		PerTest:   filteredPerTest,
	}

	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// WriteCoverageLCOV writes aggregate coverage data in LCOV format.
func WriteCoverageLCOV(path string, aggregate CoverageSnapshot) error {
	var b strings.Builder

	// Sort file paths for deterministic output
	filePaths := make([]string, 0, len(aggregate))
	for fp := range aggregate {
		filePaths = append(filePaths, fp)
	}
	sort.Strings(filePaths)

	for _, filePath := range filePaths {
		fileData := aggregate[filePath]
		b.WriteString("SF:")
		b.WriteString(filePath)
		b.WriteByte('\n')

		// Line data (DA:line,count)
		lineNums := make([]int, 0, len(fileData.Lines))
		for lineStr := range fileData.Lines {
			if n, err := strconv.Atoi(lineStr); err == nil {
				lineNums = append(lineNums, n)
			}
		}
		sort.Ints(lineNums)

		linesFound := 0
		linesHit := 0
		for _, line := range lineNums {
			count := fileData.Lines[strconv.Itoa(line)]
			b.WriteString(fmt.Sprintf("DA:%d,%d\n", line, count))
			linesFound++
			if count > 0 {
				linesHit++
			}
		}

		// Branch data (BRDA:line,block,branch,count)
		branchLines := make([]int, 0, len(fileData.Branches))
		for lineStr := range fileData.Branches {
			if n, err := strconv.Atoi(lineStr); err == nil {
				branchLines = append(branchLines, n)
			}
		}
		sort.Ints(branchLines)

		branchesFound := 0
		branchesHit := 0
		for _, line := range branchLines {
			info := fileData.Branches[strconv.Itoa(line)]
			for i := 0; i < info.Total; i++ {
				count := 0
				if i < info.Covered {
					count = 1
				}
				b.WriteString(fmt.Sprintf("BRDA:%d,0,%d,%d\n", line, i, count))
				branchesFound++
				if count > 0 {
					branchesHit++
				}
			}
		}

		b.WriteString(fmt.Sprintf("LF:%d\n", linesFound))
		b.WriteString(fmt.Sprintf("LH:%d\n", linesHit))
		if branchesFound > 0 {
			b.WriteString(fmt.Sprintf("BRF:%d\n", branchesFound))
			b.WriteString(fmt.Sprintf("BRH:%d\n", branchesHit))
		}
		b.WriteString("end_of_record\n")
	}

	return os.WriteFile(path, []byte(b.String()), 0644)
}

// filterInSuiteRecords returns only records from in-suite tests.
// If no tests have suite status set (local run, no cloud), returns all records.
func filterInSuiteRecords(records []CoverageTestRecord) []CoverageTestRecord {
	hasSuiteStatus := false
	for _, r := range records {
		if r.SuiteStatus != "" {
			hasSuiteStatus = true
			break
		}
	}
	if !hasSuiteStatus {
		return records
	}

	var filtered []CoverageTestRecord
	for _, r := range records {
		if r.SuiteStatus != "draft" {
			filtered = append(filtered, r)
		}
	}
	return filtered
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


// normalizeCoveragePaths converts absolute file paths to repo-relative paths.
//
// For non-Docker: uses git root as the base (handles monorepos, cd into subdirs).
// For Docker: coverage.strip_path_prefix strips the container mount point first
// (e.g., "/app"), then git root normalization converts the rest to repo-relative.
func normalizeCoveragePaths(snapshot CoverageSnapshot, stripPrefix string) CoverageSnapshot {
	if len(snapshot) == 0 {
		return snapshot
	}

	// Step 1: Strip container path prefix if configured (Docker Compose)
	if stripPrefix != "" {
		stripPrefix = strings.TrimRight(stripPrefix, "/")
		stripped := make(CoverageSnapshot, len(snapshot))
		for absPath, fileData := range snapshot {
			newPath := absPath
			if strings.HasPrefix(absPath, stripPrefix+"/") {
				newPath = absPath[len(stripPrefix)+1:]
			} else if absPath == stripPrefix {
				newPath = "."
			}
			stripped[newPath] = fileData
		}
		snapshot = stripped
	}

	// Step 2: Normalize to git-root-relative paths
	base := getPathNormalizationBase()
	if base == "" {
		return snapshot
	}

	normalized := make(CoverageSnapshot, len(snapshot))
	for absPath, fileData := range snapshot {
		relPath, err := filepath.Rel(base, absPath)
		if err != nil || strings.HasPrefix(relPath, "..") {
			// Already relative (from strip_prefix) or outside git root — keep as-is
			relPath = absPath
		}
		normalized[relPath] = fileData
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
