# Pre-v0.5.0 LiDAR Schema Standardisation Plan

**Status:** Draft
**Layers:** Database, L3 Grid, L4 Perception, L5 Tracks, L6 Objects, L8 Analytics, L9 Endpoints (API + web)
**Related:** [Speed Percentile Aggregation Alignment Plan](speed-percentile-aggregation-alignment-plan.md), [v0.5.0 Backward Compatibility Shim Removal Plan](v050-backward-compatibility-shim-removal-plan.md), [DECISIONS.md D-19](../DECISIONS.md), [L7 Scene Plan](lidar-l7-scene-plan.md), [L8/L9/L10 Plan](lidar-l8-analytics-l9-endpoints-l10-clients-plan.md), [Tracks Table Consolidation Plan](lidar-tracks-table-consolidation-plan.md)

## Goal

Two coordinated migrations (000030 + 000031) that standardise the LiDAR schema
before v0.5.0:

1. **000030 — Column cleanup:** drop dead/NULL columns, rename
   `peak_speed_mps` → `max_speed_mps`, rename `world_frame` → `frame_id`,
   rename `scene_hash` → `grid_hash`.
2. **000031 — Table naming standardisation:** rename 7 tables into a coherent
   family model (`bg`, `track`, `run`, `replay_case`, `tuning`) and rename
   associated FK columns.

## Motivation

Schema audit (March 2026) found three categories of waste:

| Category               | Columns                                                                                                                                            | Status                                                                    |
| ---------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------- |
| **Dead writes**        | `p50_speed_mps`, `p85_speed_mps`, `p95_speed_mps` on `lidar_tracks`                                                                                | Written on INSERT/UPDATE, never selected                                  |
| **Always NULL**        | `track_length_meters`, `track_duration_secs`, `occlusion_count`, `max_occlusion_frames`, `spatial_coverage`, `noise_point_ratio` on `lidar_tracks` | Schema exists, Go code never writes them                                  |
| **Naming mismatch**    | `peak_speed_mps` on both tables                                                                                                                    | Radar uses `max_speed`; D-19 already decided rename                       |
| **Misapplied concept** | `p50_speed_mps`, `p85_speed_mps`, `p95_speed_mps` on `lidar_run_tracks`                                                                            | Stored and returned via API but no downstream consumer computes with them |

Per-track percentiles are the wrong abstraction (see speed-percentile plan §1):
percentiles are meaningful over a _population_ of tracks, not over one track's
Kalman-filtered speed history.

## Non-goals

- Dropping `height_p95` / `height_p95_max` — these are spatial filters (robust
  per-cluster height), not population statistics. They stay.
- Adding new track-level speed metrics (e.g. `speed_variance_mps`) — separate
  future work.
- Touching `lidar_track_obs` (beyond the `world_frame` rename).
- Radar table names — this plan is explicitly LiDAR-only.
- Merging live and analysis track tables — handled separately in
  [Tracks Table Consolidation Plan](lidar-tracks-table-consolidation-plan.md).
- Storage package reorganisation (`l8analytics/store/`, etc.) — separate
  follow-on work during L8 consolidation.
- `lidar_labels` layer ownership documentation — separate follow-on.

---

## Part 1: Naming Design

### Design Rules

1. Use full words by default, but allow entrenched short forms when they are
   already clear and low-risk. `bg` is an allowed exception.
2. Group tables by conceptual owner rather than one global prefix:
   - `lidar_bg_*` for L3 persisted background/grid state
   - `lidar_track_*` for live L5 track tables and direct children
   - `lidar_run_*` for artefacts owned by an executed analysis run
   - `lidar_replay_*` for saved replay fixtures and replay-scoped scores
   - `lidar_tuning_*` for optimisation sessions
3. Reserve `scene` for future L7 canonical scene work.
4. Prefer plural entity names for tables.
5. Keep already-good anchor names when renaming them would create unnecessary
   FK and API churn.

### Current Schema Inventory

