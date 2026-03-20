# Configuration System

This directory contains the canonical LiDAR tuning config files used by
`velocity.report`.

## Schema

The runtime now accepts a versioned nested schema.

- `version` must equal `2`
- `l1` holds sensor and network settings
- `l3`, `l4`, and `l5` each contain:
  - `engine`
  - exactly one engine block matching that selector
- `pipeline` holds cross-cutting runtime settings

The runtime rejects:

- missing required fields
- unknown fields
- old flat root keys
- legacy spellings such as `neighbor_*` and `*_meters`

## Usage

```bash
./velocity-report --enable-lidar
./velocity-report --config config/tuning.example.json --enable-lidar

make config-validate CONFIG=config/tuning.defaults.json
make config-migrate IN=config/legacy-flat.json OUT=config/tuning.migrated.json
```

## Canonical Example

```json
{
  "version": 2,
  "l1": {
    "sensor": "hesai-pandar40p",
    "data_source": "live",
    "udp_port": 2369,
    "udp_rcv_buf": 4194304,
    "forward_port": 2368,
    "foreground_forward_port": 2370
  },
  "l3": {
    "engine": "ema_baseline_v1",
    "ema_baseline_v1": {
      "background_update_fraction": 0.02,
      "closeness_multiplier": 3,
      "safety_margin_metres": 0.15,
      "noise_relative": 0.02,
      "neighbour_confirmation_count": 3,
      "seed_from_first": true,
      "warmup_duration_nanos": 30000000000,
      "warmup_min_frames": 100,
      "post_settle_update_fraction": 0,
      "enable_diagnostics": false,
      "freeze_duration": "5s",
      "freeze_threshold_multiplier": 3,
      "settling_period": "5m",
      "snapshot_interval": "2h",
      "change_threshold_snapshot": 100,
      "reacquisition_boost_multiplier": 5,
      "min_confidence_floor": 3,
      "locked_baseline_threshold": 50,
      "locked_baseline_multiplier": 4,
      "sensor_movement_foreground_threshold": 0.2,
      "background_drift_threshold_metres": 0.5,
      "background_drift_ratio_threshold": 0.1,
      "settling_min_coverage": 0.8,
      "settling_max_spread_delta": 0.001,
      "settling_min_region_stability": 0.95,
      "settling_min_confidence": 10
    }
  },
  "l4": {
    "engine": "dbscan_xy_v1",
    "dbscan_xy_v1": {
      "foreground_dbscan_eps": 0.8,
      "foreground_min_cluster_points": 5,
      "foreground_max_input_points": 8000,
      "height_band_floor": -2.8,
      "height_band_ceiling": 1.5,
      "remove_ground": true,
      "max_cluster_diameter": 12,
      "min_cluster_diameter": 0.05,
      "max_cluster_aspect_ratio": 15
    }
  },
  "l5": {
    "engine": "cv_kf_v1",
    "cv_kf_v1": {
      "gating_distance_squared": 36,
      "process_noise_pos": 0.05,
      "process_noise_vel": 0.2,
      "measurement_noise": 0.05,
      "occlusion_cov_inflation": 0.5,
      "hits_to_confirm": 4,
      "max_misses": 3,
      "max_misses_confirmed": 15,
      "max_tracks": 100,
      "max_reasonable_speed_mps": 30,
      "max_position_jump_metres": 5,
      "max_predict_dt": 0.5,
      "max_covariance_diag": 100,
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
  },
  "pipeline": {
    "buffer_timeout": "500ms",
    "min_frame_points": 1000,
    "flush_interval": "60s",
    "background_flush": false
  }
}
```

## Runtime Updates

`POST /api/lidar/params` accepts partial updates in either form:

1. Nested JSON objects
2. Dot-path keys

Example:

```json
{
  "l3.ema_baseline_v1.noise_relative": 0.03,
  "l3.ema_baseline_v1.closeness_multiplier": 2.5,
  "l5.cv_kf_v1.max_tracks": 120
}
```

Runtime updates are limited on this branch to:

- `l3.ema_baseline_v1.*`
- `l4.dbscan_xy_v1.foreground_dbscan_eps`
- `l4.dbscan_xy_v1.foreground_min_cluster_points`
- `l4.dbscan_xy_v1.foreground_max_input_points`
- `l5.cv_kf_v1.*`

`l1.*`, `pipeline.*`, and engine selectors remain startup-only.

## Key Order

Canonical ordering is derived from `config/tuning.defaults.json`.

```bash
make check-config-order
make sync-config-order
make check-config-maths
```

## Field Reference

### Root

