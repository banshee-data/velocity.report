# Pre-v0.5.0 LiDAR Schema Hardening Plan

- **Status:** Implemented on branch `codex/draft-schema-improvement-proposal`; pending merge for `v0.5.0`
- **Target:** complete before the `v0.5.0` branch cut
- **Actual landing window:** March 22, 2026
- **Layers:** Database, LiDAR storage, API, macOS visualiser
- **Canonical:** [schema-hardening.md](../lidar/architecture/schema-hardening.md)
- **Related:** [Pre-v0.5.0 Schema Simplification Migration 030 Plan](schema-simplification-migration-030-plan.md), [Track Labelling, Ground Truth Evaluation & Label-Aware Auto-Tuning](lidar-track-labelling-auto-aware-tuning-plan.md), [LiDAR Immutable Run Config Asset Plan](lidar-immutable-run-config-asset-plan.md)

## Branch Status

This branch implements the core pre-`v0.5.0` schema hardening work:

- `000033` replaces live-track-owned annotations with `lidar_replay_annotations`
- `000034` enables the stricter schema shape needed for FK-on operation
- the main DB open path now enables `PRAGMA foreign_keys = ON`
- `parent_run_id` is now a real FK with `ON DELETE SET NULL`
- replay evaluations are replay-case-scoped for uniqueness and use explicit run/replay-case FKs
- `site_reports.site_id` is nullable and stores `NULL`, not `0`, when no site is attached
- `radar_data` now has a real `data_id` PK and `radar_transit_links` references that key instead of SQLite `rowid`
- legacy `lidar_track_annotations` rows are dropped during `000033` as part of the schema reset
- hardening regression tests cover the key delete/nullability/FK paths

Important implementation detail: this branch does not preserve old `lidar_track_annotations` rows. Migration `000033` removes the transient live-track annotation table and starts the replay-owned annotation model with a clean slate.

## Decision

Do the schema cleanup now, in one breaking pre-`v0.5.0` pass. Do not stretch this over multiple releases.

The pre-hardening state was tolerable for development but not a good release baseline:

- `lidar_track_annotations` is durable human-authored data attached to transient live `lidar_tracks`
- the schema declared cascades, but the main DB open path did not enable `PRAGMA foreign_keys = ON`
- several delete paths only work today because FK enforcement is effectively off
- a number of enum/range contracts already exist in Go but are not enforced in SQLite
- turning foreign keys on will also expose a small amount of non-LiDAR schema debt that should be fixed before `v0.5.0`

## Architecture Decision Record

- **Decision:** move free-form annotations to a replay-owned table, then enable foreign keys globally before `v0.5.0`
- **Status:** accepted and implemented on this branch
- **Owners:** LiDAR storage and API

### Context

- live tracks are transient and are intentionally pruned
- free-form annotations are human-authored and should outlive live-track churn
- canonical labelling already has a durable home on `lidar_run_tracks`
- before this branch, the main DB connection path left FK enforcement off, so the schema contract was aspirational rather than real

### Alternatives considered

1. **Recommended: replay-own free-form annotations and turn FKs on**
   - Effort: medium because it requires one migration window plus API/storage updates.
   - Risk: moderate because it is a breaking cleanup that deliberately drops old live-track annotations.
   - Downstream impact: yields one clear owner per lifecycle and makes delete semantics predictable.
2. **Keep current annotation table and only add a replay-case FK**
   - Effort: low because it changes one table in place.
   - Risk: high because the table would still depend on transient live tracks and would remain conceptually mixed.
   - Downstream impact: improves provenance validation but preserves the lifecycle bug.
3. **Do nothing until after `v0.5.0`**
   - Effort: none now.
   - Risk: high because release behaviour would still depend on FK-off connections and orphaning would remain possible.
   - Downstream impact: pushes migration risk into a post-release cleanup when compatibility pressure is higher.

## End State Delivered On This Branch

- SQLite foreign keys are enabled on every production/test connection.
- Free-form annotations are replay-owned, not live-track-owned.
- Canonical track truth remains on `lidar_run_tracks`.
- Run lineage and replay evaluation tables use explicit foreign keys and explicit delete behaviour.
- The database enforces the small set of enum, range, time-order, and boolean rules the code already assumes.
- Global FK-on blockers outside LiDAR are removed.

## System Boundary Diagram

