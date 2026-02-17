# Design: Session-Level QC Dashboard and Audit Export (Feature 10)

**Status:** Proposed (February 2026)

## Objective

Provide run/session-level QC visibility and exportable audit logs for training traceability and review governance.

## Goals

- Show QC health at a glance for each run.
- Support drill-down from aggregates to track-level evidence.
- Export reproducible JSON/CSV artifacts for downstream pipelines.

## Non-Goals

- Enterprise BI replacement.
- Long-term data warehouse design.

## Dashboard Metrics

Core run metrics:

- class distribution and drift from baseline
- quality score distribution (`A-F` buckets)
- violation counts by type/severity
- blocked/confirmed/overridden review-state counts
- split/merge repair activity
- queue throughput (`open`, `claimed`, `resolved`)
- labelling progress and reviewer activity

Time-series metrics:

- events per minute by category
- queue backlog over time
- quality trend over run duration

## Audit Model

Add append-only table `lidar_qc_audit_log`:

- `audit_id TEXT PRIMARY KEY`
- `run_id TEXT NOT NULL`
- `track_id TEXT`
- `action_type TEXT NOT NULL`
- `actor_id TEXT`
- `source TEXT NOT NULL` (`api|system|worker`)
- `before_json TEXT`
- `after_json TEXT`
- `metadata_json TEXT`
- `timestamp_ns INTEGER NOT NULL`

Action examples:

- label updates
- quality recomputes
- violation state changes
- repair apply/revert
- queue claim/resolve
- confirm/override actions

Indexes:

- `(run_id, timestamp_ns)`
- `(run_id, action_type)`
- `(run_id, track_id, timestamp_ns)`

## Summary Materialization

Add optional summary table `lidar_qc_run_summary`:

- one row per run,
- cached aggregates for dashboard load speed,
- recomputed incrementally and on-demand rebuild.

## API Contract

- `GET /api/lidar/runs/{run_id}/qc/summary`
- `GET /api/lidar/runs/{run_id}/qc/timeseries?bucket=1m|5m|15m`
- `GET /api/lidar/runs/{run_id}/qc/audit?action_type=&track_id=&limit=&cursor=`
- `GET /api/lidar/runs/{run_id}/qc/export?format=json|csv`
- `POST /api/lidar/runs/{run_id}/qc/rebuild`

Export payload (JSON):

- run metadata
- schema versions (`scorer_version`, `rules_version`, `queue_version`)
- all QC aggregates
- audit events (optionally filtered)

CSV export set:

- `run_summary.csv`
- `track_quality.csv`
- `violations.csv`
- `repairs.csv`
- `review_queue.csv`
- `audit_log.csv`

## macOS UI Design

Files:

- `tools/visualiser-macos/VelocityVisualiser/UI/RunBrowserView.swift`
- `tools/visualiser-macos/VelocityVisualiser/UI/ContentView.swift`
- `tools/visualiser-macos/VelocityVisualiser/App/AppState.swift`
- `tools/visualiser-macos/VelocityVisualiser/Labelling/RunTrackLabelAPIClient.swift`

UI additions:

- QC dashboard tab in run browser:
  - summary cards,
  - compact trend charts,
  - top-risk tracks list.
- Audit panel:
  - filter by action type and track,
  - jump from audit event to timestamp.
- Export controls:
  - JSON or CSV,
  - filtered/full export options.

## Web Parity

Files:

- `web/src/routes/lidar/tracks/+page.svelte`
- `web/src/lib/api.ts`

Add read-only summary widgets and export button.

## Compliance and Traceability

- Include scorer/rules/queue config versions in all exports.
- Include UTC timestamps (`ns`) for all audit events.
- Preserve immutable audit records (no updates/deletes outside retention jobs).

## Task Checklist

### Data and Migrations

- [ ] Add `lidar_qc_audit_log` table and indexes
- [ ] Add `lidar_qc_run_summary` materialized summary table
- [ ] Add migration tests and schema docs update

### Backend

- [ ] Implement audit writer utility and hook into all QC mutations
- [ ] Implement summary aggregator and incremental updater
- [ ] Implement export builders (JSON and CSV bundle)
- [ ] Implement rebuild job for summary regeneration

### API

- [ ] Add summary, timeseries, audit, export, and rebuild endpoints
- [ ] Add pagination and filter support for audit endpoint
- [ ] Add API tests for export correctness and version metadata

### macOS

- [ ] Add QC dashboard models and API calls
- [ ] Build run browser dashboard tab
- [ ] Build audit list panel with filters and jump-to-time
- [ ] Add export controls and file-save flow
- [ ] Add UI tests for dashboard and export actions

### Web

- [ ] Add QC summary and export API bindings
- [ ] Add summary cards and export action in tracks page
- [ ] Add tests for data formatting and export triggers

### Testing and Validation

- [ ] Unit tests for aggregate metric calculations
- [ ] End-to-end test from review actions to audit log entries
- [ ] Export snapshot tests for JSON and CSV schema stability
- [ ] Performance test for summary rebuild on large runs

### Documentation

- [ ] Document dashboard metric definitions
- [ ] Document audit event schema and retention policy
- [ ] Document export file schema for downstream consumers

## Acceptance Criteria

- Dashboard loads run summary in under 1 second for typical runs.
- Audit log captures all QC-changing actions with actor and timestamp.
- Export output is reproducible and includes version metadata.
- Operators can trace any confirmed track back through violations, repairs, and label history.

## Open Questions

- Should export include raw observation-level data or QC-only artifacts?
- What retention period is required for audit logs?
- Should dashboard include inter-annotator agreement once dual-review mode exists?
