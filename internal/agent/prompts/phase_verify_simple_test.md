## Phase: Verify Simple Test

Record and replay a simple health check to verify the setup works.

### Step 1: Determine Endpoint

Read `.tusk/setup/verify-cache.json` if it exists. If it contains a `simple_test` entry,
use that endpoint's URL, method, headers, and body.

If no cache exists, use the `health_endpoint` and `port` from the current state
(discovered from config.yaml in the previous phase). Default to a GET request
to `http://localhost:<port><health_endpoint>`.

### Step 2: Record a Trace

1. Start the service in RECORD mode:
   - Use start_background_process with env: {"TUSK_DRIFT_MODE": "RECORD"}
   - Do NOT set any API keys - this is LOCAL mode
2. Wait for the service to be ready using wait_for_ready
3. Check logs for "[TuskDrift]" messages to confirm SDK is active
4. Make the HTTP request to the endpoint
5. Wait 3 seconds for the trace to be written
6. Stop the service

### Step 3: Verify Recording

Run tusk_list to see if the trace was recorded.
If no traces appear, this is a verification failure.

### Step 4: Replay the Trace

Run tusk_run to replay the trace.
If it fails:

- Run with `debug: true` once more
- If still failing, mark as failed

### Step 5: Save to Cache

If the test passed, update `.tusk/setup/verify-cache.json` with the endpoint info used.
Read the existing file first (if it exists) to preserve other entries (like `complex_test`).

Format:

```json
{
  "simple_test": {
    "url": "<the full URL used>",
    "method": "<GET/POST/etc>",
    "headers": {},
    "body": ""
  }
}
```

Write the updated JSON back to `.tusk/setup/verify-cache.json`.

### Transition

Call transition_phase with:

```json
{
  "results": {
    "verify_simple_passed": true|false,
    "simple_test_passed": true|false
  },
  "notes": "Explain result"
}
```
