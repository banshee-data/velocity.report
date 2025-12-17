# PR Split Recommendation for 7DOF LIDAR Schema Migration

**Status:** Implementation Planning  
**Date:** December 17, 2025  
**Author:** Ictinus (Product Architecture Agent)  
**Related:** See `7dof-schema-compatibility-analysis.md` for detailed technical analysis

---

## Problem Statement

The current LIDAR implementation uses 4x4 transformation matrices with identity transform defaults. To support autonomous vehicle (AV) integration and proper multi-sensor fusion, the system needs to migrate to 7DOF (7 Degrees of Freedom) pose representation: position (x, y, z) + orientation (quaternion: qw, qx, qy, qz).

**Question:** Should this work be done in one massive PR or split into multiple incremental PRs?

---

## Executive Summary

**Recommendation: Split into 7 incremental PRs** (Option B - Incremental Approach)

**Rationale:**
- Current code is production-deployed (Raspberry Pi systems)
- Changes affect core data structures (high risk of breaking changes)
- Each phase delivers independent value
- Smaller PRs = faster reviews = shorter feedback cycles
- Easier to bisect bugs and rollback if issues found

**Total Scope:** ~2,700 lines across 7 PRs over 5-8 weeks

---

## Option Comparison

### Option A: Single Monolithic PR ❌

**What it would include:**
- All schema changes (sensor_poses_7dof table, cluster/track pose references)
- Complete quaternion infrastructure
- 13-state 3D Kalman tracker
- Ego-motion compensation
- REST API updates
- Production integration

**Estimated Size:** 2,700+ lines changed, 40+ files modified

**Pros:**
- ✅ All changes atomic (no intermediate states)
- ✅ Easy to reason about full system behavior
- ✅ Single comprehensive test suite

