# Metrics-First Data Science Plan

This document defines velocity.report's repo-wide data science stance: the critical path stays transparent, tunable, and replayable, while future classification work remains optional and subordinate to reproducible scorecards.

**Status:** Proposed (March 2026)
**Related:** [Product Vision](../VISION.md), [Track Labelling, Ground Truth Evaluation & Label-Aware Auto-Tuning](lidar-track-labeling-auto-aware-tuning-plan.md), [Track Description Language and Description Interface](data-track-description-language-plan.md), [LiDAR Classification Benchmarking and Optional Model Training](lidar-ml-classifier-training-plan.md)

## Objective

Give contributors one consistent contract for data science work across the repo: what is critical path, what evidence is required for algorithm changes, and how future classification research is allowed to proceed without turning the runtime into a black box.

## Critical-Path Position

1. **No black boxes in the live pipeline.**
   Runtime perception, tracking, scoring, and reporting paths must remain spec-driven and inspectable.
2. **All important runtime behaviour must stay tunable.**
   Thresholds, weights, and decision rules belong in documented configs, code, or scorecards, not hidden model internals.
3. **Reproducibility is a feature, not a nice-to-have.**
   If a result cannot be replayed on the same input and produce the same scorecard, it is not ready to drive product decisions.
4. **Metrics beat intuition.**
   Tuning, tradeoffs, and regressions must be discussed through explicit benchmark metrics, not anecdotal screenshots or one-off scenes.

## What Data Science Means In This Repo

The near-term data science role is primarily:

- defining metrics and scorecards,
- curating labelled reference sets,
- analysing distributions, drift, and uncertainty,
- comparing parameter sets on fixed replay packs,
- proposing threshold and spec changes from evidence,
- building reportable aggregate statistics for users.

This is deliberately closer to actuarial analysis, experimental design, and quantitative QA than to model-centric ML engineering.

## Reproducibility Contract

Every benchmarkable experiment should record:

- the code revision or version string,
- the PCAP or run identifiers used,
- the full parameter bundle,
- the metric schema version,
- the label set or reference-run version,
- the output scorecard and comparison summary,
- enough metadata to replay the same experiment later.

Winning a comparison means improving the agreed scorecard on the same replay pack, not merely looking better in an isolated session.

## Core Workstreams

### 1. Scorecards and Specs

Define the metrics that matter for the product: detection coverage, fragmentation, false positives, velocity coverage, stability, calibration of labels, and traffic-engineering aggregates that appear in reports.

### 2. Replayable Benchmark Packs

Maintain fixed scenes, labelled runs, and comparison procedures so that algorithm changes can be measured under repeatable conditions.

### 3. Threshold and Parameter Studies

Prefer explicit, documented threshold studies and parameter sweeps over opaque fitting. When a threshold changes, the supporting metrics should be easy to inspect and explain.

### 4. User-Facing Metric Design

Shape the transit schema, report metrics, and comparison summaries so they match traffic-engineering language and can be audited by non-ML contributors.

### 5. Optional Classification Research

Future classification work is allowed, but only as an offline research lane that compares candidate approaches against the current transparent baseline on fixed benchmarks.

## Model Policy

If classification research proceeds, it must follow these rules:

1. Start with interpretable, feature-based methods.
2. Keep the feature vector documented and exportable.
3. Compare every candidate against the current rule-based baseline.
4. Require reproducible benchmark wins before considering deployment.
5. Preserve a transparent fallback path at runtime.

Opaque end-to-end models, hidden embeddings, or cloud-only training loops are out of scope for the critical path.

## Non-Goals

- Making ML or AI a required dependency for core deployment.
- Replacing exposed thresholds with hidden model behaviour.
- Shipping auto-tuning logic that cannot explain its score inputs.
- Optimising for leaderboard metrics that do not map to product-visible outcomes.

## Near-Term Alignment

1. Rename the contributor persona from `ML & Data Scientist` to `Data Scientist`.
2. Treat labelled runs, scorecards, and reproducible tuning as the main data science work.
3. Keep optional model training explicitly off the critical path in roadmap and vision docs.
4. Align classification planning around transparent baselines and deployment gates.
