#!/usr/bin/env bash
# Fetch lidar grid status from the local monitor API.
# Usage: ./get_grid_status.sh [sensor_id]
set -euo pipefail
SENSOR_ID=${1:-${SENSOR_ID:-hesai-pandar40p}}
echo "GET /api/lidar/grid_status?sensor_id=$SENSOR_ID ->"
curl -s "http://127.0.0.1:8081/api/lidar/grid_status?sensor_id=$SENSOR_ID" | jq .
echo
