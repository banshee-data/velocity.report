```
 ░▒▓██████▓▒░ ░▒▓██████▓▒░░▒▓███████▓▒░░▒▓████████▓▒░▒▓█▓▒░░▒▓██████▓▒░
░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░      ░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░
░▒▓█▓▒░      ░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░      ░▒▓█▓▒░▒▓█▓▒░
░▒▓█▓▒░      ░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░▒▓██████▓▒░ ░▒▓█▓▒░▒▓█▓▒▒▓███▓▒░
░▒▓█▓▒░      ░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░      ░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░
░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░      ░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░
 ░▒▓██████▓▒░ ░▒▓██████▓▒░░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░      ░▒▓█▓▒░░▒▓██████▓▒░
```

This directory holds the canonical LiDAR tuning configuration for
velocity.report.

For ports, thresholds, and fixed constants outside the tuning schema, see
[MAGIC_NUMBERS.md](../MAGIC_NUMBERS.md).

## Schema

The runtime uses a versioned nested schema.

- `$.version`: must equal `2`
- `$.l1`: sensor identity and data-source selection
- `$.l3`, `$.l4`, `$.l5`: each carries an engine selector and exactly one
  engine block matching it
- `$.pipeline`: cross-cutting runtime settings

The runtime rejects:

- missing required fields
- unknown fields
- old flat root keys
- legacy spellings such as `neighbor_*` and `*_meters`

## Usage

```bash
./velocity-report --enable-lidar
./velocity-report --config config/tuning.example.json --enable-lidar

make config-validate TUNING_CONFIG=config/tuning.defaults.json
```

LiDAR networking is process-level CLI configuration:
`--lidar-udp-port`, `--lidar-udp-rcv-buf`, `--lidar-forward-port`, and
`--lidar-foreground-forward-port` are startup-only and do not appear in the
JSON tuning schema or the `/api/lidar/params` hot-reload flow.

## Canonical example

This block mirrors [tuning.example.json](tuning.example.json). CI validates
parity between them.

