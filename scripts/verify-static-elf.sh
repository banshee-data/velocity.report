#!/usr/bin/env bash
# verify-static-elf.sh — reject Linux binaries with dynamic dependencies.

set -euo pipefail

if [[ $# -ne 1 ]]; then
    echo "Usage: $0 <linux-elf-binary>" >&2
    exit 2
fi

BIN="$1"
if [[ ! -f "$BIN" ]]; then
    echo "error: binary not found: $BIN" >&2
    exit 2
fi

file_output="$(file "$BIN")"
echo "$file_output"

if [[ "$file_output" != *"ELF"* ]]; then
    echo "error: expected an ELF binary: $BIN" >&2
    exit 1
fi

if [[ "$file_output" != *"statically linked"* ]]; then
    echo "error: expected a statically linked binary: $BIN" >&2
    exit 1
fi

readelf_cmd=""
if command -v readelf >/dev/null 2>&1; then
    readelf_cmd="readelf"
elif command -v llvm-readelf >/dev/null 2>&1; then
    readelf_cmd="llvm-readelf"
fi

if [[ -n "$readelf_cmd" ]]; then
    dynamic_section="$("$readelf_cmd" -d "$BIN" 2>&1 || true)"
    if grep -q "(NEEDED)" <<<"$dynamic_section"; then
        echo "error: unexpected dynamic dependencies in $BIN" >&2
        printf '%s\n' "$dynamic_section" >&2
        exit 1
    fi
elif command -v strings >/dev/null 2>&1 && strings "$BIN" | grep -q "libpcap\\.so"; then
    echo "error: binary contains a libpcap.so reference: $BIN" >&2
    exit 1
else
    echo "warning: readelf not found; verified static linkage with file(1) only" >&2
fi
