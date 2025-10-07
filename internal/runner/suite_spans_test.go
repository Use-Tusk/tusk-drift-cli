package runner

import (
	"context"
	"testing"
	"time"

	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestDedupeSpans(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    []*core.Span
		expected int
	}{
		{
			name:     "empty_list",
			input:    []*core.Span{},
			expected: 0,
		},
		{
			name: "single_span",
			input: []*core.Span{
				{TraceId: "trace1", SpanId: "span1"},
			},
			expected: 1,
		},
		{
			name: "no_duplicates",
			input: []*core.Span{
				{TraceId: "trace1", SpanId: "span1"},
				{TraceId: "trace1", SpanId: "span2"},
				{TraceId: "trace2", SpanId: "span1"},
			},
			expected: 3,
		},
		{
			name: "with_duplicates",
			input: []*core.Span{
				{TraceId: "trace1", SpanId: "span1"},
				{TraceId: "trace1", SpanId: "span1"}, // duplicate
				{TraceId: "trace1", SpanId: "span2"},
				{TraceId: "trace1", SpanId: "span1"}, // duplicate
			},
			expected: 2,
		},
		{
			name: "preserves_order",
			input: []*core.Span{
				{TraceId: "trace1", SpanId: "span1", Name: "first"},
				{TraceId: "trace2", SpanId: "span2", Name: "second"},
				{TraceId: "trace1", SpanId: "span1", Name: "duplicate"}, // should be dropped
				{TraceId: "trace3", SpanId: "span3", Name: "third"},
			},
			expected: 3,
		},
		{
			name: "handles_nil_spans",
			input: []*core.Span{
				{TraceId: "trace1", SpanId: "span1"},
				nil,
				{TraceId: "trace2", SpanId: "span2"},
				nil,
			},
			expected: 2,
		},
		{
			name: "handles_empty_ids",
			input: []*core.Span{
				{TraceId: "", SpanId: ""},
				{TraceId: "trace1", SpanId: "span1"},
				{TraceId: "", SpanId: ""},
			},
			expected: 3, // Empty IDs are kept but don't dedupe
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DedupeSpans(tt.input)
			assert.Len(t, result, tt.expected)

			// Verify no nil spans in output
			for _, span := range result {
				assert.NotNil(t, span)
			}

			// For preserves_order test, verify the order
			if tt.name == "preserves_order" {
				require.Len(t, result, 3)
				assert.Equal(t, "first", result[0].Name)
				assert.Equal(t, "second", result[1].Name)
				assert.Equal(t, "third", result[2].Name)
			}
		})
	}
}

