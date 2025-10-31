#!/usr/bin/env bash
# Fetch lidar grid status from the local monitor API.
set -euo pipefail
echo "GET /api/lidar/grid_status ->"
curl -s "http://127.0.0.1:8081/api/lidar/grid_status" | jq .
echo
