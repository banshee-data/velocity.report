# Grid Cell Analysis API - Implementation Plan

## Objective

Investigate why we have non-settled cells in the background grid by creating an API endpoint that aggregates grid cells into coarse spatial buckets (3-degree azimuth bins) and provides metrics about cell states. This will enable visualization of which spatial regions have cells that are filled but not yet settled.

## Background

Current observations:

- Sweep tests show ~58k-63k nonzero cells consistently
- Grid has 40 rings × 1800 azimuth bins = 72,000 total cells
- We need to understand spatial patterns of filled vs settled cells
- Current `grid_status` endpoint only provides aggregate counts, not spatial distribution

## Proposed Solution

### 1. New Data Structure: Coarse Grid Aggregation

Create 3-degree azimuth buckets (120 buckets for 360°) × 40 rings = 4,800 coarse cells

For each coarse cell, track:

- **Total cells**: Number of fine cells in this bucket (should be 15 cells per bucket: 1800/120)
- **Filled cells**: Cells with `TimesSeenCount > 0`
- **Settled cells**: Cells with `TimesSeenCount >= threshold` (e.g., ≥5)
- **Mean times seen**: Average of `TimesSeenCount` for filled cells
- **Mean range**: Average of `AverageRangeMeters` for filled cells
- **Frozen cells**: Cells with `FrozenUntilUnixNanos > now()`

### 2. API Endpoint Design

**Endpoint**: `GET /api/lidar/grid_heatmap`

**Query Parameters**:

- `sensor_id` (required): Sensor identifier
- `azimuth_bucket_deg` (optional, default=3): Degrees per azimuth bucket
- `settled_threshold` (optional, default=5): Minimum `TimesSeenCount` to be considered "settled"

**Response Structure**:

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
    // ... 4800 total buckets
  ]
}
```

### 3. Visualization Support

The coarse grid data enables multiple visualization types:

**A. X-Y Heatmap (Cartesian)**

- Convert (ring, azimuth) to (x, y) using mean_range
- Color by fill_rate or unsettled ratio
- Shows spatial patterns in physical coordinates

**B. Polar Heatmap**

- Ring (radial) vs Azimuth (angular)
- Color by metrics: fill_rate, settle_rate, mean_times_seen
- Shows sensor-centric view

**C. Time-to-Settle Analysis**

- Track same endpoint over time during grid warmup
- Show progression from empty → filled → settled

## Implementation Steps

### Phase 1: Backend API (Core Functionality)

#### Step 1.1: Add Grid Aggregation Function

**File**: `internal/lidar/background.go`

```go
// CoarseBucket represents aggregated metrics for a spatial bucket
type CoarseBucket struct {
    Ring             int     `json:"ring"`
    AzimuthDegStart  float64 `json:"azimuth_deg_start"`
    AzimuthDegEnd    float64 `json:"azimuth_deg_end"`
    TotalCells       int     `json:"total_cells"`
    FilledCells      int     `json:"filled_cells"`
    SettledCells     int     `json:"settled_cells"`
    FrozenCells      int     `json:"frozen_cells"`
    MeanTimesSeen    float64 `json:"mean_times_seen"`
    MeanRangeMeters  float64 `json:"mean_range_meters"`
    MinRangeMeters   float64 `json:"min_range_meters"`
    MaxRangeMeters   float64 `json:"max_range_meters"`
}

// GridHeatmap represents the full aggregated grid state
type GridHeatmap struct {
    SensorID      string                 `json:"sensor_id"`
    Timestamp     time.Time              `json:"timestamp"`
    GridParams    map[string]interface{} `json:"grid_params"`
    HeatmapParams map[string]interface{} `json:"heatmap_params"`
    Summary       map[string]interface{} `json:"summary"`
    Buckets       []CoarseBucket         `json:"buckets"`
}

