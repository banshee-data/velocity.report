# Warmup Trails Investigation - January 13, 2026

**Status:** Resolved
**Issue:** "Trails" appearing in foreground feed for ~30 seconds after grid reset or startup.
**Root Cause:** Background model initialization variance.

## Problem Description

User observed "trails" (false positive foreground points) appearing behind vehicles or on walls for approximately 30 seconds after resetting the background grid. These artifacts disappeared once the grid stabilized.

## Technical Analysis

1.  **Zero-Init State:** When a cell is reset (`TimesSeenCount=0`), its `AverageRangeMeters` is initialized to the first observed distance, and `RangeSpreadMeters` (variance) is set to 0.0.
2.  **Learning Latency:** The exponential moving average (EMA) with `alpha=0.02` takes approximately 50 observations to converge to the true spread of the background surface.
3.  **False Positives:** During this ~50-frame window, the model underestimates the natural variance of the surface. A 10cm deviation (common noise) exceeds the calculated low threshold, triggering a foreground classification.
4.  **Trails:** These false positives manifest as "trails" or static noise points until the spread model converges.

## Solution: Warmup Sensitivity Scaling

Implemented a dynamic multiplier for the `closenessThreshold` based on cell confidence (`TimesSeenCount`).

```go
warmupMultiplier := 1.0
if cell.TimesSeenCount < 100 {
    // Linear decay from 4.0x at count=0 to 1.0x at count=100
    warmupMultiplier = 1.0 + 3.0*float64(100-cell.TimesSeenCount)/100.0
}
closenessThreshold := closenessMultiplier*(spread + noise*dist + 0.01)*warmupMultiplier + safety
```

### Effect

- **Count 0-10:** Threshold ~400% of normal. Rejects initialization noise while still detecting vehicles (which typically have >1000% deviation).
- **Count 50:** Threshold ~250% of normal.
- **Count 100+:** Threshold 100% (normal operation).

This suppresses false positives during the learning phase while maintaining vehicle detection capability, effectively eliminating the warmup trails.