**Cons:**
- ❌ **Massive review burden** (weeks of review time)
- ❌ High risk of merge conflicts during development
- ❌ Long feedback cycle (can't merge until everything perfect)
- ❌ Harder to bisect bugs if issues found later
- ❌ Blocks all other LIDAR work for weeks
- ❌ All-or-nothing rollback (can't keep good parts)

**Verdict:** Not recommended for production system

---

### Option B: Incremental PRs (7 Phases) ✅ RECOMMENDED

**PR Sequence:**

1. **PR #1: Database Schema Extensions** (~200 lines)
   - Add sensor_poses_7dof table
   - Migration script for existing data
   - No functionality change

2. **PR #2: Quaternion Utilities** (~400 lines)
   - Add quaternion math library
   - Pose7DOF struct
   - Conversion to/from 4x4 matrices

3. **PR #3: Pose-Aware Clusters** (~300 lines)
   - Add pose_id to clusters
   - Store sensor-frame + world-frame coords
   - Re-transformation API

4. **PR #4: 3D Tracking Foundation** (~600 lines)
   - New 13-state Kalman tracker
   - Keep existing 2D tracker (backward compatible)
   - Quaternion state handling

5. **PR #5: Ego-Motion Compensation** (~400 lines)
   - Moving sensor support
   - Velocity de-biasing
   - Pose-compensated predictions

6. **PR #6: REST API Extensions** (~500 lines)
   - /api/lidar/poses endpoints
   - 3D track queries
   - Backward compatible with 2D API

7. **PR #7: Production Integration** (~300 lines)
   - Feature flags (--use-3d-tracking)
   - PCAP tool updates
   - Deployment documentation

**Pros:**
- ✅ **Smaller reviews** (200-600 lines each, reviewable in 1-2 days)
- ✅ Each PR independently testable and mergeable
- ✅ Lower risk (can roll back individual phases)
- ✅ Parallel work possible (different devs on different phases)
- ✅ Delivers value incrementally (better calibration → 3D tracking → ego-motion)
- ✅ Doesn't block other LIDAR work
- ✅ Easier to bisect bugs (know which PR introduced issue)

**Cons:**
- ❌ Requires careful API design (backward compatibility)
- ❌ Need to maintain intermediate states
- ❌ More total review time across all PRs (but parallelizable)

**Verdict:** Recommended for production system ✅

---

## Detailed PR Breakdown

### PR #1: Database Schema Extensions

**Goal:** Add 7DOF representation to database without breaking existing code

**Files Changed:**
- `internal/db/migrations/000011_add_7dof_poses.up.sql` (NEW)
- `internal/db/migrations/000011_add_7dof_poses.down.sql` (NEW)
- `internal/db/schema.sql` (add sensor_poses_7dof table)
- `internal/db/migrations_test.go` (test migration)

**Key Changes:**
```sql
CREATE TABLE sensor_poses_7dof (
    pose_id INTEGER PRIMARY KEY,
    sensor_id TEXT NOT NULL,
    tx REAL, ty REAL, tz REAL,           -- Position
    qw REAL, qx REAL, qy REAL, qz REAL,  -- Orientation (quaternion)
    vx REAL, vy REAL, vz REAL,           -- Linear velocity (optional)
    wx REAL, wy REAL, wz REAL,           -- Angular velocity (optional)
    t_rowmajor_4x4 BLOB,                 -- Legacy compatibility
    valid_from_ns INTEGER NOT NULL,
    valid_to_ns INTEGER,
    ...
);
```

**Exit Criteria:**
- ✅ Migration runs on existing databases
- ✅ All existing tests pass (no functionality change)
- ✅ Down migration tested (rollback works)

**Estimated Review Time:** 1-2 hours  
**Estimated Development Time:** 2-3 days

---

### PR #2: Quaternion Utilities

**Goal:** Add quaternion math without changing existing APIs

**Files Changed:**
- `internal/lidar/quaternion.go` (NEW)
- `internal/lidar/quaternion_test.go` (NEW)
- `internal/lidar/arena.go` (add Pose7DOF struct)
- `internal/lidar/transform.go` (add conversion functions)

**Key Functions:**
```go
// Core quaternion operations
func QuaternionMultiply(q1, q2 Quaternion) Quaternion
func QuaternionInverse(q Quaternion) Quaternion
func QuaternionNormalize(q Quaternion) Quaternion
func QuaternionSLERP(q1, q2 Quaternion, t float64) Quaternion

// Conversion utilities
func Matrix4x4ToQuaternion(T [16]float64) (tx, ty, tz, qw, qx, qy, qz float64)
func QuaternionToMatrix4x4(tx, ty, tz, qw, qx, qy, qz float64) [16]float64

// Pose7DOF struct and methods
type Pose7DOF struct {
    TX, TY, TZ float64      // Position
    QW, QX, QY, QZ float64  // Orientation
    ...
}
func (p *Pose7DOF) ToMatrix4x4() [16]float64
func (p *Pose7DOF) FromMatrix4x4(T [16]float64)
func (p *Pose7DOF) Interpolate(other *Pose7DOF, t float64) *Pose7DOF
```

**Exit Criteria:**
- ✅ All quaternion operations tested (100+ unit tests)
- ✅ Conversion to/from 4x4 validated (round-trip accuracy)
- ✅ SLERP interpolation tested
- ✅ Existing Pose struct unchanged (backward compatible)

**Estimated Review Time:** 2-3 hours  
**Estimated Development Time:** 3-4 days

---

### PR #3: Pose-Aware Clusters

**Goal:** Track which pose was used for each measurement

**Files Changed:**
- `internal/lidar/arena.go` (add pose_id to WorldCluster)
- `internal/lidar/track_store.go` (update InsertCluster)
- `internal/lidar/track_store_test.go`
- `internal/db/migrations/000012_add_cluster_pose_refs.up.sql` (NEW)
- `internal/lidar/docs/schema.sql`

**Key Changes:**
```go
type WorldCluster struct {
    ...
    PoseID *int64  // NEW: Reference to sensor pose at measurement time
    
    // NEW: Raw sensor-frame coordinates (for re-transformation)
    SensorCentroidX *float32
    SensorCentroidY *float32
    SensorCentroidZ *float32
}

// NEW: Re-transformation API
func RecomputeWorldCoordinates(db *sql.DB, clusterID int64, newPoseID int64) error
```

**Exit Criteria:**
- ✅ Clusters can be stored with pose_id reference
- ✅ NULL pose_id supported (backward compatibility)
- ✅ Re-transformation API functional
- ✅ All existing tests pass with NULL pose_id

**Estimated Review Time:** 1-2 hours  
**Estimated Development Time:** 2-3 days

---

### PR #4: 3D Tracking Foundation

**Goal:** Add 3D tracker without replacing 2D tracker

**Files Changed:**
- `internal/lidar/tracking_3d.go` (NEW)
- `internal/lidar/tracking_3d_test.go` (NEW)
- `internal/lidar/arena.go` (add TrackedObject3D struct)
- `internal/lidar/docs/tracking_3d.md` (NEW - documentation)

**Key Changes:**
```go
type TrackedObject3D struct {
    // 13-state Kalman: [x, y, z, vx, vy, vz, qw, qx, qy, qz, wx, wy, wz]
    X, Y, Z       float32  // Position
    VX, VY, VZ    float32  // Linear velocity
    QW, QX, QY, QZ float32  // Orientation (quaternion)
    WX, WY, WZ    float32  // Angular velocity
    P [169]float32  // 13x13 covariance
}

type Tracker3D struct {
    Tracks map[string]*TrackedObject3D
    Config TrackerConfig3D
}

func (t *Tracker3D) Update(clusters []WorldCluster, timestamp time.Time)
```

**Exit Criteria:**
- ✅ 13-state Kalman filter functional
- ✅ Quaternion state correctly normalized
- ✅ 3D synthetic trajectories tracked accurately
- ✅ Existing 2D tracker unchanged (full backward compatibility)

**Estimated Review Time:** 3-4 hours  
**Estimated Development Time:** 5-7 days

---

### PR #5: Ego-Motion Compensation

**Goal:** Support vehicle-mounted / moving LIDAR

**Files Changed:**
- `internal/lidar/tracking_3d.go` (update Predict)
- `internal/lidar/tracking_3d_test.go` (add moving sensor tests)
- `internal/lidar/velocity_estimation.go` (velocity de-biasing)
- `internal/lidar/docs/ego_motion.md` (NEW - documentation)

**Key Changes:**
```go
type TrackedObject3D struct {
    ...
    LastSensorPoseID *int64  // NEW: Pose used for last prediction
}

// Updated prediction with ego-motion compensation
func (t *Tracker3D) Predict(dt float32, currentPose *Pose7DOF) {
    for _, track := range t.Tracks {
        // Compensate for sensor motion
        if track.LastSensorPoseID != nil {
            prevPose := loadPose(track.LastSensorPoseID)
            track.compensateEgoMotion(prevPose, currentPose, dt)
        }
        
        // Then predict object motion
        track.kalmanPredict(dt)
    }
}

// Velocity de-biasing
func DeBiasVelocity(measured Vector3, sensorVelocity Vector3) Vector3 {
    return measured.Sub(sensorVelocity)
}
```

**Exit Criteria:**
- ✅ Tracks stable with moving sensor (synthetic tests)
- ✅ Velocity de-biasing validated
- ✅ Integration test with PCAP + pose sequence
- ✅ Existing trackers unchanged

**Estimated Review Time:** 2-3 hours  
**Estimated Development Time:** 4-5 days

---

### PR #6: REST API Extensions

**Goal:** Expose 7DOF data through HTTP APIs

**Files Changed:**
- `internal/lidar/monitor/pose_api.go` (NEW)
- `internal/lidar/monitor/pose_api_test.go` (NEW)
- `internal/lidar/monitor/track_api.go` (add 3D endpoints)
- `cmd/radar/radar.go` (register routes)
- `docs/api/lidar_poses.md` (NEW - API documentation)

**New Endpoints:**
```
GET  /api/lidar/poses                    - List all poses (with 7DOF)
GET  /api/lidar/poses/{pose_id}          - Get specific pose
GET  /api/lidar/poses/current/{sensor_id} - Get current pose for sensor
GET  /api/lidar/tracks/3d                - List 3D tracks (with orientation)
GET  /api/lidar/tracks/3d/{track_id}     - Get 3D track details
```

**JSON Response Example:**
```json
{
  "pose_id": 42,
  "sensor_id": "hesai-pandar40p-001",
  "position": {"x": 0.0, "y": 0.0, "z": 1.5},
  "orientation": {"qw": 1.0, "qx": 0.0, "qy": 0.0, "qz": 0.0},
  "velocity": {"vx": 2.5, "vy": 0.0, "vz": 0.0},
  "angular_velocity": {"wx": 0.0, "wy": 0.0, "wz": 0.1},
  "valid_from_ns": 1702857600000000000,
  "valid_to_ns": null
}
```

**Exit Criteria:**
- ✅ All new endpoints functional
- ✅ Backward compatibility with existing 2D API
- ✅ API documentation complete
- ✅ Integration tests for all endpoints

**Estimated Review Time:** 2-3 hours  
**Estimated Development Time:** 3-4 days

---

### PR #7: Production Integration

**Goal:** Enable 7DOF tracking in production pipeline

**Files Changed:**
- `cmd/radar/radar.go` (add --use-3d-tracking flag)
- `cmd/tools/pcap-analyze/main.go` (add 3D support)
- `internal/lidar/dual_pipeline.go` (3D tracker option)
- `docs/deployment/3d_tracking.md` (NEW - deployment guide)
- `configs/velocity-report-3d.yaml` (NEW - example config)

**Command Line Flags:**
```bash
# Enable 3D tracking
./velocity-report --use-3d-tracking

# With moving sensor (ego-motion compensation)
./velocity-report --use-3d-tracking --enable-ego-motion

# PCAP analysis with 3D tracking
./pcap-analyze --pcap capture.pcap --use-3d-tracking --output ./results
```

**Exit Criteria:**
- ✅ 3D tracking opt-in via config flag
- ✅ PCAP analysis tool supports 3D output
- ✅ Production deployment guide updated
- ✅ Rollback plan documented
- ✅ Default behavior unchanged (2D tracking)

**Estimated Review Time:** 1-2 hours  
**Estimated Development Time:** 2-3 days

---

## Timeline Estimate

### Sequential Development (Single Developer)

```
Week 1-2:  PR #1 (Schema) + PR #2 (Quaternions) - parallel
Week 3:    PR #3 (Pose-Aware Clusters)
Week 4-5:  PR #4 (3D Tracking)
Week 6:    PR #5 (Ego-Motion)
Week 7:    PR #6 (REST APIs)
Week 8:    PR #7 (Production)

Total: 8 weeks
```

### Parallel Development (2-3 Developers)

```
Week 1-2:  Dev A: PR #1 + #2 (Foundation)
           Dev B: PR #3 (Clusters) - blocked on #2
Week 3-4:  Dev A: PR #4 (3D Tracking) - depends on #2
           Dev B: PR #6 (REST APIs) - depends on #3, #4
Week 5:    Dev A: PR #5 (Ego-Motion) - depends on #4
           Dev B: PR #7 (Production) - depends on #4, #5
Week 6:    Integration testing and documentation

Total: 5-6 weeks
```

---

## Risk Mitigation

### Risk #1: Breaking Changes Between PRs

**Mitigation:**
- Each PR maintains full backward compatibility
- Feature flags for new functionality
- Comprehensive integration tests at each phase
- Rollback plan for each PR

### Risk #2: Performance Regression

**Mitigation:**
- Benchmark tests for 3D tracker (compare to 2D)
- PCAP-based performance tests
- Monitor frame processing time in production
- Profiling before/after each PR

### Risk #3: Data Migration Issues

**Mitigation:**
- Test migrations on production-scale databases
- Backup strategy documented
- Migration can run while system is offline
- Rollback script tested

### Risk #4: API Incompatibility

**Mitigation:**
- All new APIs are additive (not replacing existing)
- Existing endpoints unchanged
- Deprecation warnings before removal (future)
- Version negotiation if needed

---

## Success Criteria

### Phase-by-Phase Success

**PR #1 Success:**
- ✅ Existing databases can be migrated without errors
- ✅ No downtime required
- ✅ Can rollback if needed

**PR #2 Success:**
- ✅ Quaternion operations mathematically correct (tested)
- ✅ Conversions to/from matrices accurate
- ✅ No impact on existing code

**PR #3 Success:**
- ✅ Calibration updates can re-transform historical data
- ✅ Pose-aware clusters stored correctly
- ✅ No performance regression

**PR #4 Success:**
- ✅ 3D tracking more accurate than 2D (synthetic tests)
- ✅ Orientation estimates stable
- ✅ No impact on existing 2D tracking

**PR #5 Success:**
- ✅ Moving sensor scenarios tracked correctly
- ✅ Velocity estimates unbiased
- ✅ Integration test passes

**PR #6 Success:**
- ✅ APIs return correct 7DOF data
- ✅ Backward compatibility maintained
- ✅ Documentation complete

**PR #7 Success:**
- ✅ Production deployment successful
- ✅ Feature flag works as expected
- ✅ Can rollback to 2D tracking if needed

---

## Final Recommendation

**Choose Incremental Approach (7 PRs)** for the following reasons:

1. **Production Safety:** Current code is deployed on Raspberry Pi systems. Incremental changes reduce risk.

2. **Review Quality:** 200-600 line PRs can be reviewed thoroughly. 2,700 line PRs cannot.

3. **Development Velocity:** Smaller PRs merge faster, reducing merge conflicts and blocking.

4. **Value Delivery:** Each phase delivers independent value:
   - PR #1-2: Better calibration representation
   - PR #3: Calibration updates without re-collection
   - PR #4: 3D tracking capability
   - PR #5: Moving sensor support (AV integration ready)
   - PR #6-7: Production deployment

5. **Risk Mitigation:** Can roll back individual phases if issues found. Can't do this with monolithic PR.

6. **Team Efficiency:** Multiple developers can work in parallel on different phases.

**Expected Outcome:**
- 5-8 weeks total timeline (vs 8-10 weeks for monolithic)
- Higher code quality (better review coverage)
- Lower risk (incremental rollout)
- Better documentation (written per phase)

---

## Next Steps

1. **Review this recommendation** with team/stakeholders
2. **Create GitHub issues** for each of the 7 PRs
3. **Set up feature branch** for incremental work
4. **Begin PR #1** (Database Schema Extensions)
5. **Parallel PR #2** once schema design approved

---

## Questions for Stakeholders

1. **Timeline:** Is 5-8 weeks acceptable for this feature?
2. **Resources:** Can we allocate 2-3 developers for parallel work?
3. **Testing:** Do we have test PCAP files for moving sensor scenarios?
4. **Deployment:** Is there a staging Raspberry Pi for testing before production?
5. **Rollback:** What is our rollback SLA if issues found in production?

---

## References

- **Technical Analysis:** `7dof-schema-compatibility-analysis.md`
- **Current Tracking:** `internal/lidar/tracking.go`
- **Current Schema:** `internal/lidar/docs/schema.sql`
- **Foreground Tracking Plan:** `internal/lidar/docs/foreground_tracking_plan.md`
