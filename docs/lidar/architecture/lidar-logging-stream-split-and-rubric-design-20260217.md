# LiDAR Logging Stream Split and Rubric Design (2026-02-17)

## Objective

Refactor LiDAR logging from a single debug stream into multiple streams based on importance and log volume, while preserving compatibility with existing `Debugf` call sites.

## Current State

- LiDAR logging currently uses one optional logger in `internal/lidar/debug.go`.
- Most instrumentation emits via `Debugf(...)`, including:
  - actionable errors (forwarding failures, dropped packets)
  - normal diagnostics (cluster counts, state transitions)
  - high-volume telemetry (per-packet/per-frame stats)

This makes operations hard because high-volume lines drown actionable events.

## Target Logging Model

Three stream model:

| Stream  | Purpose                                                      | Typical volume | Retention |
| ------- | ------------------------------------------------------------ | -------------- | --------- |
| `ops`   | Actionable warnings/errors and significant lifecycle events  | Low            | Longest   |
| `debug` | Day-to-day diagnostics for troubleshooting and tuning        | Medium         | Medium    |
| `trace` | High-frequency packet/frame telemetry and loop-level details | High           | Shortest  |

## Proposed API Surface

Keep package-local usage simple and backward compatible:

- `SetLogWriters(LogWriters{Ops, Debug, Trace io.Writer})`
- `SetLogWriter(level LogLevel, w io.Writer)`
- `Opsf(...)`
- `Diagf(...)`
- `Tracef(...)`
- `Debugf(...)` remains available and routes via rubric/classifier
- `SetDebugLogger(io.Writer)` remains as compatibility shim (writes all streams to one writer)

## Routing Rubric (Severity + Volume)

Use this rubric for each log line.

1. If it indicates operator action, failure, or data-loss risk, route to `ops`.
2. Else if it is expected at packet/frame loop frequency, route to `trace`.
3. Else route to `debug`.

### Detailed rubric matrix

| Signal                                                                                              | Route   | Rationale                                |
| --------------------------------------------------------------------------------------------------- | ------- | ---------------------------------------- |
| Error, failed operation, dropped data, timeout, repeated disconnect                                 | `ops`   | Must be visible immediately              |
| Per-packet parse messages, replay progress, queue depth every frame, FPS/bandwidth stats each cycle | `trace` | High volume; useful for deep diagnostics |
| Cluster counts, track counts, lifecycle transitions, occasional state snapshots                     | `debug` | Useful context without flooding ops logs |

### Keyword guidance (for classifier fallback)

- `ops` keywords: `error`, `failed`, `fatal`, `panic`, `warn`, `timeout`, `dropped`
- `trace` keywords: `packet`, `queued`, `parsed`, `progress`, `fps=`, `bandwidth`, `frame=`
- default: `debug`

Classifier is a migration helper, not a permanent substitute for explicit `Opsf/Diagf/Tracef` at hot call sites.

## Example Mapping from Existing LiDAR Logs

| Existing line pattern                                                      | Stream  |
| -------------------------------------------------------------------------- | ------- |
| `Error forwarding foreground packet: ...`                                  | `ops`   |
| `[PacketForwarder] Dropped ... forwarded packets ...`                      | `ops`   |
| `PCAP parsed points: packet=..., points_this_packet=...`                   | `trace` |
| `PCAP real-time replay progress: ...`                                      | `trace` |
| `[Tracking] Clustered into %d objects`                                     | `debug` |
| `[Tracking] %d confirmed tracks active`                                    | `debug` |
| `[Visualiser] Stats: fps=... bandwidth_mbps=...` (if emitted continuously) | `trace` |

## Runtime Configuration Design

Recommended env vars:

- `VELOCITY_LIDAR_OPS_LOG`
- `VELOCITY_LIDAR_DEBUG_LOG`
- `VELOCITY_LIDAR_TRACE_LOG`

Compatibility fallback:

- If only legacy `VELOCITY_DEBUG_LOG` is set, route all streams to that file.

Operational defaults:

- If only one new stream path is set, route unspecified streams to the same writer to avoid silent log loss.

## File/Retention Guidance

- `ops`: rotate daily, retain longer (incident and reliability evidence)
- `debug`: rotate daily or by size, moderate retention
- `trace`: aggressive rotation and short retention (high churn)

## Migration Plan

1. Introduce stream-aware logging primitives and compatibility wrappers.
2. Keep all existing `Debugf` call sites working through classifier routing.
3. Convert known hot paths (packet/replay loops) to explicit `Tracef`.
4. Convert clear failures/data-loss lines to explicit `Opsf`.
5. Add tests for:
   - stream routing
   - classifier behavior
   - nil writer behavior
   - concurrent write safety
6. Update runbook docs with env vars and retention expectations.

## Acceptance Criteria

1. LiDAR logging supports three streams (`ops`, `debug`, `trace`).
2. Existing callers using `Debugf` continue to function.
3. Actionable failures are visible in `ops` without trace noise.
4. High-volume loops can be isolated to `trace`.
5. Logger behavior is thread-safe under concurrent emission.

## Out of Scope

- Full structured logging migration (JSON fields, correlation IDs, external log pipeline integration).
- Rewriting every historical log line in one PR; this design allows incremental migration.

## Implementation Status

Core three-stream model is implemented in `internal/lidar/debug.go`:

- `SetLogWriters(LogWriters{Ops, Debug, Trace})` configures all streams.
- `SetLogWriter(level, w)` configures a single stream.
- `Opsf`, `Diagf`, `Tracef` emit to explicit streams.
- `Debugf` routes via keyword classifier (ops keywords first, then trace, default debug).
- `SetDebugLogger(w)` compatibility shim routes all three streams to one writer.
- Thread-safe via `sync.RWMutex` around logger pointer access.

Entry point wiring in `cmd/radar/radar.go` supports:

- `VELOCITY_LIDAR_OPS_LOG`, `VELOCITY_LIDAR_DEBUG_LOG`, `VELOCITY_LIDAR_TRACE_LOG` (new).
- `VELOCITY_DEBUG_LOG` legacy fallback (all streams to one file).
- Unspecified stream paths fall back to the first explicitly set path.

Sub-package migration (pipeline, l2frames, l3grid) is incremental and not yet started.
