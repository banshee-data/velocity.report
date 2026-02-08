# Design: Track Labelling, Ground Truth Evaluation & Label-Aware Auto-Tuning

**Status:** Approved design (February 2026) — Implementation not yet started.

**Immediate blocker:** Phase 1.1 — Register `LidarLabelAPI` routes in `WebServer.RegisterRoutes()`. The CRUD handlers already exist in `internal/api/lidar_labels.go` and the database table exists (migration 000016).

**Related documents:**

- Auto-tuning (Phases 1–2 implemented): [`../operations/auto-tuning.md`](../operations/auto-tuning.md)
- ML pipeline roadmap: [`../roadmap/ml_pipeline_roadmap.md`](../roadmap/ml_pipeline_roadmap.md)
- Tracking upgrades (6/9 implemented): [`../refactor/01-tracking-upgrades.md`](../refactor/01-tracking-upgrades.md)

Previous plan (bigger charts + sane auto-tuner defaults) is complete.

---

## Objective

Build a workflow for producing high-quality LiDAR tracks: a single vehicle counted once (handling occlusion and stopped vehicles), full velocity profile throughout the sensor's visible range, noise classification (trees, birds) excluded from vehicle profiles, and sparse points recovered along track paths. The system should support per-scene parameter profiles, manual track labelling, and label-aware auto-tuning using labelled ground truth.

The end goal is a `lidar_transits` table (analogous to `radar_data_transits`) for use in dashboards and reports.

---

## Current State (what exists)

### Database tables

- `lidar_tracks` — confirmed/tentative/deleted tracks with speed percentiles, quality metrics, classification
- `lidar_track_obs` — per-frame observations (position, velocity, bbox, heading)
- `lidar_run_tracks` — run-scoped tracks with label fields: `user_label`, `label_confidence`, `labeler_id`, `labeled_at`, `is_split_candidate`, `is_merge_candidate`, `linked_track_ids`
- `lidar_analysis_runs` — analysis sessions tied to a PCAP source + params
- `lidar_labels` — standalone labels (label_id, track_id, class_label) — migration 000016
- `lidar_clusters` — foreground clusters per frame

### Go layer

- `AnalysisRunManager` — creates runs, persists run tracks (`analysis_run_manager.go`, `analysis_run.go`)
- `UpdateTrackLabel()`, `UpdateTrackQualityFlags()`, `GetUnlabeledTracks()`, `GetLabelingProgress()` — DB methods exist but no REST API
- `RunComparison`, `TrackSplit`, `TrackMerge`, `TrackMatch` — **structs defined**, implementation pending (`analysis_run.go:245-280`)
- `LidarLabelAPI` — REST handlers exist (`internal/api/lidar_labels.go`) but **not registered** in `WebServer.RegisterRoutes()`
- Sweep runner replays PCAPs, applies params, collects metrics — no ground truth comparison
- `quality.go` — `TrackQualityMetrics`, `TrainingDataFilter` for quality scoring

### Svelte tracks UI (`web/src/routes/lidar/tracks/`)

- Canvas map + SVG timeline + TrackList sidebar
- Playback with scrubbing (10 Hz)
- Classification display (read-only) — no labelling controls
- Single hardcoded sensor — no PCAP/scene selection

### macOS visualiser

- `LabelAPIClient.swift` — REST calls to `/api/lidar/labels` (but backend not wired)
- Schema mismatch: Swift uses frame IDs, Go uses nanosecond timestamps
- Transport controls (play/pause/seek) for .vrlog replay

---

## Architecture

### Scene concept

A **scene** represents a specific environment captured in a PCAP:

```
lidar_scenes
  scene_id          TEXT PRIMARY KEY
  sensor_id         TEXT NOT NULL
  pcap_file         TEXT NOT NULL        -- relative path within PCAP safe dir
  pcap_start_secs   REAL
  pcap_duration_secs REAL
  description       TEXT
  reference_run_id  TEXT                 -- FK → lidar_analysis_runs (labelled ground truth)
  optimal_params_json TEXT               -- best-known params for this scene
  created_at_ns     INTEGER
  updated_at_ns     INTEGER
```

