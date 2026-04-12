# Config restructure: flat → layer-scoped

- **Status:** Phase 1 complete, Phase 2 adjusted, Phase 2B proposed, Phase 3 pending
- **Schema version:** `2`
- **Motivation:** Support multi-engine algorithm selection (CV, IMM, HDBSCAN),
  layer-isolated evaluation, and coherent parameter grouping.

---

## 1. Why this is a breaking change

The current `tuning.defaults.json` uses a **flat** schema: 44 keys at root
level with no nesting. This worked well for a single-engine pipeline, but the
[velocity-coherent foreground extraction](../../data/maths/proposals/20260220-velocity-coherent-foreground-extraction.md)
proposal introduces:

1. **Full layer coverage**: L1 through L6 are represented in a single
   unified config, including sensor/network (L1), frame assembly (L2),
   classification thresholds (L6), and many previously hardcoded constants.
2. **Engine selection per layer**: L3/L4/L5 each gain an `engine` field that
   selects the algorithm variant (e.g. `cv_kf_v1` vs `imm_cv_ca_v2`).
3. **Optimisation strategy**: sweep/auto-tune gains a strategy profile and
   layer-scoping controls.
4. **Engine-specific blocks**: each engine's full parameter set lives in a
   sub-object keyed by the engine name; the block is mandatory when selected
   and absent otherwise.

This is a **clean break**. The flat schema is retired. There is no dual-read
period, no compatibility shim, no migration path that preserves the old format
at runtime. A `make config-migrate` target converts old files to the new
format for convenience, but the binary only accepts the new schema.

### What breaks

| Component                                               | Impact                                                                   |
| ------------------------------------------------------- | ------------------------------------------------------------------------ |
| `tuning.defaults.json`                                  | Replaced entirely with versioned, layer-scoped format                    |
| `tuning.example.json`, `tuning.optimised.json`          | Same: must be regenerated                                                |
| `TuningConfig` Go struct                                | Replaced with nested sub-structs; no pointer fields; all fields required |
| L6 classification constants                             | 30+ hardcoded `const` values move into `l6` config object                |
| L2 frame assembly constants                             | 6 hardcoded values move into `l2` config object                          |
| L1 sensor/network constants                             | Sensor model, UDP port, data source move into `l1` config object         |
| `/api/lidar/params` endpoint                            | Schema changes; dot-path keys (`l5.cv_kf_v1.process_noise_vel`)          |
| Sweep `SweepParam.Name` references                      | Flat key names become dot-paths                                          |
| `config-order-check` / `config-order-sync`              | Updated for nested key structure                                         |
| `BackgroundConfigFromTuning`, `TrackerConfigFromTuning` | Factory functions read from sub-structs                                  |
| External deployment tooling                             | Must regenerate config files                                             |

---

## 2. Current flat structure (for reference)

44 keys, all at root level. Comment-grouped in [internal/config/tuning.go](../../internal/config/tuning.go)
but with no structural hierarchy in JSON or Go.

44 root-level keys (e.g. `background_update_fraction`, `closeness_multiplier`, … `min_observations_for_classification`), comment-grouped in [internal/config/tuning.go](../../internal/config/tuning.go) but with no structural hierarchy in JSON or Go.

Full key listing and documentation: [`config/CONFIG.md`](../../config/CONFIG.md).

---

## 3. Target nested structure

**Default values:** Canonical defaults are defined once in
`tuning.defaults.json`. This document describes structure, types, and
validation rules only.

### 3.1 Design principles

1. **Versioned.** A root-level `version` field (integer) identifies the schema.
   The binary rejects any file where `version` is absent or does not match the
   expected value. This prevents silent misconfiguration from stale files.
2. **Layer-aligned.** Each key lives under the layer it controls (`l1`
   through `l6`). Cross-cutting keys live under `pipeline`.
3. **Engine-selectable.** Each layer has an `engine` field that names the
   active algorithm. The matching engine block must be present.
4. **Strict within blocks.** Every field inside a present object is
   required: no `omitempty` on data fields, no optional data keys.
   Unknown keys are rejected. The file is either fully valid or fully
   rejected at startup.
5. **All engine params in a block keyed by engine name.** Each engine's
   full parameter set (common + engine-specific) lives in a sub-object
   keyed by the engine name (e.g. `l3.ema_baseline_v1.noise_relative`).
   The block is a complete, self-describing snapshot: every field is
   required when the block is present, enforced by `DisallowUnknownFields`.
6. **Exactly one engine block per layer.** The layer object contains
   `engine` (selector string) plus exactly one engine block matching
   that selector. Non-selected engine blocks must be absent. Switching
   engines means replacing the entire engine block. The block is
   optional at the Go level (pointer + `omitempty`) but mandatory at
   runtime when that engine is selected.

### 3.2 Target structure

Top-level schema. All top-level objects are required. Engine-selectable layers
contain `engine` (selector) plus exactly one engine block.

```
version:        int (must equal 2)
l1:             L1Config (6 fields — see §4)
l2:             L2Config (7 fields — see §4)
l3:             { engine + one of: ema_baseline_v1{26}, ema_track_assist_v2{29} }
l4:             { engine + one of: dbscan_xy_v1{9}, two_stage_mahalanobis_v2{11}, hdbscan_adaptive_v1{11} }
l5:             { engine + one of: cv_kf_v1{23}, imm_cv_ca_v2{27}, imm_cv_ca_rts_eval_v2{28} }
l6:             { engine + one of: rule_based_v1{29} }
pipeline:       PipelineConfig (6 fields — see §4)
optimisation:   OptimisationConfig (3 fields — see §4)
```

### 3.3 Engine-conditional blocks

When a different engine is selected, you replace the engine block entirely.
For example, switching L5 from `cv_kf_v1` to `imm_cv_ca_v2`:

For example, switching L5 from `cv_kf_v1` to `imm_cv_ca_v2`: the `l5.engine` field changes from `"cv_kf_v1"` to `"imm_cv_ca_v2"`, the `l5.cv_kf_v1` block (23 fields) is removed entirely, and a new `l5.imm_cv_ca_v2` block is added containing the same 23 common fields plus four IMM-specific fields (`transition_cv_to_ca`, `transition_ca_to_cv`, `ca_process_noise_acc`, `low_speed_heading_freeze_mps`).

The file is rejected if:

- The engine block matching `engine` is absent
- Any other engine block is present
- Any required field inside the engine block is missing
- Any unknown field inside the engine block is present

See §5 for the full field-count breakdown per engine.

### 3.4 Spelling corrections (applied in this version)

The flat schema predated the British English mandate. The new schema uses
corrected spellings. There is no compatibility mapping: old spellings are
rejected as unknown keys.

| Old flat key (v1)             | New key (v2)                   | Reason          |
| ----------------------------- | ------------------------------ | --------------- |
| `neighbor_confirmation_count` | `neighbour_confirmation_count` | British English |
| `max_position_jump_meters`    | `max_position_jump_metres`     | British English |
| `safety_margin_meters`        | `safety_margin_metres`         | British English |

### 3.5 Complete example (active engines, current production defaults)

This uses the active engine for each layer (`ema_baseline_v1`, `dbscan_xy_v1`,
`cv_kf_v1`, `rule_based_v1`). All engine parameters: common and engine-specific
alike: live inside the engine block. Values shown are current production
defaults. The canonical defaults will be maintained in `tuning.defaults.json` —
this example is for structural reference only.

