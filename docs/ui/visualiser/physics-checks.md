# Physics checks and confirmation gates

- **Source plan:** `docs/plans/lidar-visualiser-physics-checks-and-confirmation-gates-plan.md`

Automatic physics violation detection and review-gate state model for track confirmation.

## Violation types

6 automatic physics violation detectors:

| Type                    | Trigger                                                               | Severity      |
| ----------------------- | --------------------------------------------------------------------- | ------------- |
| `ACCELERATION_EXCEEDED` | Observation-to-observation acceleration above class-specific limit    | WARN or ERROR |
| `JERK_EXCEEDED`         | Rate of acceleration change above threshold                           | WARN          |
| `HEADING_RATE_EXCEEDED` | Heading change rate faster than vehicle dynamics allow                | WARN          |
| `TELEPORT_DETECTED`     | Position jump exceeds maximum plausible displacement for elapsed time | ERROR         |
| `SIZE_CHANGE_EXCEEDED`  | Bounding box dimensions change beyond physical possibility            | WARN          |
| `SPEED_DISCONTINUITY`   | Abrupt speed change without corresponding acceleration evidence       | ERROR         |

Thresholds are class-specific (e.g. pedestrian vs car vs cyclist).

## Review state model

Each track has a review state derived from its violations:

```
PENDING → BLOCKED → CONFIRMED
                  → OVERRIDDEN
```

- **PENDING:** No unresolved ERROR violations. Track can be confirmed.
- **BLOCKED:** One or more unresolved ERROR-severity violations. Track cannot be confirmed until violations are resolved or overridden.
- **CONFIRMED:** All violations resolved and track accepted by reviewer.
- **OVERRIDDEN:** Reviewer explicitly accepted the track despite unresolved violations (mandatory reason required).

### Gate logic

- Unresolved ERROR violations → state = `BLOCKED`.
- WARN violations do not block confirmation but are visible in the UI.
- Override requires a mandatory free-text reason stored in the audit log.

## Storage

`lidar_run_track_violations` table:

| Column              | Type       | Description                                     |
| ------------------- | ---------- | ----------------------------------------------- |
| `id`                | INTEGER PK | Auto-increment                                  |
| `run_id`            | TEXT       | Run identifier                                  |
| `track_id`          | TEXT       | Track identifier                                |
| `violation_type`    | TEXT       | One of the 6 types above                        |
| `severity`          | TEXT       | WARN or ERROR                                   |
| `frame_id`          | INTEGER    | Frame where violation detected                  |
| `timestamp_ns`      | INTEGER    | Nanosecond timestamp                            |
| `observed_value`    | REAL       | The measured value that triggered the violation |
| `threshold_value`   | REAL       | The threshold that was exceeded                 |
| `resolution_state`  | TEXT       | OPEN, RESOLVED, OVERRIDDEN                      |
| `resolution_reason` | TEXT       | Free-text (required for OVERRIDDEN)             |
| `created_at`        | TEXT       | ISO 8601                                        |
| `resolved_at`       | TEXT       | ISO 8601, nullable                              |
