# Experiment: Multi-Key Parameter Interaction Grid Search

- **Status:** Proposed (depends on per-layer sweep results)
- **Layers:** L3 Grid, L4 Perception, L5 Tracks

## Hypothesis

Per-key sweeps may miss interactions between parameters from different layers.
A reduced grid search over the top 3–4 most sensitive keys (identified by the
per-layer sweeps) will reveal whether:

1. Parameter interactions are small enough that per-key optima compose
   (i.e. the joint optimum ≈ product of per-key optima), or
2. Significant interactions exist that require joint optimisation.

## Background

The per-layer sweep experiments
([L3](l3-background-settling-sweep.md),
[L4](l4-clustering-parameter-sweep.md),
[L5](l5-tracking-noise-parameter-sweep.md))
each sweep one key at a time while holding others at defaults. This is
efficient but can miss interactions — for example, a smaller
`foreground_dbscan_eps` might need a compensating reduction in
`foreground_min_cluster_points` to maintain recall.

This experiment runs after the per-layer sweeps have identified which keys
have the steepest sensitivity curves (largest change in objective per unit
change in parameter). Only the top 3–4 keys are included to keep the
grid tractable.

## Method

### Prerequisites

Completed per-layer sweep results identifying:

- Top 3–4 most sensitive keys (steepest objective function slope)
- Stable sweep range for each (narrowed from the per-layer results)

### Protocol

1. Select the top 3–4 keys from per-layer sweep results.
2. For each key, choose 3 values: low end, current default, high end
   of the stable range.
3. Run `pcap-analyse` for every combination (3⁴ = 81 runs max for 4
   keys, 3³ = 27 runs for 3 keys) on each corpus PCAP.
4. Record the full objective function for each combination and site.
5. Analyse:
   - Are interaction terms significant (> 5% of the main effect)?
   - Does the joint optimum differ from the per-key optimum?
   - Is the interaction consistent across sites?

### Metrics

Same as the per-layer sweeps (confirmed track count, fragmentation, speed
RMSE, objective function), plus:

| Metric                | Definition                                                    |
| --------------------- | ------------------------------------------------------------- |
| Interaction magnitude | Difference between joint optimum and per-key-composed optimum |
| Consistency           | Whether the interaction direction is the same across sites    |

### Controls

- Same PCAP corpus as per-layer sweeps (byte-identical input)
- `pcap-analyse` run with deterministic replay settings (e.g. `--deterministic` flag and a fixed config shared across all runs)
- All keys not under test held at production defaults

## Acceptance criteria

1. If interaction terms are < 5% of main effects on all sites: per-key
   optima are sufficient; no joint tuning needed.
2. If interactions are > 5%: document the interaction, recommend a joint
   default, and update `tuning.defaults.json` accordingly.
3. Results recorded in `data/explore/` with full sweep data.

## Resources required

- Completed per-layer sweep results
- `pcap-analyse` for PCAP replay with parameter overrides
- `GroundTruthEvaluator` for scored quality comparison
- Compute budget: up to 81 × 5 sites = 405 runs (each ≈ 5 min on Pi 4
  ≈ 34 hours; parallelisable on faster hardware)

## Timeline

Depends on completion of per-layer sweeps. Cannot start until at least
3 of the 4 per-layer experiments have results.

## References

- [L3 background settling sweep](l3-background-settling-sweep.md)
- [L4 clustering parameter sweep](l4-clustering-parameter-sweep.md)
- [L5 tracking noise parameter sweep](l5-tracking-noise-parameter-sweep.md)
- [Parameter tuning plan](../../../docs/plans/lidar-parameter-tuning-optimisation-plan.md) — sweep infrastructure
- [Pipeline review Q7](../../maths/pipeline-review-open-questions.md) — evidence classification
