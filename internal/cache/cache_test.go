package cache

import (
	"os"
	"path/filepath"
	"testing"

	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
)

func TestDiffIds(t *testing.T) {
	tests := []struct {
		name       string
		remoteIds  []string
		cachedIds  []string
		wantFetch  []string
		wantDelete []string
	}{
		{
			name:       "empty both",
			remoteIds:  nil,
			cachedIds:  nil,
			wantFetch:  nil,
			wantDelete: nil,
		},
		{
			name:       "all new",
			remoteIds:  []string{"id1", "id2", "id3"},
			cachedIds:  nil,
			wantFetch:  []string{"id1", "id2", "id3"},
			wantDelete: nil,
		},
		{
			name:       "all cached",
			remoteIds:  nil,
			cachedIds:  []string{"id1", "id2"},
			wantFetch:  nil,
			wantDelete: []string{"id1", "id2"},
		},
		{
			name:       "no changes",
			remoteIds:  []string{"id1", "id2", "id3"},
			cachedIds:  []string{"id1", "id2", "id3"},
			wantFetch:  nil,
			wantDelete: nil,
		},
		{
			name:       "some new some deleted",
			remoteIds:  []string{"id1", "id2", "id3", "id5"},
			cachedIds:  []string{"id1", "id2", "id4"},
			wantFetch:  []string{"id3", "id5"},
			wantDelete: []string{"id4"},
		},
		{
			name:       "only additions",
			remoteIds:  []string{"id1", "id2", "id3"},
			cachedIds:  []string{"id1"},
			wantFetch:  []string{"id2", "id3"},
			wantDelete: nil,
		},
		{
			name:       "only deletions",
			remoteIds:  []string{"id1"},
			cachedIds:  []string{"id1", "id2", "id3"},
			wantFetch:  nil,
			wantDelete: []string{"id2", "id3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFetch, gotDelete := DiffIds(tt.remoteIds, tt.cachedIds)
			if !equalSlices(gotFetch, tt.wantFetch) {
				t.Errorf("DiffIds() toFetch = %v, want %v", gotFetch, tt.wantFetch)
			}
			if !equalSlices(gotDelete, tt.wantDelete) {
				t.Errorf("DiffIds() toDelete = %v, want %v", gotDelete, tt.wantDelete)
			}
		})
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTraceCache(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "test-service")
	if err := os.MkdirAll(cacheDir, 0o750); err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}

	cache := &TraceCache{cacheDir: cacheDir}

	// Test empty cache
	ids, err := cache.GetCachedIds()
	if err != nil {
		t.Fatalf("GetCachedIds() error = %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("GetCachedIds() = %v, want empty", ids)
	}

	// Test save and load
	traces := []*backend.TraceTest{
		{Id: "trace-1", TraceId: "tid-1"},
		{Id: "trace-2", TraceId: "tid-2"},
	}

	if err := cache.SaveTraces(traces); err != nil {
		t.Fatalf("SaveTraces() error = %v", err)
	}

	ids, err = cache.GetCachedIds()
	if err != nil {
		t.Fatalf("GetCachedIds() error = %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("GetCachedIds() = %v, want 2 ids", ids)
	}

	// Test load single
	loaded, err := cache.LoadTrace("trace-1")
	if err != nil {
		t.Fatalf("LoadTrace() error = %v", err)
	}
	if loaded.Id != "trace-1" || loaded.TraceId != "tid-1" {
		t.Errorf("LoadTrace() = %v, want Id=trace-1, TraceId=tid-1", loaded)
	}

	// Test load all
	allLoaded, err := cache.LoadAllTraces()
	if err != nil {
		t.Fatalf("LoadAllTraces() error = %v", err)
	}
	if len(allLoaded) != 2 {
		t.Errorf("LoadAllTraces() = %v, want 2 traces", allLoaded)
	}

	// Test delete
	if err := cache.DeleteTraces([]string{"trace-1"}); err != nil {
		t.Fatalf("DeleteTraces() error = %v", err)
	}

	ids, err = cache.GetCachedIds()
	if err != nil {
		t.Fatalf("GetCachedIds() error = %v", err)
	}
	if len(ids) != 1 || ids[0] != "trace-2" {
		t.Errorf("GetCachedIds() after delete = %v, want [trace-2]", ids)
	}

	// Test delete non-existent (should not error)
	if err := cache.DeleteTraces([]string{"non-existent"}); err != nil {
		t.Errorf("DeleteTraces(non-existent) error = %v, want nil", err)
	}
}
