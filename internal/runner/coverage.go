package runner

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Use-Tusk/tusk-cli/internal/log"
)

//go:embed scripts/process-v8-coverage.js
var processV8CoverageScript string

const coverageSnapshotTimeout = 5 * time.Second

// TakeCoverageSnapshot calls the SDK's coverage snapshot endpoint.
func (e *Executor) TakeCoverageSnapshot() error {
	if !e.coverageEnabled || e.coveragePort == 0 {
		return nil
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/snapshot", e.coveragePort)
	client := &http.Client{Timeout: coverageSnapshotTimeout}

	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to take coverage snapshot: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("coverage snapshot returned status %d: %s", resp.StatusCode, string(body))
	}

	// Small delay to let V8 finish writing the file
	time.Sleep(100 * time.Millisecond)
	return nil
}

// TakeCoverageSnapshotAndProcess takes a snapshot, processes the latest V8 file
// into compact line counts, and deletes the raw V8 file to save disk space.
// Returns the processed line counts.
func (e *Executor) TakeCoverageSnapshotAndProcess() (map[string]map[string]int, error) {
	if err := e.TakeCoverageSnapshot(); err != nil {
		return nil, err
	}

	files, err := e.ListV8CoverageFiles()
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return make(map[string]map[string]int), nil
	}

	// Process the latest file (cumulative snapshot)
	latestFile := files[len(files)-1]
	counts, err := processRawV8Coverage(latestFile)
	if err != nil {
		return nil, err
	}

	// Delete all raw V8 files except the latest (c8 needs the final one for aggregate)
	// Actually for aggregate we only need the last file, so we can delete earlier ones
	for i := 0; i < len(files)-1; i++ {
		os.Remove(files[i])
	}

	return counts, nil
}

// ListV8CoverageFiles returns sorted list of V8 coverage files in the raw dir.
func (e *Executor) ListV8CoverageFiles() ([]string, error) {
	entries, err := os.ReadDir(e.coverageRawDir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "coverage-") && strings.HasSuffix(entry.Name(), ".json") {
			files = append(files, filepath.Join(e.coverageRawDir, entry.Name()))
		}
	}
	sort.Strings(files)
	return files, nil
}

// CoverageTestRecord tracks processed coverage data for a single test.
type CoverageTestRecord struct {
	TestID      string
	TestName    string
	LineCounts  map[string]map[string]int // Processed line counts from V8 snapshot (cumulative)
}

// ProcessCoverage runs c8 report for aggregate and diffs pre-processed snapshots for per-test coverage.
// Records should include a baseline at index 0 followed by per-test records.
// Each record's LineCounts has already been extracted from raw V8 files during test execution.
func (e *Executor) ProcessCoverage(records []CoverageTestRecord) error {
	if !e.coverageEnabled {
		return nil
	}

	log.Stderrln("\n➤ Processing coverage data...")

	// 1. Run c8 report for aggregate (uses remaining raw V8 files in raw dir)
	aggregateDir := filepath.Join(e.coverageOutputDir, "aggregate")
	if err := os.MkdirAll(aggregateDir, 0o750); err != nil {
		return fmt.Errorf("failed to create aggregate dir: %w", err)
	}

	absRawDir, _ := filepath.Abs(e.coverageRawDir)
	if err := runC8Report(absRawDir, aggregateDir); err != nil {
		return fmt.Errorf("aggregate c8 report failed: %w", err)
	}

	// 2. Diff consecutive pre-processed snapshots for per-test coverage
	// records[0] = baseline, records[1..N] = per-test
	testRecords := make([]CoverageTestRecord, 0)
	for i := 1; i < len(records); i++ {
		prev := records[i-1].LineCounts
		curr := records[i].LineCounts

		diff := DiffV8LineCounts(prev, curr)

		testDir := filepath.Join(e.coverageOutputDir, sanitizeFileName(records[i].TestID))
		if err := os.MkdirAll(testDir, 0o750); err != nil {
			return err
		}

		diffPath := filepath.Join(testDir, "coverage.json")
		diffData, _ := json.MarshalIndent(diff, "", "  ")
		if err := os.WriteFile(diffPath, diffData, 0o644); err != nil {
			return err
		}

		testRecords = append(testRecords, records[i])
	}

	// 3. Clean up raw V8 files (aggregate already processed)
	os.RemoveAll(e.coverageRawDir)

	// 4. Print summary
	return e.printCoverageSummary(testRecords)
}

