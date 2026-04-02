package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Use-Tusk/tusk-cli/internal/log"
	"github.com/Use-Tusk/tusk-cli/internal/utils"
)

const (
	coverageBaselineMaxRetries   = 15
	coverageBaselineRetryDelay   = 200 * time.Millisecond
	coverageSnapshotTimeout      = 10 * time.Second
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

	return normalizeCoveragePaths(snapshot), nil
}

// BranchInfo tracks branch coverage at a specific line.
type BranchInfo struct {
	Total   int `json:"total"`
	Covered int `json:"covered"`
}

// FileCoverageData is the internal representation of per-file coverage.
type FileCoverageData struct {
	Lines           map[string]int
	TotalBranches   int
	CoveredBranches int
	Branches        map[string]BranchInfo
}

// CoverageSnapshot is the full coverage data for a snapshot.
type CoverageSnapshot map[string]FileCoverageData

// CoverageTestRecord holds per-test coverage data.
type CoverageTestRecord struct {
	TestID   string
	TestName string
	Coverage CoverageSnapshot
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

// ProcessCoverage computes aggregate coverage and prints the summary.
// All data stays in memory - no files written to user's project.
func (e *Executor) ProcessCoverage(records []CoverageTestRecord) error {
	if !e.coverageEnabled || len(records) == 0 {
		return nil
	}

	log.Stderrln("\n➤ Processing coverage data...")

	// Compute aggregate: start with baseline (all coverable lines including count=0),
	// then merge in per-test coverage. This gives accurate denominator.
	aggregate := mergeWithBaseline(e.coverageBaseline, records)

	return e.printCoverageSummary(records, aggregate)
}

// mergeWithBaseline creates aggregate coverage by starting from the baseline
// (all coverable lines including count=0) and unioning per-test data.
//
// Branch merging uses UNION semantics: if test A covers branch path 1 and test B
// covers branch path 2, the aggregate shows both paths as covered. This is done
// by summing covered counts per line (clamped to total) rather than taking max.
func mergeWithBaseline(baseline CoverageSnapshot, records []CoverageTestRecord) CoverageSnapshot {
	merged := make(CoverageSnapshot)

	// Deep-copy baseline (don't mutate the original)
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

// printCoverageSummary computes and prints the coverage summary to stderr.
func (e *Executor) printCoverageSummary(records []CoverageTestRecord, aggregate CoverageSnapshot) error {
	summary := ComputeCoverageSummary(aggregate, e.coveragePerTest, records)

	// Aggregate line
	coverageMsg := fmt.Sprintf("\n📊 Coverage: %.1f%% lines (%d/%d)",
		summary.Aggregate.CoveragePct, summary.Aggregate.TotalCoveredLines, summary.Aggregate.TotalCoverableLines)
	if summary.Aggregate.TotalBranches > 0 {
		coverageMsg += fmt.Sprintf(", %.1f%% branches (%d/%d)",
			summary.Aggregate.BranchCoveragePct, summary.Aggregate.CoveredBranches, summary.Aggregate.TotalBranches)
	}
	coverageMsg += fmt.Sprintf(" across %d files", summary.Aggregate.TotalFiles)
	log.Stderrln(coverageMsg)

	// Per-file breakdown sorted by coverage %
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
	sort.Slice(stats, func(i, j int) bool { return stats[i].pct > stats[j].pct })

	log.Stderrln("\n  Per-file:")
	for _, s := range stats {
		log.Stderrln(fmt.Sprintf("    %-40s %5.1f%% (%d/%d)", s.path, s.pct, s.cov, s.tot))
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


// normalizeCoveragePaths converts absolute file paths to repo-relative paths.
// Uses git root as the base (consistent across machines, handles monorepo
// files outside the service directory like ../shared/utils.js).
// Falls back to cwd if not in a git repo.
func normalizeCoveragePaths(snapshot CoverageSnapshot) CoverageSnapshot {
	base := getPathNormalizationBase()
	if base == "" {
		return snapshot
	}

	normalized := make(CoverageSnapshot, len(snapshot))
	for absPath, fileData := range snapshot {
		relPath, err := filepath.Rel(base, absPath)
		if err != nil || strings.HasPrefix(relPath, "..") {
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
