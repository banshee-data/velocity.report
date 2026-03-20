# LiDAR Config-to-Maths Cross Reference

Primary schema sources:

- `config/tuning.defaults.json`
- `internal/config/tuning.go`

This file lists the canonical v2 leaf paths used by the runtime.

## L1 Packets / Network

- `version`
- `l1.sensor`
- `l1.data_source`
- `l1.udp_port`
- `l1.udp_rcv_buf`
- `l1.forward_port`
- `l1.foreground_forward_port`

## L3 Background Settling

Math references:

- [`../data/maths/background-grid-settling-maths.md`](../data/maths/background-grid-settling-maths.md)
- [`../data/maths/proposals/20260219-unify-l3-l4-settling.md`](../data/maths/proposals/20260219-unify-l3-l4-settling.md)

- `l3.engine`
- `l3.ema_baseline_v1.background_update_fraction`
- `l3.ema_baseline_v1.closeness_multiplier`
- `l3.ema_baseline_v1.safety_margin_metres`
- `l3.ema_baseline_v1.noise_relative`
- `l3.ema_baseline_v1.neighbour_confirmation_count`
- `l3.ema_baseline_v1.seed_from_first`
- `l3.ema_baseline_v1.warmup_duration_nanos`
- `l3.ema_baseline_v1.warmup_min_frames`
- `l3.ema_baseline_v1.post_settle_update_fraction`
- `l3.ema_baseline_v1.enable_diagnostics`
- `l3.ema_baseline_v1.freeze_duration`
- `l3.ema_baseline_v1.freeze_threshold_multiplier`
- `l3.ema_baseline_v1.settling_period`
- `l3.ema_baseline_v1.snapshot_interval`
- `l3.ema_baseline_v1.change_threshold_snapshot`
- `l3.ema_baseline_v1.reacquisition_boost_multiplier`
- `l3.ema_baseline_v1.min_confidence_floor`
- `l3.ema_baseline_v1.locked_baseline_threshold`
- `l3.ema_baseline_v1.locked_baseline_multiplier`
- `l3.ema_baseline_v1.sensor_movement_foreground_threshold`
- `l3.ema_baseline_v1.background_drift_threshold_metres`
- `l3.ema_baseline_v1.background_drift_ratio_threshold`
- `l3.ema_baseline_v1.settling_min_coverage`
- `l3.ema_baseline_v1.settling_max_spread_delta`
- `l3.ema_baseline_v1.settling_min_region_stability`
- `l3.ema_baseline_v1.settling_min_confidence`

## L4 Clustering / Ground Filtering

Math references:

- [`../data/maths/clustering-maths.md`](../data/maths/clustering-maths.md)
- [`../data/maths/ground-plane-maths.md`](../data/maths/ground-plane-maths.md)

- `l4.engine`
- `l4.dbscan_xy_v1.foreground_dbscan_eps`
- `l4.dbscan_xy_v1.foreground_min_cluster_points`
- `l4.dbscan_xy_v1.foreground_max_input_points`
- `l4.dbscan_xy_v1.height_band_floor`
- `l4.dbscan_xy_v1.height_band_ceiling`
- `l4.dbscan_xy_v1.remove_ground`
- `l4.dbscan_xy_v1.max_cluster_diameter`
- `l4.dbscan_xy_v1.min_cluster_diameter`
- `l4.dbscan_xy_v1.max_cluster_aspect_ratio`

## L5 Tracking

Math reference:

- [`../data/maths/tracking-maths.md`](../data/maths/tracking-maths.md)

- `l5.engine`
- `l5.cv_kf_v1.gating_distance_squared`
- `l5.cv_kf_v1.process_noise_pos`
- `l5.cv_kf_v1.process_noise_vel`
- `l5.cv_kf_v1.measurement_noise`
- `l5.cv_kf_v1.occlusion_cov_inflation`
- `l5.cv_kf_v1.hits_to_confirm`
- `l5.cv_kf_v1.max_misses`
- `l5.cv_kf_v1.max_misses_confirmed`
- `l5.cv_kf_v1.max_tracks`
- `l5.cv_kf_v1.max_reasonable_speed_mps`
- `l5.cv_kf_v1.max_position_jump_metres`
- `l5.cv_kf_v1.max_predict_dt`
- `l5.cv_kf_v1.max_covariance_diag`
- `l5.cv_kf_v1.min_points_for_pca`
- `l5.cv_kf_v1.obb_heading_smoothing_alpha`
- `l5.cv_kf_v1.obb_aspect_ratio_lock_threshold`
- `l5.cv_kf_v1.max_track_history_length`
- `l5.cv_kf_v1.max_speed_history_length`
- `l5.cv_kf_v1.merge_size_ratio`
- `l5.cv_kf_v1.split_size_ratio`
- `l5.cv_kf_v1.deleted_track_grace_period`
- `l5.cv_kf_v1.min_observations_for_classification`

## Pipeline

- `pipeline.buffer_timeout`
- `pipeline.min_frame_points`
- `pipeline.flush_interval`
- `pipeline.background_flush`
