#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
    echo "usage: $0 <version> <output_binary>" >&2
    exit 1
fi

VERSION=${1#v}
VERSION=${VERSION#V}
OUTPUT_BINARY=$2

mkdir -p "$(dirname "$OUTPUT_BINARY")"
CGO_ENABLED=1 go build \
    -tags nowayland,novulkan \
    -trimpath \
    -ldflags="-s -w -X github.com/izzyreal/ciwi/internal/version.Version=v${VERSION}" \
    -o "$OUTPUT_BINARY" \
    ./cmd/ciwi-desktop
chmod +x "$OUTPUT_BINARY"
