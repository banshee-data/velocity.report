# HINT Metric Observability Plan

- **Status:** Proposed
- **Layers:** Cross-cutting (L5 Tracking, L6 Objects, L8 Analytics, Sweep, Web)
- **Related:**
  - [HINT sweep mode](../lidar/operations/hint-sweep-mode.md)
  - [Unpopulated data structures remediation](unpopulated-data-structures-remediation-plan.md)
  - [Backend surface matrix](../../data/structures/BACKEND_SURFACE_MATRIX.md)
  - [Clustering observability](lidar-clustering-observability-and-benchmark-plan.md)

---

## Problem

HINT mode's ground truth scorer (8 weighted components, 3 acceptance
criteria) is well-designed. The gap is **human observability**: the labeller
assigns quality flags (`perfect`, `good`, `truncated`, `noisy_velocity`)
that directly feed scoring weights, but does so without seeing the objective
measurements the system already computes.

The pipeline produces rich diagnostics at every stage — run statistics,
per-track quality metrics, jitter accumulators, alignment scores, cluster
density — but most of this data either stays in SQLite unexposed, or is
transient and discarded after each run. The labeller works blind.

Additionally, HINT round history stores only `BestScore` (single float) +
`BestScoreComponents`, discarding the detailed `ComboResult` (36 fields)
that would show _why_ parameters improved between rounds.

## Goals

1. Surface already-persisted quality data alongside labelling controls
2. Persist per-track diagnostics that inform quality flag assignment
3. Enrich HINT round history with detailed metric breakdowns
4. Enable round-over-round quality comparison in the HINT dashboard

## Non-Goals

- Changing the ground truth scoring formula or acceptance criteria
- ML feature vector export or training data curation (separate project)
- Per-track speed percentile exposure (design debt — removal planned)

---

## Implementation Batches

### Batch A — Surface persisted data (small effort each)

Many of these metrics are already computed and, where they are persisted to
SQLite, the work in this batch is API deserialisation and web rendering. For
metrics that are computed but not yet persisted, add the minimal SQLite
fields first, then expose them via the same API/web paths.

#### A1. Surface run statistics on web

Expose the 12 fields already in `lidar_analysis_runs.statistics_json`.

- [ ] Deserialise `StatisticsJSON` in `GetRun()` / `ListRuns()` API responses
- [ ] Add `Statistics *RunStatistics` field to `AnalysisRun` JSON response
- [ ] Render on runs detail page: noise ratio, class counts, confirmed ratio
- [ ] Add run statistics summary card to HINT round history
- [ ] Enable round-over-round comparison (e.g. noise ratio trending down)

**Files:**

- `internal/lidar/monitor/run_track_api.go` — API response
- `web/src/routes/lidar/runs/` — Svelte rendering
- `internal/lidar/monitor/assets/sweep_dashboard.js` — HINT round history

**Beneficiaries:** HINT labelling, HINT dashboard, runs page, sweep results,
PDF reports

#### A2. Surface track quality metrics on web

Include the 6 per-track quality columns already in `lidar_tracks` in API
responses.

- [ ] Add `track_length_meters`, `track_duration_secs`, `occlusion_count`,
      `max_occlusion_frames`, `spatial_coverage`, `noise_point_ratio` to
      track list and track detail API responses
- [ ] Render in track detail panel alongside label controls
- [ ] Add sortable columns to track list view

**Files:**

- `internal/lidar/monitor/run_track_api.go` — include columns in query
- `web/src/routes/lidar/tracks/` — Svelte rendering

**Beneficiaries:** HINT labelling, runs page, analysis dashboards

#### A3. Persist and surface quality score

The composite `QualityScore` (0–1) from `ComputeTrackQualityMetrics()` is
computed but never persisted.

- [ ] Add `quality_score REAL` column to `lidar_tracks` and
      `lidar_run_tracks` (migration)
