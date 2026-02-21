# Velocity-Coherent Foreground Extraction Math

**Status:** Final pre-ratification proposal (review integrated)
**Date:** February 20, 2026
**Version:** 2.0

This document is the final proposal revision before ratification. It integrates the
review feedback previously captured separately and replaces the need for a
standalone review file.

---

## 0. Integrated Decisions (Final)

1. **Future FFT radar overlay in vector space:** yes. Fusion with vector-plane
   polylines and compressible vector-point infrastructure becomes easier with
   variance-normalised velocity modelling and track-level velocity semantics.
2. **Velocity noise modelling:** yes. Explicit velocity covariance is required.
3. **Metric scaling fix:** yes. Use Mahalanobis-like normalised distance.
4. **Clustering architecture:** use **two-stage** clustering.
5. **Track continuity fixes:** yes. Reuse Kalman covariance propagation and
   variance-aware merge scoring.
6. **Foreground architecture:** use **parallel channels**, not cyclic feedback.
7. **Validation spec:** required with metrics, procedures, labelling, and
   acceptance thresholds.
8. **Complexity claim correction:** yes. Avoid optimistic 6D-index assumptions;
   validate runtime empirically.
9. **Execution posture:** proceed with this integrated proposal as final
   pre-ratification review state.

---

## 1. Problem Setup

At frame `t`, let the world-frame point cloud be `P_t = {p_i^t}` with each
point `p_i^t = (x, y, z)`.

Goal: improve sparse-object recall (as low as 3 points), reduce fragmentation at
entry/exit boundaries, and recover short occlusions while preserving precision
and real-time performance.

---

## 2. Velocity Estimation and Future Radar Overlay

### 2.1 Cluster-level velocity baseline

Point-level nearest-neighbour correspondence is ill-posed for sparse spinning
LiDAR sampling. The baseline path is:

1. spatial clustering (existing robust L4 behaviour),
2. cluster-to-track association (existing L5 assignment),
3. per-point velocity inheritance from Kalman-filtered track velocity.

For tracked clusters:

```text
v_i = v_track
```

For untracked/new clusters:

```text
v_i = 0  (or unknown-state fallback)
```

### 2.2 Velocity noise model

Velocity use in gating and distance metrics must include covariance:

```text
Σ_v(i) = Σ_v,track(i) + Σ_v,floor
```

where `Σ_v,track` comes from Kalman covariance and `Σ_v,floor` prevents
over-confidence.

### 2.3 Future FFT radar overlay answer

When FFT radar Doppler data is available, map radar velocity into the same
vector plane using nearest valid spatial association with uncertainty weighting:

```text
v_fused = argmin_r d(p_i, p_r),   d <= r_radar
```

with confidence weighting by range and association distance. This architecture
makes radar/LiDAR value marriage in vector space easier than point-correspondence
approaches.

---

## 3. Normalised Position-Velocity Metric

Use a dimensionless Mahalanobis-like metric:

```text
D²(u_i, u_j) = Δx^T Σ_x^{-1} Δx + Δv^T Σ_v^{-1} Δv
```

where `u_i = (x, y, z, vx, vy, vz)`, and `Σ_x`, `Σ_v` are range-aware diagonal
covariance models.

This removes unit mismatch between metres and metres/second and gives `eps`
interpretable meaning in sigma-space.

---

## 4. Two-Stage Clustering (Chosen)

### Stage 1: spatial clustering (robust gate)

Use the existing spatial DBSCAN-style clustering path and retain robust minimum
support (no global 6D MinPts=3 shift).

### Stage 2: velocity refinement/split

Within each spatial cluster, evaluate velocity variance. Split only when
variance exceeds threshold and accept sub-clusters only if confidence and
variance gates pass.

This preserves existing noise rejection while adding velocity discrimination.

---

## 5. Track Continuity and Fragment Handling

### 5.1 Prediction

Use existing Kalman propagation (not linear ad-hoc radius growth):

```text
P(τ) = F P_0 F^T + Q(τ)
```

with Mahalanobis association gating.

### 5.2 Sparse continuation

Replace discrete point-count tiers with continuous scaling:

```text
tolerance(n) = k / sqrt(n)
```

clamped to physically plausible bounds.

### 5.3 Fragment merge scoring

Replace equal-weight arithmetic averaging with variance-aware scoring:

```text
Λ_merge = Σ component log-likelihood terms
```

and accept only when `Λ_merge >= Λ_min` and temporal constraints pass.

---

