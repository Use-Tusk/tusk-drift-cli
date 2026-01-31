package api

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/Use-Tusk/tusk-drift-cli/internal/cache"
	"github.com/Use-Tusk/tusk-drift-cli/internal/log"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
)

var (
	ErrFetchTraceTests          = errors.New("unable to fetch tests from Tusk Cloud")
	ErrFetchTraceTestIDs        = errors.New("unable to fetch test IDs from Tusk Cloud and no cache available")
	ErrFetchNewTraceTests       = errors.New("unable to fetch new tests from Tusk Cloud")
	ErrFetchPreAppStartSpanIDs  = errors.New("unable to fetch pre-app-start span IDs from Tusk Cloud and no cache available")
	ErrFetchNewPreAppStartSpans = errors.New("unable to fetch new pre-app-start spans from Tusk Cloud")
	ErrFetchPreAppStartSpans    = errors.New("unable to fetch pre-app-start spans from Tusk Cloud")
	ErrFetchGlobalSpanIDs       = errors.New("unable to fetch global span IDs from Tusk Cloud and no cache available")
	ErrFetchNewGlobalSpans      = errors.New("unable to fetch new global spans from Tusk Cloud")
	ErrFetchGlobalSpans         = errors.New("unable to fetch global spans from Tusk Cloud")
)

// FetchAllTraceTestsOptions configures the fetch behavior
type FetchAllTraceTestsOptions struct {
	// Message to show in the progress bar
	Message string
	// PageSize for pagination (default 25)
	PageSize int32
}

// FetchAllTraceTests fetches all trace tests from the cloud with a progress bar.
// This is the shared implementation used by both `tusk list -c` and `tusk run -c`.
func FetchAllTraceTests(
	ctx context.Context,
	client *TuskClient,
	auth AuthOptions,
	serviceID string,
	opts *FetchAllTraceTestsOptions,
) ([]*backend.TraceTest, error) {
	if opts == nil {
		opts = &FetchAllTraceTestsOptions{}
	}
	if opts.Message == "" {
		opts.Message = "Fetching traces from Tusk Drift Cloud"
	}
	if opts.PageSize == 0 {
		opts.PageSize = 25
	}

	tracker := utils.NewProgressTracker(opts.Message, false, false)

	var (
		all      []*backend.TraceTest
		cursor   string
		totalSet bool
	)

	for {
		req := &backend.GetAllTraceTestsRequest{
			ObservableServiceId: serviceID,
			PageSize:            opts.PageSize,
		}
		if cursor != "" {
			req.PaginationCursor = &cursor
		}

		resp, err := client.GetAllTraceTests(ctx, req, auth)
		if err != nil {
			tracker.Stop()
			return nil, fmt.Errorf("%w: %w", ErrFetchTraceTests, err)
		}

		all = append(all, resp.TraceTests...)

		if !totalSet && resp.TotalCount > 0 {
			tracker.SetTotal(int(resp.TotalCount))
			totalSet = true
		}

		tracker.Update(len(all))

		if next := resp.GetNextCursor(); next != "" {
			cursor = next
			continue
		}
		break
	}

	tracker.Finish("")
	return all, nil
}

// FetchDriftRunTraceTests fetches trace tests for a specific drift run with a progress bar.
func FetchDriftRunTraceTests(
	ctx context.Context,
	client *TuskClient,
	auth AuthOptions,
	driftRunID string,
	opts *FetchAllTraceTestsOptions,
) ([]*backend.TraceTest, error) {
	if opts == nil {
		opts = &FetchAllTraceTestsOptions{}
	}
	if opts.Message == "" {
		opts.Message = "Fetching tests from Tusk Drift Cloud"
	}
	if opts.PageSize == 0 {
		opts.PageSize = 25
	}

	tracker := utils.NewProgressTracker(opts.Message, false, false)

	var (
		all      []*backend.TraceTest
		cursor   string
		totalSet bool
	)

	for {
		req := &backend.GetDriftRunTraceTestsRequest{
			DriftRunId: driftRunID,
			PageSize:   opts.PageSize,
		}
		if cursor != "" {
			req.PaginationCursor = &cursor
		}

		resp, err := client.GetDriftRunTraceTests(ctx, req, auth)
		if err != nil {
			tracker.Stop()
			return nil, fmt.Errorf("%w: %w", ErrFetchTraceTests, err)
		}

		all = append(all, resp.TraceTests...)

		if !totalSet && resp.TotalCount > 0 {
			tracker.SetTotal(int(resp.TotalCount))
			totalSet = true
		}

		tracker.Update(len(all))

		if next := resp.GetNextCursor(); next != "" {
			cursor = next
			continue
		}
		break
	}

	tracker.Finish("")
	return all, nil
}

