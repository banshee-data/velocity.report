# Design: Automatic Physics Checks and Confirmation Gates (Feature 7)

**Status:** Proposed (February 2026)

## Objective

Automatically detect physically implausible track behaviour and block manual "confirmed" review state until violations are resolved or explicitly overridden.

## Goals

- Catch major kinematic errors early.
- Prevent low-quality confirmations from entering ground-truth datasets.
- Provide clear, actionable violation explanations to reviewers.

## Non-Goals

- Replacing tracker state transitions (`tentative|confirmed|deleted`).
- Replacing human judgment for rare edge cases.

## Rule Set

Class-aware thresholds (initial defaults):

- Max acceleration (`m/s^2`)
- Max jerk (`m/s^3`)
- Max heading change rate (`deg/s`)
- Position teleport threshold (`m/frame`)
- Bounding-box size delta threshold (`%/sec`)
- Speed continuity gaps (sudden stop/start without evidence)

Violation types:

- `ACCELERATION_EXCEEDED`
- `JERK_EXCEEDED`
- `HEADING_RATE_EXCEEDED`
- `TELEPORT_DETECTED`
- `SIZE_CHANGE_EXCEEDED`
- `SPEED_DISCONTINUITY`

## Review State Model

Add run-track review fields:

- `review_state`: `PENDING|BLOCKED|CONFIRMED|OVERRIDDEN`
- `blocked_by_violations`: INTEGER (0/1 boolean flag)
- `override_reason`: string
- `reviewed_by`: string
- `reviewed_at_ns`: int64

Gate logic:

- if unresolved `ERROR` violations exist, set `review_state=BLOCKED`.
- allow explicit override with mandatory reason.

## Data Model

Add table `lidar_run_track_violations`:

- `violation_id TEXT PRIMARY KEY`
- `run_id TEXT NOT NULL`
- `track_id TEXT NOT NULL`
- `violation_type TEXT NOT NULL`
- `severity TEXT NOT NULL` (`WARN|ERROR`)
- `start_ns INTEGER NOT NULL`
- `end_ns INTEGER`
- `peak_value REAL`
- `threshold_value REAL`
- `evidence_json TEXT`
- `status TEXT NOT NULL` (`OPEN|ACKED|RESOLVED|OVERRIDDEN`)
- `created_at_ns INTEGER NOT NULL`
- `updated_at_ns INTEGER NOT NULL`

Add columns to `lidar_run_tracks`:

- `review_state TEXT`
- `blocked_by_violations INTEGER DEFAULT 0`
- `override_reason TEXT`
- `reviewed_by TEXT`
- `reviewed_at_ns INTEGER`

Indexes:

- `(run_id, track_id, status)`
- `(run_id, severity, status)`

## Backend Design

Add rule engine package `internal/lidar/qc/physics_rules.go`:

- class-specific thresholds,
- sliding-window violation detection,
- dedupe logic for repeated frame-level spikes.

Add orchestration package `internal/lidar/qc/violations.go`:

- evaluate track on ingestion and post-edit,
- persist violations,
- update review gate fields,
- emit events (Feature 2).

## API Contract

- `GET /api/lidar/runs/{run_id}/tracks/{track_id}/violations`
- `POST /api/lidar/runs/{run_id}/violations/recompute`
- `POST /api/lidar/runs/{run_id}/tracks/{track_id}/review/confirm`
- `POST /api/lidar/runs/{run_id}/tracks/{track_id}/review/override`

Request requirements:

- confirm request fails when blocking violations exist.
- override request requires `override_reason` and `reviewed_by`.

## macOS UI Design

Files:

- `tools/visualiser-macos/VelocityVisualiser/UI/ContentView.swift`
- `tools/visualiser-macos/VelocityVisualiser/App/AppState.swift`
- `tools/visualiser-macos/VelocityVisualiser/Labelling/RunTrackLabelAPIClient.swift`

UI additions:

- Violation section in Track Inspector:
  - grouped by severity/type,
  - inline evidence preview,
  - jump-to-event controls.
- Review actions:
  - `Confirm Track` button disabled when blocked,
  - `Override and Confirm` flow with reason input.

## Web Parity

Files:

- `web/src/lib/components/lidar/TrackList.svelte`
- `web/src/lib/components/lidar/TimelinePane.svelte`

Add warning badges and confirm/override actions.

## Observability

Metrics:

- violations per 100 tracks by type
- blocked-track count per run
- override rate per reviewer
- false-positive feedback counts

## Task Checklist

### Data and Migrations

- [ ] Add `lidar_run_track_violations` table
- [ ] Add review-state columns to `lidar_run_tracks`
- [ ] Add indexes for active violation queries
- [ ] Add migration tests

### Backend

- [ ] Implement physics rule engine with class-specific thresholds
- [ ] Implement violation persistence/update lifecycle
- [ ] Implement review-state gate transitions
- [ ] Emit events for violation open/resolve/override
- [ ] Trigger quality-score refresh on violation changes

### API

- [ ] Add violations list endpoint
- [ ] Add recompute endpoint
- [ ] Add confirm/override endpoints with validation
- [ ] Add API tests for blocked confirm and override requirements

### macOS

- [ ] Add violation and review-state models to API client
- [ ] Add inspector violation UI and jump links
- [ ] Add confirm/override controls and modal for reason capture
- [ ] Add UI tests for blocked and overridden states

### Web

- [ ] Extend run-track and violation models in TypeScript
- [ ] Add warning and review-state badges in list/timeline
- [ ] Add confirm/override controls

### Testing and Validation

- [ ] Unit tests per violation type and threshold class
- [ ] Integration test for violation -> blocked -> override flow
- [ ] Replay regression test with known invalid tracks
- [ ] Threshold calibration harness against labelled datasets

### Documentation

- [ ] Publish threshold defaults by class
- [ ] Document override policy and audit expectations

## Acceptance Criteria

- Blocking violations prevent confirmation until resolved/overridden.
- Override actions always capture user and reason.
- Violations appear in UI within 1 second of detection.
- False-positive rate remains within agreed threshold after calibration.

## Open Questions

- Should override be allowed for `ERROR` severity only with secondary reviewer?
- Should thresholds be sensor-profile specific or globally fixed?
- Should violation recalculation occur synchronously after repairs?
