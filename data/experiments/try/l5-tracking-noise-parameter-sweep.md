# Experiment: L5 Tracking Noise Parameter Sweep

- **Status:** Proposed
- **Layers:** L5 Tracks

## Hypothesis

The three provisional Kalman filter noise parameters — `process_noise_pos`,
`process_noise_vel`, and `measurement_noise` — control the trade-off between
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

See [config/README.maths.md §4](../../../config/README.maths.md) for the
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

| Metric             | Definition                                              | Threshold                 |
| ------------------ | ------------------------------------------------------- | ------------------------- |
| Track completeness | Fraction of GT tracks matched with temporal IoU ≥ 0.5   | No regression vs baseline |
| Fragmentation rate | Pipeline tracks per ground-truth track                   | < 1.2 for vehicles        |
| Objective function | Composite score from `GroundTruthEvaluator`              | Within 10% of optimal     |

**Future / manual diagnostics (not yet in evaluator):**

| Metric       | Definition                                              | Notes                              |
| ------------ | ------------------------------------------------------- | ---------------------------------- |
| Track jitter | RMS position deviation from smoothed trajectory         | Requires trajectory ground truth   |
| Speed RMSE   | RMS error of per-track speed vs ground truth            | Requires speed ground truth labels |

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

- [config/README.maths.md §4 — Tracking](../../../config/README.maths.md)
- [L3 background settling sweep](l3-background-settling-sweep.md)
- [Pipeline review Q7](../../maths/pipeline-review-open-questions.md) — evidence classification
- [Parameter tuning plan](../../../docs/plans/lidar-parameter-tuning-optimisation-plan.md) — sweep infrastructure
