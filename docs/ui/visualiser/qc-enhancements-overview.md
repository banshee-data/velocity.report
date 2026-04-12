# QC enhancements overview

- **Source plan:** `docs/plans/lidar-visualiser-labelling-qc-enhancements-overview-plan.md`

Umbrella architecture for seven QC features that share tables, conventions, and dependency ordering.

## Design principles

- `run_id` + `track_id` is the canonical identity for all QC artefacts.
- All QC tables are **append-only** for audit reproducibility.
- Deterministic replay: the same input frames + scorer version must produce identical quality scores.

## Shared database tables

Six new tables are introduced across the QC feature set:

| Table                             | Owner Feature         | Purpose                           |
| --------------------------------- | --------------------- | --------------------------------- |
| `lidar_run_track_violations`      | Physics Checks        | Per-observation violation records |
| `lidar_run_track_events`          | Track Event Timeline  | Lifecycle and diagnostic events   |
| `lidar_run_track_quality_history` | Track Quality Scoring | Score snapshots with reason codes |
| `lidar_review_queue_items`        | Priority Review Queue | Queue state per track             |
| `lidar_run_track_repairs`         | Split/Merge Repair    | Non-destructive repair layer      |
| `lidar_qc_audit_log`              | QC Dashboard          | Append-only action log            |

Denormalised columns are added to `lidar_run_tracks`:

- `quality_score` (REAL)
- `quality_grade` (TEXT, A–F)
- `quality_reason_codes` (TEXT, comma-separated)
- `violation_count` (INTEGER)
- `unresolved_violation_count` (INTEGER)
- `review_state` (TEXT: PENDING, BLOCKED, CONFIRMED, OVERRIDDEN)

## Feature dependency order

Features must be implemented in this sequence (each depends on outputs of its predecessors):

1. **Physics Checks**: violation records are inputs to scoring and review queue
2. **Track Event Timeline**: events feed from violations, scoring, repairs
3. **Track Quality Scoring**: consumes violations and track observations
4. **Priority Review Queue**: consumes scores, violations, repair state
5. **Split/Merge Repair**: consumes queue items, emits events and audit entries
6. **Trails and Uncertainty**: visual-only, no persistent state, can proceed in parallel
7. **QC Dashboard and Audit**: aggregates all prior tables for reporting

## Cross-Feature milestones

| Milestone | Content                                                      |
| --------- | ------------------------------------------------------------ |
| M1        | Schema migration: all 6 shared tables + denormalised columns |
| M2        | Physics checks + event emitters operational                  |
| M3        | Quality scoring + review queue functional                    |
| M4        | Split/merge repair + trails rendering                        |
| M5        | QC dashboard + audit export + CSV compliance                 |
