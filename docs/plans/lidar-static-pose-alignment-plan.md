# Hesai LIDAR 7DOF track production - future AV integration

- **Status:** DEFERRED - See Simplification Notes Below
- **Layers:** L4 Perception, L5 Tracks, L6 Objects
- **Scope:** Read Hesai PCAP/live streams and produce 7DOF tracks for visualisation
- **Goal:** Generate industry-standard 7DOF bounding boxes from Hesai sensor data
- **Source of Truth:** See `av-lidar-integration-plan.md` for 7DOF schema specification
- **AV Compatibility:** Aligned with AV industry standard labelling specifications
- **Canonical:** [static-pose-alignment.md](../lidar/operations/static-pose-alignment.md)

---

> **Simplification rationale, gap analysis, and benefits:** see [static-pose-alignment.md](../lidar/operations/static-pose-alignment.md).

---

## Phased implementation plan

This plan aligns with the overall ML pipeline vision while focusing on Step 1.

### Phase 1: read hesai pCAP/Live → 7DOF tracks (current release)

**Goal:** Process Hesai Pandar40P data and produce 7-DOF bounding box tracks

**Deliverables:**

1. Extend tracking to produce 7-DOF outputs (add heading, Z coordinate)
2. Visualise 7-DOF tracks in Svelte UI (oriented bounding boxes with heading arrows)
3. Store 7-DOF tracks in database (conforming to av-lidar-integration-plan.md schema)

**Timeline:** 2-3 weeks

### Phase 2: extract 9-Frame sequences (future)

**Goal:** Extract training sequences from Hesai PCAPs that match AV dataset format

**Deliverables:**

1. Sequence extraction tool (identify tracks lasting 9+ frames)
2. Match AV dataset sampling rate (9 frames @ ~2Hz)
3. Export sequences with 7-DOF labels for annotation

**Timeline:** 1-2 weeks (after Phase 1)

### Phase 3: build classifier (future)

**Goal:** Train ML classifier using AV dataset + labeled Hesai sequences

**Deliverables:**

1. Ingest AV dataset labels (see av-lidar-integration-plan.md Phase 2)
2. Combine AV dataset + Hesai sequences for training
3. Train object classifier supporting AV industry standard taxonomy
   (see §11 of [classification-maths.md](../../data/maths/classification-maths.md)
   for the full three-way mapping):
   - **Priority 0 (Core):** Car, Truck, Bus, Pedestrian, Cyclist, Motorcyclist
   - **Priority 1 (Safety):** Bicycle, Motorcycle, Ground Animal, Bird
   - **Priority 2 (Infrastructure):** Sign, Pole, Traffic Light, Construction Cone

**Timeline:** 4-6 weeks (requires labeled data)

### Phase 4: enhance track pipeline (future)

**Goal:** Integrate ML classifier into real-time tracking pipeline

**Deliverables:**

1. Replace rule-based classifier with ML classifier
2. Update classification confidence scoring
3. Performance optimisation for real-time operation

**Timeline:** 2-3 weeks (after Phase 3)

---

## Phase 1 details: hesai → 7DOF tracks

### Current implementation status

**What Works Today:**

- ✅ Hesai Pandar40P UDP packet parsing
- ✅ Frame assembly (360° rotations)
- ✅ Background grid learning (EMA-based)
- ✅ DBSCAN clustering in 3D world space
- ✅ 2D Kalman tracking (X, Y, VX, VY)
- ✅ Rule-based classification (car, pedestrian, bird, other)
- ✅ PCAP replay and analysis mode

**What's Missing for 7-DOF:**

- ❌ No heading angle (orientation)
- ❌ No Z-axis tracking in Kalman filter (assumes ground plane)
- ❌ No oriented bounding box computation
- ❌ UI shows axis-aligned boxes, not oriented
- ❌ Database schema doesn't match av-lidar-integration-plan.md

### Target schema (from av-lidar-integration-plan.md)

**BoundingBox7DOF (target format):**

**BoundingBox7DOF** fields:

| Field   | Type      | Description                |
| ------- | --------- | -------------------------- |
| CenterX | `float64` | Center position (meters)   |
| CenterY | `float64` |                            |
| CenterZ | `float64` |                            |
| Length  | `float64` | Extent along local X       |
| Width   | `float64` | Extent along local Y       |
| Height  | `float64` | Extent along local Z       |
| Heading | `float64` | Heading (radians, [-π, π]) |

**Key Properties:**

- Zero pitch, zero roll (parallel to ground plane)
- Heading = yaw angle to rotate +X to object's forward axis
- Coordinate frame: vehicle/world frame (not sensor frame)

### Current data structures (to be extended)

**WorldCluster (existing):**

**WorldCluster** fields:

| Field             | Type      | Description                   |
| ----------------- | --------- | ----------------------------- |
| ClusterID         | `int64`   |                               |
| SensorID          | `string`  |                               |
| TSUnixNanos       | `int64`   |                               |
| CentroidX         | `float32` | ✅ Have X                     |
| CentroidY         | `float32` | ✅ Have Y                     |
| CentroidZ         | `float32` | ✅ Have Z                     |
| BoundingBoxLength | `float32` | ⚠️ Axis-aligned, not oriented |
| BoundingBoxWidth  | `float32` | ⚠️ Axis-aligned, not oriented |
| BoundingBoxHeight | `float32` | ✅ Have height                |

**TrackedObject (existing):**

**TrackedObject** fields:

| Field                | Type          | Description                                      |
| -------------------- | ------------- | ------------------------------------------------ |
| TrackID              | `string`      |                                                  |
| SensorID             | `string`      |                                                  |
| X, Y                 | `float32`     | ❌ Missing Z coordinate                          |
| VX, VY               | `float32`     | ❌ Missing VZ component                          |
| P                    | `[16]float32` | ⚠️ 4x4 covariance (2D + velocity)                |
| BoundingBoxLengthAvg | `float32`     | ⚠️ Averaged, not oriented                        |
| BoundingBoxWidthAvg  | `float32`     | ⚠️ Averaged, not oriented                        |
| BoundingBoxHeightAvg | `float32`     | ✅ Have height                                   |
| ObjectClass          | `string`      | ⚠️ Only 4 classes (car, pedestrian, bird, other) |

---

## Implementation: hesai → 7DOF tracks

### Task 1.1: extend database schema for 7-DOF

**Goal:** Add columns to match BoundingBox7DOF from av-lidar-integration-plan.md

**Changes:**

1. **Add 3D position and heading to lidar_tracks:**

- Add `centroid_z` (REAL) to `lidar_tracks`
- Add `velocity_z` (REAL) to `lidar_tracks`
- Add `bbox_length` (REAL) to `lidar_tracks`
- Add `bbox_width` (REAL) to `lidar_tracks`
- Add `bbox_height` (REAL) to `lidar_tracks`
- Add `bbox_heading` (REAL) to `lidar_tracks`
- Add `pose_id` (INTEGER) to `lidar_tracks`, foreign key referencing `sensor_poses`

2. **Add 7-variable format to lidar_track_obs:**

- Add `z` (REAL) to `lidar_track_obs`
- Add `velocity_z` (REAL) to `lidar_track_obs`
- Add `bbox_length` (REAL) to `lidar_track_obs`
- Add `bbox_width` (REAL) to `lidar_track_obs`
- Add `bbox_height` (REAL) to `lidar_track_obs`
- Add `bbox_heading` (REAL) to `lidar_track_obs`
- Add `pose_id` (INTEGER) to `lidar_track_obs`, foreign key referencing `sensor_poses`

3. **Add sensor-frame storage to lidar_clusters (for re-transformation):**

- Add `sensor_centroid_x` (REAL) to `lidar_clusters`
- Add `sensor_centroid_y` (REAL) to `lidar_clusters`
- Add `sensor_centroid_z` (REAL) to `lidar_clusters`
- Add `pose_id` (INTEGER) to `lidar_clusters`, foreign key referencing `sensor_poses`

