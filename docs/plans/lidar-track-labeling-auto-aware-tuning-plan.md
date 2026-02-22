# Design: Track Labelling, Ground Truth Evaluation & Label-Aware Auto-Tuning

Status: Planned
Purpose: Defines end-to-end track labelling and ground-truth-aware auto-tuning workflow, covering data model, APIs, review tooling, scoring logic, and phased implementation for higher-quality LiDAR tracking outputs.

**Status:** Approved design (February 2026) — **Phases 1-5 complete.** Phase 6 (transits) deferred; Phase 7 (missed regions) implemented; Phase 9 (profile comparison) partially implemented (data layer complete, UI pending). Remaining: 6.x (deferred), 7.x, 8.x, 9.5-9.7.

**Next blocker:** None for Phases 1-5. Phase 6 (transit promotion) is deferred pending design. Phase 9 (profile comparison) is next priority.

**Related documents:**

- Auto-tuning (Phases 1–2 implemented): [`../lidar/operations/auto-tuning.md`](../lidar/operations/auto-tuning.md)
- ML pipeline roadmap: [`docs/ROADMAP.md`](../ROADMAP.md)
- Tracking upgrades (6/9 implemented): [`../lidar/troubleshooting/01-tracking-upgrades.md`](../lidar/troubleshooting/01-tracking-upgrades.md)

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
- `lidar_run_tracks` — run-scoped tracks with label fields: `user_label`, `quality_label`, `label_confidence`, `labeler_id`, `labeled_at`, `linked_track_ids`
- `lidar_analysis_runs` — analysis sessions tied to a PCAP source + params
- `lidar_labels` — standalone labels (label_id, track_id, class_label) — migration 000016
- `lidar_clusters` — foreground clusters per frame

### Go layer

- `AnalysisRunManager` — creates runs, persists run tracks (`analysis_run_manager.go`, `analysis_run.go`)
- `UpdateTrackLabel()`, `UpdateTrackQualityFlags()`, `GetUnlabeledTracks()`, `GetLabelingProgress()` — DB methods + REST API implemented
- `RunComparison`, `TrackSplit`, `TrackMerge`, `TrackMatch` — **structs defined**, `CompareRuns()` implementation pending
- `LidarLabelAPI` — REST handlers registered in `WebServer.RegisterRoutes()`
- `SceneStore` — full CRUD for `lidar_scenes` table, REST API in `scene_api.go`
- `GroundTruthEvaluator` — temporal IoU matching, Hungarian assignment, composite scoring with 8-term formula (`ground_truth.go`)
- `TransitStore` — insert/query for `lidar_transits`, REST API in `transit_api.go`
- Sweep runner replays PCAPs, applies params, collects metrics — no ground truth comparison yet

### Svelte tracks UI (`web/src/routes/lidar/tracks/`)

- Canvas map + SVG timeline + TrackList sidebar
- Playback with scrubbing (10 Hz)
- Scene and run selector dropdowns in header
- Labelling controls: detection labels (1-8 keyboard), quality labels (Shift+1-5), label badges, label filtering, progress bar
- Labels persisted via `PUT /api/lidar/runs/{run_id}/tracks/{track_id}/label`

### macOS visualiser

- `LabelAPIClient.swift` — REST calls to `/api/lidar/labels` (backend routes registered)
- Schema mismatch: Swift uses frame IDs, Go uses nanosecond timestamps
- Transport controls (play/pause/seek) for .vrlog replay
- TrackInspectorView and LabelPanelView in side panel

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
4. Label tracks (two dimensions):
   - Detection: good_vehicle | good_pedestrian | good_other | noise | noise_flora | split | merge | missed
   - Quality: perfect | good | truncated | noisy_velocity | stopped_recovered
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

- **Detection rate** = |matched reference vehicles| / |reference vehicles labelled good\_\*|
  - Can be class-specific: separate rates for `good_vehicle`, `good_pedestrian`, `good_other`
- **Fragmentation** = reference vehicles matched by >1 candidate track (lower = better)
- **False positive rate** = candidate tracks not matching any reference / total candidates
- **Velocity coverage** = fraction of reference track duration with matching candidate velocity data
- **Quality premium** = fraction of matched `good_*` tracks with `quality_label = perfect`
- **Truncation rate** = fraction of matched tracks with `quality_label = truncated`
- **Velocity noise rate** = fraction of matched tracks with `quality_label = noisy_velocity`
- **Stopped recovery rate** = fraction of stopped vehicles with `quality_label = stopped_recovered`
- **Composite score** = w1 × detection_rate - w2 × fragmentation - w3 × false_positives + w4 × velocity_coverage + w5 × quality_premium - w6 × truncation_rate - w7 × velocity_noise_rate + w8 × stopped_recovery_rate