### Ground truth workflow

```
1. Select/create scene (PCAP + sensor)
2. Replay PCAP with current params → analysis run created
3. Review tracks in Svelte UI
4. Label tracks: good_vehicle | noise | split | merge | missed
5. Mark run as reference (reference_run_id)
6. Auto-tune: replay PCAP per combo → compare produced tracks to reference → score
7. Best params saved as optimal_params_json on scene
```

### Track matching algorithm (ground truth evaluation)

Given reference tracks R (labelled) and candidate tracks C (from a parameter combo):

1. **Temporal overlap**: For each pair (r, c), compute IoU = intersection / union of time ranges
2. **Spatial proximity**: During overlap period, compute mean centroid distance
3. **Match criterion**: IoU > 0.3 AND mean distance < 3m
4. **Hungarian assignment**: Optimal bipartite matching (reuse existing implementation)

Score components:

- **Detection rate** = |matched reference vehicles| / |reference vehicles labelled good_vehicle|
- **Fragmentation** = reference vehicles matched by >1 candidate track (lower = better)
- **False positive rate** = candidate tracks not matching any reference / total candidates
- **Velocity coverage** = fraction of reference track duration with matching candidate velocity data
- **Composite score** = w1 × detection - w2 × fragmentation - w3 × false_positives + w4 × velocity_coverage

### LiDAR transit table

```
lidar_transits
  transit_id              INTEGER PRIMARY KEY AUTOINCREMENT
  track_id                TEXT NOT NULL UNIQUE
  sensor_id               TEXT NOT NULL
  transit_start_unix      DOUBLE NOT NULL
  transit_end_unix        DOUBLE NOT NULL
  max_speed_mps           REAL
  min_speed_mps           REAL
  avg_speed_mps           REAL
  p50_speed_mps           REAL
  p85_speed_mps           REAL
  p95_speed_mps           REAL
  track_length_m          REAL
  observation_count       INTEGER
  object_class            TEXT
  classification_confidence REAL
  quality_score           REAL
  bbox_length_avg         REAL
  bbox_width_avg          REAL
  bbox_height_avg         REAL
  created_at              DOUBLE DEFAULT (UNIXEPOCH('subsec'))
```

Populated from confirmed `lidar_tracks` that pass `TrainingDataFilter` thresholds (min quality, min duration, min length).

---

## Phased checklist

### Phase 1: Wire up existing label infrastructure

> Get the existing but unregistered label API working end-to-end.

- [ ] **1.1** Register `LidarLabelAPI` routes in `WebServer.RegisterRoutes()` (`internal/lidar/monitor/webserver.go`)
- [ ] **1.2** Add `scene_id` and `source_file` columns to `lidar_labels` table (new migration 000017)
- [ ] **1.3** Update `LidarLabel` struct and handlers to support `scene_id`, `source_file` fields
- [ ] **1.4** Fix macOS `LabelAPIClient.swift` schema mismatch: align `startFrameID`/`endFrameID` with `start_timestamp_ns`/`end_timestamp_ns`
- [ ] **1.5** Add REST API endpoints for `lidar_run_tracks` labelling:
  - `PUT /api/lidar/runs/{run_id}/tracks/{track_id}/label` — set `user_label`, `label_confidence`
  - `PUT /api/lidar/runs/{run_id}/tracks/{track_id}/flags` — set `is_split_candidate`, `is_merge_candidate`, `linked_track_ids`
  - `GET /api/lidar/runs/{run_id}/tracks` — list run tracks with labels
  - `GET /api/lidar/runs/{run_id}/labeling-progress` — labelling statistics
