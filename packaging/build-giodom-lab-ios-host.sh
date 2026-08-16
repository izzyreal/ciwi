#!/bin/sh
set -eu

if [ "$#" -ne 4 ]; then
    echo "usage: $0 <check|archive> <version> <input_framework> <output>" >&2
    exit 1
fi

ROOT_DIRECTORY=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
CIWI_IOS_BUNDLE_IDENTIFIER=${GIODOM_LAB_IOS_BUNDLE_IDENTIFIER:-nl.izmar.giodomlab}
CIWI_IOS_DISPLAY_NAME="Gio DOM Lab"
CIWI_IOS_OFFLINE_APP=1
CIWI_IOS_FRAMEWORK_NAME="GioDOMLab"
CIWI_IOS_EXPECTED_BINARY_MARKER="Gio DOM Viability Lab"
CIWI_IOS_EXPECTED_DEBUG_SYMBOL="main.main"
export CIWI_IOS_BUNDLE_IDENTIFIER CIWI_IOS_DISPLAY_NAME CIWI_IOS_OFFLINE_APP CIWI_IOS_FRAMEWORK_NAME CIWI_IOS_EXPECTED_BINARY_MARKER CIWI_IOS_EXPECTED_DEBUG_SYMBOL

exec "$ROOT_DIRECTORY/packaging/build-ios-host.sh" "$@"
