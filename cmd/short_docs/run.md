# Replay Traces

`tusk run` replays recorded traces (API tests) against your service. These tests can be run from local files, folders, or by test ID.

An interactive session is started by default; use `-p`/`--print` for noninteractive output.

## How it works

- The CLI starts your app using `.tusk/config.yaml` and waits for readiness.
- It launches a local Unix socket server and sets `TUSK_DRIFT_MODE=REPLAY` for your app.
- Your app (with the Tusk Drift SDK) connects to the CLI and requests mocks for outbound calls.
- The CLI replays inbound requests from recorded traces, blocks live outbound calls, and returns recorded responses if they are present in existing traces.
- Actual vs recorded responses are compared; results are shown/saved.

```text
+-------------------------+                                    +------------------------+
| Your service + SDK      | <================================> | tusk run (CLI)         |
| (REPLAY mode)           |          Unix domain socket        | mock server + traces   |
+-----------+-------------+                                    +-----------+------------+
            ^                                                               |
            | (1) SDK_CONNECT --------------------------------------------> |
            | <-------------------------------------------- (2) ACK/compat  |
            |                                                               |
            | <------------------------ (3) Inbound HTTP replay from trace  |
            |                                                               |
            |  (4) App triggers outbound call (PG/HTTP/GraphQL/...)         |
            |  (5) Request mock via socket -------------------------------> |
            | <---------------------------- (6) Recorded mock response ---- |
            |                                                               |
            |  (7) Service responds to inbound; CLI compares actual vs      |
            |      recorded and shows/saves results                         |
            v                                                               v
      App continues with mocked I/O                                 Logs / Results
```

## Requirements

- Your service must be instrumented with the Tusk Drift SDK and initialized early in process startup (before requiring instrumented libraries).
- `.tusk/config.yaml` must specify how to start your app and (optionally) check readiness. Use `tusk init` to create it.
- Target port must be free; stop any existing service first.
- CLI and SDK versions must be compatible; the CLI will fail fast with a helpful message if not.

### Cloud mode

- `tusk run --cloud`: fetches all tests from Tusk Drift Cloud for your service and runs them locally; no Tusk Drift run is created and results are not uploaded.
- `tusk run --cloud --ci`: creates a Tusk Drift run, fetches your test suite from Tusk Drift Cloud, and uploads test results. Use `-a/--all-cloud-trace-tests` to run all tests instead of the run-scoped suite.
- `tusk run --cloud --ci --validate-suite-if-default-branch`: on the default branch, creates a validation run, fetches draft and in-suite tests, and validates they can be replayed before adding them to the suite. On other branches, this flag is ignored and the command behaves like `--cloud --ci`.
- `--trace-test-id` may be used with `--cloud` to run a single cloud trace test.
