#!/usr/bin/env bash
# Fetch the LiDAR data source status.
# Usage: ./get_status.sh [base_url]
set -euo pipefail
BASE_URL=${1:-http://127.0.0.1:8081}

curl -s "$BASE_URL/api/lidar/status" | jq . || true
echo
