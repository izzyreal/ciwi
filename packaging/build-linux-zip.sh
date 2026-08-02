#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
    echo "usage: $0 <version> <output_zip>" >&2
    exit 1
fi

VERSION=$1
OUTPUT_ZIP=$2
WORK_DIRECTORY=$(mktemp -d "${TMPDIR:-/tmp}/ciwi-linux-client.XXXXXX")
PACKAGE_DIRECTORY="$WORK_DIRECTORY/Ciwi"
FINAL_ZIP=$(cd "$(dirname "$OUTPUT_ZIP")" && pwd)/$(basename "$OUTPUT_ZIP")

cleanup() {
    rm -rf "$WORK_DIRECTORY"
}
trap cleanup EXIT INT TERM

mkdir -p "$PACKAGE_DIRECTORY"
CGO_ENABLED=1 go build \
    -tags nowayland,novulkan \
    -trimpath \
    -ldflags="-s -w -X github.com/izzyreal/ciwi/internal/version.Version=${VERSION}" \
    -o "$PACKAGE_DIRECTORY/ciwi" \
    ./cmd/ciwi-desktop
cp packaging/icons/ciwi.png "$PACKAGE_DIRECTORY/ciwi.png"
cp packaging/linux/ciwi.desktop "$PACKAGE_DIRECTORY/ciwi.desktop"
cp packaging/linux/README.txt "$PACKAGE_DIRECTORY/README.txt"
chmod +x "$PACKAGE_DIRECTORY/ciwi"
rm -f "$FINAL_ZIP"
(cd "$WORK_DIRECTORY" && zip -qr "$FINAL_ZIP" Ciwi)
