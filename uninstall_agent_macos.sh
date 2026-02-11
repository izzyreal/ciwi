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

LABEL="nl.izmar.ciwi.agent"
PLIST_PATH="$HOME/Library/LaunchAgents/${LABEL}.plist"
LOG_DIR="$HOME/Library/Logs/ciwi"
WORKDIR="$HOME/.ciwi-agent"
NEWSYSLOG_FILE="/etc/newsyslog.d/ciwi-$(id -un).conf"
UID_NUM="$(id -u)"

# Try both common install locations used by installer versions.
BINARY_USER="$HOME/.local/bin/ciwi"
BINARY_SYSTEM="/usr/local/bin/ciwi"

echo "[1/4] Stopping LaunchAgent if loaded..."
launchctl bootout "gui/${UID_NUM}" "$PLIST_PATH" >/dev/null 2>&1 || true
launchctl disable "gui/${UID_NUM}/${LABEL}" >/dev/null 2>&1 || true

echo "[2/4] Removing LaunchAgent plist..."
rm -f "$PLIST_PATH"

echo "[3/4] Removing ciwi binary..."
if [ -f "$BINARY_USER" ]; then
  rm -f "$BINARY_USER"
  echo "Removed $BINARY_USER"
fi
if [ -f "$BINARY_SYSTEM" ]; then
  if command -v sudo >/dev/null 2>&1; then
    sudo rm -f "$BINARY_SYSTEM"
    echo "Removed $BINARY_SYSTEM"
  else
    echo "Could not remove $BINARY_SYSTEM (sudo not found)" >&2
  fi
fi

echo "[4/4] Optional cleanup..."
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
