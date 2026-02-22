# Ground Plane and Vector-Scene Maths (Proposal)

**Status:** Proposal Math (Not Active in Current Runtime)
**Layer:** L4 Perception
**Related:** [Background Grid Settling Maths](../../maths/background-grid-settling-maths.md), [Clustering Maths](../../maths/clustering-maths.md), [`docs/lidar/architecture/ground-plane-extraction.md`](../../lidar/architecture/ground-plane-extraction.md), [`docs/lidar/architecture/vector-scene-map.md`](../../lidar/architecture/vector-scene-map.md)

## 1. Scope and Design Intent

This document defines the mathematical model for long-running, high-quality ground estimation in stationary LiDAR deployments.

Current runtime boundary (2026-02-21): this file is proposal-only; implemented runtime ground filtering math is documented in `docs/maths/ground-plane-maths.md`.

It is intentionally conservative:

- bias toward stable geometry over fast adaptation,
- use L3 background confidence as a prior, not as absolute truth,
- keep online math O(1) per point and O(1) per tile fit.

## 2. Representation

Ground is represented as piecewise planar Cartesian tiles.

For tile `j`, plane parameters are:

- unit normal `n_j = [nx, ny, nz]^T`,
- offset `d_j` such that `n_j^T p + d_j = 0`,
- support/confidence metrics.

Height query at `(x, y)` uses:

`z_ground(x,y) = -(nx*x + ny*y + d)/nz` (valid when `|nz|` is not near zero).

## 3. Online Plane Estimation

### 3.1 Streaming covariance/PCA (primary online estimator)

For accepted points `p_k = [x_k, y_k, z_k]^T`, maintain running centroid and covariance with Welford updates:

- `N <- N + 1`
- `delta <- p - mu`
- `mu <- mu + delta / N`
- `C <- C + delta * (p - mu)^T`

Sample covariance is `Sigma = C / N` (or `/ (N-1)` for unbiased estimate).

Plane normal = eigenvector of smallest eigenvalue of `Sigma`.

Why this form:

- numerically safer than raw `sum(xx) - sum(x)^2/N`,
- online O(1) update,
- no point buffering required.

### 3.2 Weighted least squares (optional online variant)

If using `z = a x + b y + c` with per-point weights `w_i`, solve the **3x3** normal equations:

`A * theta = b`, where `theta = [a, b, c]^T`

`A = [[sum(w x^2), sum(w x y), sum(w x)],
      [sum(w x y), sum(w y^2), sum(w y)],
      [sum(w x),   sum(w y),   sum(w)  ]]`

`b = [sum(w x z), sum(w y z), sum(w z)]^T`

Notes:

- This avoids the singular/incorrect 4x4 form.
- Use only when `|nz|` is expected to stay away from zero (ground-like patches).

### 3.3 Offline robust refinement (optional)

For static-map batches, robustify with RANSAC or IRLS on tiles marked unstable.

Online path should remain deterministic and bounded; robust heavy fits are post-process.

## 4. Settlement: Geometry + Density + Time

A tile should settle only when three independent conditions hold.

### 4.1 Geometric confidence

Use a planarity metric from eigen spectrum, for example:

`C_geom = clamp(1 - lambda3/(lambda1 + lambda2 + lambda3 + eps), 0, 1)`

Also gate by residual quality (median absolute point-to-plane distance).

### 4.2 Density/observability confidence

Point count alone is not enough. Require:

- `N_eff` above threshold,
- azimuth diversity (multiple az bins),
- ring/elevation diversity,
- spatial occupancy inside tile.

Example aggregate:

`C_density = f(N_eff, az_coverage, ring_coverage, area_coverage)`

### 4.3 Temporal stability

Require normal/offset stability across a sliding window:

- `angle(n_t, n_{t-1}) < tau_n`
- `|z_ground_t(centre) - z_ground_{t-1}(centre)| < tau_z`

Final settle condition:

`SETTLED if C_geom >= T_geom and C_density >= T_density and C_temporal >= T_temporal for K consecutive windows.`

### 4.4 Region selection maths (what gets fitted and settled)

Ground fitting should not consume all accepted points uniformly. Define candidate
regions `R` as:

1. local Cartesian tiles,
2. region-grown tile components,
3. prior vector-scene surfaces (ground polygons, boundary polylines).

For point `p=(x,y,z)`, evaluate each candidate region with:

`S_R(p) = w_xy * w_z * w_obs * w_geom * w_density * w_prior`

with:

- `w_xy = exp(-d_xy(p,R)^2 / (2*sigma_xy^2))`,
- `w_z = exp(-d_z(p,R)^2 / (2*sigma_z^2))`,
- `w_obs = C_obs(R)` from L3 confidence,
- `w_geom = max(1 - |r_plane|/tau_plane, 0)` (point-to-plane residual gate),
- `w_density = C_density(R)` to penalize under-observed regions,
- `w_prior = C_prior(R)` from vector-scene/global-surface agreement.

Assignment rule:

`R* = argmax_R S_R(p)` and accept only if `S_R*(p) >= T_assign`.

If no region passes `T_assign`, hold point in an unassigned pool; do not force
fit into a low-confidence region.

This prevents cross-surface contamination at kerbs, ramps, walls, and sparse
zones where naive nearest-tile assignment is unstable.

### 4.5 Coupling to current settling parameters (L3 and filter keys)

Even before dedicated ground-plane config keys exist, region selection should
derive gates from current tuning values:

1. `noise_relative`, `safety_margin_meters`, `closeness_multiplier`
   - define scale for `sigma_z` and residual gates.
2. `neighbor_confirmation_count`
   - controls neighborhood support requirement before region admission near seams.
3. `warmup_duration_nanos`, `warmup_min_frames`
   - define when `w_obs` is trusted enough to influence region locking.
