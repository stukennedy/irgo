#!/bin/bash
# Creates a DMG installer from a macOS .app bundle
# Optionally signs and notarizes for distribution
#
# Usage: create-dmg.sh [options]
#
# Required options:
#   --app <path>         Path to the .app bundle
#   --name <name>        Application name for the DMG volume
#
# Optional:
#   --output <path>      Output DMG path (default: <app-dir>/<name>.dmg)
#   --version <ver>      Version to include in DMG filename
#   --icon <path>        Volume icon (.icns file)
#   --entitlements <p>   Entitlements file for code signing
#
# Environment variables for signing/notarization:
#   APPLE_DEVELOPER_ID     - Developer ID for code signing
#   APPLE_NOTARY_PROFILE   - Keychain profile for notarization
#
# Example:
#   ./create-dmg.sh \
#     --app "dist/My App.app" \
#     --name "My App" \
#     --version 1.2.3

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# Parse arguments
APP_PATH=""
APP_NAME=""
OUTPUT_PATH=""
VERSION=""
ICON_PATH=""
ENTITLEMENTS="$SCRIPT_DIR/entitlements.plist"

while [[ $# -gt 0 ]]; do
    case $1 in
        --app)
            APP_PATH="$2"
            shift 2
            ;;
        --name)
            APP_NAME="$2"
            shift 2
            ;;
        --output)
            OUTPUT_PATH="$2"
            shift 2
            ;;
        --version)
            VERSION="$2"
            shift 2
            ;;
        --icon)
            ICON_PATH="$2"
            shift 2
            ;;
        --entitlements)
            ENTITLEMENTS="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Validate required arguments
if [ -z "$APP_PATH" ] || [ -z "$APP_NAME" ]; then
    echo "Usage: $0 --app <path> --name <name> [options]"
    echo ""
    echo "Required:"
    echo "  --app <path>         Path to the .app bundle"
    echo "  --name <name>        Application name for the DMG"
    echo ""
    echo "Optional:"
    echo "  --output <path>      Output DMG path"
    echo "  --version <ver>      Version for DMG filename"
    echo "  --icon <path>        Volume icon (.icns)"
    echo "  --entitlements <p>   Entitlements for signing"
    echo ""
    echo "Environment variables:"
    echo "  APPLE_DEVELOPER_ID   - Developer ID for signing"
    echo "  APPLE_NOTARY_PROFILE - Notarization profile"
    exit 1
fi

if [ ! -d "$APP_PATH" ]; then
    echo "Error: App bundle not found at $APP_PATH"
    exit 1
fi

# Set defaults
APP_DIR=$(dirname "$APP_PATH")
if [ -n "$VERSION" ]; then
    DMG_NAME="${APP_NAME}_${VERSION}_macOS.dmg"
else
    DMG_NAME="${APP_NAME}.dmg"
fi
OUTPUT_PATH="${OUTPUT_PATH:-$APP_DIR/$DMG_NAME}"

echo "Creating DMG: $OUTPUT_PATH"

# Create staging directory
STAGING_DIR=$(mktemp -d)
cp -r "$APP_PATH" "$STAGING_DIR/"

# Sign the app bundle if APPLE_DEVELOPER_ID is set
if [ -n "$APPLE_DEVELOPER_ID" ]; then
    echo "Signing app bundle..."
    SIGN_ARGS=(--force --deep --options runtime --timestamp)
    if [ -f "$ENTITLEMENTS" ]; then
        SIGN_ARGS+=(--entitlements "$ENTITLEMENTS")
    fi
    SIGN_ARGS+=(--sign "$APPLE_DEVELOPER_ID")

    codesign "${SIGN_ARGS[@]}" "$STAGING_DIR/$(basename "$APP_PATH")"
fi

# Remove existing DMG
rm -f "$OUTPUT_PATH"

# Check if create-dmg tool is available (creates nicer DMGs)
if command -v create-dmg &> /dev/null; then
    CREATE_DMG_ARGS=(
        --volname "$APP_NAME"
        --window-pos 200 120
        --window-size 600 400
        --icon-size 100
        --icon "$(basename "$APP_PATH")" 150 190
        --hide-extension "$(basename "$APP_PATH")"
        --app-drop-link 450 190
    )

    if [ -n "$ICON_PATH" ] && [ -f "$ICON_PATH" ]; then
        CREATE_DMG_ARGS+=(--volicon "$ICON_PATH")
    fi

    create-dmg "${CREATE_DMG_ARGS[@]}" "$OUTPUT_PATH" "$STAGING_DIR"
else
    # Fallback to hdiutil
    echo "Note: Install 'create-dmg' for nicer DMG appearance"
    echo "  brew install create-dmg"
    hdiutil create -volname "$APP_NAME" \
        -srcfolder "$STAGING_DIR" \
        -ov -format UDZO \
        "$OUTPUT_PATH"
fi

# Sign the DMG if APPLE_DEVELOPER_ID is set
if [ -n "$APPLE_DEVELOPER_ID" ]; then
    echo "Signing DMG..."
    codesign --force --sign "$APPLE_DEVELOPER_ID" --timestamp "$OUTPUT_PATH"
fi

# Notarize if APPLE_NOTARY_PROFILE is set
if [ -n "$APPLE_NOTARY_PROFILE" ]; then
    echo "Submitting for notarization..."
    xcrun notarytool submit "$OUTPUT_PATH" \
        --keychain-profile "$APPLE_NOTARY_PROFILE" \
        --wait

    echo "Stapling notarization ticket..."
    xcrun stapler staple "$OUTPUT_PATH"
fi

# Cleanup staging
rm -rf "$STAGING_DIR"

echo "Created: $OUTPUT_PATH"
