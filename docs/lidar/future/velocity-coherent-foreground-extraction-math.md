# Velocity-Coherent Foreground Extraction Math

**Status:** Design Math for Planned Implementation
**Date:** February 20, 2026
**Version:** 1.0

This document defines the mathematical model for velocity-coherent foreground extraction, with explicit algorithm tradeoffs and expected benefits.

Implementation tasks and delivery sequencing live in:
- [`docs/lidar/future/velocity-coherent-foreground-extraction.md`](./velocity-coherent-foreground-extraction.md)

---

## 1. Problem Setup

At frame `t`, let the world-frame point cloud be `P_t = {p_i^t}` with each point `p_i^t = (x, y, z)`.

Goal: recover moving-object structure when objects become sparse (as low as 3 points), while reducing fragmentation near entry/exit and short occlusions.

---

## 2. Point Correspondence and Velocity Estimation

For each point `p_i^t`, choose a correspondence `c(i)` in `P_{t-1}` by minimizing a constrained cost:

```text
c(i) = argmin_j  [ w_pos * ||p_i^t - p_j^{t-1}||_2 + w_ctx * C_ctx(i, j) ]
```

subject to:

```text
||p_i^t - p_j^{t-1}||_2 <= r_search
||v_ij|| <= v_max
```

where:

```text
v_ij = (p_i^t - p_j^{t-1}) / Δt
```

Confidence for estimated velocity `v_i`:

```text
q_i = exp(-d_pos / r_search)
      * exp(-σ^2_neighbor / σ^2_ref)
      * I(||v_i|| <= v_max)
```

`q_i in [0, 1]` and is used downstream as a reliability gate.

---

## 3. Velocity-Coherent Clustering Metric

Each enriched point is `u_i = (x, y, z, vx, vy, vz)`.

A weighted position-velocity distance:

```text
D(u_i, u_j) = sqrt( α||Δx||_2^2 + β||Δv||_2^2 )
```

with:
- `Δx = (x_i-x_j, y_i-y_j, z_i-z_j)`
- `Δv = (vx_i-vx_j, vy_i-vy_j, vz_i-vz_j)`
- `α` position weight, `β` velocity weight.

Clustering uses DBSCAN-style neighborhoods over `D`, with reduced minimum points:

```text
MinPts = 3
```

Sparse clusters must also satisfy velocity-coherence gates (`mean q_i`, velocity variance bounds).

---

## 4. Track Continuity Model

### 4.1 Post-Tail Prediction

For track state `(x_t, y_t, vx_t, vy_t)` and elapsed `τ`:

```text
x̂(t+τ) = x_t + vx_t * τ
ŷ(t+τ) = y_t + vy_t * τ
```

with uncertainty growth:

```text
R(τ) = R0 + kτ
```

Associations in post-tail require:

```text
||p - p̂||_2 <= R(τ)
S_vel >= S_vel,min
```

### 4.2 Sparse Continuation

For point count `n`:

- `n >= 12`: relaxed velocity tolerance
- `6 <= n <= 11`: medium tolerance
- `3 <= n <= 5`: strict tolerance
- `n < 3`: prediction only (no direct continuation)

This is an adaptive regularization policy where tolerance shrinks as observation support decreases.

### 4.3 Fragment Merge Score

For earlier fragment `A` and later fragment `B`:

```text
S_merge = (S_pos + S_vel + S_traj) / 3
```

candidate accepted if:

```text
Δt <= Δt_max
E_pos <= E_pos,max
E_vel <= E_vel,max
S_merge >= S_min
```

---

## 5. Algorithm Tradeoffs

| Parameter / Choice | Benefit | Cost / Risk | Practical Guidance |
| --- | --- | --- | --- |
| Higher `β` (velocity weight) | Better separation of nearby objects with different motion | Can split one object if velocity estimates are noisy | Raise `β` only when velocity confidence is stable |
| Lower `MinPts` (to 3) | Recovers distant/sparse objects | More noise clusters | Require high confidence and low velocity variance for sparse clusters |
| Larger `r_search` | Better correspondence recall under fast motion | More ambiguous matches | Use plausibility caps and neighborhood consistency checks |
| Longer post-tail window | Better continuity across temporary dropouts | Ghost-track risk | Cap uncertainty radius and max prediction frames |
| Aggressive merge threshold | Fewer fragmented tracks | Wrong merges create identity errors | Prefer conservative threshold, log merge evidence |

---

## 6. Expected Benefits (Hypotheses to Validate)

Relative to the existing background-subtraction + DBSCAN path, expected gains are:

- Sparse-object recall (`3-11` points): `+20%` to `+40%`
- Fragmentation rate: `10%` to `25%` reduction
- Boundary continuity (entry/exit duration): `+10%` to `+30%`
- Occlusion recovery rate: measurable lift where gaps are short (`<5s`)

Expected constraints:

- False positives can rise if sparse gates are loose
- Runtime can increase due to correspondence and velocity-aware clustering

Net expectation: continuity and sparse recall improve materially, while precision and runtime remain acceptable if confidence gating and uncertainty caps are enforced.

---

## 7. Complexity Notes

For `N` points per frame:

- Correspondence with indexed neighborhood search: approximately `O(N log N)` expected
- DBSCAN-like clustering with local neighborhoods: data-dependent, typically near `O(N log N)` with spatial indexing
- Merge candidate search over `M` fragments: worst-case `O(M^2)`; bounded in practice by time-window pruning

---

## 8. Failure Modes and Guards

- Velocity aliasing in dense scenes:
  - Guard: require neighborhood consistency, not nearest-neighbor only
- Sparse false positives:
  - Guard: confidence and variance thresholds for `n <= 5`
- Prediction drift:
  - Guard: linear uncertainty growth with hard radius cap
- Bad merges:
  - Guard: multi-signal score and conservative acceptance threshold

---

## 9. Validation Requirements

Mathematical assumptions should be accepted only if replay evaluation confirms:

- Improvement in sparse recall and continuity metrics
- Controlled precision impact
- Acceptable compute overhead at target frame rate
