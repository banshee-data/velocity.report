# Hesai LIDAR 7DOF Track Production - Future AV Integration

**Status:** DEFERRED - See Simplification Notes Below
**Date:** December 18, 2025 (Updated January 2026)
**Scope:** Read Hesai PCAP/live streams and produce 7DOF tracks for visualization
**Goal:** Generate industry-standard 7DOF bounding boxes from Hesai sensor data
**Source of Truth:** See `av-lidar-integration-plan.md` for 7DOF schema specification
**AV Compatibility:** Aligned with AV industry standard labeling specifications

---

## ⚠️ Simplification Notice (January 2026)

**This plan is DEFERRED for traffic monitoring deployments.**

The 7DOF tracking features in this document are **not required** for the core traffic monitoring use case. The current implementation uses a simpler 2D+velocity model which is sufficient for:

- Vehicle/pedestrian counting
- Speed measurement
- Traffic flow analysis

**Implemented instead:** See `velocity-coherent-foreground-extraction.md` for the simplified approach.

**When to implement this plan:**

- AV dataset integration (importing Waymo/nuScenes data for training)
- Research applications requiring precise 3D bounding boxes
- Integration with AV perception pipelines

**Key simplifications applied:**
| This Plan | Current Implementation |
|-----------|------------------------|
| 7DOF (x,y,z,l,w,h,heading) | 2D+velocity (x,y,vx,vy) |
| Oriented bounding boxes | Axis-aligned boxes |
| PCA-based heading | Heading from velocity |
| 6-state Kalman | EMA smoothing |
| 28-class taxonomy | 4 classes |

See `../operations/lidar-foreground-tracking-status.md` §4 for full simplification rationale.

---

## Executive Summary

This document outlines **Step 1** of the LIDAR ML pipeline: Reading Hesai Pandar40P PCAP files (or live streams) and producing **7-DOF 3D bounding box tracks** that conform to the AV industry standard schema defined in `av-lidar-integration-plan.md`.

**7-DOF Format (from av-lidar-integration-plan.md):**

- Position: center_x, center_y, center_z (meters)
- Dimensions: length, width, height (meters)
- Orientation: heading (radians, yaw angle around Z-axis)

**AV Industry Standard Compatibility:**

This implementation supports the AV industry standard labeling specification with:

- 28 fine-grained semantic categories (see `av-lidar-integration-plan.md`)
- Instance segmentation for Vehicle, Pedestrian, and Cyclist classes
- Consistent tracking IDs across frames
- Shape completion for occluded surfaces (see Phase 7 of `av-lidar-integration-plan.md`)

**Current State:** Production-deployed Hesai PCAP processing with 2D tracking
**Release Scope:** Extend to 7-DOF tracks and visualize in Svelte UI
**Next Steps:** Frame sequence extraction, classifier training, integration (future phases)

**Key Deliverable:** Real-time visualization of 7-DOF bounding boxes from Hesai sensor data

---

## Phased Implementation Plan

This plan aligns with the overall ML pipeline vision while focusing on Step 1.

### Phase 1: Read Hesai PCAP/Live → 7DOF Tracks (Current Release)

**Goal:** Process Hesai Pandar40P data and produce 7-DOF bounding box tracks

**Deliverables:**

1. Extend tracking to produce 7-DOF outputs (add heading, Z coordinate)
2. Visualize 7-DOF tracks in Svelte UI (oriented bounding boxes with heading arrows)
3. Store 7-DOF tracks in database (conforming to av-lidar-integration-plan.md schema)

**Timeline:** 2-3 weeks

### Phase 2: Extract 9-Frame Sequences (Future)

**Goal:** Extract training sequences from Hesai PCAPs that match AV dataset format

**Deliverables:**

1. Sequence extraction tool (identify tracks lasting 9+ frames)
2. Match AV dataset sampling rate (9 frames @ ~2Hz)
3. Export sequences with 7-DOF labels for annotation

**Timeline:** 1-2 weeks (after Phase 1)

### Phase 3: Build Classifier (Future)

**Goal:** Train ML classifier using AV dataset + labeled Hesai sequences

**Deliverables:**

1. Ingest AV dataset labels (see av-lidar-integration-plan.md Phase 2)
2. Combine AV dataset + Hesai sequences for training
3. Train object classifier supporting AV industry standard 28-class taxonomy:
   - **Priority 0 (Core):** Car, Truck, Bus, Pedestrian, Cyclist, Motorcyclist
   - **Priority 1 (Safety):** Bicycle, Motorcycle, Ground Animal, Bird
   - **Priority 2 (Infrastructure):** Sign, Pole, Traffic Light, Construction Cone

