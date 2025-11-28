# LIDAR Tracking Plan V2 - Changes Summary

**Date:** November 22, 2025  
**Files:**
- Original: `docs/lidar-foreground-tracking-plan.md` (30KB, 1065 lines)
- Revised: `docs/lidar-foreground-tracking-plan-v2.md` (43KB, 1395 lines)

---

## Overview

Version 2 incorporates 20 architectural clarifications requested in code review, with primary focus on explicit polar/world frame separation.

---

## Changes by Category

### A. Background Grid & Foreground Extraction (Items 1-4)

**✅ Change 1: Keep background model purely polar**
- **Before:** Some ambiguity about coordinate systems
- **After:** Explicit statement that background grid operates only in (ring, azimuth, range)
- **Impact:** Section "Background Grid Infrastructure (Polar Frame)" added

**✅ Change 2: ProcessFramePolar returns mask**
- **Before:** Suggested returning `[]PointPolar` foreground points
- **After:** Returns `[]bool` mask, extraction happens outside lock
- **Impact:** New contract: `func ProcessFramePolar(points []PointPolar) ([]bool, error)`

**✅ Change 3: Extraction outside lock**
- **Before:** Foreground extraction within background lock
- **After:** Separate `extractForegroundPoints()` function called after lock release
- **Impact:** Better concurrency, reduced lock contention

**✅ Change 4: Per-frame metrics**
- **Before:** Only aggregate foreground/background counts
- **After:** Per-frame metrics: total, foreground, background, fraction
- **Impact:** New `FrameMetrics` struct, `/api/lidar/frame_metrics` endpoint

### B. Polar → World Transform Stage (Items 5-7)

**✅ Change 5: Explicit transform stage**
- **Before:** Transform was implicit in clustering section
- **After:** Dedicated "Phase 3.0: Polar → World Transform" section
- **Impact:** Clear pipeline stage between foreground extraction and clustering

**✅ Change 6: Transform scope clarified**
- **Before:** Unclear what transform stage does/doesn't do
- **After:** Explicit responsibilities: polar→sensor→world, attach metadata; does NOT update background or cluster
- **Impact:** `TransformToWorld()` function with clear boundaries

**✅ Change 7: Downstream = world only**
- **Before:** Not explicitly stated
- **After:** "Downstream modules (DBSCAN, tracking, SQL, APIs, UI) operate exclusively on world-frame coordinates"
- **Impact:** Architecture diagram showing coordinate system boundary

### C. Clustering (World Frame) (Items 8-10)

**✅ Change 8: World-frame input**
- **Before:** Some wording suggested polar clustering
- **After:** "DBSCAN runs on foreground points in world-frame (x, y [, z]) coordinates"
- **Impact:** All clustering code uses `WorldPoint` type

**✅ Change 9: Required spatial index**
- **Before:** "Optional grid index"
- **After:** "Required spatial index" with full implementation
- **Impact:** `SpatialIndex` struct with grid-based acceleration, O(n log n) complexity

**✅ Change 10: Dimensionality choice**
- **Before:** Unclear if 2D or 3D clustering
- **After:** "2D (x, y) clustering, with z used only for features"
- **Impact:** Simplified spatial index (3×3 cells vs 27), clear design rationale

### D. Tracking (Items 11-13)

**✅ Change 11: Explicit world-frame state**
- **Before:** Implied but not stated
- **After:** "Track state is maintained in world (x, y, vx, vy) coordinates"
- **Impact:** `TrackState2D` struct explicitly documented

**✅ Change 12: Track lifecycle states**
- **Before:** Mentioned birth/death but no formal states
- **After:** `Tentative → Confirmed → Deleted` with promotion/deletion rules
- **Impact:** `TrackState` enum, state machine diagram, transition logic

**✅ Change 13: Gating distance definition**
- **Before:** Mentioned Mahalanobis but unclear threshold
- **After:** "Mahalanobis distance in world coords, threshold on squared distance (25.0 = 5.0²)"
- **Impact:** `mahalanobisDistanceSquared()` function, explicit threshold tuning