Default weights: w1=1.0, w2=5.0, w3=2.0, w4=0.5, w5=0.3, w6=0.4, w7=0.4, w8=0.2

The expanded formula gives the tuner explicit gradients for each failure mode. Without quality labels, the score plateaus once detection is high; with them, the tuner continues optimising for measurement quality.

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

- [x] **1.1** Register `LidarLabelAPI` routes in `WebServer.RegisterRoutes()` (`internal/lidar/monitor/webserver.go`)
- [x] **1.2** Add `quality_label` column to `lidar_run_tracks` table (migration 000018)
  - Enum values: `perfect`, `good`, `truncated`, `noisy_velocity`, `stopped_recovered`
- [x] **1.3** Add enum validation for `user_label` and `quality_label` in API handlers
  - `user_label` allowed: `good_vehicle`, `good_pedestrian`, `good_other`, `noise`, `noise_flora`, `split`, `merge`, `missed`
  - `quality_label` allowed: `perfect`, `good`, `truncated`, `noisy_velocity`, `stopped_recovered`
  - Reject requests with invalid label strings (prevent "Good_Vehicle" vs "good_vehicle" data quality issues)
- [x] **1.4** Add `scene_id` and `source_file` columns to `lidar_labels` table
  - Clarify: `lidar_labels` is for frame-level annotation (ML training), `lidar_run_tracks.user_label` is for track-level ground truth (auto-tuning)
- [x] **1.5** Fix macOS `LabelAPIClient.swift` schema mismatch: align `startFrameID`/`endFrameID` with `start_timestamp_ns`/`end_timestamp_ns`
- [x] **1.6** Add REST API endpoints for `lidar_run_tracks` labelling:
  - `PUT /api/lidar/runs/{run_id}/tracks/{track_id}/label` — set `user_label`, `quality_label`, `label_confidence`
  - `PUT /api/lidar/runs/{run_id}/tracks/{track_id}/flags` — set `linked_track_ids` (for split/merge)
  - `GET /api/lidar/runs/{run_id}/tracks` — list run tracks with labels
  - `GET /api/lidar/runs/{run_id}/labeling-progress` — labelling statistics
  - **Note:** Remove redundant `is_split_candidate`/`is_merge_candidate` boolean flags — use `user_label = split/merge` instead
- [x] **1.7** Add REST API for analysis run management:
  - `GET /api/lidar/runs` — list runs (with filters: sensor_id, source_type, status)
  - `GET /api/lidar/runs/{run_id}` — get run details
  - `POST /api/lidar/runs/{run_id}/reprocess` — re-run analysis with different params

**Files:**

- `internal/lidar/monitor/webserver.go` (route registration)
- `internal/api/lidar_labels.go` (label struct + handlers, add enum validation)
- `internal/lidar/analysis_run.go` (run track API handlers, add quality_label support)
- `internal/db/migrations/000017_*.up.sql` (quality_label column)
- `internal/db/migrations/000018_*.up.sql` (scene_id, source_file on lidar_labels)
- `tools/visualiser-macos/VelocityVisualiser/Labelling/LabelAPIClient.swift`

### Phase 2: Scene management

> Introduce the scene concept tying PCAPs to reference runs and optimal params.

- [x] **2.1** Create `lidar_scenes` table (migration 000020)
- [x] **2.2** Create `SceneStore` with CRUD operations (`internal/lidar/scene_store.go`)
- [x] **2.3** Add REST API for scenes:
  - `GET /api/lidar/scenes` — list scenes
  - `POST /api/lidar/scenes` — create scene from PCAP file
  - `GET /api/lidar/scenes/{scene_id}` — get scene with reference run + optimal params
  - `PUT /api/lidar/scenes/{scene_id}` — update description, reference_run_id, optimal_params
  - `DELETE /api/lidar/scenes/{scene_id}` — delete scene
  - `POST /api/lidar/scenes/{scene_id}/replay` — replay PCAP with given params, create analysis run
