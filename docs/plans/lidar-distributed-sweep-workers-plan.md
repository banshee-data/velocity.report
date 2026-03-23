# Distributed Sweep Workers

- **Canonical:** [distributed-sweep.md](../lidar/architecture/distributed-sweep.md)

Architectural plan for running parameter sweeps across multiple remote worker machines, coordinated by a single driver unit with a job-submission API and shared filesystem access. Workers run as a mode of the same unified binary — not a separate executable.

**Status:** Proposed (March 2026)
**Layers:** Cross-cutting (L3 Grid, L5 Tracks, L8 Analytics, Platform)
**Related:** [Sweep/HINT Mode](lidar-sweep-hint-mode-plan.md), [Parameter Tuning](lidar-parameter-tuning-optimisation-plan.md), [Analysis Run Infrastructure](lidar-analysis-run-infrastructure-plan.md), [L8/L9/L10 Layers](lidar-l8-analytics-l9-endpoints-l10-clients-plan.md), [Distribution Packaging](deploy-distribution-packaging-plan.md)

> **Problem, goal, design principles, and current architecture:** see [distributed-sweep.md](../lidar/architecture/distributed-sweep.md).

## Target Architecture

```
                    ┌───────────────────────────────────┐
                    │        User / Svelte Dashboard    │
                    │  POST /api/lidar/sweep/start      │
                    │  target: "server" | "worker-01"   │
                    └────────────────┬──────────────────┘
                                     │
                    ┌────────────────▼──────────────────┐
                    │    DRIVER  (velocity-report)      │
                    │    normal server mode (:8080)     │
                    │                                   │
                    │  1. Expand params → combos[]      │
                    │  2. Partition combos into jobs    │
                    │  3. Dispatch to target worker(s)  │
                    │  4. Poll worker /api/worker/statu │
                    │  5. Retrieve results from workers │
                    │  6. Confirm retrieval → worker    │
                    │     flags results for cleanup     │
                    │  7. Merge into unified SweepState │
                    │  8. Persist to lidar_sweeps       │
                    │                                   │
                    │  SQLite: lidar_sweep_jobs         │
                    │  SQLite: lidar_sweep_workers,CRUD │
                    └──────┬──────────────────────┬─────┘
                           │                      │
              ┌────────────▼──────────────┐  ┌────▼──────────────────────┐
              │   WORKER A                │  │   WORKER B                │
              │                           │  │                           │
              │  velocity-report --worker │  │  velocity-report --worker │
              │  (:8082)                  │  │  (:8082)                  │
              │                           │  │                           │
              │  /health                  │  │  /health                  │
              │  /api/worker/status       │  │  /api/worker/status       │
              │  /api/worker/jobs         │  │  /api/worker/jobs         │
              │  /api/worker/jobs/check   │  │  /api/worker/jobs/check   │
              │  /api/worker/failures     │  │  /api/worker/failures     │
              │                           │  │                           │
              │  Local cache:             │  │  Local cache:             │
              │  results held until       │  │  results held until       │
              │  driver confirms          │  │  driver confirms          │
              │  retrieval                │  │  retrieval                │
              └─────────────┬─────────────┘  └─────────────┬─────────────┘
                            │                              │
              ┌─────────────▼──────────────────────────────▼─────────────┐
              │             Shared Filesystem (NFS/SMB)                  │
              │                                                          │
              │  /mnt/pcap/                                              │
              │    ├─ site-01/capture-2026-03-10.pcap                    │
              │    ├─ site-01/capture-2026-03-11.pcap                    │
              │    └─ site-02/capture-2026-03-12.pcap                    │
              └──────────────────────────────────────────────────────────┘
```

## Worker Execution Mode

The worker is the same `velocity-report` binary started with `--worker`:

```bash
velocity-report --worker \
    --driver-url http://192.168.1.10:8080 \
    --pcap-root /mnt/pcap \
    --worker-listen :8082
```

**What the worker runs:**

- LiDAR pipeline (L1–L5) for PCAP replay and combo execution
- Minimal HTTP server on port 8082 (reduced surface)
- Local SQLite for `AnalysisRun` records and result cache

