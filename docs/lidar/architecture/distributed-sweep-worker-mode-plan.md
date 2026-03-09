# Distributed Sweep Worker Mode Plan

## Goal

Add a distributed sweep execution mode that lets the main node orchestrate parameter tuning cycles across one or more remote worker nodes running on server-hosted compute. The main node remains the only place that serves the Svelte UI and stores sweep history. Worker nodes expose API-only LiDAR execution endpoints and do not serve the Svelte app.

## Current Baseline

The current sweep path already gives us useful building blocks:

- `internal/lidar/sweep.Runner` expands parameter combinations, executes them, and persists final results.
- `internal/lidar/sweep.SweepBackend` already abstracts execution against either:
  - `monitor.DirectBackend` for in-process execution on the main node
  - `monitor.ClientBackend` for HTTP execution against a remote LiDAR monitor
- `internal/lidar/monitor.Client` already knows how to drive the low-level worker actions we need:
  - apply tuning params
  - reset grid / acceptance
  - start and stop PCAP replay
  - wait for PCAP completion
  - fetch acceptance and tracking metrics
- `internal/lidar/storage/sqlite.SweepStore` already persists sweep records for the Svelte sweeps page.
- The Svelte page at `web/src/routes/lidar/sweeps/+page.svelte` already reads centralized sweep history from the main node.

The main limitations today are:

- One in-memory active sweep runner per process.
- Execution is tightly coupled to a single local LiDAR runtime.
- PCAP files are resolved against the local node's `pcapSafeDir`.
- Progress state is sweep-level only; there is no per-worker or per-combo lease/heartbeat model.
- Worker nodes currently expose debug dashboards and static assets because the LiDAR monitor has one route profile.

## Design Principles

1. Keep sweep semantics and execution placement separate.
   `mode` already means parameter-shape semantics (`multi`, `noise`, `closeness`, `neighbour`, `params`). Distributed execution should be a separate field such as `execution_mode`.

2. Keep the main node as the system of record.
   The main node should own:

- sweep creation
- combo planning
- worker selection
- progress aggregation
- persistence
- final recommendation
- all Svelte/UI endpoints

3. Keep worker nodes API-only.
   Workers should expose only the execution API surface required to run combos and report progress. No Svelte app, no sweep dashboard HTML, no debug dashboards unless explicitly enabled for operators.

4. Reuse the existing `SweepBackend` contract where possible.
   The current backend abstraction is already the right seam. The new work should factor combo execution out of `Runner` so both local and distributed paths share the same tuning-cycle semantics.

5. Prefer relative PCAP identifiers, not main-node absolute paths.
   Distributed execution only works cleanly if the request references a scene/PCAP identifier that workers can resolve locally.

## Proposed Architecture

### Main Node

Add a `DistributedCoordinator` in `internal/lidar/sweep` that:

- loads a worker registry from a JSON config file
- expands parameter combinations from the incoming `SweepRequest`
- assigns combos to eligible workers
- tracks queue/running/completed/failed state per combo
- aggregates final `ComboResult` records into the existing sweep record
- updates an extended `SweepState` that includes worker progress
- handles stop/cancel by cancelling all leased worker jobs

The existing `Runner` should remain the local execution engine. The coordinator should reuse the same combo execution logic rather than reimplementing sampling semantics.

### Worker Node

Add a worker-only mode for the LiDAR monitor process. A worker node should:

- load normal LiDAR runtime state and tuning config
- expose a restricted API profile
- accept combo execution jobs from the main node
- run one combo at a time per worker slot
- report job status, heartbeats, warnings, and results

The worker can reuse the local `DirectBackend` internally for actual combo execution.

### Shared Execution Primitive

Refactor the current per-combo logic in `Runner.run` / `Runner.runGeneric` into a shared `CycleExecutor` or `ComboExecutor` that can:

- apply params
- reset state
- start replay/live settle
- sample metrics
- produce a `ComboResult`

This gives us one authoritative implementation for both:

- local sweeps on the main node
- remote worker combo jobs

## New Request Shape

Do not overload `SweepRequest.Mode`. Extend the request instead.

Suggested additions:

