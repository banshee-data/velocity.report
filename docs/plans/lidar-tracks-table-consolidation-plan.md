# LiDAR Tracks Table Consolidation Plan

- **Status:** Proposed
- **Layers:** Database, L5 Tracks, L8 Analytics, L9 Endpoints
- **Prerequisite:** [Schema Simplification Migration 030](schema-simplification-migration-030-plan.md)
- **Related:** [L8/L9/L10 Refactor](lidar-l8-analytics-l9-endpoints-l10-clients-plan.md), [Speed Percentile Alignment](speed-percentile-aggregation-alignment-plan.md), [Analysis Run Infrastructure](lidar-analysis-run-infrastructure-plan.md)

## Problem

The schema contains two tables with heavily overlapping column sets:

| Table              | Purpose                                             | PK                   |
| ------------------ | --------------------------------------------------- | -------------------- |
| `lidar_tracks`     | Live transient tracking buffer (pruned ~5 min)      | `track_id`           |
| `lidar_run_tracks` | Immutable versioned snapshots tied to analysis runs | `(run_id, track_id)` |

After migration 030 lands, the column overlap looks like this:

| Category                    | Count | Columns                                                                                                                                                                                                                                                                                                                  |
| --------------------------- | ----- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **Shared (identical)**      | 16    | `track_id`, `sensor_id`, `track_state`, `start_unix_nanos`, `end_unix_nanos`, `observation_count`, `avg_speed_mps`, `max_speed_mps`, `bounding_box_length_avg`, `bounding_box_width_avg`, `bounding_box_height_avg`, `height_p95_max`, `intensity_mean_avg`, `object_class`, `object_confidence`, `classification_model` |
| **Only `lidar_tracks`**     | 1     | `world_frame` (L2 frame identifier; may rename to `frame_id` per migration-030 extension E2)                                                                                                                                                                                                                             |
| **Only `lidar_run_tracks`** | 10    | `run_id`, `user_label`, `label_confidence`, `labeler_id`, `labeled_at`, `quality_label`, `label_source`, `is_split_candidate`, `is_merge_candidate`, `linked_track_ids`                                                                                                                                                  |

The 16 shared columns are defined in two `CREATE TABLE` statements, written by
two separate Go code paths (`InsertTrack` / `InsertRunTrack`), and mapped to two
Go structs (`TrackedObject` / `RunTrack`). This violates DRY across three layers:
SQL schema, Go storage, and Go model.

### How the duplication arose

`lidar_tracks` (migration 000010) was introduced for real-time L5 tracking.
`lidar_run_tracks` (migration 000011) was added later for the analysis-run
infrastructure, which needed point-in-time snapshots with ML labelling
metadata. Because live tracks are pruned after ~5 minutes, the snapshot table
copies all measurement columns so the data survives track deletion.

## Current data flow

```
                          ┌─────────────────────────┐
Tracker (per frame) ──────┤ sqlite.InsertTrack()    │──▶ lidar_tracks
                          │ sqlite.InsertTrackObs() │──▶ lidar_track_obs
                          └─────────┬───────────────┘
                                    │ if analysis run active
                                    ▼
                          ┌─────────────────────────┐
RunTrackFromTrackedObject │ store.InsertRunTrack()  │──▶ lidar_run_tracks
                          └─────────────────────────┘

FK children of lidar_tracks:
  lidar_track_obs  (track_id) ON DELETE CASCADE
  lidar_labels     (track_id) ON DELETE CASCADE

FK parent of lidar_run_tracks:
  lidar_analysis_runs (run_id) ON DELETE CASCADE
```

There are **no JOINs** between the two tables anywhere in the codebase. They
are accessed through completely separate code paths and serve different
consumers:

- `lidar_tracks` → `TrackAPI` (real-time dashboard, live tracks endpoint)
- `lidar_run_tracks` → `AnalysisRunStore` (run tracks, labelling, sweeps, evaluation)

## Consolidation options

### Option A — Merge into single table with nullable `run_id`

Merge both tables into a unified `lidar_tracks` with a nullable `run_id` column.
Live tracks have `run_id IS NULL`; snapshot rows have `run_id = <analysis-run-id>`.

**Schema (post-030):**

