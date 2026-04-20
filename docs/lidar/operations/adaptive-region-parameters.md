# Adaptive region parameters for background grid

Configuration and operation of adaptive region-based parameters in the LiDAR background grid, which automatically segment the sensor's field of view to apply optimised settling and noise thresholds per region.

## Overview

The LiDAR background grid now supports **adaptive region-based parameters** that automatically segment the sensor's field of view into regions with different settling characteristics, and apply optimised parameters to each region.

This feature addresses the challenge of varying environmental conditions within a single sensor frame: trees and glass have high variance and require looser noise thresholds, while stable surfaces like walls need tighter thresholds for better foreground detection.

## Key features

- **Automatic Region Identification**: Segments the frame into contiguous regions based on variance characteristics during the settling period
- **Dynamic Parameter Assignment**: Each region gets optimised values for noise tolerance, neighbour confirmation, and settling rate
- **Static After Settling**: Regions are identified once during warmup and remain fixed thereafter (appropriate for static sensors)
- **Persistence & Restoration**: Regions are automatically persisted to database with scene hash and restored on subsequent runs from the same location, eliminating the ~30 second settling period
- **Configurable Limits**: Maximum 50 regions per frame to ensure performance
- **Debug Visualisation**: API endpoint to inspect region boundaries and parameters

## How it works

### 1. Variance collection (during settling)

During the warmup period (configured via `WarmupMinFrames` and `WarmupDurationNanos`), the `RegionManager` tracks the variance (spread) of each cell by calling `rm.UpdateVarianceMetrics(cells)` each frame.

This builds a per-cell variance profile that captures how stable or volatile each part of the frame is.

### 2. Region identification (at settling completion)

When settling completes, regions are automatically identified:

1. **Classify cells by variance**: Cells are grouped into three categories based on percentile thresholds:
   - **Stable** (low variance, < 33rd percentile): e.g., walls, pavement
   - **Variable** (medium variance, 33rd-66th percentile): e.g., foliage at distance
   - **Volatile** (high variance, > 66th percentile): e.g., trees, glass, moving vegetation

2. **Create contiguous regions**: Uses breadth-first search (BFS) to group adjacent cells with the same variance category into connected regions

3. **Merge if needed**: If more than 50 regions are identified, smallest regions are merged with their nearest neighbours

4. **Assign parameters**: Each region receives optimised parameters based on its variance category

### 3. Parameter application (during runtime)

Once regions are identified, both foreground extraction paths use region-specific parameters per cell:

- `ProcessFramePolar` (batch path)
- `ProcessFramePolarWithMask` (production runtime mask path)

For each cell, the code looks up `regionID` via `g.RegionMgr.GetRegionForCell(cellIdx)`, then retrieves the region’s `NoiseRelativeFraction`, `NeighborConfirmationCount`, and `SettleUpdateFraction` via `g.RegionMgr.GetRegionParams(regionID)`. If no region params are found, the global defaults apply.

## Parameter scaling by region type

| Region Type                  | NoiseRelativeFraction | NeighborConfirmationCount | SettleUpdateFraction | Rationale                                                                                                  |
| ---------------------------- | --------------------- | ------------------------- | -------------------- | ---------------------------------------------------------------------------------------------------------- |
| **Stable** (low variance)    | 0.8× base             | base                      | 1.5× base            | Tighter threshold for better foreground detection; faster settling                                         |
| **Variable** (medium)        | 1.0× base             | base                      | 1.0× base            | Standard parameters                                                                                        |
| **Volatile** (high variance) | 2.0× base             | base + 2                  | 0.5× base            | Looser threshold to avoid false positives; more neighbour confirmation; slower settling to handle variance |

Example with base parameters (`NoiseRelativeFraction=0.01`, `NeighborConfirmationCount=3`, `BackgroundUpdateFraction=0.02`):

- **Stable region**: noise=0.008, neighbours=3, alpha=0.03
- **Variable region**: noise=0.01, neighbours=3, alpha=0.02
- **Volatile region**: noise=0.02, neighbours=5, alpha=0.01

## Configuration

No configuration changes are needed. The feature activates automatically when:

1. A `BackgroundManager` is created (via `NewBackgroundManager`)
2. Warmup parameters are configured (`WarmupMinFrames` and/or `WarmupDurationNanos`)
3. The sensor completes its settling period

