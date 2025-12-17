# Current vs 7DOF: Feature Comparison

**Quick Reference:** Side-by-side comparison of current implementation vs future 7DOF system

---

## Pose Representation

| Feature | Current (4x4 Matrix) | Future (7DOF) |
|---------|---------------------|---------------|
| **Storage format** | 16 float64 values (4x4 matrix) | 7 float64 values (position + quaternion) |
| **Position encoding** | Embedded in matrix T[0:3][3] | Explicit (tx, ty, tz) |
| **Orientation encoding** | Rotation matrix (9 values) | Unit quaternion (qw, qx, qy, qz) |
| **Gimbal lock** | Yes (Euler angle extraction) | No (quaternion avoids singularities) |
| **Interpolation** | Matrix interpolation (imprecise) | SLERP (smooth quaternion interpolation) |
| **Storage efficiency** | 128 bytes | 56 bytes |
| **AV compatibility** | Requires conversion | Native (ROS standard) |

---

## Tracking Capabilities

| Feature | Current (2D) | Future (3D + Orientation) |
|---------|--------------|---------------------------|
| **State vector** | 4D: [x, y, vx, vy] | 13D: [x, y, z, vx, vy, vz, qw, qx, qy, qz, wx, wy, wz] |
| **Position tracking** | 2D ground plane only | Full 3D position |
| **Velocity tracking** | 2D velocity (vx, vy) | 3D velocity (vx, vy, vz) |
| **Orientation tracking** | None (derived from velocity) | Full quaternion state |
| **Angular velocity** | None | 3D angular velocity (wx, wy, wz) |
| **Height tracking** | ❌ No | ✅ Yes (Z-axis tracked) |
| **Object rotation** | ❌ No | ✅ Yes (quaternion state) |
| **Covariance size** | 4x4 (16 values) | 13x13 (169 values) |

---

## Sensor Motion Support

| Feature | Current | Future |
|---------|---------|--------|
| **Static sensor** | ✅ Supported (identity transform) | ✅ Supported |
| **Moving sensor** | ❌ Not supported | ✅ Supported (ego-motion compensation) |
| **Pose-compensated prediction** | ❌ No | ✅ Yes |
| **Velocity de-biasing** | ❌ No | ✅ Yes (removes sensor velocity) |
| **Track-pose association** | ❌ No pose_id tracking | ✅ Pose_id per observation |
| **Vehicle-mounted LIDAR** | ❌ Not supported | ✅ Supported |

---

## Calibration Management

| Feature | Current | Future |
|---------|---------|--------|
| **Pose versioning** | ✅ Time-based (valid_from_ns, valid_to_ns) | ✅ Time-based + 7DOF |
| **World coordinates stored** | ✅ Yes (baked in) | ✅ Yes (computed from pose_id) |
| **Sensor coordinates stored** | ❌ No | ✅ Yes (for re-transformation) |
| **Calibration updates** | ❌ Requires data re-collection | ✅ Can re-transform historical data |
| **Re-transformation API** | ❌ Not available | ✅ Available |
| **Pose interpolation** | ❌ Not supported | ✅ SLERP interpolation |

---

## Data Structures

### Pose Struct

| Field | Current (Pose) | Future (Pose7DOF) |
|-------|----------------|-------------------|
| **PoseID** | ✅ int64 | ✅ int64 |
| **SensorID** | ✅ string | ✅ string |
| **T (4x4 matrix)** | ✅ [16]float64 | ✅ [16]float64 (computed on-demand) |
| **TX, TY, TZ** | ❌ No (embedded in T) | ✅ float64 (explicit) |
| **QW, QX, QY, QZ** | ❌ No | ✅ float64 (quaternion) |
| **VX, VY, VZ** | ❌ No | ✅ float64 (linear velocity) |
| **WX, WY, WZ** | ❌ No | ✅ float64 (angular velocity) |
| **Covariance** | ❌ No | ✅ [36]float64 (6x6: position + orientation) |

