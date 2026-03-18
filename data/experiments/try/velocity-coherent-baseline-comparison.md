# Experiment: Velocity-Coherent vs Background-Subtraction Baseline Comparison

- **Status:** Proposed
- **Layers:** L3 Grid, L4 Perception, L5 Tracks

## Hypothesis

The velocity-coherent foreground extraction approach produces measurably
better track quality than the current background-subtraction + DBSCAN
baseline, specifically:

- Track completeness improves by ≥ 10% (absolute)
- Fragmentation rate drops to < 1.2 tracks per ground-truth vehicle
- Speed RMSE does not regress by more than 5%

## Background

The current pipeline uses background-subtraction (L3 EMA range baseline)
to classify points as foreground, followed by DBSCAN clustering (L4) and
CV Kalman tracking (L5). The velocity-coherent proposal
([data/maths/proposals/20260220-velocity-coherent-foreground-extraction.md](../../maths/proposals/20260220-velocity-coherent-foreground-extraction.md))
hypothesises that motion-aware point association improves sparse object
recall by 20–40% and reduces fragmentation by 10–25%.

These claims need empirical evidence before the baseline is replaced.

## Method

### Test data

Run on all available labelled PCAPs (initially kirk0; expand to the
five-site corpus as captures become available). Each PCAP must have ≥ 20
manually labelled ground-truth tracks covering car, truck, cyclist, and
pedestrian classes.

### Protocol

1. Run the baseline extractor (background-subtraction + DBSCAN) on each
   PCAP with the production default config (`config/tuning.defaults.json`).
2. Run the velocity-coherent extractor on the same PCAPs with the same
   downstream parameters (same DBSCAN ε, same tracker config).
3. Record both runs as analysis runs (via `pcap-analyse`) and label
   reference tracks with `user_label` per class.
4. Evaluate each candidate run against the scene's labelled reference
   run using `GroundTruthEvaluator` (Hungarian-matched temporal IoU).

### Metrics

**Gated metrics (available via GroundTruthEvaluator):**

| Metric             | Definition                                                                                 | Threshold                                 |
| ------------------ | ------------------------------------------------------------------------------------------ | ----------------------------------------- |
| Track completeness | Fraction of reference tracks matched with temporal IoU ≥ 0.5                               | ≥ 10% improvement (absolute)              |
| Fragmentation rate | Pipeline tracks per reference track (lower is better)                                      | < 1.2 vehicles, < 1.5 pedestrians         |
| Frame throughput   | Frames processed per second on reference hardware (`pcap-analyse -benchmark`)              | ≤ 20% regression vs baseline              |

**Future / manual diagnostics (not yet implemented in the evaluator):**

| Metric     | Definition                                              | Notes                           |
| ---------- | ------------------------------------------------------- | ------------------------------- |
| Speed RMSE | RMS error of per-track speed vs ground truth            | Requires speed ground truth labels |

### Controls

- Same PCAP file for both runs (byte-identical input)
- Same downstream config (DBSCAN ε, tracker parameters, classification)
- Same hardware for timing comparisons
- Deterministic mode enabled for reproducibility

## Acceptance criteria

The velocity-coherent extractor **passes** if:

1. Track completeness improves by ≥ 10% on at least 3 of 5 sites
2. Fragmentation rate meets threshold on at least 4 of 5 sites
3. Speed RMSE does not regress on any site
4. Frame throughput stays within budget on Raspberry Pi 4

If the extractor fails any criterion, document the failure mode and
determine whether parameter tuning or algorithm changes can address it
before retesting.

## Resources required

- Labelled ground-truth tracks for kirk0 (existing) and additional sites
- Velocity-coherent extractor implementation (at least L3 track-assisted
  promotion and L4 two-stage clustering)
- Access to Raspberry Pi 4 for throughput measurements
- `pcap-analyse` benchmark mode for throughput measurement
- `GroundTruthEvaluator` for scored quality comparison against labelled reference runs

## Timeline

This experiment blocks any decision to replace the baseline extractor.
It should be executed after the velocity-coherent extractor reaches a
testable state (Phases 1–5 of the implementation plan).

## References

- [Velocity-coherent foreground extraction proposal](../../maths/proposals/20260220-velocity-coherent-foreground-extraction.md)
- [Implementation plan](../../../docs/plans/lidar-velocity-coherent-foreground-extraction-plan.md)
- [Pipeline review Q6](../../maths/pipeline-review-open-questions.md)