```json
{
  "mode": "params",
  "execution_mode": "distributed",
  "worker_pool": "exec-cluster-a",
  "max_parallel_workers": 4,
  "data_source": "pcap",
  "pcap_file": "sites/kirkland/golden-run-01.pcapng"
}
```

Suggested Go fields:

```go
type SweepRequest struct {
    Mode               string `json:"mode"`
    ExecutionMode      string `json:"execution_mode,omitempty"`       // "local" (default) | "distributed"
    WorkerPool         string `json:"worker_pool,omitempty"`          // pool name from config
    MaxParallelWorkers int    `json:"max_parallel_workers,omitempty"` // optional scheduler cap
    ...
}
```

This keeps parameter semantics stable while making execution placement explicit.

## Worker Config File

Add a new server-side JSON file, separate from `config/tuning.defaults.json`.

Suggested CLI flag on the main node:

```bash
--sweep-workers-config=/etc/velocity/sweep-workers.json
```

Suggested config shape:

```json
{
  "version": 1,
  "pools": [
    {
      "name": "exec-cluster-a",
      "max_parallel_workers": 4,
      "workers": [
        {
          "id": "exec-01",
          "base_url": "http://10.0.0.21:8081",
          "sensor_id": "hesai-pandar40p",
          "auth_token_env": "SWEEP_WORKER_TOKEN_EXEC_01",
          "pcap_root_hint": "/srv/lidar-pcaps",
          "tags": ["pcap", "hesai", "high-mem"],
          "enabled": true
        }
      ]
    }
  ]
}
```

Rules:

- This file is only loaded by the main node.
- It describes reachable worker APIs and scheduler hints.
- It is not a user-supplied path in the browser request.
- The request should reference a pool name, not an arbitrary file path.

If a CLI tool also needs distributed mode, that tool can accept the same config file path locally.

## Intended Deployment Profile

The intended deployment is more specific than a generic cluster:

- one VM host
- one main-node container
- 6 to 10 worker containers
- workers and main node all run on the same Docker host
- all containers can mount a common host ZFS-backed dataset read-only

This changes the design in useful ways:

- PCAP locality is much easier because every worker can see the same dataset path.
- Control-plane networking can stay on a private Docker bridge network.
- We should avoid direct worker writes into the shared ZFS dataset if read-only is the preferred operating mode.
- Result collation should default to API push/pull back to the main node, with shared-filesystem artifact spooling used only for heavy debug tiers.

Recommended mount layout:

- shared read-only dataset mounted into every container at the same path, for example `/srv/lidar-pcap-ro`
- worker-local scratch space on container-local writable storage or tmpfs
- main-node-owned database volume for sweep metadata and final persisted results

Recommended rule:

- all worker containers must resolve the same relative `pcap_file` against the same read-only mount path
- workers should never mutate the shared replay dataset
- heavy debug exports should be opt-in and not part of the default sweep path

## Recommended Container Topology

For the first pass, prefer:

- main node on a private Docker network
- workers on the same private Docker network
- worker hostnames stable via container name or Compose service name
- HTTP/JSON for control plane and result return
- bearer auth even on the private bridge

Avoid for the first pass:

- host networking
- public worker endpoints
- direct worker writes into the main SQLite database
- shared writable result directories unless a later debug tier requires them

## Data Collation Strategy

There are really two separate decisions:

1. What level of data comes back from workers?
2. How is that data returned to the main node?

The best first-pass answer is:

- keep default sweep collation at summary/result level only
- optionally add richer per-combo raw samples next
- add track-level sync only when you need centralized run inspection or HINT over distributed workers
- keep foreground-point and full debug data as on-demand tiers, not always-on

## Sync Tier Matrix

