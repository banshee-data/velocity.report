# Foreground Track Export Investigation & Fix Plan

## Problem Statement

**Issue 1**: Foreground ASC export shows corrupt/misaligned points not matching expected LiDAR geometry
**Issue 2**: `export_frame_sequence` REST endpoint exports 5 frames out of order (appears to be sequence: 2,1,3,5,6,4 with missing frame 6)
**Context**: MapPane shows valid tracks aligned with vehicles visible in LidarView, indicating tracking pipeline works correctly

## Goal

Successfully isolate and export point clouds for individual tracks in a format suitable for ML classifier training, with correct frame ordering and point alignment.

## Root Cause Analysis

### Issue 1: Corrupt Foreground Points in ASC Export

#### Hypothesis A: Azimuth Bucket Assignment Bug
**Location**: `internal/lidar/network/foreground_forwarder.go:166-190`

**Problem**: Azimuth-based bucket assignment may have off-by-one or wrap-around errors at azimuth boundaries (0°/360°).

**Evidence**:
- Sorting by azimuth (line 159) groups points but bucket assignment may misplace boundary points
- Azimuth modulo operation `math.Mod(p.Azimuth+360.0, 360.0)` at line 176 normalizes but may not handle negative values correctly
- Bucket boundaries: `[0-36°, 36-72°, ..., 324-360°]` - points at exact boundaries may fall into wrong bucket

**Test**:
```go
// Check if points near 0°/360° boundary are correctly bucketed
testPoints := []PointPolar{
    {Azimuth: 359.5, Channel: 0}, // Should be in bucket 9
    {Azimuth: 0.5, Channel: 0},   // Should be in bucket 0
    {Azimuth: 359.9, Channel: 0}, // Edge case
}
```

**Fix Priority**: HIGH
**Fix Approach**: 
1. Add explicit boundary handling with epsilon tolerance
2. Validate bucket assignment logic at 0°/360° boundary
3. Add logging for bucket distribution (how many points per bucket)

#### Hypothesis B: Channel Assignment Mismatch
**Location**: `internal/lidar/network/foreground_forwarder.go:194-210`

**Problem**: Channel matching logic (line 198: `if p.Channel == ch`) may fail if foreground points have incorrect channel numbers.

**Evidence**:
- Foreground extraction from BackgroundManager may not preserve original channel IDs
- Points may have channel=-1 or out-of-range values (>40)
- Inner loop searches for matching channel but may find none, leaving distance=0xFFFF

**Test**:
```go
// Log channel distribution in foreground points
channelCounts := make(map[int]int)
for _, p := range foregroundPoints {
    channelCounts[p.Channel]++
    if p.Channel < 0 || p.Channel >= 40 {
        log.Printf("WARNING: Invalid channel %d", p.Channel)
    }
}
```

**Fix Priority**: HIGH
**Fix Approach**:
1. Add channel validation in foreground extraction pipeline
2. Log invalid channel warnings before encoding
3. Consider using elevation-based channel assignment if channels are invalid

#### Hypothesis C: Distance Encoding Overflow
**Location**: `internal/lidar/network/foreground_forwarder.go:200-208`

**Problem**: Distance clamping logic may still allow overflow for edge cases.

**Evidence**:
- Clamping at `MAX_DISTANCE_METERS = 130.0` (line 142)
- Encoding: `uint16(d*500)` should cap at 65,000 (130m)
- But if d > 130 slips through, wraps to small value

**Test**:
```go
// Check for distance overflow
for _, p := range foregroundPoints {
    if p.Distance > 130.0 {
        log.Printf("WARNING: Distance overflow %.2fm", p.Distance)
    }
}
```

**Fix Priority**: MEDIUM
**Fix Approach**: Add explicit clamping before uint16 cast

#### Hypothesis D: Coordinate Transform Bug
**Location**: Unknown - need to trace foreground extraction path

**Problem**: Points may be in wrong coordinate frame (Cartesian vs Polar) or have incorrect elevation/azimuth.

