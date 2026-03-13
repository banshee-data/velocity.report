# Distributed Sweep Workers

Architectural plan for running parameter sweeps across multiple remote worker machines, coordinated by a single driver unit with a job-submission API and shared filesystem access.

**Status:** Proposed (March 2026)
**Layers:** Cross-cutting (L3 Grid, L5 Tracks, L8 Analytics, Platform)
**Related:** [Sweep/HINT Mode](lidar-sweep-hint-mode-plan.md), [Parameter Tuning](lidar-parameter-tuning-optimisation-plan.md), [Analysis Run Infrastructure](lidar-analysis-run-infrastructure-plan.md), [L8/L9/L10 Layers](lidar-l8-analytics-l9-endpoints-l10-clients-plan.md)

## Problem

A single velocity-report instance processes sweep combinations sequentially. A typical multi-parameter sweep with 200 combinations takes 30+ minutes on PCAP replay. With N-dimensional sweeps (noise × closeness × neighbours × tracking parameters), the parameter space grows multiplicatively and wall-clock time becomes the bottleneck.

The existing `SweepBackend` interface already abstracts sensor operations behind an HTTP or in-process boundary — but there is no mechanism to distribute combinations across multiple machines, aggregate results, or manage worker lifecycle.

## Goal

Enable a **driver–worker** topology where:

1. A **driver** accepts sweep requests, partitions the parameter space, and dispatches work to N workers.
2. Each **worker** runs a velocity-report instance with access to the same PCAP files (shared filesystem), executes its assigned combinations, and reports results back.
3. The driver aggregates results into a single `SweepState` with the same schema as today's single-machine output.

Target: 2 workers initially, architecture supports N.

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

| Interface | File | Purpose |
|-----------|------|---------|
| `SweepBackend` | `internal/lidar/sweep/backend.go` | Abstracts sensor/grid/PCAP operations |
| `SweepPersister` | `internal/lidar/sweep/runner.go:125` | Persists sweep lifecycle to SQLite |
| `SweepRunner` | `internal/lidar/monitor/sweep_handlers.go:16` | Monitor-layer abstraction (avoids import cycle) |
| `monitor.Client` | `internal/lidar/monitor/client.go` | HTTP implementation of `SweepBackend` |
| `monitor.DirectBackend` | `internal/lidar/monitor/direct_backend.go` | In-process implementation of `SweepBackend` |

**Key types (already exist):**

| Type | Purpose |
|------|---------|
| `SweepRequest` | Defines parameters, data source, sampling config |
| `SweepParam` | Single parameter dimension (name, type, values/range) |
| `SweepState` | Status, progress, results array |
| `ComboResult` | Metrics from one parameter combination |
| `PCAPReplayConfig` | PCAP file, start/duration, speed mode |

## Target Architecture

```
                    ┌─────────────────────────────────┐
                    │           User / Dashboard       │
                    │     POST /api/lidar/sweep/start   │
                    └────────────────┬────────────────┘
                                     │
                    ┌────────────────▼────────────────┐
                    │         DRIVER (coordinator)     │
                    │                                  │
                    │  1. Expand params → combos[]     │
                    │  2. Partition combos into chunks  │
                    │  3. Create SweepJob per chunk     │
                    │  4. Assign to available workers   │
                    │  5. Poll / receive results        │
                    │  6. Merge into unified SweepState │
                    │  7. Persist to lidar_sweeps       │
                    │                                  │
                    │  SQLite: lidar_sweep_jobs table   │
                    └──────┬───────────────┬──────────┘
                           │               │
              ┌────────────▼──┐      ┌─────▼───────────┐
              │   WORKER A     │      │   WORKER B       │
              │                │      │                  │
              │  velocity-     │      │  velocity-       │
              │  report        │      │  report          │
              │  (headless)    │      │  (headless)      │
              │                │      │                  │
              │  sweep.Runner  │      │  sweep.Runner    │
              │  + DirectBack  │      │  + DirectBack    │
              │                │      │                  │
              └───────┬────────┘      └────────┬─────────┘
                      │                        │
              ┌───────▼────────────────────────▼─────────┐
              │         Shared Filesystem (NFS/SMB)       │
              │                                           │
              │  /mnt/pcap/                               │
              │    ├─ site-01/capture-2026-03-10.pcap     │
              │    ├─ site-01/capture-2026-03-11.pcap     │
              │    └─ site-02/capture-2026-03-12.pcap     │
              └───────────────────────────────────────────┘
```

## Data Model

### SweepJob

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

