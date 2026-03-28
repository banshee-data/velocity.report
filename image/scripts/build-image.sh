#!/usr/bin/env bash
# build-image.sh — Build velocity.report RPi image with pcap-enabled binaries
#
# Compiles ARM64 Go binaries inside a Docker container (so libpcap
# cross-compilation works on any host OS), then runs pi-gen to produce
# a flashable .img file.
#
# Prerequisites:
#   - Docker installed and running
#
# Usage:
#   ./image/scripts/build-image.sh [--skip-binaries]
#
# The --skip-binaries flag assumes binaries are already in place from a
# previous build; useful when iterating on pi-gen stage scripts.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
IMAGE_DIR="$REPO_ROOT/image"

# Colour codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}✓${NC} $1"; }
log_warn()  { echo -e "${YELLOW}⚠${NC} $1"; }
log_error() { echo -e "${RED}✗${NC} $1"; }

# ---------------------------------------------------------------------------
# 0. Cleanup handler — remove transient copies on exit
# ---------------------------------------------------------------------------
PIGEN_DIR="$IMAGE_DIR/.pi-gen"

cleanup() {
    log_info "Cleaning up transient build files..."
    rm -rf "$PIGEN_DIR/stage-velocity"
    rm -rf "$PIGEN_DIR/velocity-binaries"
}

# ---------------------------------------------------------------------------
# 1. Check prerequisites
# ---------------------------------------------------------------------------
if ! command -v docker &>/dev/null; then
    log_error "Docker is required but not installed"
    exit 1
fi

if ! docker info &>/dev/null; then
    log_error "Docker daemon is not running — start Docker Desktop and try again"
    exit 1
fi

# ---------------------------------------------------------------------------
# 2. Build ARM64 binaries inside Docker (with pcap support)
# ---------------------------------------------------------------------------
SKIP_BINARIES=0
for arg in "$@"; do
    case "$arg" in
        --skip-binaries) SKIP_BINARIES=1 ;;
        *) log_error "Unknown argument: $arg"; exit 1 ;;
    esac
done

BINARIES_DIR="$IMAGE_DIR/velocity-binaries"
mkdir -p "$BINARIES_DIR"