| Current table          | Current role                              | Issue                           |
| ---------------------- | ----------------------------------------- | ------------------------------- |
| `lidar_bg_regions`     | L3 persisted region state                 | acceptable (`bg` exception)     |
| `lidar_bg_snapshot`    | L3 persisted grid snapshot                | acceptable                      |
| `lidar_clusters`       | L4 cluster persistence                    | acceptable                      |
| `lidar_tracks`         | live L5 track buffer                      | acceptable anchor name          |
| `lidar_track_obs`      | per-observation track state               | abbreviated child noun          |
| `lidar_labels`         | track-linked annotation spans             | overly generic                  |
| `lidar_analysis_runs`  | run metadata                              | breaks the `lidar_run_*` family |
| `lidar_run_tracks`     | run-scoped track snapshots                | acceptable run-owned child      |
| `lidar_scenes`         | saved replay fixture                      | collides with future L7 "scene" |
| `lidar_evaluations`    | run-vs-run scores scoped to a replay case | missing replay owner            |
| `lidar_missed_regions` | run-scoped missed-detection evidence      | owner is implicit               |
| `lidar_sweeps`         | tuning/sweep metadata                     | should align to tuning family   |

### Target Schema

After both migrations, the LiDAR schema reads as:

```text
lidar_bg_regions
lidar_bg_snapshot

lidar_clusters

lidar_tracks
lidar_track_observations
lidar_track_annotations

lidar_run_records
lidar_run_tracks
lidar_run_missed_regions

lidar_replay_cases
lidar_replay_evaluations

lidar_tuning_sweeps
```

### Why Keep `lidar_tracks`

The live-track table could be named `lidar_track_states`, but renaming it
before v0.5.0 buys little and costs a lot:

- `lidar_track_obs` and `lidar_labels` FK to it
- many docs already use `lidar_tracks` as the anchor name
- the main inconsistency is the surrounding family names, not the word `tracks`

### Why Keep Separate Physical Track Tables

This plan is naming + column cleanup only. It does **not** merge live and
analysis track tables. That question is handled separately in the
[Tracks Table Consolidation Plan](lidar-tracks-table-consolidation-plan.md).

---

## Part 2: Migration 000030 — Column Cleanup

### 000030_schema_simplification.up.sql

SQLite 3.35+ supports `ALTER TABLE ... DROP COLUMN` directly.

```sql
-- lidar_tracks: drop dead percentile columns
ALTER TABLE lidar_tracks DROP COLUMN p50_speed_mps;
ALTER TABLE lidar_tracks DROP COLUMN p85_speed_mps;
ALTER TABLE lidar_tracks DROP COLUMN p95_speed_mps;

-- lidar_tracks: drop always-NULL quality columns
ALTER TABLE lidar_tracks DROP COLUMN track_length_meters;
ALTER TABLE lidar_tracks DROP COLUMN track_duration_secs;
ALTER TABLE lidar_tracks DROP COLUMN occlusion_count;
ALTER TABLE lidar_tracks DROP COLUMN max_occlusion_frames;
ALTER TABLE lidar_tracks DROP COLUMN spatial_coverage;
ALTER TABLE lidar_tracks DROP COLUMN noise_point_ratio;

-- lidar_tracks: rename peak → max
ALTER TABLE lidar_tracks RENAME COLUMN peak_speed_mps TO max_speed_mps;

-- lidar_run_tracks: drop percentile columns
ALTER TABLE lidar_run_tracks DROP COLUMN p50_speed_mps;
ALTER TABLE lidar_run_tracks DROP COLUMN p85_speed_mps;
ALTER TABLE lidar_run_tracks DROP COLUMN p95_speed_mps;

-- lidar_run_tracks: rename peak → max
ALTER TABLE lidar_run_tracks RENAME COLUMN peak_speed_mps TO max_speed_mps;

-- rename world_frame → frame_id on three tables
ALTER TABLE lidar_clusters RENAME COLUMN world_frame TO frame_id;
ALTER TABLE lidar_tracks RENAME COLUMN world_frame TO frame_id;
ALTER TABLE lidar_track_obs RENAME COLUMN world_frame TO frame_id;

-- rename scene_hash → grid_hash on lidar_bg_regions
ALTER TABLE lidar_bg_regions RENAME COLUMN scene_hash TO grid_hash;
DROP INDEX idx_bg_regions_scene_hash;
CREATE INDEX idx_bg_regions_grid_hash ON lidar_bg_regions (grid_hash);
```

### 000030_schema_simplification.down.sql