**Timeline:** 4-6 weeks (requires labeled data)

### Phase 4: Enhance Track Pipeline (Future)

**Goal:** Integrate ML classifier into real-time tracking pipeline

**Deliverables:**

1. Replace rule-based classifier with ML classifier
2. Update classification confidence scoring
3. Performance optimisation for real-time operation

**Timeline:** 2-3 weeks (after Phase 3)

---

## Phase 1 Details: Hesai → 7DOF Tracks

### Current Implementation Status

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

### Target Schema (from av-lidar-integration-plan.md)

**BoundingBox7DOF (target format):**

```go
// From av-lidar-integration-plan.md Section 1.1
type BoundingBox7DOF struct {
    // Center position (meters)
    CenterX float64 `json:"center_x"`
    CenterY float64 `json:"center_y"`
    CenterZ float64 `json:"center_z"`

    // Dimensions (meters)
    Length float64 `json:"length"` // Extent along local X
    Width  float64 `json:"width"`  // Extent along local Y
    Height float64 `json:"height"` // Extent along local Z

    // Heading (radians, [-π, π])
    Heading float64 `json:"heading"`
}
```

**Key Properties:**

- Zero pitch, zero roll (parallel to ground plane)
- Heading = yaw angle to rotate +X to object's forward axis
- Coordinate frame: vehicle/world frame (not sensor frame)

### Current Data Structures (to be extended)

**WorldCluster (existing):**

```go
type WorldCluster struct {
    ClusterID         int64
    SensorID          string
    TSUnixNanos       int64
    CentroidX         float32  // ✅ Have X
    CentroidY         float32  // ✅ Have Y
    CentroidZ         float32  // ✅ Have Z
    BoundingBoxLength float32  // ⚠️ Axis-aligned, not oriented
    BoundingBoxWidth  float32  // ⚠️ Axis-aligned, not oriented
    BoundingBoxHeight float32  // ✅ Have height
    // ❌ Missing: Heading angle
}
```

**TrackedObject (existing):**

```go
type TrackedObject struct {
    TrackID  string
    SensorID string
    X, Y     float32  // ❌ Missing Z coordinate
    VX, VY   float32  // ❌ Missing VZ component
    P        [16]float32  // ⚠️ 4x4 covariance (2D + velocity)

    BoundingBoxLengthAvg float32  // ⚠️ Averaged, not oriented
    BoundingBoxWidthAvg  float32  // ⚠️ Averaged, not oriented
    BoundingBoxHeightAvg float32  // ✅ Have height
    // ❌ Missing: Heading angle

    ObjectClass string  // ⚠️ Only 4 classes (car, pedestrian, bird, other)
}
```

---

## Gap Analysis: Current → 7DOF

### Required Changes for 7-DOF Compliance

| Component            | Current State        | Required Change         | Complexity |
| -------------------- | -------------------- | ----------------------- | ---------- |
| **Heading angle**    | None                 | Add heading estimation  | Medium     |
| **Z tracking**       | Assumed ground plane | Add Z to Kalman state   | Medium     |
| **Oriented box**     | Axis-aligned         | Compute along heading   | Medium     |
| **Database schema**  | Old format           | Add 7DOF columns        | Low        |
| **UI visualization** | Rectangles           | Oriented boxes + arrows | Medium     |
| **Object classes**   | 4 classes            | Support AV class enum   | Low        |

### Alignment with av-lidar-integration-plan.md

**Schema Compatibility:**

- ✅ Our WorldCluster maps to their BoundingBox7DOF
- ✅ Same coordinate conventions (meters, radians)
- ✅ Same zero pitch/roll assumption
- ⚠️ Need to add heading field
- ⚠️ Need oriented (not axis-aligned) dimensions

---

## Implementation: Hesai → 7DOF Tracks

### Task 1.1: Extend Database Schema for 7-DOF

**Goal:** Add columns to match BoundingBox7DOF from av-lidar-integration-plan.md

**Changes:**

1. **Add 3D position and heading to lidar_tracks:**