**Evidence**:
- MapPane shows correct track positions → tracking uses Cartesian correctly
- ASC export shows misalignment → export may use different coordinate system
- Need to verify: Does foreground extraction preserve original polar coordinates or reconstruct them?

**Investigation Required**:
1. Trace `ProcessFramePolarWithMask()` → `ExtractForegroundPoints()` → `ForwardForeground()`
2. Verify PointPolar fields (Azimuth, Elevation, Distance, Channel) are preserved
3. Check if any coordinate conversions occur in pipeline

**Fix Priority**: HIGH (if confirmed)

### Issue 2: Out-of-Order Frame Export

#### Root Cause: Frame Buffer Map Iteration Order
**Location**: `internal/lidar/frame_builder.go:98`, cleanup logic (likely around line 400-500)

**Problem**: `frameBuffer map[string]*LiDARFrame` is a Go map, which has **non-deterministic iteration order**. When frames are finalized from the buffer, they may be processed in arbitrary order.

**Evidence**:
- User reports sequence: 2,1,3,5,6,4 (with no frame 6 exported)
- Only 5 frames exported when 5 were requested → correct count but wrong order
- Map iteration in Go is deliberately randomized to prevent code from relying on order

**Code Path**:
```go
// frame_builder.go - cleanup/finalization likely iterates map
for frameID, frame := range fb.frameBuffer {
    if shouldFinalize(frame) {
        fb.finalizeFrame(frame) // This calls OnFrameComplete
    }
}
```

**Consequences**:
- Frames complete in correct order (by timestamp)
- But stored in map by FrameID (string)
- Cleanup iterates map in random order
- Export batch index increments sequentially but processes frames randomly

**Test**:
```go
// Add logging in OnFrameComplete to trace actual export order
log.Printf("[FrameBuilder] OnFrameComplete frameID=%s timestamp=%d batchIdx=%d/%d", 
    frame.FrameID, frame.StartTimestamp.UnixNano(), fb.exportBatchIndex, len(fb.exportFrameBatch))
```

**Fix Priority**: HIGH
**Fix Approach**:
1. **Option A**: Sort frames by timestamp before finalization
   - Collect frames to finalize in slice
   - Sort by `StartTimestamp`
   - Process in order
   
2. **Option B**: Use ordered data structure
   - Replace `map[string]*LiDARFrame` with ordered map or sorted slice
   - Maintain insertion order or sort by timestamp
   
3. **Option C**: Queue export requests with timestamp
   - Store `(timestamp, path)` pairs in batch export queue
   - Match finalized frames to queue by timestamp proximity

**Recommended**: Option A (minimal change, preserves existing architecture)

```go
// Proposed fix in finalizeOldFrames() or cleanup logic
func (fb *FrameBuilder) finalizeOldFrames() {
    fb.mu.Lock()
    defer fb.mu.Unlock()
    
    // Collect frames to finalize
    toFinalize := make([]*LiDARFrame, 0)
    cutoff := time.Now().Add(-fb.bufferTimeout)
    
    for _, frame := range fb.frameBuffer {
        if frame.StartTimestamp.Before(cutoff) {
            toFinalize = append(toFinalize, frame)
        }
    }
    
    // Sort by timestamp to ensure deterministic order
    sort.Slice(toFinalize, func(i, j int) bool {
        return toFinalize[i].StartTimestamp.Before(toFinalize[j].StartTimestamp)
    })
    
    // Finalize in order
    for _, frame := range toFinalize {
        fb.finalizeFrameUnlocked(frame)
        delete(fb.frameBuffer, frame.FrameID)
    }
}
```

## Investigation Plan

### Phase 1: Diagnostic Logging (1-2 hours)

**Goal**: Collect detailed telemetry to confirm hypotheses

#### Step 1.1: Add Foreground Encoding Diagnostics
**File**: `internal/lidar/network/foreground_forwarder.go`

