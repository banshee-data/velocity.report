#!/usr/bin/env bash
# smoke-test-static.sh — run a static velocity-report binary's --self-check
# inside a clean Debian container that has no extra tools installed.
#
# This is the runtime-verification half of the static build story: it
# proves the binary actually functions (DNS, UDP, libpcap) on a stock
# target, not just that it links.
#
# Usage:
#   scripts/smoke-test-static.sh <path-to-static-binary>
#
# The arch is auto-detected from the ELF header. Arm64 binaries on an
# amd64 host run under qemu — requires either Docker Desktop (built-in)
# or qemu-user-static + binfmt_misc registration on Linux hosts.

set -euo pipefail

if [[ $# -ne 1 ]]; then
    echo "Usage: $0 <binary>" >&2
    exit 2
fi

BIN="$1"
if [[ ! -f "$BIN" ]]; then
    echo "error: $BIN does not exist" >&2
    exit 2
fi

ABS_BIN="$(cd "$(dirname "$BIN")" && pwd)/$(basename "$BIN")"

case "$(file -b "$ABS_BIN")" in
    *"x86-64"*)      arch=amd64; platform=linux/amd64 ;;
    *"ARM aarch64"*) arch=arm64; platform=linux/arm64 ;;
    *) echo "error: unrecognised architecture: $(file -b "$ABS_BIN")" >&2; exit 2 ;;
esac

# debian:bookworm-slim has no compiler, no libpcap, no nothing — exactly
# the "fresh target" we want. If the static binary works here, it works
# anywhere with a Linux kernel and a working DNS resolver path to the
# host's resolv.conf.
IMAGE="debian:bookworm-slim"

echo "==> smoke-testing $BIN ($arch) in $IMAGE ($platform)"

self_check_args=(-self-check)
host_arch="$(uname -m)"
case "$host_arch/$arch" in
    x86_64/amd64|amd64/amd64|aarch64/arm64|arm64/arm64)
        self_check_args+=(-self-check-live-capture=lo)
        ;;
    *)
        echo "    live capture deferred: qemu-user does not emulate SIOCETHTOOL; run the ARM64 system-VM gate"
        ;;
esac

# --network=host is intentional: the DNS-external check needs access to
# the host's resolv.conf for the warning-vs-failure to be meaningful.
# A read-only root filesystem catches undeclared runtime writes. The embedded
# Typst extractor gets only an explicit ephemeral cache.
docker run --rm \
    --platform "$platform" \
    --network host \
    --read-only \
    --tmpfs /tmp:rw,nosuid,nodev,exec \
    -e XDG_CACHE_HOME=/tmp/cache \
    -v "$ABS_BIN:/velocity-report:ro" \
    "$IMAGE" \
    /velocity-report "${self_check_args[@]}"
