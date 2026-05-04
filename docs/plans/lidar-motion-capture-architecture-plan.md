# Motion capture architecture - future specification

- **Status:** Future Work (Not in Current Release)
- **Layers:** L2 Frames, L3 Grid, L4 Perception, L5 Tracks
- **Scope:** Moving LIDAR sensors (vehicle, bike, robot, drone mounted)
- **Purpose:** Long-term architecture specification for motion capture scenarios
- **Canonical:** [motion-capture.md](../lidar/operations/motion-capture.md)

---

> **Scope notice, concept overview, and implementation summary:** see [motion-capture.md](../lidar/operations/motion-capture.md).

---

## 7DOF pose representation

### Why 7DOF?

**7DOF = 7 Degrees of Freedom:**

- **Position:** (x, y, z) - 3 DOF
- **Orientation:** (qw, qx, qy, qz) - 4 DOF (unit quaternion)

**Advantages over 4x4 Matrix:**

- ✅ More compact (7 values vs 16)
- ✅ Avoids gimbal lock (quaternions don't have singularities)
- ✅ Efficient interpolation (SLERP for smooth motion)
- ✅ Standard in robotics (ROS, autonomous vehicle systems)
- ✅ Better for optimisation (fewer parameters)

**Current Implementation (4x4 Matrix):**

type Pose struct {
T [16]float64 // 4x4 homogeneous transformation matrix
// T = [R | t] where R is 3x3 rotation, t is 3x1 translation
// [0 | 1]
}
**Future Implementation (7DOF):**

**Pose7DOF** fields:

| Field          | Type          | Description                           |
| -------------- | ------------- | ------------------------------------- |
| TX, TY, TZ     | `float64`     | Translation in meters                 |
| QW, QX, QY, QZ | `float64`     | qw² + qx² + qy² + qz² = 1             |
| VX, VY, VZ     | `float64`     | Linear velocity (m/s)                 |
| WX, WY, WZ     | `float64`     | Angular velocity (rad/s)              |
| Covariance     | `[36]float64` | Position (3x3) + Orientation (3x3)    |
| ValidFromNanos | `int64`       | Temporal validity                     |
| ValidToNanos   | `*int64`      |                                       |
| T              | `[16]float64` | Can be computed from 7DOF when needed |

### Database schema (future)

**sensor_poses_7dof** columns:

| Column                        | Type      | Notes                             |
| ----------------------------- | --------- | --------------------------------- |
| pose_id                       | `INTEGER` | PRIMARY KEY                       |
| sensor_id                     | `TEXT`    | NOT NULL                          |
| from_frame                    | `TEXT`    | NOT NULL e.g., "sensor/hesai-01"  |
| to_frame                      | `TEXT`    | NOT NULL e.g., "site/main-st-001" |
| tx                            | `REAL`    | NOT NULL                          |
| ty                            | `REAL`    | NOT NULL                          |
| tz                            | `REAL`    | NOT NULL                          |
| qw                            | `REAL`    | NOT NULL Scalar component         |
| qx                            | `REAL`    | NOT NULL X component              |
| qy                            | `REAL`    | NOT NULL Y component              |
| qz                            | `REAL`    | NOT NULL Z component              |
| vx                            | `REAL`    | Linear velocity X (m/s)           |
| vy                            | `REAL`    | Linear velocity Y (m/s)           |
| vz                            | `REAL`    | Linear velocity Z (m/s)           |
| wx                            | `REAL`    | Angular velocity X (rad/s)        |
| wy                            | `REAL`    | Angular velocity Y (rad/s)        |
| wz                            | `REAL`    | Angular velocity Z (rad/s)        |
| covariance_6x6                | `BLOB`    | 36 float64 values                 |
| valid_from_ns                 | `INTEGER` | NOT NULL                          |
| valid_to_ns                   | `INTEGER` | NULL = current                    |
| t_rowmajor_4x4                | `BLOB`    | 16 float64 values                 |
| method                        | `TEXT`    |                                   |
| root_mean_square_error_meters | `REAL`    |                                   |

### Quaternion operations

**Required Utilities:**

| Method        | Parameters                     | Returns                                         |
| ------------- | ------------------------------ | ----------------------------------------------- |
| `Multiply`    | `q1, q2 Quaternion`            | `Quaternion`                                    |
| `Inverse`     | `q Quaternion`                 | `Quaternion`                                    |
| `Normalize`   | `q Quaternion`                 | `Quaternion`                                    |
| `Conjugate`   | `q Quaternion`                 | `Quaternion`                                    |
| `SLERP`       | `q1, q2 Quaternion, t float64` | `Quaternion  // Spherical Linear Interpolation` |
| `ToMatrix`    | `q Quaternion`                 | `[9]float64  // 3x3 rotation matrix`            |
| `FromMatrix`  | `R [9]float64`                 | `Quaternion`                                    |
| `ToEuler`     | `q Quaternion`                 | `(roll, pitch, yaw float64)`                    |
| `FromEuler`   | `roll, pitch, yaw float64`     | `Quaternion`                                    |
| `RotatePoint` | `q Quaternion, p [3]float64`   | `[3]float64`                                    |

**SLERP (Spherical Linear Interpolation):**

q(t) = sin((1-t)θ)/sin(θ) · q0 + sin(tθ)/sin(θ) · q1
where cos(θ) = q0 · q1 (dot product)
This provides smooth rotation interpolation between poses.

---

## Ego-Motion compensation

### Problem statement

When the sensor moves, measured velocities include **both object motion and sensor motion**:

v_measured = v_object + v_sensor
**Example (Vehicle-Mounted LIDAR):**

- Vehicle travelling at 20 m/s forward
- Stationary parked car in sensor view
- Measured velocity: -20 m/s (car appears to move backward)
- **Actual object velocity: 0 m/s** (car is stationary)

**Without ego-motion compensation:**

- ❌ All stationary objects appear to move backward at vehicle speed
- ❌ Tracking fails (objects don't match between frames)
- ❌ Classification fails (velocity-based features incorrect)

### Solution: pose-compensated tracking

**Kalman Filter with Ego-Motion:**

1. **Prediction Step (with sensor motion):**

**PredictWithEgoMotion** algorithm:

- Step 1: Transform track from world to sensor frame at t-1
- Step 2: Predict object motion in sensor frame (object-only motion)
- Step 3: Transform prediction back to world using current pose
- Update track position

2. **Update Step (with velocity de-biasing):**

**UpdateWithEgoMotion** algorithm:

- Measured velocity includes sensor velocity
- Compute sensor velocity at measurement point
- De-bias: Remove sensor velocity to get object velocity
- Use de-biased velocity for Kalman update
  **Sensor Velocity at Point (Pseudocode):**

- Note: This is mathematical pseudocode showing the algorithm concept.
- In actual Go implementation, vector operations require explicit element-wise computation.

**VelocityAtPoint** algorithm:

- v_point = v_sensor + ω × r
- where ω is angular velocity, r is radius vector from sensor to point
- r = point - p.Position() (element-wise subtraction)
- return v_linear + v_angular (element-wise addition)

### Requirements for ego-motion compensation

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

## 3D tracking with orientation

### Current: 2D ground plane tracking

**4-State Kalman Filter:**

State: x = [x, y, vx, vy]ᵀ
Covariance: P (4×4 matrix)
**Prediction:**

x' = F·x + w
where F = [1 0 dt 0]
[0 1 0 dt]
[0 0 1 0]
[0 0 0 1]
**Limitations:**

- ❌ No height tracking (assumes z = 0)
- ❌ No orientation (only heading from velocity)
- ❌ No vertical velocity (cannot track jumping, falling)
- ❌ No rotation (cannot track spinning objects)

### Future: 3D tracking with orientation

**13-State Kalman Filter:**

State: x = [x, y, z, vx, vy, vz, qw, qx, qy, qz, wx, wy, wz]ᵀ
Covariance: P (13×13 matrix)
**State Components:**

- **Position (3):** x, y, z (metres)
- **Linear velocity (3):** vx, vy, vz (m/s)
- **Orientation (4):** qw, qx, qy, qz (unit quaternion)
- **Angular velocity (3):** wx, wy, wz (rad/s)

**Prediction (3D with Orientation):**

**Predict** algorithm:

- Linear position prediction
- Angular position prediction (quaternion integration)
- q' = q + 0.5 · dt · Ω(ω) · q
- where Ω(ω) is the quaternion rate matrix
- Velocity predictions (constant velocity model)
- VX, VY, VZ, WX, WY, WZ remain constant
- Covariance prediction (13×13)
  **Update (with 3D Measurements):**

**Update** algorithm:

- Measurement: position + velocity (6D)
- Could be extended to include orientation measurements
- Kalman gain
- State update
- Covariance update
- Quaternion normalisation (maintain unit constraint)

### Object orientation estimation

**Use Cases:**

- Vehicle heading (different from velocity direction when turning)
- Pedestrian facing direction (different from walking direction)
- Object rotation detection (spinning, tumbling)
- Better bounding box alignment (oriented bounding boxes)

**Estimation Methods:**

1. **From Velocity (Simple):**

// Current method - heading from velocity
heading := atan2(vy, vx)
quat := QuaternionFromYaw(heading) // Rotation around Z-axis only 2. **From Point Cloud Principal Axes (Better):**

// Compute principal axes via PCA on cluster points
pca := ComputePCA(clusterPoints)
orientation := QuaternionFromPCA(pca) 3. **From Tracking History (Best):**

// Use Kalman filter to estimate orientation over time
// Integrates velocity heading, PCA, and temporal consistency
track.UpdateOrientation(measurement)

---

## Clustering and shape completion

### Consistent point cluster identification

For reliable object tracking, clusters must represent consistent real-world objects across frames.

**Multi-Stage Clustering Pipeline:**

Raw Points → Ground Removal → DBSCAN/Euclidean Clustering → Cluster Merging →
Shape Estimation → Temporal Association → 7-DOF Box Fitting
**Detailed algorithms are specified in `av-lidar-integration-plan.md` Phase 6-7:**

- **Phase 6:** Clustering Algorithms (AdaptiveDBSCAN, Octree spatial index, cluster merging)
- **Phase 7:** Occlusion Handling (symmetry completion, model-based priors, temporal refinement)

### Handling partial observations (occlusion)

**Problem:** LIDAR only observes surfaces facing the sensor. For a typical vehicle, only 1-3 sides are visible.

**Solution Approaches (see `av-lidar-integration-plan.md` for full details):**

1. **Symmetry-Based Completion:**
   - Use bilateral symmetry to estimate hidden dimensions
   - Works well when at least half the object is visible

2. **Model-Based Completion (Class Priors):**
   - Use learned shape priors per object class (Car, Truck, Bus, Pedestrian, etc.)
   - Blend observed and prior dimensions based on visibility fraction

3. **Temporal Shape Refinement:**
   - Accumulate observations as object moves and reveals different surfaces
   - Weight by visibility quality
   - Track which surfaces have been observed

4. **L-Shape Fitting (Vehicles):**
   - Detect perpendicular edges to estimate vehicle heading
   - Robust for corner-view observations

**Shape Priors for AV Industry Standard Classes:**

| Class        | Mean Length | Mean Width | Mean Height | Aspect Ratio |
| ------------ | ----------- | ---------- | ----------- | ------------ |
| Car          | 4.5m        | 1.8m       | 1.5m        | 1.8 - 3.0    |
| Truck        | 6.5m        | 2.2m       | 2.5m        | 2.0 - 4.0    |
| Bus          | 12.0m       | 2.5m       | 3.2m        | 3.5 - 6.0    |
| Pedestrian   | 0.5m        | 0.5m       | 1.7m        | 0.6 - 1.5    |
| Cyclist      | 1.8m        | 0.6m       | 1.7m        | 2.0 - 4.0    |
| Motorcyclist | 2.2m        | 0.8m       | 1.4m        | 2.0 - 3.5    |

---

## Data structure specifications

### Pose-Aware cluster (motion)

**WorldCluster** fields:

| Field                                                  | Type      | Description                                      |
| ------------------------------------------------------ | --------- | ------------------------------------------------ |
| ClusterID                                              | `int64`   |                                                  |
| SensorID                                               | `string`  |                                                  |
| WorldFrame                                             | `FrameID` |                                                  |
| TSUnixNanos                                            | `int64`   |                                                  |
| CentroidX, CentroidY, CentroidZ                        | `float32` | World coordinates (computed from sensor + pose)  |
| PoseID                                                 | `int64`   | NOT NULL for moving sensors                      |
| SensorCentroidX, SensorCentroidY, SensorCentroidZ      | `float32` | ⭐ REQUIRED for motion: Sensor-frame coordinates |
| BoundingBoxLength, BoundingBoxWidth, BoundingBoxHeight | `float32` | Bounding box and features (unchanged)            |
| HeightP95, IntensityMean                               | `float32` |                                                  |
| PointsCount                                            | `int`     |                                                  |

### 3D tracked object (motion)

**TrackedObject3D** fields:

| Field                         | Type           | Description                                            |
| ----------------------------- | -------------- | ------------------------------------------------------ |
| TrackID                       | `string`       |                                                        |
| SensorID                      | `string`       |                                                        |
| State                         | `TrackState`   |                                                        |
| X, Y, Z                       | `float32`      | Position (world frame, meters)                         |
| VX, VY, VZ                    | `float32`      | Linear velocity (world frame, m/s)                     |
| QW, QX, QY, QZ                | `float32`      | ⭐ NEW: Orientation (unit quaternion)                  |
| WX, WY, WZ                    | `float32`      | ⭐ NEW: Angular velocity (rad/s)                       |
| P                             | `[169]float32` | Row-major 13×13 matrix                                 |
| LastPoseID                    | `int64`        | ⭐ NEW: Last sensor pose (for ego-motion compensation) |
| Hits, Misses                  | `int`          | Lifecycle and features (unchanged)                     |
| FirstUnixNanos, LastUnixNanos | `int64`        |                                                        |
| ObservationCount              | `int`          |                                                        |

### Track observation (motion)

**TrackObservation3D** fields:

| Field                                                  | Type          | Description                            |
| ------------------------------------------------------ | ------------- | -------------------------------------- |
| TrackID                                                | `string`      |                                        |
| TSUnixNanos                                            | `int64`       |                                        |
| WorldFrame                                             | `string`      |                                        |
| PoseID                                                 | `int64`       | NOT NULL for moving sensors            |
| X, Y, Z                                                | `float32`     | Position (world frame)                 |
| VelocityX, VelocityY, VelocityZ                        | `float32`     | Velocity (world frame, de-biased)      |
| QW, QX, QY, QZ                                         | `float32`     | ⭐ NEW: Orientation (world frame)      |
| WX, WY, WZ                                             | `float32`     | ⭐ NEW: Angular velocity (world frame) |
| PositionCovariance                                     | `[9]float32`  | 3×3 position covariance                |
| OrientationCovariance                                  | `[16]float32` | 4×4 quaternion covariance              |
| BoundingBoxLength, BoundingBoxWidth, BoundingBoxHeight | `float32`     | Shape features (unchanged)             |
| HeightP95, IntensityMean                               | `float32`     |                                        |

---

## Implementation phases

### Phase 1: 7DOF pose infrastructure (foundation)

**Goal:** Add quaternion support without changing tracking

**Deliverables:**

- Quaternion math library (`internal/lidar/quaternion.go`)
- Pose7DOF struct with conversions
- Database schema for sensor_poses_7dof
- Unit tests for quaternion operations

**Estimated Effort:** 2-3 weeks

---

### Phase 2: 3D tracking (no orientation yet)

**Goal:** Extend Kalman to 3D position/velocity (9-state)

**Deliverables:**

- TrackedObject3D with [x, y, z, vx, vy, vz] (6-state position/velocity)
- 3D Kalman prediction/update
- Coexist with 2D tracker (parallel implementation)

**Estimated Effort:** 3-4 weeks

---

### Phase 3: orientation tracking

**Goal:** Add quaternion state to 3D tracker (13-state)

**Deliverables:**

- Full 13-state tracker [x, y, z, vx, vy, vz, qw, qx, qy, qz, wx, wy, wz]
- Quaternion prediction (integration)
- Orientation estimation from point clouds
- Quaternion normalisation in Kalman filter

**Estimated Effort:** 4-5 weeks

---

### Phase 4: ego-motion compensation

**Goal:** Support moving sensors

**Deliverables:**

- Pose-compensated Kalman prediction
- Velocity de-biasing
- Pose interpolation utilities
- Integration tests with simulated moving sensor

**Estimated Effort:** 3-4 weeks

---

### Phase 5: production integration

**Goal:** Enable motion capture in production

**Deliverables:**

- Feature flags (--enable-3d-tracking, --enable-ego-motion)
- REST API updates (/api/lidar/poses, /api/lidar/tracks/3d)
- PCAP tool support for 3D output
- Deployment documentation

**Estimated Effort:** 2-3 weeks

---

### Total timeline: 14-19 weeks (3.5-5 months)

**Dependencies:**

- Phase 1 must complete before Phase 2
- Phase 2 must complete before Phase 3
- Phase 3 must complete before Phase 4
- Phase 4 must complete before Phase 5

**Parallel Work Possible:**

- Phase 1 and documentation can be parallel
- API design (Phase 5) can start during Phase 3-4

---

## Migration path

### From static to motion (user journey)

**Step 1: Static Deployment (Current Release)**

Deploy with static pose alignment: `./velocity-report --lidar-sensor-id=hesai-01`
**Step 2: Upgrade to 7DOF Support (Future Phase 1)**

Binary supports 7DOF, but still static: `./velocity-report --lidar-sensor-id=hesai-01`
**Step 3: Enable 3D Tracking (Future Phase 2-3)**

Enable 3D tracking (still static sensor): `./velocity-report --lidar-sensor-id=hesai-01 --use-3d-tracking`
**Step 4: Motion Capture (Future Phase 4)**

Enable motion capture (moving sensor): `./velocity-report --lidar-sensor-id=hesai-01 --use-3d-tracking --enable-ego-motion --pose-source=gps+imu`

### Data migration

**No data migration needed!**

- Static pose references work with motion code
- NULL pose_id handled gracefully (legacy data)
- 7DOF poses backward compatible with 4x4 matrices
- 2D tracks can coexist with 3D tracks

---

## External dependencies

### For motion capture

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
   - LIDAR-based self-localisation
   - No external sensors needed
   - Most computationally expensive

**Integration Points:**

// Pose provider interface
type PoseProvider interface {
GetCurrentPose() (*Pose7DOF, error)
GetPoseAtTime(t time.Time) (*Pose7DOF, error)
SubscribeToPoses(callback func(\*Pose7DOF))
}

// Implementations
type GPSIMUProvider struct { ... }
type VisualOdometryProvider struct { ... }
type SLAMProvider struct { ... }
---\n\n## Related Documents

- **Current Implementation:** [../lidar/architecture/foreground-tracking.md](../lidar/architecture/foreground-tracking.md) (3DOF/2D+velocity tracking - what's actually deployed)
- **Deferred - Static Pose:** `static-pose-alignment-plan.md` (future static sensor calibration)
- **Deferred - AV Integration:** `av-lidar-integration-plan.md` (AV dataset integration, not current traffic monitoring)
- **Database Schema:** [../../internal/db/schema.sql](../../internal/db/schema.sql) (current and future tables)
- **ML Pipeline:** [LiDAR Pipeline Reference](../lidar/architecture/lidar-pipeline-reference.md) (classification pipeline)
