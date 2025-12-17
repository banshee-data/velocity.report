# 7DOF LIDAR Schema Migration - Documentation Index

**Project:** velocity.report LIDAR Tracking System  
**Feature:** 7 Degrees of Freedom (7DOF) Pose Representation  
**Status:** Planning Complete - Ready for Implementation

---

## Quick Start

**New to this project?** Start here:
1. Read: `EXECUTIVE-SUMMARY-7DOF.md` (5-min overview)
2. Review: `pr-split-recommendation.md` (understand the plan)
3. Deep dive: `7dof-schema-compatibility-analysis.md` (technical details)

**Ready to implement?** Follow the PR sequence in `pr-split-recommendation.md`

---

## Document Overview

### 📋 EXECUTIVE-SUMMARY-7DOF.md
**Purpose:** High-level overview for stakeholders and decision-makers

**Contents:**
- Problem statement and current limitations
- Solution overview (7DOF pose representation)
- PR sequence summary (7 phases)
- Timeline and resource estimates
- Success criteria and risks
- Next steps and stakeholder questions

**Audience:** Product managers, tech leads, stakeholders  
**Reading Time:** 5-10 minutes

---

### 🎯 pr-split-recommendation.md
**Purpose:** Implementation strategy and PR breakdown

**Contents:**
- Option A vs Option B comparison (monolithic vs incremental)
- Detailed breakdown of all 7 PRs with:
  - Files changed
  - Code examples
  - Exit criteria
  - Review/development time estimates
- Timeline with sequential and parallel options
- Risk mitigation strategies
- Success criteria per phase

**Audience:** Engineering team, code reviewers  
**Reading Time:** 15-20 minutes

---

### 🔬 7dof-schema-compatibility-analysis.md
**Purpose:** Comprehensive technical analysis

**Contents:**
- Current data structures audit:
  - Pose struct (Go code)
  - sensor_poses table (database)
  - WorldCluster, TrackedObject, TrackObservation
  - VelocityCoherentTrack structures
- 7DOF schema requirements:
  - Extended pose representation (position + quaternion)
  - Pose-aware cluster structure
  - 3D tracking with orientation
  - Ego-motion compensation
- 5 major compatibility gaps:
  1. Static sensor assumption
  2. 2D tracking only
  3. No ego-motion compensation
  4. No quaternion infrastructure
  5. Baked-in world coordinates
- 6-phase migration path with code examples
- Quaternion math primer
- 4x4 matrix conversion equations
- Ego-motion compensation formulas

**Audience:** Engineers implementing the changes  
**Reading Time:** 30-40 minutes

---

## Problem Statement

### Current Limitations

The existing LIDAR tracking system uses 4x4 transformation matrices with identity transform defaults. This creates several limitations:

1. **Static Sensor Assumption**
   - Cannot handle moving sensors (vehicle-mounted LIDAR)
   - No ego-motion compensation in tracking
   - Velocity estimates include sensor motion

2. **2D Tracking Only**
   - Kalman filter tracks only (X, Y, VX, VY)
   - No height changes tracked
   - No object orientation (only velocity heading)

3. **No Calibration Update Support**
   - World coordinates baked in
   - Cannot re-transform if calibration changes
   - Must re-collect data after calibration update

4. **Incompatible with AV Ecosystem**
   - AV systems use quaternion-based poses (ROS standard)
   - Cannot integrate with autonomous vehicle platforms
   - No pose interpolation (smooth sensor motion)

### Why 7DOF?

**7DOF = 7 Degrees of Freedom:**
- Position: (x, y, z) - 3 DOF
- Orientation: (qw, qx, qy, qz) - 4 DOF quaternion

