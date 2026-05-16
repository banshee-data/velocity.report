#!/usr/bin/env bash
# Build a fully-static libpcap.a for linux/amd64 and linux/arm64 using `zig cc`.
# Source: third_party/libpcap (git submodule).
# Output layout:
#   third_party/libpcap-static/out/<arch>/lib/libpcap.a
#   third_party/libpcap-static/out/<arch>/include/pcap.h  (and friends)
set -euo pipefail

HERE=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
SRC=$(cd -- "$HERE/../libpcap" 2>/dev/null && pwd || true)
if [ -z "$SRC" ] || [ ! -f "$SRC/pcap.h" ]; then
    echo "error: libpcap source missing at $HERE/../libpcap" >&2
    echo "       run: git submodule update --init third_party/libpcap" >&2
    exit 2
fi

# Need zig on PATH for the wrapper scripts to find it.
command -v zig >/dev/null || { echo "zig not found on PATH"; exit 1; }
command -v cmake >/dev/null || { echo "cmake not found on PATH"; exit 1; }

build_one() {
    local arch=$1 builddir="$HERE/build-$1" prefix="$HERE/out/$1"
    rm -rf "$builddir" "$prefix"
    mkdir -p "$builddir" "$prefix"

    cmake -S "$SRC" -B "$builddir" \
        -DCMAKE_TOOLCHAIN_FILE="$HERE/toolchains/linux-$arch.cmake" \
        -DCMAKE_INSTALL_PREFIX="$prefix" \
        -DCMAKE_BUILD_TYPE=Release \
        -DBUILD_SHARED_LIBS=OFF \
        -DENABLE_REMOTE=OFF \
        -DDISABLE_BLUETOOTH=ON \
        -DDISABLE_DBUS=ON \
        -DDISABLE_RDMA=ON \
        -DDISABLE_NETMAP=ON \
        -DDISABLE_DAG=ON \
        -DDISABLE_SNF=ON \
        -DDISABLE_LINUX_USBMON=ON \
        -DBUILD_WITH_LIBNL=OFF

    cmake --build "$builddir" --parallel "$(nproc)" --target pcap_static

    install -D -m 644 "$builddir/libpcap.a" "$prefix/lib/libpcap.a"
    mkdir -p "$prefix/include/pcap"
    install -m 644 "$SRC/pcap.h" "$SRC/pcap-bpf.h" "$SRC/pcap-namedb.h" "$prefix/include/"
    install -m 644 "$SRC"/pcap/*.h "$prefix/include/pcap/"
}

archs=(${1:-amd64 arm64})
for a in "${archs[@]}"; do
    echo
    echo "==> Building libpcap.a for linux/$a"
    build_one "$a"
done

echo
echo "==> Done."
for a in "${archs[@]}"; do
    f="$HERE/out/$a/lib/libpcap.a"
    [ -f "$f" ] && { printf '  %s  ' "$f"; file "$f"; } \
                || echo "  MISSING: $f"
done
