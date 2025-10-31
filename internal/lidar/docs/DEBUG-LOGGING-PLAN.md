# Debug Logging Plan for Grid Reset & Acceptance Rate Investigation

## Executive Summary

Two critical issues require instrumentation:

1. **Grid reset race**: `nonzero_cells` remains ~63k across parameter sweeps despite `grid_reset` calls
2. **Low acceptance rates**: Seeing ~98% max acceptance instead of expected 99%+

This plan outlines targeted logging to definitively diagnose both issues.

## Issue 1: Grid Reset Race Condition

### Symptoms from data

- `sweep-toggle-raw.csv` shows `nonzero_cells` hovering around 63k-65k across all parameter combinations
- Manual verification showed `grid_status` reported 58k+ background_count immediately after `grid_reset`
- DB snapshots contain 61k+ non_empty_cells even after reset

### Root cause hypothesis

Between `POST /api/lidar/grid_reset` and when bg-multisweep samples the grid (via snapshot or grid_status), incoming frames repopulate cells. The multisweep sequence is:

1. POST grid_reset (clears in-memory grid)
2. POST params (applies new params)
3. POST acceptance/reset (clears acceptance counters)
4. Sleep for settle_time (5s in current runs)
5. Sample acceptance/snapshot every 10s for 20 iterations

During step 4-5, frames continuously arrive and populate the grid, so by the first sample the grid already has ~63k cells.

### Required logging

#### A. In `ResetGrid()` — log the reset operation

**File**: `internal/lidar/background.go`
**Location**: Inside `BackgroundManager.ResetGrid()`

```go
func (bm *BackgroundManager) ResetGrid() error {
    if bm == nil || bm.Grid == nil {
        return fmt.Errorf("background manager or grid nil")
    }
    g := bm.Grid
    g.mu.Lock()
    defer g.mu.Unlock()

    // COUNT nonzero cells BEFORE reset
    nonzeroBefore := 0
    for i := range g.Cells {
        if g.Cells[i].AverageRangeMeters != 0 || g.Cells[i].RangeSpreadMeters != 0 || g.Cells[i].TimesSeenCount != 0 {
            nonzeroBefore++
        }
    }

    // Do the reset
    for i := range g.Cells {
        g.Cells[i].AverageRangeMeters = 0
        g.Cells[i].RangeSpreadMeters = 0
        g.Cells[i].TimesSeenCount = 0
        g.Cells[i].LastUpdateUnixNanos = 0
        g.Cells[i].FrozenUntilUnixNanos = 0
    }
    for i := range g.AcceptByRangeBuckets {
        g.AcceptByRangeBuckets[i] = 0
        g.RejectByRangeBuckets[i] = 0
    }
    g.ChangesSinceSnapshot = 0
    g.ForegroundCount = 0
    g.BackgroundCount = 0

    // COUNT nonzero cells AFTER reset (should be 0)
    nonzeroAfter := 0
    for i := range g.Cells {
        if g.Cells[i].AverageRangeMeters != 0 || g.Cells[i].RangeSpreadMeters != 0 || g.Cells[i].TimesSeenCount != 0 {
            nonzeroAfter++
        }
    }

    // LOG with timestamp and counts
    log.Printf("[ResetGrid] sensor=%s nonzero_before=%d nonzero_after=%d total_cells=%d timestamp=%d",
        g.SensorID, nonzeroBefore, nonzeroAfter, len(g.Cells), time.Now().UnixNano())

    return nil
}
```

**What this tells us**: Confirms reset actually clears the grid in-memory (nonzero_after should be 0).

#### B. In `ProcessFramePolar()` — log grid population after reset

**File**: `internal/lidar/background.go`
**Location**: End of `BackgroundManager.ProcessFramePolar()`

Add a rate-limited logger that fires every N frames (e.g., every 100th call) OR when a significant threshold is crossed:

```go
func (bm *BackgroundManager) ProcessFramePolar(points []PointPolar) {
    // ... existing logic ...

    // At the END, after releasing g.mu.Unlock(), add:

    // Rate-limited diagnostic: log grid population periodically
    // Use a simple counter on the manager struct
    bm.frameProcessCount++ // add this field to BackgroundManager

    if bm.frameProcessCount % 100 == 0 {
        // Quick snapshot of nonzero count (already computed inside the lock above if needed)
        g.mu.RLock()
        nonzero := 0
        for i := range g.Cells {
            if g.Cells[i].TimesSeenCount > 0 {
                nonzero++
            }
        }
        g.mu.RUnlock()

        log.Printf("[ProcessFramePolar] sensor=%s frames_processed=%d nonzero_cells=%d bg_count=%d fg_count=%d timestamp=%d",
            g.SensorID, bm.frameProcessCount, nonzero, backgroundCount, foregroundCount, time.Now().UnixNano())
    }
}
```

Add field to BackgroundManager struct:

```go
type BackgroundManager struct {
    Grid              *BackgroundGrid
    // ... existing fields ...
    frameProcessCount int64 // for rate-limited diagnostics
}
```

**What this tells us**: Shows how quickly the grid repopulates after a reset. If we see nonzero_cells jump from 0 to 60k+ within seconds, that confirms the race.

#### C. In monitor handler `handleGridReset` — log API call timing

**File**: `internal/lidar/monitor/webserver.go`
**Location**: Inside `handleGridReset()`

```go
func (ws *WebServer) handleGridReset(w http.ResponseWriter, r *http.Request) {
    // ... existing param validation ...

    beforeNanos := time.Now().UnixNano()

    if err := mgr.ResetGrid(); err != nil {
        ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("reset error: %v", err))
        return
    }

    afterNanos := time.Now().UnixNano()
    elapsedMs := float64(afterNanos - beforeNanos) / 1e6

    log.Printf("[API:grid_reset] sensor=%s reset_duration_ms=%.3f timestamp=%d",
        sensorID, elapsedMs, afterNanos)

    // ... existing response ...
}
```

**What this tells us**: Shows when reset API calls happen and how long they take (should be <1ms).

#### D. In `handleBackgroundParams` — log param changes with timing

**File**: `internal/lidar/monitor/webserver.go`
**Location**: Inside POST case of `handleBackgroundParams()`

```go
case http.MethodPost:
    // ... existing param decode/apply logic ...

    // After applying all params, add timing log
    timestamp := time.Now().UnixNano()
    cur := bm.GetParams()

    log.Printf("[API:params] sensor=%s noise_rel=%.6f closeness=%.3f neighbors=%d seed_from_first=%v timestamp=%d",
        sensorID, cur.NoiseRelativeFraction, cur.ClosenessSensitivityMultiplier,
        cur.NeighborConfirmationCount, cur.SeedFromFirstObservation, timestamp)

    // ... existing response ...
```

**What this tells us**: Correlates parameter changes with grid reset timing. We can see the exact sequence and timing of reset→params→acceptance_reset in the logs.

---

## Issue 2: Low Acceptance Rates

### Symptoms from data

- Seeing ~98% overall_accept_percent in many parameter combinations
- Expected 99%+ for low noise_relative values like 0.01
- Some combinations showing as low as 64% (neighbor=2 cases after reset)
- After settling, rates converge to ~99.8%+

### Root cause hypotheses

1. **Cold start rejection**: Empty grid (TimesSeenCount=0) causes initial rejections until cells populate
2. **Neighbor confirmation gating**: With neighbor=2, observations need 2 of 2 neighbors to confirm, which is strict
3. **NoiseRelativeFraction too tight**: 0.01 (1%) may be too strict for real sensor noise at longer ranges
4. **Seeding not working**: If SeedFromFirstObservation is false or not applied, empty cells reject everything initially

### Required logging

#### E. In `ProcessFramePolar()` — log acceptance decision details

**File**: `internal/lidar/background.go`
**Location**: Inside the per-cell update loop in `ProcessFramePolar()`

Add a diagnostic sample logger that logs details for a small sample of cells (e.g., first 10 rejections and first 10 acceptances per frame):

