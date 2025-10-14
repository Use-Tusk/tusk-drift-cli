package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strings"

	"github.com/Use-Tusk/tusk-drift-cli/internal/logging"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
	"github.com/agnivade/levenshtein"
)

type MockMatcherRequestData struct {
	InputValue      any
	InputValueHash  string
	InputSchema     *core.JsonSchema
	InputSchemaHash string
}

type MockMatcher struct {
	server *Server
}

func reducedInputValueHash(span *core.Span) string {
	if span == nil || span.InputValue == nil || span.InputSchema == nil {
		return ""
	}
	reduced := utils.ReduceByMatchImportance(span.InputValue.AsMap(), span.InputSchema)
	return utils.GenerateDeterministicHash(reduced)
}

func reducedInputSchemaHash(span *core.Span) string {
	if span == nil || span.InputSchema == nil {
		return ""
	}
	// Drop 0-importance fields from schema itself
	reduced := utils.ReduceSchemaByMatchImportance(span.InputSchema)
	return utils.GenerateDeterministicHash(reduced)
}

func reducedRequestValueHash(req *core.GetMockRequest) string {
	if req == nil || req.OutboundSpan == nil || req.OutboundSpan.InputValue == nil || req.OutboundSpan.InputSchema == nil {
		return ""
	}
	reduced := utils.ReduceByMatchImportance(req.OutboundSpan.InputValue.AsMap(), req.OutboundSpan.InputSchema)
	return utils.GenerateDeterministicHash(reduced)
}

func reducedRequestSchemaHash(req *core.GetMockRequest) string {
	if req == nil || req.OutboundSpan == nil || req.OutboundSpan.InputSchema == nil {
		return ""
	}
	reduced := utils.ReduceSchemaByMatchImportance(req.OutboundSpan.InputSchema)
	return utils.GenerateDeterministicHash(reduced)
}

func NewMockMatcher(server *Server) *MockMatcher {
	return &MockMatcher{server: server}
}

// FindBestMatch implements the priority matching algorithm for spans within a trace
func (mm *MockMatcher) FindBestMatchInTrace(req *core.GetMockRequest, traceID string) (*core.Span, *backend.MatchLevel, error) {
	mm.server.mu.RLock()
	spans, exists := mm.server.spans[traceID]
	mm.server.mu.RUnlock()

	if !exists {
		return nil, nil, fmt.Errorf("no spans loaded for trace %s", traceID)
	}

	var filteredSpans []*core.Span
	for _, span := range spans {
		if span.PackageName == req.OutboundSpan.PackageName {
			filteredSpans = append(filteredSpans, span)
		}
	}
	if len(filteredSpans) == 0 {
		return nil, nil, fmt.Errorf("no spans found for package name %s in trace %s", req.OutboundSpan.PackageName, traceID)
	}
	return mm.runPriorityMatchingWithTraceSpans(req, traceID, filteredSpans)
}

