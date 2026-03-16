# Unpopulated Data Structures Remediation Plan

Phased plan to wire up data structures that are computed on the Go backend
but never persisted, exposed via API, or consumed by any presentation surface.

**Status:** Proposed (March 2026)
**Related:** [Backend → Surface Matrix](../../data/structures/BACKEND_SURFACE_MATRIX.md), [Clustering observability plan](lidar-clustering-observability-and-benchmark-plan.md), [Analysis run infrastructure](lidar-analysis-run-infrastructure-plan.md)

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

- [ ] In `CompleteRun()` (`analysis_run.go:463`), call
      `l6objects.ComputeRunStatistics()` on the run's confirmed tracks and
      serialise the result to `statistics_json` via `RunStatistics.ToJSON()`.
- [ ] Update the `CompleteRun` SQL to include `statistics_json = ?`.
- [ ] Update `GetRun()` (`analysis_run.go:496`) to read and parse
      `statistics_json`, attaching it to the `AnalysisRun` struct.
- [ ] Add a `StatisticsJSON json.RawMessage` field (or typed
      `*RunStatistics`) to the `AnalysisRun` struct.
- [ ] Update `handleGetRun()` API handler so the JSON response includes
      `statistics_json` when present.
- [ ] Add a TypeScript `RunStatistics` interface to `web/src/lib/types/lidar.ts`.
- [ ] Add the field to the `AnalysisRun` TypeScript interface.
- [ ] Write Go unit tests for the round-trip: compute → serialise → store →
      retrieve → deserialise.
- [ ] Verify backward compatibility: existing rows with `NULL`
      `statistics_json` must not break `GetRun()`.

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

- [ ] Update `InsertTrack()` (`track_store.go:92`) to include
      `track_length_meters`, `track_duration_secs`, `occlusion_count`,
      `max_occlusion_frames`, `spatial_coverage`, `noise_point_ratio`.
- [ ] Update `UpdateTrack()` (`track_store.go:154`) to write the same 6
      columns on each update.
- [ ] Verify that `TrackedObject` already carries these fields (it does —
      they are set by the L5 tracker).
- [ ] Update `ON CONFLICT DO UPDATE` clause in `InsertTrack` to include the
      6 new columns.
- [ ] Add the 6 fields to the `Track` TypeScript interface in
      `web/src/lib/types/lidar.ts`.
- [ ] Update the live-tracks API handler (`handleListTracks`) to include the
      fields in the JSON response (verify the Go struct already has them).
- [ ] Write integration tests verifying the columns are non-NULL after a
      track is inserted/updated.

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

- [ ] Compute `noise_points_count` during clustering (count points below
      a noise threshold or outside the cluster core).
- [ ] Compute `cluster_density` as `points_count / bbox_volume`.
- [ ] Compute `aspect_ratio` as `bbox_length / bbox_width`.
- [ ] Update `InsertCluster()` (`track_store.go:56`) to write the 3
      additional columns.
- [ ] Add the 3 fields to the `ClusterResponse` TypeScript interface.
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

## Phase 7 — Speed Percentile Exposure

**Priority:** Low — data is already persisted but not surfaced.
**Effort:** Small (< 1 day)
**Risk:** Low.
**Schedule:** Backlog — can be picked up opportunistically.

### Checklist

- [ ] Add `p50_speed_mps`, `p85_speed_mps`, `p95_speed_mps` to the
      `RunTrack` Go struct in `analysis_run.go`.
- [ ] Update `GetRunTracks()` SQL query to SELECT these columns.
- [ ] Add the 3 fields to the `RunTrack` TypeScript interface.
- [ ] Verify the protobuf `Track` message does not need changes (speed
      percentiles are not relevant for the live gRPC stream).

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

### Immediate (current sprint)

Phases 1 and 2 should be scheduled together. They require no schema changes,
carry low risk, and eliminate the most visible gap: `statistics_json` being
computed and discarded on every run. Combined effort: 3–4 days.

### Near-term (next 1–2 sprints)

Phases 3 and 4 can be scheduled after Phase 1 ships. Phase 4 in particular
is a natural follow-on that unlocks UI consumption of run statistics.

### Backlog (schedule when needed)

Phases 5–8 depend on product direction:

- **Phase 5** (training export) should be scheduled when the ML classifier
  training pipeline is active. Without it, feature vectors are computed and
  discarded.
- **Phase 6** (run comparison) should be scheduled when the multi-run
  comparison UI is prioritised (see the
  [split-merge repair workbench plan](lidar-visualiser-split-merge-repair-workbench-plan.md)).
- **Phase 7** (speed percentiles) is small enough to pick up
  opportunistically during any LiDAR sprint.
- **Phase 8** (cleanup) is a housekeeping task best done after Phase 5
  resolves the future of the training data curation structs.

### Dependency Graph

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
Phase 7 (speed percentiles)
```

---

## Risk Register

| Risk | Mitigation |
| ----------------------------------------- | ----------------------------------------- |
| `statistics_json` bloats DB for large runs | JSON is < 1 KB; negligible |
| Track quality writes increase per-frame DB load | 6 extra columns in an existing UPDATE; ~µs overhead per frame |
| Training export returns very large responses | Add pagination / streaming; apply `TrackTrainingFilter` by default |
| Backward compatibility on API changes | All new fields are additive (not removing existing fields); `omitempty` for optional |
| Cluster quality computation depends on noise threshold definition | Use existing `NoisePointRatio` threshold from config |
