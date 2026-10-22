#!/usr/bin/env bash
# Switch LiDAR data source via the monitor API.
# Usage:
#   ./switch_data_source.sh live [sensor_id] [base_url]
#   ./switch_data_source.sh capture /path/to/file.pcap [sensor_id] [base_url]
set -euo pipefail

SOURCE=${1:-}
if [ -z "$SOURCE" ]; then
  echo "Usage: $0 {live|capture} [pcap_file] [sensor_id] [base_url]"
  exit 2
fi

BASE_URL="http://127.0.0.1:8081"
SENSOR_ID="hesai-pandar40p"
PCAP_FILE=""

case "$SOURCE" in
  live)
    SENSOR_ID=${2:-$SENSOR_ID}
    BASE_URL=${3:-$BASE_URL}
    ;;
  capture)
    PCAP_FILE=${2:-}
    if [ -z "$PCAP_FILE" ]; then
      echo "PCAP file required when switching to capture"
      exit 2
    fi
    SENSOR_ID=${3:-$SENSOR_ID}
    BASE_URL=${4:-$BASE_URL}
    ;;
  *)
    echo "Invalid source: $SOURCE"
    exit 2
    ;;

esac

if [ "$SOURCE" = "capture" ]; then
  PCAP_NAME=$(basename "$PCAP_FILE")
  echo "Starting PCAP replay ($PCAP_NAME) for sensor=$SENSOR_ID via $BASE_URL"
  curl -s -X POST "$BASE_URL/api/lidar/pcap/start?sensor_id=$SENSOR_ID" \
    -H 'Content-Type: application/json' \
    -d '{"pcap_file":"'$PCAP_NAME'"}' | jq . || true
else
  echo "Stopping PCAP replay for sensor=$SENSOR_ID via $BASE_URL"
  curl -s -X POST "$BASE_URL/api/lidar/replay/stop?sensor_id=$SENSOR_ID" | jq . || true
fi

echo