// FindBestMatchInSpans implements the priority matching algorithm for spans across a test suite
func (mm *MockMatcher) FindBestMatchAcrossTraces(req *core.GetMockRequest, traceID string, spans []*core.Span) (*core.Span, *backend.MatchLevel, error) {
	var filteredSpans []*core.Span
	for _, span := range spans {
		if span.PackageName == req.OutboundSpan.PackageName {
			filteredSpans = append(filteredSpans, span)
		}
	}
	if len(filteredSpans) == 0 {
		return nil, nil, fmt.Errorf("no spans found for package name %s in provided set", req.OutboundSpan.PackageName)
	}

	// Priorities 10–11 over the whole suite (value hash, then reduced value hash)
	var requestBody any
	if req.OutboundSpan.InputValue != nil {
		requestBody = req.OutboundSpan.InputValue.AsMap()
	}
	requestData := MockMatcherRequestData{
		InputValue:      requestBody,
		InputValueHash:  req.OutboundSpan.GetInputValueHash(),
		InputSchema:     req.OutboundSpan.InputSchema,
		InputSchemaHash: req.OutboundSpan.GetInputSchemaHash(),
	}

	sortedSpans := make([]*core.Span, len(filteredSpans))
	copy(sortedSpans, filteredSpans)
	sort.Slice(sortedSpans, func(i, j int) bool {
		if sortedSpans[i].Timestamp == nil && sortedSpans[j].Timestamp == nil {
			return sortedSpans[i].SpanId < sortedSpans[j].SpanId
		}
		if sortedSpans[i].Timestamp == nil {
			return true
		}
		if sortedSpans[j].Timestamp == nil {
			return false
		}
		return sortedSpans[i].Timestamp.AsTime().Before(sortedSpans[j].Timestamp.AsTime())
	})

	// Priority 9: Check global spans from Tusk Drift Cloud
	// TODO: not implemented

	// Priority 10: Input value across all spans in test suite
	if match := mm.findUnusedSpanByInputValueHash(requestData, sortedSpans); match != nil {
		return match, &backend.MatchLevel{
			MatchType:        backend.MatchType_MATCH_TYPE_INPUT_VALUE_HASH,
			MatchScope:       backend.MatchScope_MATCH_SCOPE_GLOBAL,
			MatchDescription: "Suite unused span by input value hash",
		}, nil
	}
	if match := mm.findUsedSpanByInputValueHash(requestData, sortedSpans); match != nil {
		return match, &backend.MatchLevel{
			MatchType:        backend.MatchType_MATCH_TYPE_INPUT_VALUE_HASH,
			MatchScope:       backend.MatchScope_MATCH_SCOPE_GLOBAL,
			MatchDescription: "Suite used span by input value hash",
		}, nil
	}

	// Priority 11: Reduced input value across all spans in test suite
	if match := mm.findUnusedSpanByReducedInputValueHash(req, sortedSpans); match != nil {
		return match, &backend.MatchLevel{
			MatchType:        backend.MatchType_MATCH_TYPE_INPUT_VALUE_HASH_REDUCED_SCHEMA,
			MatchScope:       backend.MatchScope_MATCH_SCOPE_GLOBAL,
			MatchDescription: "Suite unused span by input value hash with reduced schema",
		}, nil
	}
	if match := mm.findUsedSpanByReducedInputValueHash(req, sortedSpans); match != nil {
		return match, &backend.MatchLevel{
			MatchType:        backend.MatchType_MATCH_TYPE_INPUT_VALUE_HASH_REDUCED_SCHEMA,
			MatchScope:       backend.MatchScope_MATCH_SCOPE_GLOBAL,
			MatchDescription: "Suite used span by input value hash with reduced schema",
		}, nil
	}

	return nil, nil, fmt.Errorf("no matching span found")
}

