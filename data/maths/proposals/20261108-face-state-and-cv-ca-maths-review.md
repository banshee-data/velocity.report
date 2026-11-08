# Face state estimation and CV/CA tracking: maths review, November 2026

<!-- ignore-style-length -->

This note reviews the mathematics that stands between the current tracker and two goals: a
solid-body state estimated from the faces and facets the sensor sees, and a tracker that can run
constant-velocity (CV) and constant-acceleration (CA) motion models side by side. It reads the
latest evidence (the F0 attribution report, geom0015, figure0002 and l5est0003), the code that
implements the near-edge face update, the extent belief and the facet pose proposal, and the CV
filter's own dynamics, which it computes. It ranks the fixes by what each unblocks.

- **Status:** Proposal Math (Not Active in Current Runtime); a review with derivations and
  numerical checks, no code changed and no replay run
- **Layers:** L4 Perception (face planes, spans), L5 Tracks (state, extent, motion model,
  association), annotation (facet pose proposals)
- **Related:** [state estimation plan](../../../docs/plans/lidar-state-estimation-plan.md),
  [facet registration plan](../../../docs/plans/lidar-facet-registration-experiment-plan.md),
  [facet registration maths review](20261008-facet-registration-maths-review.md),
  [visibility-aware tracking maths](20260905-visibility-aware-object-tracking-research.md),
  [tracking maths](../tracking-maths.md), [gap analysis](../paper-implementation-gap-analysis.md)

## 1. Answer

The face measurement is sound in form and wrong in one input: it treats the believed half-extent
as exact, and the belief is short and overconfident. CA cannot help until that is fixed, because
the error it leaves lies along the track, which is the axis an acceleration state would absorb.
The high-priority fixes, in order:

1. **Put the extents in the state** (Section 3.1). Estimate half-length and half-width jointly
   with position, so a face measures `n·c + h` rather than `n·c` given an `h` the filter trusts
   absolutely. This carries the position–extent correlation the state plan requires for a bumper
   bound, and makes length observable when the front and rear are seen at different times.
2. **Make the extent belief a censored posterior, not a shrinking sample mean** (Section 3.2).
   Its sigma falls as the prior's over the square root of the frame count, to 0.25 m after 36
   frames, from lower bounds that are neither independent nor full views.
3. **Honour the current frame's span as a lower bound in the belief** (Section 3.3). The box is
   floored to it; the belief and the face update are not, which is why F0 found the believed
   length short of the labelled span in 83 % of car and 97 % of truck instants.
4. **Initialise velocity from two measurements** (Section 4.2). A track born at 6.5 m/s starts at
   zero with a 1 m/s sigma and takes 1 s to come within 10 %, past confirmation.
5. **Give CA a coast rule before it ships** (Section 4.4). Computed here, a CA filter that loses a
   braking car for 2.5 s reverses it at 2 m/s.
6. **Specify the IMM's likelihood, mode transition and state embedding before writing one**
   (Section 4.5). The association cost, the gate and the mode probability must use one
   likelihood, or the modes are scored on a cost the S3 analysis already found biased.

For the facet registration path the first fix is smaller: the pose proposal measures yaw from a
segment correspondence and then discards it, and its straightness test rejects flat panels that
the October review said must pass (Section 3.6).

## 2. What was reviewed

The evidence that sets the priorities:

| Source                                                                                           | Finding that bears on this review                                                                                                                                     |
| ------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [F0 attribution report](../../../docs/lidar/operations/facet-f0-attribution-kirk0-report.md)     | Believed length short of the labelled span in 83 % of car and 97 % of truck instants; removing the shortfall takes end-face medians from −0.30 and −2.14 m to −0.02 m |
| [geom0015](../../../docs/lidar/operations/d2-frozen-split-bias-baseline-2026-11.md)              | The published box sits 0.19 m from 22 reviewed body centres at the median; truck 2's 1.5 m error is extent, not position bias                                         |
| [figure0002 and l5est0003](../../../docs/lidar/operations/cloud-run-kirk0-labelled-2026-11.md)   | The filter's speed is 0.77 and 0.86 m/s (about 12 %) low on two trucks; four times the velocity process noise brings one within its bound, not the other              |
| [gap analysis K9](../paper-implementation-gap-analysis.md#measured-outcome-k9)                   | Longitudinal innovation RMS is 1.5 to 2.4 times what the filter expects at every site; lateral matches                                                                |
| [state plan §7.3](../../../docs/plans/lidar-state-estimation-plan.md#73-recommendation-and-gate) | G-EST-1 requires an acceleration-specific CV failure and a CA coast no worse than CV's                                                                                |

The code read, at `ffe6755`: `l5tracks/measurement_model.go` (the face measurement),
`solid_body_nearedge.go` (the sequential face update, containment truncation, extent admission,
orientation), `heading_extent.go` and `solid_body_lifecycle.go` (the extent belief and its sigma),
`tracking.go`, `tracking_association.go` and `tracking_update.go` (initialisation, prediction,
update), `tracking_covariance.go`, `annotation/facet_pose_proposal.go`,
`annotation/features_segment.go` and the visualiser's `FacetBodyRegistration.swift`.

## 3. Face and facet state estimation

### 3.1 The half-extent is a state, not a constant

`MeasureNearEdge` places the near face's plane at a percentile of the returns along its normal and
reports the centre's offset as `plane − h`, where `h` is half the believed dimension.
`applyEdgeMeasurements` then applies one scalar update per face with `R = measurement_noise`
(0.05 m², 0.22 m). The comment states the reason: the half-extent's error is a bias, and Section
8.1 of the state plan forbids widening `R` to cover a bias.

The rule is right and its consequence here is not. A bias that is neither corrected nor carried
enters the centre one for one and the filter reports it at the face's own precision. F0 measured
the result: the belief is short, so the implied centre sits toward the sensor by half the
shortfall, which is 0.25 m at the median for cars and 2.39 m for trucks. The repair Section 8.1
asks for is correction, and correction needs the extent to be estimated where the face is.

**Fix.** Augment the dynamic state with the two half-extents, `x = [x, y, vx, vy, hL, hW]`, with
`hL` and `hW` constant in the transition and a small process noise for articulation and
membership drift. A face with outward normal `n` along the length axis measures

`z = n·c + hL`, `H = [n_x, n_y, 0, 0, 1, 0]`

and a side face uses `hW` in the same place. Three properties follow.

- **Observability.** A single face cannot separate `c` from `h`: its row has a null direction in
  `(n·c, h)`. A rear face seen on approach and a front face seen on departure have rows `+n` and
  `−n` against the same `hL`, so their difference fixes `hL` and their sum fixes the centre. A
  side face supplies `hW` the same way against the opposite side, which a single sensor never
  sees; there `hW` stays at its prior and the filter says so, which is the honest output.
- **Correlation.** `P` carries the covariance between position and extent, so the projected
  bumper `c + hL·u` has a variance that includes both, as the state plan's solid-body contract
  requires (Section 5.4).
- **No smoothing.** The extent's process noise is near zero, so the update cannot follow a
  manoeuvre through it; the manoeuvre still goes through the velocity block.

Orientation can stay outside the state under Option A, but see Section 3.4 for the term it must
contribute to `R`. A Schmidt consider filter (the extent's covariance carried, its state not
updated) is the lighter alternative; it removes the overconfidence but not the bias, so it is
second best.

### 3.2 The extent sigma shrinks with frame count

`extentBeliefSigma` returns the class prior's sigma over the square root of the admitted frame
count, floored at the 0.25 m bin. For a rigid vehicle the prior sigma is 1.5 m, so the floor is
reached after 36 admitted frames, about 3.6 s at 10 Hz, whatever those frames showed.

That is the variance of a mean of independent full-dimension samples. The admitted spans are
neither: each is a lower bound (the code says so), consecutive frames of one view share their
truncation, and the estimate is the third-largest bin, not a mean. The state plan's Section 9.2.2
already rules this out: "Frame count alone cannot justify shrinking sigma."

**Fix.** Treat each admitted span `s_k` as the censored observation `L ≥ s_k − ε_k`. With a
Gaussian prior `N(μ0, σ0²)` and only lower bounds, the posterior is that prior truncated below at
`max_k(s_k) − ε`, with mean and variance from `truncatedNormalMoments`, which the containment code
already has. The posterior narrows only when a bound moves into the prior's mass. A two-sided
measurement, a resolved front and rear in one frame or across frames through Section 3.1, is the
only thing that narrows it further. Under Section 3.1 this becomes a truncation of the `hL`
marginal through the state's regression on it, exactly as `truncateAxis` already does for
position.

### 3.3 The current frame's span is not in the belief

The belief admits `minimumAxisSpan`, the smallest trimmed span over a ±10° axis window, which
understates by construction, and reports the third-largest admitted value. The containment floor
then raises the reported box to the current frame's span, but the comment is explicit that "the
beliefs never see it", and `measureNearEdgeFrame` reads the belief, not the floor, for `h`.

So the face update uses an extent the same frame contradicts. F0's table is the measured
consequence. **Fix:** feed the floor's span to the belief as a lower bound (Section 3.2's
truncation), so the posterior median is never below what the frame shows less its noise. With
Section 3.1 in place the floor becomes a constraint on `hL` and `hW` and the separate floor in
`assembleSolidBody` can go.

### 3.4 Orientation error is missing from the face noise

A face's implied centre offset depends on the axis it was measured along. If the believed axis
is off by `δψ`, returns at a tangential lever arm `t` from the centre move the face plane by about
`t·δψ` along the normal. At the end face of a 4.5 m car with 10° of heading error, which kirk0
showed on 59 % of moving near-edge frames, the lever arm of half the width moves the plane by
0.17 m; at a side face the lever arm is half the length, 0.39 m. Neither term is in `R`, and the
orientation belief already carries `VarianceRad2`.

**Fix:** add `σ_ψ² · t_eff²` to each face's variance, with `t_eff` the RMS tangential offset of
the face's supporting returns from the predicted centre. This is a variance, not a bias, so
Section 8.1 permits it. It is also what G-UNC-1's aspect strata will otherwise report as
miscalibration.

### 3.5 Smaller defects in the face path

- **Re-reference inflation is isotropic at the half-width.** When a medoid-referenced body is
  first re-referenced, or lapses back, the position variance gains `(W/2)²` on both axes. The
  medoid's bias along the body reaches `L/2`, so an end-face re-reference under-states it by a
  factor of about 2.4 for a car. Inflate `(L/2)²` along and `(W/2)²` across the believed axis.
- **Entry consider is a one-frame term for a persistent bias.** `FaceEntryConsider` adds the
  half-extent variance on the frame a face enters and never again. Section 3.1 replaces it; until
  then it is the wrong shape in both directions.
- **One NIS per fix sums ranks.** `m.NIS` adds the face terms; the rank is recorded beside it, so
  G-UNC-1's per-dimension strata can still be read, but a consumer that pools NIS across rank-one
  and rank-two fixes compares χ²₁ with χ²₂.

### 3.6 The facet pose proposal

`ProposeFacetPose` is careful: it keeps the prior verbatim, refuses contradictions, and labels
what it did not measure. Two things limit it as a state estimator.

- **It measures yaw and discards it.** For a segment, the target line's normal and the anchor's
  body-frame normal give the body's yaw directly: `ψ = angle(n_target) − angle(n_body)`, folded
  over π, with a bound of the two normal bounds and no dependence on the prior's yaw bound. The
  code computes both normals, uses them only to check consistency, and returns
  `yaw_from_prior_not_measured`. Returning the measured yaw removes the prior's yaw bound from the
  normal bound (`angular` in the code) and makes the segment a two-parameter constraint, normal
  position and yaw, which is the review's Section 4.6 case.
- **The straightness test rejects flat panels.** Both `featureSegmentRelation` and the
  visualiser's `makeSegment` measure each return's distance from the endpoint line in three
  dimensions. The October review computed that a constant-elevation ring sags 0.13 to 0.22 m
  across a flat 4.5 m face at 5 m on the steep rings, and made "fit in plan view" one of E1's
  must-pass checks. With a return bound under the sag, a flat side is refused. Measure the
  perpendicular distance in plan view, and check the vertical separately if at all.

The proposal's bounds are worst-case half-widths; the filter's are Gaussian variances. Before a
facet measurement enters the tracked state, one conversion must be declared (for example, a
half-width read as √3 sigma for a uniform error or three sigma for a Gaussian one, or the
proposal kept as a set-membership constraint applied by truncation, which `truncateAxis` can
already do). Mixing the two without a
declared rule would make the facet arm look confident by construction, which the facet plan's
uncertainty row already forbids.

## 4. CV, CA and the multiple-model filter

### 4.1 The shipped filter, computed

Per axis at the shipped tuning (`dt` 0.1 s, `measurement_noise` 0.05, `process_noise_pos` 0.05,
`process_noise_vel` 0.2, diagonal `Q`), the steady state is:

| Quantity                                  |    Value |
| ----------------------------------------- | -------: |
| Prior position variance                   | 0.030 m² |
| Innovation standard deviation             |   0.28 m |
| Position gain                             |     0.38 |
| Velocity gain                             | 0.50 s⁻¹ |
| Velocity lag behind a 1 m/s² acceleration | 0.71 m/s |
| Velocity noise at a 0.22 m measurement sd | 0.13 m/s |

The lag is linear in the acceleration: 1.42 m/s at 2 m/s², 2.13 m/s at 3 m/s². The innovation
figure matches the K9 analysis's 0.28 m. Raising the velocity process noise trades lag for noise:

| `process_noise_vel` | Lag at 1 m/s² | Velocity noise sd |
| ------------------: | ------------: | ----------------: |
|                 0.2 |      0.71 m/s |          0.13 m/s |
|                 0.4 |      0.55 m/s |          0.17 m/s |
|                 0.8 |      0.43 m/s |          0.23 m/s |

Two readings. The trucks' 0.77 and 0.86 m/s shortfall is the size a CV lag would produce under
about 1 m/s² of acceleration, so it is consistent with an acceleration failure; it is equally
consistent with the cluster centre sliding along a long body, which is K9's longitudinal excess.
And no single `process_noise_vel` removes the lag without raising noise, which is the usual case
for CA or a multiple-model filter, but only once the measurement is not the cause.

### 4.2 Velocity initialisation

`initTrack` sets the velocity to zero with unit variance per axis. A vehicle at 6.5 m/s is then
6.5 sigma from its prior. Computed without measurement noise, the velocity error is 2.40 m/s after
0.5 s and 0.53 m/s after 1 s, and comes within 10 % at 1.0 s, about six frames after confirmation at
`hits_to_confirm` 4. Through that second the prediction lags the vehicle, the gate is centred
behind it, and a young track is the one most likely to lose its cluster to a neighbour.

**Fix:** two-point initialisation. On the second association, set the velocity to the difference
of the two measurements over their capture interval, with variance `2R/Δt²` per axis (10 m²/s² at
0.1 s), or set the first-frame velocity variance from the class's speed range (for a vehicle,
`(v_max/2)²`) so the prior is honest. Either is a strict improvement and is independent of the
motion model. It may also explain part of l5est0003's identity gain from a looser velocity
process noise, since a larger `q` speeds the same convergence (0.53 to 0.21 m/s error at 1 s);
that is a hypothesis to test, not a finding.

