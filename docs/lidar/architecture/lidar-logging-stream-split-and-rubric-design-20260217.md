# LiDAR Logging Stream Split and Rubric Design (2026-02-17)

## Objective

Replace the single `Debugf` logging stream with three explicit streams (`Opsf`, `Diagf`, `Tracef`) to separate actionable events from high-volume telemetry.

## Target Logging Model

Three stream model:

| Stream  | Purpose                                                      | Typical volume | Retention |
| ------- | ------------------------------------------------------------ | -------------- | --------- |
| `ops`   | Actionable warnings/errors and significant lifecycle events  | Low            | Longest   |
| `diag`  | Day-to-day diagnostics for troubleshooting and tuning        | Medium         | Medium    |
| `trace` | High-frequency packet/frame telemetry and loop-level details | High           | Shortest  |

## API Surface

- `SetLogWriters(LogWriters{Ops, Diag, Trace io.Writer})` — root package
- `SetLogWriter(level LogLevel, w io.Writer)` — root package (panics on invalid level)
- `SetLogWriters(ops, diag, trace io.Writer)` — sub-packages (l2frames, l3grid, pipeline)
- `Opsf(...)` / `opsf(...)`
- `Diagf(...)` / `diagf(...)`
- `Tracef(...)` / `tracef(...)`

`Debugf`, the keyword classifier, and the legacy `SetDebugLogger`/`SetLegacyLogger` shims have been removed. All call sites use explicit stream functions.

## Routing Rubric (Severity + Volume)

1. If it indicates operator action, failure, or data-loss risk → `ops`.
2. Else if it is expected at packet/frame loop frequency → `trace`.
3. Else → `diag`.

### Detailed rubric matrix

| Signal                                                                                              | Route   | Rationale                                |
| --------------------------------------------------------------------------------------------------- | ------- | ---------------------------------------- |
| Error, failed operation, dropped data, timeout, repeated disconnect                                 | `ops`   | Must be visible immediately              |
| Per-packet parse messages, replay progress, queue depth every frame, FPS/bandwidth stats each cycle | `trace` | High volume; useful for deep diagnostics |
| Cluster counts, track counts, lifecycle transitions, occasional state snapshots                     | `diag`  | Useful context without flooding ops logs |

## Runtime Configuration Design

Env vars:

- `VELOCITY_LIDAR_OPS_LOG`
- `VELOCITY_LIDAR_DIAG_LOG`
- `VELOCITY_LIDAR_TRACE_LOG`

Operational defaults:

- If only one new stream path is set, route unspecified streams to the same writer to avoid silent log loss.

## File/Retention Guidance

- `ops`: rotate daily, retain longer (incident and reliability evidence)
- `diag`: rotate daily or by size, moderate retention
- `trace`: aggressive rotation and short retention (high churn)

## Out of Scope

- Full structured logging migration (JSON fields, correlation IDs, external log pipeline integration).

## Complete Call-Site Audit

All `Debugf`/`debugf` call sites have been migrated to explicit stream functions. The classifier and `Debugf` have been removed.

### ops (4 sites — errors, dropped data)

| File                                        | Line | Message pattern                                                | Confidence |
| ------------------------------------------- | ---- | -------------------------------------------------------------- | ---------- |
| `l1packets/network/foreground_forwarder.go` | 94   | `Error encoding foreground points: %v`                         | ✅ high    |
| `l1packets/network/foreground_forwarder.go` | 102  | `Error forwarding foreground packet: %v`                       | ✅ high    |
| `l1packets/network/forwarder.go`            | 75   | `[PacketForwarder] Dropped %d forwarded packets due to errors` | ✅ high    |
| `l2frames/frame_builder.go`                 | 800  | `[FrameBuilder] Dropped frame %s: callback queue full`         | ✅ high    |

### trace (31 sites — per-packet/per-frame frequency)

