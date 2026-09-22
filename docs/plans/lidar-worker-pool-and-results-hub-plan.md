# LiDAR worker pool and results hub

- **Status:** Proposed, September 21, 2026
- **Canonical:** This document
- **Layers:** offline analysis, L3–L8 LiDAR, platform, macOS visualiser, annotation
- **Supersedes:** the remote-topology portions of [Distributed sweep workers](lidar-distributed-sweep-workers-plan.md)
- **Related:** [State-estimation plan](lidar-state-estimation-plan.md), [Analysis run infrastructure](lidar-analysis-run-infrastructure-plan.md), and [Point annotation and temporal object datasets](lidar-point-annotation-and-object-dataset-plan.md)

## 1. Decision

The analysis fleet is a pool, not a small datacentre pretending that every box has
made a lifelong commitment. A worker may be powered down, noisy, moving house, or
otherwise engaged in being a computer from the last century. The system must still
know what it did when it was awake.

Use a central results hub as the only authority for job state, run comparison,
annotation metadata, and macOS-facing APIs. Workers are independent, disposable
executors. They claim one durable job at a time, read a locally available PCAP only
after verifying its content identity, and upload an immutable result bundle. No
shared SQLite database and no shared filesystem are required.

The initial pool is:

| Role                     | Host                | Availability          | Responsibility                                                                         |
| ------------------------ | ------------------- | --------------------- | -------------------------------------------------------------------------------------- |
| Results hub and main API | NAS                 | normally on           | authoritative queue, result ingest, analysis index, annotation metadata, and macOS API |
| Burst worker             | `swan` Unraid blade | intermittent and loud | bounded state-estimation, benchmarks, and parameter sweeps                             |
| NAS worker               | NAS                 | normally on           | small or long-running jobs, result validation, and fallback execution                  |
| macOS visualiser         | operator Mac        | interactive           | review annotation packs and submit revision-safe annotation changes through the hub    |

`swan` already has a directly usable SSH key path and a persistent Docker runner.
It must not become the source of truth merely because it has the most dramatic
fans.

## 2. Boundaries and invariants

1. **The hub owns state.** It is the sole writer of the central job database and
   the only place that declares a result accepted, superseded, or comparable.
2. **A worker owns only local execution.** It may cache a job and its output while
   offline. It cannot silently change central job state or overwrite a result.
3. **PCAP names are labels, hashes are identity.** `kirk0.pcapng` on two hosts is
   comparable only if the ordered capture manifest says its bytes are identical.
4. **Every result is write-once.** A retry receives a new attempt ID. A failed
   directory remains evidence, rather than becoming a tray for the next attempt.
5. **Comparison requires the whole run identity.** Source manifest digest,
   tuning digest, executable/source identity, requested window, sensor identity,
   enabled experiments, and schema version must all match. Same filename is not a
   scientific instrument.
6. **Raw captures remain local.** Workers exchange manifests and result bundles,
   not road recordings, unless an operator explicitly stages a capture to the
   destination host.
7. **Annotations are human evidence.** A worker may create an immutable candidate
   pack, but it cannot present a generated annotation as reviewed truth.

## 3. Topology

```text
                      Mac visualiser
                 annotations and review UI
                              |
                              v
     +--------------------------------------------------+
     | NAS results hub and main service                 |
     | main API, job queue, result ingest, annotation   |
     | index, central analysis, durable object storage  |
     +--------------+-------------------+---------------+
                    ^                   ^
                    | result bundles    | result bundles
                    | and heartbeats    | and heartbeats
                    |                   |
         +----------+---------+   +-----+-----------+
         | swan burst worker  |   | NAS worker      |
         | may be unavailable |   | usually present |
         | local PCAP cache   |   | local PCAP cache|
         +--------------------+   +-----------------+
```

The hub uses normal authenticated HTTPS or the private LAN. Each worker initiates
its own outbound heartbeat and result synchronisation. This means a blade that is
off is simply unavailable, rather than a special kind of emergency. It also avoids
opening inbound ports on every future worker merely to discover that it has been
unplugged.

The central service should live on the NAS once it is online. Its ordinary main
API remains the macOS application's supported service boundary. The worker queue
and results dashboard are a small companion UI, not a competing copy of the
visualiser.

## 4. Identities that make results portable

### 4.1 Capture manifest

Before a job may run, the hub creates or verifies an immutable source manifest.
It lists captures in replay order and records, for each one:

| Field                         | Meaning                                                                                      |
| ----------------------------- | -------------------------------------------------------------------------------------------- |
| logical capture ID            | stable human label, such as `morg0`                                                          |
| ordinal                       | replay order within the case                                                                 |
| relative path hint            | where this worker normally finds the capture, never an authority outside its configured root |
| byte size                     | quick accidental-mismatch guard                                                              |
| SHA-256                       | content identity                                                                             |
| container format and UDP port | replay interpretation evidence                                                               |