```sql
-- Add Z coordinate (currently only X, Y tracked)
ALTER TABLE lidar_tracks ADD COLUMN centroid_z REAL;

-- Add velocity Z component
ALTER TABLE lidar_tracks ADD COLUMN velocity_z REAL;

-- Add oriented bounding box dimensions
ALTER TABLE lidar_tracks ADD COLUMN bbox_length REAL;  -- Along heading
ALTER TABLE lidar_tracks ADD COLUMN bbox_width REAL;   -- Perpendicular
ALTER TABLE lidar_tracks ADD COLUMN bbox_height REAL;  -- Rename from height_p95_max
ALTER TABLE lidar_tracks ADD COLUMN bbox_heading REAL; -- Yaw angle (radians)

-- Add pose_id for future motion/calibration updates
ALTER TABLE lidar_tracks ADD COLUMN pose_id INTEGER;
ALTER TABLE lidar_tracks ADD FOREIGN KEY (pose_id)
    REFERENCES sensor_poses (pose_id);
```

2. **Add 7-variable format to lidar_track_obs:**

```sql
-- Add Z coordinate
ALTER TABLE lidar_track_obs ADD COLUMN z REAL;

-- Add velocity Z component
ALTER TABLE lidar_track_obs ADD COLUMN velocity_z REAL;

-- Add oriented bounding box
ALTER TABLE lidar_track_obs ADD COLUMN bbox_length REAL;
ALTER TABLE lidar_track_obs ADD COLUMN bbox_width REAL;
ALTER TABLE lidar_track_obs ADD COLUMN bbox_height REAL;  -- Rename from bounding_box_height
ALTER TABLE lidar_track_obs ADD COLUMN bbox_heading REAL;

-- Add pose_id
ALTER TABLE lidar_track_obs ADD COLUMN pose_id INTEGER;
ALTER TABLE lidar_track_obs ADD FOREIGN KEY (pose_id)
    REFERENCES sensor_poses (pose_id);
```

3. **Add sensor-frame storage to lidar_clusters (for re-transformation):**

```sql
ALTER TABLE lidar_clusters ADD COLUMN sensor_centroid_x REAL;
ALTER TABLE lidar_clusters ADD COLUMN sensor_centroid_y REAL;
ALTER TABLE lidar_clusters ADD COLUMN sensor_centroid_z REAL;
ALTER TABLE lidar_clusters ADD COLUMN pose_id INTEGER;
ALTER TABLE lidar_clusters ADD FOREIGN KEY (pose_id)
    REFERENCES sensor_poses (pose_id);
```

**Backward Compatibility:**

- ✅ All new columns are NULL-able
- ✅ Existing queries work unchanged
- ✅ Old data remains valid (NULL for new fields)

**Migration Script:**

```sql
-- Migration: 000012_add_pose_references.up.sql

-- Add pose_id to clusters (nullable for backward compatibility)
ALTER TABLE lidar_clusters ADD COLUMN pose_id INTEGER
    REFERENCES sensor_poses (pose_id);

-- Add sensor-frame coordinates (nullable)
ALTER TABLE lidar_clusters ADD COLUMN sensor_centroid_x REAL;
ALTER TABLE lidar_clusters ADD COLUMN sensor_centroid_y REAL;
ALTER TABLE lidar_clusters ADD COLUMN sensor_centroid_z REAL;

-- Add pose_id to track observations (nullable)
ALTER TABLE lidar_track_obs ADD COLUMN pose_id INTEGER
    REFERENCES sensor_poses (pose_id);

-- Create index for faster pose lookups
CREATE INDEX IF NOT EXISTS idx_lidar_clusters_pose
    ON lidar_clusters (pose_id);
CREATE INDEX IF NOT EXISTS idx_lidar_track_obs_pose
    ON lidar_track_obs (pose_id);
```

**Rollback Script:**

```sql
-- Migration: 000012_add_pose_references.down.sql

DROP INDEX IF EXISTS idx_lidar_track_obs_pose;
DROP INDEX IF EXISTS idx_lidar_clusters_pose;

-- SQLite doesn't support DROP COLUMN directly in older versions
-- For rollback, we'd need to recreate tables without these columns
-- For now, leaving columns is acceptable (they're just NULL)
```

### Phase 2: Go Struct Updates

**Goal:** Add 7-variable 3D bounding box fields to Go structs

**TrackedObject (updated to match AV spec):**

