# Design: One-Click Split/Merge Repair Workbench (Feature 3)

**Status:** Proposed (February 2026)

## Objective

Provide operator-facing, auditable split/merge repair tools with algorithmic suggestions and one-click apply/rollback.

## Goals

- Reduce manual correction time for identity errors.
- Make each repair reversible and traceable.
- Feed repair outcomes back into quality score and review queue.

## Non-Goals

- Rewriting raw source point cloud data.
- Automatic silent edits without operator approval.

## Repair Model

Repairs are run-scoped and non-destructive:

- Underlying tracked observations remain unchanged.
- A repair layer defines effective identity mapping for review/export.
- All derived views (timeline, score, queue, exports) resolve through repair layer.

Repair types:

- `MERGE`: multiple track IDs should be one logical object.
- `SPLIT`: one track ID should be multiple logical objects over time.

## Suggestion Engine

Candidate features:

- temporal overlap and handoff continuity,
- centroid/velocity continuity,
- box size consistency,
- class compatibility,
- conflict with existing labels/flags.

Suggestion output:

- `suggestion_id`
- `repair_type`
- `confidence_score` (`0-1`)
- `evidence_summary`
- `proposed_payload`

## Data Model

### Repair operations

Add table `lidar_run_track_repairs`:

- `repair_id TEXT PRIMARY KEY`
- `run_id TEXT NOT NULL`
- `repair_type TEXT NOT NULL` (`MERGE|SPLIT`)
- `status TEXT NOT NULL` (`PROPOSED|APPLIED|REVERTED|REJECTED`)
- `source_track_ids_json TEXT NOT NULL`
- `result_track_ids_json TEXT`
- `payload_json TEXT NOT NULL`
- `confidence_score REAL`
- `created_by TEXT`
- `created_at_ns INTEGER NOT NULL`
- `applied_at_ns INTEGER`
- `reverted_at_ns INTEGER`

### Repair suggestions

Add table `lidar_run_track_repair_suggestions`:

- `suggestion_id TEXT PRIMARY KEY`
- `run_id TEXT NOT NULL`
- `repair_type TEXT NOT NULL`
- `source_track_ids_json TEXT NOT NULL`
- `payload_json TEXT NOT NULL`
- `confidence_score REAL NOT NULL`
- `reasons_json TEXT`
- `status TEXT NOT NULL` (`OPEN|ACCEPTED|REJECTED|EXPIRED`)
- `created_at_ns INTEGER NOT NULL`

Indexes:

- `(run_id, status, confidence_score DESC)`
- `(run_id, created_at_ns DESC)`

## Backend Design

Add modules:

- `internal/lidar/qc/repair_engine.go` (suggestion generation)
- `internal/lidar/qc/repair_store.go` (CRUD and status transitions)
- `internal/lidar/qc/repair_resolver.go` (effective identity resolution)

Resolution strategy:

- For merges, map all source IDs to a canonical effective ID.
- For splits, assign time segments to effective IDs via boundary timestamps.
- Persist resolved mapping for deterministic playback/session export.

## API Contract

- `GET /api/lidar/runs/{run_id}/repair-suggestions?status=&limit=&cursor=`
- `POST /api/lidar/runs/{run_id}/repair-suggestions/{suggestion_id}/accept`
- `POST /api/lidar/runs/{run_id}/repair-suggestions/{suggestion_id}/reject`
- `POST /api/lidar/runs/{run_id}/repairs/{repair_id}/revert`
- `GET /api/lidar/runs/{run_id}/repairs?status=&limit=&cursor=`

On accept:

- apply repair transaction,
- emit event (Feature 2),
- trigger quality recompute (Feature 1),
- refresh queue priority (Feature 8),
- append audit log (Feature 10).

## macOS UI Design

Files:

- `tools/visualiser-macos/VelocityVisualiser/UI/ContentView.swift`
- `tools/visualiser-macos/VelocityVisualiser/App/AppState.swift`
- `tools/visualiser-macos/VelocityVisualiser/Labelling/RunTrackLabelAPIClient.swift`

UI additions:

- Repair panel in side inspector:
  - suggestion list sorted by confidence,
  - evidence chips (overlap, heading continuity, size continuity),
  - before/after preview in map and timeline,
  - `Apply`, `Reject`, `Rollback` actions.

Interaction requirements:

- all actions keyboard-accessible,
- explicit confirmation before apply/revert,
- clear visual indicator when a track is part of an active repair.

## Web Parity

Files:

- `web/src/lib/components/lidar/TrackList.svelte`
- `web/src/lib/components/lidar/TimelinePane.svelte`

Add optional review-repair tools for parity and remote reviewers.

## Consistency and Safety

- Use DB transaction for apply/revert operations.
- Idempotency token per mutation to avoid double-apply.
- Reject conflicting operations on already-mutated segments.

## Task Checklist

### Data and Migrations

- [ ] Add `lidar_run_track_repairs` table
- [ ] Add `lidar_run_track_repair_suggestions` table
- [ ] Add indexes for queueing and history views
- [ ] Add migration rollback tests

### Backend

- [ ] Implement suggestion engine in `internal/lidar/qc/repair_engine.go`
- [ ] Implement repair apply/revert transaction logic
- [ ] Implement effective identity resolver for downstream reads
- [ ] Emit events and audit entries on all state transitions
- [ ] Trigger score and queue refresh hooks after apply/revert

### API

- [ ] Add suggestion list and action endpoints
- [ ] Add repair history list endpoint
- [ ] Add optimistic concurrency guard fields
- [ ] Add integration tests for apply/revert and conflict cases

### macOS

- [ ] Add repair models and API client methods
- [ ] Build repair suggestion panel UI
- [ ] Build before/after preview overlays
- [ ] Add apply/reject/revert controls with confirmation dialogs
- [ ] Add UI tests for repair flows

### Web

- [ ] Add repair models to TypeScript types
- [ ] Add optional repair controls in track/timeline UI
- [ ] Add interaction tests for apply/revert actions

### Testing and Validation

- [ ] Unit tests for suggestion scoring logic
- [ ] Golden tests for resolver output after chained repairs
- [ ] End-to-end test for apply -> event -> score update
- [ ] Stress test with 1k repair operations in a single run

### Documentation

- [ ] Document merge/split semantics and operator playbook
- [ ] Document rollback behaviour and conflict resolution policy

## Acceptance Criteria

- Accepting a suggestion updates effective track identity in UI within 1 second.
- Every repair can be reverted without data loss.
- Conflicting edits are blocked with explicit user feedback.
- Repair operations are fully represented in audit exports.

## Open Questions

- Should repairs be limited to run scope or optionally promoted to scene profile hints?
- Should split boundaries be frame-index based, timestamp-based, or both?
- Should repair suggestions be re-ranked after each accepted repair?
