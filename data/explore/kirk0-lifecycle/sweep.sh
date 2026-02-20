#!/usr/bin/env bash
# Run kirk0.pcapng through 5 track-lifecycle parameter permutations,
# capturing JSON state snapshots after each replay completes.
#
# Phase 0 — Region settling: plays the PCAP once with default params and
#            full 30s warmup so that background grid + regions are persisted
#            to SQLite.  Subsequent permutation runs restore from DB in ~10
#            frames, skipping the 30s settling period entirely.
#
# Phase 1 — Permutation sweep: each config is replayed with a shortened
#            10s/50-frame warmup as a safety net (region restore should
#            skip it), and state is captured after each replay.
#
# Prerequisites:
#   1. Server running with LiDAR:  make dev-go-lidar
#   2. (Optional) macOS visualiser connected for live observation
#
# Usage: ./data/explore/kirk0-lifecycle/sweep.sh [--start-from N] [base_url]
#
#   --start-from N   Skip to permutation N (1-indexed), bypassing Phase 0
#                    settling (assumes grid snapshot already in SQLite).
#                    Example: ./sweep.sh --start-from 6
#
# Output: data/explore/kirk0-lifecycle/results/<timestamp>/
#   ├── 0-settling/
#   │   ├── config.json          # baseline params used for settling
#   │   ├── grid-status.json     # background grid after full settle
#   │   ├── data-source.json     # data source state + run ID
#   │   └── params.json          # resolved params from server
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

# ── CLI argument parsing ──────────────────────────────────────────────────────
START_FROM=1
while [[ $# -gt 0 ]]; do
  case "$1" in
    --start-from|-s)
      START_FROM="${2:?--start-from requires a permutation number (1-indexed)}"
      shift 2
      ;;
    -s[0-9]*)
      START_FROM="${1#-s}"
      shift
      ;;
    *)
      break
      ;;
  esac
done

BASE_URL="${1:-http://127.0.0.1:8081}"
SENSOR_ID="hesai-pandar40p"
PCAP_FILE="static/kirk0.pcapng"

# Warmup overrides for sweep runs (region restore should skip this entirely;
# the 10s/50-frame values are a safety net in case no snapshot is found).
SWEEP_WARMUP_NANOS=10000000000   # 10 seconds
SWEEP_WARMUP_FRAMES=50

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
  "kirk0-lifecycle-6-fast-confirm.json"
  "kirk0-lifecycle-7-ultra-fast.json"
  "kirk0-lifecycle-8-max-churn.json"
  "kirk0-lifecycle-9-max-churn-tight.json"
  "kirk0-lifecycle-10-max-churn-tightest.json"
)

