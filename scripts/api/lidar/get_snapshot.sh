#!/usr/bin/env bash
# Fetch the latest lidar background snapshot from the local monitor API.
# Usage: ./get_snapshot.sh [sensor_id]
set -euo pipefail
SENSOR_ID=${1:-${SENSOR_ID:-hesai-pandar40p}}
echo "GET /api/lidar/snapshot?sensor_id=$SENSOR_ID ->"
curl -s "http://127.0.0.1:8081/api/lidar/snapshot?sensor_id=$SENSOR_ID" | jq .
echo