```text
                   +--------------------------------------+
                   | HTTP / UI / Visualiser clients       |
                   |--------------------------------------|
                   | /api/lidar/labels                    |
                   | /api/lidar/runs/.../label            |
                   | /api/lidar/scenes                    |
                   +-------------------+------------------+
                                       |
                                       v
                      +----------------+----------------+
                      | LiDAR API / storage layer       |
                      |---------------------------------|
                      | Free-form replay annotations    |
                      | Run-track canonical labels      |
                      | Replay cases and evaluations    |
                      +--------+---------------+--------+
                               |               |
                durable owner  |               | transient owner
                               v               v
                 +-------------+----+   +------+----------------+
                 | Replay / run data |   | Live tracking data    |
                 |-------------------|   |-----------------------|
                 | lidar_replay_*    |   | lidar_tracks          |
                 | lidar_run_records  |   | lidar_track_obs       |
                 | lidar_run_tracks   |   | prune / clear paths   |
                 +-------------+------+   +-----------+-----------+
                               \                    /
                                \                  /
                                 v                v
                          +------+----------------------+
                          | SQLite with FK enforcement  |
                          | PRAGMA foreign_keys = ON    |
                          +-----------------------------+

Non-LiDAR global FK-on dependency:
site_reports -> site
```

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
    CHECK (
        (run_id IS NULL AND track_id IS NULL) OR
        (run_id IS NOT NULL AND track_id IS NOT NULL)
    ),
    FOREIGN KEY (replay_case_id) REFERENCES lidar_replay_cases(replay_case_id) ON DELETE CASCADE,
    FOREIGN KEY (run_id, track_id) REFERENCES lidar_run_tracks(run_id, track_id) ON DELETE SET NULL
);
```

Principles:

- replay case is the durable owner
- canonical track labels stay on `lidar_run_tracks`
- annotation rows must not depend on the lifetime of `lidar_tracks`
- optional run-track linkage should point at durable `(run_id, track_id)`, not live track buffers
- new writes should require `replay_case_id`; free-form annotations without a replay owner should not be created post-migration

Implementation on this branch:

- replace `lidar_track_annotations` with `lidar_replay_annotations` in `000033`
- write all new free-form annotations to `lidar_replay_annotations`
- do not migrate historical rows out of the live-track table
- require `replay_case_id` for all rows in the new replay-owned table

This is the main bandaid to rip off before `v0.5.0`.

### 2. Turn foreign keys on and make delete semantics real

Ship `v0.5.0` with foreign keys enabled by default in the main DB open path.

That requires tightening the tables that currently rely on FK-off behaviour:

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

- today the schema says one thing and runtime behaviour does another
- once FK enforcement is on, `DeleteRun()` and `ClearRuns()` must either cascade cleanly or fail for a deliberate reason
- the current replay evaluation uniqueness key is too coarse for a table that is explicitly replay-case scoped

### 3. Tighten the contracts the code already assumes

Do not try to perfect every table before `v0.5.0`. Tighten the ones that are already effectively enums or bounded values in code.

Landed pre-`v0.5.0` checks:

- `lidar_run_records.source_type IN ('live', 'pcap')`
- `lidar_run_records.status IN ('running', 'completed', 'failed')`
- `lidar_run_records.status` is `NOT NULL`
- `lidar_tracks.track_state IN ('tentative', 'confirmed', 'deleted')`
- `lidar_run_tracks.track_state IN ('tentative', 'confirmed', 'deleted')`
- `lidar_tuning_sweeps.mode IN ('manual', 'sweep', 'auto', 'auto-tune', 'hint')`
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

Landed taxonomy/default cleanup:

- change `lidar_run_missed_regions.expected_label` default from stale `'good_vehicle'` to `'car'`
- add an enum check for `expected_label` over the current `v0.5.0` label set

### 4. Fix the small non-LiDAR blockers before FK-on

Turning on foreign keys is global, not LiDAR-only. The main non-LiDAR blocker worth fixing now is:

- `site_reports.site_id` is `NOT NULL DEFAULT 0`, while report creation currently falls back to `siteID := 0` when no site is supplied

Recommended fix:

- make `site_reports.site_id` nullable
- change the FK to `ON DELETE SET NULL`
- write `NULL`, not `0`, when a report is not attached to a site

That keeps global FK enforcement from breaking unrelated report generation flows.

## Failure Registry

| Failure mode                                           | What fails                        | User-visible effect                                 | Required handling                                                                                  |
| ------------------------------------------------------ | --------------------------------- | --------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| Legacy `lidar_track_annotations` rows still matter     | upgrade to `000033`               | historical rows are dropped during the schema reset | accept the break before `v0.5.0`, export manually before upgrade if any of those rows still matter |
| FK enforcement exposes existing orphan rows            | app start or migration smoke test | upgrade blocked                                     | run integrity audit before final migration, repair or quarantine violating rows                    |
| Replay case delete removes annotations unexpectedly    | replay case management            | user loses free-form notes                          | document delete contract explicitly and require migration test coverage for delete paths           |
| Run delete fails because child rows still reference it | run lifecycle APIs                | delete endpoint starts erroring after FK-on         | make all run-owned children explicit before enabling FK-on                                         |
| Report creation without site still writes `0`          | non-LiDAR reporting               | unrelated report generation breaks under FK-on      | migrate `site_reports.site_id` to nullable and write `NULL` in API path                            |
| New enum checks reject legacy values                   | create/update APIs or migration   | writes begin failing after schema change            | backfill old taxonomy values during migration before adding `CHECK`s                               |

## Implementation Status

### Shipped On This Branch

- migration `000033` adds `lidar_replay_annotations`, removes `lidar_track_annotations`, and fixes replay-evaluation uniqueness/delete semantics
- migration `000033` deliberately drops historical live-track annotations as part of the pre-`v0.5.0` reset
- migration `000034` hardens the schema for FK-on operation, including:
  - `parent_run_id` foreign keying
  - enum, range, confidence, and time-order `CHECK`s
  - missed-region default/enum cleanup
  - nullable `site_reports.site_id`
  - `radar_data.data_id` plus `radar_transit_links` FK repair
- `internal/db/db.go` now enables `PRAGMA foreign_keys = ON`
- the label API and storage code now target `lidar_replay_annotations`
- regression coverage exists for replay-case delete, run delete, null-site reports, paired run/track annotation links, and transit-link persistence under FK enforcement
- the schema ordering tool now ignores self-referential FKs and has regression coverage against the current schema

### Deliberately Out Of Scope On This Branch

- preserving or rekeying old live-track annotations into the new replay-owned model
- a generic integrity-audit CLI or HTTP endpoint
- broader JSON-shape validation beyond the safe enum/range/time constraints already added

### Residual Follow-Up After Merge

- optional integrity-report tooling for orphan/constraint audits
- any broader data-model work such as immutable run-config normalisation or live/run table unification remains separate from this hardening pass

## Result

The branch now matches the original intent of the hardening plan: the release no longer depends on FK-off behaviour, replay annotations have a durable owner, and the highest-value schema contracts are enforced in SQLite before `v0.5.0`.