Add logging before packet encoding:
```go
func (f *ForegroundForwarder) encodePointsAsPackets(points []lidar.PointPolar) ([][]byte, error) {
    // NEW: Log point distribution
    channelCounts := make(map[int]int)
    azimuthBuckets := make([]int, 10) // 10 buckets
    invalidChannels := 0
    overflowDistance := 0
    
    for _, p := range points {
        // Channel validation
        if p.Channel < 0 || p.Channel >= 40 {
            invalidChannels++
        } else {
            channelCounts[p.Channel]++
        }
        
        // Distance validation
        if p.Distance > MAX_DISTANCE_METERS {
            overflowDistance++
        }
        
        // Azimuth bucket assignment (preview)
        azBucket := int(math.Mod(p.Azimuth+360.0, 360.0) / 36.0)
        if azBucket >= 0 && azBucket < 10 {
            azimuthBuckets[azBucket]++
        }
    }
    
    log.Printf("[ForegroundForwarder] Encoding %d points: channels=%d unique, invalid_ch=%d, dist_overflow=%d, az_buckets=%v",
        len(points), len(channelCounts), invalidChannels, overflowDistance, azimuthBuckets)
    
    // Continue with existing encoding...
}
```

#### Step 1.2: Add Frame Export Order Tracking
**File**: `internal/lidar/frame_builder.go`

Add timestamp logging in `OnFrameComplete`:
```go
func (fb *FrameBuilder) OnFrameComplete(frame *LiDARFrame) {
    fb.mu.Lock()
    defer fb.mu.Unlock()
    
    // NEW: Log frame completion order with batch export info
    if fb.exportBatchIndex < len(fb.exportFrameBatch) {
        log.Printf("[FrameBuilder] OnFrameComplete frameID=%s timestamp=%d batch=%d/%d path=%s",
            frame.FrameID, frame.StartTimestamp.UnixNano(), 
            fb.exportBatchIndex+1, len(fb.exportFrameBatch),
            fb.exportFrameBatch[fb.exportBatchIndex])
    }
    
    // Continue with existing logic...
}
```

#### Step 1.3: Add Foreground Extraction Verification
**File**: `internal/lidar/network/pcap_realtime.go` (or wherever `ExtractForegroundPoints` is called)

```go
// After extracting foreground points
foregroundPoints := ExtractForegroundPoints(polarPoints, foregroundMask)

// NEW: Validate extracted points
log.Printf("[Foreground] Extracted %d/%d points (%.1f%%) - verifying integrity",
    len(foregroundPoints), len(polarPoints), 
    100.0*float64(len(foregroundPoints))/float64(len(polarPoints)))

// Quick sanity checks
validPoints := 0
for _, p := range foregroundPoints {
    if p.Channel >= 0 && p.Channel < 40 && p.Distance > 0 && p.Distance < 200 {
        validPoints++
    }
}
log.Printf("[Foreground] Valid points: %d/%d (%.1f%%)",
    validPoints, len(foregroundPoints), 
    100.0*float64(validPoints)/float64(len(foregroundPoints)))
```

### Phase 2: Fix Frame Export Ordering (2-3 hours)

**Priority**: HIGH - Fixes Issue #2

#### Step 2.1: Locate Frame Finalization Logic
**File**: `internal/lidar/frame_builder.go`

Search for cleanup/finalization functions:
```bash
grep -n "finalizeOldFrames\|cleanupTimer\|for.*frameBuffer" internal/lidar/frame_builder.go
```

#### Step 2.2: Implement Sorted Finalization

**Target Function**: Likely `finalizeOldFrames()` or similar

**Changes**:
1. Collect frames to finalize in slice (instead of processing map directly)
2. Sort by `StartTimestamp` ascending
3. Process in sorted order
4. Update `exportBatchIndex` sequentially

