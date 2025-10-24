package runner

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/Use-Tusk/tusk-drift-cli/internal/api"
	"github.com/Use-Tusk/tusk-drift-cli/internal/logging"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
)

// SuiteSpanOptions contains options for building suite spans
type SuiteSpanOptions struct {
	// Cloud options
	IsCloudMode bool
	Client      *api.TuskClient
	AuthOptions api.AuthOptions
	ServiceID   string
	TraceTestID string // If set, fetch all suite spans for cross-suite matching

	// Local options
	AllTests    []Test // All tests loaded (for extracting spans)
	Interactive bool   // Whether to log errors interactively
}

// BuildSuiteSpansForRun builds the suite spans for the run.
// If running a single cloud trace test, eager-fetch all suite spans to enable cross-suite matching.
// Returns the suite spans, the number of pre-app-start spans, and the number of unique traces.
func BuildSuiteSpansForRun(
	ctx context.Context,
	opts SuiteSpanOptions,
	currentTests []Test,
) ([]*core.Span, int, int, error) {
	var suiteSpans []*core.Span

	// If running a single cloud trace test, fetch all suite spans
	if opts.IsCloudMode && opts.Client != nil && opts.TraceTestID != "" {
		all, err := fetchAllSuiteSpans(ctx, opts.Client, opts.AuthOptions, opts.ServiceID)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("fetch all suite spans: %w", err)
		}
		if len(all) > 0 {
			suiteSpans = append(suiteSpans, all...)
		}
	}

	// Fallback: use spans from the loaded tests
	if len(suiteSpans) == 0 {
		// Prefer all tests if available (for list view with all tests loaded)
		testsToUse := currentTests
		if len(opts.AllTests) > 0 {
			testsToUse = opts.AllTests
		}

		for _, t := range testsToUse {
			if len(t.Spans) > 0 {
				suiteSpans = append(suiteSpans, t.Spans...)
			}
		}
	}

	// Layer on pre-app-start spans if available
	// Prepend these spans so they get considered first
	if opts.IsCloudMode && opts.Client != nil {
		preAppStartSpans, err := FetchPreAppStartSpansFromCloud(ctx, opts.Client, opts.AuthOptions, opts.ServiceID)
		if err == nil && len(preAppStartSpans) > 0 {
			suiteSpans = append(preAppStartSpans, suiteSpans...)
		}
	} else {
		if localPreAppStartSpans, err := FetchLocalPreAppStartSpans(opts.Interactive); err == nil && len(localPreAppStartSpans) > 0 {
			suiteSpans = append(localPreAppStartSpans, suiteSpans...)
		}
	}

	suiteSpans = DedupeSpans(suiteSpans)

	preAppCount := 0
	uniq := make(map[string]struct{})
	for _, s := range suiteSpans {
		if s == nil {
			continue
		}
		if s.IsPreAppStart {
			preAppCount++
		}
		if s.TraceId != "" {
			uniq[s.TraceId] = struct{}{}
		}
	}

	return suiteSpans, preAppCount, len(uniq), nil
}

// PrepareAndSetSuiteSpans is a convenience function that builds suite spans and sets them on the executor
func PrepareAndSetSuiteSpans(
	ctx context.Context,
	exec *Executor,
	opts SuiteSpanOptions,
	currentTests []Test,
) error {
	suiteSpans, preAppCount, uniqueTraceCount, err := BuildSuiteSpansForRun(ctx, opts, currentTests)
	if opts.Interactive {
		logging.LogToService(fmt.Sprintf(
			"Loading %d suite spans for matching (%d unique traces, %d pre-app-start)",
			len(suiteSpans), uniqueTraceCount, preAppCount,
		))
	} else {
		fmt.Fprintf(os.Stderr, "  ↳ Loaded %d suite spans (%d unique traces, %d pre-app-start)\n", len(suiteSpans), uniqueTraceCount, preAppCount)
	}
	slog.Debug("Prepared suite spans for matching",
		"count", len(suiteSpans),
		"uniqueTraces", uniqueTraceCount,
		"preAppSpans", preAppCount,
		"interactive", opts.Interactive,
		"traceTestID", opts.TraceTestID,
	)
	exec.SetSuiteSpans(suiteSpans)
	return err
}

// FetchPreAppStartSpansFromCloud fetches pre-app-start spans from the cloud backend
func FetchPreAppStartSpansFromCloud(
	ctx context.Context,
	client *api.TuskClient,
	auth api.AuthOptions,
	serviceID string,
) ([]*core.Span, error) {
	var all []*core.Span
	cur := ""
	for {
		req := &backend.GetPreAppStartSpansRequest{
			ObservableServiceId: serviceID,
			PageSize:            200,
		}
		if cur != "" {
			req.PaginationCursor = &cur
		}

		resp, err := client.GetPreAppStartSpans(ctx, req, auth)
		if err != nil {
			return nil, fmt.Errorf("get pre-app-start spans: %w", err)
		}
		all = append(all, resp.Spans...)
		if next := resp.GetNextCursor(); next != "" {
			cur = next
			continue
		}
		break
	}
	return all, nil
}

// FetchLocalPreAppStartSpans fetches pre-app-start spans from local trace files
func FetchLocalPreAppStartSpans(interactive bool) ([]*core.Span, error) {
	var out []*core.Span
	seen := map[string]struct{}{}

	for _, dir := range utils.GetPossibleTraceDirs() {
		matches, err := filepath.Glob(filepath.Join(dir, "*trace*.jsonl"))
		if err != nil {
			continue
		}
		for _, f := range matches {
			spans, err := utils.ParseSpansFromFile(f, func(s *core.Span) bool { return s.IsPreAppStart })
			if err != nil {
				if interactive {
					logging.LogToService(fmt.Sprintf("❌ Failed to parse spans from %s: %v", f, err))
				}
				continue
			}
			for _, s := range spans {
				key := s.TraceId + "|" + s.SpanId
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				out = append(out, s)
			}
		}
	}
	return out, nil
}

// fetchAllSuiteSpans fetches all suite spans from cloud (used for single trace test runs)
func fetchAllSuiteSpans(
	ctx context.Context,
	client *api.TuskClient,
	auth api.AuthOptions,
	serviceID string,
) ([]*core.Span, error) {
	var spans []*core.Span
	cur := ""
	for {
		req := &backend.GetAllTraceTestsRequest{
			ObservableServiceId: serviceID,
			PageSize:            100,
		}
		if cur != "" {
			req.PaginationCursor = &cur
		}
		resp, err := client.GetAllTraceTests(ctx, req, auth)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch trace tests from backend: %w", err)
		}
		for _, tt := range resp.TraceTests {
			if len(tt.Spans) > 0 {
				spans = append(spans, tt.Spans...)
			}
		}
		if next := resp.GetNextCursor(); next != "" {
			cur = next
			continue
		}
		break
	}
	return spans, nil
}

// DedupeSpans deduplicates spans by (trace_id, span_id) while preserving order
func DedupeSpans(spans []*core.Span) []*core.Span {
	if len(spans) <= 1 {
		return spans
	}
	seen := make(map[string]struct{}, len(spans))
	out := make([]*core.Span, 0, len(spans))

	for _, s := range spans {
		if s == nil {
			continue
		}
		if s.TraceId != "" && s.SpanId != "" {
			key := s.TraceId + "|" + s.SpanId
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
		}
		out = append(out, s)
	}

	slog.Debug("Deduplicated suite spans", "inCount", len(spans), "outCount", len(out))
	return out
}
