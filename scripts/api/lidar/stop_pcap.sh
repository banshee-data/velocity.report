#!/usr/bin/env bash
# Switch back to live LiDAR data via the monitor API.
# Usage: ./stop_pcap.sh [sensor_id] [base_url]
set -euo pipefail

SENSOR_ID=${1:-hesai-pandar40p}
BASE_URL=${2:-http://127.0.0.1:8081}

echo "Stopping PCAP replay (sensor_id=$SENSOR_ID) via $BASE_URL"
curl -s "$BASE_URL/api/lidar/pcap/stop?sensor_id=$SENSOR_ID" | jq . || true
echo
