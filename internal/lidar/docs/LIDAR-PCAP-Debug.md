# Lidar PCAP Replay — Debug Summary & Next Steps

This document captures the investigation into why PCAP replay runs did not populate the lidar background grid. It summarizes what we've tried, what we observed, where we stand, and recommended next steps with concrete commands to run.

## Status (short)

- PCAP replay successfully reads the file and parses many packets/points.
- FrameBuilder received parsed points (we added per-packet and per-frame debug). Points accumulate, but frames were not finalizing under the original wrap-detection logic. After tightening wrap detection we observed points being added but snapshots still show `nonzero_cells=0` at the last poll.

## Key findings

- The PCAP reader and parser are working: logs show large numbers of parsed points (example: `PCAP parsed points: packet=50000, points_this_packet=388, total_parsed_points=18196957`).
- Many PCAP packets legitimately produce 0 points; this is expected for datasets with empty packets.
- FrameBuilder received points — we see logs like:
  - `[FrameBuilder] Added 269 points; frame_count was=28733198 now=28733467; lastAzimuth=289.00`
- **Critical observation**: Frame point counts grew to 28M+ points in a single frame without finalizing. This indicates frames are accumulating points but never being completed/delivered to the callback.
- Original azimuth wrap detection only checked `lastAzimuth > 350 && azimuth < 10`, which missed wraps like `289 -> 61` seen in the capture. We added an additional check for large negative jumps (`lastAzimuth - azimuth > 180`) to catch these cases.
- **Code analysis reveals**: `evictOldestBufferedFrame()` has a TODO comment and doesn't call `finalizeFrame()` when removing frames from the buffer. This means buffered frames are deleted without invoking the callback — **this is almost certainly the bug**.
- Despite parser + FrameBuilder activity, background snapshot persisted blobs remain tiny and `nonzero_cells==0`. That means frames are not being delivered to `BackgroundManager.ProcessFramePolar`.

## What we tried (chronological)

1. Added CLI `--db-path` and cleaned up docs/CI (prior work not directly related to PCAP debugging).
2. Implemented `SeedFromFirstObservation` (opt-in) to allow seeding background from first observations when running in PCAP mode and wired it to `--lidar-pcap-mode`.
3. Added per-frame debug logging in `internal/lidar/frame_builder.go` (SetDebug + log on finalize and when points are added).
4. Added diagnostics to PCAP reader `internal/lidar/network/pcap.go` to log when packets parse to 0 points and cumulative parsed points.
5. Ran PCAP replay (`break-80k.pcapng`) via the monitor API. Observed many "Added N points" logs but no frame completion logs initially.
6. Strengthened azimuth-wrap detection logic to also treat large negative azimuth jumps (>180°) as rotation wraps.
7. Rebuilt and reran. Parser and FrameBuilder logs confirmed points added, but snapshots still show `nonzero_cells=0`.

## Relevant log excerpts

- Parser/PCAP progress:
  - `PCAP progress: 50000 packets processed in 9.69s (5159 pkt/s)`
  - `PCAP parsed points: packet=50000, points_this_packet=388, total_parsed_points=18196957`
- FrameBuilder activity:
  - `[FrameBuilder] Added 269 points; frame_count was=28733198 now=28733467; lastAzimuth=289.00`
  - `[FrameBuilder] Added 48 points; frame_count was=28733769 now=28733817; lastAzimuth=61.16`
- Persistent snapshot (example):
  - `[BackgroundManager] Persisted snapshot: sensor=hesai-pandar40p, reason=periodic_pcap_flush, nonzero_cells=0/72000, grid_blob_size=258 bytes`

## Where we stand

- Tests: `make test-go` and focused unit tests pass.
- Runtime: PCAP replay reads and parses the file and FrameBuilder receives points. However, persisted background snapshots (which are used to seed the system) still report zero nonzero cells.

## Likely root causes

1. **Frames not finalizing**: We saw long-growing `frame_count` values (28M+ points in a single frame). The critical issue is that `finalizeCurrentFrame()` has a gate: `fb.currentFrame.PointCount < fb.minFramePoints` causes frames to be discarded. Default `minFramePoints` is 1000, but we never saw frame completion logs, suggesting frames either:

   - Never reach the finalization path (wrap detection fails), or
   - Are moved to `frameBuffer` but the cleanup timer (`cleanupFrames()`) isn't finalizing them before more points accumulate in the current frame.

2. **FrameBuilder buffer timeout issue**: Frames go into `frameBuffer` and wait for `bufferTimeout` (500ms) before `cleanupFrames()` calls `finalizeFrame()`. During fast PCAP replay, the cleanup timer may not fire frequently enough, or frames sit in the buffer while new points keep getting added to `currentFrame`.

3. **Eviction path doesn't finalize**: In `evictOldestBufferedFrame()`, there's a TODO comment suggesting frames are deleted from buffer but NOT finalized (no callback invoked): `// TODO: Add output channel or callback for completed frames`. This is a **smoking gun** — buffered frames may be discarded without ever calling the callback.

