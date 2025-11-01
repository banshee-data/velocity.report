#!/usr/bin/env bash
# Export a snapshot to ASC format via the local monitor API.
# Usage: ./export_snapshot.sh [sensor_id] [snapshot_id] [output_path]
set -euo pipefail
SENSOR_ID=${1:-${SENSOR_ID:-hesai-pandar40p}}
SNAPSHOT_ID=${2:-}
OUTPUT_PATH=${3:-}

URL="http://127.0.0.1:8081/api/lidar/export_snapshot?sensor_id=$SENSOR_ID"
if [ -n "$SNAPSHOT_ID" ]; then
  URL="${URL}&snapshot_id=${SNAPSHOT_ID}"
fi
if [ -n "$OUTPUT_PATH" ]; then
  URL="${URL}&out=${OUTPUT_PATH}"
fi

echo "GET $URL ->"
curl -s "$URL" | jq .
echo
