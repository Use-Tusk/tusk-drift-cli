# Tusk Unit

Tusk Unit provides CLI commands for viewing and applying unit tests generated from [Tusk](https://usetusk.ai). When Tusk generates unit tests on a pull request, you (or your coding agent) can use these commands to inspect runs, review individual test scenarios, and apply diffs to your codebase.

## Prerequisites

Authenticate with Tusk:

```bash
tusk auth login
```

Or set the `TUSK_API_KEY` environment variable. See [onboarding docs](https://docs.usetusk.ai/onboarding) for details.

## Commands

### `tusk unit latest-run`

Get the latest unit test run for a repo/branch. Defaults to the current git remote and branch.

```bash
tusk unit latest-run
tusk unit latest-run --repo owner/repo --branch feature-branch
```

Returns a summary of the latest run plus a history of recent runs on the branch. History items include a readable `run_type_label`, the raw `run_type`, `commit_sha`, and `retry_feedback` when present.

### `tusk unit get-run <run-id>`

Get full details for a specific unit test run, including test scenarios and coverage gains.

```bash
tusk unit get-run <run-id>
```

### `tusk unit get-scenario`

Get details for a specific test scenario within a run.

```bash
tusk unit get-scenario --run-id <run-id> --scenario-id <scenario-id>
```

### `tusk unit get-diffs <run-id>`

Get file diffs for a unit test run. Diffs are in unified diff format, ready to apply with `git apply`.

```bash
tusk unit get-diffs <run-id>
```

### `tusk unit feedback`

Submit run-level and/or scenario-level feedback for a unit test run. Add `--retry` to save the feedback and immediately trigger a retry.

```bash
tusk unit feedback --run-id <run-id> --file feedback.json
tusk unit feedback --run-id <run-id> --file feedback.json --retry
```

You can also submit feedback inline with stdin:

```bash
tusk unit feedback --run-id <run-id> --file - <<'EOF'
{
  "run_feedback": {
    "comment": "The run targeted the right files, but the mocks do not match the real service contracts and several scenarios are asserting on implementation details. Use simpler setup assumptions and focus on externally observable behavior."
  },
  "scenarios": [
    {
      "scenario_id": "uuid",
      "positive_feedback": ["covers_critical_path"],
      "comment": "Good scenario and likely worth keeping.",
      "applied_locally": true
    },
    {
      "scenario_id": "uuid",
      "negative_feedback": ["incorrect_assertion"],
      "comment": "The generated assertion does not match the behavior we want to preserve, so we did not keep this test.",
      "applied_locally": false
    }
  ]
}
EOF
```

Use `run_feedback.comment` mainly for broad retry guidance, such as wrong mocks, wrong symbols, or an overall incorrect test strategy.
Use either `positive_feedback` or `negative_feedback` for a scenario.

Allowed `positive_feedback` values:

- `covers_critical_path`
- `valid_edge_case`
- `caught_a_bug`
- `other`

Allowed `negative_feedback` values:

- `incorrect_business_assumption`
- `duplicates_existing_test`
- `no_value`
- `incorrect_assertion`
- `poor_coding_practice`
- `other`

### `tusk unit retry`

Trigger a retry for a unit test run, optionally storing a broad run-level comment first.

```bash
tusk unit retry --run-id <run-id>
tusk unit retry --run-id <run-id> --comment "The run targeted the right files, but the mocks do not match the real service contracts and several scenarios assert on implementation details. Use simpler setup assumptions and focus on externally observable behavior."
```

## Typical workflow

1. Check the latest run on your branch:

   ```bash
   tusk unit latest-run
   ```

2. Inspect the run to see test scenarios and coverage:

   ```bash
   tusk unit get-run <run-id>
   ```

3. Review individual scenarios if needed:

   ```bash
   tusk unit get-scenario --run-id <run-id> --scenario-id <scenario-id>
   ```

4. Submit feedback on scenarios you kept or rejected, and optionally add broad run-level guidance:

   ```bash
   tusk unit feedback --run-id <run-id> --file feedback.json
   ```

5. If the run was broadly wrong, trigger a retry with feedback instead of editing everything locally:

   ```bash
   tusk unit feedback --run-id <run-id> --file feedback.json --retry
   tusk unit retry --run-id <run-id> --comment "Wrong mocks for this run"
   ```

   Retries may take a while. If the tests are mostly correct, prefer small local edits instead of a full retry.

6. Apply all generated tests to your working tree when they are close and only need small edits:

   ```bash
   tusk unit get-diffs <run-id> | jq -r '.files[].diff' | git apply
   ```

   Or apply selectively by piping through `jq` to filter by file path or scenario ID.

## JSON output

All commands output JSON. Pipe through `jq` for filtering:

```bash
# Pretty-print
tusk unit get-run <run-id> | jq .

# Extract just scenario IDs
tusk unit get-run <run-id> | jq '.test_scenarios[].scenario_id'

# Apply only diffs for a specific file
tusk unit get-diffs <run-id> | jq -r '.files[] | select(.file_path | test("myfile")) | .diff' | git apply
```
