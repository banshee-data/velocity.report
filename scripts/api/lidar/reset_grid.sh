#!/usr/bin/env bash
# Reset lidar grid via the local monitor API.
# Usage: ./reset_grid.sh [sensor_id]
set -euo pipefail
SENSOR_ID=${1:-${SENSOR_ID:-hesai-pandar40p}}
echo "POST /api/lidar/grid_reset?sensor_id=$SENSOR_ID ->"
curl -s -X POST "http://127.0.0.1:8081/api/lidar/grid_reset?sensor_id=$SENSOR_ID" | jq .
echo
