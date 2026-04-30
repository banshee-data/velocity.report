#!/usr/bin/env bash
# generate-protos.sh — Canonical protobuf generation script.
#
# Usage:
#   ./scripts/generate-protos.sh go       # Generate Go stubs
#   ./scripts/generate-protos.sh swift    # Generate Swift stubs
#   ./scripts/generate-protos.sh all      # Generate both
#
# All paths are relative to the repository root. Run from any directory.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

PROTO_DIR="$REPO_ROOT/proto/velocity_visualiser/v1"
PROTO_FILE="$PROTO_DIR/visualiser.proto"
PROTO_GO_OUT="$REPO_ROOT/internal/lidar/l9endpoints/pb"
PROTO_SWIFT_OUT="$REPO_ROOT/tools/visualiser-macos/VelocityVisualiser/gRPC/Generated"

die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "$1 not found — run ./scripts/install-proto-tooling.sh"; }

generate_go() {
    need protoc
    need protoc-gen-go
    need protoc-gen-go-grpc
    mkdir -p "$PROTO_GO_OUT"
    printf 'Generating Go stubs → %s\n' "$PROTO_GO_OUT"
    protoc \
        --go_out="$PROTO_GO_OUT" \
        --go_opt=paths=source_relative \
        --go-grpc_out="$PROTO_GO_OUT" \
        --go-grpc_opt=paths=source_relative \
        -I "$PROTO_DIR" \
        "$PROTO_FILE"
    printf '✓ Go stubs generated\n'
}

generate_swift() {
    need protoc
    need protoc-gen-swift
    need protoc-gen-grpc-swift-2
    mkdir -p "$PROTO_SWIFT_OUT"
    printf 'Generating Swift stubs → %s\n' "$PROTO_SWIFT_OUT"
    protoc \
        --swift_out="$PROTO_SWIFT_OUT" \
        --plugin=protoc-gen-grpc-swift="$(command -v protoc-gen-grpc-swift-2)" \
        --grpc-swift_out="$PROTO_SWIFT_OUT" \
        -I "$PROTO_DIR" \
        "$PROTO_FILE"
    printf '✓ Swift stubs generated\n'
}

case "${1:-all}" in
    go)    generate_go   ;;
    swift) generate_swift ;;
    all)   generate_go && generate_swift ;;
    *) die "Unknown target '$1'. Use: go | swift | all" ;;
esac