- [x] **2.4** Connect scene creation to PCAP replay: when a PCAP is replayed for a scene, auto-create an analysis run and persist run tracks
- [x] **2.5** Wire sweep/auto-tune to create analysis runs per combination (currently does not)

**Files:**

- `internal/lidar/scene_store.go` (new)
- `internal/lidar/monitor/scene_api.go` (new — REST handlers)
- `internal/lidar/monitor/webserver.go` (route registration)
- `internal/db/migrations/000019_*.up.sql`
- `internal/lidar/sweep/runner.go` (create runs per combo)

### Phase 3: Svelte track labelling UI

> Add labelling controls to the tracks page, plus scene/PCAP selection.

- [x] **3.1** Add scene selector dropdown to tracks page header (replacing hardcoded sensor)
  - Fetches `GET /api/lidar/scenes` to populate
  - Selecting a scene loads its reference run's tracks
- [x] **3.2** Add analysis run selector: dropdown to pick which run's tracks to view for the selected scene
- [x] **3.3** Add labelling controls to `TrackList.svelte`:
  - **Detection label buttons**: good_vehicle, good_pedestrian, good_other, noise, noise_flora, split, merge, missed
  - **Quality label buttons**: perfect, good, truncated, noisy_velocity, stopped_recovered
  - Keyboard shortcuts (1-8 for detection, Shift+1-5 for quality) for rapid labelling
  - Visual indicator: labelled tracks show coloured badge (detection) + quality icon
  - Unlabelled count / progress bar (tracks with detection label but no quality label counted as partially labelled)
- [x] **3.4** Add bulk labelling: select multiple tracks (shift+click), apply label to all
- [x] **3.5** Add split/merge annotation:
  - "Link tracks" mode: click two tracks to mark as split (should be one)
  - "Unlink track" mode: click track to mark as merge (should be separate)
  - Linked tracks shown with visual connector in timeline
- [x] **3.6** Add label filtering: filter TrackList by label (unlabelled, good, noise, etc.)
- [x] **3.7** Persist labels via `PUT /api/lidar/runs/{run_id}/tracks/{track_id}/label`
- [x] **3.8** Show labelling progress bar (total / labelled / remaining)

**Files:**

- `web/src/routes/lidar/tracks/+page.svelte` (scene + run selectors)
- `web/src/lib/components/lidar/TrackList.svelte` (label controls)
- `web/src/lib/components/lidar/TimelinePane.svelte` (linked track connectors)
- `web/src/lib/components/lidar/MapPane.svelte` (label colour coding)
- `web/src/lib/api.ts` (new API calls)
- `web/src/lib/types/lidar.ts` (label types)

### Phase 4: Ground truth evaluation engine

> Implement track comparison: match candidate tracks against labelled reference tracks.

- [x] **4.1** Implement `CompareRuns()` in `analysis_run.go` — populate the existing `RunComparison`, `TrackMatch`, `TrackSplit`, `TrackMerge` structs
- [x] **4.2** Implement temporal-spatial matching algorithm:
  - Time overlap IoU computation
  - Mean centroid distance during overlap
  - Hungarian assignment for optimal matching
- [x] **4.3** Implement ground truth scoring:
  - Detection rate (class-specific: vehicle/pedestrian/other), fragmentation, false positive rate, velocity coverage
  - Quality metrics: quality premium (fraction `perfect`), truncation rate, velocity noise rate, stopped recovery rate
  - Composite score with configurable weights (8 terms — see scoring formula above)
- [x] **4.4** Add `GroundTruthEvaluator` struct (`internal/lidar/ground_truth.go`):
  - Takes reference_run_id + candidate run tracks
  - Returns `GroundTruthScore` (detection rates by class, fragmentation, FP rate, velocity coverage, quality premium, truncation rate, velocity noise rate, stopped recovery rate, composite)
- [x] **4.5** Add REST API endpoint:
  - `POST /api/lidar/runs/{run_id}/evaluate` — compare run against scene's reference run, return score
  - `GET /api/lidar/scenes/{scene_id}/evaluations` — list all evaluation scores for a scene

**Files:**

- `internal/lidar/analysis_run.go` (CompareRuns implementation)
- `internal/lidar/sweep/ground_truth.go` (new — evaluator)
- `internal/lidar/monitor/scene_api.go` (evaluation endpoints)

### Phase 5: Label-aware auto-tuning

> Extend auto-tuner to use ground truth labels when a reference run exists.