| File                                        | Line | Message pattern                                   | Confidence |
| ------------------------------------------- | ---- | ------------------------------------------------- | ---------- |
| `visualiser/grpc_server.go`                 | 313  | `[gRPC] Client %s: skipped %d frames to catch up` | ✅ high    |
| `visualiser/grpc_server.go`                 | 395  | `[gRPC] Client %s stats: fps=…`                   | ✅ high    |
| `visualiser/publisher.go`                   | 621  | `[Visualiser] Stats: fps=…`                       | ✅ high    |
| `l1packets/network/foreground_forwarder.go` | 109  | `[ForegroundForwarder] sent frame=…`              | ✅ high    |
| `l1packets/network/foreground_forwarder.go` | 134  | `[ForegroundForwarder] queued %d points`          | ✅ high    |
| `l1packets/network/pcap.go`                 | 157  | `PCAP packet %d parsed -> 0 points`               | ✅ high    |
| `l1packets/network/pcap.go`                 | 162  | `PCAP parsed points: packet=…`                    | ✅ high    |
| `l1packets/network/pcap_realtime.go`        | 267  | `PCAP real-time replay: packet %d -> 0 points`    | ✅ high    |
| `l1packets/network/pcap_realtime.go`        | 277  | `PCAP real-time replay: packet=…`                 | ✅ high    |
| `l1packets/network/pcap_realtime.go`        | 342  | `[ForegroundForwarder] warmup skipping frame`     | ✅ high    |
| `l1packets/network/pcap_realtime.go`        | 387  | `Foreground extraction: %d/%d points`             | ✅ high    |
| `l1packets/network/pcap_realtime.go`        | 404  | `PCAP real-time replay progress`                  | ✅ high    |
| `l2frames/frame_builder.go`                 | 387  | `[FrameBuilder] Frame completion detected`        | ✅ high    |
| `l2frames/frame_builder.go`                 | 413  | `[FrameBuilder] Added %d points`                  | ✅ high    |
| `l2frames/frame_builder.go`                 | 569  | `[FrameBuilder] Moved frame to buffer`            | ✅ high    |
| `l2frames/frame_builder.go`                 | 646  | `[FrameBuilder] cleanupFrames invoked`            | ✅ high    |
| `l2frames/frame_builder.go`                 | 695  | `[FrameBuilder] Finalizing idle current frame`    | ✅ high    |
| `l2frames/frame_builder.go`                 | 714  | `[FrameBuilder] Frame completed`                  | ✅ high    |
| `l2frames/frame_builder.go`                 | 731  | `[FrameBuilder] Incomplete or gappy frame`        | ✅ high    |
| `l2frames/frame_builder.go`                 | 791  | `[FrameBuilder] Invoking frame callback`          | ✅ high    |
| `l2frames/frame_builder.go`                 | 886  | `Frame completed (callback)`                      | ✅ high    |
| `pipeline/tracking_pipeline.go`             | 198  | `[FrameBuilder] Completed frame`                  | ✅ high    |
| `pipeline/tracking_pipeline.go`             | 295  | `[Pipeline] Throttled %d frames`                  | ✅ high    |
| `pipeline/tracking_pipeline.go`             | 332  | `[Tracking] Extracted %d foreground points`       | ✅ high    |
| `pipeline/tracking_pipeline.go`             | 352  | `[Tracking] Ground filter`                        | ✅ high    |
| `pipeline/tracking_pipeline.go`             | 369  | `[Tracking] Voxel downsample`                     | ✅ high    |
| `pipeline/tracking_pipeline.go`             | 408  | `[Tracking] Clustered into %d objects`            | ⚠️ review  |
| `pipeline/tracking_pipeline.go`             | 419  | `[Tracking] %d confirmed tracks to persist`       | ⚠️ review  |
| `pipeline/tracking_pipeline.go`             | 517  | `[Visualiser] Published frame %s to gRPC`         | ✅ high    |
| `l3grid/foreground.go`                      | 233  | `[FG_FROZEN]`                                     | ✅ high    |
| `l3grid/foreground.go`                      | 247  | `[FG_THAW]`                                       | ✅ high    |
| `l3grid/foreground.go`                      | 419  | `[FG_FREEZE]`                                     | ✅ high    |
| `l3grid/foreground.go`                      | 430  | `[FG_DEBUG]`                                      | ✅ high    |
| `l3grid/foreground.go`                      | 462  | `[Foreground] warmup active`                      | ✅ high    |

### diag (20 sites — lifecycle, occasional diagnostics)

| File                                        | Line | Message pattern                              | Confidence |
| ------------------------------------------- | ---- | -------------------------------------------- | ---------- |
| `visualiser/grpc_server.go`                 | 244  | `[gRPC] Client %s subscribed`                | ✅ high    |
| `visualiser/grpc_server.go`                 | 271  | `[gRPC] Client %s disconnected`              | ✅ high    |
| `visualiser/publisher.go`                   | 386  | `[Visualiser] Background sequence changed`   | ✅ high    |
| `visualiser/publisher.go`                   | 397  | `[Visualiser] Background interval elapsed`   | ✅ high    |
| `visualiser/publisher.go`                   | 451  | `[Visualiser] Background snapshot sent`      | ✅ high    |
| `l1packets/network/foreground_forwarder.go` | 78   | `Foreground forwarder stopping`              | ✅ high    |
| `l1packets/network/forwarder.go`            | 83   | `[PacketForwarder] Forwarding packets to %s` | ✅ high    |
| `l2frames/frame_builder.go`                 | 302  | `[FrameBuilder] Reset: cleared all buffered` | ✅ high    |
| `l2frames/frame_builder.go`                 | 553  | `[FrameBuilder] Discarding incomplete frame` | ⚠️ review  |
| `l2frames/frame_builder.go`                 | 592  | `[FrameBuilder] Evicting buffered frame`     | ✅ high    |
| `l2frames/frame_builder.go`                 | 761  | `[FrameBuilder] Exported next frame`         | ✅ high    |
| `l2frames/frame_builder.go`                 | 777  | `[FrameBuilder] Exported batch frame`        | ✅ high    |
| `l2frames/frame_builder.go`                 | 945  | `[FrameBuilder] all Z==0, recomputing XYZ`   | ✅ high    |
| `pipeline/tracking_pipeline.go`             | 328  | `[Tracking] FgForwarder is nil`              | ✅ high    |
| `pipeline/tracking_pipeline.go`             | 356  | `[Tracking] Ground removal disabled`         | ✅ high    |
| `pipeline/tracking_pipeline.go`             | 499  | `[Tracking] %d confirmed tracks active`      | ✅ high    |
| `pipeline/tracking_pipeline.go`             | 536  | `[Tracking] Pruned %d deleted tracks`        | ✅ high    |

### Sites flagged for user review (⚠️)

These 3 call sites were classified with best-effort judgement. Please confirm or reassign:

1. **`pipeline/tracking_pipeline.go:408`** — `[Tracking] Clustered into %d objects`
   - Currently: `tracef` (fires every frame → per-frame frequency)
   - Alternative: `diagf` (design doc originally listed this as "debug"; useful context)

2. **`pipeline/tracking_pipeline.go:419`** — `[Tracking] %d confirmed tracks to persist`
   - Currently: `tracef` (fires every frame → per-frame frequency)
   - Alternative: `diagf` (useful for track lifecycle visibility)

3. **`l2frames/frame_builder.go:553`** — `[FrameBuilder] Discarding incomplete frame`
   - Currently: `diagf` (normal during startup; expected behaviour)
   - Alternative: `opsf` (could indicate data quality issues or sensor problems)