```sql
-- lidar_tracks: restore percentile columns
ALTER TABLE lidar_tracks ADD COLUMN p50_speed_mps REAL;
ALTER TABLE lidar_tracks ADD COLUMN p85_speed_mps REAL;
ALTER TABLE lidar_tracks ADD COLUMN p95_speed_mps REAL;

-- lidar_tracks: restore quality columns
ALTER TABLE lidar_tracks ADD COLUMN track_length_meters REAL;
ALTER TABLE lidar_tracks ADD COLUMN track_duration_secs REAL;
ALTER TABLE lidar_tracks ADD COLUMN occlusion_count INTEGER DEFAULT 0;
ALTER TABLE lidar_tracks ADD COLUMN max_occlusion_frames INTEGER DEFAULT 0;
ALTER TABLE lidar_tracks ADD COLUMN spatial_coverage REAL;
ALTER TABLE lidar_tracks ADD COLUMN noise_point_ratio REAL;

-- lidar_tracks: restore peak name
ALTER TABLE lidar_tracks RENAME COLUMN max_speed_mps TO peak_speed_mps;

-- lidar_run_tracks: restore percentile columns
ALTER TABLE lidar_run_tracks ADD COLUMN p50_speed_mps REAL;
ALTER TABLE lidar_run_tracks ADD COLUMN p85_speed_mps REAL;
ALTER TABLE lidar_run_tracks ADD COLUMN p95_speed_mps REAL;

-- lidar_run_tracks: restore peak name
ALTER TABLE lidar_run_tracks RENAME COLUMN max_speed_mps TO peak_speed_mps;

-- restore world_frame
ALTER TABLE lidar_clusters RENAME COLUMN frame_id TO world_frame;
ALTER TABLE lidar_tracks RENAME COLUMN frame_id TO world_frame;
ALTER TABLE lidar_track_obs RENAME COLUMN frame_id TO world_frame;

-- restore scene_hash
ALTER TABLE lidar_bg_regions RENAME COLUMN grid_hash TO scene_hash;
DROP INDEX idx_bg_regions_grid_hash;
CREATE INDEX idx_bg_regions_scene_hash ON lidar_bg_regions (scene_hash);
```

### pcap-analyse: Drop Offline Percentiles (Confirmed)

The `pcap-analyse` tool computes per-track P50/P85/P95 for offline analysis
and writes them to the `lidar_run_tracks` table. With the columns dropped:

**Decision: Option A — Drop completely.** Remove `ComputeSpeedPercentiles()`
call and the struct fields. If offline percentile analysis is needed later,
it can use the TDL (Track Description Language) query layer planned in v0.5.1.

---

## Part 3: Migration 000031 — Table Naming Standardisation

### Tables to rename

| Current                | Proposed                   | Why                                                                  |
| ---------------------- | -------------------------- | -------------------------------------------------------------------- |
| `lidar_track_obs`      | `lidar_track_observations` | remove abbreviation                                                  |
| `lidar_labels`         | `lidar_track_annotations`  | make track ownership explicit; avoid collision with run-track labels |
| `lidar_analysis_runs`  | `lidar_run_records`        | align the run anchor with the `lidar_run_*` family                   |
| `lidar_missed_regions` | `lidar_run_missed_regions` | make run ownership explicit                                          |
| `lidar_scenes`         | `lidar_replay_cases`       | make replay-fixture role explicit; avoid L7 scene collision          |
| `lidar_evaluations`    | `lidar_replay_evaluations` | scores are persisted against a replay case                           |
| `lidar_sweeps`         | `lidar_tuning_sweeps`      | make the tuning-session owner explicit                               |

### Tables to keep unchanged

| Keep                | Why                                                            |
| ------------------- | -------------------------------------------------------------- |
| `lidar_bg_regions`  | `bg` is an allowed exception and already established           |
| `lidar_bg_snapshot` | paired naturally with `lidar_bg_regions`                       |
| `lidar_tracks`      | core live-track anchor; renaming would force unnecessary churn |
| `lidar_clusters`    | already clear and short; no conflicting family                 |
| `lidar_run_tracks`  | already reads correctly as a run-owned child table             |

### Column renames following table renames

- `scene_id` → `replay_case_id` on `lidar_replay_cases`, `lidar_replay_evaluations`,
  and any replay-case provenance columns

### 000031_table_naming.up.sql