**Code**:
```go
func (fb *FrameBuilder) finalizeOldFrames() {
    fb.mu.Lock()
    defer fb.mu.Unlock()
    
    if len(fb.frameBuffer) == 0 {
        return
    }
    
    cutoff := time.Now().Add(-fb.bufferTimeout)
    
    // Collect frames to finalize
    toFinalize := make([]*LiDARFrame, 0, len(fb.frameBuffer))
    for _, frame := range fb.frameBuffer {
        if frame.CompletionTimestamp.Before(cutoff) || shouldFinalizeNow(frame) {
            toFinalize = append(toFinalize, frame)
        }
    }
    
    if len(toFinalize) == 0 {
        return
    }
    
    // CRITICAL FIX: Sort by timestamp to ensure deterministic export order
    sort.Slice(toFinalize, func(i, j int) bool {
        return toFinalize[i].StartTimestamp.Before(toFinalize[j].StartTimestamp)
    })
    
    log.Printf("[FrameBuilder] Finalizing %d frames in timestamp order", len(toFinalize))
    
    // Process in sorted order
    for _, frame := range toFinalize {
        fb.OnFrameComplete(frame) // This handles batch export
        delete(fb.frameBuffer, frame.FrameID)
    }
}
```

#### Step 2.3: Test Frame Ordering

**Test Case**:
1. Start PCAP replay
2. Call `/api/lidar/export_frame_sequence?sensor_id=test`
3. Monitor logs for frame IDs and timestamps
4. Verify exported ASC filenames have sequential timestamps

**Expected**:
```
[FrameBuilder] OnFrameComplete frameID=frame_001 timestamp=1703001000 batch=1/5 path=.../frame_01.asc
[FrameBuilder] OnFrameComplete frameID=frame_002 timestamp=1703001100 batch=2/5 path=.../frame_02.asc
[FrameBuilder] OnFrameComplete frameID=frame_003 timestamp=1703001200 batch=3/5 path=.../frame_03.asc
[FrameBuilder] OnFrameComplete frameID=frame_004 timestamp=1703001300 batch=4/5 path=.../frame_04.asc
[FrameBuilder] OnFrameComplete frameID=frame_005 timestamp=1703001400 batch=5/5 path=.../frame_05.asc
```

### Phase 3: Fix Foreground Point Corruption (3-4 hours)

**Priority**: HIGH - Fixes Issue #1

#### Step 3.1: Fix Azimuth Bucket Boundary Handling

**File**: `internal/lidar/network/foreground_forwarder.go:166-190`

**Problem**: Points near 0°/360° boundary may fall into wrong bucket

**Fix**:
```go
// BEFORE (line 171-180):
minAz := float64(blockIdx) * azBucketSize
maxAz := minAz + azBucketSize
bucket := make([]lidar.PointPolar, 0)
for _, p := range packetPoints {
    az := math.Mod(p.Azimuth+360.0, 360.0)
    if az >= minAz && az < maxAz {
        bucket = append(bucket, p)
    }
}

// AFTER (with boundary fix):
minAz := float64(blockIdx) * azBucketSize
maxAz := minAz + azBucketSize
bucket := make([]lidar.PointPolar, 0)
const epsilon = 0.01 // Small tolerance for floating point comparison

for _, p := range packetPoints {
    // Normalize azimuth to [0, 360)
    az := p.Azimuth
    for az < 0 {
        az += 360.0
    }
    for az >= 360.0 {
        az -= 360.0
    }
    
    // Check if point falls in this bucket (with epsilon tolerance)
    inBucket := false
    if blockIdx == 9 {
        // Last bucket: handle wrap-around from 324° to 360°/0°
        inBucket = (az >= minAz-epsilon) || (az < (maxAz-360.0)+epsilon)
    } else {
        inBucket = (az >= minAz-epsilon && az < maxAz+epsilon)
    }
    
    if inBucket {
        bucket = append(bucket, p)
    }
}

// Log bucket distribution for debugging
if f.frameCount <= 5 || f.frameCount%100 == 0 {
    lidar.Debugf("[ForegroundForwarder] Block %d: azimuth [%.1f, %.1f) → %d points",
        blockIdx, minAz, maxAz, len(bucket))
}
```

#### Step 3.2: Add Channel Validation and Fallback

**File**: `internal/lidar/network/foreground_forwarder.go:194-210`

**Problem**: Points with invalid channels (< 0 or >= 40) cause encoding failures

