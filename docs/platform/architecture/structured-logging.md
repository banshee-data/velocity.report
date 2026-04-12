# Structured logging

Three-stream logging model for the Go codebase: `ops`, `diag`, `trace`.

Active plan: [go-structured-logging-plan.md](../../plans/go-structured-logging-plan.md)

## Problem

The Go codebase uses three distinct logging mechanisms:

1. `log.Printf()` — standard library, no levels, no structure
2. `fmt.Printf()` — not logging at all, just prints to stdout
3. `monitoring.Logf` — package-level function pointer, replaceable but not structured

Emoji appears in log output. Outside the LiDAR pipeline, logs have no consistent levels and
no structured key-value pairs. On a Raspberry Pi running as a systemd service, operators use
`journalctl` to diagnose problems. Unstructured, unlevel, mixed-destination logs make
diagnosis slower than it needs to be.

## Three-Stream model

The LiDAR pipeline established the target model with `Opsf`/`Diagf`/`Tracef`. This plan
extends that model to the entire Go codebase.

| Stream  | Purpose                                                     | Volume | Retention |
| ------- | ----------------------------------------------------------- | ------ | --------- |
| `ops`   | Actionable warnings/errors and significant lifecycle events | Low    | Longest   |
| `diag`  | Day-to-day diagnostics for troubleshooting and tuning       | Medium | Medium    |
| `trace` | High-frequency packet/frame telemetry and loop-level detail | High   | Shortest  |

### Routing rubric

1. If it indicates operator action, failure, or data-loss risk → **ops**.
2. Else if it is expected at packet/frame loop frequency → **trace**.
3. Else → **diag**.

Most non-LiDAR code produces `ops` and `diag` messages; `trace` is rare outside the
pipeline.

## Design principles

### No fourth pattern

The migration replaces `log.Printf`, `fmt.Printf`, and `monitoring.Logf` calls with the
appropriate stream function. It does not introduce a new logging framework or layer
`log/slog` on top.

Structured logging (JSON fields, correlation IDs, external log pipeline integration) remains
out of scope. If needed later, it can be layered on top of the three-stream model without
changing call sites.

### One configuration surface

Two runtime controls, set once at startup in `cmd/radar/radar.go`:

- `--log-level ops|diag|trace` (CLI flag, default: `ops`) — verbosity threshold
- `VELOCITY_DEBUG_LOG` (env var) — file path for debug output

Behaviour:

| Flag                | Output                      |
| ------------------- | --------------------------- |
| `--log-level ops`   | Only ops stream to stdout   |
| `--log-level diag`  | Ops + diag to stdout        |
| `--log-level trace` | All three streams to stdout |

`VELOCITY_DEBUG_LOG=/tmp/debug.log --log-level diag` routes diag to file, ops to stdout.

## Migration scope

### Package-level stream assignment

| Package       | Current pattern             | Count | Target stream  |
| ------------- | --------------------------- | ----- | -------------- |
| `api/`        | `log.Printf`                | ~15   | `Opsf`/`Diagf` |
| `db/`         | `log.Printf` + emoji        | ~10   | `Opsf`/`Diagf` |
| `serialmux/`  | `log.Printf`                | ~8    | `Opsf`/`Diagf` |
| `monitoring/` | `Logf` (function pointer)   | 1     | — (remove)     |
| `cmd/radar/`  | `log.Printf` + `fmt.Printf` | ~12   | ops/diag       |
| `cmd/tools/*` | `log.Printf` + `fmt.Printf` | ~15   | ops/diag       |

### Stream API location

The `Opsf`/`Diagf`/`Tracef` functions must be accessible to non-LiDAR packages.
Options:

- Promote to a shared package (e.g. `internal/logstreams/`)
- Re-export from `internal/monitoring/`

### Existing design reference

The LiDAR logging stream split and rubric is the source model:
[lidar-logging-stream-split-and-rubric-design.md](../../lidar/architecture/lidar-logging-stream-split-and-rubric-design.md)

That migration is complete for the LiDAR packages (55 call sites across `l1packets`,
`l2frames`, `l3grid`, `pipeline`, `visualiser`).

## What this does not cover

- Structured logging (JSON fields, correlation IDs, external log pipelines)
- LiDAR package logging (already migrated per the rubric design)
- v0.5.x structural issues (covered in the hygiene plan)

## Verification

1. `make lint-go && make test-go` passes
2. `grep -RInE 'log\.Printf|fmt\.Printf|monitoring\.Logf' internal/ cmd/` returns zero
   matches (excluding generated code and test files)
3. `--log-level ops` produces only ops-stream output
4. `--log-level trace` produces all three streams
5. `VELOCITY_DEBUG_LOG` routes correctly
6. `journalctl -u velocity-report` shows clean, prefix-tagged output
