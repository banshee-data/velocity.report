# Track Quality Scoring

- **Source plan:** `docs/plans/lidar-visualiser-track-quality-score-plan.md`

Per-track quality score (0–100) with reason codes and grade classification.

## Component Weights

| Component                  | Weight | Description                                       |
| -------------------------- | ------ | ------------------------------------------------- |
| Observation stability      | 25%    | Consistency of observation count and gap patterns |
| Kinematic smoothness       | 20%    | Acceleration/jerk within physical bounds          |
| Geometry consistency       | 15%    | Bounding box size stability over time             |
| Track continuity           | 15%    | Ratio of observed frames to expected frames       |
| Classification stability   | 10%    | Class label churn rate                            |
| Violation/repair penalties | 15%    | Deductions for unresolved violations and repairs  |

## Grade Buckets

| Grade | Score Range | Meaning                              |
| ----- | ----------- | ------------------------------------ |
| A     | 90–100      | High confidence, no issues           |
| B     | 75–89       | Good quality, minor concerns         |
| C     | 60–74       | Moderate quality, review recommended |
| D     | 40–59       | Low quality, review required         |
| E     | 20–39       | Poor quality, likely needs repair    |
| F     | 0–19        | Very poor, probable tracking failure |

## Reason Codes

11 initial codes:

1. `SHORT_TRACK` — fewer than minimum observation threshold
2. `HIGH_JERK` — kinematic smoothness violation
3. `SIZE_INSTABILITY` — bounding box size variance exceeds threshold
4. `OBSERVATION_GAPS` — significant gaps in observation timeline
5. `CLASS_CHURN` — classification changed frequently
6. `UNRESOLVED_VIOLATIONS` — physics violations not yet addressed
7. `PENDING_REPAIR` — split/merge suggestion outstanding
8. `LOW_POINT_DENSITY` — insufficient points per observation
9. `SPEED_DISCONTINUITY` — abrupt speed changes beyond physical limits
10. `HEADING_INSTABILITY` — heading changes faster than vehicle dynamics allow
11. `OCCLUSION_HEAVY` — track spent significant time in occluded state

## Storage

### Denormalised on `lidar_run_tracks`

- `quality_score` (REAL) — current composite score
- `quality_grade` (TEXT) — current grade letter
- `quality_reason_codes` (TEXT) — comma-separated active reason codes

### History table: `lidar_run_track_quality_history`

| Column           | Type       | Description                |
| ---------------- | ---------- | -------------------------- |
| `id`             | INTEGER PK | Auto-increment             |
| `run_id`         | TEXT       | Run identifier             |
| `track_id`       | TEXT       | Track identifier           |
| `score`          | REAL       | Score at this point        |
| `grade`          | TEXT       | Grade at this point        |
| `reason_codes`   | TEXT       | Reason codes at this point |
| `scorer_version` | TEXT       | Version of scoring code    |
| `computed_at`    | TEXT       | ISO 8601 timestamp         |

## Implementation

Scorer lives in `internal/lidar/qc/scoring.go`. Scoring is:

- Deterministic: same inputs + same scorer version = same output.
- Idempotent: re-scoring a track overwrites the previous denormalised values and appends a new history row.

## Rollout Phases

1. Schema migration + scorer package with unit tests
2. Batch scoring for existing runs
3. Online scoring during live pipeline
4. UI integration (grade badges, score charts, reason code display)
