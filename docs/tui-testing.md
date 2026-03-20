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
  command: 'node -e "console.log(''boot log line''); console.error(''some error''); process.exit(1)"'
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
    do script "tmux new-session -s tui-test -x 160 -y 45"
end tell'
sleep 3

# 2. Hide tmux status bar (otherwise a green bar appears at the bottom)
tmux set -t tui-test status off

# 3. Launch the TUI
tmux send-keys -t tui-test 'cd /path/to/test-project && /path/to/tusk drift run' Enter

# 4. Wait for the state you want to capture
#    - Normal run with tests: ~15-20s (environment start + test execution)
#    - Startup failure with sandbox retry: ~10-15s
#    - Just initial render: ~3-5s
sleep 15

# 5. Navigate if needed
tmux send-keys -t tui-test g        # go to top (select Service Logs)
tmux send-keys -t tui-test j        # move selection down
tmux send-keys -t tui-test J        # scroll log panel down
tmux send-keys -t tui-test D        # half-page down in log panel
sleep 1

# 6. Find the Terminal.app window ID
WINDOW_ID=$(python3 -c "
import Quartz
windows = Quartz.CGWindowListCopyWindowInfo(Quartz.kCGWindowListOptionOnScreenOnly, Quartz.kCGNullWindowID)
for w in windows:
    if w.get('kCGWindowOwnerName') == 'Terminal' and w.get('kCGWindowLayer', 0) == 0:
        print(w['kCGWindowNumber'])
        break
")

# 7. Capture the window
screencapture -l "$WINDOW_ID" -o screenshot.png

# 8. Cleanup
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

### Option B: Text capture (quick functional checks)

Uses a detached tmux session — no visible window, no permissions needed. Good for verifying that specific text appears in the TUI or that navigation works. **Not a substitute for screenshots** when verifying layout, colors, or visual rendering.

```bash
# 1. Detached tmux session (no visible window)
tmux new-session -d -s tui-test -x 160 -y 45

# 2. Launch the TUI
tmux send-keys -t tui-test 'cd /path/to/test-project && /path/to/tusk drift run' Enter
sleep 15

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

| Setting      | Value | Notes                           |
| ------------ | ----- | ------------------------------- |
| tmux columns | 160   | Wide enough for both TUI panels |
| tmux rows    | 45    | Tall enough to see tests + logs |

## Common Gotchas

1. **Timing is critical.** The TUI renders asynchronously. Capturing too early gives an incomplete screen. When in doubt, wait longer.
2. **tmux status bar.** The green bar at the bottom of native screenshots is tmux's status line. Hide it with `tmux set -t tui-test status off` before capturing.
3. **Scrolling.** Content often extends below the visible area of the log panel. Send `J` or `D` keys to scroll down before capturing.
4. **Screen Recording permission.** Native `screencapture -l` fails silently without it. Grant it to Terminal.app in System Settings > Privacy & Security > Screen Recording.
5. **YAML quoting.** When editing config.yaml with commands containing colons or quotes, wrap the entire value in double quotes and escape inner quotes.
6. **Restore config.** Always restore `.tusk/config.yaml` after testing with modified start commands.
7. **tmux targets by session name** (`-t tui-test`), so commands work regardless of which terminal you're focused on. You can keep working while tests run.

## Fallback: Rendering ANSI captures to images

If you have an ANSI text capture and need an image (e.g., for a bug report when screencapture isn't available):

```bash
# Capture with ANSI codes
tmux capture-pane -t tui-test -p -e > /tmp/tui-capture.txt
```

### Pillow

```bash
python3 render_ansi_png.py  # see script below
```

<details>
<summary>render_ansi_png.py</summary>

```python
#!/usr/bin/env python3
"""Render tmux ANSI capture to a high-resolution PNG using Pillow."""

import re
from PIL import Image, ImageDraw, ImageFont

INPUT_FILE = "/tmp/tui-capture.txt"
OUTPUT_FILE = "tui-screenshot.png"
FONT_SIZE = 22
BG_COLOR = (30, 30, 30)
FONT_PATH = "/System/Library/Fonts/Menlo.ttc"  # macOS; adjust for Linux


def load_font(size):
    try:
        return ImageFont.truetype(FONT_PATH, size, index=0)
    except Exception:
        try:
            return ImageFont.truetype("/System/Library/Fonts/Monaco.ttf", size)
        except Exception:
            return ImageFont.load_default()


def build_256_palette():
    palette = {}
    base16 = [
        (0, 0, 0), (205, 49, 49), (13, 188, 121), (229, 229, 16),
        (36, 114, 200), (188, 63, 188), (17, 168, 205), (204, 204, 204),
        (128, 128, 128), (241, 76, 76), (35, 209, 139), (245, 245, 67),
        (59, 142, 234), (214, 112, 214), (41, 184, 219), (255, 255, 255),
    ]
    for i, c in enumerate(base16):
        palette[i] = c
    for i in range(216):
        r, g, b = i // 36, (i % 36) // 6, i % 6
        palette[16 + i] = (
            0 if r == 0 else 55 + r * 40,
            0 if g == 0 else 55 + g * 40,
            0 if b == 0 else 55 + b * 40,
        )
    for i in range(24):
        v = 8 + i * 10
        palette[232 + i] = (v, v, v)
    return palette


PALETTE_256 = build_256_palette()
ANSI_RE = re.compile(r'\x1b\[([0-9;]*)m')


def parse_ansi_line(line):
    segments = []
    fg, bg, bold = (204, 204, 204), None, False
    pos = 0

    for match in ANSI_RE.finditer(line):
        start, end = match.span()
        for ch in line[pos:start]:
            segments.append((ch, fg, bg, bold))
        pos = end

        params = [int(p) if p else 0 for p in (match.group(1) or "0").split(';')]
        i = 0
        while i < len(params):
            p = params[i]
            if p == 0:
                fg, bg, bold = (204, 204, 204), None, False
            elif p == 1:
                bold = True
            elif p == 22:
                bold = False
            elif 30 <= p <= 37:
                fg = PALETTE_256[p - 30]
            elif p == 38:
                if i + 1 < len(params) and params[i + 1] == 5 and i + 2 < len(params):
                    fg = PALETTE_256.get(params[i + 2], fg)
                    i += 2
                elif i + 1 < len(params) and params[i + 1] == 2 and i + 4 < len(params):
                    fg = (params[i + 2], params[i + 3], params[i + 4])
                    i += 4
            elif p == 39:
                fg = (204, 204, 204)
            elif 40 <= p <= 47:
                bg = PALETTE_256[p - 40]
            elif p == 48:
                if i + 1 < len(params) and params[i + 1] == 5 and i + 2 < len(params):
                    bg = PALETTE_256.get(params[i + 2], bg)
                    i += 2
                elif i + 1 < len(params) and params[i + 1] == 2 and i + 4 < len(params):
                    bg = (params[i + 2], params[i + 3], params[i + 4])
                    i += 4
            elif p == 49:
                bg = None
            elif 90 <= p <= 97:
                fg = PALETTE_256[p - 90 + 8]
            elif 100 <= p <= 107:
                bg = PALETTE_256[p - 100 + 8]
            i += 1

    for ch in line[pos:]:
        segments.append((ch, fg, bg, bold))
    return segments


def main():
    with open(INPUT_FILE, 'r') as f:
        lines = [line.rstrip('\n') for line in f.readlines()]

    font = load_font(FONT_SIZE)
    bbox = font.getbbox("M")
    char_w = bbox[2] - bbox[0]
    char_h = int(FONT_SIZE * 1.45)

    padding = 16
    max_cols = 160
    img_w = padding * 2 + char_w * max_cols
    img_h = padding * 2 + char_h * len(lines)

    img = Image.new('RGB', (img_w, img_h), BG_COLOR)
    draw = ImageDraw.Draw(img)

    for row, line in enumerate(lines):
        segments = parse_ansi_line(line)
        y = padding + row * char_h
        for col, (ch, fg, bg, is_bold) in enumerate(segments):
            x = padding + col * char_w
            if bg:
                draw.rectangle([x, y, x + char_w, y + char_h], fill=bg)
            if ch and ch != ' ':
                draw.text((x, y), ch, fill=fg, font=font)

    img.save(OUTPUT_FILE)
    print(f"Saved: {OUTPUT_FILE} ({img_w}x{img_h})")


if __name__ == "__main__":
    main()
```

</details>

### ansi2html + Chrome headless

If `ansi2html` is installed (`pip3 install ansi2html`):

```bash
cat /tmp/tui-capture.txt | ansi2html > /tmp/tui.html
"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
    --headless --screenshot=/tmp/screenshot.png \
    --window-size=1600,900 --force-device-scale-factor=2 \
    /tmp/tui.html
```
