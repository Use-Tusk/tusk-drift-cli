# Tusk Review

Tusk Review provides CLI commands for running the [Tusk](https://usetusk.ai) AI code review against your local working tree — before you push or open a pull request. Surfaces issues in the terminal; never posts comments to GitHub or GitLab.

## Prerequisites

Authenticate with Tusk:

```bash
tusk auth login
```

Or set the `TUSK_API_KEY` environment variable. See [onboarding docs](https://docs.usetusk.ai/onboarding) for details.

The repo you're in must already be connected to Tusk (one-time setup by any user in your org). The CLI detects the repo from the `origin` remote; override with `--repo owner/name`.

## Commands

### `tusk review run`

Submit a code review for your current working tree. Generates a git patch (including uncommitted and untracked changes), uploads it, polls for completion, and prints the result.

```bash
tusk review run
tusk review run --json | jq .
tusk review run --output review.txt
tusk review run --min-severity high
```

Common flags:

- `--repo owner/name` — override the repo (defaults to `origin` remote).
- `--min-severity low|medium|high|critical` — severity floor for surfaced issues.
- `--exclude <glob>` / `--include <glob>` — extra path excludes, or re-include a default skip. Repeatable.
- `--json` — backend-rendered JSON to stdout (pipe to `jq`).
- `--output <file>` — write the result to a file instead of stdout.
- `--quiet` — suppress stderr progress.
- `--no-poll` — submit and exit immediately with the run id (see [Fire-and-forget](#fire-and-forget) below).

### `tusk review status <run-id>`

Check the status of a previously submitted run. Defaults to a one-shot snapshot.

```bash
tusk review status cr_01JK7...
tusk review status cr_01JK7... --watch
tusk review status cr_01JK7... --json
```

Pair with `--watch` to block until the run reaches a terminal state (SUCCESS, FAILED, or CANCELLED). Honors the same `--json`, `--output`, and `--quiet` flags as `run`.

## Typical workflow

1. Make changes in a repo connected to Tusk.

2. Run the review:

   ```bash
   tusk review run
   ```

   Stderr shows progress; stdout prints the final review when it's done (usually 3–5 min).

3. Fix issues locally.

## Fire-and-forget

Use `--no-poll` to submit the review and exit immediately. Useful for kicking off a review from one machine and reading it elsewhere or whenever you don't want to block a terminal:

```bash
$ tusk review run --no-poll
cr_01JK7...
$ tusk review status cr_01JK7... --watch
```

With `--json`, the run id is wrapped for programmatic consumers:

```bash
$ tusk review run --no-poll --json
{"runId":"cr_01JK7..."}
```

Ctrl+C on a `--no-poll` run has no effect on the backend — the run is independent of the CLI process.

## Filtering

Lockfiles, binaries, and common build output are skipped automatically (same list the server-side review uses). To tweak:

- `.tuskignore` at the repo root — `.gitignore`-style globs, additive to the defaults.
- `--exclude <glob>` — one-off add. Repeatable.
- `--include <glob>` — cancel a default skip (e.g. `--include 'package-lock.json'`). Repeatable.

If everything you changed is filtered out (e.g. a pure `package-lock.json` bump), the CLI exits 0 with "Nothing to review" — no wasted server-side run.

## Size limits

The CLI enforces patch size limits _before_ upload:

- **Hard caps**: 10,000 changed lines / 200 files / 1 MiB patch bytes. The CLI aborts with a top-contributors list.
- **Soft warnings** (stderr, continues): 2,000 lines / 50 files.

If you hit a hard cap, the error points at the biggest contributing files — usually unfiltered generated code or lockfiles not caught by the default filter. Add them to `.tuskignore` and rerun.

## Exit codes

- `0` — review completed (issues may or may not be found), or nothing to review after filtering.
- `1` — run failed (sandbox error, patch-apply failure, timeout, auth, network).
- `2` — user-actionable pre-flight or request failure:
  - mid-rebase / merge / cherry-pick / revert in the working tree
  - patch too large
  - submodule changes in the patch (not supported in v1)
  - repo not connected to Tusk under your current org
  - rate limit hit (with next-allowed timestamp)
  - no active seat for your code-hosting identity

## JSON output

The JSON schema is backend-rendered and documented server-side. Pipe through `jq` for filtering:

```bash
# Pretty-print
tusk review run --json --quiet | jq .
```
