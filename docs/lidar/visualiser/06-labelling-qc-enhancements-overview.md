# Labelling + QC Enhancements (Features 1, 2, 3, 5, 7, 8, 10)

**Status:** Proposed (February 2026)

## Scope

This document defines the shared architecture for the following feature designs:

1. Track quality score with reason codes
2. Timeline event bar for lifecycle and quality events
3. One-click split/merge repair with suggestions
5. Ghost trails and velocity uncertainty cones
7. Automatic physics checks with confirmation gates
8. Priority review queue
10. Session-level QC dashboard and audit export

Feature-specific designs are in:

- `docs/lidar/visualiser/07-track-quality-score.md`
- `docs/lidar/visualiser/08-track-event-timeline-bar.md`
- `docs/lidar/visualiser/09-split-merge-repair-workbench.md`
- `docs/lidar/visualiser/10-trails-and-uncertainty-visualisation.md`
- `docs/lidar/visualiser/11-physics-checks-and-confirmation-gates.md`
- `docs/lidar/visualiser/12-priority-review-queue.md`
- `docs/lidar/visualiser/13-qc-dashboard-and-audit-export.md`

## Design Principles

- Keep `run_id + track_id` as the canonical unit for labelling and QC.
- Prefer append-only audit records for all human actions and auto-generated QC signals.
- Keep replay deterministic: QC overlays cannot change frame ordering or playback semantics.
- Defer expensive recomputation to async jobs; keep UI writes low-latency.
- Expose all derived scores with version strings for reproducibility.

## Shared Constraints

- Existing run-track API is under `/api/lidar/runs/*` in `internal/lidar/monitor/run_track_api.go`.
- Existing run-track storage is `lidar_run_tracks` in `internal/db/schema.sql`.
- macOS visualiser state and controls are in:
  - `tools/visualiser-macos/VelocityVisualiser/App/AppState.swift`
  - `tools/visualiser-macos/VelocityVisualiser/UI/ContentView.swift`
  - `tools/visualiser-macos/VelocityVisualiser/Labelling/RunTrackLabelAPIClient.swift`
- Web tracks UI remains a parity target:
  - `web/src/routes/lidar/tracks/+page.svelte`

## Shared Data Additions (Cross-Feature)

Planned schema additions across features:

- `lidar_run_track_events` (timeline events)
- `lidar_run_track_quality_history` (versioned score snapshots)
- `lidar_run_track_violations` (physics violations)
- `lidar_run_track_repairs` (split/merge operations and state)
- `lidar_review_queue_items` (prioritized QC tasks)
- `lidar_qc_audit_log` (append-only action log)

Denormalized columns to add on `lidar_run_tracks` for fast UI filtering/sorting:

- `quality_score`
- `quality_grade`
- `quality_reason_codes`
- `review_state`
- `blocked_by_violations`
- `priority_score`

## Shared API Strategy

- Extend existing `/api/lidar/runs/{run_id}/tracks` payloads with QC fields.
- Add focused endpoints for heavy operations (recompute, rebuild, export).
- Add paginated list endpoints for events, queue items, and audit logs.
- Keep payloads backward-compatible by adding optional fields only.

## Dependency Order

Recommended order:

1. Feature 7 (physics checks) + Feature 2 (event bar)
2. Feature 1 (quality score)
3. Feature 8 (review queue)
4. Feature 3 (split/merge repair)
5. Feature 5 (ghost trails + uncertainty)
6. Feature 10 (QC dashboard + export)

Rationale:

- Event generation and violation signals are required inputs for quality scoring and queue ranking.
- Repair operations must emit events and update score/queue state.
- Dashboard quality is highest after all upstream signals are present.

## Cross-Feature Milestones

- M1: Data schema and API contracts merged
- M2: Physics/event pipelines producing reliable signals
- M3: Scoring, queueing, and repair loop operational
- M4: UI enhancements complete in macOS visualiser
- M5: Dashboard and export/audit finalized

## Program Checklist

- [ ] Finalize QC taxonomy (reason codes, event types, violation types)
- [ ] Add migrations for all new tables/columns
- [ ] Add API integration tests for all new endpoints
- [ ] Add deterministic replay regression tests with QC overlays enabled
- [ ] Add performance budget checks for event rendering and queue queries
- [ ] Add release docs and operator guide for the new QC workflow