| Path       | Type   | Notes                             |
| ---------- | ------ | --------------------------------- |
| `version`  | int    | Must equal `2`.                   |
| `l1`       | object | Sensor/network settings.          |
| `l3`       | object | Background/foreground extraction. |
| `l4`       | object | Clustering and ground filtering.  |
| `l5`       | object | Tracking.                         |
| `pipeline` | object | Cross-cutting runtime settings.   |

### L1

| Path                         | Type   | Default           | Notes                                   |
| ---------------------------- | ------ | ----------------- | --------------------------------------- |
| `l1.sensor`                  | string | `hesai-pandar40p` | Sensor identifier.                      |
| `l1.data_source`             | string | `live`            | One of `live`, `pcap`, `pcap_analysis`. |
| `l1.udp_port`                | int    | `2369`            | LiDAR UDP listen port.                  |
| `l1.udp_rcv_buf`             | int    | `4194304`         | Socket receive buffer in bytes.         |
| `l1.forward_port`            | int    | `2368`            | Raw packet forward port.                |
| `l1.foreground_forward_port` | int    | `2370`            | Foreground packet forward port.         |

### L3

| Path                                                      | Type    | Default           | Notes                                           |
| --------------------------------------------------------- | ------- | ----------------- | ----------------------------------------------- |
| `l3.engine`                                               | string  | `ema_baseline_v1` | Active L3 engine.                               |
| `l3.ema_baseline_v1.background_update_fraction`           | float64 | `0.02`            | Background EMA alpha.                           |
| `l3.ema_baseline_v1.closeness_multiplier`                 | float64 | `3`               | Background acceptance multiplier.               |
| `l3.ema_baseline_v1.safety_margin_metres`                 | float64 | `0.15`            | Additive safety margin.                         |
| `l3.ema_baseline_v1.noise_relative`                       | float64 | `0.02`            | Range-relative noise model.                     |
| `l3.ema_baseline_v1.neighbour_confirmation_count`         | int     | `3`               | Spatial confirmation threshold.                 |
| `l3.ema_baseline_v1.seed_from_first`                      | bool    | `true`            | Seed cells from first observation.              |
| `l3.ema_baseline_v1.warmup_duration_nanos`                | int64   | `30000000000`     | Warmup duration.                                |
| `l3.ema_baseline_v1.warmup_min_frames`                    | int     | `100`             | Minimum warmup frames.                          |
| `l3.ema_baseline_v1.post_settle_update_fraction`          | float64 | `0`               | Background alpha after settling.                |
| `l3.ema_baseline_v1.enable_diagnostics`                   | bool    | `false`           | Verbose background diagnostics.                 |
| `l3.ema_baseline_v1.freeze_duration`                      | string  | `5s`              | Freeze duration after foreground.               |
| `l3.ema_baseline_v1.freeze_threshold_multiplier`          | float64 | `3`               | Freeze trigger multiplier.                      |
| `l3.ema_baseline_v1.settling_period`                      | string  | `5m`              | Settling period before persistence.             |
| `l3.ema_baseline_v1.snapshot_interval`                    | string  | `2h`              | Snapshot cadence.                               |
| `l3.ema_baseline_v1.change_threshold_snapshot`            | int     | `100`             | Minimum changed cells before snapshot.          |
| `l3.ema_baseline_v1.reacquisition_boost_multiplier`       | float64 | `5`               | Fast background reacquisition multiplier.       |
| `l3.ema_baseline_v1.min_confidence_floor`                 | int     | `3`               | Minimum confidence preserved during foreground. |
| `l3.ema_baseline_v1.locked_baseline_threshold`            | int     | `50`              | Observation count needed before baseline lock.  |
| `l3.ema_baseline_v1.locked_baseline_multiplier`           | float64 | `4`               | Locked-baseline spread multiplier.              |
| `l3.ema_baseline_v1.sensor_movement_foreground_threshold` | float64 | `0.2`             | Sensor movement detection ratio.                |
| `l3.ema_baseline_v1.background_drift_threshold_metres`    | float64 | `0.5`             | Drift distance threshold.                       |
| `l3.ema_baseline_v1.background_drift_ratio_threshold`     | float64 | `0.1`             | Drift ratio threshold.                          |
| `l3.ema_baseline_v1.settling_min_coverage`                | float64 | `0.8`             | Minimum coverage for convergence.               |
| `l3.ema_baseline_v1.settling_max_spread_delta`            | float64 | `0.001`           | Maximum spread delta for convergence.           |
| `l3.ema_baseline_v1.settling_min_region_stability`        | float64 | `0.95`            | Minimum region stability for convergence.       |
| `l3.ema_baseline_v1.settling_min_confidence`              | float64 | `10`              | Minimum confidence for convergence.             |

### L4