```go
type TrackedObject struct {
    TrackID  string
    SensorID string
    State    TrackState

    // Lifecycle
    Hits   int
    Misses int
    FirstUnixNanos int64
    LastUnixNanos  int64

    // 3D Position (world frame) - EXPANDED from 2D
    X, Y, Z float32  // Add Z coordinate

    // 3D Velocity (world frame) - EXPANDED from 2D
    VX, VY, VZ float32  // Add VZ component

    // Kalman covariance - EXPANDED from 4x4 to 6x6
    P [36]float32  // 6x6 for [x, y, z, vx, vy, vz]

    // Oriented Bounding Box (7-variable AV format) - NEW
    Length  float32  // Along heading direction
    Width   float32  // Perpendicular to heading
    Height  float32  // Vertical dimension
    Heading float32  // Yaw angle (radians)

    // Heading tracking - NEW
    HeadingRate float32  // Angular velocity (rad/s)

    // Pose reference (for future motion/calibration)
    PoseID *int64  // NULL for now (static identity pose)

    // Classification (unchanged, 23-class expansion is Phase 3)
    ObjectClass      string
    ObjectConfidence float32
    ClassificationModel string

    // Speed statistics (unchanged)
    AvgSpeedMps  float32
    PeakSpeedMps float32
    speedHistory []float32

    // Quality metrics (unchanged)
    ObservationCount int
    TrackLengthMeters float32
    TrackDurationSecs float32
    // ... other quality fields

    // History
    History []TrackPoint
}
```

**TrackObservation (updated to match AV spec):**

```go
type TrackObservation struct {
    TrackID     string
    TSUnixNanos int64
    WorldFrame  string

    // 3D Position - EXPANDED
    X, Y, Z float32  // Add Z coordinate

    // 3D Velocity - EXPANDED
    VelocityX, VelocityY, VelocityZ float32  // Add VZ
    SpeedMps float32

    // Oriented Bounding Box (7-variable format) - NEW
    BBoxLength  float32  // Along heading
    BBoxWidth   float32  // Perpendicular
    BBoxHeight  float32  // Vertical
    BBoxHeading float32  // Yaw angle (radians)

    // Features (unchanged)
    HeightP95     float32
    IntensityMean float32

    // Pose reference
    PoseID *int64  // NULL for static sensors
}
```

**WorldCluster (updated for sensor-frame storage):**

```go
type WorldCluster struct {
    ClusterID   int64
    SensorID    string
    WorldFrame  FrameID
    TSUnixNanos int64

    // World coordinates (unchanged)
    CentroidX   float32
    CentroidY   float32
    CentroidZ   float32

    // Bounding box (unchanged)
    BoundingBoxLength float32
    BoundingBoxWidth  float32
    BoundingBoxHeight float32

    // NEW: Sensor-frame coordinates (for re-transformation)
    SensorCentroidX *float32
    SensorCentroidY *float32
    SensorCentroidZ *float32

    // NEW: Pose reference
    PoseID *int64

    // Features (unchanged)
    PointsCount   int
    HeightP95     float32
    IntensityMean float32
    // ... other features
}
```

**Backward Compatibility:**

- ✅ New fields are nullable or have zero defaults
- ✅ Existing 2D code continues to work (Z=0, VZ=0)
- ✅ Heading can start at 0 (will be estimated from velocity)
- ✅ Existing APIs don't need changes

### Phase 3: Implement 7-Variable Format

**Goal:** Compute and store x, y, z, length, width, height, heading (See `av-lidar-integration-plan.md`)

**Implementation Strategy:**

1. **Add Z Position Tracking (Kalman Filter):**

```go
// Extend Kalman filter from 4-state to 6-state
// Old: [x, y, vx, vy]
// New: [x, y, z, vx, vy, vz]

func (t *TrackedObject) Predict(dt float32) {
    // Position prediction (now 3D)
    t.X += t.VX * dt
    t.Y += t.VY * dt
    t.Z += t.VZ * dt  // NEW: Z prediction

    // Velocity prediction (constant velocity)
    // VX, VY, VZ remain constant

    // Covariance prediction (6x6 matrix)
    // F = [I3x3  dt*I3x3]
    //     [03x3  I3x3   ]
    t.P = F·t.P·Fᵀ + Q  // Updated to 6x6 dimensions
}
```

2. **Estimate Heading from Velocity:**

