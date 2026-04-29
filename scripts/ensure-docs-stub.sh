#!/bin/bash
# Copy a stub docs_html/_site/index.html if the offline docs have not been built.

set -euo pipefail

DOCS_BUILD_DIR="docs_html/_site"
STUB_FILE="$DOCS_BUILD_DIR/index.html"
STUB_SOURCE="docs_html/stub-index.html"
EMBED_MARKER="$DOCS_BUILD_DIR/.embed-stub"
EMBED_MARKER_TEXT="This placeholder keeps Go's docs_html/_site embed pattern valid on clean checkouts."

mkdir -p "$DOCS_BUILD_DIR"
printf '%s\n' "$EMBED_MARKER_TEXT" > "$EMBED_MARKER"

if [ -f "$STUB_FILE" ]; then
    exit 0
fi

if [ ! -f "$STUB_SOURCE" ]; then
    echo "Error: Stub source file $STUB_SOURCE not found" >&2
    exit 1
fi

cp "$STUB_SOURCE" "$STUB_FILE"
echo "Generated docs stub file: $STUB_FILE"