- [ ] **1.6** Add REST API for analysis run management:
  - `GET /api/lidar/runs` — list runs (with filters: sensor_id, source_type, status)
  - `GET /api/lidar/runs/{run_id}` — get run details
  - `POST /api/lidar/runs/{run_id}/reprocess` — re-run analysis with different params

**Files:**

- `internal/lidar/monitor/webserver.go` (route registration)
- `internal/api/lidar_labels.go` (label struct + handlers)
- `internal/lidar/analysis_run.go` (run track API handlers)
- `internal/db/migrations/000017_*.up.sql` (new migration)
- `tools/visualiser-macos/VelocityVisualiser/Labelling/LabelAPIClient.swift`

### Phase 2: Scene management

> Introduce the scene concept tying PCAPs to reference runs and optimal params.

- [ ] **2.1** Create `lidar_scenes` table (migration 000018)
- [ ] **2.2** Create `SceneStore` with CRUD operations (`internal/lidar/scene_store.go`)
- [ ] **2.3** Add REST API for scenes:
  - `GET /api/lidar/scenes` — list scenes
  - `POST /api/lidar/scenes` — create scene from PCAP file
  - `GET /api/lidar/scenes/{scene_id}` — get scene with reference run + optimal params
  - `PUT /api/lidar/scenes/{scene_id}` — update description, reference_run_id, optimal_params
  - `DELETE /api/lidar/scenes/{scene_id}` — delete scene
  - `POST /api/lidar/scenes/{scene_id}/replay` — replay PCAP with given params, create analysis run
- [ ] **2.4** Connect scene creation to PCAP replay: when a PCAP is replayed for a scene, auto-create an analysis run and persist run tracks
- [ ] **2.5** Wire sweep/auto-tune to create analysis runs per combination (currently does not)

**Files:**

- `internal/lidar/scene_store.go` (new)
- `internal/lidar/monitor/scene_api.go` (new — REST handlers)
- `internal/lidar/monitor/webserver.go` (route registration)
- `internal/db/migrations/000018_*.up.sql`
- `internal/lidar/sweep/runner.go` (create runs per combo)

### Phase 3: Svelte track labelling UI

> Add labelling controls to the tracks page, plus scene/PCAP selection.

- [ ] **3.1** Add scene selector dropdown to tracks page header (replacing hardcoded sensor)
  - Fetches `GET /api/lidar/scenes` to populate
  - Selecting a scene loads its reference run's tracks
- [ ] **3.2** Add analysis run selector: dropdown to pick which run's tracks to view for the selected scene
- [ ] **3.3** Add labelling controls to `TrackList.svelte`:
  - Label buttons: good_vehicle, noise, split, merge, missed
  - Keyboard shortcuts (1-5) for rapid labelling
  - Visual indicator: labelled tracks show coloured badge
  - Unlabelled count / progress bar
- [ ] **3.4** Add bulk labelling: select multiple tracks (shift+click), apply label to all
- [ ] **3.5** Add split/merge annotation:
  - "Link tracks" mode: click two tracks to mark as split (should be one)
  - "Unlink track" mode: click track to mark as merge (should be separate)
  - Linked tracks shown with visual connector in timeline
- [ ] **3.6** Add label filtering: filter TrackList by label (unlabelled, good, noise, etc.)
- [ ] **3.7** Persist labels via `PUT /api/lidar/runs/{run_id}/tracks/{track_id}/label`
- [ ] **3.8** Show labelling progress bar (total / labelled / remaining)

**Files:**

- `web/src/routes/lidar/tracks/+page.svelte` (scene + run selectors)
- `web/src/lib/components/lidar/TrackList.svelte` (label controls)
- `web/src/lib/components/lidar/TimelinePane.svelte` (linked track connectors)
- `web/src/lib/components/lidar/MapPane.svelte` (label colour coding)
- `web/src/lib/api.ts` (new API calls)
- `web/src/lib/types/lidar.ts` (label types)

