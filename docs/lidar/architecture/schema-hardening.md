# Pre-v0.5.0 schema hardening

- **Status:** Complete

One-pass schema cleanup before the `v0.5.0` branch cut: enable foreign
keys, move annotations to a durable owner, and tighten contracts the code
already assumes.

## Architecture decision record

- **Decision:** Move free-form annotations to a replay-owned table, then
  enable `PRAGMA foreign_keys = ON` globally before `v0.5.0`.
- **Status:** Accepted and implemented (March 2026).

### Context

- Live tracks are transient and intentionally pruned.
- Free-form annotations are human-authored and must outlive live-track churn.
- Canonical labelling already has a durable home on `lidar_run_tracks`.
- Before this work, the main DB connection path left FK enforcement off —
  the schema contract was aspirational rather than real.

### Alternatives considered

| Option         | Approach                                    | Verdict                                                                                                    |
| -------------- | ------------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| **1 (chosen)** | Replay-own annotations + turn FKs on        | Medium effort, moderate risk; yields one clear owner per lifecycle and predictable delete semantics        |
| **2**          | Keep current table, add replay-case FK only | Low effort, **high risk**: preserves the lifecycle bug (annotations still depend on transient live tracks) |
| **3**          | Do nothing until after `v0.5.0`             | No effort now, **high risk**: release behaviour depends on FK-off connections; orphaning remains possible  |

## System boundary diagram

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

### 1. Re-key annotations onto the durable owner

Replace `lidar_track_annotations(track_id → lidar_tracks)` with
`lidar_replay_annotations` owned by replay cases. Optional linkage to
durable run tracks via `(run_id, track_id)` pointing at
`lidar_run_tracks`.

Principles:

- Replay case is the durable owner.
- Canonical track labels stay on `lidar_run_tracks`.
- Annotations must not depend on the lifetime of `lidar_tracks`.
- New writes require `replay_case_id`.

### 2. Enable foreign keys and make delete semantics real

- `PRAGMA foreign_keys = ON` in `internal/db/db.go`.
- `lidar_track_observations → lidar_tracks ON DELETE CASCADE`.
- `parent_run_id → lidar_run_records ON DELETE SET NULL`.
- Replay evaluations scoped by `(replay_case_id, reference_run_id, candidate_run_id)`.

### 3. Tighten contracts the code already assumes

Enum, range, confidence, time-order, and boolean `CHECK` constraints added
to match existing Go-side validation:

- `source_type IN ('live', 'pcap')`
- `status IN ('running', 'completed', 'failed')`
- `track_state IN ('tentative', 'confirmed', 'deleted')`
- `mode IN ('manual', 'sweep', 'auto', 'auto-tune', 'hint')`
- Confidence columns `NULL` or `[0, 1]`
- Time-order checks (`end >= start`)
- Boolean-ish columns constrained to `0/1`
- `expected_label` default changed from stale `'good_vehicle'` to `'car'`

### 4. Non-LiDAR FK-on blockers

- `site_reports.site_id` made nullable; writes `NULL` instead of `0` when
  no site is attached.
- `radar_data.data_id` PK added; `radar_transit_links` references that key.

## Key migrations

| Migration | Purpose                                                                                                                     |
| --------- | --------------------------------------------------------------------------------------------------------------------------- |
| `000033`  | Replace `lidar_track_annotations` with `lidar_replay_annotations`; fix replay-evaluation uniqueness/delete semantics        |
| `000034`  | FK hardening: `parent_run_id` FK, enum/range/boolean `CHECK`s, nullable `site_reports.site_id`, `radar_data.data_id` repair |

## Failure registry

| Failure mode                                  | User-visible effect                  | Handling                                                                      |
| --------------------------------------------- | ------------------------------------ | ----------------------------------------------------------------------------- |
| Legacy `lidar_track_annotations` rows dropped | Historical rows lost during `000033` | Accepted break before `v0.5.0`; export manually before upgrade if rows matter |
| FK enforcement exposes orphan rows            | Upgrade blocked                      | Run integrity audit before migration; repair or quarantine violating rows     |
| Replay case delete removes annotations        | User loses free-form notes           | Document delete contract; require migration test coverage for delete paths    |
| Run delete fails due to child row references  | Delete endpoint errors after FK-on   | Make all run-owned children explicit before enabling FK-on                    |
| Report creation without site still writes `0` | Report generation breaks under FK-on | `site_reports.site_id` migrated to nullable; API writes `NULL`                |
| New enum checks reject legacy values          | Writes fail after schema change      | Backfill old taxonomy values during migration before adding `CHECK`s          |

## What shipped

- Migrations `000033` and `000034` landed.
- `internal/db/db.go` enables `PRAGMA foreign_keys = ON`.
- Label API and storage target `lidar_replay_annotations`.
- Regression coverage for replay-case delete, run delete, null-site
  reports, paired annotation links, and transit-link persistence under FK
  enforcement.
- Schema ordering tool handles self-referential FKs.

Historical live-track annotations were deliberately not preserved: the
pre-`v0.5.0` reset starts the replay-owned annotation model from a clean
slate.

## Related

- [Schema Simplification Migration 030](../../plans/schema-simplification-migration-030-plan.md)
- [Track Storage Consolidation](track-storage-consolidation.md)
