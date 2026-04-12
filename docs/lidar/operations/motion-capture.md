# Motion capture architecture

Future architecture specification for moving LiDAR sensors (vehicle-mounted, bike-mounted, robot, drone). Not in current release — current traffic monitoring uses a simpler 3DOF/2D+velocity model.

## Source

- Plan: `docs/plans/lidar-motion-capture-architecture-plan.md`
- Status: Future Work (not in current release)
- Layers: L2 Frames, L3 Grid, L4 Perception, L5 Tracks

## Scope notice

Architecture for **moving LiDAR sensors**. Current traffic monitoring uses:

- State vector: `[x, y, vx, vy]` (4-state Kalman filter)
- Object classes: `pedestrian, car, bird, other` (4 classes)
- Identity transform (sensor frame = world frame)
- Heading derived from velocity: `θ = atan2(vy, vx)`

See `docs/lidar/architecture/foreground-tracking.md` for the implemented tracking architecture.

## When motion capture is needed

| Scenario              | Why 7DOF is Needed                                           |
| --------------------- | ------------------------------------------------------------ |
| Vehicle-mounted LiDAR | Ego-motion compensation requires pose tracking               |
| Bike-mounted LiDAR    | Higher vibration, agile motion requires orientation tracking |
| Robot-mounted LiDAR   | Full 3D navigation requires Z velocity                       |
| Drone-mounted LiDAR   | Aerial mapping requires full pose estimation                 |

For static roadside sensors, the current 2D+velocity model is adequate.

## 7DOF pose representation

7DOF = 3 position (x, y, z) + 4 orientation (unit quaternion qw, qx, qy, qz).

Advantages over 4×4 matrix: more compact (7 vs 16 values), no gimbal lock, efficient interpolation (SLERP), standard in robotics.

```go
type Pose7DOF struct {
    TX, TY, TZ float64              // Position (metres)
    QW, QX, QY, QZ float64          // Orientation (unit quaternion)
    VX, VY, VZ float64              // Linear velocity (m/s)
    WX, WY, WZ float64              // Angular velocity (rad/s)
    Covariance [36]float64           // 6×6 position+orientation
    ValidFromNanos int64
    ValidToNanos   *int64
}
```

## Ego-Motion compensation

When the sensor moves, measured velocities include both object and sensor motion: `v_measured = v_object + v_sensor`.

**Solution:** Pose-compensated tracking:

1. **Prediction:** Transform track from world to sensor frame at t−1, predict object motion in sensor frame, transform back to world using current pose
2. **Update:** De-bias measured velocity by subtracting sensor velocity at measurement point (`v_object = v_measured − v_sensor`)
3. **Sensor velocity at point:** `v_point = v_sensor + ω × r` (angular velocity cross radius vector)

Requires: pose at each measurement time, pose velocity, ≥10 Hz pose estimates (preferably 100 Hz), pose interpolation.

## 3D tracking with orientation

**13-State Kalman Filter:**

- Position (3): x, y, z (metres)
- Linear velocity (3): vx, vy, vz (m/s)
- Orientation (4): qw, qx, qy, qz (unit quaternion)
- Angular velocity (3): wx, wy, wz (rad/s)

Quaternion prediction via integration: `q' = q + 0.5 · dt · Ω(ω) · q`, followed by normalisation.

Orientation estimation methods (in order of complexity): from velocity (`atan2`), from point cloud PCA, from Kalman tracking history.

## Data structures

**Pose-aware cluster:** adds `PoseID` (NOT NULL for moving sensors) and sensor-frame coordinates alongside world coordinates.

**3D tracked object:** adds Z position/velocity, quaternion orientation, angular velocity, 13×13 covariance, and last pose reference.

**3D track observation:** adds pose reference, 3×3 position covariance, 4×4 quaternion covariance.

## Implementation phases

| Phase | Goal                            | Effort    |
| ----- | ------------------------------- | --------- |
| 1     | 7DOF pose infrastructure        | 2–3 weeks |
| 2     | 3D tracking (no orientation)    | 3–4 weeks |
| 3     | Orientation tracking (13-state) | 4–5 weeks |
| 4     | Ego-motion compensation         | 3–4 weeks |
| 5     | Production integration          | 2–3 weeks |

Total: 14–19 weeks (3.5–5 months). Sequential dependencies: each phase requires the previous.

## External dependencies

Pose estimation sources: GPS+IMU (most common, 10–100 Hz), visual odometry (30–60 Hz), wheel odometry+IMU (100+ Hz), SLAM (LiDAR-based, most expensive).

```go
type PoseProvider interface {
    GetCurrentPose() (*Pose7DOF, error)
    GetPoseAtTime(t time.Time) (*Pose7DOF, error)
    SubscribeToPoses(callback func(*Pose7DOF))
}
```

## Migration path

No data migration needed between phases. Static pose references work with motion code. NULL `pose_id` handled for legacy data. 7DOF backward-compatible with 4×4 matrices. 2D and 3D tracks can coexist.