The complete example below uses the active engine for each layer. All engine parameters — common and engine-specific alike — live inside the engine block. The canonical defaults will be maintained in `tuning.defaults.json`; this table is for structural reference only.

| Path                                                      | Value                         | Notes                       |
| --------------------------------------------------------- | ----------------------------- | --------------------------- |
| `version`                                                 | 2                             | Schema version              |
| **L1**                                                    |                               |                             |
| `l1.sensor`                                               | `"pandar40p"`                 | Sensor model                |
| `l1.data_source`                                          | `"live"`                      | live / pcap / pcap_analysis |
| **L2** (7 fields)                                         |                               |                             |
| `l2.min_azimuth_coverage_deg`                             | 340.0                         |                             |
| `l2.min_frame_points_for_completion`                      | 10000                         |                             |
| `l2.azimuth_tolerance_deg`                                | 10.0                          |                             |
| `l2.max_backfill_delay`                                   | `"100ms"`                     |                             |
| `l2.cleanup_interval`                                     | `"250ms"`                     |                             |
| `l2.frame_buffer_size`                                    | 10                            |                             |
| `l2.frame_channel_capacity`                               | 8                             |                             |
| **L3** — engine `ema_baseline_v1` (26 fields)             |                               |                             |
| `l3.ema_baseline_v1.background_update_fraction`           | 0.02                          |                             |
| `l3.ema_baseline_v1.closeness_multiplier`                 | 3.0                           |                             |
| `l3.ema_baseline_v1.safety_margin_metres`                 | 0.15                          |                             |
| `l3.ema_baseline_v1.noise_relative`                       | 0.02                          |                             |
| `l3.ema_baseline_v1.neighbour_confirmation_count`         | 3                             |                             |
| `l3.ema_baseline_v1.seed_from_first`                      | true                          |                             |
| `l3.ema_baseline_v1.warmup_duration_nanos`                | 30000000000                   | 30 s                        |
| `l3.ema_baseline_v1.warmup_min_frames`                    | 100                           |                             |
| `l3.ema_baseline_v1.post_settle_update_fraction`          | 0                             |                             |
| `l3.ema_baseline_v1.enable_diagnostics`                   | false                         |                             |
| `l3.ema_baseline_v1.freeze_duration`                      | `"5s"`                        |                             |
| `l3.ema_baseline_v1.freeze_threshold_multiplier`          | 3.0                           |                             |
| `l3.ema_baseline_v1.settling_period`                      | `"5m"`                        |                             |
| `l3.ema_baseline_v1.snapshot_interval`                    | `"2h"`                        |                             |
| `l3.ema_baseline_v1.change_threshold_snapshot`            | 100                           |                             |
| `l3.ema_baseline_v1.reacquisition_boost_multiplier`       | 5.0                           |                             |
| `l3.ema_baseline_v1.min_confidence_floor`                 | 3                             |                             |
| `l3.ema_baseline_v1.locked_baseline_threshold`            | 50                            |                             |
| `l3.ema_baseline_v1.locked_baseline_multiplier`           | 4.0                           |                             |
| `l3.ema_baseline_v1.sensor_movement_foreground_threshold` | 0.2                           |                             |
| `l3.ema_baseline_v1.background_drift_threshold_metres`    | 0.5                           |                             |
| `l3.ema_baseline_v1.background_drift_ratio_threshold`     | 0.1                           |                             |
| `l3.ema_baseline_v1.settling_min_coverage`                | 0.8                           |                             |
| `l3.ema_baseline_v1.settling_max_spread_delta`            | 0.001                         |                             |
| `l3.ema_baseline_v1.settling_min_region_stability`        | 0.95                          |                             |
| `l3.ema_baseline_v1.settling_min_confidence`              | 10.0                          |                             |
| **L4** — engine `dbscan_xy_v1` (9 fields)                 |                               |                             |
| `l4.dbscan_xy_v1.foreground_dbscan_eps`                   | 0.8                           |                             |
| `l4.dbscan_xy_v1.foreground_min_cluster_points`           | 5                             |                             |
| `l4.dbscan_xy_v1.foreground_max_input_points`             | 8000                          |                             |
| `l4.dbscan_xy_v1.height_band_floor`                       | −2.8                          |                             |
| `l4.dbscan_xy_v1.height_band_ceiling`                     | 1.5                           |                             |
| `l4.dbscan_xy_v1.remove_ground`                           | true                          |                             |
| `l4.dbscan_xy_v1.max_cluster_diameter`                    | 12.0                          |                             |
| `l4.dbscan_xy_v1.min_cluster_diameter`                    | 0.05                          |                             |
| `l4.dbscan_xy_v1.max_cluster_aspect_ratio`                | 15.0                          |                             |
| **L5** — engine `cv_kf_v1` (23 fields)                    |                               |                             |
| `l5.cv_kf_v1.gating_distance_squared`                     | 36.0                          |                             |
| `l5.cv_kf_v1.process_noise_pos`                           | 0.05                          |                             |
| `l5.cv_kf_v1.process_noise_vel`                           | 0.2                           |                             |
| `l5.cv_kf_v1.measurement_noise`                           | 0.05                          |                             |
| `l5.cv_kf_v1.occlusion_cov_inflation`                     | 0.5                           |                             |
| `l5.cv_kf_v1.occlusion_threshold_nanos`                   | 200000000                     | 200 ms                      |
| `l5.cv_kf_v1.hits_to_confirm`                             | 4                             |                             |
| `l5.cv_kf_v1.max_misses`                                  | 3                             |                             |
| `l5.cv_kf_v1.max_misses_confirmed`                        | 15                            |                             |
| `l5.cv_kf_v1.max_tracks`                                  | 100                           |                             |
| `l5.cv_kf_v1.max_reasonable_speed_mps`                    | 30.0                          |                             |
| `l5.cv_kf_v1.max_position_jump_metres`                    | 5.0                           |                             |
| `l5.cv_kf_v1.max_predict_dt`                              | 0.5                           |                             |
| `l5.cv_kf_v1.max_covariance_diag`                         | 100.0                         |                             |
| `l5.cv_kf_v1.min_points_for_pca`                          | 4                             |                             |
| `l5.cv_kf_v1.obb_heading_smoothing_alpha`                 | 0.08                          |                             |
| `l5.cv_kf_v1.obb_aspect_ratio_lock_threshold`             | 0.25                          |                             |
| `l5.cv_kf_v1.max_track_history_length`                    | 200                           |                             |
| `l5.cv_kf_v1.max_speed_history_length`                    | 100                           |                             |
| `l5.cv_kf_v1.merge_size_ratio`                            | 2.5                           |                             |
| `l5.cv_kf_v1.split_size_ratio`                            | 0.3                           |                             |
| `l5.cv_kf_v1.deleted_track_grace_period`                  | `"5s"`                        |                             |
| `l5.cv_kf_v1.min_observations_for_classification`         | 5                             |                             |
| **L6** — engine `rule_based_v1` (29 fields)               |                               |                             |
| `l6.rule_based_v1.bird_height_max`                        | 0.5                           |                             |
| `l6.rule_based_v1.pedestrian_height_min`                  | 1.0                           |                             |
| `l6.rule_based_v1.pedestrian_height_max`                  | 2.2                           |                             |
| `l6.rule_based_v1.pedestrian_speed_max_mps`               | 3.0                           |                             |
| `l6.rule_based_v1.vehicle_height_min`                     | 1.2                           |                             |
| `l6.rule_based_v1.vehicle_length_min`                     | 3.0                           |                             |
| `l6.rule_based_v1.vehicle_width_min`                      | 1.5                           |                             |
| `l6.rule_based_v1.vehicle_speed_min_mps`                  | 5.0                           |                             |
| `l6.rule_based_v1.bus_length_min`                         | 7.0                           |                             |
| `l6.rule_based_v1.bus_width_min`                          | 2.3                           |                             |
| `l6.rule_based_v1.truck_length_min`                       | 5.5                           |                             |
| `l6.rule_based_v1.truck_width_min`                        | 2.0                           |                             |
| `l6.rule_based_v1.truck_height_min`                       | 2.0                           |                             |
| `l6.rule_based_v1.cyclist_height_min`                     | 1.0                           |                             |
| `l6.rule_based_v1.cyclist_height_max`                     | 2.0                           |                             |
| `l6.rule_based_v1.cyclist_speed_min_mps`                  | 2.0                           |                             |
| `l6.rule_based_v1.cyclist_speed_max_mps`                  | 10.0                          |                             |
| `l6.rule_based_v1.cyclist_width_max`                      | 1.2                           |                             |
| `l6.rule_based_v1.cyclist_length_max`                     | 2.5                           |                             |
| `l6.rule_based_v1.motorcyclist_speed_min_mps`             | 5.0                           |                             |
| `l6.rule_based_v1.motorcyclist_speed_max_mps`             | 30.0                          |                             |
| `l6.rule_based_v1.motorcyclist_width_max`                 | 1.2                           |                             |
| `l6.rule_based_v1.motorcyclist_length_min`                | 1.5                           |                             |
| `l6.rule_based_v1.motorcyclist_length_max`                | 3.0                           |                             |
| `l6.rule_based_v1.bird_speed_max_mps`                     | 1.0                           |                             |
| `l6.rule_based_v1.stationary_speed_max_mps`               | 0.5                           |                             |
| `l6.rule_based_v1.high_confidence`                        | 0.85                          |                             |
| `l6.rule_based_v1.medium_confidence`                      | 0.7                           |                             |
| `l6.rule_based_v1.low_confidence`                         | 0.5                           |                             |
| **Pipeline** (6 fields)                                   |                               |                             |
| `pipeline.buffer_timeout`                                 | `"500ms"`                     |                             |
| `pipeline.min_frame_points`                               | 1000                          |                             |
| `pipeline.flush_interval`                                 | `"60s"`                       |                             |
| `pipeline.background_flush`                               | false                         |                             |
| `pipeline.deleted_track_ttl`                              | `"5m"`                        |                             |
| `pipeline.prune_interval`                                 | `"1m"`                        |                             |
| **Optimisation** (3 fields)                               |                               |                             |
| `optimisation.strategy`                                   | `"accuracy_first_v1"`         |                             |
| `optimisation.search_engine`                              | `"hybrid_grid_stochastic_v1"` |                             |
| `optimisation.layer_scope`                                | `"full"`                      |                             |