### WorldCluster Struct

| Field | Current | Future |
|-------|---------|--------|
| **ClusterID** | ✅ int64 | ✅ int64 |
| **Centroid (world)** | ✅ CentroidX/Y/Z | ✅ CentroidX/Y/Z |
| **PoseID** | ❌ No | ✅ *int64 (pose used for transform) |
| **Centroid (sensor)** | ❌ No | ✅ SensorCentroidX/Y/Z (raw data) |
| **BoundingBox** | ✅ Length/Width/Height | ✅ Length/Width/Height |
| **Features** | ✅ HeightP95, IntensityMean | ✅ HeightP95, IntensityMean |

### TrackedObject Struct

| Field | Current (2D) | Future (3D) |
|-------|--------------|-------------|
| **TrackID** | ✅ string | ✅ string |
| **Position** | ✅ X, Y (float32) | ✅ X, Y, Z (float32) |
| **Velocity** | ✅ VX, VY (float32) | ✅ VX, VY, VZ (float32) |
| **Orientation** | ❌ No | ✅ QW, QX, QY, QZ (float32) |
| **AngularVelocity** | ❌ No | ✅ WX, WY, WZ (float32) |
| **Covariance** | ✅ P [16]float32 (4x4) | ✅ P [169]float32 (13x13) |
| **LastPoseID** | ❌ No | ✅ *int64 (for ego-motion) |

### TrackObservation Struct

| Field | Current | Future |
|-------|---------|--------|
| **TrackID** | ✅ string | ✅ string |
| **Position** | ✅ X, Y, Z | ✅ X, Y, Z |
| **Velocity** | ✅ VelocityX, VelocityY | ✅ VelocityX, VelocityY, VelocityZ |
| **PoseID** | ❌ No | ✅ *int64 |
| **Orientation** | ❌ No | ✅ QW, QX, QY, QZ |
| **AngularVelocity** | ❌ No | ✅ WX, WY, WZ |
| **PositionCovariance** | ❌ No | ✅ [9]float32 (3x3) |
| **OrientationCovariance** | ❌ No | ✅ [16]float32 (4x4) |

---

## API Endpoints

| Endpoint | Current | Future |
|----------|---------|--------|
| **GET /api/lidar/tracks** | ✅ 2D tracks | ✅ 2D tracks (backward compatible) |
| **GET /api/lidar/tracks/3d** | ❌ Not available | ✅ 3D tracks with orientation |
| **GET /api/lidar/tracks/{id}** | ✅ 2D track details | ✅ 2D track details (backward compatible) |
| **GET /api/lidar/tracks/3d/{id}** | ❌ Not available | ✅ 3D track details |
| **GET /api/lidar/poses** | ❌ Not available | ✅ List sensor poses (7DOF) |
| **GET /api/lidar/poses/{id}** | ❌ Not available | ✅ Get specific pose (7DOF) |
| **GET /api/lidar/poses/current/{sensor}** | ❌ Not available | ✅ Get current sensor pose |

---

## Use Cases Enabled

| Use Case | Current | Future | Notes |
|----------|---------|--------|-------|
| **Static roadside LIDAR** | ✅ Fully supported | ✅ Fully supported | Current primary use case |
| **Pedestrian tracking (2D)** | ✅ Supported | ✅ Supported | Ground plane tracking |
| **Vehicle tracking (2D)** | ✅ Supported | ✅ Supported | Ground plane tracking |
| **Bird tracking (3D)** | ⚠️ Partial (no height) | ✅ Full support | Needs Z-axis tracking |
| **Thrown object tracking** | ❌ Not supported | ✅ Supported | Needs 3D velocity |
| **Vehicle orientation** | ⚠️ Velocity heading only | ✅ Full quaternion | Better classification |
| **Vehicle-mounted LIDAR** | ❌ Not supported | ✅ Supported | Ego-motion compensation |
| **Robot-mounted LIDAR** | ❌ Not supported | ✅ Supported | Moving sensor support |
| **Drone-mounted LIDAR** | ❌ Not supported | ✅ Supported | 6DOF ego-motion |
| **Calibration updates** | ⚠️ Requires re-collection | ✅ Re-transform historical data | Pose-aware storage |
| **Multi-session SLAM** | ❌ Not supported | ✅ Supported | Pose graph optimization |
| **AV integration** | ⚠️ Requires conversion | ✅ Native compatibility | ROS-compatible poses |

