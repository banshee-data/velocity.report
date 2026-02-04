#!/bin/bash
# Generate compile_commands.json for SourceKit LSP
# This helps VS Code IntelliSense understand the project structure

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$SCRIPT_DIR"

echo "Generating compile_commands.json for VelocityVisualiser..."

# Build the project to generate build logs
cd "$PROJECT_DIR"
xcodebuild clean build \
    -project VelocityVisualiser.xcodeproj \
    -scheme VelocityVisualiser \
    -configuration Debug \
    -derivedDataPath build \
    | tee build/xcodebuild.log

# Extract compilation database
if command -v xcpretty >/dev/null 2>&1; then
    cat build/xcodebuild.log | xcpretty -r json-compilation-database --output compile_commands.json
else
    echo "Note: xcpretty not installed. Install with: gem install xcpretty"
    echo "SourceKit will use Xcode's build settings directly."
fi

echo "✓ Build complete. Restart VS Code to reload SourceKit."
