#!/bin/sh
set -eu

if [ "$#" -lt 3 ] || [ "$#" -gt 5 ]; then
    echo "usage: $0 <binary> <architectures> <expected_symbol> [--dwarf-in-dsym] [dsym_bundle]" >&2
    exit 1
fi

BINARY=$1
ARCHITECTURES=$2
EXPECTED_SYMBOL=$3
DWARF_IN_BINARY=1
DSYM_BUNDLE=
case "$#" in
    3) ;;
    4) DSYM_BUNDLE=$4 ;;
    5)
        if [ "$4" != "--dwarf-in-dsym" ]; then
            echo "unsupported option: $4" >&2
            exit 1
        fi
        DWARF_IN_BINARY=0
        DSYM_BUNDLE=$5
        ;;
esac

test -f "$BINARY"

verify_symbols() {
    debug_binary=$1
    description=$2
    architecture=$3

    if ! xcrun nm -arch "$architecture" "$debug_binary" 2>/dev/null | LC_ALL=C grep -Fq "$EXPECTED_SYMBOL"; then
        echo "$description is missing expected symbol for $architecture: $EXPECTED_SYMBOL" >&2
        exit 1
    fi
}

verify_dwarf() {
    debug_binary=$1
    description=$2
    architecture=$3

    if ! xcrun dwarfdump --arch "$architecture" --debug-info "$debug_binary" 2>/dev/null | LC_ALL=C grep -q 'DW_TAG_compile_unit'; then
        echo "$description is missing DWARF compile units for $architecture" >&2
        exit 1
    fi
}

for ARCHITECTURE in $ARCHITECTURES; do
    lipo "$BINARY" -verify_arch "$ARCHITECTURE"
    verify_symbols "$BINARY" "$BINARY" "$ARCHITECTURE"
    if [ "$DWARF_IN_BINARY" = "1" ]; then
        verify_dwarf "$BINARY" "$BINARY" "$ARCHITECTURE"
    fi
done

if [ -z "$DSYM_BUNDLE" ]; then
    exit 0
fi

DSYM_BINARY="$DSYM_BUNDLE/Contents/Resources/DWARF/$(basename "$BINARY")"
test -f "$DSYM_BINARY"

for ARCHITECTURE in $ARCHITECTURES; do
    BINARY_UUID=$(xcrun dwarfdump --uuid "$BINARY" | LC_ALL=C grep "($ARCHITECTURE)" | awk 'NR == 1 { print $2 }')
    DSYM_UUID=$(xcrun dwarfdump --uuid "$DSYM_BINARY" | LC_ALL=C grep "($ARCHITECTURE)" | awk 'NR == 1 { print $2 }')
    if [ -z "$BINARY_UUID" ] || [ "$BINARY_UUID" != "$DSYM_UUID" ]; then
        echo "UUID mismatch for $ARCHITECTURE: binary=${BINARY_UUID:-missing} dSYM=${DSYM_UUID:-missing}" >&2
        exit 1
    fi
    verify_symbols "$DSYM_BINARY" "$DSYM_BUNDLE" "$ARCHITECTURE"
    verify_dwarf "$DSYM_BINARY" "$DSYM_BUNDLE" "$ARCHITECTURE"
done
