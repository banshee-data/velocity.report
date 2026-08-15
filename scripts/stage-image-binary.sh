#!/usr/bin/env bash
# stage-image-binary.sh — canonical ARM64 image binary build/staging path.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
IMAGE_DIR="$REPO_ROOT/image"
OUT_DIR="${OUT_DIR:-"$IMAGE_DIR/velocity-binaries"}"

VERSION="${VERSION:-$(grep '^VERSION :=' "$REPO_ROOT/Makefile" | awk '{print $3}')}"
GIT_SHA="${GIT_SHA:-$(git -C "$REPO_ROOT" rev-parse HEAD 2>/dev/null || echo "unknown")}"
BUILD_TIME="${BUILD_TIME:-$(date -u +"%Y-%m-%dT%H:%M:%SZ")}"

mkdir -p "$OUT_DIR"

echo "==> Building embedded static assets"
make -C "$REPO_ROOT" VERSION="$VERSION" BUILD_TIME="$BUILD_TIME" build-embedded-assets
if [[ ! -f "$REPO_ROOT/docs_html/_site/index.html" ]]; then
    echo "error: embedded offline docs build did not produce docs_html/_site/index.html" >&2
    exit 1
fi
rm -f "$OUT_DIR"/velocity "$OUT_DIR"/velocity-report-*-linux-arm64-*-static

VERSION="$VERSION" GIT_SHA="$GIT_SHA" BUILD_TIME="$BUILD_TIME" \
    ARCHES=arm64 OUT_DIR="$OUT_DIR" \
    "$REPO_ROOT/scripts/build-radar-static.sh"

built="$(find "$OUT_DIR" -maxdepth 1 -type f \
    -name 'velocity-report-*-linux-arm64-*-static' -print | head -n1)"
if [[ -z "$built" ]]; then
    echo "error: static ARM64 build did not produce an artifact in $OUT_DIR" >&2
    exit 1
fi

mv "$built" "$OUT_DIR/velocity"
chmod 0755 "$OUT_DIR/velocity"
"$REPO_ROOT/scripts/verify-static-elf.sh" "$OUT_DIR/velocity"

printf '%s\n' "$VERSION" > "$OUT_DIR/VERSION"
(
    cd "$OUT_DIR"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum velocity > SHA256
    else
        shasum -a 256 velocity > SHA256
    fi
)

echo "==> Staged static ARM64 image binary in $OUT_DIR"