func (mm *MockMatcher) runPriorityMatchingWithTraceSpans(req *core.GetMockRequest, traceID string, spans []*core.Span) (*core.Span, *backend.MatchLevel, error) {
	scope := scopeTrace

	var requestBody any
	if req.OutboundSpan.InputValue != nil {
		requestBody = req.OutboundSpan.InputValue.AsMap()
		if !req.OutboundSpan.IsPreAppStart {
			bodyForLog := requestBody
			if !(isDebugEnabled()) {
				bodyForLog = redactSensitive(requestBody)
			}
			logging.LogToCurrentTest(traceID, fmt.Sprintf("Finding best match for request: %v", bodyForLog))
		}
	}

	schema := req.OutboundSpan.InputSchema
	schemaHash := req.OutboundSpan.InputSchemaHash
	valueHash := req.OutboundSpan.InputValueHash

	requestData := MockMatcherRequestData{
		InputValue:      requestBody,
		InputValueHash:  valueHash,
		InputSchema:     schema,
		InputSchemaHash: schemaHash,
	}

	sortedSpans := make([]*core.Span, len(spans))
	copy(sortedSpans, spans)
	sort.Slice(sortedSpans, func(i, j int) bool {
		// Sort by timestamp (oldest first)
		// Handle nil timestamps by treating them as oldest
		if sortedSpans[i].Timestamp == nil && sortedSpans[j].Timestamp == nil {
			return sortedSpans[i].SpanId < sortedSpans[j].SpanId // Fallback to span ID
		}
		if sortedSpans[i].Timestamp == nil {
			return true // nil timestamps come first
		}
		if sortedSpans[j].Timestamp == nil {
			return false
		}
		return sortedSpans[i].Timestamp.AsTime().Before(sortedSpans[j].Timestamp.AsTime())
	})

	slog.Debug("Finding best match for request",
		"availableSpans", len(sortedSpans),
		"traceID", traceID,
		"scope", scope)

	// Priority 1: Unused span by input value hash
	slog.Debug("Trying Priority 1: Unused span by input value hash", "traceId", traceID)
	if match := mm.findUnusedSpanByInputValueHash(requestData, sortedSpans); match != nil {
		slog.Debug("Found unused span by input value hash", "spanName", match.Name)
		mm.markSpanAsUsed(match)
		return match, &backend.MatchLevel{
			MatchType:        backend.MatchType_MATCH_TYPE_INPUT_VALUE_HASH,
			MatchScope:       backend.MatchScope_MATCH_SCOPE_TRACE,
			MatchDescription: "Unused span by input value hash",
		}, nil
	}
	slog.Debug("Priority 1 failed: No unused span by input value hash", "traceId", traceID)

	// Priority 2: Used span by input value hash
	slog.Debug("Trying Priority 2: Used span by input value hash", "traceId", traceID)
	if match := mm.findUsedSpanByInputValueHash(requestData, sortedSpans); match != nil {
		slog.Debug("Found used span by input value hash", "spanName", match.Name)
		mm.markSpanAsUsed(match)
		return match, &backend.MatchLevel{
			MatchType:        backend.MatchType_MATCH_TYPE_INPUT_VALUE_HASH,
			MatchScope:       backend.MatchScope_MATCH_SCOPE_TRACE,
			MatchDescription: "Used span by input value hash",
		}, nil
	}
	slog.Debug("Priority 2 failed: No used span by input value hash", "traceId", traceID)

	// Priority 3: Unused span by input value hash with reduced schema
	slog.Debug("Trying Priority 3: Unused span by input value hash with reduced schema", "traceId", traceID)
	if match := mm.findUnusedSpanByReducedInputValueHash(req, sortedSpans); match != nil {
		slog.Debug("Found unused span by input value hash with reduced schema", "spanName", match.Name)
		mm.markSpanAsUsed(match)
		return match, &backend.MatchLevel{
			MatchType:        backend.MatchType_MATCH_TYPE_INPUT_VALUE_HASH_REDUCED_SCHEMA,
			MatchScope:       backend.MatchScope_MATCH_SCOPE_TRACE,
			MatchDescription: "Unused span by input value hash with reduced schema",
		}, nil
	}
	slog.Debug("Priority 3 failed: No unused span by input value hash with reduced schema", "traceId", traceID)

	// Priority 4: Used span by input value hash with reduced schema
	slog.Debug("Trying Priority 4: Used span by input value hash with reduced schema", "traceId", traceID)
	if match := mm.findUsedSpanByReducedInputValueHash(req, sortedSpans); match != nil {
		slog.Debug("Found used span by input value hash with reduced schema", "spanName", match.Name)
		mm.markSpanAsUsed(match)
		return match, &backend.MatchLevel{
			MatchType:        backend.MatchType_MATCH_TYPE_INPUT_VALUE_HASH_REDUCED_SCHEMA,
			MatchScope:       backend.MatchScope_MATCH_SCOPE_TRACE,
			MatchDescription: "Used span by input value hash with reduced schema",
		}, nil
	}
	slog.Debug("Priority 4 failed: No used span by input value hash with reduced schema", "traceId", traceID)

	// Priority 5: Unused span by input schema hash
	slog.Debug("Trying Priority 5: Unused span by input schema hash", "traceId", traceID)
	if result := mm.findUnusedSpanByInputSchemaHash(requestData, sortedSpans); result.span != nil {
		slog.Debug("Found unused span by input schema hash", "spanName", result.span.Name)
		mm.markSpanAsUsed(result.span)
		return result.span, buildMatchLevelWithSimilarity(
			backend.MatchType_MATCH_TYPE_INPUT_SCHEMA_HASH,
			backend.MatchScope_MATCH_SCOPE_TRACE,
			"Unused span by input schema hash",
			result,
		), nil
	}
	slog.Debug("Priority 5 failed: No unused span by input schema hash", "traceId", traceID)

	// Priority 6: Used span by input schema hash
	slog.Debug("Trying Priority 6: Used span by input schema hash", "traceId", traceID)
	if result := mm.findUsedSpanByInputSchemaHash(requestData, sortedSpans); result.span != nil {
		slog.Debug("Found used span by input schema hash", "spanName", result.span.Name)
		mm.markSpanAsUsed(result.span)
		return result.span, buildMatchLevelWithSimilarity(
			backend.MatchType_MATCH_TYPE_INPUT_SCHEMA_HASH,
			backend.MatchScope_MATCH_SCOPE_TRACE,
			"Used span by input schema hash",
			result,
		), nil
	}
	slog.Debug("Priority 6 failed: No used span by input schema hash", "traceId", traceID)

	// Priority 7: Unused span by reduced input value hash
	slog.Debug("Trying Priority 7: Unused span by reduced input schema hash", "traceId", traceID)
	if result := mm.findUnusedSpanByReducedInputSchemaHash(req, sortedSpans); result.span != nil {
		slog.Debug("Found unused span by reduced input value hash", "spanName", result.span.Name)
		mm.markSpanAsUsed(result.span)
		return result.span, buildMatchLevelWithSimilarity(
			backend.MatchType_MATCH_TYPE_INPUT_SCHEMA_HASH_REDUCED_SCHEMA,
			backend.MatchScope_MATCH_SCOPE_TRACE,
			"Unused span by reduced input schema hash",
			result,
		), nil
	}
	slog.Debug("Priority 7 failed: No unused span by reduced input schema hash", "traceId", traceID)

	// Priority 8: Used span by reduced input schema hash
	slog.Debug("Trying Priority 8: Used span by reduced input schema hash", "traceId", traceID)
	if result := mm.findUsedSpanByReducedInputSchemaHash(req, sortedSpans); result.span != nil {
		slog.Debug("Found used span by reduced input schema hash", "spanName", result.span.Name)
		mm.markSpanAsUsed(result.span)
		return result.span, buildMatchLevelWithSimilarity(
			backend.MatchType_MATCH_TYPE_INPUT_SCHEMA_HASH_REDUCED_SCHEMA,
			backend.MatchScope_MATCH_SCOPE_TRACE,
			"Used span by reduced input schema hash",
			result,
		), nil
	}
	slog.Debug("Priority 8 failed: No used span by reduced input schema hash", "traceId", traceID)

	return nil, nil, fmt.Errorf("no matching span found")
}