### 4.3 Why CA must wait for the measurement

Every row of the state plan's motion-model matrix scores `--` on the lateral-jump criterion, and
K9 shows where the residual error is: along the track. A CA state adds an along-track degree of
freedom that will absorb whatever along-track error the measurement leaves, so a centroid
sliding toward a truck's front as it enters view would be read as acceleration. G-EST-1's second
criterion, a partial correlation of NIS with acceleration controlling for range, aspect and point
count, is the right discriminator and should be run on the face-referenced state of Section 3.1,
not on the medoid. On the medoid it is likely to pass for the wrong reason.

CA also costs noise. On the same simulated noise, CA with an acceleration spectral density of 0.5
to 3 m²/s⁵ gives a velocity noise sd of 0.23 to 0.37 m/s against CV's 0.13. That is the price a
multiple-model filter avoids by running CV when nothing accelerates.

### 4.4 CA needs a coast rule, and it needs to be part of the model

Criterion 5 of G-EST-1 asks that CA be no worse than CV across a coast. Computed on two
noise-free episodes:

| Episode and gap                      | CV reacquisition error | CA reacquisition error |
| ------------------------------------ | ---------------------: | ---------------------: |
| Steady braking, 0.5 s gap            |                +0.87 m |                +0.63 m |
| Steady braking, 1.0 s gap            |                +1.67 m |                +1.09 m |
| Steady braking, 2.5 s gap            |                +4.79 m |                +2.42 m |
| Braking to a stop inside a 1.5 s gap |                +4.61 m |                −0.61 m |
| Braking to a stop inside a 2.5 s gap |                +9.71 m |                −1.67 m |

