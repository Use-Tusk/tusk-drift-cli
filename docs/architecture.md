# Architecture Overview

## Goals & Scope

The Tusk Drift CLI fulfills these responsibilities:

1. Replay recorded API traces against a service under test deterministically.
2. Block live outbound I/O by serving recorded mocks via a local Unix socket server.
3. Compare actual vs recorded responses and report deviations.
4. In Cloud mode, coordinate with the Tusk backend to fetch tests and stream results.

## Key Concepts

- **Service under test (SUT)**: The service you're testing, instrumented with a Tusk Drift SDK.
- **Trace test**: A replayable inbound HTTP request plus associated outbound spans recorded from a prior run.
- **Inbound vs outbound requests**: The replayed HTTP request is inbound; SDK-captured calls your service makes (e.g., HTTP / Postgres / etc) are outbound.
- **Mock match scope**: Prefer the current trace; fall back to suite‑wide spans. In the future, we may implement the concept of "global spans" to optimize cross-trace spans (mostly for session-based requests).
- **Suite spans**: Spans loaded once per run to improve matching across tests (pre‑app start or common outbound calls).

## Components

### Commands

Key commands:

| Command | Description |
|:------:|-------------|
| `init` | An onboarding wizard to set up a new service with Tusk |
| `list` | List available traces for replay ([filters supported](./filter.md)) |
| `run` | Run trace tests ([filters supported](./filter.md)) |
| `login` | Login to Tusk Drift Cloud |
| `version` | Show current CLI version |
| `status` | Show authentication status |

For more details, check `tusk --help`.

### Test runner

The `runner` package implements logic to manage the lifecycle of trace replays. The key components are:

- [**Executor**](../internal/runner/executor.go): Loads tests, starts environment, runs tests, compares results, outputs/streams results.
- [**Server**](../internal/runner/server.go): Unix socket listener via protobuf protocol, responds to mock requests from the SDK, records match events and inbound replay spans.
- [**Mock matcher**](../internal/runner/mock_matcher.go): Priority‑based matching over per‑trace and suite‑wide spans.

### TUI

The interactive UI orchestrates execution around the `Executor`, surfaces live logs, and streams results in cloud mode. We use Charm packages here: [bubbletea](github.com/charmbracelet/bubbletea) (UI), [bubbles](github.com/charmbracelet/bubbles) (UI components), [lipgloss](github.com/charmbracelet/lipgloss) (styling).

- **Layout**
  - Header: spinner + progress bar + live stats (completed, running, passed, failed).
  - Left/Top: Tests table (includes a “(service logs)” row and one row per test).
  - Right/Bottom: Log panel (shows service logs or the selected test’s logs).
  - Adaptive layout (horizontal/vertical) based on terminal width; compact header on narrow terminals.
  
  ![Run result view](/assets/tui-run-result.png)

- **Logging model**
  - Service logs and per‑test logs are captured separately.
  - Test logs include replay lifecycle and deviations; service logs include environment startup/readiness and general output.
  - Selecting a test in the table switches the log panel to that test; selecting the top “(service logs)” row shows service logs.

- **Cloud streaming and persistence**
  - If configured, calls `OnTestCompleted(result, test, executor)` non‑blocking to stream results to the backend in near‑real time.
  - On completion, calls `OnAllCompleted(results, tests, executor)` and, if results output is enabled, writes a `.json` file (path shown in logs).

You can also run in non-interactive mode by passing `--print` to `tusk run`.

### Backend client

Protobuf‑over‑HTTP client for communications to the Tusk Drift Cloud backend (create run, get tests, upload results, finalize).

**Typical flow (`tusk run --cloud`)**:

1. Create run, set CI status to `RUNNING`.
2. Optionally fetch pre‑app‑start/global spans; inject into the server for matching.
3. Fetch tests (paginated) or defer to the TUI to load.
4. Execute trace tests, stream per‑test results as they complete.
5. On completion, finalize CI status (`SUCCESS` if all passed; otherwise `FAILURE`).

## High‑level Architecture (Data/Control Plane)

```mermaid
graph TD
  subgraph Local Machine
    CLI["Tusk Drift CLI"]
    RUNNER["Executor + Mock Server"]
    APP["SUT + Drift SDK"]
    TRACES["Local trace files (.jsonl)"]
  end

  subgraph Tusk Drift Cloud
    BE["API endpoints (protobuf over HTTP)"]
    SPANS["Pre-app start & global spans"]
  end

  CLI --> RUNNER
  RUNNER -- sets env TUSK_MOCK_SOCKET,TUSK_DRIFT_MODE --> APP
  APP -- Unix socket (protobuf) --> RUNNER
  RUNNER -- read spans (local only) --> TRACES
  RUNNER -- Get*Spans / CreateRun / GetTests / UploadResults / UpdateCI --> BE
  BE -- optional spans --> RUNNER
```

## End‑to‑end Flow (Local)

