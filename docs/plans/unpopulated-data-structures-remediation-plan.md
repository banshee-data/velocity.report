# Unpopulated Data Structures Remediation Plan

Phased plan to wire up data structures that are computed on the Go backend
but never persisted, exposed via API, or consumed by any presentation
surface — plus per-track speed percentile cleanup per the
[speed percentile alignment plan](speed-percentile-aggregation-alignment-plan.md).

**Status:** Active — Phases 1–3 implemented (March 2026); Phases 4–10 proposed
**Related:** [Backend → Surface Matrix](../../data/structures/BACKEND_SURFACE_MATRIX.md), [Clustering observability plan](lidar-clustering-observability-and-benchmark-plan.md), [Analysis run infrastructure](lidar-analysis-run-infrastructure-plan.md), [Speed Percentile Alignment Plan](speed-percentile-aggregation-alignment-plan.md), [Schema Simplification Plan](schema-simplification-migration-030-plan.md)

---

## Problem

A full-codebase audit identified **9 database columns** that exist in the
schema but are never written, **4 Go structs** (34 fields total) that are
computed but never persisted or exposed, and **2 feature-vector structs**
(30 fields) with no export path. This creates:

1. **Dead schema weight** — columns consume space in the SQLite page layout
   and appear in tooling (schema diagrams, migration diffs) but carry no
   data, confusing contributors and operators.
2. **Lost analytical value** — `RunStatistics` quality distribution metrics
   are computed on every analysis run and immediately discarded.
3. **Incomplete UI surfaces** — the web dashboard shows track counts but
   cannot display quality scores, noise ratios, or classification confidence
   distributions that the backend already calculates.
4. **Blocked ML pipeline** — `TrackFeatures` and `TrainingDatasetSummary`
   are the foundation for classifier training data export, but no endpoint
   or file-export path exists.

## Scope

This plan covers wiring existing, already-implemented Go code to persistence
and API layers. It does **not** cover:

- New algorithmic work (e.g. implementing the full `NoiseCoverageMetrics`
  speed/size breakdown — that remains a TODO in quality.go:229).
- Clustering observability metrics from the
  [observability plan](lidar-clustering-observability-and-benchmark-plan.md)
  (FrameStageTiming, AssociationDecision).
- New UI components — each phase notes where UI work is needed but does not
  spec the Svelte components.

---

## Phase 1 — Wire `statistics_json` (Run Statistics)

**Priority:** High — directly answers the open question in the clustering
observability plan §4.
**Effort:** Small (1–2 days)
**Risk:** Low — no schema change needed; column already exists.
**Schedule:** Next available sprint. No dependencies.

### Checklist

- [x] In `CompleteRun()` (`analysis_run.go:463`), call
      `l6objects.ComputeRunStatistics()` on the run's collected tracks and
      serialise the result to `statistics_json` via `RunStatistics.ToJSON()`.
- [x] Update the `CompleteRun` SQL to include `statistics_json = ?`.
- [x] Update `GetRun()` (`analysis_run.go:496`) to read and parse
      `statistics_json`, attaching it to the `AnalysisRun` struct.
- [x] Add a `StatisticsJSON json.RawMessage` field to the `AnalysisRun`
      struct.
- [x] Update `ListRuns()` to also read `statistics_json`.
- [x] Wire `AnalysisRunManager.CompleteRun()` to collect tracks during
      `RecordTrack()` and compute `RunStatistics` at completion.
- [ ] Update `handleGetRun()` API handler so the JSON response includes
      `statistics_json` when present.
- [ ] Add a TypeScript `RunStatistics` interface to `web/src/lib/types/lidar.ts`.
- [ ] Add the field to the `AnalysisRun` TypeScript interface.
- [x] Verify backward compatibility: existing rows with `NULL`
      `statistics_json` do not break `GetRun()` — validated by tests.

### Downstream opportunity

Once `statistics_json` is populated, the web run-detail view can display a
quality summary card (class distribution, noise ratio, track length
distribution). This is a separate UI task.

---

## Phase 2 — Populate Track Quality Columns

**Priority:** High — 6 columns in `lidar_tracks` exist but are always NULL.
**Effort:** Small–medium (2–3 days)
**Risk:** Low — columns exist; `InsertTrack`/`UpdateTrack` just need
additional parameters.
**Schedule:** Same sprint as Phase 1 or immediately after.

### Checklist

- [x] Update `InsertTrack()` (`track_store.go:92`) to include
      `track_length_meters`, `track_duration_secs`, `occlusion_count`,
      `max_occlusion_frames`, `spatial_coverage`, `noise_point_ratio`.
- [x] Update `UpdateTrack()` (`track_store.go:154`) to write the same 6
      columns on each update.
- [x] Verify that `TrackedObject` already carries these fields (it does —
      they are set by the L5 tracker).
- [x] Update `ON CONFLICT DO UPDATE` clause in `InsertTrack` to include the
      6 new columns.
