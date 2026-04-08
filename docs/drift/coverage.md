# Code Coverage

Tusk Drift can collect code coverage during test replay, showing which lines of your service code each trace test exercises.

Coverage works with Node.js and Python.

## Enabling Coverage

There are two ways to enable coverage:

### Config-driven (for CI)

Add `coverage.enabled: true` to `.tusk/config.yaml`. Coverage is automatically collected during validation runs on the default branch. No CI changes needed.

```yaml
coverage:
  enabled: true
```

Config-driven coverage is silent (no console output). Data is collected for backend upload during suite validation.

### Flag-driven (for local dev)

```bash
# Show coverage in console
tusk drift run --show-coverage --print

# Export to file (implies coverage collection)
tusk drift run --coverage-output coverage.lcov --print
```

## CLI Flags

| Flag | Description |
|------|-------------|
| `--show-coverage` | Collect and display code coverage. Forces concurrency to 1. |
| `--coverage-output <path>` | Write coverage data to a file. LCOV format by default; JSON if path ends in `.json`. Implies coverage collection. |

### When coverage activates

| Scenario | Coverage collected? | Shown in console? |
|---|---|---|
| `coverage.enabled: true` + validation run (CI) | Yes | No (silent) |
| `coverage.enabled: true` + local/PR run | No | No |
| `--show-coverage` (any context) | Yes | Yes |
| `--coverage-output` (any context) | Yes | Only if `--show-coverage` also set |

## Configuration

Optional include/exclude patterns in `.tusk/config.yaml`:

```yaml
coverage:
  include:
    - "backend/src/**"          # only report on your service's code
  exclude:
    - "**/migrations/**"        # exclude database migrations
    - "**/generated/**"         # exclude generated code
    - "**/*.test.ts"            # exclude test files loaded at startup
```

<table>
  <thead>
    <tr>
      <th>Key</th>
      <th>Type</th>
      <th>Default</th>
      <th>Description</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td><code>coverage.include</code></td>
      <td>string[]</td>
      <td>(all files)</td>
      <td>If set, only files matching at least one pattern are included in coverage reports. Useful for monorepos.</td>
    </tr>
    <tr>
      <td><code>coverage.exclude</code></td>
      <td>string[]</td>
      <td>(none)</td>
      <td>Files matching any pattern are excluded from coverage reports. Applied after include.</td>
    </tr>
  </tbody>
</table>

### Pattern syntax

Patterns use glob matching with `**` for recursive directory matching. File paths are **relative to the git root** (e.g., `backend/src/db/migrations/1700-Init.ts`).

| Pattern | Matches |
|---------|---------|
| `**/migrations/**` | Any file in any `migrations/` directory |
| `backend/src/**` | All files under `backend/src/` |
| `**/*.test.ts` | Any `.test.ts` file |
| `backend/src/db/migrations/**` | Specific subdirectory |
| `migrations/**` | **Won't match** &mdash; paths include the full git-relative prefix |

## Output

### Console output

Coverage is displayed in two places during a run:

**Per-test (inline):** After each test completes, a single line shows how many lines that specific test covered:
```
NO DEVIATION - dc14ba0733bdba8b65c11f14c6407320 (63ms)
  ↳ coverage: 59 lines across 10 files
```

**Aggregate (end of run):** After all tests complete, the full summary shows:
```
📊 Coverage: 85.9% lines (55/64), 42.9% branches (6/14) across 2 files

  Per-file:
    server.js                                 85.2% (52/61)
    tuskDriftInit.js                         100.0% (3/3)

  Per-test:
    GET /api/random-user                     4 lines across 1 files
    POST /api/create-post                    5 lines across 1 files
```

In TUI mode, the aggregate summary appears in the service logs panel after all tests complete. Per-test detail is shown in each test's log panel.

### LCOV export

```bash
tusk drift run --cloud --show-coverage --coverage-output coverage.lcov --print
```

Compatible with Codecov, Coveralls, SonarQube, VS Code, and most coverage tools.

**Note on validation runs:**
- **In-suite tests** are always included in coverage output, even if they fail (a failing test still exercises code paths).
- **Draft tests** are excluded from coverage output. Draft coverage data is uploaded to the backend for promotion decisions ("does this draft add unique coverage?").
- **After promotion**, the Tusk Cloud dashboard may show slightly higher coverage than the LCOV file (newly promoted drafts are included). The LCOV catches up on the next validation run.

### JSON export

```bash
tusk drift run --cloud --show-coverage --coverage-output coverage.json --print
```

JSON includes three top-level fields:

- `summary` — aggregate stats, per-file percentages, per-test line counts
- `aggregate` — line-level hit counts and branch data for every file
- `per_test` — per-test per-file covered lines

```json
{
  "summary": {
    "aggregate": { "coverage_pct": 85.9, "total_covered_lines": 55, "total_coverable_lines": 64 },
    "per_file": { "server.js": { "coverage_pct": 85.2, "covered_lines": 52, "coverable_lines": 61 } },
    "per_test": [{ "test_name": "GET /api/random-user", "covered_lines": 4, "files_touched": 1 }]
  },
  "aggregate": {
    "server.js": {
      "lines": { "1": 1, "5": 3, "12": 0 },
      "total_branches": 14,
      "covered_branches": 6,
      "branches": { "25": { "total": 2, "covered": 1 } }
    }
  },
  "per_test": {
    "trace-id-abc": {
      "server.js": { "covered_lines": [5, 15, 22], "covered_count": 3, "files_touched": 1 }
    }
  }
}
```

## How It Works

1. CLI starts your service with coverage env vars (`NODE_V8_COVERAGE` for Node, `TUSK_COVERAGE` for Python)
2. After the service is ready, CLI takes a **baseline snapshot** — all coverable lines (including uncovered) for the denominator
3. After each test, CLI takes a **per-test snapshot** — only lines executed since the last snapshot (counters auto-reset)
4. CLI merges per-test data with baseline to compute the aggregate

Coverage data flows via the existing CLI-SDK protobuf channel. No extra HTTP servers or ports.

**Node.js:** Uses V8's built-in precise coverage. No external dependencies. TypeScript source maps handled automatically (`sourceMap: true` in tsconfig required). See the [Node SDK coverage docs](https://github.com/Use-Tusk/drift-node-sdk/blob/main/docs/coverage.md) for internals.

**Python:** Uses `coverage.py` with `branch=True`. Requires `pip install coverage`. See the [Python SDK coverage docs](https://github.com/Use-Tusk/drift-python-sdk/blob/main/docs/coverage.md) for internals.

## Docker Compose

For services running in Docker Compose, two things are needed:

### 1. Pass coverage env vars to the container

Add to `docker-compose.tusk-override.yml`:

```yaml
services:
  your-service:
    environment:
      - TUSK_COVERAGE=${TUSK_COVERAGE:-}              # pass through from CLI
      - NODE_V8_COVERAGE=/tmp/tusk-v8-coverage        # Node.js only: fixed container path
```

`TUSK_COVERAGE` is passed through from the CLI using `${TUSK_COVERAGE:-}`. `NODE_V8_COVERAGE` must be a **fixed container path** — not `${NODE_V8_COVERAGE:-}` — because the CLI creates a host temp directory that doesn't exist inside the container.

**Python containers:** Add `coverage>=7.0` to your `requirements.txt`. No `NODE_V8_COVERAGE` needed.

### 2. Strip container path prefix

Coverage paths from Docker are container-absolute (e.g., `/app/app/api/views.py`). Use `strip_path_prefix` to convert them to repo-relative paths:

```yaml
coverage:
  enabled: true
  strip_path_prefix: "/app"    # your Docker volume mount point
```

This strips `/app` from all paths, so `/app/app/api/views.py` becomes `app/api/views.py` — matching the file path in your git repo. Set this to whatever your `docker-compose.yaml` volume mount maps your project root to (e.g., `- .:/app` → use `/app`).

## Limitations

- **Concurrency forced to 1.** Per-test snapshots rely on counter resets between tests.
- **Only loaded files tracked.** Files never imported by the server (standalone scripts, test files, unused utils) don't appear in coverage. The denominator only includes files V8/Python actually loaded.
- **Startup code inflates coverage.** Module loading, decorator execution, and DI registration all count as "covered lines." A single test may show 20%+ coverage on a large app from startup alone.
- **TypeScript compiled output.** If using `tsc`, ensure a clean build (`rm -rf dist && tsc`) to avoid stale artifacts with broken imports.
- **Multi-process servers.** Node cluster mode and gunicorn with multiple workers need single-process mode for coverage.
- **Python overhead.** `coverage.py` adds 10-30% execution overhead via `sys.settrace()`. V8 coverage is near-zero overhead.
- **Python branch coverage** uses a private coverage.py API (`_analyze()`). May break on major coverage.py upgrades.
- **Docker paths.** Coverage paths are container-absolute by default. Use `coverage.strip_path_prefix` to convert to repo-relative paths (see Docker Compose section above).
