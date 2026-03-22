# Pre-v0.5.0 LiDAR Schema Hardening Plan

- **Status:** Proposed
- **Target:** complete before the `v0.5.0` branch cut
- **Target window:** March 23, 2026 to April 3, 2026
- **Layers:** Database, LiDAR storage, API, macOS visualiser
- **Related:** [Pre-v0.5.0 Schema Simplification Migration 030 Plan](schema-simplification-migration-030-plan.md), [Track Labelling, Ground Truth Evaluation & Label-Aware Auto-Tuning](lidar-track-labelling-auto-aware-tuning-plan.md), [LiDAR Immutable Run Config Asset Plan](lidar-immutable-run-config-asset-plan.md)

## Decision

Do the schema cleanup now, in one breaking pre-`v0.5.0` pass. Do not stretch this over multiple releases.

The current state is tolerable for development but not a good release baseline:

- `lidar_track_annotations` is durable human-authored data attached to transient live `lidar_tracks`
- the schema declares cascades, but the main DB open path still does not enable `PRAGMA foreign_keys = ON`
- several delete paths only work today because FK enforcement is effectively off
- a number of enum/range contracts already exist in Go but are not enforced in SQLite
- turning foreign keys on will also expose a small amount of non-LiDAR schema debt that should be fixed before `v0.5.0`

## End State We Want For v0.5.0

- SQLite foreign keys are enabled on every production/test connection.
- Free-form annotations are replay-owned, not live-track-owned.
- Canonical track truth remains on `lidar_run_tracks`.
- Run lineage and replay evaluation tables use explicit foreign keys and explicit delete behavior.
- The database enforces the small set of enum, range, time-order, and boolean rules the code already assumes.
- Global FK-on blockers outside LiDAR are removed.

## Scope

### 1. Re-key free-form annotations onto the durable owner

Replace the current live-track-linked annotation model:

- current: `lidar_track_annotations(track_id -> lidar_tracks)`
- target: replay-owned annotations, with optional linkage to durable run tracks

Recommended table shape:

```sql
CREATE TABLE lidar_replay_annotations (
    annotation_id TEXT PRIMARY KEY,
    replay_case_id TEXT NOT NULL,
    run_id TEXT,
    track_id TEXT,
    class_label TEXT NOT NULL,
    start_timestamp_ns INTEGER NOT NULL,
    end_timestamp_ns INTEGER,
    confidence REAL,
    created_by TEXT,
    created_at_ns INTEGER NOT NULL,
    updated_at_ns INTEGER,
    notes TEXT,
    source_file TEXT,
    FOREIGN KEY (replay_case_id) REFERENCES lidar_replay_cases(replay_case_id) ON DELETE CASCADE,
    FOREIGN KEY (run_id, track_id) REFERENCES lidar_run_tracks(run_id, track_id) ON DELETE SET NULL
);
```

Principles:

- replay case is the durable owner
- canonical track labels stay on `lidar_run_tracks`
- annotation rows must not depend on the lifetime of `lidar_tracks`
- optional run-track linkage should point at durable `(run_id, track_id)`, not live track buffers

Migration approach:

- take a one-time export or legacy-table snapshot of existing `lidar_track_annotations`
- auto-migrate only rows that can be resolved safely
- keep unresolved rows in a legacy/audit table instead of silently dropping them
- remove the old live-track-owned annotation table in the same migration set, not one release later

This is the main bandaid to rip off before `v0.5.0`.

### 2. Turn foreign keys on and make delete semantics real

Ship `v0.5.0` with foreign keys enabled by default in the main DB open path.

That requires tightening the tables that currently rely on FK-off behavior:

- enable `PRAGMA foreign_keys = ON` in `internal/db/db.go`
- keep `lidar_track_observations -> lidar_tracks ON DELETE CASCADE`
- remove the annotation dependency on `lidar_tracks` by finishing the re-key above before FK-on lands
- add `FOREIGN KEY (parent_run_id) REFERENCES lidar_run_records(run_id) ON DELETE SET NULL`
- fix `lidar_replay_evaluations` run references so run deletion is explicit rather than accidental:
  - recommended: `reference_run_id` and `candidate_run_id` both `ON DELETE CASCADE`
- change evaluation uniqueness from:
  - `UNIQUE(reference_run_id, candidate_run_id)`
  - to:
  - `UNIQUE(replay_case_id, reference_run_id, candidate_run_id)`

Why this matters:

- today the schema says one thing and runtime behavior does another
- once FK enforcement is on, `DeleteRun()` and `ClearRuns()` must either cascade cleanly or fail for a deliberate reason
- the current replay evaluation uniqueness key is too coarse for a table that is explicitly replay-case scoped

### 3. Tighten the contracts the code already assumes

Do not try to perfect every table before `v0.5.0`. Tighten the ones that are already effectively enums or bounded values in code.

Recommended pre-`v0.5.0` checks:

- `lidar_run_records.source_type IN ('live', 'pcap')`
- `lidar_run_records.status IN ('running', 'completed', 'failed')`
- `lidar_run_records.status` should become `NOT NULL`
- `lidar_tracks.track_state IN ('tentative', 'confirmed', 'deleted')`
- `lidar_run_tracks.track_state IN ('tentative', 'confirmed', 'deleted')`
- `lidar_tuning_sweeps.mode IN ('sweep', 'auto', 'hint')`
- `lidar_tuning_sweeps.status IN ('running', 'completed', 'failed', 'suspended')`
- `object_confidence`, `label_confidence`, and annotation `confidence` must be `NULL` or in `[0, 1]`
- `end_unix_nanos IS NULL OR end_unix_nanos >= start_unix_nanos`
- `end_timestamp_ns IS NULL OR end_timestamp_ns >= start_timestamp_ns`
- `time_end_ns >= time_start_ns`
- `pcap_start_secs IS NULL OR pcap_start_secs >= 0`
- `pcap_duration_secs IS NULL OR pcap_duration_secs >= 0`
- `radius_m > 0`
- boolean-ish columns should be constrained to `0/1`:
  - `is_split_candidate`
  - `is_merge_candidate`
  - `include_map`
  - `is_active`

Taxonomy/default cleanup that should happen in the same window:

- change `lidar_run_missed_regions.expected_label` default from stale `'good_vehicle'` to `'car'`
- add an enum check for `expected_label` if we are comfortable freezing the current `v0.5.0` label set

### 4. Fix the small non-LiDAR blockers before FK-on

Turning on foreign keys is global, not LiDAR-only. The main non-LiDAR blocker worth fixing now is:

- `site_reports.site_id` is `NOT NULL DEFAULT 0`, while report creation currently falls back to `siteID := 0` when no site is supplied

Recommended fix:

- make `site_reports.site_id` nullable
- change the FK to `ON DELETE SET NULL`
- write `NULL`, not `0`, when a report is not attached to a site

That keeps global FK enforcement from breaking unrelated report generation flows.

## Delivery Plan

### Phase 1: Breaking Schema Reset

- **Dates:** March 23, 2026 to March 27, 2026
- **Goal:** land the real schema shape we want to ship

Deliverables:

- migration set `000032`/`000033` for:
  - replay-owned annotations
  - FK-on compatibility
  - run lineage and replay evaluation fixes
  - enum/range/time/boolean constraints
  - `site_reports.site_id` nullability fix
- production DB open path updated to enable foreign keys
- API/storage/client code updated to the new annotation table contract
- data migration path implemented:
  - auto-migrate safe rows
  - snapshot unresolved legacy annotation rows for manual review

Exit criteria:

- no durable human-authored rows depend on `lidar_tracks`
- the app starts and runs with `PRAGMA foreign_keys = ON`

### Phase 2: Release Hardening

- **Dates:** March 30, 2026 to April 3, 2026
- **Goal:** prove the new schema behaves correctly before the `v0.5.0` cut

Deliverables:

- regression tests for:
  - track update/upsert
  - track prune
  - track clear
  - run delete
  - replay case delete
  - evaluation insert/delete
  - report creation with and without `site_id`
- one migration audit command or SQL report for:
  - unresolved legacy annotations
  - orphan rows
  - invalid enum/range values blocked by the new checks
- manual smoke checks:
  - Svelte run labelling
  - `/api/lidar/labels` replacement path
  - replay-case CRUD
  - run evaluation persistence

Exit criteria:

- migration succeeds on representative existing DBs
- no FK violations remain
- no release-blocking annotation data loss is unresolved

## Explicit Pre-v0.5.0 Changes By Priority

### Must do before shipping

- replay-own free-form annotations
- enable foreign keys on all DB opens
- add `parent_run_id` FK
- fix replay evaluation delete semantics and uniqueness key
- tighten safe enum/time/confidence checks
- fix `site_reports.site_id` nullability/default mismatch
- update stale missed-region label default

### Nice to do if time remains

- add `json_valid(...)` checks for the most important JSON blobs such as `params_json` and sweep `request`
- add an integrity-report endpoint or CLI for orphan/constraint audits
- add one composite index for replay evaluation listing by `(replay_case_id, created_at DESC)`

### Defer until after v0.5.0

- full `lidar_run_configs` / immutable run-config normalization
- broader JSON-shape validation for all blob columns
- any attempt to merge live and run track tables
- a richer label ontology redesign beyond the current `v0.5.0` vocabulary

## Recommendation

Do not ship `v0.5.0` with the current split between declared FKs and actual runtime behavior.

The focused plan is:

1. replace live-track-owned annotations with replay-owned annotations
2. turn foreign keys on everywhere
3. harden the small set of schema contracts the code already relies on
4. fix the few cross-schema blockers exposed by FK-on

That is a small enough project to finish before `v0.5.0`, and it leaves the release on a much cleaner baseline than trying to defer the cleanup again.
