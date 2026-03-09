# LiDAR Classification Benchmarking and Optional Model Training

This document describes the optional future classification research lane for LiDAR tracks. It is intentionally downstream of the main pipeline: rule-based classification remains the deployed baseline until a candidate approach wins on reproducible benchmarks and stays explainable.

**Status:** Deferred / optional research lane
**Layers:** L6 Objects
**Related:** [Metrics-First Data Science Plan](platform-data-science-metrics-first-plan.md), [Backlog](../BACKLOG.md), [Track Labelling, Ground Truth Evaluation & Label-Aware Auto-Tuning](lidar-track-labeling-auto-aware-tuning-plan.md), [Classification Maths](../maths/classification-maths.md)

## Purpose

Evaluate whether future classification methods can improve on the current transparent rule-based classifier without introducing black boxes into the runtime. This work is not on the critical path for deployment.

## Guardrails

1. The live pipeline keeps the current rule-based classifier as the default and fallback path.
2. Candidate methods must use documented, exportable feature vectors.
3. Benchmark wins must be demonstrated on fixed replay packs and labelled runs.
4. Deployment is allowed only when metric gains are reproducible and the decision path remains explainable.
5. Opaque end-to-end models are out of scope for this lane.

## Dependencies

- Phase 4.0 track labelling must continue producing reference-quality labels.
- Analysis runs must persist params, metrics, and dataset provenance so comparisons can be replayed later.
- The rule-based baseline in `internal/lidar/l6objects/classification.go` remains the comparison target for every experiment.

## Benchmark Data Flow

```
Labelled Tracks → Feature Extraction → Benchmark Dataset
                                        ↓
                          Rule-Based Baseline vs Candidate
                                        ↓
                              Reproducible Scorecards
                                        ↓
                         Optional Deployment Proposal
```

## Feature Set

Candidate methods may only use documented track features such as:

- **Spatial features:** bounding box length/width/height averages, height p95 max, aspect ratios (XY, XZ)
- **Kinematic features:** average/raw-maximum speed, speed-profile descriptors, speed variance, max acceleration, heading variance
- **Temporal features:** duration, observation count, observations per second
- **Intensity features:** mean average, variance

These features must stay exportable so the same benchmark can be rerun outside the model training code.

## Candidate Approach

Start with interpretable, feature-based methods:

- threshold tables derived from labelled data,
- scorecard-driven refinements of the existing rule-based classifier,
- logistic regression, shallow trees, or similarly auditable models.

More complex models should be considered only if they still provide clear feature provenance and materially outperform the transparent baseline.

## Promotion Gate

Before any candidate reaches deployment review, it must:

1. beat the current rule-based baseline on the agreed scorecard,
2. avoid regressions in critical classes or noise handling,
3. be reproducible from versioned inputs and feature exports,
4. ship with enough metadata to audit why a class decision was made.

If any of those conditions fail, the result stays research-only.

## Expected Implementation Areas

- `tools/ml-training/features.py` — feature extraction and benchmark dataset generation
- `tools/ml-training/train_classifier.py` — offline experiments
- `internal/lidar/ml_classifier.go` — future runtime integration only if the promotion gate is passed
