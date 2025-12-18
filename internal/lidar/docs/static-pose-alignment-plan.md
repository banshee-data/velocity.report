# Static Pose Alignment Plan - Current Release

**Status:** Implementation Plan for Current Release  
**Date:** December 18, 2025  
**Scope:** Static LIDAR sensor deployment (roadside, fixed installations)  
**Goal:** Align with AV LIDAR integration spec (7-variable 3D bounding boxes)  
**Source of Truth:** See `av-lidar-integration-plan.md` for canonical format

---

## Executive Summary

This document outlines the immediate work needed to align the current **static LIDAR tracking system** with the canonical **7-variable 3D bounding box format** defined in `av-lidar-integration-plan.md`. This format is the AV industry standard (Waymo) and supports both current static sensors and future motion capture.

**7-Variable Format:** x, y, z (position) + length, width, height (dimensions) + heading (orientation)

**Current State:** Production-deployed static roadside LIDAR with 2D tracking  
**Release Scope:** Implement 7-variable 3D bounding boxes for static sensors  
**Out of Scope:** Moving sensors, ego-motion compensation, 23-class taxonomy (future)

**Key Insight:** Full 3D oriented bounding boxes CAN be achieved with static sensors - no motion needed!

---

## Current Implementation (Static Only)

### What Works Today

**Static Sensor Tracking:**
- ✅ Roadside LIDAR sensors (fixed position)
- ✅ 2D ground plane tracking (X, Y, VX, VY)
- ✅ Background grid learning (stationary environment)
- ✅ Kalman tracking for vehicles and pedestrians
- ✅ Classification (car, pedestrian, bird, other)
- ✅ Speed statistics (p50, p85, p98)

**Pose Handling:**
- ✅ Database has `sensor_poses` table (supports time-varying poses)
- ✅ 4x4 transformation matrix storage
- ⚠️ **Currently uses identity transform only** (sensor frame = world frame)
- ⚠️ No pose_id references in clusters/tracks (assumes static sensor)

### Current Data Structures

**Pose (existing):**
```go
type Pose struct {
    PoseID         int64
    SensorID       string
    FromFrame      FrameID      // e.g., "sensor/hesai-01"
    ToFrame        FrameID      // e.g., "site/main-st-001"
    T              [16]float64  // 4x4 row-major matrix
    ValidFromNanos int64
    ValidToNanos   *int64       // NULL = current
}
```

**WorldCluster (existing):**
```go
type WorldCluster struct {
    ClusterID   int64
    SensorID    string
    WorldFrame  FrameID
    TSUnixNanos int64
    CentroidX   float32  // World coordinates (baked in)
    CentroidY   float32
    CentroidZ   float32
    // ... other features
}
```

**TrackedObject (existing):**
```go
type TrackedObject struct {
    TrackID  string
    SensorID string
    X, Y     float32  // 2D position
    VX, VY   float32  // 2D velocity
    P        [16]float32  // 4x4 covariance
    // ... other fields
}
```

---

## Problem: Missing 3D Bounding Box Format

### Current vs Target (AV Spec)

**Current Implementation:**
- ❌ 2D position only (X, Y) - no Z tracking
- ❌ No heading/orientation - only velocity direction
- ❌ Averaged dimensions (length_avg, width_avg) - no oriented box
- ❌ 4 object classes - not AV-compatible

**Target (av-lidar-integration-plan.md):**
- ✅ 3D position (X, Y, Z) - full 3D centroid
- ✅ Heading (yaw angle in radians) - object orientation
- ✅ Oriented box dimensions (length, width, height) along heading
- ✅ 23 Waymo object classes (Phase 3, future)

**Additional Issues:**

**Issue 1: No Pose Association**
- Clusters and tracks don't reference which pose was used
- Can't re-transform historical data if calibration updated
- No pose versioning in tracking pipeline

**Issue 2: World Coordinates Baked In**
- Only world coordinates stored (sensor coordinates discarded)
- Can't recompute if pose changes

---

## Solution: Implement 7-Variable 3D Bounding Boxes

### Phase 1: Database Schema Updates

**Goal:** Add 7-variable format fields to database (x, y, z, length, width, height, heading)

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
./pcap-analyze --pcap test-data/static-capture.pcap --output results/

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
- **Current Tracking:** `foreground_tracking_plan.md` (existing implementation)
- **Schema:** `schema.sql` (database structure)

---

## Summary

This plan ensures the current static LIDAR tracking system is **future-compatible with motion capture** without adding unnecessary complexity today.

**Key Principles:**
1. **Additive only** - No breaking changes
2. **Backward compatible** - Old data works with new code
3. **Forward compatible** - New data ready for motion capture
4. **Production safe** - Zero functional changes for static sensors

**What Changes:**
- ✅ Add pose_id columns to database (nullable)
- ✅ Add pose_id fields to Go structs (optional)
- ✅ Populate pose_id for static sensors (identity transform)

**What Doesn't Change:**
- ✅ Tracking algorithm (same 2D Kalman)
- ✅ Clustering (same DBSCAN)
- ✅ Classification (same rules)
- ✅ Performance (same processing time)
- ✅ APIs (same endpoints)

**Result:** Static tracking continues to work exactly as before, but data structures are now ready for motion capture when needed.
