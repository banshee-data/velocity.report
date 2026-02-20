# LiDAR Config-to-Maths Cross Reference

This document maps LiDAR tuning keys to the mathematical subsystems they control.

Primary schema source:

- `config/tuning.defaults.json`
- `internal/config/tuning.go`

Note: this repo does not ship a `tracking.json` file. If your deployment tooling refers to "tracking.json", use the same key schema as `tuning.defaults.json`.

## 1. Background Settling (L3)

Math reference:

- [`docs/maths/background-grid-settling-maths.md`](../docs/maths/background-grid-settling-maths.md)

Keys:

- `background_update_fraction`
- `closeness_multiplier`
- `safety_margin_meters`
- `noise_relative`
- `neighbor_confirmation_count`
- `seed_from_first`
- `warmup_duration_nanos`
- `warmup_min_frames`
- `post_settle_update_fraction`

Code path:

- `internal/lidar/l3grid/config.go` (`BackgroundConfigFromTuning`)
- `internal/lidar/l3grid/foreground.go`
- `internal/lidar/l3grid/background.go`

## 2. Ground Filtering / Ground-Surface Inputs

Math reference:

- [`docs/maths/ground-plane-maths.md`](../docs/maths/ground-plane-maths.md)
- Proposal: [`docs/math/proposal/README.md`](../docs/math/proposal/README.md)

Keys:

- `height_band_floor`
- `height_band_ceiling`
- `remove_ground`

Code path:

- `cmd/radar/radar.go` -> `internal/lidar/pipeline/tracking_pipeline.go` -> `internal/lidar/l4perception/ground.go`

## 3. Clustering (L4)

Math reference:

- [`docs/maths/clustering-maths.md`](../docs/maths/clustering-maths.md)

Keys:

- `foreground_dbscan_eps`
- `foreground_min_cluster_points`
- `max_cluster_diameter`
- `min_cluster_diameter`
- `max_cluster_aspect_ratio`

Code path:

- `internal/lidar/l4perception/cluster.go` (`DefaultDBSCANParams`)
- `internal/lidar/pipeline/tracking_pipeline.go`

## 4. Tracking (L5)

Math reference:

- [`docs/maths/tracking-maths.md`](../docs/maths/tracking-maths.md)

Keys:

- `gating_distance_squared`
- `process_noise_pos`
- `process_noise_vel`
- `measurement_noise`
- `occlusion_cov_inflation`
- `hits_to_confirm`
- `max_misses`
- `max_misses_confirmed`
- `max_tracks`
- `max_reasonable_speed_mps`
- `max_position_jump_meters`
- `max_predict_dt`
- `max_covariance_diag`
- `min_points_for_pca`
- `obb_heading_smoothing_alpha`
- `obb_aspect_ratio_lock_threshold`
- `max_track_history_length`
- `max_speed_history_length`
- `merge_size_ratio`
- `split_size_ratio`
- `deleted_track_grace_period`
- `min_observations_for_classification`

Code path:

- `internal/lidar/l5tracks/tracking.go` (`TrackerConfigFromTuning`)
- wiring in `cmd/radar/radar.go`

## 5. Pipeline Runtime Controls (cross-cutting)

Math/system impact:

- frame completeness and cadence constraints,
- persistence cadence and diagnostics.

Keys:

- `buffer_timeout`
- `min_frame_points`
- `flush_interval`
- `background_flush`
- `enable_diagnostics`

Code path:

- `cmd/radar/radar.go`
- `internal/lidar/l3grid/background_flusher.go`

## 6. Practical Notes

1. The config file is mandatory and complete-key validated in `internal/config/tuning.go`.
2. Some settling constants are still code defaults (not file keys), notably freeze duration and lock/reacquisition defaults in L3.
3. As L4 ground-surface modelling matures, expect additional dedicated ground-plane keys to be added to this mapping.
