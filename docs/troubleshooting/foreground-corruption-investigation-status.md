# Foreground Track Export & Streaming Investigation Status

**Last Updated:** January 3, 2026
**Status:** Active Investigation
**Components:** `cmd/radar`, `internal/lidar/network`, `internal/lidar/foreground.go`

## Executive Summary

We have successfully established a foreground point stream on UDP port 2370 and enabled frame export. However, the data quality is poor ("glitchy", "corrupt", "missing points").

**Current Findings:**

1.  **Network Transport**: ✅ Working. Packets are flowing to port 2370.
2.  **Packet Encoding**: ✅ Fixed. `foreground_forwarder.go` now uses azimuth-based bucketing and correct distance encoding (0.5cm resolution), resolving previous packet corruption issues.
3.  **Foreground Extraction**: ❌ **CRITICAL ISSUE**. The background subtraction model is extremely aggressive, classifying almost all valid foreground points as background. This is due to a configuration error in `cmd/radar/radar.go`.

---

## 1. Current System State

### 1.1 Network & Encoding (Fixed)

The `ForegroundForwarder` has been updated to resolve packet corruption:

- **Azimuth Bucketing**: Points are now distributed into 10 blocks based on azimuth range (36° per block) rather than relying on `BlockID`.
- **Distance Encoding**: Updated to use 0.5cm resolution (`distance * 200.0`) matching Pandar40P spec.
- **Clamping**: Distances > 200m are clamped to `0xFFFE`.

### 1.2 Background Subtraction (Broken)

The "glitchy" results and missing points are caused by incorrect default parameters in `cmd/radar/radar.go`.

**The Smoking Gun:**

```go
// cmd/radar/radar.go
lidarBgNoiseRelative = flag.Float64("lidar-bg-noise-relative", 0.315, "...")
```

- **Current Value**: `0.315` (31.5%)
- **Impact**: At 10m range, the noise allowance is 3.15m. The rejection threshold becomes `3.0 * 3.15m ≈ 9.5m`.
- **Result**: Any object within ~9.5m of the background is classified as background. This effectively erases all cars and pedestrians.

**Recommended Value**: `0.01` (1%) or `0.02` (2%).

---

## 2. Investigation & Fix Plan

### Phase 1: Parameter Correction (Immediate)

**Goal**: Fix the background subtraction model to allow foreground points to pass through.

1.  **Update `cmd/radar/radar.go`**:

    - Change default `lidar-bg-noise-relative` from `0.315` to `0.01`.
    - Verify `ClosenessSensitivityMultiplier` (currently 3.0, consider lowering to 2.0).
    - Verify `NeighborConfirmationCount` (currently 3, consider raising to 5).

2.  **Verify Fix**:
    - Run `make dev-go` (or `make build-radar-local`).
    - Observe logs for "Foreground extraction: X/Y points".
    - Expectation: Foreground ratio should rise from ~0-1% to ~10-40% for traffic scenes.

### Phase 2: Validation & Tuning

**Goal**: Fine-tune the parameters for optimal clean output.

1.  **Live Feed Verification**:

    - Connect LidarView to port 2370.
    - Verify points are clustered around moving objects (cars/pedestrians).
    - Verify points are NOT scattered randomly (noise).

2.  **ASC Export Verification**:
    - Export a sequence of frames.
    - Load into CloudCompare.
    - Verify point density and alignment.

### Phase 3: Advanced Debugging (If Issues Persist)

If parameter tuning does not fully resolve the "glitchy" look:

1.  **Log Thresholds**:

    - Add temporary logging in `internal/lidar/foreground.go` to print `closenessThreshold` vs `cellDiff` for a sample of points.

    ```go
    if i % 1000 == 0 {
        log.Printf("Debug: dist=%.2f diff=%.2f thresh=%.2f isBg=%v", p.Distance, cellDiff, closenessThreshold, isBackgroundLike)
    }
    ```

2.  **Check Coordinate Transform**:
    - Verify that `p.Azimuth` and `p.Distance` in `ProcessFramePolarWithMask` match the raw packet values.

---

## 3. Reference: Configuration Parameters

| Parameter                        | Current (Bugged)  | Recommended   | Description                                                                                 |
| :------------------------------- | :---------------- | :------------ | :------------------------------------------------------------------------------------------ |
| `NoiseRelativeFraction`          | **0.315** (31.5%) | **0.01** (1%) | Fraction of distance considered as sensor noise.                                            |
| `ClosenessSensitivityMultiplier` | 3.0               | 2.0           | Multiplier for threshold. Lower = more sensitive (more foreground).                         |
| `NeighborConfirmationCount`      | 3                 | 5             | Neighbors needed to confirm background. Higher = harder to be background (more foreground). |
| `SafetyMarginMeters`             | 0.5               | 0.2           | Fixed margin added to threshold.                                                            |

## 4. How to Test

### 4.1 Check Foreground Ratio

Look for this log line in the terminal:

```
[Foreground] Extracted 150/400 points (37.5%)
```

- **< 1%**: Background model too aggressive (Current Issue).
- **> 90%**: Background model not seeded or too lenient.
- **10-40%**: Healthy range for traffic.

### 4.2 Network Capture

Verify packets on port 2370:

```bash
sudo tcpdump -i lo0 -n udp port 2370 -c 10
```
