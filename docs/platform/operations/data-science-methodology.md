# Data Science Methodology

Active plan: [platform-data-science-metrics-first-plan.md](../../plans/platform-data-science-metrics-first-plan.md)

Metrics-first data science methodology for velocity.report: reproducible benchmarks and transparent scorecards over opaque model-driven approaches.

## Critical-Path Position

1. **No black boxes in the live pipeline.** Runtime perception, tracking,
   scoring, and reporting paths must remain spec-driven and inspectable.
2. **All important runtime behaviour must stay tunable.** Thresholds, weights,
   and decision rules belong in documented configs, code, or scorecards.
3. **Reproducibility is a feature.** If a result cannot be replayed on the
   same input and produce the same scorecard, it is not ready to drive product
   decisions.
4. **Metrics beat intuition.** Tuning, tradeoffs, and regressions must be
   discussed through explicit benchmark metrics.

## What Data Science Means In This Repo

The near-term data science role is primarily:

- defining metrics and scorecards
- curating labelled reference sets
- analysing distributions, drift, and uncertainty
- comparing parameter sets on fixed replay packs
- proposing threshold and spec changes from evidence
- building reportable aggregate statistics for users

This is deliberately closer to actuarial analysis, experimental design, and
quantitative QA than to model-centric ML engineering.

## Reproducibility Contract

Every benchmarkable experiment must record:

- code revision or version string
- PCAP or run identifiers used
- full parameter bundle
- metric schema version
- label set or reference-run version
- output scorecard and comparison summary
- enough metadata to replay the same experiment later

Winning means improving the agreed scorecard on the same replay pack.

## Evidence Package Contract

Every investigation that influences defaults, thresholds, or roadmap priority
must answer:

- what question was asked
- what was observed (and whether the result is good enough)
- which config values or thresholds were chosen, and why
- when validation and comparison were run (exact dates)
- which source artefacts were used (PCAPs, `.vrlog`, scene IDs, baselines)
- where those artefacts live (Git paths, LFS, external releases)

If the write-up cannot identify the input artefact set and comparison date,
it is not strong enough to justify a runtime or documentation change.

## Core Workstreams

1. **Scorecards and Specs** — detection coverage, fragmentation, false
   positives, velocity coverage, stability, calibration of labels, and
   traffic-engineering aggregates.
2. **Replayable Benchmark Packs** — fixed scenes, labelled runs, and
   comparison procedures for repeatable measurement.
3. **Threshold and Parameter Studies** — explicit, documented studies and
   sweeps over opaque fitting.
4. **User-Facing Metric Design** — transit schema, report metrics, and
   comparison summaries matching traffic-engineering language.
5. **Optional Classification Research** — offline only, comparing against the
   current transparent baseline on fixed benchmarks.

## Model Policy

If classification research proceeds:

1. Start with interpretable, feature-based methods.
2. Keep the feature vector documented and exportable.
3. Compare every candidate against the current rule-based baseline.
4. Require reproducible benchmark wins before considering deployment.
5. Preserve a transparent fallback path at runtime.

Opaque end-to-end models, hidden embeddings, or cloud-only training loops are
out of scope for the critical path.

## Current Evidence Inventory

- `internal/lidar/perf/pcap/kirk0.pcapng` + `scripts/validate-lfs-files.sh`
- `internal/lidar/perf/baseline/baseline-kirk0.json` and `baseline-kirk0-ci.json`
- `data/explore/kirk0-lifecycle/` — parameter-permutation investigation
- `data/explore/convergence-neighbour/` — neighbour-confirmation sweep
- `docs/lidar/operations/parameter-comparison.md`, `config-param-tuning.md`,
  `auto-tuning.md` — parameter study guidance
- `data/structures/VRLOG_FORMAT.md` and
  `data/explore/vrlog-analysis-runs/VRLOG_ANALYSIS.md` — recording contract

## Non-Goals

- Making ML or AI a required dependency for core deployment.
- Replacing exposed thresholds with hidden model behaviour.
- Shipping auto-tuning logic that cannot explain its score inputs.
- Optimising for leaderboard metrics that do not map to product-visible
  outcomes.
