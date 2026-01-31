# LiDAR Foreground Tracking Status

**Last Updated:** January 13, 2026
**Status:** Operational — Core Pipeline Working

## Current State

**Working Features:**

| Feature                     | Status     | Notes                                            |
| --------------------------- | ---------- | ------------------------------------------------ |
| Foreground Feed (Port 2370) | ✅ Working | Foreground points visible in LidarView           |
| Real-time Parameter Tuning  | ✅ Working | Edit params via JSON textarea without restart    |
| Background Subtraction      | ✅ Working | Points correctly masked as foreground/background |
| Warmup Sensitivity Scaling  | ✅ Working | Eliminates initialisation trails                 |
| PCAP Analysis Mode          | ✅ Working | Grid preserved for analysis workflows            |

**Implementation Status:**

- ✅ Phase 3.7 (Analysis Run Infrastructure): Complete
- ✅ Port 2370 Foreground Streaming: Working
- ✅ Warmup Trail Artifacts: Resolved

## Resolved Issues

### Issue 1: Packet Corruption on Port 2370 — ✅ FIXED

**Symptom:** LidarView showed sparse rings and patchy arcs.
**Root Cause:** Forwarder reconstructed packets with incorrect azimuth values.
**Fix:** Rewrote `ForegroundForwarder` to preserve `RawBlockAzimuth` and `UDPSequence`.

### Issue 2: Foreground "Trails" After Object Pass — ✅ FIXED

**Symptom:** Points lingered as foreground for ~30 seconds after grid reset or object transit.
**Root Cause:** Two separate issues identified and fixed:

1. **Warmup Variance:** New cells underestimated true variance, causing false positives during initialisation.

   - **Fix:** Warmup sensitivity scaling in `ProcessFramePolarWithMask()` — threshold multiplied by decaying factor (4x → 1x over 100 observations).

2. **recFg Accumulation During Freeze:** Frozen cells incremented `RecentForegroundCount` on every observation, reaching 70+ by freeze end.
   - **Fix:** Don't increment recFg during freeze; reset to 0 on thaw with 1ms grace period.

**Implementation:** See [warmup-trails-fix-20260113.md](../troubleshooting/warmup-trails-fix-20260113.md)

### Issue 3: Real-time Parameter Tuning — ✅ IMPLEMENTED

**Feature:** JSON textarea in status page allows editing background params without restart.
**Implementation:** POST to `/api/lidar/params` with JSON body; changes apply immediately.

## Known Limitations

### Performance on M1 Mac

CPU usage during foreground processing is higher than expected. Not yet investigated.

**Potential Causes:**

- Per-frame allocations causing GC pressure
- Lock contention on background grid access
- Packet encoding overhead in ForegroundForwarder

**Mitigation:** Use `go tool pprof` to identify hot functions when investigating.

## Configuration Reference

### Background Parameters

| Parameter                        | Default | Description                                |
| -------------------------------- | ------- | ------------------------------------------ |
| `BackgroundUpdateFraction`       | 0.02    | EMA alpha for background learning          |
| `ClosenessSensitivityMultiplier` | 3.0     | Threshold multiplier for classification    |
| `SafetyMarginMeters`             | 0.1     | Fixed margin added to threshold            |
| `NoiseRelativeFraction`          | 0.01    | Distance-proportional noise allowance      |
| `NeighborConfirmationCount`      | 3       | Neighbours needed to confirm background    |
| `FreezeDurationNanos`            | 5e9     | Cell freeze duration after large deviation |
| `SeedFromFirstObservation`       | true    | Initialise cells from first observation    |

### Warmup Sensitivity

Cells with `TimesSeenCount < 100` have their threshold multiplied by:

```
warmupMultiplier = 1.0 + 3.0 * (100 - count) / 100
```

- Count 0: 4.0x threshold
- Count 50: 2.5x threshold
- Count 100+: 1.0x threshold (normal)

## API Endpoints

| Endpoint                 | Method   | Description                       |
| ------------------------ | -------- | --------------------------------- |
| `/api/lidar/status`      | GET      | Current pipeline status           |
| `/api/lidar/params`      | GET/POST | View/update background parameters |
| `/api/lidar/grid_status` | GET      | Background grid statistics        |
| `/api/lidar/grid_reset`  | GET      | Reset background grid             |
| `/api/lidar/pcap/start`  | POST     | Start PCAP replay                 |
| `/api/lidar/pcap/stop`   | POST     | Stop PCAP replay                  |
| `/api/lidar/data_source` | GET      | Current data source (live/pcap)   |

## Future Enhancements

See [ML Pipeline Roadmap](../roadmap/ml_pipeline_roadmap.md) for Phase 4.0+ features:

- Track labeling UI
- ML classifier training
- Parameter optimisation with grid search
