# Sweep Tool Guide

The sweep tool (`cmd/sweep`) automates parameter optimisation for the background subtraction and tracking pipelines. It systematically varies configuration values, replays a golden PCAP capture, and records quality metrics — allowing you to identify the settings that best suit your deployment site.

## Prerequisites

1. A running velocity-report server with the LiDAR sensor (or PCAP replay support):
   ```bash
   make dev-go   # Local development server (radar disabled)
   ```
2. A golden PCAP file captured at the deployment site. This file should contain representative traffic (vehicles, pedestrians, cyclists) to test detection quality.
3. The sweep binary:
   ```bash
   go build -o sweep ./cmd/sweep
   ```

## Concepts

### Background Detection Parameters

The background subtraction model classifies each LiDAR point as background (static) or foreground (moving). Three parameters control its sensitivity:

| Parameter                  | API Field                     | Description                                                                                                                                                        | Typical Range |
| -------------------------- | ----------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------- |
| **Noise relative**         | `noise_relative`              | How much variance a background voxel tolerates before a point is classified as foreground. Lower = more sensitive, higher = more tolerant of environmental noise.  | 0.005 – 0.03  |
| **Closeness multiplier**   | `closeness_multiplier`        | Spatial threshold for matching a point to its background voxel. Lower = stricter matching (more points classified as foreground).                                  | 1.5 – 3.0     |
| **Neighbour confirmation** | `neighbor_confirmation_count` | Minimum number of neighbouring voxels that must also consider a point foreground before it is accepted. Higher = fewer false positives but may miss small objects. | 0 – 3         |

### Tracker Parameters

The tracker uses a Kalman filter with Hungarian association to follow objects across frames. Key tuneable parameters:

| Parameter                    | API Field                 | Description                                                                                                     | Typical Range |
| ---------------------------- | ------------------------- | --------------------------------------------------------------------------------------------------------------- | ------------- |
| **Gating distance²**         | `gating_distance_squared` | Maximum squared Mahalanobis distance for associating a cluster with a track. Larger = more permissive matching. | 16.0 – 64.0   |
| **Process noise (position)** | `process_noise_pos`       | How much the model expects position to drift between frames. Higher = more tolerance for erratic motion.        | 0.05 – 0.5    |
| **Measurement noise**        | `measurement_noise`       | Expected variance in the cluster centroid measurement. Higher = smoother but less responsive tracks.            | 0.1 – 0.5     |

## Sweep Modes

### 1. Multi-sweep (default)

Sweeps all combinations of background detection parameters:

```bash
./sweep \
  -monitor http://localhost:8081 \
  -pcap /path/to/golden.pcap \
  -mode multi \
  -noise 0.005,0.01,0.015,0.02,0.025,0.03 \
  -closeness 1.5,2.0,2.5,3.0 \
  -neighbours 0,1,2
```

This produces a CSV with acceptance rates per speed bucket for every combination. Use it to find the noise/closeness/neighbour triple that maximises detection of genuine traffic while minimising false positives.

### 2. Single-variable sweeps

When you want to isolate one parameter while holding the others fixed:

```bash
# Sweep noise only
./sweep -mode noise \
  -noise-start 0.005 -noise-end 0.03 -noise-step 0.005 \
  -fixed-closeness 2.0 -fixed-neighbour 1 \
  -pcap /path/to/golden.pcap

# Sweep closeness only
./sweep -mode closeness \
  -closeness-start 1.5 -closeness-end 3.0 -closeness-step 0.25 \
  -fixed-noise 0.01 -fixed-neighbour 1 \
  -pcap /path/to/golden.pcap

# Sweep neighbour only
./sweep -mode neighbour \
  -neighbour-start 0 -neighbour-end 3 -neighbour-step 1 \
  -fixed-noise 0.01 -fixed-closeness 2.0 \
  -pcap /path/to/golden.pcap
```

### 3. Tracking sweep

Sweeps tracker (Kalman filter) parameters and measures velocity-trail alignment — how well the estimated velocity vector matches the actual direction of travel:

```bash
./sweep -mode tracking \
  -pcap /path/to/golden.pcap \
  -pcap-settle 25s \
  -gating-start 16 -gating-end 64 -gating-step 8 \
  -pnoise-pos-start 0.05 -pnoise-pos-end 0.5 -pnoise-pos-step 0.1 \
  -mnoise-start 0.1 -mnoise-end 0.5 -mnoise-step 0.1
```

The output CSV contains per-combination metrics:

- **mean_alignment_deg**: Mean angular error between velocity vector and displacement heading (lower is better)
- **misalignment_ratio**: Fraction of samples where the error exceeds 45° (lower is better)
- **active_tracks**: Number of active tracks at the end of the replay

## Common Options

| Flag           | Default                 | Description                                  |
| -------------- | ----------------------- | -------------------------------------------- |
| `-monitor`     | `http://localhost:8081` | Base URL for the LiDAR monitor HTTP API      |
| `-sensor`      | `hesai-pandar40p`       | Sensor ID                                    |
| `-pcap`        | _(none)_                | PCAP file to replay for each combination     |
| `-pcap-settle` | `20s`                   | Wait time after PCAP replay before sampling  |
| `-iterations`  | `30`                    | Number of samples per parameter combination  |
| `-interval`    | `2s`                    | Interval between samples                     |
| `-settle-time` | `5s`                    | Wait time after applying params (live mode)  |
| `-seed`        | `true`                  | Seed behaviour: `true`, `false`, or `toggle` |
| `-output`      | auto-generated          | Output CSV filename                          |