4. `post_settle_update_fraction`
   - sets long-run adaptation rate after region lock.
5. `height_band_floor`, `height_band_ceiling`, `remove_ground`
   - pre-gates candidate points before region scoring.

A practical coupling:

`sigma_z(r) = k_close * (noise_relative*r + safety_margin_meters)`

where `k_close = closeness_multiplier` and `r` is point range.

## 5. Density Model and Range Limits

Do not assume isotropic `1/(4*pi*r^2)` coverage for ground settlement decisions.

Use sensor geometry + visibility factors:

- azimuth spacing: `Delta_s_az ~ r * Delta_theta`,
- vertical/ring spacing: `Delta_s_el ~ r * Delta_phi_eff`,
- expected tile hits: approximately proportional to `A_tile / (Delta_s_az * Delta_s_el)` times visibility.

Include penalties for:

- occlusion,
- grazing incidence,
- ring non-uniformity,
- masked dynamic occupancy.

Practical correction:

- treat density curves as empirical, site-specific calibration artifacts,
- keep thresholds adaptive by measured coverage statistics, not fixed radius cutoffs.

## 6. Curvature and Discontinuity Math

### 6.1 Angular curvature

Between adjacent tiles `i, j`:

`kappa_ij = arccos(clamp(n_i^T n_j, -1, 1))`

### 6.2 Height discontinuity (correct form)

Do **not** compare offsets directly (`|d_i-d_j|`) when normals differ.

Instead, evaluate both planes at a shared `(x, y)` (for example edge midpoint `q_xy`):

- `z_i = z_from_plane(i, q_xy)`
- `z_j = z_from_plane(j, q_xy)`
- `Delta_h_ij = |z_i - z_j|`

Local grade across tile spacing `s`:

`grade_ij = Delta_h_ij / s`

This is coordinate-consistent and physically interpretable.

## 7. Interaction with L3 Background Grid (EWA/EMA)

The current L3 grid is an exponential moving update model (often described as EWA/EWMA/EMA in docs).

Use L3 as a reliability prior for L4 input selection:

1. Prefer points from cells with stronger evidence (`TimesSeenCount`, lower spread, not frozen).
2. De-prioritize or reject cells in freeze/reacquisition turbulence windows.
3. Preserve a shadow raw-point channel to avoid lock-in if L3 baseline drifts.

### 7.1 Weighted coupling example

Define per-point fit weight from L3 cell state:

`w_L3 = g(times_seen, spread, frozen_flag, locked_flag)` with `w_L3 in [0,1]`.

Then use `w_total = w_L3 * w_range * w_residual` in plane accumulation.

### 7.2 Consistency monitoring

Track persistent disagreement between L3 and L4 in stable regions:

- if L4 residuals remain low but L3 marks persistent foreground, suspect L3 drift;
- if L3 stable but L4 residuals rise, suspect mixed surfaces or geometry change.

This avoids one-way authority and supports long-running static operation.

### 7.3 Region-level coupling for vector scene map

When polygon/polyline priors are available (`docs/lidar/architecture/vector-scene-map.md`):

1. Use ground polygons as region seeds.
2. Use boundary polylines as split constraints (do not blend across them).
3. Use prior surface class (ground/structure/volume) to suppress invalid fits.

Per-point fit weight becomes:

`w_total = w_L3 * w_region * w_prior * w_residual`

where `w_region` comes from Section 4.4 scoring and `w_prior` penalizes
deviation from global-surface priors.

This makes L3 EWA and L4 geometry complementary: L3 stabilizes observation
trust, while region-aware L4 prevents over-smoothing across real boundaries.

## 8. Tier-2 Global Merge (Important Limits)

Coarse geodetic tiles can erase meaningful local geometry.

Recommendations:

1. Keep Tier-1 local high-resolution geometry as authoritative for runtime queries.
2. In Tier-2, store dispersion and sample support, not only means.
3. Renormalize averaged normals and reject opposite-direction merges.
4. Avoid blind merge when divergence exceeds thresholds; mark for revalidation.

## 9. Simplifications and Their Limits

1. **Single-plane per tile**
   - Simplifies fitting and memory.
   - Fails at curbs/ramps/compound surfaces crossing one tile.
   - Region selection plus polygon/polyline splits are required to avoid
     systematic seam errors.
2. **Locally linear z(x,y)**
   - Works for non-vertical ground-like patches.
   - Not valid for vertical/overhang surfaces.
3. **Stationary-scene bias**
   - Good for long-running background convergence.
   - Slower adaptation during genuine infrastructure change.
4. **Independent tile fitting**
   - Simple and parallel.
   - No intrinsic continuity constraints; seams must be handled explicitly.
   - Vector-scene region constraints reduce, but do not eliminate, seam drift.
5. **Thresholded settlement gates**
   - Operationally transparent.
   - Threshold tuning can be site-dependent and may need auto-calibration.

## 10. Computational Profile (Order-of-Growth)

For `N` accepted points and `T` active tiles per frame:

- accumulation: `O(N)`,
- fit refresh: `O(T)` with constant-size eigensolves,
- adjacency/curvature: `O(T)`.

Dominant cost is point admission/filtering and memory locality, not eigensolve math.

## 11. Recommended Long-Running Static Pipeline

1. **Warmup:** conservative acceptance, no hard downstream dependence.
2. **Settle:** require geometry+density+temporal gates.
3. **Lock:** slow adaptation for stable tiles; monitor divergence statistics.
4. **Reacquire:** only for sustained, validated change signals.
5. **Audit:** periodic residual and confidence diagnostics against L3 baselines.

This is the preferred mode for neighborhood monitoring where reliability and drift resistance matter more than rapid transient adaptation.
