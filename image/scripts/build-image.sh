#!/usr/bin/env bash
# build-image.sh — Build velocity.report RPi image
#
# Wraps pi-gen to produce a flashable .img file. ARM64 Go binaries are
# staged through scripts/stage-image-binary.sh, which builds the same
# fully static vendored-libpcap artifact used by release packaging.
#
# Prerequisites:
#   - Docker installed and running
#
# Usage:
#   ./image/scripts/build-image.sh [options]
#
# Options:
#   --skip-binaries   Reuse binaries from a previous build
#   --binaries-only   Build web/docs assets and ARM64 binaries, then stop
#   --ssh-key <path>  Install an SSH public key for the login user
#
# Tailscale: the image ships with no Tailscale payload or state. The operator
# opts in via the velocity.report web UI (Settings → Tailscale), which downloads
# a pinned and verified static payload at runtime; there is no build-time auth-key flow.

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
    rm -rf "$IMAGE_DIR/stage-velocity/03-velocity-config/files/data"
    rm -rf "$IMAGE_DIR/stage-velocity/03-velocity-config/files/public_html"
    rm -rf "$IMAGE_DIR/stage-velocity/03-velocity-config/files/config"
    rm -rf "$IMAGE_DIR/stage-velocity/00-install-packages/files"
    rm -f "$IMAGE_DIR/stage-velocity/03-velocity-config/files/velocity-report-build"
}

# ---------------------------------------------------------------------------
# 1. Parse arguments
# ---------------------------------------------------------------------------
SKIP_BINARIES=0
BINARIES_ONLY=0
SSH_KEY_PATH=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --skip-binaries) SKIP_BINARIES=1; shift ;;
        --binaries-only) BINARIES_ONLY=1; shift ;;
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

if [[ "$BINARIES_ONLY" -eq 1 && "$SKIP_BINARIES" -eq 1 ]]; then
    log_error "--binaries-only cannot be combined with --skip-binaries"
    exit 1
fi

# ---------------------------------------------------------------------------
# 2. Compute build timestamp once for the entire script
# ---------------------------------------------------------------------------
# Every timestamp in this build (Docker args, MOTD metadata, image filename)
# derives from this single date call to guarantee consistency.
BUILD_TIME="${BUILD_TIME:-$(date -u +"%Y-%m-%dT%H:%M:%SZ")}"
BUILD_TS_COMPACT="${BUILD_TIME//[-:]/}"
VERSION="${VERSION:-$(grep '^VERSION :=' "$REPO_ROOT/Makefile" | awk '{print $3}')}"
GIT_SHA="${GIT_SHA:-$(git -C "$REPO_ROOT" rev-parse HEAD 2>/dev/null || echo "unknown")}"

# ---------------------------------------------------------------------------
# 3. Check prerequisites
# ---------------------------------------------------------------------------
if ! command -v docker &>/dev/null; then
    log_error "Docker is required but not installed"
    exit 1
fi

if ! docker info &>/dev/null; then
    log_error "Docker daemon is not running — start Docker Desktop and try again"
    exit 1
fi

trap cleanup EXIT

# ---------------------------------------------------------------------------
# 4. Build ARM64 binaries
# ---------------------------------------------------------------------------
BINARIES_DIR="$IMAGE_DIR/velocity-binaries"
mkdir -p "$BINARIES_DIR"

if [[ "$SKIP_BINARIES" -eq 0 ]]; then
    VERSION="$VERSION" GIT_SHA="$GIT_SHA" BUILD_TIME="$BUILD_TIME" \
        OUT_DIR="$BINARIES_DIR" "$REPO_ROOT/scripts/stage-image-binary.sh"
    log_info "Binary staged in $BINARIES_DIR"
else
    if [[ ! -f "$BINARIES_DIR/velocity" ]]; then
        log_error "--skip-binaries requires the staged binary at $BINARIES_DIR/velocity"
        exit 1
    fi
    chmod +x "$BINARIES_DIR/velocity"
    "$REPO_ROOT/scripts/verify-static-elf.sh" "$BINARIES_DIR/velocity"
    log_info "Using pre-staged binary in $BINARIES_DIR"
fi

# Record the version string so stage 01 can name the on-disk versions/<v>/ dir
# without exec'ing the (ARM64) binary under qemu.
printf '%s\n' "$VERSION" > "$BINARIES_DIR/VERSION"
(
    cd "$BINARIES_DIR"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum velocity > SHA256
    else
        shasum -a 256 velocity > SHA256
    fi
)

if [[ "$BINARIES_ONLY" -eq 1 ]]; then
    log_info "Binary build complete; skipping image assembly (--binaries-only)"
    exit 0
fi

