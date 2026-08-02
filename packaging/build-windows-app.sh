#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
    echo "usage: $0 <version> <output_exe>" >&2
    exit 1
fi

VERSION=${1#v}
VERSION=${VERSION#V}
OUTPUT_EXE=$2
ROOT_DIRECTORY=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
RESOURCE_FILE="$ROOT_DIRECTORY/cmd/ciwi-desktop/Ciwi_windows_amd64.syso"
GOGIO_VERSION=${GOGIO_VERSION:-v0.10.0}

cleanup() {
    rm -f "$RESOURCE_FILE"
}
trap cleanup EXIT INT TERM

mkdir -p "$(dirname "$OUTPUT_EXE")"
rm -f "$RESOURCE_FILE" "$OUTPUT_EXE"
go run "gioui.org/cmd/gogio@$GOGIO_VERSION" \
    -target windows \
    -arch amd64 \
    -minsdk 10 \
    -appid nl.izmar.ciwi.desktop \
    -name Ciwi \
    -version "$VERSION.1" \
    -icon "$ROOT_DIRECTORY/packaging/icons/ciwi.png" \
    -ldflags "-s -w -X github.com/izzyreal/ciwi/internal/version.Version=v$VERSION" \
    -o "$OUTPUT_EXE" \
    "$ROOT_DIRECTORY/cmd/ciwi-desktop"
