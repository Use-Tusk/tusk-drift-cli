# Configuration Reference

This document lists all configuration options, defaults, environment overrides, and guidance. See [`docs/architecture.md`](architecture.md) for the end‑to‑end flow.

Where the CLI reads config from:

1. CLI flags (e.g., `--concurrency`, `--results-dir`, `--enable-service-logs`). See `--help` for each command for more details.
2. Environment variables (prefix `TUSK_`)
3. Config file (auto-discovered): `.tusk/config.yaml`, `.tusk/config.yml`, `tusk.yaml`, `tusk.yml`, or `~/.tusk/config.yaml`

**✨ Run `tusk init` in your service root directory to start a wizard to guide you through setting up your config file.**

## Service

<table>
  <thead>
    <tr>
      <th>Key</th>
      <th>Type</th>
      <th>Default</th>
      <th>Required</th>
      <th>Description</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td><code>service.id</code></td>
      <td>string</td>
      <td></td>
      <td>Cloud: yes</td>
      <td>Tusk Drift Cloud service identifier. Required in <code>--cloud</code> mode.</td>
    </tr>
    <tr>
      <td><code>service.name</code></td>
      <td>string</td>
      <td></td>
      <td>no</td>
      <td>Optional display name.</td>
    </tr>
    <tr>
      <td><code>service.port</code></td>
      <td>number</td>
      <td>3000</td>
      <td>no</td>
      <td>Port where your service listens. CLI will not continue if occupied.</td>
    </tr>
    <tr>
      <td><code>service.start.command</code></td>
      <td>string</td>
      <td></td>
      <td>yes</td>
      <td>Shell command to start your service. Executed via <code>/bin/sh -c</code>. e.g., <code>npm run start</code>.</td>
    </tr>
    <tr>
      <td><code>service.readiness_check.command</code></td>
      <td>string</td>
      <td></td>
      <td>no</td>
      <td>Polling command until it exits 0. If omitted, CLI waits ~10s. Highly recommended if your service has a health check endpoint.</td>
    </tr>
    <tr>
      <td><code>service.readiness_check.timeout</code></td>
      <td>duration</td>
      <td><code>10s</code> (effective)</td>
      <td>no</td>
      <td>Total time to wait for readiness. Examples: <code>30s</code>, <code>2m</code>.</td>
    </tr>
    <tr>
      <td><code>service.readiness_check.interval</code></td>
      <td>duration</td>
      <td><code>2s</code></td>
      <td>no</td>
      <td>Poll interval for the readiness command.</td>
    </tr>
  </tbody>
</table>

Runtime environment variables set by the CLI for your service:

- `TUSK_MOCK_SOCKET`: Unix socket path the SDK uses to talk to the CLI
- `TUSK_DRIFT_MODE=REPLAY`: Signals the SDK to run in replay mode

## Traces (local)

<table>
  <thead>
    <tr>
      <th>Key</th>
      <th>Type</th>
      <th>Default</th>
      <th>Required</th>
      <th>Env override</th>
      <th>Description</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td><code>traces.dir</code></td>
      <td>string</td>
      <td><code>.tusk/traces</code></td>
      <td>no</td>
      <td><code>TUSK_TRACES_DIR</code></td>
      <td>
        Directory to load local recorded traces when not in cloud mode. CLI flag <code>--trace-dir</code> overrides.
        The CLI searches this directory first; if not found, it falls back to <code>traces/</code>, <code>tmp/</code>, and <code>.</code>.
        <br><br>
        In local recording mode, the SDK will also save trace files to this directory.
      </td>
    </tr>
  </tbody>
</table>

## Tusk API (Cloud mode)