---

## 4. Key-to-Layer mapping (complete)

Every tuning key, its layer assignment, and its source. Keys marked **NEW**
were previously hardcoded constants; this restructure exposes them for the
first time.

### L1: packets (sensor / network)

L1 identifies the sensor hardware and current data source. Per-process
network binding and forwarding remain CLI-managed startup settings and are
intentionally outside the hot-reload tuning schema.

| Key           | Type   | Description                     | Source      |
| ------------- | ------ | ------------------------------- | ----------- |
| `sensor`      | string | Sensor model identifier         | Config file |
| `data_source` | string | `live`, `pcap`, `pcap_analysis` | Runtime     |

**Note:** L1 packet-structure constants (packet size, block layout, channel
count, distance resolution, azimuth resolution) are protocol-level values
fixed by the sensor model and are **not** exposed as tuning parameters.

### L2: frames (frame assembly)

L2 controls how raw packets are assembled into complete 360° frames.
All values were previously hardcoded in `l2frames/frame_builder.go`.

| Key                               | Type    | Description                                   | Source                                     |
| --------------------------------- | ------- | --------------------------------------------- | ------------------------------------------ |
| `min_azimuth_coverage_deg`        | float64 | Min azimuth arc (°) for a valid frame         | **NEW**: was `MinAzimuthCoverage`          |
| `min_frame_points_for_completion` | int     | Min points before frame completion triggers   | **NEW**: was `MinFramePointsForCompletion` |
| `azimuth_tolerance_deg`           | float64 | Tolerance for azimuth wrap detection (°)      | **NEW**: was `AzimuthTolerance`            |
| `max_backfill_delay`              | string  | Max wait for late/backfill packets            | **NEW**: was `MaxBackfillDelay`            |
| `cleanup_interval`                | string  | Interval for stale-frame cleanup sweep        | **NEW**: was `CleanupInterval`             |
| `frame_buffer_size`               | int     | Max frames buffered for out-of-order packets  | **NEW**: was `FrameBufferSize`             |
| `frame_channel_capacity`          | int     | Buffered channel capacity for frame callbacks | **NEW**: was `FrameChCapacity`             |

### L3: background/Foreground extraction

Fields shared by all L3 engines (embedded via `l3Common`).
10 existing tunable + 16 newly exposed.

| Key                                    | Type    | Maths reference / description                                                           | Source                                                        |
| -------------------------------------- | ------- | --------------------------------------------------------------------------------------- | ------------------------------------------------------------- |
| `background_update_fraction`           | float64 | [background-grid-settling-maths.md](../../data/maths/background-grid-settling-maths.md) | Tunable                                                       |
| `closeness_multiplier`                 | float64 | EMA gating threshold                                                                    | Tunable                                                       |
| `safety_margin_metres`                 | float64 | Additive minimum gate width                                                             | Tunable                                                       |
| `noise_relative`                       | float64 | Range-proportional noise model                                                          | Tunable                                                       |
| `neighbour_confirmation_count`         | int     | Spatial neighbour voting                                                                | Tunable                                                       |
| `seed_from_first`                      | bool    | Cell initialisation policy                                                              | Tunable                                                       |
| `warmup_duration_nanos`                | int64   | Settling state machine                                                                  | Tunable                                                       |
| `warmup_min_frames`                    | int     | Settling state machine                                                                  | Tunable                                                       |
| `post_settle_update_fraction`          | float64 | Post-convergence adaptation rate                                                        | Tunable                                                       |
| `enable_diagnostics`                   | bool    | Per-cell debug output                                                                   | Tunable                                                       |
| `freeze_duration`                      | string  | Cell freeze time after foreground                                                       | **NEW**: was `FreezeDuration`                                 |
| `freeze_threshold_multiplier`          | float64 | Closeness multiplier for freeze trigger                                                 | **NEW**: was `FreezeThresholdMultiplier`                      |
| `settling_period`                      | string  | Time before first persistence snapshot                                                  | **NEW**: was `SettlingPeriod`                                 |
| `snapshot_interval`                    | string  | Interval between background snapshots                                                   | **NEW**: was `SnapshotInterval`                               |
| `change_threshold_snapshot`            | int     | Min changed cells to trigger a snapshot                                                 | **NEW**: was `ChangeThresholdSnapshot`                        |
| `reacquisition_boost_multiplier`       | float64 | Fast re-acquisition alpha boost                                                         | **NEW**: was `DefaultReacquisitionBoostMultiplier`            |
| `min_confidence_floor`                 | int     | Min `TimesSeenCount` to preserve during foreground                                      | **NEW**: was `DefaultMinConfidenceFloor`                      |
| `locked_baseline_threshold`            | int     | Min observations before baseline lock                                                   | **NEW**: was `DefaultLockedBaselineThreshold`                 |
| `locked_baseline_multiplier`           | float64 | Locked spread acceptance window multiplier                                              | **NEW**: was `DefaultLockedBaselineMultiplier`                |
| `sensor_movement_foreground_threshold` | float64 | Fraction of points → sensor movement detection                                          | **NEW**: was `SensorMovementForegroundThreshold`              |
| `background_drift_threshold_metres`    | float64 | Cell drift distance for significant drift                                               | **NEW**: was `BackgroundDriftThresholdMeters`                 |
| `background_drift_ratio_threshold`     | float64 | Fraction of settled cells → full background drift                                       | **NEW**: was `BackgroundDriftRatioThreshold`                  |
| `settling_min_coverage`                | float64 | Min CoverageRate for convergence (e.g. 0.80 for 80%)                                    | **NEW**: was `DefaultSettlingThresholds().MinCoverage`        |
| `settling_max_spread_delta`            | float64 | Max acceptable SpreadDeltaRate per frame                                                | **NEW**: was `DefaultSettlingThresholds().MaxSpreadDelta`     |
| `settling_min_region_stability`        | float64 | Min region stability for convergence (e.g. 0.95 for 95%)                                | **NEW**: was `DefaultSettlingThresholds().MinRegionStability` |
| `settling_min_confidence`              | float64 | Min mean TimesSeenCount for convergence                                                 | **NEW**: was `DefaultSettlingThresholds().MinConfidence`      |