## Interpreting Results

### Background Sweep Output

The summary CSV contains one row per parameter combination with columns:

- `noise_relative`, `closeness_multiplier`, `neighbor_confirmation_count` — the parameter values
- Per speed-bucket acceptance rates (e.g. `0-5_accept_rate`, `5-10_accept_rate`, etc.)
- Standard deviations of acceptance rates

**What to look for:**

- High acceptance rates in speed buckets where you expect traffic (e.g. 20–50 km/h for urban roads)
- Low acceptance rates in the 0–5 km/h bucket (which typically represents noise/false detections)
- Low standard deviation (stable detection)

A raw CSV with individual samples is also produced for deeper analysis.

### Tracking Sweep Output

**What to look for:**

- **Low `mean_alignment_deg`** (< 15°) — velocity estimates closely track actual movement
- **Low `misalignment_ratio`** (< 0.1) — fewer than 10% of samples have wildly wrong velocity
- **Reasonable `active_tracks`** — not too many (track splitting) or too few (track merging)

## Applying Optimised Parameters

### Background Parameters

Background parameters are set via the HTTP API. They are **not** currently exposed in the web frontend:

```bash
# Read current parameters
curl -s http://localhost:8081/api/lidar/params?sensor_id=hesai-pandar40p | jq .

# Apply optimised parameters
curl -s -X POST http://localhost:8081/api/lidar/params?sensor_id=hesai-pandar40p \
  -H 'Content-Type: application/json' \
  -d '{
    "noise_relative": 0.015,
    "closeness_multiplier": 2.0,
    "neighbor_confirmation_count": 1,
    "seed_from_first_frame": true
  }'
```

Or use the provided script:

```bash
scripts/api/lidar/set_params.sh
```

### Tracker Parameters

Tracker configuration is now part of the unified `/api/lidar/params` endpoint alongside background parameters:

```bash
# Read all config including tracker params
curl -s http://localhost:8081/api/lidar/params?sensor_id=hesai-pandar40p | jq .

# Apply optimised tracker parameters (partial update — only send fields you want to change)
curl -s -X POST http://localhost:8081/api/lidar/params?sensor_id=hesai-pandar40p \
  -H 'Content-Type: application/json' \
  -d '{
    "gating_distance_squared": 36.0,
    "process_noise_pos": 0.15,
    "measurement_noise": 0.2
  }'
```

### Persisting Configuration

Parameters set via the API are applied at runtime but **not persisted across restarts**. To make changes permanent, update the server configuration or startup flags. The recommended workflow:

1. Run sweeps to identify optimal values
2. Test with the API to confirm in a live session
3. Update your deployment configuration/startup script with the chosen values
4. Restart the server to apply

### Resetting the Background Grid

After changing background parameters, you should reset the background grid to let it rebuild with the new settings:

```bash
curl -s -X POST http://localhost:8081/api/lidar/grid_reset?sensor_id=hesai-pandar40p
```

The sweep tool does this automatically between combinations.

## Workflow Example

A typical optimisation session:

```bash
# 1. Start the server with a known-good PCAP
make dev-go

# 2. Run a broad multi-sweep to narrow the search space
./sweep -mode multi \
  -pcap data/golden-capture.pcap \
  -noise 0.005,0.01,0.015,0.02,0.025,0.03 \
  -closeness 1.5,2.0,2.5,3.0 \
  -neighbours 0,1,2 \
  -iterations 20

# 3. Analyse the CSV — identify the best noise/closeness range
# Look for combinations with high acceptance in traffic buckets
# and low acceptance in the 0-5 km/h noise bucket

# 4. Run a fine-grained sweep around the best values
./sweep -mode noise \
  -noise-start 0.012 -noise-end 0.018 -noise-step 0.001 \
  -fixed-closeness 2.0 -fixed-neighbour 1 \
  -pcap data/golden-capture.pcap

# 5. Run a tracking sweep to optimise tracker parameters
./sweep -mode tracking \
  -pcap data/golden-capture.pcap \
  -pcap-settle 25s \
  -gating-start 20 -gating-end 50 -gating-step 5 \
  -pnoise-pos-start 0.1 -pnoise-pos-end 0.3 -pnoise-pos-step 0.05 \
  -mnoise-start 0.1 -mnoise-end 0.3 -mnoise-step 0.05

# 6. Apply the best combination and visually verify
curl -s -X POST http://localhost:8081/api/lidar/params?sensor_id=hesai-pandar40p \
  -H 'Content-Type: application/json' \
  -d '{
    "noise_relative": 0.015,
    "closeness_multiplier": 2.0,
    "neighbor_confirmation_count": 1,
    "gating_distance_squared": 36.0,
    "process_noise_pos": 0.15,
    "measurement_noise": 0.2
  }'
```

## Analysis Scripts

The repository includes Python scripts for visualising sweep results:

- [data/multisweep-graph/plot_noise_sweep.py](../../data/multisweep-graph/plot_noise_sweep.py) — Plot acceptance rates vs noise values
- [data/multisweep-graph/plot_noise_buckets.py](../../data/multisweep-graph/plot_noise_buckets.py) — Plot per-bucket acceptance distributions
- [data/convergance-neighbour/analyse_convergence.py](../../data/convergance-neighbour/analyse_convergence.py) — Analyse neighbour convergence behaviour
