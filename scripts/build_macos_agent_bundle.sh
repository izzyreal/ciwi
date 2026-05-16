#!/bin/sh
set -eu

if [ "$#" -ne 4 ]; then
  echo "usage: build_macos_agent_bundle.sh <binary> <version> <bundle-dir> <zip-output>" >&2
  exit 2
fi

BIN_PATH="$1"
VERSION="$2"
BUNDLE_DIR="$3"
ZIP_OUTPUT="$4"

if [ ! -f "$BIN_PATH" ]; then
  echo "binary not found: $BIN_PATH" >&2
  exit 1
fi

if [ -z "${DEV_IDENTITY_APP:-}" ]; then
  echo "DEV_IDENTITY_APP is required" >&2
  exit 1
fi

WORK_DIR="$(mktemp -d -t ciwi-macos-bundle.XXXXXX)"
cleanup() {
  rm -rf "$WORK_DIR"
}
trap cleanup EXIT INT TERM

APP_NAME="CiwiAgent.app"
APP_PATH="${WORK_DIR}/${APP_NAME}"
CONTENTS_DIR="${APP_PATH}/Contents"
MACOS_DIR="${CONTENTS_DIR}/MacOS"

mkdir -p "$MACOS_DIR"
install -m 0755 "$BIN_PATH" "${MACOS_DIR}/ciwi"

cat >"${CONTENTS_DIR}/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key>
  <string>en</string>
  <key>CFBundleDisplayName</key>
  <string>Ciwi Agent</string>
  <key>CFBundleExecutable</key>
  <string>ciwi</string>
  <key>CFBundleIdentifier</key>
  <string>nl.izmar.ciwi.agent-app</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>CFBundleName</key>
  <string>CiwiAgent</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>${VERSION}</string>
  <key>CFBundleVersion</key>
  <string>${VERSION}</string>
  <key>LSBackgroundOnly</key>
  <true/>
  <key>NSLocalNetworkUsageDescription</key>
  <string>ciwi agent connects to your ciwi server on the local network to send heartbeats and run jobs.</string>
</dict>
</plist>
EOF

codesign --force --deep --strict --options=runtime --timestamp --sign "$DEV_IDENTITY_APP" -v "$APP_PATH"

rm -rf "$BUNDLE_DIR"
mkdir -p "$(dirname "$BUNDLE_DIR")"
cp -R "$APP_PATH" "$BUNDLE_DIR"

rm -f "$ZIP_OUTPUT"
mkdir -p "$(dirname "$ZIP_OUTPUT")"
(
  cd "$WORK_DIR"
  ditto -c -k --sequesterRsrc --keepParent "$APP_NAME" "$ZIP_OUTPUT"
)
