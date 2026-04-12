# QC dashboard and audit export

- **Source plan:** `docs/plans/lidar-visualiser-qc-dashboard-and-audit-export-plan.md`

Run-level and session-level QC dashboard with full audit trail and CSV export.

## Audit log

`lidar_qc_audit_log`: append-only table:

| Column                 | Type       | Description                                       |
| ---------------------- | ---------- | ------------------------------------------------- |
| `id`                   | INTEGER PK | Auto-increment                                    |
| `run_id`               | TEXT       | Run identifier                                    |
| `track_id`             | TEXT       | Track identifier (nullable for run-level actions) |
| `action_type`          | TEXT       | See action types below                            |
| `actor`                | TEXT       | User or system identifier                         |
| `detail_json`          | TEXT       | Structured payload specific to action type        |
| `scorer_version`       | TEXT       | Scorer code version at time of action             |
| `rules_version`        | TEXT       | Physics rules version                             |
| `queue_config_version` | TEXT       | Queue scoring config version                      |
| `created_at`           | TEXT       | ISO 8601 timestamp                                |

### Action types

- `LABEL_APPLIED`, `LABEL_REMOVED`
- `SCORE_COMPUTED`, `SCORE_OVERRIDDEN`
- `VIOLATION_DETECTED`, `VIOLATION_RESOLVED`, `VIOLATION_OVERRIDDEN`
- `REPAIR_APPLIED`, `REPAIR_REVERTED`
- `QUEUE_CLAIMED`, `QUEUE_RESOLVED`, `QUEUE_SKIPPED`
- `CONFIRM_GRANTED`, `CONFIRM_BLOCKED`

## Run summary

`lidar_qc_run_summary`: materialised table rebuilt on demand:

- Total tracks, labelled count, confirmed count, blocked count
- Quality grade distribution (A–F counts)
- Violation summary (total, resolved, unresolved by type)
- Repair summary (applied, reverted)
- Review queue drain rate

## API

| Endpoint                                 | Method | Purpose                          |
| ---------------------------------------- | ------ | -------------------------------- |
| `/api/lidar/runs/{run_id}/qc/summary`    | GET    | Run-level QC summary             |
| `/api/lidar/runs/{run_id}/qc/timeseries` | GET    | Score/violation trends over time |
| `/api/lidar/runs/{run_id}/qc/audit`      | GET    | Paginated audit log              |
| `/api/lidar/runs/{run_id}/qc/export`     | GET    | Download CSV export bundle       |
| `/api/lidar/runs/{run_id}/qc/rebuild`    | POST   | Force summary rebuild            |

## CSV export set

Six files in the export bundle:

1. `tracks.csv`: all tracks with quality scores, grades, labels
2. `violations.csv`: all violations with resolution state
3. `repairs.csv`: all repairs with acceptance state
4. `events.csv`: all track events
5. `audit.csv`: full audit log
6. `summary.csv`: run-level summary row

## Compliance requirement

All exports must include scorer version, rules version, and queue config version in headers or per-row, so that audit results can be tied to the exact configuration that produced them.