// WorkerRegistration is sent by a worker when it connects to the driver.
type WorkerRegistration struct {
    WorkerID  string `json:"worker_id"`
    BaseURL   string `json:"base_url"`   // worker's HTTP endpoint
    SensorID  string `json:"sensor_id"`  // sensor this worker manages
    PCAPRoot  string `json:"pcap_root"`  // shared filesystem mount point
}
```

## API Surface

### Driver Endpoints (new)

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/sweep/jobs/submit` | Submit a distributed sweep (creates jobs) |
| `GET` | `/api/sweep/jobs/{sweep_id}` | Get all jobs for a sweep |
| `GET` | `/api/sweep/jobs/{sweep_id}/status` | Aggregated sweep progress |
| `POST` | `/api/sweep/workers/register` | Worker self-registration |
| `GET` | `/api/sweep/workers` | List registered workers |
| `DELETE` | `/api/sweep/workers/{worker_id}` | Deregister a worker |

### Worker Endpoints (new)

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/worker/jobs/claim` | Claim next pending job |
| `POST` | `/api/worker/jobs/{job_id}/heartbeat` | Worker heartbeat |
| `POST` | `/api/worker/jobs/{job_id}/complete` | Submit job results |
| `POST` | `/api/worker/jobs/{job_id}/fail` | Report job failure |

## Failure Registry

| Component | Failure Mode | Detection | Recovery |
|-----------|-------------|-----------|----------|
| Worker process | Crash during combo execution | Heartbeat timeout (configurable, default 60 s) | Driver marks job `failed`, re-queues combos |
| Shared filesystem | NFS/SMB mount lost | Worker PCAP open fails | Job fails with filesystem error; driver re-queues to different worker |
| Driver process | Crash mid-sweep | On restart, reads `lidar_sweep_jobs` | Resume: re-queue incomplete jobs, merge completed results |
| Network partition | Worker cannot reach driver | Heartbeat POST fails | Worker retries with exponential backoff; completes current combo locally |
| SQLite contention | Concurrent writes from driver | WAL mode + retry | Already handled by existing SQLite configuration |
| Combo execution | PCAP replay timeout | Existing `WaitForPCAPComplete` timeout | Job marked failed with error detail; driver re-queues |

## Phased Rollout

### Phase 1: Job Model and Persistence

**Goal:** Define the job data model, create the database migration, and implement the job store — without changing any sweep execution behaviour.

**Scope:**

- Add `lidar_sweep_jobs` table via new migration
- Create `internal/lidar/sweep/job.go` with `SweepJob` type and `JobStore` interface
- Implement `sqlite.JobStore` in `internal/lidar/storage/sqlite/job_store.go`
- CRUD operations: `CreateJob`, `ClaimJob`, `UpdateJobStatus`, `CompleteJob`, `FailJob`, `ListJobsForSweep`, `HeartbeatJob`
- Unit tests for job store

**Key design decisions:**

- Jobs are SQLite rows on the driver — workers never write to the driver's database directly
- Job assignment is pull-based: workers call `ClaimJob` which atomically sets `status = 'assigned'` and `worker_id`
- Each job contains a self-contained `combos_json` so workers need no access to the driver's parameter expansion logic

**Files changed:**

```
internal/db/migrations/000031_sweep_jobs.sql      (new)
internal/lidar/sweep/job.go                        (new — types + interface)
internal/lidar/storage/sqlite/job_store.go         (new — SQLite implementation)
internal/lidar/storage/sqlite/job_store_test.go    (new — tests)
```

**Effort:** S · **Risk:** Low — additive only, no existing behaviour changes

---

### Phase 2: Driver Coordinator and API

**Goal:** Add the coordinator logic that partitions a sweep into jobs and exposes an HTTP API for job lifecycle.

**Scope:**

- Create `internal/lidar/sweep/coordinator.go` with `Coordinator` type
  - `SubmitDistributedSweep(req SweepRequest, workerCount int)` — expands params, partitions combos into N chunks, creates jobs
  - `MergeResults(sweepID string)` — collects completed job results into a unified `SweepState`
  - Partitioning strategy: round-robin combo assignment (combo `i` → worker `i % N`)
- Worker registry: in-memory map of `WorkerRegistration` with health status
- HTTP handlers in `internal/lidar/monitor/distributed_sweep_handlers.go`
  - Wire into existing `WebServer` route registration
- Integration tests: submit a sweep, verify jobs are created with correct combo partitions

**Key design decisions:**

- The coordinator does not execute combos — it only creates jobs and merges results
- Combo partitioning happens after `cartesianProduct()` expansion, reusing existing `sweep_params.go` logic
- The coordinator reuses `SweepPersister` for the parent sweep record; jobs get their own store
- Worker registry is in-memory (not persisted) — workers re-register on restart

**Files changed:**

```
internal/lidar/sweep/coordinator.go                       (new)
internal/lidar/sweep/coordinator_test.go                   (new)
internal/lidar/monitor/distributed_sweep_handlers.go       (new)
internal/lidar/monitor/distributed_sweep_handlers_test.go  (new)
internal/lidar/monitor/webserver.go                        (route registration)
```

**Effort:** M · **Risk:** Low — new endpoints, no changes to existing sweep path

---

### Phase 3: Worker Agent

**Goal:** Build a headless worker binary that claims jobs from the driver, executes sweep combinations locally, and reports results back.

**Scope:**

- Create `cmd/sweep-worker/main.go` — new binary
  - Flags: `--driver-url`, `--sensor-id`, `--pcap-root`, `--worker-id` (auto-generated if omitted)
  - On startup: register with driver, start poll loop
  - Poll loop: `POST /api/worker/jobs/claim` → if job received, execute combos → `POST /api/worker/jobs/{id}/complete`
  - Heartbeat goroutine: `POST /api/worker/jobs/{id}/heartbeat` every 15 s while executing
- Worker uses existing `sweep.Runner` with a `monitor.DirectBackend` pointing at its local sensor pipeline
  - The worker runs a stripped-down velocity-report server (LiDAR pipeline + PCAP replay, no web dashboard)
  - PCAP file paths resolved against `--pcap-root` (shared filesystem mount)
- Worker executes combos sequentially within its assigned chunk (same as current single-machine behaviour)
- Results are `[]ComboResult` serialised as JSON and POSTed back to the driver

**Key design decisions:**

- Workers are stateless — all job state lives on the driver's SQLite
- Workers run their own LiDAR pipeline (L1–L5) for PCAP replay — they need sensor processing, not just an API client
- Each worker has its own local SQLite for `AnalysisRun` records (operational data) but sweep results go back to the driver
- `--pcap-root` must match the shared filesystem mount; the driver sends PCAP filenames (basenames), workers resolve full paths

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
cmd/sweep-worker/main.go            (new — worker binary)
cmd/sweep-worker/worker.go           (new — poll loop, job execution)
cmd/sweep-worker/worker_test.go      (new — unit tests)
Makefile                             (build-sweep-worker target)
```