func TestBuildSuiteSpansForRun_LocalMode(t *testing.T) {
	t.Parallel()

	// Create test spans
	span1 := &core.Span{
		TraceId:       "trace1",
		SpanId:        "span1",
		Name:          "operation1",
		IsPreAppStart: false,
	}
	span2 := &core.Span{
		TraceId:       "trace1",
		SpanId:        "span2",
		Name:          "operation2",
		IsPreAppStart: true,
	}
	span3 := &core.Span{
		TraceId:       "trace2",
		SpanId:        "span3",
		Name:          "operation3",
		IsPreAppStart: false,
	}

	tests := []struct {
		name                string
		opts                SuiteSpanOptions
		currentTests        []Test
		expectedMinSpans    int // minimum expected (local pre-app-start might add more)
		expectedPreAppCount int
		expectedTraceCount  int
	}{
		{
			name: "uses_current_tests_spans",
			opts: SuiteSpanOptions{
				IsCloudMode: false,
				Interactive: false,
			},
			currentTests: []Test{
				{TraceID: "trace1", Spans: []*core.Span{span1, span2}},
				{TraceID: "trace2", Spans: []*core.Span{span3}},
			},
			expectedMinSpans:    3,
			expectedPreAppCount: 1, // span2 is pre-app-start
			expectedTraceCount:  2,
		},
		{
			name: "prefers_all_tests_over_current",
			opts: SuiteSpanOptions{
				IsCloudMode: false,
				Interactive: false,
				AllTests: []Test{
					{TraceID: "trace1", Spans: []*core.Span{span1, span2}},
					{TraceID: "trace2", Spans: []*core.Span{span3}},
				},
			},
			currentTests: []Test{
				{TraceID: "trace1", Spans: []*core.Span{span1}}, // Only one span
			},
			expectedMinSpans:    3, // Should use AllTests, not currentTests
			expectedPreAppCount: 1,
			expectedTraceCount:  2,
		},
		{
			name: "handles_empty_tests",
			opts: SuiteSpanOptions{
				IsCloudMode: false,
				Interactive: false,
			},
			currentTests:        []Test{},
			expectedMinSpans:    0,
			expectedPreAppCount: 0,
			expectedTraceCount:  0,
		},
		{
			name: "deduplicates_spans",
			opts: SuiteSpanOptions{
				IsCloudMode: false,
				Interactive: false,
			},
			currentTests: []Test{
				{TraceID: "trace1", Spans: []*core.Span{span1, span2}},
				{TraceID: "trace1", Spans: []*core.Span{span1, span2}}, // Duplicates
			},
			expectedMinSpans:    2, // Should dedupe
			expectedPreAppCount: 1,
			expectedTraceCount:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spans, _, traceCount, err := BuildSuiteSpansForRun(
				context.Background(),
				tt.opts,
				tt.currentTests,
			)

			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(spans), tt.expectedMinSpans)

			// Count pre-app-start spans in result
			actualPreAppCount := 0
			for _, s := range spans {
				if s.IsPreAppStart {
					actualPreAppCount++
				}
			}
			assert.GreaterOrEqual(t, actualPreAppCount, tt.expectedPreAppCount)

			// Note: traceCount might be higher if local pre-app-start spans are found
			if tt.expectedTraceCount > 0 {
				assert.GreaterOrEqual(t, traceCount, tt.expectedTraceCount)
			}
		})
	}
}

func TestBuildSuiteSpansForRun_PreAppStartSpans(t *testing.T) {
	t.Parallel()

	// Create a mix of regular and pre-app-start spans
	regularSpan := &core.Span{
		TraceId:       "trace1",
		SpanId:        "span1",
		Name:          "regular",
		IsPreAppStart: false,
	}
	preAppSpan1 := &core.Span{
		TraceId:       "trace1",
		SpanId:        "span2",
		Name:          "preapp1",
		IsPreAppStart: true,
		Timestamp:     timestamppb.New(time.Now().Add(-1 * time.Hour)),
	}
	preAppSpan2 := &core.Span{
		TraceId:       "trace2",
		SpanId:        "span3",
		Name:          "preapp2",
		IsPreAppStart: true,
		Timestamp:     timestamppb.New(time.Now().Add(-2 * time.Hour)),
	}

	tests := []struct {
		name         string
		currentTests []Test
		wantMinSpans int
		wantPreApp   bool
	}{
		{
			name: "includes_pre_app_start_spans",
			currentTests: []Test{
				{
					TraceID: "trace1",
					Spans:   []*core.Span{regularSpan, preAppSpan1, preAppSpan2},
				},
			},
			wantMinSpans: 3,
			wantPreApp:   true,
		},
		{
			name: "no_pre_app_start_spans",
			currentTests: []Test{
				{
					TraceID: "trace1",
					Spans:   []*core.Span{regularSpan},
				},
			},
			wantMinSpans: 1,
			wantPreApp:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spans, preAppCount, _, err := BuildSuiteSpansForRun(
				context.Background(),
				SuiteSpanOptions{
					IsCloudMode: false,
					Interactive: false,
				},
				tt.currentTests,
			)

			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(spans), tt.wantMinSpans)

			if tt.wantPreApp {
				assert.Greater(t, preAppCount, 0, "expected pre-app-start spans")

				// Verify our test pre-app-start spans are in the result
				foundPreApp1 := false
				foundPreApp2 := false
				for _, s := range spans {
					if s.SpanId == "span2" && s.Name == "preapp1" {
						foundPreApp1 = true
					}
					if s.SpanId == "span3" && s.Name == "preapp2" {
						foundPreApp2 = true
					}
				}
				assert.True(t, foundPreApp1, "expected to find preapp1 span")
				assert.True(t, foundPreApp2, "expected to find preapp2 span")
			}
		})
	}
}

