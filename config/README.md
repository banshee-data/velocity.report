# Configuration System

This directory contains configuration files for tuning the velocity.report system.

## Maths Cross-Reference

For a direct mapping between config keys and the maths/algorithm docs, see:

- [`README.maths.md`](README.maths.md)
- [`../docs/maths/README.md`](../docs/maths/README.md)

## LiDAR Tuning Configuration

`tuning.defaults.json` is the **single source of truth** for all tuning parameters. The Go binary **requires** a valid configuration file at startup — there are **no hardcoded fallback defaults** in the codebase. If the file cannot be loaded or is missing required keys, the process will not start.

### Usage

```bash
# Uses the default config file (config/tuning.defaults.json)
./velocity-report --enable-lidar

# Uses a custom config file
./velocity-report --config config/tuning.custom.json --enable-lidar
```

### Configuration Structure

The configuration uses a flat JSON schema. **All keys are required** — the file must specify every parameter. The same schema is used by the `/api/lidar/params` endpoint for runtime updates.

```json
{
  "background_update_fraction": 0.02,
  "closeness_multiplier": 8.0,
  "safety_margin_meters": 0.4,
  "noise_relative": 0.04,
  "neighbor_confirmation_count": 7,
  "seed_from_first": true,
  "warmup_duration_nanos": 30000000000,
  "warmup_min_frames": 100,
  "post_settle_update_fraction": 0,
  "enable_diagnostics": false,
  "foreground_dbscan_eps": 0.8,
  "foreground_min_cluster_points": 5,
  "buffer_timeout": "500ms",
  "min_frame_points": 1000,
  "flush_interval": "60s",
  "background_flush": false,
  "gating_distance_squared": 9.21,
  "process_noise_pos": 1.0,
  "process_noise_vel": 5.0,
  "measurement_noise": 0.3,
  "occlusion_cov_inflation": 0.5,
  "hits_to_confirm": 3,
  "max_misses": 3,
  "max_misses_confirmed": 15,
  "max_tracks": 100,
  "height_band_floor": -2.8,
  "height_band_ceiling": 1.5,
  "remove_ground": true,
  "max_cluster_diameter": 12.0,
  "min_cluster_diameter": 0.05,
  "max_cluster_aspect_ratio": 15.0,
  "max_reasonable_speed_mps": 30.0,
  "max_position_jump_meters": 5.0,
  "max_predict_dt": 0.5,
  "max_covariance_diag": 100.0,
  "min_points_for_pca": 4,
  "obb_heading_smoothing_alpha": 0.08,
  "obb_aspect_ratio_lock_threshold": 0.25,
  "max_track_history_length": 200,
  "max_speed_history_length": 100,
  "merge_size_ratio": 2.5,
  "split_size_ratio": 0.3,
  "deleted_track_grace_period": "5s",
  "min_observations_for_classification": 5
}
```

### Key Order Consistency

Key order is validated in CI across:

- `internal/config/tuning.go` (`TuningConfig` JSON tag order)
- `config/tuning*.json`
- this README JSON example block

Use:

```bash
# check only (CI)
make config-order-check

# rewrite targets to canonical order
make config-order-sync
```

### Parameters

#### Background Model

Controls how the background (static scene) model is built and updated. The model classifies each LiDAR return as background or foreground by comparing measured range to an exponential moving average (EMA).

| Key                           | Type    | Default     | Description                                                                                                                                                                                                                                                 |
| ----------------------------- | ------- | ----------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `background_update_fraction`  | float64 | 0.02        | EMA blend factor for background range updates. Each frame, the stored range moves toward the measurement by this fraction. Higher = faster adaptation; lower = more stable. Range: (0, 1].                                                                  |
| `closeness_multiplier`        | float64 | 8.0         | Multiplier for the closeness threshold. A point is background if its range falls within: `average ± closeness_multiplier × max(noise_relative × range, safety_margin)`. Higher = more permissive (fewer foreground); lower = more sensitive. Range: (0, ∞). |
| `safety_margin_meters`        | float64 | 0.4         | Additive safety margin (metres) for the closeness threshold. Ensures a minimum acceptance window at close range where `noise_relative × range` is small. Range: [0, ∞).                                                                                     |
| `noise_relative`              | float64 | 0.04        | Fractional noise estimate relative to measured range. 0.04 = 4% of range (at 50 m → ±2 m noise band). Range: [0, 1].                                                                                                                                        |
| `neighbor_confirmation_count` | int     | 7           | Number of neighbouring polar cells that must also classify a point as foreground before the centre cell is confirmed. Higher = fewer false positives; lower = higher recall. Range: [0, 8].                                                                 |
| `seed_from_first`             | bool    | true        | Seed each background cell from its first observation. When true, warmup converges faster; when false, the EMA starts from zero.                                                                                                                             |
| `warmup_duration_nanos`       | int64   | 30000000000 | Duration (nanoseconds) of the warmup phase during which foreground detections are suppressed. 30 000 000 000 ns = 30 s.                                                                                                                                     |
| `warmup_min_frames`           | int     | 100         | Minimum frames that must elapse during warmup regardless of wall-clock time.                                                                                                                                                                                |
| `post_settle_update_fraction` | float64 | 0           | EMA alpha applied after the settling period. Set to 0 to freeze the background model once settled (useful for fixed scenes).                                                                                                                                |
| `enable_diagnostics`          | bool    | false       | Enable detailed per-cell background diagnostics. Generates significant log output; development only.                                                                                                                                                        |