**Effort:** L · **Risk:** Medium — new binary, depends on LiDAR pipeline initialisation in headless mode

---

### Phase 4: End-to-End Integration and Dashboard

**Goal:** Wire the distributed sweep path into the existing dashboard, add progress aggregation, and validate the full driver–worker–shared-filesystem flow.

**Scope:**

- Dashboard integration (Svelte):
  - New "Distributed Sweep" option in sweep configuration panel
  - Worker status display: registered workers, health, current job
  - Aggregated progress bar: completed combos across all workers / total combos
  - Results merge: display unified `ComboResult[]` from all workers, identical to single-machine output
- Driver-side progress aggregation:
  - Periodic poll of job statuses → compute aggregate `SweepState`
  - WebSocket or SSE push for real-time progress (reuse existing sweep status polling pattern)
- End-to-end integration test:
  - Spin up driver + 2 worker instances (in-process or Docker)
  - Submit a 20-combo sweep on a test PCAP
  - Verify: combos partitioned, workers execute, results merged, final `SweepState` matches single-machine baseline
- Operational documentation:
  - Setup guide: NFS share configuration, worker deployment, firewall rules
  - Troubleshooting: common failure modes and recovery

**Files changed:**

```
web/src/lib/api.ts                                         (new API calls)
web/src/lib/types/lidar.ts                                 (SweepJob, WorkerStatus types)
web/src/routes/lidar/sweeps/+page.svelte                   (distributed sweep UI)
internal/lidar/monitor/distributed_sweep_handlers.go       (progress aggregation)
docs/lidar/operations/distributed-sweep-setup.md           (new — operational guide)
```

**Effort:** L · **Risk:** Medium — cross-component integration, UI changes

---

### Phase 5: Resilience and Operational Hardening

**Goal:** Handle real-world failure modes: worker crashes, network partitions, stale jobs, and graceful degradation.

**Scope:**