## 6. Foreground Extraction Architecture (No Cycles)

The sparse-recall bottleneck remains at foreground classification. Chosen design:
parallel channels with union, avoiding cyclic dependencies.

- **Channel A:** existing EMA/range foreground logic.
- **Channel B:** temporal-gradient motion foreground logic.
- **Output:** `foreground = A OR B`.

No L5→L3 feedback loop is introduced; code stays acyclic and easier to reason
about, tune, and validate.

---

## 7. Complexity, Tuning Hotspots, and Config Keys

### 7.1 Complexity stance

Do not rely on optimistic 6D index assumptions. With the two-stage design,
practical complexity remains dominated by low-dimensional operations and bounded
candidate sets. Validate latency empirically on replay.

### 7.2 Tuning hotspots

- velocity split threshold,
- sparse continuation tolerance,
- merge likelihood threshold,
- temporal-channel motion threshold.

### 7.3 Config keys to add

| Key                                   | Default | Purpose                                             |
| ------------------------------------- | ------- | --------------------------------------------------- |
| `clustering.split.sigma2_v`           | `4.0`   | Velocity-variance split threshold (m²/s²)           |
| `clustering.split.q_min`              | `0.5`   | Minimum confidence for sparse velocity sub-clusters |
| `clustering.metric.eps_sigma`         | `3.0`   | Dimensionless neighbourhood radius in sigma-space   |
| `tracking.sparse.k_tol`               | `2.0`   | Sparse continuation scale factor                    |
| `tracking.merge.lambda_min`           | `-6.0`  | Minimum merge log-likelihood score                  |
| `tracking.predict.tau_max_s`          | `3.0`   | Maximum prediction/coast horizon                    |
| `foreground.temporal.enabled`         | `true`  | Enable temporal-gradient foreground channel         |
| `foreground.temporal.v_min_mps`       | `1.0`   | Minimum motion for temporal foreground trigger      |
| `foreground.channel_union_mode`       | `"or"`  | Foreground channel union policy                     |
| `validation.temporal_overlap_min`     | `0.7`   | Minimum labelled trajectory coverage fraction       |
| `validation.occlusion_gap_min_frames` | `2`     | Lower short-occlusion bound                         |
| `validation.occlusion_gap_max_frames` | `50`    | Upper short-occlusion bound                         |
| `performance.frame_budget_ms_p95`     | `50`    | Target p95 per-frame latency budget                 |

---

## 8. Expected Benefits (to Validate)

Relative to baseline, expected outcomes:

- sparse recall (`3–11` points): `+20%` to `+40%`,
- fragmentation: `-15%` to `-25%`,
- boundary continuity: `+10%` to `+30%`,
- short-occlusion recovery: material uplift,
- precision: controlled impact (no unacceptable degradation),
- runtime: within configured frame budget.

---

## 9. Validation and Acceptance Specification

### 9.1 Labelling and data

- Curate representative replay segments.
- Label per-frame vehicle identity and trajectory continuity.
- Mark entry/exit and occlusion windows.

### 9.2 Primary metrics

- sparse recall,
- precision,
- fragmentation (ID-switch proxy),
- occlusion recovery for short gaps (`2–50` frames),
- p95 frame latency.

Occlusion recovery counts only when the same track ID appears before the gap and
again on the first non-occluded frame after the gap.

### 9.3 Acceptance gates

- sparse recall uplift: **>= +20%**,
- fragmentation improvement: **>= -15%**,
- precision change: **>= -5% absolute floor from baseline**,
- runtime: **p95 <= frame budget**.

### 9.4 Procedure

1. Run baseline and candidate pipelines on identical replay data.
2. Compute paired per-segment metric deltas.
3. Report confidence intervals and decision against gates.
4. Audit known failure modes (false sparse clusters, wrong merges, ghost tracks,
   temporal-channel false positives).
5. Freeze tuned parameters before ratification.

---

## 10. Ratification Note

This document is the single proposal artifact for this branch and is treated as
the final review-integrated version before ratification.

## References

- Bernardin, K. & Stiefelhagen, R. (2008). "Evaluating Multiple Object Tracking
  Performance: The CLEAR MOT Metrics." _EURASIP Journal on Image and Video
  Processing._
- Dewan, A., Caselitz, T., Tipaldi, G. D. & Burgard, W. (2016). "Rigid Scene
  Flow for 3D LiDAR Scans." _IEEE/RSJ International Conference on Intelligent
  Robots and Systems (IROS)._
