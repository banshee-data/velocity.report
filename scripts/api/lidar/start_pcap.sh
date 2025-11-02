#!/usr/bin/env bash
# Start a PCAP replay via the local monitor API.
# Usage: ./start_pcap.sh /absolute/path/to/file.pcapng [sensor_id] [base_url]
set -euo pipefail
PCAP_FILE=${1:-}
SENSOR_ID=${2:-hesai-pandar40p}
BASE_URL=${3:-http://127.0.0.1:8081}
if [ -z "$PCAP_FILE" ]; then
  echo "Usage: $0 /path/to/pcap.pcapng [sensor_id] [base_url]"
  exit 2
fi

PCAP_NAME=$(basename "$PCAP_FILE")

echo "Starting PCAP replay: $PCAP_NAME (sensor_id=$SENSOR_ID) via $BASE_URL"
curl -s -X POST "$BASE_URL/api/lidar/pcap/start?sensor_id=$SENSOR_ID" \
  -H 'Content-Type: application/json' \
  -d '{"pcap_file":"'$PCAP_NAME'"}' | jq . || true
echo
