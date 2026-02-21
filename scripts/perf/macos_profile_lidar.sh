#!/usr/bin/env bash
# Poll macOS process metrics for velocity-report (Go) and VelocityVisualiser (Swift),
# capture an idle baseline, trigger PCAP replay, and print a summary.
#
# Example:
#   ./scripts/perf/macos_profile_lidar.sh \
#     --pcap internal/lidar/perf/pcap/kirk0.pcapng \
#     --interval 5 \
#     --idle-seconds 120

set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  macos_profile_lidar.sh --pcap /path/to/file.pcapng [options]

Options:
  --pcap PATH                Path to .pcap/.pcapng file (required).
  --sensor ID                Sensor ID (default: hesai-pandar40p).
  --base-url URL             API base URL (default: http://127.0.0.1:8081).
  --interval SECONDS         Poll interval (default: 5).
  --idle-seconds SECONDS     Idle baseline duration before PCAP start (default: 120).
  --max-active-seconds SEC   Safety timeout for active phase (default: 1800).
  --go-pid PID               Use explicit Go server PID (optional).
  --swift-pid PID            Use explicit Swift app PID (optional).
  --out-dir DIR              Output directory root (default: logs/perf).
  --help                     Show this help.

Notes:
  - Designed for 'make dev-go-lidar' (radar.go server).
  - If --go-pid is omitted, the script uses the newest logs/pids/velocity-*.pid.
  - If --swift-pid is omitted, it tries 'pgrep -x VelocityVisualiser'.
  - PCAP start uses /api/lidar/pcap/start and expects pcap_file basename under safe dir.
EOF
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1" >&2
    exit 1
  fi
}

