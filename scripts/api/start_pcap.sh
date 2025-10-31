#!/usr/bin/env bash
# Start a PCAP replay via the local monitor API.
# Usage: ./start_pcap.sh /absolute/path/to/file.pcapng [sensor_id]
set -euo pipefail
PCAP_FILE=${1:-}
SENSOR_ID=${2:-hesai-pandar40p}
if [ -z "$PCAP_FILE" ]; then
  echo "Usage: $0 /path/to/pcap.pcapng [sensor_id]"
  exit 2
fi

echo "Starting PCAP replay: $PCAP_FILE (sensor_id=$SENSOR_ID)"
curl -s -X POST "http://127.0.0.1:8081/api/lidar/pcap/start?sensor_id=$SENSOR_ID" \
  -H 'Content-Type: application/json' \
  -d '{"pcap_file":"'$PCAP_FILE'"}' | jq . || true
echo