# Short names for result directories
DIR_NAMES=(
  "1-baseline"
  "2-quick-confirm"
  "3-strict-confirm"
  "4-persistent"
  "5-aggressive-cleanup"
  "6-fast-confirm"
  "7-ultra-fast"
  "8-max-churn"
  "9-max-churn-tight"
  "10-max-churn-tightest"
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
  local extra_overrides="${2:-}"
  echo "  → Apply params"
  if [ -n "$extra_overrides" ]; then
    # Merge config file with runtime overrides (overrides win)
    jq -s '.[0] * .[1]' "${json_file}" <(echo "$extra_overrides") \
      | api_post "/api/lidar/params" -H 'Content-Type: application/json' -d @- | jq -c . || true
  else
    api_post "/api/lidar/params" -H 'Content-Type: application/json' -d @"${json_file}" | jq -c . || true
  fi
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

start_pcap_fastest() {
  echo "  → Start pcap replay (analysis_mode=true, speed=fastest)"
  api_post "/api/lidar/pcap/start" \
    -H 'Content-Type: application/json' \
    -d "{\"pcap_file\":\"${PCAP_FILE}\",\"analysis_mode\":true,\"speed_mode\":\"fastest\"}" | jq -c . || true
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

# ── Phase 0: Region settling run ─────────────────────────────────────────────

echo "═══════════════════════════════════════════════════════════"
echo "  kirk0 lifecycle sweep — ${#CONFIGS[@]} permutations"
echo "  server: ${BASE_URL}  sensor: ${SENSOR_ID}"
if [ "$START_FROM" -gt 1 ]; then
  echo "  start-from: permutation ${START_FROM} (skipping Phase 0)"
fi
echo "  output: ${RESULTS_DIR}"
echo "═══════════════════════════════════════════════════════════"
echo ""

if [ "$START_FROM" -le 1 ]; then
  # ── Phase 0: Region settling run ────────────────────────────────────────
  echo "───────────────────────────────────────────────────────────"
  echo "  Phase 0 — Region settling (full 30s warmup)"
  echo "  Populates background grid + regions in SQLite so"
  echo "  subsequent runs can restore in ~10 frames."
  echo "───────────────────────────────────────────────────────────"
  echo ""

  SETTLE_DIR="${RESULTS_DIR}/0-settling"
  SETTLE_CFG="${SCRIPT_DIR}/${CONFIGS[0]}"  # Use baseline config for settling

  # 1. Stop any running pcap
  stop_pcap

  # 2. Apply baseline params (with default 30s warmup — no overrides)
  set_params "$SETTLE_CFG"

  # 3. Reset grid for clean start
  reset_grid

  # 4. Brief pause for reset
  sleep 1

  # 5. Start pcap replay at fastest speed (just need the grid to settle)
  start_pcap_fastest

  # 6. Wait for replay to complete
  wait_for_pcap_complete || true

  # 7. Capture settling state
  mkdir -p "${SETTLE_DIR}"
  cp "${SETTLE_CFG}" "${SETTLE_DIR}/config.json"
  api_get "/api/lidar/grid_status" | jq . > "${SETTLE_DIR}/grid-status.json"
  api_get "/api/lidar/params"      | jq . > "${SETTLE_DIR}/params.json"
  api_get "/api/lidar/data_source"  | jq . > "${SETTLE_DIR}/data-source.json"

  # Verify settling completed and snapshot was persisted
  settle_complete=$(jq -r '.settling_complete // false' "${SETTLE_DIR}/grid-status.json" 2>/dev/null || echo "unknown")
  bg_count=$(jq -r '.background_count // 0' "${SETTLE_DIR}/grid-status.json" 2>/dev/null || echo "0")
  echo ""
  echo "  → Settling complete: ${settle_complete}"
  echo "  → Background cells: ${bg_count}"
  echo ""

  echo "  Phase 0 done — grid snapshot should now be persisted in SQLite."
  echo "  Subsequent runs will attempt region restore from DB."
  echo ""
  echo "  ▶ Press Enter to start Phase 1 (permutation sweep)..."
  read -r
else
  echo "  Skipping Phase 0 (--start-from ${START_FROM}), assuming grid snapshot exists."
  echo ""
fi

# ── Phase 1: Permutation sweep (with region restore) ────────────────────────

WARMUP_OVERRIDES="{\"warmup_duration_nanos\":${SWEEP_WARMUP_NANOS},\"warmup_min_frames\":${SWEEP_WARMUP_FRAMES}}"

echo ""
echo "───────────────────────────────────────────────────────────"
echo "  Phase 1 — ${#CONFIGS[@]} permutations (10s warmup safety net)"
echo "  Region restore should skip warmup after ~10 frames."
echo "───────────────────────────────────────────────────────────"
echo ""

SUMMARY_ITEMS=()

for i in "${!CONFIGS[@]}"; do
  n=$((i + 1))

  # Skip permutations before START_FROM
  if [ "$n" -lt "$START_FROM" ]; then
    continue
  fi

  cfg="${CONFIGS[$i]}"
  cfg_path="${SCRIPT_DIR}/${cfg}"
  dir_name="${DIR_NAMES[$i]}"
  out_dir="${RESULTS_DIR}/${dir_name}"

  label=$(jq -r '._label // "unknown"' "$cfg_path")

  echo "───────────────────────────────────────────────────────────"
  echo "  ${label}"
  echo "  config: ${cfg}"
  echo "───────────────────────────────────────────────────────────"
  echo ""

  # Show tuning params (exclude metadata keys)
  jq 'with_entries(select(.key | startswith("_") | not))' "$cfg_path"
  echo "  + warmup_duration_nanos: ${SWEEP_WARMUP_NANOS} (10s safety net)"
  echo "  + warmup_min_frames: ${SWEEP_WARMUP_FRAMES}"
  echo ""

  # 1. Stop any running pcap
  stop_pcap

  # 2. Apply params with shortened warmup overrides
  set_params "$cfg_path" "$WARMUP_OVERRIDES"

  # 3. Reset grid (region restore will be attempted after ~10 frames)
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

  # 9. Check if region restore was used (settling_complete should be true very early)
  settle_status=$(jq -r '.settling_complete // false' "${out_dir}/grid-status.json" 2>/dev/null || echo "unknown")
  echo "  → Grid settling_complete: ${settle_status}"

  # 10. Build summary entry
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
