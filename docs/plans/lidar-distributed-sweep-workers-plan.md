# Distributed Sweep Workers

Architectural plan for running parameter sweeps across multiple remote worker machines, coordinated by a single driver unit with a job-submission API and shared filesystem access. Workers run as a mode of the same unified binary — not a separate executable.

**Status:** Proposed (March 2026)
**Layers:** Cross-cutting (L3 Grid, L5 Tracks, L8 Analytics, Platform)
**Related:** [Sweep/HINT Mode](lidar-sweep-hint-mode-plan.md), [Parameter Tuning](lidar-parameter-tuning-optimisation-plan.md), [Analysis Run Infrastructure](lidar-analysis-run-infrastructure-plan.md), [L8/L9/L10 Layers](lidar-l8-analytics-l9-endpoints-l10-clients-plan.md), [Distribution Packaging](deploy-distribution-packaging-plan.md)

## Problem

A single velocity-report instance processes sweep combinations sequentially. A typical multi-parameter sweep with 200 combinations takes 30+ minutes on PCAP replay. With N-dimensional sweeps (noise × closeness × neighbours × tracking parameters), the parameter space grows multiplicatively and wall-clock time becomes the bottleneck.

The existing `SweepBackend` interface already abstracts sensor operations behind an HTTP or in-process boundary — but there is no mechanism to distribute combinations across multiple machines, aggregate results, or manage worker lifecycle.

## Goal

Enable a **driver–worker** topology where:

1. A **driver** (the normal velocity-report server) accepts sweep requests, partitions the parameter space, and dispatches work to N workers.
2. Each **worker** runs the same `velocity-report` binary in `--worker` mode with access to the same PCAP files (shared filesystem), executes its assigned combinations, caches results locally, and reports them back to the driver.
3. The driver aggregates results into a single `SweepState` with the same schema as today's single-machine output.
4. The web dashboard lets users choose whether a sweep runs on the server host or on a specific worker from a **configured list of worker servers** managed under Settings.

Target: 2 workers initially, architecture supports N.

## Principles

1. **Unified binary** — no separate `cmd/sweep-worker` binary. Worker mode is an execution flag (`--worker`) on the same `velocity-report` binary we already ship. This aligns with the single-binary direction in the [distribution packaging plan](deploy-distribution-packaging-plan.md).
2. **Reduced worker surface** — a worker does NOT expose the full web API (no dashboard, no radar endpoints, no report generation). It listens on port 8082 with a minimal HTTP surface: job status, past failures, health, and result retrieval.
3. **Local result cache** — workers cache completed results locally. The driver confirms retrieval before results are scheduled for removal (flagged as retrieved, cleaned up later or when disk space is needed).
4. **Pre-flight validation** — the `/jobs/check` endpoint on the worker confirms that PCAP files are available and readable, processes one frame to validate the configuration, then stops — before the full job kicks off.
5. **Operator-configured workers** — worker hosts are defined via CRUD under the Settings page, not self-registered at runtime. The sweep UI picks from this configured list.

## Current Architecture

```
┌──────────────────────────────────────────────────────┐
│           velocity-report (single machine)           │
│                                                      │
│   SweepRequest                                       │
│       │                                              │
│       ▼                                              │
│   sweep.Runner                                       │
│       │  cartesianProduct(params) → combos[]         │
│       │                                              │
│       │  for each combo:                             │
│       │    ├─ backend.SetTuningParams(combo)         │
│       │    ├─ backend.StartPCAPReplayWithConfig()    │
│       │    ├─ backend.WaitForGridSettle()            │
│       │    ├─ sampler.Sample() × iterations          │
│       │    ├─ computeComboResult()                   │
│       │    └─ state.Results = append(result)         │
│       │                                              │
│       ▼                                              │
│   SweepState { Results: []ComboResult }              │
│       │                                              │
│       ▼                                              │
│   SweepPersister → SQLite (lidar_sweeps)             │
└──────────────────────────────────────────────────────┘
```

