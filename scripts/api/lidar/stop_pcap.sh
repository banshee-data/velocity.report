#!/usr/bin/env bash
# Stop an in-progress PCAP replay via the local monitor API.
# Usage: ./stop_pcap.sh [sensor_id]
set -euo pipefail

SENSOR_ID=${1:-hesai-pandar40p}

echo "Stopping PCAP replay (sensor_id=$SENSOR_ID)"
curl -s -X POST "http://127.0.0.1:8081/api/lidar/pcap/stop?sensor_id=$SENSOR_ID" \
  -H 'Content-Type: application/json' \
  -d '{}' | jq . || true
echo
