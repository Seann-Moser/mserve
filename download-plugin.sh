#!/usr/bin/env bash
set -euo pipefail

# Usage check
if [ $# -ne 1 ]; then
  echo "Usage: $0 <url-to-js>"
  exit 1
fi
URL="$1"

# Prepare
PLUGINS_DIR="plugins"
hdr=$(mktemp /tmp/plugin_hdr.XXXXXX)
body="${hdr}.body"

# Cleanup temp files on exit
trap 'rm -f "$hdr" "$body"' EXIT

mkdir -p "$PLUGINS_DIR"

# Fetch: headers → $hdr, body → $body
if ! curl -sSL -D "$hdr" -o "$body" "$URL"; then
  echo "Error: download failed"
  exit 1
fi

# Extract "Name:" header (case-insensitive), strip CR/LF
fn=$(grep -i '^Name:' "$hdr" \
     | sed 's/^[Nn]ame:[[:space:]]*//; s/\r$//')

if [ -z "$fn" ]; then
  echo "Error: Name header not found"
  exit 1
fi

# Move into plugins/ under that filename
mv "$body" "$PLUGINS_DIR/$fn"
echo "Downloaded → $PLUGINS_DIR/$fn"
