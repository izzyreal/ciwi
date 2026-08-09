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
    -name GioDOMLab \
    -version "${VERSION}.1" \
    -o "$OUTPUT_FRAMEWORK" \
    "$ROOT_DIRECTORY/cmd/giodom-lab"

test -f "$OUTPUT_FRAMEWORK/Versions/A/GioDOMLab"
test -f "$OUTPUT_FRAMEWORK/Versions/A/Headers/GioDOMLab.h"
