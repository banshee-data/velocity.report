# Motion Capture Architecture - Future Specification

**Status:** Future Work (Not in Current Release)  
**Date:** December 17, 2025  
**Scope:** Moving LIDAR sensors (vehicle, bike, robot, drone mounted)  
**Purpose:** Long-term architecture specification with current implementation context

---

## Executive Summary

This document specifies the complete architecture for **motion capture scenarios** where the LIDAR sensor itself is moving (vehicle-mounted, bike-mounted, robot, drone). This is **future work** and is not included in the current release, which focuses only on static roadside sensors.

**Current Release:** Static pose alignment (see `static-pose-alignment-plan.md`)  
**Future Release:** Full motion capture support (this document)

**Key Capabilities Enabled:**
- Vehicle-mounted LIDAR (autonomous vehicles, mapping vehicles)
- Bike-mounted LIDAR (mobile sensing)
- Robot-mounted LIDAR (mobile robotics, warehouse automation)
- Drone-mounted LIDAR (aerial mapping)

---

## Table of Contents

1. [Current State: Static Tracking](#current-state-static-tracking)
2. [Future State: Motion Tracking](#future-state-motion-tracking)
3. [7DOF Pose Representation](#7dof-pose-representation)
4. [Ego-Motion Compensation](#ego-motion-compensation)
5. [3D Tracking with Orientation](#3d-tracking-with-orientation)
6. [Data Structure Specifications](#data-structure-specifications)
7. [Implementation Phases](#implementation-phases)
8. [Migration Path](#migration-path)

---

## Current State: Static Tracking

### What We Have Today

**Static Roadside LIDAR:**
- Sensor mounted in fixed position (utility pole, building, traffic light)
- Background is stationary (buildings, trees, road surface)
- Foreground objects move (vehicles, pedestrians, animals)
- Sensor coordinate frame = world coordinate frame (identity transform)

**2D Tracking:**
- Kalman filter tracks ground plane motion: [x, y, vx, vy] (4 states)
- No height tracking (assumes objects on flat ground)
- No orientation tracking (only velocity heading)
- Object classes: car, pedestrian, bird, other

**Data Flow (Static):**
```
UDP Packets → Parse → Frame → Background Grid → Foreground Extraction
    ↓
Polar → World Transform (identity) → Clustering (DBSCAN) → Tracking (Kalman 2D)
    ↓
Classification → Database → API
```

**Current Assumptions:**
1. Sensor is stationary (fixed to ground/structure)
2. Background is stationary (static environment)
3. All motion is from objects being tracked
4. Sensor frame = world frame (no transformation needed)

**Current Limitations:**
- ❌ Cannot handle moving sensor (velocity bias in measurements)
- ❌ Cannot track 3D objects (birds, drones, thrown objects)
- ❌ Cannot estimate object orientation (only heading from velocity)
- ❌ Cannot update calibration without re-collection

---

## Future State: Motion Tracking

### What We Need for Motion Capture

**Moving Sensor Scenarios:**

1. **Vehicle-Mounted LIDAR**
   - Sensor moves with vehicle (position + orientation changing)
   - Need to compensate for vehicle motion in tracking
   - Need to separate object velocity from sensor velocity
   - Use case: Autonomous vehicles, mapping vehicles, street scanning

2. **Bike-Mounted LIDAR**
   - Similar to vehicle but higher vibration, more agile motion
   - Need robust pose estimation (bumpy roads, curbs)
   - Use case: Mobile traffic monitoring, bike lane analysis

3. **Robot-Mounted LIDAR**
   - Indoor or outdoor mobile robots
   - Need 6DOF pose tracking (x, y, z, roll, pitch, yaw)
   - Use case: Warehouse automation, delivery robots

4. **Drone-Mounted LIDAR**
   - Aerial platform (all 6DOF actively changing)
   - Need IMU integration for high-frequency pose estimates
   - Use case: Aerial mapping, power line inspection

### Key Challenges

**1. Ego-Motion Compensation**
- Sensor velocity must be subtracted from measured velocities
- Sensor rotation affects apparent object positions
- Prediction step must account for sensor motion

**2. Time-Varying Poses**
- Pose changes every frame (not static)
- Need interpolation between pose measurements
- Need high-frequency pose estimates (100 Hz+)

**3. 3D Tracking Required**
- Ground plane assumption breaks (sensor tilts, pitches)
- Need full 6DOF state estimation
- Need orientation tracking for objects

**4. Coordinate Frame Complexity**
- Sensor frame moves relative to world frame
- Need consistent world frame definition
- Need pose graph optimization for loop closure

---

## 7DOF Pose Representation

### Why 7DOF?

**7DOF = 7 Degrees of Freedom:**
- **Position:** (x, y, z) - 3 DOF
- **Orientation:** (qw, qx, qy, qz) - 4 DOF (unit quaternion)

**Advantages over 4x4 Matrix:**
- ✅ More compact (7 values vs 16)
- ✅ Avoids gimbal lock (quaternions don't have singularities)
- ✅ Efficient interpolation (SLERP for smooth motion)
- ✅ Standard in robotics (ROS, autonomous vehicle systems)
- ✅ Better for optimization (fewer parameters)

**Current Implementation (4x4 Matrix):**
```go
type Pose struct {
    T [16]float64  // 4x4 homogeneous transformation matrix
    // T = [R | t]  where R is 3x3 rotation, t is 3x1 translation
    //     [0 | 1]
}
```

**Future Implementation (7DOF):**
```go
type Pose7DOF struct {
    // Position (3 DOF)
    TX, TY, TZ float64  // Translation in meters
    
    // Orientation (4 DOF - unit quaternion)
    QW, QX, QY, QZ float64  // qw² + qx² + qy² + qz² = 1
    
    // Optional: Velocity (for moving sensors)
    VX, VY, VZ float64  // Linear velocity (m/s)
    WX, WY, WZ float64  // Angular velocity (rad/s)
    
    // Uncertainty (6x6 covariance)
    Covariance [36]float64  // Position (3x3) + Orientation (3x3)
    
    // Temporal validity
    ValidFromNanos int64
    ValidToNanos   *int64
    
    // Legacy compatibility (computed on-demand)
    T [16]float64  // Can be computed from 7DOF when needed
}
```

### Database Schema (Future)

```sql
CREATE TABLE sensor_poses_7dof (
    pose_id INTEGER PRIMARY KEY,
    sensor_id TEXT NOT NULL,
    from_frame TEXT NOT NULL,  -- e.g., "sensor/hesai-01"
    to_frame TEXT NOT NULL,    -- e.g., "site/main-st-001"
    
    -- Position (meters)
    tx REAL NOT NULL,
    ty REAL NOT NULL,
    tz REAL NOT NULL,
    
    -- Orientation (unit quaternion)
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
    
    -- Uncertainty (6x6 covariance, flattened)
    covariance_6x6 BLOB,  -- 36 float64 values
    
    -- Temporal validity
    valid_from_ns INTEGER NOT NULL,
    valid_to_ns INTEGER,  -- NULL = current
    
    -- Legacy compatibility (computed from 7DOF)
    t_rowmajor_4x4 BLOB,  -- 16 float64 values
    
    -- Calibration metadata
    method TEXT,
    root_mean_square_error_meters REAL,
    
    FOREIGN KEY (sensor_id) REFERENCES sensors (sensor_id)
);
```

### Quaternion Operations

**Required Utilities:**

```go
package quaternion

// Core operations
func Multiply(q1, q2 Quaternion) Quaternion
func Inverse(q Quaternion) Quaternion
func Normalize(q Quaternion) Quaternion
func Conjugate(q Quaternion) Quaternion

// Interpolation
func SLERP(q1, q2 Quaternion, t float64) Quaternion  // Spherical Linear Interpolation

// Conversions
func ToMatrix(q Quaternion) [9]float64  // 3x3 rotation matrix
func FromMatrix(R [9]float64) Quaternion
func ToEuler(q Quaternion) (roll, pitch, yaw float64)
func FromEuler(roll, pitch, yaw float64) Quaternion

// Application
func RotatePoint(q Quaternion, p [3]float64) [3]float64
```

**SLERP (Spherical Linear Interpolation):**
```
q(t) = sin((1-t)θ)/sin(θ) * q0 + sin(tθ)/sin(θ) * q1
where cos(θ) = q0 · q1  (dot product)
```

This provides smooth rotation interpolation between poses.

---

## Ego-Motion Compensation

### Problem Statement

When the sensor moves, measured velocities include **both object motion and sensor motion**:

```
v_measured = v_object + v_sensor
```

**Example (Vehicle-Mounted LIDAR):**
- Vehicle traveling at 20 m/s forward
- Stationary parked car in sensor view
- Measured velocity: -20 m/s (car appears to move backward)
- **Actual object velocity: 0 m/s** (car is stationary)

**Without ego-motion compensation:**
- ❌ All stationary objects appear to move backward at vehicle speed
- ❌ Tracking fails (objects don't match between frames)
- ❌ Classification fails (velocity-based features incorrect)

### Solution: Pose-Compensated Tracking

**Kalman Filter with Ego-Motion:**

1. **Prediction Step (with sensor motion):**
```go
func (t *Tracker) PredictWithEgoMotion(dt float32, prevPose, currPose *Pose7DOF) {
    for _, track := range t.Tracks {
        // Step 1: Transform track from world to sensor frame at t-1
        p_sensor_prev := prevPose.WorldToSensor(track.Position())
        
        // Step 2: Predict object motion in sensor frame (object-only motion)
        p_sensor_pred := p_sensor_prev + track.Velocity() * dt
        
        // Step 3: Transform prediction back to world using current pose
        p_world_pred := currPose.SensorToWorld(p_sensor_pred)
        
        // Update track position
        track.SetPosition(p_world_pred)
    }
}
```

2. **Update Step (with velocity de-biasing):**
```go
func (t *Tracker) UpdateWithEgoMotion(measurement Measurement, currPose *Pose7DOF) {
    // Measured velocity includes sensor velocity
    v_measured := measurement.Velocity()
    
    // Compute sensor velocity at measurement point
    v_sensor := currPose.VelocityAtPoint(measurement.Position())
    
    // De-bias: Remove sensor velocity to get object velocity
    v_object := v_measured - v_sensor
    
    // Use de-biased velocity for Kalman update
    track.Update(measurement.Position(), v_object)
}
```

**Sensor Velocity at Point:**
```go
func (p *Pose7DOF) VelocityAtPoint(point [3]float64) [3]float64 {
    // v_point = v_sensor + ω × r
    // where ω is angular velocity, r is radius vector from sensor to point
    
    r := point - p.Position()
    v_linear := p.LinearVelocity()
    v_angular := cross(p.AngularVelocity(), r)
    
    return v_linear + v_angular
}
```

### Requirements for Ego-Motion Compensation

**Data Requirements:**
1. ✅ Pose at each measurement time (pose_id stored with clusters)
2. ✅ Pose velocity (vx, vy, vz, wx, wy, wz) available
3. ✅ High-frequency pose estimates (>=10 Hz, preferably 100 Hz)
4. ✅ Pose interpolation between measurements

**Algorithm Requirements:**
1. ✅ Pose-compensated Kalman prediction
2. ✅ Velocity de-biasing in update step
3. ✅ Pose-aware data association (account for sensor motion)
4. ✅ Reference frame consistency checks

---

## 3D Tracking with Orientation

### Current: 2D Ground Plane Tracking

**4-State Kalman Filter:**
```
State: x = [x, y, vx, vy]ᵀ
Covariance: P (4×4 matrix)
```

**Prediction:**
```
x' = F·x + w
where F = [1 0 dt  0]
          [0 1  0 dt]
          [0 0  1  0]
          [0 0  0  1]
```

**Limitations:**
- ❌ No height tracking (assumes z = 0)
- ❌ No orientation (only heading from velocity)
- ❌ No vertical velocity (cannot track jumping, falling)
- ❌ No rotation (cannot track spinning objects)

### Future: 3D Tracking with Orientation

**13-State Kalman Filter:**
```
State: x = [x, y, z, vx, vy, vz, qw, qx, qy, qz, wx, wy, wz]ᵀ
Covariance: P (13×13 matrix)
```

**State Components:**
- **Position (3):** x, y, z (meters)
- **Linear velocity (3):** vx, vy, vz (m/s)
- **Orientation (4):** qw, qx, qy, qz (unit quaternion)
- **Angular velocity (3):** wx, wy, wz (rad/s)

**Prediction (3D with Orientation):**
```go
func (track *TrackedObject3D) Predict(dt float32) {
    // Linear position prediction
    track.X += track.VX * dt
    track.Y += track.VY * dt
    track.Z += track.VZ * dt
    
    // Angular position prediction (quaternion integration)
    // q' = q + 0.5 * dt * Ω(ω) * q
    // where Ω(ω) is the quaternion rate matrix
    dq := 0.5 * dt * QuaternionRate(track.W()) * track.Q()
    track.Q() = Normalize(track.Q() + dq)
    
    // Velocity predictions (constant velocity model)
    // VX, VY, VZ, WX, WY, WZ remain constant
    
    // Covariance prediction (13×13)
    track.P = F·track.P·Fᵀ + Q
}
```

**Update (with 3D Measurements):**
```go
func (track *TrackedObject3D) Update(z Measurement) {
    // Measurement: position + velocity (6D)
    // Could be extended to include orientation measurements
    
    // Kalman gain
    K = P·Hᵀ·(H·P·Hᵀ + R)⁻¹
    
    // State update
    x' = x + K·(z - H·x)
    
    // Covariance update
    P' = (I - K·H)·P
    
    // Quaternion normalization (maintain unit constraint)
    track.Q() = Normalize(track.Q())
}
```

### Object Orientation Estimation

**Use Cases:**
- Vehicle heading (different from velocity direction when turning)
- Pedestrian facing direction (different from walking direction)
- Object rotation detection (spinning, tumbling)
- Better bounding box alignment (oriented bounding boxes)

**Estimation Methods:**

1. **From Velocity (Simple):**
```go
// Current method - heading from velocity
heading := atan2(vy, vx)
quat := QuaternionFromYaw(heading)  // Rotation around Z-axis only
```

2. **From Point Cloud Principal Axes (Better):**
```go
// Compute principal axes via PCA on cluster points
pca := ComputePCA(clusterPoints)
orientation := QuaternionFromPCA(pca)
```

3. **From Tracking History (Best):**
```go
// Use Kalman filter to estimate orientation over time
// Integrates velocity heading, PCA, and temporal consistency
track.UpdateOrientation(measurement)
```

---

## Data Structure Specifications

### Pose-Aware Cluster (Motion)

```go
type WorldCluster struct {
    ClusterID   int64
    SensorID    string
    WorldFrame  FrameID
    TSUnixNanos int64
    
    // World coordinates (computed from sensor + pose)
    CentroidX, CentroidY, CentroidZ float32
    
    // ⭐ REQUIRED for motion: Reference to sensor pose
    PoseID      int64  // NOT NULL for moving sensors
    
    // ⭐ REQUIRED for motion: Sensor-frame coordinates
    SensorCentroidX, SensorCentroidY, SensorCentroidZ float32
    
    // Bounding box and features (unchanged)
    BoundingBoxLength, BoundingBoxWidth, BoundingBoxHeight float32
    HeightP95, IntensityMean float32
    PointsCount int
}
```

### 3D Tracked Object (Motion)

```go
type TrackedObject3D struct {
    TrackID  string
    SensorID string
    State    TrackState
    
    // Position (world frame, meters)
    X, Y, Z float32
    
    // Linear velocity (world frame, m/s)
    VX, VY, VZ float32
    
    // ⭐ NEW: Orientation (unit quaternion)
    QW, QX, QY, QZ float32
    
    // ⭐ NEW: Angular velocity (rad/s)
    WX, WY, WZ float32
    
    // ⭐ EXPANDED: Covariance (13×13 for full state)
    P [169]float32  // Row-major 13×13 matrix
    
    // ⭐ NEW: Last sensor pose (for ego-motion compensation)
    LastPoseID int64
    
    // Lifecycle and features (unchanged)
    Hits, Misses int
    FirstUnixNanos, LastUnixNanos int64
    ObservationCount int
    // ... other features
}
```

### Track Observation (Motion)

```go
type TrackObservation3D struct {
    TrackID     string
    TSUnixNanos int64
    WorldFrame  string
    
    // ⭐ REQUIRED: Sensor pose at observation time
    PoseID      int64  // NOT NULL for moving sensors
    
    // Position (world frame)
    X, Y, Z float32
    
    // Velocity (world frame, de-biased)
    VelocityX, VelocityY, VelocityZ float32
    
    // ⭐ NEW: Orientation (world frame)
    QW, QX, QY, QZ float32
    
    // ⭐ NEW: Angular velocity (world frame)
    WX, WY, WZ float32
    
    // ⭐ NEW: Measurement uncertainty
    PositionCovariance [9]float32      // 3×3 position covariance
    OrientationCovariance [16]float32  // 4×4 quaternion covariance
    
    // Shape features (unchanged)
    BoundingBoxLength, BoundingBoxWidth, BoundingBoxHeight float32
    HeightP95, IntensityMean float32
}
```

---

## Implementation Phases

### Phase 1: 7DOF Pose Infrastructure (Foundation)

**Goal:** Add quaternion support without changing tracking

**Deliverables:**
- Quaternion math library (`internal/lidar/quaternion.go`)
- Pose7DOF struct with conversions
- Database schema for sensor_poses_7dof
- Unit tests for quaternion operations

**Estimated Effort:** 2-3 weeks

---

### Phase 2: 3D Tracking (No Orientation Yet)

**Goal:** Extend Kalman to 3D position/velocity (9-state)

**Deliverables:**
- TrackedObject3D with [x, y, z, vx, vy, vz] (6-state position/velocity)
- 3D Kalman prediction/update
- Coexist with 2D tracker (parallel implementation)

**Estimated Effort:** 3-4 weeks

---

### Phase 3: Orientation Tracking

**Goal:** Add quaternion state to 3D tracker (13-state)

**Deliverables:**
- Full 13-state tracker [x, y, z, vx, vy, vz, qw, qx, qy, qz, wx, wy, wz]
- Quaternion prediction (integration)
- Orientation estimation from point clouds
- Quaternion normalization in Kalman filter

**Estimated Effort:** 4-5 weeks

---

### Phase 4: Ego-Motion Compensation

**Goal:** Support moving sensors

**Deliverables:**
- Pose-compensated Kalman prediction
- Velocity de-biasing
- Pose interpolation utilities
- Integration tests with simulated moving sensor

**Estimated Effort:** 3-4 weeks

---

### Phase 5: Production Integration

**Goal:** Enable motion capture in production

**Deliverables:**
- Feature flags (--enable-3d-tracking, --enable-ego-motion)
- REST API updates (/api/lidar/poses, /api/lidar/tracks/3d)
- PCAP tool support for 3D output
- Deployment documentation

**Estimated Effort:** 2-3 weeks

---

### Total Timeline: 14-19 weeks (3.5-5 months)

**Dependencies:**
- Phase 1 must complete before Phase 2
- Phase 2 must complete before Phase 3
- Phase 3 must complete before Phase 4
- Phase 4 must complete before Phase 5

**Parallel Work Possible:**
- Phase 1 and documentation can be parallel
- API design (Phase 5) can start during Phase 3-4

---

## Migration Path

### From Static to Motion (User Journey)

**Step 1: Static Deployment (Current Release)**
```bash
# Deploy with static pose alignment
./velocity-report --lidar-sensor-id=hesai-01
# System uses identity transform, populates pose_id
```

**Step 2: Upgrade to 7DOF Support (Future Phase 1)**
```bash
# Binary supports 7DOF, but still static
./velocity-report --lidar-sensor-id=hesai-01
# System still uses identity, but can store 7DOF poses
```

**Step 3: Enable 3D Tracking (Future Phase 2-3)**
```bash
# Enable 3D tracking (still static sensor)
./velocity-report --lidar-sensor-id=hesai-01 --use-3d-tracking
# Tracks full 3D state with orientation
```

**Step 4: Motion Capture (Future Phase 4)**
```bash
# Enable motion capture (moving sensor)
./velocity-report --lidar-sensor-id=hesai-01 \
    --use-3d-tracking \
    --enable-ego-motion \
    --pose-source=gps+imu
# Tracks objects with ego-motion compensation
```

### Data Migration

**No data migration needed!**
- Static pose references work with motion code
- NULL pose_id handled gracefully (legacy data)
- 7DOF poses backward compatible with 4x4 matrices
- 2D tracks can coexist with 3D tracks

---

## External Dependencies

### For Motion Capture

**Pose Estimation Sources:**

1. **GPS + IMU (Most Common)**
   - GPS provides position (x, y, z with altitude)
   - IMU provides orientation (roll, pitch, yaw)
   - Combined at 10-100 Hz

2. **Visual Odometry**
   - Camera-based pose estimation
   - Works indoors (no GPS)
   - 30-60 Hz typical

3. **Wheel Odometry + IMU**
   - For ground vehicles
   - Dead reckoning with IMU correction
   - 100+ Hz possible

4. **SLAM (Simultaneous Localization and Mapping)**
   - LIDAR-based self-localization
   - No external sensors needed
   - Most computationally expensive

**Integration Points:**

```go
// Pose provider interface
type PoseProvider interface {
    GetCurrentPose() (*Pose7DOF, error)
    GetPoseAtTime(t time.Time) (*Pose7DOF, error)
    SubscribeToPoses(callback func(*Pose7DOF))
}

// Implementations
type GPSIMUProvider struct { ... }
type VisualOdometryProvider struct { ... }
type SLAMProvider struct { ... }
```

---

## Conclusion

This architecture specification provides a complete roadmap for adding motion capture capabilities to the velocity.report LIDAR tracking system.

**Current Release Focus:** Static pose alignment (see `static-pose-alignment-plan.md`)

**Future Work (This Document):**
- 7DOF pose representation
- Ego-motion compensation
- 3D tracking with orientation
- Moving sensor support

**Timeline:** 3.5-5 months after static alignment complete

**Dependencies:** External pose estimation source (GPS+IMU, visual odometry, etc.)

**Benefits:**
- Vehicle-mounted LIDAR support
- Mobile sensing applications
- Better object tracking (3D + orientation)
- AV ecosystem integration

---

## Related Documents

- **Current Release:** `static-pose-alignment-plan.md` (immediate work)
- **Current Implementation:** `foreground_tracking_plan.md` (existing tracking)
- **Database Schema:** `schema.sql` (current and future tables)
- **ML Pipeline:** `ml_pipeline_roadmap.md` (classification with 3D features)