```sql
CREATE TABLE lidar_tracks (
    track_id TEXT NOT NULL,
    run_id TEXT,                            -- NULL = live track; non-NULL = run snapshot
    sensor_id TEXT NOT NULL,
    frame_id TEXT,                          -- non-NULL for live; NULL for snapshots
    track_state TEXT NOT NULL,
    start_unix_nanos INTEGER NOT NULL,
    end_unix_nanos INTEGER,
    observation_count INTEGER,
    avg_speed_mps REAL,
    max_speed_mps REAL,
    bounding_box_length_avg REAL,
    bounding_box_width_avg REAL,
    bounding_box_height_avg REAL,
    height_p95_max REAL,
    intensity_mean_avg REAL,
    object_class TEXT,
    object_confidence REAL,
    classification_model TEXT,
    -- Run-snapshot-only fields (NULL for live tracks)
    user_label TEXT,
    label_confidence REAL,
    labeler_id TEXT,
    labeled_at INTEGER,
    quality_label TEXT,
    label_source TEXT,
    is_split_candidate INTEGER DEFAULT 0,
    is_merge_candidate INTEGER DEFAULT 0,
    linked_track_ids TEXT,
    PRIMARY KEY (track_id, COALESCE(run_id, '')),
    FOREIGN KEY (run_id) REFERENCES lidar_analysis_runs(run_id) ON DELETE CASCADE
);
```

**Problem:** SQLite does not support expressions in `PRIMARY KEY`. The real PK
strategy must be one of:

- **A1 — Surrogate PK:** `id INTEGER PRIMARY KEY AUTOINCREMENT` with a `UNIQUE`
  index on `(track_id, run_id)`. Breaks the FK from `lidar_track_obs` (currently
  FK on `track_id`) — would need a column change or filter constraint.
- **A2 — Sentinel value:** Use `run_id TEXT NOT NULL DEFAULT '__live__'` instead
  of NULL. PK becomes `(track_id, run_id)`. The FK to `lidar_analysis_runs`
  needs an exception for the sentinel, which means dropping the FK constraint
  and enforcing run validity in application code.
- **A3 — Composite PK with NULL:** SQLite technically allows NULLs in a composite
  PK (unlike PostgreSQL), so `PRIMARY KEY (track_id, run_id)` is valid. But
  `lidar_track_obs.track_id` FK currently references `lidar_tracks(track_id)`,
  which assumes `track_id` alone is unique — it would no longer be.

All three sub-options require rewriting the FK from `lidar_track_obs` and
`lidar_labels` to scope to live-only rows. This is the dominant migration cost.

| Aspect       | Pros                                                              | Cons                                                                                         |
| ------------ | ----------------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| DRY          | Single column definition; single Go struct                        | —                                                                                            |
| Queries      | Can query across live + historical in one SELECT                  | Must add `WHERE run_id IS [NOT] NULL` to every existing query                                |
| Pruning      | `DELETE WHERE run_id IS NULL AND track_state = 'deleted' AND ...` | Must never accidentally prune snapshot rows                                                  |
| FK integrity | —                                                                 | `lidar_track_obs` and `lidar_labels` FK must change from `track_id` → composite or surrogate |
| Performance  | —                                                                 | Hot upsert table now contains immutable snapshot rows; table size grows unbounded per run    |
| Migration    | —                                                                 | Multi-step data migration: create new table, copy from both, rewrite FKs, drop old tables    |
| Complexity   | —                                                                 | Mixed lifecycle (ephemeral + permanent) in one table                                         |

### Option B — Keep separate tables; normalise Go layer only

Accept the SQL-level duplication as an intentional **snapshot pattern** — the
same track measurement is stored twice because the two copies have different
lifecycles (transient vs permanent). Focus on eliminating the Go-level
duplication:

1. Extract a shared `TrackMeasurement` struct embedded in both `TrackedObject`
   and `RunTrack`.
2. Write a single `scanTrackMeasurement(rows)` helper used by both
   `GetActiveTracks()` and `GetRunTracks()`.
3. Generate the shared SQL column list from a single constant slice, used in
   both `InsertTrack` and `InsertRunTrack`.
