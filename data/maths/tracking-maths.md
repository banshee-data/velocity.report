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
4. lifecycle state transitions using hit/miss counters,
5. offline retrospective refinement: a fixed-assignment Rauch–Tung–Striebel smoother over the
   filter's own record (Section 11).

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

`R = r·I` with the tuned scalar `r = MeasurementNoise` is the shipped model. The default-off
adaptive model of Section 12 replaces `R` alone, in the gate, the likelihood cost and the update
alike.

### 2.3 Initialisation and covariance hygiene (default-off options)

Three operations set or change the state without the covariance the filter's own maths implies.
Each has a default-off `TrackerConfig` option that replaces it; with the option off the shipped
arithmetic runs unchanged. They are Go-level options and not tuning keys, because
`TuningConfig.Fingerprint` hashes the whole tuning config and a new key would break the committed
perf baselines. A replay switches each on by name. Source: the face-state maths review
(`data/maths/proposals/20261108-face-state-and-cv-ca-maths-review.md`, §4.2 and §4.6).

| Option                    | Replay experiment           | Replaces                                        |
| ------------------------- | --------------------------- | ----------------------------------------------- |
| `TwoPointVelocityInit`    | `two_point_velocity_init`   | the second update of a zero-velocity prior      |
| `ConsistentCovarianceCap` | `consistent_covariance_cap` | the diagonal clamp at `MaxCovarianceDiag`       |
| `ConsistentSpeedClamp`    | `consistent_speed_clamp`    | the velocity rescale at `MaxReasonableSpeedMps` |

#### Two-point velocity initialisation

`initTrack` starts a track at the measurement with `v = 0`, `P_vv = 1 m²/s²` per axis and
`P_xx = 10 m²`. A vehicle at 6.5 m/s is then 6.5 standard deviations from its velocity prior.
Under `TwoPointVelocityInit` the track's second accepted association replaces the update. With
`z₁` the founding measurement, `z₂` this one, and `Δt` the interval between their acquisition
times:

- `x = z₂`
- `v = (z₂ − z₁) / Δt`
- `P_xx = R`, `P_xv = R/Δt`, `P_vv = (2R + G)/Δt²`, with `G = σ_g² u uᵀ`

`u` is the unit displacement `(z₂ − z₁)/|z₂ − z₁|`. `σ_g` is the reference point's slack: half
the change between the two clusters' extents along `u`, each read from its cluster's OBB as
`L|cos(θ − φ)| + W|sin(θ − φ)|`. A body seen first as a fragment and then whole moves its cluster
mean by up to that much without moving. Without an OBB the extent is the larger dimension, and
with no displacement `G = σ_g² I`. With `G = 0` this is the exact mean and covariance of the pair
when both measurements carry noise `R` and nothing is known of the velocity beforehand. At the
shipped `R = 0.05 m²` and `Δt = 0.1 s` the velocity variance is then 10 m²/s². `R` is the
isotropic `MeasurementNoise`. Under the adaptive model it is this measurement's `R`, used for both
measurements because the founding one's `R` is not retained, and `R`'s own x–y term carries into
each block. `G` is the only other cross-axis term, and it reaches only the velocity block.

It applies only while the track has exactly one measurement and its velocity is still zero. It
declines, and the shipped update runs, when:

- either acquisition time is missing;
- `Δt` is not positive and finite;
- the implied velocity is not finite;
- the implied speed exceeds `MaxReasonableSpeedMps`;
- `z₂` lies in another confirmed track's shadow as the sensor sees it: on a bearing between its
  box's corner bearings and farther than its nearest corner. The sensor is at `NoiseSensorX`,
  `NoiseSensorY`, zero while the pipeline clusters without a pose. A tentative track, or a box
  without positive, finite extent, casts no shadow;
- a diagonal of `P_vv` exceeds `MaxCovarianceDiag`. At the shipped `R` and cap with no slack that
  is a `Δt` under about 32 ms, too short a baseline to carry a velocity the covariance can
  represent. At `Δt = 0.1 s` it is a slack above about 0.95 m, an extent change of about 1.9 m.

