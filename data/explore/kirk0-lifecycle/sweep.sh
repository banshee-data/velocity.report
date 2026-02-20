#!/usr/bin/env bash
# Run kirk0.pcapng through 5 track-lifecycle parameter permutations,
# capturing JSON state snapshots after each replay completes.
#
# Prerequisites:
#   1. Server running with LiDAR:  make dev-go-lidar
#   2. (Optional) macOS visualiser connected for live observation
#
# Usage: ./data/explore/kirk0-lifecycle/sweep.sh [base_url]
#
# Output: data/explore/kirk0-lifecycle/results/<timestamp>/
#   ├── 1-baseline/
#   │   ├── config.json          # tuning params applied
#   │   ├── tracks-active.json   # active tracks at capture time
#   │   ├── tracks-metrics.json  # tracking metrics (counts, rates)
#   │   ├── tracks-summary.json  # per-track summary statistics
#   │   ├── acceptance.json      # acceptance/rejection buckets
#   │   ├── grid-status.json     # background grid status
#   │   ├── params.json          # full resolved params from server
#   │   └── data-source.json     # data source state + run ID
#   ├── 2-quick-confirm/
#   │   └── ...
#   └── summary.json             # combined summary of all permutations
set -euo pipefail

BASE_URL="${1:-http://127.0.0.1:8081}"
SENSOR_ID="hesai-pandar40p"
PCAP_FILE="static/kirk0.pcapng"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
RESULTS_DIR="${SCRIPT_DIR}/results/${TIMESTAMP}"
mkdir -p "${RESULTS_DIR}"

CONFIGS=(
  "kirk0-lifecycle-1-baseline.json"
  "kirk0-lifecycle-2-quick-confirm.json"
  "kirk0-lifecycle-3-strict-confirm.json"
  "kirk0-lifecycle-4-persistent.json"
  "kirk0-lifecycle-5-aggressive-cleanup.json"
)

# Short names for result directories
DIR_NAMES=(
  "1-baseline"
  "2-quick-confirm"
  "3-strict-confirm"
  "4-persistent"
  "5-aggressive-cleanup"
)

# ── helpers ──────────────────────────────────────────────────────────────────

api_get() {
  local endpoint="$1"
  curl -sf "${BASE_URL}${endpoint}?sensor_id=${SENSOR_ID}" 2>/dev/null || echo '{"error":"request failed"}'
}

api_post() {
  local endpoint="$1"
  shift
  curl -sf -X POST "${BASE_URL}${endpoint}?sensor_id=${SENSOR_ID}" "$@" 2>/dev/null || echo '{"error":"request failed"}'
}

set_params() {
  local json_file="$1"
  echo "  → Apply params"
  api_post "/api/lidar/params" -H 'Content-Type: application/json' -d @"${json_file}" | jq -c . || true
}

reset_grid() {
  echo "  → Reset grid"
  api_post "/api/lidar/grid_reset" | jq -c . || true
}

start_pcap() {
  echo "  → Start pcap replay (analysis_mode=true, speed=realtime)"
  api_post "/api/lidar/pcap/start" \
    -H 'Content-Type: application/json' \
    -d "{\"pcap_file\":\"${PCAP_FILE}\",\"analysis_mode\":true,\"speed_mode\":\"realtime\",\"speed_ratio\":1.0}" | jq -c . || true
}

stop_pcap() {
  api_post "/api/lidar/pcap/stop" > /dev/null 2>&1 || true
}

wait_for_pcap_complete() {
  echo "  → Waiting for pcap replay to finish..."
  local max_wait=300  # 5 minutes max
  local elapsed=0
  while [ $elapsed -lt $max_wait ]; do
    local ds
    ds=$(api_get "/api/lidar/data_source")
    local source
    source=$(echo "$ds" | jq -r '.current_source // "unknown"' 2>/dev/null || echo "unknown")
    if [ "$source" != "pcap" ]; then
      echo "  → Replay complete (${elapsed}s)"
      return 0
    fi
    sleep 2
    elapsed=$((elapsed + 2))
  done
  echo "  ⚠ Timed out waiting for pcap replay (${max_wait}s)"
  return 1
}

