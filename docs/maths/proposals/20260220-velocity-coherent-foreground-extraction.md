# Velocity-Coherent Foreground Extraction Math

**Status:** Proposal Math (Supersedes Prior Review Split)
**Version:** 2.0

This document is the single mathematical proposal for velocity-accurate LiDAR
tracking in this branch. It keeps the existing layered architecture and does
not introduce intermediary ".5" layers.

Implementation sequencing and task ownership can continue to live in planning
artefacts, but the math and evaluation contract are defined here.

---

## 1. Scope and Architectural Constraint

Pipeline remains strictly:

1. **L3** foreground extraction (background EMA + gating)
2. **L4** clustering/perception (spatial candidate extraction + motion-coherent refinement)
3. **L5** tracking/state estimation (velocity, acceleration, heading stability)

No additional intermediary layers are introduced.

---

## 2. Primary Objective

Optimise for the most accurate motion estimates for tracked clusters:

1. accurate velocity vector `v`
2. accurate speed `||v||`
3. accurate speed change / acceleration `a = dv/dt`
4. stable low-speed heading (no box spin / direction chatter)
5. quantified confidence for motion estimates

This proposal treats runtime and evaluation as separate concerns:

1. runtime path prioritises robust online estimation
2. evaluation path can use stronger offline smoothing for truth-aligned analysis

---

## 3. L3 Mathematics: Track-Assisted Foreground Promotion

### 3.1 Baseline foreground decision (retained)

For each polar point `p` in cell `c`, background likeness remains governed by
range residual against settled baseline:

`r = |d(p) - mu_c|`

`tau_c = k_close * (sigma_c + sigma_range(d) + eps) + m_safe`

with distance-scaled sensor noise model:

`sigma_range(d) = alpha_noise * d + beta_noise`

### 3.2 Motion-aware promotion (new in L3 decision path)

When baseline logic classifies a point as background but residual is near gate,
apply promotion test against predicted track states from L5:

`near_gate := r in [gamma1 * tau_c, gamma2 * tau_c]`

For each candidate track `j` with predicted position `x_hat_j` and covariance
`P_j`, compute motion proximity score:

`z_j = p_xy - x_hat_j`

`m_j = exp(-0.5 * z_j^T * S_j^{-1} * z_j)`

where `S_j` is a position uncertainty projection using track covariance and
range-dependent point uncertainty.

Promotion condition:

`foreground <- foreground OR (near_gate AND max_j(m_j) >= theta_promote)`

### 3.3 Per-point foreground confidence output

Each point emits confidence (not only binary mask):

`q_fg = clamp(w_bg * q_bg + w_motion * max_j(m_j), 0, 1)`

where `q_bg` is the baseline confidence term from residual vs threshold and
observation support.

---

## 4. L4 Mathematics: Engine-Selectable Two-Stage Clustering

L4 engine is configurable. Default highest-accuracy engine:

`two_stage_mahalanobis_v2`

### 4.1 Stage A: spatial candidate extraction

Use spatial DBSCAN/HDBSCAN candidate extraction in `(x, y)` (optionally `z`
for tie-breaking), preserving existing operational constraints on point count
and cluster geometry.

### 4.2 Stage B: velocity-coherence split/merge

Refine Stage A candidates with track-informed motion coherence.

For candidate cluster pair or point/cluster refinement:

`D_m^2 = Delta_x^T * Sigma_x^{-1} * Delta_x + Delta_v^T * Sigma_v^{-1} * Delta_v`

where:

1. `Sigma_x` is position uncertainty (cluster + track projected)
2. `Sigma_v` is velocity uncertainty from L5 covariance

Use `D_m` gates for:

1. splitting spatially merged candidates with incompatible motion
2. merging fragmented candidates with compatible motion

Raw 6D Euclidean DBSCAN is retained only as optional debug engine, not default.

---

## 5. L5 Mathematics: Velocity, Acceleration, Confidence, Heading Stability

### 5.1 Motion model engines

L5 engine is configurable:

1. `cv_kf_v1` (fallback / realtime conservative)
2. `imm_cv_ca_v2` (default accuracy runtime)
3. `imm_cv_ca_rts_eval_v2` (offline eval enhancement)

State includes acceleration observability for speed-change estimation.

### 5.2 Velocity and acceleration confidence

Confidence is derived from covariance and innovation consistency.

For velocity:

`q_v = exp(-lambda_v * tr(P_vv))`

For acceleration:

`q_a = exp(-lambda_a * tr(P_aa))`

Innovation consistency via normalised innovation squared:

`NIS_k = y_k^T * S_k^{-1} * y_k`

Confidence is down-weighted when rolling NIS exceeds expected bounds.

### 5.3 Low-speed heading stability policy

At low speed, heading is frozen by default to prevent spin/jitter.

If `||v|| < v_freeze`, heading update is allowed only when both hold:

1. geometry anisotropy is strong enough
2. displacement evidence exceeds minimum support threshold

If `||v|| >= v_freeze`, heading follows velocity-consistent disambiguation with
covariance-gated updates.

---

## 6. Config Contract (`config.tuning.json`)

