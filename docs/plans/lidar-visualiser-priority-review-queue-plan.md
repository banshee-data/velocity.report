# Design: Priority Review Queue by Uncertainty and Impact (Feature 8)

Status: Planned
Purpose/Summary: lidar-visualiser-priority-review-queue.

**Status:** Proposed (February 2026)

## Objective

Replace linear review order with a prioritised queue so reviewers address the most uncertain and impactful tracks first.

## Goals

- Maximise labelling/QC value per reviewer minute.
- Reduce time-to-resolution for high-risk tracks.
- Support multi-reviewer claiming and progress tracking.

## Non-Goals

- Workforce scheduling or staffing optimisation.
- Replacing full run browsing.

## Priority Scoring

Compute `priority_score` (`0-100`) from weighted factors:

- low quality score (Feature 1)
- active physics violations (Feature 7)
- split/merge suspicion and pending repairs (Feature 3)
- low confidence/class instability
- rare class weighting
- ego-path proximity weighting
- unresolved manual flags

Default formula:

- `priority = w1*(100-quality_score) + w2*violation_severity + w3*repair_risk + w4*rarity + w5*ego_proximity + w6*manual_flag_penalty`

## Queue Data Model

Add table `lidar_review_queue_items`:

- `item_id TEXT PRIMARY KEY`
- `run_id TEXT NOT NULL`
- `track_id TEXT NOT NULL`
- `priority_score REAL NOT NULL`
- `priority_components_json TEXT NOT NULL`
- `status TEXT NOT NULL` (`OPEN|CLAIMED|RESOLVED|SKIPPED`)
- `assignee TEXT`
- `claimed_at_ns INTEGER`
- `resolved_at_ns INTEGER`
- `resolution_note TEXT`
- `last_recomputed_at_ns INTEGER NOT NULL`

Constraints and indexes:

- partial unique index on `(run_id, track_id)` for active items, e.g. `CREATE UNIQUE INDEX ... WHERE status IN ('OPEN','CLAIMED')`
- index `(run_id, status, priority_score DESC)`
- index `(assignee, status)`

## Queue Refresh Strategy

Triggers:

- quality score updates,
- violation updates,
- repair actions,
- label/flag changes.

Refresh modes:

- incremental recompute per track (default)
- full run rebuild endpoint (maintenance)

## API Contract

- `GET /api/lidar/runs/{run_id}/review-queue?status=&assignee=&limit=&cursor=`
- `POST /api/lidar/runs/{run_id}/review-queue/{item_id}/claim`
- `POST /api/lidar/runs/{run_id}/review-queue/{item_id}/release`
- `POST /api/lidar/runs/{run_id}/review-queue/{item_id}/resolve`
- `POST /api/lidar/runs/{run_id}/review-queue/rebuild`

Response includes jump context:

- `start_ns`
- `end_ns`
- `top_reasons[]`

## macOS UI Design

Files:

- `tools/visualiser-macos/VelocityVisualiser/UI/ContentView.swift`
- `tools/visualiser-macos/VelocityVisualiser/App/AppState.swift`
- `tools/visualiser-macos/VelocityVisualiser/UI/RunBrowserView.swift`
- `tools/visualiser-macos/VelocityVisualiser/Labelling/RunTrackLabelAPIClient.swift`

UI additions:

- Queue panel with:
  - sorted item list,
  - status/assignee chips,
  - top-reason preview,
  - claim/release/resolve actions.
- `Jump to Segment` button seeks replay timeline to issue window.

## Web Parity

Files:

- `web/src/routes/lidar/tracks/+page.svelte`
- `web/src/lib/components/lidar/TrackList.svelte`

Add optional queue mode and claim/resolve actions.

## Concurrency Model

- Claim endpoint uses optimistic locking (status must be `OPEN`).
- Resolve endpoint requires claimant or reviewer role.
- Expire stale claims via server-side timeout policy.

## Task Checklist

### Data and Migrations

- [ ] Add `lidar_review_queue_items` table
- [ ] Add priority and assignee indexes
- [ ] Add constraints for single active item per run-track
- [ ] Add migration and rollback tests

### Backend

- [ ] Implement priority scoring function and config versioning
- [ ] Implement incremental queue recompute hooks
- [ ] Implement stale-claim expiration policy
- [ ] Integrate queue updates with score/violation/repair pipelines

### API

- [ ] Add queue list endpoint with pagination and filters
- [ ] Add claim/release/resolve endpoints
- [ ] Add rebuild endpoint
- [ ] Add API tests for concurrency and status transitions

### macOS

- [ ] Add queue item models and API methods
- [ ] Add queue panel and filters
- [ ] Add claim/release/resolve controls
- [ ] Add jump-to-segment behaviour from queue item
- [ ] Add UI tests for queue workflows

### Web

- [ ] Add queue models and API calls
- [ ] Add optional queue mode in tracks page
- [ ] Add claim/resolve controls and visual indicators

### Testing and Validation

- [ ] Unit tests for priority formula and component bounds
- [ ] Integration tests for queue status transitions
- [ ] End-to-end test: score change causes reordering
- [ ] Performance test for 50k queue items

### Documentation

- [ ] Document queue scoring factors and weights
- [ ] Document reviewer claim/resolve workflow

## Acceptance Criteria

- Queue list sorts deterministically by priority score and tie-breakers.
- Claim conflicts are handled without duplicate assignment.
- Queue updates within 2 seconds after upstream QC signal changes.
- Review throughput improves versus baseline linear workflow.

## Open Questions

- Should queue be run-scoped only or support cross-run global queue views?
- Should rare-class weighting be static or adaptive per scene/run?
- Should skipped items auto-reopen after new violations?
