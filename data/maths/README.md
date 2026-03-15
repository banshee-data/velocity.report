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

### Active Maths (implemented in current runtime)

- [Background Grid Settling Maths](background-grid-settling-maths.md)
  — Polar-cell EWA/EMA update equations, warmup/settling state machine, freeze/lock behaviour, and confidence dynamics.
- [Ground Plane Maths](ground-plane-maths.md)
  — Tile/region plane estimation, region-selection math, robust confidence/settlement criteria, curvature math, density constraints, and L3-L4 interaction.
- [Clustering Maths](clustering-maths.md)
  — Downsampling, neighbourhood indexing, DBSCAN, cluster geometry extraction (medoid + OBB/PCA), and complexity bounds.
- [Tracking Maths](tracking-maths.md)
  — CV Kalman model, Mahalanobis gating, Hungarian assignment, lifecycle transitions, and stability metrics.

### Proposals (not yet active — see [Roadmap](#prioritised-proposal-roadmap) below)

- [OBB Heading Stability Review](proposals/20260222-obb-heading-stability-review.md) — **Partially Implemented**
  — Root cause analysis of spinning bounding boxes: PCA ambiguity, axis swaps, dimension averaging, and renderer mismatches. Guard 3 (90° jump rejection), fixes B, C, G applied; remaining fixes superseded by geometry-coherent model.
- [Geometry-Coherent Track State](proposals/20260222-geometry-coherent-tracking.md)
  — Per-track Bayesian geometry model replacing reactive guards with axis selection via likelihood test, uncertainty-gated EMA updates, shape classification, and heading-motion coupling.
- [Velocity-Coherent Foreground Extraction](proposals/20260220-velocity-coherent-foreground-extraction.md)
  — Layer-integrated (L3/L4/L5) velocity/acceleration estimation, covariance-aware confidence, low-speed heading stability policy, and layer-scoped optimisation/evaluation protocol. [Implementation plan](../../docs/plans/lidar-velocity-coherent-foreground-extraction-plan.md).
- [Ground Plane and Vector-Scene Maths](proposals/20260221-ground-plane-vector-scene-maths.md)
  — Streaming PCA ground estimation, multi-criteria settlement (geometry + density + time), region-selection scoring, and vector-scene integration.
- [Reflective Sign and Static Surface Pose Anchors](proposals/20260310-reflective-sign-pose-anchor-maths.md)
  — Use sign-first reflective anchors with controlled fallback to walls, facades, and ground-support surfaces for frame-to-frame micro-pose estimation; the base case stops at L7/L8 diagnostics, while the reference extension adds a cached stability signal for L3 settling/reset control.
- [Unify L3/L4 Settling](proposals/20260219-unify-l3-l4-settling.md)
  — Overlap analysis, interference risks, and a single-settlement architecture with shared lifecycle per surface-region key.
- Bodies in Motion Maths (proposal — to be written)
  — CA/CTRV state equations, IMM blending and transition matrix, corridor probability model, sparse-cluster gating extensions, and scene-graph relation confidence accumulation. [Design doc](../../docs/plans/lidar-bodies-in-motion-plan.md).

---

## Prioritised Proposal Roadmap

Work items drawn from the proposals above, ordered by user-visible impact and
dependency readiness. Each item can be implemented independently, but earlier
items improve later ones.

### P1 — Geometry-Coherent Track State _(highest priority)_

**Source:** [geometry-coherent-tracking.md](proposals/20260222-geometry-coherent-tracking.md)
**Layer:** L5 tracking
**Status:** Proposal — not started
**Effort:** L (6–7 days)
**Dependencies:** None (works standalone; enhanced by P2)

Replaces the reactive OBB guards (aspect-ratio lock, 90° jump rejection,
dimension sync) with a single Bayesian geometry model per track. Each frame's
PCA observation is tested in both axis interpretations; the interpretation with
the lower Mahalanobis residual wins. Uncertainty shrinks with observations,
shape classification modulates heading trust, and motion coupling provides a
heading prior when velocity data is available.

**Why first:** Directly fixes the most visible user-facing problem (bounding
boxes that spin, change shape, or fail to capture all cluster points). No
upstream changes required.

**Remaining OBB review work subsumed:** Fixes D (threshold tuning), E (renderer
consistency), and F (debug cluster rendering) from the
[OBB heading stability review](proposals/20260222-obb-heading-stability-review.md)
become unnecessary once the geometry-coherent model replaces the guards.

