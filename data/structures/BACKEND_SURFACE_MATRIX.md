# Backend Surface Matrix

Complete mapping of components, API endpoints, database tables, data
structures, and pipeline stages across the velocity.report codebase. Shows
which surfaces consume each item: **DB** (SQLite persistence), **Web**
(Svelte UI on `:8080`), **PDF** (Python LaTeX generator), **Mac** (Metal
visualiser via gRPC).

**Source:** Full-codebase audit (March 2026)
**Related:** [Remediation plan](../../docs/plans/unpopulated-data-structures-remediation-plan.md) · [Clustering observability](../../docs/plans/lidar-clustering-observability-and-benchmark-plan.md) · [HINT metric observability](../../docs/plans/hint-metric-observability-plan.md)

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
**DB column:** `lidar_analysis_runs.statistics_json` — 🗄️ column exists but **never written**

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

**Status (March 2026):** `ComputeRunStatistics()` is implemented and tested
but **never called from production code**. `CompleteRun()` writes basic
`AnalysisStats` (duration, frames, clusters, tracks, confirmed count,
processing time) but does not invoke `ComputeRunStatistics()`. The
`statistics_json` column exists in the schema (migration 000012) but
remains NULL for all rows.

---

## 2. Track Quality Metrics (per-track)

**Go source:** `internal/lidar/l6objects/quality.go` — `TrackQualityMetrics` struct (8 fields)
**Computation:** `ComputeTrackQualityMetrics(track)` — fully implemented, tested

| Field                  | SQLite | Web | PDF | macOS |
| ---------------------- | ------ | --- | --- | ----- |
| `track_id`             | ✅     | ✅  | ⬜  | ✅    |
| `track_length_meters`  | ✅     | ❌  | ❌  | ✅    |
| `track_duration_secs`  | ✅     | ❌  | ❌  | ✅    |
| `occlusion_count`      | ✅     | ❌  | ❌  | ✅    |
| `max_occlusion_frames` | ✅     | ❌  | ❌  | ❌    |
| `spatial_coverage`     | ✅     | ❌  | ❌  | ❌    |
| `noise_point_ratio`    | ✅     | ❌  | ❌  | ❌    |
| `quality_score`        | ❌     | ❌  | ❌  | ❌    |

**Status (March 2026):** `InsertTrack()` and `UpdateTrack()` now write all 6
quality columns. The `TrackedObject` fields are populated by the L5 tracker
(`ComputeQualityMetrics()`). `track_length_meters`, `track_duration_secs`,
and `occlusion_count` are also sent via gRPC and rendered by the macOS
visualiser. Web/PDF surface exposure is pending for all quality fields.
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
| `cluster_density`    | ✅     | ❌  | ❌  | ❌    |
| `aspect_ratio`       | ✅     | ❌  | ❌  | ❌    |

**Status (March 2026):** `InsertCluster()` now computes and writes
`cluster_density` (points/volume) and `aspect_ratio` (length/width).
`noise_points_count` remains unwritten — requires upstream noise-point
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

## 11. Classification Pipeline — Fully Wired (Reference)

**Go source:** `internal/lidar/l6objects/classification.go` — `TrackClassifier` (27 usages)

| Component                 | SQLite | Web | PDF | macOS |
| ------------------------- | ------ | --- | --- | ----- |
| `ObjectClass` (8 classes) | ✅     | ✅  | ⬜  | ✅    |
| `ClassificationResult`    | ✅     | ✅  | ⬜  | ✅    |
| `ClassificationFeatures`  | ❌     | ❌  | ❌  | 🔶    |
| `TrackClassifier`         | ⬜     | ✅  | ⬜  | ✅    |

**Notes:**

- `ObjectClass` and `ClassificationResult` (class + confidence) flow
  through the full pipeline: tracker → DB → Web API → gRPC → Mac.
- `ClassificationFeatures` (9 spatial/kinematic inputs) are only exposed
  via the gRPC `classifyOrConvert()` replay path — the Mac visualiser can
  see them during VRLOG playback but they are never persisted.
- `TrackClassifier` itself is set on the WebServer (for UI display) and
  on the gRPC server (for VRLOG replay classification), but is a service
  object, not persisted data.

---

## 12. FrameBundle — macOS-Only Proto Fields

**Proto:** `proto/velocity_visualiser/v1/visualiser.proto` — `FrameBundle` message
**Consumer:** macOS Metal visualiser via `StreamFrames` gRPC stream (~30 fps)

These fields are **only** consumed by the macOS visualiser. They are not
persisted to SQLite, not exposed to the Web UI, and not used by the PDF
generator.

| Field Group                | Field Count | macOS | SQLite | Web | PDF |
| -------------------------- | ----------- | ----- | ------ | --- | --- |
| Point cloud (x/y/z/i/c)    | 7           | ✅    | ❌     | ❌  | ❌  |
| Background snapshot (M3.5) | 6           | ✅    | ❌     | ❌  | ❌  |
| Cluster OBB (7-DOF)        | 7           | ✅    | ❌     | ❌  | ❌  |
| Track trails               | 3           | ✅    | ❌     | ❌  | ❌  |
| Track alpha/opacity        | 1           | ✅    | ❌     | ❌  | ❌  |
| Covariance 4×4             | 1           | ✅    | ❌     | ❌  | ❌  |
| Debug: associations        | 4           | ✅    | ❌     | ❌  | ❌  |
| Debug: gating ellipses     | 5           | ✅    | ❌     | ❌  | ❌  |
| Debug: residuals           | 5           | ✅    | ❌     | ❌  | ❌  |
| Debug: predictions         | 5           | ✅    | ❌     | ❌  | ❌  |
| Playback info              | 8           | ✅    | ❌     | ❌  | ❌  |
| Coordinate frame           | 6           | ✅    | ❌     | ❌  | ❌  |