```mermaid
sequenceDiagram
participant TR as Local Traces
  participant CLI as CLI (run)
  participant RUN as Mock Server
  participant APP as SUT + Drift SDK
  

  CLI->>RUN: Start Unix socket server
  CLI->>APP: Start process with env<br/>TUSK_MOCK_SOCKET, TUSK_DRIFT_MODE=REPLAY
  APP->>RUN: SDK_CONNECT(sdkVersion, minCli)
  RUN-->>APP: ACK or ERR (version check)
  CLI->>TR: Load recorded spans for test
  CLI->>APP: Replay inbound HTTP (adds x-td-trace-id, x-td-env-vars)
  APP->>RUN: MOCK_REQUEST(outboundSpan, stackTrace)
  RUN->>RUN: FindBestMatch (trace → suite → (future) global)
  RUN-->>APP: Mock response
  APP-->>CLI: Inbound HTTP response
  CLI->>CLI: Compare status/headers/body (ignore dynamic fields per config)
  CLI->>CLI: Save results (optionally .tusk/results/*.json)
```

Notes:

- CLI waits up to ~10s for SDK ACK; messages are length‑prefixed (4‑byte big endian), with a 1MB cap.
- Inbound replay adds headers: `x-td-trace-id` and `x-td-env-vars`.

## End‑to‑end Flow (Cloud)

```mermaid
sequenceDiagram
  participant CLI
  participant RUN as Mock Server + Executor
  participant APP as SUT + Drift SDK
  participant BE as Tusk Drift Cloud server

  CLI->>BE: CreateDriftRun(commit/pr/branch/checkRunID)
  CLI->>BE: (optional) GetGlobalSpans / GetPreAppStartSpans
  CLI->>RUN: Start server, set suite spans if any
  CLI->>APP: Start service with env
  APP->>RUN: SDK_CONNECT → ACK/ERR
  loop For each test
    CLI->>BE: GetDriftRunTraceTests (or preloaded)
    CLI->>APP: Replay inbound
    APP->>RUN: MOCK_REQUEST(...)
    RUN-->>APP: Mock response
    APP-->>CLI: Inbound response
    CLI->>CLI: Compare & produce result
    CLI->>BE: UploadTraceTestResults (stream per test)
  end
  CLI->>BE: UpdateDriftRunCIStatus(SUCCESS/FAILURE)
```

Tusk will leave a comment on your pull request with a summary of test results.

## Matching Mocks

When SUT encounters an outbound request over the lifetime of a trace, SDK intercepts this and requests a mock span from traces loaded in the CLI. We implement a mock matching algorithm to fulfill this ([`internal/runner/mock_matcher.go`](../internal/runner/mock_matcher.go)).

Spans are first considered per trace, then suite‑wide fallback.

Highest to lowest priority:

1. Input value hash (unused → used).
2. Reduced input value (headers removed) hash (unused → used).
3. Input schema hash (unused → used).
4. [Future] Global mocks from Tusk Drift Cloud.
5. Suite‑wide versions of the above when no per‑trace match.

Notes:

- Ties break by earliest recorded timestamp.
- Each match emits a match event (priority, scope, strategy, optional stack trace), and these events are attached to results.

## Evaluation of Trace Results

We compare the runtime response of the root/server span with its recorded output.

Basis for comparison:

- Status code equality
- Header equality (subset based on recorded expectations)
- JSON body structural comparison with support to ignore:
  - Fields (paths), regex patterns
  - UUIDs, timestamps, dates (ignored by default based on regex)
  - This is configurable via `.tusk/config.yaml` → `comparison.*`.

For CI/CD checks, Tusk Drift Cloud adds a more intelligent and powerful layer of classification capabilities, scoped to your pull requests, to determine whether deviations are intended.

## Configuration

Environment: `TUSK_` prefix overrides (e.g., `TUSK_SERVICE_PORT=8080`).

Example:

```yaml
service:
  id: "my-service-id"                 # required for --cloud
  name: "my-service"
  port: 3000
  start:
    command: "npm run dev"
  readiness_check:
    command: "curl -sf http://localhost:3000/health"
    timeout: "30s"
    interval: "2s"

tusk_api:
  url: "https://example.usetusk.ai"   # required for --cloud

test_execution:
  concurrency: 5
  timeout: "30s"

comparison:
  ignore_fields: ["$.data.id", "$.metadata.requestId"]
  ignore_patterns: ["^x-request-id$"]
  ignore_uuids: true
  ignore_timestamps: true
  ignore_dates: true

results:
  dir: ".tusk/results"
```

## Trace Replay Operational Details

- **Env/Flags**: `TUSK_MOCK_SOCKET` and `TUSK_DRIFT_MODE=REPLAY` are injected by the CLI into the service env.
- **Readiness**: If no readiness command is configured, the CLI waits ~10s before replay.
- **Concurrency**: Tests run concurrently (default 5), with per‑test attribution for mock events.
- **Service logs**: Written to `.tusk/logs/tusk-replay-*.log` when `--enable-service-logs` is provided.
- **Timeouts**: SDK ACK ~10s, HTTP client ~30s. Socket messages are capped at 1MB.

## Troubleshooting

- **Version mismatch**: `SDK_CONNECT` fails if CLI < min required by SDK or SDK < min required by CLI. The CLI will abort early with guidance.
- **Port in use**: CLI will not start the SUT if the configured port is occupied.
- **Readiness timeout**: Fails the environment start; nothing is replayed.
- **No mock found**: The SDK’s outbound may fail or be blocked; investigate suite/global spans and match priorities.
- **Missing API URL (cloud)**: CLI requires `tusk_api.url` and either API key (`x-api-key`) or bearer token.