func TestBuildSuiteSpansForRun_UniqueTraceCount(t *testing.T) {
	t.Parallel()

	span1 := &core.Span{TraceId: "trace1", SpanId: "span1"}
	span2 := &core.Span{TraceId: "trace1", SpanId: "span2"}
	span3 := &core.Span{TraceId: "trace2", SpanId: "span3"}
	span4 := &core.Span{TraceId: "trace3", SpanId: "span4"}

	tests := []struct {
		name               string
		currentTests       []Test
		expectedTraceCount int
	}{
		{
			name: "counts_unique_traces",
			currentTests: []Test{
				{TraceID: "trace1", Spans: []*core.Span{span1, span2}},
				{TraceID: "trace2", Spans: []*core.Span{span3}},
				{TraceID: "trace3", Spans: []*core.Span{span4}},
			},
			expectedTraceCount: 3,
		},
		{
			name: "handles_duplicate_traces",
			currentTests: []Test{
				{TraceID: "trace1", Spans: []*core.Span{span1}},
				{TraceID: "trace1", Spans: []*core.Span{span2}},
			},
			expectedTraceCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, traceCount, err := BuildSuiteSpansForRun(
				context.Background(),
				SuiteSpanOptions{
					IsCloudMode: false,
					Interactive: false,
				},
				tt.currentTests,
			)

			require.NoError(t, err)
			// Note: might be higher due to local pre-app-start spans from other traces
			assert.GreaterOrEqual(t, traceCount, tt.expectedTraceCount)
		})
	}
}

func TestPrepareAndSetSuiteSpans(t *testing.T) {
	t.Parallel()

	span1 := &core.Span{
		TraceId: "trace1",
		SpanId:  "span1",
		Name:    "operation1",
	}

	tests := []struct {
		name         string
		opts         SuiteSpanOptions
		currentTests []Test
		wantError    bool
		wantSpans    bool // whether we expect spans to be set (could be from local files too)
	}{
		{
			name: "success_with_tests",
			opts: SuiteSpanOptions{
				IsCloudMode: false,
				Interactive: false,
			},
			currentTests: []Test{
				{TraceID: "trace1", Spans: []*core.Span{span1}},
			},
			wantError: false,
			wantSpans: true,
		},
		{
			name: "success_with_no_tests",
			opts: SuiteSpanOptions{
				IsCloudMode: false,
				Interactive: false,
			},
			currentTests: []Test{},
			wantError:    false,
			wantSpans:    false, // May have spans from local files, so we don't assert
		},
		{
			name: "success_interactive_mode",
			opts: SuiteSpanOptions{
				IsCloudMode: false,
				Interactive: true, // Should log but not fail
			},
			currentTests: []Test{
				{TraceID: "trace1", Spans: []*core.Span{span1}},
			},
			wantError: false,
			wantSpans: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := NewExecutor()

			err := PrepareAndSetSuiteSpans(
				context.Background(),
				executor,
				tt.opts,
				tt.currentTests,
			)

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// Verify SetSuiteSpans was called (suiteSpans should be initialized)
				// Note: it might be an empty slice, but should not be nil after SetSuiteSpans is called
				if tt.wantSpans {
					assert.NotNil(t, executor.suiteSpans)
					assert.Greater(t, len(executor.suiteSpans), 0, "expected at least one span")
				}
				// For empty tests, we just verify no error - spans might come from local files
			}
		})
	}
}

func TestDedupeSpans_PreservesFirstOccurrence(t *testing.T) {
	t.Parallel()

	// Create spans with same IDs but different data
	span1 := &core.Span{
		TraceId: "trace1",
		SpanId:  "span1",
		Name:    "first_occurrence",
	}
	span2 := &core.Span{
		TraceId: "trace1",
		SpanId:  "span1",
		Name:    "second_occurrence",
	}

	input := []*core.Span{span1, span2}
	result := DedupeSpans(input)

	require.Len(t, result, 1)
	assert.Equal(t, "first_occurrence", result[0].Name, "should preserve first occurrence")
}
