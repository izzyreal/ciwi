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
  echo "run as your normal user (not root); this updates the user agent" >&2
  exit 1
fi

if [ -z "${CIWI_GITHUB_TOKEN:-}" ]; then
  echo "CIWI_GITHUB_TOKEN is required" >&2
  exit 1
fi

trim_single_line() {
  printf '%s' "$1" | tr -d '\r\n'
}

env_quote() {
  printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"
}

LABEL="nl.izmar.ciwi.agent"
APP_SUPPORT_DIR="$HOME/Library/Application Support/ciwi"
AGENT_ENV_FILE="$APP_SUPPORT_DIR/agent.env"
SERVICE_HELPER_PATH="$APP_SUPPORT_DIR/CiwiAgent.app/Contents/MacOS/ciwi-service"
TOKEN="$(trim_single_line "${CIWI_GITHUB_TOKEN}")"

if [ ! -f "$AGENT_ENV_FILE" ]; then
  echo "agent config not found: $AGENT_ENV_FILE" >&2
  echo "install the macOS agent first" >&2
  exit 1
fi
if [ ! -x "$SERVICE_HELPER_PATH" ]; then
  echo "service helper not found: $SERVICE_HELPER_PATH" >&2
  echo "install the macOS agent first" >&2
  exit 1
fi

echo "[1/3] Updating agent token..."
tmp="${AGENT_ENV_FILE}.tmp.$$"
awk -F= '$1 !~ /^[[:space:]]*CIWI_GITHUB_TOKEN[[:space:]]*$/' "$AGENT_ENV_FILE" >"$tmp"
printf 'CIWI_GITHUB_TOKEN=%s\n' "$(env_quote "$TOKEN")" >>"$tmp"
chmod 0600 "$tmp"
mv "$tmp" "$AGENT_ENV_FILE"

echo "[2/3] Ensuring bundled service is registered..."
"$SERVICE_HELPER_PATH" register-agent

echo "[3/3] Restarting agent..."
launchctl kickstart -k "gui/$(id -u)/${LABEL}" >/dev/null 2>&1 || true

echo
echo "ciwi macOS agent GitHub token updated."
echo "Agent config: $AGENT_ENV_FILE"
