# Slack thread context

sohil  [4:56 PM]
when service fails to start in CLI (but `--enable-service-logs` is false), thoughts on trying to start the service but with service logs enabled? rn i feel like it's hard to debug in CI when things go wrong

Sohan  [4:58 PM]
yea i like that. can enforce adding the following in CI too to upload log file

```yaml
- name: Upload Tusk service logs
   if: always()
   uses: actions/upload-artifact@v4
   with:
     name: tusk-service-logs
     path: .tusk/logs
     if-no-files-found: ignore
```

sohil  [4:59 PM]
nice, yeah could do that our just output to logs and they'll show up in CI anyways
marcel  [4:59 PM]
nice nice
Sohan  [5:00 PM]
sdks should also support env var override for log level so we can run in debug mode (edited) 
sohil  [7:00 PM]
@Jun Yu thoughts on this? feels like this could even be default behavior
(as in, if service doesn't start, then retry starting with service logs enabled)
Jun Yu  [7:09 PM]
why not just explicitly add `--enable-service-logs` to our CI workflow? (edited) 
sohil  [7:11 PM]
i'm thinking for customers, that's just an extra step they may not realize. so when they're iterating on their CI workflow, if smth fails, immediately they'll get logs explaining why
Jun Yu  [7:15 PM]
not a huge fan of your suggested pattern :thinking_face: bc of the obscure magic

i'm thinking smth like this: `--service-logs [enable | enable-with-retry]` or something like that

- `enable` = enable every time
- `enable-with-retry` = retry with service logs
  - honestly still could be better (we're coupling retry logic with service logs)

sohil  [7:18 PM]
hmm but that doesn't solve problem for customers who are iterating on workflow
i feel like it could get frustrating to iterate when things are failing, and then having to wait for iteration
another solution could just be to enable service logs by default in CI
Jun Yu  [7:19 PM]
hmm service logs by default in CI feels cleaner, can just add to the help text
sohil  [7:20 PM]
or our github action adds it by default, and we can update docs/prompts for setup agent
rn we don't output service logs anywhere though right, it just persists to file system? we could also add logic in CLI where if it's run in `--print` mode, and service fails to start, it outputs service logs
Jun Yu  [7:22 PM]
do you think that could be verbose? with run output and service logs being interleaved
sohil  [7:22 PM]
it would only show if service failed to start, so there's no run output really?
Jun Yu  [7:23 PM]
oh yea makes sense
sohil  [7:23 PM]
maybe this is only in CI mode, since user doesn't have access to file system

# Final solution

Service logs (stdout/stderr from the user's service) are always captured during startup. This gives users immediate visibility when things go wrong, without needing to know about any flags.

## Behavior

### During service startup (always)
- Service stdout/stderr is always captured during startup
- If `--enable-service-logs` is set: stream directly to `.tusk/logs/` file during startup (file acts as the buffer)
- If `--enable-service-logs` is NOT set: buffer in memory during startup

### On startup failure
- **TUI**: dump captured service logs into the right side panel
- **Print mode**: dump last N lines to **stderr** (not stdout, to avoid breaking `--output-format json` consumers)
- Update the startup failure help message (`executor.go:503-521`) — currently it suggests using `--enable-service-logs`, which is no longer relevant. New suggestions:
  - "Check the service logs above for details"
  - "For more verbose SDK logging, set the log level to debug in your SDK initialization"
  - Note: `--debug` on the CLI only controls fence sandbox debug mode, not useful for customers debugging service startup

### Sandbox retry and service logs
- Default sandbox mode will change to `strict` (in a subsequent commit)
- In `auto` mode (`environment.go:24-37`): if sandboxed startup fails, it retries without sandbox. Both attempts' logs should be kept — append a separator (e.g. `"⚠️ Retrying without sandbox..."`) and continue capturing. If the retry also fails, the user sees logs from both attempts. If the retry succeeds and `--enable-service-logs` isn't set, discard everything
- For the in-memory buffer: just keep appending across retries
- For the file buffer (`--enable-service-logs`): reuse the same file handle across retries rather than creating a new file

### On startup success
- If `--enable-service-logs` is set: continue streaming to `.tusk/logs/` file for the entire run (same as today's behavior)
- If `--enable-service-logs` is NOT set: swap `e.serviceCmd.Stdout/Stderr` to `io.Discard` and nil out the buffer. This stops memory growth for the rest of the run while the process keeps writing harmlessly to nowhere

### In-memory buffer implementation
- Use a `bytes.Buffer` as the `io.Writer` for `e.serviceCmd.Stdout/Stderr` — it satisfies the same interface as `*os.File`
- Add `startupLogBuffer *bytes.Buffer` field to `Executor`
- In `service.go:150-158`, the wiring becomes:
  - `--enable-service-logs` on → stream to file (existing path)
  - `--enable-service-logs` off → `e.startupLogBuffer = &bytes.Buffer{}`, assign to stdout/stderr
- On failure: read `startupLogBuffer.String()` (or read back the file if using file path) to dump to TUI/stderr
- On success without flag: swap to `io.Discard`, nil the buffer

### Flag changes
- `--enable-service-logs` keeps its current name, no deprecation needed
- No `--disable-service-logs` flag needed
- `--debug` stays separate — controls fence sandbox debug mode

### Early exit on process failure
- Currently if the service process exits immediately (e.g. bad config, missing dependency), `waitForReadiness` keeps polling until the full timeout — wasted time
- Improvement: after `e.serviceCmd.Start()`, run a goroutine that waits on `e.serviceCmd.Wait()`. If the process exits with a non-zero code before readiness is reached, bail immediately and dump service logs
- Pairs well with the service logs change — user gets immediate feedback (fast failure + logs) instead of waiting 10+ seconds for timeout

## Other improvements

### Print mode timestamp logging
Add duration logging to print mode phases that currently have none:
- Test loading (`"➤ Found/Loaded %d tests"`)
- Pre-app-start span fetching (`"✓ Loaded %d pre-app-start spans"`)
- Suite span preparation (`"Loaded %d suite spans"`)
- Environment start (`"➤ Starting environment..."` → `"✓ Environment ready"`)
- Overall test execution and summary

Per-test results already log duration (e.g. `"NO DEVIATION - traceId (150ms)"`).

### SDK debug log level via CLI
When `--debug` is used with the CLI, SDKs should automatically run with log level "debug" (may need a separate env var or command to propagate this)

### Setup agent
- Update `internal/agent/prompts/phase_simple_test.md` — remove the "Logs only appear if `debug: true` is set" note since logs are always captured during startup
- Setup agent can always check service logs when iterating on start commands
- Setup agent should know that if sandbox fails to start, it can set `sandbox_mode: "off"` to test if the service works without sandbox (currently referenced in prompts but should be more explicit given the move to strict default)

### Disk accumulation
- With the new design, `.tusk/logs/` only accumulates when `--enable-service-logs` is explicitly set, so this is less of a concern
- Could still add a retention policy (e.g. older than 7 days) as a nice-to-have

## Out of scope (future)
- Deviation analysis with interlaced test markers in service logs
- Concurrent run log file collision (timestamp has second-level granularity, low risk)

## Considerations

Does enabling service logs cause any meaningful performance degradation?

Enabling service logs could potentially help us with deviation analysis in the future, how could we use this?
Potentially in the service logs file, we interlace Tusk Drift logs that say "X trace test started" and "X trace test completed" within the service logs? That way, we can programtically extract the service logs for a specific trace test and use it in deviation analysis?