| Tier | Synced back to main node                                  | Typical payload per combo | Main-node value                                                      | Main risks                                                 | Recommendation                              |
| ---- | --------------------------------------------------------- | ------------------------- | -------------------------------------------------------------------- | ---------------------------------------------------------- | ------------------------------------------- |
| T0   | `ComboResult` summary only                                | KB                        | Sweep dashboards, ranking, recommendation                            | Almost none beyond normal job retries                      | Default baseline                            |
| T1   | Summary + per-iteration `SampleResult` raw metrics        | KB to low MB              | Variance charts, deeper sweep diagnostics                            | More DB/blob size, partial-upload handling                 | Early follow-up                             |
| T2   | Summary + track/run artifacts                             | MB to tens of MB          | Central run inspection, track dashboards, later HINT/label workflows | Run-ID namespacing, replay-time alignment, dedupe on retry | Add when centralized run analysis is needed |
| T3   | Summary + tracks + foreground cluster/point debug windows | Tens to hundreds of MB    | Deep debug of bad combos and clustering failures                     | Bandwidth, storage, ordering, artifact explosion           | Debug-only, gated                           |
| T4   | Full VRLOG / raw point-cloud style artifacts              | Hundreds of MB to GB      | Full remote replay and forensic analysis                             | Too expensive for routine sweeps, very high storage churn  | Non-goal for first pass                     |

### Tier Details

#### T0: Summary Only

Worker returns only:

- combo metadata
- final `ComboResult`
- warnings
- worker timing / heartbeat metadata

This is enough for:

- sweep ranking
- recommendation generation
- current sweeps dashboard
- operator progress tracking

This should be the default path for the first milestone.

#### T1: Raw Sampling Metrics

Worker also returns:

- per-iteration `SampleResult`
- optional per-bucket sample breakdowns
- optional timing stats for settle/replay/sample phases

This is the best next step after T0 because:

- it gives much better diagnostics without huge network cost
- it still fits naturally into the sweep model
- it does not require moving full track/run data around

#### T2: Track/Run Sync

Worker returns or uploads:

- track summary rows
- track observations
- run-level metadata
- optional links between `combo_id` and analysis `run_id`

This tier is the turning point where the main node can support:

- run dashboards over distributed worker output
- centralized qualitative review
- future distributed HINT/reference workflows

This tier should not be the first milestone because it introduces more schema and synchronization complexity than T0/T1.

#### T3: Foreground / Debug Sync

Worker returns only debug-scoped data, such as:

- foreground point windows
- cluster snapshots
- grid snapshots
- misbehaving track debug bundles

This should be:

- opt-in
- time-windowed
- combo-filtered
- compressed

Do not make this part of the normal sweep happy path.

#### T4: Full Replay Artifacts

This includes:

- VRLOG
- full frame-level point exports
- complete remote replay bundles

This tier is operationally expensive and should stay outside the first several milestones.

## Backhaul Method Matrix

| Method                                         | Best for tiers       | Pros                                                                  | Cons                                                                                     | Recommendation                              |
| ---------------------------------------------- | -------------------- | --------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- | ------------------------------------------- |
| API return in job status / completion payload  | T0, small T1         | Simple, no extra storage layer, clean ownership on main node          | Large payloads make polling and retries awkward                                          | Use first                                   |
| API upload endpoint for result blobs/artifacts | T1, T2, selective T3 | Keeps ownership on main node, supports compression and chunking       | Needs upload state machine and cleanup                                                   | Use second                                  |
| Shared filesystem artifact spool on host       | T2, T3, T4           | Good for large artifacts on same host, avoids repeated network copies | Requires writable shared path, cleanup, file-claim protocol, stale-file handling         | Use only if heavy debug tiers become common |
| Direct worker DB writes                        | None recommended     | Looks simple on paper                                                 | Tight coupling, lock contention, retry ambiguity, SQLite pain, weak ownership boundaries | Avoid                                       |

## Recommended Return Path by Milestone

| Milestone | Data tier           | Backhaul                                    | Why                                                             |
| --------- | ------------------- | ------------------------------------------- | --------------------------------------------------------------- |
| M1        | T0 summary only     | API completion payload                      | Fastest path to working distributed sweeps                      |
| M2        | T1 raw samples      | API completion payload or small blob upload | Better diagnostics with manageable cost                         |
| M3        | T2 tracks/runs      | Blob upload endpoint plus main-node import  | Enables centralized track dashboards and future label workflows |
| M4        | T3 foreground debug | Artifact upload or shared spool             | Only for targeted debugging, not routine sweeps                 |

## Single-Host ZFS Considerations

