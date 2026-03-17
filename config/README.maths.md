# LiDAR Config-to-Maths Cross Reference

This document maps LiDAR tuning keys to the mathematical subsystems they control.

Primary schema source:

- `config/tuning.defaults.json`
- `internal/config/tuning.go`

Note: this repo does not ship a `tracking.json` file. If your deployment tooling refers to "tracking.json", use the same key schema as `tuning.defaults.json`.

## 1. Background Settling (L3)

Math reference:

- [`data/maths/background-grid-settling-maths.md`](../data/maths/background-grid-settling-maths.md)

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

- [`data/maths/ground-plane-maths.md`](../data/maths/ground-plane-maths.md)
- Proposal: [`data/maths/proposals/20260219-unify-l3-l4-settling.md`](../data/maths/proposals/20260219-unify-l3-l4-settling.md)

Keys:

- `height_band_floor`
- `height_band_ceiling`
- `remove_ground`
- region-selection coupling (derived from L3 keys):
  - `noise_relative`
  - `safety_margin_meters`
  - `closeness_multiplier`
  - `neighbor_confirmation_count`

Code path:

- `cmd/radar/radar.go` -> `internal/lidar/pipeline/tracking_pipeline.go` -> `internal/lidar/l4perception/ground.go`

## 3. Clustering (L4)

Math reference:

- [`data/maths/clustering-maths.md`](../data/maths/clustering-maths.md)

Keys:

- `foreground_dbscan_eps`
- `foreground_min_cluster_points`
- `foreground_max_input_points`
- `max_cluster_diameter`
- `min_cluster_diameter`
- `max_cluster_aspect_ratio`

Code path:

- `internal/lidar/l4perception/cluster.go` (`DefaultDBSCANParams`)
- `internal/lidar/pipeline/tracking_pipeline.go`

## 4. Tracking (L5)

Math reference:

- [`data/maths/tracking-maths.md`](../data/maths/tracking-maths.md)

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

## 6. Evidence Levels and Value Rationale

Each config key in `tuning.defaults.json` carries an evidence level.
This is the **canonical record** of current defaults, derivations, and
evidence classification. Experiment files reference this section rather
than duplicating values.

| Level           | Meaning                                                             | Count | Action                                   |
| --------------- | ------------------------------------------------------------------- | ----- | ---------------------------------------- |
| **Theoretical** | Derived from mathematical first principles or sensor specifications | ~5    | No action — document derivation          |
| **Literature**  | Matches published values for the algorithm class                    | ~2    | Verify applicability to our sensor/scene |
| **Empirical**   | Validated on at least one site with measured improvement            | ~3    | Extend to multi-site validation          |
| **Provisional** | Tuned on kirk0 only, no formal comparison                           | ~8    | **Must validate before stable release**  |

### Theoretical keys

| Config key                   | Default | Derivation                                                                 |
| ---------------------------- | ------- | -------------------------------------------------------------------------- |
| `background_update_fraction` | 0.02    | EMA α for 50-frame effective window; matches typical settling target       |
| `gating_distance_squared`    | 36.0    | χ²(2) conservative gate (≈6σ); effectively never rejects on distance alone |
| `max_reasonable_speed_mps`   | 30.0    | ~108 km/h; reasonable upper bound for UK residential/urban roads           |

### Literature keys

| Config key              | Default | Source                                             | Verification                                                 |
| ----------------------- | ------- | -------------------------------------------------- | ------------------------------------------------------------ |
| `foreground_dbscan_eps` | 0.8     | DBSCAN literature, adapted for LiDAR point density | Sweep [0.3, 1.5] across sites with different point densities |

### Provisional keys requiring validation

| Config key                      | Default | Layer | Risk if wrong                                            |
| ------------------------------- | ------- | ----- | -------------------------------------------------------- |
| `closeness_multiplier`          | 3.0     | L3    | False foreground/background at range extremes            |
| `safety_margin_meters`          | 0.15    | L3    | Ground leakage into foreground                           |
| `noise_relative`                | 0.02    | L3    | Incorrect range-dependent thresholds                     |
| `neighbor_confirmation_count`   | 3       | L3    | Missed foreground at scene edges                         |
| `foreground_min_cluster_points` | 5       | L4    | Missed small objects (cyclists, pedestrians)             |
| `process_noise_pos`             | 0.05    | L5    | Track instability or sluggish response                   |
| `process_noise_vel`             | 0.2     | L5    | Velocity estimate lag or overshoot                       |
| `measurement_noise`             | 0.05    | L5    | Incorrect Kalman gain, over/under-weighting measurements |

### Graduation criteria

A key graduates from "provisional" to "empirical" when:

1. Swept across ≥ 3 sites (target: 5)
2. Optimal value is consistent (within 10% of the chosen default) across
   all sites
3. Objective function sensitivity is documented (slope near the default)
4. Results are recorded in a dated entry under `data/explore/`

### Validation experiments

The per-layer sweep experiments that will validate these keys are:

- [L3 background settling sweep](../data/experiments/try/l3-background-settling-sweep.md)
- [L4 clustering parameter sweep](../data/experiments/try/l4-clustering-parameter-sweep.md)
- [L5 tracking noise parameter sweep](../data/experiments/try/l5-tracking-noise-parameter-sweep.md)
- [Multi-key interaction grid](../data/experiments/try/multi-key-interaction-grid.md)

## 7. Practical Notes

1. The config file is mandatory and complete-key validated in `internal/config/tuning.go`.
2. Some settling constants are still code defaults (not file keys), notably freeze duration and lock/reacquisition defaults in L3.
3. As L4 ground-surface modelling matures, expect additional dedicated ground-plane keys to be added to this mapping.
4. **Breaking change ahead:** the flat config schema is being restructured into layer-scoped sub-objects (`l3`, `l4`, `l5`, `pipeline`, `optimisation`). See [`CONFIG-RESTRUCTURE.md`](CONFIG-RESTRUCTURE.md) for the migration plan.

## 8. Config Value Drift Prevention

Default values are documented in several places:

- `config/tuning.defaults.json` — **canonical source of truth** for current
  defaults.
- This file (§6) — evidence levels, derivations, and graduation criteria.
- [`data/maths/pipeline-review-open-questions.md` §Q7](../data/maths/pipeline-review-open-questions.md)
  — historical snapshot of defaults with evidence classification (kept for
  context but should reference this file for current values).

**Problem:** Default values duplicated across docs drift when
`tuning.defaults.json` is updated. The Q7 table in the pipeline review
previously listed stale values for 13 of 15 keys.

**Recommended prevention approach:**

1. **Short term — single canonical reference.** This file
   (`config/README.maths.md`) should be the authoritative mapping of
   config keys → maths rationale → evidence level. Other docs should
   reference it rather than embedding default values inline.
2. **Medium term — extend `config-order-sync --check-values`.** The
   existing `scripts/config-order-sync` tool already parses Markdown
   JSON blocks and Go struct tags for key-order consistency. A
   `--check-values` flag could additionally:
   - Parse Markdown tables that contain backtick-quoted config key names
     and a "Default" column
   - Compare extracted values against `tuning.defaults.json`
   - Report mismatches with file, line, key, expected, and actual values
   - Exit non-zero when drift is detected
   - Wire into `make lint-docs` alongside the existing
     `check-config-order` target
3. **Target files for value checking:** This file (§6 tables),
   `data/maths/pipeline-review-open-questions.md` (Q7 table), and the
   experiment sweep tables in `data/experiments/try/`.
