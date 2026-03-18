# Experiment: L4 Clustering Parameter Sweep

- **Status:** Proposed
- **Layers:** L4 Perception

## Hypothesis

The two provisional/literature L4 clustering parameters —
`foreground_min_cluster_points` and `foreground_dbscan_eps` — may not
generalise from kirk0 to sites with different traffic densities, sensor
heights, or road geometries. Sweeping each across ≥ 3 sites will determine
whether the current defaults are robust or need adjustment.

## Background

L4 uses DBSCAN to cluster foreground points. `foreground_dbscan_eps` sets
the neighbourhood radius; `foreground_min_cluster_points` sets the minimum
cluster size. The current ε = 0.8 is literature-derived but adapted for
Hesai P40 point density. The minimum cluster size = 5 was tuned on kirk0
and is classified as provisional.

See [config/README.maths.md §3](../../../config/README.maths.md) for the
mathematical context.

## Method

### Test data

Same corpus as the L3 experiment: all available labelled PCAPs.

### Protocol

For each key, run `pcap-analyse` with ≥ 5 sweep values, holding all other
keys at production defaults.

#### Keys under test

| Config key                      | Default | Sweep range | Evidence level | Risk if wrong                             |
| ------------------------------- | ------- | ----------- | -------------- | ----------------------------------------- |
| `foreground_dbscan_eps`         | 0.8     | [0.3, 1.5]  | Literature     | Under/over-clustering at different ranges |
| `foreground_min_cluster_points` | 5       | [2, 8]      | Provisional    | Missed small objects (cyclists, peds)     |

### Metrics

**Gated metrics (available via GroundTruthEvaluator):**

| Metric              | Definition                                  | Threshold                 |
| ------------------- | ------------------------------------------- | ------------------------- |
| Confirmed track count | Confirmed tracks downstream               | No regression vs baseline |
| Objective function  | Composite score from `GroundTruthEvaluator` | Within 10% of optimal     |

**Future / manual diagnostics (cluster-level, not yet in evaluator):**

| Metric           | Definition                                                           | Notes                             |
| ---------------- | -------------------------------------------------------------------- | --------------------------------- |
| Cluster precision | Clusters that map 1:1 to ground-truth objects / total clusters      | Requires cluster-level ground truth |
| Per-class recall | Ground-truth objects with ≥ 1 matching cluster, per class            | Requires cluster-level ground truth |
| Split rate       | Ground-truth objects matched to > 1 cluster                          | Requires cluster-level ground truth |
| Merge rate       | Clusters containing points from > 1 ground-truth object             | Requires cluster-level ground truth |

### Controls

- Same as L3 experiment (byte-identical input, same `pcap-analyse` version, same hardware)
- L3 parameters held at production defaults

## Acceptance criteria

Same graduation protocol as the L3 experiment (see
[L3 sweep](l3-background-settling-sweep.md) § Acceptance criteria).

## Resources required

Same as L3 experiment.

## Timeline

Can run in parallel with the L3 sweep on the same corpus PCAPs.

## References

- [config/README.maths.md §3 — Clustering](../../../config/README.maths.md)
- [L3 background settling sweep](l3-background-settling-sweep.md)
- [Pipeline review Q7](../../maths/pipeline-review-open-questions.md) — evidence classification
- [Parameter tuning plan](../../../docs/plans/lidar-parameter-tuning-optimisation-plan.md) — sweep infrastructure
