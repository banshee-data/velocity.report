# LiDAR job queue: implementation map

- **Status:** Draft
- **Layers:** L9 Endpoints (hub API), L10 Clients (dashboard, macOS), offline analysis (worker), platform
- **Target:** branch `dd/lidar/job-queue`, off `main`. First delivery: submit a job from the dashboard, run it on `swan` or the NAS worker, see its result on the hub. The macOS app keeps its local server for VRLOG generation and annotation until Item 7.
- **Companion plans:** [lidar-worker-pool-and-results-hub-plan.md](lidar-worker-pool-and-results-hub-plan.md)
  (the design this maps onto code),
  [lidar-analysis-run-infrastructure-plan.md](lidar-analysis-run-infrastructure-plan.md), and the
  point annotation plan, which lands with PR #579
- **Canonical:** [lidar-worker-pool-and-results-hub-plan.md](lidar-worker-pool-and-results-hub-plan.md)

## Motivation

The worker pool plan says what the system is: a hub that owns job state, disposable workers
that claim one job at a time, write-once result bundles, identities that make results
comparable across hosts. It does not say which packages, tables, routes and pages that is, or
what order to build them in so that something useful runs early. This document is that map.

It is written against what is on `main` today and what is on `dd/docs/state-est`, because the
first jobs an operator wants to submit are the state-estimation sweeps that run on `swan`, and
the tools that run them have not landed on `main`. The design has to let a worker run a tool
from a stated commit without the worker itself depending on that branch.

## Current state

What exists, and what each contributes.

| Piece                                                                       | Where                                                                                     | Reused as                                                                        |
| --------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------- |
| A one-process job runner: claim, progress, cancel, fail, release on restart | `internal/lidar/capjobs`, table `lidar_capture_jobs`                                      | The shape of the worker's local loop. Its `Store` and `Handler` seams carry over |
| Tuning config fingerprint and composed run config with build identity       | `config.TuningConfig.Fingerprint`, `configasset`                                          | The tuning and code halves of run identity                                       |
| Benchmark document with fingerprint, platform and work counters             | `internal/lidar/lidarbench.BenchmarkResult`                                               | The model for a result bundle's summary: identity first, numbers second          |
| Immutable source manifest of ordered PCAP hashes                            | `cmd/tools/lidar-state-estimation-baseline/source_manifest.go` (state-est)                | The capture manifest, moved to a shared package so hub and worker agree on it    |
| Campaign harness that runs a tool per stage and records outcomes            | `data/experiments/try/option-scorecard/run_scorecard_sweep.py` (state-est)                | The list of what a job actually invokes, and what a result directory holds       |
| Static Linux binary build in Docker                                         | `make build-radar-static`, D-26                                                           | How a worker binary reaches a Linux host                                         |
| Capture index and its jobs page                                             | `/api/lidar/capture/*`, `web/src/routes/lidar/captures`                                   | The dashboard's queue table starts from this page's job list                     |
| Independent capture, recording, plots and annotation directories            | `--lidar-*-dir` flags, `resolveLidarDir`                                                  | Worker and hub roots are configured the same way                                 |
| `swan`                                                                      | root SSH, Unraid Docker runner, read-only PCAP mount, persistent state and results shares | The first worker. One failed smoke attempt to repair (plan §8)                   |

What does not exist: any table, route, page or command for a job that runs on another host;
any way to tell that two hosts ran the same bytes; any worker authentication.

Two facts that shape the map:

- **The state-est tools are not on `main`.** `lidar-state-estimation-baseline`,
  `lidar-track-scorecard` and `lidar-evidence-oracle` exist only on `dd/docs/state-est`. A
  worker that linked them in-process could not be built from `main`. So a job names a tool and a
  commit, and the worker builds or fetches that tool from a staged source snapshot, which is
  also what `swan`'s runner already does and what plan §8's repair requires ("the working
  directory is explicit").
- **Migration numbers collide.** `main` is at migration 46 and state-est holds 47 to 49. A job
  table numbered 47 on this branch would fight state-est at every rebase. The queue therefore
  lives in its own SQLite file with its own small migration set, as the sidecar and the capture
  index each keep their own state. It is also the right shape: queue state is operational, and
  the hub's `sensor_data.db` is capture history that gets cloned and migrated on its own terms.

## Findings

| Area             | Current state                                                     | Severity | Release view                        |
| ---------------- | ----------------------------------------------------------------- | -------- | ----------------------------------- |
| Job contract     | Prose in the pool plan; no types, no digests, no validation       | High     | Everything else waits on it         |
| Capture identity | Manifest format exists on state-est only, inside a `main` package | High     | Move to a shared package first      |
| Worker           | `swan` runs one hand-launched Docker command; no loop, no lease   | High     | Phase A                             |
| Hub              | No queue, no bundle store, no worker registry                     | High     | Phase B                             |
| Dashboard        | Captures page lists local jobs only                               | Medium   | Phase B                             |
| Auth             | None between hosts                                                | Medium   | A shared token per worker, LAN only |
| macOS backend    | Three `localhost:8080` bases and one gRPC address, hard-coded     | Low      | One origin setting, Item 7          |

## Design / approach

### One binary, three modes

`velocity serve` is the hub when started with `--jobs-db` (and, on the NAS, it is also the
ordinary server the macOS app will one day talk to). `velocity worker` is the worker: a loop
that leases one job from a hub, runs it, and uploads the bundle. `velocity jobs` is a small CLI
for the same API (submit, list, show, cancel), so the dashboard is never the only client and a
job can be submitted from a script.

### The contract, as code

Package `internal/lidar/jobs` holds what hub, worker, dashboard and CLI must agree on, and
nothing that does I/O beyond hashing a file. It is the first commit.

| Type              | Holds                                                                                                                                                           |
| ----------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `CaptureManifest` | Ordered captures, each with logical id, relative path hint, byte size, SHA-256, container and UDP port; its own digest                                          |
| `JobRequest`      | Kind, capture-manifest digest, tuning (a document, digested), code identity, replay contract, kind-specific parameters as typed JSON, requested worker or `any` |
| `RunIdentity`     | The canonical digest of a request's comparable parts (§4.2 of the pool plan); never includes the worker or the wall clock                                       |
| `Attempt`         | One execution of a job: attempt id, worker, lease, state, timestamps, failure evidence                                                                          |
| `ResultBundle`    | Identity, worker profile, machine measurements, kind-specific summary, log and evidence paths, bundle digest                                                    |
| `Kind`            | A registry entry: name, parameter schema, the tool it invokes, how its summary is read                                                                          |

Job kinds are an allowlist. A request carries parameters, never a command line, a host path
outside the worker's roots, or a Docker option. The worker turns a kind into an invocation:
`state_estimation_baseline` becomes `lidar-state-estimation-baseline -corpus … -source-manifest …`,
built from the job's commit in the worker's staged source tree. Kinds for the first delivery:

| Kind                        | Tool                              | On `main` today | Summary read from         |
| --------------------------- | --------------------------------- | --------------- | ------------------------- |
| `benchmark`                 | `lidar-bench` (in-process)        | yes             | `BenchmarkResult`         |
| `state_estimation_baseline` | `lidar-state-estimation-baseline` | state-est       | `phase0-summary.json`     |
| `track_scorecard`           | `lidar-track-scorecard`           | state-est       | `scorecard.json`          |
| `vrlog_record`              | replay with recording             | yes             | run record, VRLOG digests |

`sweep` (partitioned combos, the older plan's job model) comes after these: it is a set of
`benchmark` or `state_estimation_baseline` jobs with a shared parent, not a kind of its own.

### Hub

Routes under `/api/analysis/` as the pool plan lists them (§6.1), served by the existing
`internal/lidar/server` mux, stored in `jobs.db` through `internal/lidar/jobs/hub`. The queue is
a state machine with the states in pool plan §5; every transition is a row in an attempt log,
so "why is this lost" is a query rather than a guess. Leases are short and renewed by heartbeat;
a lease that expires marks the attempt `lost` and a retry is a new attempt.

Bundle upload is idempotent by bundle digest: the same bytes twice produce one accepted result.
Bundles land in `--lidar-results-dir/<run_identity>/<attempt_id>/` and are never overwritten.

Workers authenticate with a per-worker bearer token the hub issues when the worker is
registered. LAN only; mutual TLS is a later item. The worker's own status page on `swan:8084`
is read-only and shows what the hub knows about it plus its local log tail.

### Worker

`velocity worker --hub URL --token … --pcap-root … --work-dir … --source-dir …` loops: register
and report capabilities (verified capture-manifest digests from its local cache, free space,
version); poll for a compatible job; lease it; verify the captures' bytes against the manifest,
failing closed; stage the tool from the job's commit; run it with an explicit working directory
and the job's parameters; write the attempt record and bundle locally first; upload, resuming
on failure; report. One job at a time. A cancelled lease or a stopped process leaves the local
bundle where it is, marked partial, for a later upload.

Capture verification hashes multi-gigabyte files on purpose. The worker caches (path, size,
mtime, device, digest) and re-hashes when any of those move or when asked to.

### Dashboard

A page under `web/src/routes/lidar/jobs`: pool, queue, attempt and result tables as the pool
plan's §6.2 lists them, plus a submit form that offers only registered kinds and their typed
parameters. It polls the hub as the captures page polls its jobs today.

### What the macOS app does meanwhile

Nothing changes for it in this branch. It talks to the local Go server for frames, runs, VRLOG
generation and annotation packs. Item 7 gives it one configurable origin so that, when the NAS
hub is also its server, switching is a setting and not a build.

## Scope

### Item 1: the contract package

**Summary:** `internal/lidar/jobs` with the types above, the capture manifest moved from the
state-est tool into it in a compatible form, run-identity and bundle digests, kind registry with
parameter validation, and state-machine transitions, all tested.

**Steps:**

1. Types and JSON encodings, with the state-est manifest's field names kept so its manifests
   validate unchanged.
2. Canonical digests: capture manifest, tuning document, request, bundle. Tests pin one example
   of each so a later change to canonicalisation fails here first.
3. Kind registry: `benchmark`, `state_estimation_baseline`, `track_scorecard`, `vrlog_record`,
   each with a parameter schema and a validator that rejects a path outside a named root.
4. Attempt state machine with the allowed transitions and who may make each.

**Milestone:** first commit on `dd/lidar/job-queue`

### Item 2: hub store and API

**Summary:** `jobs.db`, the queue and attempt log, the routes, and bundle ingest.

**Steps:**

1. `internal/lidar/jobs/hub`: SQLite store with its own migrations (capture manifests, jobs,
   attempts, transitions, workers, results), leases with expiry, and an idempotent bundle
   ingest that verifies digests and identity before accepting.
2. Routes in `internal/lidar/server`: the pool plan's §6.1 table, plus the worker's private
   surface (`register`, `lease`, `heartbeat`, `bundle`) behind the bearer token.
3. `velocity serve --jobs-db --lidar-results-dir`; without them the routes answer 501, as the
   annotation export does without its directory.
4. `velocity jobs submit|list|show|cancel`.

**Milestone:** a job submitted by CLI to a hub on the Mac reaches `queued`, and a fake worker
in the tests takes it through to `accepted`

### Item 3: worker

**Summary:** `velocity worker`, with the local attempt record and the `swan:8084` status page.

**Steps:**

1. `internal/lidar/jobs/worker`: the loop, capture verification with the digest cache, tool
   staging by commit, invocation by kind, local bundle, resumable upload.
2. `benchmark` end to end first: it is in-process and on `main`, so it proves the loop with no
   dependency on state-est.
3. `state_estimation_baseline` and `track_scorecard` by staged tool, proved against the `swan`
   smoke attempt that failed (pool plan §8): the repaired attempt is a new attempt id and the
   failed one stays.
4. The read-only status page on 8084.

**Milestone:** the three-capture state-estimation smoke attempt runs on `swan` from a job the hub
issued, and its bundle is accepted

### Item 4: dashboard

**Summary:** the jobs page and submit form.

**Milestone:** an operator submits a job, watches it run on `swan`, and opens its result without
SSH

### Item 5: NAS worker and scheduling

**Summary:** the NAS registered as a second worker; the scheduler prefers a worker holding the
verified captures, then the NAS for ordinary work and `swan` for burst; `draining`.

**Milestone:** pool plan Phase C acceptance

### Item 6: sweeps

**Summary:** a parent job that expands into many child jobs of one kind, with a merged summary.
The older distributed sweep plan's partitioning, on the new contract.

### Item 7: the macOS app's origin

**Summary:** one server origin setting in the app (gRPC and HTTP derived from it), replacing
three hard-coded `localhost:8080` bases. Nothing else changes until the NAS hub is also the
macOS server, when `vrlog_record` jobs produce what the run browser lists.

## Dependencies

- `swan` reachable for Item 3. Its root SSH key is held by the 1Password agent and needs an
  interactive approval per connection; an agent session cannot open one on its own.
- The NAS: hostname, OS, container runtime, and whether it mounts the PCAP volume, for Item 5.
  Not needed before then.
- For `state_estimation_baseline` on a worker, a Go toolchain and libpcap in the worker's
  image, or a build host that ships the tool by digest. `swan`'s runner already stages source.

## Risks

| Risk                                               | Likelihood | Impact | Mitigation                                                                    |
| -------------------------------------------------- | ---------- | ------ | ----------------------------------------------------------------------------- |
| State-est rebases move the tools this branch names | High       | Low    | Tools are named and built by commit; the worker does not link them            |
| Migration numbering collides with state-est        | High       | Medium | Own `jobs.db` with its own migrations                                         |
| A worker runs something a request smuggled in      | Low        | High   | Kinds are an allowlist with typed parameters; paths are resolved inside roots |
| Hashing a 4 GB PCAP on every lease                 | Medium     | Medium | Digest cache keyed on size, mtime and device; forced re-hash on request       |
| Two hosts' numbers compared as if one              | Medium     | High   | Run identity excludes the host; performance keeps a host label                |
| Lost lease while a bundle was half uploaded        | Medium     | Medium | Local bundle is write-once; upload is idempotent by digest                    |

## Checklist

### Complete

- [x] Pool plan brought onto this branch (`f4ac0feed`, from state-est `bc3baae17`)

### Outstanding

- [ ] Item 1: contract package (`M`)
- [ ] Item 2: hub store and API (`L`)
- [ ] Item 3: worker (`L`)
- [ ] Item 4: dashboard (`M`)
- [ ] Item 5: NAS worker and scheduling (`M`)
- [ ] Item 6: sweeps (`M`)
- [ ] Item 7: macOS origin setting (`S`)

### Deferred

- [ ] Annotation packs published through the hub and the macOS app synchronising revisions with
      it: pool plan Phase D, after the NAS hub is the macOS server
- [ ] Mutual TLS between hub and workers

### Accepted residuals (no action planned)

- VRLOG generation and annotation stay on the operator's Mac until Item 7 and Phase D.
- No public exposure of any of this; LAN only, per the pool plan's non-goals.
