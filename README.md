![Tusk CLI Banner](assets/tusk-banner.png)

Tusk Drift is an API test record/replay system that lets you run realistic tests generated from real traffic. This CLI orchestrates local and CI test runs, coordinating with a Tusk Drift SDK and Tusk Drift Cloud.

<div align="center">

![GitHub Release](https://img.shields.io/github/v/release/Use-Tusk/tusk-drift-cli)
[![Build and test](https://github.com/Use-Tusk/tusk-drift-cli/actions/workflows/main.yml/badge.svg?branch=main)](https://github.com/Use-Tusk/tusk-drift-cli/actions/workflows/main.yml)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![X URL](https://img.shields.io/twitter/url?url=https%3A%2F%2Fx.com%2Fusetusk&style=flat&logo=x&label=Tusk&color=BF40BF)](https://x.com/usetusk)

</div>

SDKs:

- [Node.js](https://github.com/Use-Tusk/drift-node-sdk)
- ...more to come!

## Features

- Replay recorded traces against your service under test
- Deterministic outbound I/O via local mock server
- JSON response comparison with dynamic field rules (UUIDs, timestamps, dates, etc.)
- Tusk Drift Cloud: fetch and replay tests stored with Tusk, and upload test results for intelligent classification of regressions in CI/CD checks

<div align="center">

![Demo](assets/tusk-drift-demo.gif)
<p><a href="https://github.com/Use-Tusk/drift-node-demo">Try it on a demo repo →</a></p>

</div>

## Install

### Quick install (recommended)

**Linux/macOS:**

```bash
curl -fsSL https://raw.githubusercontent.com/Use-Tusk/tusk-drift-cli/main/install.sh | sh
```

**Homebrew:**

Coming soon.

**Windows:**

Download the latest release from [GitHub Releases](https://github.com/Use-Tusk/tusk-drift-cli/releases/latest):

1. Download `tusk-drift-cli_*_Windows_x86_64.zip`
2. Extract the ZIP file
3. Move `tusk.exe` to a directory in your PATH, or add the extracted directory to your PATH

### Manual Download

Download pre-built binaries from [GitHub Releases](https://github.com/Use-Tusk/tusk-drift-cli/releases/latest).

### Build from source

```bash
# Go 1.25+
git clone https://github.com/Use-Tusk/tusk-drift-cli.git
cd tusk-drift-cli
make deps
make build

tusk --help
```

## Quick start

Initialize a service:

```bash
cd path/to/your/service
tusk init
```

An onboarding wizard will guide you to create your `.tusk/config.yaml` config file.
You can also create the `.tusk` directory and config file manually in your root directory of your service. See [configuration docs](/docs/configuration.md).

You will need to record traces for your service. See your respective SDK's guide for more details. Once you have traces recorded, you can replay them with the `tusk run` command.

Local traces (default):

```bash
# Run all tests from local traces
tusk run

# Or specify source
tusk run --trace-dir .tusk/traces
tusk run --trace-file path/to/trace.jsonl
tusk run --trace-id <traceId>

# Common flags
tusk run --filter '^/api/users' --concurrency 10 --enable-service-logs
tusk run --save-results --results-dir .tusk/results
```

Cloud mode:

```bash
# Provide a service ID and API URL in config (see below)
# Auth via API key (recommended) or device login
export TUSK_API_KEY=your-key

# Or, device login
tusk login

# Run against Tusk Drift Cloud
tusk run --cloud
tusk run --cloud --trace-test-id <id>           # single test from backend
tusk run --cloud --all-cloud-trace-tests        # run all tests for service
```

## Usage

List traces:

```bash
# Local traces
tusk list
tusk list --trace-dir .tusk/traces

# With Tusk Drift Cloud
tusk list --cloud
```

Interactive TUI (default when attached to a terminal):

```bash
tusk run
```

Run headless mode with JSON output for a single test:

```bash
tusk run --trace-id <id> --print --output-format=json
```

How this program uses your `.tusk` directory:

- Recordings of your app's traffic will be stored in `.tusk/traces` by default.
Specify `traces.dir` in your `.tusk/config.yaml` to override.
- If `--save-results` is provided, results will be stored in `.tusk/results` by default. Specify `results.dir` in your `.tusk/config.yaml` to override.
- If `--enable-service-logs` or `--debug` is used, trace replay service logs will be stored in `.tusk/logs`.

We recommend adding to your `.gitignore`:

- `.tusk/results`
- `.tusk/logs`
- `.tusk/traces` (if you primarily intend to use Tusk Drift Cloud)

## Troubleshooting

- SDK connect failure: ensure your service uses the Tusk Drift SDK and is started by the CLI (so it sees `TUSK_MOCK_SOCKET`).
- Port in use: the CLI will block if `service.port` is already taken.
- Readiness: if `service.readiness_check.command` is omitted, the CLI waits ~10s before replay.
- No mock found: check suite spans availability and matching rules; ensure traces exist for the trace being replayed.
- For Cloud mode, ensure `service.id`, `tusk_api.url`, and `TUSK_API_KEY` or `tusk login` are set.

If you have any questions, feel free to open an issue or reach us at [support@usetusk.ai](support@usetusk.ai).

## Resources

- [Architecture overview](docs/architecture.md)
- [Configuration](docs/configuration.md)

## Development

See [`CONTRIBUTING.md`](./CONTRIBUTING.md).

## License

See [`LICENSE`](./LICENSE).
