# Percentile Aggregation Semantics

Active plan: [speed-percentile-aggregation-alignment-plan.md](../../plans/speed-percentile-aggregation-alignment-plan.md)

## Core Rule

**Percentiles are aggregate-only.** `p50/p85/p98` are reserved for
report/group/aggregation-period outputs across a filtered population.
Tracks use distinct non-percentile speed metrics.

## Decisions Settled

1. **Percentiles are aggregate-only.** No track-level public field reuses
   aggregate percentile labels.
2. **Canonical high-end percentile is `p98`, not `p95`.** `p95` is
   historical LiDAR schema residue; live aggregate/report code already uses
   `p98`.
3. **Raw maximum = `max`, not `peak`.** `peak` is reserved for a future
   outlier-filtered or context-aware top-speed measure.
4. **Track-level speed summaries use different terminology.** Working names:
   `typical_observed_speed_mps` and `reliable_peak_speed_mps`.
5. **Unmerged single-track speed-label proto/API work backed out.**
6. **No percentile-of-percentile rollups.** Aggregate summaries cannot be
   derived from prior bucket percentile outputs.

## Metric Inventory

| Metric | Live aggregate/report use | Track use             | Direction                  |
| ------ | ------------------------- | --------------------- | -------------------------- |
| `p50`  | Radar stats, PDF, charts  | None (aggregate only) | Keep for grouped/report    |
| `p85`  | Radar stats, PDF, charts  | None (aggregate only) | Keep for grouped/report    |
| `p98`  | Radar stats, PDF, charts  | None (aggregate only) | Keep as high-end aggregate |
| `max`  | `max_speed` in radar/TDL  | Raw maximum per track | Rename from `peak`         |

## Why `p95` Exists (Historical)

Originated from original LiDAR track schema (migration 000010) and early
track-analysis work. The codebase later standardised on `p98` (migration
000030). `p95` survives as historical residue and in non-speed domains
(`height_p95`, latency).

## Surfaces Already Aligned

- Proto: `max_speed_mps` (field 25), no percentile fields.
- REST API: `max_speed_mps`, no per-track percentiles.
- TypeScript types: No per-track percentile fields.
- Go structs: `MaxSpeedMps` on `TrackedObject` and `RunTrack`.
- Aggregate report: Population-level p50/p85/p98 over max speeds.
- PDF generator: P50/P85/P98 aggregate stats.
- Web charts: P50/P85/P98/Max aggregate display.

## Pending Work

### Phase 2 — Track Metric Redesign

- Define replacement public track metrics with non-percentile names.
- Specify outlier rejection and smoothing rules.
- Decide which stay public API vs internal-only.

### Phase 4 — Aggregate-Only Percentile Path

- Shared Go helper for dataset-level `p50/p85/p98`.
- Remove `derive_overall_from_granular()` / `derive_daily_from_granular()`,
  non-canonical fallbacks.
- Serve TDL `speed summary` from filtered transit max speeds.

## Acceptance Criteria

- No track-level public field reuses aggregate percentile labels.
- No raw maximum field publicly named `peak`.
- One documented percentile algorithm across radar, LiDAR, reporting.
- No report summary derived from prior percentile buckets.
