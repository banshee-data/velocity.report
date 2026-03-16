# Schema Simplification Migration 030 Plan

- **Status:** Draft — prerequisite proto rename complete (#352); migration SQL and Go code changes pending
- **Layers:** Database, L3 Grid, L5 Tracks, L6 Objects, L8 Analytics, L9 Endpoints (API + web)
- **Related:** [Speed Percentile Aggregation Alignment Plan](speed-percentile-aggregation-alignment-plan.md), [v0.5.0 Backward Compatibility Shim Removal Plan](v050-backward-compatibility-shim-removal-plan.md), [DECISIONS.md D-19](../DECISIONS.md), [L7 Scene Plan](lidar-l7-scene-plan.md), [L8/L9/L10 Plan](lidar-l8-analytics-l9-endpoints-l10-clients-plan.md), [Tracks Table Consolidation Plan](lidar-tracks-table-consolidation-plan.md)

## Prerequisites

| Prerequisite                             | Status         | Notes                                                                                |
| ---------------------------------------- | -------------- | ------------------------------------------------------------------------------------ |
| Proto `peak_speed_mps` → `max_speed_mps` | ✅ Complete    | Landed in #352 (proto field 25, Go/Swift/TS model); SQL column is the remaining step |
| D-19 decision recorded                   | ✅ Complete    | Raw maximum renamed to `max_speed_mps`; `peak` reserved for future filtered metric   |
| Migration SQL drafted                    | ✅ Complete    | DROP COLUMN + RENAME COLUMN statements ready (see §3 below)                          |
| Go code changes                          | ❌ Not started | Track store, analysis run, l5tracks, l6objects, monitor API all need field renames   |
| Web frontend changes                     | ❌ Not started | TypeScript type field renames and percentile field removal                           |

## Goal

Single migration (000030) that:

1. Drops dead per-track percentile columns from both `lidar_tracks` and
   `lidar_run_tracks`.
2. Renames `peak_speed_mps` → `max_speed_mps` on both tables for
   radar/lidar consistency.

## Motivation

Schema audit (March 2026) found two categories of waste in the track tables:

| Category               | Columns                                                                  | Status                                                                    |
| ---------------------- | ------------------------------------------------------------------------ | ------------------------------------------------------------------------- |
| **Dead columns**       | `p50_speed_mps`, `p85_speed_mps`, `p95_speed_mps` on `lidar_tracks`     | Never selected; Go code no longer writes them                             |
| **Naming mismatch**    | `peak_speed_mps` on both tables                                          | Radar uses `max_speed`; D-19 already decided rename                       |
| **Misapplied concept** | `p50_speed_mps`, `p85_speed_mps`, `p95_speed_mps` on `lidar_run_tracks` | Stored and returned via API but no downstream consumer computes with them |

Per-track percentiles are the wrong abstraction (see speed-percentile plan §1):
percentiles are meaningful over a _population_ of tracks, not over one track's
Kalman-filtered speed history.

**Quality columns are now populated:** The 6 quality columns on `lidar_tracks`
(`track_length_meters`, `track_duration_secs`, `occlusion_count`,
`max_occlusion_frames`, `spatial_coverage`, `noise_point_ratio`) are now written
by `InsertTrack()` and `UpdateTrack()` (see unpopulated-data-structures
remediation Phase 2). They must **not** be dropped.

## Non-goals

- Dropping `height_p95` / `height_p95_max` — these are spatial filters (robust
  per-cluster height), not population statistics. They stay.
- Adding new track-level speed metrics (e.g. `speed_variance_mps`) — separate
  future work.
- Touching `lidar_track_obs` — no changes needed.
- Dropping quality columns (`track_length_meters`, `track_duration_secs`,
  `occlusion_count`, `max_occlusion_frames`, `spatial_coverage`,
  `noise_point_ratio`) — these are now populated by `InsertTrack()` and
  `UpdateTrack()`.

## Migration SQL

### 000030_schema_simplification.up.sql

SQLite 3.35+ supports `ALTER TABLE ... DROP COLUMN` directly.

```sql
-- lidar_tracks: drop dead percentile columns
ALTER TABLE lidar_tracks DROP COLUMN p50_speed_mps;
ALTER TABLE lidar_tracks DROP COLUMN p85_speed_mps;
ALTER TABLE lidar_tracks DROP COLUMN p95_speed_mps;

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

**track_store.go** — Rename and remove percentile column references:

- `InsertTrack()`: percentile columns already removed from UPSERT (March 2026).
  Rename `peak_speed_mps` → `max_speed_mps` in all SQL strings.
- `UpdateTrack()`: percentile SET clauses already removed.
  Rename `peak_speed_mps` → `max_speed_mps`.
- `GetActiveTracks()`, `GetTracksInRange()`, other SELECT queries: rename
  `peak_speed_mps` → `max_speed_mps` in SQL strings.

**analysis_run.go** — Remove percentile columns, rename peak:

- `RunTrack` struct: `MaxSpeedMps` field already exists (renamed from
  `PeakSpeedMps`). Remove any residual `P50SpeedMps`, `P85SpeedMps`,
  `P95SpeedMps` fields if present.
- `InsertRunTrack()`: drop percentile columns from INSERT SQL. Rename
  `peak_speed_mps` → `max_speed_mps`.
- `GetRunTracks()`, `GetRunTrack()`: drop percentile columns from SELECT and
  `Scan()` calls. Rename `peak_speed_mps` → `max_speed_mps`.

**analysis_run_manager.go** — No changes needed (already uses `MaxSpeedMps`).

### Tracking layer (`internal/lidar/l5tracks/`)

**tracking.go** — ✅ Already done. `TrackedObject.MaxSpeedMps` is the current
field name. Speed comparison logic already uses `track.MaxSpeedMps`.

### Classification layer (`internal/lidar/l6objects/`)

**classification.go** — `ClassificationFeatures` uses `MaxSpeed` (already
renamed). `ComputeSpeedPercentiles()` stays as internal-only — used by
`extractFeatures()` for classifier feature extraction and by
`TrackFeatures.Compute()`. Not exposed via API.

**features.go** — `TrackFeatures.SpeedP50`, `.SpeedP85`, `.SpeedP95` are
internal feature-vector fields for ML training data export. These are **not**
per-track percentile columns in the DB. Decision: keep them as internal
feature descriptors (not stored in DB, not exposed via API) until the ML
pipeline is active.

### API layer (`internal/lidar/monitor/`)

**track_api.go** — ✅ Already done. `TrackSummary.MaxSpeedMps` uses JSON tag
`"max_speed_mps"`. `ClassStats.MaxSpeedMps` uses JSON tag `"max_speed_mps"`.
Accumulator already uses `accum.maxSpeed`. No per-track percentile fields in
the API response.

### PCAP analysis tool (`cmd/tools/pcap-analyse/`)

**main.go** — `SpeedStatistics` struct still has `P50Speed`, `P85Speed`,
`P95Speed`. These compute percentiles across a **population** of per-track
max speeds — this is correct aggregate usage. However, the naming
(`p50_speed_mps` etc. in JSON tags) conflicts with the per-track column names
being dropped.

**Recommendation:** Rename to `p50_max_speed_mps`, `p85_max_speed_mps`,
`p95_max_speed_mps` to make clear these are population percentiles of track max
speeds, not per-track percentiles. Also rename `P95` to `P98` to align with
D-18 (`p98` is the canonical high-end aggregate percentile). Update tests.

### Web frontend (`web/src/`)

**lib/types/lidar.ts** — No per-track percentile fields exist. No
`peak_speed_mps` exists. Already aligned.

### Proto (`proto/velocity_visualiser/v1/`)

**visualiser.proto** — ✅ Already done. Field is `float max_speed_mps = 25;`.
No per-track percentile fields in the proto.

**Swift** — Already uses `MaxSpeedMps`.

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
[L8 consolidation plan](lidar-l8-analytics-l9-endpoints-l10-clients-plan.md)),
`lidar_labels` should move with the L8 evaluation stores.

### E5 — Storage Package Reorganisation

**Problem:** All lidar SQLite stores live in a single package
`internal/lidar/storage/sqlite/`. This conflates L3 grid stores, L5 track
stores, and L8 analytics stores. The
[L8 plan](lidar-l8-analytics-l9-endpoints-l10-clients-plan.md) already identifies
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
- [L8 Analytics / L9 Endpoints / L10 Client Plan](lidar-l8-analytics-l9-endpoints-l10-clients-plan.md) —
  covers `analysis_run.go` ownership, storage reorganisation, `l9endpoints`
  rename
- [Layer Model](../lidar/architecture/lidar-data-layer-model.md) — frozen
  L1–L10 numbering from v0.5.0
