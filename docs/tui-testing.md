# CLI & TUI Testing Guide

How to manually test the tusk CLI in both print mode and interactive TUI mode.

## Print Mode Testing

Print mode (`--print`) runs headlessly — no interactive UI. Run it directly and inspect stderr:

```bash
cd /path/to/test-project
/path/to/tusk drift run --print 2>&1
```

Filter for specific output:

```bash
/path/to/tusk drift run --print 2>&1 | grep -E "(➤|✓|Tests:|Error:)"
```

### Testing failure scenarios

To test startup failures, temporarily change the start command in `.tusk/config.yaml`:

```yaml
start:
    command: node -e "console.log('boot log line'); console.error('some error'); process.exit(1)"
```

To test with a service that starts but behaves differently, adjust the command or example codebase as needed.

**Always restore the config after testing.**

## TUI Testing with tmux

The TUI (interactive mode, no `--print`) requires a terminal. We use tmux for programmatic control — it lets us send keystrokes and capture output without needing to be in the terminal ourselves.

### Option A: Native screenshots (recommended)

Opens a real Terminal.app window with tmux inside it, then uses macOS `screencapture -l` to capture that specific window by ID. This produces pixel-perfect Retina screenshots and should always be used to verify TUI visual changes.

**One-time setup:** Grant Screen Recording permission to Terminal.app in System Settings > Privacy & Security > Screen Recording.

```bash
# 1. Open Terminal.app with a tmux session
osascript -e 'tell application "Terminal"
    do script "tmux new-session -s tui-test -x 200 -y 55"
end tell'
sleep 3

# 2. Resize the window to fill most of the screen (fits 14"/16" MacBook Pro)
osascript -e 'tell application "Terminal" to set bounds of front window to {0, 25, 1700, 1100}'
sleep 1

# 3. Hide tmux status bar (otherwise a green bar appears at the bottom)
tmux set -t tui-test status off

# 4. Launch the TUI
tmux send-keys -t tui-test 'cd /path/to/test-project && /path/to/tusk drift run' Enter

# 5. Wait for the state you want to capture
#    - Normal run with tests: ~25-30s (environment start + test execution)
#    - Startup failure with sandbox retry: ~15-18s
#    - Just initial render: ~3-5s
sleep 25

# 6. Navigate if needed
tmux send-keys -t tui-test g        # go to top (select Service Logs)
tmux send-keys -t tui-test j        # move selection down
tmux send-keys -t tui-test J        # scroll log panel down
tmux send-keys -t tui-test D        # half-page down in log panel
sleep 1

# 7. Find the Terminal.app window ID
WINDOW_ID=$(python3 -c "
import Quartz
windows = Quartz.CGWindowListCopyWindowInfo(Quartz.kCGWindowListOptionOnScreenOnly, Quartz.kCGNullWindowID)
for w in windows:
    if w.get('kCGWindowOwnerName') == 'Terminal' and w.get('kCGWindowLayer', 0) == 0:
        print(w['kCGWindowNumber'])
        break
")

# 8. Capture the window
screencapture -l "$WINDOW_ID" -o screenshot.png

# 9. Cleanup
tmux send-keys -t tui-test q
sleep 2
tmux kill-session -t tui-test
osascript -e 'tell application "Terminal" to close front window' 2>/dev/null
```

**Output:** ~2800x1800 Retina PNG with native font rendering.

**Notes:**
- `screencapture -l` captures by window ID — the Terminal window doesn't need to be in the foreground. You can keep working in other windows.
- The `-o` flag removes the window shadow.
- `screencapture -l` fails silently without Screen Recording permission — you get a blank or tiny image.
- When finding the window ID, make sure to match `kCGWindowOwnerName == 'Terminal'` — other apps (Chrome, etc.) may be in front.

### Option B: Text capture (quick functional checks)

Uses a detached tmux session — no visible window, no permissions needed. Good for verifying that specific text appears in the TUI or that navigation works. **Not a substitute for screenshots** when verifying layout, colors, or visual rendering.

```bash
# 1. Detached tmux session (no visible window)
tmux new-session -d -s tui-test -x 200 -y 55

# 2. Launch the TUI
tmux send-keys -t tui-test 'cd /path/to/test-project && /path/to/tusk drift run' Enter
sleep 25

# 3. Capture the screen as plain text
SCREEN=$(tmux capture-pane -t tui-test -p)

# 4. Assert on content
echo "$SCREEN" | grep -q "TEST EXECUTION" || echo "FAIL: header not found"
echo "$SCREEN" | grep -q "Environment ready" || echo "FAIL: environment didn't start"

# 5. Navigate and capture again
tmux send-keys -t tui-test j
sleep 0.5
SCREEN=$(tmux capture-pane -t tui-test -p)

# 6. Cleanup
tmux send-keys -t tui-test q
sleep 1
tmux kill-session -t tui-test
```

### TUI keyboard shortcuts reference

| Key       | Action                                  |
| --------- | --------------------------------------- |
| `j` / `k` | Select next/previous test in left panel |
| `g` / `G` | Jump to top/bottom of test list         |
| `u` / `d` | Half-page up/down in test list          |
| `J` / `K` | Scroll log panel down/up                |
| `U` / `D` | Half-page up/down in log panel          |
| `y`       | Copy all logs                           |
| `q`       | Quit                                    |

## Recommended Dimensions

| Setting      | Value | Notes                                      |
| ------------ | ----- | ------------------------------------------ |
| tmux columns | 200   | Wide enough for both TUI panels + detail   |
| tmux rows    | 55    | Tall enough to see tests + logs            |
| Window bounds | {0, 25, 1700, 1100} | Fits 14"/16" MacBook Pro (adjust for your display) |

## Common Gotchas

1. **Timing is critical.** The TUI renders asynchronously. Capturing too early gives an incomplete screen. When in doubt, wait longer.
2. **tmux status bar.** The green bar at the bottom of native screenshots is tmux's status line. Hide it with `tmux set -t tui-test status off` before capturing.
3. **Scrolling.** Content often extends below the visible area of the log panel. Send `J` or `D` keys to scroll down before capturing.
4. **Screen Recording permission.** Native `screencapture -l` fails silently without it. Grant it to Terminal.app in System Settings > Privacy & Security > Screen Recording.
5. **YAML quoting.** When editing config.yaml with commands containing colons or quotes, wrap the entire value in double quotes and escape inner quotes.
6. **Restore config.** Always restore `.tusk/config.yaml` after testing with modified start commands.
7. **tmux targets by session name** (`-t tui-test`), so commands work regardless of which terminal you're focused on. You can keep working while tests run.
8. **Window ID targeting.** When using `screencapture -l`, make sure the Python script finds the Terminal window, not Chrome or another app that may be in front.
