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

Call transition_phase with:
{
  "results": {
    "complex_test_passed": true|false,
    "has_external_calls": true|false
  },
  "notes": "Explain what was tested or why it was skipped"
}
