# Schema Simplification Migration 030 Plan

**Status:** Draft
**Layers:** Database, L5 Tracks, L6 Objects, L9 Endpoints (API + web)
**Related:** [Speed Percentile Aggregation Alignment Plan](speed-percentile-aggregation-alignment-plan.md), [v0.5.0 Backward Compatibility Shim Removal Plan](v050-backward-compatibility-shim-removal-plan.md), [DECISIONS.md D-19](../DECISIONS.md)

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
