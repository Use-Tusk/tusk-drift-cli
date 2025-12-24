package runner

import (
	"testing"
	"time"

	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"

	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
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

func makeSpan(t *testing.T, traceID, spanID, pkg string, inputValueMap map[string]any, inputSchema *core.JsonSchema, tsMs int64) *core.Span {
	var inputValueStruct *structpb.Struct
	var inputValueHash, inputSchemaHash string
	if inputValueMap != nil {
		inputValueStruct = toStruct(t, inputValueMap)
		inputValueHash = utils.GenerateDeterministicHash(inputValueMap)
	}
	if inputSchema != nil {
		inputSchemaHash = utils.GenerateDeterministicHash(inputSchema)
	}
	return &core.Span{
		TraceId:         traceID,
		SpanId:          spanID,
		PackageName:     pkg,
		InputValue:      inputValueStruct,
		InputSchema:     inputSchema,
		InputValueHash:  inputValueHash,
		InputSchemaHash: inputSchemaHash,
		Timestamp:       unixMsToTimestamp(tsMs),
	}
}

func makeMockRequest(t *testing.T, pkg string, inputValueMap map[string]any, inputSchema *core.JsonSchema) *core.GetMockRequest {
	var inputValueStruct *structpb.Struct
	var inputValueHash, inputSchemaHash string
	if inputValueMap != nil {
		inputValueStruct = toStruct(t, inputValueMap)
		inputValueHash = utils.GenerateDeterministicHash(inputValueMap)
	}
	if inputSchema != nil {
		inputSchemaHash = utils.GenerateDeterministicHash(inputSchema)
	}
	return &core.GetMockRequest{
		OutboundSpan: &core.Span{
			PackageName:     pkg,
			InputValue:      inputValueStruct,
			InputSchema:     inputSchema,
			InputValueHash:  inputValueHash,
			InputSchemaHash: inputSchemaHash,
		},
		Operation: "GET",
	}
}

func TestFindBestMatchWithTracePriority_InputValueHash_PrefersUnusedOldest(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	mm := NewMockMatcher(server)

	traceID := "trace-1"
	pkg := "http"

	inputValueMap := map[string]any{"method": "GET", "path": "/users"}
	var inputSchema *core.JsonSchema

	spanOld := makeSpan(t, traceID, "s1", pkg, inputValueMap, inputSchema, 1000)
	spanNew := makeSpan(t, traceID, "s2", pkg, inputValueMap, inputSchema, 2000)
	server.LoadSpansForTrace(traceID, []*core.Span{spanNew, spanOld})

	req := makeMockRequest(t, pkg, inputValueMap, inputSchema)

	match, level, err := mm.FindBestMatchWithTracePriority(req, traceID)
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

	match2, level2, err := mm.FindBestMatchWithTracePriority(req, traceID)
	require.NoError(t, err)
	require.NotNil(t, match2)
	require.NotNil(t, level2)
	assert.Equal(t, "s2", match2.SpanId)
	assert.Equal(t, backend.MatchType_MATCH_TYPE_INPUT_VALUE_HASH, level2.MatchType)
	assert.Equal(t, backend.MatchScope_MATCH_SCOPE_TRACE, level2.MatchScope)

	// Both used now; should fall back to used (earliest)
	match3, level3, err := mm.FindBestMatchWithTracePriority(req, traceID)
	require.NoError(t, err)
	require.NotNil(t, match3)
	require.NotNil(t, level3)
	assert.Equal(t, "s1", match3.SpanId)
	assert.Equal(t, backend.MatchType_MATCH_TYPE_INPUT_VALUE_HASH, level3.MatchType)
	assert.Equal(t, backend.MatchScope_MATCH_SCOPE_TRACE, level3.MatchScope)
}

func TestFindBestMatchWithTracePriority_ReducedInputValueHash_MatchesWhenDirectHashDiffers(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	mm := NewMockMatcher(server)

	traceID := "trace-2"
	pkg := "http"

	// token has matchImportance 0; differing token values should not affect reduced hash
	inputRequestMap := map[string]any{"method": "GET", "path": "/a", "token": "alpha"}
	inputValueMap := map[string]any{"method": "GET", "path": "/a", "token": "beta"}

	matchImportanceZero := 0.0
	inputSchema := &core.JsonSchema{
		Properties: map[string]*core.JsonSchema{
			"method": {},
			"path":   {},
			"token":  {MatchImportance: &matchImportanceZero},
		},
	}

	span := makeSpan(t, traceID, "sR", pkg, inputValueMap, inputSchema, 1000)
	server.LoadSpansForTrace(traceID, []*core.Span{span})
	req := makeMockRequest(t, pkg, inputRequestMap, inputSchema)

	// Sanity: direct hashes differ
	assert.NotEqual(t, utils.GenerateDeterministicHash(inputRequestMap), utils.GenerateDeterministicHash(inputValueMap))

	match, level, err := mm.FindBestMatchWithTracePriority(req, traceID)
	require.NoError(t, err)
	require.NotNil(t, match)
	require.NotNil(t, level)

	assert.Equal(t, "sR", match.SpanId)
	assert.Equal(t, backend.MatchType_MATCH_TYPE_INPUT_VALUE_HASH_REDUCED_SCHEMA, level.MatchType)
	assert.Equal(t, backend.MatchScope_MATCH_SCOPE_TRACE, level.MatchScope)
}

func TestFindBestMatchWithTracePriority_InputSchemaHash_WithHTTPShape(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
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
	inputSchema := &core.JsonSchema{
		Properties: map[string]*core.JsonSchema{
			"method": {},
			"url":    {},
		},
	}

	span := makeSpan(t, traceID, "sH", pkg, inputValueMap, inputSchema, 1000)
	server.LoadSpansForTrace(traceID, []*core.Span{span})
	req := makeMockRequest(t, pkg, inputRequestMap, inputSchema)

	// Ensure value hashes differ so we test schema path (priority 5)
	assert.NotEqual(t, span.InputValueHash, req.OutboundSpan.InputValueHash)
	// Ensure schema hashes equal
	assert.Equal(t, span.InputSchemaHash, req.OutboundSpan.InputSchemaHash)

	match, level, err := mm.FindBestMatchWithTracePriority(req, traceID)
	require.NoError(t, err)
	require.NotNil(t, match)
	require.NotNil(t, level)

	assert.Equal(t, "sH", match.SpanId)
	assert.Equal(t, backend.MatchType_MATCH_TYPE_INPUT_SCHEMA_HASH, level.MatchType)
	assert.Equal(t, backend.MatchScope_MATCH_SCOPE_TRACE, level.MatchScope)
}

func TestSchemaMatchWithHttpShape_GraphQLNormalization(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	mm := NewMockMatcher(server)

	inputSchema := &core.JsonSchema{
		Properties: map[string]*core.JsonSchema{
			"body": {
				Properties: map[string]*core.JsonSchema{
					"query": {},
				},
			},
		},
	}
	inputSchemaHash := utils.GenerateDeterministicHash(inputSchema)

	span := makeSpan(t, "trace-gql", "sg1", "https", map[string]any{
		"body": map[string]any{
			"query": "query { user(id:1) { id   name } }",
		},
	}, inputSchema, 0)

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
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	mm := NewMockMatcher(server)

	pkg := "http"
	inputValueMap := map[string]any{"method": "GET", "path": "/suite"}
	inputSchema := &core.JsonSchema{
		Properties: map[string]*core.JsonSchema{
			"method": {},
			"path":   {},
		},
	}

	spanA := makeSpan(t, "trace-A", "sa", pkg, inputValueMap, inputSchema, 100)
	spanB := makeSpan(t, "trace-B", "sb", pkg, map[string]any{"method": "POST", "path": "/suite"}, inputSchema, 200)

	server.SetSuiteSpans([]*core.Span{spanB, spanA})

	req := makeMockRequest(t, pkg, inputValueMap, inputSchema)

	match, level, err := mm.FindBestMatchAcrossTraces(req, "irrelevant-trace", server.GetSuiteSpans())
	require.NoError(t, err)
	require.NotNil(t, match)
	require.NotNil(t, level)

	assert.Equal(t, "sa", match.SpanId)
	assert.Equal(t, backend.MatchType_MATCH_TYPE_INPUT_VALUE_HASH, level.MatchType)
	assert.Equal(t, backend.MatchScope_MATCH_SCOPE_GLOBAL, level.MatchScope)
}

func TestFindBestMatchAcrossTraces_GlobalSchemaHash(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	mm := NewMockMatcher(server)

	pkg := "http"
	inputSchema := &core.JsonSchema{
		Properties: map[string]*core.JsonSchema{
			"method": {},
			"path":   {},
		},
	}

	// Request value differs from span value, but schema matches
	requestValueMap := map[string]any{"method": "GET", "path": "/users/123"}
	spanValueMap := map[string]any{"method": "GET", "path": "/users/456"}

	spanA := makeSpan(t, "trace-A", "sa", pkg, spanValueMap, inputSchema, 100)
	spanA.IsPreAppStart = true

	server.SetSuiteSpans([]*core.Span{spanA})

	req := makeMockRequest(t, pkg, requestValueMap, inputSchema)
	req.OutboundSpan.IsPreAppStart = true

	// Sanity: value hashes differ
	assert.NotEqual(t, spanA.InputValueHash, req.OutboundSpan.InputValueHash)
	// Schema hashes should match
	assert.Equal(t, spanA.InputSchemaHash, req.OutboundSpan.InputSchemaHash)

	match, level, err := mm.FindBestMatchAcrossTraces(req, "irrelevant-trace", server.GetSuiteSpans())
	require.NoError(t, err)
	require.NotNil(t, match)
	require.NotNil(t, level)

	assert.Equal(t, "sa", match.SpanId)
	assert.Equal(t, backend.MatchType_MATCH_TYPE_INPUT_SCHEMA_HASH, level.MatchType)
	assert.Equal(t, backend.MatchScope_MATCH_SCOPE_GLOBAL, level.MatchScope)
}

func TestFindBestMatchAcrossTraces_GlobalReducedSchemaHash(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	mm := NewMockMatcher(server)

	pkg := "http"
	matchImportanceZero := 0.0

	// Request schema has an extra low-importance field
	requestSchema := &core.JsonSchema{
		Properties: map[string]*core.JsonSchema{
			"method":  {},
			"path":    {},
			"ignored": {MatchImportance: &matchImportanceZero},
		},
	}

	// Span schema is missing the low-importance field (reduced schemas should match)
	spanSchema := &core.JsonSchema{
		Properties: map[string]*core.JsonSchema{
			"method": {},
			"path":   {},
		},
	}

	requestValueMap := map[string]any{"method": "GET", "path": "/a", "ignored": "X"}
	spanValueMap := map[string]any{"method": "GET", "path": "/b"}

	spanA := makeSpan(t, "trace-A", "sa", pkg, spanValueMap, spanSchema, 100)
	spanA.IsPreAppStart = true

	server.SetSuiteSpans([]*core.Span{spanA})

	req := makeMockRequest(t, pkg, requestValueMap, requestSchema)
	req.OutboundSpan.IsPreAppStart = true

	// Sanity: full schema hashes differ
	assert.NotEqual(t, spanA.InputSchemaHash, req.OutboundSpan.InputSchemaHash)

	match, level, err := mm.FindBestMatchAcrossTraces(req, "irrelevant-trace", server.GetSuiteSpans())
	require.NoError(t, err)
	require.NotNil(t, match)
	require.NotNil(t, level)

	assert.Equal(t, "sa", match.SpanId)
	assert.Equal(t, backend.MatchType_MATCH_TYPE_INPUT_SCHEMA_HASH_REDUCED_SCHEMA, level.MatchType)
	assert.Equal(t, backend.MatchScope_MATCH_SCOPE_GLOBAL, level.MatchScope)
}

func TestFindBestMatchAcrossTraces_PrefersValueHashOverSchemaHash(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	mm := NewMockMatcher(server)

	pkg := "http"
	inputSchema := &core.JsonSchema{
		Properties: map[string]*core.JsonSchema{
			"method": {},
			"path":   {},
		},
	}

	requestValueMap := map[string]any{"method": "GET", "path": "/users"}
	differentValueMap := map[string]any{"method": "GET", "path": "/posts"}

	// spanExact has exact value match (Priority 10)
	spanExact := makeSpan(t, "trace-A", "exact", pkg, requestValueMap, inputSchema, 200)
	// spanSchema only has schema match (Priority 12)
	spanSchema := makeSpan(t, "trace-B", "schema-only", pkg, differentValueMap, inputSchema, 100)

	server.SetSuiteSpans([]*core.Span{spanSchema, spanExact})

	req := makeMockRequest(t, pkg, requestValueMap, inputSchema)

	match, level, err := mm.FindBestMatchAcrossTraces(req, "irrelevant-trace", server.GetSuiteSpans())
	require.NoError(t, err)
	require.NotNil(t, match)
	require.NotNil(t, level)

	// Should pick value hash match (Priority 10) over schema hash match (Priority 12)
	assert.Equal(t, "exact", match.SpanId)
	assert.Equal(t, backend.MatchType_MATCH_TYPE_INPUT_VALUE_HASH, level.MatchType)
}

func TestFindBestMatchAcrossTraces_NonPreAppStart_DoesNotMatchOnSchema(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	mm := NewMockMatcher(server)

	pkg := "http"
	inputSchema := &core.JsonSchema{
		Properties: map[string]*core.JsonSchema{
			"method": {},
			"path":   {},
		},
	}

	// Request and span have same schema but different values
	requestValueMap := map[string]any{"method": "GET", "path": "/users/123"}
	spanValueMap := map[string]any{"method": "GET", "path": "/users/456"}

	spanA := makeSpan(t, "trace-A", "sa", pkg, spanValueMap, inputSchema, 100)
	spanA.IsPreAppStart = false

	server.SetSuiteSpans([]*core.Span{spanA})

	req := makeMockRequest(t, pkg, requestValueMap, inputSchema)
	req.OutboundSpan.IsPreAppStart = false

	// Should not match - schema matching is disabled for non-pre-app-start
	match, _, err := mm.FindBestMatchAcrossTraces(req, "irrelevant-trace", server.GetSuiteSpans())
	require.Error(t, err)
	require.Nil(t, match)
}

func TestFindBestMatchAcrossTraces_SchemaHash_PreAppStartFiltering(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	mm := NewMockMatcher(server)

	pkg := "http"
	inputSchema := &core.JsonSchema{
		Properties: map[string]*core.JsonSchema{
			"method": {},
			"path":   {},
		},
	}

	requestValueMap := map[string]any{"method": "GET", "path": "/init"}
	spanValueMap := map[string]any{"method": "GET", "path": "/init-other"}

	// Create a pre-app-start span
	spanPreApp := makeSpan(t, "trace-A", "pre-app", pkg, spanValueMap, inputSchema, 100)
	spanPreApp.IsPreAppStart = true

	// Create a non-pre-app-start span
	spanNormal := makeSpan(t, "trace-B", "normal", pkg, spanValueMap, inputSchema, 200)
	spanNormal.IsPreAppStart = false

	server.SetSuiteSpans([]*core.Span{spanPreApp, spanNormal})

	// Request is pre-app-start
	req := makeMockRequest(t, pkg, requestValueMap, inputSchema)
	req.OutboundSpan.IsPreAppStart = true

	match, level, err := mm.FindBestMatchAcrossTraces(req, "irrelevant-trace", server.GetSuiteSpans())
	require.NoError(t, err)
	require.NotNil(t, match)
	require.NotNil(t, level)

	// Should only match the pre-app-start span
	assert.Equal(t, "pre-app", match.SpanId)
	assert.Equal(t, backend.MatchType_MATCH_TYPE_INPUT_SCHEMA_HASH, level.MatchType)
	assert.Equal(t, backend.MatchScope_MATCH_SCOPE_GLOBAL, level.MatchScope)
}

func TestReducedInputSchemaHash_WithHttpShape(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	mm := NewMockMatcher(server)

	traceID := "trace-7"
	pkg := "http"

	// Schema includes a low-importance property 'ignored'
	matchImportanceZero := 0.0
	inputSchema := &core.JsonSchema{
		Properties: map[string]*core.JsonSchema{
			"method":  {},
			"path":    {},
			"ignored": {MatchImportance: &matchImportanceZero},
		},
	}

	inputRequestMap := map[string]any{"method": "GET", "path": "/reduced", "ignored": "X"}
	inputValueMap := map[string]any{"method": "GET", "path": "/reduced", "ignored": "Y"}

	// Ensure schema hashes differ only if reduced (reduced should match)
	span := makeSpan(t, traceID, "sRS", pkg, inputValueMap, inputSchema, 1000)
	server.LoadSpansForTrace(traceID, []*core.Span{span})
	req := makeMockRequest(t, pkg, inputRequestMap, inputSchema)

	// Force non-match for value hashes
	assert.NotEqual(t, span.InputValueHash, req.OutboundSpan.InputValueHash)

	// Reduced schema hash should align; function under test computes reduced from schema itself
	result := mm.findUnusedSpanByReducedInputSchemaHash(req, []*core.Span{span}, traceID)
	require.NotNil(t, result.span)
	assert.Equal(t, "sRS", result.span.SpanId)
}

// TestFindBestMatchWithTracePriority_SimilarityScoring_PicksClosestMatch tests that when multiple spans
// match on schema, the matcher picks the one with the closest input value using Levenshtein similarity
func TestFindBestMatchWithTracePriority_SimilarityScoring_PicksClosestMatch(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	mm := NewMockMatcher(server)

	traceID := "trace-similarity"
	pkg := "postgres"

	// Schema that matches for SQL queries
	inputSchema := &core.JsonSchema{
		Properties: map[string]*core.JsonSchema{
			"query":      {},
			"parameters": {},
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

	span1 := makeSpan(t, traceID, "span-different", pkg, span1ValueMap, inputSchema, 1000)
	span2 := makeSpan(t, traceID, "span-similar", pkg, span2ValueMap, inputSchema, 2000)

	// Load spans in reverse order to ensure timestamp isn't the primary factor
	server.LoadSpansForTrace(traceID, []*core.Span{span1, span2})

	req := makeMockRequest(t, pkg, requestValueMap, inputSchema)

	// Both spans have same schema but different values
	match, level, err := mm.FindBestMatchWithTracePriority(req, traceID)
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

// TestFindBestMatchWithTracePriority_SimilarityScoring_TiebreakByTimestamp tests that when similarity
// scores are identical, the oldest span is picked
func TestFindBestMatchWithTracePriority_SimilarityScoring_TiebreakByTimestamp(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	mm := NewMockMatcher(server)

	traceID := "trace-tiebreak"
	pkg := "http"

	inputSchema := &core.JsonSchema{
		Properties: map[string]*core.JsonSchema{
			"method": {},
			"path":   {},
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
	spanOlder := makeSpan(t, traceID, "span-older", pkg, spanValueMap1, inputSchema, 1000)
	spanNewer := makeSpan(t, traceID, "span-newer", pkg, spanValueMap2, inputSchema, 3000)

	// Load in random order
	server.LoadSpansForTrace(traceID, []*core.Span{spanNewer, spanOlder})

	req := makeMockRequest(t, pkg, requestValueMap, inputSchema)

	match, level, err := mm.FindBestMatchWithTracePriority(req, traceID)
	require.NoError(t, err)
	require.NotNil(t, match)
	require.NotNil(t, level)

	// Should pick the older span when similarity is identical
	assert.Equal(t, "span-older", match.SpanId, "Should pick oldest span when similarity scores are identical")
}

// TestFindBestMatchWithTracePriority_SimilarityScoring_NestedStructures tests similarity scoring
// with nested maps and arrays
func TestFindBestMatchWithTracePriority_SimilarityScoring_NestedStructures(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	mm := NewMockMatcher(server)

	traceID := "trace-nested"
	pkg := "http"

	inputSchema := &core.JsonSchema{
		Properties: map[string]*core.JsonSchema{
			"body": {},
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

	span1 := makeSpan(t, traceID, "span-product", pkg, span1ValueMap, inputSchema, 1000)
	span2 := makeSpan(t, traceID, "span-user", pkg, span2ValueMap, inputSchema, 2000)

	server.LoadSpansForTrace(traceID, []*core.Span{span1, span2})

	req := makeMockRequest(t, pkg, requestValueMap, inputSchema)

	match, level, err := mm.FindBestMatchWithTracePriority(req, traceID)
	require.NoError(t, err)
	require.NotNil(t, match)
	require.NotNil(t, level)

	// Should pick span2 because nested structure is much more similar
	assert.Equal(t, "span-user", match.SpanId, "Should pick the span with more similar nested structure")
}

// TestFindBestMatchWithTracePriority_SimilarityScoring_ReturnsTop5Candidates tests that when multiple
// candidates exist, the top 5 alternatives are returned with their scores
func TestFindBestMatchWithTracePriority_SimilarityScoring_ReturnsTop5Candidates(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	mm := NewMockMatcher(server)

	traceID := "trace-top5"
	pkg := "postgres"

	inputSchema := &core.JsonSchema{
		Properties: map[string]*core.JsonSchema{
			"query":      {},
			"parameters": {},
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
		}, inputSchema, 1000),
		makeSpan(t, traceID, "span-second", pkg, map[string]any{
			"query":      "UPDATE users SET name = $1 WHERE id = $3",
			"parameters": []any{"Alice", 456},
		}, inputSchema, 2000),
		makeSpan(t, traceID, "span-third", pkg, map[string]any{
			"query":      "UPDATE users SET email = $1 WHERE id = $2",
			"parameters": []any{"test@example.com", 123},
		}, inputSchema, 3000),
		makeSpan(t, traceID, "span-fourth", pkg, map[string]any{
			"query":      "UPDATE posts SET title = $1 WHERE id = $2",
			"parameters": []any{"New Title", 123},
		}, inputSchema, 4000),
		makeSpan(t, traceID, "span-fifth", pkg, map[string]any{
			"query":      "INSERT INTO users (name) VALUES ($1)",
			"parameters": []any{"Alice"},
		}, inputSchema, 5000),
		makeSpan(t, traceID, "span-sixth", pkg, map[string]any{
			"query":      "SELECT * FROM users WHERE id = $1",
			"parameters": []any{123},
		}, inputSchema, 6000),
		makeSpan(t, traceID, "span-seventh", pkg, map[string]any{
			"query":      "DELETE FROM users WHERE id = $1",
			"parameters": []any{999},
		}, inputSchema, 7000),
	}

	server.LoadSpansForTrace(traceID, spans)

	req := makeMockRequest(t, pkg, requestValueMap, inputSchema)

	match, level, err := mm.FindBestMatchWithTracePriority(req, traceID)
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

// TestFindBestMatchWithTracePriority_SimilarityScoring_DeepNesting tests that similarity scoring works
// correctly beyond depth 5 by stringifying deeply nested structures
func TestFindBestMatchWithTracePriority_SimilarityScoring_DeepNesting(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	mm := NewMockMatcher(server)

	traceID := "trace-deep"
	pkg := "http"

	inputSchema := &core.JsonSchema{
		Properties: map[string]*core.JsonSchema{
			"body": {},
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

	span1 := makeSpan(t, traceID, "span-deep-different", pkg, span1Deep, inputSchema, 1000)
	span2 := makeSpan(t, traceID, "span-deep-similar", pkg, span2Deep, inputSchema, 2000)

	server.LoadSpansForTrace(traceID, []*core.Span{span1, span2})

	req := makeMockRequest(t, pkg, requestValueMap, inputSchema)

	match, level, err := mm.FindBestMatchWithTracePriority(req, traceID)
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

// TestFindBestMatchWithTracePriority_SuiteValueHash_MatchesAcrossTraces tests that Priority 5
// (suite-wide value hash) finds matches from other traces when the current trace has no match
// This test uses validation mode where all suite spans are searchable
func TestFindBestMatchWithTracePriority_SuiteValueHash_MatchesAcrossTraces(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	server.SetAllowSuiteWideMatching(true) // Enable suite-wide matching to search all suite spans
	mm := NewMockMatcher(server)

	pkg := "postgres"
	inputSchema := &core.JsonSchema{
		Properties: map[string]*core.JsonSchema{
			"query": {},
		},
	}

	// Request value - this is what we're looking for
	requestValueMap := map[string]any{"query": "SELECT * FROM auth_tokens"}

	// Span in a different trace with exact matching value
	suiteSpan := makeSpan(t, "trace-other", "suite-span", pkg, requestValueMap, inputSchema, 1000)

	// Span in current trace with different value (won't match on value hash)
	currentTraceSpan := makeSpan(t, "trace-current", "current-span", pkg,
		map[string]any{"query": "SELECT * FROM users"}, inputSchema, 2000)

	// Load current trace spans
	server.LoadSpansForTrace("trace-current", []*core.Span{currentTraceSpan})

	// Set suite spans (includes span from other trace)
	server.SetSuiteSpans([]*core.Span{suiteSpan, currentTraceSpan})

	req := makeMockRequest(t, pkg, requestValueMap, inputSchema)

	match, level, err := mm.FindBestMatchWithTracePriority(req, "trace-current")
	require.NoError(t, err)
	require.NotNil(t, match)
	require.NotNil(t, level)

	// Should match the suite span from another trace via Priority 5
	assert.Equal(t, "suite-span", match.SpanId, "Should find match from suite via Priority 5")
	assert.Equal(t, backend.MatchType_MATCH_TYPE_INPUT_VALUE_HASH, level.MatchType)
	assert.Equal(t, backend.MatchScope_MATCH_SCOPE_GLOBAL, level.MatchScope, "Suite match should have GLOBAL scope")
}

// TestFindBestMatchWithTracePriority_SuiteReducedValueHash_MatchesAcrossTraces tests that Priority 6
// (suite-wide reduced value hash) finds matches when matchImportance:0 fields differ
// This test uses validation mode where all suite spans are searchable
func TestFindBestMatchWithTracePriority_SuiteReducedValueHash_MatchesAcrossTraces(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	server.SetAllowSuiteWideMatching(true) // Enable suite-wide matching to search all suite spans
	mm := NewMockMatcher(server)

	pkg := "postgres"
	matchImportanceZero := 0.0
	inputSchema := &core.JsonSchema{
		Properties: map[string]*core.JsonSchema{
			"query":     {},
			"timestamp": {MatchImportance: &matchImportanceZero}, // This field is ignored in reduced hash
		},
	}

	// Request value with one timestamp
	requestValueMap := map[string]any{
		"query":     "SELECT * FROM auth_tokens",
		"timestamp": "2025-01-01T00:00:00Z",
	}

	// Suite span with same query but different timestamp (should match via reduced hash)
	suiteSpanValueMap := map[string]any{
		"query":     "SELECT * FROM auth_tokens",
		"timestamp": "2025-06-15T12:00:00Z",
	}
	suiteSpan := makeSpan(t, "trace-other", "suite-span", pkg, suiteSpanValueMap, inputSchema, 1000)

	// Span in current trace with completely different query
	currentTraceSpan := makeSpan(t, "trace-current", "current-span", pkg,
		map[string]any{"query": "SELECT * FROM users", "timestamp": "2025-01-01T00:00:00Z"}, inputSchema, 2000)

	server.LoadSpansForTrace("trace-current", []*core.Span{currentTraceSpan})
	server.SetSuiteSpans([]*core.Span{suiteSpan, currentTraceSpan})

	req := makeMockRequest(t, pkg, requestValueMap, inputSchema)

	// Sanity check: exact value hashes should differ
	assert.NotEqual(t, suiteSpan.InputValueHash, req.OutboundSpan.InputValueHash,
		"Exact value hashes should differ due to timestamp")

	match, level, err := mm.FindBestMatchWithTracePriority(req, "trace-current")
	require.NoError(t, err)
	require.NotNil(t, match)
	require.NotNil(t, level)

	// Should match via Priority 6 (reduced value hash)
	assert.Equal(t, "suite-span", match.SpanId, "Should find match from suite via Priority 6")
	assert.Equal(t, backend.MatchType_MATCH_TYPE_INPUT_VALUE_HASH_REDUCED_SCHEMA, level.MatchType)
	assert.Equal(t, backend.MatchScope_MATCH_SCOPE_GLOBAL, level.MatchScope, "Suite match should have GLOBAL scope")
}

// TestFindBestMatchWithTracePriority_PrefersTraceOverSuite tests that trace-level matches
// (Priorities 1-4) are preferred over suite-level matches (Priorities 5-6)
func TestFindBestMatchWithTracePriority_PrefersTraceOverSuite(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	mm := NewMockMatcher(server)

	pkg := "http"
	inputSchema := &core.JsonSchema{
		Properties: map[string]*core.JsonSchema{
			"method": {},
			"path":   {},
		},
	}

	requestValueMap := map[string]any{"method": "GET", "path": "/api/auth"}

	// Both trace and suite have spans with exact matching value
	traceSpan := makeSpan(t, "trace-current", "trace-span", pkg, requestValueMap, inputSchema, 2000)
	suiteSpan := makeSpan(t, "trace-other", "suite-span", pkg, requestValueMap, inputSchema, 1000) // older

	server.LoadSpansForTrace("trace-current", []*core.Span{traceSpan})
	server.SetSuiteSpans([]*core.Span{suiteSpan, traceSpan})

	req := makeMockRequest(t, pkg, requestValueMap, inputSchema)

	match, level, err := mm.FindBestMatchWithTracePriority(req, "trace-current")
	require.NoError(t, err)
	require.NotNil(t, match)
	require.NotNil(t, level)

	// Should prefer trace-level match (Priority 1) over suite-level (Priority 5)
	assert.Equal(t, "trace-span", match.SpanId, "Should prefer trace match over suite match")
	assert.Equal(t, backend.MatchType_MATCH_TYPE_INPUT_VALUE_HASH, level.MatchType)
	assert.Equal(t, backend.MatchScope_MATCH_SCOPE_TRACE, level.MatchScope, "Trace match should have TRACE scope")
}

// TestFindBestMatchWithTracePriority_SuiteValueHash_PrefersUnusedOverUsed tests that Priority 5
// prefers unused spans over used spans when matching from suite
// This test uses validation mode where all suite spans are searchable
func TestFindBestMatchWithTracePriority_SuiteValueHash_PrefersUnusedOverUsed(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	server.SetAllowSuiteWideMatching(true) // Enable suite-wide matching to search all suite spans
	mm := NewMockMatcher(server)

	pkg := "http"
	requestValueMap := map[string]any{"method": "GET", "path": "/api/data"}

	// Two suite spans with same value
	suiteSpan1 := makeSpan(t, "trace-A", "suite-first", pkg, requestValueMap, nil, 1000)
	suiteSpan2 := makeSpan(t, "trace-B", "suite-second", pkg, requestValueMap, nil, 2000)

	// Current trace has a span with different value (so it won't match via Priorities 1-4)
	currentSpan := makeSpan(t, "trace-current", "current-span", pkg,
		map[string]any{"method": "POST", "path": "/api/other"}, nil, 3000)
	server.LoadSpansForTrace("trace-current", []*core.Span{currentSpan})

	// Suite spans are indexed in the order provided
	server.SetSuiteSpans([]*core.Span{suiteSpan1, suiteSpan2})

	req := makeMockRequest(t, pkg, requestValueMap, nil)

	// First match should get first unused (in index order)
	match1, level1, err := mm.FindBestMatchWithTracePriority(req, "trace-current")
	require.NoError(t, err)
	require.NotNil(t, match1)
	assert.Equal(t, "suite-first", match1.SpanId, "First match should be first unused in index")
	assert.Equal(t, backend.MatchScope_MATCH_SCOPE_GLOBAL, level1.MatchScope)

	// Second match should get next unused
	match2, level2, err := mm.FindBestMatchWithTracePriority(req, "trace-current")
	require.NoError(t, err)
	require.NotNil(t, match2)
	assert.Equal(t, "suite-second", match2.SpanId, "Second match should be next unused")
	assert.Equal(t, backend.MatchScope_MATCH_SCOPE_GLOBAL, level2.MatchScope)

	// Third match should fall back to used (first in index)
	match3, level3, err := mm.FindBestMatchWithTracePriority(req, "trace-current")
	require.NoError(t, err)
	require.NotNil(t, match3)
	assert.Equal(t, "suite-first", match3.SpanId, "Third match should fall back to first used")
	assert.Equal(t, backend.MatchScope_MATCH_SCOPE_GLOBAL, level3.MatchScope)
}

// TestFindBestMatchWithTracePriority_RegularReplayMode_OnlySearchesGlobalSpans tests that in regular
// replay mode (validation mode = false), only explicitly marked global spans are searched for cross-trace
// matching, not all suite spans
func TestFindBestMatchWithTracePriority_RegularReplayMode_OnlySearchesGlobalSpans(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	// Explicitly NOT setting validation mode (default is false)
	mm := NewMockMatcher(server)

	pkg := "postgres"
	inputSchema := &core.JsonSchema{
		Properties: map[string]*core.JsonSchema{
			"query": {},
		},
	}

	// Request value - this is what we're looking for
	requestValueMap := map[string]any{"query": "SELECT * FROM auth_tokens"}

	// Span in suite (from another trace) with exact matching value - but NOT marked as global
	suiteSpan := makeSpan(t, "trace-other", "suite-span", pkg, requestValueMap, inputSchema, 1000)

	// Span marked as global with exact matching value
	// (global spans are those passed to SetGlobalSpans, fetched from GetGlobalSpans API)
	globalSpan := makeSpan(t, "trace-global", "global-span", pkg, requestValueMap, inputSchema, 2000)

	// Span in current trace with different value (won't match on value hash)
	currentTraceSpan := makeSpan(t, "trace-current", "current-span", pkg,
		map[string]any{"query": "SELECT * FROM users"}, inputSchema, 3000)

	// Load current trace spans
	server.LoadSpansForTrace("trace-current", []*core.Span{currentTraceSpan})

	// Set suite spans (includes non-global span from other trace)
	server.SetSuiteSpans([]*core.Span{suiteSpan, currentTraceSpan})

	// Set global spans (only the explicitly marked global span)
	server.SetGlobalSpans([]*core.Span{globalSpan})

	req := makeMockRequest(t, pkg, requestValueMap, inputSchema)

	match, level, err := mm.FindBestMatchWithTracePriority(req, "trace-current")
	require.NoError(t, err)
	require.NotNil(t, match)
	require.NotNil(t, level)

	// Should match the global span, NOT the suite span (because we're in regular replay mode)
	assert.Equal(t, "global-span", match.SpanId, "Should find match from global spans, not suite spans")
	assert.Equal(t, backend.MatchType_MATCH_TYPE_INPUT_VALUE_HASH, level.MatchType)
	assert.Equal(t, backend.MatchScope_MATCH_SCOPE_GLOBAL, level.MatchScope)
}

// TestFindBestMatchWithTracePriority_RegularReplayMode_NoMatchWhenNotGlobal tests that in regular
// replay mode, a span from another trace that is NOT marked as global will not be found
func TestFindBestMatchWithTracePriority_RegularReplayMode_NoMatchWhenNotGlobal(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	// Explicitly NOT setting validation mode (default is false)
	mm := NewMockMatcher(server)

	pkg := "postgres"
	// Use different schemas to prevent schema-based matching (Priority 7+)
	requestSchema := &core.JsonSchema{
		Properties: map[string]*core.JsonSchema{
			"query":  {},
			"params": {},
		},
	}
	suiteSchema := &core.JsonSchema{
		Properties: map[string]*core.JsonSchema{
			"query": {},
		},
	}
	currentSchema := &core.JsonSchema{
		Properties: map[string]*core.JsonSchema{
			"sql": {},
		},
	}

	// Request value
	requestValueMap := map[string]any{"query": "SELECT * FROM auth_tokens", "params": []any{}}

	// Span in suite (from another trace) with exact matching value - but NOT marked as global
	// Different schema to prevent schema-based matching
	suiteSpan := makeSpan(t, "trace-other", "suite-span", pkg, map[string]any{"query": "SELECT * FROM auth_tokens"}, suiteSchema, 1000)

	// Span in current trace with different value AND different schema (won't match)
	currentTraceSpan := makeSpan(t, "trace-current", "current-span", pkg,
		map[string]any{"sql": "SELECT * FROM users"}, currentSchema, 3000)

	// Load current trace spans
	server.LoadSpansForTrace("trace-current", []*core.Span{currentTraceSpan})

	// Set suite spans (includes non-global span from other trace)
	server.SetSuiteSpans([]*core.Span{suiteSpan, currentTraceSpan})

	// No global spans set - the suite span is not marked as global

	req := makeMockRequest(t, pkg, requestValueMap, requestSchema)

	// Should NOT find a match because:
	// - Current trace span has different value and schema
	// - Suite span is not in global spans index (and has different schema)
	// - Regular replay mode doesn't search suite spans
	match, _, err := mm.FindBestMatchWithTracePriority(req, "trace-current")
	require.Error(t, err, "Should not find match when span is not in global index")
	require.Nil(t, match)
}

// TestFindBestMatchWithTracePriority_RegularReplayMode_GlobalReducedValueHash tests that in regular
// replay mode, reduced value hash matching works for global spans
func TestFindBestMatchWithTracePriority_RegularReplayMode_GlobalReducedValueHash(t *testing.T) {
	cfg, _ := config.Get()
	server, err := NewServer("svc", &cfg.Service)
	require.NoError(t, err)
	// Explicitly NOT setting validation mode (default is false)
	mm := NewMockMatcher(server)

	pkg := "postgres"
	matchImportanceZero := 0.0
	inputSchema := &core.JsonSchema{
		Properties: map[string]*core.JsonSchema{
			"query":     {},
			"timestamp": {MatchImportance: &matchImportanceZero}, // Ignored in reduced hash
		},
	}

	// Request value with one timestamp
	requestValueMap := map[string]any{
		"query":     "SELECT * FROM auth_tokens",
		"timestamp": "2025-01-01T00:00:00Z",
	}

	// Global span with same query but different timestamp (should match via reduced hash)
	// (global spans are those passed to SetGlobalSpans, fetched from GetGlobalSpans API)
	globalSpanValueMap := map[string]any{
		"query":     "SELECT * FROM auth_tokens",
		"timestamp": "2025-06-15T12:00:00Z",
	}
	globalSpan := makeSpan(t, "trace-global", "global-span", pkg, globalSpanValueMap, inputSchema, 1000)

	// Span in current trace with completely different query
	currentTraceSpan := makeSpan(t, "trace-current", "current-span", pkg,
		map[string]any{"query": "SELECT * FROM users", "timestamp": "2025-01-01T00:00:00Z"}, inputSchema, 2000)

	server.LoadSpansForTrace("trace-current", []*core.Span{currentTraceSpan})
	server.SetGlobalSpans([]*core.Span{globalSpan})

	req := makeMockRequest(t, pkg, requestValueMap, inputSchema)

	// Sanity check: exact value hashes should differ
	assert.NotEqual(t, globalSpan.InputValueHash, req.OutboundSpan.InputValueHash,
		"Exact value hashes should differ due to timestamp")

	match, level, err := mm.FindBestMatchWithTracePriority(req, "trace-current")
	require.NoError(t, err)
	require.NotNil(t, match)
	require.NotNil(t, level)

	// Should match via Priority 6 (reduced value hash in global spans)
	assert.Equal(t, "global-span", match.SpanId, "Should find match from global spans via reduced hash")
	assert.Equal(t, backend.MatchType_MATCH_TYPE_INPUT_VALUE_HASH_REDUCED_SCHEMA, level.MatchType)
	assert.Equal(t, backend.MatchScope_MATCH_SCOPE_GLOBAL, level.MatchScope)
}
