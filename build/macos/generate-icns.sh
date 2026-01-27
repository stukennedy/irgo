#!/bin/bash
# Generate macOS .icns icon from source PNG
# Requires: sips (built into macOS), iconutil (built into macOS)
#
# Usage: ./generate-icns.sh <source.png> [output.icns]
#        The source PNG should be 1024x1024 for best quality.

set -e

if [ -z "$1" ]; then
    echo "Usage: $0 <source.png> [output.icns]"
    echo ""
    echo "Generates a macOS .icns icon file from a PNG source."
    echo "The source PNG should be 1024x1024 for best quality."
    echo ""
    echo "Example:"
    echo "  $0 static/icon.png build/macos/icon.icns"
    exit 1
fi

SOURCE="$1"
OUTPUT="${2:-${SOURCE%.png}.icns}"

if [ ! -f "$SOURCE" ]; then
    echo "Error: Source icon not found at $SOURCE"
    echo "Please provide a 1024x1024 PNG icon."
    exit 1
fi

echo "Generating macOS icon from: $SOURCE"

# Create temporary iconset directory
ICONSET=$(mktemp -d)/icon.iconset
mkdir -p "$ICONSET"

# Generate all required sizes for macOS icons
# Standard sizes and retina (@2x) versions
sips -z 16 16     "$SOURCE" --out "$ICONSET/icon_16x16.png" > /dev/null
sips -z 32 32     "$SOURCE" --out "$ICONSET/icon_16x16@2x.png" > /dev/null
sips -z 32 32     "$SOURCE" --out "$ICONSET/icon_32x32.png" > /dev/null
sips -z 64 64     "$SOURCE" --out "$ICONSET/icon_32x32@2x.png" > /dev/null
sips -z 128 128   "$SOURCE" --out "$ICONSET/icon_128x128.png" > /dev/null
sips -z 256 256   "$SOURCE" --out "$ICONSET/icon_128x128@2x.png" > /dev/null
sips -z 256 256   "$SOURCE" --out "$ICONSET/icon_256x256.png" > /dev/null
sips -z 512 512   "$SOURCE" --out "$ICONSET/icon_256x256@2x.png" > /dev/null
sips -z 512 512   "$SOURCE" --out "$ICONSET/icon_512x512.png" > /dev/null
sips -z 1024 1024 "$SOURCE" --out "$ICONSET/icon_512x512@2x.png" > /dev/null

# Ensure output directory exists
mkdir -p "$(dirname "$OUTPUT")"

# Convert iconset to icns
iconutil -c icns "$ICONSET" -o "$OUTPUT"

# Cleanup
rm -rf "$(dirname "$ICONSET")"

echo "Created: $OUTPUT"
