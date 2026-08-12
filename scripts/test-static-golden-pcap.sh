#!/usr/bin/env bash
# Exercise the exact static release binary against the canonical LiDAR capture.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN="${1:-}"
PCAP="$REPO_ROOT/internal/lidar/perf/pcap/kirk0.pcapng"

if [[ -z "$BIN" || ! -f "$BIN" ]]; then
    echo "usage: $0 <static-linux-velocity-binary>" >&2
    exit 2
fi
command -v jq >/dev/null || { echo "error: jq is required" >&2; exit 1; }
if [[ ! -s "$PCAP" || "$(stat -c %s "$PCAP")" -lt 1000000 ]]; then
    echo "error: canonical PCAP is missing; run git lfs pull" >&2
    exit 1
fi

RUN_DIR="$(mktemp -d)"
trap 'rm -rf -- "$RUN_DIR"' EXIT
ln -s "$(realpath "$BIN")" "$RUN_DIR/velocity"

"$RUN_DIR/velocity" lidar settling-eval \
    --port 2369 --output "$RUN_DIR/report.json" "$PCAP"

jq -e '
    .total_samples == 832 and
    .total_frames == 832 and
    .recommended_settling_frame == 11 and
    (.metrics_history | length) == 832 and
    .metrics_history[-1].frame_number == 832 and
    .metrics_history[-1].region_stability == 1 and
    (.metrics_history[-1].coverage_rate > 0.97)
' "$RUN_DIR/report.json" >/dev/null

echo "==> canonical PCAP release-binary validation passed"