**Key interfaces (already exist):**

| Interface               | File                                          | Purpose                                         |
| ----------------------- | --------------------------------------------- | ----------------------------------------------- |
| `SweepBackend`          | `internal/lidar/sweep/backend.go`             | Abstracts sensor/grid/PCAP operations           |
| `SweepPersister`        | `internal/lidar/sweep/runner.go:125`          | Persists sweep lifecycle to SQLite              |
| `SweepRunner`           | `internal/lidar/monitor/sweep_handlers.go:16` | Monitor-layer abstraction (avoids import cycle) |
| `monitor.Client`        | `internal/lidar/monitor/client.go`            | HTTP implementation of `SweepBackend`           |
| `monitor.DirectBackend` | `internal/lidar/monitor/direct_backend.go`    | In-process implementation of `SweepBackend`     |

**Key types (already exist):**

| Type               | Purpose                                               |
| ------------------ | ----------------------------------------------------- |
| `SweepRequest`     | Defines parameters, data source, sampling config      |
| `SweepParam`       | Single parameter dimension (name, type, values/range) |
| `SweepState`       | Status, progress, results array                       |
| `ComboResult`      | Metrics from one parameter combination                |
| `PCAPReplayConfig` | PCAP file, start/duration, speed mode                 |

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
                    │  4. Poll worker /status endpoints │
                    │  5. Retrieve results from workers │
                    │  6. Confirm retrieval → worker    │
                    │     flags results for cleanup     │
                    │  7. Merge into unified SweepState │
                    │  8. Persist to lidar_sweeps       │
                    │                                   │
                    │  SQLite: lidar_sweep_jobs         │
                    │  SQLite: lidar_sweep_workers,CRUD │
                    └──────┬───────────────┬────────────┘
                           │               │
              ┌────────────▼──┐      ┌─────▼────────────┐
              │   WORKER A    │      │   WORKER B       │
              │               │      │                  │
              │  velocity-    │      │  velocity-       │
              │  report       │      │  report          │
              │  --worker     │      │  --worker        │
              │  (:8082)      │      │  (:8082)         │
              │               │      │                  │
              │  Reduced API: │      │  Reduced API:    │
              │  /status      │      │  /status         │
              │  /jobs        │      │  /jobs           │
              │  /jobs/check  │      │  /jobs/check     │
              │  /health      │      │  /health         │
              │               │      │                  │
              │  Local cache: │      │  Local cache:    │
              │  results held │      │  results held    │
              │  until driver │      │  until driver    │
              │  confirms     │      │  confirms        │
              │  retrieval    │      │  retrieval       │
              └───────┬───────┘      └────────┬─────────┘
                      │                       │
              ┌───────▼───────────────────────▼─────────┐
              │         Shared Filesystem (NFS/SMB)     │
              │                                         │
              │  /mnt/pcap/                             │
              │    ├─ site-01/capture-2026-03-10.pcap   │
              │    ├─ site-01/capture-2026-03-11.pcap   │
              │    └─ site-02/capture-2026-03-12.pcap   │
              └─────────────────────────────────────────┘
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

| Method | Path                     | Purpose                                                   |
| ------ | ------------------------ | --------------------------------------------------------- |
| `GET`  | `/health`                | Liveness check (uptime, version, disk space)              |
| `GET`  | `/status`                | Current state: idle, running, job ID, progress            |
| `GET`  | `/jobs`                  | List recent jobs (last 50) with status and timing         |
| `GET`  | `/jobs/{job_id}`         | Single job detail including results if complete           |
| `GET`  | `/jobs/{job_id}/results` | Retrieve cached results for a completed job               |
| `POST` | `/jobs/{job_id}/confirm` | Driver confirms result retrieval; flags for cleanup       |
| `POST` | `/jobs/submit`           | Driver submits a job (combos + sweep config)              |
| `POST` | `/jobs/check`            | Pre-flight: validate PCAP readable, process 1 frame, stop |
| `POST` | `/jobs/{job_id}/cancel`  | Cancel a running job                                      |
| `GET`  | `/failures`              | List past job failures with error details                 |

