#!/usr/bin/env bash
# Build static assets embedded into velocity-report binaries.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

VERSION="${VERSION:-$(make -s -C "$REPO_ROOT" print-version)}"
BUILD_TIME="${BUILD_TIME:-$(date -u +"%Y-%m-%dT%H:%M:%SZ")}"

make -C "$REPO_ROOT" VERSION="$VERSION" BUILD_TIME="$BUILD_TIME" build-web
make -C "$REPO_ROOT" install-docs-offline
make -C "$REPO_ROOT" VERSION="$VERSION" BUILD_TIME="$BUILD_TIME" build-docs-offline