**Fix**:
```go
// BEFORE (line 194-210):
for ch := 0; ch < CHANNELS_PER_BLOCK; ch++ {
    var distance uint16 = 0xFFFF
    var intensity uint8 = 0
    for _, p := range bucket {
        if p.Channel == ch {
            // encode distance/intensity
        }
    }
    // write to packet
}

// AFTER (with validation):
validPointsInBlock := 0
for ch := 0; ch < CHANNELS_PER_BLOCK; ch++ {
    var distance uint16 = 0xFFFF
    var intensity uint8 = 0
    
    for _, p := range bucket {
        // Validate channel before matching
        if p.Channel < 0 || p.Channel >= CHANNELS_PER_BLOCK {
            // Log first occurrence of invalid channel
            if validationErrors[p.Channel] == 0 {
                lidar.Debugf("[ForegroundForwarder] Invalid channel %d (valid range: 0-39)", p.Channel)
            }
            validationErrors[p.Channel]++
            continue
        }
        
        if p.Channel == ch {
            d := p.Distance
            // Existing encoding logic...
            validPointsInBlock++
            break // Found match for this channel
        }
    }
    
    // Write channel data
    binary.LittleEndian.PutUint16(packet[channelOffset+ch*3:], distance)
    packet[channelOffset+ch*3+2] = intensity
}

if validPointsInBlock > 0 {
    blockHasData = true
    filledBlocks++
}
```

#### Step 3.3: Add Empty Block Warning

**File**: `internal/lidar/network/foreground_forwarder.go:220-230` (after block loop)

**Purpose**: Detect packets with mostly empty blocks (indicates bucketing bug)

**Fix**:
```go
// After processing all blocks for this packet
if filledBlocks < 5 { // Less than 50% of blocks have data
    lidar.Debugf("[ForegroundForwarder] Warning: Sparse packet - only %d/10 blocks have data (points=%d)",
        filledBlocks, len(packetPoints))
}

// Log overall packet quality
packets = append(packets, packet)
if f.frameCount <= 5 || f.frameCount%100 == 0 {
    lidar.Debugf("[ForegroundForwarder] Packet %d: %d points → %d filled blocks (%.0f%% coverage)",
        len(packets), len(packetPoints), filledBlocks, 100.0*float64(filledBlocks)/float64(BLOCKS_PER_PACKET))
}
```

### Phase 4: Track-Specific Point Cloud Export (4-6 hours)

**Priority**: MEDIUM - Enables ML training workflow

**Goal**: Export point clouds for individual tracks (not just foreground frames)

#### Step 4.1: Design Track Point Cloud Cache

**New File**: `internal/lidar/track_point_cache.go`

**Purpose**: Associate foreground points with tracked objects during real-time processing

**Architecture**:
```go
// TrackPointCache stores point clouds for each active track
type TrackPointCache struct {
    mu            sync.RWMutex
    trackPoints   map[int][]PointPolar // trackID → accumulated points
    trackMetadata map[int]*TrackMetadata
    maxPointsPerTrack int // Memory limit
}

type TrackMetadata struct {
    TrackID        int
    Classification string
    StartTime      time.Time
    LastSeen       time.Time
    FrameCount     int
}

func (tpc *TrackPointCache) AddPointsForTrack(trackID int, points []PointPolar) {
    tpc.mu.Lock()
    defer tpc.mu.Unlock()
    
    if tpc.trackPoints[trackID] == nil {
        tpc.trackPoints[trackID] = make([]PointPolar, 0, 1000)
    }
    
    // Append points (with max limit)
    existing := tpc.trackPoints[trackID]
    if len(existing) + len(points) > tpc.maxPointsPerTrack {
        // Keep most recent points only
        excess := len(existing) + len(points) - tpc.maxPointsPerTrack
        existing = existing[excess:]
    }
    
    tpc.trackPoints[trackID] = append(existing, points...)
}

func (tpc *TrackPointCache) ExportTrackASC(trackID int, outPath string) error {
    tpc.mu.RLock()
    points := tpc.trackPoints[trackID]
    meta := tpc.trackMetadata[trackID]
    tpc.mu.RUnlock()
    
    if len(points) == 0 {
        return fmt.Errorf("no points for track %d", trackID)
    }
    
    // Export to ASC with metadata header
    return exportTrackPointsToASC(points, meta, outPath)
}
```