// GetGridHeatmap aggregates the fine-grained grid into coarse spatial buckets
func (bm *BackgroundManager) GetGridHeatmap(azimuthBucketDeg float64, settledThreshold uint32) *GridHeatmap {
    if bm == nil || bm.Grid == nil {
        return nil
    }

    g := bm.Grid
    g.mu.RLock()
    defer g.mu.RUnlock()

    // Calculate bucket dimensions
    azBinResDeg := 360.0 / float64(g.AzimuthBins)
    numAzBuckets := int(360.0 / azimuthBucketDeg)
    cellsPerAzBucket := int(azimuthBucketDeg / azBinResDeg)

    // Initialize buckets
    buckets := make([]CoarseBucket, 0, g.Rings*numAzBuckets)
    nowNanos := time.Now().UnixNano()

    totalFilled := 0
    totalSettled := 0
    totalFrozen := 0

    // Aggregate by ring and azimuth bucket
    for ring := 0; ring < g.Rings; ring++ {
        for azBucket := 0; azBucket < numAzBuckets; azBucket++ {
            bucket := CoarseBucket{
                Ring:            ring,
                AzimuthDegStart: float64(azBucket) * azimuthBucketDeg,
                AzimuthDegEnd:   float64(azBucket+1) * azimuthBucketDeg,
                TotalCells:      cellsPerAzBucket,
                MinRangeMeters:  math.MaxFloat64,
            }

            // Aggregate stats from fine cells in this bucket
            startAzBin := azBucket * cellsPerAzBucket
            endAzBin := startAzBin + cellsPerAzBucket

            var sumTimesSeen uint32
            var sumRange float64
            filledCount := 0

            for azBin := startAzBin; azBin < endAzBin && azBin < g.AzimuthBins; azBin++ {
                idx := g.Idx(ring, azBin)
                cell := g.Cells[idx]

                if cell.TimesSeenCount > 0 {
                    bucket.FilledCells++
                    filledCount++
                    sumTimesSeen += cell.TimesSeenCount
                    sumRange += float64(cell.AverageRangeMeters)

                    if cell.TimesSeenCount >= settledThreshold {
                        bucket.SettledCells++
                    }

                    if cell.AverageRangeMeters < float32(bucket.MinRangeMeters) {
                        bucket.MinRangeMeters = float64(cell.AverageRangeMeters)
                    }
                    if cell.AverageRangeMeters > float32(bucket.MaxRangeMeters) {
                        bucket.MaxRangeMeters = float64(cell.AverageRangeMeters)
                    }
                }

                if cell.FrozenUntilUnixNanos > nowNanos {
                    bucket.FrozenCells++
                }
            }

            // Calculate means
            if filledCount > 0 {
                bucket.MeanTimesSeen = float64(sumTimesSeen) / float64(filledCount)
                bucket.MeanRangeMeters = sumRange / float64(filledCount)
            }
            if bucket.FilledCells == 0 {
                bucket.MinRangeMeters = 0
                bucket.MaxRangeMeters = 0
            }

            totalFilled += bucket.FilledCells
            totalSettled += bucket.SettledCells
            totalFrozen += bucket.FrozenCells

            buckets = append(buckets, bucket)
        }
    }

    totalCells := g.Rings * g.AzimuthBins

    return &GridHeatmap{
        SensorID:  g.SensorID,
        Timestamp: time.Now(),
        GridParams: map[string]interface{}{
            "total_rings":                 g.Rings,
            "total_azimuth_bins":          g.AzimuthBins,
            "azimuth_bin_resolution_deg":  360.0 / float64(g.AzimuthBins),
            "total_cells":                 totalCells,
        },
        HeatmapParams: map[string]interface{}{
            "azimuth_bucket_deg":  azimuthBucketDeg,
            "azimuth_buckets":     numAzBuckets,
            "ring_buckets":        g.Rings,
            "settled_threshold":   settledThreshold,
            "cells_per_bucket":    cellsPerAzBucket,
        },
        Summary: map[string]interface{}{
            "total_filled":  totalFilled,
            "total_settled": totalSettled,
            "total_frozen":  totalFrozen,
            "fill_rate":     float64(totalFilled) / float64(totalCells),
            "settle_rate":   float64(totalSettled) / float64(totalCells),
        },
        Buckets: buckets,
    }
}
```

#### Step 1.2: Add HTTP Handler

**File**: `internal/lidar/monitor/webserver.go`

Add route in `SetupRoutes()`:

```go
mux.HandleFunc("/api/lidar/grid_heatmap", ws.handleGridHeatmap)
```

Add handler:

```go
// handleGridHeatmap returns aggregated grid metrics in coarse spatial buckets
// Query params:
//   - sensor_id (required)
//   - azimuth_bucket_deg (optional, default 3.0)
//   - settled_threshold (optional, default 5)
func (ws *WebServer) handleGridHeatmap(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        ws.writeJSONError(w, http.StatusMethodNotAllowed, "only GET supported")
        return
    }

    sensorID := r.URL.Query().Get("sensor_id")
    if sensorID == "" {
        ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
        return
    }

    bm := lidar.GetBackgroundManager(sensorID)
    if bm == nil || bm.Grid == nil {
        ws.writeJSONError(w, http.StatusNotFound, "no background manager for sensor")
        return
    }

    // Parse optional parameters
    azBucketDeg := 3.0
    if azStr := r.URL.Query().Get("azimuth_bucket_deg"); azStr != "" {
        if val, err := strconv.ParseFloat(azStr, 64); err == nil && val > 0 {
            azBucketDeg = val
        }
    }

    settledThreshold := uint32(5)
    if stStr := r.URL.Query().Get("settled_threshold"); stStr != "" {
        if val, err := strconv.ParseUint(stStr, 10, 32); err == nil {
            settledThreshold = uint32(val)
        }
    }

    heatmap := bm.GetGridHeatmap(azBucketDeg, settledThreshold)
    if heatmap == nil {
        ws.writeJSONError(w, http.StatusInternalServerError, "failed to generate heatmap")
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(heatmap)
}
```

### Phase 2: Python Visualization Tools

#### Step 2.1: Heatmap Plotting Script

**File**: `tools/grid-heatmap/plot_grid_heatmap.py`

```python
#!/usr/bin/env python3
"""
Plot grid heatmap visualization from /api/lidar/grid_heatmap endpoint

Usage:
    python3 tools/grid-heatmap/plot_grid_heatmap.py --url http://localhost:8081 --sensor hesai-pandar40p
"""

