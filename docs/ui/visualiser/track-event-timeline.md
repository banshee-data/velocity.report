# Track event timeline

- **Source plan:** `docs/plans/lidar-visualiser-track-event-timeline-bar-plan.md`

Event lane rendered below the replay slider, showing lifecycle and diagnostic events per track.

## Event types

12 event types across three severity levels:

| Event Type        | Severity | Description                                 |
| ----------------- | -------- | ------------------------------------------- |
| `TRACK_BIRTH`     | INFO     | Track first observed                        |
| `TRACK_DEATH`     | INFO     | Track ended (timeout or out-of-range)       |
| `TRACK_CONFIRM`   | INFO     | Track confirmed by reviewer                 |
| `TRACK_DELETE`    | WARN     | Track deleted by reviewer                   |
| `CLASS_CHANGE`    | INFO     | Classification label changed                |
| `QUALITY_DROP`    | WARN     | Quality score dropped below grade threshold |
| `SPLIT_SUSPECTED` | WARN     | Potential track split detected              |
| `MERGE_SUSPECTED` | WARN     | Potential track merge detected              |
| `PHYSICS_WARNING` | ERROR    | Physics violation detected                  |
| `LABEL_UPDATED`   | INFO     | User label applied or removed               |
| `REPAIR_APPLIED`  | INFO     | Split/merge repair accepted                 |
| `REPAIR_REVERTED` | WARN     | Previously accepted repair reverted         |

## Storage

`lidar_run_track_events` table:

| Column         | Type       | Description                       |
| -------------- | ---------- | --------------------------------- |
| `id`           | INTEGER PK | Auto-increment                    |
| `run_id`       | TEXT       | Run identifier                    |
| `track_id`     | TEXT       | Track identifier                  |
| `event_type`   | TEXT       | One of the 12 types above         |
| `severity`     | TEXT       | INFO, WARN, or ERROR              |
| `frame_id`     | INTEGER    | Frame where event occurred        |
| `timestamp_ns` | INTEGER    | Nanosecond timestamp              |
| `detail_json`  | TEXT       | Event-specific structured payload |
| `created_at`   | TEXT       | ISO 8601                          |

## Event emitters

Three categories of emitter:

- **Online emitters:** fire during pipeline processing (birth, death, class change, physics violations).
- **Async emitters:** fire from background scoring or detection (quality drop, split/merge suspected).
- **User-action emitters:** fire from UI actions (confirm, delete, label update, repair apply/revert).

## UI: event lane

Rendered in `PlaybackControlsView`:

- Marker icons colour-coded by severity (info=blue, warn=amber, error=red).
- Tooltip on hover shows event type, track ID, and detail summary.
- Click marker to seek replay cursor to that frame.
- Filterable by event type and severity.