**Engine variants:**

| Engine                | Description                                                   | Status     |
| --------------------- | ------------------------------------------------------------- | ---------- |
| `ema_baseline_v1`     | Current production EMA background model                       | **Active** |
| `ema_track_assist_v2` | EMA + track-assisted foreground promotion (§3 of VC proposal) | Proposed   |

### L4: perception (clustering + ground filtering)

All 9 existing tunable fields. No newly exposed constants (L4 has minimal
hardcoded values beyond numerical stability guards).

| Key                             | Type    | Maths reference                                                 |
| ------------------------------- | ------- | --------------------------------------------------------------- |
| `foreground_dbscan_eps`         | float64 | [clustering-maths.md](../../data/maths/clustering-maths.md)     |
| `foreground_min_cluster_points` | int     | DBSCAN MinPts                                                   |
| `foreground_max_input_points`   | int     | Downsampling cap                                                |
| `height_band_floor`             | float64 | [ground-plane-maths.md](../../data/maths/ground-plane-maths.md) |
| `height_band_ceiling`           | float64 | Vertical band filter                                            |
| `remove_ground`                 | bool    | Master ground-filter switch                                     |
| `max_cluster_diameter`          | float64 | Post-DBSCAN cluster geometry filter                             |
| `min_cluster_diameter`          | float64 | Minimum cluster extent                                          |
| `max_cluster_aspect_ratio`      | float64 | Elongation filter                                               |

**Engine variants:**

| Engine                     | Description                                                        | Status     |
| -------------------------- | ------------------------------------------------------------------ | ---------- |
| `dbscan_xy_v1`             | Current production 2D DBSCAN                                       | **Active** |
| `two_stage_mahalanobis_v2` | Spatial DBSCAN + velocity-coherent split/merge (§4 of VC proposal) | Proposed   |
| `hdbscan_adaptive_v1`      | Hierarchical DBSCAN with adaptive density                          | Proposed   |

### L5: tracking (state estimation + assignment)

22 existing tunable + 1 newly exposed.

| Key                                   | Type    | Maths reference / description                                             | Source                                 |
| ------------------------------------- | ------- | ------------------------------------------------------------------------- | -------------------------------------- |
| `gating_distance_squared`             | float64 | [tracking-maths.md](../../data/maths/tracking-maths.md): Mahalanobis gate | Tunable                                |
| `process_noise_pos`                   | float64 | KF process noise (position)                                               | Tunable                                |
| `process_noise_vel`                   | float64 | KF process noise (velocity)                                               | Tunable                                |
| `measurement_noise`                   | float64 | KF measurement noise                                                      | Tunable                                |
| `occlusion_cov_inflation`             | float64 | Coast-mode covariance growth                                              | Tunable                                |
| `occlusion_threshold_nanos`           | int64   | Gap duration (ns) triggering occlusion mode (~200 ms at 10 Hz)            | **NEW**: was `occlusionThresholdNanos` |
| `hits_to_confirm`                     | int     | Track lifecycle                                                           | Tunable                                |
| `max_misses`                          | int     | Tentative track deletion                                                  | Tunable                                |
| `max_misses_confirmed`                | int     | Confirmed track deletion                                                  | Tunable                                |
| `max_tracks`                          | int     | Capacity limit                                                            | Tunable                                |
| `max_reasonable_speed_mps`            | float64 | Velocity clamp                                                            | Tunable                                |
| `max_position_jump_metres`            | float64 | Association plausibility gate                                             | Tunable                                |
| `max_predict_dt`                      | float64 | Maximum prediction horizon                                                | Tunable                                |
| `max_covariance_diag`                 | float64 | Covariance cap (numerical guard)                                          | Tunable                                |
| `min_points_for_pca`                  | int     | OBB geometry minimum                                                      | Tunable                                |
| `obb_heading_smoothing_alpha`         | float64 | Heading EMA                                                               | Tunable                                |
| `obb_aspect_ratio_lock_threshold`     | float64 | Aspect-ratio lock guard                                                   | Tunable                                |
| `max_track_history_length`            | int     | History buffer size                                                       | Tunable                                |
| `max_speed_history_length`            | int     | Speed statistics buffer                                                   | Tunable                                |
| `merge_size_ratio`                    | float64 | Track merge heuristic                                                     | Tunable                                |
| `split_size_ratio`                    | float64 | Track split heuristic                                                     | Tunable                                |
| `deleted_track_grace_period`          | string  | Grace period for deleted-track reuse                                      | Tunable                                |
| `min_observations_for_classification` | int     | Classification confidence gate                                            | Tunable                                |

**Engine variants:**

| Engine                  | Description                                                                | Status     |
| ----------------------- | -------------------------------------------------------------------------- | ---------- |
| `cv_kf_v1`              | Current production constant-velocity Kalman filter                         | **Active** |
| `imm_cv_ca_v2`          | Interacting Multiple Model: CV + constant-acceleration (§5 of VC proposal) | Proposed   |
| `imm_cv_ca_rts_eval_v2` | IMM + Rauch-Tung-Striebel offline smoothing (evaluation only)              | Proposed   |

### L6: objects (classification)

All L6 classification thresholds were previously hardcoded as Go `const`
values in `l6objects/classification.go`. Exposing them enables per-deployment
tuning (e.g. different thresholds for UK residential vs. rural roads).

