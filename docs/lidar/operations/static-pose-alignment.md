# Static pose alignment

Deferred plan for 7DOF track production from Hesai LiDAR data. The 7DOF features are not required for the core traffic monitoring use case. Current implementation uses a simpler 2D+velocity model.

## Source

- Plan: `docs/plans/lidar-static-pose-alignment-plan.md`
- Status: DEFERRED for traffic monitoring deployments

## Simplification applied

| This Plan (Deferred)             | Current Implementation     |
| -------------------------------- | -------------------------- |
| 7DOF (x, y, z, l, w, h, heading) | 2D+velocity (x, y, vx, vy) |
| Oriented bounding boxes          | Axis-aligned boxes         |
| PCA-based heading                | Heading from velocity      |
| 6-state Kalman                   | EMA smoothing              |
| AV taxonomy (19–32 classes)      | 4 classes                  |

Implemented instead: see `docs/lidar/architecture/velocity-foreground-extraction.md` for the simplified approach.

## When to implement

- AV dataset integration (importing Waymo/nuScenes data for training)
- Research applications requiring precise 3D bounding boxes
- Integration with AV perception pipelines

## Gap analysis: current → 7DOF

| Component        | Current State        | Required Change         | Complexity |
| ---------------- | -------------------- | ----------------------- | ---------- |
| Heading angle    | None                 | Add heading estimation  | Medium     |
| Z tracking       | Assumed ground plane | Add Z to Kalman state   | Medium     |
| Oriented box     | Axis-aligned         | Compute along heading   | Medium     |
| Database schema  | Old format           | Add 7DOF columns        | Low        |
| UI visualisation | Rectangles           | Oriented boxes + arrows | Medium     |
| Object classes   | 4 classes            | Support AV class enum   | Low        |

## Implementation summary (when activated)

**PR #1:** Database schema; add `pose_id` columns (nullable, backward-compatible).

**PR #2:** Go struct updates; add `pose_id`, sensor-frame coordinates to `WorldCluster`, `TrackObservation`, `TrackedObject`. All existing tests pass with `pose_id=NULL`.

**PR #3:** Populate static pose references; create static identity pose at startup, store `pose_id` on clusters and observations.

**PR #4 (7DOF extension):**

1. Extend Kalman filter from 4-state `[x, y, vx, vy]` to 6-state `[x, y, z, vx, vy, vz]`
2. Estimate heading from velocity (moving objects) or PCA (stationary objects)
3. Compute oriented bounding boxes: length along heading, width perpendicular
4. Store `bbox_length`, `bbox_width`, `bbox_height`, `bbox_heading` in database
5. Svelte UI: render oriented rectangles, heading arrows, 7DOF detail panel

## Benefits of static pose infrastructure

- Pose versioning: can update calibration without breaking historical data
- Audit trail for calibration changes
- Zero functional changes during initial setup (identity transform unchanged)
- Future-compatible: data structures already support `pose_id` for motion capture transition

## Related

- Motion capture architecture: `docs/lidar/operations/motion-capture.md` (full future specification)
- Current tracking: `docs/lidar/architecture/foreground-tracking.md`
