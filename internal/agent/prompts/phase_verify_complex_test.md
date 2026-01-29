## Phase: Verify Complex Test (Optional)

Test an endpoint with external calls if one was previously cached or can be found.

### Step 1: Determine Endpoint

Read `.tusk/verify-cache.json`. If a `complex_test` entry exists, use that endpoint's
URL, method, headers, and body.

If no cache entry exists, look through the codebase (using grep) for an endpoint that
makes external HTTP requests, database queries, or Redis calls. Look for route handlers
that contain fetch, axios, http.get, database queries, etc.

If you cannot find a suitable endpoint and there is no cache, skip this phase by calling
transition_phase immediately with `verify_complex_passed: false` and a note explaining
no suitable endpoint was found.

CRITICAL:
The endpoint must return a 200 response with content type "application/json" or "text/plain".
The Tusk Drift SDK excludes "text/html" traces.

### Step 2: Record and Replay

Follow the same process as the simple test:
1. Start service with env: {"TUSK_DRIFT_MODE": "RECORD"}
2. Wait for the service to be ready
3. Make the HTTP request to the endpoint
4. Wait 3 seconds for trace to be written
5. Stop the service
6. Run tusk_list to verify trace was recorded
7. Run tusk_run to replay (try with `debug: true` on failure)

### Step 3: Save to Cache

If the test passed, update `.tusk/verify-cache.json` with the `complex_test` entry.
Read the existing file first to preserve the `simple_test` entry.

### Transition

This phase is optional - if it fails but simple test passed, that is acceptable.

Call transition_phase with:
```json
{
  "results": {
    "verify_complex_passed": true|false,
    "complex_test_passed": true|false,
    "has_external_calls": true|false
  },
  "notes": "Explain what was tested or why it was skipped"
}
```
