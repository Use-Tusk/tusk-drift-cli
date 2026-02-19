# Mock Matching Evals

## Problem

The mock matching algorithm (`internal/runner/mock_matcher.go`) is the core of Tusk Drift's replay engine. When a service makes an outbound call during replay, the matcher decides which recorded mock to return. It uses a 15-priority cascade across three dimensions — compression level (exact hash, reduced hash, schema), scope (trace, suite/global), and consumption state (unused, used).

Today we have 29 unit tests that verify individual behaviors (e.g., "P1 prefers unused over used", "HTTP shape validation rejects method mismatch"). What we lack is:

1. A structured eval dataset that lets us measure **aggregate accuracy** across realistic scenarios
2. The ability to see **tradeoffs** when changing the algorithm (e.g., reordering priorities, adding/removing steps, tuning similarity scoring)
3. Tooling to **diagnose production matching failures** and feed learnings back into the eval set

Without evals, any change to the matching algorithm is a leap of faith — it might improve one class of matches while silently breaking another.

## Goals

- **Confidently make algorithm changes**: Run evals before/after a change, see exactly what improved and what regressed
- **Catch regressions**: If a change breaks a previously-correct match, a specific eval case fails with a clear error
- **Grow the dataset over time**: When we find a production matching failure, we can add a targeted eval case that reproduces it

## Approach: Two Components

### 1. Golden Eval Dataset + Harness (in `tusk-drift-cli`)

A set of hand-crafted JSON eval examples, each describing a self-contained matching problem. A Go test harness loads them, runs the real `MockMatcher`, and asserts correctness.

**Why hand-crafted, not auto-generated from production traces:**
- Hand-crafted examples are concise — they include only the fields that matter for the scenario being tested
- Auto-converted production traces are bloated (every field on every span) and still require human review
  - E.g a production trace would require all spans from all traces to be present in the eval incase we need to match across traces. This would be a lot of data to store.
- Each eval example should test a specific behavior, not be a dump of "what happened in production"
- When a production failure is found, the analyzer (below) presents the findings, and a coding agent can create a minimal eval entry that reproduces the issue

**Why in the CLI repo (not a separate repo):**
- The golden dataset is synthetic/hand-crafted — no sensitive production data
- Co-locating the harness and data avoids cross-repo dependency issues
- The harness needs to import `internal/runner` (the `MockMatcher`), which is only possible from within the module

### 2. Trace Match Analyzer (in `tusk_quick_fixes`)

A backend script that queries existing `trace_test_result` and `trace_test_span_result` tables to analyze how well matching worked for a real replay run. It presents findings in a digestible format so a coding agent can understand what happened and, if needed, create a targeted eval case.

## Eval Dataset Design

### Format

Each eval file contains an array of examples. Each example is a self-contained matching scenario:

```json
{
  "id": "schema_match_picks_same_table",
  "description": "When exact/reduced hash matching fails, schema match should pick the query against the same table based on similarity scoring",
  "tags": ["schema_match", "similarity", "postgres"],
  "config": {
    "allowSuiteWideMatching": true
  },
  "trace_mocks": [
    {
      "spanId": "span-1",
      "traceId": "trace-1",
      "packageName": "pg",
      "inputValue": {"query": "SELECT * FROM users WHERE id = 123"},
      "inputSchema": {"type": "OBJECT", "properties": {"query": {"type": "STRING"}}},
      "inputValueHash": "hash-users-123",
      "inputSchemaHash": "hash-pg-query-schema",
      "isPreAppStart": false,
      "timestamp": "2025-01-01T00:00:01Z"
    },
    {
      "spanId": "span-2",
      "traceId": "trace-1",
      "packageName": "pg",
      "inputValue": {"query": "SELECT * FROM orders WHERE id = 789"},
      "inputSchema": {"type": "OBJECT", "properties": {"query": {"type": "STRING"}}},
      "inputValueHash": "hash-orders-789",
      "inputSchemaHash": "hash-pg-query-schema",
      "isPreAppStart": false,
      "timestamp": "2025-01-01T00:00:02Z"
    }
  ],
  "suite_mocks": [],
  "global_mocks": [],
  "requests": [
    {
      "request": {
        "packageName": "pg",
        "inputValue": {"query": "SELECT * FROM orders WHERE id = 456"},
        "inputSchema": {"type": "OBJECT", "properties": {"query": {"type": "STRING"}}},
        "inputValueHash": "hash-orders-456",
        "inputSchemaHash": "hash-pg-query-schema",
        "isPreAppStart": false
      },
      "expected": {
        "matchedSpanId": "span-2",
        "matchType": "INPUT_SCHEMA_HASH",
        "matchScope": "TRACE"
      }
    }
  ]
}
```

