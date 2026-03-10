# Schema Simplification Migration 030 Plan

**Status:** Draft
**Layers:** Database, L3 Grid, L5 Tracks, L6 Objects, L8 Analytics, L9 Endpoints (API + web)
**Related:** [Speed Percentile Aggregation Alignment Plan](speed-percentile-aggregation-alignment-plan.md), [v0.5.0 Backward Compatibility Shim Removal Plan](v050-backward-compatibility-shim-removal-plan.md), [DECISIONS.md D-19](../DECISIONS.md), [L7 Scene Plan](lidar-l7-scene-plan.md), [L8/L9/L10 Plan](lidar-l8-analytics-l9-endpoints-l10-client-plan.md)

## Goal

Single migration (000030) that:

1. Drops dead per-track percentile columns from both `lidar_tracks` and `lidar_run_tracks`.
2. Drops always-NULL quality columns from `lidar_tracks`.
3. Renames `peak_speed_mps` → `max_speed_mps` on both tables for radar/lidar consistency.

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
- Touching `lidar_track_obs` — no changes needed.

## Migration SQL

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
```

## Go Code Changes

### Storage layer (`internal/lidar/storage/sqlite/`)

**track_store.go** — Remove percentile columns from INSERT/UPDATE/SELECT:

- `InsertTrack()`: drop `p50_speed_mps, p85_speed_mps, p95_speed_mps` from UPSERT columns and ON CONFLICT SET
- `UpdateTrack()`: drop percentile SET clauses
- `GetActiveTracks()`, `GetTracksInRange()`: already don't select them — no change
- Rename `peak_speed_mps` → `max_speed_mps` in all SQL strings

**analysis_run.go** — Remove percentile columns, rename peak:

- `RunTrack` struct: remove `P50SpeedMps`, `P85SpeedMps`, `P95SpeedMps` fields; rename `PeakSpeedMps` → `MaxSpeedMps`
- `RunTrackFromTrackedObject()`: remove percentile population; rename `PeakSpeedMps` → `MaxSpeedMps`
- `InsertRunTrack()`: drop percentile columns from INSERT SQL
- `GetRunTracks()`, `GetRunTrack()`: drop percentile columns from SELECT and `Scan()` calls
- `ExportTracksCSV()` (if exists): update column headers

**analysis_run_manager.go** — Update any `RecordTrack()` calls that reference removed fields.

### Tracking layer (`internal/lidar/l5tracks/`)

**tracking.go** — Rename struct field:

- `TrackedObject.PeakSpeedMps` → `TrackedObject.MaxSpeedMps`
- Update speed comparison logic: `if speed > track.PeakSpeedMps` → `if speed > track.MaxSpeedMps`

### Classification layer (`internal/lidar/l6objects/`)

**classification.go** — Rename field references:

- All `f.PeakSpeed` accesses (used for vehicle/cyclist classification thresholds) — rename to `f.MaxSpeed`
- `ClassificationFeatures.PeakSpeed` → `ClassificationFeatures.MaxSpeed`

**features.go** — Rename field:

- `TrackFeatures.PeakSpeedMps` → `TrackFeatures.MaxSpeedMps`
- Update CSV export header from `"peak_speed_mps"` to `"max_speed_mps"`
- Update feature extraction: `f.PeakSpeedMps = track.PeakSpeedMps` → `f.MaxSpeedMps = track.MaxSpeedMps`

### API layer (`internal/lidar/monitor/`)

**track_api.go** — Rename JSON fields:

- `TrackSummary.PeakSpeedMps` JSON tag: `"peak_speed_mps"` → `"max_speed_mps"`
- `ClassStats.PeakSpeedMps` JSON tag: `"peak_speed_mps"` → `"max_speed_mps"`
- Update accumulator field name (`accum.peakSpeed` → `accum.maxSpeed`)

### PCAP analysis tool (`cmd/tools/pcap-analyse/`)

**main.go** — Rename and drop:

- `TrackExport.PeakSpeedMps` → `TrackExport.MaxSpeedMps`
- Remove `P50SpeedMps`, `P85SpeedMps`, `P95SpeedMps` from struct and SQL INSERT
- Update CSV headers
- Remove `ComputeSpeedPercentiles()` call (or keep as offline-only — decision needed)

### Web frontend (`web/src/`)

**lib/types/lidar.ts** — Rename type fields:

- `peak_speed_mps` → `max_speed_mps` on track and run-track types
- Remove `p50_speed_mps`, `p85_speed_mps`, `p95_speed_mps` from run-track type

**Components** — Update any display references from "Peak" to "Max" in labels.

### Proto (`proto/velocity_visualiser/v1/`)

**visualiser.proto** — Rename field:

- `float peak_speed_mps = 25;` → `float max_speed_mps = 25;` (same field number, rename only)
- Regenerate `visualiser.pb.go`

**Swift** — Regenerate and update `PeakSpeedMps` → `MaxSpeedMps` references.

## pcap-analyse: Keep or Drop Offline Percentiles?

The `pcap-analyse` tool computes per-track P50/P85/P95 for offline analysis
and writes them to the `lidar_run_tracks` table. Once we drop those columns,
two options:

**Option A — Drop completely:** Remove `ComputeSpeedPercentiles()` call and the
struct fields. Offline analysis uses the same schema as the server. Simplest.

**Option B — Keep as local-only CSV export:** Compute percentiles for the
CSV export (offline analysis) but don't persist to the database. Requires
splitting the export struct from the DB struct.

**Recommendation:** Option A. If offline percentile analysis is needed later,
it can use the TDL (Track Description Language) query layer planned in v0.5.1.

## Testing

- Run existing Go tests — `make test-go` must pass after all renames
- Verify migration applies cleanly on a fresh DB and on a DB with existing data
- Verify `make test-web` passes after TypeScript type changes
- Run `make schema-erd` to regenerate ERD after migration

## Risk

- **Low:** All dropped columns are either never-read or always-NULL
- **Medium:** `peak` → `max` rename touches many files (~15 Go, ~3 TS, 1 proto,
  ~5 Swift) — requires coordinated commit
- **Mitigation:** The branch already drops percentile _additions_ from the proto;
  this migration aligns the database with that direction

---

## Optional Extension: Layer-Model Alignment

The schema audit also surfaced terminology collisions and ownership ambiguities
when mapping the current database tables to the
[L1–L10 layer model](../lidar/architecture/lidar-data-layer-model.md). These can
be addressed independently of the core migration 000030, but are documented here
for coherent planning.

### Table → Layer Ownership Map

| Table                  | Current Layer | Notes                                                                |
| ---------------------- | ------------- | -------------------------------------------------------------------- |
| `lidar_clusters`       | L4 Perception | Correct — per-frame cluster primitives                               |
| `lidar_track_obs`      | L5 Tracks     | Correct — per-observation state within a track                       |
| `lidar_tracks`         | L5 Tracks     | Correct — live transient track buffer (pruned after ~5 min)          |
| `lidar_labels`         | L6 → L8       | Human-assigned ground truth; consumed by L8 evaluation scoring       |
| `lidar_bg_regions`     | L3 Grid       | Correct — background grid state                                      |
| `lidar_bg_snapshot`    | L3 Grid       | Correct — serialised grid snapshot for PCAP restoration              |
| `lidar_analysis_runs`  | L8 Analytics  | Correct — run metadata and aggregate statistics                      |
| `lidar_run_tracks`     | L8 Analytics  | Correct — versioned track snapshots from analysis runs               |
| `lidar_scenes`         | L8 Analytics  | **Naming collision** — see §E1 below                                 |
| `lidar_evaluations`    | L8 Analytics  | Correct — run-vs-run comparison scores                               |
| `lidar_missed_regions` | L8 Analytics  | Correct — evaluation detail (undetected ground-truth regions)        |
| `lidar_sweeps`         | L8 Analytics  | Correct — parameter sweep metadata                                   |
| `radar_*`              | Mixed         | Radar tables predate the layer model; alignment is out-of-scope here |
| `site` / `site_*`      | L9 Endpoints  | Correct — server configuration and report metadata                   |

### E1 — `lidar_scenes` Naming Collision with L7 Scene

**Problem:** The layer model reserves "Scene" (L7) for a _persistent canonical
world model_ — accumulated geometry, canonical objects, OSM priors, and
multi-sensor fusion. The current `lidar_scenes` table is an _evaluation context_:
it ties a PCAP file to a sensor, stores a reference run and optimal parameters,
and groups `lidar_evaluations` under it. This is L8 Analytics, not L7 Scene.

When L7 is eventually implemented (planned for v1.0), there will be a genuine
`l7scene` package and likely a `lidar_scene_*` table family. Having the current
L8 evaluation-context table already named `lidar_scenes` will cause confusion.

**Options:**

| Option | Rename to                         | Pros                           | Cons                                        |
| ------ | --------------------------------- | ------------------------------ | ------------------------------------------- |
| A      | `lidar_evaluation_contexts`       | Precise, self-documenting      | Long; FK references in evaluations/sweeps   |
| B      | `lidar_pcap_scenes`               | Keeps "scene" but qualifies it | Still collides when L7 adds `lidar_scene_*` |
| C      | `lidar_evaluation_sets`           | Short, avoids "scene"          | Set implies a many-to-many grouping         |
| D      | Do nothing; document the conflict | Zero migration effort          | Confusion grows as L7 materialises          |

**Recommendation:** Option A (`lidar_evaluation_contexts`). Although the FK
cascade makes it a medium-effort rename, it eliminates the collision cleanly.
Can be deferred to the L8 consolidation phase but should be done before any L7
work begins.

**If accepted, migration scope:**

- Rename `lidar_scenes` → `lidar_evaluation_contexts`
- Rename `scene_id` column → `context_id` throughout `lidar_evaluation_contexts`,
  `lidar_evaluations`, `lidar_sweeps`
- Update `SceneID` / `Scene` struct names in `scene_store.go`, `scene_api.go`
- Update web frontend type definitions and API paths (`/api/lidar/scenes/` →
  `/api/lidar/evaluation-contexts/`)

### E2 — `world_frame` Column Name Ambiguity

**Problem:** The `world_frame` column on `lidar_clusters`, `lidar_tracks`, and
`lidar_track_obs` stores an L2 `FrameID` string (format: `"sensorID-frame-N"`).
The name "world_frame" suggests an L7 world-model concept — a coordinate frame
or global geometry reference — which it is not. It is purely a temporal frame
identifier from L2.

**Options:**

| Option | Rename to     | Pros                           | Cons                                        |
| ------ | ------------- | ------------------------------ | ------------------------------------------- |
| A      | `frame_id`    | Matches L2 `FrameID` type name | Very generic; could collide with future FKs |
| B      | `l2_frame_id` | Layer-prefixed, unambiguous    | Unconventional; "l2" prefix in column names |
| C      | Do nothing    | Zero effort                    | Misleading name persists                    |

**Recommendation:** Option A (`frame_id`). It aligns with the Go type name and
is short. Future FK collisions are unlikely since no other table has a
`frame_id` column. Can bundle into migration 000030 or defer.

**If accepted, migration scope:**

- `ALTER TABLE lidar_clusters RENAME COLUMN world_frame TO frame_id;`
- `ALTER TABLE lidar_tracks RENAME COLUMN world_frame TO frame_id;`
- `ALTER TABLE lidar_track_obs RENAME COLUMN world_frame TO frame_id;`
- Update Go code: `WorldFrame` field/SQL references → `FrameID` / `frame_id`
  across `track_store.go`, `cluster_store.go`, and `l5tracks/tracking.go`

### E3 — `scene_hash` on `lidar_bg_regions`

**Problem:** The `scene_hash` column on `lidar_bg_regions` is a hash of the L3
background grid state, used for PCAP restoration (matching a regions snapshot to
its grid configuration). The name suggests an L7 Scene hash.

**Options:**

| Option | Rename to   | Pros                             | Cons                         |
| ------ | ----------- | -------------------------------- | ---------------------------- |
| A      | `grid_hash` | Accurate — it hashes the L3 grid | Minor migration + Go changes |
| B      | Do nothing  | Zero effort                      | Misleading once L7 exists    |

**Recommendation:** Option A (`grid_hash`). Small change, high clarity. The
indexed column `idx_bg_regions_scene_hash` would also rename to
`idx_bg_regions_grid_hash`.

**If accepted, migration scope:**

- `ALTER TABLE lidar_bg_regions RENAME COLUMN scene_hash TO grid_hash;`
- `DROP INDEX idx_bg_regions_scene_hash;`
- `CREATE INDEX idx_bg_regions_grid_hash ON lidar_bg_regions (grid_hash);`
- Update Go code in `bg_region_store.go` / `bg_store.go`

### E4 — `lidar_labels` Layer Ownership

**Problem:** `lidar_labels` contains human-assigned ground-truth labels applied
to tracks. The labels are _authored_ at L6 Objects (by a human labeller
classifying tracked objects) but _consumed_ at L8 Analytics (evaluation scoring
compares predicted vs labelled classes). The store currently lives alongside L5
track storage.

**Action:** No schema change needed. Document that `lidar_labels` is an L6→L8
bridge table. When L8 storage is eventually separated (per the
[L8 consolidation plan](lidar-l8-analytics-l9-endpoints-l10-client-plan.md)),
`lidar_labels` should move with the L8 evaluation stores.

### E5 — Storage Package Reorganisation

**Problem:** All lidar SQLite stores live in a single package
`internal/lidar/storage/sqlite/`. This conflates L3 grid stores, L5 track
stores, and L8 analytics stores. The
[L8 plan](lidar-l8-analytics-l9-endpoints-l10-client-plan.md) already identifies
`analysis_run.go` and `analysis_run_compare.go` as belonging in a future
`l8analytics/` package.

**Action:** No schema change involved — this is a Go package reorganisation.
Document as a prerequisite for L8 consolidation:

- Move `analysis_run.go`, `analysis_run_compare.go`, `scene_store.go`,
  `evaluation_store.go`, `missed_region_store.go`, `sweep_store.go` into
  `internal/lidar/l8analytics/store/` (or similar)
- L3 stores (`bg_region_store.go`, `bg_store.go`) stay or move to
  `internal/lidar/l3grid/store/`
- L5 stores (`track_store.go`, `cluster_store.go`, `track_obs_store.go`) stay

### Sequencing

These extensions are independent of each other and of the core migration 000030.
Recommended ordering:

1. **Migration 000030** (this plan) — drop dead columns, rename peak → max
2. **E2 + E3** (low effort) — rename `world_frame` → `frame_id` and
   `scene_hash` → `grid_hash`; can bundle into 000030 or a follow-on 000031
3. **E1** (medium effort) — rename `lidar_scenes` table; best done during L8
   consolidation phase to minimise API churn
4. **E4 + E5** (code-only) — document label ownership; reorganise storage
   packages during L8 consolidation

### Cross-references

- [L7 Scene Plan](lidar-l7-scene-plan.md) — defines what L7 Scene will
  actually be (persistent canonical world model)
- [L8 Analytics / L9 Endpoints / L10 Client Plan](lidar-l8-analytics-l9-endpoints-l10-client-plan.md) —
  covers `analysis_run.go` ownership, storage reorganisation, `l9endpoints`
  rename
- [Layer Model](../lidar/architecture/lidar-data-layer-model.md) — frozen
  L1–L10 numbering from v0.5.0