CA wins every row on magnitude, and in the last it ends the gap reversing at 2.0 m/s. A reversing
vehicle is a model output that the behaviour layer would read as a physical event. The rule that
ships with CA:

- **No reversal while unobserved.** During a coast, when the predicted velocity's projection on
  the course at the gap's start reaches zero, hold the velocity and acceleration at zero. This is
  a constraint on the prediction, applied as a projection of the state and its covariance, not a
  clamp of the state alone.
- **Acceleration decay.** Use a Singer-type acceleration with a time constant of about 1 to 2 s,
  so an unobserved acceleration relaxes toward zero. With the decay set to an immediate reset the
  computed errors return to about CV's, so the time constant is the parameter G-EST-1 should select.
- **Capture-gap prediction on.** `max_predict_dt` clamps a prediction at 0.5 s. For CV that
  under-predicts a gap linearly; for CA the omitted `dt²` term makes it worse. CA should require
  the existing default-off `CaptureGapPrediction` sub-stepping.

### 4.5 What an IMM over CV and CA must specify first

The state plan defers IMM behind a gate and lists where implementations stumble: mixing,
mode-conditioned covariances and gating. Four specifics follow from this tracker's own maths.

1. **One state space.** Embed CV in the six-state CA space with the acceleration fixed at zero
   and its variance near zero, so mixing combines like with like. Mixing a four-state CV estimate
   with a six-state CA one needs an explicit rule for the missing rows, and an ad hoc one biases
   the mixed acceleration.
