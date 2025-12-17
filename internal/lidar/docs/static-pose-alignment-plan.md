# Static Pose Alignment Plan - Current Release

**Status:** Implementation Plan for Current Release  
**Date:** December 17, 2025  
**Scope:** Static LIDAR sensor deployment (roadside, fixed installations)  
**Goal:** Align current static collection with future-compatible data structures

---

## Executive Summary

This document outlines the immediate work needed to make the current **static LIDAR tracking system** compatible with future motion-capture scenarios while maintaining all existing functionality. The focus is on data structure alignment and forward compatibility, not on implementing motion capture itself.

**Current State:** Production-deployed static roadside LIDAR with 2D tracking  
**Release Scope:** Make static pose tracking future-compatible  
**Out of Scope:** Moving sensors, ego-motion compensation, 3D orientation tracking

**Key Principle:** Design data structures today that won't require migration when motion capture is added later.

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

## Problem: Future Incompatibility

### What Breaks When Adding Motion Capture?

**Issue 1: No Pose Association**
- Clusters and tracks don't reference which pose was used
- If sensor moves (future), we can't reconstruct sensor position at measurement time
- Can't re-transform historical data if calibration updated

**Issue 2: World Coordinates Baked In**
- Only world coordinates stored (sensor coordinates discarded)
- Can't recompute if pose changes
- Tied to specific calibration forever

**Issue 3: Missing Metadata**
- No tracking of which pose was "current" during measurement
- No way to validate consistency across time periods
- No support for pose versioning in tracking pipeline

---

## Solution: Add Pose References (Static-Safe)

### Phase 1: Database Schema Updates

**Goal:** Add optional pose_id to clusters and tracks without breaking existing code.

**Changes:**

1. **Add pose_id column to lidar_clusters:**
```sql
ALTER TABLE lidar_clusters ADD COLUMN pose_id INTEGER;
ALTER TABLE lidar_clusters ADD FOREIGN KEY (pose_id) 
    REFERENCES sensor_poses (pose_id);
```

2. **Add pose_id column to lidar_track_obs:**
```sql
ALTER TABLE lidar_track_obs ADD COLUMN pose_id INTEGER;
ALTER TABLE lidar_track_obs ADD FOREIGN KEY (pose_id) 
    REFERENCES sensor_poses (pose_id);
```

3. **Add optional sensor-frame storage to lidar_clusters:**
```sql
ALTER TABLE lidar_clusters ADD COLUMN sensor_centroid_x REAL;
ALTER TABLE lidar_clusters ADD COLUMN sensor_centroid_y REAL;
ALTER TABLE lidar_clusters ADD COLUMN sensor_centroid_z REAL;
```

**Backward Compatibility:**
- ✅ All new columns are NULL-able
- ✅ Existing queries work unchanged (ignore NULL pose_id)
- ✅ Existing code doesn't need to populate pose_id (can remain NULL for now)

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

**Goal:** Add pose_id fields to Go structs without changing existing APIs.

**WorldCluster (updated):**
```go
type WorldCluster struct {
    ClusterID   int64
    SensorID    string
    WorldFrame  FrameID
    TSUnixNanos int64
    
    // World coordinates (computed from sensor coords + pose)
    CentroidX   float32
    CentroidY   float32
    CentroidZ   float32
    
    // NEW: Pose reference (NULL for static sensors with identity transform)
    PoseID      *int64   // Reference to sensor_poses.pose_id
    
    // NEW: Sensor-frame coordinates (for re-transformation if pose changes)
    SensorCentroidX *float32
    SensorCentroidY *float32
    SensorCentroidZ *float32
    
    // ... existing features unchanged
}
```

**TrackObservation (updated):**
```go
type TrackObservation struct {
    TrackID     string
    TSUnixNanos int64
    WorldFrame  string
    
    // Position (world frame)
    X, Y, Z     float32
    
    // NEW: Pose reference
    PoseID      *int64  // Reference to sensor_poses.pose_id used for this obs
    
    // ... existing fields unchanged
}
```

**Backward Compatibility:**
- ✅ PoseID is `*int64` (nullable pointer)
- ✅ For static sensors, PoseID remains NULL
- ✅ Existing code doesn't need to set PoseID
- ✅ All existing APIs work unchanged

### Phase 3: Populate Pose References (Static Only)

**Goal:** Start storing pose_id for new measurements without changing behavior.

**Implementation Strategy:**

1. **Load Current Static Pose at Startup:**
```go
// In cmd/radar/radar.go at startup
func loadOrCreateStaticPose(db *sql.DB, sensorID string) (*lidar.Pose, error) {
    // Look for existing static pose (identity transform)
    poses, err := lidar.GetCurrentPoses(db, sensorID)
    if err != nil {
        return nil, err
    }
    
    if len(poses) > 0 {
        return poses[0], nil  // Use existing
    }
    
    // Create default identity transform pose for static sensor
    staticPose := &lidar.Pose{
        SensorID:       sensorID,
        FromFrame:      lidar.FrameID(fmt.Sprintf("sensor/%s", sensorID)),
        ToFrame:        lidar.FrameID("site/default"),
        T:              lidar.IdentityTransform4x4,
        ValidFromNanos: time.Now().UnixNano(),
        ValidToNanos:   nil,  // Current
        Method:         "static-identity",
        RootMeanSquareErrorMeters: 0.0,
    }
    
    return lidar.InsertPose(db, staticPose)
}
```

2. **Store PoseID with Clusters:**
```go
// In cmd/radar/radar.go when inserting clusters
cluster.PoseID = &currentStaticPose.PoseID  // Reference static pose

// Also store sensor-frame coordinates
cluster.SensorCentroidX = &sensorX
cluster.SensorCentroidY = &sensorY
cluster.SensorCentroidZ = &sensorZ

lidar.InsertCluster(db, cluster)
```

3. **Store PoseID with Track Observations:**
```go
// In cmd/radar/radar.go when inserting observations
obs := &lidar.TrackObservation{
    TrackID:     track.TrackID,
    TSUnixNanos: timestamp,
    PoseID:      &currentStaticPose.PoseID,  // NEW
    // ... existing fields
}
lidar.InsertTrackObservation(db, obs)
```

**For Static Sensors:**
- ✅ PoseID always points to same static identity transform
- ✅ Sensor coordinates = world coordinates (identity)
- ✅ Behavior identical to before (no functional change)
- ✅ **Future-compatible:** When motion is added, this field is already populated

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
