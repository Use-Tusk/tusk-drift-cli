package api

import (
	"context"
	"fmt"

	"github.com/Use-Tusk/tusk-drift-cli/internal/cache"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
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
			return nil, fmt.Errorf("failed to fetch trace tests from backend: %w", err)
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
			return nil, fmt.Errorf("failed to fetch trace tests from backend: %w", err)
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
) ([]*backend.TraceTest, error) {
	traceCache, err := cache.NewTraceCache(serviceID)
	if err != nil {
		// Cache init failed, fall back to full fetch
		return FetchAllTraceTests(ctx, client, auth, serviceID, nil)
	}

	// 1. Fetch all IDs from API
	tracker := utils.NewProgressTracker("Syncing traces from Tusk Drift Cloud", false, false)
	idsResp, err := client.GetAllTraceTestIds(ctx, &backend.GetAllTraceTestIdsRequest{
		ObservableServiceId: serviceID,
	}, auth)
	if err != nil {
		tracker.Stop()
		// Network error: try loading from cache
		cached, cacheErr := traceCache.LoadAllTraces()
		if cacheErr != nil || len(cached) == 0 {
			return nil, fmt.Errorf("failed to fetch trace test IDs and no cache available: %w", err)
		}
		fmt.Printf("Warning: Using cached data due to network error: %v\n", err)
		return cached, nil
	}
	remoteIds := idsResp.TraceTestIds

	// 2. Get cached IDs
	cachedIds, err := traceCache.GetCachedIds()
	if err != nil {
		tracker.Stop()
		// Cache read failed, fall back to full fetch
		return FetchAllTraceTests(ctx, client, auth, serviceID, nil)
	}

	// 3. Compute diff
	toFetch, toDelete := cache.DiffIds(remoteIds, cachedIds)

	// 4. Delete removed traces
	if len(toDelete) > 0 {
		if err := traceCache.DeleteTraces(toDelete); err != nil {
			// Non-fatal, continue
			fmt.Printf("Warning: failed to delete some cached traces: %v\n", err)
		}
	}

	// 5. Batch fetch new traces and save
	if len(toFetch) > 0 {
		tracker.SetTotal(len(toFetch))
		tracker.Update(0)

		newTraces, err := client.GetTraceTestsByIds(ctx, &backend.GetTraceTestsByIdsRequest{
			ObservableServiceId: serviceID,
			TraceTestIds:        toFetch,
		}, auth)
		if err != nil {
			tracker.Stop()
			return nil, fmt.Errorf("failed to fetch new trace tests: %w", err)
		}

		if err := traceCache.SaveTraces(newTraces.TraceTests); err != nil {
			// Non-fatal, traces are still usable
			fmt.Printf("Warning: failed to save some traces to cache: %v\n", err)
		}
		tracker.Update(len(toFetch))
	}

	tracker.Finish("")

	// 6. Load all from cache for display
	return traceCache.LoadAllTraces()
}