import argparse
import requests
import numpy as np
import matplotlib.pyplot as plt
from matplotlib.colors import LinearSegmentedColormap

def fetch_heatmap(base_url, sensor_id, azimuth_bucket_deg=3, settled_threshold=5):
    url = f"{base_url}/api/lidar/grid_heatmap"
    params = {
        "sensor_id": sensor_id,
        "azimuth_bucket_deg": azimuth_bucket_deg,
        "settled_threshold": settled_threshold
    }
    resp = requests.get(url, params=params)
    resp.raise_for_status()
    return resp.json()

def plot_polar_heatmap(heatmap, metric='fill_rate', output='grid_heatmap.png'):
    """Plot polar heatmap showing ring vs azimuth"""
    buckets = heatmap['buckets']
    params = heatmap['heatmap_params']

    rings = params['ring_buckets']
    az_buckets = params['azimuth_buckets']

    # Create 2D array for heatmap
    data = np.zeros((rings, az_buckets))

    for bucket in buckets:
        ring = bucket['ring']
        az_idx = int(bucket['azimuth_deg_start'] / params['azimuth_bucket_deg'])

        if metric == 'fill_rate':
            data[ring, az_idx] = bucket['filled_cells'] / bucket['total_cells']
        elif metric == 'settle_rate':
            if bucket['filled_cells'] > 0:
                data[ring, az_idx] = bucket['settled_cells'] / bucket['filled_cells']
        elif metric == 'unsettled_ratio':
            if bucket['filled_cells'] > 0:
                data[ring, az_idx] = (bucket['filled_cells'] - bucket['settled_cells']) / bucket['filled_cells']
        elif metric == 'mean_times_seen':
            data[ring, az_idx] = bucket['mean_times_seen']

    fig, ax = plt.subplots(figsize=(14, 8))

    im = ax.imshow(data, aspect='auto', cmap='viridis', origin='lower',
                   extent=[0, 360, 0, rings])

    ax.set_xlabel('Azimuth (degrees)', fontsize=12)
    ax.set_ylabel('Ring Index', fontsize=12)
    ax.set_title(f"Grid Heatmap: {metric}\n{heatmap['sensor_id']} at {heatmap['timestamp']}",
                 fontsize=14, fontweight='bold')

    cbar = plt.colorbar(im, ax=ax)
    cbar.set_label(metric.replace('_', ' ').title(), fontsize=11)

    # Add summary text
    summary = heatmap['summary']
    summary_text = (f"Filled: {summary['total_filled']:,} ({summary['fill_rate']:.1%})\n"
                   f"Settled: {summary['total_settled']:,} ({summary['settle_rate']:.1%})")
    ax.text(0.02, 0.98, summary_text, transform=ax.transAxes,
            fontsize=10, verticalalignment='top',
            bbox=dict(boxstyle='round', facecolor='white', alpha=0.8))

    plt.tight_layout()
    plt.savefig(output, dpi=150, bbox_inches='tight')
    print(f"Saved {output}")

