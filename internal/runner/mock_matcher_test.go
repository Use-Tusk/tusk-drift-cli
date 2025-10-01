package runner

import (
	"testing"
	"time"

	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"

	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func toStruct(t *testing.T, m map[string]any) *structpb.Struct {
	if m == nil {
		return nil
	}
	s, err := structpb.NewStruct(m)
	require.NoError(t, err)
	return s
}

func unixMsToTimestamp(tsMs int64) *timestamppb.Timestamp {
	sec := tsMs / 1000
	nsec := (tsMs % 1000) * int64(time.Millisecond)
	return timestamppb.New(time.Unix(sec, nsec))
}

func makeSpan(t *testing.T, traceID, spanID, pkg string, inputValueMap, inputSchemaMap map[string]any, tsMs int64) *core.Span {
	var inputValueStruct, inputSchemaStruct *structpb.Struct
	var inputValueHash, inputSchemaHash string
	if inputValueMap != nil {
		inputValueStruct = toStruct(t, inputValueMap)
		inputValueHash = utils.GenerateDeterministicHash(inputValueMap)
	}
	if inputSchemaMap != nil {
		inputSchemaStruct = toStruct(t, inputSchemaMap)
		inputSchemaHash = utils.GenerateDeterministicHash(inputSchemaMap)
	}
	return &core.Span{
		TraceId:         traceID,
		SpanId:          spanID,
		PackageName:     pkg,
		InputValue:      inputValueStruct,
		InputSchema:     inputSchemaStruct,
		InputValueHash:  inputValueHash,
		InputSchemaHash: inputSchemaHash,
		Timestamp:       unixMsToTimestamp(tsMs),
	}
}

func makeMockRequest(t *testing.T, pkg string, inputValueMap, inputSchemaMap map[string]any) *core.GetMockRequest {
	var inputValueStruct, inputSchemaStruct *structpb.Struct
	var inputValueHash, inputSchemaHash string
	if inputValueMap != nil {
		inputValueStruct = toStruct(t, inputValueMap)
		inputValueHash = utils.GenerateDeterministicHash(inputValueMap)
	}
	if inputSchemaMap != nil {
		inputSchemaStruct = toStruct(t, inputSchemaMap)
		inputSchemaHash = utils.GenerateDeterministicHash(inputSchemaMap)
	}
	return &core.GetMockRequest{
		OutboundSpan: &core.Span{
			PackageName:     pkg,
			InputValue:      inputValueStruct,
			InputSchema:     inputSchemaStruct,
			InputValueHash:  inputValueHash,
			InputSchemaHash: inputSchemaHash,
		},
		Operation: "GET",
	}
}

func TestFindBestMatchInTrace_InputValueHash_PrefersUnusedOldest(t *testing.T) {
	server, err := NewServer("svc")
	require.NoError(t, err)
	mm := NewMockMatcher(server)

	traceID := "trace-1"
	pkg := "http"

	inputValueMap := map[string]any{"method": "GET", "path": "/users"}
	var inputSchemaMap map[string]any

	spanOld := makeSpan(t, traceID, "s1", pkg, inputValueMap, inputSchemaMap, 1000)
	spanNew := makeSpan(t, traceID, "s2", pkg, inputValueMap, inputSchemaMap, 2000)
	server.LoadSpansForTrace(traceID, []*core.Span{spanNew, spanOld})

	req := makeMockRequest(t, pkg, inputValueMap, inputSchemaMap)

	match, level, err := mm.FindBestMatchInTrace(req, traceID)
	require.NoError(t, err)
	require.NotNil(t, match)
	require.NotNil(t, level)

	assert.Equal(t, "s1", match.SpanId)
	assert.Equal(t, backend.MatchType_MATCH_TYPE_INPUT_VALUE_HASH, level.MatchType)
	assert.Equal(t, backend.MatchScope_MATCH_SCOPE_TRACE, level.MatchScope)

	server.mu.RLock()
	assert.True(t, server.spanUsage[traceID]["s1"])
	assert.False(t, server.spanUsage[traceID]["s2"])
	server.mu.RUnlock()

	match2, level2, err := mm.FindBestMatchInTrace(req, traceID)
	require.NoError(t, err)
	require.NotNil(t, match2)
	require.NotNil(t, level2)
	assert.Equal(t, "s2", match2.SpanId)
	assert.Equal(t, backend.MatchType_MATCH_TYPE_INPUT_VALUE_HASH, level2.MatchType)
	assert.Equal(t, backend.MatchScope_MATCH_SCOPE_TRACE, level2.MatchScope)

	// Both used now; should fall back to used (earliest)
	match3, level3, err := mm.FindBestMatchInTrace(req, traceID)
	require.NoError(t, err)
	require.NotNil(t, match3)
	require.NotNil(t, level3)
	assert.Equal(t, "s1", match3.SpanId)
	assert.Equal(t, backend.MatchType_MATCH_TYPE_INPUT_VALUE_HASH, level3.MatchType)
	assert.Equal(t, backend.MatchScope_MATCH_SCOPE_TRACE, level3.MatchScope)
}

func TestFindBestMatchInTrace_ReducedInputValueHash_MatchesWhenDirectHashDiffers(t *testing.T) {
	server, err := NewServer("svc")
	require.NoError(t, err)
	mm := NewMockMatcher(server)

	traceID := "trace-2"
	pkg := "http"

	// token has matchImportance 0; differing token values should not affect reduced hash
	inputRequestMap := map[string]any{"method": "GET", "path": "/a", "token": "alpha"}
	inputValueMap := map[string]any{"method": "GET", "path": "/a", "token": "beta"}
	inputSchemaMap := map[string]any{
		"properties": map[string]any{
			"method": map[string]any{},
			"path":   map[string]any{},
			"token":  map[string]any{"matchImportance": 0.0},
		},
	}

	span := makeSpan(t, traceID, "sR", pkg, inputValueMap, inputSchemaMap, 1000)
	server.LoadSpansForTrace(traceID, []*core.Span{span})
	req := makeMockRequest(t, pkg, inputRequestMap, inputSchemaMap)

	// Sanity: direct hashes differ
	assert.NotEqual(t, utils.GenerateDeterministicHash(inputRequestMap), utils.GenerateDeterministicHash(inputValueMap))

	match, level, err := mm.FindBestMatchInTrace(req, traceID)
	require.NoError(t, err)
	require.NotNil(t, match)
	require.NotNil(t, level)

	assert.Equal(t, "sR", match.SpanId)
	assert.Equal(t, backend.MatchType_MATCH_TYPE_INPUT_VALUE_HASH_REDUCED_SCHEMA, level.MatchType)
	assert.Equal(t, backend.MatchScope_MATCH_SCOPE_TRACE, level.MatchScope)
}

func TestFindBestMatchInTrace_InputSchemaHash_WithHTTPShape(t *testing.T) {
	server, err := NewServer("svc")
	require.NoError(t, err)
	mm := NewMockMatcher(server)

	traceID := "trace-3"
	pkg := "http"

	// Different query values but same key set; method/host/path equal
	inputRequestMap := map[string]any{
		"method": "GET",
		"url":    "https://api.example.com/users?foo=1&bar=2",
	}
	inputValueMap := map[string]any{
		"method": "GET",
		"url":    "https://api.example.com/users?bar=9&foo=zzz",
	}
	// Simple schema (all fields important) to drive InputSchemaHash equality
	inputSchemaMap := map[string]any{
		"properties": map[string]any{
			"method": map[string]any{},
			"url":    map[string]any{},
		},
	}

	span := makeSpan(t, traceID, "sH", pkg, inputValueMap, inputSchemaMap, 1000)
	server.LoadSpansForTrace(traceID, []*core.Span{span})
	req := makeMockRequest(t, pkg, inputRequestMap, inputSchemaMap)

	// Ensure value hashes differ so we test schema path (priority 5)
	assert.NotEqual(t, span.InputValueHash, req.OutboundSpan.InputValueHash)
	// Ensure schema hashes equal
	assert.Equal(t, span.InputSchemaHash, req.OutboundSpan.InputSchemaHash)

	match, level, err := mm.FindBestMatchInTrace(req, traceID)
	require.NoError(t, err)
	require.NotNil(t, match)
	require.NotNil(t, level)

	assert.Equal(t, "sH", match.SpanId)
	assert.Equal(t, backend.MatchType_MATCH_TYPE_INPUT_SCHEMA_HASH, level.MatchType)
	assert.Equal(t, backend.MatchScope_MATCH_SCOPE_TRACE, level.MatchScope)
}

func TestSchemaMatchWithHttpShape_GraphQLNormalization(t *testing.T) {
	server, err := NewServer("svc")
	require.NoError(t, err)
	mm := NewMockMatcher(server)

	inputSchemaMap := map[string]any{
		"properties": map[string]any{
			"body": map[string]any{
				"properties": map[string]any{
					"query": map[string]any{},
				},
			},
		},
	}
	inputSchemaHash := utils.GenerateDeterministicHash(inputSchemaMap)

	span := makeSpan(t, "trace-gql", "sg1", "https", map[string]any{
		"body": map[string]any{
			"query": "query { user(id:1) { id   name } }",
		},
	}, inputSchemaMap, 0)

	// Same query with different whitespace; provided as JSON string body
	reqData1 := MockMatcherRequestData{
		InputValue: map[string]any{
			"body": `{"query":" query{  user(id:1){id name} } "}`,
		},
		InputSchemaHash: inputSchemaHash,
	}
	assert.True(t, mm.schemaMatchWithHttpShape(reqData1, span))

	// Different query -> should fail
	reqData2 := MockMatcherRequestData{
		InputValue: map[string]any{
			"body": map[string]any{
				"query": "query { me { id } }",
			},
		},
		InputSchemaHash: inputSchemaHash,
	}
	assert.False(t, mm.schemaMatchWithHttpShape(reqData2, span))
}

func TestFindBestMatchAcrossTraces_GlobalValueHash(t *testing.T) {
	server, err := NewServer("svc")
	require.NoError(t, err)
	mm := NewMockMatcher(server)

	pkg := "http"
	inputValueMap := map[string]any{"method": "GET", "path": "/suite"}
	inputSchemaMap := map[string]any{"properties": map[string]any{"method": map[string]any{}, "path": map[string]any{}}}

	spanA := makeSpan(t, "trace-A", "sa", pkg, inputValueMap, inputSchemaMap, 100)
	spanB := makeSpan(t, "trace-B", "sb", pkg, map[string]any{"method": "POST", "path": "/suite"}, inputSchemaMap, 200)

	server.SetSuiteSpans([]*core.Span{spanB, spanA})

	req := makeMockRequest(t, pkg, inputValueMap, inputSchemaMap)

	match, level, err := mm.FindBestMatchAcrossTraces(req, "irrelevant-trace", server.GetSuiteSpans())
	require.NoError(t, err)
	require.NotNil(t, match)
	require.NotNil(t, level)

	assert.Equal(t, "sa", match.SpanId)
	assert.Equal(t, backend.MatchType_MATCH_TYPE_INPUT_VALUE_HASH, level.MatchType)
	assert.Equal(t, backend.MatchScope_MATCH_SCOPE_GLOBAL, level.MatchScope)
}

func TestReducedInputSchemaHash_WithHttpShape(t *testing.T) {
	server, err := NewServer("svc")
	require.NoError(t, err)
	mm := NewMockMatcher(server)

	traceID := "trace-7"
	pkg := "http"

	// Schema includes a low-importance property 'ignored'
	inputSchemaMap := map[string]any{
		"properties": map[string]any{
			"method":  map[string]any{},
			"path":    map[string]any{},
			"ignored": map[string]any{"matchImportance": 0.0},
		},
	}

	inputRequestMap := map[string]any{"method": "GET", "path": "/reduced", "ignored": "X"}
	inputValueMap := map[string]any{"method": "GET", "path": "/reduced", "ignored": "Y"}

	// Ensure schema hashes differ only if reduced (reduced should match)
	span := makeSpan(t, traceID, "sRS", pkg, inputValueMap, inputSchemaMap, 1000)
	server.LoadSpansForTrace(traceID, []*core.Span{span})
	req := makeMockRequest(t, pkg, inputRequestMap, inputSchemaMap)

	// Force non-match for value hashes
	assert.NotEqual(t, span.InputValueHash, req.OutboundSpan.InputValueHash)

	// Reduced schema hash should align; function under test computes reduced from schema itself
	result := mm.findUnusedSpanByReducedInputSchemaHash(req, []*core.Span{span})
	require.NotNil(t, result.span)
	assert.Equal(t, "sRS", result.span.SpanId)
}

// TestFindBestMatchInTrace_SimilarityScoring_PicksClosestMatch tests that when multiple spans
// match on schema, the matcher picks the one with the closest input value using Levenshtein similarity
func TestFindBestMatchInTrace_SimilarityScoring_PicksClosestMatch(t *testing.T) {
	server, err := NewServer("svc")
	require.NoError(t, err)
	mm := NewMockMatcher(server)

	traceID := "trace-similarity"
	pkg := "postgres"

	// Schema that matches for SQL queries
	inputSchemaMap := map[string]any{
		"properties": map[string]any{
			"query":      map[string]any{},
			"parameters": map[string]any{},
		},
	}

	// Request: UPDATE sessions with specific parameters
	requestValueMap := map[string]any{
		"query": "update \"tests\" set \"expires_at\" = $1, \"updated_at\" = $2 where \"tests\".\"id\" = $3",
		"parameters": []any{
			"2025-10-01T05:24:30.809Z",
			"2025-10-01T01:59:47.260Z",
			"random-id-1",
		},
	}

	// Span 1: Very different query (SELECT from roles) - should NOT be picked
	span1ValueMap := map[string]any{
		"query": "select \"id\", \"name\", \"repo_id\", \"created_at\", \"updated_at\", \"random_column\" from \"users\" \"usersTable\" where (\"usersTable\".\"repo_id\" = $1 and \"usersTable\".\"random_column\" = $2) limit $3",
		"parameters": []any{
			"results_viewer",
			"random-id-2",
			1,
		},
	}

	// Span 2: Almost identical query (UPDATE sessions with different timestamps) - SHOULD be picked
	span2ValueMap := map[string]any{
		"query": "update \"tests\" set \"expires_at\" = $1, \"updated_at\" = $2 where \"tests\".\"id\" = $3",
		"parameters": []any{
			"2025-10-01T14:20:17.076Z",
			"2025-10-01T06:20:17.077Z",
			"random-id-1",
		},
	}

	span1 := makeSpan(t, traceID, "span-different", pkg, span1ValueMap, inputSchemaMap, 1000)
	span2 := makeSpan(t, traceID, "span-similar", pkg, span2ValueMap, inputSchemaMap, 2000)

	// Load spans in reverse order to ensure timestamp isn't the primary factor
	server.LoadSpansForTrace(traceID, []*core.Span{span1, span2})

	req := makeMockRequest(t, pkg, requestValueMap, inputSchemaMap)

	// Both spans have same schema but different values
	match, level, err := mm.FindBestMatchInTrace(req, traceID)
	require.NoError(t, err)
	require.NotNil(t, match)
	require.NotNil(t, level)

	// Should pick span2 because it has much higher similarity (same query, similar params)
	assert.Equal(t, "span-similar", match.SpanId, "Should pick the span with more similar input value")

	// Should be a schema hash match (Priority 5)
	assert.Equal(t, backend.MatchType_MATCH_TYPE_INPUT_SCHEMA_HASH, level.MatchType)

	// Match description should include similarity scores
	assert.Contains(t, level.MatchDescription, "similarity:")
}

// TestFindBestMatchInTrace_SimilarityScoring_TiebreakByTimestamp tests that when similarity
// scores are identical, the oldest span is picked
func TestFindBestMatchInTrace_SimilarityScoring_TiebreakByTimestamp(t *testing.T) {
	server, err := NewServer("svc")
	require.NoError(t, err)
	mm := NewMockMatcher(server)

	traceID := "trace-tiebreak"
	pkg := "http"

	inputSchemaMap := map[string]any{
		"properties": map[string]any{
			"method": map[string]any{},
			"path":   map[string]any{},
		},
	}

	requestValueMap := map[string]any{
		"method": "POST",
		"path":   "/api/users",
	}

	// Both spans have identical values (perfect similarity score = 1.0)
	spanValueMap1 := map[string]any{
		"method": "POST",
		"path":   "/api/users",
	}

	spanValueMap2 := map[string]any{
		"method": "POST",
		"path":   "/api/users",
	}

	// Create spans with different timestamps
	spanOlder := makeSpan(t, traceID, "span-older", pkg, spanValueMap1, inputSchemaMap, 1000)
	spanNewer := makeSpan(t, traceID, "span-newer", pkg, spanValueMap2, inputSchemaMap, 3000)

	// Load in random order
	server.LoadSpansForTrace(traceID, []*core.Span{spanNewer, spanOlder})

	req := makeMockRequest(t, pkg, requestValueMap, inputSchemaMap)

	match, level, err := mm.FindBestMatchInTrace(req, traceID)
	require.NoError(t, err)
	require.NotNil(t, match)
	require.NotNil(t, level)

	// Should pick the older span when similarity is identical
	assert.Equal(t, "span-older", match.SpanId, "Should pick oldest span when similarity scores are identical")
}

// TestFindBestMatchInTrace_SimilarityScoring_NestedStructures tests similarity scoring
// with nested maps and arrays
func TestFindBestMatchInTrace_SimilarityScoring_NestedStructures(t *testing.T) {
	server, err := NewServer("svc")
	require.NoError(t, err)
	mm := NewMockMatcher(server)

	traceID := "trace-nested"
	pkg := "http"

	inputSchemaMap := map[string]any{
		"properties": map[string]any{
			"body": map[string]any{},
		},
	}

	// Request with nested structure
	requestValueMap := map[string]any{
		"body": map[string]any{
			"user": map[string]any{
				"name":  "Alice",
				"email": "alice@example.com",
				"tags":  []any{"admin", "active"},
			},
		},
	}

	// Span 1: Completely different structure
	span1ValueMap := map[string]any{
		"body": map[string]any{
			"product": map[string]any{
				"id":    "123",
				"price": 99.99,
			},
		},
	}

	// Span 2: Very similar structure (same user with slightly different email)
	span2ValueMap := map[string]any{
		"body": map[string]any{
			"user": map[string]any{
				"name":  "Alice",
				"email": "alice@other.com",
				"tags":  []any{"admin", "active"},
			},
		},
	}

	span1 := makeSpan(t, traceID, "span-product", pkg, span1ValueMap, inputSchemaMap, 1000)
	span2 := makeSpan(t, traceID, "span-user", pkg, span2ValueMap, inputSchemaMap, 2000)

	server.LoadSpansForTrace(traceID, []*core.Span{span1, span2})

	req := makeMockRequest(t, pkg, requestValueMap, inputSchemaMap)

	match, level, err := mm.FindBestMatchInTrace(req, traceID)
	require.NoError(t, err)
	require.NotNil(t, match)
	require.NotNil(t, level)

	// Should pick span2 because nested structure is much more similar
	assert.Equal(t, "span-user", match.SpanId, "Should pick the span with more similar nested structure")
}

// TestFindBestMatchInTrace_SimilarityScoring_ReturnsTop5Candidates tests that when multiple
// candidates exist, the top 5 alternatives are returned with their scores
func TestFindBestMatchInTrace_SimilarityScoring_ReturnsTop5Candidates(t *testing.T) {
	server, err := NewServer("svc")
	require.NoError(t, err)
	mm := NewMockMatcher(server)

	traceID := "trace-top5"
	pkg := "postgres"

	inputSchemaMap := map[string]any{
		"properties": map[string]any{
			"query":      map[string]any{},
			"parameters": map[string]any{},
		},
	}

	// Request: UPDATE query
	requestValueMap := map[string]any{
		"query":      "UPDATE users SET name = $1 WHERE id = $2",
		"parameters": []any{"Alice", 123},
	}

	// Create 7 spans with varying similarity to the request
	// All have same schema but different query content
	spans := []*core.Span{
		makeSpan(t, traceID, "span-best", pkg, map[string]any{
			"query":      "UPDATE users SET name = $1 WHERE id = $2",
			"parameters": []any{"Bob", 123},
		}, inputSchemaMap, 1000),
		makeSpan(t, traceID, "span-second", pkg, map[string]any{
			"query":      "UPDATE users SET name = $1 WHERE id = $3",
			"parameters": []any{"Alice", 456},
		}, inputSchemaMap, 2000),
		makeSpan(t, traceID, "span-third", pkg, map[string]any{
			"query":      "UPDATE users SET email = $1 WHERE id = $2",
			"parameters": []any{"test@example.com", 123},
		}, inputSchemaMap, 3000),
		makeSpan(t, traceID, "span-fourth", pkg, map[string]any{
			"query":      "UPDATE posts SET title = $1 WHERE id = $2",
			"parameters": []any{"New Title", 123},
		}, inputSchemaMap, 4000),
		makeSpan(t, traceID, "span-fifth", pkg, map[string]any{
			"query":      "INSERT INTO users (name) VALUES ($1)",
			"parameters": []any{"Alice"},
		}, inputSchemaMap, 5000),
		makeSpan(t, traceID, "span-sixth", pkg, map[string]any{
			"query":      "SELECT * FROM users WHERE id = $1",
			"parameters": []any{123},
		}, inputSchemaMap, 6000),
		makeSpan(t, traceID, "span-seventh", pkg, map[string]any{
			"query":      "DELETE FROM users WHERE id = $1",
			"parameters": []any{999},
		}, inputSchemaMap, 7000),
	}

	server.LoadSpansForTrace(traceID, spans)

	req := makeMockRequest(t, pkg, requestValueMap, inputSchemaMap)

	match, level, err := mm.FindBestMatchInTrace(req, traceID)
	require.NoError(t, err)
	require.NotNil(t, match)
	require.NotNil(t, level)

	// Should pick the best match (very similar query)
	assert.Equal(t, "span-best", match.SpanId, "Should pick the best match")

	// Should have similarity score populated (schema hash match with multiple candidates)
	require.NotNil(t, level.SimilarityScore, "Similarity score should be populated for multiple matches")
	assert.Greater(t, *level.SimilarityScore, float32(0.7), "Best match should have high similarity score")

	// Should have top 5 candidates (excluding the best match)
	require.NotNil(t, level.TopCandidates, "Top candidates should be populated")
	assert.Len(t, level.TopCandidates, 5, "Should return exactly 5 top candidates (excluding the best)")

	// Verify candidates are sorted by score (highest first)
	for i := 0; i < len(level.TopCandidates)-1; i++ {
		assert.GreaterOrEqual(t, level.TopCandidates[i].Score, level.TopCandidates[i+1].Score,
			"Candidates should be sorted by score (highest first)")
	}

	// Verify the top candidate is the second-best match
	assert.Equal(t, "span-second", level.TopCandidates[0].SpanId, "Top candidate should be the second-best match")

	t.Logf("Best match: %s (score: %.4f)", match.SpanId, *level.SimilarityScore)
	for i, candidate := range level.TopCandidates {
		t.Logf("Candidate #%d: %s (score: %.4f)", i+1, candidate.SpanId, candidate.Score)
	}
}

// TestFindBestMatchInTrace_SimilarityScoring_DeepNesting tests that similarity scoring works
// correctly beyond depth 5 by stringifying deeply nested structures
func TestFindBestMatchInTrace_SimilarityScoring_DeepNesting(t *testing.T) {
	server, err := NewServer("svc")
	require.NoError(t, err)
	mm := NewMockMatcher(server)

	traceID := "trace-deep"
	pkg := "http"

	inputSchemaMap := map[string]any{
		"properties": map[string]any{
			"body": map[string]any{},
		},
	}

	// Create a deeply nested structure (depth > 5)
	// Level 1 -> 2 -> 3 -> 4 -> 5 -> 6 -> 7 (beyond max depth)
	deepStructure := map[string]any{
		"body": map[string]any{
			"level1": map[string]any{
				"level2": map[string]any{
					"level3": map[string]any{
						"level4": map[string]any{
							"level5": map[string]any{
								"level6": map[string]any{
									"level7": map[string]any{
										"data": "target-value-123",
										"meta": "important-info",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Request with deep nesting
	requestValueMap := deepStructure

	// Span 1: Different value at depth 7 (should stringify and compare)
	span1Deep := map[string]any{
		"body": map[string]any{
			"level1": map[string]any{
				"level2": map[string]any{
					"level3": map[string]any{
						"level4": map[string]any{
							"level5": map[string]any{
								"level6": map[string]any{
									"level7": map[string]any{
										"data": "completely-different-xyz",
										"meta": "other-data",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Span 2: Very similar value at depth 7 (should stringify and show high similarity)
	span2Deep := map[string]any{
		"body": map[string]any{
			"level1": map[string]any{
				"level2": map[string]any{
					"level3": map[string]any{
						"level4": map[string]any{
							"level5": map[string]any{
								"level6": map[string]any{
									"level7": map[string]any{
										"data": "target-value-124", // Very similar
										"meta": "important-info",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	span1 := makeSpan(t, traceID, "span-deep-different", pkg, span1Deep, inputSchemaMap, 1000)
	span2 := makeSpan(t, traceID, "span-deep-similar", pkg, span2Deep, inputSchemaMap, 2000)

	server.LoadSpansForTrace(traceID, []*core.Span{span1, span2})

	req := makeMockRequest(t, pkg, requestValueMap, inputSchemaMap)

	match, level, err := mm.FindBestMatchInTrace(req, traceID)
	require.NoError(t, err)
	require.NotNil(t, match)
	require.NotNil(t, level)

	// Should pick span2 because even at depth > 5, stringification shows higher similarity
	assert.Equal(t, "span-deep-similar", match.SpanId, "Should pick the span with more similar deep structure")

	// Log similarity scores to verify depth > 5 comparison works
	require.NotNil(t, level.SimilarityScore, "Similarity score should be populated")
	t.Logf("Best match: %s (score: %.4f)", match.SpanId, *level.SimilarityScore)
	if len(level.TopCandidates) > 0 {
		t.Logf("Next best: %s (score: %.4f)", level.TopCandidates[0].SpanId, level.TopCandidates[0].Score)
	}
}
