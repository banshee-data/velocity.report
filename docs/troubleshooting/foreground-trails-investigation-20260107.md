# Foreground Trails Investigation - January 7, 2026

**Status:** Active Investigation
**Log File:** `velocity-debug-20260107-115640.log.txt`
**Debug Region:** Ring 30-35, Azimuth 323°-340°

## Executive Summary

The user reports experiencing "trails" in the foreground data feed (port 2370) when a car passes and the background behind the car fails to settle. After deep analysis of the debug log, we've identified several key findings about the foreground extraction system behavior.

---

## Methodology

The debug log contains detailed per-cell foreground extraction telemetry with the format:
```
[FG_DEBUG] r=<ring> az=<azimuth> dist=<observed_distance> avg=<background_avg> spread=<variance> 
           diff=<abs_difference> thresh=<closeness_threshold> seen=<confidence_count> 
           recFg=<recent_foreground_count> frozen=<is_frozen> isBg=<is_background>
```

Key metrics analyzed:
- **dist**: Current LIDAR distance measurement
- **avg**: Background model's exponential moving average (EMA)
- **diff**: Absolute difference between dist and avg
- **thresh**: Closeness threshold for background classification
- **recFg**: Counter tracking recent foreground observations (for fast re-acquisition)
- **frozen**: Whether cell is frozen to prevent background corruption during occlusion

---

## Finding 1: Background Model Remains Stable During Vehicle Pass

**Observation:** Tracing cell r=35 az=324.4 through the vehicle pass event:

| Time | Event | dist | avg | diff | seen | recFg | Classification |
|------|-------|------|-----|------|------|-------|----------------|
| 11:58:06.509 | Pre-vehicle | 10.152 | 10.159 | 0.007 | 26 | 0 | Background ✓ |
| 11:58:06.644 | Vehicle enters | **8.776** | 10.159 | 1.383 | 25 | 1 | **Foreground** |
| 11:58:06.706 | Vehicle exits | 10.124 | **10.155** | 0.035 | 26 | 0 | Background ✓ |
| 11:58:06.790 | Vehicle re-enters | **8.224** | 10.155 | 1.931 | 25 | 1 | **Foreground** |
| ... (oscillates with moving vehicle) |
| 11:58:08.659 | Stable | 10.136 | 10.152 | 0.018 | 26 | 0 | Background ✓ |

**Conclusion:** The background average (avg) only moved from 10.159m to 10.152m (0.07m / 0.7%) during the entire event. **The fast re-acquisition mechanism is working correctly** for this cell.

---

## Finding 2: Freeze Mechanism Causes recFg Accumulation

**Observation:** Cell r=35 az=323.4 exhibits a different pattern:

| Time | Event | recFg | frozen | Classification |
|------|-------|-------|--------|----------------|
| 11:58:07.460 | Multiple foreground | 4 | false | Foreground |
| 11:58:07.532 | **Freeze triggered** | 5 | **true** | Foreground |
| 11:58:12.572 | After 5s freeze | **70** | false | Background |

**Root Cause:** During the 5-second freeze period, the cell continues incrementing `RecentForegroundCount` on each observation (see `foreground.go` lines 158-166):

```go
if cell.FrozenUntilUnixNanos > nowNanos {
    foregroundMask[i] = true
    foregroundCount++
    if cell.RecentForegroundCount < 65535 {
        cell.RecentForegroundCount++  // ← Accumulates during freeze
    }
    continue
}
```

**Impact:** After freeze ends, `recFg=70` means the fast re-acquisition boost is applied for 70 subsequent observations. While this doesn't cause incorrect classification, it does create unnecessary processing overhead.

**Recommendation:** Consider resetting `RecentForegroundCount` when freeze ends, or don't increment during freeze.

---

## Finding 3: Foreground Detections Are Actual Vehicle Observations

**Observation:** All `isBg=false` entries in the log correspond to actual vehicle observations:

- Last foreground detection: **11:58:08.206552**
- Log ends at: **11:58:19.726**
- Time with no foreground issues: **~11.5 seconds**

Every foreground classification in the debug window has:
- `dist=8.2-8.8m` (vehicle distance)
- `avg=10.1-10.2m` (true background distance)
- `diff=1.4-2.0m` (correctly exceeds threshold)

**Conclusion:** The system is **correctly** classifying vehicle observations as foreground. There are no "ghost trails" in this log segment.

---

## Finding 4: No Evidence of Background Corruption in Analyzed Cells

