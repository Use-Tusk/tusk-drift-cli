package api

import (
	"context"
	"fmt"

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
