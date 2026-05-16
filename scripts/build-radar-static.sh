#!/usr/bin/env bash
# build-radar-static.sh — produce fully statically-linked velocity-report
# binaries for linux/{amd64,arm64} inside a hermetic Docker image.
#
# The image pins every input that affects the output bytes: base image
# digest, Go and zig tarball SHA-256s, libpcap submodule pointer. The
# host only needs docker and git.
#
# The image always builds both arches in one go (so the Docker layer
# cache stays warm across arch-specific invocations). The host script
# extracts a configurable subset.
#
# Inputs (env):
#   ARCHES          arches to extract (still builds both inside);
#                   default "amd64 arm64"
#   OUT_DIR         where to write binaries; default: ${REPO_ROOT}/build/static
#   IMAGE_TAG       Docker image tag; default: velocity-report-static-build
#   ALLOW_DIRTY     1 to permit building from a dirty tree (binary will
#                   stamp "<sha>-dirty"); default: refuse
#   VERSION/GIT_SHA/BUILD_TIME  override version stamping
#     (defaults: VERSION from Makefile, GIT_SHA from `git rev-parse HEAD`,
#      BUILD_TIME from the HEAD commit's committer date in UTC ISO 8601 —
#      identical source produces identical binaries).
#
# Dependencies: docker (with BuildKit), git.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

EXTRACT_ARCHES=${ARCHES:-"amd64 arm64"}
OUT_DIR=${OUT_DIR:-"$REPO_ROOT/build/static"}
IMAGE_TAG=${IMAGE_TAG:-"velocity-report-static-build"}
ALLOW_DIRTY=${ALLOW_DIRTY:-0}

command -v docker >/dev/null || { echo "error: docker not found on PATH" >&2; exit 1; }
command -v git    >/dev/null || { echo "error: git not found on PATH"    >&2; exit 1; }

# Ensure the libpcap submodule is checked out before Docker copies the tree in.
if [[ ! -f "$REPO_ROOT/third_party/libpcap/pcap.h" ]]; then
    echo "==> Initialising third_party/libpcap submodule"
    git -C "$REPO_ROOT" submodule update --init --recursive third_party/libpcap
fi

# Dirty-tree gate. Reproducibility requires that the stamped SHA matches
# the actual source. If you're iterating on the build itself, set
# ALLOW_DIRTY=1 to accept a "<sha>-dirty" stamp.
DIRTY=0
if ! git -C "$REPO_ROOT" diff --quiet HEAD -- 2>/dev/null \
        || ! git -C "$REPO_ROOT" diff --quiet --cached HEAD -- 2>/dev/null; then
    DIRTY=1
fi
if [[ "$DIRTY" == "1" && "$ALLOW_DIRTY" != "1" ]]; then
    echo "error: working tree is dirty; refusing to stamp HEAD timestamp into binary." >&2
    echo "       commit your changes, or set ALLOW_DIRTY=1 to stamp '<sha>-dirty'." >&2
    exit 3
fi

# Version metadata. BUILD_TIME defaults to the HEAD commit's committer
# date in UTC, so the same source produces the same binary regardless of
# when you build.
VERSION=${VERSION:-$(awk -F':= ' '/^VERSION[[:space:]]*:=/ {print $2; exit}' Makefile | tr -d ' ')}
GIT_SHA=${GIT_SHA:-$(git -C "$REPO_ROOT" rev-parse HEAD 2>/dev/null || echo unknown)}
if [[ "$DIRTY" == "1" ]]; then
    GIT_SHA="${GIT_SHA}-dirty"
fi
GIT_SHA_SHORT="${GIT_SHA:0:7}"
if [[ -z "${BUILD_TIME:-}" ]]; then
    BUILD_TIME=$(TZ=UTC git -C "$REPO_ROOT" log -1 --format='%cd' --date='format:%Y-%m-%dT%H:%M:%SZ' HEAD 2>/dev/null || echo "unknown")
fi
DEV_VERSION="${VERSION//-/.}"

# Parse Go version from go.mod so the Dockerfile pin and the module
# directive can't drift. The `go.mod` directive is the source of truth.
GO_VERSION=$(awk '/^go [0-9]/{print $2; exit}' go.mod)
if [[ -z "$GO_VERSION" ]]; then
    echo "error: failed to parse Go version from go.mod" >&2
    exit 1
fi

# Pre-build embed stubs on the host so they get copied into the image.
"$REPO_ROOT/scripts/ensure-web-stub.sh"
"$REPO_ROOT/scripts/ensure-docs-stub.sh"

echo "==> Building Docker image: $IMAGE_TAG"
echo "    VERSION:    $VERSION"
echo "    GIT_SHA:    $GIT_SHA_SHORT$([ "$DIRTY" = "1" ] && echo "  (DIRTY)")"
echo "    BUILD_TIME: $BUILD_TIME  (HEAD commit timestamp)"
echo "    GO_VERSION: $GO_VERSION  (from go.mod)"
echo "    EXTRACT:    $EXTRACT_ARCHES"

# Build the image, exporting binaries directly to OUT_DIR via BuildKit's
# local output. The image always builds both arches inside (so the layer
# cache stays warm across single-arch invocations); we filter on extract.
mkdir -p "$OUT_DIR"
TMP_OUT="$(mktemp -d -t velocity-static.XXXXXX)"
trap 'rm -rf "$TMP_OUT"' EXIT

DOCKER_BUILDKIT=1 docker build \
    --file image/Dockerfile.static-build \
    --target export \
    --output "type=local,dest=$TMP_OUT" \
    --build-arg GO_VERSION="$GO_VERSION" \
    --build-arg VERSION="$VERSION" \
    --build-arg DEV_VERSION="$DEV_VERSION" \
    --build-arg GIT_SHA="$GIT_SHA" \
    --build-arg GIT_SHA_SHORT="$GIT_SHA_SHORT" \
    --build-arg BUILD_TIME="$BUILD_TIME" \
    --build-arg ARCHES="amd64 arm64" \
    --tag "$IMAGE_TAG" \
    .

# Move the requested arches into OUT_DIR; ignore the rest.
for arch in $EXTRACT_ARCHES; do
    src="$TMP_OUT/velocity-report-${DEV_VERSION}-linux-${arch}-${GIT_SHA_SHORT}-static"
    if [[ ! -f "$src" ]]; then
        echo "error: expected binary not produced: $src" >&2
        exit 1
    fi
    dst="$OUT_DIR/$(basename "$src")"
    mv -f "$src" "$dst"
    file "$dst"
done

echo
echo "==> Done. Binaries in $OUT_DIR/"