```json
{
  "version": 2,
  "l1": {
    "sensor": "hesai-pandar40p",
    "data_source": "live"
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

## Runtime updates

`POST /api/lidar/params` accepts partial updates as nested JSON matching
the [tuning.defaults.json](tuning.defaults.json) structure. The server
normalises legacy dot-path keys, but nested format is preferred.

Example:

```json
{
  "l3": {
    "ema_baseline_v1": {
      "noise_relative": 0.03,
      "closeness_multiplier": 2.5
    }
  },
  "l5": {
    "cv_kf_v1": {
      "max_tracks": 120
    }
  }
}
```

Hot-reloadable parameters:

- L3 engine parameters (`l3.ema_baseline_v1.*`)
- L4 DBSCAN: `foreground_dbscan_eps`, `foreground_min_cluster_points`, `foreground_max_input_points`
- L5 tracker parameters (`l5.cv_kf_v1.*`)

`l1.*`, `pipeline.*`, and engine selectors are startup-only.

## Key order

Canonical ordering is derived from `config/tuning.defaults.json`.

```bash
make check-config-order
make sync-config-order
make check-config-maths
```

## Field reference

Schema sources: [tuning.defaults.json](tuning.defaults.json),
[internal/config/tuning.go](../internal/config/tuning.go).

### Root

| Path       | Type   | Primary consumer                                  | Notes                            |
| ---------- | ------ | ------------------------------------------------- | -------------------------------- |
| `version`  | int    | [Validate](../internal/config/tuning_validate.go) | Must equal `2`                   |
| `l1`       | object | [LoadTuningConfig](../internal/config/tuning.go)  | Sensor identity and data source  |
| `l3`       | object | [LoadTuningConfig](../internal/config/tuning.go)  | Background/foreground extraction |
| `l4`       | object | [LoadTuningConfig](../internal/config/tuning.go)  | Clustering and ground filtering  |
| `l5`       | object | [LoadTuningConfig](../internal/config/tuning.go)  | Tracking                         |
| `pipeline` | object | [LoadTuningConfig](../internal/config/tuning.go)  | Cross-cutting runtime settings   |

### L1

| Path             | Type   | Primary consumer                                        | Notes                                  |
| ---------------- | ------ | ------------------------------------------------------- | -------------------------------------- |
| `l1.sensor`      | string | [GetSensor](../internal/config/tuning_accessors.go)     | Sensor identifier                      |
| `l1.data_source` | string | [GetDataSource](../internal/config/tuning_accessors.go) | One of `live`, `pcap`, `pcap_analysis` |

### L3

Maths: [background-grid-settling-maths.md](../data/maths/background-grid-settling-maths.md),
[20260219-unify-l3-l4-settling.md](../data/maths/proposals/20260219-unify-l3-l4-settling.md)

| Path                                                      | Type    | Primary consumer                                                               | Notes                                           |
| --------------------------------------------------------- | ------- | ------------------------------------------------------------------------------ | ----------------------------------------------- |
| `l3.engine`                                               | string  | [(\*L3Config).ActiveCommon](../internal/config/tuning_accessors.go)            | Active L3 engine.                               |
| `l3.ema_baseline_v1.background_update_fraction`           | float64 | [GetBackgroundUpdateFraction](../internal/config/tuning_accessors.go)          | Background EMA alpha.                           |
| `l3.ema_baseline_v1.closeness_multiplier`                 | float64 | [GetClosenessMultiplier](../internal/config/tuning_accessors.go)               | Background acceptance multiplier.               |
| `l3.ema_baseline_v1.safety_margin_metres`                 | float64 | [GetSafetyMarginMetres](../internal/config/tuning_accessors.go)                | Additive safety margin.                         |
| `l3.ema_baseline_v1.noise_relative`                       | float64 | [GetNoiseRelative](../internal/config/tuning_accessors.go)                     | Range-relative noise model.                     |
| `l3.ema_baseline_v1.neighbour_confirmation_count`         | int     | [GetNeighbourConfirmationCount](../internal/config/tuning_accessors.go)        | Spatial confirmation threshold.                 |
| `l3.ema_baseline_v1.seed_from_first`                      | bool    | [GetSeedFromFirst](../internal/config/tuning_accessors.go)                     | Seed cells from first observation.              |
| `l3.ema_baseline_v1.warmup_duration_nanos`                | int64   | [GetWarmupDurationNanos](../internal/config/tuning_accessors.go)               | Warmup duration.                                |
| `l3.ema_baseline_v1.warmup_min_frames`                    | int     | [GetWarmupMinFrames](../internal/config/tuning_accessors.go)                   | Minimum warmup frames.                          |
| `l3.ema_baseline_v1.post_settle_update_fraction`          | float64 | [GetPostSettleUpdateFraction](../internal/config/tuning_accessors.go)          | Background alpha after settling.                |
| `l3.ema_baseline_v1.enable_diagnostics`                   | bool    | [GetEnableDiagnostics](../internal/config/tuning_accessors.go)                 | Verbose background diagnostics.                 |
| `l3.ema_baseline_v1.freeze_duration`                      | string  | [GetFreezeDuration](../internal/config/tuning_accessors.go)                    | Freeze duration after foreground.               |
| `l3.ema_baseline_v1.freeze_threshold_multiplier`          | float64 | [GetFreezeThresholdMultiplier](../internal/config/tuning_accessors.go)         | Freeze trigger multiplier.                      |
| `l3.ema_baseline_v1.settling_period`                      | string  | [GetSettlingPeriod](../internal/config/tuning_accessors.go)                    | Settling period before persistence.             |
| `l3.ema_baseline_v1.snapshot_interval`                    | string  | [GetSnapshotInterval](../internal/config/tuning_accessors.go)                  | Snapshot cadence.                               |
| `l3.ema_baseline_v1.change_threshold_snapshot`            | int     | [GetChangeThresholdSnapshot](../internal/config/tuning_accessors.go)           | Minimum changed cells before snapshot.          |
| `l3.ema_baseline_v1.reacquisition_boost_multiplier`       | float64 | [GetReacquisitionBoostMultiplier](../internal/config/tuning_accessors.go)      | Fast background reacquisition multiplier.       |
| `l3.ema_baseline_v1.min_confidence_floor`                 | int     | [GetMinConfidenceFloor](../internal/config/tuning_accessors.go)                | Minimum confidence preserved during foreground. |
| `l3.ema_baseline_v1.locked_baseline_threshold`            | int     | [GetLockedBaselineThreshold](../internal/config/tuning_accessors.go)           | Observation count needed before baseline lock.  |
| `l3.ema_baseline_v1.locked_baseline_multiplier`           | float64 | [GetLockedBaselineMultiplier](../internal/config/tuning_accessors.go)          | Locked-baseline spread multiplier.              |
| `l3.ema_baseline_v1.sensor_movement_foreground_threshold` | float64 | [GetSensorMovementForegroundThreshold](../internal/config/tuning_accessors.go) | Sensor movement detection ratio.                |
| `l3.ema_baseline_v1.background_drift_threshold_metres`    | float64 | [GetBackgroundDriftThresholdMetres](../internal/config/tuning_accessors.go)    | Drift distance threshold.                       |
| `l3.ema_baseline_v1.background_drift_ratio_threshold`     | float64 | [GetBackgroundDriftRatioThreshold](../internal/config/tuning_accessors.go)     | Drift ratio threshold.                          |
| `l3.ema_baseline_v1.settling_min_coverage`                | float64 | [GetSettlingMinCoverage](../internal/config/tuning_accessors.go)               | Minimum coverage for convergence.               |
| `l3.ema_baseline_v1.settling_max_spread_delta`            | float64 | [GetSettlingMaxSpreadDelta](../internal/config/tuning_accessors.go)            | Maximum spread delta for convergence.           |
| `l3.ema_baseline_v1.settling_min_region_stability`        | float64 | [GetSettlingMinRegionStability](../internal/config/tuning_accessors.go)        | Minimum region stability for convergence.       |
| `l3.ema_baseline_v1.settling_min_confidence`              | float64 | [GetSettlingMinConfidence](../internal/config/tuning_accessors.go)             | Minimum confidence for convergence.             |

### L4

Maths: [clustering-maths.md](../data/maths/clustering-maths.md),
[ground-plane-maths.md](../data/maths/ground-plane-maths.md)

| Path                                            | Type    | Primary consumer                                                        | Notes                                  |
| ----------------------------------------------- | ------- | ----------------------------------------------------------------------- | -------------------------------------- |
| `l4.engine`                                     | string  | [(\*L4Config).ActiveCommon](../internal/config/tuning_accessors.go)     | Active L4 engine.                      |
| `l4.dbscan_xy_v1.foreground_dbscan_eps`         | float64 | [GetForegroundDBSCANEps](../internal/config/tuning_accessors.go)        | DBSCAN epsilon.                        |
| `l4.dbscan_xy_v1.foreground_min_cluster_points` | int     | [GetForegroundMinClusterPoints](../internal/config/tuning_accessors.go) | DBSCAN min points.                     |
| `l4.dbscan_xy_v1.foreground_max_input_points`   | int     | [GetForegroundMaxInputPoints](../internal/config/tuning_accessors.go)   | DBSCAN input cap.                      |
| `l4.dbscan_xy_v1.height_band_floor`             | float64 | [GetHeightBandFloor](../internal/config/tuning_accessors.go)            | Lower Z filter bound.                  |
| `l4.dbscan_xy_v1.height_band_ceiling`           | float64 | [GetHeightBandCeiling](../internal/config/tuning_accessors.go)          | Upper Z filter bound.                  |
| `l4.dbscan_xy_v1.remove_ground`                 | bool    | [GetRemoveGround](../internal/config/tuning_accessors.go)               | Ground filter master switch.           |
| `l4.dbscan_xy_v1.max_cluster_diameter`          | float64 | [GetMaxClusterDiameter](../internal/config/tuning_accessors.go)         | Maximum accepted cluster diameter.     |
| `l4.dbscan_xy_v1.min_cluster_diameter`          | float64 | [GetMinClusterDiameter](../internal/config/tuning_accessors.go)         | Minimum accepted cluster diameter.     |
| `l4.dbscan_xy_v1.max_cluster_aspect_ratio`      | float64 | [GetMaxClusterAspectRatio](../internal/config/tuning_accessors.go)      | Maximum accepted cluster aspect ratio. |

### L5

Maths: [tracking-maths.md](../data/maths/tracking-maths.md)

| Path                                              | Type    | Primary consumer                                                              | Notes                                       |
| ------------------------------------------------- | ------- | ----------------------------------------------------------------------------- | ------------------------------------------- |
| `l5.engine`                                       | string  | [(\*L5Config).ActiveCommon](../internal/config/tuning_accessors.go)           | Active L5 engine.                           |
| `l5.cv_kf_v1.gating_distance_squared`             | float64 | [GetGatingDistanceSquared](../internal/config/tuning_accessors.go)            | Mahalanobis association gate.               |
| `l5.cv_kf_v1.process_noise_pos`                   | float64 | [GetProcessNoisePos](../internal/config/tuning_accessors.go)                  | Position process noise.                     |
| `l5.cv_kf_v1.process_noise_vel`                   | float64 | [GetProcessNoiseVel](../internal/config/tuning_accessors.go)                  | Velocity process noise.                     |
| `l5.cv_kf_v1.measurement_noise`                   | float64 | [GetMeasurementNoise](../internal/config/tuning_accessors.go)                 | Measurement noise.                          |
| `l5.cv_kf_v1.occlusion_cov_inflation`             | float64 | [GetOcclusionCovInflation](../internal/config/tuning_accessors.go)            | Coast-mode covariance inflation.            |
| `l5.cv_kf_v1.hits_to_confirm`                     | int     | [GetHitsToConfirm](../internal/config/tuning_accessors.go)                    | Hits required for confirmation.             |
| `l5.cv_kf_v1.max_misses`                          | int     | [GetMaxMisses](../internal/config/tuning_accessors.go)                        | Tentative-track miss budget.                |
| `l5.cv_kf_v1.max_misses_confirmed`                | int     | [GetMaxMissesConfirmed](../internal/config/tuning_accessors.go)               | Confirmed-track miss budget.                |
| `l5.cv_kf_v1.max_tracks`                          | int     | [GetMaxTracks](../internal/config/tuning_accessors.go)                        | Tracker capacity, validated in `[1,1000]`.  |
| `l5.cv_kf_v1.max_reasonable_speed_mps`            | float64 | [GetMaxReasonableSpeedMps](../internal/config/tuning_accessors.go)            | Speed sanity limit.                         |
| `l5.cv_kf_v1.max_position_jump_metres`            | float64 | [GetMaxPositionJumpMetres](../internal/config/tuning_accessors.go)            | Max plausible position jump.                |
| `l5.cv_kf_v1.max_predict_dt`                      | float64 | [GetMaxPredictDt](../internal/config/tuning_accessors.go)                     | Maximum prediction horizon.                 |
| `l5.cv_kf_v1.max_covariance_diag`                 | float64 | [GetMaxCovarianceDiag](../internal/config/tuning_accessors.go)                | Covariance clamp.                           |
| `l5.cv_kf_v1.min_points_for_pca`                  | int     | [GetMinPointsForPCA](../internal/config/tuning_accessors.go)                  | Minimum points for OBB PCA.                 |
| `l5.cv_kf_v1.obb_heading_smoothing_alpha`         | float64 | [GetOBBHeadingSmoothingAlpha](../internal/config/tuning_accessors.go)         | Heading smoothing factor.                   |
| `l5.cv_kf_v1.obb_aspect_ratio_lock_threshold`     | float64 | [GetOBBAspectRatioLockThreshold](../internal/config/tuning_accessors.go)      | Heading lock threshold.                     |
| `l5.cv_kf_v1.max_track_history_length`            | int     | [GetMaxTrackHistoryLength](../internal/config/tuning_accessors.go)            | Track history capacity.                     |
| `l5.cv_kf_v1.max_speed_history_length`            | int     | [GetMaxSpeedHistoryLength](../internal/config/tuning_accessors.go)            | Speed history capacity.                     |
| `l5.cv_kf_v1.merge_size_ratio`                    | float64 | [GetMergeSizeRatio](../internal/config/tuning_accessors.go)                   | Merge heuristic ratio.                      |
| `l5.cv_kf_v1.split_size_ratio`                    | float64 | [GetSplitSizeRatio](../internal/config/tuning_accessors.go)                   | Split heuristic ratio.                      |
| `l5.cv_kf_v1.deleted_track_grace_period`          | string  | [GetDeletedTrackGracePeriod](../internal/config/tuning_accessors.go)          | Deleted-track reuse window.                 |
| `l5.cv_kf_v1.min_observations_for_classification` | int     | [GetMinObservationsForClassification](../internal/config/tuning_accessors.go) | Minimum observations before classification. |

### Pipeline

| Path                        | Type   | Primary consumer                                             | Notes                                       |
| --------------------------- | ------ | ------------------------------------------------------------ | ------------------------------------------- |
| `pipeline.buffer_timeout`   | string | [GetBufferTimeout](../internal/config/tuning_accessors.go)   | Frame assembly timeout.                     |
| `pipeline.min_frame_points` | int    | [GetMinFramePoints](../internal/config/tuning_accessors.go)  | Minimum points required to process a frame. |
| `pipeline.flush_interval`   | string | [GetFlushInterval](../internal/config/tuning_accessors.go)   | Background snapshot cadence.                |
| `pipeline.background_flush` | bool   | [GetBackgroundFlush](../internal/config/tuning_accessors.go) | Background snapshot master switch.          |