- [ ] Add the 6 fields to the `Track` TypeScript interface in
      `web/src/lib/types/lidar.ts`.
- [ ] Update the live-tracks API handler (`handleListTracks`) to include the
      fields in the JSON response (verify the Go struct already has them).
- [x] All existing Go tests pass with new column writes.

### Downstream opportunity

With quality columns populated, an `idx_lidar_tracks_quality` index (already
exists in schema) becomes useful for filtering high-quality tracks for
labelling.

---

## Phase 3 — Populate Cluster Quality Columns

**Priority:** Medium — 3 columns in `lidar_clusters` are always NULL.
**Effort:** Small (1 day)
**Risk:** Low — requires computing `noise_points_count`, `cluster_density`,
and `aspect_ratio` from data already available at insert time.
**Schedule:** Backlog — schedule after Phase 2 when cluster diagnostics
become a priority.

### Checklist

- [ ] Compute `noise_points_count` during clustering (requires adding
      a `NoisePointsCount` field to `WorldCluster` in `l4perception/types.go`
      and populating it during the L4 clustering step).
- [x] Compute `cluster_density` as `points_count / bbox_volume`.
- [x] Compute `aspect_ratio` as `bbox_length / bbox_width`.
- [x] Update `InsertCluster()` (`track_store.go:56`) to write the 2
      computable quality columns.
- [ ] Add the fields to the `ClusterResponse` TypeScript interface.
- [ ] Write tests for edge cases (zero-volume bbox, single-point cluster).

---

## Phase 4 — Run Statistics API Endpoint

**Priority:** Medium — enables UI consumption of Phase 1 data.
**Effort:** Small (1 day)
**Risk:** Low.
**Schedule:** After Phase 1. Can be combined with Phase 1 if capacity allows.

### Checklist

- [ ] Add `GET /api/lidar/runs/{run_id}/statistics` endpoint in
      `run_track_api.go` returning `RunStatistics` JSON.
- [ ] Return `404` if `statistics_json` is NULL (pre-Phase-1 runs).
- [ ] Add `getRunStatistics(runId)` function to `web/src/lib/api.ts`.
- [ ] Write handler tests with populated and NULL statistics.

---

## Phase 5 — Training Data Export Endpoint

**Priority:** Medium–low — enables ML classifier training pipeline.
**Effort:** Medium (3–5 days)
**Risk:** Medium — defines a new public API contract for feature vectors.
**Schedule:** Backlog — schedule when ML classifier training becomes active.
Depends on Phases 1 and 2.

### Checklist

- [ ] Add `GET /api/lidar/runs/{run_id}/training-export` endpoint.
- [ ] Accept query parameters: `min_quality_score`, `min_duration`,
      `min_length`, `require_class` (matching `TrackTrainingFilter` fields).
- [ ] Return JSON with `TrainingDatasetSummary` header and array of
      `TrackFeatures` vectors (using `SortedFeatureNames()` for column
      headers).
- [ ] Optional: support `Accept: text/csv` for direct CSV export.
- [ ] Wire `FilterTracksForTraining()` and `SummarizeTrainingDataset()`.
- [ ] Write handler tests with filtered and unfiltered exports.
- [ ] Document the endpoint in `data/structures/README.md`.

---

## Phase 6 — Run Comparison API

**Priority:** Low — comparison logic exists but is not user-accessible.
**Effort:** Medium (2–3 days)
**Risk:** Low.
**Schedule:** Backlog — schedule when multi-run comparison UI is prioritised.

### Checklist

- [ ] Add `GET /api/lidar/runs/compare?ref={run_id}&candidate={run_id}`
      endpoint.
- [ ] Return parameter diff (from `compareParams()`), temporal IoU matrix,
      and split/merge candidates.
- [ ] Add TypeScript interfaces for the comparison response.
- [ ] Write handler tests for same-params, different-params, and
      missing-run scenarios.

---

## Phase 7 — ~~Speed Percentile Exposure~~ Per-Track Speed Percentile Removal

**Priority:** Medium — design debt per D-18 and the speed percentile
alignment plan.
**Effort:** Small–medium (1–2 days) — most Go renames already done.
**Risk:** Low–medium — migration drops columns; Go code already aligned.
**Schedule:** Same sprint as Phase 4 or immediately after. Aligned with
[schema simplification migration 030 plan](schema-simplification-migration-030-plan.md).

### Checklist

Per the [speed percentile alignment plan](speed-percentile-aggregation-alignment-plan.md),
percentiles are reserved for grouped/report aggregates only. Per-track
percentile columns are the wrong abstraction and must be removed, not
surfaced.

**Already done (Go structs, proto, API, TypeScript):**

- [x] `TrackedObject.MaxSpeedMps` — already renamed from `PeakSpeedMps`.
- [x] `RunTrack.MaxSpeedMps` — already renamed.
- [x] Proto uses `max_speed_mps` (field 25), no percentile fields.
- [x] REST API (`track_api.go`) uses `max_speed_mps`, no per-track
      percentile fields exposed.
