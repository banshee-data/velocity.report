# Go Structured Logging Plan (v0.6+)

- **Status:** Draft
- **Layers:** Cross-cutting (Go server, API, database, LiDAR pipeline)
- **Target:** v0.6.0 — unified logging model across the Go codebase
- **Prerequisite plans:**
  [go-codebase-structural-hygiene-plan.md](go-codebase-structural-hygiene-plan.md) (v0.5.x)
- **Existing design:**
  [LiDAR logging stream split and rubric](../lidar/architecture/lidar-logging-stream-split-and-rubric-design.md)

## Motivation

The Go codebase uses three distinct logging mechanisms:

1. `log.Printf()` — standard library, no levels, no structure
2. `fmt.Printf()` — not logging at all, just prints to stdout
3. `monitoring.Logf` — package-level function pointer, replaceable but not structured

Emoji appears in log output (`⚠️ WARNING`). Outside the LiDAR pipeline, logs have no consistent levels and no structured key-value pairs.
There is no correlation between a request and its log lines. On a Raspberry Pi running as a systemd service,
operators use `journalctl` to diagnose problems. Unstructured, unlevel, mixed-destination
logs make diagnosis slower than it needs to be.

The LiDAR pipeline has already addressed this problem within its own boundary. The
[logging stream split and rubric design](../lidar/architecture/lidar-logging-stream-split-and-rubric-design.md)
replaced the single `Debugf` stream with three explicit streams (`Opsf`, `Diagf`, `Tracef`)
routed by a severity × volume rubric. That migration is complete for the LiDAR packages
(55 call sites across `l1packets`, `l2frames`, `l3grid`, `pipeline`, `visualiser`).

This plan extends the same three-stream model to the rest of the Go codebase — the HTTP
API layer, the database package, the serial multiplexer, and the deployment tools — and
wires the streams to a unified output configuration.

## Design Principles

### Align with the LiDAR Rubric

The LiDAR logging rubric defines three streams:

| Stream  | Purpose                                                     | Volume | Retention |
| ------- | ----------------------------------------------------------- | ------ | --------- |
| `ops`   | Actionable warnings/errors and significant lifecycle events | Low    | Longest   |
| `diag`  | Day-to-day diagnostics for troubleshooting and tuning       | Medium | Medium    |
| `trace` | High-frequency packet/frame telemetry and loop-level detail | High   | Shortest  |

The routing rubric is:

1. If it indicates operator action, failure, or data-loss risk → **ops**.
2. Else if it is expected at packet/frame loop frequency → **trace**.
3. Else → **diag**.

This plan applies the same model to non-LiDAR packages. The streams are the same. The
routing rubric is the same. The only difference is that most non-LiDAR code produces `ops`
and `diag` messages; `trace` is rare outside the pipeline.

### Do Not Introduce a Fourth Pattern

The LiDAR packages use `Opsf`/`Diagf`/`Tracef` function-pointer loggers set via
`SetLogWriters`. This plan does not layer `log/slog` on top. The existing stream model is
the target. The migration replaces `log.Printf`, `fmt.Printf`, and `monitoring.Logf` calls
with the appropriate stream function — it does not introduce a new logging framework.

Structured logging (JSON fields, correlation IDs, external log pipeline integration) remains
out of scope, consistent with the LiDAR rubric design document.

### One Configuration Surface

The LiDAR rubric defines two runtime controls:

- `--log-level ops|diag|trace` (CLI flag, default: `ops`) — verbosity threshold
- `VELOCITY_DEBUG_LOG` (env var) — file path for debug output

These controls should apply to the entire process, not just LiDAR packages. The
configuration surface is set once at startup in `cmd/radar/radar.go` and propagated to all
packages.

## Backlog Items

### Item 1: Extend Stream API to Non-LiDAR Packages

**Summary:** Expose the `ops`/`diag`/`trace` stream functions to packages outside
`internal/lidar/`. Migrate `log.Printf`, `fmt.Printf`, and `monitoring.Logf` calls to the
appropriate stream.

**Scope:**

1. **Move or re-export stream API** so that packages outside `internal/lidar/` can call
   `Opsf`/`Diagf`/`Tracef` without importing a LiDAR-internal package. Options:
   - Promote the stream API to a shared package (e.g. `internal/logstreams/`)
   - Re-export from `internal/monitoring/` which already exists as the cross-cutting
     logging package
2. **Migrate `internal/api/`** — replace `log.Printf` calls in HTTP handlers with `Opsf`
   (errors) or `Diagf` (lifecycle/request diagnostics)
3. **Migrate `internal/db/`** — replace `log.Printf` calls with `Opsf` (migration warnings,
   schema sync failures) or `Diagf` (transit worker progress, stats)
4. **Migrate `internal/serialmux/`** — replace `log.Printf` calls with `Opsf` (parse
   errors, dropped data) or `Diagf` (device state changes, connection lifecycle)