Because all workers live on the same VM host, the most important storage issue is not distributed consistency across machines. It is repeatability and I/O contention across containers.

Recommended practices:

- mount a fixed read-only ZFS snapshot, not a mutable live dataset, for sweep inputs
- mount that snapshot at the same path in every container
- include dataset snapshot identity in the sweep request or persisted metadata
- keep worker scratch output off the shared read-only dataset

Why snapshot identity matters:

- if the underlying dataset changes mid-sweep, identical `pcap_file` paths may refer to different bytes across attempts or across separate sweeps
- the main node must know which exact input snapshot produced a recommendation

Suggested extra request metadata:

```json
{
  "dataset_id": "zfs:lidoruns/golden@2026-03-09",
  "pcap_file": "sites/kirkland/golden-run-01.pcapng"
}
```

## Network and Synchronization Problems

Even on one VM host, distributed sweeps still have real synchronization problems.

### 1. Input Drift

Problem:

- workers read a path that changes underneath them

Mitigation:

- use read-only ZFS snapshots
- persist `dataset_id` / snapshot name in sweep metadata
- reject distributed sweeps if workers do not report the same mounted dataset identity

### 2. Worker Image / Config Drift

Problem:

- two containers may run different image tags or different tuning defaults

Mitigation:

- worker health endpoint should report image version, git SHA, config hash, and dataset identity
- coordinator should reject mixed-version pools unless explicitly allowed

### 3. Duplicate Result Submission

Problem:

- a worker finishes, upload succeeds, coordinator times out, job gets retried, and the combo result arrives twice

Mitigation:

- use stable `combo_id`
- make result ingestion idempotent on `(sweep_id, combo_id, attempt)`
- only one successful result can close a combo

### 4. Heartbeat vs Wall-Clock Skew

Problem:

- `started_at` and `heartbeat_at` from different containers are not perfectly aligned

Mitigation:

- main node owns lease expiry decisions
- worker heartbeats are advisory; replay timestamps and combo IDs are the real data identity
- for track data, prefer replay-time timestamps from the PCAP/run, not container wall-clock time

### 5. Partial Artifact Uploads

Problem:

- larger T2/T3 uploads can fail midway and leave dangling partial state

Mitigation:

- stage uploads with explicit `pending -> committed` states
- store artifact manifest rows separately from final combo success
- garbage-collect stale pending uploads

### 6. Shared-Host I/O Contention

Problem:

- 6 to 10 workers replaying large PCAPs from the same ZFS pool can bottleneck storage and distort sweep timing

Mitigation:

- cap concurrency per pool
- measure replay throughput in worker health
- start with fewer concurrent workers than containers if storage is the bottleneck

### 7. Main-Node Backpressure

Problem:

- richer sync tiers can overwhelm the main node with ingest work

Mitigation:

- keep T0 as default
- gate T2/T3 behind explicit request flags
- compress uploads
- process artifact import asynchronously from combo completion if needed

## Milestone Bundles

### Milestone A: Reliable Distributed Sweeps

Scope:

- Docker workers on one VM host
- shared read-only ZFS snapshot mount
- T0 summary-only collation
- centralized sweep progress on main node

This milestone gets you practical distributed parameter sweeps with low risk.

### Milestone B: Better Diagnostics

Scope:

- add T1 raw sample sync
- add richer worker timing and warning telemetry
- improve dashboard drill-down on combo stability

This is the best next milestone if the main goal is better tuning analysis, not visual debug.

### Milestone C: Centralized Run Inspection

Scope:

- add T2 track/run sync
- map worker-produced runs into main-node browseable artifacts
- support sweep-to-run drill-down in dashboards

Choose this when you need to inspect remote worker output centrally from the main node.

### Milestone D: Heavy Debug Data

Scope:

- add T3 selective foreground/cluster sync
- add artifact upload/spool path
- add retention and cleanup rules

Only do this after T0 through T2 are stable.

## Worker API Contract

Do not make the coordinator drive every low-level monitor endpoint directly for long-lived jobs. Add a higher-level worker job API.

Suggested worker API:

- `POST /api/lidar/worker/jobs`
  - create a combo execution job