**Status:** These are intentionally Mac-only — they serve real-time
visualisation and debugging, not analysis or reporting. No wiring gap.

---

## 13. ECharts Dashboard Endpoints

**Go source:** `internal/lidar/monitor/chart_api.go` + `webserver.go`
**Consumer:** Embedded ECharts dashboards (served from `/assets/*` via `go:embed`)

| Endpoint                      | Method | Data Source       | Web | macOS |
| ----------------------------- | ------ | ----------------- | --- | ----- |
| `/api/lidar/chart/polar`      | GET    | Background grid   | ✅  | ❌    |
| `/api/lidar/chart/heatmap`    | GET    | Background grid   | ✅  | ❌    |
| `/api/lidar/chart/foreground` | GET    | Foreground points | ✅  | ❌    |
| `/api/lidar/chart/clusters`   | GET    | Cluster positions | ✅  | ❌    |
| `/api/lidar/chart/traffic`    | GET    | Track activity    | ✅  | ❌    |

**Notes:** These endpoints return ECharts-compatible JSON for the debug
dashboards at `/debug/lidar/*`. They are consumed by embedded HTML pages
(not the Svelte SPA). The Svelte frontend uses LayerChart for its own
charts (e.g. `RadarOverviewChart.svelte` consuming `/api/radar_stats`).

---

## 14. cmd/ Entry Points

| Binary              | Location                              | Consumers                                      |
| ------------------- | ------------------------------------- | ---------------------------------------------- |
| `velocity-report`   | `cmd/radar/radar.go`                  | Full server: API, DB, serial, LiDAR pipeline   |
| `velocity-sweep`    | `cmd/sweep/main.go`                   | LiDAR monitor, sweep engine, PCAP replay       |
| `velocity-deploy`   | `cmd/deploy/main.go`                  | Deployment: install, upgrade, backup, rollback |
| `transit-backfill`  | `cmd/transit-backfill/main.go`        | DB: TransitWorker backfill                     |
| `gen-vrlog`         | `cmd/tools/gen-vrlog/main.go`         | Synthetic VRLOG generation (no DB)             |
| `vrlog-analyse`     | `cmd/tools/vrlog-analyse/main.go`     | VRLOG file analysis and comparison             |
| `visualiser-server` | `cmd/tools/visualiser-server/main.go` | Standalone gRPC server (synthetic/replay/live) |
| `settling-eval`     | `cmd/tools/settling-eval/main.go`     | Background grid settling evaluation            |

**Notes:** Only `velocity-report` and `transit-backfill` write to the
production SQLite database. The sweep and eval tools operate on
temporary/in-memory databases. The visualiser-server exposes the same
gRPC interface as the main server but can run standalone for development.

---

## 15. HTTP API Surface Summary

| Server              | Endpoints | Source                                |
| ------------------- | --------- | ------------------------------------- |
| Radar API (`:8080`) | 12        | `internal/api/server.go`              |
| LiDAR Monitor       | 61        | `internal/lidar/monitor/webserver.go` |
| **Total**           | **73**    |                                       |

**Sub-route dispatchers** add ~15 additional paths (sites CRUD,
site_config_periods, run track API with labelling/reprocessing/evaluation).

**Svelte frontend** consumes **51 API endpoints** via `web/src/lib/api.ts`.
**PDF generator** consumes **3 API endpoints** (`/api/radar_stats`,
`/api/sites/{id}`, `/api/site_config_periods`).

---

## Summary: Unpopulated Structures by Severity

### 🔴 Schema columns exist but are never written (14 columns)

| Table                 | Columns                                                       |
| --------------------- | ------------------------------------------------------------- |
| `lidar_analysis_runs` | `statistics_json` (1) — `ComputeRunStatistics()` never called |
| `lidar_clusters`      | `noise_points_count` (1) — requires L4 pipeline change        |

### 🟠 Persisted to SQLite but not surfaced to Web or PDF (8 fields)

| Table            | Columns                                                                                   |
| ---------------- | ----------------------------------------------------------------------------------------- |
| `lidar_tracks`   | `max_occlusion_frames`, `spatial_coverage`, `noise_point_ratio` (3) — needs API + web     |
| `lidar_clusters` | `cluster_density`, `aspect_ratio` (2) — needs API + web                                   |
| `lidar_tracks`   | `track_length_meters`, `track_duration_secs`, `occlusion_count` (3) — DB + Mac ✅, Web ❌ |

### 🟡 Structs computed in Go but never persisted or exposed (2 structs)

| Struct                   | File                   | Fields |
| ------------------------ | ---------------------- | ------ |
| `NoiseCoverageMetrics`   | `l6objects/quality.go` | 7      |
| `TrainingDatasetSummary` | `l6objects/quality.go` | 7      |

`RunStatistics` (12 fields) is implemented and tested but never called
from production code — see §1. `TrackQualityMetrics` fields ARE persisted
(6 of 8 fields written to `lidar_tracks`) — see §2.

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