// FetchAllTraceTestsWithCache fetches trace tests using ID-based cache diffing.
// It only fetches new traces and removes deleted ones from cache.
// On network error, it falls back to cached data if available.
func FetchAllTraceTestsWithCache(
	ctx context.Context,
	client *TuskClient,
	auth AuthOptions,
	serviceID string,
	interactive bool,
	quiet bool,
) ([]*backend.TraceTest, error) {
	traceCache, err := cache.NewTraceCache(serviceID)
	if err != nil {
		return FetchAllTraceTests(ctx, client, auth, serviceID, nil)
	}

	idsResp, err := client.GetAllTraceTestIds(ctx, &backend.GetAllTraceTestIdsRequest{
		ObservableServiceId: serviceID,
	}, auth)
	if err != nil {
		cached, cacheErr := traceCache.LoadAllTraces()
		if cacheErr != nil || len(cached) == 0 {
			return nil, fmt.Errorf("%w: %w", ErrFetchTraceTestIDs, err)
		}
		log.Warn("Using cached data due to network error", "error", err)
		return cached, nil
	}
	remoteIds := idsResp.TraceTestIds

	cachedIds, err := traceCache.GetCachedIds()
	if err != nil {
		return FetchAllTraceTests(ctx, client, auth, serviceID, nil)
	}

	toFetch, toDelete := cache.DiffIds(remoteIds, cachedIds)
	if len(toDelete) > 0 {
		if err := traceCache.DeleteTraces(toDelete); err != nil {
			// Non-fatal, continue
			fmt.Printf("Warning: failed to delete some cached traces: %v\n", err)
		}
	}

	if len(toFetch) > 0 {
		const chunkSize = 20
		tracker := utils.NewProgressTracker("Fetching new traces from Tusk Drift Cloud", false, false)
		tracker.SetTotal(len(toFetch))

		for i := 0; i < len(toFetch); i += chunkSize {
			end := i + chunkSize
			if end > len(toFetch) {
				end = len(toFetch)
			}
			chunk := toFetch[i:end]

			newTraces, err := client.GetTraceTestsByIds(ctx, &backend.GetTraceTestsByIdsRequest{
				ObservableServiceId: serviceID,
				TraceTestIds:        chunk,
			}, auth)
			if err != nil {
				tracker.Stop()
				return nil, fmt.Errorf("%w: %w", ErrFetchNewTraceTests, err)
			}

			if err := traceCache.SaveTraces(newTraces.TraceTests); err != nil {
				// Non-fatal, traces are still usable
				log.Warn("Failed to save some traces to cache", "error", err)
			}
			tracker.Update(end)
		}
		tracker.Finish(fmt.Sprintf("✓ Fetched %d new traces", len(toFetch)))
	}

	all, err := traceCache.LoadAllTraces()
	if err != nil {
		return nil, err
	}

	if len(toFetch) == 0 && len(all) > 0 && !quiet {
		if interactive {
			log.ServiceLog(fmt.Sprintf("✓ Using %d cached traces", len(all)))
		} else {
			fmt.Fprintf(os.Stderr, "✓ Using %d cached traces\n", len(all))
		}
	}

	return all, nil
}