4. **Background update gating**: Even if frames reach `BackgroundManager.ProcessFramePolar`, the closeness threshold, neighbor confirmation, and noise relative fraction may be too strict for replayed data and reject observations as foreground.

5. **SeedFromFirstObservation**: May not have been enabled at runtime for this run, or snapshot timing didn't capture newly-initialized cells.

## Options to pursue (ranked)

### Critical fix (highest priority)

**Fix `evictOldestBufferedFrame()` to actually finalize frames**

- **Issue**: Line ~436 in `frame_builder.go` has a TODO comment and doesn't call `fb.finalizeFrame(oldestFrame)` when evicting from buffer. Frames are deleted but the callback is never invoked.
- **Fix**: Add `fb.finalizeFrame(oldestFrame)` after deleting from buffer.
- **Impact**: This is likely THE bug. Frames accumulate in `frameBuffer`, get evicted when buffer is full, but are discarded without invoking the callback that feeds `BackgroundManager`.
- **Code location**: `internal/lidar/frame_builder.go:~436`
- **One-line fix**:
  ```go
  if oldestFrame != nil {
      delete(fb.frameBuffer, oldestID)
      fb.finalizeFrame(oldestFrame)  // ADD THIS LINE
  }
  ```

### High priority diagnostics

1. **Add frame buffer monitoring logs**

   - Log when frames enter `frameBuffer`, when they're evicted, and current buffer size.
   - This will confirm whether frames sit in buffer or are immediately finalized.
   - Add in `finalizeCurrentFrame()`: `log.Printf("[FrameBuilder] Frame buffered: %s, buffer_size=%d", frame.FrameID, len(fb.frameBuffer))`

2. **Tail logs with grep for critical events** (low-risk, immediate)

   - Tail the running process stdout/stderr while replaying and grep for:
     - `Frame completed` (frame finalization via callback)
     - `Frame buffered` (if we add the log above)
     - `Sending .* points to BackgroundManager` (callback invocation)
     - `Persisted snapshot.*nonzero_cells` (grid population)
   - Commands:
     ```bash
     go build -tags=pcap -o app-radar-local ./cmd/radar
     ./app-radar-local --disable-radar --enable-lidar --lidar-pcap-mode --debug 2>&1 | tee pcap-debug.log &
     curl -X POST 'http://127.0.0.1:8081/api/lidar/pcap/start?sensor_id=hesai-pandar40p' \
       -d '{"pcap_file":"/Users/david/code/sensor_data/lidar/break-80k.pcapng"}'
     # Wait 30s, then:
     grep -E "Frame (completed|buffered)|Sending.*BackgroundManager|nonzero_cells" pcap-debug.log | tail -50
     ```

3. **Lower `minFramePoints` for PCAP mode**
   - Current default is 1000 points. For PCAP replay with potentially sparse data or different packet patterns, lower to 100.
   - Add to `FrameBuilderConfig` in `cmd/radar/radar.go` when `lidarPCAPMode`:
     ```go
     config := lidar.FrameBuilderConfig{
         SensorID:        *lidarSensor,
         FrameCallback:   callback,
         FrameBufferSize: 100,
         BufferTimeout:   500 * time.Millisecond,
         CleanupInterval: 250 * time.Millisecond,
     }
     if *lidarPCAPMode {
         config.MinFramePoints = 100  // Lower threshold for PCAP
     }
     frameBuilder = lidar.NewFrameBuilder(config)
     ```

### Medium priority

4. **Toggle runtime params to be permissive** (medium-risk, reversible)

   - Use the monitor API to set `noise_relative` to `1.0` and enable diagnostic logging on `BackgroundManager`.
   - This will make ProcessFramePolar accept observations as background and show detailed acceptance metrics.
   - If `/api/lidar/params` supports POST, change:
     - `noise_relative` -> `1.0`
     - `enable_diagnostics` -> `true`

5. **Add explicit frame finalization on PCAP completion**

   - When PCAP reader finishes (`ReadPCAPFile` returns), explicitly flush any pending frames.
   - After the packet loop in `pcap.go`, add:
     ```go
     if frameBuilder != nil {
         log.Printf("PCAP replay complete, flushing pending frames...")
         // Force flush via a stop/restart or explicit finalizeAll method
     }
     ```

6. **Reduce cleanup interval for PCAP mode**
   - Current `CleanupInterval` is 250ms. During fast PCAP replay (5k+ pkt/s), frames may accumulate faster than cleanup runs.
   - Set `CleanupInterval: 50 * time.Millisecond` when in PCAP mode.

### Diagnostic-only (use sparingly)

7. **Diagnostic direct-feed** (highly diagnostic)

   - Add a temporary debug path in the PCAP reader to call `BackgroundManager.ProcessFramePolar` directly per-packet (bypass FrameBuilder).
   - This confirms whether ProcessFramePolar accepts per-packet points and updates the grid.
   - Use only for short tests due to possible noisy updates.

