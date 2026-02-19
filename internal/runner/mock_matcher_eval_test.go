package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"

	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// convertJsonSchemaEval recursively converts a JsonSchemaEval to a core.JsonSchema protobuf.
func convertJsonSchemaEval(eval *JsonSchemaEval) *core.JsonSchema {
	if eval == nil {
		return nil
	}
	schema := &core.JsonSchema{
		MatchImportance: eval.MatchImportance,
	}
	if eval.Type != "" {
		enumKey := "JSON_SCHEMA_TYPE_" + strings.ToUpper(eval.Type)
		if val, ok := core.JsonSchemaType_value[enumKey]; ok {
			schema.Type = core.JsonSchemaType(val)
		}
	}
	if len(eval.Properties) > 0 {
		schema.Properties = make(map[string]*core.JsonSchema)
		for k, v := range eval.Properties {
			schema.Properties[k] = convertJsonSchemaEval(v)
		}
	}
	if eval.Items != nil {
		schema.Items = convertJsonSchemaEval(eval.Items)
	}
	return schema
}

// convertEvalSpan converts an EvalSpan to a core.Span protobuf, auto-computing hashes if not provided.
func convertEvalSpan(t *testing.T, es EvalSpan) *core.Span {
	t.Helper()

	protoSchema := convertJsonSchemaEval(es.InputSchema)

	var inputValueStruct = toStruct(t, es.InputValue)

	inputValueHash := es.InputValueHash
	if inputValueHash == "" && es.InputValue != nil {
		inputValueHash = utils.GenerateDeterministicHash(es.InputValue)
	}
	inputSchemaHash := es.InputSchemaHash
	if inputSchemaHash == "" && protoSchema != nil {
		inputSchemaHash = utils.GenerateDeterministicHash(protoSchema)
	}

	var ts *timestamppb.Timestamp
	if es.Timestamp != "" {
		parsed, err := time.Parse(time.RFC3339, es.Timestamp)
		require.NoError(t, err, "invalid timestamp %q in span %s", es.Timestamp, es.SpanId)
		ts = timestamppb.New(parsed)
	}

	return &core.Span{
		TraceId:         es.TraceId,
		SpanId:          es.SpanId,
		PackageName:     es.PackageName,
		SubmoduleName:   es.SubmoduleName,
		Name:            es.Name,
		InputValue:      inputValueStruct,
		InputSchema:     protoSchema,
		InputValueHash:  inputValueHash,
		InputSchemaHash: inputSchemaHash,
		IsPreAppStart:   es.IsPreAppStart,
		Timestamp:       ts,
	}
}

// convertEvalRequest converts an EvalRequestData to a core.GetMockRequest protobuf.
func convertEvalRequest(t *testing.T, er EvalRequestData) *core.GetMockRequest {
	t.Helper()

	protoSchema := convertJsonSchemaEval(er.InputSchema)

	var inputValueStruct = toStruct(t, er.InputValue)

	inputValueHash := er.InputValueHash
	if inputValueHash == "" && er.InputValue != nil {
		inputValueHash = utils.GenerateDeterministicHash(er.InputValue)
	}
	inputSchemaHash := er.InputSchemaHash
	if inputSchemaHash == "" && protoSchema != nil {
		inputSchemaHash = utils.GenerateDeterministicHash(protoSchema)
	}

	operation := er.Operation
	if operation == "" {
		operation = "GET"
	}

	return &core.GetMockRequest{
		OutboundSpan: &core.Span{
			PackageName:     er.PackageName,
			SubmoduleName:   er.SubmoduleName,
			Name:            er.Name,
			InputValue:      inputValueStruct,
			InputSchema:     protoSchema,
			InputValueHash:  inputValueHash,
			InputSchemaHash: inputSchemaHash,
			IsPreAppStart:   er.IsPreAppStart,
		},
		Operation: operation,
	}
}

// matchTypeFromString converts a JSON string like "INPUT_VALUE_HASH" to core.MatchType.
func matchTypeFromString(s string) core.MatchType {
	enumKey := "MATCH_TYPE_" + s
	if val, ok := core.MatchType_value[enumKey]; ok {
		return core.MatchType(val)
	}
	return core.MatchType_MATCH_TYPE_UNSPECIFIED
}

// matchScopeFromString converts a JSON string like "TRACE" to core.MatchScope.
func matchScopeFromString(s string) core.MatchScope {
	enumKey := "MATCH_SCOPE_" + s
	if val, ok := core.MatchScope_value[enumKey]; ok {
		return core.MatchScope(val)
	}
	return core.MatchScope_MATCH_SCOPE_UNSPECIFIED
}

// resolveTraceID determines which traceID to use for a request.
func resolveTraceID(er EvalRequestData, example EvalExample) string {
	if er.TraceId != "" {
		return er.TraceId
	}
	for _, span := range example.TraceMocks {
		if span.TraceId != "" {
			return span.TraceId
		}
	}
	return ""
}

func spanIdOrNil(span *core.Span) string {
	if span == nil {
		return "<nil>"
	}
	return span.SpanId
}

