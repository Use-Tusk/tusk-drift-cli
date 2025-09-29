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
	match := mm.findUnusedSpanByReducedInputSchemaHash(req, []*core.Span{span})
	require.NotNil(t, match)
	assert.Equal(t, "sRS", match.SpanId)
}