// FetchPreAppStartSpansWithCache fetches pre-app-start spans using ID-based cache diffing.
// It only fetches new spans and removes deleted ones from cache.
// On network error, it falls back to cached data if available.
func FetchPreAppStartSpansWithCache(
	ctx context.Context,
	client *TuskClient,
	auth AuthOptions,
	serviceID string,
	interactive bool,
	quiet bool,
) ([]*core.Span, error) {
	spanCache, err := cache.NewSpanCache(serviceID, cache.SpanTypePreAppStart)
	if err != nil {
		return FetchAllPreAppStartSpans(ctx, client, auth, serviceID)
	}

	tracker := utils.NewProgressTracker("Syncing pre-app-start spans", interactive, quiet)
	idsResp, err := client.GetAllPreAppStartSpanIds(ctx, &backend.GetAllPreAppStartSpanIdsRequest{
		ObservableServiceId: serviceID,
	}, auth)
	if err != nil {
		tracker.Stop()
		cached, cacheErr := spanCache.LoadAllSpans()
		if cacheErr != nil || len(cached) == 0 {
			return nil, fmt.Errorf("%w: %w", ErrFetchPreAppStartSpanIDs, err)
		}
		fmt.Fprintf(os.Stderr, "⚠ Using %d cached pre-app-start spans (network error: %v)\n", len(cached), err)
		return cached, nil
	}
	remoteIds := idsResp.SpanIds

	cachedIds, err := spanCache.GetCachedIds()
	if err != nil {
		tracker.Stop()
		return FetchAllPreAppStartSpans(ctx, client, auth, serviceID)
	}

	toFetch, toDelete := cache.DiffIds(remoteIds, cachedIds)

	if len(toDelete) > 0 {
		if err := spanCache.DeleteSpans(toDelete); err != nil {
			// Non-fatal, continue
			fmt.Fprintf(os.Stderr, "Warning: failed to delete some cached spans: %v\n", err)
		}
	}

	if len(toFetch) > 0 {
		tracker.SetTotal(len(toFetch))
		tracker.Update(0)

		const chunkSize = 20
		for i := 0; i < len(toFetch); i += chunkSize {
			end := i + chunkSize
			if end > len(toFetch) {
				end = len(toFetch)
			}
			chunk := toFetch[i:end]

			newSpans, err := client.GetPreAppStartSpansByIds(ctx, &backend.GetPreAppStartSpansByIdsRequest{
				ObservableServiceId: serviceID,
				SpanIds:             chunk,
			}, auth)
			if err != nil {
				tracker.Stop()
				return nil, fmt.Errorf("%w: %w", ErrFetchNewPreAppStartSpans, err)
			}

			if err := spanCache.SaveSpans(newSpans.Spans); err != nil {
				// Non-fatal, spans are still usable
				fmt.Fprintf(os.Stderr, "\nWarning: failed to save some spans to cache: %v\n", err)
			}
			tracker.Update(end)
		}
		tracker.Finish(fmt.Sprintf("✓ Fetched %d new pre-app-start spans", len(toFetch)))
	} else if len(remoteIds) > 0 {
		tracker.Finish(fmt.Sprintf("✓ Using %d cached pre-app-start spans", len(remoteIds)))
	} else {
		tracker.Finish("✓ No pre-app-start spans configured")
	}

	// 6. Load all from cache for use
	return spanCache.LoadAllSpans()
}

// FetchAllPreAppStartSpans fetches all pre-app-start spans without caching (fallback).
func FetchAllPreAppStartSpans(
	ctx context.Context,
	client *TuskClient,
	auth AuthOptions,
	serviceID string,
) ([]*core.Span, error) {
	var all []*core.Span
	var cursor string

	for {
		req := &backend.GetPreAppStartSpansRequest{
			ObservableServiceId: serviceID,
			PageSize:            200,
		}
		if cursor != "" {
			req.PaginationCursor = &cursor
		}

		resp, err := client.GetPreAppStartSpans(ctx, req, auth)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrFetchPreAppStartSpans, err)
		}

		all = append(all, resp.Spans...)

		if next := resp.GetNextCursor(); next != "" {
			cursor = next
			continue
		}
		break
	}

	return all, nil
}

