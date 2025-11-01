#!/usr/bin/env bash
# Export the next completed frame to ASC format via the local monitor API.
# Usage: ./export_next_frame.sh [sensor_id] [output_path]
set -euo pipefail
SENSOR_ID=${1:-${SENSOR_ID:-hesai-pandar40p}}
OUTPUT_PATH=${2:-}

URL="http://127.0.0.1:8081/api/lidar/export_next_frame?sensor_id=$SENSOR_ID"
if [ -n "$OUTPUT_PATH" ]; then
  URL="${URL}&out=${OUTPUT_PATH}"
fi

echo "GET $URL ->"
curl -s "$URL" | jq .
echo