func (mm *MockMatcher) markSpanAsUsed(span *core.Span) {
	mm.server.mu.Lock()
	defer mm.server.mu.Unlock()

	if mm.server.spanUsage[span.TraceId] == nil {
		mm.server.spanUsage[span.TraceId] = make(map[string]bool)
	}

	mm.server.spanUsage[span.TraceId][span.SpanId] = true
}

func (mm *MockMatcher) isUnused(span *core.Span) bool {
	mm.server.mu.RLock()
	defer mm.server.mu.RUnlock()

	if traceUsage, exists := mm.server.spanUsage[span.TraceId]; exists {
		if isUsed, exists := traceUsage[span.SpanId]; exists {
			return !isUsed
		}
	}

	// Default to unused if not found in tracking map
	return true
}

func (mm *MockMatcher) isUsed(span *core.Span) bool {
	return !mm.isUnused(span)
}

// buildMatchLevelWithSimilarity creates a MatchLevel with similarity scoring data
func buildMatchLevelWithSimilarity(matchType backend.MatchType, matchScope backend.MatchScope, baseDescription string, result spanMatchResult) *backend.MatchLevel {
	level := &backend.MatchLevel{
		MatchType:        matchType,
		MatchScope:       matchScope,
		MatchDescription: baseDescription,
	}

	if result.multipleMatches {
		// Populate best score
		bestScore := float32(result.bestScore)
		level.SimilarityScore = &bestScore

		// Build description with scores
		if len(result.topCandidates) > 0 {
			nextBestScore := result.topCandidates[0].score
			level.MatchDescription = fmt.Sprintf("%s (similarity: %.2f, next best: %.2f)", baseDescription, result.bestScore, nextBestScore)
		} else {
			level.MatchDescription = fmt.Sprintf("%s (similarity: %.2f)", baseDescription, result.bestScore)
		}

		// Populate top candidates (up to 5)
		for _, candidate := range result.topCandidates {
			level.TopCandidates = append(level.TopCandidates, &backend.SimilarityCandidate{
				SpanId: candidate.span.SpanId,
				Score:  float32(candidate.score),
			})
		}
	}

	return level
}

