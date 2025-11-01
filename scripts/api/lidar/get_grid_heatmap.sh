#!/usr/bin/env bash
# Fetch grid heatmap from the local monitor API.
# Usage: ./get_grid_heatmap.sh [sensor_id] [azimuth_bucket_deg] [settled_threshold]
set -euo pipefail
SENSOR_ID=${1:-${SENSOR_ID:-hesai-pandar40p}}
AZIMUTH_BUCKET=${2:-3.0}
SETTLED_THRESHOLD=${3:-5}
echo "GET /api/lidar/grid_heatmap?sensor_id=$SENSOR_ID&azimuth_bucket_deg=$AZIMUTH_BUCKET&settled_threshold=$SETTLED_THRESHOLD ->"
curl -s "http://127.0.0.1:8081/api/lidar/grid_heatmap?sensor_id=$SENSOR_ID&azimuth_bucket_deg=$AZIMUTH_BUCKET&settled_threshold=$SETTLED_THRESHOLD" | jq .
echo
