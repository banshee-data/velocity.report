# Tracking maths

- **Status:** Implementation-aligned math note
- **Layers:** L5 Tracks ([internal/lidar/l5tracks](../../internal/lidar/l5tracks))
- **Related:** [Clustering Maths](clustering-maths.md)

## 1. Purpose

Tracking estimates persistent object state over time from frame-level cluster measurements.

Core mathematical components:

1. constant-velocity Kalman filtering,
2. Mahalanobis gating with physical plausibility guards,
3. global assignment with Hungarian optimisation,
4. lifecycle state transitions using hit/miss counters.

## 2. State-Space model

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

### 2.1.1 Coast inflation

A track with no associated cluster in a frame is predicted and then widened. The shipped form adds
a fixed amount per missed frame, whatever the frame interval:

`P_xx += c,  P_yy += c`, with `c = OcclusionCovInflation` (0.5 m²)

Over an unobserved interval `T` that is `c · n`, where `n` is the number of frames that happened
to reach the tracker in it: 5 m² per second at 10 Hz, 10 m² at 20 Hz, and 0.5 m² for a 2 s gap
of empty frames that never reached the tracker at all.

Under the default-off `OcclusionContinuity.CaptureTimeInflation` the widening is charged per
second of unobserved capture time instead:

`P_xx += r · Δt_u,  P_yy += r · Δt_u`

where `Δt_u` is the capture time the frame added to the track's coast age (the time since its last
accepted observation) and `r` is the track's class rate, interpolated from `unknown`'s toward its
class's by classification confidence. Over `T` the added variance is `r · T` however many frames
arrived. `r = 0` leaves only the filter's own `Q`: pure CV growth. Every starting rate (1 to
3 m²/s) is below the shipped 5 m²/s at 10 Hz, so at the nominal frame rate the association
discount this term gives a coasting track (gap analysis S3) is never deeper than the shipped one.
Both forms are capped by `MaxCovarianceDiag`. See
[time-domain model](../../docs/lidar/architecture/time-domain-model.md#coast-existence-and-expiry).

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

## 3. Gating and plausibility

Each cluster-track candidate gets a squared Mahalanobis cost:

`d_M^2 = y^T S^{-1} y`

Candidate is forbidden if any of:

1. Euclidean jump exceeds `MaxPositionJumpMeters`.
2. Implied speed (`jump/dt`) exceeds `MaxReasonableSpeedMps`. `dt` is the frame interval, so at
   10 Hz a pairing more than 3 m from the prediction is refused. For a track coasting through
   several frames that is the binding constraint on reacquisition, since its discrepancy accrued
   over the whole unobserved interval. Under the default-off `ReacquisitionGuard` a reacquiring
   track divides by its capture-time coast age instead.
3. `d_M^2 > GatingDistanceSquared`.
4. Numerical singularity detected.

Forbidden costs are represented as a large sentinel (`+inf`) in assignment.

## 4. Global association (hungarian)

Build cost matrix `C` with rows = clusters, columns = active tracks.

- `C_ij = d_M^2` if candidate valid,
- `C_ij = hungarianlnf` (1e18, standing for `+inf`) if gated out.

Solve the rectangular assignment with the Hungarian method (Kuhn-Munkres, JV-style shortest
augmenting paths with dual potentials). The objective is lexicographic: first the most admitted
pairs, then the least total cost among assignments with that many.

The solver never pads and never lets the sentinel into its arithmetic. It solves on the smaller
side (transposing when there are more clusters than tracks), and gives each gated-out cell a finite
penalty larger than any complete assignment of admitted pairs could cost, then drops penalty pairs.
Adjacent float64 values near 1e18 are 128 apart, so the padded solver, which let the sentinel into
its potentials, lost every real cost beside it and could follow row order rather than cost: on
kirk0 it returned a costlier assignment on 6 of 705 association frames. NaN and infinite costs are
treated as gated out.

This avoids greedy collision artifacts where two clusters compete for one track.

## 5. Lifecycle dynamics

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

Default-off, `OcclusionContinuity.ClassCoastBounds` replaces rule 4 with capture-time coast
bounds: `T_tentative` for a tentative track; for a confirmed one its class's unexplained
allowance, or its longer explained allowance while a nearer cluster covers the predicted
footprint. Every instant and deletion records its support and reason; existence (whether the
object is seen, and if not why) is held separately from this lifecycle. See Section 2.1.1 and the
[time-domain model](../../docs/lidar/architecture/time-domain-model.md#coast-existence-and-expiry).

## 6. Secondary stability metrics

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

## 7. OBB heading handling

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
- Hungarian assignment: `O(min(C,T)^2 * max(C,T))`

In typical road scenes, assignment cost is acceptable; gating prunes many impossible pairs.

## 9. Assumptions and limits

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

## 10. Practical tuning direction

For long-running static traffic monitoring:

1. keep gating conservative but physically plausible,
2. allow enough confirmed-track coasting to survive brief occlusions,
3. monitor jitter/alignment metrics continuously,
4. co-tune with clustering and L3 foreground thresholds, not in isolation.

## 11. References

| Reference                       | BibTeX key        | Relevance                                                                                     |
| ------------------------------- | ----------------- | --------------------------------------------------------------------------------------------- |
| Kalman (1960)                   | `Kalman1960`      | Original Kalman filter predict-update cycle (Sections 2–3)                                    |
| Kuhn (1955)                     | `Kuhn1955`        | Hungarian method for global assignment (Section 4)                                            |
| Munkres (1957)                  | `Munkres1957`     | Munkres reformulation of Hungarian assignment; our `hungarian.go` implementation follows this |
| Mahalanobis (1936)              | `Mahalanobis1936` | Mahalanobis distance used in gating (Section 3)                                               |
| Weng et al. (2020)              | `Weng2020`        | AB3DMOT: Kalman+Hungarian 3D MOT baseline our architecture closely follows                    |
| Bewley et al. (2016)            | `Bewley2016`      | SORT: 2D Kalman+Hungarian lifecycle model; our lifecycle (Section 5) follows SORT conventions |
| Bernardin & Stiefelhagen (2008) | `Bernardin2008`   | CLEAR MOT metrics (MOTA, MOTP) used in L8 run comparisons                                     |
| Blom & Bar-Shalom (1988)        | `Blom1988`        | IMM algorithm: foundation for planned `imm_cv_ca_v2` motion-model extension (Section 10)      |
| Rauch et al. (1965)             | `Rauch1965`       | RTS smoother: evaluation-only path in planned `imm_cv_ca_rts_eval_v2` (Section 10)            |

Full BibTeX entries: [data/maths/references.bib](references.bib)
