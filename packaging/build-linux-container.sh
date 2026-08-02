#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
    echo "usage: $0 <version> <output_zip>" >&2
    exit 1
fi

VERSION=${1#v}
VERSION=${VERSION#V}
OUTPUT_ZIP=$2
ROOT_DIRECTORY=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)

mkdir -p "$(dirname "$OUTPUT_ZIP")"
OUTPUT_DIRECTORY=$(CDPATH= cd -- "$(dirname "$OUTPUT_ZIP")" && pwd)
OUTPUT_NAME=$(basename "$OUTPUT_ZIP")
IMAGE_TAG=ciwi-linux-desktop-builder:go1.25.7-amd64
CONTAINER_ENGINE=${CONTAINER_ENGINE:-docker}
CONTAINER_PLATFORM=${CONTAINER_PLATFORM:-linux/amd64}

if ! command -v "$CONTAINER_ENGINE" >/dev/null 2>&1; then
    echo "container engine not found: $CONTAINER_ENGINE" >&2
    exit 1
fi

"$CONTAINER_ENGINE" build \
    --platform "$CONTAINER_PLATFORM" \
    --tag "$IMAGE_TAG" \
    --file "$ROOT_DIRECTORY/packaging/linux/Dockerfile" \
    "$ROOT_DIRECTORY/packaging/linux"

set -- "$CONTAINER_ENGINE" run --rm \
    --platform "$CONTAINER_PLATFORM" \
    --env HOME=/tmp/ciwi-home \
    --env GOTELEMETRY=off \
    --volume "$ROOT_DIRECTORY:/work:ro" \
    --volume "$OUTPUT_DIRECTORY:/output" \
    --workdir /work

case "$(basename "$CONTAINER_ENGINE")" in
    podman) set -- "$@" --userns keep-id ;;
    *) set -- "$@" --user "$(id -u):$(id -g)" ;;
esac

if [ -n "${GOCACHE:-}" ]; then
    mkdir -p "$GOCACHE"
    set -- "$@" \
        --env GOCACHE=/ciwi-cache/go-build \
        --volume "$GOCACHE:/ciwi-cache/go-build"
fi

if [ -n "${GOMODCACHE:-}" ]; then
    mkdir -p "$GOMODCACHE"
    set -- "$@" \
        --env GOMODCACHE=/ciwi-cache/go-mod \
        --volume "$GOMODCACHE:/ciwi-cache/go-mod"
fi

"$@" "$IMAGE_TAG" sh -c \
    'mkdir -p "$HOME" && exec sh packaging/build-linux-zip.sh "$1" "$2"' \
    sh "$VERSION" "/output/$OUTPUT_NAME"