def plot_cartesian_heatmap(heatmap, metric='unsettled_ratio', output='grid_heatmap_xy.png'):
    """Plot X-Y heatmap in cartesian coordinates"""
    buckets = heatmap['buckets']

    # Convert polar to cartesian
    x_coords = []
    y_coords = []
    values = []

    for bucket in buckets:
        if bucket['filled_cells'] == 0:
            continue

        # Use mean azimuth and mean range
        az_mid = (bucket['azimuth_deg_start'] + bucket['azimuth_deg_end']) / 2
        az_rad = np.radians(az_mid)
        r = bucket['mean_range_meters']

        x = r * np.cos(az_rad)
        y = r * np.sin(az_rad)

        if metric == 'unsettled_ratio':
            if bucket['filled_cells'] > 0:
                val = (bucket['filled_cells'] - bucket['settled_cells']) / bucket['filled_cells']
            else:
                val = 0
        elif metric == 'fill_rate':
            val = bucket['filled_cells'] / bucket['total_cells']
        elif metric == 'mean_times_seen':
            val = bucket['mean_times_seen']
        else:
            val = 0

        x_coords.append(x)
        y_coords.append(y)
        values.append(val)

    fig, ax = plt.subplots(figsize=(12, 10))

    scatter = ax.scatter(x_coords, y_coords, c=values, s=100, cmap='RdYlGn_r',
                        alpha=0.8, edgecolors='k', linewidth=0.5)

    ax.set_xlabel('X (meters)', fontsize=12)
    ax.set_ylabel('Y (meters)', fontsize=12)
    ax.set_title(f"Spatial Heatmap: {metric}\n{heatmap['sensor_id']}",
                 fontsize=14, fontweight='bold')
    ax.set_aspect('equal')
    ax.grid(True, alpha=0.3)

    cbar = plt.colorbar(scatter, ax=ax)
    cbar.set_label(metric.replace('_', ' ').title(), fontsize=11)

    plt.tight_layout()
    plt.savefig(output, dpi=150, bbox_inches='tight')
    print(f"Saved {output}")

def main():
    parser = argparse.ArgumentParser(description="Plot grid heatmap from LiDAR monitor API")
    parser.add_argument('--url', default='http://localhost:8081', help='Monitor base URL')
    parser.add_argument('--sensor', default='hesai-pandar40p', help='Sensor ID')
    parser.add_argument('--azimuth-bucket', type=float, default=3.0, help='Azimuth bucket size in degrees')
    parser.add_argument('--settled-threshold', type=int, default=5, help='Min times seen for settled')
    parser.add_argument('--metric', default='unsettled_ratio',
                       choices=['fill_rate', 'settle_rate', 'unsettled_ratio', 'mean_times_seen'],
                       help='Metric to visualize')
    parser.add_argument('--polar', action='store_true', help='Create polar heatmap')
    parser.add_argument('--cartesian', action='store_true', help='Create cartesian heatmap')
    parser.add_argument('--output', default='grid_heatmap.png', help='Output filename')

    args = parser.parse_args()

    if not args.polar and not args.cartesian:
        args.polar = True  # default

    print(f"Fetching heatmap data from {args.url}...")
    heatmap = fetch_heatmap(args.url, args.sensor, args.azimuth_bucket, args.settled_threshold)

    print(f"Grid: {heatmap['grid_params']['total_rings']} rings × "
          f"{heatmap['grid_params']['total_azimuth_bins']} azimuth bins")
    print(f"Aggregation: {heatmap['heatmap_params']['ring_buckets']} rings × "
          f"{heatmap['heatmap_params']['azimuth_buckets']} azimuth buckets")
    print(f"Summary: {heatmap['summary']['total_filled']:,} filled, "
          f"{heatmap['summary']['total_settled']:,} settled")

    if args.polar:
        plot_polar_heatmap(heatmap, args.metric, args.output)

    if args.cartesian:
        xy_output = args.output.replace('.png', '_xy.png')
        plot_cartesian_heatmap(heatmap, args.metric, xy_output)

