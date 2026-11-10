# Motion models maths: CV, CA and IMM, offline

- **Status:** Implemented offline math note; not active in the live tracker. G-EST-1 decides
  whether any of it ships.
- **Layers:** L5 Tracks ([internal/lidar/l5tracks/motion](../../internal/lidar/l5tracks/motion)),
  L8 Analytics ([motion_gest1.go](../../internal/lidar/l8analytics/motion_gest1.go))
- **Related:** [state estimation plan §7.3](../../docs/plans/lidar-state-estimation-plan.md#73-recommendation-and-gate),
  [tracking maths](tracking-maths.md)
- **References:** `Blom1988`, `BarShalom2011`, `Scholler2020`, `Singer1970`

## 1. Purpose and scope

This note defines the motion-model estimators that the state estimation plan's Phase 4 prototypes
offline: constant velocity (CV), constant acceleration (CA) with a coast rule, and an interacting
multiple model (IMM) over the two. It also defines the G-EST-1 analyses that decide whether the
tracker should leave CV. The specification is Sections 4.1 and 4.3 to 4.6 of the November 2026
maths review of face state and CV/CA tracking (`data/maths/proposals/`, on the review branch).

None of this runs in the live tracker. The package reads stored observations: capture time,
measured position, a 2×2 measurement covariance, and optionally a rank-one direction. An adapter
builds those sequences from the live tracker's filter-step record (`l5tracks.FilterFrame`). No
tuning key, `TrackerConfig` default or `config/tuning.defaults.json` entry changes. All
arithmetic is float64. Time is capture time in Unix nanoseconds, and the package never reads the
host clock; a test holds that rule.

## 2. One state space

Every model works in the same six-state space per track:

```text
x = [x, y, vx, vy, ax, ay]ᵀ
```

Each transition and process-noise matrix is block-diagonal over the two axes, built from one 3×3
block over `[p, v, a]`. The axes are independent in the model; the measurement covariance can
couple them.

**CV is embedded with acceleration fixed at zero.** Its transition block is

```text
F_cv = | 1  T  0 |
       | 0  1  0 |
       | 0  0  0 |
```

The zero third row maps any acceleration the state carries to zero, and the process noise gives
acceleration no variance. So after a CV prediction the acceleration mean is zero, and every
acceleration row and column of `P` is exactly zero. Velocity is propagated without reference to
the acceleration, so dropping it marginalises it rather than conditioning on it. That is what lets
an IMM mix a CV estimate with a CA one like with like, with no rule needed for missing rows.

## 3. Process noise

All three forms are exact discretisations of a continuous model over a step `T`, with spectral
density `q` (`BarShalom2011`).

**CV, white-noise acceleration** (`q` in m²/s³), per axis over `[p, v]`:

```text
Q_cv = q | T³/3  T²/2 |  + σ_p² T on p
         | T²/2  T    |
```

`σ_p²` is the live filter's independent position term (`process_noise_pos`), which the continuous
model does not have; it is an option, and zero gives the pure continuous form. A third form,
`diag(σ_p² T, q T)`, reproduces the live filter's diagonal `Q`. It exists only so the live
filter's behaviour can be reproduced in this space.

**CA, white-noise jerk** (`q` in m²/s⁵), per axis:

```text
F_ca = | 1  T  T²/2 |     Q_ca = q | T⁵/20  T⁴/8  T³/6 |
       | 0  1  T    |              | T⁴/8   T³/3  T²/2 |
       | 0  0  1    |              | T³/6   T²/2  T    |
```

**Singer acceleration** (`Singer1970`). The acceleration is a first-order Markov process,
`ȧ = −a/τ + w`, with `w` white of density `q`. With `α = 1/τ` and `E = e^{−αT}`:

```text
F_s = | 1  T  (αT − 1 + E)/α² |
      | 0  1  (1 − E)/α       |
      | 0  0  E               |

q11 = (1 − E² + 2αT + 2α³T³/3 − 2α²T² − 4αT·E) / (2α⁵)
q12 = (E² + 1 − 2E + 2αT·E − 2αT + α²T²)       / (2α⁴)
q13 = (1 − E² − 2αT·E)                          / (2α³)
q22 = (4E − 3 − E² + 2αT)                       / (2α³)
q23 = (E² + 1 − 2E)                             / (2α²)
q33 = (1 − E²)                                  / (2α)
Q_s = q · [q_ij]
```

Singer writes the noise density as `2ασ_m²`, with `σ_m²` the manoeuvre variance. Here `q` is the
density itself, so `σ_m² = qτ/2`, and as `τ → ∞` the model tends to white-noise-jerk CA with the
same `q`. The closed form computes `q11` as a difference of O(1) terms that cancels to O((αT)⁵).
Below `αT = 0.05` the code therefore evaluates the defining integral `∫₀ᵀ g(s) g(s)ᵀ ds` by
10-point Gauss–Legendre instead, with `g(s) = [s² φ₂(αs), s φ₁(αs), e^{−αs}]`. Here
`φ₁(x) = (1 − e^{−x})/x` and `φ₂(x) = (x − 1 + e^{−x})/x²`, each computed stably. The two methods
agree to 10⁻⁷ relative across the crossover.

## 4. Measurement update

A position observation has `H` equal to the first two rows of the identity. A rank-one
observation along a unit direction `n` measures `z = n·(x, y)` with variance `nᵀRn`, the form a
face measurement takes. The update uses the Joseph form:

```text
S = H P Hᵀ + R,   K = P Hᵀ S⁻¹,   x⁺ = x + K (z − H x)
P⁺ = (I − K H) P (I − K H)ᵀ + K R Kᵀ
```

This keeps `P` symmetric positive semi-definite for any gain (`BarShalom2011`). The result is
then symmetrised. A test checks it against the standard form at the optimal gain to 10⁻¹⁴.

## 5. The coast rule

A prediction is _unobserved_ when its interval exceeds `CoastAfterSecs` (default 0.15 s, so any
dropped frame at 10 Hz counts). CA without a coast rule carries an estimated deceleration through
a stop and reverses the vehicle. `Scholler2020` attributes constant acceleration's poor
long-horizon pedestrian displacement to the same assumption that acceleration continues. The
rule has three parts. Each is configurable, and all three are on by default for CA.

**(a) Acceleration decay.** During an unobserved prediction, CA's transition is replaced by
Singer's with time constant `τ` (default 1.5 s; the review names 1 to 2 s). The acceleration
nobody is measuring relaxes as `e^{−t/τ}`. Observed predictions keep the white-noise-jerk model,
which has no steady-state lag under constant acceleration.

**(b) No reversal.** At the gap's start the course `u` is recorded: the unit velocity, if the
speed is at least `MinCourseSpeed` (default 0.3 m/s). Below that speed there is no course and
the rule has nothing to hold. Within each sub-step, if the predicted `uᵀv` would cross zero, the
crossing time is found by bisection on the mean. The transition is linear, so the crossing is a
property of the mean alone. The state is predicted to the crossing and conditioned there on the
two equalities `h_v x = uᵀv = 0` and `h_a x = uᵀa = 0`, one after the other, as noise-free
measurements:

```text
k = P hᵀ / (h P hᵀ),   x ← x + k (0 − h x),   P ← P − k h P
```

Each component correlated with the constrained one moves by its regression on it, and `P` loses
exactly the variance along `h`. It is not a clamp of the state alone. When `h P hᵀ` is already
zero the component is known exactly, and only the mean moves, along `h`. The prediction then
continues to the end of the interval. Conditioning moves position as well as velocity. In the
braking test below, zeroing the velocity and acceleration means alone would reacquire at
+0.32 m; conditioning through the regression reacquires at −0.02 m.

**(c) Sub-stepped prediction.** An interval longer than `MaxStepSecs` (default 0.1 s) is
predicted in equal sub-steps no longer than that, and it is never clamped. The live tracker's
`max_predict_dt` clamps at 0.5 s. That under-predicts a CV gap linearly, and for CA it drops the
`T²` term as well. For the linear models, sub-steps compose exactly: `F(a)F(b) = F(a+b)`, and `Q`
composes the same way, which a test checks to 10⁻¹². So sub-steps change no mean or covariance by
themselves. They set the grid on which the crossing is sought and the covariance cap is applied.

## 6. Covariance hygiene

Mixing propagates any inconsistency in one mode's `P` into both. The package bounds and
constrains `P` in only three ways:

- **Symmetrise:** `P ← (P + Pᵀ)/2`.
- **PSD-preserving cap:** `P ← D P D`, with `d_i = min(1, √(σ²_max / P_ii))`. The capped row and
  its column are scaled by the same factor, so every correlation coefficient is unchanged. The
  default `σ²_max` is 100, the live `max_covariance_diag`. A test shows that clamping only the
  diagonal of a correlated pair (correlation 0.975, variance 400 capped at 100) leaves a matrix
  that is not PSD. The scaled cap keeps it PSD.
- **Linear constraint:** the conditioning in Section 5(b).

No diagonal entry is clamped by itself. A ten-minute noisy run with gaps up to 3 s, braking and
stops keeps every prior and posterior symmetric PSD (smallest eigenvalue ≥ −10⁻⁹ of the largest
variance) for CV, both CA variants and the IMM.

## 7. Interacting multiple model

The cycle is Blom and Bar-Shalom's (`Blom1988`), over modes CV (1) and CA (2), across each
capture interval `Δt`:

1. **Transition.** Form `Π(Δt)` (below).
2. **Mixing.** `c_j = Σ_i Π_ij μ_i` and `μ_{i|j} = Π_ij μ_i / c_j`. Each mode starts from
   `x0_j = Σ_i μ_{i|j} x_i` and `P0_j = Σ_i μ_{i|j} (P_i + (x_i − x0_j)(x_i − x0_j)ᵀ)`. CV's
   transition then sets the mixed acceleration to zero.
3. **Mode-matched filtering.** Each mode predicts its mixed state and applies the Joseph update.
   The CA mode uses the coast rule. Both modes hold the same course: the combined estimate's at
   the gap's start.
4. **Mode probabilities.** `μ_j ∝ c_j L_j`, normalised in the log domain so that a large
   innovation cannot underflow both terms.
5. **Combination.** `x = Σ μ_j x_j` and `P = Σ μ_j (P_j + (x_j − x)(x_j − x)ᵀ)`.

Each step records `Π`, `c`, the mixing weights, each mode's log likelihood, and `μ`. "CA carried
0.75 of the weight while braking" can then be read off the record.

**One likelihood.** The mode likelihood is the full Gaussian:

```text
ln L = −d²/2 − (m/2) ln 2π − ½ ln det S,   d² = yᵀ S⁻¹ y
```

It includes `ln det S`, which a bare `d²` association cost leaves out. Both densities live in the
leaf package
[`l5tracks/likelihood`](../../internal/lidar/l5tracks/likelihood/likelihood.go), and
`motion.Likelihood.LogDensity` calls it. The package imports neither `l5tracks` nor `motion`, so
the tracker can import it too. Its functions take the innovation's dimension, `d²` and `ln det S`.
The caller computes both from one `S`.

**Robust option.** The Student-t density with scale `S` and `ν` degrees of freedom (default 4):

```text
ln L = ln Γ((ν+m)/2) − ln Γ(ν/2) − (m/2) ln(νπ) − ½ ln det S − ((ν+m)/2) ln(1 + d²/ν)
```

It is the IMM's default, because K9 measured 7 to 17 % of moving innovations beyond the 95 %
bound. Under a Gaussian, each of those would move most of the probability to the wider mode.

**Association shares it.** The tracker's default-off `RobustLikelihoodAssociationCost` (replay
`robust_likelihood_cost`) ranks admitted pairings by this Student-t at the same default `ν`, from
the same function. The cluster a track wins and the mode it credits are then scored on the same
terms, as the maths review requires
([20261108 §4.5 item 2](proposals/20261108-face-state-and-cv-ca-maths-review.md)). The cost is
`2 × (−ln L) − c`, where `c` is the value at `d² = 0` and the smallest `det S` the gate admits.
The factor of two keeps the cost on the `d²` scale. Subtracting `c` keeps it non-negative. The
gate stays a `d²` test. The cost reads the gate's own `S`, including A2's face-residual `S` under
near-edge tracking. See [gap analysis, Computed: S3](paper-implementation-gap-analysis.md#computed-s3).

**Transition in capture time.** Mode switching is a continuous-time Markov chain with rates
`λ12 = 1/dwell_CV` and `λ21 = 1/dwell_CA`, so `Π(Δt) = expm(Λ Δt)`. With `λ = λ12 + λ21` and
`E = e^{−λΔt}`:

```text
Π(Δt) = | (λ21 + λ12 E)/λ    λ12 (1 − E)/λ  |
        | λ21 (1 − E)/λ      (λ12 + λ21 E)/λ |
```

The off-diagonal terms use `expm1`, and each diagonal is one minus its row's off-diagonal. A
dropped frame or a capture gap therefore switches with the probability its length implies. A
per-frame matrix tuned at 10 Hz would not. Rows sum to one to 10⁻¹⁵,
`Π(a+b) = Π(a)Π(b)` to 10⁻¹⁴, the closed form matches the `expm` series to 10⁻¹⁵, and the
stationary distribution `(λ21, λ12)/λ` is invariant.

## 8. Offline defaults

These are prototype values for G-EST-1 to select, not tuning.

| Parameter                      | Default                | Reason                                                            |
| ------------------------------ | ---------------------- | ----------------------------------------------------------------- |
| CV `q`, `σ_p²`                 | 0.2 m²/s³, 0.05 m²/s   | The live `process_noise_vel` and `process_noise_pos`, coupled `Q` |
| CA `q`                         | 1 m²/s⁵                | The review's simulations; inside its 0.5 to 3 range               |
| Coast decay `τ`                | 1.5 s                  | Middle of the review's 1 to 2 s                                   |
| Course minimum speed           | 0.3 m/s                | A course read from a slower velocity is noise                     |
| Sub-step, coast threshold      | 0.1 s, 0.15 s          | One frame at 10 Hz; any dropped frame coasts                      |
| Variance cap                   | 100                    | The live `max_covariance_diag`                                    |
| Initial velocity, acceleration | 25 m²/s², 4 m²/s⁴      | 5 m/s sd for an unknown vehicle; the review's CA prior            |
| Measurement variance (adapter) | 0.05 m²                | The live `measurement_noise`                                      |
| IMM CV mode `q`, `σ_p²`        | 0.05 m²/s³, 0          | The quiet mode; CA carries manoeuvres                             |
| IMM CA mode `q`                | 2 m²/s⁵                | Wider than the CV mode; upper half of the review's range          |
| IMM dwell CV, CA               | 6 s, 3 s               | Steady driving is common; a speed change takes a few seconds      |
| IMM likelihood                 | Student-t, `ν = 4`     | K9's tails                                                        |
| IMM initial `μ`                | stationary, (2/3, 1/3) | The switching process's own long-run share                        |

The IMM's CV mode is quieter than single-model CV on purpose. The live `process_noise_vel` of 0.2
is sized to absorb manoeuvres that CV cannot represent. Run with that value beside the CA default,
the two modes are about as wide: steady prior position variance 0.030 against 0.031 m². Nothing
then separates them on a steady track, and the mode probabilities stay at or below the stationary
distribution whatever the motion. With the 6 s and 3 s dwells, noise-free constant velocity then
gives `μ_CV` 0.626 against a stationary 0.667.

## 9. Verified properties

Every figure below comes from the package tests: noise-free, along one axis, at 10 Hz, with
`R = 0.05 I` unless stated. These are derived results under the stated models, not measurements
on captures.

| Test                                                 | Result                                                                                                            |
| ---------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| Constant 6 m/s, 20 s                                 | CV and IMM velocity error below 10⁻³ m/s; IMM `μ_CV` ≥ 0.9 over the last 5 s, 0.911 at 20 s                       |
| Constant 1 m/s², 20 s, velocity lag                  | Live diagonal CV 0.708 m/s (the review's 0.71); coupled CV 0.707; CA 1×10⁻¹² m/s; IMM 0.224 m/s with `μ_CA` 0.560 |
| Braking 8 m/s at 2 m/s², stop at 4 s, gap 3 to 5.5 s | End velocity along course: CA without the rule −3.04 m/s, CA with it 0 (constrained 1.61 s into the gap)          |
| Same, reacquisition error                            | CV +7.78 m, CA with the rule −0.02 m, CA without −2.32 m, IMM +2.98 m (IMM's CA mode ends at +0.02 m/s)           |
| Same, IMM while braking (2.9 s)                      | `μ_CA` 0.755                                                                                                      |
| One 10σ lateral outlier (2.54 m) at 10 s             | Gaussian IMM `μ_CV` 0.911 → 0.015; Student-t (`ν = 4`) 0.911 → 0.873                                              |

The braking figures agree in direction with the review's Section 4.4 table, which uses a
different episode. The IMM's reacquisition error lies between CA's and CV's, because over a 2.5 s
gap `Π` pulls the predicted mode probabilities to about (0.55, 0.45). Its CA mode, mixed with
CV's zero acceleration, decays to rest without the constraint firing. With a CA-dominant
configuration (dwell 0.5 s and 100 s) the constraint does fire inside the IMM.

## 10. G-EST-1 analyses

The functions are pure, live in `l8analytics`, and take observation sequences and estimator
output.

**Criterion 2: partial correlation.** Per observation: the CV filter's NIS `y`, a smoothed
acceleration `a`, and covariates range, aspect and point count. Both `y` and `a` are residualised
by ordinary least squares on an intercept and the covariates. The basis is built by modified
Gram–Schmidt, so a collinear covariate is detected and dropped rather than amplified. `r` is the
Pearson correlation of the two residuals. Its interval is the Fisher-z interval: with `k`
covariates kept, `tanh(atanh r ± z_{(1+level)/2} / √(n − k − 3))`. The result reports `n`, `k`,
the interval and the level (default 0.95). On synthetic data with an injected partial correlation
of 0.5145, n = 20,000 gives 0.5141 [0.5038, 0.5242]. When NIS depends on acceleration only
through a shared covariate, the raw correlation is 0.563 and the partial correlation 0.003
[−0.011, 0.017].

The smoothed acceleration comes from `SmoothedKinematics`: an unweighted least-squares quadratic
in time per coordinate over ±0.5 s, needing five observations. It is non-causal, which suits a
label and keeps it independent of every model being scored. Aspect is the angle between the
course and the sensor ray, folded to [0, π/2]. The review's caution stands: run criterion 2 on
the face-referenced state, not the medoid, or it may pass for the wrong reason.

**Criterion 3: run-length signature.** A step's longitudinal residual is its innovation along the
prior velocity, normalised by its own predicted sd, `uᵀy / √(uᵀSu)`. Founding, rank-one and
near-stationary steps are skipped. A track is flagged when its sequence contains at least 3
consecutive values of one sign, each beyond 1 in magnitude. A value inside the threshold, or a
change of sign, ends a run. The share is flagged tracks over non-empty tracks offered; the gate
is over 5 % of moving tracks.

**Criterion 5: synthetic occlusion.** Gap starts are chosen once per track, from the observations
alone, so every model is scored on the same cases. A start must be an observation at least 1 s
after the first and at least 1 s after the previous start. For each start and horizon `H`
(0.5 s, 1.0 s and the longest permitted coast), the gap ends at the first two-dimensional
observation at or after `start + H`; a rank-one face fix measures one direction and is never a
gap's end. That observation must fall within 0.25 s of the target, or the case is skipped. Each
model runs once over the track. At a start it is cloned, and the clone predicts to the gap's end
with nothing observed, which is what withholding the gap's observations would give. The error is
the Euclidean distance from the predicted position to the observed one. A signed longitudinal
component along the smoothed course is kept per case; it shows reversal as a negative value. The
models are CV, CA with the coast rule, CA without it, and the IMM. The summary gives the
nearest-rank median and p95 (`stats.NearestRank`, decision E-03) per model and horizon, for all
gaps and for gaps whose smoothed longitudinal acceleration at the start is below −0.5 m/s².
`NoWorseThan` applies the gate: the candidate's median and p95 must not exceed the baseline's in
any row both share.

A hand-computed set checks the scorer: tracks at 10 and 4 m/s and one braking at 2 m/s², scored
with an estimator that holds the last position, reproduce every case error and every
median and p95. On noise-free braking-to-stop tracks (6 to 12 m/s at 2 m/s²) CA with the coast
rule passes criterion 5 against CV in every row. Bare CA ends more than 0.25 m behind a stopped
vehicle in 14 of the 28 decelerating 2.5 s gaps. Where braking continues through the gap, bare CA
is the true model and the decay costs accuracy: at 2.5 s, decelerating, the medians are 0.98 m
bare against 2.11 m with the rule and 9.00 m for CV. That trade is what `τ` is selected on.

**Two measurements, one replay.** `GEST1Collector` (`motion_gest1_face.go`) records, per track
and frame, the residuals of the tracked filter, from the filter-step record, and of the shadow
solid body's near-edge fixes, from the tracker after each update. A fix records each applied
face's innovation against the state the faces updated, `y_k = z_k − h_k·x⁻`, with the joint
`S = H P⁻ Hᵀ + R` (`l5tracks.FaceInnovation`); `yᵀS⁻¹y` equals the sequential NIS the update
reports, to rounding. A two-face fix's normals are perpendicular, so its residual in the site frame
is `Nᵀy` with covariance `Nᵀ S N`; a rank-one fix has only its normal component, attributed to
the course axis its normal lies nearer. Point count is joined from the cluster associated with the
track that frame. Each source's observations feed the offline models: a face's implied centre
along its normal is `n·c⁻ + y`, with variance its noise plus the predicted half-extent's variance.

**The smoothed acceleration** for both sources is the same estimator on each source's own
observations: the offline CV filter, smoothed over the whole chain (`motion.SmoothCV`, the
Rauch–Tung–Striebel pass), then the centred difference of the smoothed velocity at the nearest
observations at or beyond ±0.5 s, at most 1.5 s apart, projected on the smoothed course. It is
centred, so it has no filter lag; within about 1.5 s of a chain's end the smoothed velocity is
the filter's and the estimate is attenuated toward zero (to about 0.55 to 0.8 of a constant
deceleration's 0.5 s from the end, at the shipped measurement noise). The report's interior read
keeps only samples beyond that margin. Criterion 2's response is NIS divided by its degrees of
freedom, with the dimension as a covariate.

**Samples within a track are not independent.** Consecutive residuals of one track share its
range, aspect and manoeuvre, so the Fisher-z interval, which counts every sample as independent,
is too narrow when the samples come from few tracks. The report gives beside it a percentile
interval over 2,000 resamples of whole tracks with replacement (seeded). On four tracks sharing a
track-level effect the bootstrap is more than five times the Fisher interval's width; on forty
independent tracks the two agree.

The first replay of both sources, kirk0 under l5est0005, is in the
[G-EST-1 face-state screen](../../docs/lidar/operations/gest1-face-state-kirk0-2026-11.md): both
filters' NIS correlates with acceleration and neither is over-dispersed, so the criterion's
over-dispersion premise is not met there. Its face-referenced figures are superseded at `fc9d897`
(#989): the extent state's span-bound projection moves the face offsets, and criterion 4 no longer
passes on the faces (along ratio 0.729, across 1.108). The medoid figures and this reading are
unchanged; see the report's
[Since this report](../../docs/lidar/operations/gest1-face-state-kirk0-2026-11.md#since-this-report).

**Criterion 2 restated (l5est0007).** The screen's criterion read a correlation off zero, which an
under-dispersed filter can pass. The restated read (`motion_gest1_corpus.go`) asks first for
over-dispersion along the track, then for its cause, on one-step residuals (prior predicted across
at most 0.15 s, no re-reference):

- **Dispersion.** `z = along / √(uᵀSu)`, pooled RMS and the share of `|z|` beyond 1.96, with a
  percentile interval over 2,000 resamples of whole tracks; across-track the same. Each track's
  own RMS (five values or more) gives the per-track distribution. Over-dispersed means RMS above 1
  or more than 5 % beyond 1.96.
- **Cause.** The partial correlation of `z_along²` with the smoothed |longitudinal acceleration|,
  controlling for range, aspect and point count. The bootstrap keeps each track's centred moment
  sums over the five columns, and a draw's `r` is `−P₀₁/√(P₀₀P₁₁)` from the precision of their
  pooled covariance, equal to the Gram–Schmidt `r` to rounding.
- **Strata.** The along-track z RMS by smoothed speed, range, aspect, point count and track age
  (under or over 1 s since the track's first tracked-filter observation).
- **Age split** (l5est0008). The dispersion, the cause and criterion 4's offline tails read
  separately on residuals at a track age of at most 1 s and after it, each with its own
  whole-track bootstrap over the tracks that have a value on that side. A track record carries the
  age of each offline one-step value for this; records written before it carry none.
- **Jerk** (Punzo et al. 2011). For each estimator's output, the live filter's and each offline
  model's over the scored observations: `a = Δ|v|/Δt` over consecutive intervals of at most
  0.15 s with speeds of at least 0.5 m/s, `j = Δa` over the mean of the two intervals. The checks
  are the share of `|j|` above 15 m/s³ and the sign changes between consecutive jerk samples per
  second between them. The bound is Punzo's for passenger cars at 10 Hz, a plausibility check.
- **Turning.** The smoothed heading's centred rate over the acceleration's window, where the
  smoothed speed reaches 2 m/s at both ends and the middle; the share of track-frames above 0.1
  and 0.2 rad/s.

Every read is a sum over tracks, so a case writes its moving tracks' records and
`lidar-gest1-pool` reads any union: the corpus, one case, or the corpus with one case left out.
The first corpus read, on the 23 first segments, is the
[G-EST-1 corpus read](../../docs/lidar/operations/gest1-corpus-read-2026-11.md): the medoid's
along-track excess is in the first second of each track, and the residuals after it are
under-dispersed. The [second-segment read](../../docs/lidar/operations/gest1-second-segments-2026-11.md)
held that split out on the 22 second segments under a rule fixed before its replays: 2.42 at a
track age of at most 1 s, 0.85 after it.

## 11. What this does not establish

- No capture was replayed. Every figure is a noise-free or seeded-noise simulation under the
  stated model.
- The defaults are not selected. `τ`, the CA and IMM-mode `q`, the dwell times and `ν` are the
  parameters G-EST-1 selects. The IMM's 0.9 on steady CV holds at the defaults, by a margin of
  0.011. It falls to 0.63, below the stationary 0.67, if the modes take the single-model noise
  levels.
- The adapter carries the scalar `R` the live gain used, or the cluster-shape covariance as an
  option. The filter-step record holds no rank-one direction or point count; `GEST1Collector`
  joins the point count from the tracker's input and reads the solid body's face record for the
  direction.
- A face fix converted to a centre observation drops the position–extent covariance the solid
  body carries; the offline models are labels and comparisons, not the solid body.
- Velocity is initialised at zero with a 5 m/s sd. The review's two-point initialisation (its
  Section 4.2) is a separate change and is not implemented here. The first second of each track
  is therefore a poorer basis for criterion 5 than later seconds; the 1 s warm-up limits that.
- The IMM covers CV and CA only. The plan's stationary mode, and any gate on IMM itself, remain
  deferred.