**Backward Compatibility:**

- ✅ All new columns are NULL-able
- ✅ Existing queries work unchanged
- ✅ Old data remains valid (NULL for new fields)

**Migration Script:**

- Add `pose_id` (INTEGER) to `lidar_clusters`
- Add `sensor_centroid_x` (REAL) to `lidar_clusters`
- Add `sensor_centroid_y` (REAL) to `lidar_clusters`
- Add `sensor_centroid_z` (REAL) to `lidar_clusters`
- Add `pose_id` (INTEGER) to `lidar_track_obs`
  **Rollback Script:**

-- Migration: 000012_add_pose_references.down.sql

DROP INDEX IF EXISTS idx_lidar_track_obs_pose;
DROP INDEX IF EXISTS idx_lidar_clusters_pose;

-- SQLite doesn't support DROP COLUMN directly in older versions
-- For rollback, we'd need to recreate tables without these columns
-- For now, leaving columns is acceptable (they're just NULL)

### Phase 2: Go struct updates

**Goal:** Add 7-variable 3D bounding box fields to Go structs

**TrackedObject (updated to match AV spec):**

**TrackedObject** fields:

| Field               | Type           | Description                                               |
| ------------------- | -------------- | --------------------------------------------------------- |
| TrackID             | `string`       |                                                           |
| SensorID            | `string`       |                                                           |
| State               | `TrackState`   |                                                           |
| Hits                | `int`          | Lifecycle                                                 |
| Misses              | `int`          |                                                           |
| FirstUnixNanos      | `int64`        |                                                           |
| LastUnixNanos       | `int64`        |                                                           |
| X, Y, Z             | `float32`      | Add Z coordinate                                          |
| VX, VY, VZ          | `float32`      | Add VZ component                                          |
| P                   | `[36]float32`  | 6x6 for [x, y, z, vx, vy, vz]                             |
| Length              | `float32`      | Along heading direction                                   |
| Width               | `float32`      | Perpendicular to heading                                  |
| Height              | `float32`      | Vertical dimension                                        |
| Heading             | `float32`      | Yaw angle (radians)                                       |
| HeadingRate         | `float32`      | Angular velocity (rad/s)                                  |
| PoseID              | `*int64`       | NULL for now (static identity pose)                       |
| ObjectClass         | `string`       | Classification (unchanged, 23-class expansion is Phase 3) |
| ObjectConfidence    | `float32`      |                                                           |
| ClassificationModel | `string`       |                                                           |
| AvgSpeedMps         | `float32`      | Speed statistics (unchanged)                              |
| MaxSpeedMps         | `float32`      |                                                           |
| speedHistory        | `[]float32`    |                                                           |
| ObservationCount    | `int`          | Quality metrics (unchanged)                               |
| TrackLengthMeters   | `float32`      |                                                           |
| TrackDurationSecs   | `float32`      |                                                           |
| History             | `[]TrackPoint` | History                                                   |

**TrackObservation (updated to match AV spec):**

**TrackObservation** fields:

| Field                           | Type      | Description             |
| ------------------------------- | --------- | ----------------------- |
| TrackID                         | `string`  |                         |
| TSUnixNanos                     | `int64`   |                         |
| FrameID                         | `string`  |                         |
| X, Y, Z                         | `float32` | Add Z coordinate        |
| VelocityX, VelocityY, VelocityZ | `float32` | Add VZ                  |
| SpeedMps                        | `float32` |                         |
| BBoxLength                      | `float32` | Along heading           |
| BBoxWidth                       | `float32` | Perpendicular           |
| BBoxHeight                      | `float32` | Vertical                |
| BBoxHeading                     | `float32` | Yaw angle (radians)     |
| HeightP95                       | `float32` | Features (unchanged)    |
| IntensityMean                   | `float32` |                         |
| PoseID                          | `*int64`  | NULL for static sensors |

**WorldCluster (updated for sensor-frame storage):**

**WorldCluster** fields:

| Field             | Type       | Description                                           |
| ----------------- | ---------- | ----------------------------------------------------- |
| ClusterID         | `int64`    |                                                       |
| SensorID          | `string`   |                                                       |
| FrameID           | `FrameID`  |                                                       |
| TSUnixNanos       | `int64`    |                                                       |
| CentroidX         | `float32`  | World coordinates (unchanged)                         |
| CentroidY         | `float32`  |                                                       |
| CentroidZ         | `float32`  |                                                       |
| BoundingBoxLength | `float32`  | Bounding box (unchanged)                              |
| BoundingBoxWidth  | `float32`  |                                                       |
| BoundingBoxHeight | `float32`  |                                                       |
| SensorCentroidX   | `*float32` | NEW: Sensor-frame coordinates (for re-transformation) |
| SensorCentroidY   | `*float32` |                                                       |
| SensorCentroidZ   | `*float32` |                                                       |
| PoseID            | `*int64`   | NEW: Pose reference                                   |
| PointsCount       | `int`      | Features (unchanged)                                  |
| HeightP95         | `float32`  |                                                       |
| IntensityMean     | `float32`  |                                                       |

**Backward Compatibility:**

- ✅ New fields are nullable or have zero defaults
- ✅ Existing 2D code continues to work (Z=0, VZ=0)
- ✅ Heading can start at 0 (will be estimated from velocity)
- ✅ Existing APIs don't need changes

### Phase 3: implement 7-Variable format

**Goal:** Compute and store x, y, z, length, width, height, heading (See `av-lidar-integration-plan.md`)

**Implementation Strategy:**

1. **Add Z Position Tracking (Kalman Filter):**

- Extend Kalman filter from 4-state to 6-state
- Old: [x, y, vx, vy]
- New: [x, y, z, vx, vy, vz]

**Predict** algorithm:

- Position prediction (now 3D)
- Velocity prediction (constant velocity)
- VX, VY, VZ remain constant
- Covariance prediction (6x6 matrix)
- F = [I3x3 dt*I3x3]
- [03x3 I3x3 ]

2. **Estimate Heading from Velocity:**

- In internal/lidar/tracking.go

**EstimateHeadingFromVelocity** algorithm:

- Update heading each frame

**UpdateHeading** algorithm:

- Smooth heading changes

3. **Compute Oriented Bounding Box:**

- In internal/lidar/clustering.go

**ComputeOrientedBBox** algorithm:

- Transform to box-aligned coordinate system
- Rotate to box-aligned frame

4. **Alternative: PCA-Based Heading (for Parked Vehicles):**

- Better for stationary objects

**EstimateHeadingFromPCA** algorithm:

- Compute covariance matrix of XY positions
- Principal axis (largest eigenvector)

5. **Store 7-Variable Format:**

- In cmd/radar/radar.go when updating tracks

**updateTrackWith7Variables** algorithm:

- Update 3D position
- Estimate heading
- Moving: use velocity heading
- Stationary: use PCA heading
- Compute oriented bounding box (7-variable format)
- Store pose reference (static identity for now)
- Store observation with 7-variable format
  **For Static Sensors:**

- ✅ Z position from cluster centroids (not just ground plane)
- ✅ Heading from velocity (moving) or PCA (stationary)
- ✅ Oriented box: length along heading, width perpendicular
- ✅ **Same 7-variable format** as AV spec (`av-lidar-integration-plan.md`)
- ✅ **Ready for motion:** Data structures compatible with future ego-motion

---

## Implementation plan

### PR #1: database schema (static-safe)

**Scope:** Add pose_id columns without changing functionality

**Files Changed:**

- `internal/db/migrations/000012_add_pose_references.up.sql` (NEW)
- `internal/db/migrations/000012_add_pose_references.down.sql` (NEW)
- [internal/db/schema.sql](../../internal/db/schema.sql) (update with new columns)

**Testing:**

- ✅ Run migration on test database
- ✅ Verify existing queries work with NULL pose_id
- ✅ Verify rollback migration works

**Exit Criteria:**

- Migration runs successfully
- All existing tests pass (no functionality change)
- Schema documentation updated

**Estimated Effort:** 1-2 days

---

### PR #2: Go struct updates (static-safe)

**Scope:** Add pose_id fields to structs without changing APIs

**Files Changed:**

- `internal/lidar/arena.go` (update WorldCluster, add PoseID field)
- `internal/lidar/track_store.go` (update TrackObservation, update Insert functions)
- `internal/lidar/track_store_test.go` (update tests to handle NULL pose_id)

**Testing:**

- ✅ All existing tests pass with pose_id=NULL
- ✅ New tests with pose_id populated
- ✅ Verify NULL handling in queries

**Exit Criteria:**

- Structs updated with pose_id fields
- Insert functions accept pose_id parameter (optional)
- All tests pass

**Estimated Effort:** 2-3 days

---

### PR #3: populate static pose references

**Scope:** Start storing pose_id for static sensors

**Files Changed:**

- [cmd/radar/radar.go](../../cmd/radar/radar.go) (load static pose at startup, populate pose_id)
- `internal/lidar/track_store.go` (add GetCurrentPoses, InsertPose if missing)
- `internal/lidar/track_store_test.go` (test pose loading)

**Testing:**

- ✅ Static pose created at startup (if not exists)
- ✅ Clusters stored with pose_id reference
- ✅ Observations stored with pose_id reference
- ✅ Verify identity transform behaviour unchanged

**Exit Criteria:**

- Static sensors populate pose_id
- All tracking behaviour unchanged
- Database queries show pose_id populated

**Estimated Effort:** 2-3 days

---

## Validation

### Test cases (static only)

**Test 1: Backward Compatibility**

- Start system without migration (old schema): `./velocity-report --lidar-sensor-id=test-01`
- Verify tracking works: `curl http://localhost:8082/api/lidar/tracks`
- Apply migration: `sqlite3 sensor_data.db < migrations/000012_add_pose_references.up.sql`
- Restart system (new schema, NULL pose_id): `./velocity-report --lidar-sensor-id=test-01`
- Verify tracking still works: `curl http://localhost:8082/api/lidar/tracks`
  **Test 2: Static Pose Population**

- Start system with new code: `./velocity-report --lidar-sensor-id=test-01`
- Verify static pose created: `sqlite3 sensor_data.db "SELECT * FROM sensor_poses WHERE sensor_id='test-01';"`
- Verify clusters reference pose: `sqlite3 sensor_data.db "SELECT COUNT(*) FROM lidar_clusters WHERE pose_id IS NOT NULL;"`
- Verify observations reference pose: `sqlite3 sensor_data.db "SELECT COUNT(*) FROM lidar_track_obs WHERE pose_id IS NOT NULL;"`
  **Test 3: Tracking Accuracy (Unchanged)**

## Process test PCAP with known tracks: `./pcap-analyse --pcap test-data/static-capture.pcap --output results/`

## Out of scope (future work)

The following are **explicitly NOT included** in this release:

### Moving sensor support

- ❌ Ego-motion compensation in tracking
- ❌ Velocity de-biasing (removing sensor velocity)
- ❌ Pose interpolation between measurements
- ❌ IMU integration

### 3D tracking with orientation

- ❌ 13-state Kalman filter [x, y, z, vx, vy, vz, qw, qx, qy, qz, wx, wy, wz]
- ❌ Quaternion state handling
- ❌ 3D orientation tracking
- ❌ Object rotation estimation

### 7DOF pose representation

- ❌ Quaternion storage (position + quaternion)
- ❌ SLERP interpolation
- ❌ Quaternion math utilities
- ❌ sensor_poses_7dof table

**Rationale:** These features are for future motion-capture scenarios. Current release focuses on making static tracking future-compatible without adding complexity.

**See:** `motion-capture-architecture.md` for complete future specification.

---

## Timeline

**Total Effort:** 5-8 days (1-2 weeks)

Week 1:
Day 1-2: PR #1 - Database schema updates
Day 3-4: PR #2 - Go struct updates
Day 5-6: PR #3 - Populate static pose references
Day 7-8: Testing and documentation
**Dependencies:** None (all changes are additive)

**Risk:** Very low (backward compatible, no functional changes)

---

## Success criteria

**Technical:**

- ✅ All existing tests pass
- ✅ Static tracking behaviour unchanged
- ✅ pose_id populated for new measurements
- ✅ NULL pose_id handled correctly (legacy data)

**Functional:**

- ✅ Tracking accuracy unchanged
- ✅ Performance unchanged (no regression)
- ✅ Can query tracks with/without pose_id

**Compatibility:**

- ✅ Old data works with new code (NULL pose_id)
- ✅ New data works with new code (pose_id populated)
- ✅ Schema can be extended for motion capture later

---

## Migration for existing deployments

### Production deployment steps

1. **Backup database:**

Run `cp /var/lib/velocity-report/sensor_data.db /var/lib/velocity-report/sensor_data.db.backup`

2. **Apply migration (system offline):**

Run `sqlite3 /var/lib/velocity-report/sensor_data.db < /path/to/000012_add_pose_references.up.sql`

3. **Update binary:**

- `sudo systemctl stop velocity-report`
- `sudo cp velocity-report /usr/local/bin/velocity-report`
- `sudo systemctl start velocity-report`

4. **Verify:**

- Check static pose created: `sqlite3 /var/lib/velocity-report/sensor_data.db "SELECT * FROM sensor_poses;"`
- Check tracking works: `curl http://localhost:8082/api/lidar/tracks`

**Rollback Plan:**

- Stop service: `sudo systemctl stop velocity-report`
- Restore backup: `cp /var/lib/velocity-report/sensor_data.db.backup /var/lib/velocity-report/sensor_data.db`
- Restore old binary: `sudo cp velocity-report.old /usr/local/bin/velocity-report`
- Start service: `sudo systemctl start velocity-report`

---

## Related documents

- **Future Architecture:** `motion-capture-architecture.md` (complete future spec)
- **Current Tracking:** [../lidar/architecture/foreground-tracking.md](../lidar/architecture/foreground-tracking.md) (existing implementation)
- **Schema:** [../../internal/db/schema.sql](../../internal/db/schema.sql) (database structure)

---

## Phase 1 implementation checklist

### PR #1: database schema + boundingBox7DOF type (week 1)

**Files:**

- `internal/db/migrations/000013_add_7dof_schema.up.sql` (NEW)
- `internal/lidar/av_types.go` (NEW - BoundingBox7DOF from av-lidar-integration-plan.md)
- [internal/db/schema.sql](../../internal/db/schema.sql) (UPDATE - add 7DOF columns)

**Tasks:**

- [ ] Create BoundingBox7DOF type matching av-lidar-integration-plan.md spec
- [ ] Add bbox_heading column to lidar_clusters, lidar_tracks, lidar_track_obs
- [ ] Add centroid_z to lidar_tracks (upgrade from 2D to 3D)
- [ ] Add velocity_z to lidar_tracks (upgrade velocity to 3D)
- [ ] Unit tests for BoundingBox7DOF methods (IoU, ContainsPoint, Corners)

**Exit Criteria:**

- ✅ Migration runs on test database
- ✅ All existing tests pass
- ✅ BoundingBox7DOF type matches av-lidar-integration-plan.md exactly

---

### PR #2: extend Kalman tracker to 3D + heading (week 2)

**Files:**

- `internal/lidar/tracking.go` (UPDATE - 4-state → 6-state + heading)
- `internal/lidar/arena.go` (UPDATE - add Z, Heading to TrackedObject)
- `internal/lidar/tracking_test.go` (UPDATE - 3D test cases)

**Tasks:**

- [ ] Extend Kalman filter: [x, y, vx, vy] → [x, y, z, vx, vy, vz]
- [ ] Add heading estimation from velocity: `heading = atan2(vy, vx)`
- [ ] Add Z coordinate smoothing (Kalman update for Z)
- [ ] Add heading to TrackedObject struct
- [ ] Update covariance matrix from 4x4 to 6x6

**Exit Criteria:**

- ✅ Tracks include Z coordinate (not just ground plane)
- ✅ Tracks include heading angle (radians)
- ✅ Existing 2D tracks still work (backward compatible)

---

### PR #3: compute oriented bounding boxes (week 2-3)

**Files:**

- `internal/lidar/clustering.go` (UPDATE - add ComputeOrientedBBox)
- `internal/lidar/track_store.go` (UPDATE - store 7DOF format)

**Tasks:**

- [ ] Implement `ComputeOrientedBBox(points []WorldPoint, heading float32) (length, width, height float32)`
- [ ] Rotate points to box-aligned frame using heading
- [ ] Compute min/max along heading (length) and perpendicular (width)
- [ ] Store bbox_length, bbox_width, bbox_height, bbox_heading in database
- [ ] Update cluster → track association to use oriented boxes

**Exit Criteria:**

- ✅ Bounding boxes are oriented (not axis-aligned)
- ✅ Length measured along heading direction
- ✅ Width measured perpendicular to heading

---

### PR #4: Svelte UI visualisation (week 3)

**Files:**

- `web/src/lib/components/LidarTrackView.svelte` (UPDATE)
- `web/src/lib/components/Track3DPanel.svelte` (NEW)

**Tasks:**

- [ ] Render oriented rectangles (rotate by heading angle)
- [ ] Add heading arrow indicators
- [ ] Display 7DOF values in track detail panel (center_x/y/z, length, width, height, heading)
- [ ] Colour-code by object class
- [ ] Add Z-height visualisation (colour gradient or label)

**Exit Criteria:**

- ✅ Oriented bounding boxes visible in top-down view
- ✅ Heading arrows show object orientation
- ✅ Track panel shows all 7 DOF values

---

## Success criteria (phase 1 complete)

**Functional:**

- ✅ Hesai PCAP files processed to produce 7DOF tracks
- ✅ Live Hesai stream produces 7DOF tracks in real-time
- ✅ Tracks stored in database matching av-lidar-integration-plan.md schema
- ✅ UI visualizes oriented bounding boxes with heading

**Technical:**

- ✅ BoundingBox7DOF type matches av-lidar-integration-plan.md exactly
- ✅ All existing tests pass (backward compatible)
- ✅ Performance unchanged (no regression)

**AV Industry Standard Compatibility:**

- ✅ 7-DOF bounding box format matches AV industry standard specification
- ✅ Object taxonomy supports 28 fine-grained categories
- ✅ Instance segmentation labels for Vehicle, Pedestrian, Cyclist
- ✅ Occlusion handling with shape completion (see av-lidar-integration-plan.md Phase 7)

**Compatibility:**

- ✅ Ready for Phase 2 (9-frame sequence extraction)
- ✅ Ready for Phase 3 (ML classifier integration)
- ✅ Compatible with AV dataset format for future training
- ✅ AV industry standard Parquet data can be imported for training

---

## Next steps after phase 1

**Phase 2:** Extract 9-frame sequences (1-2 weeks)

- Tool to identify tracks lasting 9+ frames
- Export sequences in format matching AV dataset
- Ready for manual labelling or classifier training

**Phase 3:** Build ML Classifier (4-6 weeks)

- Ingest AV dataset labels (see av-lidar-integration-plan.md)
- Train classifier on AV + Hesai data
- Support full AV industry standard taxonomy with priority focus
  (see §11 of [classification-maths.md](../../data/maths/classification-maths.md)
  for the three-way mapping):
  - P0: Car, Truck, Bus, Pedestrian, Cyclist, Motorcyclist
  - P1: Bicycle, Motorcycle, Ground Animal, Bird
  - P2: Sign, Pole, Traffic Light, Construction Cone

**Phase 4:** Integrate Classifier (2-3 weeks)

- Replace rule-based classification
- Real-time inference in tracking pipeline
- Confidence scoring and uncertainty estimation
