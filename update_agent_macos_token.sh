#!/bin/sh
set -eu

if [ "$#" -ne 0 ]; then
  echo "usage: CIWI_GITHUB_TOKEN=<token> sh update_agent_macos_token.sh" >&2
  exit 2
fi

if [ "$(uname -s)" != "Darwin" ]; then
  echo "this token updater is for macOS only" >&2
  exit 1
fi

if [ "$(id -u)" -eq 0 ]; then
  echo "run as your normal user (not root); this updates a LaunchAgent" >&2
  exit 1
fi

if [ -z "${CIWI_GITHUB_TOKEN:-}" ]; then
  echo "CIWI_GITHUB_TOKEN is required" >&2
  exit 1
fi

trim_single_line() {
  printf '%s' "$1" | tr -d '\r\n'
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

LABEL="nl.izmar.ciwi.agent"
PLIST_PATH="$HOME/Library/LaunchAgents/${LABEL}.plist"
UID_NUM="$(id -u)"
TOKEN="$(trim_single_line "${CIWI_GITHUB_TOKEN}")"

require_cmd launchctl
require_cmd /usr/libexec/PlistBuddy

if [ ! -f "$PLIST_PATH" ]; then
  echo "agent plist not found: $PLIST_PATH" >&2
  echo "install the macOS agent first" >&2
  exit 1
fi

echo "[1/3] Updating LaunchAgent token..."
/usr/libexec/PlistBuddy -c "Delete :EnvironmentVariables:CIWI_GITHUB_TOKEN" "$PLIST_PATH" >/dev/null 2>&1 || true
/usr/libexec/PlistBuddy -c "Add :EnvironmentVariables:CIWI_GITHUB_TOKEN string $TOKEN" "$PLIST_PATH"

echo "[2/3] Reloading LaunchAgent..."
launchctl bootout "gui/${UID_NUM}" "$PLIST_PATH" >/dev/null 2>&1 || true
launchctl bootstrap "gui/${UID_NUM}" "$PLIST_PATH"

echo "[3/3] Restarting agent..."
launchctl kickstart -k "gui/${UID_NUM}/${LABEL}" >/dev/null 2>&1 || true

echo
echo "ciwi macOS agent GitHub token updated."
echo "LaunchAgent plist: $PLIST_PATH"