> **Canonical reference:** The full config restructure plan, including the
> complete key-to-layer mapping, nested JSON schema, migration strategy, and
> engine-specific options, is maintained in
> [`config/CONFIG-RESTRUCTURE.md`](../../../config/CONFIG-RESTRUCTURE.md).
>
> This section retains the mathematical config contract for engine selection
> and optimisation strategy only.

Existing flat tuning keys are migrated into layer-scoped sub-objects (`l3`,
`l4`, `l5`, `pipeline`). The `engine` field on each layer selects the
algorithm variant; all engine parameters (common + engine-specific) live
inside a block keyed by the engine name. The block is required when that
engine is selected and absent otherwise. New `optimisation` block controls
sweep strategy:

```json
{
  "l3": {
    "engine": "ema_track_assist_v2",
    "ema_track_assist_v2": {
      "background_update_fraction": 0.02,
      "promotion_near_gate_low": 0.7,
      "...": "(29 fields total: 26 common + 3 track-assist)"
    }
  },
  "l4": {
    "engine": "two_stage_mahalanobis_v2",
    "two_stage_mahalanobis_v2": {
      "foreground_dbscan_eps": 0.8,
      "velocity_coherence_gate": 4.0,
      "...": "(11 fields total: 9 common + 2 VC)"
    }
  },
  "l5": {
    "engine": "imm_cv_ca_v2",
    "imm_cv_ca_v2": {
      "gating_distance_squared": 36.0,
      "transition_cv_to_ca": 0.05,
      "...": "(27 fields total: 23 common + 4 IMM)"
    }
  },
  "optimisation": {
    "strategy": "accuracy_first_v1",
    "search_engine": "hybrid_grid_stochastic_v1",
    "layer_scope": "full"
  }
}
```

All engine parameters live inside the engine block — the block is a
self-describing snapshot where every field is enforced when present. Only the
selected engine's block may be present (see CONFIG-RESTRUCTURE.md §3.1
principles 5–6). The full field set per engine is defined in
CONFIG-RESTRUCTURE.md §5.

Allowed engine values:

1. `l3.engine`: `ema_baseline_v1` (current default), `ema_track_assist_v2`
2. `l4.engine`: `dbscan_xy_v1` (current default), `two_stage_mahalanobis_v2`, `hdbscan_adaptive_v1`
3. `l5.engine`: `cv_kf_v1` (current default), `imm_cv_ca_v2`, `imm_cv_ca_rts_eval_v2`

Allowed optimisation values:

1. `optimisation.strategy`: `accuracy_first_v1`, `balanced_v1`, `realtime_v1`
2. `optimisation.search_engine`: `grid_narrowing_v1`, `hybrid_grid_stochastic_v1`, `local_perturb_v1`
3. `optimisation.layer_scope`: `full`, `l3_only`, `l4_only`, `l5_only`

---

## 7. Harness Comparison Protocol (Layer-Isolated + Full Stack)

Evaluation protocol compares identical replay windows across scenarios:

1. baseline
2. L3-only change
3. L4-only change
4. L5-only change
5. full-stack change

For each scenario compute per-layer and global metrics.

### 7.1 Core metrics

1. velocity RMSE
2. acceleration RMSE
3. low-speed heading jitter
4. fragmentation / ID-switch rates
5. foreground capture and unbounded-point ratios

### 7.2 Statistical confidence

Use paired bootstrap on per-window metric deltas:

`Delta = metric(candidate) - metric(baseline)`

Store confidence interval `[CI_low, CI_high]` for each gate metric.

### 7.3 Regression gates

Require all:

1. velocity RMSE improvement with CI support
2. acceleration RMSE improvement with CI support
3. no low-speed heading jitter regression
4. no fragmentation/ID-switch regression

Persist layer-wise explanation into sweep metadata (`score_components` and
recommendation explanation payload).

---

## 8. Performance Envelope and Strategy Profiles

Expected impact:

1. L3 track-assisted promotion: low/moderate overhead
2. L4 two-stage refinement: primary compute increase
3. L5 IMM CV+CA: moderate compute increase
4. RTS smoothing: evaluation-only path

Profile intent:

1. `accuracy_first_v1`: best math fidelity, higher compute
2. `balanced_v1`: moderate compute/quality tradeoff
3. `realtime_v1`: strict budget, conservative engines

---

## 9. Failure Modes and Guards

1. **False promotion in L3**
   - Guard: promotion only near background gate with covariance consistency.
2. **Over-splitting or over-merging in L4**
   - Guard: Mahalanobis gates + geometric plausibility constraints.
3. **Low-speed heading flip in L5**
   - Guard: freeze-first policy with anisotropy + displacement unlock requirements.
4. **Optimiser overfitting to proxy metrics**
   - Guard: per-layer + global gates with CI-backed acceptance rules.

---

## 10. Acceptance Requirements

Proposal acceptance requires replay-backed evidence that the configured default
(`accuracy_first_v1` + default layer engines) improves velocity and
acceleration accuracy while keeping low-speed heading stable and preserving
fragmentation quality within defined non-regression bounds.
