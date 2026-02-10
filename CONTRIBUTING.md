# Contributing

Thanks for helping improve the Tusk Drift CLI!

If you have any questions, feel free to open an issue or email [support@usetusk.ai](support@usetusk.ai).

## Quick start

- Requirements:
  - Go 1.25+
  - (Optional) Nix: `nix develop` to enter a dev shell
- Clone and prepare:
  - Private modules: `go env -w GOPRIVATE="github.com/Use-Tusk/*"`
  - Install deps: `make deps`
  - Build: `make build`
  - Run: `go run . --help`

## Dev workflow

- Common targets:
  - `make build` — build the binary (`./tusk`)
  - `make run` — build and run
  - `make test` — run tests (module root)
  - `make test-ci` — run tests for all packages
  - `make deps` — download/tidy modules
  - `make fmt` — `go fmt .`
  - `make lint` — run `golangci-lint`
  - `make build-ci` — build with version info (used in CI)
- Logging:
  - Use `log.Debug` for debug logs (from `internal/log`).
  - Use `log.ServiceLog` or `log.TestLog` to display logs in the TUI.
  - Use `log.UserInfo`, `log.UserWarn`, `log.UserError`, `log.UserSuccess` for styled user-facing output.

## Local replay (developer service)

1) Create `.tusk/config.yaml` in your service repo (see `docs/configuration.md`).
2) Ensure `service.start.command` and `service.port` are correct.
3) Run:

   ```bash
   tusk run --trace-dir .tusk/traces
   # or
   tusk run --trace-file path/to/trace.jsonl
   ```

4) Useful flags:
   - `--concurrency N`
   - `--enable-service-logs` (writes `.tusk/logs/...`)
   - `--filter 'regex'`
   - `--save-results [--results-dir DIR]`

- Keep CLI, SDK, and backend aligned with schema changes.

## Code structure

- `cmd/` — Cobra commands (e.g., `run`)
- `internal/`
  - `api/` — protobuf-over-HTTP client for backend (TestRunService)
  - `auth/` — Auth0 device code flow + token persistence
  - `config/` — config loader (Koanf)
  - `runner/` — Executor, Unix socket server, matcher, comparison
  - `tui/` — interactive test runner (Bubble Tea)
  - `utils/` — helpers
- `docs/` — architecture and configuration docs

## Style and conventions

- Keep edits focused and covered by tests where possible.
- Update [`docs/architecture.md`](docs/architecture.md) and/or [`docs/configuration.md`](docs/configuration.md) when adding flags or config.
- Prefer small, reviewable PRs with a clear rationale.
- Use meaningful slog fields (avoid logging sensitive values).

## Testing

```bash
go test ./...
go test -v ./...
go test -cover ./...
```

## Troubleshooting

**"command not found" after go install:**

- Add `$GOPATH/bin` to your PATH
- Or use `go env GOPATH` to find the path

**Module issues:**

```bash
go mod tidy    # Clean up dependencies
```

**Build cache issues:**

```bash
go clean -cache
go clean -modcache
```

**Runtime issues:**

- Port in use: stop any process on `service.port`.
- Readiness failing: check `service.readiness_check.*` or add a health endpoint.
- SDK connect failure: version mismatch or missing `TUSK_MOCK_SOCKET`.

## For Maintainers

### Development environment variables

Set required env vars in your shell if necessary. To avoid potential conflicts with the service's env vars when running tests locally, this CLI doesn't load from `.env`.

Testing auth to Tusk dev:

- `TUSK_AUTH0_DOMAIN`
- `TUSK_AUTH0_CLIENT_ID`

By default, if there's a `.tusk/config.yaml` we'll use the API url in that for all Tusk Cloud requests. If no `.tusk/config.yaml` is found, we'll use the default API url (prod API url). You can override this by setting `TUSK_API_URL` in your shell.

### Disabling analytics

Analytics can be disabled by setting `is_tusk_developer` to `true` in the config file (e.g. `Users/name/Library/Application Support/tusk/cli.json`).

### Releasing

Releases are automated using [GoReleaser](https://goreleaser.com/) via GitHub Actions.

#### Creating a release

Use the release script to create and push a new version tag:

```bash
# Patch release (v1.0.0 → v1.0.1)
./scripts/release.sh patch

# Minor release (v1.0.0 → v1.1.0)
./scripts/release.sh minor
```

The script runs preflight checks, calculates the next version, and prompts for confirmation before tagging.

Once the tag is pushed, GitHub Actions will automatically:

- Build binaries for all supported platforms
- Create archives with README and LICENSE
- Generate checksums
- Create a GitHub release with changelog
- Upload all artifacts
- Update the latest version manifest on [GitHub Pages](https://use-tusk.github.io/tusk-drift-cli/).

#### Supported platforms

The release workflow builds for:

- **Linux**: amd64, arm64
- **macOS (darwin)**: amd64, arm64
- **Windows**: amd64, arm64

All binaries include embedded version information:

- `internal/version.Version` — git tag
- `internal/version.BuildTime` — build timestamp  
- `internal/version.GitCommit` — git commit hash

#### Building locally for distribution

For local testing or manual builds:

```bash
# Build for current platform
go build -o tusk

# Cross-compile (example: Linux)
GOOS=linux GOARCH=amd64 go build -o tusk-linux

# With version info (mimics CI builds)
VERSION=v1.2.3
BUILD_TIME=$(date -u '+%Y-%m-%d %H:%M:%S UTC')
GIT_COMMIT=$(git rev-parse HEAD)

go build \
  -ldflags "-X github.com/Use-Tusk/tusk-drift-cli/internal/version.Version=${VERSION} \
            -X github.com/Use-Tusk/tusk-drift-cli/internal/version.BuildTime=${BUILD_TIME} \
            -X github.com/Use-Tusk/tusk-drift-cli/internal/version.GitCommit=${GIT_COMMIT}" \
  -o tusk
```

To test the GoReleaser configuration locally:

```bash
goreleaser release --snapshot --clean
```
