# Grid Heatmap Visualization Guide

## Overview

The grid heatmap visualization tools create spatial analysis plots from the `/api/lidar/grid_heatmap` endpoint, showing patterns of filled, settled, and unsettled cells across the LiDAR sensor's field of view.

## Quick Start

### Using Makefile (Recommended)

```bash
# Basic polar heatmap (default: unsettled_ratio)
make plot-grid-heatmap

# With custom metric
make plot-grid-heatmap METRIC=fill_rate

# Polar + Cartesian views
make plot-grid-heatmap POLAR=true CARTESIAN=true

# All views combined
make plot-grid-heatmap COMBINED=true

# Custom server URL and sensor
make plot-grid-heatmap URL=http://192.168.1.100:8081 SENSOR=my-sensor
```

### Direct Python Usage

```bash
# Polar heatmap (ring vs azimuth)
.venv/bin/python3 tools/grid-heatmap/plot_grid_heatmap.py --url http://localhost:8081 --polar

# Cartesian heatmap (X-Y spatial)
.venv/bin/python3 tools/grid-heatmap/plot_grid_heatmap.py --url http://localhost:8081 --cartesian

# Combined multi-metric view
.venv/bin/python3 tools/grid-heatmap/plot_grid_heatmap.py --url http://localhost:8081 --combined

# Custom metric
.venv/bin/python3 tools/grid-heatmap/plot_grid_heatmap.py --metric fill_rate --polar

# All options
.venv/bin/python3 tools/grid-heatmap/plot_grid_heatmap.py \
  --url http://localhost:8081 \
  --sensor hesai-pandar40p \
  --azimuth-bucket 3.0 \
  --settled-threshold 5 \
  --metric unsettled_ratio \
  --polar --cartesian --combined \
  --output my_heatmap.png \
  --dpi 200
```

## Makefile Parameters

| Parameter           | Default               | Description                   |
| ------------------- | --------------------- | ----------------------------- |
| `URL`               | http://localhost:8081 | Monitor server URL            |
| `SENSOR`            | hesai-pandar40p       | Sensor ID                     |
| `METRIC`            | unsettled_ratio       | Metric to visualize           |
| `OUT`               | grid-heatmap.png      | Output filename               |
| `AZIMUTH_BUCKET`    | 3.0                   | Azimuth bucket size (degrees) |
| `SETTLED_THRESHOLD` | 5                     | Min times seen for settled    |
| `POLAR`             | (auto)                | Create polar heatmap          |
| `CARTESIAN`         | (auto)                | Create cartesian heatmap      |
| `COMBINED`          | (auto)                | Create combined view          |
| `DPI`               | 150                   | Image resolution              |

## Visualization Types

### 1. Polar Heatmap (Ring vs Azimuth)

Shows the sensor-centric view with rings (elevation) on Y-axis and azimuth on X-axis.

**Best for:**

- Identifying sensor-specific patterns
- Finding occlusions or blind spots
- Analyzing azimuthal symmetry
- Quick overview of grid state

**Example:**

```bash
make plot-grid-heatmap POLAR=true METRIC=unsettled_ratio
```

### 2. Cartesian Heatmap (X-Y Spatial)

Shows physical spatial distribution in meters, with sensor at origin (0, 0).

**Best for:**

- Understanding real-world spatial patterns
- Identifying obstacles or occluded regions
- Analyzing range-dependent behavior
- Correlating with environment layout

**Example:**

```bash
make plot-grid-heatmap CARTESIAN=true METRIC=fill_rate
```

### 3. Combined Multi-Metric View

Shows 4 metrics side-by-side: fill_rate, settle_rate, unsettled_ratio, mean_times_seen.

**Best for:**

- Comprehensive analysis
- Comparing different metrics
- Publication/reporting
- Diagnostic overview

**Example:**

```bash
make plot-grid-heatmap COMBINED=true
```

## Available Metrics

### `fill_rate` (0.0 to 1.0)

- **Definition**: Fraction of cells in bucket that have been observed (TimesSeenCount > 0)
- **Color**: Yellow (low) → Green (high)
- **Use**: Identify regions that aren't receiving observations

### `settle_rate` (0.0 to 1.0)

- **Definition**: Fraction of filled cells that are settled (TimesSeenCount ≥ threshold)
- **Color**: Yellow (low) → Green (high)
- **Use**: Find regions taking longer to stabilise

### `unsettled_ratio` (0.0 to 1.0)

- **Definition**: Fraction of filled cells that are NOT settled
- **Color**: Green (low/good) → Red (high/bad)
- **Use**: **Primary diagnostic** - highlights problematic areas

### `mean_times_seen` (0 to N)

- **Definition**: Average observation count for filled cells
- **Color**: Viridis (blue → yellow)
- **Use**: Understand observation frequency across regions

### `frozen_ratio` (0.0 to 1.0)

- **Definition**: Fraction of cells currently frozen (dynamic obstacle protection)
- **Color**: Blues (white → dark blue)
- **Use**: Identify regions with frequent dynamic obstacles

