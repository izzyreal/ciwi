#!/bin/sh
set -eu

if [ "$#" -ne 4 ]; then
    echo "usage: $0 <check|archive> <version> <input_framework> <output>" >&2
    exit 1
fi

MODE=$1
VERSION=${2#v}
VERSION=${VERSION#V}
INPUT_FRAMEWORK=$3
OUTPUT=$4
ROOT_DIRECTORY=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
APPLE_TEAM_ID=${APPLE_TEAM_ID:-KFBA7Q5H76}
BUILD_NUMBER=${CIWI_IOS_BUILD_NUMBER:-1}
WORK_DIRECTORY=$(mktemp -d "${TMPDIR:-/tmp}/ciwi-ios-host.XXXXXX")

cleanup() {
    rm -rf "$WORK_DIRECTORY"
}
trap cleanup EXIT INT TERM

case "$MODE" in
    check|archive) ;;
    *)
        echo "unsupported iOS host build mode: $MODE" >&2
        exit 1
        ;;
esac
case "$VERSION" in
    [0-9]*.[0-9]*.[0-9]*) ;;
    *)
        echo "invalid iOS marketing version: $VERSION" >&2
        exit 1
        ;;
esac
case "$BUILD_NUMBER" in
    ''|*[!0-9]*)
        echo "invalid iOS build number: $BUILD_NUMBER" >&2
        exit 1
        ;;
esac
if [ "$BUILD_NUMBER" -lt 1 ]; then
    echo "invalid iOS build number: $BUILD_NUMBER" >&2
    exit 1
fi
test -f "$INPUT_FRAMEWORK/Versions/A/Ciwi"

cp -R "$ROOT_DIRECTORY/packaging/ios" "$WORK_DIRECTORY/ios"
mkdir -p "$WORK_DIRECTORY/ios/Generated"
cp -R "$INPUT_FRAMEWORK" "$WORK_DIRECTORY/ios/Generated/Ciwi.framework"

COMMON_ARGUMENTS="MARKETING_VERSION=$VERSION CURRENT_PROJECT_VERSION=$BUILD_NUMBER DEVELOPMENT_TEAM=$APPLE_TEAM_ID"
if [ "$MODE" = "check" ]; then
    DERIVED_DATA="$WORK_DIRECTORY/DerivedData"
    # shellcheck disable=SC2086
    xcodebuild \
        -project "$WORK_DIRECTORY/ios/Ciwi.xcodeproj" \
        -scheme Ciwi \
        -configuration Release \
        -sdk iphoneos \
        -destination "generic/platform=iOS" \
        -derivedDataPath "$DERIVED_DATA" \
        $COMMON_ARGUMENTS \
        CODE_SIGNING_ALLOWED=NO \
        CODE_SIGNING_REQUIRED=NO \
        build
    BUILT_APP="$DERIVED_DATA/Build/Products/Release-iphoneos/Ciwi.app"
    test -x "$BUILT_APP/Ciwi"
    rm -rf "$OUTPUT"
    mkdir -p "$(dirname "$OUTPUT")"
    cp -R "$BUILT_APP" "$OUTPUT"
    exit 0
fi

rm -rf "$OUTPUT"
mkdir -p "$(dirname "$OUTPUT")"
# shellcheck disable=SC2086
xcodebuild \
    -project "$WORK_DIRECTORY/ios/Ciwi.xcodeproj" \
    -scheme Ciwi \
    -configuration Release \
    -sdk iphoneos \
    -destination "generic/platform=iOS" \
    -archivePath "$OUTPUT" \
    $COMMON_ARGUMENTS \
    CODE_SIGN_STYLE=Automatic \
    -allowProvisioningUpdates \
    archive

test -x "$OUTPUT/Products/Applications/Ciwi.app/Ciwi"
