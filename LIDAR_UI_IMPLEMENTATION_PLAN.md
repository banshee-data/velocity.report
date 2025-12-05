# LiDAR Track Visualization UI - Implementation Plan

**Date:** December 5, 2025  
**Status:** Planning Phase  
**Version:** 1.0

---

## Executive Summary

This document provides a comprehensive analysis and implementation plan for adding track visualization capabilities to the velocity.report web interface. The system has completed Phases 1-3.7 of LiDAR implementation (UDP ingestion through analysis run infrastructure) and is ready for Phase 4.0: Track Visualization UI.

**Key Goals:**
1. Add a new `/lidar/tracks` tab to visualize real-time and historical track data
2. Implement a two-pane view: top-down map visualization + timeline playback
3. Enable live streaming of track updates with efficient memory management
4. Support historical playback of recorded tracks

**Current State:**
- ✅ Complete tracking pipeline (background subtraction → clustering → Kalman tracking → classification)
- ✅ REST API endpoints for tracks, clusters, and observations
- ✅ Database schema with proper foreign keys and indexes
- ✅ Classification system (pedestrian, car, bird, other)
- 📋 **Missing:** Frontend UI for visualization and interaction

---

## Table of Contents

1. [Current Implementation Analysis](#1-current-implementation-analysis)
2. [SQL Schema Analysis & Recommendations](#2-sql-schema-analysis--recommendations)
3. [UI Component Architecture](#3-ui-component-architecture)
4. [Communication Flow & Long-Lived Connections](#4-communication-flow--long-lived-connections)
5. [Memory Management Strategy](#5-memory-management-strategy)
6. [Implementation Roadmap](#6-implementation-roadmap)
7. [Testing Strategy](#7-testing-strategy)
8. [Performance Considerations](#8-performance-considerations)

---

## 1. Current Implementation Analysis

### 1.1 Go Backend - Tracking Pipeline (✅ Complete)

**Location:** `internal/lidar/`

The tracking pipeline is fully implemented with the following components:

#### Data Flow
```
UDP Packets → Frame Builder → Background Classification (Polar) → Foreground Extraction
    ↓
Transform to World Frame (Cartesian)
    ↓
DBSCAN Clustering → Kalman Tracking → Classification → Database Persistence
    ↓
REST API Endpoints
```

#### Key Components

**Foreground Extraction** (`foreground.go`)
- `ProcessFramePolarWithMask()`: Per-point foreground/background classification in polar coordinates
- `ExtractForegroundPoints()`: Filters foreground points from mask
- Returns frame-level metrics (total, foreground, background counts)

**Transform** (`clustering.go`)
- `TransformToWorld()`: Converts polar points to world-frame Cartesian coordinates
- Currently uses identity transform (sensor frame = world frame)
- Ready for future pose-based transformations

**Clustering** (`clustering.go`)
- `DBSCAN()`: Density-based spatial clustering with configurable eps (0.6m) and minPts (12)
- `SpatialIndex`: Grid-based indexing for O(1) neighbor queries using Szudzik pairing
- `computeClusterMetrics()`: Centroid, bounding box, height P95, intensity mean

**Tracking** (`tracking.go`)
- `Tracker`: Multi-object tracker with Kalman filter (constant velocity model)
- Track lifecycle: `Tentative → Confirmed → Deleted`
- Mahalanobis distance gating for cluster-to-track association
- Speed statistics: average, peak, history for percentiles (P50/P85/P95)
- Configurable parameters: MaxTracks (100), MaxMisses (3), HitsToConfirm (3)

**Classification** (`classification.go`)
- Rule-based classifier for object types: pedestrian, car, bird, other
- Features: height, length, width, speed, duration, observation count
- Confidence scoring based on feature match quality
- Ready for future ML model integration

**Track Store** (`track_store.go`)
- `InsertCluster()`: Persist DBSCAN clusters
- `InsertTrack()`: Create new track record
- `UpdateTrack()`: Update track state, features, classification
- `InsertTrackObservation()`: Record per-observation trajectory data
- `GetActiveTracks()`: Query tracks by sensor and state
- `GetTrackObservations()`: Retrieve trajectory for visualization

### 1.2 REST API Endpoints (✅ Complete)

**Location:** `internal/lidar/monitor/track_api.go`

All necessary API endpoints are implemented:

#### Track Endpoints
- `GET /api/lidar/tracks` - List tracks with optional state filter
- `GET /api/lidar/tracks/active` - Active tracks (real-time from memory or DB)
- `GET /api/lidar/tracks/{track_id}` - Get specific track details
- `PUT /api/lidar/tracks/{track_id}` - Update track metadata (class, confidence)
- `GET /api/lidar/tracks/{track_id}/observations` - Get track trajectory
- `GET /api/lidar/tracks/summary` - Aggregated statistics by class/state

#### Cluster Endpoints
- `GET /api/lidar/clusters` - Recent clusters by time range

### 1.3 Web Frontend Structure (Current)

**Location:** `web/`

The existing web frontend uses:
- **Framework:** SvelteKit with TypeScript
- **UI Library:** svelte-ux (Button, Card, Grid, Header, etc.)
- **Charting:** layerchart (Chart, Axis, Spline, Svg)
- **Styling:** Tailwind CSS v4
- **Date Handling:** date-fns
- **Scales:** d3-scale

**Current Routes:**
- `/` - Main dashboard with radar statistics
- `/settings` - Configuration settings
- `/site` - Site management
- `/site/[id]` - Site-specific views

**API Client:** `src/lib/api.ts` provides typed API functions

---

## 2. SQL Schema Analysis & Recommendations

### 2.1 Current Schema (✅ Well-Designed)

The database schema is already well-structured with proper foreign keys:

#### `lidar_tracks` Table
```sql
CREATE TABLE lidar_tracks (
    track_id TEXT PRIMARY KEY,
    sensor_id TEXT NOT NULL,
    world_frame TEXT NOT NULL,
    track_state TEXT NOT NULL,  -- 'tentative', 'confirmed', 'deleted'
    start_unix_nanos INTEGER NOT NULL,
    end_unix_nanos INTEGER,
    observation_count INTEGER,
    -- Speed statistics
    avg_speed_mps REAL,
    peak_speed_mps REAL,
    p50_speed_mps REAL,  -- Median
    p85_speed_mps REAL,  -- 85th percentile
    p95_speed_mps REAL,  -- 95th percentile
    -- Bounding box averages
    bounding_box_length_avg REAL,
    bounding_box_width_avg REAL,
    bounding_box_height_avg REAL,
    height_p95_max REAL,
    intensity_mean_avg REAL,
    -- Classification
    object_class TEXT,           -- 'pedestrian', 'car', 'bird', 'other'
    object_confidence REAL,
    classification_model TEXT
);

-- Indexes for efficient queries
CREATE INDEX idx_lidar_tracks_sensor ON lidar_tracks(sensor_id);
CREATE INDEX idx_lidar_tracks_state ON lidar_tracks(track_state);
CREATE INDEX idx_lidar_tracks_time ON lidar_tracks(start_unix_nanos, end_unix_nanos);
CREATE INDEX idx_lidar_tracks_class ON lidar_tracks(object_class);
```

#### `lidar_track_obs` Table (Trajectory Data)
```sql
CREATE TABLE lidar_track_obs (
    track_id TEXT NOT NULL,
    ts_unix_nanos INTEGER NOT NULL,
    world_frame TEXT NOT NULL,
    -- Position (meters)
    x REAL,
    y REAL,
    z REAL,
    -- Velocity (m/s)
    velocity_x REAL,
    velocity_y REAL,
    speed_mps REAL,
    heading_rad REAL,
    -- Shape
    bounding_box_length REAL,
    bounding_box_width REAL,
    bounding_box_height REAL,
    height_p95 REAL,
    intensity_mean REAL,
    PRIMARY KEY (track_id, ts_unix_nanos),
    FOREIGN KEY (track_id) REFERENCES lidar_tracks(track_id) ON DELETE CASCADE
);

CREATE INDEX idx_lidar_track_obs_track ON lidar_track_obs(track_id);
CREATE INDEX idx_lidar_track_obs_time ON lidar_track_obs(ts_unix_nanos);
```

### 2.2 Schema Assessment: ✅ Excellent

**Strengths:**
1. ✅ **Proper Foreign Keys:** `lidar_track_obs` has `FOREIGN KEY (track_id) REFERENCES lidar_tracks(track_id) ON DELETE CASCADE`
2. ✅ **Appropriate Indexes:** All critical query paths are indexed
3. ✅ **Composite Primary Key:** `(track_id, ts_unix_nanos)` in `lidar_track_obs` enables efficient trajectory queries
4. ✅ **Time-Based Indexes:** Both `start_unix_nanos, end_unix_nanos` composite index and single timestamp indexes
5. ✅ **Normalization:** Clean separation between track metadata and observations
6. ✅ **Classification Support:** Fields for ML model evolution (`classification_model`)
7. ✅ **Speed Percentiles:** Pre-computed P50/P85/P95 for efficient queries

**Recommendation:** Keep current schema as-is. It is excellent and ready for UI implementation.

---

## 3. UI Component Architecture

### 3.1 Route Structure

Add new routes to `web/src/routes/`:

```
web/src/routes/
├── lidar/
│   ├── +layout.svelte          # Shared layout for lidar pages
│   ├── +page.svelte            # Main lidar dashboard
│   └── tracks/
│       ├── +page.svelte        # Track browser list view (two-pane visualization)
│       ├── [track_id]/
│       │   └── +page.svelte    # Individual track detail view
│       └── live/
│           └── +page.svelte    # Live track visualization
```

### 3.2 Component Hierarchy

#### Main Components

**`TrackVisualizationPage.svelte`** (Two-Pane Layout)
```svelte
<div class="h-screen flex flex-col">
  <!-- Top Pane: Map Visualization (60% height) -->
  <div class="flex-[3] border-b">
    <MapPane 
      tracks={$trackStore.activeTracks}
      currentTime={selectedTime}
      backgroundGrid={$trackStore.backgroundGrid}
    />
  </div>
  
  <!-- Bottom Pane: Timeline (40% height) -->
  <div class="flex-[2]">
    <TimelinePane 
      tracks={$trackStore.allTracks}
      currentTime={selectedTime}
      onTimeChange={(t) => selectedTime = t}
      onPlaybackChange={(playing, speed) => { ... }}
    />
  </div>
</div>
```

#### Key Components

**1. MapPane.svelte** - Top-Down View
- Canvas-based rendering for performance (60fps at 100 tracks)
- Background grid overlay (optional visualization)
- Track rendering with bounding boxes
- Trail visualization (last N positions)
- Color coding by classification
- Zoom and pan controls
- Hover tooltips with track details

**2. TimelinePane.svelte** - Temporal Visualization
- Horizontal timeline with playback controls
- Track lifecycle bars (start → end)
- Scrubber for time navigation
- Speed percentile visualization
- Classification transitions
- Play/Pause/Speed controls
- Jump to interesting events

**3. TrackList.svelte** - Track Browser
- Filterable list (by class, state, speed)
- Sort by start time, duration, speed
- Virtual scrolling for 100+ tracks
- Click to focus track on map

**4. TrackDetails.svelte** - Detail Panel
- Track metadata (ID, class, confidence)
- Kinematic statistics (speed percentiles, avg velocity)
- Bounding box dimensions
- Trajectory plot (X-Y, speed over time)

### 3.3 Data Structures (TypeScript)

```typescript
export interface Track {
  track_id: string;
  sensor_id: string;
  state: 'tentative' | 'confirmed' | 'deleted';
  position: { x: number; y: number; z: number };
  velocity: { vx: number; vy: number };
  speed_mps: number;
  heading_rad: number;
  object_class?: 'pedestrian' | 'car' | 'bird' | 'other';
  object_confidence?: number;
  observation_count: number;
  age_seconds: number;
  avg_speed_mps: number;
  peak_speed_mps: number;
  bounding_box: {
    length_avg: number;
    width_avg: number;
    height_avg: number;
  };
  first_seen: string;
  last_seen: string;
}

export interface TrackObservation {
  track_id: string;
  timestamp: string;
  position: { x: number; y: number; z: number };
  velocity: { vx: number; vy: number };
  speed_mps: number;
  heading_rad: number;
  bounding_box: {
    length: number;
    width: number;
    height: number;
  };
}
```

---

## 4. Communication Flow & Long-Lived Connections

### 4.1 Recommended Approach: Server-Sent Events (SSE)

**Pros:**
- Built into browsers (EventSource API)
- Automatic reconnection
- Simpler than WebSocket for one-way data flow
- Works through proxies/firewalls
- No additional Go libraries needed

**Implementation:**

Go Backend:
```go
func (api *TrackAPI) handleTrackSSE(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    
    ticker := time.NewTicker(100 * time.Millisecond)  // 10Hz
    defer ticker.Stop()
    
    for {
        select {
        case <-r.Context().Done():
            return
        case <-ticker.C:
            tracks := api.tracker.GetActiveTracks()
            data, _ := json.Marshal(tracks)
            fmt.Fprintf(w, "data: %s\n\n", data)
            flusher.Flush()
        }
    }
}
```

Frontend:
```typescript
export function createTrackSSE(sensorId: string) {
  const eventSource = new EventSource(
    `/api/lidar/tracks/stream?sensor_id=${sensorId}`
  );
  
  eventSource.onmessage = (event) => {
    const tracks: Track[] = JSON.parse(event.data);
    trackStore.update(tracks);
  };
  
  return { close: () => eventSource.close() };
}
```

### 4.2 Update Frequency & Batching

**Strategy:**
- **Live Mode:** 10Hz update rate (every 100ms)
- **Playback Mode:** 20-30Hz for smooth animation
- **Batch Updates:** Send all active tracks in single JSON payload

**Payload Size Estimation:**
- Track object: ~300 bytes JSON
- 100 active tracks: ~30KB per update
- 10Hz: 300KB/s (manageable for modern connections)

### 4.3 New API Endpoints Needed

1. **SSE Stream for Live Updates**
```
GET /api/lidar/tracks/stream?sensor_id={sensor_id}&state={state}
Content-Type: text/event-stream
```

2. **Historical Track Query**
```
GET /api/lidar/tracks/history?sensor_id={sensor_id}&start_time={unix_nanos}&end_time={unix_nanos}
Response: { tracks: Track[], observations: { [track_id]: TrackObservation[] } }
```

3. **Spatial Query (Bounding Box)** - Optional
```
GET /api/lidar/tracks/spatial?sensor_id={sensor_id}&min_x={x}&max_x={x}&min_y={y}&max_y={y}
```

---

## 5. Memory Management Strategy

### 5.1 Frontend Memory Management

**Key Strategies:**

1. **Circular Buffer for Track History**
```typescript
class TrackHistoryBuffer {
  private maxSize = 1000;  // Keep last 1000 observations per track
  private buffer: Map<string, CircularBuffer<TrackObservation>>;
  
  add(trackId: string, observation: TrackObservation) {
    if (!this.buffer.has(trackId)) {
      this.buffer.set(trackId, new CircularBuffer(this.maxSize));
    }
    const buf = this.buffer.get(trackId)!;
    buf.push(observation);
    
    // Remove old tracks
    if (buf.isOld(10 * 60 * 1000)) {  // 10 minutes
      this.buffer.delete(trackId);
    }
  }
}
```

2. **Track Cleanup**
```typescript
// Automatically remove deleted tracks after grace period
function cleanupDeletedTracks(tracks: Map<string, Track>) {
  const now = Date.now();
  const gracePeriodMs = 5000;  // 5 seconds
  
  for (const [trackId, track] of tracks.entries()) {
    if (track.state === 'deleted') {
      const timeSinceDeletion = now - new Date(track.last_seen).getTime();
      if (timeSinceDeletion > gracePeriodMs) {
        tracks.delete(trackId);
      }
    }
  }
}
```

3. **Canvas Rendering Optimization**
```typescript
// Reuse canvas objects, avoid creating new objects per frame
class TrackRenderer {
  private cachedPaths = new Map<string, Path2D>();
  
  render(tracks: Track[]) {
    for (const track of tracks) {
      // Reuse Path2D objects
      let path = this.cachedPaths.get(track.track_id);
      if (!path || this.needsUpdate(track)) {
        path = this.createBoundingBoxPath(track);
        this.cachedPaths.set(track.track_id, path);
      }
      this.ctx.stroke(path);
    }
  }
}
```

4. **Virtual Scrolling**
- Only render visible tracks in list view
- Lazy load track observations on demand

### 5.2 Backend Memory Management

**Current Implementation (Already Efficient):**
- Track Limit: `MaxTracks = 100` prevents unbounded growth
- Deleted Track Cleanup: 5-second grace period
- Speed History Limit: 100 observations
- Database Persistence: Old tracks moved to SQLite

**Additional Recommendations:**

1. **SSE Connection Limit** (10 concurrent connections)
2. **Rate Limiting** for historical queries (24-hour max range)

### 5.3 Memory Leak Detection

**Testing Strategy:**

1. Chrome DevTools Memory Profiler
   - Heap snapshots before/after 10 minutes
   - Look for growing object counts

2. Performance Monitoring
```typescript
if (import.meta.env.DEV) {
  setInterval(() => {
    if (performance.memory) {
      console.log({
        usedJSHeapSize: performance.memory.usedJSHeapSize,
        totalJSHeapSize: performance.memory.totalJSHeapSize
      });
    }
  }, 10000);
}
```

3. Automated Memory Leak Tests

---

## 6. Implementation Roadmap

### Phase 1: Foundation (Week 1-2)

**Goal:** Basic infrastructure and static visualization

**Tasks:**
1. Create route structure (`/lidar/tracks`)
2. Add TypeScript type definitions
3. Implement API client functions
4. Create basic `TrackList.svelte` component
5. Implement `MapPane.svelte` with static rendering
6. Add unit tests

**Deliverables:**
- Static track visualization from API data
- Track list with filtering
- Basic map view with zoom/pan

### Phase 2: Timeline & Playback (Week 3-4)

**Goal:** Temporal visualization and historical playback

**Tasks:**
1. Implement `TimelinePane.svelte`
2. Add playback controls (Play/Pause, speed adjustment)
3. Integrate timeline with map view
4. Implement `TrackDetails.svelte` panel
5. Add trajectory visualization

**Deliverables:**
- Working playback of historical tracks
- Synchronized map + timeline views
- Track detail panel with statistics

### Phase 3: Live Streaming (Week 5-6)

**Goal:** Real-time track updates with SSE

**Tasks:**
1. Implement SSE endpoint in Go
2. Create `trackStore.ts` Svelte store
3. Add live/playback mode toggle
4. Implement track interpolation for smooth animation
5. Test memory management (1+ hour)

**Deliverables:**
- Live track streaming at 10Hz
- Smooth track updates in map view
- Memory-efficient track history management

### Phase 4: Polish & Optimization (Week 7-8)

**Goal:** Performance optimization and UX improvements

**Tasks:**
1. Optimize canvas rendering
2. Add visual enhancements (trails, speed coloring)
3. Implement advanced filtering
4. Add export functionality (CSV)
5. Comprehensive testing

**Deliverables:**
- Polished UI with smooth animations
- Advanced filtering and export features
- Comprehensive test coverage
- 60fps at 100 tracks validated

### Phase 5: Documentation & Deployment (Week 9)

**Goal:** Production-ready release

**Tasks:**
1. Write user documentation
2. Create developer guide
3. Update ARCHITECTURE.md
4. Deploy to staging
5. Performance testing under production load
6. Security review

**Deliverables:**
- Complete documentation
- Production deployment
- Demo materials

---

## 7. Testing Strategy

### 7.1 Unit Tests
- Type definitions and API client
- Store logic and state management
- Rendering components (isolated)

### 7.2 Integration Tests
- API communication
- SSE connection and updates
- Component interactions

### 7.3 End-to-End Tests
- View live tracks workflow
- Playback historical data
- Filter and export functionality

### 7.4 Performance Tests
- Rendering performance (60fps target)
- Memory usage over time
- Network bandwidth consumption

---

## 8. Performance Considerations

### 8.1 Frontend Performance

**Target Metrics:**
- **Initial Load:** <2 seconds to first render
- **Frame Rate:** 60fps during live updates
- **Memory Growth:** <10MB/hour continuous operation
- **Network Bandwidth:** <500KB/s for live streaming

**Optimization Techniques:**
1. Canvas rendering with `requestAnimationFrame`
2. Object pooling for Path2D
3. Off-screen canvas for static elements
4. Virtual scrolling for track list
5. Debounced resize/zoom/pan events

### 8.2 Backend Performance

**Already Optimized:**
- In-memory tracker
- Prepared SQL statements
- Batch database writes

**Additional Optimizations:**
- SSE payload compression (optional gzip)
- Connection pooling
- Query result caching (5-second TTL)

### 8.3 Database Query Optimization

**Already Implemented:**
- Indexes on all critical paths
- Composite indexes for time queries
- Foreign key constraints

**Recommendations:**
- Cursor-based pagination for large queries
- Query result caching (optional)

---

## Appendix A: Technology Stack Summary

### Backend (Go)
- Framework: Standard library `net/http`
- Database: SQLite with WAL mode
- Streaming: Server-Sent Events (SSE)

### Frontend (TypeScript/Svelte)
- Framework: SvelteKit
- UI Library: svelte-ux
- Charting: layerchart (timeline), Canvas 2D API (map)
- Styling: Tailwind CSS v4

---

## Appendix B: Key Decisions & Rationale

### SSE vs WebSocket
**Chosen:** SSE

**Rationale:**
- One-way data flow sufficient
- Built-in browser support
- Automatic reconnection
- Simpler implementation

### Canvas vs SVG
**Chosen:** Canvas 2D API

**Rationale:**
- Better performance for dynamic content
- Lower memory overhead
- 60fps at 100 tracks achievable

### Current Schema
**Chosen:** Keep as-is

**Rationale:**
- Proper foreign keys
- Appropriate indexes
- Clean normalization
- Optional enhancements can be added later

---

## Conclusion

This plan provides a comprehensive roadmap for implementing track visualization in the velocity.report web interface.

**Key Strengths:**
- ✅ Complete tracking pipeline
- ✅ Well-designed database schema
- ✅ Comprehensive REST API
- ✅ Solid foundation for UI

**Estimated Timeline:** 9 weeks
**Estimated Effort:** 1 full-time developer

**Dependencies:**
- No new Go libraries required
- No new npm packages required
- No database migrations needed

This plan is ready for review and implementation.