PCAP_PATH=""
SENSOR_ID="hesai-pandar40p"
BASE_URL="http://127.0.0.1:8081"
INTERVAL=5
IDLE_SECONDS=120
MAX_ACTIVE_SECONDS=1800
GO_PID=""
SWIFT_PID=""
OUT_ROOT="logs/perf"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --pcap)
      if [[ $# -lt 2 ]]; then
        echo "Missing value for --pcap" >&2
        usage
        exit 2
      fi
      PCAP_PATH="$2"
      shift 2
      ;;
    --sensor)
      if [[ $# -lt 2 ]]; then
        echo "Missing value for --sensor" >&2
        usage
        exit 2
      fi
      SENSOR_ID="$2"
      shift 2
      ;;
    --base-url)
      if [[ $# -lt 2 ]]; then
        echo "Missing value for --base-url" >&2
        usage
        exit 2
      fi
      BASE_URL="$2"
      shift 2
      ;;
    --interval)
      if [[ $# -lt 2 ]]; then
        echo "Missing value for --interval" >&2
        usage
        exit 2
      fi
      INTERVAL="$2"
      shift 2
      ;;
    --idle-seconds)
      if [[ $# -lt 2 ]]; then
        echo "Missing value for --idle-seconds" >&2
        usage
        exit 2
      fi
      IDLE_SECONDS="$2"
      shift 2
      ;;
    --max-active-seconds)
      if [[ $# -lt 2 ]]; then
        echo "Missing value for --max-active-seconds" >&2
        usage
        exit 2
      fi
      MAX_ACTIVE_SECONDS="$2"
      shift 2
      ;;
    --go-pid)
      if [[ $# -lt 2 ]]; then
        echo "Missing value for --go-pid" >&2
        usage
        exit 2
      fi
      GO_PID="$2"
      shift 2
      ;;
    --swift-pid)
      if [[ $# -lt 2 ]]; then
        echo "Missing value for --swift-pid" >&2
        usage
        exit 2
      fi
      SWIFT_PID="$2"
      shift 2
      ;;
    --out-dir)
      if [[ $# -lt 2 ]]; then
        echo "Missing value for --out-dir" >&2
        usage
        exit 2
      fi
      OUT_ROOT="$2"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage
      exit 2
      ;;
  esac
done

require_cmd ps
require_cmd awk
require_cmd curl
require_cmd date
require_cmd jq
require_cmd cp
require_cmd mkdir

if [[ -z "$PCAP_PATH" ]]; then
  echo "--pcap is required" >&2
  usage
  exit 2
fi

if [[ ! -f "$PCAP_PATH" ]]; then
  echo "PCAP file not found: $PCAP_PATH" >&2
  exit 2
fi

if ! [[ "$INTERVAL" =~ ^[0-9]+$ ]] || (( INTERVAL <= 0 )); then
  echo "--interval must be a positive integer" >&2
  exit 2
fi
if ! [[ "$IDLE_SECONDS" =~ ^[0-9]+$ ]] || (( IDLE_SECONDS < 0 )); then
  echo "--idle-seconds must be a non-negative integer" >&2
  exit 2
fi
if ! [[ "$MAX_ACTIVE_SECONDS" =~ ^[0-9]+$ ]] || (( MAX_ACTIVE_SECONDS <= 0 )); then
  echo "--max-active-seconds must be a positive integer" >&2
  exit 2
fi

if [[ -z "$GO_PID" ]]; then
  latest_pid_file="$(ls -1t logs/pids/velocity-*.pid 2>/dev/null | head -n1 || true)"
  if [[ -n "$latest_pid_file" ]]; then
    GO_PID="$(cat "$latest_pid_file" 2>/dev/null || true)"
  fi
fi
if [[ -z "$GO_PID" ]]; then
  echo "Could not determine Go PID. Start server with 'make dev-go-lidar' or pass --go-pid." >&2
  exit 2
fi
if ! kill -0 "$GO_PID" 2>/dev/null; then
  echo "Go PID is not running: $GO_PID" >&2
  exit 2
fi

if [[ -z "$SWIFT_PID" ]]; then
  SWIFT_PID="$(pgrep -x VelocityVisualiser | head -n1 || true)"
fi
if [[ -n "$SWIFT_PID" ]] && ! kill -0 "$SWIFT_PID" 2>/dev/null; then
  echo "Warning: Swift PID not running ($SWIFT_PID). Continuing with Go-only metrics." >&2
  SWIFT_PID=""
fi

ts="$(date +%Y%m%d-%H%M%S)"
OUT_DIR="${OUT_ROOT%/}/$ts"
mkdir -p "$OUT_DIR"

SAMPLES_CSV="$OUT_DIR/samples.csv"
PHASE_LOG="$OUT_DIR/phase.log"
PCAP_START_JSON="$OUT_DIR/pcap_start_response.json"
PCAP_FILES_JSON="$OUT_DIR/pcap_files_response.json"
DATASOURCE_JSONL="$OUT_DIR/data_source.jsonl"
SUMMARY_TXT="$OUT_DIR/summary.txt"
START_ERROR_FILES_JSON="$OUT_DIR/pcap_files_after_start_error.json"
START_ERROR_STATUS_JSON="$OUT_DIR/lidar_status_after_start_error.json"

echo "timestamp,phase,process,pid,cpu_pct,mem_pct,rss_kb,vsz_kb,threads,cpu_time" >"$SAMPLES_CSV"
echo "start_ts=$(date -u +%Y-%m-%dT%H:%M:%SZ)" >"$PHASE_LOG"

sample_pid() {
  local phase="$1"
  local proc="$2"
  local pid="$3"
  local epoch
  epoch="$(date +%s)"

  if ! kill -0 "$pid" 2>/dev/null; then
    echo "$epoch,$phase,$proc,$pid,NA,NA,NA,NA,NA,NA" >>"$SAMPLES_CSV"
    return
  fi

  local line pid_out cpu mem rss vsz th cpu_time
  line="$(ps -o pid=,%cpu=,%mem=,rss=,vsz=,time= -p "$pid" 2>/dev/null | xargs || true)"
  if [[ -z "$line" ]]; then
    echo "$epoch,$phase,$proc,$pid,NA,NA,NA,NA,NA,NA" >>"$SAMPLES_CSV"
    return
  fi

  read -r pid_out cpu mem rss vsz cpu_time <<<"$line"

  # Portable thread count fallback for macOS:
  # - Prefer counting per-thread rows from `ps -M` if available.
  # - If unavailable, leave as NA (does not break sampling).
  th="$(ps -M -p "$pid" 2>/dev/null | awk 'NR>1 { n++ } END { if (n > 0) print n; else print "NA" }' || true)"
  if [[ -z "$th" ]]; then
    th="NA"
  fi

  echo "$epoch,$phase,$proc,$pid_out,$cpu,$mem,$rss,$vsz,$th,$cpu_time" >>"$SAMPLES_CSV"
}

sample_once() {
  local phase="$1"
  sample_pid "$phase" "go" "$GO_PID"
  if [[ -n "$SWIFT_PID" ]]; then
    sample_pid "$phase" "swift" "$SWIFT_PID"
  fi
}

poll_datasource() {
  local epoch json
  epoch="$(date +%s)"
  json="$(curl -fsS "$BASE_URL/api/lidar/data_source" || true)"
  if [[ -n "$json" ]]; then
    printf '%s\t%s\n' "$epoch" "$json" >>"$DATASOURCE_JSONL"
  fi
  printf '%s' "$json"
}

stage_pcap_for_api() {
  local source_path="$1"
  local source_name source_abs files_json safe_dir dest_path dest_abs

  source_name="$(basename "$source_path")"
  source_abs="$(cd "$(dirname "$source_path")" && pwd -P)/$(basename "$source_path")"

  # Query server-advertised safe dir. If unavailable, fall back to basename behavior.
  files_json="$(curl -fsS "$BASE_URL/api/lidar/pcap/files" 2>/dev/null || true)"
  if [[ -z "$files_json" ]]; then
    echo "pcap_files_endpoint=unavailable" >>"$PHASE_LOG"
    echo "$source_name"
    return 0
  fi
  printf '%s\n' "$files_json" >"$PCAP_FILES_JSON"

  safe_dir="$(jq -r '.pcap_dir // empty' "$PCAP_FILES_JSON" 2>/dev/null || true)"
  if [[ -z "$safe_dir" || "$safe_dir" == "null" ]]; then
    echo "pcap_safe_dir=unknown" >>"$PHASE_LOG"
    echo "$source_name"
    return 0
  fi
  echo "pcap_safe_dir=$safe_dir" >>"$PHASE_LOG"

  mkdir -p "$safe_dir"
  dest_path="${safe_dir%/}/$source_name"
  dest_abs="$(cd "$(dirname "$dest_path")" && pwd -P)/$(basename "$dest_path")"

  # Auto-stage only when source is outside the configured safe directory.
  if [[ "$source_abs" != "$dest_abs" ]]; then
    echo "Staging PCAP into safe dir: $dest_path" >&2
    cp -f "$source_path" "$dest_path"
    echo "pcap_staged_to=$dest_path" >>"$PHASE_LOG"
  else
    echo "pcap_staged_to=already_in_safe_dir" >>"$PHASE_LOG"
  fi

  # Refresh file list after staging for diagnostics.
  curl -fsS "$BASE_URL/api/lidar/pcap/files" >"$PCAP_FILES_JSON" 2>/dev/null || true
  if jq -e --arg p "$source_name" 'any(.files[]?; .path == $p)' "$PCAP_FILES_JSON" >/dev/null 2>&1; then
    echo "pcap_present_in_safe_dir=true" >>"$PHASE_LOG"
  else
    echo "pcap_present_in_safe_dir=false" >>"$PHASE_LOG"
  fi

  echo "$source_name"
}

printf "Profiling run directory: %s\n" "$OUT_DIR"
printf "Go PID: %s\n" "$GO_PID"
if [[ -n "$SWIFT_PID" ]]; then
  printf "Swift PID: %s\n" "$SWIFT_PID"
else
  printf "Swift PID: not found (Go-only run)\n"
fi
printf "Idle phase: %ss @ %ss cadence\n" "$IDLE_SECONDS" "$INTERVAL"

PCAP_REQUEST_PATH="$(stage_pcap_for_api "$PCAP_PATH")"
echo "pcap_request_path=$PCAP_REQUEST_PATH" >>"$PHASE_LOG"

idle_samples=0
if (( IDLE_SECONDS > 0 )); then
  idle_loops=$(( IDLE_SECONDS / INTERVAL ))
  if (( IDLE_SECONDS % INTERVAL != 0 )); then
    idle_loops=$(( idle_loops + 1 ))
  fi
  for ((i=1; i<=idle_loops; i++)); do
    sample_once "idle"
    idle_samples=$i
    sleep "$INTERVAL"
  done
fi
printf "idle_samples=%s\n" "$idle_samples" >>"$PHASE_LOG"

printf "Starting PCAP replay: %s\n" "$PCAP_REQUEST_PATH"
# Use "realtime" speed_mode so the full tracking pipeline runs (BackgroundManager
# warmup, ForegroundForwarder, background parameter tuning). "fastest" mode
# skips foreground extraction and produces 0 tracks.
payload="$(jq -n --arg p "$PCAP_REQUEST_PATH" '{pcap_file:$p, speed_mode:"realtime", analysis_mode:true}')"
echo "pcap_start_payload=$payload" >>"$PHASE_LOG"
start_http_code="$(curl -sS -o "$PCAP_START_JSON" -w "%{http_code}" -X POST "$BASE_URL/api/lidar/pcap/start?sensor_id=$SENSOR_ID" \
  -H 'Content-Type: application/json' \
  -d "$payload" || true)"
echo "pcap_start_http_code=$start_http_code" >>"$PHASE_LOG"

start_status="$(jq -r '.status // empty' "$PCAP_START_JSON" 2>/dev/null || true)"
if [[ "${start_http_code:-000}" != 2* || "$start_status" != "started" ]]; then
  echo "PCAP start failed (http_code=${start_http_code:-unknown}, status=${start_status:-empty})." >&2
  echo "PCAP start payload: $payload" >&2
  curl -fsS "$BASE_URL/api/lidar/pcap/files" >"$START_ERROR_FILES_JSON" 2>/dev/null || true
  curl -fsS "$BASE_URL/api/lidar/status" >"$START_ERROR_STATUS_JSON" 2>/dev/null || true
  echo "PCAP start response body:" >&2
  cat "$PCAP_START_JSON" >&2
  echo "Saved diagnostics:" >&2
  echo "  $PCAP_START_JSON" >&2
  echo "  $PCAP_FILES_JSON" >&2
  echo "  $START_ERROR_FILES_JSON" >&2
  echo "  $START_ERROR_STATUS_JSON" >&2
  exit 1
fi

printf "Active phase started. Polling until replay completes (max %ss).\n" "$MAX_ACTIVE_SECONDS"
active_elapsed=0
active_samples=0

while true; do
  sample_once "active"
  active_samples=$((active_samples + 1))

  ds_json="$(poll_datasource)"
  pcap_in_progress="$(printf '%s' "$ds_json" | jq -r '.pcap_in_progress // false' 2>/dev/null || echo "false")"

  if [[ "$pcap_in_progress" != "true" && "$active_elapsed" -gt 0 ]]; then
    break
  fi
  if (( active_elapsed >= MAX_ACTIVE_SECONDS )); then
    echo "Reached max active duration ($MAX_ACTIVE_SECONDS s). Stopping sampling."
    break
  fi

  sleep "$INTERVAL"
  active_elapsed=$((active_elapsed + INTERVAL))
done
printf "active_samples=%s\n" "$active_samples" >>"$PHASE_LOG"
echo "end_ts=$(date -u +%Y-%m-%dT%H:%M:%SZ)" >>"$PHASE_LOG"

summarise_phase_process() {
  local phase="$1"
  local proc="$2"
  awk -F, -v phase="$phase" -v proc="$proc" '
    $2 == phase && $3 == proc && $5 != "NA" {
      n += 1
      cpu += $5
      if ($5 > cpu_max) cpu_max = $5
      rss += $7
      if ($7 > rss_max) rss_max = $7
      mem += $6
      if ($6 > mem_max) mem_max = $6
    }
    END {
      if (n == 0) {
        printf "%s,%s,0,NA,NA,NA,NA,NA,NA\n", phase, proc
      } else {
        printf "%s,%s,%d,%.2f,%.2f,%.2f,%.2f,%.1f,%.1f\n",
          phase, proc, n, cpu / n, cpu_max, mem / n, mem_max, (rss / n) / 1024.0, rss_max / 1024.0
      }
    }
  ' "$SAMPLES_CSV"
}

{
  echo "PCAP profiling summary"
  echo "======================"
  echo "output_dir=$OUT_DIR"
  echo "go_pid=$GO_PID"
  if [[ -n "$SWIFT_PID" ]]; then
    echo "swift_pid=$SWIFT_PID"
  else
    echo "swift_pid=not-found"
  fi
  echo "interval_seconds=$INTERVAL"
  echo "idle_seconds=$IDLE_SECONDS"
  echo "max_active_seconds=$MAX_ACTIVE_SECONDS"
  echo
  echo "phase,process,samples,avg_cpu_pct,peak_cpu_pct,avg_mem_pct,peak_mem_pct,avg_rss_mb,peak_rss_mb"
  summarise_phase_process "idle" "go"
  summarise_phase_process "active" "go"
  summarise_phase_process "idle" "swift"
  summarise_phase_process "active" "swift"
} | tee "$SUMMARY_TXT"

echo
echo "Saved:"
echo "  $SUMMARY_TXT"
echo "  $SAMPLES_CSV"
echo "  $PHASE_LOG"
echo "  $PCAP_START_JSON"
echo "  $PCAP_FILES_JSON"
echo "  $DATASOURCE_JSONL"