# Write build metadata for the login MOTD and on-device diagnostics.
# The stage script installs this to /etc/velocity-report-build.
GIT_SHA_SHORT="${GIT_SHA:0:7}"
cat > "$IMAGE_DIR/stage-velocity/03-velocity-config/files/velocity-report-build" << BUILDEOF
# velocity.report image build metadata — stamped at image creation time.
VR_VERSION="$VERSION"
VR_BUILD_TIME="$BUILD_TIME"
VR_GIT_SHA="$GIT_SHA_SHORT"
BUILDEOF
log_info "Build metadata stamped (v${VERSION}, ${GIT_SHA_SHORT})"

# ---------------------------------------------------------------------------
# 5. Clone pi-gen if not already present
# ---------------------------------------------------------------------------
# Use the bookworm-arm64 branch — master targets armhf (32-bit) with
# setarch linux32, which fails under Apple Silicon's QEMU emulation.
PIGEN_BRANCH="bookworm-arm64"
if [[ ! -d "$PIGEN_DIR" ]]; then
    log_info "Cloning pi-gen (branch: $PIGEN_BRANCH)..."
    git clone --depth 1 --branch "$PIGEN_BRANCH" \
        https://github.com/RPi-Distro/pi-gen.git "$PIGEN_DIR"
fi

# Tuning defaults now ship embedded in the velocity binary (config.TuningDefaults
# via assets.go), so there is no tuning.defaults.json to stage on disk.

# Remove legacy raw Markdown docs staging from older image builds. The offline
# docs now ship inside the velocity-report binary and are served at /docs/.
rm -rf "$IMAGE_DIR/stage-velocity/03-velocity-config/files/docs"

# Copy reference data (maths, structures, experiments) — excludes explore/
DATA_DEST="$IMAGE_DIR/stage-velocity/03-velocity-config/files/data"
rm -rf "$DATA_DEST"
mkdir -p "$DATA_DEST"
for subdir in maths structures experiments; do
    if [ -d "$REPO_ROOT/data/$subdir" ]; then
        cp -r "$REPO_ROOT/data/$subdir" "$DATA_DEST/"
    fi
done
# Research papers are not needed at runtime — exclude to avoid ~110 MB
# of academic PDFs being installed on every device.
rm -rf "$DATA_DEST/maths/papers"
for f in README.md QUESTIONS.md; do
    [ -f "$REPO_ROOT/data/$f" ] && cp "$REPO_ROOT/data/$f" "$DATA_DEST/"
done
find "$DATA_DEST" -name '.DS_Store' -delete 2>/dev/null || true
log_info "Copied data reference files"

# Copy built documentation site (Eleventy output) for on-device reference.
# Auto-build if _site/ is missing, matching the web frontend pattern above.
if [ ! -d "$REPO_ROOT/public_html/_site" ]; then
    log_info "Building documentation site..."
    if command -v pnpm &>/dev/null; then
        (cd "$REPO_ROOT/public_html" && pnpm run build)
    elif command -v npm &>/dev/null; then
        (cd "$REPO_ROOT/public_html" && npm run build)
    else
        log_error "pnpm or npm is required to build the documentation site"
        exit 1
    fi
    log_info "Documentation site built"
fi
PUBLIC_HTML_DEST="$IMAGE_DIR/stage-velocity/03-velocity-config/files/public_html"
rm -rf "$PUBLIC_HTML_DEST"
mkdir -p "$PUBLIC_HTML_DEST"
cp -r "$REPO_ROOT/public_html/_site/"* "$PUBLIC_HTML_DEST/"
find "$PUBLIC_HTML_DEST" -name '.DS_Store' -delete 2>/dev/null || true
log_info "Copied public_html site"

# ---------------------------------------------------------------------------
# 7. Copy custom stage and binaries into pi-gen
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
# Use UTC so the timestamp is unambiguous regardless of build host timezone.
echo "IMG_DATE=$(date -u +%Y-%m-%d-%H%M%S)" >> "$PIGEN_DIR/config"

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
    log_info "SSH public key staged for login user"
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
    awk '
        /\tcapsh \$CAPSH_ARG "--chroot=\$\{ROOTFS_DIR\}\/" -- -e "\$@"/ && !patched {
            print "\tcp /etc/resolv.conf \"${ROOTFS_DIR}/etc/resolv.conf\" 2>/dev/null || true"
            patched=1
        }
        { print }
    ' "$COMMON_FILE" > "$COMMON_FILE.tmp" && mv "$COMMON_FILE.tmp" "$COMMON_FILE"
    log_info "Patched on_chroot for Docker Desktop DNS compatibility"
fi

