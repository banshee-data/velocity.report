# Distributed Sweep Workers

Active plan: [lidar-distributed-sweep-workers-plan.md](../../plans/lidar-distributed-sweep-workers-plan.md)

**Status:** Proposed

Driver–worker architecture for distributing LiDAR parameter sweep workloads across multiple machines to reduce wall-clock evaluation time.

## Problem

A single velocity-report instance processes sweep combinations sequentially.
A typical multi-parameter sweep with 200 combinations takes 30+ minutes on
PCAP replay. N-dimensional sweeps grow multiplicatively.

## Architecture: Driver–Worker Topology

```
           Svelte Dashboard (:8080)
                   │
           ┌───────▼────────┐
           │  DRIVER        │  velocity-report (normal mode)
           │  Expand params │  SQLite: lidar_sweep_jobs,
           │  Partition     │          lidar_sweep_workers
           │  Dispatch      │
           └──┬────────┬────┘
              │        │
       ┌──────▼─┐ ┌────▼────┐
       │WORKER A│ │WORKER B │  velocity-report --worker (:8082)
       └───┬────┘ └────┬────┘
           │           │
       ┌───▼───────────▼────┐
       │ Shared Filesystem  │  /mnt/pcap/ (NFS/SMB)
       └────────────────────┘
```

## Design Principles

1. **Unified binary** — worker mode is `--worker` on the same `velocity-report`
   binary. No separate `cmd/sweep-worker/`. Aligns with single-binary
   direction in distribution-packaging.

2. **Reduced worker surface** — worker port 8082 exposes only job lifecycle
   and health endpoints. No dashboard, radar, PDF, or full LiDAR monitor.

3. **Local result cache** — workers cache completed results in local SQLite
   until the driver confirms retrieval.

4. **Pre-flight validation** — `/api/worker/jobs/check` confirms PCAP
   readable, processes one frame, and reports before the full job runs.

5. **Operator-configured workers** — worker hosts defined via Settings UI
   CRUD, not self-registered at runtime.

## Worker HTTP surface (port 8082)

| Method | Path                            | Purpose                                |
|--------|---------------------------------|----------------------------------------|
| GET    | `/health`                       | Liveness (uptime, version, disk)       |
| GET    | `/api/worker/status`            | Current state + job progress           |
| GET    | `/api/worker/jobs`              | Recent jobs (last 50)                  |
| GET    | `/api/worker/jobs/{id}`         | Single job detail                      |
| GET    | `/api/worker/jobs/{id}/results` | Retrieve cached results                |
| POST   | `/api/worker/jobs/{id}/confirm` | Confirm retrieval → flag for cleanup   |
| POST   | `/api/worker/jobs/submit`       | Submit a job                           |
| POST   | `/api/worker/jobs/check`        | Pre-flight validation                  |
| POST   | `/api/worker/jobs/{id}/cancel`  | Cancel a running job                   |
| GET    | `/api/worker/failures`          | Past job failures                      |

## Data Model

### lidar_sweep_jobs (driver-side)

```sql
CREATE TABLE lidar_sweep_jobs (
    job_id       TEXT PRIMARY KEY,
    sweep_id     TEXT NOT NULL,
    worker_id    TEXT,
    status       TEXT NOT NULL DEFAULT 'pending',
    combo_start  INTEGER NOT NULL,
    combo_end    INTEGER NOT NULL,
    combos_json  TEXT NOT NULL,
    results_json TEXT,
    error_message TEXT,
    assigned_at  DATETIME,
    started_at   DATETIME,
    completed_at DATETIME,
    heartbeat_at DATETIME,
    FOREIGN KEY (sweep_id) REFERENCES lidar_sweeps(sweep_id)
);
```

### lidar_sweep_workers (driver-side, CRUD)

```sql
CREATE TABLE lidar_sweep_workers (
    worker_id  TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    host       TEXT NOT NULL,
    port       INTEGER NOT NULL DEFAULT 8082,
    pcap_root  TEXT NOT NULL DEFAULT '/mnt/pcap',
    enabled    BOOLEAN NOT NULL DEFAULT TRUE,
    notes      TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

### worker_result_cache (worker-side)

Results held locally until driver confirms retrieval. Background cleanup
removes retrieved results older than 24 hours. Emergency cleanup removes
oldest retrieved results first if disk exceeds threshold.

## Failure Registry

| Failure Mode      | Detection               | Recovery                                |
|-------------------|-------------------------|-----------------------------------------|
| Worker crash      | Heartbeat timeout (60s) | Driver re-queues combos                 |
| NFS mount lost    | PCAP open fails         | Job fails; driver reports to user       |
| Driver crash      | Restart reads jobs      | Resume: re-queue incomplete, merge done |
| Network partition | Poll fails              | Worker holds cache; driver retries      |
| Config invalid    | Pre-flight check fails  | Job never starts; error shown           |

## Phased Rollout

1. **Phase 1** — Job model, worker server CRUD, persistence (S, low risk)
2. **Phase 2** — Driver coordinator, settings UI, worker CRUD API (M, low risk)
3. **Phase 3** — Worker mode in unified binary (L, medium risk)
4. **Phase 4** — Full integration + sweep dashboard (L, medium risk)
5. **Phase 5** — Resilience and operational hardening (M, low risk)

Phases 1–3 strictly sequential. Phases 4–5 can overlap once Phase 3 is
functional.

## Design Constraints

- Privacy preserved — no data leaves local network.
- SQLite remains the database everywhere.
- Raspberry Pi 4 compatible (≤ 512 MB RAM during PCAP replay).
- Backward compatible — single-machine sweep path unchanged.
- Shared filesystem required (relative PCAP paths; absolute/`..` rejected).

## Alternatives Rejected

- **Separate worker binary** — conflicts with single-binary direction.
- **Worker self-registration** — harder to audit; Settings CRUD preferred.
- **Message queue (Redis/NATS)** — adds infrastructure dependency.
- **gRPC streaming** — deferred; HTTP polling sufficient for now.
- **SSH-based remote execution** — no job lifecycle or recovery.
- **Shared SQLite** — fundamentally unsupported for multi-machine writes.