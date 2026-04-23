# Tusk Review

Tusk Review runs the Tusk AI code review against your local working tree — before you push or open a PR. It uploads a git patch (not the whole repo), surfaces issues in the terminal, and never posts comments to GitHub or GitLab.

## Subcommands

- `tusk review run` — submit a review of your current working tree.
- `tusk review status <run-id>` — check the status of a previously submitted run (pair with `--watch` to block until it finishes).

## Authentication

Run `tusk auth login`, or set `TUSK_API_KEY` for non-interactive use.

Run `tusk review <subcommand> --help` for flags, exit codes, and behavior specific to each subcommand.