- [x] TypeScript types — no per-track percentile fields.
- [x] `InsertTrack()` / `UpdateTrack()` — no longer write `p50/p85/p95`.
- [x] `ComputeSpeedPercentiles()` kept as internal-only for classifier
      feature extraction.

**Remaining (migration 000030 and SQL cleanup):**

- [ ] Write and apply migration 000030 to drop `p50_speed_mps`,
      `p85_speed_mps`, `p95_speed_mps` from `lidar_tracks` and
      `lidar_run_tracks`, and rename `peak_speed_mps` → `max_speed_mps` on
      both tables.
- [ ] Update `schema.sql` to match post-migration state.
- [ ] Remove per-track percentile columns from `InsertRunTrack()`,
      `GetRunTracks()`, `GetRunTrack()` SQL in `analysis_run.go`.
- [ ] Rename `peak_speed_mps` → `max_speed_mps` in all SQL strings
      (`track_store.go`, `analysis_run.go`).
- [ ] Update all test fixtures that reference percentile columns or
      `peak_speed_mps` (`coverage_boost_test.go`, `track_api_test.go`,
      `webserver_coverage_test.go`, `scene_api_coverage_test.go`,
      `analysis_run_extended_test.go`).
- [ ] Update `pcap-analyse` `SpeedStatistics` to use `p98` not `p95`
      as the high-end aggregate percentile (D-18).
- [ ] Regenerate schema ERD with `make schema-erd`.

---

## Phase 8 — Cleanup Scaffolding Structs

**Priority:** Low — removes dead code that has no concrete consumer.
**Effort:** Small (1 day)
**Risk:** Low — deletion only. No runtime impact.
**Schedule:** Backlog — schedule after Phase 5 determines whether
`NoiseCoverageMetrics` will be fully implemented or removed.

### Checklist

- [ ] Decide: complete `NoiseCoverageMetrics` (implement speed/size
      breakdown) or delete the struct and its placeholder computation.
- [ ] If deleting: remove `NoiseCoverageMetrics`,
      `ComputeNoiseCoverageMetrics()`, and associated tests.
- [ ] If completing: implement the full speed/size breakdown, add
      persistence, and add an API endpoint.
- [ ] Audit `TrainingDatasetSummary.TotalPoints` — if point cloud storage
      is not on the roadmap, remove the field and its TODO comment.

---

## Scheduling Guidance

### ✅ Completed (March 2026)

Phases 1, 2, and 3 (partial) are done:

- `statistics_json` is computed and persisted on every completed analysis
  run.
- Track quality columns (`track_length_meters`, `track_duration_secs`,
  `occlusion_count`, `max_occlusion_frames`, `spatial_coverage`,
  `noise_point_ratio`) are written on every INSERT/UPDATE.
- Cluster `cluster_density` and `aspect_ratio` are computed and persisted
  on every INSERT.

### Immediate (current sprint)

Phase 4 (statistics API endpoint) and Phase 7 (per-track percentile
removal / migration 000030) should be scheduled next. Phase 4 unlocks UI
consumption of the newly persisted statistics. Phase 7 cleans up design
debt identified by D-18/D-19.

### Near-term (next 1–2 sprints)

Phase 3 completion (noise_points_count) requires an L4 pipeline change.
Schedule when cluster diagnostics become a priority.

### Backlog (schedule when needed)

Phases 5–8 depend on product direction:

- **Phase 5** (training export) should be scheduled when the ML classifier
  training pipeline is active. Without it, feature vectors are computed and
  discarded.
- **Phase 6** (run comparison) should be scheduled when the multi-run
  comparison UI is prioritised (see the
  [split-merge repair workbench plan](lidar-visualiser-split-merge-repair-workbench-plan.md)).
- **Phase 8** (cleanup) is a housekeeping task best done after Phase 5
  resolves the future of the training data curation structs.

### Dependency Graph

```
Phase 1 (statistics_json) ──[DONE]──► Phase 4 (statistics API) ──► UI card
     │
     ├──► Phase 5 (training export) ──► Phase 8 (cleanup)
     │
Phase 2 (track quality cols) ──[DONE]──► Phase 5
     │
Phase 3 (cluster quality cols) ──[PARTIAL]
     │
Phase 6 (run comparison)
     │
Phase 7 (percentile removal / migration 030)
```

---

## Risk Register

| Risk                                                              | Mitigation                                                                           |
| ----------------------------------------------------------------- | ------------------------------------------------------------------------------------ |
| `statistics_json` bloats DB for large runs                        | JSON is < 1 KB; negligible                                                           |
| Track quality writes increase per-frame DB load                   | 6 extra columns in an existing UPDATE; ~µs overhead per frame                        |
| Training export returns very large responses                      | Add pagination / streaming; apply `TrackTrainingFilter` by default                   |
| Backward compatibility on API changes                             | All new fields are additive (not removing existing fields); `omitempty` for optional |
| Cluster quality computation depends on noise threshold definition | Use existing `NoisePointRatio` threshold from config                                 |