```go
// In internal/lidar/tracking.go
func EstimateHeadingFromVelocity(vx, vy float32) float32 {
    if vx == 0 && vy == 0 {
        return 0  // Stationary, keep previous heading
    }
    return atan2(vy, vx)  // Heading in radians
}

// Update heading each frame
func (t *TrackedObject) UpdateHeading() {
    if t.VX != 0 || t.VY != 0 {
        newHeading := EstimateHeadingFromVelocity(t.VX, t.VY)
        // Smooth heading changes
        t.Heading = smoothAngle(t.Heading, newHeading, 0.3)
    }
}
```

3. **Compute Oriented Bounding Box:**

```go
// In internal/lidar/clustering.go
func ComputeOrientedBBox(points []WorldPoint, heading float32) (length, width, height float32) {
    if len(points) == 0 {
        return 0, 0, 0
    }

    // Transform to box-aligned coordinate system
    cos_h := cos(heading)
    sin_h := sin(heading)

    var minAlong, maxAlong, minPerp, maxPerp, minZ, maxZ float32
    minAlong, minPerp, minZ = math.MaxFloat32, math.MaxFloat32, math.MaxFloat32
    maxAlong, maxPerp, maxZ = -math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32

    for _, p := range points {
        // Rotate to box-aligned frame
        along := p.X*cos_h + p.Y*sin_h      // Along heading
        perp  := -p.X*sin_h + p.Y*cos_h     // Perpendicular

        minAlong = min(minAlong, along)
        maxAlong = max(maxAlong, along)
        minPerp = min(minPerp, perp)
        maxPerp = max(maxPerp, perp)
        minZ = min(minZ, p.Z)
        maxZ = max(maxZ, p.Z)
    }

    length = maxAlong - minAlong  // Along heading (forward)
    width  = maxPerp - minPerp    // Perpendicular (side-to-side)
    height = maxZ - minZ          // Vertical

    return length, width, height
}
```

4. **Alternative: PCA-Based Heading (for Parked Vehicles):**

```go
// Better for stationary objects
func EstimateHeadingFromPCA(points []WorldPoint) float32 {
    // Compute covariance matrix of XY positions
    meanX, meanY := computeMean(points)

    var cov_xx, cov_xy, cov_yy float32
    for _, p := range points {
        dx := p.X - meanX
        dy := p.Y - meanY
        cov_xx += dx * dx
        cov_xy += dx * dy
        cov_yy += dy * dy
    }

    // Principal axis (largest eigenvector)
    heading := 0.5 * atan2(2*cov_xy, cov_xx - cov_yy)
    return heading
}
```

5. **Store 7-Variable Format:**

```go
// In cmd/radar/radar.go when updating tracks
func updateTrackWith7Variables(
    track *lidar.TrackedObject,
    cluster *lidar.WorldCluster,
    clusterPoints []lidar.WorldPoint,
) {
    // Update 3D position
    track.X = cluster.CentroidX
    track.Y = cluster.CentroidY
    track.UpdateZ(cluster.CentroidZ)  // Kalman smoothing

    // Estimate heading
    if track.VX != 0 || track.VY != 0 {
        // Moving: use velocity heading
        track.Heading = EstimateHeadingFromVelocity(track.VX, track.VY)
    } else if len(clusterPoints) > 10 {
        // Stationary: use PCA heading
        track.Heading = EstimateHeadingFromPCA(clusterPoints)
    }

    // Compute oriented bounding box (7-variable format)
    track.Length, track.Width, track.Height =
        ComputeOrientedBBox(clusterPoints, track.Heading)

    // Store pose reference (static identity for now)
    track.PoseID = &currentStaticPose.PoseID

    // Store observation with 7-variable format
    obs := &lidar.TrackObservation{
        TrackID:     track.TrackID,
        TSUnixNanos: timestamp,
        X: track.X, Y: track.Y, Z: track.Z,
        VelocityX: track.VX, VelocityY: track.VY, VelocityZ: track.VZ,
        BBoxLength:  track.Length,
        BBoxWidth:   track.Width,
        BBoxHeight:  track.Height,
        BBoxHeading: track.Heading,
        PoseID:      &currentStaticPose.PoseID,
    }
    lidar.InsertTrackObservation(db, obs)
}
```

**For Static Sensors:**

- ✅ Z position from cluster centroids (not just ground plane)
- ✅ Heading from velocity (moving) or PCA (stationary)
- ✅ Oriented box: length along heading, width perpendicular
- ✅ **Same 7-variable format** as AV spec (`av-lidar-integration-plan.md`)
- ✅ **Ready for motion:** Data structures compatible with future ego-motion

---

## Implementation Plan