| Key                          | Type    | Description                                  | Source  |
| ---------------------------- | ------- | -------------------------------------------- | ------- |
| `bird_height_max`            | float64 | Max height for bird classification (m)       | **NEW** |
| `pedestrian_height_min`      | float64 | Min height for pedestrian (m)                | **NEW** |
| `pedestrian_height_max`      | float64 | Max height for pedestrian (m)                | **NEW** |
| `pedestrian_speed_max_mps`   | float64 | Max walking speed (m/s, ~10.8 km/h)          | **NEW** |
| `vehicle_height_min`         | float64 | Min height for vehicle (m)                   | **NEW** |
| `vehicle_length_min`         | float64 | Min length for vehicle (m)                   | **NEW** |
| `vehicle_width_min`          | float64 | Min width for vehicle (m)                    | **NEW** |
| `vehicle_speed_min_mps`      | float64 | Min speed for vehicle (m/s)                  | **NEW** |
| `bus_length_min`             | float64 | Min length for bus (m)                       | **NEW** |
| `bus_width_min`              | float64 | Min width for bus (m)                        | **NEW** |
| `truck_length_min`           | float64 | Min length for truck (m)                     | **NEW** |
| `truck_width_min`            | float64 | Min width for truck (m)                      | **NEW** |
| `truck_height_min`           | float64 | Min height for truck (m)                     | **NEW** |
| `cyclist_height_min`         | float64 | Min height for cyclist (m)                   | **NEW** |
| `cyclist_height_max`         | float64 | Max height for cyclist (m)                   | **NEW** |
| `cyclist_speed_min_mps`      | float64 | Min speed for cyclist (m/s, ~7.2 km/h)       | **NEW** |
| `cyclist_speed_max_mps`      | float64 | Max speed for cyclist (m/s, ~36 km/h)        | **NEW** |
| `cyclist_width_max`          | float64 | Max width for cyclist (m)                    | **NEW** |
| `cyclist_length_max`         | float64 | Max length for cyclist (m)                   | **NEW** |
| `motorcyclist_speed_min_mps` | float64 | Min speed for motorcyclist (m/s, ~18 km/h)   | **NEW** |
| `motorcyclist_speed_max_mps` | float64 | Max speed for motorcyclist (m/s, ~108 km/h)  | **NEW** |
| `motorcyclist_width_max`     | float64 | Max width for motorcyclist (m)               | **NEW** |
| `motorcyclist_length_min`    | float64 | Min length for motorcyclist (m)              | **NEW** |
| `motorcyclist_length_max`    | float64 | Max length for motorcyclist (m)              | **NEW** |
| `bird_speed_max_mps`         | float64 | Max speed for bird detection (m/s)           | **NEW** |
| `stationary_speed_max_mps`   | float64 | Speed below which object is stationary (m/s) | **NEW** |
| `high_confidence`            | float64 | High-confidence classification threshold     | **NEW** |
| `medium_confidence`          | float64 | Medium-confidence classification threshold   | **NEW** |
| `low_confidence`             | float64 | Low-confidence classification threshold      | **NEW** |

**Engine variants:**

| Engine          | Description                              | Status     |
| --------------- | ---------------------------------------- | ---------- |
| `rule_based_v1` | Current production rule-based classifier | **Active** |

### Pipeline: cross-cutting

| Key                 | Type   | Description                         | Source                         |
| ------------------- | ------ | ----------------------------------- | ------------------------------ |
| `buffer_timeout`    | string | Frame completion timeout            | Tunable                        |
| `min_frame_points`  | int    | Minimum frame size for processing   | Tunable                        |
| `flush_interval`    | string | Background grid snapshot cadence    | Tunable                        |
| `background_flush`  | bool   | Master flush switch                 | Tunable                        |
| `deleted_track_ttl` | string | TTL for soft-deleted tracks in DB   | **NEW**: was `deletedTrackTTL` |
| `prune_interval`    | string | Interval for pruning deleted tracks | **NEW**: was `pruneInterval`   |

### Optimisation: sweep/Auto-tune controls

| Key             | Type   | Allowed values                                                       |
| --------------- | ------ | -------------------------------------------------------------------- |
| `strategy`      | string | `accuracy_first_v1`, `balanced_v1`, `realtime_v1`                    |
| `search_engine` | string | `grid_narrowing_v1`, `hybrid_grid_stochastic_v1`, `local_perturb_v1` |
| `layer_scope`   | string | `full`, `l3_only`, `l4_only`, `l5_only`                              |

### Intentionally excluded from config

These constants are **not** exposed because they are protocol-level, sensor-
specific, or numerical stability guards that should never be tuned:

| Constant                    | Value     | Layer | Reason                                          |
| --------------------------- | --------- | ----- | ----------------------------------------------- |
| `PACKET_SIZE_STANDARD`      | `1262`    | L1    | Hesai Pandar40P protocol spec                   |
| `CHANNELS_PER_BLOCK`        | `40`      | L1    | 40-beam sensor hardware                         |
| `DISTANCE_RESOLUTION`       | `0.004` m | L1    | Sensor distance LSB (fixed by firmware)         |
| `AZIMUTH_RESOLUTION`        | `0.01°`   | L1    | Sensor azimuth LSB (fixed by firmware)          |
| `MinDeterminantThreshold`   | `1e-6`    | L5    | Numerical stability for covariance inversion    |
| `SingularDistanceRejection` | `1e9`     | L5    | Infinity stand-in for rejected associations     |
| `obbCovarianceEpsilon`      | `1e-9`    | L4    | Numerical stability for OBB PCA                 |
| `hungarianlnf`              | `1e18`    | L5    | Hungarian algorithm infinity                    |
| `ThawGracePeriodNanos`      | `1ms`     | L3    | Prevents false thaw triggers after freeze       |
| `regionRestoreMinFrames`    | `10`      | L3    | Min frames before attempting DB region restore  |
| aspect-ratio noise floor    | `0.03` m  | L4    | Shortest-axis threshold for aspect-ratio filter |
| `maxBackgroundChartPoints`  | `5000`    | Pipe  | Debug visualisation downsampling cap            |
| `MaxFrameRate`              | (wiring)  | Pipe  | Pipeline frame-rate cap; runtime-set            |
| `VoxelLeafSize`             | (wiring)  | Pipe  | Voxel downsampling leaf size; runtime-set       |

---

## 5. Engine block field counts

All engine parameters (common + engine-specific) live inside the engine block.
The block is a self-describing snapshot: every field is required when the
block is present. Validation is strict:

1. Read `engine` to identify the active engine.
2. Extract and validate the matching engine block with
   `DisallowUnknownFields`: every field must be present, no unknowns.
3. Reject the file if the matching engine block is absent, if any
   non-selected engine block is present, or if any field is missing or
   unknown inside the block.

### L5 engines

#### `cv_kf_v1`: 23 fields

The 23 common tracking params listed in §4 are the complete block.

#### `imm_cv_ca_v2`: 27 fields (23 common + 4 IMM)

| Key                            | Type    | Description                                     |
| ------------------------------ | ------- | ----------------------------------------------- |
| `transition_cv_to_ca`          | float64 | Model-jump probability CV → CA per step         |
| `transition_ca_to_cv`          | float64 | Model-jump probability CA → CV per step         |
| `ca_process_noise_acc`         | float64 | Acceleration process noise for the CA sub-model |
| `low_speed_heading_freeze_mps` | float64 | Speed below which heading updates are frozen    |

#### `imm_cv_ca_rts_eval_v2`: 28 fields (23 common + 4 IMM + 1 RTS)

All 4 `imm_cv_ca_v2` fields (inherited via struct embedding), plus:

| Key                    | Type | Description                           |
| ---------------------- | ---- | ------------------------------------- |
| `rts_smoothing_window` | int  | Number of steps for RTS backward pass |

### L4 engines

#### `dbscan_xy_v1`: 9 fields

The 9 clustering params listed in §4 are the complete block.

