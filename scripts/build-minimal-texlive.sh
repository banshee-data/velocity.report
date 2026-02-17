#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

MANIFEST="${MANIFEST:-${REPO_ROOT}/tools/pdf-generator/tex/dependency-manifest.txt}"
INI_FILE="${INI_FILE:-${REPO_ROOT}/tools/pdf-generator/tex/velocity-report.ini}"
OUTPUT_DIR="${OUTPUT_DIR:-${REPO_ROOT}/build/texlive-minimal}"
TEXLIVE_ROOT="${TEXLIVE_ROOT:-}"
XELATEX_BIN="${XELATEX_BIN:-}"
BUILD_FMT="${BUILD_FMT:-1}"
FMT_ONLY="${FMT_ONLY:-0}"
COPY_SHARED_LIBS="${COPY_SHARED_LIBS:-1}"

if [[ "${OUTPUT_DIR}" != /* ]]; then
  OUTPUT_DIR="${REPO_ROOT}/${OUTPUT_DIR}"
fi

log() {
  printf '[build-minimal-texlive] %s\n' "$*"
}

die() {
  printf '[build-minimal-texlive] ERROR: %s\n' "$*" >&2
  exit 1
}

require_command() {
  local cmd="$1"
  command -v "${cmd}" >/dev/null 2>&1 || die "Required command missing: ${cmd}"
}

resolve_texlive_root() {
  if [[ "${FMT_ONLY}" == "1" && -d "${OUTPUT_DIR}/texmf-dist" ]]; then
    TEXLIVE_ROOT="${OUTPUT_DIR}"
    return
  fi

  if [[ -n "${TEXLIVE_ROOT}" ]]; then
    TEXLIVE_ROOT="$(cd "${TEXLIVE_ROOT}" && pwd)"
    return
  fi

  require_command kpsewhich
  local texmf_dist
  texmf_dist="$(kpsewhich -var-value=TEXMFDIST 2>/dev/null || true)"
  [[ -n "${texmf_dist}" ]] || die "Unable to resolve TEXMFDIST via kpsewhich."
  [[ -d "${texmf_dist}" ]] || die "Resolved TEXMFDIST does not exist: ${texmf_dist}"
  TEXLIVE_ROOT="$(cd "${texmf_dist}/.." && pwd)"
}

resolve_xelatex_bin() {
  if [[ "${FMT_ONLY}" == "1" && -x "${OUTPUT_DIR}/bin/xelatex" ]]; then
    XELATEX_BIN="${OUTPUT_DIR}/bin/xelatex"
    return
  fi

  if [[ -n "${XELATEX_BIN}" ]]; then
    XELATEX_BIN="$(cd "$(dirname "${XELATEX_BIN}")" && pwd)/$(basename "${XELATEX_BIN}")"
    return
  fi

  if command -v xelatex >/dev/null 2>&1; then
    XELATEX_BIN="$(command -v xelatex)"
    return
  fi

  if [[ -x "${TEXLIVE_ROOT}/bin/xelatex" ]]; then
    XELATEX_BIN="${TEXLIVE_ROOT}/bin/xelatex"
    return
  fi

  die "Could not locate xelatex binary."
}

prepare_output_tree() {
  [[ "${FMT_ONLY}" == "1" ]] && return

  [[ -n "${OUTPUT_DIR}" ]] || die "OUTPUT_DIR must not be empty."
  [[ "${OUTPUT_DIR}" != "/" ]] || die "Refusing to operate on '/'."

  rm -rf "${OUTPUT_DIR}"
  mkdir -p "${OUTPUT_DIR}/bin"
  mkdir -p "${OUTPUT_DIR}/texmf-dist"
  mkdir -p "${OUTPUT_DIR}/texmf"
  mkdir -p "${OUTPUT_DIR}/texmf-var"
  mkdir -p "${OUTPUT_DIR}/lib"
}

copy_manifest_files() {
  [[ "${FMT_ONLY}" == "1" ]] && return

  [[ -f "${MANIFEST}" ]] || die "Manifest not found: ${MANIFEST}"
  local copied=0
  local missing=0

  while IFS= read -r rel_path || [[ -n "${rel_path}" ]]; do
    rel_path="${rel_path#"${rel_path%%[![:space:]]*}"}"
    rel_path="${rel_path%"${rel_path##*[![:space:]]}"}"
    [[ -z "${rel_path}" ]] && continue
    [[ "${rel_path}" == \#* ]] && continue

    local src="${TEXLIVE_ROOT}/${rel_path}"
    local dst="${OUTPUT_DIR}/${rel_path}"
    if [[ -f "${src}" ]]; then
      mkdir -p "$(dirname "${dst}")"
      cp -a "${src}" "${dst}"
      copied=$((copied + 1))
    elif [[ -d "${src}" ]]; then
      mkdir -p "${dst}"
      cp -a "${src}/." "${dst}/"
      copied=$((copied + 1))
    else
      log "warning: manifest entry not found: ${src}"
      missing=$((missing + 1))
    fi
  done < "${MANIFEST}"

  [[ "${copied}" -gt 0 ]] || die "No TeX files copied from manifest."
  log "copied ${copied} manifest files (${missing} missing)"
}

copy_xelatex_binary() {
  [[ "${FMT_ONLY}" == "1" ]] && return

  [[ -x "${XELATEX_BIN}" ]] || die "xelatex binary not executable: ${XELATEX_BIN}"
  # Dereference symlinks so the staged binary is self-contained.
  cp -aL "${XELATEX_BIN}" "${OUTPUT_DIR}/bin/xelatex"
  chmod +x "${OUTPUT_DIR}/bin/xelatex"
  log "copied xelatex binary to ${OUTPUT_DIR}/bin/xelatex"

  local helper
  for helper in xdvipdfmx; do
    if command -v "${helper}" >/dev/null 2>&1; then
      cp -aL "$(command -v "${helper}")" "${OUTPUT_DIR}/bin/${helper}"
      chmod +x "${OUTPUT_DIR}/bin/${helper}"
      log "copied helper binary to ${OUTPUT_DIR}/bin/${helper}"
    else
      log "warning: helper binary not found on PATH: ${helper}"
    fi
  done

  if [[ "${COPY_SHARED_LIBS}" != "1" ]]; then
    return
  fi

  if ! command -v ldd >/dev/null 2>&1; then
    log "ldd not available; skipping shared library copy"
    return
  fi

  local dep
  while IFS= read -r dep; do
    [[ -f "${dep}" ]] || continue
    cp -a "${dep}" "${OUTPUT_DIR}/lib/" || true
  done < <(ldd "${XELATEX_BIN}" | awk '/=> \// {print $3} /^\/[^ ]+/ {print $1}')

  log "copied shared libraries to ${OUTPUT_DIR}/lib (best-effort)"
}

write_texmf_cnf() {
  mkdir -p "${OUTPUT_DIR}/texmf-dist/web2c"
  local source_cnf="${TEXLIVE_ROOT}/texmf-dist/web2c/texmf.cnf"
  if [[ -f "${source_cnf}" ]]; then
    cp -a "${source_cnf}" "${OUTPUT_DIR}/texmf-dist/web2c/texmf.cnf"
  else
    : > "${OUTPUT_DIR}/texmf-dist/web2c/texmf.cnf"
  fi

  cat >> "${OUTPUT_DIR}/texmf-dist/web2c/texmf.cnf" <<EOF

% --- velocity.report minimal tree overrides ---
TEXMFROOT = ${OUTPUT_DIR}
TEXMFDIST = ${OUTPUT_DIR}/texmf-dist
TEXMFHOME = ${OUTPUT_DIR}/texmf
TEXMFVAR = ${OUTPUT_DIR}/texmf-var

% Runtime memory settings required by the precompiled format.
pool_size = 6250000
max_strings = 600000
font_mem_size = 8000000
main_memory = 10000000
extra_mem_bot = 2000000
extra_mem_top = 2000000
hash_extra = 600000
EOF
  log "wrote ${OUTPUT_DIR}/texmf-dist/web2c/texmf.cnf"
}

build_ls_r() {
  if ! command -v mktexlsr >/dev/null 2>&1; then
    log "mktexlsr not available; skipping ls-R index"
    return
  fi
  mktexlsr "${OUTPUT_DIR}/texmf-dist" >/dev/null 2>&1 || true
  log "generated ls-R index"
}

build_format() {
  [[ "${BUILD_FMT}" == "1" ]] || {
    log "BUILD_FMT=${BUILD_FMT}; skipping .fmt compilation"
    return
  }

  [[ -x "${OUTPUT_DIR}/bin/xelatex" ]] || die "Expected compiler at ${OUTPUT_DIR}/bin/xelatex"
  [[ -f "${INI_FILE}" ]] || die "Format source not found: ${INI_FILE}"
  local xelatex_ini="${OUTPUT_DIR}/texmf-dist/tex/latex/tex-ini-files/xelatex.ini"
  [[ -f "${xelatex_ini}" ]] || die "Base XeLaTeX ini file not found: ${xelatex_ini}"

  local fmt_out_dir="${OUTPUT_DIR}/texmf-dist/web2c/xelatex"
  mkdir -p "${fmt_out_dir}"

  local build_dir
  build_dir="$(mktemp -d)"
  cp "${INI_FILE}" "${build_dir}/velocity-report.ini"
  local ini_abs="${build_dir}/velocity-report.ini"
  local xelatex_log="${build_dir}/xelatex-fmt.log"
  local velocity_fmt_log="${build_dir}/velocity-report-fmt.log"

  if ! (
    cd "${build_dir}"
    export PATH="${OUTPUT_DIR}/bin:${PATH}"
    export TEXMFHOME="${OUTPUT_DIR}/texmf"
    export TEXMFDIST="${OUTPUT_DIR}/texmf-dist"
    export TEXMFVAR="${OUTPUT_DIR}/texmf-var"
    export TEXINPUTS="${OUTPUT_DIR}/texmf-dist/tex//:"
    export TFMFONTS="${OUTPUT_DIR}/texmf-dist/fonts/tfm//:"
    "${OUTPUT_DIR}/bin/xelatex" \
      -cnf-line=pool_size=6250000 \
      -cnf-line=max_strings=600000 \
      -cnf-line=font_mem_size=8000000 \
      -cnf-line=main_memory=10000000 \
      -cnf-line=extra_mem_bot=2000000 \
      -cnf-line=extra_mem_top=2000000 \
      -cnf-line=hash_extra=600000 \
      -etex \
      -ini \
      -interaction=nonstopmode \
      -halt-on-error \
      -jobname=xelatex \
      "${xelatex_ini}" >"${xelatex_log}" 2>&1
  ); then
    log "base xelatex.fmt build failed; log: ${xelatex_log}"
    tail -n 120 "${xelatex_log}" || true
    die "Failed to build xelatex.fmt (working dir preserved: ${build_dir})"
  fi

  [[ -f "${build_dir}/xelatex.fmt" ]] || die "xelatex did not emit xelatex.fmt"
  cp -a "${build_dir}/xelatex.fmt" "${fmt_out_dir}/xelatex.fmt"

  if ! (
    cd "${build_dir}"
    export PATH="${OUTPUT_DIR}/bin:${PATH}"
    export TEXMFHOME="${OUTPUT_DIR}/texmf"
    export TEXMFDIST="${OUTPUT_DIR}/texmf-dist"
    export TEXMFVAR="${OUTPUT_DIR}/texmf-var"
    export TEXINPUTS="${OUTPUT_DIR}/texmf-dist/tex//:"
    export TFMFONTS="${OUTPUT_DIR}/texmf-dist/fonts/tfm//:"
    export TEXFORMATS="${fmt_out_dir}:"
    "${OUTPUT_DIR}/bin/xelatex" \
      -cnf-line=pool_size=6250000 \
      -cnf-line=max_strings=600000 \
      -cnf-line=font_mem_size=8000000 \
      -cnf-line=main_memory=10000000 \
      -cnf-line=extra_mem_bot=2000000 \
      -cnf-line=extra_mem_top=2000000 \
      -cnf-line=hash_extra=600000 \
      -ini \
      -interaction=nonstopmode \
      -halt-on-error \
      -jobname=velocity-report \
      "&xelatex ${ini_abs}" >"${velocity_fmt_log}" 2>&1
  ); then
    log "velocity-report.fmt build failed; log: ${velocity_fmt_log}"
    tail -n 120 "${velocity_fmt_log}" || true
    die "Failed to build velocity-report.fmt (working dir preserved: ${build_dir})"
  fi

  [[ -f "${build_dir}/velocity-report.fmt" ]] || die "xelatex did not emit velocity-report.fmt"
  cp -a "${build_dir}/velocity-report.fmt" "${fmt_out_dir}/velocity-report.fmt"
  rm -rf "${build_dir}"
  log "wrote ${fmt_out_dir}/xelatex.fmt"
  log "wrote ${fmt_out_dir}/velocity-report.fmt"
}

main() {
  resolve_texlive_root
  resolve_xelatex_bin
  prepare_output_tree

  log "texlive root: ${TEXLIVE_ROOT}"
  log "xelatex bin: ${XELATEX_BIN}"
  log "output dir: ${OUTPUT_DIR}"
  log "fmt only mode: ${FMT_ONLY}"

  copy_manifest_files
  copy_xelatex_binary
  write_texmf_cnf
  build_ls_r
  build_format

  local size_mb
  size_mb="$(du -sm "${OUTPUT_DIR}" | awk '{print $1}')"
  log "completed; output size: ${size_mb} MB"
}

main "$@"
