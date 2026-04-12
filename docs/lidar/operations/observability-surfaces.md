# Observability Surfaces

HINT metric observability plan: surfacing already-computed quality data alongside labelling controls, persisting per-track diagnostics, enriching HINT round history with detailed metric breakdowns.

## Source

- Plan: `docs/plans/hint-metric-observability-plan.md`
- Status: Proposed
- Layers: Cross-cutting (L5 Tracking, L6 Objects, L8 Analytics, Sweep, Web)

## Problem

HINT mode's ground truth scorer (8 weighted components, 3 acceptance criteria) is well-designed. The gap is **human observability**: the labeller assigns quality flags without seeing the objective measurements the system already computes.

The pipeline produces rich diagnostics at every stage — run statistics, per-track quality metrics, jitter accumulators, alignment scores, cluster density — but most of this data either stays in SQLite unexposed, or is transient and discarded.

Additionally, HINT round history stores only `BestScore` + `BestScoreComponents`, discarding the detailed `ComboResult` (36 fields) that would show _why_ parameters improved between rounds.

## Implementation Batches

### Batch A — Surface Persisted Data (small effort each)

**A1. Surface run statistics on web.** Expose the 12 fields already in `statistics_json`. Deserialise in API responses, render on runs detail page and HINT round history. Enable round-over-round comparison (e.g. noise ratio trending down).

**A2. Surface track quality metrics on web.** Include 6 per-track quality columns (`track_length_meters`, `track_duration_secs`, `occlusion_count`, `max_occlusion_frames`, `spatial_coverage`, `noise_point_ratio`) in track API responses. Render in track detail panel alongside label controls.

**A3. Persist and surface quality score.** Composite `QualityScore` (0–1) from `ComputeTrackQualityMetrics()` is computed but never persisted. Add `quality_score REAL` column to `lidar_tracks` and `lidar_run_tracks`. Display as suggested quality indicator during labelling, enable "sort by quality".

### Batch B — API Endpoints (medium effort)

**B1. Run comparison API endpoint.** Expose `compareParams()` and `computeTemporalIoU()` via `GET /api/lidar/runs/{runId}/compare/{otherRunId}`. Return parameter diff, per-track temporal IoU matrix, split/merge candidates. Render inter-round track correspondence in HINT dashboard.

**B2. Complete and surface noise coverage metrics.** Finish the `ComputeNoiseCoverageMetrics()` implementation (speed/size breakdown TODOs). Persist to `statistics_json` or new column. Render noise profile.

### Batch C — Pipeline Metrics in HINT Round History (moderate effort)

**C1. Aggregate foreground fraction per sweep combo.** `FrameMetrics.ForegroundFraction` is per-frame but transient. Add accumulator to `SampleResult`, compute mean+stddev in `ComboResult`.

**C2. Surface tracking state transitions.** Include `TracksCreated`/`TracksConfirmed` in `HINTRound` struct for round-over-round comparison.

**C3. Full combo result in HINT round history.** `ComboResult` has 36 fields but HINT stores only `BestScore` + `BestScoreComponents`. Attach full best-combo `ComboResult` to `HINTRound`.

### Batch D — Per-Track Diagnostics (medium effort)

**D1. Persist per-track jitter metrics.** Add `heading_jitter_deg REAL`, `speed_jitter_mps REAL` to `lidar_tracks` and `lidar_run_tracks`. Compute from `HeadingJitterSumSq`/`HeadingJitterCount` accumulators in L5.

**D2. Persist per-track alignment metrics.** Add `alignment_deg REAL`, `misalignment_rate REAL` columns. Write from `TrackAlignmentMetrics`. Display during HINT labelling.

### Batch E — Diagnostic Value, Low HINT Urgency

- E1: Surface cluster `cluster_density` and `aspect_ratio` on web
- E2: Populate `noise_points_count` in L4 pipeline
- E3: Surface speed-bucketed acceptance rates (`BucketMeans[]`) in sweep results frontend

## Cross-System Benefit Map

| Item                     | HINT Label | HINT Dash | Runs | Sweep | PDF | macOS | ML  |
|--------------------------|------------|-----------|------|-------|-----|-------|-----|
| A1 Run statistics        | ✅          | ✅         | ✅    | ✅     | ✅   | —     | —   |
| A2 Track quality on web  | ✅          | —         | ✅    | —     | —   | 🔶     | —   |
| A3 quality_score persist | ✅          | —         | ✅    | —     | —   | —     | ✅   |
| B1 Run comparison API    | ✅          | ✅         | ✅    | ✅     | —   | —     | —   |
| B2 Noise coverage        | ✅          | ✅         | ✅    | —     | —   | —     | —   |
| C1 Foreground fraction   | —          | ✅         | —    | ✅     | —   | —     | —   |
| C2 State transitions     | —          | ✅         | ✅    | ✅     | —   | —     | —   |
| C3 Full combo in history | —          | ✅         | —    | ✅     | —   | —     | —   |
| D1 Per-track jitter      | ✅          | —         | ✅    | —     | —   | 🔶     | —   |
| D2 Per-track alignment   | ✅          | —         | ✅    | —     | —   | 🔶     | —   |