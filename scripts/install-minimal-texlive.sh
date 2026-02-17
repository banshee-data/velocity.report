#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

SOURCE_DIR="${SOURCE_DIR:-${REPO_ROOT}/build/texlive-minimal}"
DEST_DIR="${DEST_DIR:-/opt/velocity-report/texlive-minimal}"

if [[ "${SOURCE_DIR}" != /* ]]; then
  SOURCE_DIR="${REPO_ROOT}/${SOURCE_DIR}"
fi

log() {
  printf '[install-minimal-texlive] %s\n' "$*"
}

die() {
  printf '[install-minimal-texlive] ERROR: %s\n' "$*" >&2
  exit 1
}

[[ -d "${SOURCE_DIR}" ]] || die "Source directory not found: ${SOURCE_DIR}"
[[ -x "${SOURCE_DIR}/bin/xelatex" ]] || die "Missing xelatex binary under ${SOURCE_DIR}/bin"

parent_dir="$(dirname "${DEST_DIR}")"
sudo_cmd=()
if [[ ! -w "${parent_dir}" ]]; then
  if command -v sudo >/dev/null 2>&1; then
    sudo_cmd=(sudo)
  else
    die "Write access to ${parent_dir} is required (install sudo or choose writable DEST_DIR)."
  fi
fi

run_cmd() {
  if [[ "${#sudo_cmd[@]}" -gt 0 ]]; then
    "${sudo_cmd[@]}" "$@"
  else
    "$@"
  fi
}

log "Installing ${SOURCE_DIR} -> ${DEST_DIR}"
run_cmd mkdir -p "${parent_dir}"
run_cmd rm -rf "${DEST_DIR}"
run_cmd mkdir -p "${DEST_DIR}"

if command -v rsync >/dev/null 2>&1; then
  run_cmd rsync -a --delete "${SOURCE_DIR}/" "${DEST_DIR}/"
else
  run_cmd cp -a "${SOURCE_DIR}/." "${DEST_DIR}/"
fi

run_cmd chmod -R a+rX "${DEST_DIR}"
log "Installed minimal TeX tree at ${DEST_DIR}"
