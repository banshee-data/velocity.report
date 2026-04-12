# Data completeness remediation

Phased plan to wire up data structures that are computed on the Go backend but never persisted, exposed via API, or consumed by any presentation surface: plus per-track speed percentile cleanup per the speed percentile alignment plan.

## Source

- Plan: `docs/plans/unpopulated-data-structures-remediation-plan.md`
- Status: Active; Phases 1–3 proposed; Phases 4–10 proposed
- Related: Backend → Surface Matrix (`data/structures/MATRIX.md`)

## Problem

A full-codebase audit identified:

- **9 database columns** that exist in the schema but are never written
- **4 Go structs** (34 fields total) computed but never persisted or exposed
- **2 feature-vector structs** (30 fields) with no export path

This creates dead schema weight, lost analytical value, incomplete UI surfaces, and a blocked ML pipeline.

## Phase summary

### Phase 1: wire `statistics_json` (run statistics)

**Priority:** High. Effort: Small (1–2 days). Risk: Low; no schema change; column already exists.

In `CompleteRun()` (`analysis_run.go:463`), call `l6objects.ComputeRunStatistics()` on the run's collected tracks and serialise to `statistics_json`. Update `GetRun()` and `ListRuns()` to read and parse it. Add `StatisticsJSON json.RawMessage` to the `AnalysisRun` struct.

Downstream: enables web run-detail quality summary card.

### Phase 2: populate track quality columns

**Priority:** High. Effort: Small–medium (2–3 days). Risk: Low; columns exist.

6 columns in `lidar_tracks` exist but hold NULL/0 defaults: `track_length_meters`, `track_duration_secs`, `occlusion_count`, `max_occlusion_frames`, `spatial_coverage`, `noise_point_ratio`. Update `InsertTrack()` and `UpdateTrack()` to populate them from `TrackedObject` fields.

Downstream: `idx_lidar_tracks_quality` index becomes useful for filtering high-quality tracks for labelling.

### Phase 3: populate cluster quality columns

**Priority:** Medium. Effort: Small (1 day). Risk: Low.

3 quality-related columns in `lidar_clusters` are unpopulated: `noise_points_count`, `cluster_density`, `aspect_ratio`. Compute from data already available at insert time.

### Phase 4: run statistics API endpoint

`GET /api/lidar/runs/{run_id}/statistics` endpoint. Returns `RunStatistics` JSON; 404 if NULL (pre-Phase-1 runs).

### Phase 5: training data export endpoint

`GET /api/lidar/runs/{run_id}/training-export` with filters (`min_quality_score`, `min_duration`, `min_length`, `require_class`). Returns `TrainingDatasetSummary` header + `TrackFeatures` vectors. Optional CSV via `Accept: text/csv`.

### Phase 6: run comparison API

`GET /api/lidar/runs/compare?ref={run_id}&candidate={run_id}`. Returns parameter diff, temporal IoU matrix, split/merge candidates.

### Phase 7: per-track speed percentile removal

Per the speed percentile alignment plan, percentiles are reserved for grouped/report aggregates only. Per-track percentile columns are the wrong abstraction. Migration 000030 drops `p50_speed_mps`, `p85_speed_mps`, `p95_speed_mps` from `lidar_tracks` and `lidar_run_tracks`, and renames `peak_speed_mps` → `max_speed_mps` on both tables.

Go struct renames already done (`TrackedObject.MaxSpeedMps`, proto `max_speed_mps`). Remaining: write and apply migration, update SQL strings, update test fixtures, switch `pcap-analyse` to p98 high-end aggregate.

### Phase 8: cleanup scaffolding structs

Decide whether to complete `NoiseCoverageMetrics` (implement speed/size breakdown) or delete. Audit `TrainingDatasetSummary.TotalPoints`.

## Dependency graph

```
Phase 1 (statistics_json) ──► Phase 4 (statistics API) ──► UI card
     │
     ├──► Phase 5 (training export) ──► Phase 8 (cleanup)
     │
Phase 2 (track quality cols) ──► Phase 5
     │
Phase 3 (cluster quality cols)
     │
Phase 6 (run comparison)
     │
Phase 7 (percentile removal / migration 030)
```

## Scheduling guidance

- **Immediate:** Phases 1–3 (wire existing data, minimal risk)
- **Near-term:** Phase 4 (API) + Phase 7 (migration 030 cleanup)
- **Backlog:** Phases 5–8 (depend on product direction; ML pipeline, comparison UI)

## Risk register

| Risk                                                   | Mitigation                                                   |
| ------------------------------------------------------ | ------------------------------------------------------------ |
| `statistics_json` bloats DB for large runs             | JSON is < 1 KB; negligible                                   |
| Track quality writes increase per-frame DB load        | 6 extra columns in existing UPDATE; ~µs overhead per frame   |
| Training export returns very large responses           | Pagination/streaming; apply `TrackTrainingFilter` by default |
| Backward compatibility on API changes                  | All new fields additive; `omitempty` for optional            |
| Cluster quality computation depends on noise threshold | Use existing `NoisePointRatio` threshold from config         |