2. **One likelihood.** A mode's probability is updated by its Gaussian likelihood,
   `exp(−d²/2)/√det(2πS)`, which contains `ln det S`. The shipped association cost is bare `d²`,
   which the S3 analysis found lets a coasting track outbid a fresh one; the likelihood cost arm
   already adds `ln det S`. If association uses `d²` while mode probabilities use the likelihood,
   the cluster a track wins and the mode it credits are scored on different terms. Choose the
   likelihood for both, or state why not.
3. **A robust likelihood.** K9 measured 7 to 17 % of moving-track innovations beyond the 95 %
   bound. A Gaussian likelihood lets one such innovation move most of the probability to the
   wider mode. Use a Student-t or a likelihood floor, and test mode stability on constant-speed
   segments; the state plan's IMM trigger is CA over-dispersion there.
4. **A transition matrix in capture time.** Specify mode switching as a rate (for example a mean
   dwell of a few seconds) and form `Π(Δt) = exp(Λ Δt)` from each frame's capture interval. A
   per-frame matrix tuned at 10 Hz is wrong across a capture gap or a dropped frame, which the
   time-domain work made explicit for the rest of the filter.

Two existing gaps reopen with a larger state, as the decision review noted: K1 (use the coupled
`Q` derived from the continuous model, which CA's `Q` is by construction) and K2 (the Joseph form,
which the face update already uses).

### 4.6 Covariance hygiene before mixing

Two operations change the state without keeping `P` consistent, and mixing would propagate them.

- **The diagonal cap.** `predict` and the coast inflation clamp each diagonal at
  `max_covariance_diag` and leave the off-diagonals, so a correlation can exceed one and `P` stops
  being positive semi-definite. At the shipped 100 m² and 25 confirmed misses it is not reached
  (12.5 m² of inflation), so it is latent. Scale the row and column instead,
  `P ← D P D` with `D` the ratio of the capped to the uncapped standard deviation.
- **The speed clamp.** `clampVelocity` rescales the velocity and leaves `P`. Under CA the
  acceleration's covariance with velocity makes the next update pull the clamped velocity back.
  Apply the clamp as a truncation through the state's regression, as `truncateAxis` does.

