# Experiment: L3 Background Settling Parameter Sweep

- **Status:** Proposed
- **Layers:** L3 Grid

## Hypothesis

The four provisional L3 background settling parameters — `closeness_multiplier`,
`safety_margin_meters`, `noise_relative`, and `neighbor_confirmation_count` —
were tuned on kirk0 only. Sweeping each across ≥ 3 sites will either confirm
the current defaults are robust or reveal site-specific sensitivity that
requires per-scene adaptation or different default values.

## Background

The L3 background settling layer classifies each range-bin as foreground or
background using an EMA baseline with configurable thresholds. The current
defaults were set during initial kirk0 development. The pipeline review (Q7)
classified these keys as "provisional" because they lack multi-site evidence.

See [config/README.maths.md §1](../../../config/README.maths.md) for the
mathematical rationale behind each key.

## Method

### Test data

Run on all available labelled PCAPs (initially kirk0; expand to the five-site
corpus as captures become available per the
[test corpus plan](../../../docs/plans/lidar-test-corpus-plan.md)).
Each PCAP must have a labelled reference analysis run (tracks annotated
with `user_label` per class via the track-labelling UI).

### Protocol

For each key listed below, run `pcap-analyse` on each corpus PCAP with ≥ 5
sweep values, holding all other keys at production defaults
(`config/tuning.defaults.json`).

#### Keys under test

| Config key                    | Default | Sweep range   | Risk if wrong                                 |
| ----------------------------- | ------- | ------------- | --------------------------------------------- |
| `closeness_multiplier`        | 3.0     | [1.5, 5.0]    | False foreground/background at range extremes |
| `safety_margin_meters`        | 0.15    | [0.05, 0.30]  | Ground leakage into foreground                |
| `noise_relative`              | 0.02    | [0.005, 0.05] | Incorrect range-dependent thresholds          |
| `neighbor_confirmation_count` | 3       | [1, 5]        | Missed foreground at scene edges              |

### Metrics

**Gated metrics (available via GroundTruthEvaluator):**

| Metric              | Definition                                                                                     | Threshold                 |
| ------------------- | ---------------------------------------------------------------------------------------------- | ------------------------- |
| Confirmed track count | Number of confirmed tracks downstream                                                        | No regression vs baseline |
| Objective function  | Composite score from `GroundTruthEvaluator`                                                    | Within 10% of optimal     |

**Future / manual diagnostics (point-level, not yet in evaluator):**

| Metric                | Definition                                                       | Notes                           |
| --------------------- | ---------------------------------------------------------------- | ------------------------------- |
| Foreground precision  | True foreground points / all foreground-classified points        | Requires point-level ground truth |
| Foreground recall     | True foreground points / all ground-truth foreground points      | Requires point-level ground truth |
| Ground false-pos rate | Ground points incorrectly classified as foreground / total       | Requires point-level ground truth |

### Controls

- Same PCAP file for all sweep values (byte-identical input)
- Same downstream config (L4, L5 parameters held at defaults)
- Same hardware for timing comparisons
- Deterministic mode enabled for reproducibility

## Acceptance criteria

A key graduates from "provisional" to "empirical" when:

1. Swept across ≥ 3 sites (target: 5)
2. Optimal value is consistent (within 10% of the chosen default) across
   all sites
3. Objective function sensitivity is documented (slope near the default)
4. Results are recorded in a dated entry under `data/explore/`

If any key shows ≥ 15% variation in optimal value across sites, escalate to
a site-adaptive approach rather than a single default.

## Resources required

- `pcap-analyse` for PCAP replay with parameter overrides
- `GroundTruthEvaluator` for scored quality comparison against labelled
  reference runs
- Access to Raspberry Pi 4 for throughput measurements (`pcap-analyse -benchmark`)

## Timeline

Depends on test corpus availability (≥ 3 labelled PCAPs). Can begin with
kirk0-only as a dry run to validate the methodology.

## References

- [config/README.maths.md §1 — Background Settling](../../../config/README.maths.md)
- [Pipeline review Q7](../../maths/pipeline-review-open-questions.md) — evidence classification
- [Parameter tuning plan](../../../docs/plans/lidar-parameter-tuning-optimisation-plan.md) — sweep infrastructure
- [Test corpus plan](../../../docs/plans/lidar-test-corpus-plan.md) — five-site PCAP corpus