The shadow and slack conditions were added after
[l5est0006](../../docs/lidar/operations/two-point-init-guards-2026-11.md) traced the option's
published speed defect. Without them, every track over 15 m/s on kirk0 and ashbury-downey's first
segment was a thin cluster on a nearer car's bearing. Its range ratio to the car matched its speed
ratio, 1.6 to 3.0, and the inferred cause is a reflection. The shipped tracker cannot hold a mover
born above about 14 m/s, so it never confirmed one. The pair initialised it exactly, and it
confirmed and coasted at that speed. With the guards no row exceeds 15 m/s on either case. kirk0's
identity is then within one fragmentation of the defaults, and the shadow test declines about half
the pairs. The guards are a fallback, not a reflection filter. A pair they decline is treated as
the defaults treat it.

It does not apply to a track whose state the near-edge state machine owns (`NearEdgeTracking`).
That reference point can move between the two frames, and a difference across a re-reference
would read the translation as motion. Such a track runs the shipped update.

The frame is recorded with disposition `two_point_init`. The record keeps the innovation against
the zero-velocity prediction, and the NIS the gate computed, so the frame still has its estimate
row. That innovation is not a filter residual: no gain was applied. The consistency accumulators
do not see it, and the smoother does not list the frame as update evidence.

On a synthetic noise-free scene at the shipped tuning, the shipped tracker does not hold a
vehicle born at 14 m/s or faster. The second update leaves the velocity near zero. The posterior
position variance is then near `R`, so by the third or fourth frame the vehicle is outside the
gate and is born again. At 20 m/s it was born 10 times in 3 s and never confirmed. With the
option on it was born once and confirmed (`TestTwoPointVelocityInitHoldsAFastVehicleFromBirth`).
This is a property of the synthetic scene. It is not a measurement on a capture. The same limit
is what kept reflections from confirming under the shipped tracker
(`TestTwoPointVelocityInitDoesNotHoldAReflection`).

#### Honest velocity prior at birth

`VelocityPriorInit` (replay experiment `velocity_prior_init_7p5`, default off) keeps `v = 0` at
birth but sets `P_vv = (7.5 m/s)² = 56.25 m²/s²` per axis, the review's `(v_max/2)²`. On six
first segments it took the first second's one-step along z RMS from 2.24 to 0.80, and it also
widened the newborn's gate, so fragments and sparse clusters confirmed and kept the speed of one
pair. Duplicates rose on every segment, kirk0 gained 27 switches, and it failed
[l5est0009](../../docs/lidar/operations/velocity-prior-init-2026-11.md)'s V3 to V5.
`VelocityPriorGated` (replay experiment `velocity_prior_init_7p5_gated`, default off) keeps that
variance in the update but gates and costs a tentative track with the covariance it would hold
under the shipped prior. It kept the first-second gain (0.79) and removed the brief tracks, the
clutter and kirk0's extra switches, but duplicates still rose on all six segments and fast rows
remained, so [l5est0010](../../docs/lidar/operations/velocity-prior-gated-2026-11.md) put the
remaining cost in the update.

#### Covariance cap that keeps `P` positive semi-definite

The shipped cap sets `P_ii ← min(P_ii, cap)` and leaves the off-diagonals. It runs in the
prediction, in the coast inflation and in the solid body's prediction and coast. Lowering `P_ii`
while `P_ij` keeps its size can push `|P_ij|` past `√(P_ii P_jj)`, and `P` is then indefinite.
Under `ConsistentCovarianceCap` every one of these sites scales instead:

- `D = diag(min(1, √(cap / P_ii)))`
- `P ← D P D`

`D P D` is a congruence. Every correlation `P_ij / √(P_ii P_jj)` is kept, so a positive
semi-definite `P` stays so, and each capped diagonal is stored at exactly `cap`. With the coupled
process noise and a 2 m² cap, an 80-frame coast drove a position-velocity correlation to 4.4
under the shipped cap and kept it at 0.51 under this one (`TestCovarianceCapOnALongCoast`). At
the shipped 100 m² cap and 25 confirmed misses the cap is not reached, so the defect is latent.

#### Speed clamp consistent with the covariance

The shipped clamp scales `v` to `MaxReasonableSpeedMps` along its own direction and leaves `P`.
It runs in the tracked filter's `clampVelocity` and in the solid body's state step.