- Heartbeat timeout and job reassignment:
  - Driver goroutine scans for stale jobs (no heartbeat within configurable timeout)
  - Stale jobs reset to `pending` with incremented retry counter
  - Maximum retry limit (default 3) before permanent failure
- Graceful worker shutdown:
  - SIGTERM handler: complete current combo, report partial results, deregister
  - Partial results: driver accepts `ComboResult[]` for completed combos within a chunk even if the chunk is incomplete
- Driver crash recovery:
  - On startup, scan `lidar_sweep_jobs` for in-progress sweeps
  - Re-queue `assigned`/`running` jobs that have no recent heartbeat
  - Merge any completed job results into the parent sweep
- Observability:
  - Structured logging for job lifecycle events (created, claimed, completed, failed, reassigned)
  - Metrics: jobs_pending, jobs_running, jobs_completed, jobs_failed per sweep
  - Worker health endpoint: `/api/worker/health` returns uptime, current job, last heartbeat

**Files changed:**

```
internal/lidar/sweep/coordinator.go            (heartbeat scanner, crash recovery)
internal/lidar/sweep/coordinator_test.go       (failure mode tests)
cmd/sweep-worker/worker.go                     (graceful shutdown, partial results)
internal/lidar/monitor/distributed_sweep_handlers.go  (health endpoint)
```

**Effort:** M · **Risk:** Low — defensive additions, no happy-path changes

---

## Phase Dependencies

```
Phase 1 ─── Phase 2 ─── Phase 3 ─── Phase 4
  (model)    (driver)    (worker)    (dashboard)
                                        │
                                    Phase 5
                                   (hardening)
```

Phases 1–3 are strictly sequential. Phase 4 (dashboard) and Phase 5 (hardening) can overlap once Phase 3 is functional.

## Design Constraints

1. **Privacy preserved** — no data leaves the local network. Workers and driver communicate on the same LAN or VPN. PCAP files stay on the shared filesystem; no cloud upload.

2. **SQLite remains the database** — the driver's SQLite holds all job and sweep state. Workers have local SQLite for operational data. No external database server required.

3. **Raspberry Pi compatible** — workers can run on Raspberry Pi 4 (ARM64). The headless worker binary should be ≤ 30 MB and consume ≤ 512 MB RAM during PCAP replay.

4. **Backward compatible** — the single-machine sweep path (`POST /api/lidar/sweep/start`) continues to work unchanged. Distributed sweep is an opt-in mode via the new API.

5. **Shared filesystem required** — workers must have read access to the same PCAP directory tree. The driver sends PCAP basenames; workers resolve full paths against their configured `--pcap-root`.

## Alternatives Considered

### Message queue (Redis, NATS, RabbitMQ)

**Rejected.** Adds an infrastructure dependency that conflicts with the local-first, Raspberry-Pi-deployable constraint. SQLite + HTTP polling achieves the same semantics with zero additional services.

### gRPC streaming (bidirectional)

**Deferred.** The existing gRPC infrastructure serves the macOS visualiser (one-way stream). Bidirectional gRPC for job dispatch would be more efficient than HTTP polling but adds protobuf contract complexity. Can be introduced in a future phase if polling latency becomes a bottleneck.

### SSH-based remote execution

**Rejected.** Running sweep commands over SSH provides no job lifecycle management, no failure recovery, and no result aggregation. The worker agent model is more robust and observable.

### Shared SQLite (multi-writer)

**Rejected.** SQLite does not support concurrent writers from multiple machines. The driver-owns-state model with HTTP result submission avoids this fundamental limitation.

## Migration Path

Existing single-machine deployments are unaffected:

- No schema changes affect existing tables (new `lidar_sweep_jobs` table only)
- No existing API endpoints change behaviour
- The new `sweep-worker` binary is optional — install only on worker machines
- The driver functionality is opt-in — existing `velocity-report` binary gains coordinator capabilities but does not activate them unless workers register

## Open Questions

1. **Auto-discovery vs. manual registration** — should workers discover the driver via mDNS/Bonjour, or is manual `--driver-url` configuration sufficient for 2-worker deployments?

2. **Result storage location** — should merged results write to the driver's SQLite only, or should each worker also persist its chunk locally for offline inspection?

3. **Heterogeneous workers** — if workers have different hardware (Pi 4 vs. x86 workstation), should the coordinator weight combo assignment by worker capability?

4. **Auto-tune compatibility** — the `AutoTuner` and `HINTRunner` use iterative refinement where each round's bounds depend on the previous round's results. Distributing within a round is safe; distributing across rounds requires sequential aggregation. Phase 2 coordinator should handle this explicitly.
