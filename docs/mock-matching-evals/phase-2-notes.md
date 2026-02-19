# Phase 2: Golden Dataset — Implementation Notes

## 1. Implementation Notes

### Deviations from the design doc

- **P9-P10 (reduced schema match) examples were omitted.** The design doc lists these as a category in `schema_match.json`. During implementation, analysis of `mock_matcher.go` revealed that `findUnusedSpanByReducedInputSchemaHash` and `findUsedSpanByReducedInputSchemaHash` both call `schemaMatchWithHttpShape()`, which checks the **full** `InputSchemaHash` match (not the reduced hash). This means P9-P10 can only match spans that already have the same full schema hash as the request — but those would have already been caught at P7-P8. P9-P10 are effectively unreachable dead code paths. Rather than writing eval cases that can never exercise these paths, the slot was replaced with `schema_match_low_similarity_still_picks` which tests that a single schema candidate with low similarity is still matched.

- **`schema_match.json` example 4 replaced.** The plan called for a "reduced schema match (P9-P10)" example. Replaced with "single schema match candidate" and "low similarity still picks" examples, which test more useful real-world scenarios.

- **`no_match.json` example 2 differs from seed.json's `no_match_wrong_package`.** Both test "wrong package" but the new `no_match_different_value_and_schema` tests same package with completely different schemas, while `no_match_cross_protocol_no_interference` tests multiple different packages coexisting.

### Assumptions made where the design doc wasn't clear

- **GraphQL variables should use string values for similarity testing.** The similarity scorer stringifies numbers via `fmt.Sprintf("%v")` before computing Levenshtein distance. Small numeric differences (e.g., 99 vs 100) produce poor distance signals. String variables (e.g., usernames) produce clearer differentiation.

- **Suite mocks need a `traceId` field.** The `markSpanAsUsed()` function uses `span.TraceId` as a key for the `spanUsage` map. Suite mocks without a traceId would have usage tracked under the empty string. Eval examples use descriptive traceIds like `"trace-init"` or `"trace-other"` for suite mocks.

- **`allowSuiteWideMatching: false` for most examples.** Unless specifically testing suite-wide matching (suite_scope.json, pre_app_start.json), examples use `false` to keep the matching scope narrow and test trace-level behavior in isolation.

- **HTTP shape tests use both `http` and `https` packages.** The `schemaMatchWithHttpShape` function checks `span.PackageName != "http" && span.PackageName != "https"`, so both packages trigger HTTP validation. The host mismatch test uses `https` to confirm both are covered.

### Decisions made during implementation

- **Each JSON file maps to one category from the design doc** — `exact_match.json`, `reduced_match.json`, `suite_scope.json`, `schema_match.json`, `http_shape.json`, `graphql.json`, `consumption.json`, `pre_app_start.json`, `db_schema_skip.json`, `no_match.json`.

- **Tags are granular.** Each example includes both category tags (e.g., `"exact_match"`) and priority level tags (e.g., `"P1"`, `"P7"`). Multi-behavior examples include all relevant tags (e.g., `["consumption", "P1", "P7", "cross_priority"]`).

- **P12 tested via `allowSuiteWideMatching: false`.** To test P12 (suite value hash in `FindBestMatchAcrossTraces`), the example uses `allowSuiteWideMatching: false` so P5 searches global spans (empty), forcing the request to fall through to the cross-trace fallback where P12 searches suite spans. This is the only way to exercise P12 independently of P5.

## 2. Edge Cases

### Edge cases not covered in the original design

- **Numeric similarity scoring limitations.** The similarity scorer converts numbers to strings before comparison. This means `99` vs `100` has the same Levenshtein distance as `99` vs `200` (both are distance 2-3 on short strings). This is documented behavior but could cause surprising tiebreakers in production when numeric fields are the only differentiator. Handled by using string variables in the GraphQL similarity test.

- **P9-P10 unreachable.** As noted above, the `schemaMatchWithHttpShape()` call in the reduced schema hash path checks full schema hashes, making P9-P10 dead code. This is a potential bug — if the intent was to allow reduced schema matching when full schemas differ, `schemaMatchWithHttpShape` should use reduced schema hashes in the P9-P10 path. **This needs future investigation.**

- **Empty `traceMocks` with `traceId` on request.** When there are no trace mocks but the request specifies a `traceId`, `FindBestMatchWithTracePriority` correctly handles the empty trace (no spans loaded). The `no_match_empty_mocks` example tests this implicitly.

