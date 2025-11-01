# Grid Heatmap API

## Overview

The Grid Heatmap API aggregates the fine-grained LiDAR background grid (40 rings × 1800 azimuth bins = 72,000 cells) into coarse spatial buckets for analysis and visualization of cell fill and settlement patterns.

## Endpoint

```
GET /api/lidar/grid_heatmap
```

## Query Parameters

| Parameter            | Type   | Default      | Description                                                    |
| -------------------- | ------ | ------------ | -------------------------------------------------------------- |
| `sensor_id`          | string | **required** | Sensor identifier (e.g., "hesai-pandar40p")                    |
| `azimuth_bucket_deg` | float  | 3.0          | Size of each azimuth bucket in degrees (creates 360/N buckets) |
| `settled_threshold`  | uint32 | 5            | Minimum `TimesSeenCount` to consider a cell "settled"          |

## Response Structure

```json
{
  "sensor_id": "hesai-pandar40p",
  "timestamp": "2025-10-31T17:30:00Z",
  "grid_params": {
    "total_rings": 40,
    "total_azimuth_bins": 1800,
    "azimuth_bin_resolution_deg": 0.2,
    "total_cells": 72000
  },
  "heatmap_params": {
    "azimuth_bucket_deg": 3.0,
    "azimuth_buckets": 120,
    "ring_buckets": 40,
    "settled_threshold": 5,
    "cells_per_bucket": 15
  },
  "summary": {
    "total_filled": 58234,
    "total_settled": 52100,
    "total_frozen": 1234,
    "fill_rate": 0.809,
    "settle_rate": 0.724
  },
  "buckets": [
    {
      "ring": 0,
      "azimuth_deg_start": 0.0,
      "azimuth_deg_end": 3.0,
      "total_cells": 15,
      "filled_cells": 14,
      "settled_cells": 12,
      "frozen_cells": 2,
      "mean_times_seen": 8.5,
      "mean_range_meters": 25.3,
      "min_range_meters": 22.1,
      "max_range_meters": 28.7
    }
    // ... 4800 total buckets (40 rings × 120 azimuth buckets)
  ]
}
```

## Field Descriptions

### Grid Params

- `total_rings`: Number of elevation rings in the sensor
- `total_azimuth_bins`: Number of fine azimuth bins (determines angular resolution)
- `azimuth_bin_resolution_deg`: Angular resolution per fine bin (360° / total_azimuth_bins)
- `total_cells`: Total number of fine cells (rings × azimuth_bins)

### Heatmap Params

- `azimuth_bucket_deg`: Requested azimuth bucket size
- `azimuth_buckets`: Number of coarse azimuth buckets (360 / azimuth_bucket_deg)
- `ring_buckets`: Number of ring buckets (same as total_rings)
- `settled_threshold`: TimesSeenCount threshold for settled classification
- `cells_per_bucket`: Number of fine cells aggregated per coarse bucket

### Summary

- `total_filled`: Number of cells with `TimesSeenCount > 0`
- `total_settled`: Number of cells with `TimesSeenCount >= settled_threshold`
- `total_frozen`: Number of cells currently frozen (dynamic obstacle protection)
- `fill_rate`: Fraction of total cells that are filled (total_filled / total_cells)
- `settle_rate`: Fraction of total cells that are settled (total_settled / total_cells)

### Bucket Fields

- `ring`: Ring index (0 to rings-1)
- `azimuth_deg_start`: Starting azimuth angle for this bucket
- `azimuth_deg_end`: Ending azimuth angle for this bucket
- `total_cells`: Number of fine cells in this bucket
- `filled_cells`: Number of cells with TimesSeenCount > 0
- `settled_cells`: Number of cells with TimesSeenCount >= threshold
- `frozen_cells`: Number of currently frozen cells
- `mean_times_seen`: Average TimesSeenCount for filled cells in this bucket
- `mean_range_meters`: Average range for filled cells in this bucket
- `min_range_meters`: Minimum range for filled cells (0 if no filled cells)
- `max_range_meters`: Maximum range for filled cells (0 if no filled cells)

## Usage Examples

### Basic Usage

```bash
curl "http://localhost:8081/api/lidar/grid_heatmap?sensor_id=hesai-pandar40p"
```

### Custom Bucket Size (6-degree buckets)

```bash
curl "http://localhost:8081/api/lidar/grid_heatmap?sensor_id=hesai-pandar40p&azimuth_bucket_deg=6.0"
```

### Higher Settled Threshold

```bash
curl "http://localhost:8081/api/lidar/grid_heatmap?sensor_id=hesai-pandar40p&settled_threshold=10"
```

### Extract Summary with jq

```bash
curl -s "http://localhost:8081/api/lidar/grid_heatmap?sensor_id=hesai-pandar40p" | jq '.summary'
```

### Find Buckets with Low Settlement

```bash
curl -s "http://localhost:8081/api/lidar/grid_heatmap?sensor_id=hesai-pandar40p" | \
  jq '.buckets[] | select(.filled_cells > 0 and .settled_cells == 0) | {ring, azimuth_deg_start, filled_cells}'
```

## Testing

Run the test script:

```bash
./tools/test_grid_heatmap.sh
```

Or with custom parameters:

```bash
MONITOR_URL=http://localhost:8081 \
SENSOR_ID=hesai-pandar40p \
AZIMUTH_BUCKET=6.0 \
SETTLED_THRESHOLD=10 \
./tools/test_grid_heatmap.sh
```

## Performance

- **Response Time**: ~5-20ms for 72,000 cells → 4,800 buckets
- **Memory**: Minimal additional allocation (reuses grid lock)
- **Concurrency**: Safe for concurrent requests (read-only grid access with RLock)

## Use Cases

1. **Spatial Pattern Analysis**: Identify regions of the grid that aren't filling or settling properly
2. **Parameter Tuning**: Visualize how different noise/closeness/neighbor parameters affect grid settlement
3. **Diagnostic Visualization**: Create heatmaps showing filled vs settled cells across the sensor's field of view
4. **Anomaly Detection**: Find unexpected patterns in grid population (e.g., specific azimuth ranges with low settlement)
5. **Temporal Analysis**: Track grid settlement progress over time during warmup or parameter changes

## Related Endpoints

- `GET /api/lidar/grid_status` - Simple aggregate statistics (faster, less detailed)
- `POST /api/lidar/grid_reset` - Reset grid for A/B testing
- `GET /api/lidar/acceptance` - Acceptance rate metrics by range bucket

## Implementation Details

- **Location**:

  - Backend: `internal/lidar/background.go` (`GetGridHeatmap()`)
  - HTTP Handler: `internal/lidar/monitor/webserver.go` (`handleGridHeatmap()`)
  - Tests: `internal/lidar/background_heatmap_test.go`

- **Thread Safety**: Uses read lock (RLock) on grid, safe for concurrent access
- **Aggregation**: Buckets fine cells by (ring, azimuth_range), computes statistics per bucket
- **Edge Cases**: Handles nil manager/grid, empty buckets, unfilled cells gracefully