The limit is an inequality: the speed along the current velocity direction `u` is at most
`v_max`. Under `ConsistentSpeedClamp` the clamp is a Gaussian truncation of that speed, carried
through the state's regression on it. This is the same form the containment truncation
(`truncateAxis`) uses for a position bound. With `h = [0, 0, u_x, u_y]`, the speed's marginal is
`N(μ, σ²)`, where `μ = h·x` and `σ² = h P hᵀ`. Truncated to `(−∞, v_max]`, with
`β = (v_max − μ)/σ` and the inverse Mills ratio `λ = φ(β)/Φ(β)`, its moments are:

- `m = μ − σ λ`
- `s² = σ² (1 − β λ − λ²)`

They replace the marginal's through every component's regression on it:

- `k = P hᵀ / σ²`
- `x ← x + k (m − μ)`
- `P ← P + (s² − σ²) k kᵀ`

The lower bound is infinite, and it is taken explicitly. The two-sided moments would form
`α φ(α) = ∞ · 0` at `α = −∞`. `Φ` comes from `erfc`. Beyond 30 standard deviations, where `Φ`
nears underflow, `λ` comes from its continued fraction, `z + 1/(z + 2/(z + …))` at `z = −β`.

The along-track speed moves to `m`, just below the limit, and keeps the variance `s²`. That is
less than `σ²` but not zero, so the next update can still move the speed. Position moves too, by
its covariance with the along-track speed. `P` is symmetrised first. `s²` is floored at
(1 cm/s)², never above `σ²`, and any diagonal below zero is floored at zero.

A projection onto `h·x = v_max` would treat the limit as an equality. It would zero the
along-track variance and, with it, the next update's along-track velocity gain. In the unit test
(`TestConsistentSpeedClampBoundsTheNextVelocityGain`), the shipped clamp left that gain at
1.15 s⁻¹, a projection left it at zero to rounding, and the truncation leaves it at 0.063 s⁻¹. In
that case a speed of 33.24 ± 0.72 m/s is truncated at 30 m/s to 29.85 ± 0.14 m/s.

The cross-track velocity also moves by its covariance with the along-track one. That can leave
the speed a little above `v_max`, and the shipped rescale then removes the excess, so the speed
bound holds as it always has. If the velocity variance along `u` is below 1e-9 m²/s² or not
finite, the clamp falls back to the shipped rescale.

The truncation does not stop a body that really travels above the limit from outrunning its
track. In the synthetic scene of `TestHygieneOptionsOffKeepTheShippedArithmetic`, a vehicle
accelerates at 5 m/s² to 37 m/s. The clamp first binds at a true speed of 34 m/s. Each truncation
puts the speed just below 30 m/s with a small variance, so the track falls behind faster than
under the shipped clamp. That clamp holds the speed at 30 m/s with its untouched variance and
lets the position update catch up. At a true speed of 36 m/s, after four clamped frames, the
truncated track misses one frame and a stray track is born. The original track reacquires the
vehicle on the next frame. Under the shipped clamp the same track holds throughout, its lag
growing from 0.62 m to 0.98 m. A body above `MaxReasonableSpeedMps` is outside the model by
definition.

On kirk0's six-second moving window, neither covariance option changed the recording. The
two-point option did (`TestHygieneExperimentsOnKirk0`, `pcap` build tag). That run checks the
wiring only. It is not evidence for or against any of the three options.

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

## 11. Retrospective refinement

State-estimation plan §10 and Phase 5. Implementation:
[smoother.go](../../internal/lidar/l5tracks/smoother.go) over the record kept by
[filter_steps.go](../../internal/lidar/l5tracks/filter_steps.go). Offline only: the live tracker
attaches no observer.

### 11.1 What the filter records

For each track and each Update call `k` the tracker records, exactly as it computed them:

- the prior `x_{k|k-1}, P_{k|k-1}` entering the Kalman update. For a step with no accepted
  observation the prior and posterior are the same stored numbers, after the miss handling's
  covariance inflation, which therefore counts as part of that step's transition;
- the posterior `x_{k|k}, P_{k|k}`;
- `tau_k`, the interval `predict()` actually applied, summed over sub-steps after the
  `MaxPredictDt` clamp. It equals `t_k - t_{k-1}` except where that clamp shortened a gap; the
  smoother uses `tau_k` because the prior was built with it, and flags the clamped transition.

