#!/bin/sh
set -eu

if [ "$#" -ne 3 ]; then
    echo "usage: $0 <input_binary> <version> <output_zip>" >&2
    exit 1
fi

INPUT_BINARY=$1
VERSION=${2#v}
VERSION=${VERSION#V}
OUTPUT_ZIP=$3
WORK_DIRECTORY=$(mktemp -d "${TMPDIR:-/tmp}/ciwi-linux-client.XXXXXX")
PACKAGE_DIRECTORY="$WORK_DIRECTORY/Ciwi"
FINAL_ZIP=$(cd "$(dirname "$OUTPUT_ZIP")" && pwd)/$(basename "$OUTPUT_ZIP")

cleanup() {
    rm -rf "$WORK_DIRECTORY"
}
trap cleanup EXIT INT TERM

mkdir -p "$PACKAGE_DIRECTORY"
test -f "$INPUT_BINARY"
cp "$INPUT_BINARY" "$PACKAGE_DIRECTORY/ciwi"
cp packaging/icons/ciwi.png "$PACKAGE_DIRECTORY/ciwi.png"
cp packaging/linux/ciwi.desktop "$PACKAGE_DIRECTORY/ciwi.desktop"
cp packaging/linux/README.txt "$PACKAGE_DIRECTORY/README.txt"
chmod +x "$PACKAGE_DIRECTORY/ciwi"
rm -f "$FINAL_ZIP"
(cd "$WORK_DIRECTORY" && zip -qr "$FINAL_ZIP" Ciwi)
