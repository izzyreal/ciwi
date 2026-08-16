#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
    echo "usage: $0 <version> <output_directory>" >&2
    exit 1
fi

VERSION=${1#v}
VERSION=${VERSION#V}
OUTPUT_DIRECTORY=$2
ROOT_DIRECTORY=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
GOGIO_VERSION=${GOGIO_VERSION:-v0.10.0}
MINIMUM_MACOS_VERSION=11.0
WORK_DIRECTORY=$(mktemp -d "${TMPDIR:-/tmp}/ciwi-macos-app.XXXXXX")
GOFLAGS="${GOFLAGS:+$GOFLAGS }-trimpath"

# Keep the universal bundle compatible with the first macOS release that
# supports Apple Silicon. Without this, cgo inherits the build machine's SDK
# version and the resulting app may require that newer macOS release.
MACOSX_DEPLOYMENT_TARGET=$MINIMUM_MACOS_VERSION
export GOFLAGS MACOSX_DEPLOYMENT_TARGET

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
    -ldflags "-compressdwarf=false -X github.com/izzyreal/ciwi/internal/version.Version=v${VERSION}" \
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
/usr/libexec/PlistBuddy -c "Add :CFBundleName string Ciwi" "$FINAL_APP/Contents/Info.plist" 2>/dev/null || \
    /usr/libexec/PlistBuddy -c "Set :CFBundleName Ciwi" "$FINAL_APP/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Set :CFBundlePackageType APPL" "$FINAL_APP/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Add :CFBundleShortVersionString string ${VERSION}" "$FINAL_APP/Contents/Info.plist" 2>/dev/null || \
    /usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString ${VERSION}" "$FINAL_APP/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Add :CFBundleVersion string ${VERSION}" "$FINAL_APP/Contents/Info.plist" 2>/dev/null || \
    /usr/libexec/PlistBuddy -c "Set :CFBundleVersion ${VERSION}" "$FINAL_APP/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Add :LSMinimumSystemVersion string ${MINIMUM_MACOS_VERSION}" "$FINAL_APP/Contents/Info.plist" 2>/dev/null || \
    /usr/libexec/PlistBuddy -c "Set :LSMinimumSystemVersion ${MINIMUM_MACOS_VERSION}" "$FINAL_APP/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Delete :NSBonjourServices" "$FINAL_APP/Contents/Info.plist" 2>/dev/null || true
/usr/libexec/PlistBuddy -c "Add :NSBonjourServices array" "$FINAL_APP/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Add :NSBonjourServices:0 string _ciwi-native._udp" "$FINAL_APP/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Add :NSBonjourServices:1 string _ciwi-native._tcp" "$FINAL_APP/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Add :NSLocalNetworkUsageDescription string Ciwi discovers and connects to your Ciwi server on the local network." "$FINAL_APP/Contents/Info.plist" 2>/dev/null || \
    /usr/libexec/PlistBuddy -c "Set :NSLocalNetworkUsageDescription Ciwi discovers and connects to your Ciwi server on the local network." "$FINAL_APP/Contents/Info.plist"
test "$(/usr/libexec/PlistBuddy -c 'Print :CFBundlePackageType' "$FINAL_APP/Contents/Info.plist")" = "APPL"
lipo "$FINAL_APP/Contents/MacOS/Ciwi" -verify_arch arm64 x86_64

"$ROOT_DIRECTORY/packaging/verify-apple-debug-info.sh" \
    "$FINAL_APP/Contents/MacOS/Ciwi" \
    "arm64 x86_64" \
    "github.com/izzyreal/ciwi/internal/adapters/gio.(*Renderer).SetOperations"

for ARCH in arm64 x86_64; do
    ACTUAL_MINIMUM=$(vtool -show-build -arch "$ARCH" "$FINAL_APP/Contents/MacOS/Ciwi" | awk '$1 == "minos" { print $2; exit }')
    if [ "$ACTUAL_MINIMUM" != "$MINIMUM_MACOS_VERSION" ]; then
        echo "unexpected macOS deployment target for ${ARCH}: ${ACTUAL_MINIMUM:-missing} (expected ${MINIMUM_MACOS_VERSION})" >&2
        exit 1
    fi
done
