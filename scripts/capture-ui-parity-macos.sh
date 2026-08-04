#!/bin/sh
set -eu

surface="${1:-front-page}"
output_dir="${2:-/tmp/ciwi-ui-parity}"

mkdir -p "$output_dir"

echo "Select the Ciwi native window for the ${surface} capture."
screencapture -i "$output_dir/${surface}-native.png"

echo "Select the Ciwi browser window showing the same state."
screencapture -i "$output_dir/${surface}-web.png"

echo "Captured:"
echo "  $output_dir/${surface}-native.png"
echo "  $output_dir/${surface}-web.png"
