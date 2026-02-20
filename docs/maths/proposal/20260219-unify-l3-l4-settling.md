# Proposal: Unify L3/L4 Settling

**Status:** Proposal
**Scope:** L3 background settling + L4 ground-surface settling harmonisation
**Related:** [`docs/maths/background-grid-settling-maths.md`](../../maths/background-grid-settling-maths.md), [`docs/maths/ground-plane-maths.md`](../../maths/ground-plane-maths.md), [`docs/lidar/architecture/vector-scene-map.md`](../../lidar/architecture/vector-scene-map.md)

## 1. Problem Statement

Today, L3 and proposed L4 both perform settling-like behavior:

- L3 settles a **range-baseline model** (EMA/EWA, freeze/lock, warmup suppression).
- L4 settles a **surface-geometry model** (plane fit confidence, temporal stability).

Running them independently creates duplicated work and inconsistent readiness states.

## 2. Overlap and Duplication

## 2.1 Shared concerns implemented twice

1. Warmup windows and temporal gating.
2. Confidence accumulation from repeated observations.
3. Outlier handling (reject/freeze/reacquire patterns).
4. Slow-lock behavior for long-running static scenes.

## 2.2 Where problems occur

1. **Double-settling latency**
   - If L4 only trusts settled L3 points and then applies its own settling timer, effective convergence is delayed.
2. **State disagreement**
   - L3 may be "stable" while L4 is "unsettled" (or vice versa), making downstream behavior discontinuous.
3. **Feedback starvation**
   - L3 rejection can starve L4 geometry updates, which then cannot help refine dynamic ground thresholds.
4. **Config coupling drift**
   - Different thresholds in different stages can fight each other and produce brittle tuning.

## 3. Interference Assessment

- **Data-path interference:** High (same observations influence both models).
- **Behavioral interference:** High (foreground gating changes L4 input quality directly).
- **CPU/memory interference:** Medium (both are linear-time, but duplicate update logic and state increases churn).
- **Operational complexity:** High (two independent settling lifecycles to reason about).

## 4. Should we settle once?

Recommendation: **settle once at the observation-confidence layer, validate twice at outputs**.

Interpretation:

1. One shared settling state machine controls warmup/freeze/reacquire/lock timing.
2. L3 and L4 keep separate model outputs, but consume shared confidence state.
3. L4 geometric validity remains distinct (cannot be replaced by L3 range stability), but it should not run an independent warmup policy.

## 5. Unified Architecture

Introduce a shared `SettlementCore` per **surface region key**, not only per
polar cell.

`SurfaceRegionKey = (surface_class, local_region_id, optional_global_feature_id)`

This aligns with vector scene map planning where runtime geometry may belong to:

- ground polygons,
- structure polylines/polygons,
- volume regions.

Per key maintain:

- `C_obs` (observation stability from L3 statistics),
- `C_geom` (geometry fitness from L4 residual/eigenspectrum),
- `C_temp` (temporal consistency),
- `C_region` (region assignment confidence),
- common lifecycle state.

### 5.1 Shared lifecycle

`EMPTY -> LEARNING -> REGION_BOUND -> OBS_STABLE -> GEOM_STABLE -> LOCKED`

- `OBS_STABLE`: sufficient for L3 background confidence usage.
- `GEOM_STABLE`: sufficient for ground-surface queries.
- `LOCKED`: slow-adaptation mode for long-running static monitoring.

### 5.2 Shared freeze/reacquire policy

- One freeze trigger policy (large residual relative to adaptive threshold).
- One thaw policy.
- One reacquisition acceleration policy.

Both L3 and L4 subscribe to the same transition events.

## 6. Proposed Data Flow

1. Ingest point.
2. Update L3 baseline stats (range mean/spread/confidence).
3. Perform region selection (`C_region`) using local tiles + polygon/polyline priors.
4. Compute `C_obs` once.
5. Compute L4 candidate weight from `C_obs`, `C_region`, and geometric residual guards.
6. Update L4 accumulators and `C_geom`.
7. Update shared lifecycle state from `C_obs`, `C_geom`, `C_temp`, `C_region`.
8. Emit:
   - L3 foreground/background decision.
   - L4 ground-surface readiness and confidence.

## 7. Configuration Simplification

Unify stage-control parameters:

- warmup duration/frames,
- freeze duration and threshold multiplier,
- lock entry threshold,
- reacquisition boost behavior.

Keep model-specific parameters separate:

- L3 range thresholding params,
- L4 plane-fit and residual thresholds,
- clustering/tracking params.

### 7.1 Immediate mapping to existing config

With current schema (`config/tuning.defaults.json`), merged settling should map:

- lifecycle timing:
  - `warmup_duration_nanos`
  - `warmup_min_frames`
  - `post_settle_update_fraction`
- observation confidence scale:
  - `noise_relative`
  - `safety_margin_meters`
  - `closeness_multiplier`
  - `neighbor_confirmation_count`
- pre-gating to avoid class confusion:
  - `height_band_floor`
  - `height_band_ceiling`
  - `remove_ground`

No second L4-only warmup timer should remain after merge.

## 8. Consistency Check Against Ground-Plane + Vector-Scene Planning

### 8.1 What is already consistent

1. Stationary-scene bias and lock/freeze semantics are aligned.
2. Piecewise local geometry assumption matches both tile and polygon models.
3. Long-running conservative adaptation is shared across docs.

### 8.2 Current inconsistencies

1. Proposal keys `polar cell + Cartesian tile` are too narrow for polygon/polyline surfaces.
2. Proposal lacks explicit region-selection maths, while vector scene depends on
   region growing and boundary-aware splits.
3. Proposal treats vector scene as post-process only; in practice, global-surface
   priors should feed online region assignment and settling confidence.
4. Proposal does not distinguish surface class in lifecycle state, creating risk
   of blending ground and vertical structures.

### 8.3 Required alterations for merged L3/L4 settling

1. Replace per-cell keying with `SurfaceRegionKey`.
2. Add `C_region` into shared state and transition guards.
3. Add boundary constraints from polylines so assignment does not cross known edges.
4. Feed global-surface priors as soft weights, not hard overrides.
5. Keep separate readiness outputs by class:
   - `READY_BG` for L3 range decisions,
   - `READY_GROUND_GEOM` for height queries,
   - `READY_STRUCTURE` / `READY_VOLUME` for vector-scene context.

## 9. Migration Plan

1. **Phase A (No behavior change):**
   Add shared `SettlementCore` telemetry in parallel with existing logic.
2. **Phase B (Control unification):**
   Route warmup/freeze/lock transitions through shared lifecycle; keep model math unchanged.
3. **Phase C (Input unification):**
   Use shared confidence (`C_obs`) as first-class weight for L4 updates.
4. **Phase D (Cleanup):**
   Remove duplicated settling timers/counters from L4 path.

## 10. Risks and Mitigations

1. **Risk:** Over-coupling L4 to L3 mistakes.
   - **Mitigation:** Keep raw-point shadow path and cap L3 influence by weight, not hard gate.
2. **Risk:** Regression in transient scenes.
   - **Mitigation:** Preserve per-model residual checks and monitor disagreement metrics.
3. **Risk:** Tuning churn during migration.
   - **Mitigation:** staged rollout with compatibility mode and comparative diagnostics.

## 11. Decision Summary

- Do not run two independent settling lifecycles.
- Keep two model outputs (range baseline vs geometry).
- Share one lifecycle controller and one confidence substrate, including region
  confidence tied to polygon/polyline-aware assignment.

This yields lower interference, less duplicated logic, and clearer operational behavior.
