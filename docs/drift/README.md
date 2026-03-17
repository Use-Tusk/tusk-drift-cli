# Tusk Drift

Tusk Drift is an API test record/replay system that lets you run realistic tests generated from real traffic. It coordinates with a [Tusk Drift SDK](#sdks) and Tusk Drift Cloud.

## Features

- Replay recorded traces against your service under test
- Deterministic outbound I/O via local mock server
- JSON response comparison with dynamic field rules (UUIDs, timestamps, dates, etc.)
- Tusk Drift Cloud: fetch and replay tests stored with Tusk, and upload test results for intelligent classification of regressions in CI/CD checks

<div align="center">

![Demo](../../assets/demo.gif)
<p><a href="https://github.com/Use-Tusk/drift-node-demo">Try it on a demo repo →</a></p>

</div>

## SDKs

- [Node.js](https://github.com/Use-Tusk/drift-node-sdk)
- [Python](https://github.com/Use-Tusk/drift-python-sdk)
- ...more to come!

## Quick start

### Setup agent (recommended)

Use our AI setup agent to automatically set up Tusk Drift for your service:

```bash
cd path/to/your/service
tusk drift setup
```

The agent will analyze your codebase, instrument the SDK, create configuration files, and test the setup with recording and replay.

<details>
<summary><b>Manual setup (alternative)</b></summary>

You can use the interactive wizard:

```bash
cd path/to/your/service
tusk drift init
```

This will guide you to create your `.tusk/config.yaml` config file. You can also create the `.tusk` directory and config file manually. See [configuration docs](configuration.md).

You will need to record traces for your service. See your respective SDK's guide for more details.
</details>

Once you have traces recorded, you can replay them with the `tusk drift run` command. For example, to replay local traces:

```bash
# Run all tests from local traces
tusk drift run

# Or specify source
tusk drift run --trace-dir .tusk/traces
tusk drift run --trace-file path/to/trace.jsonl
tusk drift run --trace-id <traceId>

# Common flags
tusk drift run --filter '^/api/users' --concurrency 10 --enable-service-logs
tusk drift run --save-results --results-dir .tusk/results
tusk drift run --sandbox-mode auto   # default: auto (choices: auto|strict|off)
```

## Tusk Drift Cloud

<div align="center">

![Tusk Drift Animated Diagram](../../assets/tusk-drift-animated-diagram-light.gif#gh-light-mode-only)
![Tusk Drift Animated Diagram](../../assets/tusk-drift-animated-diagram-dark.gif#gh-dark-mode-only)

</div>

You can use Tusk Drift as API tests in your CI/CD pipeline by running your test suite against commits in your pull requests. Tusk Drift Cloud offers storage of these tests alongside an additional layer of intelligence on deviations detected:

- Automatic recording of traces based on live traffic in your environment of choice
- Securely store these traces as test suites
- Analyze deviations (classification of intended vs unintended deviations), root cause of deviations against your code changes, and suggested fixes.

If you used `tusk drift setup`, cloud configuration is included. If you previously configured local setup, you can resume the agent for cloud setup using:

```bash
tusk drift setup --skip-to-cloud
```

<details>
<summary>Manual setup (wizard)</summary>

If you previously used the onboarding wizard via `tusk drift init`, you can also run the cloud onboarding wizard:

```bash
tusk drift init-cloud
```

</details>

You will be guided to:

- Authorize the Tusk app for your code hosting service
- Register your service for Tusk Drift Cloud
- Obtain an API key to use Tusk Drift in CI/CD pipelines

### Run Tusk Drift in CI/CD

#### GitHub

- We recommend adding your `TUSK_API_KEY` to your GitHub secrets.
- Refer to an [example GitHub Actions workflow](./cloud/github-workflow-example.yml). Adapt this accordingly for your service.

## Usage

List traces:

```bash
# Local traces
tusk drift list
tusk drift list --trace-dir .tusk/traces

# With Tusk Drift Cloud
tusk drift list --cloud
```

Interactive TUI (default when attached to a terminal):

```bash
tusk drift run

# Run against Tusk Drift Cloud
tusk drift run --cloud
tusk drift run --cloud --trace-test-id <id>           # single test from backend
tusk drift run --cloud --all-cloud-trace-tests        # run all tests for service
```

> [!TIP]
> The TUI is best viewed in a window size of at least 150 x 40.

Run headless mode with JSON output for a single test:

```bash
tusk drift run --trace-id <id> --print --output-format=json
```

How this program uses your `.tusk` directory:

- Recordings of your app's traffic will be stored in `.tusk/traces` by default.
Specify `traces.dir` in your `.tusk/config.yaml` to override.
- If `--save-results` is provided, results will be stored in `.tusk/results` by default. Specify `results.dir` in your `.tusk/config.yaml` to override.
- If `--enable-service-logs` or `--debug` is used, trace replay service logs will be stored in `.tusk/logs`.

We recommend adding to your `.gitignore`:

- `.tusk/results`
- `.tusk/logs`
- `.tusk/setup`
- `.tusk/traces` (if you primarily intend to use Tusk Drift Cloud)

## Resources

- [Architecture overview](architecture.md)
- [Configuration](configuration.md)
- [Troubleshooting](troubleshooting.md)
- [Filtering](filter.md)