```sql
-- L5 track children
ALTER TABLE lidar_track_obs RENAME TO lidar_track_observations;
ALTER TABLE lidar_labels RENAME TO lidar_track_annotations;

-- L8 run family
ALTER TABLE lidar_analysis_runs RENAME TO lidar_run_records;
ALTER TABLE lidar_missed_regions RENAME TO lidar_run_missed_regions;

-- L8 replay family
ALTER TABLE lidar_scenes RENAME TO lidar_replay_cases;
ALTER TABLE lidar_replay_cases RENAME COLUMN scene_id TO replay_case_id;
ALTER TABLE lidar_evaluations RENAME TO lidar_replay_evaluations;
ALTER TABLE lidar_replay_evaluations RENAME COLUMN scene_id TO replay_case_id;

-- L8 tuning
ALTER TABLE lidar_sweeps RENAME TO lidar_tuning_sweeps;
```

### 000031_table_naming.down.sql

```sql
-- L8 tuning
ALTER TABLE lidar_tuning_sweeps RENAME TO lidar_sweeps;

-- L8 replay family
ALTER TABLE lidar_replay_evaluations RENAME COLUMN replay_case_id TO scene_id;
ALTER TABLE lidar_replay_evaluations RENAME TO lidar_evaluations;
ALTER TABLE lidar_replay_cases RENAME COLUMN replay_case_id TO scene_id;
ALTER TABLE lidar_replay_cases RENAME TO lidar_scenes;

-- L8 run family
ALTER TABLE lidar_run_missed_regions RENAME TO lidar_missed_regions;
ALTER TABLE lidar_run_records RENAME TO lidar_analysis_runs;

-- L5 track children
ALTER TABLE lidar_track_annotations RENAME TO lidar_labels;
ALTER TABLE lidar_track_observations RENAME TO lidar_track_obs;
```

---

## Part 4: Go Code Changes

### Migration 000030 code changes

#### Storage layer (`internal/lidar/storage/sqlite/`)

**track_store.go** — Remove percentile columns from INSERT/UPDATE/SELECT:

- `InsertTrack()`: drop `p50_speed_mps, p85_speed_mps, p95_speed_mps` from UPSERT columns and ON CONFLICT SET
- `UpdateTrack()`: drop percentile SET clauses
- `GetActiveTracks()`, `GetTracksInRange()`: already don't select them — no change
- Rename `peak_speed_mps` → `max_speed_mps` in all SQL strings
- Rename `world_frame` → `frame_id` in all SQL strings; rename `WorldFrame` Go field → `FrameID`

**cluster_store.go** — Rename `world_frame` → `frame_id` in SQL strings;
rename `WorldFrame` Go field → `FrameID`.

**bg_region_store.go** / **bg_store.go** — Rename `scene_hash` → `grid_hash`
in SQL and index references; rename `SceneHash` Go field → `GridHash`.

**analysis_run.go** — Remove percentile columns, rename peak:

- `RunTrack` struct: remove `P50SpeedMps`, `P85SpeedMps`, `P95SpeedMps` fields; rename `PeakSpeedMps` → `MaxSpeedMps`
- `RunTrackFromTrackedObject()`: remove percentile population; rename `PeakSpeedMps` → `MaxSpeedMps`
- `InsertRunTrack()`: drop percentile columns from INSERT SQL
- `GetRunTracks()`, `GetRunTrack()`: drop percentile columns from SELECT and `Scan()` calls
- `ExportTracksCSV()` (if exists): update column headers

**analysis_run_manager.go** — Update any `RecordTrack()` calls that reference removed fields.

#### Tracking layer (`internal/lidar/l5tracks/`)

**tracking.go** — Rename struct field:

- `TrackedObject.PeakSpeedMps` → `TrackedObject.MaxSpeedMps`
- `TrackedObject.WorldFrame` → `TrackedObject.FrameID`
- Update speed comparison logic: `if speed > track.PeakSpeedMps` → `if speed > track.MaxSpeedMps`

#### Classification layer (`internal/lidar/l6objects/`)

**classification.go** — Rename field references:

- `ClassificationFeatures.PeakSpeed` → `ClassificationFeatures.MaxSpeed`

**features.go** — Rename field:

- `TrackFeatures.PeakSpeedMps` → `TrackFeatures.MaxSpeedMps`
- Update CSV export header from `"peak_speed_mps"` to `"max_speed_mps"`