### Pre-Flight Validation (`/jobs/check`)

Before dispatching a full sweep job, the driver calls `/jobs/check` with the job's `SweepRequest` (PCAP file, sensor config). The worker:

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
2. Driver calls `GET /jobs/{job_id}/results` → worker returns results JSON
3. Driver calls `POST /jobs/{job_id}/confirm` → worker sets `retrieved = TRUE`
4. Background cleanup: results with `retrieved = TRUE` and `retrieved_at` older than 24 hours are deleted
5. Emergency cleanup: if disk usage exceeds a threshold, oldest retrieved results are deleted first

## Data Model

### SweepJob (driver-side)

New table `lidar_sweep_jobs` tracks individual work units:

```sql
CREATE TABLE lidar_sweep_jobs (
    job_id          TEXT PRIMARY KEY,
    sweep_id        TEXT NOT NULL,             -- parent sweep
    worker_id       TEXT,                      -- assigned worker (NULL = unassigned)
    status          TEXT NOT NULL DEFAULT 'pending',  -- pending, assigned, running, complete, failed
    combo_start     INTEGER NOT NULL,          -- first combo index (inclusive)
    combo_end       INTEGER NOT NULL,          -- last combo index (exclusive)
    combos_json     TEXT NOT NULL,             -- JSON: the parameter combinations for this chunk
    results_json    TEXT,                      -- JSON: []ComboResult when complete
    error_message   TEXT,
    assigned_at     DATETIME,
    started_at      DATETIME,
    completed_at    DATETIME,
    heartbeat_at    DATETIME,                  -- last worker heartbeat
    FOREIGN KEY (sweep_id) REFERENCES lidar_sweeps(sweep_id)
);
```

### WorkerServer (driver-side, CRUD)

New table `lidar_sweep_workers` stores configured worker hosts (managed via Settings UI):