#### `two_stage_mahalanobis_v2`: 11 fields (9 common + 2 VC)

| Key                       | Type    | Description                                             |
| ------------------------- | ------- | ------------------------------------------------------- |
| `velocity_coherence_gate` | float64 | Mahalanobis distance gate for velocity split/merge      |
| `min_velocity_confidence` | float64 | Minimum L5 velocity confidence to use motion refinement |

#### `hdbscan_adaptive_v1`: 11 fields (9 common + 2 HDBSCAN)

| Key                | Type | Description                                  |
| ------------------ | ---- | -------------------------------------------- |
| `min_cluster_size` | int  | HDBSCAN minimum cluster size                 |
| `min_samples`      | int  | HDBSCAN core-point neighbourhood requirement |

### L3 engines

#### `ema_baseline_v1`: 26 fields

The 26 background/foreground params listed in §4 are the complete block.

#### `ema_track_assist_v2`: 29 fields (26 common + 3 TA)

| Key                        | Type    | Description                                        |
| -------------------------- | ------- | -------------------------------------------------- |
| `promotion_near_gate_low`  | float64 | Lower gamma for near-gate range (`gamma1 × tau`)   |
| `promotion_near_gate_high` | float64 | Upper gamma for near-gate range (`gamma2 × tau`)   |
| `promotion_threshold`      | float64 | Motion proximity score threshold (`theta_promote`) |

### L6 engines

#### `rule_based_v1`: 29 fields

The 29 classification params listed in §4 are the complete block.

### Summary: field counts

All fields are inside the engine block (excluding the `engine` selector).
When the block is present, every field is required.

| Layer | Engine                     | Block fields |
| ----- | -------------------------- | ------------ |
| L1    | (no engine)                | **6**        |
| L2    | (no engine)                | **7**        |
| L3    | `ema_baseline_v1`          | **26**       |
| L3    | `ema_track_assist_v2`      | **29**       |
| L4    | `dbscan_xy_v1`             | **9**        |
| L4    | `two_stage_mahalanobis_v2` | **11**       |
| L4    | `hdbscan_adaptive_v1`      | **11**       |
| L5    | `cv_kf_v1`                 | **23**       |
| L5    | `imm_cv_ca_v2`             | **27**       |
| L5    | `imm_cv_ca_rts_eval_v2`    | **28**       |
| L6    | `rule_based_v1`            | **29**       |

---

## 6. Go implementation plan

### 6.1 Struct design: all fields inside engine blocks

Each engine-selectable layer has a **wrapper struct** containing the `engine`
selector and one pointer field per engine variant. Each **engine struct**
embeds the common type for that layer, so all fields (common + engine-specific)
live inside the engine block. The engine block pointer is optional at the Go
level (`omitempty`): absent when that engine is not selected, present and
strictly validated when it is. Data fields inside the block are concrete values
(no pointers, no `omitempty`).

`DisallowUnknownFields` is applied at two levels:

1. **Wrapper level**: rejects unknown keys at the layer object level (only
   `engine` and the selected engine block are allowed)
2. **Engine block level**: rejects unknown/misspelled fields inside the
   block; all fields are required (no `omitempty`)

`CurrentConfigVersion = 2`

**`TuningConfig` (root):**

| Field          | Type               | JSON           | Notes             |
| -------------- | ------------------ | -------------- | ----------------- |
| `Version`      | int                | `version`      | Must equal 2      |
| `L1`           | L1Config           | `l1`           |                   |
| `L2`           | L2Config           | `l2`           |                   |
| `L3`           | L3Config           | `l3`           | Engine-selectable |
| `L4`           | L4Config           | `l4`           | Engine-selectable |
| `L5`           | L5Config           | `l5`           | Engine-selectable |
| `L6`           | L6Config           | `l6`           | Engine-selectable |
| `Pipeline`     | PipelineConfig     | `pipeline`     |                   |
| `Optimisation` | OptimisationConfig | `optimisation` |                   |

**`L1Config`** (2 fields): `Sensor` (string), `DataSource` (string).

**`L2Config`** (7 fields): `MinAzimuthCoverageDeg` (float64), `MinFramePointsForCompletion` (int), `AzimuthToleranceDeg` (float64), `MaxBackfillDelay` (string), `CleanupInterval` (string), `FrameBufferSize` (int), `FrameChannelCapacity` (int).

**`PipelineConfig`** (6 fields): `BufferTimeout` (string), `MinFramePoints` (int), `FlushInterval` (string), `BackgroundFlush` (bool), `DeletedTrackTTL` (string), `PruneInterval` (string).

**`OptimisationConfig`** (3 fields): `Strategy` (string), `SearchEngine` (string), `LayerScope` (string).

**`L3Config`** wrapper: `Engine` (string) + pointer to one engine block (`EmaBaselineV1` or `EmaTrackAssistV2`, `omitempty`).

**`l3Common`** (26 fields, embedded in all L3 engines): `BackgroundUpdateFraction`, `ClosenessMultiplier`, `SafetyMarginMetres`, `NoiseRelative`, `NeighbourConfirmationCount`, `SeedFromFirst`, `WarmupDurationNanos`, `WarmupMinFrames`, `PostSettleUpdateFraction`, `EnableDiagnostics`, `FreezeDuration`, `FreezeThresholdMultiplier`, `SettlingPeriod`, `SnapshotInterval`, `ChangeThresholdSnapshot`, `ReacquisitionBoostMultiplier`, `MinConfidenceFloor`, `LockedBaselineThreshold`, `LockedBaselineMultiplier`, `SensorMovementForegroundThreshold`, `BackgroundDriftThresholdMetres`, `BackgroundDriftRatioThreshold`, `SettlingMinCoverage`, `SettlingMaxSpreadDelta`, `SettlingMinRegionStability`, `SettlingMinConfidence`.

- **`L3EmaBaselineV1`** embeds `l3Common` (26 fields). No additional fields.
- **`L3EmaTrackAssistV2`** embeds `l3Common` + 3 fields: `PromotionNearGateLow`, `PromotionNearGateHigh`, `PromotionThreshold` (all float64).

**`L4Config`** wrapper: `Engine` + one of `DbscanXyV1`, `TwoStageMahalanobisV2`, `HdbscanAdaptiveV1`.

**`l4Common`** (9 fields): `ForegroundDBSCANEps`, `ForegroundMinClusterPoints`, `ForegroundMaxInputPoints`, `HeightBandFloor`, `HeightBandCeiling`, `RemoveGround`, `MaxClusterDiameter`, `MinClusterDiameter`, `MaxClusterAspectRatio`.

- **`L4DbscanXyV1`** embeds `l4Common` (9 fields). No additional fields.
- **`L4TwoStageMahalanobisV2`** embeds `l4Common` + 2 fields: `VelocityCoherenceGate`, `MinVelocityConfidence`.
- **`L4HdbscanAdaptiveV1`** embeds `l4Common` + 2 fields: `MinClusterSize`, `MinSamples`.

**`L5Config`** wrapper: `Engine` + one of `CvKfV1`, `ImmCvCaV2`, `ImmCvCaRtsEvalV2`.

