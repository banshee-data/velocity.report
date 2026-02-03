# Adaptive Region Parameters for Background Grid

## Overview

The LiDAR background grid now supports **adaptive region-based parameters** that automatically segment the sensor's field of view into regions with different settling characteristics, and apply optimized parameters to each region.

This feature addresses the challenge of varying environmental conditions within a single sensor frame: trees and glass have high variance and require looser noise thresholds, while stable surfaces like walls need tighter thresholds for better foreground detection.

## Key Features

- **Automatic Region Identification**: Segments the frame into contiguous regions based on variance characteristics during the settling period
- **Dynamic Parameter Assignment**: Each region gets optimized values for noise tolerance, neighbor confirmation, and settling rate
- **Static After Settling**: Regions are identified once during warmup and remain fixed thereafter (appropriate for static sensors)
- **Configurable Limits**: Maximum 50 regions per frame to ensure performance
- **Debug Visualization**: API endpoint to inspect region boundaries and parameters

## How It Works

### 1. Variance Collection (During Settling)

During the warmup period (configured via `WarmupMinFrames` and `WarmupDurationNanos`), the `RegionManager` tracks the variance (spread) of each cell:

```go
// Variance is collected each frame during settling
rm.UpdateVarianceMetrics(cells)
```

This builds a per-cell variance profile that captures how stable or volatile each part of the frame is.

### 2. Region Identification (At Settling Completion)

When settling completes, regions are automatically identified:

1. **Classify cells by variance**: Cells are grouped into three categories based on percentile thresholds:
   - **Stable** (low variance, < 33rd percentile): e.g., walls, pavement
   - **Variable** (medium variance, 33rd-66th percentile): e.g., foliage at distance
   - **Volatile** (high variance, > 66th percentile): e.g., trees, glass, moving vegetation

2. **Create contiguous regions**: Uses breadth-first search (BFS) to group adjacent cells with the same variance category into connected regions

3. **Merge if needed**: If more than 50 regions are identified, smallest regions are merged with their nearest neighbors

4. **Assign parameters**: Each region receives optimized parameters based on its variance category

### 3. Parameter Application (During Runtime)

Once regions are identified, `ProcessFramePolar` uses region-specific parameters for each cell:

```go
// Look up region for this cell
regionID := g.RegionMgr.GetRegionForCell(cellIdx)
if regionParams := g.RegionMgr.GetRegionParams(regionID); regionParams != nil {
    cellNoiseRel = float64(regionParams.NoiseRelativeFraction)
    cellNeighborConfirm = regionParams.NeighborConfirmationCount
    cellAlpha = float64(regionParams.SettleUpdateFraction)
}
```

## Parameter Scaling by Region Type

| Region Type                  | NoiseRelativeFraction | NeighborConfirmationCount | SettleUpdateFraction | Rationale                                                                                                 |
| ---------------------------- | --------------------- | ------------------------- | -------------------- | --------------------------------------------------------------------------------------------------------- |
| **Stable** (low variance)    | 0.8× base             | base                      | 1.5× base            | Tighter threshold for better foreground detection; faster settling                                        |
| **Variable** (medium)        | 1.0× base             | base                      | 1.0× base            | Standard parameters                                                                                       |
| **Volatile** (high variance) | 2.0× base             | base + 2                  | 0.5× base            | Looser threshold to avoid false positives; more neighbor confirmation; slower settling to handle variance |

Example with base parameters (`NoiseRelativeFraction=0.01`, `NeighborConfirmationCount=3`, `BackgroundUpdateFraction=0.02`):

- **Stable region**: noise=0.008, neighbors=3, alpha=0.03
- **Variable region**: noise=0.01, neighbors=3, alpha=0.02
- **Volatile region**: noise=0.02, neighbors=5, alpha=0.01

## Configuration

No configuration changes are needed. The feature activates automatically when:

1. A `BackgroundManager` is created (via `NewBackgroundManager`)
2. Warmup parameters are configured (`WarmupMinFrames` and/or `WarmupDurationNanos`)
3. The sensor completes its settling period

