#!/usr/bin/env bash
set -euo pipefail

# Example CI bootstrap for Tusk Drift on Linux/macOS when you cannot use
# Use-Tusk/drift-action.
# Copy this file into your repo (for example `.ci/tusk-drift-ci.sh`) and either:
#
#   1. execute it with the Tusk command you want to run:
#      bash ./.ci/tusk-drift-ci.sh tusk run -c -p --ci --validate-suite-if-default-branch
#
#   2. or run it first, then keep PATH for later commands in the same CI step:
#      bash ./.ci/tusk-drift-ci.sh
#      export PATH="$HOME/.local/bin:/usr/local/bin:$PATH"
#      tusk run -c -p --ci --validate-suite-if-default-branch
#
# Linux-only sandbox setup in this script:
# - installs bubblewrap, socat, and uidmap when missing
# - ensures /etc/subuid and /etc/subgid contain an entry for the CI user
# - ensures bwrap has the setuid bit that many CI runners require
#
# This example auto-installs Linux packages only on Debian/Ubuntu runners that
# expose apt-get. Other distros can still use this script, but you must install
# the equivalent packages yourself before running it.

APT_UPDATED=0

log() {
  printf '[tusk-drift] %s\n' "$*"
}

have() {
  command -v "$1" >/dev/null 2>&1
}

require_sudo() {
  if have sudo; then
    return 0
  fi

  log "sudo is required to install or repair Linux sandbox prerequisites."
  return 1
}

apt_install() {
  require_sudo

  if ! have apt-get; then
    log "apt-get is unavailable, so install these packages manually: $*"
    return 1
  fi

  if [ "$APT_UPDATED" -eq 0 ]; then
    sudo apt-get update
    APT_UPDATED=1
  fi

  sudo apt-get install -y "$@"
}

ensure_curl() {
  if have curl; then
    return 0
  fi

  log "curl not found; installing it"
  apt_install curl
}

install_tusk_cli() {
  local install_script_url="${TUSK_INSTALL_SCRIPT_URL:-https://cli.usetusk.ai/install.sh}"

  export PATH="$HOME/.local/bin:/usr/local/bin:$PATH"

  if have tusk; then
    log "Tusk CLI already installed: $(tusk --version)"
    return 0
  fi

  ensure_curl

  log "Installing Tusk Drift CLI"
  if [ -n "${TUSK_VERSION:-}" ]; then
    curl -fsSL "$install_script_url" | sh -s -- "$TUSK_VERSION"
  else
    curl -fsSL "$install_script_url" | sh
  fi

  export PATH="$HOME/.local/bin:/usr/local/bin:$PATH"
  log "Tusk CLI ready: $(tusk --version)"
}

run_linux_sandbox_preflight() {
  bwrap \
    --ro-bind / / \
    --unshare-user \
    --uid 0 \
    --gid 0 \
    -- \
    /bin/true >/dev/null 2>&1
}

install_linux_sandbox_packages() {
  local packages=()

  if ! have bwrap; then
    packages+=("bubblewrap")
  fi

  if ! have socat; then
    packages+=("socat")
  fi

  if ! have newuidmap || ! have newgidmap; then
    packages+=("uidmap")
  fi

  if [ "${#packages[@]}" -eq 0 ]; then
    return 0
  fi

  log "Installing Linux sandbox packages: ${packages[*]}"
  apt_install "${packages[@]}"
}

ensure_subid_entry() {
  local file_path="$1"
  local user_name
  local block_size=65536
  local min_start=100000
  local start

  user_name="$(id -un)"

  require_sudo
  sudo touch "$file_path"
  if sudo grep -q "^${user_name}:" "$file_path"; then
    return 0
  fi

  start="$(
    sudo awk -F: -v block_size="$block_size" -v min_start="$min_start" '
      BEGIN { max = min_start - 1 }
      NF >= 3 {
        start = $2 + 0
        count = $3 + 0
        end = start + count - 1
        if (end > max) {
          max = end
        }
      }
      END {
        next = max + 1
        if (next < min_start) {
          next = min_start
        }
        rem = next % block_size
        if (rem != 0) {
          next += block_size - rem
        }
        print next
      }
    ' "$file_path"
  )"

  log "Adding ${user_name} entry to ${file_path}"
  printf '%s\n' "${user_name}:${start}:${block_size}" | sudo tee -a "$file_path" >/dev/null
}

ensure_bwrap_setuid() {
  local bwrap_path

  bwrap_path="$(command -v bwrap || true)"
  if [ -z "$bwrap_path" ]; then
    log "bwrap not found after package install"
    return 1
  fi

  if [ -u "$bwrap_path" ]; then
    return 0
  fi

  require_sudo
  log "Enabling setuid on ${bwrap_path}"
  sudo chmod u+s "$bwrap_path"
}

ensure_linux_sandbox() {
  if [ "$(uname -s)" != "Linux" ]; then
    return 0
  fi

  install_linux_sandbox_packages

  if run_linux_sandbox_preflight; then
    log "Linux sandbox preflight passed"
    return 0
  fi

  log "Linux sandbox preflight failed; repairing common CI prerequisites"
  ensure_subid_entry /etc/subuid
  ensure_subid_entry /etc/subgid
  ensure_bwrap_setuid

  if run_linux_sandbox_preflight; then
    log "Linux sandbox preflight passed after repair"
    return 0
  fi

  log "Linux sandbox preflight still failed."
  log "If your CI blocks user namespaces, run tusk with --sandbox-mode auto or --sandbox-mode off."
  return 1
}

main() {
  install_tusk_cli
  ensure_linux_sandbox

  if [ "$#" -gt 0 ]; then
    exec "$@"
  fi

  log "Bootstrap complete."
  log "If this script is executed instead of sourced, keep PATH in later steps with:"
  log "  export PATH=\"\$HOME/.local/bin:/usr/local/bin:\$PATH\""
}

main "$@"