# ---------------------------------------------------------------------------
# 6c. Increase export-image root partition margin
# ---------------------------------------------------------------------------
# pi-gen sizes the exported image as rootfs*1.2 + 200 MB.  After our
# 06-cleanup stage purges dev packages, the rootfs is significantly smaller
# and the default margin leaves insufficient headroom for ext4 metadata
# overhead + the ~130 MB of uncompressed apt indices that export-image's
# 02-set-sources re-downloads.  Double the fixed component to 400 MB.
PRERUN="$PIGEN_DIR/export-image/prerun.sh"
if [ -f "$PRERUN" ]; then
    sed -i.bak 's/200 \* 1024 \* 1024/400 * 1024 * 1024/' "$PRERUN"
    rm -f "$PRERUN.bak"
    log_info "Increased export-image root partition margin to 400 MB"
fi

# ---------------------------------------------------------------------------
# 6d. Skip userconf-pi in export-image
# ---------------------------------------------------------------------------
# pi-gen's export-image/01-user-rename unconditionally installs userconf-pi,
# which depends on raspi-config → binutils, lua5.1, alsa-utils, triggerhappy,
# ssh-import-id, dos2unix, rpi-eeprom, and many Python packages (~80 MB).
# Our config sets DISABLE_FIRST_BOOT_USER_RENAME=1 so userconf-pi is unused.
# Empty the package list to prevent it re-installing everything we just purged.
USERCONF_PKG="$PIGEN_DIR/export-image/01-user-rename/00-packages"
if [ -f "$USERCONF_PKG" ]; then
    : > "$USERCONF_PKG"
    log_info "Cleared userconf-pi from export-image (DISABLE_FIRST_BOOT_USER_RENAME=1)"
fi

# ---------------------------------------------------------------------------
# 6e. Disable autoremove in export-image dist-upgrade
# ---------------------------------------------------------------------------
# pi-gen's export-image/02-set-sources runs:
#   apt-get -y dist-upgrade --auto-remove --purge
# This sweeps auto-installed packages that lost their dependents during
# our 06-cleanup stage, including runtime packages we need (e.g.
# raspberrypi-sys-mods, console-setup).  Remove the --auto-remove flag
# so dist-upgrade only upgrades without removing anything.
SETSRC="$PIGEN_DIR/export-image/02-set-sources/01-run.sh"
if [ -f "$SETSRC" ]; then
    sed -i.bak 's/--auto-remove --purge//' "$SETSRC"
    rm -f "$SETSRC.bak"
    log_info "Disabled --auto-remove in export-image dist-upgrade"
fi

# ---------------------------------------------------------------------------
# 8. Build the image
# ---------------------------------------------------------------------------
if docker inspect pigen_work &>/dev/null; then
    log_warn "Removing stale pigen_work container from previous build..."
    docker rm -fv pigen_work >/dev/null
fi

log_info "Building Raspberry Pi image with pi-gen..."
# Run in a subshell so the cd does not change the parent script's working
# directory.  When bash is invoked via a relative path (e.g.
# ./image/scripts/build-image.sh from make), it may reopen the script file
# to seek forward; if CWD has changed, the relative path resolves to the
# wrong location and bash hits EOF mid-parse, emitting a spurious
# "unexpected EOF while looking for matching '\"'" error.
(cd "$PIGEN_DIR" && ./build-docker.sh)

# ---------------------------------------------------------------------------
# 9. Locate and compress output
# ---------------------------------------------------------------------------
DEPLOY_DIR="$PIGEN_DIR/deploy"

OUTPUT_ZIP=$(find "$DEPLOY_DIR" -name "*.zip" -type f -print0 | xargs -0 ls -t 2>/dev/null | head -1)
if [[ -n "$OUTPUT_ZIP" ]]; then
    log_info "Extracting image from $(basename "$OUTPUT_ZIP")..."
    unzip -o "$OUTPUT_ZIP" -d "$DEPLOY_DIR"
fi
OUTPUT_IMG=$(find "$DEPLOY_DIR" -name "*.img" -type f -print0 | xargs -0 ls -t 2>/dev/null | head -1)

if [[ -n "$OUTPUT_IMG" ]]; then
    # Rename to match the asset naming convention before compressing.
    # Dev:     {datetime}-velocity-report-{devversion}-{sha7}.img.xz
    # The VERSION and GIT_SHA_SHORT variables were set earlier in the script.
    # BUILD_TS_COMPACT was computed once at the top of the script.
    IMG_DEV_VERSION="${VERSION//-/.}"
    IMG_GIT_SHA_SHORT=$(git -C "$REPO_ROOT" rev-parse --short=7 HEAD 2>/dev/null || echo "unknown")
    NAMED_IMG="$DEPLOY_DIR/${BUILD_TS_COMPACT}-velocity-report-${IMG_DEV_VERSION}-${IMG_GIT_SHA_SHORT}.img"
    mv "$OUTPUT_IMG" "$NAMED_IMG"

    log_info "Compressing image with xz..."
    xz -9 --keep --force "$NAMED_IMG"
    COMPRESSED="${NAMED_IMG}.xz"
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
