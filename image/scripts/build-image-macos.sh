#!/usr/bin/env bash
# build-image.sh — Local image build helper for velocity.report RPi images
#
# Wraps pi-gen to produce a flashable .img file. Designed for local
# development and testing; CI uses the GitHub Actions workflow directly.
#
# Prerequisites:
#   - Docker installed and running
#   - Go toolchain (for cross-compiling ARM64 binaries)
#
# On macOS, pcap cross-compilation requires an ARM64 Linux cross-compiler
# (aarch64-linux-gnu-gcc). Without it, the script falls back to a non-pcap
# build — which is fine for testing the image pipeline.
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
cleanup() {
    log_info "Cleaning up transient build files..."
    rm -rf "$PIGEN_DIR/stage-velocity"
    rm -rf "$PIGEN_DIR/velocity-binaries"
}
PIGEN_DIR="$IMAGE_DIR/.pi-gen"

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
# 2. Prepare binary artifacts
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
    log_info "Building ARM64 Go binaries..."
    cd "$REPO_ROOT"

    # Try pcap build first (needs aarch64-linux-gnu-gcc); fall back to non-pcap
    if make build-radar-linux-pcap 2>/dev/null; then
        log_info "Built velocity-report with pcap support"
    else
        log_warn "pcap cross-compile unavailable; building without pcap"
        make build-radar-linux
    fi
    make build-ctl-linux

    # Copy binaries to staging area (Makefile outputs *-linux-arm64)
    cp -f "$REPO_ROOT/velocity-report-linux-arm64" "$BINARIES_DIR/velocity-report"
    cp -f "$REPO_ROOT/velocity-ctl-linux-arm64" "$BINARIES_DIR/velocity-ctl"
    chmod +x "$BINARIES_DIR"/*
fi

# ---------------------------------------------------------------------------
# 3. Clone pi-gen if not already present
# ---------------------------------------------------------------------------
# Use the bookworm-arm64 branch — master targets armhf (32-bit) with
# setarch linux32, which fails under Apple Silicon's QEMU emulation.
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
# These must be populated BEFORE we copy stage-velocity into pi-gen (step 5)
# so the copies include the PDF generator and config files.
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
# We copy rather than symlink because pi-gen's build-docker.sh sends the
# pi-gen directory as a Docker build context — symlinks pointing outside
# the context are not followed and the files would be missing in the image.
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
# pi-gen's on_chroot() mounts a fresh tmpfs over ${ROOTFS_DIR}/run.
# On Bookworm, /etc/resolv.conf inside the debootstrapped rootfs may be
# a symlink into /run/systemd/resolve/, so mounting tmpfs over /run
# destroys the resolver config. This causes apt inside the chroot to
# fail resolving archive.raspberrypi.com (or any non-cached host).
#
# Fix: insert a line in on_chroot() that copies the container's
# resolv.conf into the rootfs before entering the chroot.
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
# Remove any stale pi-gen container from a previous failed build
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

    # Generate SHA-256 checksum (shasum on macOS, sha256sum on Linux)
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
