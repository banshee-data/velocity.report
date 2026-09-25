# Adaptive uncertainty: G-UNC-1 predeclared criteria

The criteria the adaptive measurement-noise model must meet before it ships, pinned before any
decision-partition replay is scored; and the label-free evidence gathered so far.

- **Status:** Predeclared. Model, pre-gate statistics, gate-rejection evidence, calibration fit and
  report implemented and tested on synthetic scenes; held-out decision-partition scoring and the
  labelled manoeuvre set are outstanding
- **Layers:** L5 Tracks, L8 Analytics, offline replay
- **Related:** [State-estimation plan §8](../../plans/lidar-state-estimation-plan.md#8-uncertainty-matrix),
  [Phase 3](../../plans/lidar-state-estimation-plan.md#phase-3-adaptive-uncertainty-and-residual-statistics),
  [tracking maths §12](../../../data/maths/tracking-maths.md#12-adaptive-measurement-uncertainty),
  [Phase 0/1 corpus baseline](state-estimation-phase01-corpus-baseline.md),
  [per-frame evaluation](per-frame-evaluation.md)
- **Code:** [noise model](../../../internal/lidar/l5tracks/adaptive_noise.go),
  [pre-gate statistics](../../../internal/lidar/l5tracks/pregate.go),
  [fit and report](../../../internal/lidar/l8analytics/uncertainty_calibration.go),
  [replay wiring](../../../internal/lidar/replayeval/uncertainty.go),
  [corpus tool](../../../cmd/tools/lidar-state-estimation-baseline/uncertainty.go)

## Declaration

Everything under [Criteria](#criteria), [Population](#population) and
[Calibration protocol](#calibration-protocol) was fixed before the kirk0 evidence run recorded
below and before any decision-partition replay. Wiring smoke runs on kirk0's 70–76 s window
preceded this page; their figures are not evidence and are not reported. A threshold changed after
a held-out result has been seen makes that comparison worthless, so a change is an amendment:
dated, with its reason, recorded under [Amendments](#amendments) before the next held-out run, and
never applied to a result already produced. The thresholds are the plan's Section 8.3 table,
unchanged; this page makes each row operational.

## Arms

| Arm                 | Replay configuration                                                          |
| ------------------- | ----------------------------------------------------------------------------- |
| Shipped             | Default: isotropic `measurement_noise` R                                      |
| Adaptive prior      | `-experiment adaptive_uncertainty`, no table: every coefficient is the scalar |
| Adaptive calibrated | `-experiment adaptive_uncertainty -uncertainty-calibration <frozen table>`    |

The model is Section 8.1's: R diagonal along and across the sensor's line of sight to the
measurement, rotated into the site frame. Each axis's variance is a per-stratum coefficient
(measurement source × range bin × support bin × folded aspect bin) plus the sensor physics terms.
Only the calibrated arm is a candidate for G-UNC-1; the prior arm exists to show the physics terms
alone change little.

## Population

Eligible pre-gate observations, as `l5tracks` records them inside a calibration window:

1. The track is confirmed at association time.
2. Exactly one cluster is physically plausible for the track (inside `max_position_jump_metres`,
   under `max_reasonable_speed_mps` implied speed, not a guarded fragment), and the track is the
   only active track for which that cluster is plausible.
3. A line of sight exists (the measurement is at least 0.5 m from the sensor).

Membership never depends on S, so no noise model can select its own passing sample. Every eligible
pairing is recorded whether the innovation gate accepts it or not. Ambiguous scenes are excluded,
and the Euclidean bound truncates the far tail lightly; both are properties of the population, not
of an arm. Accepted-only statistics are reported beside, never instead.

## Evidence sets

| Set                | Contents                                                                                                                                    | Use                                                    |
| ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------ |
| Fitting partition  | `marina-webster-beach` and `columbus-broadway` from the [Phase 0/1 corpus](state-estimation-phase01-corpus-baseline.md), 70 s warm-up       | Fit and iterate the table; tune nothing else           |
| Decision partition | `embarcadero-folsom`, the corpus's reserved held-out site, 70 s warm-up                                                                     | Rows 1–5, scored once with the frozen table            |
| Manoeuvre set      | Labelled turns, brakes and lane changes on decision-partition captures, from the [annotation packs](point-annotation-tool.md) once reviewed | Row 6                                                  |
| Synthetic          | `l5tracks` scene tests                                                                                                                      | Row 7, and the unit contracts                          |
| kirk0              | The in-repo capture                                                                                                                         | Development evidence only: one site, one short capture |

## Criteria

Each row applies to the adaptive calibrated arm on the decision partition unless stated. "Mean
NIS/m" is the mean normalised innovation squared over the population above; m is the measurement
dimension, 2 for every online position measurement.

| #   | Row                                          | Operational test                                                                                                                                                                                                                                      | Threshold                                |
| --- | -------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------- |
| 1   | NIS mean by measurement dimension            | `gate_checks` `nis_mean_joint_m2`; `m1` too if a rank-one measurement ever enters                                                                                                                                                                     | Mean NIS/m in `[0.7, 1.4]`               |
| 2   | Chi-squared goodness of fit                  | `gof_joint_m2`: Pearson over 10 equiprobable chi-squared(m) bins, 9 degrees of freedom, needs at least 50 samples                                                                                                                                     | Not rejected at p = 0.01                 |
| 3   | Range deciles                                | `range_deciles`: every decile with at least 30 samples                                                                                                                                                                                                | Within `[0.5, 2.0]` × overall mean NIS/m |
| 4   | Point-count deciles                          | `support_deciles`: as row 3; tied counts may merge deciles                                                                                                                                                                                            | As row 3                                 |
| 5   | Aspect octants                               | `aspect_octants`: the sensor's 45° octant in the body frame, every octant with at least 30 samples                                                                                                                                                    | As row 3                                 |
| 6   | Genuine manoeuvres falsely gated             | Labelled manoeuvre frames whose object's eligible sample in `uncertainty_samples.jsonl` has `gated: true`, over all labelled manoeuvre frames with an eligible sample, matched by frame time and within 1 m plus half the object's footprint diagonal | Under 1 %                                |
| 7   | Recovery after a synthetic 5-frame occlusion | The scene of `TestRecoveryAfterFiveFrameOcclusionIsUnderThreeFrames`, with the frozen table's coefficients loaded                                                                                                                                     | Under 3 frames                           |

Rows 1–5 are computed by the report builder; the tool reports `within`, `outside` or
`insufficient_samples` and never an overall pass. A row with `insufficient_samples` has not
passed. Row 6 needs a scorer that joins the samples file to the labels; it is not built. Row 7's
test runs today with a representative synthetic table; loading the frozen table into that scene is
a step of the decision run.

**Promotion guard, beside the gate.** The calibrated arm must not worsen identity switches or IDF1
against the shipped arm on the reviewed held-out episodes of the
[per-frame harness](per-frame-evaluation.md). A noise model that passes rows 1–7 while splitting
identities is not promoted: D5 withdrew the OBB centre for exactly that.

**Reported, not gating.** The radial and tangential one-dimensional marginals and their fits,
95 % interval coverage, the accepted-only figures, the coast split, the per-stratum table, the
gate-rejection outcomes (persistent, transient, unresolved) and births against confirmations. They
explain a row's result; they do not replace one.

## Calibration protocol

1. Replay the fitting partition with the shipped arm and `-uncertainty-report`; the pooled report
   is round 0.
2. Replay it again with `-experiment adaptive_uncertainty` and the previous round's pooled table.
   Stop when the pooled fit reports `converged` (every fitted coefficient within 10 % of the one
   the round ran with), or after five rounds. Freeze the last round's table; its content address
   is its ID.
3. Replay the decision partition once with each arm, the calibrated arm on the frozen table, and
   read rows 1–5 from the reports. Score row 6 from the calibrated arm's samples file, row 7 with
   the frozen coefficients, and the promotion guard with the per-frame harness.

The fit's estimator is `mean`: it matches the statistic rows 1 and 3–5 judge. The median
estimator exists for diagnosis only. Coasting samples are excluded from the fit and included in
every row.

### kirk0 development protocol

kirk0 has no held-out partner, so the in-repo run is a time split of one capture, fixed here
before it ran. It can show whether a table fitted on one half of a capture holds on the other; it
cannot show that a table holds at another site, and passes no row of G-UNC-1.

1. Shipped and adaptive prior arms over the whole capture, scored from 20 s after a 20 s warm-up.
2. Fitting half: scored 20–50 s after a 20 s warm-up, iterated by the protocol above.
3. Evaluation half: shipped and calibrated arms scored from 50 s to the end, warmed from 20 s, the
   calibrated arm on the table frozen in step 2.
4. Report rows 1–5 as the tool computes them, the gate-rejection outcomes, and births against
   confirmations, for every arm.

## Selection rule

The adaptive calibrated arm passes G-UNC-1 only if rows 1–7 all hold and the promotion guard
holds. Otherwise it stays default-off, and the report's failing rows say why: a range or aspect
stratum outside its bound is a model-structure problem, a failed fit with in-bound means is a
heavy tail, and a floor-clamped coefficient says the process noise, not R, sets the spread.

## Running it

The fitting and decision replays for a local agent with the corpus and labels. `OUT` names an
empty directory on a different device from `LIDAR_PCAP_DIR`.

```bash
# Round 0, fitting partition, shipped arm
go run -tags=pcap ./cmd/tools/lidar-state-estimation-baseline -pcap-root "$LIDAR_PCAP_DIR" \
  -case marina-webster-beach,columbus-broadway -uncertainty-report -out "$OUT/fit-r0"

# Round k (k = 1..5), until fit.converged in $OUT/fit-rk/uncertainty-calibration.json
go run -tags=pcap ./cmd/tools/lidar-state-estimation-baseline -pcap-root "$LIDAR_PCAP_DIR" \
  -case marina-webster-beach,columbus-broadway -uncertainty-report \
  -experiment adaptive_uncertainty -uncertainty-calibration "$OUT/fit-r$((k-1))/uncertainty-calibration.json" \
  -out "$OUT/fit-r$k"

# Decision partition, once per arm, with the frozen table
go run -tags=pcap ./cmd/tools/lidar-state-estimation-baseline -pcap-root "$LIDAR_PCAP_DIR" \
  -case embarcadero-folsom -uncertainty-report -out "$OUT/decision-shipped"
go run -tags=pcap ./cmd/tools/lidar-state-estimation-baseline -pcap-root "$LIDAR_PCAP_DIR" \
  -case embarcadero-folsom -uncertainty-report -experiment adaptive_uncertainty \
  -uncertainty-calibration "$OUT/fit-rN/uncertainty-calibration.json" -out "$OUT/decision-calibrated"
```

Each case's report is at `<out>/<case>/first/uncertainty_calibration.json`, with its samples beside
it; the pooled report is `<out>/uncertainty-calibration.json`. The tool checks every case's report
is byte-identical on its repeat run.

## Evidence so far

### Synthetic scenes

The `l5tracks` scene tests use a seeded generator with known noise along and across the line of
sight. Under the shipped, prior and a calibrated model:

- a 5-frame occlusion recovers to within 0.5 m of truth in under 3 frames (row 7's mechanism);
- a collision-grade stop (20 m/s²) and a limit turn (0.8 rad/s from 12 m/s) are the manoeuvres the
  shipped gate of 36 rejects at all, and every rejection they provoke closes as persistent; one
  displaced cluster across or along the body closes as transient;
- eligibility excludes two bodies inside each other's plausibility radius;
- the zero-velocity initialiser fragments any body above about 14.5 m/s at 10 Hz under the
  shipped R and gate, and a calibrated coefficient of 0.01 m² stops it acquiring 12 m/s. A fitted
  R that tightens the gate also governs birth, so rows 1–5 are necessary, not sufficient.

The `l8analytics` tests recover a known R per stratum and axis, show the range-decile row catching
errors that cancel in the aggregate, and, end to end through the real tracker, move the tangential
marginal from 2.1 to 1.0 when bearing noise dominates.

## Amendments

None.
