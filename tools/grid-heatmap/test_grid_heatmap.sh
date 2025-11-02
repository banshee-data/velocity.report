#!/bin/bash
# Test script for grid heatmap API endpoint

set -e

MONITOR_URL="${MONITOR_URL:-http://localhost:8081}"
SENSOR_ID="${SENSOR_ID:-hesai-pandar40p}"
AZIMUTH_BUCKET="${AZIMUTH_BUCKET:-3.0}"
SETTLED_THRESHOLD="${SETTLED_THRESHOLD:-5}"

echo "Testing grid heatmap API endpoint..."
echo "  URL: $MONITOR_URL"
echo "  Sensor: $SENSOR_ID"
echo "  Azimuth bucket: ${AZIMUTH_BUCKET}°"
echo "  Settled threshold: $SETTLED_THRESHOLD"
echo ""

# Build URL with parameters
URL="${MONITOR_URL}/api/lidar/grid_heatmap?sensor_id=${SENSOR_ID}&azimuth_bucket_deg=${AZIMUTH_BUCKET}&settled_threshold=${SETTLED_THRESHOLD}"

echo "Fetching: $URL"
echo ""

# Make request and pretty-print JSON
response=$(curl -s "$URL")

if [ -z "$response" ]; then
    echo "ERROR: No response from server"
    exit 1
fi

# Check if response is valid JSON
if ! echo "$response" | jq . > /dev/null 2>&1; then
    echo "ERROR: Invalid JSON response"
    echo "$response"
    exit 1
fi

# Display summary
echo "=== Summary ==="
echo "$response" | jq -r '.summary | to_entries | .[] | "\(.key): \(.value)"'
echo ""

echo "=== Grid Parameters ==="
echo "$response" | jq -r '.grid_params | to_entries | .[] | "\(.key): \(.value)"'
echo ""

echo "=== Heatmap Parameters ==="
echo "$response" | jq -r '.heatmap_params | to_entries | .[] | "\(.key): \(.value)"'
echo ""

echo "=== Sample Buckets (first 5) ==="
echo "$response" | jq '.buckets[:5]'
echo ""

echo "=== Statistics ==="
total_buckets=$(echo "$response" | jq '.buckets | length')
echo "Total buckets: $total_buckets"

# Find buckets with most filled cells
echo ""
echo "Top 5 buckets by filled cells:"
echo "$response" | jq -r '.buckets | sort_by(-.filled_cells) | .[:5] | .[] | "Ring \(.ring), Az \(.azimuth_deg_start)°-\(.azimuth_deg_end)°: \(.filled_cells)/\(.total_cells) filled, \(.settled_cells) settled, mean_range=\(.mean_range_meters)m"'

echo ""
echo "✓ Test completed successfully"
