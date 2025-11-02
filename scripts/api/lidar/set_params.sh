#!/usr/bin/env bash
# Update background parameters via the local monitor API.
# Usage: ./set_params.sh <sensor_id> <json_params>
# Example: ./set_params.sh hesai-pandar40p '{"noise_relative": 0.15, "closeness_multiplier": 2.5}'
set -euo pipefail

SENSOR_ID=${1:-}
JSON_PARAMS=${2:-}

if [ -z "$SENSOR_ID" ] || [ -z "$JSON_PARAMS" ]; then
  echo "Usage: $0 <sensor_id> <json_params>"
  echo "Example: $0 hesai-pandar40p '{\"noise_relative\": 0.15, \"closeness_multiplier\": 2.5}'"
  exit 1
fi

echo "POST /api/lidar/params?sensor_id=$SENSOR_ID ->"
curl -s -X POST "http://127.0.0.1:8081/api/lidar/params?sensor_id=$SENSOR_ID" \
  -H 'Content-Type: application/json' \
  -d "$JSON_PARAMS" | jq .
echo