The stored prior is used as is rather than recomputed as `F P F^T + Q`, so the covariance cap, the
occlusion inflation and the velocity clamp enter exactly as the filter applied them. Where the
velocity clamp makes `x_{k+1|k} != F x_{k|k}` the recursion below remains well defined but is no
longer an exact Gaussian smoother for that step.

### 11.2 Backward recursion

With `F = F(tau_{k+1})`:

- `C_k = P_{k|k} F^T P_{k+1|k}^{-1}`
- `x_{k|j} = x_{k|k} + C_k (x_{k+1|j} - x_{k+1|k})`
- `P_{k|j} = P_{k|k} + C_k (P_{k+1|j} - P_{k+1|k}) C_k^T`

starting from `x_{j|j}, P_{j|j}`. Starting at a later posterior gives `E[x_k | z_1 .. z_j]`
exactly, and the states before `k` are not needed, which is what bounds the window. The end index
sets the look-ahead:

| Lag              | `j` for state `k`                                                                                                                                                     | Stage       |
| ---------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------- |
| `L` frames       | `k + L`                                                                                                                                                               | `fixed_lag` |
| Capture time `Λ` | the last step with `t_j <= t_k + Λ`, once a step at or beyond `t_k + Λ` arrives; both within 5 ms, so a sensor period a fraction short of nominal still meets the lag | `fixed_lag` |
| Whole track      | the chain's last step                                                                                                                                                 | `final`     |

A state whose track or input ends first is released with the look-ahead it has, flagged. Its
estimate then equals the whole-track one, bit for bit, because it is the same computation.

Association is never revisited: this is the fixed-assignment control experiment of the
asynchronous tracking plan. A wrong assignment is smoothed into the path, not repaired.

### 11.3 No evidence, no revision

If every step after `k` up to `j` is unobserved, each of those steps has `x_{i|i} = x_{i|i-1}` as
stored numbers, the bracket in 11.2 is exactly zero, and `x_{k|j} = x_{k|k}` to the bit. A revision
therefore needs an observation in `(k, j]`, and the revision record lists those observations. A
non-zero revision without one is counted as a defect, not tolerated.

### 11.4 Numerics

- Everything is computed in float64 from the float32 record; covariances are symmetrised,
  `½(P + P^T)`, on entry and after every backward step.
- Positive definiteness is tested by Cholesky with a pivot floor of `1e-12` times the largest
  diagonal entry. The gain solves `P_{k+1|k} X = F P_{k|k}` and takes `C_k = X^T`.
- A prior that fails the test (the diagonal-only covariance cap can produce one), a state time that
  runs backwards, or a step with a non-finite value is a barrier: the chain is split and the states
  before it keep only the evidence before it.
- A smoothed covariance that fails the test is replaced by `P_{k|k}`, which is never smaller in
  exact arithmetic, and flagged.

The Joseph form belongs to the forward update (Section 2.2); the backward covariance recursion is
kept symmetric and checked instead.

### 11.5 Consistency of a residual

For an observed step, `r = z_k - H x_{k|j}` has covariance `R - H P_{k|j} H^T` under the model, and
`r^T (R - H P_{k|j} H^T)^{-1} r` has expectation `m = 2`. The same holds for the posterior residual
with `P_{k|k}`. The replay comparison reports its mean per arm as a label-free consistency check;
steps where the covariance is not positive definite are counted, not scored.

### 11.6 Bandwidth, and why manoeuvres flatten

Treat the filter in continuous time: velocity driven by white acceleration of density `q`
(`ProcessNoiseVel`), position by a white-velocity term of density `q_p` (`ProcessNoisePos`), and
measurements with noise density `r = R dt`. The spectrum of position is `q_p / w^2 + q / w^4`, so
the whole-track smoother's gain on true motion at angular frequency `w` is

`H(w) = (q + q_p w^2) / (q + q_p w^2 + r w^4)`.

