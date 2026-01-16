#!/bin/bash
set -e

# Claude Code Web Environment Setup for tusk-drift-cli
# This script runs on SessionStart in remote environments

# Only run in remote (web) environments
if [ "$CLAUDE_CODE_REMOTE" != "true" ]; then
  echo "Skipping remote-only setup (running locally)"
  exit 0
fi

echo "=== Setting up tusk-drift-cli development environment ==="
echo "Started at: $(date)"

echo ""
echo "[1/5] Building tusk CLI..."
cd "$CLAUDE_PROJECT_DIR"
make deps
make build
echo "✓ tusk CLI built successfully"

echo ""
echo "[2/5] Installing sandboxing dependencies (bubblewrap, socat)..."
if command -v apt-get &> /dev/null; then
  sudo apt-get update -qq
  sudo apt-get install -y -qq bubblewrap socat
  echo "✓ Sandboxing dependencies installed"
else
  echo "⚠ apt-get not available, skipping sandboxing dependencies"
fi

echo ""
echo "[3/5] Cloning tusk backend..."
mkdir -p ~/repos
if [ -d ~/repos/tusk ]; then
  echo "  tusk repo already exists, pulling latest..."
  cd ~/repos/tusk && git pull --depth 1 || true
else
  git clone --depth 1 https://github.com/Use-Tusk/tusk.git ~/repos/tusk
fi
echo "✓ tusk backend available at ~/repos/tusk"

echo ""
echo "[4/5] Setting up demo repos..."

# Node.js demo
if [ -d ~/repos/drift-node-demo ]; then
  echo "  drift-node-demo already exists"
else
  git clone --depth 1 https://github.com/Use-Tusk/drift-node-demo.git ~/repos/drift-node-demo
fi
cd ~/repos/drift-node-demo && npm install --silent 2>/dev/null || npm install
echo "✓ drift-node-demo ready at ~/repos/drift-node-demo"

# Python demo
if [ -d ~/repos/drift-python-demo ]; then
  echo "  drift-python-demo already exists"
else
  git clone --depth 1 https://github.com/Use-Tusk/drift-python-demo.git ~/repos/drift-python-demo
fi
cd ~/repos/drift-python-demo
if [ ! -d venv ]; then
  python3 -m venv venv
fi
source venv/bin/activate
pip install -q -r requirements.txt
deactivate
echo "✓ drift-python-demo ready at ~/repos/drift-python-demo"

echo ""
echo "[5/5] Configuring environment..."
if [ -n "$CLAUDE_ENV_FILE" ]; then
  cat >> "$CLAUDE_ENV_FILE" << 'ENVEOF'
export PATH="$PATH:$CLAUDE_PROJECT_DIR"
export GOPRIVATE="github.com/Use-Tusk/*"
# Alias for convenience
alias tusk="$CLAUDE_PROJECT_DIR/tusk"
ENVEOF
  echo "✓ Environment configured"
else
  echo "⚠ CLAUDE_ENV_FILE not set, environment not persisted"
fi

echo ""
echo "=== Setup complete! ==="
echo "Finished at: $(date)"
echo ""
echo "Available tools:"
echo "  - tusk CLI:         $CLAUDE_PROJECT_DIR/tusk"
echo "  - tusk backend:     ~/repos/tusk"
echo "  - Node.js demo:     ~/repos/drift-node-demo"
echo "  - Python demo:      ~/repos/drift-python-demo"
echo ""
echo "Quick test commands:"
echo "  ./tusk --version"
echo "  ./tusk list --cloud"
echo "  cd ~/repos/drift-node-demo && $CLAUDE_PROJECT_DIR/tusk run"

exit 0
