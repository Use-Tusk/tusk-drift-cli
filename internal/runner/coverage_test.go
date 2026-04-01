package runner

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeFileName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "no special chars", input: "simple", expected: "simple"},
		{name: "forward slashes", input: "a/b/c", expected: "a_b_c"},
		{name: "backslashes", input: "a\\b\\c", expected: "a_b_c"},
		{name: "colons", input: "a:b:c", expected: "a_b_c"},
		{name: "spaces", input: "a b c", expected: "a_b_c"},
		{name: "mixed separators", input: "path/to:some file\\here", expected: "path_to_some_file_here"},
		{name: "empty string", input: "", expected: ""},
		{name: "already clean", input: "test_id_123", expected: "test_id_123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeFileName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

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

func TestLinecountsToCoverageDetail(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		result := LinecountsToCoverageDetail(nil)
		assert.Empty(t, result)
	})

	t.Run("empty map", func(t *testing.T) {
		result := LinecountsToCoverageDetail(map[string]map[string]int{})
		assert.Empty(t, result)
	})

	t.Run("single file with covered lines", func(t *testing.T) {
		input := map[string]map[string]int{
			"/app/main.go": {
				"1":  1,
				"2":  3,
				"5":  0,
				"10": 1,
			},
		}
		result := LinecountsToCoverageDetail(input)
		require.Contains(t, result, "/app/main.go")

		fd := result["/app/main.go"]
		assert.Equal(t, []int{1, 2, 10}, fd.CoveredLines)
		assert.Equal(t, 3, fd.CoveredCount)
		assert.Equal(t, 4, fd.CoverableLines) // total lines in the map
	})

	t.Run("file with only zero counts is excluded", func(t *testing.T) {
		input := map[string]map[string]int{
			"/app/unused.go": {
				"1": 0,
				"2": 0,
			},
		}
		result := LinecountsToCoverageDetail(input)
		assert.Empty(t, result)
	})

	t.Run("invalid line number strings are skipped", func(t *testing.T) {
		input := map[string]map[string]int{
			"/app/main.go": {
				"abc": 1,
				"0":   1, // line 0 is invalid (1-based)
				"-1":  1, // negative line is invalid
				"5":   1, // valid
			},
		}
		result := LinecountsToCoverageDetail(input)
		require.Contains(t, result, "/app/main.go")
		fd := result["/app/main.go"]
		assert.Equal(t, []int{5}, fd.CoveredLines)
		assert.Equal(t, 1, fd.CoveredCount)
	})

	t.Run("multiple files", func(t *testing.T) {
		input := map[string]map[string]int{
			"/app/a.go": {"1": 1, "2": 1},
			"/app/b.go": {"10": 2, "20": 0},
		}
		result := LinecountsToCoverageDetail(input)
		assert.Len(t, result, 2)
		assert.Equal(t, 2, result["/app/a.go"].CoveredCount)
		assert.Equal(t, 1, result["/app/b.go"].CoveredCount)
	})

	t.Run("covered lines are sorted and deduped", func(t *testing.T) {
		input := map[string]map[string]int{
			"/app/main.go": {
				"10": 1,
				"3":  1,
				"7":  1,
				"1":  1,
			},
		}
		result := LinecountsToCoverageDetail(input)
		require.Contains(t, result, "/app/main.go")
		assert.Equal(t, []int{1, 3, 7, 10}, result["/app/main.go"].CoveredLines)
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
				TestID: "test-1",
				LineCounts: map[string]map[string]int{
					"/app/main.go": {"1": 1, "2": 3},
				},
			},
		}
		result := mergeWithBaseline(nil, records)
		require.Contains(t, result, "/app/main.go")
		assert.Equal(t, 1, result["/app/main.go"]["1"])
		assert.Equal(t, 3, result["/app/main.go"]["2"])
	})

	t.Run("baseline with no records", func(t *testing.T) {
		baseline := map[string]map[string]int{
			"/app/main.go": {"1": 0, "2": 0, "3": 0},
		}
		result := mergeWithBaseline(baseline, nil)
		require.Contains(t, result, "/app/main.go")
		assert.Equal(t, 0, result["/app/main.go"]["1"])
		assert.Equal(t, 0, result["/app/main.go"]["2"])
		assert.Equal(t, 0, result["/app/main.go"]["3"])
	})

	t.Run("baseline merged with records adds counts", func(t *testing.T) {
		baseline := map[string]map[string]int{
			"/app/main.go": {"1": 0, "2": 0, "3": 0, "4": 0},
		}
		records := []CoverageTestRecord{
			{
				TestID: "test-1",
				LineCounts: map[string]map[string]int{
					"/app/main.go": {"1": 1, "3": 2},
				},
			},
			{
				TestID: "test-2",
				LineCounts: map[string]map[string]int{
					"/app/main.go": {"1": 1, "4": 1},
				},
			},
		}
		result := mergeWithBaseline(baseline, records)
		require.Contains(t, result, "/app/main.go")
		assert.Equal(t, 2, result["/app/main.go"]["1"]) // 0 + 1 + 1
		assert.Equal(t, 0, result["/app/main.go"]["2"]) // baseline 0, no test coverage
		assert.Equal(t, 2, result["/app/main.go"]["3"]) // 0 + 2
		assert.Equal(t, 1, result["/app/main.go"]["4"]) // 0 + 1
	})

	t.Run("records can add new files not in baseline", func(t *testing.T) {
		baseline := map[string]map[string]int{
			"/app/main.go": {"1": 0},
		}
		records := []CoverageTestRecord{
			{
				TestID: "test-1",
				LineCounts: map[string]map[string]int{
					"/app/new_file.go": {"10": 5},
				},
			},
		}
		result := mergeWithBaseline(baseline, records)
		assert.Len(t, result, 2)
		assert.Equal(t, 5, result["/app/new_file.go"]["10"])
	})

	t.Run("baseline is not mutated", func(t *testing.T) {
		baseline := map[string]map[string]int{
			"/app/main.go": {"1": 0},
		}
		records := []CoverageTestRecord{
			{
				TestID: "test-1",
				LineCounts: map[string]map[string]int{
					"/app/main.go": {"1": 5},
				},
			},
		}
		_ = mergeWithBaseline(baseline, records)
		// Original baseline should be untouched
		assert.Equal(t, 0, baseline["/app/main.go"]["1"])
	})
}