// processRawV8Coverage runs the Node.js helper to extract line-level counts from a V8 coverage file.
func processRawV8Coverage(v8FilePath string) (map[string]map[string]int, error) {
	absPath, _ := filepath.Abs(v8FilePath)

	// Find the helper script - it's embedded alongside the binary or in the source tree
	scriptPath := findCoverageScript()
	if scriptPath == "" {
		return nil, fmt.Errorf("could not find process-v8-coverage.js helper script")
	}

	sourceRoot, _ := os.Getwd()
	cmd := exec.Command("node", scriptPath, absPath, sourceRoot)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("V8 coverage processing failed: %w\nOutput: %s", err, string(output))
	}

	// Parse the output: { "/path/file.js": { "lines": { "10": 5, "11": 3 } } }
	var raw map[string]struct {
		Lines map[string]int `json:"lines"`
	}
	if err := json.Unmarshal(output, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse V8 coverage output: %w", err)
	}

	result := make(map[string]map[string]int)
	for filePath, data := range raw {
		result[filePath] = data.Lines
	}
	return result, nil
}

var cachedScriptPath string

// findCoverageScript writes the embedded script to a temp file and returns its path.
func findCoverageScript() string {
	if cachedScriptPath != "" {
		if _, err := os.Stat(cachedScriptPath); err == nil {
			return cachedScriptPath
		}
	}

	tmpFile, err := os.CreateTemp("", "tusk-v8-coverage-*.js")
	if err != nil {
		return ""
	}
	if _, err := tmpFile.WriteString(processV8CoverageScript); err != nil {
		tmpFile.Close()
		return ""
	}
	tmpFile.Close()
	cachedScriptPath = tmpFile.Name()
	return cachedScriptPath
}

// DiffV8LineCounts computes which lines were newly executed between two V8 snapshots.
func DiffV8LineCounts(prev, curr map[string]map[string]int) map[string]CoverageFileDiff {
	result := make(map[string]CoverageFileDiff)

	for filePath, currLines := range curr {
		prevLines := prev[filePath]

		var coveredLines []int
		for lineStr, currCount := range currLines {
			prevCount := 0
			if prevLines != nil {
				prevCount = prevLines[lineStr]
			}

			if currCount > prevCount {
				line := 0
				fmt.Sscanf(lineStr, "%d", &line)
				if line > 0 {
					coveredLines = append(coveredLines, line)
				}
			}
		}

		if len(coveredLines) > 0 {
			sort.Ints(coveredLines)
			coveredLines = dedup(coveredLines)
			result[filePath] = CoverageFileDiff{
				CoveredLines:   coveredLines,
				CoverableLines: len(currLines),
				CoveredCount:   len(coveredLines),
			}
		}
	}

	return result
}

// prepareSnapshotDir creates a temp dir with a single V8 coverage file for c8 to process.
func prepareSnapshotDir(v8FilePath string) (string, error) {
	tempDir, err := os.MkdirTemp("", "tusk-coverage-snapshot-*")
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(v8FilePath)
	if err != nil {
		os.RemoveAll(tempDir)
		return "", err
	}

	destPath := filepath.Join(tempDir, filepath.Base(v8FilePath))
	if err := os.WriteFile(destPath, data, 0o644); err != nil {
		os.RemoveAll(tempDir)
		return "", err
	}

	return tempDir, nil
}

