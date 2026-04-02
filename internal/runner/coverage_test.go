package runner

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDedup(t *testing.T) {
	tests := []struct {
		name     string
		input    []int
		expected []int
	}{
		{name: "empty", input: []int{}, expected: []int{}},
		{name: "single element", input: []int{1}, expected: []int{1}},
		{name: "no duplicates", input: []int{1, 2, 3}, expected: []int{1, 2, 3}},
		{name: "all duplicates", input: []int{5, 5, 5}, expected: []int{5}},
		{name: "some duplicates", input: []int{1, 1, 2, 3, 3, 4}, expected: []int{1, 2, 3, 4}},
		{name: "duplicates at end", input: []int{1, 2, 3, 3}, expected: []int{1, 2, 3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := dedup(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper to create a simple FileCoverageData with just line counts
func makeFileData(lines map[string]int) FileCoverageData {
	return FileCoverageData{Lines: lines}
}

// Helper to create FileCoverageData with branches
func makeFileDataWithBranches(lines map[string]int, totalB, covB int, branches map[string]BranchInfo) FileCoverageData {
	return FileCoverageData{
		Lines:           lines,
		TotalBranches:   totalB,
		CoveredBranches: covB,
		Branches:        branches,
	}
}

func TestSnapshotToCoverageDetail(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		result := SnapshotToCoverageDetail(nil)
		assert.Empty(t, result)
	})

	t.Run("single file with covered lines", func(t *testing.T) {
		input := CoverageSnapshot{
			"/app/main.go": makeFileData(map[string]int{"1": 1, "2": 3, "5": 0, "10": 1}),
		}
		result := SnapshotToCoverageDetail(input)
		require.Contains(t, result, "/app/main.go")
		fd := result["/app/main.go"]
		assert.Equal(t, []int{1, 2, 10}, fd.CoveredLines)
		assert.Equal(t, 3, fd.CoveredCount)
		assert.Equal(t, 4, fd.CoverableLines)
	})

	t.Run("file with only zero counts is excluded", func(t *testing.T) {
		input := CoverageSnapshot{
			"/app/unused.go": makeFileData(map[string]int{"1": 0, "2": 0}),
		}
		result := SnapshotToCoverageDetail(input)
		assert.Empty(t, result)
	})

	t.Run("includes branch data", func(t *testing.T) {
		input := CoverageSnapshot{
			"/app/main.go": makeFileDataWithBranches(
				map[string]int{"1": 1, "5": 1},
				4, 2,
				map[string]BranchInfo{"5": {Total: 2, Covered: 1}},
			),
		}
		result := SnapshotToCoverageDetail(input)
		fd := result["/app/main.go"]
		assert.Equal(t, 4, fd.TotalBranches)
		assert.Equal(t, 2, fd.CoveredBranches)
		assert.Equal(t, 2, fd.Branches["5"].Total)
		assert.Equal(t, 1, fd.Branches["5"].Covered)
	})
}

func TestMergeWithBaseline(t *testing.T) {
	t.Run("nil baseline nil records", func(t *testing.T) {
		result := mergeWithBaseline(nil, nil)
		assert.Empty(t, result)
	})

	t.Run("nil baseline with records", func(t *testing.T) {
		records := []CoverageTestRecord{
			{
				TestID:   "test-1",
				Coverage: CoverageSnapshot{"/app/main.go": makeFileData(map[string]int{"1": 1, "2": 3})},
			},
		}
		result := mergeWithBaseline(nil, records)
		require.Contains(t, result, "/app/main.go")
		assert.Equal(t, 1, result["/app/main.go"].Lines["1"])
		assert.Equal(t, 3, result["/app/main.go"].Lines["2"])
	})

	t.Run("baseline with no records", func(t *testing.T) {
		baseline := CoverageSnapshot{
			"/app/main.go": makeFileData(map[string]int{"1": 0, "2": 0, "3": 0}),
		}
		result := mergeWithBaseline(baseline, nil)
		require.Contains(t, result, "/app/main.go")
		assert.Equal(t, 0, result["/app/main.go"].Lines["1"])
		assert.Equal(t, 0, result["/app/main.go"].Lines["3"])
	})

	t.Run("baseline merged with records adds counts", func(t *testing.T) {
		baseline := CoverageSnapshot{
			"/app/main.go": makeFileData(map[string]int{"1": 0, "2": 0, "3": 0, "4": 0}),
		}
		records := []CoverageTestRecord{
			{TestID: "test-1", Coverage: CoverageSnapshot{"/app/main.go": makeFileData(map[string]int{"1": 1, "3": 2})}},
			{TestID: "test-2", Coverage: CoverageSnapshot{"/app/main.go": makeFileData(map[string]int{"1": 1, "4": 1})}},
		}
		result := mergeWithBaseline(baseline, records)
		require.Contains(t, result, "/app/main.go")
		assert.Equal(t, 2, result["/app/main.go"].Lines["1"]) // 0+1+1
		assert.Equal(t, 0, result["/app/main.go"].Lines["2"]) // baseline 0, no test
		assert.Equal(t, 2, result["/app/main.go"].Lines["3"]) // 0+2
		assert.Equal(t, 1, result["/app/main.go"].Lines["4"]) // 0+1
	})

	t.Run("records can add new files not in baseline", func(t *testing.T) {
		baseline := CoverageSnapshot{
			"/app/main.go": makeFileData(map[string]int{"1": 0}),
		}
		records := []CoverageTestRecord{
			{TestID: "test-1", Coverage: CoverageSnapshot{"/app/new.go": makeFileData(map[string]int{"10": 5})}},
		}
		result := mergeWithBaseline(baseline, records)
		assert.Len(t, result, 2)
		assert.Equal(t, 5, result["/app/new.go"].Lines["10"])
	})

	t.Run("baseline is not mutated", func(t *testing.T) {
		baseline := CoverageSnapshot{
			"/app/main.go": makeFileData(map[string]int{"1": 0}),
		}
		records := []CoverageTestRecord{
			{TestID: "test-1", Coverage: CoverageSnapshot{"/app/main.go": makeFileData(map[string]int{"1": 5})}},
		}
		_ = mergeWithBaseline(baseline, records)
		assert.Equal(t, 0, baseline["/app/main.go"].Lines["1"])
	})

	t.Run("merges branch data", func(t *testing.T) {
		baseline := CoverageSnapshot{
			"/app/main.go": makeFileDataWithBranches(
				map[string]int{"1": 0},
				4, 0,
				map[string]BranchInfo{"5": {Total: 2, Covered: 0}},
			),
		}
		records := []CoverageTestRecord{
			{TestID: "test-1", Coverage: CoverageSnapshot{
				"/app/main.go": makeFileDataWithBranches(
					map[string]int{"1": 1},
					2, 1,
					map[string]BranchInfo{"5": {Total: 2, Covered: 1}},
				),
			}},
		}
		result := mergeWithBaseline(baseline, records)
		assert.Equal(t, 1, result["/app/main.go"].Branches["5"].Covered)
		assert.Equal(t, 2, result["/app/main.go"].Branches["5"].Total)
	})

	t.Run("branch union semantics: two tests cover different branches", func(t *testing.T) {
		baseline := CoverageSnapshot{
			"/app/main.go": makeFileDataWithBranches(
				map[string]int{"1": 0},
				2, 0,
				map[string]BranchInfo{"5": {Total: 2, Covered: 0}},
			),
		}
		records := []CoverageTestRecord{
			{TestID: "test-1", Coverage: CoverageSnapshot{
				"/app/main.go": makeFileDataWithBranches(
					map[string]int{"1": 1},
					2, 1,
					map[string]BranchInfo{"5": {Total: 2, Covered: 1}}, // test 1 covers 1 branch
				),
			}},
			{TestID: "test-2", Coverage: CoverageSnapshot{
				"/app/main.go": makeFileDataWithBranches(
					map[string]int{"1": 1},
					2, 1,
					map[string]BranchInfo{"5": {Total: 2, Covered: 1}}, // test 2 covers 1 branch
				),
			}},
		}
		result := mergeWithBaseline(baseline, records)
		// Union: 1 + 1 = 2, clamped to total 2
		assert.Equal(t, 2, result["/app/main.go"].Branches["5"].Covered)
		assert.Equal(t, 2, result["/app/main.go"].Branches["5"].Total)
	})

	t.Run("baseline branches not mutated", func(t *testing.T) {
		baseline := CoverageSnapshot{
			"/app/main.go": makeFileDataWithBranches(
				map[string]int{"1": 0},
				2, 0,
				map[string]BranchInfo{"5": {Total: 2, Covered: 0}},
			),
		}
		records := []CoverageTestRecord{
			{TestID: "test-1", Coverage: CoverageSnapshot{
				"/app/main.go": makeFileDataWithBranches(
					map[string]int{"1": 1},
					2, 1,
					map[string]BranchInfo{"5": {Total: 2, Covered: 1}},
				),
			}},
		}
		_ = mergeWithBaseline(baseline, records)
		// Original baseline branches should be untouched
		assert.Equal(t, 0, baseline["/app/main.go"].Branches["5"].Covered)
	})
}

func TestComputeCoverageSummary(t *testing.T) {
	t.Run("empty aggregate", func(t *testing.T) {
		summary := ComputeCoverageSummary(nil, nil, nil)
		assert.Equal(t, 0, summary.Aggregate.TotalCoverableLines)
		assert.Equal(t, 0.0, summary.Aggregate.CoveragePct)
	})

	t.Run("computes aggregate percentages", func(t *testing.T) {
		aggregate := CoverageSnapshot{
			"main.go": makeFileData(map[string]int{"1": 1, "2": 1, "3": 0, "4": 0}),
		}
		summary := ComputeCoverageSummary(aggregate, nil, nil)
		assert.Equal(t, 4, summary.Aggregate.TotalCoverableLines)
		assert.Equal(t, 2, summary.Aggregate.TotalCoveredLines)
		assert.Equal(t, 50.0, summary.Aggregate.CoveragePct)
		assert.Equal(t, 1, summary.Aggregate.TotalFiles)
		assert.Equal(t, 1, summary.Aggregate.CoveredFiles)
	})

	t.Run("computes per-file summaries", func(t *testing.T) {
		aggregate := CoverageSnapshot{
			"a.go": makeFileData(map[string]int{"1": 1, "2": 0}),
			"b.go": makeFileData(map[string]int{"1": 1, "2": 1}),
		}
		summary := ComputeCoverageSummary(aggregate, nil, nil)
		assert.Equal(t, 50.0, summary.PerFile["a.go"].CoveragePct)
		assert.Equal(t, 100.0, summary.PerFile["b.go"].CoveragePct)
	})

	t.Run("includes branch coverage", func(t *testing.T) {
		aggregate := CoverageSnapshot{
			"main.go": makeFileDataWithBranches(
				map[string]int{"1": 1},
				4, 2,
				map[string]BranchInfo{"5": {Total: 2, Covered: 1}},
			),
		}
		summary := ComputeCoverageSummary(aggregate, nil, nil)
		assert.Equal(t, 4, summary.Aggregate.TotalBranches)
		assert.Equal(t, 2, summary.Aggregate.CoveredBranches)
		assert.Equal(t, 50.0, summary.Aggregate.BranchCoveragePct)
	})

	t.Run("includes per-test summaries", func(t *testing.T) {
		aggregate := CoverageSnapshot{
			"main.go": makeFileData(map[string]int{"1": 1}),
		}
		perTest := map[string]map[string]CoverageFileDiff{
			"test-1": {"main.go": {CoveredCount: 5, CoverableLines: 10}},
			"test-2": {"main.go": {CoveredCount: 3, CoverableLines: 10}},
		}
		records := []CoverageTestRecord{
			{TestID: "test-1", TestName: "GET /api"},
			{TestID: "test-2", TestName: "POST /api"},
		}
		summary := ComputeCoverageSummary(aggregate, perTest, records)
		require.Len(t, summary.PerTest, 2)
		assert.Equal(t, 5, summary.PerTest[0].CoveredLines)
		assert.Equal(t, "GET /api", summary.PerTest[0].TestName)
		assert.Equal(t, 3, summary.PerTest[1].CoveredLines)
	})
}

func TestNormalizeCoveragePaths(t *testing.T) {
	t.Run("nil input returns empty", func(t *testing.T) {
		result := normalizeCoveragePaths(nil)
		assert.Len(t, result, 0)
	})

	t.Run("empty input returns empty", func(t *testing.T) {
		result := normalizeCoveragePaths(CoverageSnapshot{})
		assert.Empty(t, result)
	})

	// Note: full path normalization depends on git root which is environment-specific.
	// We test the function handles edge cases; full integration is tested E2E.
}