Shipped values are `q = 0.2 m^2/s^3`, `q_p = 0.05 m^2/s` and `R = 0.05 m^2` at 10 Hz, so
`r = 0.005 m^2 s`. Gain falls to one half at `w^2 = (q_p + sqrt(q_p^2 + 4 r q)) / (2 r)`,
`w = 3.6 rad/s` (0.58 Hz). A 3 s lane change has its lateral acceleration at `w = 2.1 rad/s`, where
`H = 0.81`, and harmonics near `4.2 rad/s`, where `H = 0.41`: its peak rates are attenuated.
With `q = 3` the half-gain point moves to `5.5 rad/s` and those gains become 0.97 and 0.71. The
synthetic tests measure the consequence (whole-track RTS keeps 48 % of the true peak course rate at
the shipped `q`, and at least 85 % at `q = 3`). A longer look-ahead approaches `H(w)`; a short one
stays closer to the causal filter. Horizon choice and process-noise calibration are therefore one
decision, not two.

### 11.7 Memory

Each window holds at most its cap at any instant: `L + 1` steps for a frame lag, `1 + ceil(20 Λ)`
for a capture-time lag (every step of a 20 Hz capture), and 3,000 for the whole track, beyond which
the oldest state is released early as `fixed_lag`. A held step is about 0.45 kB (the record and one
4×4 gain). On the full kirk0 replay the peak across all tracks was 60, 93, 168, 294 and 695 steps
for three frames, 0.5 s, 1 s, 2 s and the whole track.

## 12. Adaptive measurement uncertainty

State-estimation plan Phase 3 and Section 8.1, behind `TrackerConfig.AdaptiveMeasurementNoise`
(experiment `adaptive_uncertainty`). Default off: with it off every output is unchanged.

### 12.1 The model

For a measurement at `m` seen from the sensor at `s`, let `r = |m - s|`, `u = (m - s)/r` the line
of sight and `v = (-u_y, u_x)` across it. `R` is diagonal in `(u, v)` and rotated into the site
frame:

`R = σ_rad² u uᵀ + σ_tan² v vᵀ`

`σ_axis² = c_axis(stratum) + φ_axis(r, N, a)`

The physics terms `φ` are Section 8.1's, with the Pandar40P's range accuracy `σ_range = 0.02 m`,
azimuth step `δ = 0.2°`, cluster point count `N` and folded aspect `a ∈ [0, π/2]` (0 end-on, `π/2`
broadside) of the sensor under the track's believed heading:

`φ_rad = σ_range² + (r δ tan θ)²`, with `θ = min(a, π/2 - a)`

`φ_tan = (r δ)²/12 + (r δ)²/N`

They are small: at 30 m the whole tangential term is under 0.02 m². The edge-localisation term
`σ_edge²` needs a truncated-face flag the online measurement lacks, so it is absent rather than
guessed. The coefficient `c` carries what `φ` does not explain (clustering jitter, visible-surface
hops) and is looked up per stratum: measurement source × range `{0, 10, 20, 30, 50}` m × support
`{0, 10, 30, 100}` points × folded aspect `{end-on, oblique, broadside}` × axis. With no fitted
table every `c` is the scalar `MeasurementNoise`. Below 0.5 m no line of sight exists and `R`
falls back to `r·I`. The Joseph option uses the full `KRKᵀ` with this `R`.

### 12.2 Pre-gate NIS

Accepted-only NIS is censored by the gate. Inside a calibration window the tracker also evaluates,
after prediction and before association, every physically plausible pairing, and records the
eligible ones: a confirmed track whose only plausible cluster is plausible for no other track.
Eligibility is decided by Euclidean plausibility, never by `S`. For each it records the
innovation's projections `y_u = uᵀy`, `y_v = vᵀy`, the projected prediction `uᵀHP⁻Hᵀu`, the `R`
used, `φ` and the joint `d² = yᵀS⁻¹y`, whether the gate forbade it and whether it was assigned. For
a consistent model `d²/m` has mean 1 and each marginal `y_u²/(uᵀSu)` is chi-squared with one
degree of freedom.

### 12.3 Calibration

Per stratum and axis the fit finds `c` for which the mean over samples of

`(y_u - ȳ_u)² / (uᵀHP⁻Hᵀu + φ_u + c)`

