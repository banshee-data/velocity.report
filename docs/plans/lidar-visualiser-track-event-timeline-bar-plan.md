# Design: Timeline Event Bar (Feature 2)

Status: Planned
Purpose/Summary: lidar-visualiser-track-event-timeline-bar.

**Status:** Proposed (February 2026)

## Objective

Add an event lane to replay controls so reviewers can jump directly to critical moments:

- track birth/death,
- split/merge signals,
- class changes,
- low-confidence or physics-warning windows,
- manual edit actions.

## Goals

- Make failure discovery non-linear (event-first instead of frame-by-frame).
- Keep events queryable for downstream QC analytics.
- Maintain deterministic replay while improving navigation speed.

## Non-Goals

- Full anomaly detection system.
- Replacing current timeline scrub behaviour.

## Event Taxonomy

Event types (`event_type`):

- `TRACK_BIRTH`
- `TRACK_DEATH`
- `TRACK_CONFIRM`
- `TRACK_DELETE`
- `CLASS_CHANGE`
- `QUALITY_DROP`
- `SPLIT_SUSPECTED`
- `MERGE_SUSPECTED`
- `PHYSICS_WARNING`
- `LABEL_UPDATED`
- `REPAIR_APPLIED`
- `REPAIR_REVERTED`

Event severity (`severity`):

- `INFO`
- `WARN`
- `ERROR`

## Data Model

Add table `lidar_run_track_events`:

- `event_id TEXT PRIMARY KEY`
- `run_id TEXT NOT NULL`
- `track_id TEXT`
- `event_type TEXT NOT NULL`
- `severity TEXT NOT NULL`
- `timestamp_ns INTEGER NOT NULL`
- `frame_index INTEGER`
- `source TEXT NOT NULL` (`tracker|qc|labeler|repair`)
- `payload_json TEXT`
- `created_at_ns INTEGER NOT NULL`

Indexes:

- `(run_id, timestamp_ns)`
- `(run_id, track_id, timestamp_ns)`
- `(run_id, event_type, severity)`

## Event Generation Pipeline

### Online emitters

Emit during run processing for lifecycle transitions:

- birth/death/confirm/delete
- class changes

### Async emitters

Emit from QC jobs:

- quality drops
- physics warnings

### User-action emitters

Emit from API handlers:

- label updates
- split/merge repair actions

## Backend Design

Add `internal/lidar/qc/events.go`:

- event validation,
- idempotency key generation,
- write helpers,
- rebuild function for backfill.

Add API route handlers in `internal/lidar/monitor/run_track_api.go`:

- query events by time/type/track,
- jump-target lookups,
- rebuild events endpoint.

## API Contract

- `GET /api/lidar/runs/{run_id}/events?start_ns=&end_ns=&types=&severity=&track_id=&limit=&cursor=`
- `POST /api/lidar/runs/{run_id}/events/rebuild`
- `GET /api/lidar/runs/{run_id}/events/highlights` (summary for UI badges)

Response shape:

- `events[]`
- `next_cursor`
- `count`
- `highlights` (counts by type/severity)

## macOS UI Design

Files:

- `tools/visualiser-macos/VelocityVisualiser/UI/ContentView.swift`
- `tools/visualiser-macos/VelocityVisualiser/App/AppState.swift`
- `tools/visualiser-macos/VelocityVisualiser/Labelling/RunTrackLabelAPIClient.swift`

UI additions:

- Add an event lane below replay slider in `PlaybackControlsView`.
- Event markers colored by severity and shaped by type.
- Hover tooltip with event summary.
- Click marker to seek to `timestamp_ns`.
- Filter chips (`all`, `warnings`, `repairs`, `label changes`).

## Web Parity

Files:

- `web/src/lib/components/lidar/TimelinePane.svelte`
- `web/src/lib/api.ts`
- `web/src/lib/types/lidar.ts`

Add event lane and event-filter controls.

## Performance and Caching

- Cache event windows by run and viewport time range.
- Server-side pagination for large runs.
- Prefetch nearest +/- 10% of current viewport for smooth scrubbing.

## Task Checklist

### Data and Migrations

- [ ] Add `lidar_run_track_events` table and indexes
- [ ] Add migration tests for schema consistency

### Backend

- [ ] Implement event writer and validator in `internal/lidar/qc/events.go`
- [ ] Hook lifecycle event emission into tracking pipeline
- [ ] Hook label/repair APIs to emit user action events
- [ ] Implement event backfill/rebuild job

### API

- [ ] Add event list endpoint with pagination and filters
- [ ] Add event rebuild endpoint
- [ ] Add highlight summary endpoint
- [ ] Add API tests for filters, paging, and idempotency

### macOS

- [ ] Add event models to `RunTrackLabelAPIClient.swift`
- [ ] Add event lane UI in `PlaybackControlsView`
- [ ] Wire marker click-to-seek through `AppState.seek(...)`
- [ ] Add event filters and tooltips
- [ ] Add snapshot/UI tests for event rendering

### Web

- [ ] Add event models and API functions
- [ ] Add event lane in `TimelinePane.svelte`
- [ ] Add keyboard shortcuts for next/previous warning event

### Testing and Validation

- [ ] Unit tests for event taxonomy validation
- [ ] Integration tests for event generation from label updates
- [ ] Replay test confirming event jump lands on expected frame index
- [ ] Load test with runs containing >100k events

### Documentation

- [ ] Publish event type definitions and severity policy
- [ ] Add operator guide for event-filter workflows

## Acceptance Criteria

- Event markers render for runs with at least 10k events without timeline lag.
- Clicking an event seeks to target frame/timestamp within 200 ms median.
- Event filters return deterministic counts across repeated queries.
- Label updates and repairs appear in event bar within 1 second.

## Open Questions

- Should dense event clusters collapse into aggregate markers at lower zoom levels?
- Should event severity be immutable once written?
- Should we support custom operator-defined event tags in payloads?