| Path                                            | Type    | Default        | Notes                                  |
| ----------------------------------------------- | ------- | -------------- | -------------------------------------- |
| `l4.engine`                                     | string  | `dbscan_xy_v1` | Active L4 engine.                      |
| `l4.dbscan_xy_v1.foreground_dbscan_eps`         | float64 | `0.8`          | DBSCAN epsilon.                        |
| `l4.dbscan_xy_v1.foreground_min_cluster_points` | int     | `5`            | DBSCAN min points.                     |
| `l4.dbscan_xy_v1.foreground_max_input_points`   | int     | `8000`         | DBSCAN input cap.                      |
| `l4.dbscan_xy_v1.height_band_floor`             | float64 | `-2.8`         | Lower Z filter bound.                  |
| `l4.dbscan_xy_v1.height_band_ceiling`           | float64 | `1.5`          | Upper Z filter bound.                  |
| `l4.dbscan_xy_v1.remove_ground`                 | bool    | `true`         | Ground filter master switch.           |
| `l4.dbscan_xy_v1.max_cluster_diameter`          | float64 | `12`           | Maximum accepted cluster diameter.     |
| `l4.dbscan_xy_v1.min_cluster_diameter`          | float64 | `0.05`         | Minimum accepted cluster diameter.     |
| `l4.dbscan_xy_v1.max_cluster_aspect_ratio`      | float64 | `15`           | Maximum accepted cluster aspect ratio. |

### L5

| Path                                              | Type    | Default    | Notes                                       |
| ------------------------------------------------- | ------- | ---------- | ------------------------------------------- |
| `l5.engine`                                       | string  | `cv_kf_v1` | Active L5 engine.                           |
| `l5.cv_kf_v1.gating_distance_squared`             | float64 | `36`       | Mahalanobis association gate.               |
| `l5.cv_kf_v1.process_noise_pos`                   | float64 | `0.05`     | Position process noise.                     |
| `l5.cv_kf_v1.process_noise_vel`                   | float64 | `0.2`      | Velocity process noise.                     |
| `l5.cv_kf_v1.measurement_noise`                   | float64 | `0.05`     | Measurement noise.                          |
| `l5.cv_kf_v1.occlusion_cov_inflation`             | float64 | `0.5`      | Coast-mode covariance inflation.            |
| `l5.cv_kf_v1.hits_to_confirm`                     | int     | `4`        | Hits required for confirmation.             |
| `l5.cv_kf_v1.max_misses`                          | int     | `3`        | Tentative-track miss budget.                |
| `l5.cv_kf_v1.max_misses_confirmed`                | int     | `15`       | Confirmed-track miss budget.                |
| `l5.cv_kf_v1.max_tracks`                          | int     | `100`      | Tracker capacity, validated in `[1,1000]`.  |
| `l5.cv_kf_v1.max_reasonable_speed_mps`            | float64 | `30`       | Speed sanity limit.                         |
| `l5.cv_kf_v1.max_position_jump_metres`            | float64 | `5`        | Max plausible position jump.                |
| `l5.cv_kf_v1.max_predict_dt`                      | float64 | `0.5`      | Maximum prediction horizon.                 |
| `l5.cv_kf_v1.max_covariance_diag`                 | float64 | `100`      | Covariance clamp.                           |
| `l5.cv_kf_v1.min_points_for_pca`                  | int     | `4`        | Minimum points for OBB PCA.                 |
| `l5.cv_kf_v1.obb_heading_smoothing_alpha`         | float64 | `0.08`     | Heading smoothing factor.                   |
| `l5.cv_kf_v1.obb_aspect_ratio_lock_threshold`     | float64 | `0.25`     | Heading lock threshold.                     |
| `l5.cv_kf_v1.max_track_history_length`            | int     | `200`      | Track history capacity.                     |
| `l5.cv_kf_v1.max_speed_history_length`            | int     | `100`      | Speed history capacity.                     |
| `l5.cv_kf_v1.merge_size_ratio`                    | float64 | `2.5`      | Merge heuristic ratio.                      |
| `l5.cv_kf_v1.split_size_ratio`                    | float64 | `0.3`      | Split heuristic ratio.                      |
| `l5.cv_kf_v1.deleted_track_grace_period`          | string  | `5s`       | Deleted-track reuse window.                 |
| `l5.cv_kf_v1.min_observations_for_classification` | int     | `5`        | Minimum observations before classification. |

### Pipeline

| Path                        | Type   | Default | Notes                                       |
| --------------------------- | ------ | ------- | ------------------------------------------- |
| `pipeline.buffer_timeout`   | string | `500ms` | Frame assembly timeout.                     |
| `pipeline.min_frame_points` | int    | `1000`  | Minimum points required to process a frame. |
| `pipeline.flush_interval`   | string | `60s`   | Background snapshot cadence.                |
| `pipeline.background_flush` | bool   | `false` | Background snapshot master switch.          |