// runC8Report runs npx c8 report on V8 coverage files in tempDir, outputting Istanbul JSON to outputDir.
func runC8Report(tempDir string, outputDir string) error {
	absOutputDir, _ := filepath.Abs(outputDir)
	absTempDir, _ := filepath.Abs(tempDir)

	cmd := exec.Command("npx", "c8", "report",
		"--all",
		"--temp-directory", absTempDir,
		"--report-dir", absOutputDir,
		"--reporter", "json",
	)
	cmd.Dir = "." // Run from project root so --all finds source files
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("c8 report failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

// IstanbulCoverage represents the Istanbul JSON coverage format.
type IstanbulCoverage map[string]IstanbulFileCoverage

type IstanbulFileCoverage struct {
	Path         string                       `json:"path"`
	StatementMap map[string]IstanbulLocation   `json:"statementMap"`
	S            map[string]int               `json:"s"`
	FnMap        map[string]json.RawMessage   `json:"fnMap,omitempty"`
	F            map[string]int               `json:"f,omitempty"`
	BranchMap    map[string]json.RawMessage   `json:"branchMap,omitempty"`
	B            map[string]json.RawMessage   `json:"b,omitempty"`
}

type IstanbulLocation struct {
	Start IstanbulPosition `json:"start"`
	End   IstanbulPosition `json:"end"`
}

type IstanbulPosition struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// CoverageFileDiff represents per-test coverage for a single file.
type CoverageFileDiff struct {
	CoveredLines   []int `json:"covered_lines"`
	CoverableLines int   `json:"coverable_lines"`
	CoveredCount   int   `json:"covered_count"`
}

// diffIstanbulCoverage computes which statements were newly executed between two Istanbul JSON snapshots.
func diffIstanbulCoverage(prevPath, currPath string) (map[string]CoverageFileDiff, error) {
	prev, err := loadIstanbulJSON(prevPath)
	if err != nil {
		return nil, fmt.Errorf("loading prev coverage: %w", err)
	}
	curr, err := loadIstanbulJSON(currPath)
	if err != nil {
		return nil, fmt.Errorf("loading curr coverage: %w", err)
	}

	result := make(map[string]CoverageFileDiff)

	for filePath, currFile := range curr {
		prevFile, hasPrev := prev[filePath]

		var coveredLines []int
		coverableLines := len(currFile.S)

		for stmtID, currCount := range currFile.S {
			prevCount := 0
			if hasPrev {
				prevCount = prevFile.S[stmtID]
			}

			// This statement was executed during this test (delta > 0)
			if currCount > prevCount {
				// Get the line number from statementMap
				if loc, ok := currFile.StatementMap[stmtID]; ok {
					coveredLines = append(coveredLines, loc.Start.Line)
				}
			}
		}

		if len(coveredLines) > 0 {
			sort.Ints(coveredLines)
			// Deduplicate (multiple statements can be on same line)
			coveredLines = dedup(coveredLines)
			result[filePath] = CoverageFileDiff{
				CoveredLines:   coveredLines,
				CoverableLines: coverableLines,
				CoveredCount:   len(coveredLines),
			}
		}
	}

	return result, nil
}

func loadIstanbulJSON(path string) (IstanbulCoverage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cov IstanbulCoverage
	if err := json.Unmarshal(data, &cov); err != nil {
		return nil, err
	}
	return cov, nil
}

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
	// Replace characters that are problematic in file paths
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_")
	return replacer.Replace(name)
}

// CoverageSummary is the final output written to summary.json.
type CoverageSummary struct {
	Timestamp     string                       `json:"timestamp"`
	Aggregate     CoverageAggregate            `json:"aggregate"`
	PerFile       map[string]CoverageFileSummary `json:"per_file"`
	PerTest       []CoverageTestSummary        `json:"per_test"`
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

func (e *Executor) printCoverageSummary(records []CoverageTestRecord) error {
	// Load aggregate Istanbul JSON
	aggregatePath := filepath.Join(e.coverageOutputDir, "aggregate", "coverage-final.json")
	aggCov, err := loadIstanbulJSON(aggregatePath)
	if err != nil {
		return fmt.Errorf("failed to load aggregate coverage: %w", err)
	}

	summary := CoverageSummary{
		Timestamp: time.Now().Format(time.RFC3339),
		PerFile:   make(map[string]CoverageFileSummary),
	}

	totalCoverable := 0
	totalCovered := 0
	coveredFiles := 0

	for filePath, fileCov := range aggCov {
		coverable := len(fileCov.S)
		covered := 0
		for _, count := range fileCov.S {
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
			CoveredLines:   covered,
			CoverableLines: coverable,
			CoveragePct:    pct,
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
		TotalFiles:          len(aggCov),
		CoveredFiles:        coveredFiles,
	}

	// Per-test summaries
	for _, record := range records {
		testDir := filepath.Join(e.coverageOutputDir, sanitizeFileName(record.TestID))
		diffPath := filepath.Join(testDir, "coverage.json")

		testSummary := CoverageTestSummary{
			TestID:   record.TestID,
			TestName: record.TestName,
		}

		data, err := os.ReadFile(diffPath)
		if err == nil {
			var diff map[string]CoverageFileDiff
			if json.Unmarshal(data, &diff) == nil {
				totalLines := 0
				for _, fd := range diff {
					totalLines += fd.CoveredCount
				}
				testSummary.CoveredLines = totalLines
				testSummary.FilesTouched = len(diff)
			}
		}

		summary.PerTest = append(summary.PerTest, testSummary)
	}

	// Write summary.json
	summaryPath := filepath.Join(e.coverageOutputDir, "summary.json")
	summaryData, _ := json.MarshalIndent(summary, "", "  ")
	if err := os.WriteFile(summaryPath, summaryData, 0o644); err != nil {
		return err
	}

	// Print to console
	log.Stderrln(fmt.Sprintf("\n📊 Coverage: %.1f%% (%d/%d coverable lines across %d files)",
		aggPct, totalCovered, totalCoverable, len(aggCov)))

	// Print top files
	type fileStat struct {
		path string
		pct  float64
		cov  int
		tot  int
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
