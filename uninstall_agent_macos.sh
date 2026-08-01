#!/bin/sh
set -eu

if [ "$#" -ne 0 ]; then
  echo "this uninstaller takes no options; run it directly" >&2
  exit 2
fi

if [ "$(uname -s)" != "Darwin" ]; then
  echo "this uninstaller is for macOS only" >&2
  exit 1
fi

if [ "$(id -u)" -eq 0 ]; then
  echo "run as your normal user (not root); this removes a LaunchAgent" >&2
  exit 1
fi

LOG_DIR="$HOME/Library/Logs/ciwi"
WORKDIR="$HOME/.ciwi-agent"
NEWSYSLOG_FILE="/etc/newsyslog.d/ciwi-$(id -un).conf"
APP_SUPPORT_DIR="$HOME/Library/Application Support/ciwi"
AGENT_ENV_FILE="$APP_SUPPORT_DIR/agent.env"
APP_BUNDLE="$APP_SUPPORT_DIR/CiwiAgent.app"
SERVICE_HELPER_PATH="$APP_BUNDLE/Contents/MacOS/ciwi-service"

echo "[1/3] Unregistering agent service..."
if [ -x "$SERVICE_HELPER_PATH" ]; then
  "$SERVICE_HELPER_PATH" unregister-agent >/dev/null 2>&1 || true
fi

echo "[2/3] Removing agent configuration and app bundle..."
rm -f "$AGENT_ENV_FILE"
if [ -d "$APP_BUNDLE" ]; then
  rm -rf "$APP_BUNDLE"
  echo "Removed $APP_BUNDLE"
fi

echo "[3/3] Optional cleanup..."
echo "To also remove logs/workdir manually:"
echo "  rm -rf \"$LOG_DIR\" \"$WORKDIR\""
if [ -f "$NEWSYSLOG_FILE" ]; then
  if command -v sudo >/dev/null 2>&1; then
    sudo rm -f "$NEWSYSLOG_FILE" || true
    echo "Removed $NEWSYSLOG_FILE"
  else
    echo "Could not remove $NEWSYSLOG_FILE (sudo not found)" >&2
  fi
fi
echo
echo "ciwi macOS agent uninstall complete."