// spanWithScore holds a span and its similarity score
type spanWithScore struct {
	span  *core.Span
	score float64
}

// calculateSimilarityScore computes a normalized similarity score between two values
// by recursively comparing their structure using Levenshtein distance.
// Returns a score between 0 and 1, where 1 is identical and 0 is completely different.
func calculateSimilarityScore(a, b any, depth int) float64 {
	const maxDepth = 5
	if depth > maxDepth {
		// Beyond max depth, stringify and compare as strings
		aStr := safeStringify(a)
		bStr := safeStringify(b)
		return compareStrings(aStr, bStr)
	}

	// Handle nil cases
	if a == nil && b == nil {
		return 1.0
	}
	if a == nil || b == nil {
		return 0.0
	}

	switch aVal := a.(type) {
	case map[string]any:
		bMap, ok := b.(map[string]any)
		if !ok {
			return 0.0
		}
		return compareMaps(aVal, bMap, depth)

	case []any:
		bSlice, ok := b.([]any)
		if !ok {
			return 0.0
		}
		return compareSlices(aVal, bSlice, depth)

	case string:
		bStr, ok := b.(string)
		if !ok {
			return 0.0
		}
		return compareStrings(aVal, bStr)

	default:
		// For numbers, bools, and other primitives, convert to string and compare
		aStr := fmt.Sprintf("%v", a)
		bStr := fmt.Sprintf("%v", b)
		return compareStrings(aStr, bStr)
	}
}

// safeStringify converts any value to a string representation safely
func safeStringify(v any) string {
	if v == nil {
		return ""
	}

	switch val := v.(type) {
	case string:
		return val
	case map[string]any, []any:
		// For complex types, use JSON marshaling
		bytes, err := json.Marshal(val)
		if err != nil {
			// Fallback to fmt if JSON fails
			return fmt.Sprintf("%v", val)
		}
		return string(bytes)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func compareMaps(a, b map[string]any, depth int) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}

	// Get all unique keys
	allKeys := make(map[string]struct{})
	for k := range a {
		allKeys[k] = struct{}{}
	}
	for k := range b {
		allKeys[k] = struct{}{}
	}

	totalScore := 0.0
	for key := range allKeys {
		aVal, aExists := a[key]
		bVal, bExists := b[key]

		if aExists && bExists {
			totalScore += calculateSimilarityScore(aVal, bVal, depth+1)
		}
		// If key doesn't exist in both, it contributes 0 to the score
	}

	return totalScore / float64(len(allKeys))
}

func compareSlices(a, b []any, depth int) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}

	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}

	totalScore := 0.0
	for i := 0; i < maxLen; i++ {
		if i >= len(a) || i >= len(b) {
			// One slice is shorter, contributes 0
			continue
		}
		totalScore += calculateSimilarityScore(a[i], b[i], depth+1)
	}

	return totalScore / float64(maxLen)
}

func compareStrings(a, b string) float64 {
	if a == b {
		return 1.0
	}

	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	if maxLen == 0 {
		return 1.0
	}

	distance := levenshtein.ComputeDistance(a, b)
	return 1.0 - (float64(distance) / float64(maxLen))
}

