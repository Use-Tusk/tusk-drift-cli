# Tusk Review

`tusk review` runs the Tusk AI code review against your local working tree — before you push or open a PR. It uploads a git patch (not the whole repo), polls for completion, and prints the result to stdout.

The CLI never posts comments to GitHub or GitLab. All output is local.

## Typical workflow

1. Make some changes in a repo that's connected to Tusk.
2. Run `tusk review` from the repo directory.
3. Stderr shows progress; stdout shows the final review when it's done.

## Authentication

Run `tusk auth login`, or set `TUSK_API_KEY` for non-interactive use.

## Repo identity

By default, the repo is detected from the `origin` remote (`owner/name`). Override with `--repo owner/name`. The repo must already be connected to Tusk.

## Base branch resolution

The "base" is the commit your changes sit on top of. By default, `tusk review` uses `git merge-base origin/HEAD HEAD` — the point your branch diverged from origin's default branch.

Pass `--base <ref-or-sha>` to override. This is the right thing to do on **stacked branches** (feature-2 branched off feature-1 branched off main): without it, your review will critique feature-1's changes as if they were yours.

```
main:       A ─ B ─ C
                     \
feature-1:            D ─ E           (open PR, not merged)
                            \
feature-2:                    F ─ G   (current branch)
```

Here, run `tusk review --base origin/feature-1` so the diff is just F–G.

## Output

- Default: human-readable text to stdout, progress on stderr.
- `--json`: backend-rendered JSON document to stdout, suitable for `| jq`.
- `--output <file>`: write the result to a file (format follows `--json`).
- `--quiet`: suppress stderr progress (final output unchanged).

## Filtering

Locked files and common build output are skipped automatically (same list the server-side review uses). To tweak:

- `.tuskignore` at the repo root: `.gitignore`-style globs, additive to the defaults.
- `--exclude <glob>`: one-off add. Repeatable.
- `--include <glob>`: cancel a default skip (e.g. `--include 'package-lock.json'`). Repeatable.

## Exit codes

- `0` — review completed (issues may or may not be found)
- `1` — run failed, or network / auth error
- `2` — user-actionable pre-flight error (mid-rebase, rate limit, patch too large, repo not connected, couldn't determine base)

## Status subcommand

```
tusk review status <run-id>           # print current status snapshot
tusk review status <run-id> --watch   # block until run reaches a terminal state
```
