#!/usr/bin/env bash
# install-proto-tooling.sh — Install pinned protobuf + gRPC toolchain.
#
# Usage:
#   ./scripts/install-proto-tooling.sh           # install all tools
#   ./scripts/install-proto-tooling.sh --check   # verify installed, exit 1 if missing
#   ./scripts/install-proto-tooling.sh --force   # reinstall even if already present
#
# macOS: installs protoc + swift-protobuf via Homebrew (latest available; no
# Homebrew API for pinning formula versions), builds protoc-gen-grpc-swift-2
# from source at a pinned git tag, and installs Go plugins via `go install`
# at pinned module versions.
#
# Linux: installs protoc via apt (system package; version varies by distro)
# and Go plugins only (no Swift tooling).
#
# Pinned here: GRPC_SWIFT_PROTOBUF_VERSION, PROTOC_GEN_GO_VERSION,
#              PROTOC_GEN_GO_GRPC_VERSION.
# Not pinned:  protoc (brew/apt), swift-protobuf (brew).

set -euo pipefail

# ---------------------------------------------------------------------------
# Version pins — update here and nowhere else
# ---------------------------------------------------------------------------
GRPC_SWIFT_PROTOBUF_VERSION="2.1.2"    # git tag in grpc/grpc-swift-protobuf
PROTOC_GEN_GO_VERSION="v1.36.11"       # matches google.golang.org/protobuf in go.mod
PROTOC_GEN_GO_GRPC_VERSION="v1.5.1"    # latest stable grpc protoc plugin

BUILD_DIR="${GRPC_SWIFT_BUILD_DIR:-/tmp/grpc-swift-protobuf-build}"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
info()  { printf '  %s\n' "$*"; }
ok()    { printf '✓ %s\n' "$*"; }
die()   { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
need()  { command -v "$1" >/dev/null 2>&1 || die "$1 not found — $2"; }

require() {
    local cmd="$1"
    if ! command -v "$cmd" >/dev/null 2>&1; then
        if [ "${CHECK_MODE:-0}" = "1" ]; then
            printf 'MISSING: %s\n' "$cmd" >&2
            MISSING_COUNT=$((MISSING_COUNT + 1))
        else
            return 1
        fi
    fi
}

# ---------------------------------------------------------------------------
# Parse args
# ---------------------------------------------------------------------------
CHECK_MODE=0
FORCE_MODE=0
MISSING_COUNT=0
for arg in "$@"; do
    case "$arg" in
        --check) CHECK_MODE=1 ;;
        --force) FORCE_MODE=1 ;;
        *) die "Unknown argument: $arg. Use --check or --force." ;;
    esac
done

OS="$(uname -s)"

# ---------------------------------------------------------------------------
# Check mode — verify tools exist, exit non-zero if any are missing
# ---------------------------------------------------------------------------
if [ "$CHECK_MODE" = "1" ]; then
    require protoc
    require protoc-gen-go
    require protoc-gen-go-grpc
    if [ "$OS" = "Darwin" ]; then
        require protoc-gen-swift
        require protoc-gen-grpc-swift-2
    fi
    if [ "$MISSING_COUNT" -gt 0 ]; then
        printf '\nRun ./scripts/install-proto-tooling.sh to install missing tools.\n' >&2
        exit 1
    fi
    ok "All proto tools present"
    exit 0
fi

# ---------------------------------------------------------------------------
# Install mode
# ---------------------------------------------------------------------------

# Go plugins (cross-platform, pinned module versions)
need go "install Go from https://go.dev/dl/"

info "Installing Go proto plugins..."
GOBIN="${GOBIN:-$(go env GOPATH)/bin}"
GOBIN="$GOBIN" go install "google.golang.org/protobuf/cmd/protoc-gen-go@${PROTOC_GEN_GO_VERSION}"
GOBIN="$GOBIN" go install "google.golang.org/grpc/cmd/protoc-gen-go-grpc@${PROTOC_GEN_GO_GRPC_VERSION}"
ok "protoc-gen-go ${PROTOC_GEN_GO_VERSION}"
ok "protoc-gen-go-grpc ${PROTOC_GEN_GO_GRPC_VERSION}"

if [ "$OS" = "Darwin" ]; then
    need brew "install Homebrew from https://brew.sh"

    # On ARM macOS runners the shell may be launched under Rosetta (x86_64),
    # which conflicts with Homebrew's native ARM prefix (/opt/homebrew).
    # Force native ARM execution for brew to avoid the
    # "Cannot install under Rosetta 2 in ARM default prefix" error.
    BREW="brew"
    if [ "$(uname -m)" = "arm64" ]; then
        BREW="arch -arm64 brew"
    fi

    # protoc + swift-protobuf via Homebrew (latest; no formula-version pinning)
    info "Installing protoc + swift-protobuf via Homebrew..."
    $BREW install protobuf swift-protobuf
    ok "protoc ($(protoc --version 2>/dev/null | head -1), Homebrew latest)"
    ok "protoc-gen-swift (swift-protobuf, Homebrew latest)"

    # protoc-gen-grpc-swift-2: build from source at pinned tag
    ALREADY_INSTALLED=0
    if command -v protoc-gen-grpc-swift-2 >/dev/null 2>&1; then
        ALREADY_INSTALLED=1
    fi

    if [ "$ALREADY_INSTALLED" = "1" ] && [ "$FORCE_MODE" = "0" ]; then
        ok "protoc-gen-grpc-swift-2 already installed (run --force to reinstall at ${GRPC_SWIFT_PROTOBUF_VERSION})"
    else
        need git "install Xcode Command Line Tools: xcode-select --install"
        need swift "install Xcode from the App Store"
        info "Building protoc-gen-grpc-swift-2 @ ${GRPC_SWIFT_PROTOBUF_VERSION}..."
        rm -rf "$BUILD_DIR"
        git clone --depth 1 --branch "$GRPC_SWIFT_PROTOBUF_VERSION" \
            https://github.com/grpc/grpc-swift-protobuf.git "$BUILD_DIR"
        (cd "$BUILD_DIR" && swift build -c release --product protoc-gen-grpc-swift-2)
        sudo install -m 755 \
            "$BUILD_DIR/.build/release/protoc-gen-grpc-swift-2" \
            /usr/local/bin/protoc-gen-grpc-swift-2
        ok "protoc-gen-grpc-swift-2 ${GRPC_SWIFT_PROTOBUF_VERSION}"
    fi
else
    # Linux: protoc via apt (system package, version varies by distro)
    if ! command -v protoc >/dev/null 2>&1; then
        info "Installing protoc via apt..."
        sudo apt-get update -qq
        sudo apt-get install -y --no-install-recommends protobuf-compiler
    fi
    ok "protoc ($(protoc --version 2>/dev/null | head -1), system package)"
    info "Swift tools not available on Linux — skipping"
fi

ok "Proto toolchain ready"