if __name__ == '__main__':
    main()
```

#### Step 2.2: Add Makefile Targets

**File**: `Makefile`

```makefile
# Grid heatmap visualization
plot-grid-heatmap:
	@if [ -z "$(URL)" ]; then \
		URL="http://localhost:8081"; \
	fi; \
	SENSOR=$${SENSOR:-hesai-pandar40p}; \
	METRIC=$${METRIC:-unsettled_ratio}; \
	OUT=$${OUT:-grid-heatmap.png}; \
	EXTRA=""; \
	[ "$(POLAR)" = "true" ] && EXTRA="$$EXTRA --polar"; \
	[ "$(CARTESIAN)" = "true" ] && EXTRA="$$EXTRA --cartesian"; \
	echo "Fetching grid heatmap from $$URL for $$SENSOR"; \
	$(VENV_PYTHON) tools/grid-heatmap/plot_grid_heatmap.py --url "$$URL" --sensor "$$SENSOR" --metric "$$METRIC" --output "$$OUT" $$EXTRA
```

### Phase 3: Integration with Sweep Testing

Add heatmap capture to sweep workflows to track spatial patterns across parameter combinations.

#### Step 3.1: Capture Heatmaps During Sweeps

**File**: `cmd/sweep/main.go` (enhancement)

Add flag:

```go
captureHeatmaps := flag.Bool("capture-heatmaps", false, "Capture grid heatmaps during sweep")
```

Add to sampling loop to save heatmap snapshots for each parameter combination.

## Testing Plan

### Unit Tests

1. Test `GetGridHeatmap()` with mock grid data
2. Verify bucket aggregation math
3. Test edge cases (empty grid, partially filled)

### Integration Tests

1. Start monitor with PCAP replay
2. Call `/api/lidar/grid_heatmap` endpoint
3. Verify response structure and metrics
4. Generate plots and visually inspect

### Performance Tests

1. Measure endpoint latency (target: <50ms for 72k cells)
2. Test with concurrent requests
3. Verify no memory leaks during repeated calls

## Success Criteria

1. **API endpoint** returns valid JSON with coarse grid aggregation
2. **Polar heatmap** clearly shows spatial patterns of filled/settled cells
3. **Cartesian heatmap** shows physical location patterns
4. **Performance** meets <50ms response time target
5. **Integration** works seamlessly with existing sweep tooling

## Timeline Estimate

- **Phase 1** (Backend API): 2-3 hours

  - Grid aggregation function: 1 hour
  - HTTP handler: 30 minutes
  - Testing: 1 hour

- **Phase 2** (Visualization): 2-3 hours

  - Plotting script: 1.5 hours
  - Makefile integration: 30 minutes
  - Testing/refinement: 1 hour

- **Phase 3** (Sweep integration): 1-2 hours
  - Sweep modifications: 1 hour
  - Documentation: 30 minutes

**Total**: 5-8 hours

## Files to Create/Modify

### New Files

- `tools/grid-heatmap/plot_grid_heatmap.py` - Visualization script

### Modified Files

- `internal/lidar/background.go` - Add `GetGridHeatmap()` method
- `internal/lidar/monitor/webserver.go` - Add HTTP handler
- `Makefile` - Add plotting target
- `cmd/sweep/main.go` - Optional heatmap capture integration

## Future Enhancements

1. **Temporal tracking**: Store heatmap snapshots over time to analyze settling patterns
2. **Anomaly detection**: Automatically flag regions with abnormal fill/settle patterns
3. **3D visualization**: Interactive web-based viewer for grid state
4. **Comparison mode**: Side-by-side heatmaps for different parameter sets
5. **Export formats**: Support CSV/JSON export for custom analysis