#### Step 4.2: Integrate with Clustering/Tracking Pipeline

**File**: `internal/lidar/clustering.go` or wherever clusters are assigned to tracks

**Integration Point**: After cluster-to-track assignment, associate cluster points with track

**Pseudocode**:
```go
// In clustering/tracking loop
for _, cluster := range clusters {
    trackID := assignClusterToTrack(cluster)
    
    if trackID > 0 {
        // Get original polar points for this cluster
        clusterPoints := getClusterPolarPoints(cluster)
        
        // Add to track point cache
        trackPointCache.AddPointsForTrack(trackID, clusterPoints)
        
        // Update metadata
        trackPointCache.UpdateTrackMetadata(trackID, TrackMetadata{
            TrackID: trackID,
            Classification: track.Classification,
            LastSeen: time.Now(),
        })
    }
}
```

#### Step 4.3: Add REST Endpoint for Track Export

**File**: `internal/lidar/monitor/webserver.go`

**New Endpoint**: `/api/lidar/export_track?track_id=<id>&sensor_id=<sensor>&out=<path>`

```go
func (ws *WebServer) handleExportTrackASC(w http.ResponseWriter, r *http.Request) {
    trackIDStr := r.URL.Query().Get("track_id")
    sensorID := r.URL.Query().Get("sensor_id")
    outPath := r.URL.Query().Get("out")
    
    if trackIDStr == "" || sensorID == "" {
        ws.writeJSONError(w, http.StatusBadRequest, "missing track_id or sensor_id")
        return
    }
    
    trackID, err := strconv.Atoi(trackIDStr)
    if err != nil {
        ws.writeJSONError(w, http.StatusBadRequest, "invalid track_id")
        return
    }
    
    // Get track point cache
    cache := lidar.GetTrackPointCache(sensorID)
    if cache == nil {
        ws.writeJSONError(w, http.StatusNotFound, "no track cache for sensor")
        return
    }
    
    // Generate output path if not provided
    if outPath == "" {
        outPath = filepath.Join(os.TempDir(), 
            fmt.Sprintf("track_%s_%d_%d.asc", sensorID, trackID, time.Now().Unix()))
    }
    
    // Validate path
    absPath, err := filepath.Abs(outPath)
    if err != nil || security.ValidateExportPath(absPath) != nil {
        ws.writeJSONError(w, http.StatusForbidden, "invalid output path")
        return
    }
    
    // Export track points
    if err := cache.ExportTrackASC(trackID, absPath); err != nil {
        ws.writeJSONError(w, http.StatusInternalServerError, 
            fmt.Sprintf("export error: %v", err))
        return
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{
        "status": "ok",
        "track_id": trackIDStr,
        "out": absPath,
    })
}
```

## Testing Strategy

### Test 1: Frame Export Ordering
**Goal**: Verify frames export in correct chronological order

**Steps**:
1. Start PCAP replay with known data
2. Call `/api/lidar/export_frame_sequence?sensor_id=test`
3. Examine exported ASC files
4. Load each in LidarView and verify temporal continuity

**Success Criteria**:
- 5 files named `frame_*_01.asc` through `frame_*_05.asc`
- Each file loads successfully in LidarView
- Frames show progressive motion (e.g., vehicle moving through scene)
- No spatial discontinuities between consecutive frames

### Test 2: Foreground Point Integrity
**Goal**: Verify foreground points encode correctly and match source data

**Steps**:
1. Capture foreground stream on port 2370 with tcpdump
2. Parse captured packets with existing parser
3. Compare decoded points to original foreground extraction
4. Load decoded points in LidarView

