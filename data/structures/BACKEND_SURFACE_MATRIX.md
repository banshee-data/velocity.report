# Backend → Surface Publication Matrix

Mapping of all data structures computed on the Go backend to the surfaces
that consume them: **Web** (Svelte UI on `:8080`), **PDF** (Python LaTeX
generator), **macOS** (Metal visualiser via gRPC), and **SQLite** (database
persistence). Unpopulated or partially wired fields are flagged.

**Source:** Full-codebase audit (March 2026)
**Related:** [Remediation plan](../../docs/plans/unpopulated-data-structures-remediation-plan.md), [Clustering observability plan](../../docs/plans/lidar-clustering-observability-and-benchmark-plan.md)

---

## Legend

| Symbol | Meaning                                            |
| ------ | -------------------------------------------------- |
| ✅     | Computed, persisted, and consumed by this surface  |
| 🔶     | Computed but only partially wired (see notes)      |
| ❌     | Computed on backend but never reaches this surface |
| ⬜     | Not applicable to this surface                     |
| 🗄️     | Column exists in schema but is never written       |

---

## 1. Analysis Run Statistics

**Go source:** `internal/lidar/l6objects/quality.go` — `RunStatistics` struct (12 fields)
**Computation:** `ComputeRunStatistics(tracks)` — fully implemented, tested
**DB column:** `lidar_analysis_runs.statistics_json` — 🗄️ column exists, never written

| Field                        | SQLite | Web | PDF | macOS |
| ---------------------------- | ------ | --- | --- | ----- |
| `avg_track_length_meters`    | 🗄️     | ❌  | ❌  | ❌    |
| `median_track_length_meters` | 🗄️     | ❌  | ❌  | ❌    |
| `avg_track_duration_secs`    | 🗄️     | ❌  | ❌  | ❌    |
| `avg_occlusion_count`        | 🗄️     | ❌  | ❌  | ❌    |
| `class_counts`               | 🗄️     | ❌  | ❌  | ❌    |
| `class_confidence_avg`       | 🗄️     | ❌  | ❌  | ❌    |
| `unknown_ratio`              | 🗄️     | ❌  | ❌  | ❌    |
| `avg_noise_ratio`            | 🗄️     | ❌  | ❌  | ❌    |
| `avg_spatial_coverage`       | 🗄️     | ❌  | ❌  | ❌    |
| `tentative_ratio`            | 🗄️     | ❌  | ❌  | ❌    |
| `confirmed_ratio`            | 🗄️     | ❌  | ❌  | ❌    |
| `avg_observations_per_track` | 🗄️     | ❌  | ❌  | ❌    |

**Status (March 2026):** `ComputeRunStatistics()` is fully implemented and
tested but never called during `CompleteRun()`. The column exists in schema.
Wiring `AnalysisRunManager.CompleteRun()` to collect tracks during
`RecordTrack()` and compute/serialise `RunStatistics` at completion is
planned (see remediation plan Phase 1).

---

## 2. Track Quality Metrics (per-track)

**Go source:** `internal/lidar/l6objects/quality.go` — `TrackQualityMetrics` struct (8 fields)
**Computation:** `ComputeTrackQualityMetrics(track)` — fully implemented, tested

| Field                  | SQLite | Web | PDF | macOS |
| ---------------------- | ------ | --- | --- | ----- |
| `track_id`             | ✅     | ✅  | ⬜  | ✅    |
| `track_length_meters`  | 🗄️     | ❌  | ❌  | 🔶    |
| `track_duration_secs`  | 🗄️     | ❌  | ❌  | 🔶    |
| `occlusion_count`      | 🗄️     | ❌  | ❌  | 🔶    |
| `max_occlusion_frames` | 🗄️     | ❌  | ❌  | ❌    |
| `spatial_coverage`     | 🗄️     | ❌  | ❌  | ❌    |
| `noise_point_ratio`    | 🗄️     | ❌  | ❌  | ❌    |
| `quality_score`        | ❌     | ❌  | ❌  | ❌    |