- [ ] Write in `InsertTrack()` / `UpdateTrack()`
- [ ] Include in track list and detail API responses
- [ ] Display as suggested quality indicator during labelling
- [ ] Enable "sort by quality" in track list

**Files:**

- `internal/db/` — migration
- `internal/lidar/storage/sqlite/` — write logic
- `internal/lidar/monitor/run_track_api.go` — API
- `web/src/routes/lidar/tracks/` — rendering

**Beneficiaries:** HINT labelling, runs page, training data curation

---

### Batch B — API endpoints (medium effort)

#### B1. Run comparison API endpoint

Expose `compareParams()` and `computeTemporalIoU()` via a new endpoint.

- [ ] Add `GET /api/lidar/runs/{runId}/compare/{otherRunId}` endpoint
- [ ] Return: parameter diff, per-track temporal IoU matrix, split/merge
      candidates
- [ ] Render inter-round track correspondence in HINT dashboard
- [ ] Show label carryover accuracy (IoU values for carried-over labels)

**Files:**

- `internal/lidar/monitor/run_track_api.go` — new handler
- `internal/lidar/storage/sqlite/analysis_run_compare.go` — already implemented
- `internal/lidar/monitor/assets/sweep_dashboard.js` — HINT round comparison

**Beneficiaries:** HINT labelling, HINT dashboard, runs page, sweep results

#### B2. Complete and surface noise coverage metrics

Finish the `ComputeNoiseCoverageMetrics()` implementation (speed/size
breakdown TODOs) and persist.

- [ ] Complete speed-bucket and size-bucket unknown ratio computation
- [ ] Complete noise ratio histogram binning
- [ ] Persist to `statistics_json` (extend `RunStatistics`) or new column
- [ ] Render noise profile on runs page and HINT round history

**Files:**

- `internal/lidar/l6objects/quality.go` — finish implementation
- `internal/lidar/storage/sqlite/analysis_run_manager.go` — persistence
- `web/` — rendering

**Beneficiaries:** HINT labelling, HINT dashboard, runs page, clustering
observability

---

### Batch C — Pipeline metrics in HINT round history (moderate effort)

#### C1. Aggregate foreground fraction per sweep combo

`FrameMetrics.ForegroundFraction` is computed per frame but transient.

- [ ] Add foreground fraction accumulator to sweep runner's `SampleResult`
- [ ] Compute mean + stddev in `ComboResult`
- [ ] Surface in sweep results and HINT round comparison

**Files:**

- `internal/lidar/l3grid/foreground.go` — already computes `FrameMetrics`
- `internal/lidar/sweep/runner.go` — accumulate
- `internal/lidar/sweep/output.go` — `SampleResult` extension

**Beneficiaries:** HINT dashboard, sweep results

#### C2. Surface tracking state transitions in HINT round history

`TracksCreated` and `TracksConfirmed` are computed in `TrackingMetrics` but
only exposed as the derived `FragmentationRatio`.

- [ ] Include `TracksCreated` / `TracksConfirmed` in `HINTRound` struct
- [ ] Display in HINT round history for round-over-round comparison

**Files:**

- `internal/lidar/sweep/hint.go` — `HINTRound` struct
- `internal/lidar/monitor/assets/sweep_dashboard.js` — rendering

**Beneficiaries:** HINT dashboard, sweep results, tracker debugging

#### C3. Full combo result in HINT round history

`ComboResult` has 36 fields but HINT stores only `BestScore` +
`BestScoreComponents`.

- [ ] Attach best-combo `ComboResult` to `HINTRound` struct
- [ ] Render detailed metric breakdown comparison across rounds

**Files:**

- `internal/lidar/sweep/hint.go` — `HINTRound` struct
- `internal/lidar/monitor/assets/sweep_dashboard.js` — rendering

**Beneficiaries:** HINT dashboard, sweep analysis

---

### Batch D — Per-track diagnostics (medium effort)

#### D1. Persist per-track jitter metrics