### E. Schema, APIs & Classification (Items 14-17)

**✅ Change 14: Schema = world frame only**
- **Before:** Not explicitly stated
- **After:** "All tables store world-frame coordinates only. Polar coordinates...never persisted"
- **Impact:** Schema documentation emphasizes world-frame storage

**✅ Change 15: Classification fields**
- **Before:** Basic track schema
- **After:** Added `object_class`, `object_confidence`, `classification_model`, speed quantiles (p50, p85, p95)
- **Impact:** Enhanced `lidar_tracks` table schema

**✅ Change 16: Classification phase**
- **Before:** No classification phase
- **After:** "Phase 3.4: Track-level classification" with features and logic
- **Impact:** New phase with `TrackClassifier`, rule-based classification

**✅ Change 17: Summary endpoints**
- **Before:** Basic track endpoints
- **After:** `/api/lidar/tracks/active`, `/api/lidar/tracks/summary` (aggregated by class)
- **Impact:** API section expanded with aggregation endpoints

### F. Concurrency, Performance & Tests (Items 18-20)

**✅ Change 18: Locking boundaries**
- **Before:** General mention of thread safety
- **After:** "Background lock covers only polar classification. Transform, clustering, tracking, DB writes occur outside lock"
- **Impact:** "Performance & Concurrency" section with explicit boundaries

**✅ Change 19: Latency budget**
- **Before:** Overall <100ms target
- **After:** Per-stage targets: background (5ms), transform (3ms), clustering (30ms), tracking (10ms), DB (5ms)
- **Impact:** Table with stage-by-stage breakdown for profiling

**✅ Change 20: Test plan for split**
- **Before:** General testing strategy
- **After:** Separate tests for polar masks, transform accuracy, world-frame clustering/tracking
- **Impact:** 5 test categories with code examples

---

## Key Additions

### 1. Architecture Diagram
Visual representation of polar vs world frame boundary:
```
[POLAR FRAME: Background, Classification]
          ↓ Phase 3.0: Transform
[WORLD FRAME: Clustering, Tracking, DB, API, UI]
```

### 2. New Phase 3.0
Dedicated section for coordinate transformation:
- Input: `[]PointPolar` + Pose
- Output: `[]WorldPoint`
- Implementation with 4×4 homogeneous matrix

### 3. New Phase 3.4
Track classification with:
- Feature extraction (spatial, kinematic, temporal)
- Rule-based classifier
- Future ML model hook

### 4. Spatial Index Implementation
Full implementation of required grid-based spatial index:
- Cell size ≈ eps (0.6m)
- O(1) cell lookup
- 3×3 neighbor query for 2D

### 5. Track Lifecycle State Machine
Formal state transitions:
- Tentative (birth)
- Confirmed (after N hits)
- Deleted (after MaxMisses)

---

## Migration Guide

**For existing code:**
1. Update `ProcessFramePolar` to return `[]bool` mask
2. Add `extractForegroundPoints()` function
3. Implement `TransformToWorld()` stage
4. Update DBSCAN to use spatial index
5. Add track state field to Track struct
6. Migrate schema to add classification fields

**For tests:**
1. Add polar mask validation tests
2. Add transform accuracy tests
3. Ensure clustering tests use world points only
4. Add track lifecycle tests

---

## Benefits of V2

1. **Clearer Architecture:** Explicit coordinate system boundaries
2. **Better Performance:** Lock-free foreground extraction, required spatial index
3. **Enhanced Tracking:** Formal lifecycle, proper gating
4. **Classification:** Object type labeling capability
5. **Better Testing:** Separate test strategies for polar vs world
6. **Production Ready:** Per-stage latency targets for profiling

---

**Status:** V2 Complete - Ready for Implementation  
**Recommendation:** Use V2 as canonical implementation guide
