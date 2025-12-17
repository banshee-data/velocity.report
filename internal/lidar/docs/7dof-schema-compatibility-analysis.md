# LIDAR 7DOF Schema Compatibility Analysis

**Status:** Analysis Document (Architectural Planning)  
**Date:** December 17, 2025  
**Author:** Ictinus (Product Architecture Agent)  
**Version:** 1.0

---

## Executive Summary

This document analyzes the current LIDAR data structures (tracks, objects, poses, clusters) and identifies incompatibilities with proper 7DOF (7 Degrees of Freedom) pose representation required for autonomous vehicle (AV) integration and advanced multi-sensor fusion.

**Current State:** The system uses 4x4 homogeneous transformation matrices with identity transform defaults. While mathematically complete, this approach has limitations for:
- AV integration (which uses quaternion-based pose representations)
- Real-time pose interpolation and smoothing
- Visual odometry/SLAM integration
- IMU fusion and sensor ego-motion compensation

**7DOF Representation:** Position (x, y, z) + Orientation (quaternion: qw, qx, qy, qz)
- **Advantages:** Compact, avoids gimbal lock, efficient interpolation (SLERP)
- **Standard in:** ROS, autonomous vehicles, robotics middleware
- **Storage:** 7 float64 values vs 16 float64 for 4x4 matrix

**Key Findings:**
1. ✅ **Sensor poses table exists** but only stores 4x4 matrices (no quaternion representation)
2. ⚠️ **Track/cluster structures** have no pose/timestamp association (assume static sensor)
3. ⚠️ **No ego-motion support** (moving sensor platform)
4. ⚠️ **No pose interpolation** infrastructure (time-varying calibration)
5. ✅ **World frame abstraction** exists but not fully utilized

---

## Table of Contents