### Key Design Decisions

**`requests` is an array, not a single request.** This lets us test consumption tracking — where earlier matches consume spans and affect later matches. For example, three identical `SELECT 1` health checks should consume three different mocks in timestamp order.

**`config.allowSuiteWideMatching` is explicit per-example.** We start with `true` only (validation mode / local runs). The schema supports adding `false` later without changing the format.

**Three mock pools: `trace_mocks`, `suite_mocks`, `global_mocks`.** These map directly to how the `MockMatcher` loads data:
- `trace_mocks` → loaded via `LoadSpansForTrace()` (searched at P1-P10)
- `suite_mocks` → loaded via `SetSuiteSpans()` (searched at P5-P6 when `AllowSuiteWideMatching=true`, and P12-P15 in `FindBestMatchAcrossTraces`)
- `global_mocks` → loaded via `SetGlobalSpans()` (searched at P5-P6 when `AllowSuiteWideMatching=false`)

For now, with `AllowSuiteWideMatching=true`, `suite_mocks` is the relevant cross-trace pool and `global_mocks` can be empty.

**`expected` includes `matchType` and `matchScope`, not just `matchedSpanId`.** This lets us verify the matcher found the right span *for the right reason*. If an algorithm change causes a span to match at P7 (schema) instead of P1 (exact hash), that's a signal even if the matched span is correct.

**`expected.matchedSpanId` can be `null`.** This tests cases where the matcher should correctly return "no match" — e.g., when `shouldSkipSchemaFallbackMatching` prevents a bad schema match for psycopg queries.

### Categories

| Category | What it tests | Priority levels exercised |
|---|---|---|
| `exact_match` | Identical hash lookup, unused-before-used ordering | P1-P2 |
| `reduced_match` | matchImportance=0 field stripping | P3-P4 |
| `suite_value_match` | Cross-trace exact/reduced hash matching | P5-P6 |
| `schema_match` | Schema hash + similarity scoring picks best candidate | P7-P10 |
| `http_shape` | Method/path/host/query-key validation rejects bad candidates | P7-P10 |
| `graphql` | Whitespace normalization for GraphQL queries | P7-P10 |
| `consumption` | Sequential requests consume different mocks in order | P1-P2, P7-P8 |
| `pre_app_start` | Lifecycle filtering: startup spans don't match runtime and vice versa | P12-P15 |
| `db_schema_skip` | psycopg/sqlalchemy queries skip schema fallback (return no-match over wrong-match) | P7-P10 skipped |
| `no_match` | Correctly returns error when no valid mock exists | All |
| `cross_protocol` | Different package types don't interfere | All |

### Metrics

Per eval run, the harness computes:

- **Accuracy**: % of requests where the correct span was matched (or correctly returned no-match)
- **Priority accuracy**: % where the match was at the expected priority level
- **False positive rate**: % where a wrong span was returned (worse than no-match)
- **Breakdown by tag**: accuracy per category
- **Regression delta**: compared to a saved baseline (optional)

## Eval Harness

A Go test file at `internal/runner/mock_matcher_eval_test.go` that:

1. Reads JSON eval files from `internal/runner/testdata/eval/` (or a configurable path)
2. For each example:
   a. Constructs a `Server` with the specified config
   b. Loads `trace_mocks` via `LoadSpansForTrace()`
   c. Loads `suite_mocks` via `SetSuiteSpans()` and `global_mocks` via `SetGlobalSpans()`
   d. Processes each request in order, calling `FindBestMatchWithTracePriority()` (and `FindBestMatchAcrossTraces()` if needed)
   e. Asserts `matchedSpanId`, `matchType`, and `matchScope` against expected
