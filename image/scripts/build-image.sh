#!/usr/bin/env bash
# build-image.sh — Local image build helper for velocity.report RPi images
#
# Wraps pi-gen to produce a flashable .img file. Designed for local
# development and testing; CI uses the GitHub Actions workflow directly.
#
# Prerequisites:
#   - Docker installed and running
#   - ARM64 Go binaries pre-built (make build-radar-linux-pcap build-ctl-linux)
#   - Python PDF generator source available
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
# 1. Check prerequisites
# ---------------------------------------------------------------------------
if ! command -v docker &>/dev/null; then
    log_error "Docker is required but not installed"
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
    GOOS=linux GOARCH=arm64 CGO_ENABLED=1 make build-radar-linux-pcap 2>/dev/null || \
        make build-radar-linux
    GOOS=linux GOARCH=arm64 make build-ctl-linux 2>/dev/null || true

    # Copy binaries to staging area
    cp -f "$REPO_ROOT/velocity-report-linux" "$BINARIES_DIR/velocity-report" 2>/dev/null || \
        log_warn "velocity-report binary not found; supply manually"
    cp -f "$REPO_ROOT/velocity-ctl-linux-arm64" "$BINARIES_DIR/velocity-ctl" 2>/dev/null || \
        log_warn "velocity-ctl binary not found; supply manually"
fi

# ---------------------------------------------------------------------------
# 3. Clone pi-gen if not already present
# ---------------------------------------------------------------------------
PIGEN_DIR="$IMAGE_DIR/.pi-gen"
if [[ ! -d "$PIGEN_DIR" ]]; then
    log_info "Cloning pi-gen..."
    git clone --depth 1 https://github.com/RPi-Distro/pi-gen.git "$PIGEN_DIR"
fi

# ---------------------------------------------------------------------------
# 4. Link custom stage into pi-gen
# ---------------------------------------------------------------------------
ln -sfn "$IMAGE_DIR/stage-velocity" "$PIGEN_DIR/stage-velocity"
cp -f "$IMAGE_DIR/config" "$PIGEN_DIR/config"

# Create SKIP files for stages 3–5
for stage in stage3 stage4 stage5; do
    mkdir -p "$PIGEN_DIR/$stage"
    touch "$PIGEN_DIR/$stage/SKIP"
done

# Skip images for intermediate stages
touch "$PIGEN_DIR/stage2/SKIP_IMAGES"

# Make binary artifacts available to pi-gen
ln -sfn "$BINARIES_DIR" "$PIGEN_DIR/velocity-binaries"

# ---------------------------------------------------------------------------
# 5. Build the image
# ---------------------------------------------------------------------------
log_info "Building Raspberry Pi image with pi-gen..."
cd "$PIGEN_DIR"
./build-docker.sh

# ---------------------------------------------------------------------------
# 6. Compress output
# ---------------------------------------------------------------------------
OUTPUT_IMG=$(find "$PIGEN_DIR/deploy" -name "*.img" | head -1)
if [[ -n "$OUTPUT_IMG" ]]; then
    log_info "Compressing image with xz..."
    xz -9 --keep "$OUTPUT_IMG"
    COMPRESSED="${OUTPUT_IMG}.xz"
    log_info "Image ready: $COMPRESSED"
    log_info "Size: $(du -h "$COMPRESSED" | cut -f1)"

    # Generate SHA-256 checksum
    sha256sum "$COMPRESSED" > "${COMPRESSED}.sha256"
    log_info "Checksum: ${COMPRESSED}.sha256"
else
    log_error "No .img file found in pi-gen deploy directory"
    exit 1
fi
