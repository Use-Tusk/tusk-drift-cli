# Tusk Review Run

`tusk review run` submits a code review for your local working tree. The CLI builds a git patch, uploads it, polls for completion, and prints the result to stdout. Nothing is pushed to your remote, and the CLI never posts comments to GitHub or GitLab — all output is local.

## Typical workflow

1. Make some changes in a repo that's connected to Tusk.
2. Run `tusk review run` from the repo directory.
3. Stderr shows progress; stdout shows the final review when it's done.

## Repo identity

By default, the repo is detected from the `origin` remote (`owner/name`). Override with `--repo owner/name`. The repo must already be connected to Tusk.

## Output

- Default: human-readable text to stdout, progress on stderr.
- `--json`: backend-rendered JSON document to stdout, suitable for `| jq`.
- `--output <file>`: write the result to a file (format follows `--json`).
- `--quiet`: suppress stderr progress (final output unchanged).

## Filtering

Lockfiles and common build output are skipped automatically (same list the server-side review uses). To tweak:

- `.tuskignore` at the repo root: `.gitignore`-style globs, additive to the defaults.
- `--exclude <glob>`: one-off add. Repeatable.
- `--include <glob>`: cancel a default skip (e.g. `--include 'package-lock.json'`). Repeatable.

## Fire-and-forget (`--no-poll`)

`tusk review run --no-poll` uploads the patch, prints the run id to stdout, and exits immediately — no polling, no blocking. Useful for CI scripts and for kicking off a review from one machine and reading it on another.

```
$ tusk review run --no-poll
cr_01JK7...
$ tusk review status cr_01JK7... --watch
```

With `--json`:

```
$ tusk review run --no-poll --json
{"runId":"cr_01JK7..."}
```

Ctrl+C on a `--no-poll` run has no effect on the backend (the run is already submitted and independent of the CLI process).

## Exit codes

- `0` — review completed (issues may or may not be found)
- `1` — run failed, or network / auth error
- `2` — user-actionable pre-flight error (mid-rebase, rate limit, patch too large, repo not connected, couldn't determine clone pivot, no active seat)