## 5. Order of work

| Order | Fix                                                                  | Unblocks                                                                                     | Size          |
| ----: | -------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- | ------------- |
|     1 | Extents in the state, face row `[n, 0, 0, ±1]` (3.1)                 | A bumper bound with correlation; length from front and rear; the facet arms' common baseline | M, L5         |
|     2 | Censored extent posterior and the current span as a bound (3.2, 3.3) | F0's shortfall; an honest extent sigma for the following report                              | S, inside 1   |
|     3 | Orientation variance in the face noise (3.4)                         | G-UNC-1's aspect strata reading calibration rather than a missing term                       | S             |
|     4 | Two-point velocity initialisation (4.2)                              | The first second of every track; a cleaner G-EST-1 population                                | S             |
|     5 | Facet segment yaw and plan-view straightness (3.6)                   | Facet proposals that measure yaw; flat sides admitted as E1 requires                         | S, annotation |
|     6 | G-EST-1 criterion 2 on the face-referenced state (4.3)               | The decision whether CA is needed at all                                                     | S, analysis   |
|     7 | CA with the coast rule and capture-gap prediction (4.4)              | G-EST-1 criterion 5                                                                          | M             |
|     8 | Covariance hygiene (4.6), then the IMM specification (4.5)           | The deferred IMM gate                                                                        | S, then M     |

Items 1 to 3 are one change to the solid-body filter and should be scored as one arm against the
geom0015 box rows on kirk0 and then held out, since geom0015 already pins the comparison. Item 4
changes every track's first second and should be its own arm, read against l5est0004's identity
rule. Nothing in items 1 to 6 changes the motion model, so the state plan's rule of one change per
increment holds.

## 6. Numerical checks performed

The figures in Sections 4.1, 4.2 and 4.4 come from a per-axis simulation of the shipped
prediction and update: `F = [[1, dt], [0, 1]]`, `Q = diag(0.05 dt, 0.2 dt)`, `R = 0.05`,
`dt = 0.1 s`, initial covariance `diag(10, 1)` for initialisation runs and a converged covariance
for steady-state and coast runs. Lag is read after 20 s at constant acceleration; velocity noise
over 54 s of a constant 6 m/s with Gaussian measurement noise of 0.22 m (seed 1). The CA filter
uses the continuous white-noise-jerk `Q` with spectral density 1 m²/s⁵ unless stated, `R = 0.05`
and initial covariance `diag(10, 1, 4)`. The braking episodes are 2.9 to 1.1 m/s at 1.8 m/s² then
a stop at 1.1 m/s², with gaps from 2.5 s and 4.2 s; and 8 m/s braking at 2 m/s² to a stop at 5 s
with gaps from 3 s. Reacquisition error is the predicted position at the first observation after
the gap less the true position. These are derived results under the stated model, not
measurements on captures.

## 7. What this does not establish

- No arm was replayed. Every fix is a proposal whose benefit is a prediction; Section 3's are
  predicted from F0's post-hoc shortfall analysis on one capture, two of whose vehicles are trucks.
- The trucks' speed shortfall is shown to be the size a CV lag would produce, not shown to be one.
  Telling acceleration from centroid sliding is G-EST-1's job on the corrected state.
- The coast simulations are noise-free and one-dimensional; they bound the direction of each
  effect, not its size on real gaps.
- The facet proposal findings are read from the code against the October review's checks; no
  operator pack was run through them.
- The Singer time constant, the extent process noise and the IMM dwell rate are named as
  parameters to select, not chosen.

## 8. Provenance

- Repository at `ffe6755` (main, 2026-11-06), branch `claude/lucid-bardeen-ux375z`.
- Tuning read from `config/tuning.defaults.json` (`l5.cv_kf_v1`), fingerprint `ab0c4035780d` as
  geom0015 records it.
- References: `Blom1988` for the IMM, `BarShalom2011` for the discretised process noise and the
  Joseph form, `Scholler2020` for CA's extrapolation failure; all in
  [references.bib](../references.bib).
- The simulation script lived in the session's scratch space and is not committed; Section 6
  gives every parameter needed to reproduce it.
