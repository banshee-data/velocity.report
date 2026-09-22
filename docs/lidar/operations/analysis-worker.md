# Analysis worker

How to run analysis jobs on another host with `velocity worker`, and drive it with
`velocity jobs`.

- **Status:** Implemented; hub and dashboard to follow
- **Layers:** L9 Endpoints, offline analysis
- **Canonical:** [lidar-worker-pool-and-results-hub-plan.md](../../plans/lidar-worker-pool-and-results-hub-plan.md)
- **Related:** [lidar-job-queue-implementation-plan.md](../../plans/lidar-job-queue-implementation-plan.md)

## What it is

A worker is one host that runs one job at a time: given a capture, a tuning config and either
one kind of job or a campaign of configs, it verifies the capture's bytes, runs the kind's tool,
keeps everything the tool wrote as an immutable bundle, and serves that bundle back over HTTP.
`swan` is the first; the NAS will be the second.

There is no central queue yet. Each worker has its own, and the client talks to a worker
directly. The hub that will own the queue and index results across workers is the next item
in the implementation plan, and it will consume the same API and bundles.

## Starting a worker

```bash
export VELOCITY_WORKER_TOKEN=$(openssl rand -hex 24)
velocity worker --pcap-root /mnt/pcap --work-dir /var/lib/velocity-worker \
  --listen 0.0.0.0:8084 --source-repo /opt/velocity.report
```

| Flag            | Meaning                                                                                                                                      |
| --------------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| `--pcap-root`   | Where the captures are. A job's captures are found under it and hashed before anything runs                                                  |
| `--work-dir`    | Attempts, bundles, logs and the capture digest cache. Every attempt is a directory under it                                                  |
| `--token`       | Required. Every request but `/health` must carry it as a bearer token; without one the API serves nothing. Also `$VELOCITY_WORKER_TOKEN`     |
| `--listen`      | Loopback or a private address only. The worker refuses a public one                                                                          |
| `--source-repo` | A clone of this repository on the worker, for building a job's tool at the job's commit. Without it, only in-process kinds (`benchmark`) run |
| `--id`          | The worker's name in listings. Defaults to the hostname                                                                                      |

For the tool kinds the worker needs `git`, a Go toolchain and libpcap's headers, because it
builds `cmd/tools/<tool>` from the staged commit with `-tags pcap`. Built tools are cached under
`<work-dir>/tools` by commit.

The binary for a Linux worker comes from `make build-radar-static` (the same static build as the
Pi), or from `go build -tags pcap ./cmd/velocity` on the worker itself.

## Submitting a job

```bash
export VELOCITY_WORKER_URL=http://swan:8084 VELOCITY_WORKER_TOKEN=…
velocity jobs kinds                       # what this worker can run
velocity jobs submit benchmark-kirk0.json # one job
velocity jobs wait att_…                  # until it is accepted, failed, cancelled or lost
velocity jobs log att_…
velocity jobs fetch att_… --out ./results # the bundle as a tar
```