### Recommended Warmup Settings

For optimal region identification:

```go
params := BackgroundParams{
    WarmupMinFrames:      100,  // ~5 seconds at 20Hz
    WarmupDurationNanos:  int64(30 * time.Second), // 30 seconds max
    // ... other parameters
}
```

The system needs enough frames to build a stable variance profile. Aim for 20-30 seconds of settling time.

## Debug API

### Get Region Information

```bash
# Get region metadata (compact)
curl http://localhost:8081/debug/lidar/background/regions?sensor_id=hesai-01

# Get full region details including cell lists
curl http://localhost:8081/debug/lidar/background/regions?sensor_id=hesai-01&include_cells=true
```

**Response Structure:**

```json
{
  "sensor_id": "hesai-01",
  "timestamp": "2026-01-14T07:00:00Z",
  "identification_complete": true,
  "identification_time": "2026-01-14T06:59:30Z",
  "frames_sampled": 150,
  "region_count": 12,
  "regions": [
    {
      "id": 0,
      "cell_count": 4500,
      "mean_variance": 0.05,
      "params": {
        "noise_relative_fraction": 0.008,
        "neighbor_confirmation_count": 3,
        "settle_update_fraction": 0.03
      },
      "cells": [...]  // Only if include_cells=true
    },
    ...
  ],
  "grid_mapping": [0, 0, 0, 1, 1, 2, ...]  // Maps cell index -> region ID
}
```

### Visualize Regions

Use the grid mapping to visualize which cells belong to which region:

```python
import json
import numpy as np
import matplotlib.pyplot as plt

# Fetch region data
data = requests.get('http://localhost:8081/debug/lidar/background/regions?sensor_id=hesai-01').json()

# Reshape grid mapping to match sensor geometry
rings = 40
azimuth_bins = 1800
region_grid = np.array(data['grid_mapping']).reshape(rings, azimuth_bins)

# Plot heatmap
plt.figure(figsize=(16, 8))
plt.imshow(region_grid, cmap='tab20', aspect='auto')
plt.colorbar(label='Region ID')
plt.xlabel('Azimuth Bin')
plt.ylabel('Ring')
plt.title(f'Background Grid Regions ({data["region_count"]} regions)')
plt.show()
```

## Performance Characteristics

- **Settling Time**: < 30 seconds (typical: 20-25 seconds at 20Hz frame rate)
- **Memory Overhead**: ~40 KB per sensor (variance tracking + region metadata)
- **Runtime Overhead**: Negligible (single region lookup per cell, already in cache)
- **Region Count**: Typically 5-20 regions for outdoor scenes; limited to max 50

## Troubleshooting

### Regions Not Identified

Check that:

1. Warmup parameters are configured (`WarmupMinFrames > 0` or `WarmupDurationNanos > 0`)
2. Sensor has received enough frames
3. Check logs for `[RegionManager] Identified N regions` message

### Too Many/Too Few Regions

- **Too many**: Increase variance classification thresholds (requires code change)
- **Too few**: Decrease thresholds or check if scene actually has uniform variance

### Unexpected Parameter Values

Check the base `BackgroundParams` values. Region parameters are scaled relative to these base values.

## Future Enhancements

1. **Persistence**: Save identified regions to database for faster restart
2. **Manual Override**: API to manually define region boundaries
3. **Dynamic Re-identification**: Optionally re-segment if scene changes (e.g., seasonal)
4. **Per-Region Diagnostics**: Track acceptance rates per region for tuning

## Related Documentation

- [LiDAR Background Grid Standards](../architecture/lidar-background-grid-standards.md)
- [Warmup Trails Fix](../troubleshooting/warmup-trails-fix-20260113.md)
- [PCAP Analysis Mode](pcap-analysis-mode.md)

## References

- Implementation: `internal/lidar/background.go`
- Tests: `internal/lidar/regions_test.go`
- API Endpoint: `internal/lidar/monitor/webserver.go:handleBackgroundRegions`
