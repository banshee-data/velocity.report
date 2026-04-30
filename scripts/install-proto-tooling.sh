#!/usr/bin/env bash
# install-proto-tooling.sh — Install pinned protobuf + gRPC toolchain.
#
# Usage:
#   ./scripts/install-proto-tooling.sh          # install all tools
#   ./scripts/install-proto-tooling.sh --check  # verify installed, exit 1 if missing
#
# macOS: installs protoc + swift-protobuf via Homebrew, builds
# protoc-gen-grpc-swift-2 from source at a pinned tag, and installs Go
# plugins via `go install`.
#
# Linux: installs protoc + Go plugins only (no Swift tooling).
#
# All versions are pinned here — change them in one place.

set -euo pipefail

# ---------------------------------------------------------------------------
# Version pins — update here and nowhere else
# ---------------------------------------------------------------------------
GRPC_SWIFT_PROTOBUF_VERSION="2.0.0"          # git tag in grpc/grpc-swift-protobuf
PROTOC_GEN_GO_VERSION="v1.36.11"             # matches google.golang.org/protobuf in go.mod
PROTOC_GEN_GO_GRPC_VERSION="v1.5.1"          # latest stable grpc protoc plugin

# Binary cache path (used by CI caching and local installs)
GRPC_SWIFT_BIN="${GRPC_SWIFT_CACHE_PATH:-/usr/local/bin}/protoc-gen-grpc-swift-2"
BUILD_DIR="${GRPC_SWIFT_BUILD_DIR:-/tmp/grpc-swift-protobuf-build}"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
info()  { printf '  %s\n' "$*"; }
ok()    { printf '✓ %s\n' "$*"; }
die()   { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

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
MISSING_COUNT=0
for arg in "$@"; do
    case "$arg" in
        --check) CHECK_MODE=1 ;;
        *) die "Unknown argument: $arg" ;;
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

# Go plugins (cross-platform)
if ! command -v go >/dev/null 2>&1; then
    die "go not found — install Go first"
fi

info "Installing Go proto plugins..."
GOBIN="${GOBIN:-$(go env GOPATH)/bin}"
GOBIN="$GOBIN" go install "google.golang.org/protobuf/cmd/protoc-gen-go@${PROTOC_GEN_GO_VERSION}"
GOBIN="$GOBIN" go install "google.golang.org/grpc/cmd/protoc-gen-go-grpc@${PROTOC_GEN_GO_GRPC_VERSION}"
ok "protoc-gen-go ${PROTOC_GEN_GO_VERSION}"
ok "protoc-gen-go-grpc ${PROTOC_GEN_GO_GRPC_VERSION}"

if [ "$OS" = "Darwin" ]; then
    # protoc + swift-protobuf via Homebrew
    if ! command -v brew >/dev/null 2>&1; then
        die "Homebrew not found — install from https://brew.sh"
    fi
    info "Installing protoc + swift-protobuf via Homebrew..."
    brew install protobuf swift-protobuf
    ok "protoc $(protoc --version 2>/dev/null | head -1)"
    ok "protoc-gen-swift (swift-protobuf)"

    # protoc-gen-grpc-swift-2: build from source at pinned tag
    if command -v protoc-gen-grpc-swift-2 >/dev/null 2>&1; then
        ok "protoc-gen-grpc-swift-2 already installed — skipping build"
    else
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
    # Linux: protoc via apt if not present
    if ! command -v protoc >/dev/null 2>&1; then
        info "Installing protoc via apt..."
        sudo apt-get update -qq
        sudo apt-get install -y --no-install-recommends protobuf-compiler
    fi
    ok "protoc $(protoc --version 2>/dev/null | head -1)"
    info "Swift tools not available on Linux — skipping"
fi

ok "Proto toolchain ready"