3. Outputs a summary report

Run with:
```bash
go test ./internal/runner/ -run TestMockMatcherEval -v
```

## Trace Match Analyzer

A TypeScript script in `tusk_quick_fixes/backend/` that analyzes an existing replay result from the database.

**Input:** A `traceTestResultId`

**Output:** A structured report showing, for each span in the trace:

```text
Trace Test: GET /api/orders/123
Status: FAILED (MOCK_NOT_FOUND)
Total spans: 8 matched, 1 unmatched

SPAN 1: pg.query "SELECT * FROM users WHERE id = $1"
  Match: P1 (exact value hash, trace scope)
  Confidence: HIGH

SPAN 2: pg.query "SELECT * FROM orders WHERE id = $1"
  Match: P7 (schema hash, trace scope, similarity: 0.82)
  Confidence: MEDIUM
  Top candidates:
    -> span-abc (0.82) <- SELECTED
    -> span-def (0.79) "SELECT * FROM orders WHERE status = $1"
    -> span-ghi (0.71) "SELECT * FROM products WHERE id = $1"

SPAN 3: https.request GET /api/payments/charge
  Match: NONE
  Confidence: N/A
  Diagnosis:
    Available mocks with same schema hash: 2
    -> span-xyz: POST /api/payments/charge (likely rejected: method mismatch)
    -> span-uvw: GET /api/payments/refund (likely rejected: path mismatch)
    Available mocks with same package (https): 5
```

For unmatched spans, the analyzer queries `span_recording` to find what mocks *were* available and infers why they weren't matched (same schema but different method, different path, etc.). This is the key diagnostic — it tells you whether the failure was a matcher limitation or a genuine gap in recorded data.

**Run with:**
```bash
npm run script -- src/scripts/analyzeTraceMatchQuality.ts --traceTestResultId=<uuid>
```

## Workflow

### Making an algorithm change

1. Run eval suite → note baseline accuracy (e.g., 47/50 passing)
2. Make the change (e.g., swap priority ordering, tune similarity threshold)
3. Run eval suite → note new accuracy (e.g., 49/50 passing)
4. Inspect the diff: which cases improved? Did any regress?
5. If a case regressed, decide whether the tradeoff is acceptable

### Investigating a production failure

1. Run the trace match analyzer on the failed `traceTestResultId`
2. Review the report — identify if the failure was a mock matching issue (correct mock existed but wasn't found) or a recording gap (no appropriate mock existed)
3. If it was a matcher issue, use the analyzer output to understand what happened (which priority level matched, what candidates existed, why the right one was rejected)
4. Create a minimal eval entry that reproduces the issue
5. Verify the new eval case fails with the current algorithm
6. Fix the algorithm, verify the case passes, verify no regressions

## File Structure

```
tusk-drift-cli/
  internal/runner/
    mock_matcher.go                    # The algorithm being tested
    mock_matcher_test.go               # Existing unit tests (keep as-is)
    mock_matcher_eval_test.go          # Eval harness
    testdata/
      eval/
        exact_match.json
        reduced_match.json
        schema_match.json
        http_shape.json
        graphql.json
        consumption.json
        pre_app_start.json
        db_schema_skip.json
        suite_scope.json
        no_match.json

tusk_quick_fixes/
  backend/src/scripts/
    analyzeTraceMatchQuality.ts        # Trace match analyzer
```

## Future Extensions

- **`AllowSuiteWideMatching=false` eval cases**: Add examples that test the restricted cloud replay mode where only `global_mocks` are searched at P5-P6
- **Similarity score range assertions**: Assert that `similarityScore` falls within an expected range, not just that the right span was picked
- **Top candidates assertions**: Assert that the expected alternatives appear in `topCandidates` with reasonable scores
- **Enhanced match metadata**: Add fields like `priorities_attempted` and `total_candidates_at_priority` to the CLI's `MatchLevel` proto to improve analyzer diagnostics
- **Batch analysis**: Analyze all trace test results in an `ApiDriftCommitCheckRun` to surface systemic matching patterns
- **CI integration**: Run evals as part of CLI CI, fail the build if accuracy drops below a threshold
