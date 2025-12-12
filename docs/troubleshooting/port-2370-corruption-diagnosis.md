# Port 2370 Corruption & Low Foreground Point Diagnosis

## Problem Summary

**Issue 1**: Corrupt network stream on port 2370  
**Issue 2**: Very few points in foreground ASC export

## Root Cause Analysis

### Issue 1: Packet Corruption - Channel/BlockID Mapping Problem

**Location**: `internal/lidar/network/foreground_forwarder.go:148-192`

#### The Problem

The Pandar40P packet encoder has a **critical channel-to-block-to-azimuth mapping issue**:

```go
// Lines 148-192: encodePointsAsPackets()
for blockIdx := 0; blockIdx < BLOCKS_PER_PACKET; blockIdx++ {
    // PROBLEM 1: Azimuth selection logic is flawed
    var blockAzimuth float64
    azFound := false
    for _, p := range packetPoints {
        if p.BlockID == blockIdx {  // ← Assumption: BlockID matches packet block index
            blockAzimuth = p.Azimuth
            azFound = true
            break
        }
    }
    if !azFound && blockIdx < len(packetPoints) {
        blockAzimuth = packetPoints[blockIdx].Azimuth  // ← Fallback uses array index
    }
    
    // PROBLEM 2: Channel matching assumes BlockID correspondence
    for ch := 0; ch < CHANNELS_PER_BLOCK; ch++ {
        for _, p := range packetPoints {
            if p.Channel == ch && p.BlockID == blockIdx {  // ← Invalid assumption
                distance = uint16(p.Distance * 500)
                intensity = p.Intensity
                break
            }
        }
    }
}
```

#### Why This Corrupts Packets

1. **BlockID Mismatch**: `PointPolar.BlockID` represents the **original block** from the source packet (0-9). When foreground extraction filters points, the BlockID values are **preserved from the original scan**.

2. **Sparse Point Distribution**: After foreground extraction, you might have:
   - Point 1: BlockID=0, Channel=5, Azimuth=0.2°
   - Point 2: BlockID=3, Channel=12, Azimuth=10.5°
   - Point 3: BlockID=9, Channel=38, Azimuth=89.7°

3. **Encoding Failure**: The encoder tries to fill 10 blocks sequentially (blockIdx 0-9) but:
   - Block 0: Finds Point 1 (BlockID=0) ✓
   - Block 1: **No points with BlockID=1** → azimuth=0.0, all channels empty
   - Block 2: **No points with BlockID=2** → azimuth=0.0, all channels empty
   - ...
   - Block 9: Finds Point 3 (BlockID=9) ✓

4. **Result**: Packets have mostly empty blocks with azimuth=0.0 and no channel data, causing **corrupt/invalid LiDAR frames** that parsers reject or misinterpret.

#### The Fix

**Option A: Rebuild Blocks by Azimuth Range** (Recommended)
```go
// Sort points by azimuth
sort.Slice(packetPoints, func(i, j int) bool {
    return packetPoints[i].Azimuth < packetPoints[j].Azimuth
})

// Distribute points into 10 azimuth ranges
azimuthRange := 360.0 / float64(BLOCKS_PER_PACKET)
for blockIdx := 0; blockIdx < BLOCKS_PER_PACKET; blockIdx++ {
    minAz := float64(blockIdx) * azimuthRange
    maxAz := minAz + azimuthRange
    
    // Collect points in this azimuth range
    var blockPoints []PointPolar
    for _, p := range packetPoints {
        if p.Azimuth >= minAz && p.Azimuth < maxAz {
            blockPoints = append(blockPoints, p)
        }
    }
    
    // Use median azimuth for block
    blockAzimuth := minAz + azimuthRange/2
    if len(blockPoints) > 0 {
        blockAzimuth = blockPoints[len(blockPoints)/2].Azimuth
    }
    
    // Encode channels (assign points to nearest channel)
    for ch := 0; ch < CHANNELS_PER_BLOCK; ch++ {
        // Find point with matching or closest channel
        for _, p := range blockPoints {
            if p.Channel == ch {
                distance = uint16(p.Distance * 500)
                intensity = p.Intensity
                break
            }
        }
    }
}
```

**Option B: Ignore BlockID, Use Sequential Assignment**
```go
// Assign points to blocks round-robin
pointsPerBlock := (len(packetPoints) + BLOCKS_PER_PACKET - 1) / BLOCKS_PER_PACKET

for blockIdx := 0; blockIdx < BLOCKS_PER_PACKET; blockIdx++ {
    startIdx := blockIdx * pointsPerBlock
    endIdx := min(startIdx + pointsPerBlock, len(packetPoints))
    
    if startIdx >= len(packetPoints) {
        // No points for this block - fill with empty data
        continue
    }
    
    blockPoints := packetPoints[startIdx:endIdx]
    blockAzimuth := blockPoints[0].Azimuth  // Use first point's azimuth
    
    // Distribute block points across channels
    for i, p := range blockPoints {
        ch := i % CHANNELS_PER_BLOCK  // Sequential channel assignment
        distance = uint16(p.Distance * 500)
        intensity = p.Intensity
        // Encode at channel offset
    }
}
```

