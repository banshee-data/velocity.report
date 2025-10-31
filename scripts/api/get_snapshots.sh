#!/usr/bin/env bash
# Fetch recent lidar snapshots from the local monitor API.
set -euo pipefail
echo "GET /api/lidar/snapshots ->"
curl -s "http://127.0.0.1:8081/api/lidar/snapshots" | jq .
echo
