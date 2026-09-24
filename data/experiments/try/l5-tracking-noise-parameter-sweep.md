# Experiment: L5 tracking noise parameter sweep

- **Status:** Proposed for the ground-truth acceptance criteria below. A preliminary,
  non-substituting pass ran 2026-09-17 using `tune sweep`'s own alignment metric — see
  "Preliminary pass" below. It does not satisfy this experiment's acceptance criteria and must not
  be read as though it did. Scheduled as **Batch 3** of the
  [2026-09 parameter experiment campaign](../../../docs/plans/lidar-parameter-experiment-campaign-2026-09.md#batch-3--l5-noise-sweep-ground-truth-scored-kirk1-built-blocked-on-a-port-conflict):
  a real `GroundTruthEvaluator`-scored rerun on kirk0 (the only site with any
  labelled reference tracks), pending a small standalone CLI wrapper around
  `adapters.EvaluateGroundTruth` — that function is fully implemented and
  tested, just not reachable outside the live HINT HTTP flow today.
- **Layers:** L5 Tracks

## Hypothesis

The three provisional Kalman filter noise parameters: `process_noise_pos`,
`process_noise_vel`, and `measurement_noise`: control the trade-off between
track smoothness and responsiveness. Values tuned on kirk0 may not generalise
to sites with different speed distributions, turning vehicles, or sensor noise
characteristics. Sweeping each will quantify sensitivity and identify robust
defaults.

## Background

The L5 CV Kalman tracker uses three noise parameters:

- `process_noise_pos` (0.05): position process noise, controls how much the
  filter trusts the motion model vs measurements for position.
- `process_noise_vel` (0.2): velocity process noise, controls velocity
  responsiveness to acceleration.
- `measurement_noise` (0.05): observation noise, should match actual sensor
  measurement uncertainty.

`measurement_noise` has a theoretical derivation path (sensor spec) but was
tuned empirically on kirk0. All three are classified as provisional.

See [config/CONFIG.md §4](../../../config/CONFIG.md#config-to-maths-cross-reference) for the
mathematical context.

## Method

### Test data

Same corpus as the L3/L4 experiments.

### Protocol

For each key, run `pcap-analyse` with ≥ 5 sweep values, holding all other
keys at production defaults.

#### Keys under test

| Config key          | Default | Sweep range  | Risk if wrong                                            |
| ------------------- | ------- | ------------ | -------------------------------------------------------- |
| `process_noise_pos` | 0.05    | [0.01, 0.5]  | Track instability or sluggish response                   |
| `process_noise_vel` | 0.2     | [0.05, 2.0]  | Velocity estimate lag or overshoot                       |
| `measurement_noise` | 0.05    | [0.01, 0.25] | Incorrect Kalman gain, over/under-weighting measurements |

### Additional validation for `measurement_noise`

`measurement_noise` should ideally be derived from the sensor specification
rather than tuned empirically. As a secondary check, compute the innovation
sequence (measurement minus predicted measurement) variance on a set of
straight-line, constant-speed tracks. The innovation variance should match
the configured `measurement_noise` within a factor of 2. If it does not,
the configured value is mismatched to the actual sensor noise.

### Metrics

**Gated metrics (available via GroundTruthEvaluator):**

| Metric             | Definition                                            | Threshold                 |
| ------------------ | ----------------------------------------------------- | ------------------------- |
| Track completeness | Fraction of GT tracks matched with temporal IoU ≥ 0.5 | No regression vs baseline |
| Fragmentation rate | Pipeline tracks per ground-truth track                | < 1.2 for vehicles        |
| Objective function | Composite score from `GroundTruthEvaluator`           | Within 10% of optimal     |

**Future / manual diagnostics (not yet in evaluator):**

| Metric       | Definition                                      | Notes                              |
| ------------ | ----------------------------------------------- | ---------------------------------- |
| Track jitter | RMS position deviation from smoothed trajectory | Requires trajectory ground truth   |
| Speed RMSE   | RMS error of per-track speed vs ground truth    | Requires speed ground truth labels |

### Controls

- Same as L3/L4 experiments
- L3 and L4 parameters held at production defaults

## Acceptance criteria

Same graduation protocol as the L3 experiment (see
[L3 sweep](l3-background-settling-sweep.md) § Acceptance criteria).

## Resources required

Same as L3 experiment, plus:

- Innovation-sequence analysis tooling for measurement noise validation
- Ground-truth speed labels (from labelled track trajectories)

## Timeline

Can run in parallel with L3 and L4 sweeps.

## References

- [config/CONFIG.md §4: tracking](../../../config/CONFIG.md#config-to-maths-cross-reference)
- [L3 background settling sweep](l3-background-settling-sweep.md)
- [Pipeline review Q7](../../maths/pipeline-review-open-questions.md): evidence classification
- [Parameter tuning plan](../../../docs/plans/lidar-parameter-tuning-optimisation-plan.md): sweep infrastructure

## Preliminary pass, 2026-09-17: `tune sweep -mode tracking` on kirk0

This is a lightweight run against the sweep tool that already exists
([docs/lidar/operations/sweep-tool.md](../../../docs/lidar/operations/sweep-tool.md)), not the
GroundTruthEvaluator-based protocol this experiment specifies above. It measures the sweep tool's
own metric — mean angular error between the Kalman velocity vector and displacement heading — not
track completeness, fragmentation, or the composite objective. **It does not satisfy this
experiment's acceptance criteria and does not license a default change.**

### What ran

`gating_distance_squared` × 7 (16–64, step 8) × `process_noise_pos` × 5 (0.05–0.45, step 0.1) ×
`measurement_noise` × 5 (0.1–0.5, step 0.1) = 175 combinations, live against a running dev server
via its monitor API, replaying `kirk0.pcapng` for each combination (8 samples/combination, 1 s
apart, 4 s settle). Raw results: `l5-tracking-noise-parameter-sweep-kirk0-20260917.csv`, archived
with the campaign (see the
[campaign plan](../../../docs/plans/lidar-parameter-experiment-campaign-2026-09.md#archived-evidence)).

### What it shows

The clearest, most consistent pattern is `measurement_noise`: at the shipped default (0.05,
below this grid's own floor of 0.1) and at 0.1, `mean_alignment_deg` runs 5.8° to 14.9° with
misalignment ratios of 2–9% across the `gating`/`process_noise_pos` combinations tested; at 0.4–0.5
it drops to 3–4° with misalignment at or near 0% in almost every combination. `gating_distance_sq`
and `process_noise_pos` show no comparably consistent trend across the grid: the ten best-scoring
combinations span gating values from 16 to 64 and `process_noise_pos` from 0.05 to 0.35, with
`measurement_noise` at 0.5 or 0.4 in eight of the ten.

### Why this is not a recommendation to raise `measurement_noise`

This is exactly the shape of pathology RC6 already named elsewhere in this repository
([lidar-heading-coherence-sprint-plan.md §2](../../../docs/plans/lidar-heading-coherence-sprint-plan.md)):
"a locked heading has near-zero jitter" — an alignment metric computed against the tracker's own
smoothed estimate rewards smoothing, not accuracy. A higher `measurement_noise` tells the Kalman
filter to trust its own constant-velocity prediction over each new noisy measurement; that
mechanically reduces disagreement between the published velocity heading and the displacement
heading regardless of whether the track is actually following the vehicle better, and it would be
expected to blunt responsiveness to real turns and accelerations in a way this metric cannot see.
Distinguishing "genuinely better tracking" from "smoother output measured against itself" is
exactly what this experiment's own GroundTruthEvaluator-based metrics (track completeness,
fragmentation rate, the composite objective) exist to do, and this pass used none of them.

Also unaddressed: single site (kirk0 only, the same one-capture risk flagged throughout this
plan's other A/B work), and small per-combination samples (35–60 alignment samples per
combination) — enough to see a pattern this size, not enough to treat any single combination's
figure as precise.

### What this closes and what remains

Closes nothing against this experiment's acceptance criteria. What it does establish: the existing
`tune sweep -mode tracking` infrastructure works end-to-end against a live server and a real
capture, at a pace (175 combinations in ~41 minutes) that makes the _real_ experiment — the same
grid, scored by `GroundTruthEvaluator` against labelled tracks, across more than one site —
tractable to actually run rather than merely propose. That remains the open work; ground-truth
speed/trajectory labels and multi-site coverage are still required before any default changes.
