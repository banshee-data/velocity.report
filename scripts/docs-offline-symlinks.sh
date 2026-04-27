#!/bin/bash
# Create or remove the symlinked source tree used by docs_html.

set -euo pipefail

MODE="${1:-create}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SRC_DIR="$REPO_ROOT/docs_html/src"

ensure_replaceable() {
    local path="$1"
    if [ -e "$path" ] && [ ! -L "$path" ]; then
        echo "Error: $path exists and is not a symlink; refusing to replace it" >&2
        exit 1
    fi
}

clean_links() {
    for name in docs data; do
        local path="$SRC_DIR/$name"
        if [ -L "$path" ]; then
            rm -f "$path"
        fi
    done

    find "$SRC_DIR" -maxdepth 1 -type l -name '*.md' -exec rm -f {} +
}

case "$MODE" in
    create)
        mkdir -p "$SRC_DIR"
        for name in docs data; do
            ensure_replaceable "$SRC_DIR/$name"
            ln -snf "../../$name" "$SRC_DIR/$name"
        done

        for file in "$REPO_ROOT"/*.md; do
            [ -f "$file" ] || continue
            name="$(basename "$file")"
            ensure_replaceable "$SRC_DIR/$name"
            ln -snf "../../$name" "$SRC_DIR/$name"
        done
        ;;
    clean)
        clean_links
        ;;
    *)
        echo "Usage: $0 [create|clean]" >&2
        exit 2
        ;;
esac
