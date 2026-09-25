# LiDAR job queue: implementation map

- **Status:** Draft
- **Layers:** L9 Endpoints (hub API), L10 Clients (dashboard, macOS), offline analysis (worker), platform
- **Target:** branch `dd/lidar/job-queue`, off `main`. First delivery: submit a job from the dashboard, run it on `swan` or the NAS worker, see its result on the hub. The macOS app keeps its local server for VRLOG generation and annotation until Item 7.
- **Companion plans:** [lidar-worker-pool-and-results-hub-plan.md](lidar-worker-pool-and-results-hub-plan.md)
  (the design this maps onto code),
  [lidar-analysis-run-infrastructure-plan.md](lidar-analysis-run-infrastructure-plan.md), and the
  point annotation plan, which lands with PR #579
- **Canonical:** [analysis-worker.md](../lidar/operations/analysis-worker.md)

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
| Campaign harness that runs a tool per stage and records outcomes            | `option-scorecard/run_scorecard_sweep.py` (state-est campaign, archived)                  | The list of what a job actually invokes, and what a result directory holds       |
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

### The runner first, the hub after

The operator's direction was to keep the runner simple: given a capture, a config and a
campaign, run, log, and serve the results. So the worker comes first and stands on its own.
`velocity worker` has its own queue and API; `velocity jobs` drives it from a script or by hand.
The hub, when it comes, submits to workers through that same API and collects their bundles; it
does not replace them. `velocity serve --jobs-db` will be the hub, and on the NAS also the
ordinary server the macOS app will one day talk to.

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

### Worker

`velocity worker --pcap-root … --work-dir … --token … [--source-repo …]` serves an API on a
LAN port (8084 on `swan`) and drains its own queue, one attempt at a time. For each: verify the
captures' bytes against the manifest, failing closed; stage the kind's tool from the job's commit
with `git worktree` and `go build`; run it in that tree with arguments the worker composed from
the typed parameters; write the bundle and its manifest; verify the bundle against the job's
identity; mark it accepted. It plays both actors of the contract's state machine for now. A
stopped process leaves what was running marked lost with its partial bundle kept.

State is a directory tree under the work directory, not a database: an operator can read it
over SSH when nothing else answers, and it is what a hub will one day be sent. Capture digests
are cached against size, mtime and inode and re-hashed when any of those move. The API takes a
bearer token on everything but `/health` and serves nothing without one configured. The
operator's guide is [analysis-worker.md](../lidar/operations/analysis-worker.md).

### Hub

Routes under `/api/analysis/` as the pool plan lists them (§6.1), served by the existing
`internal/lidar/server` mux, stored in `jobs.db` through `internal/lidar/jobs/hub`. The hub
registers workers by URL and token, submits to them through the worker API, watches their
attempts, fetches accepted bundles into `--lidar-results-dir/<run_identity>/<attempt_id>/`,
and indexes them by run identity. Bundle ingest is idempotent by bundle digest. A worker that is
off is one the hub cannot reach, and its attempts stay where they are until it is back.

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

### Item 2: worker and client

**Summary:** `velocity worker` and `velocity jobs`: the runner, its API, campaigns, and the
in-process `benchmark` and staged-tool kinds.

**Milestone:** a benchmark job over `kirk0` submitted by the client on the Mac, verified,
run, accepted and fetched as a tar. Done.

### Item 3: the state-estimation smoke attempt on swan

**Summary:** the worker on `swan`, and the three-capture smoke attempt that failed (pool plan
§8) run again as a new attempt from a job, with the failed one kept.

**Steps:**

1. Install the agent's SSH key, the worker binary, a clone for `--source-repo`, and a token
   on `swan`; run the worker under the existing Docker runner with the read-only PCAP mount
   as `--pcap-root` and the persistent state share as `--work-dir`.
2. Write the capture manifest for `morg0`, `kirk0` and `claren0` from their bytes on `swan`.
3. Submit `state_estimation_baseline` from the state-est commit; watch it stage, build and run.
4. Compare its `phase0-summary.json` with the Mac's for the same identity.

**Milestone:** the smoke attempt's bundle is accepted on `swan` and fetched to the Mac

### Item 4: hub and dashboard

**Summary:** `jobs.db` and the `/api/analysis/` routes in `velocity serve`, registering workers
and collecting their bundles by run identity; the jobs page and submit form in the web app.

**Milestone:** an operator submits a job from the dashboard, watches it run on `swan`, and opens
its result without SSH

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

- `swan` reachable for Item 3: the agent's own key, `~/.ssh/velocity_agent_swan_ed25519.pub`,
  installed in root's `authorized_keys` there. The operator's key is held by the 1Password agent
  and needs an interactive approval per connection, which an agent session cannot give.
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
- [x] Item 1: contract package, `internal/lidar/jobs` (`c8c542a7c`)
- [x] Item 2: worker and client, `internal/lidar/jobs/runner`, `velocity worker`, `velocity jobs`
      (`928f61897`, `edf68ef4a`); benchmark over `kirk0` run end to end on the Mac

### Outstanding

- [ ] Item 3: the smoke attempt on `swan` (`M`)
- [ ] Item 4: hub and dashboard (`L`)
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