is 1, by bisection on `[0.0025, 4]` m² (the statistic is non-increasing in `c`). Each sample is
normalised by its own predicted covariance: the moment estimate `Var(y_u) - mean(uᵀP⁻u) - mean(φ_u)`
assumes one `S` per stratum and goes wrong when `P⁻` varies, as it does on real replays. Centring
on `ȳ` means a biased stratum is flagged, not given a wider `R`. Samples from coasting tracks,
whose `P⁻` carries occlusion inflation, are excluded from the fit. A smaller `R` shrinks the
filter's own `P`, so the fit is one step of a fixed-point iteration, repeated until each fitted
coefficient is within 10 % of the one its replay used. Like any innovation-based estimate it
attributes to `R` whatever `P⁻` does not explain, process-model error included, which is why the
decision is taken on a held-out partition against the
[predeclared G-UNC-1 criteria](../../docs/lidar/operations/adaptive-uncertainty-criteria.md).

### 12.4 Gate-rejection evidence

When the gate forbids an eligible pairing, the tracker carries two hypotheses forward: the track's
own prediction, and the rejected measurement `z` moved at the track's velocity, `z + v·Δt`. The
first later cluster explained at least twice as well by one of them decides: the rejected path
continuing is a genuine manoeuvre (persistent), the track's own path continuing is a spurious
measurement (transient); nothing within three association frames is unresolved.

## 13. Joint extent covariance and the projected surface

Behind `SolidBodyOptions.ExtentState` (experiment `solid_body_extent_state`), from
[the face-state maths review §3.1](proposals/20261108-face-state-and-cv-ca-maths-review.md#31-the-half-extent-is-a-state-not-a-constant).
Default off: with it off every output is unchanged, which a digest test pins.

**State.** `x = [x, y, vx, vy, hL, hW]`. The transition is the constant-velocity `F` on the first
four states and the identity on the half-extents, with process noise `(0.02 m)²/s` on each. With
the cross block `P_xh = Cov([x, y, vx, vy], [hL, hW])`, prediction gives
`P_xh ← F₄ P_xh` (the position rows gain `Δt` times the velocity rows) and
`P_hh ← P_hh + q_h Δt`. A face with outward normal `n` measures `z = n·c + h` with
`H = [n_x, n_y, 0, 0, 1, 0]`, or the `hW` column for a side face.

**Tracked filter.** Under `NearEdgeTracking` the same block is the tracked filter's. The 4×4
stays `TrackedObject.P`; every step that changes it changes the extent rows too. The covariance cap
scales the joint matrix's capped rows and columns, `P ← D P D`. The consistent speed clamp regresses
all six states on the along-track speed. The medoid update, with tracked gain `K` and `A = H P_xh`,
applies `h ← h + Aᵀ S⁻¹ y`, `P_hh ← P_hh − Aᵀ S⁻¹ A` and `P_xh ← P_xh − K A`. Association is
unchanged: the A2 gate is the 2×2 position innovation against the 4×4, with the state's `h` as if
exact. Along a face normal its variance therefore differs from the joint innovation's by
`Var(h) + 2 nᵀP_ch`.

**Projected surface.** With body axis `u`, `d = +u` for the front and `−u` for the rear, the
surface is `c + hL d`. Along `d` its variance is

`Var(d·c + hL) = uᵀ P_cc u + Var(hL) + 2 dᵀ P_ch`

where `P_ch` is the position's covariance with `hL`. The orientation term `hL² σ_ψ²` is added as
before. An end face measures `d·c + hL` directly, so after it the two parts are negatively
correlated along `d`. The joint bound on that surface is then tighter than the independent sum
`uᵀ P_cc u + Var(hL)`. When the parts are positively correlated along `d`, the joint bound is wider.
`SurfaceProjection.JointBound` says which form was used. `ProjectSurfaceJoint` refuses the
independent form, which is the suppression Section 5.4 of the state plan asks for.

**Published and persisted.** The reading carries `P_xh` and `P_hh` beside its 4×4. A store writes
the 6×6 under `state_model = cv_cartesian_extent_v1` and reads it back by that name, never by its
length. A smoothed reading drops the extent blocks, because the smoother revises only
`[x, y, vx, vy]` and the online `P_xh` describes the online position.

## 14. References

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
| Rauch et al. (1965)             | `Rauch1965`       | RTS smoother: fixed-assignment fixed-lag and whole-track refinement (Section 11)              |

Full BibTeX entries: [data/maths/references.bib](references.bib)
