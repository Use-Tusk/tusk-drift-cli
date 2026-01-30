package cache

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
	"google.golang.org/protobuf/proto"
)

// ErrInvalidID is returned when an ID contains path traversal characters.
var ErrInvalidID = errors.New("invalid ID: contains path traversal characters")

// isValidPathComponent checks that an ID is safe to use as a single path component.
// It rejects IDs containing path separators or parent directory references.
func isValidPathComponent(id string) bool {
	if id == "" || id == "." || id == ".." {
		return false
	}
	if strings.ContainsAny(id, `/\`) {
		return false
	}
	if strings.Contains(id, "..") {
		return false
	}
	return true
}

// TraceCache manages local caching of cloud trace tests.
// Cache files are stored as protobuf-serialized .bin files named by trace test ID.
type TraceCache struct {
	cacheDir string
}

// NewTraceCache creates a new TraceCache for the given service ID.
// The cache directory is: <os.UserCacheDir()>/tusk/<serviceID>/
func NewTraceCache(serviceID string) (*TraceCache, error) {
	if !isValidPathComponent(serviceID) {
		return nil, fmt.Errorf("invalid service ID %q: %w", serviceID, ErrInvalidID)
	}

	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user cache directory: %w", err)
	}

	cacheDir := filepath.Join(userCacheDir, "tusk", serviceID)
	if err := os.MkdirAll(cacheDir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &TraceCache{cacheDir: cacheDir}, nil
}

// GetCachedIds returns the IDs of all cached trace tests by listing .bin files.
func (c *TraceCache) GetCachedIds() ([]string, error) {
	entries, err := os.ReadDir(c.cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read cache directory: %w", err)
	}

	var ids []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".bin") {
			ids = append(ids, strings.TrimSuffix(name, ".bin"))
		}
	}
	return ids, nil
}

// LoadTrace loads a single trace test from cache by ID.
func (c *TraceCache) LoadTrace(id string) (*backend.TraceTest, error) {
	if !isValidPathComponent(id) {
		return nil, fmt.Errorf("invalid trace ID %q: %w", id, ErrInvalidID)
	}

	filePath := filepath.Join(c.cacheDir, id+".bin")
	data, err := os.ReadFile(filePath) //nolint:gosec // G304: id is validated above
	if err != nil {
		return nil, fmt.Errorf("failed to read cache file %s: %w", filePath, err)
	}

	var trace backend.TraceTest
	if err := proto.Unmarshal(data, &trace); err != nil {
		return nil, fmt.Errorf("failed to unmarshal trace %s: %w", id, err)
	}
	return &trace, nil
}

// LoadAllTraces loads all cached trace tests.
func (c *TraceCache) LoadAllTraces() ([]*backend.TraceTest, error) {
	ids, err := c.GetCachedIds()
	if err != nil {
		return nil, err
	}

	traces := make([]*backend.TraceTest, 0, len(ids))
	for _, id := range ids {
		trace, err := c.LoadTrace(id)
		if err != nil {
			// Skip corrupted cache files
			continue
		}
		traces = append(traces, trace)
	}
	return traces, nil
}

// SaveTraces saves multiple trace tests to cache.
func (c *TraceCache) SaveTraces(traces []*backend.TraceTest) error {
	for _, trace := range traces {
		if err := c.saveTrace(trace); err != nil {
			return err
		}
	}
	return nil
}

// saveTrace saves a single trace test to cache.
func (c *TraceCache) saveTrace(trace *backend.TraceTest) error {
	if !isValidPathComponent(trace.Id) {
		return fmt.Errorf("invalid trace ID %q: %w", trace.Id, ErrInvalidID)
	}

	data, err := proto.Marshal(trace)
	if err != nil {
		return fmt.Errorf("failed to marshal trace %s: %w", trace.Id, err)
	}

	filePath := filepath.Join(c.cacheDir, trace.Id+".bin")
	if err := os.WriteFile(filePath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write cache file %s: %w", filePath, err)
	}
	return nil
}

// DeleteTraces removes the specified trace test IDs from cache.
func (c *TraceCache) DeleteTraces(ids []string) error {
	for _, id := range ids {
		if !isValidPathComponent(id) {
			return fmt.Errorf("invalid trace ID %q: %w", id, ErrInvalidID)
		}

		filePath := filepath.Join(c.cacheDir, id+".bin")
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to delete cache file %s: %w", filePath, err)
		}
	}
	return nil
}

// DiffIds computes which IDs need to be fetched and which need to be deleted.
// Returns (toFetch, toDelete).
func DiffIds(remoteIds, cachedIds []string) (toFetch, toDelete []string) {
	remoteSet := make(map[string]struct{}, len(remoteIds))
	for _, id := range remoteIds {
		remoteSet[id] = struct{}{}
	}

	cachedSet := make(map[string]struct{}, len(cachedIds))
	for _, id := range cachedIds {
		cachedSet[id] = struct{}{}
	}

	// toFetch = remoteIds - cachedIds
	for _, id := range remoteIds {
		if _, exists := cachedSet[id]; !exists {
			toFetch = append(toFetch, id)
		}
	}

	// toDelete = cachedIds - remoteIds
	for _, id := range cachedIds {
		if _, exists := remoteSet[id]; !exists {
			toDelete = append(toDelete, id)
		}
	}

	return toFetch, toDelete
}

// SpanType identifies the type of spans being cached.
type SpanType string

const (
	// SpanTypePreAppStart is for pre-app-start spans.
	SpanTypePreAppStart SpanType = "preappstart"
	// SpanTypeGlobal is for global spans.
	SpanTypeGlobal SpanType = "global"
)

// SpanCache manages local caching of spans (pre-app-start or global).
// Cache files are stored as protobuf-serialized .bin files named by span ID.
type SpanCache struct {
	cacheDir string
}

// NewSpanCache creates a new SpanCache for the given service ID and span type.
// The cache directory is: <os.UserCacheDir()>/tusk/<serviceID>/spans/<spanType>/
func NewSpanCache(serviceID string, spanType SpanType) (*SpanCache, error) {
	if !isValidPathComponent(serviceID) {
		return nil, fmt.Errorf("invalid service ID %q: %w", serviceID, ErrInvalidID)
	}
	if !isValidPathComponent(string(spanType)) {
		return nil, fmt.Errorf("invalid span type %q: %w", spanType, ErrInvalidID)
	}

	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user cache directory: %w", err)
	}

	cacheDir := filepath.Join(userCacheDir, "tusk", serviceID, "spans", string(spanType))
	if err := os.MkdirAll(cacheDir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &SpanCache{cacheDir: cacheDir}, nil
}

// GetCachedIds returns the IDs of all cached spans by listing .bin files.
func (c *SpanCache) GetCachedIds() ([]string, error) {
	entries, err := os.ReadDir(c.cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read cache directory: %w", err)
	}

	var ids []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".bin") {
			ids = append(ids, strings.TrimSuffix(name, ".bin"))
		}
	}
	return ids, nil
}

// LoadSpan loads a single span from cache by ID.
func (c *SpanCache) LoadSpan(id string) (*core.Span, error) {
	if !isValidPathComponent(id) {
		return nil, fmt.Errorf("invalid span ID %q: %w", id, ErrInvalidID)
	}

	filePath := filepath.Join(c.cacheDir, id+".bin")
	data, err := os.ReadFile(filePath) //nolint:gosec // G304: id is validated above
	if err != nil {
		return nil, fmt.Errorf("failed to read cache file %s: %w", filePath, err)
	}

	var span core.Span
	if err := proto.Unmarshal(data, &span); err != nil {
		return nil, fmt.Errorf("failed to unmarshal span %s: %w", id, err)
	}
	return &span, nil
}

// LoadAllSpans loads all cached spans.
func (c *SpanCache) LoadAllSpans() ([]*core.Span, error) {
	ids, err := c.GetCachedIds()
	if err != nil {
		return nil, err
	}

	spans := make([]*core.Span, 0, len(ids))
	for _, id := range ids {
		span, err := c.LoadSpan(id)
		if err != nil {
			// Skip corrupted cache files
			continue
		}
		spans = append(spans, span)
	}
	return spans, nil
}

// SaveSpans saves multiple spans to cache.
func (c *SpanCache) SaveSpans(spans []*core.Span) error {
	for _, span := range spans {
		if err := c.saveSpan(span); err != nil {
			return err
		}
	}
	return nil
}

// saveSpan saves a single span to cache.
func (c *SpanCache) saveSpan(span *core.Span) error {
	id := span.GetId()
	if !isValidPathComponent(id) {
		return fmt.Errorf("invalid span ID %q: %w", id, ErrInvalidID)
	}

	data, err := proto.Marshal(span)
	if err != nil {
		return fmt.Errorf("failed to marshal span %s: %w", id, err)
	}

	filePath := filepath.Join(c.cacheDir, id+".bin")
	if err := os.WriteFile(filePath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write cache file %s: %w", filePath, err)
	}
	return nil
}

// DeleteSpans removes the specified span IDs from cache.
func (c *SpanCache) DeleteSpans(ids []string) error {
	for _, id := range ids {
		if !isValidPathComponent(id) {
			return fmt.Errorf("invalid span ID %q: %w", id, ErrInvalidID)
		}

		filePath := filepath.Join(c.cacheDir, id+".bin")
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to delete cache file %s: %w", filePath, err)
		}
	}
	return nil
}
