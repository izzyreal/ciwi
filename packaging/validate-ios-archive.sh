#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
    echo "usage: $0 <version> <archive>" >&2
    exit 1
fi

VERSION=${1#v}
VERSION=${VERSION#V}
ARCHIVE=$2
APP="$ARCHIVE/Products/Applications/Ciwi.app"
PLIST="$APP/Info.plist"
PLIST_BUDDY=/usr/libexec/PlistBuddy

test -d "$ARCHIVE"
test -x "$APP/Ciwi"
test -f "$PLIST"
test -f "$APP/embedded.mobileprovision"
test "$($PLIST_BUDDY -c 'Print :CFBundleIdentifier' "$PLIST")" = "nl.izmar.ciwi"
test "$($PLIST_BUDDY -c 'Print :CFBundleShortVersionString' "$PLIST")" = "$VERSION"
test "$($PLIST_BUDDY -c 'Print :MinimumOSVersion' "$PLIST")" = "16.0"
test "$($PLIST_BUDDY -c 'Print :NSBonjourServices:0' "$PLIST")" = "_ciwi-native._udp"
test "$($PLIST_BUDDY -c 'Print :NSBonjourServices:1' "$PLIST")" = "_ciwi-native._tcp"
test -n "$($PLIST_BUDDY -c 'Print :NSLocalNetworkUsageDescription' "$PLIST")"
test "$($PLIST_BUDDY -c 'Print :ITSAppUsesNonExemptEncryption' "$PLIST")" = "false"
test "$($PLIST_BUDDY -c 'Print :UIDeviceFamily:0' "$PLIST")" = "1"
test "$($PLIST_BUDDY -c 'Print :UIDeviceFamily:1' "$PLIST")" = "2"
lipo "$APP/Ciwi" -verify_arch arm64
"$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)/packaging/verify-apple-debug-info.sh" \
    "$APP/Ciwi" \
    arm64 \
    "github.com/izzyreal/ciwi/internal/adapters/gio.(*Renderer).SetOperations" \
    --dwarf-in-dsym \
    "$ARCHIVE/dSYMs/Ciwi.app.dSYM"
codesign --verify --deep --strict -v "$APP"
