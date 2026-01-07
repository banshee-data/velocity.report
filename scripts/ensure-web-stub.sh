#!/bin/bash
# ensure-web-stub.sh - Copy stub web/build/index.html if it doesn't exist
# This allows Go embed to work on fresh clones without tracking generated files

set -e

WEB_BUILD_DIR="web/build"
STUB_FILE="$WEB_BUILD_DIR/index.html"
STUB_SOURCE="web/stub-index.html"

# If index.html already exists (from actual build or previous stub), do nothing
if [ -f "$STUB_FILE" ]; then
    exit 0
fi

# Create directory if it doesn't exist
mkdir -p "$WEB_BUILD_DIR"

# Copy stub file from source
if [ ! -f "$STUB_SOURCE" ]; then
    echo "Error: Stub source file $STUB_SOURCE not found" >&2
    exit 1
fi

cp "$STUB_SOURCE" "$STUB_FILE"
echo "Generated stub file: $STUB_FILE"