A request is JSON in the contract's shape (`internal/lidar/jobs`): the kind, a capture manifest
(each capture's logical id, path hint under the root, size, SHA-256, container and UDP port), the
whole tuning document, the commit to build from, the replay contract, and the kind's typed
parameters. Nothing in it is a command line; the worker composes the invocation. An unknown field
is refused rather than ignored.

```json
{
  "kind": "benchmark",
  "capture_manifest": {
    "schema_version": 1,
    "sensor_id": "hesai-pandar40p",
    "captures": [
      {
        "id": "kirk0",
        "ordinal": 0,
        "relative_path": "kirk0.pcapng",
        "byte_size": 200657872,
        "sha256": "sha256:2864ebde…",
        "container": "pcapng",
        "udp_port": 2369
      }
    ]
  },
  "tuning": { "…": "the whole resolved tuning document" },
  "code": { "git_sha": "339548fbe88926851b93178f09c3155d39e7957e", "build_tags": ["pcap"] },
  "replay": { "sensor_id": "hesai-pandar40p", "duration_seconds": 20, "warmup_seconds": 0 },
  "params": { "profile": "detect", "repeats": 1 }
}
```

| Kind                        | Tool                              | Parameters                                                                                | Summary               |
| --------------------------- | --------------------------------- | ----------------------------------------------------------------------------------------- | --------------------- |
| `benchmark`                 | `lidar-bench`, in-process         | `profile`, `repeats`                                                                      | `benchmark.json`      |
| `state_estimation_baseline` | `lidar-state-estimation-baseline` | `case`, `surface_ground`, `keep_evidence`                                                 | `phase0-summary.json` |
| `track_scorecard`           | `lidar-track-scorecard`           | `evidence_job` (an accepted baseline attempt on the same worker), `scoring_start_seconds` | `scorecard.json`      |

The two tool kinds live on `dd/docs/state-est` today: run them from a commit on that branch.

## A campaign

A campaign is one base request and a list of named configs, each a set of dotted-key overrides
on the tuning document with its own experiments and parameters. It is what
`data/experiments/try/option-scorecard/pass7-configs.json` is.

```json
{
  "kind": "state_estimation_baseline",
  "base": { "…": "a request as above, with params.case set" },
  "configs": [
    { "name": "eps05", "overrides": { "l4.dbscan_xy_v1.foreground_dbscan_eps": 0.5 } },
    {
      "name": "eps08-cascade",
      "overrides": { "l4.dbscan_xy_v1.foreground_dbscan_eps": 0.8 },
      "experiments": ["cascade"]
    }
  ],
  "note": "pass 8"
}
```

`velocity jobs campaign FILE` queues one child attempt per config, in order, under a parent
that completes when they all have. The parent's bundle holds `campaign.json`, a table of every
config's state and summary. An override that names a key the document does not have is refused,
so a typo cannot run the defaults under a new name.

## What an attempt leaves behind

```text
<work-dir>/attempts/att_20260922T012907.707779_ef097d56/
  record.json      the job as submitted, its run identity, state, timestamps, progress, failure
  log.txt          what the worker did, with UTC timestamps
  bundle/
    bundle.json    identity, worker profile, outcome, the kind's summary, every file's digest
    benchmark.json the tool's output, and whatever else it wrote
```

States are the pool plan's: `queued`, `leased`, `running`, `uploading`, `verifying`, then
`accepted`, `failed`, `cancelled` or `lost`. A failed attempt keeps what the tool wrote, under a
bundle marked partial. An attempt that was running when the worker stopped is marked lost on the
next start. Nothing is ever overwritten; a retry is a new attempt.

Two accepted bundles with the same `identity_digest` computed the same thing: the same capture
bytes in the same order, tuning, experiments, commit, build tags, replay window and parameters.
Where they ran is in the worker profile and not in the identity, so performance figures keep
their host and correctness figures compare across hosts.

## The API

| Method | Path                                 | Purpose                                                           |
| ------ | ------------------------------------ | ----------------------------------------------------------------- |
| `GET`  | `/health`                            | Liveness; the only route without the token                        |
| `GET`  | `/api/worker/status`                 | Worker profile, queue depth, verified captures, kinds, free space |
| `GET`  | `/api/worker/kinds`                  | Every kind, and whether this build can run it                     |
| `POST` | `/api/worker/jobs`                   | Submit a job request                                              |
| `POST` | `/api/worker/campaigns`              | Submit a campaign                                                 |
| `GET`  | `/api/worker/jobs?state=`            | Attempts, newest first                                            |
| `GET`  | `/api/worker/jobs/{id}`              | The full record                                                   |
| `GET`  | `/api/worker/jobs/{id}/log?lines=`   | Log tail                                                          |
| `POST` | `/api/worker/jobs/{id}/cancel`       | Cancel; a campaign cancels its unfinished configs                 |
| `GET`  | `/api/worker/jobs/{id}/bundle`       | The bundle manifest                                               |
| `GET`  | `/api/worker/jobs/{id}/files/{path}` | One bundle file, by its manifest path                             |
| `GET`  | `/api/worker/jobs/{id}/bundle.tar`   | The whole bundle                                                  |
