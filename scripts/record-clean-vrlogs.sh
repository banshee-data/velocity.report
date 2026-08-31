#!/usr/bin/env bash
#
# Record three clean VRLOGs from PCAP captures, one after another.
#
# Each run uses the settle-before-recording flow: a first pass settles the
# background grid unrecorded, the settled snapshot is restored, then the same
# window is replayed and recorded. That is what produces a recording whose
# first frame is a background, so replaying it shows a scene immediately
# rather than foreground over nothing.
#
# Runs are paced at 0.1x so the perception pipeline sees every frame with time
# to work, which is the point of re-recording rather than reusing the old logs.
#
# Usage:
#   scripts/record-clean-vrlogs.sh            # run all three
#   scripts/record-clean-vrlogs.sh --dry-run  # print what would happen
#
# Override with environment variables:
#   API=http://localhost:8081  SENSOR=hesai-pandar40p  SPEED=0.1

set -euo pipefail

API="${API:-http://localhost:8081}"
SENSOR="${SENSOR:-hesai-pandar40p}"
SPEED="${SPEED:-0.1}"
PCAP_DIR="${PCAP_DIR:-/Users/david/code/sensor_data/lidar/static}"
DRY_RUN=""
[ "${1:-}" = "--dry-run" ] && DRY_RUN="yes"

CAPTURES=(
  "kirk0.pcapng"
  "clar0-1.pcapng"
  "soma0-static-0.pcap"
  "soma1-static-0-1.pcap"
  "soma3-static-0-1.pcap"
)

log() { printf '%s %s\n' "$(date '+%Y/%m/%d %H:%M:%S')" "$*"; }

# source_state reports the pipeline's current data source and whether a replay
# is running, as one line.
source_state() {
  curl -fsS "${API}/api/lidar/data_source?sensor_id=${SENSOR}" 2>/dev/null || echo '{}'
}

replay_running() {
  source_state | grep -q '"pcap_in_progress":true'
}

wait_for_idle() {
  local label="$1" waited=0
  # Give the server a moment to claim the replay slot before watching for it to
  # clear, or a run that has not started yet reads as one that has finished.
  sleep 5
  while replay_running; do
    sleep 10
    waited=$((waited + 10))
    if [ $((waited % 300)) -eq 0 ]; then
      local pass
      pass=$(source_state | grep -oE '"replay_pass":"[^"]*"' | cut -d'"' -f4)
      log "  ${label}: still running after $((waited / 60))m (pass=${pass:-unknown})"
    fi
  done
  log "  ${label}: finished after $((waited / 60))m$((waited % 60))s"
}

start_run() {
  local file="$1" path="${PCAP_DIR}/$1"

  if [ ! -f "$path" ]; then
    log "SKIP ${file}: not found at ${path}"
    return 1
  fi

  if [ -n "$DRY_RUN" ]; then
    log "would start ${file} at ${SPEED}x with settle-before-recording"
    return 0
  fi

  log "starting ${file} at ${SPEED}x"
  curl -fsS -X POST "${API}/api/lidar/pcap/start?sensor_id=${SENSOR}" \
    --data-urlencode "pcap_file=${path}" \
    --data-urlencode "analysis_mode=true" \
    --data-urlencode "settle_before_recording=true" \
    --data-urlencode "speed_mode=scaled" \
    --data-urlencode "speed_ratio=${SPEED}" \
    | sed 's/^/  /'
  echo
}

log "Recording ${#CAPTURES[@]} VRLOGs from ${PCAP_DIR} at ${SPEED}x"
log "Each capture runs twice: once to settle the grid, once recorded."
echo

for capture in "${CAPTURES[@]}"; do
  if start_run "$capture"; then
    [ -z "$DRY_RUN" ] && wait_for_idle "$capture"
  fi
  echo
done

log "All runs complete. New VRLOGs:"
curl -fsSL "${API}/api/lidar/runs?limit=10" 2>/dev/null \
  | python3 -c "
import json,sys
try:
    runs = json.load(sys.stdin)
    runs = runs if isinstance(runs, list) else runs.get('runs', [])
    for r in runs[:10]:
        print('  {} {}'.format(r.get('run_id','?'), r.get('created_at','')))
except Exception:
    print('  (could not parse the run list; check GET /api/lidar/runs)')
"
