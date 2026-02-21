# Warmup Trails Fix — January 2026

Status: Historical
Purpose/Summary: warmup-trails-fix-20260113.

**Status:** Resolved
**Issue:** False positive foreground points ("trails") appearing for ~30 seconds after grid reset.

## Problem

When a cell is reset (`TimesSeenCount=0`), `RangeSpreadMeters` is initialised to 0. The EMA takes ~50-100 observations to learn the true variance. During this window, normal surface noise (10-20cm) exceeds the calculated threshold, causing false foreground classifications that appear as "trails" on walls and static surfaces.

## Solution

Implemented warmup sensitivity scaling in `ProcessFramePolarWithMask()`:

```go
warmupMultiplier := 1.0
if cell.TimesSeenCount < 100 {
    warmupMultiplier = 1.0 + 3.0*float64(100-cell.TimesSeenCount)/100.0
}
closenessThreshold := closenessMultiplier*(spread + noise*dist + 0.01)*warmupMultiplier + safety
```

**Effect:**

- Count 0-10: ~4x threshold (rejects initialisation noise)
- Count 50: ~2.5x threshold
- Count 100+: 1x threshold (normal operation)

Vehicles (typically >1m deviation) are still detected during warmup; only small noise is suppressed.

## Files Modified

- `internal/lidar/foreground.go` — Added warmup multiplier logic
- `internal/lidar/foreground_warmup_test.go` — Regression tests

## Related Fixes

The recFg accumulation bug (separate issue) was also fixed:

- Frozen cells no longer increment `RecentForegroundCount`
- recFg reset to 0 on thaw (with 1ms grace period)
