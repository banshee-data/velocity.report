# Design: Track Quality Score and Reason Codes (Feature 1)

**Status:** Proposed (February 2026)

## Objective

Provide a per-track quality score (`0-100`) with explainable reason codes so reviewers can:

- triage weak tracks first,
- understand why a track is low quality,
- track quality trends across runs and model revisions.

## Goals

- Produce deterministic score outputs for the same run data and scorer version.
- Keep scoring explainable with machine-readable reason codes.
- Make score usable in UI, queue ranking, and QC dashboard aggregations.

## Non-Goals

- Replacing human labels.
- Replacing downstream model quality metrics.
- Real-time online training.

## Inputs

- `lidar_run_tracks` per-track summary fields.
- `lidar_track_obs` per-observation kinematics.
- Physics violations from Feature 7.
- Event history from Feature 2.
- Repair actions from Feature 3.

## Scoring Model

Score range: `0-100`.

Initial component weights:

- Observation stability: 25
- Kinematic smoothness: 20
- Geometry consistency: 15
- Track continuity (split/merge risk, ID churn): 15
- Classification stability: 10
- Violation/repair penalties: 15

Formula:

- `score_raw = sum(component_i * weight_i)` where each component is normalized `0-1`.
- `score = clamp(round(score_raw), 0, 100)`.
- `quality_grade` buckets:
  - `A` >= 90
  - `B` 75-89
  - `C` 60-74
  - `D` 40-59
  - `F` < 40

## Reason Code Taxonomy

Initial reason codes:

- `LOW_HITS`
- `HIGH_MISSES`
- `HEADING_JUMP`
- `ACCEL_OUTLIER`
- `SIZE_DRIFT`
- `CLASS_FLIP`
- `SPLIT_RISK`
- `MERGE_RISK`
- `LOW_CONFIDENCE`
- `PHYSICS_VIOLATION`
- `MANUAL_REPAIR_REQUIRED`

Each reason code stores:

- severity (`info|warn|error`)
- source (`score_model|physics|repair`)
- evidence payload (small JSON)

## Data Model Changes

### Migration A: denormalized fields on `lidar_run_tracks`

Add columns:

- `quality_score REAL`
- `quality_grade TEXT`
- `quality_reason_codes TEXT` (JSON array)
- `quality_version TEXT`
- `quality_computed_at_ns INTEGER`

### Migration B: score history

Add table `lidar_run_track_quality_history`:

- `id INTEGER PRIMARY KEY AUTOINCREMENT`
- `run_id TEXT NOT NULL`
- `track_id TEXT NOT NULL`
- `quality_score REAL NOT NULL`
- `quality_grade TEXT NOT NULL`
- `reason_codes_json TEXT NOT NULL`
- `components_json TEXT NOT NULL`
- `quality_version TEXT NOT NULL`
- `computed_at_ns INTEGER NOT NULL`

Indexes:

- `(run_id, track_id, computed_at_ns DESC)`
- `(run_id, quality_score)`

## Backend Design

### Scorer package

Add `internal/lidar/qc/scoring.go`:

- pure scoring functions,
- versioned config,
- structured reason code output,
- deterministic rounding rules.

### Store integration

Add methods to `AnalysisRunStore` in `internal/lidar/analysis_run.go`:

- `UpdateTrackQuality(...)`
- `GetTrackQuality(runID, trackID)`
- `ListTrackQualitySummary(runID)`

### Recompute job

Add async recompute path:

- full recompute for run
- incremental recompute for one track after label/repair/violation updates

## API Contract

Add endpoints in `internal/lidar/monitor/run_track_api.go`:

- `GET /api/lidar/runs/{run_id}/tracks/{track_id}/quality`
- `POST /api/lidar/runs/{run_id}/quality/recompute`
- `GET /api/lidar/runs/{run_id}/quality/summary`

Extend `GET /api/lidar/runs/{run_id}/tracks` response with optional fields:

- `quality_score`
- `quality_grade`
- `quality_reason_codes`
- `quality_version`

## macOS UI Changes

Files:

- `tools/visualiser-macos/VelocityVisualiser/UI/ContentView.swift`
- `tools/visualiser-macos/VelocityVisualiser/App/AppState.swift`
- `tools/visualiser-macos/VelocityVisualiser/Labelling/RunTrackLabelAPIClient.swift`

UI additions:

- Track inspector quality card:
  - score badge,
  - grade,
  - top reason codes,
  - last computed timestamp/version.
- Track list row badge color by grade.
- Optional sort toggle: `quality asc|desc`.

## Web Parity

Files:

- `web/src/lib/types/lidar.ts`
- `web/src/lib/api.ts`
- `web/src/lib/components/lidar/TrackList.svelte`

Add optional quality fields and quality-sort/filter controls.

## Observability

Metrics:

- score compute latency (`p50`, `p95`)
- recompute queue depth
- reason-code frequency histogram
- score distribution per run

Logs:

- scorer version used
- top 5 lowest-scoring track IDs per run

## Rollout

- Phase 1: schema and backend scorer behind feature flag
- Phase 2: API exposure + internal validation against labelled runs
- Phase 3: macOS UI and queue integration
- Phase 4: web parity and dashboard consumption

## Task Checklist

### Data and Migrations

- [ ] Add denormalized quality fields to `lidar_run_tracks`
- [ ] Add `lidar_run_track_quality_history` table
- [ ] Add indexes for run-level sorting/filtering
- [ ] Add migration tests in `internal/db/migrate*_test.go`

### Backend

- [ ] Implement scorer package in `internal/lidar/qc/scoring.go`
- [ ] Add store methods in `internal/lidar/analysis_run.go`
- [ ] Add recompute worker and trigger hooks
- [ ] Update run-track serialization for new fields

### API

- [ ] Add quality endpoints to `internal/lidar/monitor/run_track_api.go`
- [ ] Add request/response validation
- [ ] Add API tests in `internal/lidar/monitor/run_track_api_test.go`

### macOS

- [ ] Extend `RunTrack` model in `RunTrackLabelAPIClient.swift`
- [ ] Render quality card in `TrackInspectorView`
- [ ] Add track list quality badges and sorting
- [ ] Add UI tests for quality rendering in `VelocityVisualiserTests`

### Web

- [ ] Extend TypeScript models in `web/src/lib/types/lidar.ts`
- [ ] Add API bindings in `web/src/lib/api.ts`
- [ ] Add quality sort/filter controls in `TrackList.svelte`

### Testing and Validation

- [ ] Unit tests for scoring components and reason-code generation
- [ ] Golden test for deterministic score output
- [ ] End-to-end test: label change triggers score refresh
- [ ] Performance test: recompute 10k tracks under target SLA

### Documentation

- [ ] Document score rubric and reason code meanings
- [ ] Add operator guidance for interpreting low-score tracks

## Acceptance Criteria

- Same run and scorer version produce identical score outputs.
- Every score below 75 includes at least one reason code.
- Track list can be sorted by quality score in under 150 ms for 2k tracks.
- Score recompute for a single track completes in under 500 ms.

## Open Questions

- Should quality score participate directly in model auto-tuner objective weights?
- Should grade thresholds be globally fixed or scene-profile specific?
- Do we need per-class score calibration (`car` vs `ped` vs `noise`)?
