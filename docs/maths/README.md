# LiDAR Maths: Assumptions and Architecture

This folder documents the mathematically significant parts of the LiDAR pipeline.

Scope:

- Covers estimation, filtering, optimisation, gating, and confidence math.
- Covers subsystem-level assumptions and approximation limits.
- Excludes CRUD/data-shape plumbing and API transport details.

## High-Level System View

The production pipeline uses four math-heavy layers:

1. **L3 background grid settling (polar EWA/EMA model)**
   - Learns stable per-cell range baselines and variance envelopes.
   - Produces foreground/background decisions and confidence surrogates.
2. **L4 ground surface modelling (Cartesian local planes)**
   - Fits piecewise local ground geometry from static observations.
   - Produces height-above-ground and surface-confidence outputs.
3. **L4 clustering (density + geometry extraction)**
   - Groups foreground points into object candidates using DBSCAN.
   - Derives medoid centroids and PCA/OBB geometry.
4. **L5 tracking (state estimation + assignment)**
   - Predicts and updates object states with a CV Kalman filter.
   - Solves global data association with Hungarian assignment.

## Core Assumptions (Cross-Cutting)

1. **Primary deployment is stationary-sensor traffic monitoring.**
   Long-running convergence and conservative adaptation are preferred over fast but noisy adaptation.
2. **Most of the scene is static most of the time.**
   Dynamic occupancy is sparse relative to road/building background.
3. **Distance noise grows with range.**
   Thresholds and confidence should scale with distance and observed spread.
4. **Ground is locally smooth but not globally planar.**
   Piecewise planes are valid at tile scale, not at whole-scene scale.
5. **Kinematics are bounded.**
   Tracking gates rely on plausible position jumps and speeds.

## Architecture-Level Couplings

1. **Background grid to ground plane:**
   - L3 confidence (`TimesSeenCount`, spread, freeze/locked state) gates which points are trusted for L4 fitting.
2. **Ground plane to clustering/tracking:**
   - Height-above-ground and local surface orientation improve semantic stability and false-positive rejection.
   - Region-selection confidence controls which surface model receives each point near boundaries.
3. **Clustering to tracking:**
   - Cluster geometry uncertainty appears in association residuals, gating, and lifecycle stability.
4. **Tracking to diagnostics/tuning:**
   - Alignment, jitter, fragmentation, and gating metrics close the loop on parameter quality.

## Detailed Documents

- [Background Grid Settling Maths](background-grid-settling-maths.md)
  - Polar-cell EWA/EMA update equations, warmup/settling state machine, freeze/lock behaviour, and confidence dynamics.
- [Ground Plane Maths](ground-plane-maths.md)
  - Tile/region plane estimation, region-selection math, robust confidence/settlement criteria, curvature math, density constraints, and L3-L4 interaction.
- [Clustering Maths](clustering-maths.md)
  - Downsampling, neighbourhood indexing, DBSCAN, cluster geometry extraction (medoid + OBB/PCA), and complexity bounds.
- [Tracking Maths](tracking-maths.md)
  - CV Kalman model, Mahalanobis gating, Hungarian assignment, lifecycle transitions, and stability metrics.
- [Unify L3/L4 Settling Proposal](proposals/20260219-unify-l3-l4-settling.md)
  - Overlap analysis, interference risks, and a single-settlement architecture updated for polygon/polyline region keys.
- [OBB Heading Stability Review](proposals/20260222-obb-heading-stability-review.md)
  - Root cause analysis of spinning bounding boxes: PCA ambiguity, axis swaps, dimension averaging, and renderer mismatches. Proposes six targeted fixes.

## Config Mapping

Note on naming: this repository does **not** contain a `config/tracking.json` file. Runtime tuning is loaded from `config/tuning.defaults.json` (or another JSON passed with `--config`) via `internal/config/tuning.go`.

### L3 Background Grid Settling Maths (`background-grid-settling-maths.md`)

- Keys:
  - `background_update_fraction`
  - `closeness_multiplier`
  - `safety_margin_meters`
  - `noise_relative`
  - `neighbor_confirmation_count`
  - `seed_from_first`
  - `warmup_duration_nanos`
  - `warmup_min_frames`
  - `post_settle_update_fraction`
- Getter/source path:
  - `internal/config/tuning.go`
- Runtime mapping:
  - `internal/lidar/l3grid/config.go` (`BackgroundConfigFromTuning`)
  - `internal/lidar/l3grid/foreground.go` and `internal/lidar/l3grid/background.go`
- Important non-file defaults still applied in code:
  - freeze duration, lock thresholds, reacquisition boost (`internal/lidar/l3grid/config.go`, `internal/lidar/l3grid/foreground.go`)

### L4 Ground Surface Maths (`ground-plane-maths.md`)

- Current config status:
  - No dedicated ground-plane tuning block is wired yet.
- Effective upstream controls:
  - L3 settling keys above (input quality to L4)
  - `height_band_floor`, `height_band_ceiling`, `remove_ground`
  - region-selection gates derived from `noise_relative`, `safety_margin_meters`, `closeness_multiplier`, and `neighbor_confirmation_count`
- Runtime mapping:
  - `cmd/radar/radar.go` -> `internal/lidar/pipeline/tracking_pipeline.go` -> `internal/lidar/l4perception/ground.go`

### L4 Clustering Maths (`clustering-maths.md`)

- Keys:
  - `foreground_dbscan_eps`
  - `foreground_min_cluster_points`
  - `max_cluster_diameter`
  - `min_cluster_diameter`
  - `max_cluster_aspect_ratio`
- Getter/source path:
  - `internal/config/tuning.go`
- Runtime mapping:
  - `internal/lidar/l4perception/cluster.go` (`DefaultDBSCANParams`)
  - pipeline use in `internal/lidar/pipeline/tracking_pipeline.go`

### L5 Tracking Maths (`tracking-maths.md`)

- Keys:
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
- Getter/source path:
  - `internal/config/tuning.go`
- Runtime mapping:
  - `internal/lidar/l5tracks/tracking.go` (`TrackerConfigFromTuning`)
  - tracker wiring in `cmd/radar/radar.go`

### Pipeline Timing/Persistence Controls (cross-cutting)

- Keys:
  - `buffer_timeout`
  - `min_frame_points`
  - `flush_interval`
  - `background_flush`
  - `enable_diagnostics`
- Runtime mapping:
  - pipeline/bootstrap in `cmd/radar/radar.go`
  - background flusher in `internal/lidar/l3grid/background_flusher.go`

## What Is Intentionally Not Here

- Endpoint contracts, serialization schemas, and UI payload shaping.
- Storage-oriented table/column walkthroughs unless mathematically required.
- Feature backlog and implementation scheduling (kept in plans/architecture docs).