// findBestMatchBySimilarity ranks spans by similarity score and returns the best match
func (mm *MockMatcher) findBestMatchBySimilarity(requestData MockMatcherRequestData, spans []*core.Span, isUnused bool) (*core.Span, float64, []spanWithScore) {
	if len(spans) == 0 {
		return nil, 0.0, nil
	}

	// Limit to first 50 spans for performance
	maxSpansToScore := 50
	spansToScore := spans
	if len(spans) > maxSpansToScore {
		spansToScore = spans[:maxSpansToScore]
	}

	var scored []spanWithScore
	for _, span := range spansToScore {
		// Filter by usage status
		if isUnused && !mm.isUnused(span) {
			continue
		}
		if !isUnused && mm.isUnused(span) {
			continue
		}

		var spanValue any
		if span.InputValue != nil {
			spanValue = span.InputValue.AsMap()
		}

		score := calculateSimilarityScore(requestData.InputValue, spanValue, 0)
		scored = append(scored, spanWithScore{span: span, score: score})
	}

	if len(scored) == 0 {
		return nil, 0.0, nil
	}

	// Sort by score (highest first), then by timestamp (oldest first)
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		// Tiebreaker: oldest timestamp
		if scored[i].span.Timestamp == nil && scored[j].span.Timestamp == nil {
			return scored[i].span.SpanId < scored[j].span.SpanId
		}
		if scored[i].span.Timestamp == nil {
			return true
		}
		if scored[j].span.Timestamp == nil {
			return false
		}
		return scored[i].span.Timestamp.AsTime().Before(scored[j].span.Timestamp.AsTime())
	})

	bestScore := scored[0].score

	// Get top 5 candidates (excluding the best match)
	var topCandidates []spanWithScore
	maxCandidates := 5
	for i := 1; i < len(scored) && i <= maxCandidates; i++ {
		topCandidates = append(topCandidates, scored[i])
	}

	return scored[0].span, bestScore, topCandidates
}

func (mm *MockMatcher) findUnusedSpanByInputValueHash(requestData MockMatcherRequestData, spans []*core.Span) *core.Span {
	for i := range spans {
		if mm.isUnused(spans[i]) && spans[i].InputValueHash == requestData.InputValueHash {
			return spans[i]
		}
	}
	return nil
}

func (mm *MockMatcher) findUsedSpanByInputValueHash(requestData MockMatcherRequestData, spans []*core.Span) *core.Span {
	for i := range spans {
		if mm.isUsed(spans[i]) && spans[i].InputValueHash == requestData.InputValueHash {
			return spans[i]
		}
	}
	return nil
}

type spanMatchResult struct {
	span            *core.Span
	bestScore       float64
	topCandidates   []spanWithScore // Top 5 candidates with scores (excluding the best match)
	multipleMatches bool
}

func (mm *MockMatcher) findUnusedSpanByInputSchemaHash(requestData MockMatcherRequestData, spans []*core.Span) spanMatchResult {
	var candidates []*core.Span
	for i := range spans {
		span := spans[i]
		if !mm.isUnused(span) {
			continue
		}
		if mm.schemaMatchWithHttpShape(requestData, span) {
			candidates = append(candidates, span)
		}
	}

	if len(candidates) == 0 {
		return spanMatchResult{}
	}
	if len(candidates) == 1 {
		return spanMatchResult{span: candidates[0], multipleMatches: false}
	}

	// Multiple matches - use similarity scoring
	bestMatch, bestScore, topCandidates := mm.findBestMatchBySimilarity(requestData, candidates, true)
	return spanMatchResult{
		span:            bestMatch,
		bestScore:       bestScore,
		topCandidates:   topCandidates,
		multipleMatches: true,
	}
}

func (mm *MockMatcher) findUsedSpanByInputSchemaHash(requestData MockMatcherRequestData, spans []*core.Span) spanMatchResult {
	var candidates []*core.Span
	for i := range spans {
		span := spans[i]
		if !mm.isUsed(span) {
			continue
		}
		if mm.schemaMatchWithHttpShape(requestData, span) {
			candidates = append(candidates, span)
		}
	}

	if len(candidates) == 0 {
		return spanMatchResult{}
	}
	if len(candidates) == 1 {
		return spanMatchResult{span: candidates[0], multipleMatches: false}
	}

	// Multiple matches - use similarity scoring
	bestMatch, bestScore, topCandidates := mm.findBestMatchBySimilarity(requestData, candidates, false)
	return spanMatchResult{
		span:            bestMatch,
		bestScore:       bestScore,
		topCandidates:   topCandidates,
		multipleMatches: true,
	}
}

func (mm *MockMatcher) findUnusedSpanByReducedInputValueHash(req *core.GetMockRequest, spans []*core.Span) *core.Span {
	target := reducedRequestValueHash(req)
	if target == "" {
		return nil
	}
	for i := range spans {
		if !mm.isUnused(spans[i]) {
			continue
		}
		if reducedInputValueHash(spans[i]) == target {
			return spans[i]
		}
	}
	return nil
}