func TestMockMatcherEval(t *testing.T) {
	evalDir := filepath.Join("testdata", "eval")
	files, err := filepath.Glob(filepath.Join(evalDir, "*.json"))
	require.NoError(t, err, "failed to glob eval files")
	require.NotEmpty(t, files, "no eval files found in %s", evalDir)

	var allExamples []EvalExample
	for _, f := range files {
		data, err := os.ReadFile(f)
		require.NoError(t, err, "failed to read %s", f)

		var evalFile EvalFile
		err = json.Unmarshal(data, &evalFile)
		require.NoError(t, err, "failed to parse %s", f)

		allExamples = append(allExamples, evalFile.Examples...)
	}

	require.NotEmpty(t, allExamples, "no eval examples loaded")

	// Track results for summary
	type evalResult struct {
		id     string
		tags   []string
		passed bool
	}
	var results []evalResult

	for _, example := range allExamples {
		example := example
		passed := t.Run(example.ID, func(t *testing.T) {
			// Server setup
			cfg, _ := config.Get()
			server, err := NewServer("eval-svc", &cfg.Service)
			require.NoError(t, err)
			mm := NewMockMatcher(server)

			// Convert and group trace mocks by traceId
			traceGroups := make(map[string][]*core.Span)
			for _, es := range example.TraceMocks {
				span := convertEvalSpan(t, es)
				traceGroups[es.TraceId] = append(traceGroups[es.TraceId], span)
			}
			for traceID, spans := range traceGroups {
				server.LoadSpansForTrace(traceID, spans)
			}

			// Load suite mocks
			var suiteMocks []*core.Span
			for _, es := range example.SuiteMocks {
				suiteMocks = append(suiteMocks, convertEvalSpan(t, es))
			}
			if len(suiteMocks) > 0 {
				server.SetSuiteSpans(suiteMocks)
			}

			// Load global mocks
			var globalMocks []*core.Span
			for _, es := range example.GlobalMocks {
				globalMocks = append(globalMocks, convertEvalSpan(t, es))
			}
			if len(globalMocks) > 0 {
				server.SetGlobalSpans(globalMocks)
			}

			// Apply config
			server.SetAllowSuiteWideMatching(example.Config.AllowSuiteWideMatching)

			// Process each request sequentially
			for i, evalReq := range example.Requests {
				req := convertEvalRequest(t, evalReq.Request)
				traceID := resolveTraceID(evalReq.Request, example)

				// Try trace-level matching first
				match, level, _ := mm.FindBestMatchWithTracePriority(req, traceID)

				// If no match and (pre-app-start or no traceID), try cross-trace fallback
				if match == nil && (req.OutboundSpan.IsPreAppStart || traceID == "") {
					candidates := server.GetSuiteSpans()
					if len(candidates) > 0 {
						match, level, _ = mm.FindBestMatchAcrossTraces(req, traceID, candidates)
					}
				}

				expected := evalReq.Expected

				if expected.MatchedSpanId == nil {
					assert.Nil(t, match,
						"request[%d]: expected no match but got span %q", i, spanIdOrNil(match))
				} else {
					require.NotNil(t, match,
						"request[%d]: expected span %q but got no match", i, *expected.MatchedSpanId)
					assert.Equal(t, *expected.MatchedSpanId, match.SpanId,
						"request[%d]: expected span %q but got %q", i, *expected.MatchedSpanId, match.SpanId)

					if expected.MatchType != "" {
						require.NotNil(t, level, "request[%d]: match level is nil", i)
						assert.Equal(t, matchTypeFromString(expected.MatchType), level.MatchType,
							"request[%d]: expected matchType %s but got %s", i, expected.MatchType, level.MatchType)
					}
					if expected.MatchScope != "" {
						require.NotNil(t, level, "request[%d]: match level is nil", i)
						assert.Equal(t, matchScopeFromString(expected.MatchScope), level.MatchScope,
							"request[%d]: expected matchScope %s but got %s", i, expected.MatchScope, level.MatchScope)
					}
				}
			}
		})

		results = append(results, evalResult{
			id:     example.ID,
			tags:   example.Tags,
			passed: passed,
		})
	}

	// Print summary
	passCount := 0
	failCount := 0
	tagStats := make(map[string][2]int) // [pass, fail]
	for _, r := range results {
		if r.passed {
			passCount++
		} else {
			failCount++
		}
		for _, tag := range r.tags {
			stats := tagStats[tag]
			if r.passed {
				stats[0]++
			} else {
				stats[1]++
			}
			tagStats[tag] = stats
		}
	}

	t.Log("=== EVAL SUMMARY ===")
	t.Logf("Total: %d passed, %d failed, %d examples", passCount, failCount, len(results))
	if len(tagStats) > 0 {
		t.Log("By tag:")
		for tag, stats := range tagStats {
			t.Logf("  %s: %d passed, %d failed", tag, stats[0], stats[1])
		}
	}
	if failCount > 0 {
		t.Log("Failed examples:")
		for _, r := range results {
			if !r.passed {
				t.Logf("  - %s", r.id)
			}
		}
	}
	fmt.Println() // ensure summary is visible even without -v
}