capture_state() {
  local out_dir="$1"
  local cfg_file="$2"
  mkdir -p "${out_dir}"

  echo "  → Capturing state → $(basename "$(dirname "$out_dir")")/$(basename "$out_dir")/"

  # Copy the config that was applied
  cp "${cfg_file}" "${out_dir}/config.json"

  # Active tracks
  api_get "/api/lidar/tracks/active" | jq . > "${out_dir}/tracks-active.json"

  # Tracking metrics (confirmed count, tentative count, deleted count, etc.)
  api_get "/api/lidar/tracks/metrics" | jq . > "${out_dir}/tracks-metrics.json"

  # Track summary (per-track stats: speed, duration, length, class)
  api_get "/api/lidar/tracks/summary" | jq . > "${out_dir}/tracks-summary.json"

  # Acceptance metrics (bucket counts, rejection rates)
  api_get "/api/lidar/acceptance" | jq . > "${out_dir}/acceptance.json"

  # Grid status (background model convergence)
  api_get "/api/lidar/grid_status" | jq . > "${out_dir}/grid-status.json"

  # Full resolved params from server
  api_get "/api/lidar/params" | jq . > "${out_dir}/params.json"

  # Data source state (includes last_run_id for VRLOG)
  api_get "/api/lidar/data_source" | jq . > "${out_dir}/data-source.json"

  echo "  → Captured 7 state files"
}

# ── main loop ────────────────────────────────────────────────────────────────

echo "═══════════════════════════════════════════════════════════"
echo "  kirk0 lifecycle sweep — ${#CONFIGS[@]} permutations"
echo "  server: ${BASE_URL}  sensor: ${SENSOR_ID}"
echo "  output: ${RESULTS_DIR}"
echo "═══════════════════════════════════════════════════════════"
echo ""

SUMMARY_ITEMS=()

for i in "${!CONFIGS[@]}"; do
  cfg="${CONFIGS[$i]}"
  cfg_path="${SCRIPT_DIR}/${cfg}"
  dir_name="${DIR_NAMES[$i]}"
  out_dir="${RESULTS_DIR}/${dir_name}"
  n=$((i + 1))

  label=$(jq -r '._label // "unknown"' "$cfg_path")

  echo "───────────────────────────────────────────────────────────"
  echo "  ${label}"
  echo "  config: ${cfg}"
  echo "───────────────────────────────────────────────────────────"
  echo ""

  # Show tuning params (exclude metadata keys)
  jq 'with_entries(select(.key | startswith("_") | not))' "$cfg_path"
  echo ""

  # 1. Stop any running pcap
  stop_pcap

  # 2. Apply params
  set_params "$cfg_path"

  # 3. Reset grid
  reset_grid

  # 4. Brief pause for reset
  sleep 1

  # 5. Start pcap replay
  start_pcap

  # 6. Wait for replay to complete
  wait_for_pcap_complete || true

  # 7. Small delay for final state to settle
  sleep 2

  # 8. Capture state
  capture_state "${out_dir}" "${cfg_path}"

  # 9. Build summary entry
  track_count=$(jq '.total_tracks // .tracks // [] | if type == "array" then length else . end' "${out_dir}/tracks-active.json" 2>/dev/null || echo "0")
  SUMMARY_ITEMS+=("{\"permutation\":${n},\"label\":\"${label}\",\"dir\":\"${dir_name}\",\"tracks\":${track_count}}")

  echo ""
  if [ "$n" -lt "${#CONFIGS[@]}" ]; then
    echo "  ▶ Press Enter for next permutation..."
    read -r
  fi
  echo ""
done

# Stop pcap after final run
stop_pcap

# Build combined summary
{
  echo "{"
  echo "  \"timestamp\": \"${TIMESTAMP}\","
  echo "  \"pcap_file\": \"${PCAP_FILE}\","
  echo "  \"sensor_id\": \"${SENSOR_ID}\","
  echo "  \"permutations\": ["
  for j in "${!SUMMARY_ITEMS[@]}"; do
    if [ "$j" -lt $((${#SUMMARY_ITEMS[@]} - 1)) ]; then
      echo "    ${SUMMARY_ITEMS[$j]},"
    else
      echo "    ${SUMMARY_ITEMS[$j]}"
    fi
  done
  echo "  ]"
  echo "}"
} | jq . > "${RESULTS_DIR}/summary.json"

echo "═══════════════════════════════════════════════════════════"
echo "  Done — ${#CONFIGS[@]} permutations complete."
echo "  Results: ${RESULTS_DIR}"
echo "═══════════════════════════════════════════════════════════"
echo ""
echo "Summary:"
jq -r '.permutations[] | "  \(.permutation). \(.label) — tracks: \(.tracks)"' "${RESULTS_DIR}/summary.json"
