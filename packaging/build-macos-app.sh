#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
    echo "usage: $0 <version> <output_directory>" >&2
    exit 1
fi

VERSION=${1#v}
VERSION=${VERSION#V}
OUTPUT_DIRECTORY=$2
GOGIO_VERSION=${GOGIO_VERSION:-v0.10.0}
WORK_DIRECTORY=$(mktemp -d "${TMPDIR:-/tmp}/ciwi-macos-app.XXXXXX")

cleanup() {
    rm -rf "$WORK_DIRECTORY"
}
trap cleanup EXIT INT TERM

mkdir -p "$OUTPUT_DIRECTORY"
go run "gioui.org/cmd/gogio@${GOGIO_VERSION}" \
    -target macos \
    -arch arm64,amd64 \
    -appid nl.izmar.ciwi.desktop \
    -name Ciwi \
    -version "${VERSION}.1" \
    -icon packaging/icons/ciwi.png \
    -ldflags "-s -w -X github.com/izzyreal/ciwi/internal/version.Version=v${VERSION}" \
    -o "$WORK_DIRECTORY/apps/Ciwi.app" \
    ./cmd/ciwi-desktop

ARM_APP="$WORK_DIRECTORY/apps/Ciwi.app/Ciwi_arm64.app"
AMD_APP="$WORK_DIRECTORY/apps/Ciwi.app/Ciwi_amd64.app"
FINAL_APP="$OUTPUT_DIRECTORY/Ciwi.app"
test -x "$ARM_APP/Contents/MacOS/Ciwi"
test -x "$AMD_APP/Contents/MacOS/Ciwi"
rm -rf "$FINAL_APP"
cp -R "$ARM_APP" "$FINAL_APP"
lipo -create \
    "$ARM_APP/Contents/MacOS/Ciwi" \
    "$AMD_APP/Contents/MacOS/Ciwi" \
    -output "$FINAL_APP/Contents/MacOS/Ciwi.universal"
mv "$FINAL_APP/Contents/MacOS/Ciwi.universal" "$FINAL_APP/Contents/MacOS/Ciwi"
chmod +x "$FINAL_APP/Contents/MacOS/Ciwi"
/usr/libexec/PlistBuddy -c "Add :CFBundleDisplayName string Ciwi" "$FINAL_APP/Contents/Info.plist" 2>/dev/null || \
    /usr/libexec/PlistBuddy -c "Set :CFBundleDisplayName Ciwi" "$FINAL_APP/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Add :CFBundleShortVersionString string ${VERSION}" "$FINAL_APP/Contents/Info.plist" 2>/dev/null || \
    /usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString ${VERSION}" "$FINAL_APP/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Add :CFBundleVersion string ${VERSION}" "$FINAL_APP/Contents/Info.plist" 2>/dev/null || \
    /usr/libexec/PlistBuddy -c "Set :CFBundleVersion ${VERSION}" "$FINAL_APP/Contents/Info.plist"
lipo "$FINAL_APP/Contents/MacOS/Ciwi" -verify_arch arm64 x86_64
