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
  - Use `slog.Debug` for debug logs.
  - Use `fmt.Println` for runtime logs in headless mode.
  - Use `logging.LogToService` or `logging.LogToCurrentTest` to display logs in the TUI.

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

## Releasing (maintainers)

### Building for distribution

```bash
# Build for current platform
go build -o tusk

# Cross-compile (example: Linux)
GOOS=linux GOARCH=amd64 go build -o tusk-linux

# With version and build info (in CI/CD)
VERSION=${GITHUB_REF#refs/tags/}
BUILD_TIME=$(date -u '+%Y-%m-%d %H:%M:%S UTC')
GIT_COMMIT=$(git rev-parse HEAD)

go build \
  -ldflags "-X github.com/Use-Tusk/tusk-drift-cli/internal/version.Version=${VERSION} \
            -X github.com/Use-Tusk/tusk-drift-cli/internal/version.BuildTime=${BUILD_TIME} \
            -X github.com/Use-Tusk/tusk-drift-cli/internal/version.GitCommit=${GIT_COMMIT}" \
  -o tusk
```

- Tag with semantic version.
- CI uses `make build-ci` to embed:
  - `internal/version.Version`
  - `internal/version.BuildTime`
  - `internal/version.GitCommit`
- Publish artifacts for supported platforms (TBD/goreleaser in future).

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