**What the worker does NOT run:**

- No radar serial handler
- No full web dashboard (no port 8080 / 8081 UI)
- No PDF report generation
- No transit worker
- No gRPC visualiser server

### Worker HTTP Surface (port 8082)

> Endpoint table: see [distributed-sweep.md § Worker HTTP Surface](../lidar/architecture/distributed-sweep.md#worker-http-surface-port-8082).

### Pre-Flight Validation (`/api/worker/jobs/check`)

Before dispatching a full sweep job, the driver calls `/api/worker/jobs/check` with the job's `SweepRequest` (PCAP file, sensor config). The worker:

1. Resolves the PCAP file path against `--pcap-root`
2. Confirms the file exists and is readable
3. Initialises the LiDAR pipeline with the specified tuning parameters
4. Replays exactly one frame to verify the configuration produces valid output
5. Tears down the pipeline
6. Returns a `CheckResult`:

```go
type CheckResult struct {
    OK           bool   `json:"ok"`
    PCAPReadable bool   `json:"pcap_readable"`
    FrameValid   bool   `json:"frame_valid"`
    ErrorMessage string `json:"error_message,omitempty"`
    DiskFreeMB   int64  `json:"disk_free_mb"`
}
```

If the check fails, the driver does not submit the job and reports the error to the user.

### Local Result Cache

Workers keep completed results in local SQLite until the driver confirms retrieval:

```sql
CREATE TABLE worker_result_cache (
    job_id          TEXT PRIMARY KEY,
    sweep_id        TEXT NOT NULL,
    results_json    TEXT NOT NULL,
    completed_at    DATETIME NOT NULL,
    retrieved       BOOLEAN NOT NULL DEFAULT FALSE,
    retrieved_at    DATETIME,
    size_bytes      INTEGER NOT NULL DEFAULT 0
);
```

**Lifecycle:**

1. Job completes → results written to `worker_result_cache`
2. Driver calls `GET /api/worker/jobs/{job_id}/results` → worker returns results JSON
3. Driver calls `POST /api/worker/jobs/{job_id}/confirm` → worker sets `retrieved = TRUE`
4. Background cleanup: results with `retrieved = TRUE` and `retrieved_at` older than 24 hours are deleted
5. Emergency cleanup: if disk usage exceeds a threshold, oldest retrieved results are deleted first

## Data Model

> SQL schema (`lidar_sweep_jobs`, `lidar_sweep_workers`): see [distributed-sweep.md § Data Model](../lidar/architecture/distributed-sweep.md#data-model).

### Go Types

```go
// SweepJob represents a unit of work assigned to a worker.
type SweepJob struct {
    JobID        string          `json:"job_id"`
    SweepID      string          `json:"sweep_id"`
    WorkerID     string          `json:"worker_id,omitempty"`
    Status       string          `json:"status"`        // pending, assigned, running, complete, failed
    ComboStart   int             `json:"combo_start"`
    ComboEnd     int             `json:"combo_end"`
    Combos       json.RawMessage `json:"combos"`        // []map[string]interface{}
    Results      json.RawMessage `json:"results,omitempty"`
    ErrorMessage string          `json:"error_message,omitempty"`
    AssignedAt   *time.Time      `json:"assigned_at,omitempty"`
    StartedAt    *time.Time      `json:"started_at,omitempty"`
    CompletedAt  *time.Time      `json:"completed_at,omitempty"`
    HeartbeatAt  *time.Time      `json:"heartbeat_at,omitempty"`
}

// WorkerServer is a configured remote worker host (persisted in settings).
type WorkerServer struct {
    WorkerID  string `json:"worker_id"`
    Name      string `json:"name"`       // display name, e.g. "Lab Pi"
    Host      string `json:"host"`       // hostname or IP
    Port      int    `json:"port"`       // worker HTTP port (default 8082)
    PCAPRoot  string `json:"pcap_root"`  // shared filesystem mount point
    Enabled   bool   `json:"enabled"`
    Notes     string `json:"notes,omitempty"`
}

// CheckResult is returned by the worker /api/worker/jobs/check pre-flight endpoint.
type CheckResult struct {
    OK           bool   `json:"ok"`
    PCAPReadable bool   `json:"pcap_readable"`
    FrameValid   bool   `json:"frame_valid"`
    ErrorMessage string `json:"error_message,omitempty"`
    DiskFreeMB   int64  `json:"disk_free_mb"`
}
```

## API Surface

### Driver Endpoints (new)

**Job lifecycle (under existing :8080 API):**

| Method | Path                                      | Purpose                                                           |
| ------ | ----------------------------------------- | ----------------------------------------------------------------- |
| `POST` | `/api/lidar/sweep/start`                  | Extended: accepts optional `target` field ("server" or worker ID) |
| `GET`  | `/api/lidar/sweep/jobs/{sweep_id}`        | Get all jobs for a sweep                                          |
| `GET`  | `/api/lidar/sweep/jobs/{sweep_id}/status` | Aggregated sweep progress                                         |

**Worker server CRUD (Settings):**

| Method   | Path                                        | Purpose                                  |
| -------- | ------------------------------------------- | ---------------------------------------- |
| `GET`    | `/api/lidar/sweep/workers`                  | List configured worker servers           |
| `GET`    | `/api/lidar/sweep/workers/{worker_id}`      | Get single worker server                 |
| `POST`   | `/api/lidar/sweep/workers`                  | Create a worker server entry             |
| `PUT`    | `/api/lidar/sweep/workers/{worker_id}`      | Update a worker server entry             |
| `DELETE` | `/api/lidar/sweep/workers/{worker_id}`      | Delete a worker server entry             |
| `POST`   | `/api/lidar/sweep/workers/{worker_id}/test` | Test connectivity + run pre-flight check |

### Worker Endpoints (port 8082)

See [Worker HTTP Surface](#worker-http-surface-port-8082) above.

## Failure Registry

> Failure mode table: see [distributed-sweep.md § Failure Registry](../lidar/architecture/distributed-sweep.md#failure-registry).

## Phased Rollout

### Phase 1: Job Model, Worker Server CRUD, and Persistence

**Goal:** Define the data model for jobs and worker servers, create database migrations, and implement the stores — without changing any sweep execution behaviour.

**Scope:**

- Add `lidar_sweep_jobs` table via new migration
- Add `lidar_sweep_workers` table via same migration (configured worker hosts)
- Create `internal/lidar/sweep/job.go` with `SweepJob` type and `JobStore` interface
- Create `internal/lidar/sweep/worker_server.go` with `WorkerServer` type and `WorkerServerStore` interface
- Implement `sqlite.JobStore` in `internal/lidar/storage/sqlite/job_store.go`
- Implement `sqlite.WorkerServerStore` in `internal/lidar/storage/sqlite/worker_server_store.go`
- Job CRUD: `CreateJob`, `ClaimJob`, `UpdateJobStatus`, `CompleteJob`, `FailJob`, `ListJobsForSweep`, `HeartbeatJob`
- Worker server CRUD: `CreateWorker`, `GetWorker`, `UpdateWorker`, `DeleteWorker`, `ListWorkers`, `ListEnabledWorkers`
- Unit tests for both stores

**Key design decisions:**

- Jobs are SQLite rows on the driver — workers never write to the driver's database directly
- Worker servers are operator-configured (not self-registered) — added via Settings UI or API, persisted in SQLite
- Each job contains a self-contained `combos_json` so workers need no access to the driver's parameter expansion logic

**Files changed:**

```
internal/db/migrations/000031_sweep_workers_and_jobs.sql       (new)
internal/lidar/sweep/job.go                                     (new — types + interface)
internal/lidar/sweep/worker_server.go                           (new — types + interface)
internal/lidar/storage/sqlite/job_store.go                      (new — SQLite implementation)
internal/lidar/storage/sqlite/job_store_test.go                 (new — tests)
internal/lidar/storage/sqlite/worker_server_store.go            (new — SQLite implementation)
internal/lidar/storage/sqlite/worker_server_store_test.go       (new — tests)
```

**Effort:** S · **Risk:** Low — additive only, no existing behaviour changes

---

### Phase 2: Driver Coordinator, Worker Server API, and Settings UI

**Goal:** Add the coordinator logic that partitions a sweep into jobs, the CRUD API for worker servers, and the Settings UI for managing worker hosts.

**Scope:**

- Create `internal/lidar/sweep/coordinator.go` with `Coordinator` type
  - `SubmitDistributedSweep(req SweepRequest, workerID string)` — expands params, partitions combos into N chunks, creates jobs, dispatches to worker(s)
  - `MergeResults(sweepID string)` — retrieves completed job results from workers, confirms retrieval, merges into a unified `SweepState`
  - Partitioning strategy: round-robin combo assignment (combo `i` → worker `i % N`)
  - Pre-flight: calls `POST /api/worker/jobs/check` on target worker(s) before dispatching
- Worker server CRUD endpoints under `/api/lidar/sweep/workers` (see [API Surface](#api-surface))
  - Wire into existing `WebServer` route registration
  - Test connectivity endpoint calls worker's `GET /health`
- Settings UI (Svelte):
  - New "Sweep Workers" section under `/settings`
  - Table listing configured workers: name, host, port, enabled, status (reachable/unreachable)
  - Add / Edit / Delete workers (modal form, follows existing serial-config pattern)
  - "Test Connection" button per worker — calls `/api/lidar/sweep/workers/{id}/test`
- Integration tests: submit a sweep, verify jobs are created with correct combo partitions

**Key design decisions:**

- The coordinator does not execute combos — it only creates jobs, dispatches them, and merges results
- Combo partitioning happens after `cartesianProduct()` expansion, reusing existing `sweep_params.go` logic
- The coordinator reuses `SweepPersister` for the parent sweep record; jobs get their own store
- Worker servers are database-backed (not in-memory) — survive restarts, editable via UI

**Files changed:**

```
internal/lidar/sweep/coordinator.go                       (new)
internal/lidar/sweep/coordinator_test.go                   (new)
internal/lidar/monitor/worker_server_handlers.go           (new — CRUD endpoints)
internal/lidar/monitor/worker_server_handlers_test.go      (new)
internal/lidar/monitor/webserver.go                        (route registration)
web/src/lib/api.ts                                         (worker server API calls)
web/src/lib/types/lidar.ts                                 (WorkerServer type)
web/src/routes/(constrained)/settings/+page.svelte         (worker CRUD section)
```

**Effort:** M · **Risk:** Low — new endpoints and UI section, no changes to existing sweep path

---

### Phase 3: Worker Mode in Unified Binary

**Goal:** Add `--worker` mode to the existing `velocity-report` binary. The worker runs a stripped-down server with a reduced HTTP surface on port 8082, executes sweep jobs, and caches results locally.

**Scope:**

- Add `--worker` flag to `cmd/radar/radar.go`
  - When set: skip radar init, skip dashboard, skip gRPC visualiser, skip transit worker
  - Start LiDAR pipeline (L1–L5) for PCAP replay capability
  - Start minimal HTTP server on `--worker-listen` (default `:8082`)
  - Use driver at `--driver-url` for job polling and result reporting (worker identity is configured via Settings)
- Create `internal/lidar/worker/` package:
  - `server.go` — minimal HTTP server implementing the [worker endpoint table](#worker-http-surface-port-8082)
  - `executor.go` — poll loop: accept job from driver, execute combos via `sweep.Runner` + `DirectBackend`, cache results
  - `cache.go` — local result cache (`worker_result_cache` table in worker's local SQLite)
  - `check.go` — pre-flight validation: resolve PCAP, init pipeline, process 1 frame, tear down
- Worker flags:
  - `--worker` — enable worker mode
  - `--driver-url` — driver's HTTP address (e.g. `http://192.168.1.10:8080`)
  - `--pcap-root` — shared filesystem mount (e.g. `/mnt/pcap`)
  - `--worker-listen` — HTTP listen address (default `:8082`)
  - `--worker-id` — optional; auto-generated from hostname if omitted
- Heartbeat goroutine: POSTs to driver `/api/lidar/sweep/jobs/{id}/heartbeat` every 15 s while executing
- SIGTERM handler: complete current combo, cache partial results, shut down cleanly

**Key design decisions:**

- No separate binary — `--worker` is a flag on the existing `cmd/radar/radar.go` entry point, consistent with `--enable-lidar`, `--disable-radar`, `--debug`
- Worker is not stateless — it caches results locally until the driver confirms retrieval. This survives transient network issues and driver restarts.
- Workers run their own LiDAR pipeline (L1–L5) for PCAP replay — they need sensor processing, not just an API client
- `--pcap-root` must match the shared filesystem mount; the driver sends validated relative PCAP paths from this root (e.g. `site-01/capture-2026-03-10.pcap`), workers reject absolute/`..` paths and resolve full paths under `--pcap-root`

**Shared filesystem layout:**

```
/mnt/pcap/                          ← mount point (NFS, SMB, or sshfs)
├── site-01/
│   ├── capture-2026-03-10.pcap
│   └── capture-2026-03-11.pcap
└── site-02/
    └── capture-2026-03-12.pcap
```

**Files changed:**

```
cmd/radar/radar.go                          (--worker flag, mode branching)
internal/lidar/worker/server.go             (new — minimal HTTP server)
internal/lidar/worker/executor.go           (new — job execution + poll loop)
internal/lidar/worker/cache.go              (new — local result cache)
internal/lidar/worker/check.go              (new — pre-flight validation)
internal/lidar/worker/server_test.go        (new — unit tests)
internal/lidar/worker/executor_test.go      (new — unit tests)
internal/lidar/worker/cache_test.go         (new — unit tests)
```

**Effort:** L · **Risk:** Medium — modifies main binary, depends on LiDAR pipeline initialisation in headless mode

---

### Phase 4: End-to-End Integration and Dashboard

**Goal:** Wire the distributed sweep path into the existing sweep dashboard, add worker selection, progress aggregation, and validate the full driver–worker–shared-filesystem flow.

**Scope:**

- Sweep UI changes (Svelte):
  - **Target selector** in sweep configuration panel: "Run on: Server (local) / Worker-01 / Worker-02 / ..."
    - Dropdown populated from `/api/lidar/sweep/workers` (enabled workers only)
    - Default: "Server (local)" — runs sweep locally as today
    - Selecting a worker dispatches via the coordinator
  - Worker status badges: show each worker's current state (idle / running / error) next to its name
  - Aggregated progress bar: completed combos across all workers / total combos
  - Results merge: display unified `ComboResult[]` from all workers, identical to single-machine output
- Driver-side progress aggregation:
  - Periodic poll of worker `/api/worker/status` endpoints → compute aggregate `SweepState`
  - Reuse existing sweep status polling pattern for dashboard updates
- Pre-flight integration:
  - Before starting a distributed sweep, driver calls `POST /api/worker/jobs/check` on selected worker
  - If check fails, sweep is not started and error is shown in the UI
- End-to-end integration test:
  - Spin up driver + 2 worker instances (in-process, using `--worker` flag)
  - Submit a 20-combo sweep on a test PCAP
  - Verify: combos partitioned, pre-flight passes, workers execute, results cached, driver retrieves and confirms, final `SweepState` matches single-machine baseline
- Operational documentation:
  - Setup guide: NFS share configuration, worker deployment, firewall rules
  - Troubleshooting: common failure modes and recovery

**Files changed:**

```
web/src/lib/api.ts                                         (sweep target, worker status calls)
web/src/lib/types/lidar.ts                                 (SweepJob, WorkerStatus types)
web/src/routes/lidar/sweeps/+page.svelte                   (worker target selector)
internal/lidar/monitor/distributed_sweep_handlers.go       (new — progress aggregation)
internal/lidar/monitor/distributed_sweep_handlers_test.go  (new)
docs/lidar/operations/distributed-sweep-setup.md           (new — operational guide)
```

**Effort:** L · **Risk:** Medium — cross-component integration, UI changes

---

### Phase 5: Resilience and Operational Hardening

**Goal:** Handle real-world failure modes: worker crashes, network partitions, stale jobs, result cache lifecycle, and graceful degradation.

**Scope:**

- Heartbeat timeout and job reassignment:
  - Driver goroutine scans for stale jobs (no heartbeat within configurable timeout)
  - Stale jobs reset to `pending` with incremented retry counter
  - Maximum retry limit (default 3) before permanent failure
- Graceful worker shutdown:
  - SIGTERM handler: complete current combo, cache partial results, shut down cleanly
  - Partial results: driver accepts `ComboResult[]` for completed combos within a chunk even if the chunk is incomplete
- Result cache lifecycle:
  - Retrieved results: cleaned up after 24 hours (configurable)
  - Emergency cleanup: if disk usage exceeds threshold, oldest retrieved results are deleted first
  - Unretrieved results: retained indefinitely until driver confirms or operator manually clears
- Driver crash recovery:
  - On startup, scan `lidar_sweep_jobs` for in-progress sweeps
  - Re-query workers for cached results from incomplete sweeps
  - Merge any completed job results into the parent sweep
- Observability:
  - Structured logging for job lifecycle events (created, dispatched, preflight_passed, running, completed, retrieved, failed, reassigned)
  - Metrics: jobs_pending, jobs_running, jobs_completed, jobs_failed per sweep
  - Worker `GET /health` endpoint: uptime, current job, last heartbeat, disk free, cache size

**Files changed:**

```
internal/lidar/sweep/coordinator.go            (heartbeat scanner, crash recovery)
internal/lidar/sweep/coordinator_test.go       (failure mode tests)
internal/lidar/worker/executor.go              (graceful shutdown, partial results)
internal/lidar/worker/cache.go                 (cleanup policies)
internal/lidar/monitor/distributed_sweep_handlers.go  (health aggregation)
```

**Effort:** M · **Risk:** Low — defensive additions, no happy-path changes

---

## Phase Dependencies

```
Phase 1 ─── Phase 2 ─── Phase 3 ─── Phase 4
 (model +    (coord +    (worker     (sweep UI +
  CRUD        settings    mode in      e2e)
  stores)     UI)         binary)
                                        │
                                    Phase 5
                                   (hardening)
```

Phases 1–3 are strictly sequential. Phase 4 (dashboard) and Phase 5 (hardening) can overlap once Phase 3 is functional.

## Design Constraints

> Constraint list: see [distributed-sweep.md § Design Constraints](../lidar/architecture/distributed-sweep.md#design-constraints).

## Alternatives Considered

> Rejected/deferred alternatives: see [distributed-sweep.md § Alternatives Rejected](../lidar/architecture/distributed-sweep.md#alternatives-rejected).

## Migration Path

Existing single-machine deployments are unaffected:

- No schema changes affect existing tables (new `lidar_sweep_jobs` and `lidar_sweep_workers` tables only)
- No existing API endpoints change behaviour
- Worker mode is opt-in: only activated when `--worker` flag is passed
- The coordinator is opt-in: only activates when a sweep targets a configured worker (not "server")
- Settings UI gains a new "Sweep Workers" section but does not alter existing settings

## Open Questions

1. **Auto-discovery vs. manual only** — should workers be discoverable via mDNS/Bonjour in addition to manual configuration, or is the Settings CRUD sufficient for small deployments?

2. **Heterogeneous workers** — if workers have different hardware (Pi 4 vs. x86 workstation), should the coordinator weight combo assignment by worker capability, or is round-robin sufficient?

3. **Auto-tune compatibility** — the `AutoTuner` and `HINTRunner` use iterative refinement where each round's bounds depend on the previous round's results. Distributing within a round is safe; distributing across rounds requires sequential aggregation. Phase 2 coordinator should handle this explicitly.

4. **Worker port conflict** — port 8082 is chosen to avoid colliding with 8080 (main API) and 8081 (LiDAR monitor). If a machine runs both driver and worker (testing scenario), these ports coexist. Confirm this is acceptable or make the worker port configurable via `--worker-listen`.
