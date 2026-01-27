#!/bin/bash
# Creates a macOS .app bundle from a built binary
#
# Usage: create-app-bundle.sh [options]
#
# Required options:
#   --binary <path>      Path to the compiled binary
#   --name <name>        Application name (e.g., "My App")
#   --bundle-id <id>     Bundle identifier (e.g., "com.example.myapp")
#
# Optional:
#   --output <dir>       Output directory (default: same as binary)
#   --icon <path>        Path to .icns icon file
#   --static <dir>       Static files directory to bundle
#   --config <path>      Config file to bundle
#   --version <ver>      Version string (default: "1.0.0")
#   --info-plist <path>  Custom Info.plist (instead of template)
#
# Example:
#   ./create-app-bundle.sh \
#     --binary dist/myapp \
#     --name "My App" \
#     --bundle-id com.example.myapp \
#     --icon build/macos/icon.icns \
#     --static static \
#     --version 1.2.3

set -e

# Parse arguments
BINARY=""
APP_NAME=""
BUNDLE_ID=""
OUTPUT_DIR=""
ICON_PATH=""
STATIC_DIR=""
CONFIG_PATH=""
VERSION="1.0.0"
INFO_PLIST=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --binary)
            BINARY="$2"
            shift 2
            ;;
        --name)
            APP_NAME="$2"
            shift 2
            ;;
        --bundle-id)
            BUNDLE_ID="$2"
            shift 2
            ;;
        --output)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        --icon)
            ICON_PATH="$2"
            shift 2
            ;;
        --static)
            STATIC_DIR="$2"
            shift 2
            ;;
        --config)
            CONFIG_PATH="$2"
            shift 2
            ;;
        --version)
            VERSION="$2"
            shift 2
            ;;
        --info-plist)
            INFO_PLIST="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Validate required arguments
if [ -z "$BINARY" ] || [ -z "$APP_NAME" ] || [ -z "$BUNDLE_ID" ]; then
    echo "Usage: $0 --binary <path> --name <name> --bundle-id <id> [options]"
    echo ""
    echo "Run '$0 --help' for more information."
    exit 1
fi

if [ ! -f "$BINARY" ]; then
    echo "Error: Binary not found at $BINARY"
    exit 1
fi

# Set defaults
BINARY_NAME=$(basename "$BINARY")
OUTPUT_DIR="${OUTPUT_DIR:-$(dirname "$BINARY")}"
APP_BUNDLE="$OUTPUT_DIR/${APP_NAME}.app"

echo "Creating app bundle: $APP_BUNDLE"

# Create .app bundle structure
CONTENTS_DIR="$APP_BUNDLE/Contents"
MACOS_DIR="$CONTENTS_DIR/MacOS"
RESOURCES_DIR="$CONTENTS_DIR/Resources"

rm -rf "$APP_BUNDLE"
mkdir -p "$MACOS_DIR" "$RESOURCES_DIR"

# Copy binary
cp "$BINARY" "$MACOS_DIR/$BINARY_NAME"
chmod +x "$MACOS_DIR/$BINARY_NAME"

# Create or copy Info.plist
if [ -n "$INFO_PLIST" ] && [ -f "$INFO_PLIST" ]; then
    cp "$INFO_PLIST" "$CONTENTS_DIR/Info.plist"
else
    # Generate Info.plist from values
    cat > "$CONTENTS_DIR/Info.plist" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleDevelopmentRegion</key>
    <string>en</string>
    <key>CFBundleExecutable</key>
    <string>$BINARY_NAME</string>
    <key>CFBundleIconFile</key>
    <string>icon.icns</string>
    <key>CFBundleIdentifier</key>
    <string>$BUNDLE_ID</string>
    <key>CFBundleInfoDictionaryVersion</key>
    <string>6.0</string>
    <key>CFBundleName</key>
    <string>$APP_NAME</string>
    <key>CFBundleDisplayName</key>
    <string>$APP_NAME</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>$VERSION</string>
    <key>CFBundleVersion</key>
    <string>1</string>
    <key>LSMinimumSystemVersion</key>
    <string>12.0</string>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>NSSupportsAutomaticGraphicsSwitching</key>
    <true/>
    <key>LSApplicationCategoryType</key>
    <string>public.app-category.productivity</string>
    <key>NSAppTransportSecurity</key>
    <dict>
        <key>NSAllowsLocalNetworking</key>
        <true/>
    </dict>
</dict>
</plist>
EOF
fi

# Create PkgInfo
echo "APPL????" > "$CONTENTS_DIR/PkgInfo"

# Copy icon if provided
if [ -n "$ICON_PATH" ] && [ -f "$ICON_PATH" ]; then
    cp "$ICON_PATH" "$RESOURCES_DIR/icon.icns"
fi

# Copy static files if provided
if [ -n "$STATIC_DIR" ] && [ -d "$STATIC_DIR" ]; then
    cp -r "$STATIC_DIR" "$RESOURCES_DIR/"
fi

# Copy config file if provided
if [ -n "$CONFIG_PATH" ] && [ -f "$CONFIG_PATH" ]; then
    mkdir -p "$RESOURCES_DIR/configs"
    cp "$CONFIG_PATH" "$RESOURCES_DIR/configs/"
fi

echo "Created: $APP_BUNDLE"