### PR #1: Database Schema (Static-Safe)

**Scope:** Add pose_id columns without changing functionality

**Files Changed:**

- `internal/db/migrations/000012_add_pose_references.up.sql` (NEW)
- `internal/db/migrations/000012_add_pose_references.down.sql` (NEW)
- `internal/db/schema.sql` (update with new columns)

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

### PR #2: Go Struct Updates (Static-Safe)

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

### PR #3: Populate Static Pose References

**Scope:** Start storing pose_id for static sensors

**Files Changed:**

- `cmd/radar/radar.go` (load static pose at startup, populate pose_id)
- `internal/lidar/track_store.go` (add GetCurrentPoses, InsertPose if missing)
- `internal/lidar/track_store_test.go` (test pose loading)

**Testing:**

- ✅ Static pose created at startup (if not exists)
- ✅ Clusters stored with pose_id reference
- ✅ Observations stored with pose_id reference
- ✅ Verify identity transform behavior unchanged

**Exit Criteria:**

- Static sensors populate pose_id
- All tracking behavior unchanged
- Database queries show pose_id populated

**Estimated Effort:** 2-3 days

---

## Validation

### Test Cases (Static Only)

**Test 1: Backward Compatibility**

```bash
# Start system without migration (old schema)
./velocity-report --lidar-sensor-id=test-01

# Verify tracking works
curl http://localhost:8082/api/lidar/tracks

# Apply migration
sqlite3 sensor_data.db < migrations/000012_add_pose_references.up.sql

# Restart system (new schema, NULL pose_id)
./velocity-report --lidar-sensor-id=test-01

# Verify tracking still works
curl http://localhost:8082/api/lidar/tracks
```

**Test 2: Static Pose Population**

```bash
# Start system with new code
./velocity-report --lidar-sensor-id=test-01

# Verify static pose created
sqlite3 sensor_data.db "SELECT * FROM sensor_poses WHERE sensor_id='test-01';"

# Verify clusters reference pose
sqlite3 sensor_data.db "SELECT COUNT(*) FROM lidar_clusters WHERE pose_id IS NOT NULL;"

# Verify observations reference pose
sqlite3 sensor_data.db "SELECT COUNT(*) FROM lidar_track_obs WHERE pose_id IS NOT NULL;"
```

**Test 3: Tracking Accuracy (Unchanged)**

```bash
# Process test PCAP with known tracks
./pcap-analyse --pcap test-data/static-capture.pcap --output results/

# Compare track statistics with baseline
# - Track count should match
# - Speed statistics should match
# - Classification should match
```

---

## Benefits of This Approach

### Immediate Benefits (Static Sensors)

1. **Pose Versioning Support**
   - Can update calibration without breaking historical data
   - Pose changes tracked with timestamps (valid_from_ns, valid_to_ns)
   - Re-transformation possible if calibration improves

2. **Better Metadata**
   - Know exactly which pose was used for each measurement
   - Can validate consistency across time periods
   - Audit trail for calibration changes

3. **Production Safety**
   - Zero functional changes (identity transform behavior unchanged)
   - All existing tests pass
   - Rollback possible if issues found

### Future Compatibility (Motion Capture)

When motion capture is added later:

- ✅ Data structures already support pose_id
- ✅ Sensor-frame coordinates already stored
- ✅ No database migration needed
- ✅ No data loss or re-collection needed
- ✅ Smooth transition from static to motion

**What Changes for Motion:**

- Pose loading: Load time-varying poses instead of static identity
- Tracking: Add ego-motion compensation in Kalman predict step
- Clustering: Same code, different poses referenced
- Storage: Same schema, pose_id now varies over time

---

## Out of Scope (Future Work)

The following are **explicitly NOT included** in this release:

### Moving Sensor Support

- ❌ Ego-motion compensation in tracking
- ❌ Velocity de-biasing (removing sensor velocity)
- ❌ Pose interpolation between measurements
- ❌ IMU integration

### 3D Tracking with Orientation

- ❌ 13-state Kalman filter [x, y, z, vx, vy, vz, qw, qx, qy, qz, wx, wy, wz]
- ❌ Quaternion state handling
- ❌ 3D orientation tracking
- ❌ Object rotation estimation

### 7DOF Pose Representation

- ❌ Quaternion storage (position + quaternion)
- ❌ SLERP interpolation
- ❌ Quaternion math utilities
- ❌ sensor_poses_7dof table

