package runner

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/Use-Tusk/tusk-drift-cli/internal/api"
	"github.com/Use-Tusk/tusk-drift-cli/internal/log"
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
	TraceTestID string

	// Local options
	Interactive bool // Whether to log errors interactively
	Quiet       bool // Whether to suppress progress messages (only works with --print)

	// AllowSuiteWideMatching allows matching against all suite spans (for main branch validation or local runs)
	// When false (normal cloud replay), only global spans are loaded for cross-trace matching
	AllowSuiteWideMatching bool

	// PreloadedPreAppStartSpans allows passing pre-fetched pre-app-start spans to avoid fetching again
	PreloadedPreAppStartSpans []*core.Span

	// PreloadedGlobalSpans allows passing pre-fetched global spans to avoid fetching again
	PreloadedGlobalSpans []*core.Span
}

// BuildSuiteSpansResult contains the result of building suite spans
type BuildSuiteSpansResult struct {
	SuiteSpans       []*core.Span
	GlobalSpans      []*core.Span // Only populated in non-validation mode
	PreAppStartCount int
	UniqueTraceCount int
}

// BuildSuiteSpansForRun builds the suite spans for the run.
// Returns the suite spans, global spans (when not suite-wide matching), pre-app-start count, and unique trace count.
func BuildSuiteSpansForRun(
	ctx context.Context,
	opts SuiteSpanOptions,
	currentTests []Test,
) (*BuildSuiteSpansResult, error) {
	var suiteSpans []*core.Span
	var globalSpans []*core.Span

	// Fetch global spans (use preloaded if available)
	if opts.IsCloudMode && opts.Client != nil {
		var global []*core.Span
		if len(opts.PreloadedGlobalSpans) > 0 {
			// Use preloaded spans if available
			global = opts.PreloadedGlobalSpans
		} else {
			// Fetch from cloud with cache
			var err error
			global, err = FetchGlobalSpansFromCloudWithCache(ctx, opts.Client, opts.AuthOptions, opts.ServiceID)
			if err != nil {
				log.Warn("Failed to fetch global spans", "error", err)
			}
		}

		if opts.AllowSuiteWideMatching {
			// Validation mode: add global spans directly to suite spans for matching
			suiteSpans = append(suiteSpans, global...)
		} else {
			// Normal replay mode: keep global spans separate for dedicated index
			globalSpans = global
		}
	}

	// Add spans from current tests
	for _, t := range currentTests {
		suiteSpans = append(suiteSpans, t.Spans...)
	}

	// Pre-app-start spans are always included (both modes)
	// Prepend these spans so they get considered first
	if opts.IsCloudMode && opts.Client != nil {
		var preAppStartSpans []*core.Span
		if len(opts.PreloadedPreAppStartSpans) > 0 {
			// Use preloaded spans if available
			preAppStartSpans = opts.PreloadedPreAppStartSpans
		} else {
			// Fetch from cloud with cache
			var err error
			preAppStartSpans, err = FetchPreAppStartSpansFromCloudWithCache(ctx, opts.Client, opts.AuthOptions, opts.ServiceID)
			if err != nil {
				log.Warn("Failed to fetch pre-app-start spans", "error", err)
			}
		}
		if len(preAppStartSpans) > 0 {
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

	return &BuildSuiteSpansResult{
		SuiteSpans:       suiteSpans,
		GlobalSpans:      globalSpans,
		PreAppStartCount: preAppCount,
		UniqueTraceCount: len(uniq),
	}, nil
}

// PrepareAndSetSuiteSpans is a convenience function that builds suite spans and sets them on the executor
func PrepareAndSetSuiteSpans(
	ctx context.Context,
	exec *Executor,
	opts SuiteSpanOptions,
	currentTests []Test,
) error {
	result, err := BuildSuiteSpansForRun(ctx, opts, currentTests)
	if err != nil {
		return err
	}
	if opts.Interactive {
		log.ServiceLog(fmt.Sprintf(
			"Loading %d suite spans for matching (%d unique traces, %d pre-app-start)",
			len(result.SuiteSpans), result.UniqueTraceCount, result.PreAppStartCount,
		))
	} else if !opts.Quiet {
		log.UserProgress(fmt.Sprintf("  ↳ Loaded %d suite spans (%d unique traces, %d pre-app-start)", len(result.SuiteSpans), result.UniqueTraceCount, result.PreAppStartCount))
	}
	log.Debug("Prepared suite spans for matching",
		"count", len(result.SuiteSpans),
		"uniqueTraces", result.UniqueTraceCount,
		"preAppSpans", result.PreAppStartCount,
		"globalSpans", len(result.GlobalSpans),
		"interactive", opts.Interactive,
		"traceTestID", opts.TraceTestID,
	)
	exec.SetSuiteSpans(result.SuiteSpans)
	// Set global spans separately for dedicated index (used in regular replay mode)
	if len(result.GlobalSpans) > 0 {
		exec.SetGlobalSpans(result.GlobalSpans)
	}

	// Enable suite-wide matching when:
	// - Explicitly requested (validation mode or other use cases)
	// - Local (non-cloud) runs since there are no explicit global spans
	if opts.AllowSuiteWideMatching || !opts.IsCloudMode {
		exec.SetAllowSuiteWideMatching(true)
	}
	return nil
}

// FetchPreAppStartSpansFromCloud fetches pre-app-start spans from the cloud backend
func FetchPreAppStartSpansFromCloud(
	ctx context.Context,
	client *api.TuskClient,
	auth api.AuthOptions,
	serviceID string,
	interactive bool,
	quiet bool,
) ([]*core.Span, error) {
	tracker := utils.NewProgressTracker("Fetching pre-app-start spans", interactive, quiet)
	defer tracker.Stop()

	var all []*core.Span
	cur := ""
	for {
		req := &backend.GetPreAppStartSpansRequest{
			ObservableServiceId: serviceID,
			PageSize:            50,
		}
		if cur != "" {
			req.PaginationCursor = &cur
		}

		resp, err := client.GetPreAppStartSpans(ctx, req, auth)
		if err != nil {
			return nil, fmt.Errorf("get pre-app-start spans: %w", err)
		}

		if cur == "" && resp.TotalCount > 0 {
			tracker.SetTotal(int(resp.TotalCount))
		}

		all = append(all, resp.Spans...)
		tracker.Update(len(all))

		if next := resp.GetNextCursor(); next != "" {
			cur = next
			continue
		}
		break
	}

	if len(all) > 0 {
		tracker.Finish(fmt.Sprintf("✓ Loaded %d pre-app-start spans", len(all)))
	}

	return all, nil
}

// FetchPreAppStartSpansFromCloudWithCache fetches pre-app-start spans using cache.
// It only fetches new spans and removes deleted ones from cache.
func FetchPreAppStartSpansFromCloudWithCache(
	ctx context.Context,
	client *api.TuskClient,
	auth api.AuthOptions,
	serviceID string,
) ([]*core.Span, error) {
	return api.FetchPreAppStartSpansWithCache(ctx, client, auth, serviceID)
}

// FetchGlobalSpansFromCloudWithCache fetches global spans using cache.
// It only fetches new spans and removes deleted ones from cache.
func FetchGlobalSpansFromCloudWithCache(
	ctx context.Context,
	client *api.TuskClient,
	auth api.AuthOptions,
	serviceID string,
) ([]*core.Span, error) {
	return api.FetchGlobalSpansWithCache(ctx, client, auth, serviceID)
}

// FetchGlobalSpansFromCloud fetches only spans marked as global (is_global=true) from cloud
func FetchGlobalSpansFromCloud(
	ctx context.Context,
	client *api.TuskClient,
	auth api.AuthOptions,
	serviceID string,
	interactive bool,
	quiet bool,
) ([]*core.Span, error) {
	tracker := utils.NewProgressTracker("Fetching global spans", interactive, quiet)
	defer tracker.Stop()

	var all []*core.Span
	cur := ""
	for {
		req := &backend.GetGlobalSpansRequest{
			ObservableServiceId: serviceID,
			PageSize:            50,
		}
		if cur != "" {
			req.PaginationCursor = &cur
		}

		resp, err := client.GetGlobalSpans(ctx, req, auth)
		if err != nil {
			return nil, fmt.Errorf("get global spans: %w", err)
		}

		if cur == "" && resp.TotalCount > 0 {
			tracker.SetTotal(int(resp.TotalCount))
		}

		all = append(all, resp.Spans...)
		tracker.Update(len(all))

		if next := resp.GetNextCursor(); next != "" {
			cur = next
			continue
		}
		break
	}

	if len(all) > 0 {
		tracker.Finish(fmt.Sprintf("✓ Loaded %d global spans", len(all)))
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
					log.ServiceLog(fmt.Sprintf("❌ Failed to parse spans from %s: %v", f, err))
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

	log.Debug("Deduplicated suite spans", "inCount", len(spans), "outCount", len(out))
	return out
}