- **Suite mocks with `isPreAppStart: false` vs omitted.** The Go default for `bool` is `false`, so omitting `isPreAppStart` is equivalent to setting it to `false`. Both forms are used across examples for clarity.

- **Query parameter extraction from path field.** The `extractPathAndQueryKeys` function parses query params from the `path` field (e.g., `/api/users?page=1&limit=10`). The tests exercise this specific parsing behavior with the `http_shape_same_query_keys_different_values_matched` and `http_shape_different_query_keys_rejected` examples.

### Areas that need future work

- **P9-P10 eval cases** should be added if/when the `schemaMatchWithHttpShape` behavior is corrected to use reduced schema hashes.
- **`AllowSuiteWideMatching: false` with global mocks** — no eval case currently tests the non-validation (regular replay) mode with explicitly marked global mocks. Could be added to `suite_scope.json`.
- **Similarity score range assertions** — the design doc mentions asserting similarity score ranges as a future extension. Currently we only verify the matched span, not the score.

## 3. Potential Issues

- **Flaky similarity ordering.** If two spans have very close similarity scores (differing only by floating point precision), the tiebreaker is timestamp. The `schema_match_tiebreaker_oldest_timestamp` example tests this with identical values, but production scenarios might have near-ties that aren't as clean. The parallel worker pool could theoretically introduce non-determinism in score computation order, though the sorting afterward should be deterministic.

- **Hash determinism across platforms.** The eval harness auto-computes hashes via `utils.GenerateDeterministicHash()`. If this function produces different hashes on different architectures (unlikely for SHA256 of JSON, but worth noting), evals could fail in CI on a different OS.

- **GraphQL normalization edge cases.** The `normalizeGQL` function only normalizes brace spacing and collapses whitespace. It does NOT normalize colon spacing (e.g., `id:1` vs `id: 1` are different after normalization). The eval examples are written to avoid this pitfall, but production GraphQL queries might hit it.

- **Pre-app-start cross-trace fallback.** The harness calls `FindBestMatchAcrossTraces` only when `isPreAppStart || traceID == ""`. In production, the real server might call this differently. The harness behavior matches the real code flow in `handleMockRequest`, but changes to that flow could make the eval results diverge from production behavior.

## 4. Testing Checklist

### Run all evals
```bash
cd tusk-drift-cli
go test ./internal/runner/ -run TestMockMatcherEval -v
```
Expected: **42 passed, 0 failed** (3 seed + 39 new)

### Run a specific category
```bash
go test ./internal/runner/ -run TestMockMatcherEval/exact_match -v
go test ./internal/runner/ -run TestMockMatcherEval/http_shape -v
go test ./internal/runner/ -run TestMockMatcherEval/pre_app_start -v
```

### Verify existing unit tests still pass
```bash
go test ./internal/runner/ -run TestMockMatcher -v
```

### Verify tag breakdown
In the eval summary output, confirm:
- `exact_match`: 5 passed (4 new + 1 seed)
- `reduced_match`: 3 passed
- `suite_scope`: 6 passed (4 examples, some with multiple tags)
- `schema_match`: 7 passed (5 new + 1 seed + 1 in suite_scope that also tags schema)
- `http_shape`: 6 passed (5 new + 1 in no_match)
- `graphql`: 3 passed
- `consumption`: 6 passed (3 examples with multiple tags)
- `pre_app_start`: 6 passed (4 new + 2 with overlap tags)
- `db_schema_skip`: 4 passed
- `no_match`: 9 passed (4 new + 1 seed + 4 with overlap tags)

### What to verify works correctly
1. **P1-P2 exact matching**: Correct span picked, oldest unused preferred, used fallback works
2. **P3-P4 reduced matching**: matchImportance=0 fields stripped, nested fields handled
3. **P5-P6 suite matching**: Suite spans found when trace has no match, trace preferred over suite
4. **P7-P8 schema matching**: Similarity picks closest, tiebreaker works, single candidate OK
5. **HTTP shape guardrails**: Method/path/host/query-key validation rejects mismatches
6. **GraphQL normalization**: Whitespace normalization passes, different queries rejected
7. **Consumption tracking**: Sequential requests consume different mocks in order
8. **Pre-app-start filtering**: Bidirectional filtering (runtime rejects preapp, preapp rejects runtime)
9. **DB schema skip guardrail**: psycopg2/sqlalchemy skip P7-P10, pre-app-start exempted
10. **No-match scenarios**: Correct nil return for empty mocks, wrong package, wrong schema