### P2 — Velocity-Coherent Foreground Extraction

**Source:** [velocity-coherent-foreground-extraction.md](proposals/20260220-velocity-coherent-foreground-extraction.md)
**Layer:** L4 perception (pre-clustering)
**Status:** Proposal — not started
**Effort:** L (estimated)
**Dependencies:** None (independent of P1, but improves P1 when combined)
**Plan:** [lidar-velocity-coherent-foreground-extraction-plan.md](../../docs/plans/lidar-velocity-coherent-foreground-extraction-plan.md)

Enriches each foreground point with a per-frame velocity estimate via
track-assisted foreground promotion (L3), engine-selectable two-stage
clustering with motion-coherence refinement (L4), and CV/CA-capable tracking
with low-speed heading freeze policy (L5). Evaluation is layer-scoped and
confidence-backed using replay comparisons and CI gates.

**Why second:** Provides per-cluster velocity vectors that tighten P1's
heading-motion prior. Also independently improves sparse object recall by
20–40 % and reduces fragmentation by 10–25 % (hypothesised, pending
validation).

### P3 — Ground Plane and Vector-Scene Maths

**Source:** [ground-plane-vector-scene-maths.md](proposals/20260221-ground-plane-vector-scene-maths.md)
**Layer:** L4 perception (ground surface)
**Status:** Proposal — not started
**Effort:** L (estimated)
**Dependencies:** Benefits from P4 (shared settling); can start independently

Replaces the existing basic ground model with streaming PCA plane estimation
per tile, multi-criteria settlement (geometry fitness + density/observability

- temporal stability), scored region selection, and vector-scene polygon/polyline
  integration.

**Why third:** Improves foreground/background separation quality, which feeds
into clustering (P2) and tracking (P1). Important for long-running
deployment accuracy and drift resistance. Less immediately visible to
end users than P1/P2.

### P4 — Unify L3/L4 Settling

**Source:** [unify-l3-l4-settling.md](proposals/20260219-unify-l3-l4-settling.md)
**Layer:** L3–L4 boundary (infrastructure)
**Status:** Proposal — not started
**Effort:** M–L (estimated, phased migration)
**Dependencies:** None; simplifies P3 implementation

Replaces independent settling lifecycles in L3 (range baseline) and L4 (ground
geometry) with a shared `SettlementCore` per surface-region key. One warmup,
one freeze/thaw policy, one confidence substrate — two model outputs.

**Why fourth:** Infrastructure simplification that reduces operational
complexity and config coupling drift. Less user-visible impact on its own,
but lowers the complexity cost of P3.

### Candidate Add-on — Reflective Sign and Static Surface Pose Anchors

**Source:** [reflective-sign-pose-anchor-maths.md](proposals/20260310-reflective-sign-pose-anchor-maths.md)
**Layers:** L2 Frames, L3 Grid, L4 Perception, L5 Tracks, L6 Objects, L7 Scene, L8 Analytics
**Status:** Proposal — not started
**Effort:** M (estimated)
**Dependencies:** Best with L7 scene anchors; analytics-only mode can start earlier

Uses highly reflective static signs as the preferred anchors, then falls back
through reflective patches, wall/facade planes, and low-authority ground
support when signs are absent or occluded. The strict base case keeps this in
L7/L8 as diagnostics and scorecards only; the reference extension publishes a
cached stability signal for L3 warmup/reacquire control and may optionally
correct replay/live geometry. This is not a replacement for static pose
configuration or full ego-motion; it is a stationary-sensor stability aid.

**Why it matters:** It gives the system a physically meaningful static
reference for mount vibration, transform drift, and noise diagnosis even in
sign-poor scenes, provided there is enough persistent wall, facade, or ground
structure to build a redundant anchor set.

### Maintenance — OBB Heading Stability Review (remaining items)

**Source:** [obb-heading-stability-review.md](proposals/20260222-obb-heading-stability-review.md)
**Status:** Guard 3, fixes B, C, G **implemented**. Fix D (config-only), E, F not started.

Fix D (tighten aspect-ratio lock threshold) is a low-risk config change that
can be applied any time. Fixes E and F provide incremental improvement but
are **superseded by P1** — once the geometry-coherent model lands, the guards
they improve will be removed.

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

- Endpoint contracts, serialisation schemas, and UI payload shaping.
- Storage-oriented table/column walkthroughs unless mathematically required.
- Feature backlog and implementation scheduling (kept in plans/architecture docs).