if [[ "$SKIP_BINARIES" -eq 0 ]]; then
    log_info "Building ARM64 Go binaries with pcap support (in Docker)..."
    cd "$REPO_ROOT"

    # Version metadata — same as the Makefile
    VERSION=$(grep '^VERSION :=' Makefile | awk '{print $3}')
    GIT_SHA=$(git rev-parse HEAD 2>/dev/null || echo "unknown")
    BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

    docker build \
        --platform linux/amd64 \
        -f "$IMAGE_DIR/Dockerfile.build" \
        --build-arg VERSION="$VERSION" \
        --build-arg GIT_SHA="$GIT_SHA" \
        --build-arg BUILD_TIME="$BUILD_TIME" \
        -t velocity-builder \
        .

    # Extract binaries from the built image
    CONTAINER_ID=$(docker create velocity-builder)
    docker cp "$CONTAINER_ID:/out/velocity-report" "$BINARIES_DIR/velocity-report"
    docker cp "$CONTAINER_ID:/out/velocity-ctl" "$BINARIES_DIR/velocity-ctl"
    docker rm "$CONTAINER_ID" >/dev/null
    chmod +x "$BINARIES_DIR"/*

    log_info "Built velocity-report and velocity-ctl with pcap support"
fi

# ---------------------------------------------------------------------------
# 3. Clone pi-gen if not already present
# ---------------------------------------------------------------------------
PIGEN_BRANCH="bookworm-arm64"
if [[ ! -d "$PIGEN_DIR" ]]; then
    log_info "Cloning pi-gen (branch: $PIGEN_BRANCH)..."
    git clone --depth 1 --branch "$PIGEN_BRANCH" \
        https://github.com/RPi-Distro/pi-gen.git "$PIGEN_DIR"
fi

# Register cleanup trap now that PIGEN_DIR is resolved
trap cleanup EXIT

# ---------------------------------------------------------------------------
# 4. Copy PDF generator and config into stage directory
# ---------------------------------------------------------------------------
PDF_DEST="$IMAGE_DIR/stage-velocity/02-velocity-python/files/pdf-generator"
mkdir -p "$PDF_DEST"
cp -r "$REPO_ROOT/tools/pdf-generator/"* "$PDF_DEST/"
log_info "Copied PDF generator source"

CONFIG_DEST="$IMAGE_DIR/stage-velocity/03-velocity-config/files/config"
mkdir -p "$CONFIG_DEST"
cp "$REPO_ROOT/config/tuning.defaults.json" "$CONFIG_DEST/"
log_info "Copied tuning defaults"

# ---------------------------------------------------------------------------
# 5. Copy custom stage and binaries into pi-gen
# ---------------------------------------------------------------------------
rm -rf "$PIGEN_DIR/stage-velocity"
cp -r "$IMAGE_DIR/stage-velocity" "$PIGEN_DIR/stage-velocity"
cp -f "$IMAGE_DIR/config" "$PIGEN_DIR/config"

# Create SKIP files for stages 3–5
for stage in stage3 stage4 stage5; do
    mkdir -p "$PIGEN_DIR/$stage"
    touch "$PIGEN_DIR/$stage/SKIP"
done

# Skip images for intermediate stages
touch "$PIGEN_DIR/stage2/SKIP_IMAGES"

# Copy binary artifacts into pi-gen root
rm -rf "$PIGEN_DIR/velocity-binaries"
cp -r "$BINARIES_DIR" "$PIGEN_DIR/velocity-binaries"

# ---------------------------------------------------------------------------
# 5b. Patch pi-gen for Docker Desktop DNS compatibility
# ---------------------------------------------------------------------------
COMMON_FILE="$PIGEN_DIR/scripts/common"
if ! grep -q "resolv.conf" "$COMMON_FILE" 2>/dev/null; then
    python3 - "$COMMON_FILE" << 'PYEOF'
import sys
path = sys.argv[1]
with open(path) as f:
    content = f.read()
target = '\tcapsh $CAPSH_ARG "--chroot=${ROOTFS_DIR}/" -- -e "$@"'
fix = '\tcp /etc/resolv.conf "${ROOTFS_DIR}/etc/resolv.conf" 2>/dev/null || true\n'
content = content.replace(target, fix + target, 1)
with open(path, 'w') as f:
    f.write(content)
PYEOF
    log_info "Patched on_chroot for Docker Desktop DNS compatibility"
fi

# ---------------------------------------------------------------------------
# 6. Build the image
# ---------------------------------------------------------------------------
if docker inspect pigen_work &>/dev/null; then
    log_warn "Removing stale pigen_work container from previous build..."
    docker rm -v pigen_work >/dev/null
fi

log_info "Building Raspberry Pi image with pi-gen..."
cd "$PIGEN_DIR"
./build-docker.sh

# ---------------------------------------------------------------------------
# 7. Locate and compress output
# ---------------------------------------------------------------------------
DEPLOY_DIR="$PIGEN_DIR/deploy"

# Always extract from the newest zip to ensure we compress the latest build
OUTPUT_ZIP=$(find "$DEPLOY_DIR" -name "*.zip" -type f -print0 | xargs -0 ls -t 2>/dev/null | head -1)
if [[ -n "$OUTPUT_ZIP" ]]; then
    log_info "Extracting image from $(basename "$OUTPUT_ZIP")..."
    unzip -o "$OUTPUT_ZIP" -d "$DEPLOY_DIR"
fi
OUTPUT_IMG=$(find "$DEPLOY_DIR" -name "*.img" -type f -print0 | xargs -0 ls -t 2>/dev/null | head -1)

if [[ -n "$OUTPUT_IMG" ]]; then
    log_info "Compressing image with xz..."
    xz -9 --keep --force "$OUTPUT_IMG"
    COMPRESSED="${OUTPUT_IMG}.xz"
    log_info "Image ready: $COMPRESSED"
    log_info "Size: $(du -h "$COMPRESSED" | cut -f1)"

    if command -v sha256sum &>/dev/null; then
        sha256sum "$COMPRESSED" > "${COMPRESSED}.sha256"
    else
        shasum -a 256 "$COMPRESSED" > "${COMPRESSED}.sha256"
    fi
    log_info "Checksum: ${COMPRESSED}.sha256"
else
    log_error "No .img file found in pi-gen deploy directory"
    log_error "Contents of deploy/:"
    ls -la "$DEPLOY_DIR" 2>&1 || true
    exit 1
fi