**Rationale:** These features are for future motion-capture scenarios. Current release focuses on making static tracking future-compatible without adding complexity.

**See:** `motion-capture-architecture.md` for complete future specification.

---

## Timeline

**Total Effort:** 5-8 days (1-2 weeks)

```
Week 1:
  Day 1-2:  PR #1 - Database schema updates
  Day 3-4:  PR #2 - Go struct updates
  Day 5-6:  PR #3 - Populate static pose references
  Day 7-8:  Testing and documentation
```

**Dependencies:** None (all changes are additive)

**Risk:** Very low (backward compatible, no functional changes)

---

## Success Criteria

**Technical:**

- ✅ All existing tests pass
- ✅ Static tracking behavior unchanged
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

## Migration for Existing Deployments

### Production Deployment Steps

1. **Backup database:**

```bash
cp /var/lib/velocity-report/sensor_data.db \
   /var/lib/velocity-report/sensor_data.db.backup
```

2. **Apply migration (system offline):**

```bash
sqlite3 /var/lib/velocity-report/sensor_data.db \
   < /path/to/000012_add_pose_references.up.sql
```

3. **Update binary:**

```bash
sudo systemctl stop velocity-report
sudo cp velocity-report /usr/local/bin/velocity-report
sudo systemctl start velocity-report
```

4. **Verify:**

```bash
# Check static pose created
sqlite3 /var/lib/velocity-report/sensor_data.db \
   "SELECT * FROM sensor_poses;"

# Check tracking works
curl http://localhost:8082/api/lidar/tracks
```

**Rollback Plan:**

```bash
# Stop service
sudo systemctl stop velocity-report

# Restore backup
cp /var/lib/velocity-report/sensor_data.db.backup \
   /var/lib/velocity-report/sensor_data.db

# Restore old binary
sudo cp velocity-report.old /usr/local/bin/velocity-report

# Start service
sudo systemctl start velocity-report
```

---

## Related Documents

- **Future Architecture:** `motion-capture-architecture.md` (complete future spec)
- **Current Tracking:** `../architecture/foreground_tracking_plan.md` (existing implementation)
- **Schema:** `../reference/schema.sql` (database structure)

---

## Phase 1 Implementation Checklist

### PR #1: Database Schema + BoundingBox7DOF Type (Week 1)

**Files:**

- `internal/db/migrations/000013_add_7dof_schema.up.sql` (NEW)
- `internal/lidar/av_types.go` (NEW - BoundingBox7DOF from av-lidar-integration-plan.md)
- `internal/db/schema.sql` (UPDATE - add 7DOF columns)

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

### PR #2: Extend Kalman Tracker to 3D + Heading (Week 2)

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

### PR #3: Compute Oriented Bounding Boxes (Week 2-3)

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

### PR #4: Svelte UI Visualization (Week 3)

**Files:**

- `web/src/lib/components/LidarTrackView.svelte` (UPDATE)
- `web/src/lib/components/Track3DPanel.svelte` (NEW)

**Tasks:**

- [ ] Render oriented rectangles (rotate by heading angle)
- [ ] Add heading arrow indicators
- [ ] Display 7DOF values in track detail panel (center_x/y/z, length, width, height, heading)
- [ ] Color-code by object class
- [ ] Add Z-height visualization (color gradient or label)

**Exit Criteria:**

- ✅ Oriented bounding boxes visible in top-down view
- ✅ Heading arrows show object orientation
- ✅ Track panel shows all 7 DOF values

---

## Success Criteria (Phase 1 Complete)

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

## Next Steps After Phase 1

**Phase 2:** Extract 9-frame sequences (1-2 weeks)

- Tool to identify tracks lasting 9+ frames
- Export sequences in format matching AV dataset
- Ready for manual labeling or classifier training

**Phase 3:** Build ML Classifier (4-6 weeks)

- Ingest AV dataset labels (see av-lidar-integration-plan.md)
- Train classifier on AV + Hesai data
- Support full AV industry standard 28-class taxonomy with priority focus:
  - P0: Car, Truck, Bus, Pedestrian, Cyclist, Motorcyclist
  - P1: Bicycle, Motorcycle, Ground Animal, Bird
  - P2: Sign, Pole, Traffic Light, Construction Cone

**Phase 4:** Integrate Classifier (2-3 weeks)

- Replace rule-based classification
- Real-time inference in tracking pipeline
- Confidence scoring and uncertainty estimation
