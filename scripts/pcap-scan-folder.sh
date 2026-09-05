#!/usr/bin/env bash
set -euo pipefail

# Root of the capture library. Overridable so the script is not tied to one
# machine's mount point.
LIDAR_ROOT="${LIDAR_ROOT:-/Volumes/lidar/lidar}"

if [ ! -d "$LIDAR_ROOT" ]; then
  echo "Capture root not found: $LIDAR_ROOT (set LIDAR_ROOT to override)" >&2
  exit 1
fi

if [ ! -x "./pcap-split" ]; then
  echo "Missing ./pcap-split binary in repo root. Build it first with: make build-pcap-split" >&2
  exit 1
fi

find "$LIDAR_ROOT" -type f \( -path "$LIDAR_ROOT/s2*" -o -path "$LIDAR_ROOT/s2/*" \) -print0 |
while IFS= read -r -d '' pcap; do
  dir_name="$(dirname "$pcap")"
  file_name="$(basename "$pcap")"

  if [[ "$file_name" != *.pcap ]]; then
    new_name="${file_name%.*}.pcap"
    new_path="$dir_name/$new_name"
    echo "Renaming $pcap -> $new_path"
    mv -- "$pcap" "$new_path"
    pcap="$new_path"
  fi

  analysis_dir="$dir_name/analysis"
  if [ -d "$analysis_dir" ]; then
    echo "Skipping $pcap: analysis already exists at $analysis_dir"
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
done
