#!/usr/bin/env bash
# Download a typst release binary for a given Go target and place it at DEST.
#
# Usage: download-typst.sh VERSION GOOS GOARCH DEST
#
# Used by the Makefile (install-typst / install-typst-dist / the build targets)
# and the Pi image build to fetch the platform binary that gets embedded into
# the velocity binary via `-tags typst_embed`.
#
# Downloads are cached per (version, target) under $TYPST_CACHE_DIR (default
# ~/.cache/velocity-typst), so repeated and cross-target builds reuse the
# fetched binary and work offline after the first fetch. CI can cache that dir.
set -euo pipefail

VERSION="${1:?usage: download-typst.sh VERSION GOOS GOARCH DEST}"
GOOS="${2:?missing GOOS}"
GOARCH="${3:?missing GOARCH}"
DEST="${4:?missing DEST}"

# Map Go GOOS/GOARCH to the Rust target triple used in typst release asset names.
case "$GOOS/$GOARCH" in
  linux/arm64)   TARGET=aarch64-unknown-linux-musl ;;
  linux/amd64)   TARGET=x86_64-unknown-linux-musl ;;
  darwin/arm64)  TARGET=aarch64-apple-darwin ;;
  darwin/amd64)  TARGET=x86_64-apple-darwin ;;
  # TODO(windows): re-enable once installer-style targets consistently write
  # typst.exe for PATH/PATHEXT discovery instead of accepting an extensionless
  # DEST.
  *) echo "download-typst: unsupported target $GOOS/$GOARCH" >&2; exit 1 ;;
esac

cache_dir="${TYPST_CACHE_DIR:-$HOME/.cache/velocity-typst}"
cached="$cache_dir/typst-${VERSION}-${TARGET}"

if [ ! -x "$cached" ]; then
  mkdir -p "$cache_dir"
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  base="https://github.com/typst/typst/releases/download/v${VERSION}"
  curl -fsSL -o "$tmp/typst.tar.xz" "${base}/typst-${TARGET}.tar.xz"
  tar -xJf "$tmp/typst.tar.xz" -C "$tmp"
  install -m 0755 "$tmp/typst-${TARGET}/typst" "$cached"
  echo "download-typst: cached $cached (typst ${VERSION}, ${TARGET})"
fi

mkdir -p "$(dirname "$DEST")"
install -m 0755 "$cached" "$DEST"
echo "download-typst: wrote $DEST (typst ${VERSION}, ${TARGET})"