**Success Criteria**:
- Packets parse without errors (valid 0xFFEE preambles, no corrupt blocks)
- At least 50% of blocks per packet contain data
- Decoded points form recognizable object shapes in LidarView
- Azimuth distribution covers 360° (not clustered in one quadrant)
- Channel distribution matches sensor (40 channels, roughly even distribution)

### Test 3: Track Point Cloud Export
**Goal**: Verify track-specific export contains only points from that track

**Steps**:
1. Run PCAP analysis with tracking enabled
2. Identify high-quality track (MapPane shows clear trajectory)
3. Export track with `/api/lidar/export_track?track_id=<id>`
4. Load in LidarView and verify spatial coherence

**Success Criteria**:
- ASC file contains 500-5000 points (enough for ML training)
- Points form single coherent object trajectory
- No background clutter (ground, buildings, etc.)
- Temporal progression visible (object moves/rotates over time)
- Metadata includes classification, duration, quality score

## Implementation Priority

### Phase 1: Frame Ordering Fix (URGENT)
**Effort**: 2-3 hours
**Impact**: HIGH - Fixes immediate usability issue

**Deliverables**:
1. Sorted frame finalization in `frame_builder.go`
2. Logging to verify export order
3. Test case validating correct sequencing

### Phase 2: Foreground Encoding Fixes (HIGH)
**Effort**: 3-4 hours
**Impact**: HIGH - Fixes data corruption

**Deliverables**:
1. Azimuth bucket boundary fix with epsilon tolerance
2. Channel validation and fallback
3. Empty block detection and logging
4. Validation tests with tcpdump + parser roundtrip

### Phase 3: Diagnostic Logging (MEDIUM)
**Effort**: 1-2 hours
**Impact**: MEDIUM - Enables faster debugging of future issues

**Deliverables**:
1. Channel/azimuth distribution logging in foreground forwarder
2. Frame export order tracking in frame builder
3. Point validation logging in extraction pipeline

### Phase 4: Track Point Cloud Export (FUTURE)
**Effort**: 4-6 hours
**Impact**: MEDIUM - Enables ML training workflow but not blocking current issues

**Deliverables**:
1. TrackPointCache implementation
2. Integration with tracking pipeline
3. REST endpoint for track export
4. Test cases for track-specific export

## Success Metrics

### Short-term (Phases 1-2)
- ✅ Frame export produces 5 files in correct chronological order
- ✅ Foreground ASC files load in LidarView without errors
- ✅ Foreground points show recognizable object shapes (not random noise)
- ✅ Port 2370 packets parse correctly with existing parser
- ✅ &gt;50% of packet blocks contain data (not mostly empty)

### Medium-term (Phase 3)
- ✅ Logs clearly show point distribution (channels, azimuth, distance)
- ✅ Warnings trigger for validation failures (invalid channels, distance overflow)
- ✅ Frame export logs show monotonic timestamp progression
- ✅ Foreground ratio stabilizes at 15-40% (per diagnosis doc)

### Long-term (Phase 4)
- ✅ Track-specific exports contain 500-5000 points per track
- ✅ Exported tracks show spatial coherence (single object trajectory)
- ✅ Classification labels included in export metadata
- ✅ High-quality tracks (quality≥0.6) exportable for ML training

## Related Documentation

- **Corruption diagnosis**: `docs/troubleshooting/port-2370-corruption-diagnosis.md` (root causes for low foreground count)
- **Streaming setup**: `docs/troubleshooting/port-2370-foreground-streaming.md` (initialization checklist)
- **Enhancement plan**: `docs/features/lidar-tracking-enhancements.md` (Phase 2 training data preparation)

## Next Steps

1. **Review this plan** with stakeholders - validate priorities and approach
2. **Implement Phase 1** (frame ordering fix) - quick win, high impact
3. **Implement Phase 2** (foreground encoding fixes) - critical for data quality
4. **Test with real PCAP data** - validate fixes with actual sensor data
5. **Iterate on Phase 3** (diagnostics) - add logging as needed during debugging
6. **Design Phase 4** (track export) - finalize API design before implementation