1. [Current Data Structures](#current-data-structures)
2. [7DOF Schema Requirements](#7dof-schema-requirements)
3. [Compatibility Gaps](#compatibility-gaps)
4. [Migration Path](#migration-path)
5. [Implementation Plan](#implementation-plan)
6. [PR Split Strategy](#pr-split-strategy)

---

## Current Data Structures

### 1. Sensor Poses (Database Schema)

**Location:** `internal/lidar/docs/schema.sql` lines 85-120

```sql
CREATE TABLE IF NOT EXISTS sensor_poses (
    pose_id INTEGER PRIMARY KEY,
    sensor_id TEXT NOT NULL,
    from_frame TEXT NOT NULL,        -- e.g., "sensor/hesai-01"
    to_frame TEXT NOT NULL,          -- e.g., "site/main-st-001"
    t_rowmajor_4x4 BLOB NOT NULL,    -- 16 float64 values (4x4 matrix)
    valid_from_ns INTEGER NOT NULL,
    valid_to_ns INTEGER,             -- NULL = current
    method TEXT,
    root_mean_square_error_meters REAL,
    FOREIGN KEY (sensor_id) REFERENCES sensors (sensor_id)
);
```

**Issues:**
- ✅ Supports time-varying poses (valid_from_ns, valid_to_ns)
- ⚠️ Only stores 4x4 matrices (no quaternion representation)
- ⚠️ No velocity/angular velocity (assumes static sensor)
- ⚠️ No covariance/uncertainty representation

### 2. Pose Struct (Go Code)

**Location:** `internal/lidar/arena.go` lines 32-44

```go
type Pose struct {
    PoseID                    int64
    SensorID                  string
    FromFrame                 FrameID
    ToFrame                   FrameID
    T                         [16]float64  // 4x4 row-major matrix
    ValidFromNanos            int64
    ValidToNanos              *int64
    Method                    string
    RootMeanSquareErrorMeters float32
}
```

**Issues:**
- ⚠️ No quaternion fields (qw, qx, qy, qz)
- ⚠️ No position fields (tx, ty, tz) - embedded in T matrix
- ⚠️ No velocity fields (vx, vy, vz, wx, wy, wz)
- ⚠️ No covariance matrix (6x6 for position + orientation uncertainty)

### 3. WorldCluster (Clustering Output)

**Location:** `internal/lidar/arena.go` lines 162-188

```go
type WorldCluster struct {
    ClusterID         int64
    SensorID          string
    WorldFrame        FrameID      // e.g., "site/main-st-001"
    TSUnixNanos       int64
    CentroidX         float32
    CentroidY         float32
    CentroidZ         float32
    // ... bounding box, features
    
    // ⚠️ NO POSE REFERENCE - assumes static sensor
    // ⚠️ NO SENSOR POSITION AT TIME OF MEASUREMENT
}
```

**Issues:**
- ⚠️ No pose_id reference (assumes sensor hasn't moved)
- ⚠️ Cannot reconstruct sensor position at time of measurement
- ⚠️ World coordinates baked in (can't re-transform if calibration updated)

### 4. TrackedObject (Kalman Tracking)

**Location:** `internal/lidar/tracking.go` lines 64-116

```go
type TrackedObject struct {
    TrackID  string
    SensorID string
    State    TrackState
    
    // Kalman state (world frame): [x, y, vx, vy]
    X  float32   // Position X
    Y  float32   // Position Y
    VX float32   // Velocity X
    VY float32   // Velocity Y
    P [16]float32 // Covariance (4x4)
    
    // ⚠️ NO ORIENTATION (heading only derived from velocity)
    // ⚠️ NO 3D VELOCITY (vz missing)
    // ⚠️ NO ANGULAR VELOCITY (wx, wy, wz)
    // ⚠️ NO POSE REFERENCE FOR EGO-MOTION COMPENSATION
}
```

**Issues:**
- ⚠️ 2D tracking only (X, Y, VX, VY) - Z position not tracked
- ⚠️ No orientation state (only velocity heading)
- ⚠️ No ego-motion compensation (sensor movement not accounted for)
- ⚠️ No pose_id association (can't compensate for sensor motion)

### 5. TrackObservation (Per-Frame Track Data)

**Location:** `internal/lidar/track_store.go` lines 26-45

```go
type TrackObservation struct {
    TrackID     string
    TSUnixNanos int64
    WorldFrame  string
    
    X, Y, Z     float32
    VelocityX, VelocityY float32
    SpeedMps    float32
    HeadingRad  float32
    
    // ⚠️ NO POSE_ID REFERENCE
    // ⚠️ NO SENSOR POSITION AT OBSERVATION TIME
}
```

**Issues:**
- ⚠️ No pose_id (cannot reconstruct sensor state during observation)
- ⚠️ No uncertainty/covariance stored
- ⚠️ Cannot re-transform if calibration updated

### 6. VelocityCoherentTrack (Advanced Tracking)

**Location:** `internal/lidar/velocity_coherent_tracking.go` lines 282-323

```go
type VelocityCoherentTrack struct {
    TrackID  string
    SensorID string
    
    // Position and velocity (world frame)
    X  float32
    Y  float32
    VX float32
    VY float32
    
    // Velocity confidence metrics
    VelocityConfidence  float32
    VelocityConsistency float32
    
    // ⚠️ SAME ISSUES AS TrackedObject
    // ⚠️ NO ORIENTATION STATE
    // ⚠️ NO Z-AXIS TRACKING
    // ⚠️ NO EGO-MOTION COMPENSATION
}
```

**Issues:**
- Same as `TrackedObject` - 2D only, no orientation, no ego-motion

---

## 7DOF Schema Requirements

### 1. Extended Pose Representation

**Proposed Schema Change:**

```sql
CREATE TABLE IF NOT EXISTS sensor_poses_7dof (
    pose_id INTEGER PRIMARY KEY,
    sensor_id TEXT NOT NULL,
    from_frame TEXT NOT NULL,
    to_frame TEXT NOT NULL,
    
    -- Position (meters)
    tx REAL NOT NULL,
    ty REAL NOT NULL,
    tz REAL NOT NULL,
    
    -- Orientation (unit quaternion: qw² + qx² + qy² + qz² = 1)
    qw REAL NOT NULL,  -- Scalar component
    qx REAL NOT NULL,  -- X component
    qy REAL NOT NULL,  -- Y component
    qz REAL NOT NULL,  -- Z component
    
    -- Optional: Velocity (for moving sensors)
    vx REAL,  -- Linear velocity X (m/s)
    vy REAL,  -- Linear velocity Y (m/s)
    vz REAL,  -- Linear velocity Z (m/s)
    wx REAL,  -- Angular velocity X (rad/s)
    wy REAL,  -- Angular velocity Y (rad/s)
    wz REAL,  -- Angular velocity Z (rad/s)
    
    -- Covariance (6x6 flattened: position + orientation uncertainty)
    covariance_6x6 BLOB,  -- 36 float64 values (optional)
    
    -- Legacy compatibility
    t_rowmajor_4x4 BLOB,  -- Can be computed from 7DOF on demand
    
    -- Temporal validity
    valid_from_ns INTEGER NOT NULL,
    valid_to_ns INTEGER,
    
    -- Calibration metadata
    method TEXT,
    root_mean_square_error_meters REAL,
    
    FOREIGN KEY (sensor_id) REFERENCES sensors (sensor_id)
);
```

### 2. Extended Pose Struct (Go)

**Proposed:**

```go
type Pose7DOF struct {
    // Identity
    PoseID   int64
    SensorID string
    FromFrame FrameID
    ToFrame   FrameID
    
    // Position (meters)
    TX, TY, TZ float64
    
    // Orientation (unit quaternion)
    QW, QX, QY, QZ float64
    
    // Optional: Velocity (for moving sensors / ego-motion)
    VX, VY, VZ float64  // Linear velocity (m/s)
    WX, WY, WZ float64  // Angular velocity (rad/s)
    
    // Uncertainty (6x6 covariance: 3 position + 3 orientation)
    Covariance [36]float64  // Row-major 6x6 matrix (optional)
    
    // Temporal validity
    ValidFromNanos int64
    ValidToNanos   *int64
    
    // Calibration metadata
    Method                    string
    RootMeanSquareErrorMeters float32
    
    // Legacy compatibility (computed on-demand)
    T [16]float64  // 4x4 matrix (can be computed from 7DOF)
}

// Methods needed:
func (p *Pose7DOF) ToMatrix4x4() [16]float64
func (p *Pose7DOF) FromMatrix4x4(T [16]float64)
func (p *Pose7DOF) Interpolate(other *Pose7DOF, t float64) *Pose7DOF  // SLERP
func (p *Pose7DOF) ApplyToPoint(x, y, z float64) (float64, float64, float64)
func (p *Pose7DOF) Inverse() *Pose7DOF
```

### 3. Pose-Aware Cluster Structure

**Proposed:**

```go
type WorldCluster struct {
    ClusterID         int64
    SensorID          string
    WorldFrame        FrameID
    TSUnixNanos       int64
    PoseID            *int64  // ⭐ NEW: Reference to sensor pose at measurement time
    
    // World coordinates (computed using PoseID transform)
    CentroidX         float32
    CentroidY         float32
    CentroidZ         float32
    
    // Sensor coordinates (raw, untransformed - for recomputation)
    SensorCentroidX   *float32  // ⭐ NEW: Optional sensor-frame coords
    SensorCentroidY   *float32
    SensorCentroidZ   *float32
    
    // ... existing fields
}
```

**Benefits:**
- Can re-transform if calibration updated (using SensorCentroid + new PoseID)
- Supports moving sensors (each cluster has associated pose)
- Enables ego-motion compensation in tracking

### 4. 3D Tracking with Orientation

**Proposed:**

```go
type TrackedObject struct {
    TrackID  string
    SensorID string
    State    TrackState
    
    // Kalman state (world frame): [x, y, z, vx, vy, vz, qw, qx, qy, qz, wx, wy, wz]
    // Position (meters)
    X, Y, Z float32
    
    // Linear velocity (m/s)
    VX, VY, VZ float32
    
    // Orientation (unit quaternion) ⭐ NEW
    QW, QX, QY, QZ float32
    
    // Angular velocity (rad/s) ⭐ NEW
    WX, WY, WZ float32
    
    // Covariance (13x13 for full state) ⭐ EXPANDED
    P [169]float32  // 13x13 covariance matrix
    
    // Sensor ego-motion compensation ⭐ NEW
    LastSensorPoseID *int64  // Pose used for last prediction
    
    // ... existing fields
}
```

**Benefits:**
- Full 3D tracking (not just ground plane)
- Orientation tracking (useful for vehicle heading, object rotation)
- Ego-motion compensation (predict() accounts for sensor motion)

### 5. Pose-Aware Observations

**Proposed:**

```go
type TrackObservation struct {
    TrackID     string
    TSUnixNanos int64
    WorldFrame  string
    PoseID      *int64  // ⭐ NEW: Sensor pose at observation time
    
    // Position (world frame)
    X, Y, Z     float32
    
    // Velocity (world frame)
    VelocityX, VelocityY, VelocityZ float32
    
    // Orientation (world frame) ⭐ NEW
    QW, QX, QY, QZ float32
    
    // Angular velocity (world frame) ⭐ NEW
    WX, WY, WZ float32
    
    // Measurement uncertainty ⭐ NEW
    PositionCovarianceXYZ [9]float32   // 3x3 covariance
    OrientationCovarianceQuat [16]float32  // 4x4 covariance
    
    // ... existing shape/feature fields
}
```

---

## Compatibility Gaps

### Gap 1: Static Sensor Assumption

**Current:** All code assumes sensor is static (identity transform or fixed calibration)

**Impact:** Cannot support:
- Mobile robots / autonomous vehicles with LIDAR
- Sensor re-calibration without data migration
- Multi-session SLAM where sensor moves between sessions

**Required Changes:**
1. Add pose_id references to clusters, observations
2. Store raw sensor-frame data alongside world-frame data
3. Implement pose interpolation for time-varying calibration

### Gap 2: 2D Tracking Only

**Current:** Tracker uses 4-state Kalman filter (X, Y, VX, VY)

**Impact:** Cannot track:
- Height changes (jumping, falling objects)
- Vertical velocity (thrown objects, birds)
- 3D orientation (vehicle pitch/roll, object rotation)

**Required Changes:**
1. Extend Kalman state to 13D: [x, y, z, vx, vy, vz, qw, qx, qy, qz, wx, wy, wz]
2. Update prediction/update matrices for 3D motion
3. Add quaternion normalization and covariance manifold handling

### Gap 3: No Ego-Motion Compensation

**Current:** Tracker assumes sensor is stationary

**Impact:** If sensor moves (vehicle-mounted LIDAR):
- Track predictions incorrect (don't account for sensor motion)
- Velocity estimates include sensor velocity (not object velocity)
- Association fails (objects appear to jump)

**Required Changes:**
1. Store sensor pose at each prediction/update step
2. Compensate predictions: `predicted_world = pose_current * (pose_prev^-1 * track_state_prev)`
3. De-bias velocities: `object_velocity = measured_velocity - sensor_velocity`

### Gap 4: No Quaternion Infrastructure

**Current:** All orientation is derived from velocity heading (2D angle)

**Impact:**
- Cannot represent 3D orientation (pitch, roll)
- Cannot interpolate orientations smoothly (SLERP)
- Incompatible with ROS/AV ecosystems (which use quaternions)

**Required Changes:**
1. Add quaternion utility functions (multiply, inverse, SLERP, to/from matrix)
2. Update all pose-related code to use quaternions
3. Add conversion functions for legacy 4x4 matrix support

### Gap 5: Baked-In World Coordinates

**Current:** Clusters and observations only store world coordinates

**Impact:**
- Cannot re-transform if calibration updated
- Cannot reprocess with different pose
- Data tied to specific calibration (not reusable)

**Required Changes:**
1. Store both sensor-frame and world-frame coordinates
2. Add pose_id foreign keys to associate measurements with calibration
3. Add re-transformation API for calibration updates

---

## Migration Path

### Phase 1: Schema Extensions (Database)

**Goal:** Add 7DOF fields to existing tables without breaking current code

**Changes:**
1. Add `sensor_poses_7dof` table (new schema)
2. Add migration script to convert existing 4x4 matrices to 7DOF
3. Add optional 7DOF fields to Go structs (backward compatible)

**Deliverables:**
- Migration script: `000011_add_7dof_poses.up.sql`
- Conversion functions: `Matrix4x4ToQuaternion()`, `QuaternionToMatrix4x4()`
- Tests for conversion accuracy

### Phase 2: Quaternion Infrastructure (Go Code)

**Goal:** Add quaternion support without changing existing APIs

**Changes:**
1. Create `internal/lidar/quaternion.go` with utilities
2. Add `Pose7DOF` struct alongside existing `Pose`
3. Add conversion methods between `Pose` and `Pose7DOF`
4. Unit tests for quaternion operations

**Deliverables:**
- `quaternion.go`: multiply, inverse, SLERP, normalize, to/from matrix
- `Pose7DOF` struct and methods
- Comprehensive unit tests (rotation composition, interpolation)

### Phase 3: Pose-Aware Clusters (Data Association)

**Goal:** Track which pose was used for each measurement

**Changes:**
1. Add `pose_id` field to `WorldCluster` struct
2. Add `sensor_centroid_*` fields for raw sensor-frame data
3. Update `InsertCluster()` to store both sensor and world coordinates
4. Add `RecomputeWorldCoordinates(clusterID, newPoseID)` API

**Deliverables:**
- Updated `WorldCluster` struct with pose_id
- Database migration: `000012_add_cluster_pose_refs.up.sql`
- Re-transformation API for calibration updates

### Phase 4: 3D Tracking (Kalman Extensions)

**Goal:** Extend tracker to 3D with orientation

**Changes:**
1. Create `TrackedObject3D` struct with 13-state Kalman filter
2. Implement 3D prediction/update with quaternion state
3. Add quaternion normalization and manifold covariance
4. Keep existing 2D tracker for backward compatibility

**Deliverables:**
- `tracking_3d.go`: 13-state Kalman with quaternion state
- Prediction/update matrices for 3D constant-velocity model
- Unit tests with synthetic 3D trajectories

### Phase 5: Ego-Motion Compensation (Moving Sensors)

**Goal:** Support vehicle-mounted / moving LIDAR

**Changes:**
1. Add `last_pose_id` to `TrackedObject3D`
2. Implement pose-compensated prediction step
3. Add velocity de-biasing (remove sensor velocity)
4. Update association to use pose-compensated distances

**Deliverables:**
- Ego-motion compensation in `Tracker3D.Predict()`
- Velocity de-biasing utility functions
- Integration tests with moving sensor scenarios

### Phase 6: API Updates (REST Endpoints)

**Goal:** Expose 7DOF data through HTTP APIs

**Changes:**
1. Add `/api/lidar/poses` endpoint (list sensor poses with 7DOF)
2. Add `/api/lidar/poses/{pose_id}` (get specific pose)
3. Add `/api/lidar/tracks` with 3D orientation fields (optional)
4. Update JSON responses to include pose_id references

**Deliverables:**
- REST API handlers for pose queries
- JSON schemas with 7DOF fields
- API documentation updates

---

## Implementation Plan

### Option A: Monolithic PR (Single Large Change)

**Pros:**
- All changes atomic (no intermediate broken states)
- Easy to reason about full system behavior
- Tests validate end-to-end compatibility

**Cons:**
- **Massive review burden** (thousands of lines changed)
- High risk of merge conflicts during development
- Long feedback cycle (weeks before merge)
- Harder to bisect bugs if issues found later

**Recommended For:**
- Small teams with dedicated reviewer
- Low-urgency features (can wait for full review)
- Systems with comprehensive integration tests

### Option B: Incremental PRs (Phased Approach)

**Pros:**
- **Smaller reviews** (hundreds of lines per PR)
- Each PR is independently testable and valuable
- Lower risk (can roll back individual phases)
- Parallel work possible (different phases by different devs)

**Cons:**
- Requires careful API design (backward compatibility)
- Need to maintain intermediate states (more code branches)
- Integration testing across PRs is harder

**Recommended For:**
- Large teams / open-source projects
- Production systems requiring stability
- Features that deliver value incrementally

---

## PR Split Strategy

### Recommendation: **Incremental Approach (Option B)**

**Rationale:**
1. Current LIDAR code is production-deployed (Raspberry Pi systems)
2. Changes affect core data structures (high risk)
3. Team may have limited review bandwidth
4. 7DOF support can deliver value in phases:
   - Phase 1-2: Better calibration representation (immediate value)
   - Phase 3: Calibration updates without re-collection (valuable)
   - Phase 4-5: 3D tracking and ego-motion (future AV integration)

### Proposed PR Sequence

#### PR #1: Database Schema Extensions
**Scope:** Add 7DOF tables without breaking existing code

**Files Changed:**
- `internal/db/migrations/000011_add_7dof_poses.up.sql` (new)
- `internal/db/migrations/000011_add_7dof_poses.down.sql` (new)
- `internal/db/schema.sql` (add sensor_poses_7dof table)
- Tests: Schema migration tests

**Exit Criteria:**
- Migration runs successfully on existing databases
- Existing tests pass (no functionality change)
- Documentation updated with new schema

**Estimated Size:** ~200 lines

---

#### PR #2: Quaternion Utilities
**Scope:** Add quaternion math without changing existing APIs

**Files Changed:**
- `internal/lidar/quaternion.go` (new)
- `internal/lidar/quaternion_test.go` (new)
- `internal/lidar/arena.go` (add Pose7DOF struct)
- `internal/lidar/transform.go` (add 7DOF conversion functions)

**Exit Criteria:**
- All quaternion operations have unit tests
- Conversion to/from 4x4 matrices validated
- SLERP interpolation tested
- Existing tests pass (no API breakage)

**Estimated Size:** ~400 lines

---

#### PR #3: Pose-Aware Clusters
**Scope:** Add pose_id tracking to clusters and observations

**Files Changed:**
- `internal/lidar/arena.go` (add pose_id to WorldCluster)
- `internal/lidar/track_store.go` (update InsertCluster with pose_id)
- `internal/lidar/track_store_test.go` (update tests)
- `internal/db/migrations/000012_add_cluster_pose_refs.up.sql` (new)
- `internal/lidar/docs/schema.sql` (update lidar_clusters table)

**Exit Criteria:**
- Clusters can be stored with pose_id reference
- NULL pose_id supported (backward compatibility)
- Re-transformation API functional
- Existing tests pass with NULL pose_id

**Estimated Size:** ~300 lines

---

#### PR #4: 3D Tracking Foundation
**Scope:** Add 3D tracker without replacing 2D tracker

**Files Changed:**
- `internal/lidar/tracking_3d.go` (new)
- `internal/lidar/tracking_3d_test.go` (new)
- `internal/lidar/arena.go` (add TrackedObject3D struct)
- Documentation updates

**Exit Criteria:**
- 13-state Kalman filter functional
- Quaternion state correctly normalized
- 3D synthetic trajectories tracked accurately
- Existing 2D tracker unchanged

**Estimated Size:** ~600 lines

---

#### PR #5: Ego-Motion Compensation
**Scope:** Add moving sensor support to 3D tracker

**Files Changed:**
- `internal/lidar/tracking_3d.go` (update Predict with ego-motion)
- `internal/lidar/tracking_3d_test.go` (add moving sensor tests)
- `internal/lidar/velocity_estimation.go` (add velocity de-biasing)
- Documentation updates

**Exit Criteria:**
- Tracks stable with moving sensor (synthetic tests)
- Velocity de-biasing validated
- Integration test with PCAP + pose sequence
- Existing trackers unchanged

**Estimated Size:** ~400 lines

---

#### PR #6: REST API Extensions
**Scope:** Expose 7DOF data through HTTP APIs

**Files Changed:**
- `internal/lidar/monitor/pose_api.go` (new)
- `internal/lidar/monitor/pose_api_test.go` (new)
- `internal/lidar/monitor/track_api.go` (add 3D track endpoints)
- `cmd/radar/radar.go` (register pose API routes)
- API documentation

**Exit Criteria:**
- `/api/lidar/poses` returns 7DOF poses
- `/api/lidar/tracks/3d` returns 3D tracks with orientation
- Backward compatibility with existing 2D API
- API documentation complete

**Estimated Size:** ~500 lines

---

#### PR #7: Production Integration
**Scope:** Enable 7DOF tracking in production pipeline

**Files Changed:**
- `cmd/radar/radar.go` (add --use-3d-tracking flag)
- `cmd/tools/pcap-analyze/main.go` (add 3D tracking support)
- `internal/lidar/dual_pipeline.go` (add 3D tracker option)
- Configuration file examples
- Deployment documentation

**Exit Criteria:**
- 3D tracking opt-in via config flag
- PCAP analysis tool supports 3D output
- Production deployment guide updated
- Rollback plan documented

**Estimated Size:** ~300 lines

---

### Total Estimated Effort

**Total Lines Changed:** ~2,700 lines across 7 PRs

**Timeline Estimate:**
- PR #1-2: Foundational (parallel, 1-2 weeks)
- PR #3: Data layer (1 week)
- PR #4-5: Tracking (sequential, 2-3 weeks)
- PR #6-7: Integration (parallel, 1-2 weeks)

**Total Duration:** 5-8 weeks with sequential reviews, 3-5 weeks with parallel work

---

## Conclusion

The current LIDAR implementation has a solid foundation (4x4 matrix poses, world frame abstraction) but lacks the 7DOF representation and ego-motion support needed for AV integration.

**Recommended Path:**
1. **Incremental PRs** (Option B) for lower risk and better reviewability
2. **Backward compatibility** maintained throughout (existing 2D tracking unchanged)
3. **Opt-in 3D tracking** (feature flag for production)
4. **Complete testing** at each phase (unit + integration tests)

**Key Success Factors:**
- Comprehensive unit tests for quaternion math
- Integration tests with synthetic moving sensor data
- Clear migration documentation for users
- Rollback plan for each phase

**Future Work (Beyond 7DOF):**
- Visual odometry integration (camera + LIDAR fusion)
- IMU fusion for ego-motion estimation
- Multi-sensor pose graph optimization
- Loop closure detection for long-term mapping

---

## References

- Current schema: `internal/lidar/docs/schema.sql`
- Pose struct: `internal/lidar/arena.go` lines 32-44
- Tracker: `internal/lidar/tracking.go`
- Clustering: `internal/lidar/clustering.go`
- VC Tracking: `internal/lidar/velocity_coherent_tracking.go`

---

## Appendices

### A. Quaternion Primer

**Unit Quaternion:** q = (qw, qx, qy, qz) where qw² + qx² + qy² + qz² = 1

**Rotation by q:**
```
q * p * q^-1
where p = (0, x, y, z)  -- point as pure quaternion
```

**Composition:**
```
q_total = q2 * q1  -- Apply q1 first, then q2
```

**Interpolation (SLERP):**
```
q(t) = sin((1-t)θ)/sin(θ) * q0 + sin(tθ)/sin(θ) * q1
where cos(θ) = q0 · q1
```

### B. 4x4 Matrix to Quaternion Conversion

**Given 4x4 matrix T:**
```
T = [R | t]
    [0 | 1]
where R is 3x3 rotation, t is 3x1 translation
```

**Extract quaternion from R:**
```
trace = R[0][0] + R[1][1] + R[2][2]

if trace > 0:
    s = 0.5 / sqrt(trace + 1)
    qw = 0.25 / s
    qx = (R[2][1] - R[1][2]) * s
    qy = (R[0][2] - R[2][0]) * s
    qz = (R[1][0] - R[0][1]) * s
else:
    # Handle near-singular cases (see Shepperd's method)
```

**Extract translation:** tx = T[0][3], ty = T[1][3], tz = T[2][3]

### C. Ego-Motion Compensation Equations

**Prediction with moving sensor:**

```
// State at time t-1: x_prev = [p_prev, v_prev, q_prev, w_prev]
// Sensor pose at t-1: T_prev
// Sensor pose at t: T_curr

// Step 1: Transform track to sensor frame at t-1
p_sensor_prev = T_prev^-1 * p_prev

// Step 2: Predict in sensor frame (object motion only)
p_sensor_pred = p_sensor_prev + v_prev * dt

// Step 3: Transform back to world using current pose
p_world_pred = T_curr * p_sensor_pred

// This accounts for sensor motion between t-1 and t
```

**Velocity de-biasing:**

```
// Measured velocity includes sensor velocity
v_measured = v_object + v_sensor

// De-bias to get object velocity in world frame
v_object = v_measured - v_sensor

where:
v_sensor = dT/dt * p_object
         ≈ (T_curr^-1 * T_prev - I) / dt * p_object
```