8. **Add comprehensive frame lifecycle logging**
   - Log every state transition: `startNewFrame`, `shouldStartNewFrame` (with reason), `finalizeCurrentFrame`, `evictOldestBufferedFrame`, `finalizeFrame`, `cleanupFrames`.
   - This creates a complete audit trail but generates high log volume.

### Long-term fixes

9. **Harden frame detection**

   - Combine azimuth-jump, timeouts, and motor-speed-based heuristics.
   - Add unit/integration tests that exercise PCAP replay -> frame building -> background persistence flow (simulate PCAP data with known patterns).

10. **Redesign FrameBuilder buffering**
    - Current buffer pattern (frames wait for backfill) may not suit PCAP replay where packets arrive in-order and fast.
    - Consider immediate finalization for PCAP mode (no buffering delay).

## Recommended immediate plan (conservative)

1. **APPLY THE CRITICAL FIX FIRST** — add `fb.finalizeFrame(oldestFrame)` to `evictOldestBufferedFrame()` in `frame_builder.go`. This is a one-line change that's very likely the root cause.

2. **Rebuild and test** — rebuild with the fix and replay the PCAP. Monitor logs for:

   - `[FrameBuilder] Frame completed` messages
   - `[FrameBuilder] Sending N points to BackgroundManager` messages
   - `[BackgroundManager] Persisted snapshot:` with `nonzero_cells > 0`

3. **If still no frames finalize**, add frame buffer monitoring logs (option 1 from high-priority list) and lower `minFramePoints` to 100 for PCAP mode (option 3).

4. **If frames finalize but background stays empty**, toggle `noise_relative` to `1.0` and enable diagnostics (option 4) for a short run and re-check snapshots.

## Additional technical insights from code analysis

### FrameBuilder flow and the missing callback

The FrameBuilder uses a two-stage approach:

1. `finalizeCurrentFrame()` moves frames from `currentFrame` to `frameBuffer` (for potential late-packet backfill)
2. `cleanupFrames()` timer (every 250ms) checks buffer for old frames and calls `finalizeFrame()` which invokes the callback

**The bug**: When `frameBuffer` exceeds `frameBufferSize` (100), `evictOldestBufferedFrame()` is called to make room. This function deletes the oldest frame but has a TODO comment and **never calls `finalizeFrame()`**. This means:

- Frames enter the buffer via azimuth wrap detection
- Buffer fills up (100 frames)
- Oldest frames get deleted to make room
- **Callback is never invoked for evicted frames**
- BackgroundManager never receives points

### Why frames grew to 28M+ points

- Azimuth wrap detection initially failed (didn't catch 289->61 wraps), so frames never moved to buffer
- Points kept accumulating in `currentFrame`
- After we added the negative-jump detection, frames may now enter the buffer but still don't finalize due to the eviction bug

### Buffer timeout vs. fast PCAP replay

- `bufferTimeout` is 500ms — frames wait this long before `cleanupFrames()` finalizes them
- PCAP replay runs at 5k+ pkt/s, processing the entire file in ~18 seconds
- During fast replay, new frames may fill the buffer before cleanup timer fires
- Eviction kicks in, discards frames without callback

### Why unit tests pass

Unit tests likely:

- Don't fill the frame buffer to capacity (test with <100 frames)
- Use slower packet rates where cleanup timer fires before buffer fills
- Test individual functions but not the full buffer-eviction code path

## Notes and safety

- All proposed code changes for debugging are gated behind `--lidar-pcap-mode` or are temporary and reversible.
- Increasing `noise_relative` or lowering thresholds can cause spurious acceptance of noisy returns — do this only for short diagnostic runs.
- **The critical fix** (adding `finalizeFrame()` call in eviction) is safe and should be permanent — this is clearly a bug not a tuning parameter.

If useful, I can add a helper script `scripts/debug_pcap_replay.sh` that: builds, starts the binary (killing existing listener on 8081 if needed), triggers the PCAP replay, tails logs for N seconds, and fetches snapshots/grid_status. Tell me if you'd like that and I will add it.

## Next steps checklist

- [ ] Apply critical fix to `evictOldestBufferedFrame()`
- [ ] Rebuild with `go build -tags=pcap -o app-radar-local ./cmd/radar`
- [ ] Restart binary with `--lidar-pcap-mode --debug`
- [ ] Trigger PCAP replay via API
- [ ] Monitor logs for "Frame completed" and "Sending ... to BackgroundManager"
- [ ] Check snapshots for `nonzero_cells > 0`
- [ ] If still empty, add frame buffer monitoring logs
- [ ] If still empty, lower `minFramePoints` to 100 for PCAP mode
- [ ] If frames work but background stays empty, toggle `noise_relative` to 1.0

---

Created by the debugging session on branch `dd/lidar/read-pcap`.
