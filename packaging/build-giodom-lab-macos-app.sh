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
WORK_DIRECTORY=$(mktemp -d "${TMPDIR:-/tmp}/giodom-lab-macos.XXXXXX")
MACOSX_DEPLOYMENT_TARGET=$MINIMUM_MACOS_VERSION
export MACOSX_DEPLOYMENT_TARGET

case "$VERSION" in
    [0-9]*.[0-9]*.[0-9]*) ;;
    *)
        echo "invalid macOS marketing version: $VERSION" >&2
        exit 1
        ;;
esac

cleanup() {
    rm -rf "$WORK_DIRECTORY"
}
trap cleanup EXIT INT TERM

mkdir -p "$OUTPUT_DIRECTORY"
go run "gioui.org/cmd/gogio@${GOGIO_VERSION}" \
    -target macos \
    -arch arm64,amd64 \
    -appid nl.izmar.giodomlab \
    -name GioDOMLab \
    -version "${VERSION}.1" \
    -o "$WORK_DIRECTORY/apps/GioDOMLab.app" \
    "$ROOT_DIRECTORY/cmd/giodom-lab"

ARM_APP="$WORK_DIRECTORY/apps/GioDOMLab.app/GioDOMLab_arm64.app"
AMD_APP="$WORK_DIRECTORY/apps/GioDOMLab.app/GioDOMLab_amd64.app"
FINAL_APP="$OUTPUT_DIRECTORY/GioDOMLab.app"
test -x "$ARM_APP/Contents/MacOS/GioDOMLab"
test -x "$AMD_APP/Contents/MacOS/GioDOMLab"
rm -rf "$FINAL_APP"
cp -R "$ARM_APP" "$FINAL_APP"
lipo -create \
    "$ARM_APP/Contents/MacOS/GioDOMLab" \
    "$AMD_APP/Contents/MacOS/GioDOMLab" \
    -output "$FINAL_APP/Contents/MacOS/GioDOMLab.universal"
mv "$FINAL_APP/Contents/MacOS/GioDOMLab.universal" "$FINAL_APP/Contents/MacOS/GioDOMLab"
chmod +x "$FINAL_APP/Contents/MacOS/GioDOMLab"
/usr/libexec/PlistBuddy -c "Add :CFBundleDisplayName string Gio DOM Lab" "$FINAL_APP/Contents/Info.plist" 2>/dev/null || \
    /usr/libexec/PlistBuddy -c "Set :CFBundleDisplayName Gio DOM Lab" "$FINAL_APP/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Add :CFBundleName string GioDOMLab" "$FINAL_APP/Contents/Info.plist" 2>/dev/null || \
    /usr/libexec/PlistBuddy -c "Set :CFBundleName GioDOMLab" "$FINAL_APP/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Set :CFBundlePackageType APPL" "$FINAL_APP/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Add :LSMinimumSystemVersion string ${MINIMUM_MACOS_VERSION}" "$FINAL_APP/Contents/Info.plist" 2>/dev/null || \
    /usr/libexec/PlistBuddy -c "Set :LSMinimumSystemVersion ${MINIMUM_MACOS_VERSION}" "$FINAL_APP/Contents/Info.plist"
test "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$FINAL_APP/Contents/Info.plist")" = "nl.izmar.giodomlab"
lipo "$FINAL_APP/Contents/MacOS/GioDOMLab" -verify_arch arm64 x86_64