<table>
  <thead>
    <tr>
      <th>Key</th>
      <th>Type</th>
      <th>Default</th>
      <th>Required</th>
      <th>Env override</th>
      <th>Description</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td><code>tusk_api.url</code></td>
      <td>string</td>
      <td></td>
      <td>Cloud: yes</td>
      <td><code>TUSK_API_URL</code></td>
      <td>Base URL of Tusk Drift Cloud (e.g., <code>https://api.usetusk.ai</code>). The CLI targets <code>/api/drift/test_run_service</code> under this host.</td>
    </tr>
  </tbody>
</table>

For authentication in cloud mode, either use:

- Auth0: `tusk login`
- API key: `TUSK_API_KEY`

## Test execution

<table>
  <thead>
    <tr>
      <th>Key</th>
      <th>Type</th>
      <th>Default</th>
      <th>Required</th>
      <th>Notes</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td><code>test_execution.concurrency</code></td>
      <td>number</td>
      <td>5</td>
      <td>no</td>
      <td>Max concurrent tests. CLI flag <code>--concurrency</code> overrides. You generally do not need to modify this.</td>
    </tr>
    <tr>
      <td><code>test_execution.timeout</code></td>
      <td>duration</td>
      <td><code>30s</code></td>
      <td>no</td>
      <td>Timeout for each trace test (a test usually completes in <1 second).</td>
    </tr>
  </tbody>
</table>

## Comparison (response diffing)

<table>
  <thead>
    <tr>
      <th>Key</th>
      <th>Type</th>
      <th>Default</th>
      <th>Description</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td><code>comparison.ignore_fields</code></td>
      <td>string[]</td>
      <td><code>[]</code></td>
      <td>Exact field names (by last path segment) to ignore during JSON comparison.</td>
    </tr>
    <tr>
      <td><code>comparison.ignore_patterns</code></td>
      <td>string[]</td>
      <td><code>[]</code></td>
      <td>Regex patterns for values to ignore when both sides match.</td>
    </tr>
    <tr>
      <td><code>comparison.ignore_uuids</code></td>
      <td>boolean</td>
      <td><code>true</code></td>
      <td>Ignore UUID‑like values when both sides are UUIDs.</td>
    </tr>
    <tr>
      <td><code>comparison.ignore_timestamps</code></td>
      <td>boolean</td>
      <td><code>true</code></td>
      <td>Ignore ISO‑8601 timestamps when both sides are timestamps.</td>
    </tr>
    <tr>
      <td><code>comparison.ignore_dates</code></td>
      <td>boolean</td>
      <td><code>true</code></td>
      <td>Ignore date formats (e.g., <code>YYYY-MM-DD</code>) when both sides are dates.</td>
    </tr>
  </tbody>
</table>

## Recording (for SDK)

<table>
  <thead>
    <tr>
      <th>Key</th>
      <th>Type</th>
      <th>Default</th>
      <th>Env override</th>
      <th>Description</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td><code>recording.sampling_rate</code></td>
      <td>number</td>
      <td>0.1</td>
      <td><code>TUSK_RECORDING_SAMPLING_RATE</code></td>
      <td>Target sampling fraction when recording traces.</td>
    </tr>
  </tbody>
</table>

This will not affect CLI behavior. See SDK for more details:

- [Node](https://github.com/Use-Tusk/drift-node-sdk)

## Results

<table>
  <thead>
    <tr>
      <th>Key</th>
      <th>Type</th>
      <th>Default</th>
      <th>Env override</th>
      <th>Description</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td><code>results.dir</code></td>
      <td>string</td>
      <td><code>.tusk/results</code></td>
      <td><code>TUSK_RESULTS_DIR</code></td>
      <td>Directory for saved run outputs when <code>--save-results</code> is used. CLI flag <code>--results-dir</code> takes precedence.</td>
    </tr>
  </tbody>
</table>

## Config overrides

### Flags that override config

- `--concurrency` → overrides `test_execution.concurrency`
- `--enable-service-logs` → enables service log capture (not a config key)
- `--save-results` and `--results-dir` → control result file output (uses `results.dir` if not provided)
- `--cloud` and metadata flags (e.g., `--trace-test-id`, `--all-cloud-trace-tests`, CI context flags)
- `--trace-dir` → overrides `traces.dir`

### Environment variables that override config

- `TUSK_TRACES_DIR` → `traces.dir`
- `TUSK_API_URL` → `tusk_api.url`
- `TUSK_RESULTS_DIR` → `results.dir`
- `TUSK_RECORDING_SAMPLING_RATE` → `recording.sampling_rate`

## Minimal examples

Local:

```yaml
service:
  name: my-service
  port: 3000
  start:
    command: npm run dev
  readiness_check:
    command: curl -sf http://localhost:3000/health
    timeout: 30s
    interval: 2s

traces:
  dir: .tusk/traces

test_execution:
  concurrency: 5

comparison:
  ignore_fields: ["request_id"]
  ignore_uuids: false

results:
  dir: .tusk/results
```

Cloud:

```yaml
service:
  id: 1165f64c-5a5e-4586-a22a-2d7cab42af83
  name: acme-backend
  port: 3000
  start:
    command: npm run dev
  readiness_check:
    command: curl -sf http://localhost:3000/health
    timeout: 30s
    interval: 2s

tusk_api:
  url: https://app.usetusk.ai
```
