## Phase: Complex Test (Optional)

If the service has endpoints that make external calls (HTTP, database, etc.), test one of those.

### Step 1: Find a Suitable Endpoint

Look for endpoints that:

- Make HTTP requests to external services
- Query a database
- Call Redis or other services

CRITICAL:
The endpoint should return a 200 response with content type "application/json" or "text/plain". This is necessary for Tusk Drift to record the trace (the Tusk Drift SDK excludes "text/html" traces).

NOTE:
If you can't find one or it requires authentication you don't have, skip this phase.

### Step 2: Record and Replay

Follow the same process as Phase 5:

1. Start in RECORD mode
2. Make a request to the endpoint
3. Stop and verify the trace
4. Replay and verify it passes

This phase is optional - if you can't test a complex endpoint, that's okay.

### Step 3: Save to Verify Cache

If the test passed, save the endpoint info used to `.tusk/verify-cache.json` so that
future `tusk setup --verify` runs can reuse it. Read the existing file first (if it
exists) to preserve other entries (like `simple_test`).

If `.tusk/verify-cache.json` does not exist, create it.

Format:

```json
{
  "complex_test": {
    "url": "<the full URL used>",
    "method": "<GET/POST/etc>",
    "headers": {},
    "body": ""
  }
}
```

Write the updated JSON back to `.tusk/verify-cache.json`.

Call transition_phase with:

```json
{
  "results": {
    "complex_test_passed": true|false,
    "has_external_calls": true|false
  },
  "notes": "Explain what was tested or why it was skipped"
}
```