#### API layer (`internal/lidar/monitor/`)

**track_api.go** — Rename JSON fields:

- `TrackSummary.PeakSpeedMps` JSON tag: `"peak_speed_mps"` → `"max_speed_mps"`
- `ClassStats.PeakSpeedMps` JSON tag: `"peak_speed_mps"` → `"max_speed_mps"`

#### PCAP analysis tool (`cmd/tools/pcap-analyse/`)

**main.go** — Rename and drop:

- `TrackExport.PeakSpeedMps` → `TrackExport.MaxSpeedMps`
- Remove `P50SpeedMps`, `P85SpeedMps`, `P95SpeedMps` from struct and SQL INSERT
- Update CSV headers
- Remove `ComputeSpeedPercentiles()` call

#### Proto (`proto/velocity_visualiser/v1/`)

**visualiser.proto** — Rename field:

- `float peak_speed_mps = 25;` → `float max_speed_mps = 25;` (same field number, rename only)
- Regenerate `visualiser.pb.go`

**Swift** — Regenerate and update `PeakSpeedMps` → `MaxSpeedMps` references.

#### Web frontend (`web/src/`)

**lib/types/lidar.ts** — Rename type fields:

- `peak_speed_mps` → `max_speed_mps` on track and run-track types
- Remove `p50_speed_mps`, `p85_speed_mps`, `p95_speed_mps` from run-track type

**Components** — Update display references from "Peak" to "Max" in labels.

### Migration 000031 code changes

#### Storage layer (`internal/lidar/storage/sqlite/`)

- All SQL referencing `lidar_track_obs` → `lidar_track_observations`
- All SQL referencing `lidar_labels` → `lidar_track_annotations`
- All SQL referencing `lidar_analysis_runs` → `lidar_run_records`
- All SQL referencing `lidar_missed_regions` → `lidar_run_missed_regions`
- All SQL referencing `lidar_scenes` → `lidar_replay_cases`; `scene_id` → `replay_case_id`
- All SQL referencing `lidar_evaluations` → `lidar_replay_evaluations`
- All SQL referencing `lidar_sweeps` → `lidar_tuning_sweeps`

#### Go struct / type renames

- `SceneStore` → `ReplayCaseStore`; `Scene` → `ReplayCase`; `SceneID` → `ReplayCaseID`
- `AnalysisRun` struct references → `RunRecord` (if applicable)
- Update FK field names where `scene_id` appeared

#### API paths (`internal/lidar/monitor/`)

- `/api/lidar/scenes/` → `/api/lidar/replay-cases/`
- Update handler registrations and route constants

#### Web frontend

- Update TypeScript type definitions for renamed tables and API paths
- Update any UI references

---

## Part 5: Testing

- `make test-go` must pass after all renames
- Verify both migrations apply cleanly on a fresh DB and on a DB with existing data
- `make test-web` passes after TypeScript type changes
- `make schema-erd` to regenerate ERD after migrations

## Risk

- **Low:** All dropped columns are either never-read or always-NULL
- **Medium:** `peak` → `max` rename touches ~15 Go, ~3 TS, 1 proto, ~5 Swift files
- **Medium:** 7 table renames touch SQL strings across ~12 Go files + API paths + web types
- **Mitigation:** v0.5.0 is the coordinated breaking-change release — one clean
  cut, no long-lived aliases

## Sequencing

1. **Migration 000030** — column drops, column renames (`peak`→`max`,
   `world_frame`→`frame_id`, `scene_hash`→`grid_hash`)
2. **Migration 000031** — 7 table renames + `scene_id`→`replay_case_id`
3. Both land in the same release window (v0.5.0)

## Cross-references

- [L7 Scene Plan](lidar-l7-scene-plan.md) — defines what L7 Scene will
  actually be (persistent canonical world model)
- [L8 Analytics / L9 Endpoints / L10 Client Plan](lidar-l8-analytics-l9-endpoints-l10-clients-plan.md) —
  covers `analysis_run.go` ownership, storage reorganisation, `l9endpoints`
  rename
- [Layer Model](../lidar/architecture/lidar-data-layer-model.md) — frozen
  L1–L10 numbering from v0.5.0
- [Tracks Table Consolidation Plan](lidar-tracks-table-consolidation-plan.md) —
  separate plan for live vs analysis track table structure
