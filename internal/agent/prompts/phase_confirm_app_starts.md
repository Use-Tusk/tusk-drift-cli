## Phase: Confirm App Starts

Before instrumenting the SDK, verify the service starts correctly as-is.

1. Start the service using the start command you discovered (use start_background_process)
2. Wait for the service to be ready using wait_for_ready with the health endpoint
3. Make a request to the health endpoint to verify it's working
4. Stop the service using stop_background_process

If the service doesn't start:

- Get the logs with get_process_logs
- Look for errors and try to understand what's wrong
- Ask the user if you can't figure it out
- This must work before we proceed

When confirmed working, call transition_phase with:
{
  "results": {
    "app_starts_without_sdk": true
  }
}