- `GET /api/lidar/worker/jobs/{job_id}`
  - get job status, heartbeat, warnings, partial progress, final result
- `POST /api/lidar/worker/jobs/{job_id}/cancel`
  - cancel a running job
- `GET /api/lidar/worker/health`
  - expose worker readiness and capacity

Suggested job payload:

```json
{
  "sweep_id": "sw-123",
  "combo_id": "combo-0042",
  "sensor_id": "hesai-pandar40p",
  "param_values": {
    "noise_relative": 0.015,
    "closeness_multiplier": 2.0,
    "neighbor_confirmation_count": 1
  },
  "iterations": 20,
  "interval": "2s",
  "settle_time": "5s",
  "settle_mode": "once",
  "data_source": "pcap",
  "pcap_file": "sites/kirkland/golden-run-01.pcapng",
  "pcap_start_secs": 0,
  "pcap_duration_secs": -1,
  "enable_recording": false
}
```

Suggested job status payload:

```json
{
  "job_id": "job-abc",
  "sweep_id": "sw-123",
  "combo_id": "combo-0042",
  "worker_id": "exec-01",
  "status": "running",
  "started_at": "2026-03-09T12:00:00Z",
  "heartbeat_at": "2026-03-09T12:00:08Z",
  "phase": "sampling",
  "warnings": [],
  "result": null
}
```

Why use a higher-level job API:

- fewer coordinator round trips
- clean cancellation semantics
- easier retries and leases
- better per-worker observability
- workers can evolve internal low-level execution without changing coordinator logic

## Route Profile for Workers

Add a monitor route profile such as:

- `full`
- `worker`

Worker profile should keep:

- health
- worker job endpoints
- low-level LiDAR execution endpoints needed internally
- optional `GET /api/lidar/pcap/files` for validation

Worker profile should exclude:

- debug HTML dashboards
- embedded JS/CSS assets
- Svelte app integration
- historical sweep list/get endpoints owned by the main node

Suggested flag:

```bash
--lidar-route-profile=worker
```

## Persistence Changes

Keep `lidar_sweeps` as the top-level sweep record table, but add a new table for distributed execution detail.

Suggested new table:

`lidar_sweep_jobs`

Fields:

- `job_id`
- `sweep_id`
- `combo_id`
- `worker_id`
- `status`
- `attempt`
- `lease_expires_at`
- `started_at`
- `heartbeat_at`
- `completed_at`
- `request_json`
- `result_json`
- `error`
- `warnings_json`

Why add a table instead of only storing JSON in `lidar_sweeps`:

- supports per-worker progress in the main node
- supports retry/requeue after failure
- supports resume after main-node restart
- gives operators an audit trail for combo placement and execution failures

`lidar_sweeps.request` should also persist the selected execution mode and worker pool.

## Scheduling Model

Start simple:

- one active combo per worker
- round-robin assignment across healthy workers
- combos are independent and can be executed in any order
- if a worker stops heartbeating, mark the job failed and requeue once

Then add capability filters:

- worker tags
- sensor compatibility
- data source support
- PCAP availability

The coordinator should treat combo execution as idempotent from the sweep's perspective. The primary key should be `combo_id`, not just `(noise, closeness, neighbour)` stringification.

## PCAP and Data Locality

This is the biggest operational constraint.

Recommended rule:

- scenes and sweeps should reference a relative `pcap_file`
- every worker in a pool must resolve that relative path inside its own safe directory

If that cannot be guaranteed, add one of these later:

- a dataset registry mapping logical dataset IDs to worker-local paths
- per-worker path rewrite rules in the worker config
- object-storage download into a worker cache

For the first pass, require consistent relative PCAP layout across the pool.

## Auth and Security

Do not expose unauthenticated worker execution endpoints.

Minimum acceptable first pass:

- bearer token per worker
- main node injects token from environment, not from browser requests
- worker routes reject all unauthenticated execution calls

Better long term:

- mTLS between main and workers
- worker allowlist for main-node source IPs

## UI and Main-Node Monitoring

The main node should remain the only UI surface.

Extend the current sweep status and sweep detail views with:

- execution mode
- worker pool
- queued / running / completed / failed combo counts
- per-worker status cards
- per-worker last heartbeat
- retry count and failed combo list

The worker nodes should not host Svelte pages.

## Recommended Implementation Order

### Phase 1: Refactor Local Execution

- extract a shared combo executor from `Runner`
- keep all behavior local
- verify zero behavior change for manual sweep, auto-tune, and HINT

### Phase 2: Worker Process Mode

- add worker route profile
- add worker job API
- run combo jobs locally on a worker node via the shared executor

### Phase 3: Main-Node Coordinator

- add worker config loading
- add distributed scheduler and heartbeats
- persist job rows in `lidar_sweep_jobs`
- aggregate results back into `lidar_sweeps`

### Phase 4: UI and Ops

- show distributed progress in the main-node sweeps page
- add worker health validation
- add structured logs and failure surfaces

### Phase 5: Auto-Tune and HINT Integration

- let auto-tune rounds dispatch combos through the coordinator
- let HINT sweep phases use the same distributed execution path
- keep reference-run creation and final recommendation persistence on the main node

## Build Checklist

- [ ] Add `execution_mode`, `worker_pool`, and `max_parallel_workers` to `SweepRequest`.
- [ ] Refactor per-combo logic out of `Runner.run` and `Runner.runGeneric` into a shared executor.
- [ ] Keep local `Runner` behavior unchanged by routing local sweeps through the shared executor.
- [ ] Add a worker registry loader for a new `--sweep-workers-config` JSON file.
- [ ] Define worker config validation rules and startup errors.
- [ ] Standardize the shared read-only dataset mount path across main and worker containers.
- [ ] Persist a `dataset_id` or snapshot identity with each distributed sweep.
- [ ] Add a monitor route profile so worker nodes can run API-only.
- [ ] Add authenticated worker job endpoints for create/status/cancel/health.
- [ ] Extend worker health reporting to include image version, config hash, and dataset identity.
- [ ] Implement worker-side combo execution using the shared executor and `DirectBackend`.
- [ ] Add a distributed coordinator on the main node that expands combos and assigns them to workers.
- [ ] Add lease, heartbeat, timeout, and retry handling for worker jobs.
- [ ] Create a new `lidar_sweep_jobs` table and persistence layer.
- [ ] Extend sweep state to include queue depth, running jobs, per-worker progress, and warnings.
- [ ] Persist execution metadata (`execution_mode`, `worker_pool`) in `lidar_sweeps.request`.
- [ ] Update the sweeps API responses to expose distributed progress data.
- [ ] Update the Svelte sweeps page to show worker-level progress from the main node.
- [ ] Enforce auth from main node to worker nodes.
- [ ] Require relative `pcap_file` references and document worker-local safe directory expectations.
- [ ] Add health checks that verify worker reachability and required PCAP availability before a distributed sweep starts.
- [ ] Implement T0 summary-only result collation as the default distributed path.
- [ ] Add optional T1 raw sample sync for richer sweep diagnostics.
- [ ] Add optional T2 track/run sync for centralized run inspection.
- [ ] Add optional T3 debug artifact sync for foreground/cluster analysis.
- [ ] Add integration tests for local sweeps after the executor refactor.
- [ ] Add integration tests for worker job lifecycle.
- [ ] Add integration tests for coordinator scheduling across multiple mocked workers.
- [ ] Add integration tests for worker timeout and requeue.
- [ ] Add integration tests for cancellation fan-out.
- [ ] Add integration tests for mixed success/failure aggregation.
- [ ] Add an operator doc for standing up a worker node and registering it in the main-node config.

## Recommended Non-Goals for First Pass

- multi-tenant scheduling
- dynamic autoscaling
- object-storage PCAP syncing
- more than one concurrent combo per worker process
- serving any Svelte UI from workers

## Bottom Line

The safest path is not to bolt remote execution directly into the existing local runner. The right change is:

- shared combo executor
- API-only worker mode
- main-node distributed coordinator
- worker config file loaded by the main node
- per-job persistence and heartbeats

That preserves the existing sweep semantics, keeps the Svelte UI centralized, and gives you a clean foundation for running tuning cycles on server-hosted execution nodes.
