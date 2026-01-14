#!/usr/bin/env bash
# Trigger manual persistence of a background grid snapshot via the local monitor API.
# Usage: ./trigger_persist.sh [sensor_id]
set -euo pipefail
SENSOR_ID=${1:-${SENSOR_ID:-hesai-pandar40p}}
echo "POST /api/lidar/persist?sensor_id=$SENSOR_ID ->"
curl -s -X POST "http://127.0.0.1:8081/api/lidar/persist?sensor_id=$SENSOR_ID" | jq .
echo