### Issue 2: Low Foreground Point Count

**Location**: `internal/lidar/foreground.go:18-185` (ProcessFramePolarWithMask)

#### The Problem

The background subtraction is **too aggressive**, classifying most points as background. This is caused by:

#### Root Cause 1: Default Parameters Too Conservative

```go
// internal/lidar/background.go:19-40
type BackgroundParams struct {
    BackgroundUpdateFraction       float32 // Default: 0.02 (very slow adaptation)
    ClosenessSensitivityMultiplier float32 // Default: 3.0 (very tight threshold)
    SafetyMarginMeters             float32 // Default: 0.5m
    NoiseRelativeFraction          float32 // Default: 0.01 (1% of range)
    NeighborConfirmationCount      int     // Default: 3 neighbors needed
}
```

**Impact**:
- `ClosenessSensitivityMultiplier=3.0` + `NoiseRelativeFraction=0.01` = Very narrow acceptance window
- At 10m distance: threshold = 3.0 × (0.01 × 10 + 0.01) = 3.0 × 0.11 = **0.33m**
- At 50m distance: threshold = 3.0 × (0.01 × 50 + 0.01) = 3.0 × 0.51 = **1.53m**
- Moving vehicles (0.5-2m offset from background) may fall within threshold at longer ranges

#### Root Cause 2: Neighbor Confirmation Too Lenient

```go
// foreground.go:109-123
neighborConfirmCount := 0
for deltaAz := -1; deltaAz <= 1; deltaAz++ {
    // Only checks +/- 1 azimuth bin on SAME ring
    // If ANY 2 neighbors match (neighConfirm=3), point is background
}

if cellDiff <= closenessThreshold || neighborConfirmCount >= neighConfirm {
    // Classified as BACKGROUND
}
```

**Issue**: The `||` operator means:
- Point is background if **either** it matches the cell **or** if 2 neighbors match
- This is too lenient - vehicles passing through often have 2+ neighbors also seeing the vehicle

#### Root Cause 3: Seed-From-First Not Enabled

```go
// foreground.go:134-136
if seedFromFirst && cell.TimesSeenCount == 0 {
    initIfEmpty = true  // Treat as background
}
```

**Issue**: When replaying PCAP files:
- No warmup period - background model is **completely empty**
- Empty cells default to **foreground** (line 78-81)
- BUT: Most cells stay empty because first observation is foreground → never seeded
- Result: Oscillation between too many foreground points (empty cells) and too few (after a few frames)

**Required Fix**: Set `SeedFromFirstObservation=true` for PCAP replay mode

#### Root Cause 4: Motor Speed Encoding Error

**Location**: `internal/lidar/network/foreground_forwarder.go:209`

```go
// Line 209: INCORRECT encoding
motorSpeedEncoded := uint16(math.Round(f.sensorConfig.MotorSpeedRPM * 100))
```

**Pandar40P Specification**: Motor speed field is in **0.01 RPM units**
- For 600 RPM motor: should encode as `600 * 100 = 60,000` (0x EA60)
- Current code encodes as `600 * 100 = 60,000` ✓ (accidentally correct!)

**However**: Line 138 in `track_export.go` is **WRONG**:
```go
// track_export.go:138 - INCORRECT (RPM * 60 instead of RPM * 100)
motorSpeedEncoded := uint16(config.MotorSpeedRPM * 60)  // ← Should be * 100
```

For 600 RPM:
- Current: `600 * 60 = 36,000` (0x8CA0) → Parser sees **360 RPM** motor
- Correct: `600 * 100 = 60,000` (0xEA60) → Parser sees **600 RPM** motor

**Impact**: Azimuth calculation errors in parser, causing point cloud timing/alignment issues.

### Issue 3: Distance Encoding Overflow

**Location**: Both `foreground_forwarder.go:182` and `track_export.go:112`

```go
// Distance encoding: 2mm resolution
distance = uint16(p.Distance * 500)
```

**Problem**: Maximum representable distance with uint16:
- Max value: 65,535 (0xFFFF)
- Max distance: 65,535 / 500 = **131.07 meters**
- Points beyond 131m → **overflow** → wrap around to small values

**Pandar40P Spec**: 0xFFFF is the **no-return marker** (should not be used for valid distances)

**Fix**:
```go
// Clamp to max representable distance
const MAX_DISTANCE_M = 130.0  // Leave margin for no-return marker
if p.Distance > MAX_DISTANCE_M {
    distance = 0xFFFE  // Max valid distance (130.996m)
} else if p.Distance < 0.004 {
    distance = 0xFFFF  // No return
} else {
    distance = uint16(p.Distance * 500)
}
```

## Recommended Fixes Priority

### Priority 1: Fix Packet Corruption (Blocking Issue)

**File**: `internal/lidar/network/foreground_forwarder.go`

