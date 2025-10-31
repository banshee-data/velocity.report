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
- Original azimuth wrap detection only checked `lastAzimuth > 350 && azimuth < 10`, which missed wraps like `289 -> 61` seen in the capture. We added an additional check for large negative jumps (`lastAzimuth - azimuth > 180`) to catch these cases.
- Despite parser + FrameBuilder activity, background snapshot persisted blobs remain tiny and `nonzero_cells==0`. That means either frames are not being finalized and delivered to `BackgroundManager.ProcessFramePolar`, or the background update logic is still rejecting observations (thresholding/neighbour checks) and `TimesSeenCount` remains zero.

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

1. Frames still may not be finalizing reliably in all cases (we saw long-growing `frame_count` values earlier), so the callback into `BackgroundManager` may not be invoked at frame boundaries.
2. Background update gating (closeness threshold, neighbor confirmation, noise relative fraction) may be too strict for replayed data and is rejecting observations as foreground or not seeding cells.
3. `SeedFromFirstObservation` may not have been enabled at runtime for this run, or side-effects mean snapshot timing didn't capture newly-initialized cells.

## Options to pursue (ranked)

1. Tail logs (low-risk, immediate)

   - Tail the running process stdout/stderr while replaying and look for:
     - `[FrameBuilder] Frame completed - ID: ...` (frame finalization)
     - `[BackgroundManager] Persisted snapshot: ... nonzero_cells=...` (grid population)
   - Commands:
     - `go build -tags=pcap -o app-radar-local ./cmd/radar`
     - `./app-radar-local --disable-radar --enable-lidar --lidar-pcap-mode --debug`
     - `curl -X POST 'http://127.0.0.1:8081/api/lidar/pcap/start?sensor_id=hesai-pandar40p' -d '{"pcap_file":"/path/break-80k.pcapng"}'`

2. Toggle runtime params to be permissive (medium-risk, reversible)

   - Use the monitor API to set `noise_relative` to `1.0` and enable diagnostic logging on `BackgroundManager`.
   - This will make ProcessFramePolar accept observations as background and show detailed acceptance metrics.
   - If `/api/lidar/params` supports POST, change:
     - `noise_relative` -> `1.0`
     - `enable_diagnostics` -> `true`

3. Add a PCAP-only fallback to finalize frames sooner (low-medium risk)

   - When `--lidar-pcap-mode` is enabled, finalize frames on a shorter inactivity timeout or lower `MinFramePointsForCompletion` so we don't rely only on azimuth wrap.
   - This is safe behind the PCAP-mode flag and can be reverted.

4. Diagnostic direct-feed (highly diagnostic)

   - Add a temporary debug path in the PCAP reader to call `BackgroundManager.ProcessFramePolar` directly per-packet (bypass FrameBuilder). This confirms whether ProcessFramePolar accepts per-packet points and updates the grid.
   - Use only for short tests due to possible noisy updates.

5. Long-term fixes
   - Harden frame detection further (combine azimuth-jump, timeouts, and motor-speed-based heuristics).
   - Add unit/integration tests that exercise PCAP replay -> frame building -> background persistence flow (simulate PCAP data with known patterns).

## Recommended immediate plan (conservative)

1. Tail logs during a PCAP replay (option 1). If you prefer I can do this for you (I may need permission to stop & restart the process listening on :8081).
2. If frames still don't finalize, flip `noise_relative` to `1.0` and enable diagnostics (option 2) for a short run and re-check snapshots.

## Notes and safety

- All proposed code changes for debugging are gated behind `--lidar-pcap-mode` or are temporary and reversible.
- Increasing `noise_relative` or lowering thresholds can cause spurious acceptance of noisy returns — do this only for short diagnostic runs.

If useful, I can add a helper script `scripts/debug_pcap_replay.sh` that: builds, starts the binary (killing existing listener on 8081 if needed), triggers the PCAP replay, tails logs for N seconds, and fetches snapshots/grid_status. Tell me if you'd like that and I will add it.

---

Created by the debugging session on branch `dd/lidar/read-pcap`.