**Benefits:**
- ✅ Compact representation (7 values vs 16 for 4x4 matrix)
- ✅ Avoids gimbal lock (quaternions don't have singularities)
- ✅ Efficient interpolation (SLERP for smooth motion)
- ✅ Standard in robotics (ROS, autonomous vehicles)
- ✅ Enables ego-motion compensation (moving sensors)
- ✅ Supports 3D tracking with orientation

---

## Solution Overview

### Migration Strategy: Incremental Approach

**7 PRs over 5-8 weeks:**

1. **PR #1: Database Schema** (~200 lines, Week 1-2)
   - Add sensor_poses_7dof table
   - Migration for existing data

2. **PR #2: Quaternion Utilities** (~400 lines, Week 1-2)
   - Quaternion math library
   - Pose7DOF struct

3. **PR #3: Pose-Aware Clusters** (~300 lines, Week 3)
   - Add pose_id to clusters
   - Store sensor + world coords

4. **PR #4: 3D Tracking** (~600 lines, Week 4-5)
   - 13-state Kalman filter
   - Quaternion state handling

5. **PR #5: Ego-Motion** (~400 lines, Week 6)
   - Moving sensor support
   - Velocity de-biasing

6. **PR #6: REST APIs** (~500 lines, Week 7)
   - /api/lidar/poses endpoints
   - 3D track queries

7. **PR #7: Production** (~300 lines, Week 8)
   - Feature flags
   - PCAP tool updates

**Total:** ~2,700 lines across 7 PRs

---

## Key Benefits by Phase

### After Foundational PRs (1-2)
- ✅ Better calibration representation
- ✅ Quaternion infrastructure available
- ✅ Can store sensor velocities

### After Data Layer PR (3)
- ✅ Calibration updates without re-collection
- ✅ Historical data can be re-transformed
- ✅ Moving sensor measurements properly associated

### After Tracking PRs (4-5)
- ✅ Full 3D object tracking
- ✅ Vehicle-mounted LIDAR supported
- ✅ Ready for AV integration

### After Integration PRs (6-7)
- ✅ 7DOF data accessible via API
- ✅ Production-ready deployment
- ✅ Optional feature (backward compatible)

---

## Timeline

### Sequential Development (1 Developer)
```
Week 1-2:  PR #1 (Schema) + PR #2 (Quaternions)
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
           Dev B: PR #3 (Clusters) - depends on #2
Week 3-4:  Dev A: PR #4 (3D Tracking) - depends on #2
           Dev B: PR #6 (REST APIs) - depends on #3, #4
Week 5:    Dev A: PR #5 (Ego-Motion) - depends on #4
           Dev B: PR #7 (Production) - depends on #4, #5
Week 6:    Integration testing

Total: 5-6 weeks
```

---

## Success Criteria

### Technical
- ✅ All existing tests pass throughout migration
- ✅ 3D tracking accuracy >= 2D tracking accuracy
- ✅ No performance regression
- ✅ Calibration updates work correctly
- ✅ Moving sensor scenarios tracked correctly

### Process
- ✅ Each PR merged within 1 week
- ✅ No blocking merge conflicts
- ✅ Documentation complete per phase
- ✅ Rollback tested

### Product
- ✅ AV integration ready
- ✅ Users can update calibration without re-collection
- ✅ Moving sensor use cases enabled
- ✅ Better classification using 3D features

---

## Risk Mitigation

### Backward Compatibility
- ✅ All existing APIs unchanged
- ✅ 2D tracker remains default
- ✅ 3D tracking opt-in via feature flag

### Performance
- ✅ Benchmark tests before/after
- ✅ PCAP-based validation
- ✅ Monitor frame processing time

### Data Migration
- ✅ Tested on production-scale databases
- ✅ Can run offline (no downtime)
- ✅ Rollback scripts provided

---

## Related Documentation

### Current System Documentation
- `foreground_tracking_plan.md` - Current tracking implementation
- `lidar_sidecar_overview.md` - System architecture overview
- `schema.sql` - Database schema
- `lidar-tracking-integration.md` - Integration status

### Implementation Guides
- `ml_pipeline_roadmap.md` - ML classification roadmap
- `velocity-coherent-foreground-extraction.md` - Advanced tracking

### Architecture Documents
- `ARCHITECTURE.md` (repository root) - System design
- `README.md` (repository root) - Project overview

---

## Next Steps

### For Stakeholders
1. Review `EXECUTIVE-SUMMARY-7DOF.md`
2. Approve timeline and resource allocation
3. Answer stakeholder questions (see summary doc)

### For Engineers
1. Read `pr-split-recommendation.md`
2. Review `7dof-schema-compatibility-analysis.md`
3. Create GitHub issues for 7 PRs
4. Begin PR #1 implementation

### For Code Reviewers
1. Understand PR sequence
2. Review exit criteria per phase
3. Allocate review time (1-3 hours per PR)

---

## Questions?

**Technical Questions:** See `7dof-schema-compatibility-analysis.md` Appendices

**Implementation Questions:** See `pr-split-recommendation.md` detailed breakdown

**High-Level Questions:** See `EXECUTIVE-SUMMARY-7DOF.md`

---

**Status:** ✅ Planning Complete - Ready for Implementation  
**Next Action:** Create GitHub issues and begin PR #1