func (mm *MockMatcher) findUsedSpanByReducedInputValueHash(req *core.GetMockRequest, spans []*core.Span) *core.Span {
	target := reducedRequestValueHash(req)
	if target == "" {
		return nil
	}
	for i := range spans {
		if !mm.isUsed(spans[i]) {
			continue
		}
		if reducedInputValueHash(spans[i]) == target {
			return spans[i]
		}
	}
	return nil
}

func (mm *MockMatcher) findUnusedSpanByReducedInputSchemaHash(req *core.GetMockRequest, spans []*core.Span) spanMatchResult {
	target := reducedRequestSchemaHash(req)
	if target == "" {
		return spanMatchResult{}
	}

	requestData := reqToRequestData(req)
	var candidates []*core.Span
	for i := range spans {
		if !mm.isUnused(spans[i]) {
			continue
		}
		if reducedInputSchemaHash(spans[i]) == target && mm.schemaMatchWithHttpShape(requestData, spans[i]) {
			candidates = append(candidates, spans[i])
		}
	}

	if len(candidates) == 0 {
		return spanMatchResult{}
	}
	if len(candidates) == 1 {
		return spanMatchResult{span: candidates[0], multipleMatches: false}
	}

	// Multiple matches - use similarity scoring
	bestMatch, bestScore, topCandidates := mm.findBestMatchBySimilarity(requestData, candidates, true)
	return spanMatchResult{
		span:            bestMatch,
		bestScore:       bestScore,
		topCandidates:   topCandidates,
		multipleMatches: true,
	}
}

func (mm *MockMatcher) findUsedSpanByReducedInputSchemaHash(req *core.GetMockRequest, spans []*core.Span) spanMatchResult {
	target := reducedRequestSchemaHash(req)
	if target == "" {
		return spanMatchResult{}
	}

	requestData := reqToRequestData(req)
	var candidates []*core.Span
	for i := range spans {
		if !mm.isUsed(spans[i]) {
			continue
		}
		if reducedInputSchemaHash(spans[i]) == target && mm.schemaMatchWithHttpShape(requestData, spans[i]) {
			candidates = append(candidates, spans[i])
		}
	}

	if len(candidates) == 0 {
		return spanMatchResult{}
	}
	if len(candidates) == 1 {
		return spanMatchResult{span: candidates[0], multipleMatches: false}
	}

	// Multiple matches - use similarity scoring
	bestMatch, bestScore, topCandidates := mm.findBestMatchBySimilarity(requestData, candidates, false)
	return spanMatchResult{
		span:            bestMatch,
		bestScore:       bestScore,
		topCandidates:   topCandidates,
		multipleMatches: true,
	}
}

func reqToRequestData(req *core.GetMockRequest) MockMatcherRequestData {
	var body any
	if req.OutboundSpan != nil && req.OutboundSpan.InputValue != nil {
		body = req.OutboundSpan.InputValue.AsMap()
	}
	return MockMatcherRequestData{
		InputValue:      body,
		InputValueHash:  req.OutboundSpan.GetInputValueHash(),
		InputSchema:     req.OutboundSpan.InputSchema,
		InputSchemaHash: req.OutboundSpan.GetInputSchemaHash(),
	}
}

func (mm *MockMatcher) schemaMatchWithHttpShape(requestData MockMatcherRequestData, span *core.Span) bool {
	// Base schema-hash match
	if span.InputSchemaHash != requestData.InputSchemaHash {
		return false
	}

	// Build maps once
	reqMap, ok := requestData.InputValue.(map[string]any)
	if !ok {
		return false
	}
	var spanMap map[string]any
	if span.InputValue != nil {
		spanMap = span.InputValue.AsMap()
	}

	// GraphQL-aware guard (for GraphQL over http/https)
	// We may also want to handle more generically using package type
	reqGQL := normalizeGQL(extractGraphQLQuery(reqMap))
	spanGQL := normalizeGQL(extractGraphQLQuery(spanMap))
	if reqGQL != "" && spanGQL != "" && reqGQL != spanGQL {
		return false
	}

	// Only enforce HTTP-shape for HTTP/HTTPS
	if span.PackageName != "http" && span.PackageName != "https" {
		return true
	}

	// Method must match if present on both
	if !stringFieldEqualIfPresent(reqMap, spanMap, "method") {
		return false
	}

	// Hostname must match if both can be derived
	reqHost := extractHost(reqMap)
	spanHost := extractHost(spanMap)
	if reqHost != "" && spanHost != "" && reqHost != spanHost {
		return false
	}

	// Pathname must match (exclude query), and query key sets must be identical
	reqPath, reqKeys := extractPathAndQueryKeys(reqMap)
	spanPath, spanKeys := extractPathAndQueryKeys(spanMap)
	if reqPath != "" && spanPath != "" && reqPath != spanPath {
		return false
	}
	if !stringSetEqual(reqKeys, spanKeys) {
		return false
	}

	return true
}