**Status (March 2026):** Columns exist in schema but `InsertTrack()` and
`UpdateTrack()` do not write them. The `TrackedObject` fields are populated
by the L5 tracker (`ComputeQualityMetrics()`), so the data is available —
wiring it to the INSERT/UPDATE SQL is planned (see remediation plan Phase 2).
`quality_score` remains computed-only in `l6objects` with no DB column.

---

## 3. Cluster Quality Metrics

**Go source:** `internal/lidar/l4perception/types.go` — `WorldCluster` struct
**DB table:** `lidar_clusters`

| Field                | SQLite | Web | PDF | macOS |
| -------------------- | ------ | --- | --- | ----- |
| `centroid_x/y/z`     | ✅     | ✅  | ⬜  | ✅    |
| `bounding_box_*`     | ✅     | ✅  | ⬜  | ✅    |
| `points_count`       | ✅     | ✅  | ⬜  | ✅    |
| `height_p95`         | ✅     | ✅  | ⬜  | ✅    |
| `intensity_mean`     | ✅     | ✅  | ⬜  | ✅    |
| `noise_points_count` | 🗄️     | ❌  | ❌  | ❌    |
| `cluster_density`    | 🗄️     | ❌  | ❌  | ❌    |
| `aspect_ratio`       | 🗄️     | ❌  | ❌  | ❌    |

**Status (March 2026):** All 3 columns exist in schema but are never written.
`cluster_density` (points/volume) and `aspect_ratio` (length/width) can be
computed from data already available at insert time — wiring is planned (see
remediation plan Phase 3). `noise_points_count` requires upstream noise-point
tracking in the L4 clustering pipeline (the `WorldCluster` struct does not
currently carry a noise count).

---

## 4. ML Feature Vectors

**Go source:** `internal/lidar/l6objects/features.go` — `TrackFeatures` struct (20 features)
**Computation:** `ExtractTrackFeatures(track)` — fully implemented, tested
**Export:** `ToVector()` produces a `[]float32` in canonical order; `SortedFeatureNames()` provides column headers

| Component               | SQLite | Web | PDF | macOS |
| ----------------------- | ------ | --- | --- | ----- |
| `ClusterFeatures` (10)  | ❌     | ❌  | ❌  | ❌    |
| `TrackFeatures` (20)    | ❌     | ❌  | ❌  | ❌    |
| `ToVector()` export     | ❌     | ❌  | ❌  | ❌    |
| `SortFeatureImportance` | ❌     | ❌  | ❌  | ❌    |

**Root cause:** Feature extraction is used in-memory by the classifier but
has no persistence layer, no API endpoint, and no export capability.
The pipeline exposes a `FeatureExportFunc` callback but no consumer is wired.

---

## 5. Noise Coverage Metrics

**Go source:** `internal/lidar/l6objects/quality.go` — `NoiseCoverageMetrics` struct (7 fields)
**Computation:** `ComputeNoiseCoverageMetrics(tracks)` — **partially implemented** (TODO at line 229)

| Field                          | SQLite | Web | PDF | macOS |
| ------------------------------ | ------ | --- | --- | ----- |
| `total_tracks`                 | ❌     | ❌  | ❌  | ❌    |
| `tracks_with_high_noise`       | ❌     | ❌  | ❌  | ❌    |
| `tracks_unknown_class`         | ❌     | ❌  | ❌  | ❌    |
| `tracks_low_confidence`        | ❌     | ❌  | ❌  | ❌    |
| `unknown_ratio_by_speed`       | ❌     | ❌  | ❌  | ❌    |
| `unknown_ratio_by_size`        | ❌     | ❌  | ❌  | ❌    |
| `noise_ratio_histogram_counts` | ❌     | ❌  | ❌  | ❌    |

**Root cause:** Entire struct is scaffolding for future coverage analysis.
The computation is a placeholder (counts only high-noise and unknown tracks;
speed/size breakdown is allocated but never filled).

---

## 6. Training Data Curation