- [x] **5.1** Add `scene_id` field to `AutoTuneRequest`
- [x] **5.2** When `scene_id` is set and the scene has a `reference_run_id`:
  - Each sweep combo creates an analysis run
  - After PCAP replay completes, run `GroundTruthEvaluator` against reference tracks
  - Use composite ground truth score as the objective function (instead of acceptance rate)
- [x] **5.3** Modify sweep runner to reset state between combinations when ground truth mode is active:
  - Clear all tracks via `ClearTracks(sensorID)`
  - **Reset background model** — replay PCAP from scratch (not just tracker state)
  - Each combo must evaluate independently; background state from previous combos must not bleed through
- [x] **5.4** Add ground truth objective to sweep dashboard UI:
  - When auto-tune is selected with a scene that has labelled tracks, show "Ground Truth" objective option
  - Show ground truth score components in results table: detection % by class, fragmentation, FP rate, quality premium, truncation rate, velocity noise rate, stopped recovery rate
- [x] **5.5** Save best params as `optimal_params_json` on scene when auto-tune completes
- [x] **5.6** Add "Apply Scene Params" button in sweep dashboard — loads optimal params for selected scene

**Files:**

- `internal/lidar/sweep/auto.go` (scene_id, ground truth objective)
- `internal/lidar/sweep/runner.go` (analysis run creation, track clearing)
- `internal/lidar/monitor/html/sweep_dashboard.html` (ground truth UI)
- `internal/lidar/scene_store.go` (save optimal params)

### Phase 6: LiDAR transit table _(deferred)_

> Create the polished transit table for dashboards and reports. **Deferred** — migration and code removed; to be re-implemented when transit promotion logic is designed.

- [ ] **6.1** Create `lidar_transits` table _(deferred — migration and code removed)_
- [ ] **6.2** Add `TransitStore` with insert/query operations _(deferred)_
- [ ] **6.3** Add transit promotion logic _(deferred)_
- [ ] **6.4** Add REST API for transits _(deferred)_
- [ ] **6.5** Add transit data to Svelte dashboard _(deferred)_

**Files:**

- `internal/db/migrations/000020_*.up.sql`
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

### Phase 9: Profile comparison system (design)

> Compare parameter profiles across runs for the same scene to find optimal tracker settings.

A **profile** = scene + parameter set → analysis run (track set). Comparing profiles means evaluating which parameter sets produce the best tracks for a given scene.

#### Data model

New table to persist evaluation results:

```sql
CREATE TABLE lidar_evaluations (
    evaluation_id TEXT PRIMARY KEY,
    scene_id TEXT NOT NULL,
    reference_run_id TEXT NOT NULL,
    candidate_run_id TEXT NOT NULL,
    detection_rate REAL,
    fragmentation REAL,
    false_positive_rate REAL,
    velocity_coverage REAL,
    quality_premium REAL,
    truncation_rate REAL,
    velocity_noise_rate REAL,
    stopped_recovery_rate REAL,
    composite_score REAL,
    params_json TEXT,           -- snapshot of params used for candidate run
    created_at INTEGER,
    FOREIGN KEY (scene_id) REFERENCES lidar_scenes(scene_id) ON DELETE CASCADE,
    FOREIGN KEY (reference_run_id) REFERENCES lidar_analysis_runs(run_id),
    FOREIGN KEY (candidate_run_id) REFERENCES lidar_analysis_runs(run_id)
);
CREATE UNIQUE INDEX idx_evaluations_pair ON lidar_evaluations(reference_run_id, candidate_run_id);
```