#### Foreground Clustering (DBSCAN)

After foreground extraction, world-frame points are clustered via DBSCAN. These parameters control how points are grouped into object clusters.

| Key                             | Type    | Default | Description                                                                                                                                                                                                                                           |
| ------------------------------- | ------- | ------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `foreground_dbscan_eps`         | float64 | 0.8     | DBSCAN neighbourhood radius (metres). Two points closer than this (2D Euclidean, X/Y) are neighbours. **Must be ≥0.5** to avoid catastrophic cluster fragmentation — values below this cause vehicles to split into many sub-clusters. Range: (0, ∞). |
| `foreground_min_cluster_points` | int     | 5       | Minimum points to form a cluster. Smaller clusters are discarded as noise. Lower values detect sparser/more distant objects but admit more noise. Range: [1, ∞).                                                                                      |

#### Frame Builder

| Key                | Type   | Default | Description                                                                                                                 |
| ------------------ | ------ | ------- | --------------------------------------------------------------------------------------------------------------------------- |
| `buffer_timeout`   | string | "500ms" | Maximum time to wait for additional packets before finalising an incomplete frame. Go duration string (e.g. "250ms", "1s"). |
| `min_frame_points` | int    | 1000    | Minimum LiDAR returns required to accept a frame. Frames with fewer points are discarded.                                   |

#### Background Grid Persistence

| Key                | Type   | Default | Description                                                                              |
| ------------------ | ------ | ------- | ---------------------------------------------------------------------------------------- |
| `flush_interval`   | string | "60s"   | Interval between periodic background-grid snapshots to the database. Go duration string. |
| `background_flush` | bool   | false   | Master switch for periodic flushing. When false, no snapshots are written.               |

#### Kalman Tracker

Constant-velocity Kalman filter with 2D state [x, y, vx, vy]. Cluster centroids are used as position measurements.

| Key                       | Type    | Default | Description                                                                                                                                                                                              |
| ------------------------- | ------- | ------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `gating_distance_squared` | float64 | 9.21    | Squared Mahalanobis distance threshold for cluster-to-track association. Associations exceeding this are forbidden in the Hungarian assignment. 9.21 = χ²(2, 0.99) — 99% confidence gate. Range: (0, ∞). |
| `process_noise_pos`       | float64 | 1.0     | Position process noise (m²/s), dt-normalised. Actual noise added is `value × dt`. Controls how quickly position uncertainty grows during prediction.                                                     |
| `process_noise_vel`       | float64 | 5.0     | Velocity process noise (m²/s³), dt-normalised. Controls how quickly velocity uncertainty grows, allowing tracking of accelerating objects.                                                               |
| `measurement_noise`       | float64 | 0.3     | Measurement noise (m²). Models uncertainty in cluster centroid positions. Higher = trust predictions more; lower = trust measurements more.                                                              |
| `occlusion_cov_inflation` | float64 | 0.5     | Extra covariance inflation per frame when a confirmed track has no matching cluster. Widens the gating ellipse for easier re-association.                                                                |
| `hits_to_confirm`         | int     | 3       | Consecutive successful associations required to promote a track from tentative to confirmed.                                                                                                             |
| `max_misses`              | int     | 3       | Consecutive missed associations before a **tentative** track is deleted.                                                                                                                                 |
| `max_misses_confirmed`    | int     | 15      | Consecutive missed associations before a **confirmed** track is deleted. Higher values allow coasting through brief occlusions. At 10 Hz: 15 frames = 1.5 s.                                             |
| `max_tracks`              | int     | 100     | Maximum number of concurrent tracked objects.                                                                                                                                                            |

#### Height Band Filter

Vertical (Z-axis) band filter applied after foreground extraction to remove ground-plane and overhead-structure returns before clustering. Bounds are expressed in the **input coordinate frame** — when no sensor→world pose transform is applied (identity pose), Z=0 is the sensor's horizontal plane. For a sensor mounted ≈ 3 m above road level, ground sits at approximately Z = −3.0 m.

| Key                   | Type    | Default | Description                                                                                                                                                       |
| --------------------- | ------- | ------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `height_band_floor`   | float64 | −2.8    | Lower Z bound (metres, sensor frame). Points below this are assumed to be road surface returns. −2.8 ≈ 0.2 m above ground for a 3 m mount. Range: (−∞, 0].        |
| `height_band_ceiling` | float64 | 1.5     | Upper Z bound (metres, sensor frame). Points above this are assumed to be overhead structures. +1.5 allows objects extending ≈ 1.5 m above sensor. Range: [0, ∞). |
| `remove_ground`       | bool    | true    | Master switch for vertical height band filtering. When false, all points pass through to clustering regardless of Z value.                                        |

### Creating Custom Configurations

1. Copy `tuning.defaults.json` to a new file (e.g. `tuning.custom.json`)
2. Modify the values as needed — **all keys must be present**
3. Use the `--config` flag:

```bash
./velocity-report --config config/tuning.custom.json --enable-lidar
```

### No Hardcoded Defaults

The Go binary contains **no hardcoded parameter values**. Every tuning parameter is loaded exclusively from the configuration file at startup. If the file is missing, unreadable, or incomplete (missing required keys), the process exits with an error.

This design ensures:

- A single source of truth (`tuning.defaults.json`)
- No hidden parameter divergence between code and config
- Full reproducibility — the config file completely determines system behaviour

Runtime updates can be made via the `/api/lidar/params` endpoint, but the initial configuration must come from the file.