5. **Migrate `cmd/radar/`** — replace `log.Printf`/`fmt.Printf` calls in startup code with
   `Opsf` (startup failures) or `Diagf` (configuration summary, version banner)
6. **Retire `monitoring.Logf`** — once all call sites are migrated, remove the function
   pointer and `SetLogger` API
7. **Remove emoji** from all log messages

**Call-site audit (non-LiDAR packages):**

The following is an indicative audit. Exact line numbers will shift as v0.5.x changes land.

| Package          | Current pattern                 | Count | Target stream      |
| ---------------- | ------------------------------- | ----- | ------------------ |
| `api/`           | `log.Printf`                    | ~15   | `Opsf`/`Diagf`     |
| `db/`            | `log.Printf` + emoji            | ~10   | `Opsf`/`Diagf`     |
| `serialmux/`     | `log.Printf`                    | ~8    | `Opsf`/`Diagf`     |
| `monitoring/`    | `Logf` (function pointer)       | 1     | —  (remove)   |
| `cmd/radar/`     | `log.Printf` + `fmt.Printf`    | ~12   | ops/diag      |
| `cmd/deploy/`    | `log.Printf` + `fmt.Printf`    | ~20   | ops/diag      |
| `cmd/tools/*`    | `log.Printf` + `fmt.Printf`    | ~15   | ops/diag      |

**Estimated effort:** 3–5 days. Mechanical migration with clear routing rubric.

**Dependencies:** The v0.5.x package splits (hygiene plan Item 2) should land first so
the migration applies to the final file layout.

---

### Item 2: Unified Stream Configuration

**Summary:** Wire the `--log-level` flag and `VELOCITY_DEBUG_LOG` env var to configure all
streams — LiDAR and non-LiDAR — from a single startup path.

**Scope:**

1. **Centralise writer setup** in `cmd/radar/radar.go`: parse `--log-level`, open
   `VELOCITY_DEBUG_LOG` if set, and call `SetLogWriters` for every package that has one
2. **Propagate to non-LiDAR packages**: call the new shared `SetLogWriters` (or equivalent)
   for `api`, `db`, `serialmux`
3. **Default behaviour**: `--log-level ops` → only ops stream to stdout; `--log-level diag`
   → ops + diag; `--log-level trace` → all three
4. **Verify systemd integration**: `journalctl -u velocity-report` shows clean,
   prefix-tagged output at the configured level
5. **Document** the `--log-level` flag and `VELOCITY_DEBUG_LOG` env var in the setup guide

**Estimated effort:** 1–2 days. The LiDAR packages already support this; this item extends
the wiring to the rest of the process.

**Dependencies:** Item 1 (stream API migration).

---

### Item 3: Test Infrastructure — Flaky Sleep Elimination

**Summary:** Replace `time.Sleep` synchronisation in test files with deterministic polling
helpers.

**Scope:**

1. Add `WaitFor(t, condition func() bool, timeout)` to `internal/testutil/`
2. Migrate the 9 `time.Sleep` test files to use polling helpers
3. Add `SetupTestDB(t) *db.DB` and `CleanupTestDB(t, *db.DB)` as canonical DB test helpers
4. Standardise database test setup — deprecate raw `sql.Open` patterns

**Estimated effort:** 2–3 days. Incremental, no functional changes.

**Dependencies:** None. Can proceed independently of Items 1–2.

---

## Scheduling Recommendation

| Milestone | Items                                       | Rationale                                           |
| --------- | ------------------------------------------- | --------------------------------------------------- |
| v0.6.0    | Item 1 (stream migration), Item 2 (config)  | Unified logging model across the entire Go process. |
| v0.6.0    | Item 3 (test infra)                         | Reduces flaky test risk. Independent of logging.    |

Items 1 and 2 are sequential (2 depends on 1). Item 3 is independent and can proceed in
parallel.

## What This Plan Does Not Cover

- **Structured logging** (JSON fields, correlation IDs, external log pipelines) — the
  LiDAR rubric design explicitly marks this out of scope. If needed later, it can be layered
  on top of the three-stream model without changing call sites.
- **LiDAR package logging** — already migrated per the
  [rubric design](../lidar/architecture/lidar-logging-stream-split-and-rubric-design.md).
  This plan only extends the model to non-LiDAR packages.
- **v0.5.x structural issues** — covered in the
  [hygiene plan](go-codebase-structural-hygiene-plan.md).

## Verification

1. `make lint-go && make test-go` — no regressions
2. `grep -rn 'log\.Printf\|fmt\.Printf\|monitoring\.Logf' internal/ cmd/` returns zero
   matches (excluding generated code and test files that legitimately use `log` for test
   output)
3. `--log-level ops` produces only ops-stream output
4. `--log-level trace` produces ops + diag + trace output
5. `VELOCITY_DEBUG_LOG=/tmp/debug.log --log-level diag` routes diag to file, ops to stdout
6. No `time.Sleep` in test files outside explicitly justified cases
