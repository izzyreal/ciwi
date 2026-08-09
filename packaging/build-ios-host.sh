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
APP_BUNDLE_IDENTIFIER=${CIWI_IOS_BUNDLE_IDENTIFIER:-nl.izmar.ciwi}
APP_DISPLAY_NAME=${CIWI_IOS_DISPLAY_NAME:-Ciwi}
OFFLINE_APP=${CIWI_IOS_OFFLINE_APP:-0}
FRAMEWORK_NAME=${CIWI_IOS_FRAMEWORK_NAME:-Ciwi}
EXPECTED_BINARY_MARKER=${CIWI_IOS_EXPECTED_BINARY_MARKER:-}
WORK_DIRECTORY=$(mktemp -d "${TMPDIR:-/tmp}/ciwi-ios-host.XXXXXX")
DERIVED_DATA="$WORK_DIRECTORY/DerivedData"

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
case "$FRAMEWORK_NAME" in
    ''|*[!A-Za-z0-9_]*)
        echo "invalid iOS framework name: $FRAMEWORK_NAME" >&2
        exit 1
        ;;
esac
if [ "$BUILD_NUMBER" -lt 1 ]; then
    echo "invalid iOS build number: $BUILD_NUMBER" >&2
    exit 1
fi
test -f "$INPUT_FRAMEWORK/Versions/A/$FRAMEWORK_NAME"

cp -R "$ROOT_DIRECTORY/packaging/ios" "$WORK_DIRECTORY/ios"
mkdir -p "$WORK_DIRECTORY/ios/Generated"
cp -R "$INPUT_FRAMEWORK" "$WORK_DIRECTORY/ios/Generated/$FRAMEWORK_NAME.framework"
if [ "$FRAMEWORK_NAME" != "Ciwi" ]; then
    /usr/bin/sed -i '' -e "s/Ciwi\\.framework/$FRAMEWORK_NAME.framework/g" "$WORK_DIRECTORY/ios/Ciwi.xcodeproj/project.pbxproj"
    /usr/bin/sed -i '' -e "s#<Ciwi/Ciwi.h>#<$FRAMEWORK_NAME/$FRAMEWORK_NAME.h>#g" "$WORK_DIRECTORY/ios/Ciwi/AppDelegate.m"
fi
/usr/libexec/PlistBuddy -c "Set :CFBundleDisplayName $APP_DISPLAY_NAME" "$WORK_DIRECTORY/ios/Ciwi/Info.plist"
if [ "$OFFLINE_APP" = "1" ]; then
    /usr/libexec/PlistBuddy -c "Delete :NSBonjourServices" "$WORK_DIRECTORY/ios/Ciwi/Info.plist" 2>/dev/null || true
    /usr/libexec/PlistBuddy -c "Delete :NSLocalNetworkUsageDescription" "$WORK_DIRECTORY/ios/Ciwi/Info.plist" 2>/dev/null || true
fi

COMMON_ARGUMENTS="MARKETING_VERSION=$VERSION CURRENT_PROJECT_VERSION=$BUILD_NUMBER DEVELOPMENT_TEAM=$APPLE_TEAM_ID PRODUCT_BUNDLE_IDENTIFIER=$APP_BUNDLE_IDENTIFIER"

verify_app() {
    app_executable=$1
    test -x "$app_executable"
    if [ -n "$EXPECTED_BINARY_MARKER" ] && ! LC_ALL=C /usr/bin/grep -aFq "$EXPECTED_BINARY_MARKER" "$app_executable"; then
        echo "iOS app executable does not contain expected payload marker: $EXPECTED_BINARY_MARKER" >&2
        exit 1
    fi
}

if [ "$MODE" = "check" ]; then
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
    verify_app "$BUILT_APP/Ciwi"
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
    -derivedDataPath "$DERIVED_DATA" \
    -archivePath "$OUTPUT" \
    $COMMON_ARGUMENTS \
    CODE_SIGN_STYLE=Automatic \
    -allowProvisioningUpdates \
    archive

verify_app "$OUTPUT/Products/Applications/Ciwi.app/Ciwi"
