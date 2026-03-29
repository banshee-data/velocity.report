#!/usr/bin/env bash
# build-image.sh — Build velocity.report RPi image
#
# Wraps pi-gen to produce a flashable .img file. By default, ARM64 Go
# binaries are cross-compiled inside a Docker container (pcap-enabled).
# Pass --host-build to use the host Go toolchain instead (faster
# iteration; falls back to non-pcap if the cross-compiler is absent).
#
# Prerequisites:
#   - Docker installed and running
#   - Go toolchain (only when --host-build is used)
#
# Usage:
#   ./image/scripts/build-image.sh [options]
#
# Options:
#   --skip-binaries   Reuse binaries from a previous build
#   --host-build      Build binaries with the host Go toolchain (no Docker compile)
#   --ssh-key <path>  Install an SSH public key for the velocity user

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
    # Remove transient copies staged from the repo into the working tree
    rm -rf "$IMAGE_DIR/stage-velocity/03-velocity-config/files/docs"
    rm -rf "$IMAGE_DIR/stage-velocity/02-velocity-python/files/pdf-generator"
}

# ---------------------------------------------------------------------------
# 1. Parse arguments
# ---------------------------------------------------------------------------
SKIP_BINARIES=0
HOST_BUILD=0
SSH_KEY_PATH=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --skip-binaries) SKIP_BINARIES=1; shift ;;
        --host-build)    HOST_BUILD=1; shift ;;
        --ssh-key)
            if [[ -z "${2:-}" ]]; then
                log_error "--ssh-key requires a path to a public key file"
                exit 1
            fi
            SSH_KEY_PATH="$2"; shift 2 ;;
        *) log_error "Unknown argument: $1"; exit 1 ;;
    esac
done

if [[ -n "$SSH_KEY_PATH" && ! -f "$SSH_KEY_PATH" ]]; then
    log_error "SSH public key not found: $SSH_KEY_PATH"
    exit 1
fi

# ---------------------------------------------------------------------------
# 2. Check prerequisites
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
# 3. Build ARM64 binaries
# ---------------------------------------------------------------------------
BINARIES_DIR="$IMAGE_DIR/velocity-binaries"
mkdir -p "$BINARIES_DIR"