**`l5Common`** (23 fields): `GatingDistanceSquared`, `ProcessNoisePos`, `ProcessNoiseVel`, `MeasurementNoise`, `OcclusionCovInflation`, `OcclusionThresholdNanos`, `HitsToConfirm`, `MaxMisses`, `MaxMissesConfirmed`, `MaxTracks`, `MaxReasonableSpeedMps`, `MaxPositionJumpMetres`, `MaxPredictDt`, `MaxCovarianceDiag`, `MinPointsForPCA`, `OBBHeadingSmoothingAlpha`, `OBBAspectRatioLockThreshold`, `MaxTrackHistoryLength`, `MaxSpeedHistoryLength`, `MergeSizeRatio`, `SplitSizeRatio`, `DeletedTrackGracePeriod`, `MinObservationsForClassification`.

- **`L5CvKfV1`** embeds `l5Common` (23 fields). No additional fields.
- **`L5ImmCvCaV2`** embeds `l5Common` + 4 IMM fields: `TransitionCVToCA`, `TransitionCAToCV`, `CAProcessNoiseAcc`, `LowSpeedHeadingFreezeMps`.
- **`L5ImmCvCaRtsEvalV2`** embeds `L5ImmCvCaV2` (27 fields) + 1: `RTSSmoothingWindow` (int).

**`L6Config`** wrapper: `Engine` + `RuleBasedV1`.

**`l6Common`** (29 fields): `BirdHeightMax`, `PedestrianHeightMin`, `PedestrianHeightMax`, `PedestrianSpeedMaxMps`, `VehicleHeightMin`, `VehicleLengthMin`, `VehicleWidthMin`, `VehicleSpeedMinMps`, `BusLengthMin`, `BusWidthMin`, `TruckLengthMin`, `TruckWidthMin`, `TruckHeightMin`, `CyclistHeightMin`, `CyclistHeightMax`, `CyclistSpeedMinMps`, `CyclistSpeedMaxMps`, `CyclistWidthMax`, `CyclistLengthMax`, `MotorcyclistSpeedMinMps`, `MotorcyclistSpeedMaxMps`, `MotorcyclistWidthMax`, `MotorcyclistLengthMin`, `MotorcyclistLengthMax`, `BirdSpeedMaxMps`, `StationarySpeedMaxMps`, `HighConfidence`, `MediumConfidence`, `LowConfidence`.

- **`L6RuleBasedV1`** embeds `l6Common` (29 fields). No additional fields.

### 6.2 `LoadTuningConfig`: strict validation

```
1. Unmarshal root JSON with DisallowUnknownFields.
2. Check version == CurrentConfigVersion (reject with clear error if not).
3. Validate L1, L2, Pipeline, and Optimisation (all fields present, valid enums).
4. For each engine-selectable layer (L3, L4, L5, L6):
   a. Read the wrapper's Engine field.
   b. Look up the engine in the registry.
   c. Verify the matching engine block pointer is non-nil.
   d. Unmarshal/validate the engine block with DisallowUnknownFields
      — every field inside the block is required (no omitempty on data
      fields), so missing fields cause a decode error.
   e. Verify no non-selected engine blocks are present.
   f. If validation fails, return error listing which fields are
      missing or unknown (e.g. "l5.imm_cv_ca_v2: requires
      transition_cv_to_ca but it was not provided").
5. Validate engine names against known set.
6. Return populated, validated config.
```

### 6.2.1 Engine registry

The registry maps engine names to their struct type. Adding a new engine is a
single-line addition:

**`EngineSpec` struct** — `Layer` (string: `"l3"`, `"l4"`, `"l5"`, `"l6"`) and `NewConfig` (factory returning a pointer to a zero-value engine struct).

**`engineRegistry`** maps engine names to specs:

| Engine name                | Layer | Struct                    |
| -------------------------- | ----- | ------------------------- |
| `ema_baseline_v1`          | l3    | `L3EmaBaselineV1`         |
| `ema_track_assist_v2`      | l3    | `L3EmaTrackAssistV2`      |
| `dbscan_xy_v1`             | l4    | `L4DbscanXyV1`            |
| `two_stage_mahalanobis_v2` | l4    | `L4TwoStageMahalanobisV2` |
| `hdbscan_adaptive_v1`      | l4    | `L4HdbscanAdaptiveV1`     |
| `cv_kf_v1`                 | l5    | `L5CvKfV1`                |
| `imm_cv_ca_v2`             | l5    | `L5ImmCvCaV2`             |
| `imm_cv_ca_rts_eval_v2`    | l5    | `L5ImmCvCaRtsEvalV2`      |
| `rule_based_v1`            | l6    | `L6RuleBasedV1`           |

The registry also enforces layer assignment: an L4 engine cannot be placed
in the L5 slot.

### 6.3 Factory function updates

`BackgroundConfigFromTuning` and `TrackerConfigFromTuning` accept the
concrete engine struct (e.g. `*L3EmaBaselineV1`, `*L5CvKfV1`) rather than
the layer wrapper. Common fields are accessed via the embedded common type.

A new `L4ConfigFromTuning` factory is added: it accepts the concrete L4
engine struct.

**Cross-layer dependency:** `BackgroundConfigFromTuning` also reads L4
clustering parameters (`ForegroundDBSCANEps`, `ForegroundMinClusterPoints`,
`ForegroundMaxInputPoints`) from the active L4 engine struct. The function
signature accepts both the L3 engine struct and the L4 engine struct.

### 6.4 Sweep parameter paths

All engine-selectable params use three-segment dot-paths through the engine
block: `"l5.cv_kf_v1.process_noise_vel"`, `"l3.ema_baseline_v1.noise_relative"`,
`"l6.rule_based_v1.bird_height_max"`.

Non-engine layers use two-segment paths: `"l1.sensor"`,
`"pipeline.buffer_timeout"`.

Flat key names are no longer accepted.

---

## 7. Evaluation protocol (layer-scoped)

When evaluating algorithm changes, the harness must compare identical replay
windows across five scenarios:

1. **Baseline**: current production config
2. **L3-only change**: only L3 engine/params differ
3. **L4-only change**: only L4 engine/params differ
4. **L5-only change**: only L5 engine/params differ
5. **Full-stack change**: all layers updated simultaneously

For each scenario, compute:

- Velocity RMSE, acceleration RMSE
- Low-speed heading jitter
- Fragmentation / ID-switch rates
- Foreground capture ratio

Results are compared with paired bootstrap confidence intervals. All regression
gates must pass before a configuration change is promoted.

See: [velocity-coherent-foreground-extraction.md §7](../../data/maths/proposals/20260220-velocity-coherent-foreground-extraction.md)
for the full statistical protocol.

### 7.1 Long-term posture gaps (post-branch review)

This branch substantially improves **schema discipline**:

- the runtime now has a strict, versioned nested schema,
- config files can be migrated and validated locally,
- runtime updates use canonical layer/engine dot-paths,
- the monitor/sweep surfaces are converging on one config contract.

That is necessary, but it is not yet the full long-term config posture needed
for **experimentation**, **deterministic replay**, and **trustworthy run
provenance**. The main remaining gaps are:

1. **Schema-valid is not yet experiment-valid.** We can load a config file, but
   we do not yet persist a first-class _resolved run config_ that freezes the
   exact effective tuning, active engine selectors, and build identity that
   were actually executed.
2. **Requested params, effective config, and execution metadata are still only
   partially separated.** Some run/replay/recommendation surfaces still rely on
   copied JSON blobs rather than immutable references to canonical config
   assets.
3. **Config identity is not yet the system boundary.** The branch introduces
   validation and hashing, but hashes are not yet the canonical FK-backed
   identity used consistently across runs, replay cases, UI diff views,
   recordings, and reports.
4. **Deterministic experiment inputs are incomplete.** Search seed, replay
   window, objective profile, and dataset selection are not yet promoted to a
   stable experiment manifest that can be re-run exactly.