```sql
CREATE TABLE lidar_sweep_workers (
    worker_id   TEXT PRIMARY KEY,             -- e.g. "worker-01"
    name        TEXT NOT NULL,                -- display name, e.g. "Lab Pi"
    host        TEXT NOT NULL,                -- hostname or IP, e.g. "192.168.1.42"
    port        INTEGER NOT NULL DEFAULT 8082,-- worker HTTP port
    pcap_root   TEXT NOT NULL DEFAULT '/mnt/pcap', -- shared filesystem mount
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    notes       TEXT,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

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

// CheckResult is returned by the worker /jobs/check pre-flight endpoint.
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

| Component         | Failure Mode                     | Detection                                      | Recovery                                                                 |
| ----------------- | -------------------------------- | ---------------------------------------------- | ------------------------------------------------------------------------ |
| Worker process    | Crash during combo execution     | Heartbeat timeout (configurable, default 60 s) | Driver marks job `failed`, re-queues combos                              |
| Shared filesystem | NFS/SMB mount lost               | Worker PCAP open fails / `/jobs/check` fails   | Job fails with filesystem error; driver reports to user                  |
| Driver process    | Crash mid-sweep                  | On restart, reads `lidar_sweep_jobs`           | Resume: re-queue incomplete jobs, merge completed results                |
| Network partition | Worker cannot reach driver       | Driver poll fails                              | Driver retries; worker holds results in local cache                      |
| Result retrieval  | Driver crashes before confirming | Worker retains cached results                  | Driver re-fetches on restart; worker does not delete unconfirmed results |
| SQLite contention | Concurrent writes from driver    | WAL mode + retry                               | Already handled by existing SQLite configuration                         |
| Combo execution   | PCAP replay timeout              | Existing `WaitForPCAPComplete` timeout         | Job marked failed with error detail; driver re-queues                    |
| Config invalid    | Bad params or corrupt PCAP       | `/jobs/check` pre-flight fails                 | Job never starts; error shown to user immediately                        |

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
  - Pre-flight: calls `POST /jobs/check` on target worker(s) before dispatching
- Worker server CRUD endpoints under `/api/lidar/sweep/workers` (see [API Surface](#api-surface))
  - Wire into existing `WebServer` route registration
  - Test connectivity endpoint calls worker's `/health`
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
  - Periodic poll of worker `/status` endpoints → compute aggregate `SweepState`
  - Reuse existing sweep status polling pattern for dashboard updates
- Pre-flight integration:
  - Before starting a distributed sweep, driver calls `POST /jobs/check` on selected worker
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
  - Worker `/health` endpoint: uptime, current job, last heartbeat, disk free, cache size

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

1. **Unified binary** — worker mode is a flag (`--worker`) on the same binary, not a separate executable. No `cmd/sweep-worker/` directory. This follows the project's [single-binary direction](deploy-distribution-packaging-plan.md).

2. **Privacy preserved** — no data leaves the local network. Workers and driver communicate on the same LAN or VPN. PCAP files stay on the shared filesystem; no cloud upload.

3. **SQLite remains the database** — the driver's SQLite holds all job, sweep, and worker-server state. Workers have local SQLite for operational data and result cache. No external database server required.

4. **Raspberry Pi compatible** — workers can run on Raspberry Pi 4 (ARM64). The worker mode uses the same binary (≤ 30 MB) and should consume ≤ 512 MB RAM during PCAP replay.

5. **Backward compatible** — the single-machine sweep path (`POST /api/lidar/sweep/start` with no `target` field) continues to work unchanged. Distributed sweep is opt-in: configure worker servers in Settings, select a worker target in the sweep UI.

6. **Shared filesystem required** — workers must have read access to the same PCAP directory tree. The driver sends validated relative PCAP paths; workers resolve full paths against their configured `--pcap-root` and reject absolute or `..` paths.

7. **Reduced worker surface** — the worker HTTP server (port 8082) exposes only job lifecycle and health endpoints. No dashboard, no radar, no PDF, no full LiDAR monitor UI.

## Alternatives Considered

### Separate worker binary (`cmd/sweep-worker/`)

**Rejected.** The project is moving toward a single unified binary with subcommands/flags (see [distribution packaging plan](deploy-distribution-packaging-plan.md)). A separate binary adds build targets, deployment complexity, and version-skew risk. Worker mode as a flag (`--worker`) keeps the distribution surface minimal.

### Worker self-registration (no Settings CRUD)

**Rejected.** Self-registration adds a dynamic discovery surface that is harder to reason about and audit. Operator-configured worker servers (CRUD under Settings) are explicit, auditable, and consistent with the existing serial-config and site-config patterns.

### Message queue (Redis, NATS, RabbitMQ)

**Rejected.** Adds an infrastructure dependency that conflicts with the local-first, Raspberry-Pi-deployable constraint. SQLite + HTTP polling achieves the same semantics with zero additional services.

### gRPC streaming (bidirectional)

**Deferred.** The existing gRPC infrastructure serves the macOS visualiser (one-way stream). Bidirectional gRPC for job dispatch would be more efficient than HTTP polling but adds protobuf contract complexity. Can be introduced in a future phase if polling latency becomes a bottleneck.

### SSH-based remote execution

**Rejected.** Running sweep commands over SSH provides no job lifecycle management, no failure recovery, and no result aggregation. The worker agent model is more robust and observable.

### Shared SQLite (multi-writer)

**Rejected.** SQLite does not support concurrent writers from multiple machines. The driver-owns-state model with HTTP result submission avoids this fundamental limitation.

### Full web API on worker

**Rejected.** Workers do not need the dashboard, radar endpoints, or report generation. Exposing the full API surface on workers increases attack surface and resource usage. A reduced surface on port 8082 with only job lifecycle endpoints is sufficient and easier to secure.

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