1. **Rewrite `encodePointsAsPackets()` to use azimuth-based block assignment** (Option A above)
2. **Add validation**: Log warning if >50% of blocks are empty
3. **Add packet integrity check**: Verify at least 3 blocks have data before sending

### Priority 2: Fix Parameter Tuning (Low Foreground Count)

**File**: Caller code (e.g., `cmd/tools/pcap-analyze/main.go`)

1. **Enable seed-from-first for PCAP replay**:
   ```go
   bgParams.SeedFromFirstObservation = true
   ```

2. **Relax sensitivity for moving object detection**:
   ```go
   bgParams.ClosenessSensitivityMultiplier = 2.0  // Was 3.0
   bgParams.NoiseRelativeFraction = 0.02         // Was 0.01
   bgParams.NeighborConfirmationCount = 5        // Was 3
   ```

3. **Add warmup period**: Process first 100 frames without forwarding to seed background model

### Priority 3: Fix Motor Speed Encoding

**File**: `internal/lidar/track_export.go:138`

```go
// Change from:
motorSpeedEncoded := uint16(config.MotorSpeedRPM * 60)
// To:
motorSpeedEncoded := uint16(math.Round(config.MotorSpeedRPM * 100))
```

### Priority 4: Fix Distance Overflow

**Files**: Both `foreground_forwarder.go` and `track_export.go`

Add distance clamping before encoding (see code above).

## Validation Tests

### Test 1: Packet Integrity
```bash
# Capture port 2370 traffic
sudo tcpdump -i any -n udp port 2370 -w /tmp/port2370.pcap -c 1000

# Verify packet structure
tcpdump -r /tmp/port2370.pcap -X | head -100
# Look for: 0xFFEE preambles every 124 bytes, non-zero azimuth values
```

**Expected**: At least 5-7 blocks per packet should have non-zero azimuth and channel data

### Test 2: Foreground Extraction Ratio
```bash
# Enable debug logging in pcap-realtime.go (line 187-191)
# Run PCAP replay and check logs

# Expected output:
# "Foreground extraction: 150/400 points (37.5%)"  ← Good
# "Foreground extraction: 5/400 points (1.25%)"    ← Too low (background too aggressive)
# "Foreground extraction: 395/400 points (98.75%)" ← Too high (background not seeded)
```

**Good ratio**: 15-40% foreground for typical street scene with vehicles

### Test 3: LidarView Load Test
```bash
# After fixes, try loading foreground PCAP in LidarView
# Should see:
# - Points distributed across azimuth range (not clustered at 0°)
# - Realistic elevations (not all in single plane)
# - Smooth point cloud (not flickering/jumping)
```

## Configuration Example (Post-Fix)

```go
// In pcap-analyze or replay tool
bgParams := lidar.BackgroundParams{
    BackgroundUpdateFraction:       0.02,  // EMA update rate
    ClosenessSensitivityMultiplier: 2.0,   // Relaxed from 3.0
    SafetyMarginMeters:             0.3,   // Reduced from 0.5m
    NoiseRelativeFraction:          0.02,  // Increased from 0.01
    FreezeDurationNanos:            5e9,   // 5 seconds
    NeighborConfirmationCount:      5,     // Increased from 3
    SeedFromFirstObservation:       true,  // CRITICAL for PCAP replay
}

// Initialize background manager
bgManager := lidar.NewBackgroundManager(bgParams, 40, 1800, "sensor/test")

// Warmup period: process first 100 frames to seed background
for i := 0; i < 100; i++ {
    packet := readNextPacket()
    points, _ := parser.ParsePacket(packet)
    bgManager.ProcessFramePolarWithMask(points)  // Seeds background, don't use mask yet
}

// Now start forwarding foreground
config := RealtimeReplayConfig{
    SpeedMultiplier:         1.0,
    PacketForwarder:         forwarder,
    ForegroundForwarder:     fgForwarder,
    BackgroundManager:       bgManager,  // Now warmed up
}
```

## Summary Checklist

- [ ] **Packet corruption fix**: Rewrite block assignment logic (azimuth-based)
- [ ] **Motor speed fix**: Change RPM×60 → RPM×100 in track_export.go
- [ ] **Distance overflow fix**: Add clamping for distances >130m
- [ ] **Parameter tuning**: Enable SeedFromFirstObservation + relax sensitivity
- [ ] **Warmup period**: Process 100 frames before forwarding
- [ ] **Validation**: Capture port 2370, verify packet structure
- [ ] **Validation**: Check foreground ratio logs (should be 15-40%)
- [ ] **Validation**: Load in LidarView, verify point cloud quality

## Expected Outcomes After Fixes

1. **Port 2370 packets**: Valid Pandar40P format, loadable in LidarView
2. **Foreground ratio**: 15-40% of points (instead of 1-5%)
3. **Point distribution**: Spread across azimuth range (not clustered at 0°)
4. **ASC export**: Hundreds to thousands of points per file (instead of <10)
5. **Visual quality**: Recognizable vehicle/pedestrian shapes in point clouds