`HeadingJitterDeg` and `SpeedJitterMps` are accumulated per-track via L5
accumulators but only rolled up into run-level `TrackingMetrics`.

- [ ] Add `heading_jitter_deg REAL`, `speed_jitter_mps REAL` columns to
      `lidar_tracks` and `lidar_run_tracks` (migration)
- [ ] Compute from `HeadingJitterSumSq` / `HeadingJitterCount` accumulators
- [ ] Write in `InsertTrack()` / `UpdateTrack()`
- [ ] Include in track list and detail API responses
- [ ] Display alongside tracks during HINT labelling

**Files:**

- `internal/db/` — migration
- `internal/lidar/l5tracks/tracking.go` — finalise per-track values
- `internal/lidar/storage/sqlite/` — write logic
- `internal/lidar/monitor/run_track_api.go` — API

**Beneficiaries:** HINT labelling, runs page, tracker debugging

#### D2. Persist per-track alignment metrics

`MeanAlignmentDeg` and `MisalignmentRate` per track show velocity-trajectory
coherence.

- [ ] Add `alignment_deg REAL`, `misalignment_rate REAL` columns to
      `lidar_tracks` and `lidar_run_tracks` (migration)
- [ ] Write from `TrackAlignmentMetrics` in `InsertTrack()`
- [ ] Include in track list and detail API responses

**Files:**

- `internal/db/` — migration
- `internal/lidar/l5tracks/tracking.go` — expose per-track values
- `internal/lidar/storage/sqlite/` — write logic

**Beneficiaries:** HINT labelling, runs page, Kalman diagnostics

---

### Batch E — Diagnostic value, low HINT urgency

- [ ] **E1.** Surface cluster `cluster_density` and `aspect_ratio` on web
- [ ] **E2.** Populate `noise_points_count` in L4 pipeline
- [ ] **E3.** Surface speed-bucketed acceptance rates (`BucketMeans[]`) in
      sweep results frontend

---

### Removed from Scope

| Item                              | Reason                                                                                             |
| --------------------------------- | -------------------------------------------------------------------------------------------------- |
| ML feature vectors → export (§4)  | No HINT benefit. Separate ML infrastructure project.                                               |
| Training data curation → API (§6) | Downstream of HINT, not an input to it.                                                            |
| Per-track speed percentiles       | Design debt — removal planned via [migration 000030](schema-simplification-migration-030-plan.md). |

---

## Cross-System Benefit Map

| Item                      | HINT Label | HINT Dash | Runs | Sweep | PDF | macOS | ML  |
| ------------------------- | ---------- | --------- | ---- | ----- | --- | ----- | --- |
| A1 Run statistics         | ✅         | ✅        | ✅   | ✅    | ✅  | —     | —   |
| A2 Track quality on web   | ✅         | —         | ✅   | —     | —   | 🔶    | —   |
| A3 quality_score persist  | ✅         | —         | ✅   | —     | —   | —     | ✅  |
| B1 Run comparison API     | ✅         | ✅        | ✅   | ✅    | —   | —     | —   |
| B2 Noise coverage         | ✅         | ✅        | ✅   | —     | —   | —     | —   |
| C1 Foreground fraction    | —          | ✅        | —    | ✅    | —   | —     | —   |
| C2 State transitions      | —          | ✅        | ✅   | ✅    | —   | —     | —   |
| C3 Full combo in history  | —          | ✅        | —    | ✅    | —   | —     | —   |
| D1 Per-track jitter       | ✅         | —         | ✅   | —     | —   | 🔶    | —   |
| D2 Per-track alignment    | ✅         | —         | ✅   | —     | —   | 🔶    | —   |
| E1 Cluster quality on web | —          | —         | ✅   | —     | —   | —     | —   |
| E2 noise_points_count     | —          | —         | ✅   | —     | —   | —     | —   |
| E3 Speed-bucketed rates   | —          | —         | —    | ✅    | —   | —     | —   |

✅ = direct benefit, 🔶 = partial, — = no impact