if [[ "$SKIP_BINARIES" -eq 0 ]]; then
    cd "$REPO_ROOT"

    if [[ "$HOST_BUILD" -eq 1 ]]; then
        # Host toolchain — fast path for local iteration.
        # Needs aarch64-linux-gnu-gcc for pcap; falls back to non-pcap.
        log_info "Building ARM64 Go binaries (host toolchain)..."
        if make build-radar-linux-pcap 2>/dev/null; then
            log_info "Built velocity-report with pcap support"
        else
            log_warn "pcap cross-compile unavailable; building without pcap"
            make build-radar-linux
        fi
        make build-ctl-linux

        cp -f "$REPO_ROOT/velocity-report-linux-arm64" "$BINARIES_DIR/velocity-report"
        cp -f "$REPO_ROOT/velocity-ctl-linux-arm64" "$BINARIES_DIR/velocity-ctl"
    else
        # Docker build — canonical path, always produces pcap-enabled binaries.
        log_info "Building ARM64 Go binaries with pcap support (in Docker)..."

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

        CONTAINER_ID=$(docker create velocity-builder)
        docker cp "$CONTAINER_ID:/out/velocity-report" "$BINARIES_DIR/velocity-report"
        docker cp "$CONTAINER_ID:/out/velocity-ctl" "$BINARIES_DIR/velocity-ctl"
        docker rm "$CONTAINER_ID" >/dev/null
    fi

    chmod +x "$BINARIES_DIR"/*
    log_info "Binaries staged in $BINARIES_DIR"
fi

# ---------------------------------------------------------------------------
# 4. Clone pi-gen if not already present
# ---------------------------------------------------------------------------
# Use the bookworm-arm64 branch — master targets armhf (32-bit) with
# setarch linux32, which fails under Apple Silicon's QEMU emulation.
PIGEN_BRANCH="bookworm-arm64"
if [[ ! -d "$PIGEN_DIR" ]]; then
    log_info "Cloning pi-gen (branch: $PIGEN_BRANCH)..."
    git clone --depth 1 --branch "$PIGEN_BRANCH" \
        https://github.com/RPi-Distro/pi-gen.git "$PIGEN_DIR"
fi

trap cleanup EXIT

# ---------------------------------------------------------------------------
# 5. Copy PDF generator and config into stage directory
# ---------------------------------------------------------------------------
# These must be populated BEFORE we copy stage-velocity into pi-gen (step 6)
# so the copies include the PDF generator and config files.
PDF_DEST="$IMAGE_DIR/stage-velocity/02-velocity-python/files/pdf-generator"
mkdir -p "$PDF_DEST"
cp -r "$REPO_ROOT/tools/pdf-generator/"* "$PDF_DEST/"
log_info "Copied PDF generator source"

CONFIG_DEST="$IMAGE_DIR/stage-velocity/03-velocity-config/files/config"
mkdir -p "$CONFIG_DEST"
cp "$REPO_ROOT/config/tuning.defaults.json" "$CONFIG_DEST/"
log_info "Copied tuning defaults"

# Copy project documentation into the stage for installation to /opt
DOCS_DEST="$IMAGE_DIR/stage-velocity/03-velocity-config/files/docs"
rm -rf "$DOCS_DEST"
cp -r "$REPO_ROOT/docs" "$DOCS_DEST"
# Remove macOS metadata and other non-documentation artefacts
find "$DOCS_DEST" -name '.DS_Store' -delete 2>/dev/null || true
log_info "Copied docs"

# ---------------------------------------------------------------------------
# 6. Copy custom stage and binaries into pi-gen
# ---------------------------------------------------------------------------
# We copy rather than symlink because pi-gen's build-docker.sh sends the
# pi-gen directory as a Docker build context — symlinks pointing outside
# the context are not followed and the files would be missing in the image.
rm -rf "$PIGEN_DIR/stage-velocity"
cp -r "$IMAGE_DIR/stage-velocity" "$PIGEN_DIR/stage-velocity"
cp -f "$IMAGE_DIR/config" "$PIGEN_DIR/config"

# Include time with seconds in the image filename to avoid collisions
# when rebuilding on the same day. pi-gen sources this config inside Docker
# so the variable must be in the file, not just the host environment.
echo "IMG_DATE=$(date +%Y-%m-%d-%H%M%S)" >> "$PIGEN_DIR/config"

for stage in stage3 stage4 stage5; do
    mkdir -p "$PIGEN_DIR/$stage"
    touch "$PIGEN_DIR/$stage/SKIP"
done
touch "$PIGEN_DIR/stage2/SKIP_IMAGES"

rm -rf "$PIGEN_DIR/velocity-binaries"
cp -r "$BINARIES_DIR" "$PIGEN_DIR/velocity-binaries"

# ---------------------------------------------------------------------------
# 6a. Install SSH public key for velocity user (optional)
# ---------------------------------------------------------------------------
if [[ -n "$SSH_KEY_PATH" ]]; then
    SSH_DEST="$PIGEN_DIR/stage-velocity/03-velocity-config/files/authorized_keys"
    cp "$SSH_KEY_PATH" "$SSH_DEST"
    log_info "SSH public key staged for velocity user"
fi

# ---------------------------------------------------------------------------
# 6b. Patch pi-gen for Docker Desktop DNS compatibility
# ---------------------------------------------------------------------------
# pi-gen's on_chroot() mounts a fresh tmpfs over ${ROOTFS_DIR}/run.
# On Bookworm, /etc/resolv.conf inside the rootfs may be a symlink into
# /run/systemd/resolve/, so mounting tmpfs over /run destroys the resolver
# config. Fix: copy the container's resolv.conf before entering the chroot.
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
# 7. Build the image
# ---------------------------------------------------------------------------
if docker inspect pigen_work &>/dev/null; then
    log_warn "Removing stale pigen_work container from previous build..."
    docker rm -v pigen_work >/dev/null
fi

log_info "Building Raspberry Pi image with pi-gen..."
cd "$PIGEN_DIR"
./build-docker.sh

# ---------------------------------------------------------------------------
# 8. Locate and compress output
# ---------------------------------------------------------------------------
DEPLOY_DIR="$PIGEN_DIR/deploy"

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
