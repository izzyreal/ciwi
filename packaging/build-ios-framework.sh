#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
    echo "usage: $0 <version> <output_framework>" >&2
    exit 1
fi

VERSION=${1#v}
VERSION=${VERSION#V}
OUTPUT_FRAMEWORK=$2
ROOT_DIRECTORY=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
GOGIO_VERSION=${GOGIO_VERSION:-v0.10.0}
GOFLAGS="${GOFLAGS:+$GOFLAGS }-trimpath"
export GOFLAGS

case "$VERSION" in
    [0-9]*.[0-9]*.[0-9]*) ;;
    *)
        echo "invalid iOS marketing version: $VERSION" >&2
        exit 1
        ;;
esac

rm -rf "$OUTPUT_FRAMEWORK"
mkdir -p "$(dirname "$OUTPUT_FRAMEWORK")"
go run "gioui.org/cmd/gogio@${GOGIO_VERSION}" \
    -target ios \
    -buildmode archive \
    -arch arm64 \
    -minsdk 16 \
    -name Ciwi \
    -version "${VERSION}.1" \
    -ldflags "-s=false -w=false -compressdwarf=false -X github.com/izzyreal/ciwi/internal/version.Version=v${VERSION}" \
    -o "$OUTPUT_FRAMEWORK" \
    "$ROOT_DIRECTORY/cmd/ciwi-desktop"

test -f "$OUTPUT_FRAMEWORK/Versions/A/Ciwi"
test -f "$OUTPUT_FRAMEWORK/Versions/A/Headers/Ciwi.h"
"$ROOT_DIRECTORY/packaging/verify-apple-debug-info.sh" \
    "$OUTPUT_FRAMEWORK/Versions/A/Ciwi" \
    arm64 \
    "github.com/izzyreal/ciwi/internal/adapters/gio.(*Renderer).SetOperations"