```go
func (bm *BackgroundManager) ProcessFramePolar(points []PointPolar) {
    // ... existing setup ...

    // Diagnostic counters
    var acceptSampleCount, rejectSampleCount int
    const maxSamplesPerType = 10

    g.mu.Lock()
    defer g.mu.Unlock()

    // ... read params under lock as before ...

    for ringIdx := 0; ringIdx < rings; ringIdx++ {
        for azBinIdx := 0; azBinIdx < azBins; azBinIdx++ {
            cellIdx := g.Idx(ringIdx, azBinIdx)
            if counts[cellIdx] == 0 {
                continue
            }

            observationMean := sums[cellIdx] / float64(counts[cellIdx])
            // ... existing cell update logic ...

            // AFTER deciding isBackgroundLike, log sample decisions
            if bm.EnableDiagnostics {
                logThis := false
                if isBackgroundLike && acceptSampleCount < maxSamplesPerType {
                    acceptSampleCount++
                    logThis = true
                } else if !isBackgroundLike && rejectSampleCount < maxSamplesPerType {
                    rejectSampleCount++
                    logThis = true
                }

                if logThis {
                    log.Printf("[ProcessFramePolar:decision] sensor=%s ring=%d azbin=%d obs_mean=%.3f cell_avg=%.3f cell_spread=%.3f times_seen=%d neighbor_confirm=%d closeness_threshold=%.3f cell_diff=%.3f is_background=%v init_if_empty=%v",
                        g.SensorID, ringIdx, azBinIdx, observationMean,
                        cell.AverageRangeMeters, cell.RangeSpreadMeters, cell.TimesSeenCount,
                        neighborConfirmCount, closenessThreshold, cellDiff,
                        isBackgroundLike, initIfEmpty)
                }
            }

            // ... continue with existing update logic ...
        }
    }

    // ... existing telemetry update ...
}
```

**What this tells us**: Shows exactly why observations are being rejected vs accepted. We'll see:

- Empty cells (times_seen=0) and whether initIfEmpty (seeding) kicked in
- Closeness threshold calculations at various ranges
- Neighbor confirmation counts
- Cell diff values relative to thresholds

**Important**: Only fires when `EnableDiagnostics` is true (set via API), so it doesn't spam logs in production.

#### F. Add acceptance rate summary per ProcessFramePolar call

**File**: `internal/lidar/background.go`
**Location**: End of `ProcessFramePolar()`, after the main loop

```go
func (bm *BackgroundManager) ProcessFramePolar(points []PointPolar) {
    // ... all existing logic ...

    // At the end, after updating telemetry:
    if bm.EnableDiagnostics && (foregroundCount > 0 || backgroundCount > 0) {
        total := foregroundCount + backgroundCount
        acceptPct := 0.0
        if total > 0 {
            acceptPct = (float64(backgroundCount) / float64(total)) * 100.0
        }

        log.Printf("[ProcessFramePolar:summary] sensor=%s points_in=%d cells_updated=%d bg_accept=%d fg_reject=%d accept_pct=%.2f%% noise_rel=%.6f closeness_mult=%.3f neighbor_confirm=%d seed_from_first=%v",
            g.SensorID, len(points), total, backgroundCount, foregroundCount, acceptPct,
            noiseRel, closenessMultiplier, neighConfirm, seedFromFirst)
    }

    // ... existing code ...
}
```

**What this tells us**: Frame-level acceptance summary showing what fraction of observations are accepted vs rejected, plus the active parameter values.

#### G. In `GetAcceptanceMetrics()` — add debug endpoint to dump raw bucket counts

**File**: `internal/lidar/monitor/webserver.go`
**Location**: Add a new debug endpoint (or extend existing one)

Add query param `?debug=true` to `/api/lidar/acceptance` that also returns per-bucket details:

```go
func (ws *WebServer) handleAcceptanceMetrics(w http.ResponseWriter, r *http.Request) {
    // ... existing logic to get metrics ...

    debug := r.URL.Query().Get("debug") == "true"

    resp := RichAcceptance{
        BucketsMeters:   metrics.BucketsMeters,
        AcceptCounts:    metrics.AcceptCounts,
        RejectCounts:    metrics.RejectCounts,
        Totals:          totals,
        AcceptanceRates: rates,
    }

    if debug {
        // Add verbose breakdown
        debugInfo := map[string]interface{}{
            "metrics": resp,
            "timestamp": time.Now().Format(time.RFC3339Nano),
            "sensor_id": sensorID,
        }
        // Also fetch current params for context
        if mgr := lidar.GetBackgroundManager(sensorID); mgr != nil {
            params := mgr.GetParams()
            debugInfo["params"] = map[string]interface{}{
                "noise_relative": params.NoiseRelativeFraction,
                "closeness_multiplier": params.ClosenessSensitivityMultiplier,
                "neighbor_confirmation": params.NeighborConfirmationCount,
                "seed_from_first": params.SeedFromFirstObservation,
            }
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(debugInfo)
        return
    }

    // ... existing response ...
}
```

**What this tells us**: Detailed bucket-by-bucket counts with active parameter context, useful for post-analysis.

---

## Issue 3: Frame Delivery (from earlier PCAP work)

### Already identified in debug doc

The `evictOldestBufferedFrame()` bug is confirmed and needs the one-line fix. Adding buffer lifecycle logging will help verify the fix worked.

#### H. In FrameBuilder — add buffer lifecycle logging

**File**: `internal/lidar/frame_builder.go`

Add logs in key lifecycle functions:

```go
func (fb *FrameBuilder) finalizeCurrentFrame(reason string) {
    // ... existing logic ...

    if fb.currentFrame != nil && fb.currentFrame.PointCount >= fb.minFramePoints {
        // ... move to buffer ...

        if fb.debug {
            log.Printf("[FrameBuilder:finalize] frame=%s points=%d reason=%s buffer_size=%d timestamp=%d",
                fb.currentFrame.FrameID, fb.currentFrame.PointCount, reason,
                len(fb.frameBuffer), time.Now().UnixNano())
        }
    }
}

func (fb *FrameBuilder) evictOldestBufferedFrame() {
    // ... existing logic to find oldest ...

    if oldestFrame != nil {
        if fb.debug {
            log.Printf("[FrameBuilder:evict] evicting_frame=%s points=%d buffer_size_before=%d timestamp=%d",
                oldestID, oldestFrame.PointCount, len(fb.frameBuffer), time.Now().UnixNano())
        }

        delete(fb.frameBuffer, oldestID)
        // ADD THE CRITICAL FIX HERE:
        fb.finalizeFrame(oldestFrame)  // Ensure evicted frames are finalized!
    }
}

func (fb *FrameBuilder) finalizeFrame(frame *LiDARFrame) {
    if frame == nil {
        return
    }

    if fb.debug {
        log.Printf("[FrameBuilder:callback] frame=%s points=%d azimuth_range=%.1f-%.1f timestamp=%d",
            frame.FrameID, len(frame.Points), frame.MinAzimuth, frame.MaxAzimuth,
            time.Now().UnixNano())
    }

    // ... existing callback invocation ...
}
```

**What this tells us**: Complete audit trail of frame lifecycle: finalization → buffering → eviction → callback delivery.

---

## Implementation Strategy

### Phase 1: Grid Reset Race (highest priority for multisweep)

1. Add logging A, B, C, D (ResetGrid, ProcessFramePolar population, API handlers)
2. Run a **single** parameter combination with bg-multisweep:
   - noise=0.01, closeness=2.0, neighbor=1, iterations-per=5, interval-per=2s, settle-time=1s
3. Grep logs for `[ResetGrid]`, `[API:grid_reset]`, `[API:params]`, `[ProcessFramePolar]`
4. Analyze timing: how quickly does nonzero_cells grow after reset?

Expected outcome: Logs will show grid reset at time T, then ProcessFramePolar logs showing nonzero_cells climbing from 0 to 60k+ within 1-2 seconds, confirming the race.

### Phase 2: Acceptance Rate Investigation