The background average values remain stable throughout:
- Ring 35 cells: avg ≈ 10.1-10.2m (wall/fence at ~10m)
- Ring 30 cells: avg ≈ 17.4-17.8m (distant background)

No cells show evidence of the avg drifting toward vehicle distances (8-9m).

---

## Hypotheses for "Trails" Symptom

If the user is seeing visual trails, possible causes outside the background model:

### Hypothesis A: Visualization Latency
The web frontend or LidarView may have rendering delays that show old foreground points.

### Hypothesis B: Track Persistence
The tracking pipeline (`tracking.go`) may keep tracks alive for several frames after object disappears:
- `tentative → confirmed → deleted` lifecycle
- Kalman filter prediction continues position estimation

### Hypothesis C: Different Azimuth Sector Issue
The debug log only covers azimuth 323°-340°. The "trails" may appear in a different sector not captured.

### Hypothesis D: Frozen Cell Visual Artifact
Frozen cells (5-second duration) are unconditionally classified as foreground. If many cells freeze simultaneously, this creates a "frozen foreground region" even after the vehicle leaves.

---

## Additional Debug Logging Recommendations

To further investigate, add the following diagnostics:

### 1. Log Freeze Events
```go
// In foreground.go, after line 285 (freeze trigger)
if cell.FrozenUntilUnixNanos > nowNanos {
    debugf("[FG_FREEZE] r=%d az=%.1f froze for %.1fs, cellDiff=%.3f, thresh=%.3f",
        ring, az, float64(freezeDur)/1e9, cellDiff, FreezeThresholdMultiplier*closenessThreshold)
}
```

### 2. Log Freeze End Transitions
```go
// Add after checking if cell was previously frozen but now expired
if wasPreviouslyFrozen && cell.FrozenUntilUnixNanos <= nowNanos {
    debugf("[FG_THAW] r=%d az=%.1f thawed, recFg=%d, avg=%.3f, dist=%.3f",
        ring, az, cell.RecentForegroundCount, cell.AverageRangeMeters, p.Distance)
}
```

### 3. Log Fast Re-acquisition Boosts
```go
// In foreground.go around line 245
if cell.RecentForegroundCount > 0 {
    debugf("[FG_REACQ] r=%d az=%.1f boost applied: base_alpha=%.4f, boost_alpha=%.4f, recFg=%d",
        ring, az, effectiveAlpha, updateAlpha, cell.RecentForegroundCount)
}
```

---

## Applied Fix: Reset recFg After Freeze

The following fix was applied to address the accumulated recFg during freeze:

```go
// In foreground.go, modified freeze handling block (lines 157-166)
if cell.FrozenUntilUnixNanos > nowNanos {
    foregroundMask[i] = true
    foregroundCount++
    // DON'T increment recFg during freeze - it was already high enough to trigger freeze
    // The freeze itself is sufficient protection against background corruption
    continue
}
```

Additionally, recFg is reset when freeze expires (with 1ms grace period to avoid false triggers when FreezeDurationNanos=0):

```go
// Check if freeze just expired (with grace period)
const thawGraceNanos = int64(1_000_000) // 1ms
if cell.FrozenUntilUnixNanos > 0 && cell.FrozenUntilUnixNanos+thawGraceNanos <= nowNanos {
    cell.RecentForegroundCount = 0
    cell.FrozenUntilUnixNanos = 0 // Clear the expired freeze timestamp
}
```

**Note:** Thaw detection only runs when a point observation hits the cell. If a cell's azimuth bin doesn't receive a point in the frame where freeze expires, the timestamp cleanup is deferred to the next observation.

---

## Recommended Next Steps

1. **Add freeze/thaw logging** to capture exactly when cells enter/exit frozen state
2. **Expand debug region** to cover full 360° for one complete vehicle pass
3. **Capture visualization** to see if the "trail" is in foreground points or track rendering
4. **Check tracking pipeline** for object persistence settings that might show ghost tracks
5. **Review freeze duration** (currently 5s) - may be too long for fast-moving vehicles

---

## Files Analyzed

- `internal/lidar/foreground.go` - Foreground extraction logic
- `internal/lidar/background.go` - Background model data structures
- `velocity-debug-20260107-115640.log.txt` - Debug telemetry

---

## Related Documentation

- [Foreground Corruption Investigation Status](foreground-corruption-investigation-status.md)
- [Port 2370 Foreground Streaming Troubleshooting](port-2370-foreground-streaming.md)