### Phase 4: Ground truth evaluation engine

> Implement track comparison: match candidate tracks against labelled reference tracks.

- [ ] **4.1** Implement `CompareRuns()` in `analysis_run.go` — populate the existing `RunComparison`, `TrackMatch`, `TrackSplit`, `TrackMerge` structs
- [ ] **4.2** Implement temporal-spatial matching algorithm:
  - Time overlap IoU computation
  - Mean centroid distance during overlap
  - Hungarian assignment for optimal matching
- [ ] **4.3** Implement ground truth scoring:
  - Detection rate, fragmentation, false positive rate, velocity coverage
  - Composite score with configurable weights
- [ ] **4.4** Add `GroundTruthEvaluator` struct (`internal/lidar/sweep/ground_truth.go`):
  - Takes reference_run_id + candidate run tracks
  - Returns `GroundTruthScore` (detection, fragmentation, FP rate, velocity coverage, composite)
- [ ] **4.5** Add REST API endpoint:
  - `POST /api/lidar/runs/{run_id}/evaluate` — compare run against scene's reference run, return score
  - `GET /api/lidar/scenes/{scene_id}/evaluations` — list all evaluation scores for a scene

**Files:**

- `internal/lidar/analysis_run.go` (CompareRuns implementation)
- `internal/lidar/sweep/ground_truth.go` (new — evaluator)
- `internal/lidar/monitor/scene_api.go` (evaluation endpoints)

### Phase 5: Label-aware auto-tuning

> Extend auto-tuner to use ground truth labels when a reference run exists.

- [ ] **5.1** Add `scene_id` field to `AutoTuneRequest`
- [ ] **5.2** When `scene_id` is set and the scene has a `reference_run_id`:
  - Each sweep combo creates an analysis run
  - After PCAP replay completes, run `GroundTruthEvaluator` against reference tracks
  - Use composite ground truth score as the objective function (instead of acceptance rate)
- [ ] **5.3** Modify sweep runner to clear tracks between combinations when ground truth mode is active
- [ ] **5.4** Add ground truth objective to sweep dashboard UI:
  - When auto-tune is selected with a scene that has labelled tracks, show "Ground Truth" objective option
  - Show ground truth score components in results table (detection %, fragmentation, FP rate)
- [ ] **5.5** Save best params as `optimal_params_json` on scene when auto-tune completes
- [ ] **5.6** Add "Apply Scene Params" button in sweep dashboard — loads optimal params for selected scene

**Files:**

- `internal/lidar/sweep/auto.go` (scene_id, ground truth objective)
- `internal/lidar/sweep/runner.go` (analysis run creation, track clearing)
- `internal/lidar/monitor/html/sweep_dashboard.html` (ground truth UI)
- `internal/lidar/scene_store.go` (save optimal params)

### Phase 6: LiDAR transit table

> Create the polished transit table for dashboards and reports.

- [ ] **6.1** Create `lidar_transits` table (migration 000019)
- [ ] **6.2** Add `TransitStore` with insert/query operations
- [ ] **6.3** Add transit promotion logic: when a confirmed track is finalised (deleted after grace period), if it passes quality thresholds → insert into `lidar_transits`
  - Quality thresholds from `TrainingDataFilter`: min quality >= 0.6, min duration >= 2s, min length >= 5m
- [ ] **6.4** Add REST API:
  - `GET /api/lidar/transits` — list transits with time range and speed filters
  - `GET /api/lidar/transits/summary` — aggregate stats (count, speed distribution, by class)
- [ ] **6.5** Add transit data to Svelte dashboard (chart, table) — reuse patterns from radar transit display

**Files:**

- `internal/db/migrations/000019_*.up.sql`
- `internal/lidar/transit_store.go` (new)
- `internal/lidar/tracking.go` (promotion on track deletion)
- `internal/lidar/monitor/transit_api.go` (new)
- `web/src/routes/lidar/dashboard/` (transit visualisation)