This replaces the current transient evaluation (POST returns score but doesn't persist). Stored results enable comparison without re-running evaluation.

#### Score comparison table

On the scenes page, when a scene has a `reference_run_id`, show a comparison table of all evaluated runs:

| Run ID | Params Summary | Detection % | Fragmentation | FP Rate | Vel. Coverage | Quality | Composite | Actions     |
| ------ | -------------- | ----------- | ------------- | ------- | ------------- | ------- | --------- | ----------- |
| abc123 | dist=2.5m ...  | 92.3%       | 0.05          | 3.1%    | 87.4%         | 0.82    | 0.874     | View / Diff |
| def456 | dist=3.0m ...  | 88.1%       | 0.02          | 5.2%    | 91.0%         | 0.79    | 0.841     | View / Diff |

"Evaluate" button per row calls `POST /api/lidar/runs/{run_id}/evaluate` and persists the result. Rows are sortable by any score column.

#### Parameter diff view

Select two runs to see a side-by-side diff:

- Left column: Run A params + scores
- Right column: Run B params + scores
- Highlight parameters that differ between the two runs
- Show score deltas with colour coding (green = improved, red = degraded)

This helps identify which parameter changes caused which score changes, providing intuition for manual tuning.

#### Visual overlay (future)

Render tracks from 2-3 runs on the same map with different colours for visual comparison. Each run's tracks would use a distinct colour palette (e.g. blue for reference, orange for candidate A, green for candidate B). Mismatches (missed detections, false positives, fragmentation) would be highlighted with markers.

#### REST API additions

- `GET /api/lidar/scenes/{scene_id}/evaluations` — list all persisted evaluation scores for a scene (currently 501)
- `POST /api/lidar/scenes/{scene_id}/evaluations` — trigger evaluation of a candidate run against the scene's reference
- `GET /api/lidar/evaluations/{evaluation_id}` — get detailed evaluation result
- `DELETE /api/lidar/evaluations/{evaluation_id}` — remove stale evaluation

#### Connection to auto-tuning

Once sweep creates analysis runs per combo (Phase 2.5), evaluation scores can be computed and persisted automatically after each replay. The profile comparison UI then becomes a way to browse sweep results with full ground truth context — the auto-tuner picks the best composite score, and the human reviewer can drill into why by comparing score components and parameter diffs.

#### Checklist

- [x] **9.1** Create `lidar_evaluations` table (new migration)
- [x] **9.2** Add `EvaluationStore` with Insert, ListByScene, Get, Delete
- [x] **9.3** Modify `POST /api/lidar/runs/{run_id}/evaluate` to persist results
- [x] **9.4** Implement `GET /api/lidar/scenes/{scene_id}/evaluations` (replace 501 stub)
- [ ] **9.5** Add score comparison table to scenes page
- [ ] **9.6** Add parameter diff view (select two runs to compare)
- [ ] **9.7** Visual multi-run overlay on map (future)

---

## Key design decisions

### Label types for run tracks

Track labelling uses **two independent label fields** on `lidar_run_tracks` for ground truth:

1. **`user_label`** — Detection correctness (is this the right object?)
2. **`quality_label`** — Measurement quality (how good is the track for this object?)

The labels are scoped to a specific analysis run, so run_tracks is the natural home. The `lidar_labels` table is reserved for frame-level annotation (ML training on raw sensor data), not track-level labelling (auto-tuning ground truth).

#### Detection Labels (`user_label`)

| Label             | Meaning                                                                        | Used in Scoring                                       |
| ----------------- | ------------------------------------------------------------------------------ | ----------------------------------------------------- |
| `good_vehicle`    | Correctly tracked single vehicle                                               | Detection rate (vehicle-specific)                     |
| `good_pedestrian` | Correctly tracked pedestrian                                                   | Detection rate (pedestrian-specific)                  |
| `good_other`      | Correctly tracked non-vehicle object (cyclist, etc.)                           | Detection rate (other-specific)                       |
| `noise`           | Not a real object — sensor artefact, multipath, ground clutter                 | False positive penalty                                |
| `noise_flora`     | Tree, bush, vegetation motion                                                  | False positive penalty (background tuning diagnostic) |
| `split`           | Fragment — should be part of another track (use `linked_track_ids` to specify) | Fragmentation penalty                                 |
| `merge`           | Over-association — contains multiple objects                                   | Merge penalty                                         |
| `missed`          | Object present but no track produced (see spatial anchoring below)             | Miss penalty (requires spatial/temporal reference)    |

**Rationale for expanded taxonomy:**

- **Class-specific detection labels** (`good_vehicle`, `good_pedestrian`, `good_other`): The classification system already distinguishes pedestrian/car/bird/other. Using a single `good_vehicle` label prevents measuring class-specific detection rates — the tuner can't tell if a parameter change improved car detection but degraded pedestrian detection.
- **Noise split** (`noise` vs `noise_flora`): The auto-tuner optimises background subtraction parameters that have different sensitivities to sensor artefacts (multipath, ground reflections) vs. environmental motion (trees swaying). A single `noise` label hides which noise type the parameters are failing on.

#### Quality Labels (`quality_label`)

| Label               | Meaning                                                                     | Used in Scoring                                           |
| ------------------- | --------------------------------------------------------------------------- | --------------------------------------------------------- |
| `perfect`           | Full transit, clean velocity profile, stable bbox, no gaps                  | Highest weight in velocity coverage & quality premium     |
| `good`              | Minor imperfections — small speed jumps, brief gap, slight bbox instability | Standard weight                                           |
| `truncated`         | Track starts or ends prematurely (not split — only track for this vehicle)  | Truncation penalty (background/confirmation tuning issue) |
| `noisy_velocity`    | Correct association but velocity profile is unreliable (jumps, oscillation) | Velocity noise penalty (process/measurement noise issue)  |
| `stopped_recovered` | Vehicle stopped and was correctly maintained through occlusion              | Bonus — rewards occlusion coasting parameter tuning       |

**Rationale for quality dimension:**

The auto-tuner needs to distinguish "correct but noisy" tracks from "correct and clean" tracks. Without quality labels, the composite score plateaus once detection rate is high, with no gradient to push toward better measurement quality. The quality labels provide explicit feedback on velocity reliability (critical for transit promotion, Phase 6 — deferred), truncation (slow track initiation/termination issues), and occlusion handling (stopped vehicle recovery).

#### Spatial Anchoring for `missed` Labels

The `missed` label requires spatial/temporal reference since no track exists. Add to `lidar_run_tracks` or create a separate `lidar_missed_regions` table:

```sql
-- Option 1: Add columns to lidar_run_tracks (for when track_id references a nearby track)
missed_region_x REAL
missed_region_y REAL
missed_time_start_ns INTEGER
missed_time_end_ns INTEGER

-- Option 2: Separate table for missed detections
CREATE TABLE lidar_missed_regions (
  region_id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  center_x REAL NOT NULL,
  center_y REAL NOT NULL,
  time_start_ns INTEGER NOT NULL,
  time_end_ns INTEGER NOT NULL,
  labeler_id TEXT,
  labeled_at INTEGER,
  notes TEXT,
  FOREIGN KEY (run_id) REFERENCES lidar_analysis_runs(run_id) ON DELETE CASCADE
);
```

The evaluator can then check whether candidate runs produce tracks covering those spatiotemporal regions.

### Why scenes and not just PCAPs

A PCAP file is just bytes — a scene adds:

- Which sensor produced it
- Start offset and duration (subset of the PCAP)
- Reference run with labelled tracks
- Best-known parameter profile
- Human description of the environment

Different scenes from the same PCAP (e.g. different time segments) can have different optimal parameters.

### State reset between sweep combinations

Currently tracks persist across combinations. For ground truth evaluation, **all state must be reset** between combinations so each combo is evaluated independently:

1. **Clear all tracks** via `ClearTracks(sensorID)`
2. **Reset background model** — reinitialise background grid from scratch
3. **Replay PCAP from beginning** — do not resume from previous position

The background model is stateful (per-cell averages, freeze timestamps, observation counts). If combo A's background state is reused for combo B, the evaluation is contaminated. Each combo must process the PCAP as if it's the first time the system has seen it.

Implementation: In `sweep/runner.go`, when `ground_truth_mode` is active, call `ResetBackgroundModel(sensorID)` and `ClearTracks(sensorID)` before each PCAP replay.

### Sweep runner → analysis run integration

Currently the sweep runner does not create analysis runs. Each combination should create a lightweight analysis run so tracks can be stored and compared. The run's `parent_run_id` should reference the scene's `reference_run_id` for traceability.

---

## Suggested implementation order

Phases 1-3 can be worked on partly in parallel:

- Phase 1 (label infra) is prerequisite for everything
- Phase 2 (scenes) and Phase 3 (Svelte UI) can interleave
- Phase 4 (evaluation) requires Phases 1+2
- Phase 5 (auto-tune) requires Phase 4
- Phase 6 (transits) is deferred
- Phases 7-8 are future work
- Phase 9 (profile comparison) requires Phases 4+5; designed but not yet implemented

---

## Verification

1. `make test-go && make lint-go` passes after each phase
2. Phase 1: `curl POST /api/lidar/labels` creates a label; `curl GET /api/lidar/labels` returns it
3. Phase 2: Create scene via API, replay PCAP, verify analysis run created
4. Phase 3: Open tracks page, select scene, label tracks, verify labels persist on reload
5. Phase 4: Label reference run, create second run with different params, evaluate and get score
6. Phase 5: Run auto-tune with scene_id, verify ground truth scores in results table
7. Phase 6: _(deferred)_