The manifest itself has a SHA-256 digest. A worker advertises the manifest digests
it can satisfy from its local cache. It checks the bytes before a job starts and
fails closed on a mismatch. A NAS mount, a blade-local copy, and a future archive
can therefore produce comparable results without sharing a path or a disk.

Hashing a multi-gigabyte PCAP is deliberately a real operation. Cache verified
digests with file size, device identity, and modification metadata as a hint, but
re-hash whenever those hints differ or an operator asks for verification.

### 4.2 Run identity and result bundle

The hub derives a `run_identity` from the canonical job request. It includes:

| Component                         | Why it is required                                                                      |
| --------------------------------- | --------------------------------------------------------------------------------------- |
| source-manifest digest            | fixes the actual input bytes and order                                                  |
| tuning digest and experiment list | fixes pipeline behaviour                                                                |
| code identity                     | Git commit where available, otherwise an archived source-tree digest and build metadata |
| executable identity               | binary digest, Go version, build tags, and dependency mode                              |
| replay contract                   | sensor, UDP port, warm-up, scoring window, settling policy, and measurement mode        |
| output schema version             | makes future readers reject an incompatible bundle                                      |

The worker returns a result bundle with this identity, a worker profile, machine
measurements, structured summaries, logs, and any immutable evidence paths. The
hub verifies the identity before accepting it. Performance figures remain
host-specific; correctness and scoring summaries with matching identities may be
compared across hosts.

## 5. Job lifecycle

| State       | Owner          | Meaning                                                                                   |
| ----------- | -------------- | ----------------------------------------------------------------------------------------- |
| `queued`    | hub            | validated request waits for a compatible worker                                           |
| `leased`    | hub and worker | worker has a short renewable claim, but has not started replay                            |
| `running`   | worker         | replay or analysis is active; progress and heartbeat are advisory, lease is authoritative |
| `uploading` | worker         | local immutable bundle is being sent or resumed                                           |
| `verifying` | hub            | hub checks schema, digests, identity, and result completeness                             |
| `accepted`  | hub            | central analysis may use this result                                                      |
| `failed`    | hub            | attempt failed with preserved error evidence                                              |
| `cancelled` | hub            | operator cancelled before acceptance; partial output remains marked partial               |
| `lost`      | hub            | lease expired without a valid completion; retry becomes a new attempt                     |

Workers poll or maintain a single outbound connection. A returned worker reports
its version, capabilities, free space, resource profile, and verified source
manifest digests. The scheduler should favour data locality and compatibility,
then prefer the NAS for ordinary work and `swan` for burst work. It must never
wait indefinitely for an intermittent blade when the NAS worker can complete the
same job.

One worker runs one heavy replay at a time by default. The scheduler may split a
large sweep into independent attempts, but it must not make one old Xeon run three
PCAP replays at once just because JSON makes that look tidy.

## 6. API and UI split

### 6.1 Central hub API

The hub is the public local-network API for the dashboard and macOS client.

| Method | Path                                   | Purpose                                                             |
| ------ | -------------------------------------- | ------------------------------------------------------------------- |
| `GET`  | `/api/analysis/workers`                | worker inventory, availability, and capabilities                    |
| `GET`  | `/api/analysis/jobs`                   | filtered queue and recent attempts                                  |
| `POST` | `/api/analysis/jobs`                   | create a schema-validated job, never arbitrary shell input          |
| `GET`  | `/api/analysis/jobs/{id}`              | status, provenance, and result links                                |
| `POST` | `/api/analysis/jobs/{id}/cancel`       | request cancellation of a non-accepted attempt                      |
| `GET`  | `/api/analysis/results/{id}`           | accepted bundle metadata and analysis summary                       |
| `POST` | `/api/analysis/results/{id}/reanalyse` | create a separate central analysis task, preserving raw result data |
| `GET`  | `/api/annotations/packs`               | immutable annotation-pack inventory                                 |
| `POST` | `/api/annotations/{pack}/revisions`    | revision-safe human annotation submission                           |

### 6.2 Worker API

The worker surface is private to the hub. It accepts only signed or mutually
authenticated job leases and result-transfer acknowledgements. It has no endpoint
that accepts an arbitrary command, host path, Docker option, or source URL.

For the present blade, reserve `swan:8084` for a small LAN-only worker console:
health, capability report, current job, recent local attempts, log tail, and a
link to the central result. Do not expose it outside the LAN. The NAS hub may use
its own port 8084 for the central dashboard because it is a different host, but
the central application's documented origin must be explicit in the macOS
configuration.

The first UI should show a table, not a control room from a space opera:

| View             | Minimum content                                                                |
| ---------------- | ------------------------------------------------------------------------------ |
| Pool             | name, availability, PCAP manifests, version, free space, active lease          |
| Queue            | identity, source set, requested window, assigned worker, status, retry lineage |
| Attempt          | immutable provenance, progress, log tail, bundle checks, failure reason        |
| Result           | compatible comparisons, host-specific performance, and analysis state          |
| Annotation packs | source digest, review state, revision, and Mac download/open link              |

## 7. Annotation across hosts

The macOS visualiser does not talk to whichever worker happened to make a result.
It talks to the NAS hub. This keeps a person from reviewing a pack that disappears
when a blade is turned off, a tradition best left to haunted filing cabinets.

1. A worker may emit a candidate pack reference only after its result bundle has
   passed hub verification.
2. The hub stores the immutable pack, pack digest, source-manifest digest, run
   identity, and export policy.
3. The Mac downloads or opens that named immutable pack, creates local edits, and
   submits an annotation revision with its parent revision and source digest.
4. The hub serialises revisions, rejects stale or mismatched-source writes, and
   keeps the previous revision retrievable.
5. Central analysis consumes only explicitly reviewed revisions. Suggestions and
   worker-generated candidates remain suggestions.

The Mac may work offline against one immutable pack. Its pending revision stays
local until the hub accepts it. A changed pack digest or parent revision requires
an explicit rebase or review; no nearest-neighbour rescue act is permitted.

## 8. Current `swan` delivery and the first repair

`swan` currently has a direct root SSH key path, a non-privileged Unraid Docker
runner, read-only PCAP mount, persistent worker state, persistent results share,
CPU pinning, memory limit, and Docker autostart. It has a capped state-estimation
smoke attempt over `morg0`, `kirk0`, and `claren0`.

The first attempt wrote its source manifest and then failed before scoring because
the detached runner executed outside its staged repository. A legacy configuration
lookup therefore could not find `config/tuning.defaults.json`. Do not delete the
partial result: it records a real failed attempt. Create a new attempt ID whose
runner changes directory to the exact staged source snapshot before invocation.

The repair is also an acceptance test for the job contract:

1. Source snapshot identity is recorded in the job.
2. The working directory is explicit, never inherited from Docker's default.
3. The custom three-capture corpus and index are retained alongside the job.
4. The source manifest is new for the new attempt and hashes all three PCAPs.
5. First and repeat bounded replays both finish, their deterministic evidence is
   compared, and the hub-style result bundle marks the attempt accepted or failed.

## 9. Delivery order

### Phase A: make the blade repeatable

- Repair and complete the fresh three-capture state-estimation smoke attempt.
- Write a durable local attempt record, including source snapshot digest, capture
  manifest digest, command contract, start/end UTC times, and exit outcome.
- Provide the read-only `swan:8084` status page. It does not submit arbitrary
  jobs and it does not become a second scheduler.

**Acceptance:** an SSH operator and the page report the same current attempt and
the same immutable result directory after a Docker or host restart.

### Phase B: establish the NAS hub

- Put the central SQLite database, bundle store, and analysis service on the NAS.
- Add worker inventory, compatibility checking, leases, idempotent bundle upload,
  and retry lineage.
- Add the central dashboard and a supported macOS configuration endpoint.

**Acceptance:** a worker that goes offline mid-run leaves a recoverable local
bundle, and a later upload produces exactly one accepted central result.

### Phase C: add the NAS worker and flexible scheduling

- Register the NAS as a worker with the same capability and source-manifest
  rules as `swan`.
- Prefer a local verified PCAP cache; stage captures explicitly when absent.
- Add availability windows and a `draining` state so a noisy blade can finish or
  decline new work without being treated as broken.

**Acceptance:** one compatible job completes on either host and is correctly
grouped by run identity, while performance figures retain their host label.

### Phase D: connect annotation and central analysis

- Publish verified candidate annotation packs through the hub.
- Add revision-safe Mac synchronisation and reviewed/unreviewed boundaries.
- Run central analysis only from accepted result bundles and reviewed annotation
  revisions.

**Acceptance:** an annotation made on the Mac survives worker loss, rejects a
stale source digest, and is traceable to one accepted run identity.

## 10. Non-goals for the first delivery

- No public internet exposure, cloud queue, shared SQLite file, or Docker socket
  in the API container.
- No automatic PCAP replication based on filename alone.
- No cross-host performance league table without an explicit host class.
- No worker-generated annotation treated as ground truth.
- No requirement that `swan` be awake for ordinary NAS-hosted analysis.

## 11. Handoff checklist

The next implementation session should begin by reading this document, the
state-estimation plan, and the annotation plan. Then it should:

1. repair the fresh `swan` smoke attempt without overwriting its failed precursor;
2. implement the smallest durable local attempt record and 8084 read-only status;
3. design the central hub schema and ingest protocol before adding a second worker;
4. keep every cross-host comparison behind source-manifest and run-identity checks;
5. add annotation synchronisation only through the central hub, never directly
   from the Mac to an intermittent worker.