1. Add logging E, F, G (ProcessFramePolar decision details, summary, debug endpoint)
2. Enable diagnostics via API: `curl -X POST 'http://localhost:8081/api/lidar/params?sensor_id=hesai-pandar40p' -d '{"enable_diagnostics": true}'`
3. Run a short capture (10 seconds) and collect logs
4. Analyze decision logs to see:
   - How many cells start empty (times_seen=0)
   - Whether seeding is kicking in (init_if_empty=true)
   - Typical closeness_threshold values at various ranges
   - Neighbor confirmation patterns

Expected outcome: Logs will show either:

- Many empty cells rejecting observations (if seeding is off), OR
- Tight thresholds causing rejections at longer ranges (if noise_rel is too low), OR
- Neighbor confirmation failing frequently (if neighbor count is too strict)

### Phase 3: Frame Delivery Verification

1. Add logging H (FrameBuilder lifecycle)
2. Apply the critical fix to `evictOldestBufferedFrame()`
3. Run PCAP replay in `--debug` mode
4. Grep for `[FrameBuilder:callback]` and `[BackgroundManager] Persisted snapshot`
5. Verify frames are delivered and background populates (nonzero_cells > 0 in snapshots)

Expected outcome: Logs confirm frames finalize, callback fires, and background grid fills.

---

## Log Analysis Tools

### Quick grep patterns for investigation

```bash
# Grid reset timing
grep -E "\[ResetGrid\]|\[API:grid_reset\]|\[API:params\]" app.log | tail -50

# Population growth after reset
grep "\[ProcessFramePolar\]" app.log | awk '{print $NF, $(NF-1)}' | tail -20

# Acceptance decision samples
grep "\[ProcessFramePolar:decision\]" app.log | head -50

# Frame delivery
grep -E "\[FrameBuilder:(finalize|evict|callback)\]" app.log | tail -30

# Acceptance summary per frame
grep "\[ProcessFramePolar:summary\]" app.log | tail -20
```

### CSV export of key metrics

For post-processing, pipe logs to structured format:

```bash
# Extract reset→population timeline
grep "\[ResetGrid\]" app.log | \
  awk -F'nonzero_before=' '{print $2}' | \
  awk -F' ' '{print $1, $3, $5}' > reset_timeline.csv

# Extract acceptance rates over time
grep "\[ProcessFramePolar:summary\]" app.log | \
  awk -F'accept_pct=' '{print $2}' | \
  awk '{print $1}' > acceptance_rates.csv
```

---

## Success Criteria

### For grid reset race

- Logs show `[ResetGrid] nonzero_before=N nonzero_after=0` (confirms reset works)
- Logs show `[ProcessFramePolar]` entries with nonzero_cells climbing rapidly after reset timestamp
- We can correlate API call timestamps with grid population to measure the race window

### For acceptance rate investigation

- Logs show decision samples with empty cells and seeding behavior
- We identify the specific conditions causing rejections (empty cells, tight thresholds, neighbor gating)
- We can tune parameters based on observed closeness_threshold and cell_diff values

### For frame delivery

- Logs show `[FrameBuilder:callback]` entries (frames delivered)
- Background snapshots show nonzero_cells > 0 after PCAP replay
- No `[FrameBuilder:evict]` logs without corresponding `[FrameBuilder:callback]` (confirms fix)

---

## Files to Edit

1. `internal/lidar/background.go` — logs A, B, E, F
2. `internal/lidar/monitor/webserver.go` — logs C, D, G
3. `internal/lidar/frame_builder.go` — log H + critical fix

All logs are:

- Gated behind `--debug` or `EnableDiagnostics` flags
- Include timestamps (UnixNano) for precise correlation
- Include sensor_id for multi-sensor setups
- Structured for easy grep/awk parsing

---

## Next Steps

1. Review this plan
2. Implement Phase 1 logging (grid reset race) first — this is blocking bg-multisweep reliability
3. Run short test with logging and analyze
4. Iterate based on findings
5. Implement Phase 2 (acceptance) and Phase 3 (frame delivery) as needed
