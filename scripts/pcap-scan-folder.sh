#!/usr/bin/env bash
set -euo pipefail

# Root of the capture library. Overridable so the script is not tied to one
# machine's mount point.
LIDAR_ROOT="${LIDAR_ROOT:-/Volumes/lidar/lidar/s2}"

MODE="rename-and-analyse"
if [ "${1:-}" = "--rename-only" ]; then
  MODE="rename-only"
  shift
fi

if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
  cat <<'EOF'
Usage: scripts/pcap-scan-folder.sh [--rename-only]

Modes:
  default       Rename eligible s2_* files to .pcap if needed, then run pcap-split
                only for captures that do not yet have an analysis subfolder.
  --rename-only Rename eligible s2_* files only; do not run pcap-split.
EOF
  exit 0
fi

if [ "$#" -ne 0 ]; then
  echo "Unexpected arguments: $*" >&2
  echo "Run with --help for usage." >&2
  exit 2
fi

if [ ! -d "$LIDAR_ROOT" ]; then
  echo "Capture root not found: $LIDAR_ROOT (set LIDAR_ROOT to override)" >&2
  exit 1
fi

if [ "$MODE" != "rename-only" ] && [ ! -x "./pcap-split" ]; then
  echo "Missing ./pcap-split binary in repo root. Build it first with: make build-pcap-split" >&2
  exit 1
fi

total_candidates=0
renamed_count=0
analysed_count=0
skipped_count=0

while IFS= read -r -d '' pcap; do
  total_candidates=$((total_candidates + 1))
  dir_name="$(dirname "$pcap")"
  file_name="$(basename "$pcap")"

  if [[ "$file_name" == *.pcap || "$file_name" == *.pcapng ]]; then
    :
  elif [[ "$file_name" == *.* ]]; then
    skipped_count=$((skipped_count + 1))
    continue
  else
    new_name="${file_name}.pcap"
    new_path="$dir_name/$new_name"
    if [ -e "$new_path" ]; then
      skipped_count=$((skipped_count + 1))
      continue
    fi
    echo "Renaming $pcap -> $new_path"
    mv -- "$pcap" "$new_path"
    pcap="$new_path"
    renamed_count=$((renamed_count + 1))
  fi

  if [ "$MODE" = "rename-only" ]; then
    continue
  fi

  capture_stem="$(basename "${pcap%.*}")"
  analysis_dir="$dir_name/analysis/$capture_stem"
  if [ -d "$analysis_dir" ]; then
    skipped_count=$((skipped_count + 1))
    continue
  fi

  mkdir -p "$analysis_dir"

  ./pcap-split \
    --pcap "$pcap" \
    --output "$analysis_dir" \
    --settling-sec 75 \
    --motion-trigger-sec 5 \
    --max-motion-gap-sec 45 \
    --min-segment-sec 10 \
    --stats-10s \
    --timeline-units seconds \
    --motion-json "$analysis_dir/motion_timeline.json" \
    --export-json \
    --export-metrics \
    --progress 20 \
    --dry-run 2>&1 | tee "$analysis_dir/analysis.log"
  analysed_count=$((analysed_count + 1))
done < <(
  find "$LIDAR_ROOT" \
    -type f \
    -name 's2_*' \
    ! -path '*/analysis/*' \
    ! -path '*/pcap_split_analysis_*/*' \
    -print0
)

echo "Summary: mode=$MODE candidates=$total_candidates renamed=$renamed_count analysed=$analysed_count skipped=$skipped_count"