// FetchGlobalSpansWithCache fetches global spans using ID-based cache diffing.
// It only fetches new spans and removes deleted ones from cache.
// On network error, it falls back to cached data if available.
func FetchGlobalSpansWithCache(
	ctx context.Context,
	client *TuskClient,
	auth AuthOptions,
	serviceID string,
	interactive bool,
	quiet bool,
) ([]*core.Span, error) {
	spanCache, err := cache.NewSpanCache(serviceID, cache.SpanTypeGlobal)
	if err != nil {
		return FetchAllGlobalSpans(ctx, client, auth, serviceID)
	}

	tracker := utils.NewProgressTracker("Syncing global spans", interactive, quiet)
	idsResp, err := client.GetAllGlobalSpanIds(ctx, &backend.GetAllGlobalSpanIdsRequest{
		ObservableServiceId: serviceID,
	}, auth)
	if err != nil {
		tracker.Stop()
		cached, cacheErr := spanCache.LoadAllSpans()
		if cacheErr != nil || len(cached) == 0 {
			return nil, fmt.Errorf("%w: %w", ErrFetchGlobalSpanIDs, err)
		}
		fmt.Fprintf(os.Stderr, "⚠ Using %d cached global spans (network error: %v)\n", len(cached), err)
		return cached, nil
	}
	remoteIds := idsResp.SpanIds

	cachedIds, err := spanCache.GetCachedIds()
	if err != nil {
		tracker.Stop()
		return FetchAllGlobalSpans(ctx, client, auth, serviceID)
	}

	toFetch, toDelete := cache.DiffIds(remoteIds, cachedIds)

	if len(toDelete) > 0 {
		if err := spanCache.DeleteSpans(toDelete); err != nil {
			// Non-fatal, continue
			fmt.Fprintf(os.Stderr, "Warning: failed to delete some cached global spans: %v\n", err)
		}
	}

	if len(toFetch) > 0 {
		tracker.SetTotal(len(toFetch))
		tracker.Update(0)

		const chunkSize = 20
		for i := 0; i < len(toFetch); i += chunkSize {
			end := i + chunkSize
			if end > len(toFetch) {
				end = len(toFetch)
			}
			chunk := toFetch[i:end]

			newSpans, err := client.GetGlobalSpansByIds(ctx, &backend.GetGlobalSpansByIdsRequest{
				ObservableServiceId: serviceID,
				SpanIds:             chunk,
			}, auth)
			if err != nil {
				tracker.Stop()
				return nil, fmt.Errorf("%w: %w", ErrFetchNewGlobalSpans, err)
			}

			if err := spanCache.SaveSpans(newSpans.Spans); err != nil {
				// Non-fatal, spans are still usable
				fmt.Fprintf(os.Stderr, "\nWarning: failed to save some global spans to cache: %v\n", err)
			}
			tracker.Update(end)
		}
		tracker.Finish(fmt.Sprintf("✓ Fetched %d new global spans", len(toFetch)))
	} else if len(remoteIds) > 0 {
		tracker.Finish(fmt.Sprintf("✓ Using %d cached global spans", len(remoteIds)))
	} else {
		tracker.Finish("✓ No global spans configured")
	}

	return spanCache.LoadAllSpans()
}

// FetchAllGlobalSpans fetches all global spans without caching (fallback).
func FetchAllGlobalSpans(
	ctx context.Context,
	client *TuskClient,
	auth AuthOptions,
	serviceID string,
) ([]*core.Span, error) {
	var all []*core.Span
	var cursor string

	for {
		req := &backend.GetGlobalSpansRequest{
			ObservableServiceId: serviceID,
			PageSize:            200,
		}
		if cursor != "" {
			req.PaginationCursor = &cursor
		}

		resp, err := client.GetGlobalSpans(ctx, req, auth)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrFetchGlobalSpans, err)
		}

		all = append(all, resp.Spans...)

		if next := resp.GetNextCursor(); next != "" {
			cursor = next
			continue
		}
		break
	}

	return all, nil
}