### Phase 7: Sparse point recovery (future)

> Post-processing to include sparse points along established track paths.

- [ ] **7.1** After track is confirmed, re-scan foreground points in the track's time window
- [ ] **7.2** For each unassociated foreground point: check if position + velocity are consistent with the track's predicted state at that timestamp
- [ ] **7.3** Include consistent points in the track's observation set (mark as "recovered")
- [ ] **7.4** Recompute track velocity profile with recovered points

This requires storing raw foreground points alongside clusters, which is not currently done. Consider adding a `lidar_foreground_points` table or extending the cluster export to include individual points.

**Files:**

- `internal/lidar/tracking.go` (post-processing pass)
- `internal/lidar/foreground.go` (point-level export)

### Phase 8: ML classification (future)

> Use labelled tracks to train classification models.

- [ ] **8.1** Feature extraction: speed profile, bbox dimensions, duration, heading changes, trajectory shape
- [ ] **8.2** Export labelled tracks as training data (CSV/JSON format for external ML)
- [ ] **8.3** Integrate trained model: load model weights, classify tracks in real-time
- [ ] **8.4** Classification confidence thresholds: auto-label high-confidence, flag low-confidence for review

This is deferred — the labelling infrastructure from Phases 1-3 provides the foundation for data collection.

---

## Key design decisions

### Label types for run tracks

Use `user_label` on `lidar_run_tracks` (not `lidar_labels` table) for ground truth. The labels are scoped to a specific analysis run, so run_tracks is the natural home:

| Label          | Meaning                                                                             |
| -------------- | ----------------------------------------------------------------------------------- |
| `good_vehicle` | Correctly tracked single vehicle                                                    |
| `noise`        | Not a real vehicle (tree, bird, sensor artefact)                                    |
| `split`        | Should be part of another track (fragmentation)                                     |
| `merge`        | Contains multiple vehicles (over-association)                                       |
| `missed`       | (label on a time/space region, not a track) — vehicle present but no track produced |

### Why scenes and not just PCAPs

A PCAP file is just bytes — a scene adds:

- Which sensor produced it
- Start offset and duration (subset of the PCAP)
- Reference run with labelled tracks
- Best-known parameter profile
- Human description of the environment

Different scenes from the same PCAP (e.g. different time segments) can have different optimal parameters.

### Track clearing between sweep combinations

Currently tracks persist across combinations. For ground truth evaluation, tracks **must** be cleared between combinations so each combo is evaluated independently. Add explicit `ClearTracks(sensorID)` call in the sweep runner before each PCAP replay when ground truth mode is active.

### Sweep runner → analysis run integration

Currently the sweep runner does not create analysis runs. Each combination should create a lightweight analysis run so tracks can be stored and compared. The run's `parent_run_id` should reference the scene's `reference_run_id` for traceability.

---

## Suggested implementation order

Phases 1-3 can be worked on partly in parallel:

- Phase 1 (label infra) is prerequisite for everything
- Phase 2 (scenes) and Phase 3 (Svelte UI) can interleave
- Phase 4 (evaluation) requires Phases 1+2
- Phase 5 (auto-tune) requires Phase 4
- Phase 6 (transits) is independent, can happen any time after Phase 1
- Phases 7-8 are future work

---

## Verification

1. `make test-go && make lint-go` passes after each phase
2. Phase 1: `curl POST /api/lidar/labels` creates a label; `curl GET /api/lidar/labels` returns it
3. Phase 2: Create scene via API, replay PCAP, verify analysis run created
4. Phase 3: Open tracks page, select scene, label tracks, verify labels persist on reload
5. Phase 4: Label reference run, create second run with different params, evaluate and get score
6. Phase 5: Run auto-tune with scene_id, verify ground truth scores in results table
7. Phase 6: Confirm a track, verify it appears in `lidar_transits` table and API
