# Tracking Maths

- **Status:** Implementation-aligned math note
- **Layers:** L5 Tracks (`internal/lidar/l5tracks`)
- **Related:** [Clustering Maths](clustering-maths.md)

## 1. Purpose

Tracking estimates persistent object state over time from frame-level cluster measurements.

Core mathematical components:

1. constant-velocity Kalman filtering,
2. Mahalanobis gating with physical plausibility guards,
3. global assignment with Hungarian optimisation,
4. lifecycle state transitions using hit/miss counters.

## 2. State-Space Model

Track state (world frame):

`x = [pos_x, pos_y, vel_x, vel_y]^T`

Measurement:

`z = [cluster_centroid_x, cluster_centroid_y]^T`

### 2.1 Prediction model

Constant-velocity transition:

`F(dt) = [[1,0,dt,0],
          [0,1,0,dt],
          [0,0,1,0],
          [0,0,0,1]]`

Prediction:

- `x^- = F x`
- `P^- = F P F^T + Q`

Implementation applies per-diagonal process noise terms scaled by `dt`, and clamps diagonal covariance growth by `MaxCovarianceDiag`.

### 2.2 Update model

Observation matrix:

`H = [[1,0,0,0],
      [0,1,0,0]]`

Innovation:

`y = z - H x^-`

Innovation covariance:

`S = H P^- H^T + R`

Kalman gain:

`K = P^- H^T S^{-1}`

Posterior:

- `x = x^- + K y`
- `P = (I - K H) P^-`

The implementation rejects updates with near-singular `S` (determinant below threshold).

## 3. Gating and Plausibility

Each cluster-track candidate gets a squared Mahalanobis cost:

`d_M^2 = y^T S^{-1} y`

Candidate is forbidden if any of:

1. Euclidean jump exceeds `MaxPositionJumpMeters`.
2. Implied speed (`jump/dt`) exceeds `MaxReasonableSpeedMps`.
3. `d_M^2 > GatingDistanceSquared`.
4. Numerical singularity detected.

Forbidden costs are represented as a large sentinel (`+inf`) in assignment.

## 4. Global Association (Hungarian)

Build cost matrix `C` with rows = clusters, columns = active tracks.

- `C_ij = d_M^2` if candidate valid,
- `C_ij = +inf` if gated out.

Solve rectangular assignment by padded square Hungarian (Kuhn-Munkres/JV-style potentials).

This avoids greedy collision artifacts where two clusters compete for one track.

## 5. Lifecycle Dynamics

States:

- `tentative`,
- `confirmed`,
- `deleted`.

Rules:

1. New unassociated cluster initializes tentative track.
2. Consecutive hits promote tentative to confirmed (`HitsToConfirm`).
3. Unmatched tracks increment misses and coast on prediction.
4. Miss thresholds differ for tentative vs confirmed (`MaxMisses`, `MaxMissesConfirmed`).
5. Deleted tracks are purged after grace period.

During occlusion (misses), covariance inflation widens future gating windows for re-association.

## 6. Secondary Stability Metrics

Tracker computes additional quality statistics:

1. **Velocity-trail alignment**
   - compare velocity heading vs displacement heading,
   - accumulate mean angular mismatch and misalignment count.
2. **Heading jitter**
   - RMS of frame-to-frame OBB heading deltas.
3. **Speed jitter**
   - RMS of frame-to-frame speed deltas.
4. **Fragmentation**
   - proportion of created tracks that never confirm.

These metrics are not primary filter states; they are diagnostics/tuning signals.

## 7. OBB Heading Handling

OBB heading has 180-degree ambiguity from PCA.

Tracking resolves and stabilises heading by:

1. optional flip toward velocity direction when speed is sufficient,
2. wrap-aware EMA smoothing:
   - compute shortest signed angular delta in `[-pi, pi]`,
   - `theta_smooth <- theta_prev + alpha * delta`.

Heading updates are skipped for low-point or near-square clusters where orientation is poorly conditioned.

## 8. Complexity

For `C` clusters and `T` tracks:

- prediction/update: `O(T)`
- cost matrix build: `O(C*T)`
- Hungarian assignment: `O(max(C,T)^3)`

In typical road scenes, assignment cost is acceptable; gating prunes many impossible pairs.

## 9. Assumptions and Limits

1. **Constant velocity model**
   - Good short horizon, less accurate for sharp turns/accelerations.
2. **2D positional observation**
   - Ignores vertical dynamics in filter state.
3. **Gaussian error assumptions in Mahalanobis gating**
   - Real cluster errors can be heavy-tailed/non-Gaussian.
4. **Parameter sensitivity**
   - Miss budgets and gating thresholds strongly affect identity continuity vs false matches.
5. **Cluster quality dependency**
   - Tracking cannot fully recover from severe upstream merge/split/noise errors.

## 10. Practical Tuning Direction

For long-running static traffic monitoring:

1. keep gating conservative but physically plausible,
2. allow enough confirmed-track coasting to survive brief occlusions,
3. monitor jitter/alignment metrics continuously,
4. co-tune with clustering and L3 foreground thresholds, not in isolation.

## 11. References

| Reference                       | BibTeX key        | Relevance                                                                                      |
| ------------------------------- | ----------------- | ---------------------------------------------------------------------------------------------- |
| Kalman (1960)                   | `Kalman1960`      | Original Kalman filter predict-update cycle (Sections 2–3)                                     |
| Kuhn (1955)                     | `Kuhn1955`        | Hungarian method for global assignment (Section 4)                                             |
| Munkres (1957)                  | `Munkres1957`     | Munkres reformulation of Hungarian assignment; our `hungarian.go` implementation follows this  |
| Mahalanobis (1936)              | `Mahalanobis1936` | Mahalanobis distance used in gating (Section 3)                                                |
| Weng et al. (2020)              | `Weng2020`        | AB3DMOT — Kalman+Hungarian 3D MOT baseline our architecture closely follows                    |
| Bewley et al. (2016)            | `Bewley2016`      | SORT — 2D Kalman+Hungarian lifecycle model; our lifecycle (Section 5) follows SORT conventions |
| Bernardin & Stiefelhagen (2008) | `Bernardin2008`   | CLEAR MOT metrics (MOTA, MOTP) used in L8 run comparisons                                      |
| Blom & Bar-Shalom (1988)        | `Blom1988`        | IMM algorithm — foundation for planned `imm_cv_ca_v2` motion-model extension (Section 10)      |
| Rauch et al. (1965)             | `Rauch1965`       | RTS smoother — evaluation-only path in planned `imm_cv_ca_rts_eval_v2` (Section 10)            |

Full BibTeX entries: [data/maths/references.bib](references.bib)
