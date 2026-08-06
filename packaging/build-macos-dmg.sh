#!/bin/sh
set -eu

if [ "$#" -ne 3 ]; then
    echo "usage: $0 <app_bundle> <version> <output_dmg>" >&2
    exit 1
fi

APP_BUNDLE=$1
VERSION=$2
OUTPUT_DMG=$3
WORK_DIRECTORY=$(mktemp -d "${TMPDIR:-/tmp}/ciwi-dmg.XXXXXX")
FINAL_DMG=$(cd "$(dirname "$OUTPUT_DMG")" && pwd)/$(basename "$OUTPUT_DMG")
APP_BUNDLE_ABS=$(cd "$(dirname "$APP_BUNDLE")" && pwd)/$(basename "$APP_BUNDLE")

cleanup() {
    rm -rf "$WORK_DIRECTORY"
}
trap cleanup EXIT INT TERM

test -d "$APP_BUNDLE_ABS"
test -f ui/assets/ciwi-logo.png
command -v dmgbuild >/dev/null 2>&1
swift packaging/create-dmg-background.swift \
    ui/assets/ciwi-logo.png \
    "$WORK_DIRECTORY/background.png"

rm -f "$FINAL_DMG"
CIWI_DMG_APP="$APP_BUNDLE_ABS" \
CIWI_DMG_BACKGROUND="$WORK_DIRECTORY/background.png" \
CIWI_DMG_VERSION="$VERSION" \
    dmgbuild -s packaging/dmgbuild-settings.py "Ciwi" "$FINAL_DMG"