### Recommended warmup settings

For optimal region identification:

| Parameter             | Recommended Value         | Rationale                   |
| --------------------- | ------------------------- | --------------------------- |
| `WarmupMinFrames`     | 100 (~5 seconds at 20 Hz) | Minimum frames for variance |
| `WarmupDurationNanos` | 30 seconds                | Maximum settling window     |

The system needs enough frames to build a stable variance profile. Aim for 20–30 seconds of settling time.

## Debug API

### Get region information

```bash
# Get region metadata (compact)
curl http://localhost:8081/debug/lidar/background/regions?sensor_id=hesai-01

# Get full region details including cell lists
curl http://localhost:8081/debug/lidar/background/regions?sensor_id=hesai-01&include_cells=true
```

**Response Structure:**

The response includes top-level fields: `sensor_id`, `timestamp`, `identification_complete` (bool), `identification_time`, `frames_sampled`, and `region_count`.

The `regions` array contains one entry per region with:

| Field           | Type    | Description                                 |
| --------------- | ------- | ------------------------------------------- |
| `id`            | integer | Region identifier                           |
| `cell_count`    | integer | Number of cells in this region              |
| `mean_variance` | float   | Average variance across member cells        |
| `params`        | object  | Region-specific parameters (see below)      |
| `cells`         | array   | Cell indices (only if `include_cells=true`) |

Region params contain `noise_relative_fraction`, `neighbor_confirmation_count`, and `settle_update_fraction`.

The response also includes `grid_mapping`: an array mapping each cell index to its region ID.

### Visualise regions

Use the grid mapping to visualise which cells belong to which region:

Fetch the region data from the debug API, extract the `grid_mapping` array, reshape it to the sensor geometry (40 rings × 1800 azimuth bins), and render as a heatmap using matplotlib’s `imshow` with a categorical colour map (e.g. `tab20`). The colour bar maps to region IDs. Axis labels: X = Azimuth Bin, Y = Ring.

## Performance characteristics

- **Settling Time**: < 30 seconds (typical: 20-25 seconds at 20Hz frame rate)
- **Memory Overhead**: ~40 KB per sensor (variance tracking + region metadata)
- **Runtime Overhead**: Negligible (single region lookup per cell, already in cache)
- **Region Count**: Typically 5-20 regions for outdoor scenes; limited to max 50

## Troubleshooting

### Regions not identified

Check that:

1. Warmup parameters are configured (`WarmupMinFrames > 0` or `WarmupDurationNanos > 0`)
2. Sensor has received enough frames
3. Check logs for `[RegionManager] Identified N regions` message

### Too many/Too few regions

- **Too many**: Increase variance classification thresholds (requires code change)
- **Too few**: Decrease thresholds or check if scene actually has uniform variance

### Unexpected parameter values

Check the base `BackgroundParams` values. Region parameters are scaled relative to these base values.

## Region persistence & restoration

**Status**: ✅ Implemented (February 2026)

Identified regions are automatically persisted to the database (`lidar_bg_regions` table) along with a scene hash derived from the range/spread distribution. When processing PCAPs from the same location:

1. After ~10 warmup frames, system computes scene signature
2. Looks up matching region snapshot by scene hash
3. Restores regions if a match is found
4. Skips remaining settling period (saves ~20-30 seconds)

This enables immediate foreground detection on subsequent runs from the same sensor location without repeating the full settling process.

**Database Schema**: Migration `000017_create_lidar_bg_regions`

**Implementation**: `internal/lidar/background.go` (lines 1407-1483)

## Future enhancements

1. **Manual Override**: API to manually define region boundaries
2. **Dynamic Re-identification**: Optionally re-segment if scene changes (e.g., seasonal)
3. **Per-Region Diagnostics**: Track acceptance rates per region for tuning

## Related documentation

- [LiDAR Background Grid Standards](../architecture/lidar-background-grid-standards.md)
- [Warmup Trails Fix](../../../DEBUGGING.md#lidar-background-grid--warmup-trails-fixed-january-2026)
- [PCAP Analysis Mode](pcap-analysis-mode.md)

## References

- Implementation: `internal/lidar/background.go`
- Tests: `internal/lidar/regions_test.go`
- API Endpoint: `internal/lidar/monitor/webserver.go:handleBackgroundRegions`
