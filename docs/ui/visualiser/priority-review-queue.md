# Priority Review Queue

- **Source plan:** `docs/plans/lidar-visualiser-priority-review-queue-plan.md`

Priority-ranked queue of tracks requiring human review, ordered by composite scoring.

## Priority Score

Composite score (0–100) computed from weighted factors:

| Factor          | Weight   | Description                                             |
|-----------------|----------|---------------------------------------------------------|
| Quality score   | High     | Lower quality → higher review priority                  |
| Violation count | High     | Unresolved physics violations                           |
| Repair risk     | Medium   | Tracks with pending split/merge suggestions             |
| Rarity          | Medium   | Unusual class or singleton tracks                       |
| Ego-proximity   | Low      | Tracks near sensor (higher confidence data, edge cases) |
| Manual flag     | Override | User-flagged tracks jump to top                         |

## Queue State Model

Stored in `lidar_review_queue_items`:

| State      | Meaning                                               |
|------------|-------------------------------------------------------|
| `OPEN`     | Awaiting review                                       |
| `CLAIMED`  | Reviewer is actively inspecting                       |
| `RESOLVED` | Review complete (accepted or corrected)               |
| `SKIPPED`  | Reviewer deferred, remains in queue at lower priority |

## Queue Refresh Triggers

The queue is rebuilt/updated when any of these change:

- Quality score recalculation
- New physics violation detected
- Split/merge repair applied or reverted
- Label change on a queued track

## API

| Endpoint                                                  | Method | Purpose                               |
|-----------------------------------------------------------|--------|---------------------------------------|
| `/api/lidar/runs/{run_id}/review-queue`                   | GET    | List queue items (sorted by priority) |
| `/api/lidar/runs/{run_id}/review-queue/{item_id}/claim`   | POST   | Claim item for review                 |
| `/api/lidar/runs/{run_id}/review-queue/{item_id}/release` | POST   | Release claim without resolving       |
| `/api/lidar/runs/{run_id}/review-queue/{item_id}/resolve` | POST   | Mark resolved with outcome            |
| `/api/lidar/runs/{run_id}/review-queue/rebuild`           | POST   | Force full queue rebuild              |

## Concurrency

- **Optimistic locking:** claim/resolve operations check `updated_at` to detect stale state.
- **Stale claim expiration:** claims older than a configurable timeout (default 30 minutes) are automatically released back to `OPEN`.