5. **Legacy artifact posture is still transitional.** Runtime/API compatibility
   can smooth upgrades, but the durable answer is migration from copied legacy
   blobs to canonical parameter-set / run-config assets.

**Boundary decision:** this document remains the canonical home for the
**runtime schema and config contract**. Detailed database/API design for
immutable run-config assets remains canonical in
[LiDAR Deterministic Run Config and Execution Metadata Plan](lidar-immutable-run-config-asset-plan.md).
The new phase below is the sequencing bridge between this schema restructure
and that deterministic asset plan.

---

## 8. Implementation sequence

### Phase 1: structural realignment (v0.5.0) ✅ complete

Reorganise the existing 44 flat params into the versioned, layer-scoped,
engine-selectable schema. No new parameters are added in this phase: the
config surface area is identical, only the structure changes.

Delivered in `dd/config-restructure` (commits `5f3994f`, `51bfab3`).

| Step | Description                                                                                                         | Depends on | Status |
| ---- | ------------------------------------------------------------------------------------------------------------------- | ---------- | ------ |
| 1    | Define engine structs with embedded common types; wrapper structs with engine selector + pointers (L3, L4, L5 only) | -          | ✅     |
| 2    | Implement engine registry and `LoadTuningConfig` with strict validation                                             | Step 1     | ✅     |
| 3    | Add `make config-migrate` target (converts v1 flat → v2 nested)                                                     | Step 1     | ✅     |
| 4    | Regenerate `tuning.defaults.json`, `tuning.example.json`, `tuning.optimised.json`                                   | Step 3     | ✅     |
| 5    | Apply spelling corrections (`neighbor` → `neighbour`, `meters` → `metres`)                                          | Step 4     | ✅     |
| 6    | Update factory functions to accept concrete engine structs                                                          | Step 1     | ✅     |
| 7    | Update sweep param path resolution (dot-paths only)                                                                 | Step 1     | ✅     |
| 8    | Update `config-order-check` / `config-order-sync` for nested keys                                                   | Step 4     | ✅     |
| 9    | Update `config/README.md` and `config/README.maths.md`                                                              | Step 4     | ✅     |
| 10   | Update `/api/lidar/params` endpoint schema                                                                          | Step 6     | ✅     |
| 11   | Add `make config-validate` target: CLI wrapper that loads a JSON file and runs `LoadTuningConfig` validation        | Step 2     | ✅     |
| 12   | Delete old `TuningConfig` flat struct and all pointer-field helpers                                                 | Step 10    | ✅     |

### Phase 2: essential new variable exposure (v0.6.0) ✅ complete

Expose the highest-impact hardcoded constants: L1 sensor identity plus L3
background/foreground parameters. LiDAR network binding and forwarding stay
per-process CLI settings (`--lidar-udp-port`, `--lidar-udp-rcv-buf`,
`--lidar-forward-port`, `--lidar-foreground-forward-port`) and are not part
of the hot-reload tuning schema on this branch.

| Step | Description                                                                     | Depends on  | Status      |
| ---- | ------------------------------------------------------------------------------- | ----------- | ----------- |
| 13   | Add `L1Config` struct; wire sensor/UDP/forward-port fields; deprecate CLI flags | Phase 1     | ✅ Complete |
| 14   | Expand `l3Common` with 16 new fields; wire through background/foreground logic  | Phase 1     | ✅          |
| 15   | Regenerate config files with new L1 and L3 fields                               | Steps 13–14 | ✅          |
| 16   | Update `config/README.md` with new field documentation                          | Step 15     | ✅          |

### Phase 2B: experiment contract + deterministic config identity (v0.6.x / v0.7.0)

Turn the new nested schema into a reproducible **experiment surface**, not just
a startup file format. This phase creates the bridge from runtime tuning config
to immutable experiment assets. The detailed DB/API/storage design lives in
[LiDAR Deterministic Run Config and Execution Metadata Plan](lidar-immutable-run-config-asset-plan.md);
this plan tracks the config-system work required to make that design land
cleanly.

| Step | Description                                                                                                                                                                             | Depends on  | Status   |
| ---- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------- | -------- |
| 17   | Define the canonical resolved-config projection: exact effective tuning payload, canonical key order, normalised durations, no timestamps/UUIDs in deterministic identity               | Phase 2     | Proposed |
| 18   | Split config concepts explicitly in code/docs: requested param set vs effective resolved config vs non-deterministic execution metadata                                                 | Step 17     | Proposed |
| 19   | Add local tooling to export and verify deterministic config assets (`config-resolve`, `config-canonicalise`, `config-hash`)                                                             | Step 17     | Proposed |
| 20   | Persist exact config identity for sweep/replay runs: build identity, effective config identity, replay window, objective profile, and search seed                                       | Steps 17–19 | Proposed |
| 21   | Replace copied JSON blobs in run/replay/recommendation surfaces with references to canonical param-set / run-config assets; keep legacy blobs only as migration shims while backfilling | Step 20     | Proposed |
| 22   | Upgrade UI, reports, and VRLOG provenance to diff/display exact config assets rather than ad hoc blobs or best-effort hashes                                                            | Steps 19–21 | Proposed |

### Phase 3: remaining variable exposure + L6 classification (v2.0)

Expose lower-priority hardcoded constants (L2 frame assembly, L5 occlusion,
pipeline TTL/prune) and L6 classification thresholds. L2, L5, and pipeline
constants are stable and work well at current values. L6 classification is
deferred until the classifier strategy is settled: the rule-based classifier
is a candidate for replacement by an ML classifier (see
[ML classifier training pipeline](lidar-ml-classifier-training-plan.md)).

| Step | Description                                                                       | Depends on  |
| ---- | --------------------------------------------------------------------------------- | ----------- |
| 23   | Add `L2Config` struct; wire frame-assembly constants through `FrameBuilder`       | Phase 2B    |
| 24   | Add `OcclusionThresholdNanos` to `L5Common`; wire through tracker                 | Phase 2B    |
| 25   | Add `DeletedTrackTTL`, `PruneInterval` to `PipelineConfig`; wire through pipeline | Phase 2B    |
| 26   | Add `L6Common` + `L6RuleBasedV1` struct; wire classification thresholds           | Phase 2B    |
| 27   | Add L6 engine to registry; update validation for engine-selectable L6             | Step 26     |
| 28   | Regenerate config files with all Phase 3 fields                                   | Steps 23–27 |
| 29   | Update `config/README.md` with Phase 3 field documentation                        | Step 28     |

---

## 9. Related documents

- [Config reference](../../config/CONFIG.md): current parameter documentation
- [Config-to-maths cross reference](../../config/CONFIG.md#config-to-maths-cross-reference): key-to-maths mapping
- [LiDAR Deterministic Run Config and Execution Metadata Plan](lidar-immutable-run-config-asset-plan.md): canonical design for immutable config assets and exact run provenance
- [Velocity-Coherent Foreground Extraction](../../data/maths/proposals/20260220-velocity-coherent-foreground-extraction.md): engine variants and config contract (§6)
- [ML Solver Expansion](../lidar/architecture/ml-solver-expansion.md): optimisation platform plan
- [MATHS.md](../../data/maths/MATHS.md): proposal roadmap (P1–P4)
- [Geometry-Coherent Tracking](../../data/maths/proposals/20260222-geometry-coherent-tracking.md): P1 proposal (L5 geometry model)
