# Experiment: L3 background settling parameter sweep

- **Status:** Batch 1 of the
  [2026-09 parameter experiment campaign](../../../docs/plans/lidar-parameter-experiment-campaign-2026-09.md)
  — prepared, broad sweep ready to run across all 24 S2 corpus sites. See
  that doc for why `pcap-analyse` (named below) no longer exists and has
  been replaced with `settling-eval`, and why the metrics below differ from
  the original GroundTruthEvaluator-gated version of this doc.
- **Layers:** L3 Grid

## Hypothesis

The four provisional L3 background settling parameters: `closeness_multiplier`,
`safety_margin_metres`, `noise_relative`, and `neighbour_confirmation_count` —
were tuned on kirk0 only. Sweeping each across ≥ 3 sites will either confirm
the current defaults are robust or reveal site-specific sensitivity that
requires per-scene adaptation or different default values.

## Background

The L3 background settling layer classifies each range-bin as foreground or
background using an EMA baseline with configurable thresholds. The current
defaults were set during initial kirk0 development. The pipeline review (Q7)
classified these keys as "provisional" because they lack multi-site evidence.

See [config/CONFIG.md §1](../../../config/CONFIG.md#config-to-maths-cross-reference) for the
mathematical rationale behind each key.

## Method

### Test data

The [24-site S2 corpus](../../../docs/lidar/operations/state-estimation-phase01-corpus-baseline.md)
(312,315 frames, deterministic replay verified, zero labelled tracks). No
labelled reference run is required for the protocol below — see "Metrics".

### Protocol

`pcap-analyse` no longer exists in this codebase (removed; only a stale
gitignored binary remains). The current offline, ground-truth-free equivalent
for these keys is `settling-eval`
([internal/cmd/lidar/settling.go](../../../internal/cmd/lidar/settling.go)),
which replays one PCAP through a standalone `BackgroundManager` and reports
convergence computed from the grid's own measured state — no live server, no
labelled tracks. For each key listed below, run `settling-eval` on each
corpus site with ≥ 5 sweep values, holding all other keys at production
defaults ([config/tuning.defaults.json](../../../config/tuning.defaults.json)).
The driver, `l3-settling-sweep/run_sweep.py` (archived with the campaign's results), did
this across all 24 sites; see
[the campaign plan, Batch 1](../../../docs/plans/lidar-parameter-experiment-campaign-2026-09.md#batch-1--l3-background-settling-broad-sweep-ready-now)
for the exact command and current results.

#### Keys under test

| Config key                     | Default | Sweep range   | Risk if wrong                                 |
| ------------------------------ | ------- | ------------- | --------------------------------------------- |
| `closeness_multiplier`         | 3.0     | [1.5, 5.0]    | False foreground/background at range extremes |
| `safety_margin_metres`         | 0.15    | [0.05, 0.30]  | Ground leakage into foreground                |
| `noise_relative`               | 0.02    | [0.005, 0.05] | Incorrect range-dependent thresholds          |
| `neighbour_confirmation_count` | 3       | [1, 5]        | Missed foreground at scene edges              |

### Metrics

**Available today, non-circular (via `settling-eval`, computed from the
background grid's own measured per-cell state — not a downstream track
comparison):**

| Metric                     | Definition                                                                                    | Threshold (from `tuning.defaults.json`)         |
| -------------------------- | --------------------------------------------------------------------------------------------- | ----------------------------------------------- |
| Recommended settling frame | Frame index at which coverage/spread/stability/confidence all cross their settling thresholds | Should not regress vs current per-site baseline |
| Coverage rate              | Fraction of grid cells classified background                                                  | ≥ `settling_min_coverage` (0.8)                 |
| Spread-delta rate          | Rate of change of per-cell measured spread                                                    | ≤ `settling_max_spread_delta` (0.001)           |
| Region stability           | Fraction of regions with stable classification                                                | ≥ `settling_min_region_stability` (0.95)        |
| Mean confidence            | Mean per-cell confidence (Welford sample count-derived)                                       | ≥ `settling_min_confidence` (10)                |

**Gated on labelled reference tracks that don't exist for this corpus (see
[campaign plan](../../../docs/plans/lidar-parameter-experiment-campaign-2026-09.md)):
confirmed track count, `GroundTruthEvaluator` composite score. Deferred, not
part of Batch 1.**

**Not implemented in the evaluator at all (do not cite as available):**
`Fragmentation` is hardcoded to `0.0` in
[ground_truth.go](../../../internal/lidar/adapters/ground_truth.go) — "future
enhancement," not a real signal today.

**Future / manual diagnostics (point-level, not yet in evaluator):**

| Metric                | Definition                                                  | Notes                             |
| --------------------- | ----------------------------------------------------------- | --------------------------------- |
| Foreground precision  | True foreground points / all foreground-classified points   | Requires point-level ground truth |
| Foreground recall     | True foreground points / all ground-truth foreground points | Requires point-level ground truth |
| Ground false-pos rate | Ground points incorrectly classified as foreground / total  | Requires point-level ground truth |

### Controls

- Same PCAP file for all sweep values (byte-identical input)
- Same downstream config (L4, L5 parameters held at defaults)
- Same hardware for timing comparisons
- Software versions and configs recorded for reproducibility

## Acceptance criteria

A key graduates from "provisional" to "empirical" when:

1. Swept across ≥ 3 sites (target: 5)
2. Optimal value is consistent (within 10% of the chosen default) across
   all sites
3. Objective function sensitivity is documented (slope near the default)
4. Results are recorded in a dated entry under [data/explore/](../../explore)

If any key shows ≥ 15% variation in optimal value across sites, escalate to
a site-adaptive approach rather than a single default.

## Resources required

- `settling-eval` for offline PCAP replay with parameter overrides
- `GroundTruthEvaluator` for scored quality comparison against labelled
  reference runs, once available for this corpus (see Metrics above)
- Access to Raspberry Pi 4 for throughput measurements (`lidar-bench`, the
  current replacement for `pcap-analyse -benchmark`)

## Timeline

Batch 1 of the
[2026-09 parameter experiment campaign](../../../docs/plans/lidar-parameter-experiment-campaign-2026-09.md)
runs against the 24-site corpus, which already exists — no longer blocked on
corpus availability.

## References

- [config/CONFIG.md §1: background settling](../../../config/CONFIG.md#config-to-maths-cross-reference)
- [Pipeline review Q7](../../maths/pipeline-review-open-questions.md): evidence classification
- [Parameter tuning plan](../../../docs/plans/lidar-parameter-tuning-optimisation-plan.md): sweep infrastructure
- [2026-09 parameter experiment campaign](../../../docs/plans/lidar-parameter-experiment-campaign-2026-09.md): execution plan and current results