---

## Performance Comparison

| Metric | Current (2D) | Future (3D) | Impact |
|--------|--------------|-------------|--------|
| **State vector size** | 4 values | 13 values | +225% |
| **Covariance size** | 16 values | 169 values | +956% |
| **Prediction cost** | O(1) - 4x4 matrix ops | O(1) - 13x13 matrix ops | ~3x slower |
| **Update cost** | O(1) - Kalman update | O(1) - Kalman update + quaternion normalization | ~4x slower |
| **Memory per track** | ~200 bytes | ~800 bytes | +300% |
| **Frame processing** | ~10ms @ 100 tracks | ~30ms @ 100 tracks (estimated) | Within real-time budget |

**Note:** 3D tracking has higher computational cost but remains real-time feasible on Raspberry Pi 4.

---

## Migration Path Summary

| Phase | PR | Description | Lines | Breaks Existing? |
|-------|------|-------------|-------|------------------|
| **Foundation** | #1 | Database schema extensions | ~200 | ❌ No |
| **Foundation** | #2 | Quaternion utilities | ~400 | ❌ No |
| **Data Layer** | #3 | Pose-aware clusters | ~300 | ❌ No |
| **Tracking** | #4 | 3D tracking foundation | ~600 | ❌ No (parallel tracker) |
| **Tracking** | #5 | Ego-motion compensation | ~400 | ❌ No |
| **Integration** | #6 | REST API extensions | ~500 | ❌ No (additive) |
| **Integration** | #7 | Production integration | ~300 | ❌ No (opt-in) |

**Total:** ~2,700 lines, 7 PRs, 5-8 weeks

**Backward Compatibility:** ✅ Maintained throughout (2D tracker remains default, 3D is opt-in)

---

## Decision Matrix

### When to Use 2D Tracking (Current)

✅ **Use 2D tracking when:**
- Static roadside LIDAR (no sensor motion)
- Ground plane objects only (cars, pedestrians on flat road)
- Performance critical (need lowest latency)
- Stable production deployment (risk-averse)
- Don't need orientation information

### When to Use 3D Tracking (Future)

✅ **Use 3D tracking when:**
- Moving sensor (vehicle-mounted, robot, drone)
- 3D objects (birds, thrown objects, jumping)
- Need orientation (vehicle heading, object rotation)
- Calibration may change (want re-transformation)
- AV integration required (ROS compatibility)
- Multi-sensor fusion planned

---

## Summary

**Current System:**
- ✅ Production-proven 2D tracking
- ✅ Works well for static roadside LIDAR
- ⚠️ Limited to ground plane objects
- ❌ Cannot handle moving sensors
- ❌ No orientation tracking

**Future System (7DOF):**
- ✅ Full 3D tracking with orientation
- ✅ Moving sensor support (ego-motion)
- ✅ Calibration updates without re-collection
- ✅ AV-ready (ROS-compatible poses)
- ✅ Backward compatible (2D still available)
- ⚠️ Higher computational cost (~3-4x)

**Recommendation:** Implement 7DOF incrementally (7 PRs) while maintaining 2D as default. Enable 3D via feature flag for specific use cases.

---

## Related Documents

- **Technical Analysis:** `7dof-schema-compatibility-analysis.md`
- **Implementation Plan:** `pr-split-recommendation.md`
- **Executive Summary:** `EXECUTIVE-SUMMARY-7DOF.md`
- **Navigation:** `README-7DOF-MIGRATION.md`