## Examples

### Investigating Non-Settled Cells

```bash
# Generate unsettled ratio heatmap
make plot-grid-heatmap METRIC=unsettled_ratio POLAR=true CARTESIAN=true

# High unsettled ratio in specific regions indicates:
# - Insufficient observations (check fill_rate)
# - High variance (noisy measurements)
# - Parameter tuning needed (closeness_multiplier, noise_relative)
```

### Parameter Tuning Workflow

```bash
# 1. Baseline before parameter change
make plot-grid-heatmap COMBINED=true OUT=before.png

# 2. Apply parameter changes via API
curl -X POST "http://localhost:8081/api/lidar/params?sensor_id=hesai-pandar40p" \
  -d '{"noise_relative": 0.02, "closeness_multiplier": 2.5}'

# 3. Wait for grid to settle
sleep 60

# 4. Generate after heatmap
make plot-grid-heatmap COMBINED=true OUT=after.png

# 5. Compare visually
```

### Temporal Analysis

```bash
# Capture heatmaps over time to track settlement
for i in {1..10}; do
  make plot-grid-heatmap METRIC=settle_rate OUT=settle_t${i}.png
  sleep 30
done

# Create animation (requires imagemagick)
convert -delay 50 settle_t*.png settle_progression.gif
```

### Finding Occlusions

```bash
# Look for consistent gaps in fill_rate
make plot-grid-heatmap METRIC=fill_rate CARTESIAN=true

# Regions with persistently low fill_rate indicate:
# - Physical occlusions (buildings, poles)
# - Sensor blind spots
# - Range limitations
```

## Testing Without Server

Use the test script to generate example plots with synthetic data:

```bash
.venv/bin/python3 tools/grid-heatmap/test_plot_grid_heatmap.py
```

This creates 6 example visualizations demonstrating all plot types and metrics.

## Output Interpretation

### Polar Heatmap Features

- **X-axis (0-360°)**: Azimuth angle around sensor
- **Y-axis (0-40)**: Ring index (elevation angle)
- **Color**: Metric value per coarse bucket
- **Grid lines**: Every 30° azimuth, every 5 rings
- **Summary box**: Total filled/settled/frozen counts

### Cartesian Heatmap Features

- **X/Y axes**: Physical distance in meters
- **Red star**: Sensor location (origin)
- **Point size**: Number of filled cells in bucket
- **Color**: Metric value
- **Circular pattern**: Range-dependent coverage

### Combined View Features

- **4 subplots**: fill_rate, settle_rate, unsettled_ratio, mean_times_seen
- **Shared scale**: Consistent color mapping
- **Compact**: Single image for all metrics

## Performance

- **Fetch time**: ~5-20ms (API call)
- **Plotting time**: ~1-3 seconds per plot
- **File sizes**:
  - Polar: ~100-150 KB
  - Cartesian: ~1-2 MB (many points)
  - Combined: ~150-200 KB

## Tips & Best Practices

### For Initial Grid Analysis

1. Start with **combined view** for overview
2. Focus on **unsettled_ratio** to find problems
3. Use **cartesian view** to correlate with environment

### For Parameter Tuning

1. Baseline with current params
2. Adjust one parameter at a time
3. Compare before/after using same metric
4. Look for improvements in settle_rate

### For Debugging

1. Use **fill_rate** to verify sensor coverage
2. Check **mean_times_seen** for observation frequency
3. Investigate **unsettled_ratio** hot spots
4. Correlate with acceptance rate data

### For Reporting

1. Use **combined view** for comprehensive snapshot
2. Set `DPI=300` for high-quality output
3. Capture at stable state (after warmup)
4. Include timestamp in documentation

## Troubleshooting

### "No response from server"

- Check server is running: `curl http://localhost:8081/api/lidar/grid_status?sensor_id=hesai-pandar40p`
- Verify URL and sensor ID are correct
- Check firewall/network connectivity

### "Invalid JSON response"

- Verify API endpoint is working: `./tools/test_grid_heatmap.sh`
- Check server logs for errors
- Ensure sensor ID exists

### Empty or sparse plots

- Grid may be empty (just started)
- Wait for warmup period (~1-2 minutes)
- Check fill_rate metric to confirm observations arriving

### Python import errors

```bash
# Install missing dependencies
.venv/bin/pip install matplotlib numpy requests
```

## Related Tools

- `tools/grid-heatmap/test_grid_heatmap.sh` - Test API endpoint and display summary
- `tools/grid-heatmap/test_plot_grid_heatmap.py` - Generate example plots with mock data
- `internal/lidar/docs/GRID-HEATMAP-API.md` - API documentation

## Files

- **Script**: `tools/grid-heatmap/plot_grid_heatmap.py`
- **Test**: `tools/grid-heatmap/test_plot_grid_heatmap.py`
- **Makefile target**: `plot-grid-heatmap`
- **Dependencies**: matplotlib, numpy, requests (in `.venv`)