4. Optionally create a SQL `VIEW` that unions the two tables for read-only
   cross-table queries (e.g. "show me all tracks for sensor X, live and
   historical"):

```sql
CREATE VIEW lidar_all_tracks AS
  SELECT track_id, NULL AS run_id, sensor_id, track_state,
         start_unix_nanos, end_unix_nanos, observation_count,
         avg_speed_mps, max_speed_mps,
         bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
         height_p95_max, intensity_mean_avg,
         object_class, object_confidence, classification_model,
         NULL AS user_label, NULL AS quality_label
    FROM lidar_tracks
   UNION ALL
  SELECT track_id, run_id, sensor_id, track_state,
         start_unix_nanos, end_unix_nanos, observation_count,
         avg_speed_mps, max_speed_mps,
         bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
         height_p95_max, intensity_mean_avg,
         object_class, object_confidence, classification_model,
         user_label, quality_label
    FROM lidar_run_tracks;
```

| Aspect       | Pros                                                               | Cons                                                           |
| ------------ | ------------------------------------------------------------------ | -------------------------------------------------------------- |
| DRY (Go)     | Single struct + scan helper eliminates Go duplication              | SQL schema still has two column lists                          |
| DRY (SQL)    | Optional VIEW provides unified read surface                        | Schema migration still maintains two `CREATE TABLE` statements |
| FK integrity | No changes to lidar_track_obs or lidar_labels FKs                  | —                                                              |
| Performance  | Hot table stays small (live only); snapshot table grows separately | —                                                              |
| Lifecycle    | Clean separation: live rows pruned independently                   | —                                                              |
| Migration    | Minimal: Go refactor only (no SQL migration beyond 030)            | —                                                              |
| Risk         | Low — no schema change, existing queries unchanged                 | —                                                              |

### Option C — Slim `lidar_run_tracks` to labels-only; reference live tracks

Keep `lidar_run_tracks` but remove the duplicated measurement columns. Instead,
store only `(run_id, track_id)` plus the label/flag fields, and JOIN to
`lidar_tracks` for measurement data.

**Fatal flaw:** Live tracks are pruned after ~5 minutes. Once pruned, the JOIN
returns no data, and historical run analysis becomes impossible. This option is
only viable if we stop pruning tracks referenced by runs — which requires a
"pinning" mechanism and fundamentally changes the `lidar_tracks` lifecycle.

**Verdict:** Rejected unless combined with a track-pinning system, which adds
more complexity than the duplication it removes.

## Recommendation

**Option B — Keep separate tables; normalise Go layer only.**

The SQL duplication is an intentional snapshot pattern with sound engineering
rationale: different lifecycles (transient vs permanent), different primary
keys, and different FK relationships. Forcing both into one table (Option A)
creates more complexity than it saves: mixed-lifecycle management, FK rewrites,
performance regression on the hot upsert path, and a multi-step data migration.

The real DRY violation is in Go code, where 16 columns are spelled out in two
structs, two INSERT functions, and two scan loops. Extracting a shared
`TrackMeasurement` type and a column-list constant eliminates the duplication at
the layer that actually hurts maintainability.

## Work breakdown

### Phase 1 — Prerequisite: migration 030 (already planned)

Tracked in [schema-simplification-migration-030-plan.md](schema-simplification-migration-030-plan.md).
This drops dead columns and renames `peak→max`, bringing the two tables' shared
column set into clean alignment.

### Phase 2 — Go model normalisation

1. **Extract `TrackMeasurement` struct** in `internal/lidar/storage/sqlite/`:
   ```go
   type TrackMeasurement struct {
       SensorID             string  `json:"sensor_id"`
       TrackState           string  `json:"track_state"`
       StartUnixNanos       int64   `json:"start_unix_nanos"`
       EndUnixNanos         int64   `json:"end_unix_nanos,omitempty"`
       ObservationCount     int     `json:"observation_count"`
       AvgSpeedMps          float32 `json:"avg_speed_mps"`
       MaxSpeedMps          float32 `json:"max_speed_mps"`
       BoundingBoxLengthAvg float32 `json:"bounding_box_length_avg"`
       BoundingBoxWidthAvg  float32 `json:"bounding_box_width_avg"`
       BoundingBoxHeightAvg float32 `json:"bounding_box_height_avg"`
       HeightP95Max         float32 `json:"height_p95_max"`
       IntensityMeanAvg     float32 `json:"intensity_mean_avg"`
       ObjectClass          string  `json:"object_class,omitempty"`
       ObjectConfidence     float32 `json:"object_confidence,omitempty"`
       ClassificationModel  string  `json:"classification_model,omitempty"`
   }
   ```
2. **Embed in both structs:**

   ```go
   type TrackedObject struct {
       TrackID    string
       TrackMeasurement
       WorldFrame string         // live-only
       // ... L5 tracking fields (Kalman state, history, etc.)
   }

   type RunTrack struct {
       RunID   string `json:"run_id"`
       TrackID string `json:"track_id"`
       TrackMeasurement
       // Run-only label/flag fields
       UserLabel       string  `json:"user_label,omitempty"`
       // ...
   }
   ```

3. **Shared SQL column constant:**
   ```go
   var trackMeasurementColumns = []string{
       "sensor_id", "track_state",
       "start_unix_nanos", "end_unix_nanos", "observation_count",
       "avg_speed_mps", "max_speed_mps",
       "bounding_box_length_avg", "bounding_box_width_avg", "bounding_box_height_avg",
       "height_p95_max", "intensity_mean_avg",
       "object_class", "object_confidence", "classification_model",
   }
   ```
4. **Shared scan helper:**
   ```go
   func scanTrackMeasurement(row Scanner) (TrackMeasurement, error) { ... }
   ```
   Used by both `GetActiveTracks()` and `GetRunTracks()`.
5. **Shared INSERT helper** for the measurement columns, called by both
   `InsertTrack()` and `InsertRunTrack()`.

### Phase 3 — Optional SQL VIEW

Create a `lidar_all_tracks` VIEW (as shown above) via a new migration. This is
useful for ad-hoc SQL analysis but has no Go code dependency — purely a
convenience for operators using TailSQL or direct sqlite3 access.

### Phase 4 — Documentation

1. Add an inline comment block to `schema.sql` explaining the snapshot pattern
   and why two tables exist.
2. Update the layer model docs to clarify that `lidar_run_tracks` is an L8
   Analytics table that snapshots L5 `lidar_tracks` state.

## Effort estimates

| Phase            | Effort | Files touched                       |
| ---------------- | ------ | ----------------------------------- |
| Phase 1 (030)    | `M`    | ~15 Go + 3 TS + 1 proto + migration |
| Phase 2 (Go DRY) | `S`    | 3–4 Go files in `storage/sqlite/`   |
| Phase 3 (VIEW)   | `S`    | 1 migration file                    |
| Phase 4 (docs)   | `S`    | 2–3 Markdown files                  |

## Checklist

- [ ] Phase 1: Land migration 030 (tracked separately in [schema-simplification-migration-030-plan](schema-simplification-migration-030-plan.md))
- [ ] Phase 2: Extract `TrackMeasurement` struct and embed in `TrackedObject` + `RunTrack`
- [ ] Phase 2: Create shared `trackMeasurementColumns` constant
- [ ] Phase 2: Create `scanTrackMeasurement()` helper
- [ ] Phase 2: Refactor `InsertTrack()` and `InsertRunTrack()` to use shared column list
- [ ] Phase 2: Refactor `GetActiveTracks()` and `GetRunTracks()` to use shared scan helper
- [ ] Phase 2: Update `RunTrackFromTrackedObject()` to copy embedded struct directly
- [ ] Phase 2: Verify all tests pass (`make test-go`)
- [ ] Phase 3: Create migration for `lidar_all_tracks` VIEW
- [ ] Phase 4: Add schema.sql comment block explaining snapshot pattern
- [ ] Phase 4: Update layer model docs

## Risk

- **Low:** Phase 2 is a Go-only refactor with no schema or API changes.
  Existing tests catch any regression in scan/insert logic.
- **Low:** Phase 3 VIEW is read-only and additive — no existing code changes.
- **None:** Phase 4 is documentation only.

## Decision log

| Decision                       | Rationale                                                                                                                                       |
| ------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| Keep two tables (reject merge) | Different lifecycles (transient vs permanent), different PKs, FK relationships cannot be cleanly unified without surrogate keys and FK rewrites |
| Normalise at Go layer          | Eliminates the real maintenance cost (duplicate structs, scan loops, column lists) without schema risk                                          |
| Optional VIEW                  | Provides a unified read surface for operators without coupling live and snapshot write paths                                                    |
| Sequence after 030             | Migration 030 aligns the column sets; consolidation is simpler once dead/renamed columns are removed                                            |