**Go source:** `internal/lidar/l6objects/quality.go` — `TrainingDatasetSummary` struct (7 fields)
**Computation:** `SummarizeTrainingDataset(tracks)` — implemented (TODO: `TotalPoints` not populated)

| Field                | SQLite | Web | PDF | macOS |
| -------------------- | ------ | --- | --- | ----- |
| `total_tracks`       | ❌     | ❌  | ❌  | ❌    |
| `total_frames`       | ❌     | ❌  | ❌  | ❌    |
| `total_points`       | ❌     | ❌  | ❌  | ❌    |
| `class_distribution` | ❌     | ❌  | ❌  | ❌    |
| `avg_quality_score`  | ❌     | ❌  | ❌  | ❌    |
| `avg_duration_secs`  | ❌     | ❌  | ❌  | ❌    |
| `avg_length_meters`  | ❌     | ❌  | ❌  | ❌    |

**Root cause:** No API endpoint exists for training data export.
`TrackTrainingFilter` and `FilterTracksForTraining()` are implemented and
tested but never called from any handler. `TotalPoints` has a TODO: "Add
point count when point cloud storage is integrated."

---

## 7. Run Comparison / Split-Merge Analysis

**Go source:** `internal/lidar/storage/sqlite/analysis_run_compare.go` — comparison functions
**Computation:** `compareParams()`, `computeTemporalIoU()` — implemented

| Component             | SQLite | Web | PDF | macOS |
| --------------------- | ------ | --- | --- | ----- |
| Parameter diff        | ❌     | ❌  | ❌  | ❌    |
| Temporal IoU          | ❌     | ❌  | ❌  | ❌    |
| Track split detection | 🔶     | 🔶  | ❌  | ❌    |
| Track merge detection | 🔶     | 🔶  | ❌  | ❌    |

**Notes:** `is_split_candidate` and `is_merge_candidate` flags are stored in
`lidar_run_tracks` and exposed in the web UI's labelling view. However, the
comparison logic that _generates_ these flags (`compareParams`,
`computeTemporalIoU`) has no API endpoint — there is no way to trigger a
run-vs-run comparison from any surface.

---

## 8. Live Track Fields — Fully Wired (Reference)

These fields flow correctly from pipeline through all surfaces:

| Field                | SQLite | Web | PDF | macOS |
| -------------------- | ------ | --- | --- | ----- |
| `track_id`           | ✅     | ✅  | ⬜  | ✅    |
| `sensor_id`          | ✅     | ✅  | ⬜  | ✅    |
| `track_state`        | ✅     | ✅  | ⬜  | ✅    |
| `position (x,y,z)`   | ✅     | ✅  | ⬜  | ✅    |
| `velocity (vx,vy)`   | ✅     | ✅  | ⬜  | ✅    |
| `speed_mps`          | ✅     | ✅  | ⬜  | ✅    |
| `heading_rad`        | ✅     | ✅  | ⬜  | ✅    |
| `observation_count`  | ✅     | ✅  | ⬜  | ✅    |
| `avg_speed_mps`      | ✅     | ✅  | ⬜  | ✅    |
| `peak_speed_mps`     | ✅     | ✅  | ⬜  | ✅    |
| `bounding_box_*`     | ✅     | ✅  | ⬜  | ✅    |
| `height_p95_max`     | ✅     | ✅  | ⬜  | ✅    |
| `intensity_mean_avg` | ✅     | ✅  | ⬜  | ✅    |
| `object_class`       | ✅     | ✅  | ⬜  | ✅    |
| `object_confidence`  | ✅     | ✅  | ⬜  | ✅    |
| `heading_source`     | ❌     | ✅  | ⬜  | ✅    |

**Note:** `heading_source` is published via gRPC and the live tracks API but
is not persisted to SQLite.

---

## 9. Radar Data → PDF Surface

The PDF generator consumes radar data only (not LiDAR). For completeness:

| API                            | Web | PDF | macOS |
| ------------------------------ | --- | --- | ----- |
| `GET /api/radar_stats`         | ✅  | ✅  | ⬜    |
| `GET /api/events`              | ✅  | ❌  | ⬜    |
| `GET /api/sites`               | ✅  | ✅  | ⬜    |
| `GET /api/site_config_periods` | ✅  | ✅  | ⬜    |
| `POST /api/generate_report`    | ✅  | ✅  | ⬜    |
| LiDAR analysis runs            | ✅  | ❌  | ❌    |
| LiDAR tracks/observations      | ✅  | ❌  | ✅    |
| LiDAR sweeps/HINT              | ✅  | ❌  | ❌    |

---

## 10. Speed Percentile Columns — Design Debt

**DB columns:** `lidar_tracks` and `lidar_run_tracks` both have
`p50_speed_mps`, `p85_speed_mps`, `p95_speed_mps`.

| Field           | SQLite | Web | PDF | macOS |
| --------------- | ------ | --- | --- | ----- |
| `p50_speed_mps` | ✅     | ❌  | ❌  | ❌    |
| `p85_speed_mps` | ✅     | ❌  | ❌  | ❌    |
| `p95_speed_mps` | ✅     | ❌  | ❌  | ❌    |

**Status:** These are **design debt, not missing wiring**. Per the
[speed percentile alignment plan](../../docs/plans/speed-percentile-aggregation-alignment-plan.md)
(D-18), percentiles are reserved for grouped/report aggregates only.
Per-track percentile columns are the wrong abstraction and should be
**removed** via migration 000030, not surfaced to more UIs.

The `ComputeSpeedPercentiles()` function in
`l6objects/classification.go:514` is used internally by the classifier
feature extraction. It should remain internal and not be exposed as a
public per-track field.

---

## Summary: Unpopulated Structures by Severity

### 🔴 Schema columns exist but are never written (9 columns)

| Table                 | Columns                                                                                                                                                   |
| --------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `lidar_analysis_runs` | `statistics_json` (1) — Go struct exists, computation implemented but not wired to persistence                                                            |
| `lidar_tracks`        | `track_length_meters`, `track_duration_secs`, `occlusion_count`, `max_occlusion_frames`, `spatial_coverage`, `noise_point_ratio` (6) — data available in `TrackedObject`, not wired to INSERT/UPDATE |
| `lidar_clusters`      | `noise_points_count`, `cluster_density`, `aspect_ratio` (3) — density/ratio computable at insert time; noise count requires L4 pipeline change            |

### 🟡 Structs computed in Go but never persisted or exposed (4 structs)

| Struct                   | File                   | Fields |
| ------------------------ | ---------------------- | ------ |
| `RunStatistics`          | `l6objects/quality.go` | 12     |
| `TrackQualityMetrics`    | `l6objects/quality.go` | 8      |
| `NoiseCoverageMetrics`   | `l6objects/quality.go` | 7      |
| `TrainingDatasetSummary` | `l6objects/quality.go` | 7      |

### 🟢 Feature vectors computed but no export path (2 structs)

| Struct            | File                    | Fields |
| ----------------- | ----------------------- | ------ |
| `ClusterFeatures` | `l6objects/features.go` | 10     |
| `TrackFeatures`   | `l6objects/features.go` | 20     |

### ⚪ Comparison logic implemented but no triggering endpoint

| Function               | File                             |
| ---------------------- | -------------------------------- |
| `compareParams()`      | `sqlite/analysis_run_compare.go` |
| `computeTemporalIoU()` | `sqlite/analysis_run_compare.go` |

### 🔵 Per-track speed percentile columns — design debt (see §10)

Per the [speed percentile alignment plan](../../docs/plans/speed-percentile-aggregation-alignment-plan.md),
per-track percentile columns (`p50_speed_mps`, `p85_speed_mps`,
`p95_speed_mps` on `lidar_tracks` and `lidar_run_tracks`) are the **wrong
abstraction** and should be removed via migration 000030. See
[schema simplification plan](../../docs/plans/schema-simplification-migration-030-plan.md).
