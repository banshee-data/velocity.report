# Metrics-First Data Science Plan

This document defines velocity.report's repo-wide data science stance: the critical path stays transparent, tunable, and replayable, while future classification work remains optional and subordinate to reproducible scorecards.

**Status:** Proposed (March 2026)
**Layers:** Cross-cutting (L5 Tracks, L6 Objects, L8 Analytics)
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

## Evidence Package Contract

Every investigation that influences defaults, thresholds, or roadmap priority should also answer:

- what question was asked;
- what was observed, including whether the result is actually good enough for the intended use;
- which config values or thresholds were chosen, and why those values beat the compared alternatives;
- when validation and comparison were run, using exact dates;
- which source artifacts were used: PCAPs, `.vrlog` recordings, scene IDs, reference runs, benchmark baselines, and any exported comparison JSON;
- where those artifacts live: normal Git paths, Git LFS paths, or an external dataset/release location.

If the write-up cannot identify the input artifact set and comparison date, it is not strong enough to justify a runtime or documentation change.

## Current Evidence Inventory

The repo already contains the beginnings of a reproducible data-science corpus. As of March 10, 2026, the most important pieces are:

- `internal/lidar/perf/pcap/kirk0.pcapng` plus `scripts/validate-lfs-files.sh` — the main LFS-backed replay artifact currently called out for validation.
- `internal/lidar/perf/baseline/baseline-kirk0.json` and `internal/lidar/perf/baseline/baseline-kirk0-ci.json` — saved performance baselines for replay comparison.
- `data/explore/kirk0-lifecycle/` — parameter-permutation investigation outputs tied to `kirk0.pcapng`.
- `data/convergance-neighbour/` — neighbour-confirmation sweep analysis and findings.
- `docs/lidar/operations/parameter-comparison.md`, `docs/lidar/operations/config-param-tuning.md`, and `docs/lidar/operations/auto-tuning.md` — the current parameter-study and scoring guidance.
- `docs/plans/lidar-track-labeling-auto-aware-tuning-plan.md` — the reference-run, scene, and labelled-ground-truth workflow for replayable evaluation.
- `docs/data/vrlog-format.md` and `docs/data/vrlog-analysis.md` — the current `.vrlog` artifact contract and comparison/report format.

This inventory is not complete. One standing task for data-science work is to keep a clearer map of which investigations, scorecards, and artifact packs are canonical versus exploratory.

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

## Open Questions

As of March 10, 2026, the highest-value open questions are:

1. **Rotating bounding boxes in LiDAR replay**
   The 2026-02-22 OBB review landed Guard 3 plus fixes B, C, and G, but the geometry-coherent replacement is still only a proposal. Are the replay results now good enough across fixed packs, or do rotating boxes still justify the geometry-prior model?
2. **Radar + LiDAR fusion boundary**
   Should radar velocity stay a per-track association problem first, or should fusion be evaluated mainly at the future L7 scene/canonical-object layer where scene vectors and multi-sensor objects exist?
3. **Ground-plane maths**
   The current runtime still uses a height-band filter. Which static or replay captures actually demonstrate that tile-plane fitting and vector-scene region selection outperform the simpler baseline enough to justify extra runtime and operational complexity?
4. **Geometry priors plan review**
   How should OSM and community geometry priors be diffed, shifted, signed, reviewed, and exported (`.osc`, GeoJSON, synthetic aggregates) so that the workflow is useful without weakening provenance or manual review gates?
5. **Reflective and static-surface anchors**
   LiDAR intensity is available throughout the pipeline, but it is still unclear whether high-return signs can be turned into reliable static pose anchors, how far the threshold can be relaxed when signs are absent, whether walls, facades, or road geometry provide enough redundant fallback structure without causing false resets under occlusion, and whether a cached runtime signal back into lower layers is worth the architectural cost.
6. **Velocity coherence**
   Velocity-coherent extraction remains proposal/planning material on the main runtime path. What benchmark pack, scorecard, and acceptance gates would prove that it beats the current foreground-plus-DBSCAN baseline strongly enough to adopt?
7. **Config-value provenance**
   For current defaults and "optimised" settings, which values are backed by repeatable comparison results, which are still provisional, and when were those comparisons last rerun on fixed replay packs?
8. **Reference data coverage**
   Do current scenes, labels, and reference runs cover the classes, ranges, weather, and site types that matter, or are we overfitting to a small set of urban captures and a few familiar artifacts such as `kirk0`?
9. **L3/L4 settlement boundary**
   Should the future ground-plane system share one settlement core with L3, or is the added coupling riskier than keeping independent lifecycles?
10. **Bodies-in-motion and sparse-cluster linking**
    At what range, point count, and occlusion profile does the current CV tracker fragment too heavily, and what evidence would justify CA/CTRV/IMM models or L7 corridor-constrained linking?
11. **Performance-versus-accuracy tradeoff**
    Which proposed math upgrades improve scorecards enough to justify their CPU, memory, and latency cost on edge hardware, rather than only in offline replay?

These questions are intended to produce dated answers with artifact-backed comparisons, not discussion-only design notes.

## Near-Term Alignment

1. Rename the contributor persona from `ML & Data Scientist` to `Data Scientist`.
2. Treat labelled runs, scorecards, and reproducible tuning as the main data science work.
3. Keep optional model training explicitly off the critical path in roadmap and vision docs.
4. Align classification planning around transparent baselines and deployment gates.