func stringFieldEqualIfPresent(a, b map[string]any, key string) bool {
	va, okA := a[key].(string)
	vb, okB := b[key].(string)
	if okA && okB {
		return va == vb
	}
	return true
}

func extractHost(m map[string]any) string {
	// Prefer explicit hostname
	if hn, ok := m["hostname"].(string); ok && hn != "" {
		return hn
	}
	// Try to parse from url if present
	if raw, ok := m["url"].(string); ok && raw != "" {
		if u, err := url.Parse(raw); err == nil && u.Hostname() != "" {
			return u.Hostname()
		}
	}
	return ""
}

func extractPathAndQueryKeys(m map[string]any) (string, map[string]struct{}) {
	// Prefer 'path', else derive from 'url', else 'target'
	var s string
	if v, ok := m["path"].(string); ok && v != "" {
		s = v
	} else if v, ok := m["url"].(string); ok && v != "" {
		if u, err := url.Parse(v); err == nil {
			s = u.Path
			return s, parseQueryKeys(u.RawQuery)
		}
	} else if v, ok := m["target"].(string); ok && v != "" {
		s = v
	}

	base, rawQuery := splitPathQuery(s)
	return base, parseQueryKeys(rawQuery)
}

func splitPathQuery(p string) (string, string) {
	if i := strings.IndexByte(p, '?'); i >= 0 {
		return p[:i], p[i+1:]
	}
	return p, ""
}

func parseQueryKeys(raw string) map[string]struct{} {
	keys := make(map[string]struct{})
	if raw == "" {
		return keys
	}
	for pair := range strings.SplitSeq(raw, "&") {
		if pair == "" {
			continue
		}
		k := pair
		if i := strings.IndexByte(pair, '='); i >= 0 {
			k = pair[:i]
		}
		if dk, err := url.QueryUnescape(k); err == nil {
			k = dk
		}
		k = strings.TrimSpace(k)
		if k != "" {
			keys[k] = struct{}{}
		}
	}
	return keys
}

func stringSetEqual(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

// extractGraphQLQuery extracts the GraphQL query from the request body
// supports `body` as an object with `query`
func extractGraphQLQuery(m map[string]any) string {
	if m == nil {
		return ""
	}
	if b, ok := m["body"]; ok && b != nil {
		switch v := b.(type) {
		case map[string]any:
			if q, ok := v["query"].(string); ok {
				return q
			}
		case string:
			var obj map[string]any
			if err := json.Unmarshal([]byte(v), &obj); err == nil {
				if q, ok := obj["query"].(string); ok {
					return q
				}
			}
		}
	}
	return ""
}

func normalizeGQL(q string) string {
	// Normalize brace adjacency then collapse whitespace
	q = strings.NewReplacer("{", " { ", "}", " } ").Replace(strings.TrimSpace(q))
	return strings.Join(strings.Fields(q), " ")
}

func isDebugEnabled() bool {
	return slog.Default().Enabled(context.Background(), slog.LevelDebug)
}

// redactSensitive redacts sensitive fields from the given value.
// Useful for displaying logs to the CLI.
func redactSensitive(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			kl := strings.ToLower(k)
			if kl == "token" || kl == "authorization" || kl == "secret" || kl == "secretorpublickey" {
				out[k] = "[REDACTED]"
				continue
			}
			out[k] = redactSensitive(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i := range t {
			out[i] = redactSensitive(t[i])
		}
		return out
	default:
		return v
	}
}
