## Phase: Simple Test

Test the setup with a simple health check endpoint.

### Step 1: Ensure Health Endpoint Exists

If there's no health endpoint, ask the user if you should create a simple one, or which existing endpoint to use.

CRITICAL:
The health endpoint must return 200 response with content type "application/json" or "text/plain". This is necessary for Tusk Drift to record the trace (the Tusk Drift SDK excludes "text/html" traces).
If there is an existing health endpoint that does not meet this criteria, create (and register) a new one with a comment above it saying: "Health endpoint with JSON response for Tusk Drift setup. Remove this endpoint if not needed (and update ./tusk/config.yaml accordingly)."
Do not modify existing endpoints as it may break the service.

### Step 2: Record a Trace

1. Start the service in RECORD mode:
   - Set ONLY the environment variable TUSK_DRIFT_MODE=RECORD
   - Do NOT set any API keys - this is LOCAL mode, no API key needed
   - Use start_background_process with env: {"TUSK_DRIFT_MODE": "RECORD"}
2. Wait for the service to be ready
3. Check the logs for "[TuskDrift]" messages to confirm SDK is active
4. Make a request to the health endpoint
5. Wait a few seconds for trace to be written
6. Stop the service

### Step 3: Verify Recording

Run tusk_list to see if the trace was recorded.
If no traces, check:

- Is TUSK_DRIFT_MODE=RECORD set?
- Did you call TuskDrift.markAppAsReady()?
- Set logLevel: "debug" in the SDK initialization

```typescript/javascript
TuskDrift.initialize({
 ...
 logLevel: "debug",
  });
```

### Step 4: Replay the Trace

Run tusk_run to replay the trace.
If it fails:

- Run with debug: true
- Check for errors in the output or in the logs (in .tusk/logs/)
- Try to fix issues and retry (max 3 attempts)
- If still failing, ask the user for help

When a simple test passes, call transition_phase with:
{
  "results": {
    "simple_test_passed": true
  }
}

If you cannot get it working after reasonable attempts, call transition_phase with simple_test_passed: false and explain what went wrong in the notes.
