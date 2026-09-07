# LiDAR captures: multi-file replay cases (v0.6.0)

- **Status:** Active
- **Layers:** Cross-cutting (LiDAR L1 ingest, Go API, database, Svelte frontend)
- **Target:** v0.6.0; the field workflow now produces many 5-minute PCAPs per site visit, and
  a replay case that can only name one file cannot describe a static period that straddles a
  file boundary.
- **Companion plans:** [lidar-replay-case-terminology-alignment-plan](lidar-replay-case-terminology-alignment-plan.md) <!-- link-ignore -->
- **Canonical:** [replay-case-management-implementation](../lidar/operations/replay-case-management-implementation.md)

## Motivation

Field capture writes a rolling series of ~5-minute PCAP files to an external volume. A single
site visit is therefore 3-12 files, not one. The sensor is parked for most of that time and in
motion (being driven or carried) for the rest; only the parked stretches are useful as replay
material, and the motion stretches are reserved for the SLAM work described in
[motion-capture.md](../lidar/operations/motion-capture.md).

Two facts collide:

1. A parked stretch is bounded by _when the vehicle stopped and started_, not by when the
   capture tool rolled its output file. A useful static period routinely straddles one or more
   file boundaries.
2. `lidar_replay_cases` carries exactly one `pcap_file` column, and
   `network.ReadPCAPFile` opens exactly one file per replay.

The consequence of inaction is that every interesting capture must first be manually
concatenated into a unioned PCAP before it can become a replay case. That doubles the storage
on a volume already holding 1.2 GB per five minutes, loses the provenance of which source file
a frame came from, and puts a manual `mergecap` step between the operator and every evaluation
run.

This plan makes a replay case an ordered _set_ of capture files plus a session-relative window,
and makes the L1 replay path read that set sequentially — as one continuous packet stream, with
no intermediate file on disk.

## Current state

| Concern                      | Where it lives today                                                                          | Shape                                                                                            |
| ---------------------------- | --------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| PCAP discovery               | [pcap_files_api.go](../../internal/lidar/server/pcap_files_api.go)                            | One flat walk of `--lidar-pcap-dir`, capped at 500 files, no persistence                         |
| Replay case record           | [scene_store.go](../../internal/lidar/storage/sqlite/scene_store.go) `ReplayCase`             | Single `PCAPFile string`, plus `PCAPStartSecs` / `PCAPDurationSecs`                              |
| Replay case schema           | `lidar_replay_cases` (migration 031)                                                          | `pcap_file TEXT`, indexed by `idx_lidar_replay_cases_pcap`                                       |
| Replay execution             | [pcap.go](../../internal/lidar/l1packets/network/pcap.go) `ReadPCAPFile`                      | One `pcap.OpenOffline` per call; start/duration thresholds derived from that file's first packet |
| Packet extent probe          | [pcap_count.go](../../internal/lidar/l1packets/network/pcap_count.go) `CountPCAPPackets`      | Already returns `Count`, `FirstTimestampNs`, `LastTimestampNs`                                   |
| Motion/static classification | [pcapsplit](../../internal/lidar/pcapsplit/)                                                  | Working; `BuildTimeline` is pure Go and libpcap-free, `Run` is behind the `pcap` tag             |
| Motion/static invocation     | `velocity lidar pcap-split` ([split.go](../../internal/cmd/lidar/split.go))                   | CLI only; no API, no job record, no persisted timeline                                           |
| Frontend                     | [replay-cases/+page.svelte](../../web/src/routes/lidar/replay-cases/+page.svelte) (745 lines) | CRUD list plus a one-shot scan panel; no timeline, no sessions, no health                        |

Two pieces of existing work carry most of the weight and should not be rewritten:
`pcapsplit.BuildTimeline` already produces exactly the motion/static period list the UI needs,
and `CountPCAPPackets` already produces exactly the packet-time extents the continuity check
needs.

## Findings

| Area                     | Current state                                                           | Severity | Release view                                                                   |
| ------------------------ | ----------------------------------------------------------------------- | -------- | ------------------------------------------------------------------------------ |
| Case → file cardinality  | 1:1, enforced by a scalar column                                        | High     | Blocks the entire workflow; must land first                                    |
| Multi-file replay        | Absent; `ReadPCAPFile` is single-file                                   | High     | Blocks evaluation runs over a real site visit                                  |
| Boundary continuity      | Unchecked; nothing validates that two files abut                        | High     | A silent gap corrupts tracking without any signal                              |
| Frame assembly at a seam | Undefined; a revolution split across two files has never been exercised | Medium   | Produces one malformed frame per seam if unhandled                             |
| Capture root selection   | Single `--lidar-pcap-dir` flag, fixed at process start                  | Medium   | An external volume cannot be chosen without a restart                          |
| Scan persistence         | None; every scan is a fresh walk                                        | Medium   | No drift detection, no incremental update                                      |
| Motion pass from UI      | CLI only                                                                | Medium   | Operator must drop to a shell mid-workflow                                     |
| Timeline visualisation   | Absent                                                                  | Medium   | The coverage question ("what do we have, and when?") is unanswerable in the UI |
| Naming                   | "Replay Cases" in nav and route                                         | Low      | Cosmetic; fold into the UI phase                                               |

## Design / approach

### The three nouns

The mockup and the field workflow agree on three levels, and the plan adopts them:

- **Capture file** — one PCAP on disk. Immutable. Has a packet-time extent, a size, a digest,
  and a health verdict.
- **Capture session** — a maximal run of capture files from one sensor whose packet-time
  extents abut within tolerance. Derived, not authored: the indexer computes sessions from
  extents. This is what the UI's collapsible group row shows.
- **Replay case** — an _ordered subset_ of one session's files plus a session-relative window.
  Authored, by an operator or by the split engine. This is the unit an evaluation run consumes.

A replay case therefore no longer names a file; it names a window over a session.

### Continuity: the load-bearing invariant

For adjacent files `A`, `B` the seam gap is `B.FirstPacket - A.LastPacket`. Three grades, with
the tolerances the field workflow asked for:

| Grade        | Condition          | Meaning                                  | Replay behaviour                                                 |
| ------------ | ------------------ | ---------------------------------------- | ---------------------------------------------------------------- |
| `seamless`   | `                  | gap                                      | <= 10ms`                                                         | Capture tool rolled the file without dropping a packet | Feed straight through; the in-flight revolution completes normally |
| `acceptable` | `10ms < gap <= 1s` | A real but tolerable loss                | Feed through, but **discard the revolution straddling the seam** |
| `broken`     | `gap > 1s`         | Too much missing to call this one stream | Refuse to build a sequence across it                             |
| `overlap`    | `gap < -10ms`      | Files overlap; duplicate packets         | Refuse; the operator has mis-selected files                      |

The `acceptable` frame-drop rule is the subtle part. A Hesai revolution is assembled by azimuth
wrap, so feeding two files sequentially across a 400 ms gap does not produce an _error_ — it
produces one plausible-looking frame containing points from two different moments half a second
apart. That frame is worse than a missing frame, because L3 will treat it as observed
background. Dropping it is cheap (one frame in 6000) and removes the failure mode entirely.

At `seamless` the same reasoning inverts: at 10 ms the two halves of the revolution are within
one-fifth of a 20 Hz rotation period, the points are genuinely contiguous, and dropping the
frame would discard good data at every file boundary.

### No intermediate files

`Sequence.Plan(startSecs, durationSecs)` converts a session-relative window into a list of
per-file read steps — path, file-relative start offset, file-relative duration, and whether to
drop the frame in flight when that step begins. The existing `ReadPCAPFile` is then called once
per step against **one shared frame builder and parser**, so state (background model, motor
speed, track IDs) carries across the boundary while the packet source changes underneath. No
`mergecap`, no temporary union, no second copy of 1.2 GB.

Cancellation, progress, and packet counting aggregate across steps: total packets is the sum of
the steps' counts, which `CountPCAPPackets` already provides per file at index time.

### Capture roots and the safe-directory boundary

`--lidar-pcap-dir` exists as a containment boundary: only files beneath it can be replayed. A
UI that lets an operator type an arbitrary server-side path would dissolve that boundary and
turn the replay endpoint into an arbitrary-file-read primitive.

The design keeps the boundary and moves the _choice_ inside it:

- `--lidar-capture-root` becomes repeatable, and defaults to the current `--lidar-pcap-dir`
  value when not given, so existing deployments are unchanged.
- The UI can select among, scan, and index the configured roots. It cannot add one.
- Adding a root is an operator action (flag, or config file), the same trust level as choosing
  the database path.
- Every path returned by the API stays relative to its root, and every path accepted by the API
  is resolved against its root and rejected if it escapes (the check
  `pcap_files_api.go` already performs, applied per root).

An external drive is then supported by naming its mount point as a root at start-up; a root
whose directory is absent is reported `unreachable` rather than failing the process, which is
what the mockup's "volume unreachable" state renders.

### Where motion classification runs

`pcapsplit.BuildTimeline` is already the right function and already libpcap-free. This plan
adds a persistence and invocation layer around it rather than touching the classifier:

- A `motion_pass` job per capture file, queued from the API, executed by a bounded worker pool.
- Results stored as `lidar_capture_motion_periods` rows, one per `MotionPeriod`.
- The UI reads periods, normalises them onto the session timeline, and renders the strip.

Sessions inherit their strip by concatenating their files' periods in packet-time order, which
is exactly what the mockup's session-level track does.

## Scope

### Item 1: capture sequencing core

**Summary:** A pure-Go package that orders capture files, grades their seams, and turns a
session-relative window into per-file read steps.

**Steps:**

1. Add `internal/lidar/capseq` with `Segment`, `Seam`, `SeamGrade`, `Sequence`, `Tolerances`.
2. Implement `Build(segments, tolerances)`: sort by first-packet time, grade every seam,
   compute covered/lost duration, return the worst grade.
3. Implement `Sequence.Plan(startSecs, durationSecs) []ReadStep` including the
   `DropFrameAtStart` flag derived from the preceding seam's grade.
4. Table-driven tests: seamless, acceptable, broken, overlap, single file, unordered input,
   window entirely inside one file, window spanning three files, zero-length window.

**Milestone:** v0.6.0. No libpcap dependency, so it runs in the default `go test ./...`.

### Item 2: multi-file replay execution

**Summary:** Drive `ReadPCAPFile` from a plan, sharing one parser and frame builder across steps.

**Steps:**

1. Add `network.ReadPCAPSequence` (behind the `pcap` tag) taking `[]capseq.ReadStep`.
2. Add a `DropNextFrame()` capability to the frame builder, invoked at a step boundary when the
   step's `DropFrameAtStart` is set.
3. Aggregate progress across steps against the summed packet count.
4. Extend `ReplayConfig` with the ordered file list; keep the single-file field working by
   promoting it to a one-element sequence.
5. Integration test over two truncated fixtures cut from a reference capture at a known seam.

**Milestone:** v0.6.0.

**Landed with two constraints.** Multi-file replay requires `analysis` speed mode: realtime and
scaled replay pace packets against a single file's own clock, and crossing a join there needs a
sequence-aware variant of `ReadPCAPFileRealtime`. A multi-file request in either mode is refused
rather than silently replaying only the first file.

The second constraint is a finding. In `kirk0.pcapng` the payload clock does not advance across
a gap in the capture the way the pcap timestamps do, so the frame builder sees a continuous
point stream either side of 400 ms of missing packets and cannot itself tell anything is wrong.
That validates the design — the drop is decided from capture-time sequencing at plan time, never
inferred downstream — but it means the integration test asserts the cost of a join (exactly one
revolution, only when marked) rather than the discarded revolution's shape, which is asserted in
the L2 unit tests where it can be constructed deterministically. Whether that clock behaviour is
a property of this capture or of the parser's LiDAR timestamp mode is worth its own look.

### Item 3: capture index and roots

**Summary:** Persist what is on the volume, and detect what changed since the last scan.

**Steps:**

1. Migration 039: `lidar_capture_roots`, `lidar_capture_files`, `lidar_capture_sessions`.
2. Make `--lidar-capture-root` repeatable, defaulting to `--lidar-pcap-dir`.
3. Indexer: walk each root, stat, digest, probe extents with `CountPCAPPackets`, upsert.
4. Session derivation from extents using `capseq` tolerances.
5. Drift report: new on disk, missing from disk, digest mismatch.
6. API: `GET /api/lidar/capture/roots`, `POST /api/lidar/capture/scan`,
   `GET /api/lidar/capture/sessions`, `GET /api/lidar/capture/files`,
   `POST /api/lidar/capture/session/label`.

**Milestone:** v0.6.0.

**Landed as a two-phase scan.** The original step 3 folded stat, digest and extent probe into
one walk. Measured against the field volume that is untenable: probing an extent means reading
every byte, and `/Volumes/lidar/lidar/s2` holds 195 captures of roughly 700 MB. A scan that
probed them all would run for hours and would re-run on every look.

So a scan is cheap metadata — path, size, mtime, and a content tag over the file's first and
last mebibyte plus its length — and the extent probe is scoped to the files that scan found new
or materially changed. A volume that has not moved costs one walk and no reads. The content tag
is deliberately not a whole-file checksum: hashing 700 MB per file on every scan buys nothing
that size and mtime do not already give, except against deliberate tampering, while it does
catch the failure that actually happens, which is a capture truncated or rewritten in place.

A file that was merely touched — copying a volume moves every mtime without changing a byte —
keeps the extent it already has. One whose size or tag moved has its probe reset, because an
extent recorded for the old bytes says nothing about the new ones.

### Item 4: motion pass as a job

**Summary:** Run the existing classifier from the API and persist its timeline.

**Steps:**

1. Migration 040: `lidar_capture_motion_periods`, `lidar_capture_jobs`.
2. Bounded worker pool; one job per capture file; progress and cancellation.
3. `POST /api/lidar/capture/files/{id}/motion-pass`, `GET .../jobs`.
4. Store `pcapsplit.MotionPeriod` rows keyed by capture file.

**Milestone:** v0.6.0.

### Item 5: replay case ↔ file association

**Summary:** Make a replay case reference an ordered set of files and a session window.

**Steps:**

1. Migration 041: `lidar_replay_case_files (replay_case_id, ordinal, capture_file_id)`;
   backfill one row per existing case from `pcap_file`; retain `pcap_file` for one release as
   the read-only legacy projection.
2. Extend `ReplayCase` with `Files []ReplayCaseFile` and session-relative window fields.
3. Reject a case whose files do not form a sequence with worst grade `acceptable` or better.
4. Update the replay launch path to build a plan from the case.

**Milestone:** v0.6.0.

### Item 6: Captures page

**Summary:** Replace the replay-cases CRUD page with the Captures view from the mockup.

**Steps:**

1. Route `/lidar/replay-cases` → `/lidar/captures`; nav label "Captures"; redirect the old path.
2. Session list with collapsible file rows, status glyph column, and per-row motion strip.
3. Coverage timeline: which periods have files, and static/motion within them.
4. Root selector, scan control, drift pill, unreachable and first-scan empty states.
5. Detail slide-over: metadata, health checks, motion strip, jobs, actions.
6. Multi-select → create a case from the selected files, with seam grades shown before commit.

**Milestone:** v0.6.0. Items 1-5 must land first; this is the last phase, not the first.

## Dependencies

- `pcapsplit.BuildTimeline` and `CountPCAPPackets` are reused as-is; a change to either
  re-sequences items 1 and 4.
- Item 2 needs a frame-builder hook (`DropNextFrame`) in L2; that is the only L2 change.
- Items 3-6 all depend on item 1's tolerances being the single definition of "continuous".
- The perf gate baselines are untouched: no tuning parameter changes, so no recapture.

## Risks

| Risk                                                   | Likelihood        | Impact                                 | Mitigation                                                                                                                 |
| ------------------------------------------------------ | ----------------- | -------------------------------------- | -------------------------------------------------------------------------------------------------------------------------- |
| Frame straddling a seam is silently malformed          | High if unhandled | High — poisons the L3 background model | `DropFrameAtStart` on any non-seamless seam; assert in the integration test that frame count drops by exactly one per seam |
| Runtime-configurable roots widen the file-read surface | Medium            | High                                   | Roots are operator-configured only; UI selects, never adds; per-root escape check retained                                 |
| Digesting 1.2 GB files on every scan is slow           | High              | Medium                                 | Digest only when size or mtime changed; store the digest with the row                                                      |
| Session derivation disagrees with operator intent      | Medium            | Medium                                 | Sessions are derived and advisory; a case may name any subset that forms a valid sequence                                  |
| `pcap_file` removal breaks external consumers          | Low               | Medium                                 | Keep the column as a read-only projection for one release; remove in v0.6.1                                                |
| Scope creep into the split/cut-review UI               | High              | Medium                                 | Cut-point review is explicitly out of scope here; it depends on items 1-6 and gets its own plan                            |

## Checklist

### Complete

- [x] Item 1: `internal/lidar/capseq` sequencing core — ordering, seam grading, windowed
      planning; 98.1% statement coverage, no libpcap dependency
- [x] Item 2: `ReadPCAPSequence`, the L2 `DropNextFrame` hook, `ReplayConfig.ReplayFiles`, and
      the server's `buildReplaySequence`; sequencing proven transparent against `kirk0.pcapng`
      (a capture cut in two and replayed as a sequence yields identical frames and points to the
      uncut original)
- [x] Item 3: `internal/lidar/capindex` (scan, drift, session derivation, indexer),
      migration 039, `CaptureStore`, repeatable `--lidar-capture-root`, and the
      `/api/lidar/capture/*` endpoints

### Outstanding

- [ ] Item 4: motion pass job runner and persisted periods (`M`)
- [ ] Item 5: replay case ↔ file association and migration (`M`)
- [ ] Item 6: Captures page, timeline, slide-over (`L`)

### Deferred

- [ ] Sequence-aware realtime replay: `ReadPCAPFileRealtime` paces packets against one file's
      clock, so the visualiser's realtime and scaled modes stay single-file. Needed only when
      someone wants to watch a multi-file case play back at wall-clock speed
- [ ] Cut-point review and split-commit UI: depends on all six items; own plan once item 6 lands
- [ ] L1 health-check suite (packet loss, timestamp monotonicity, dual-return consistency):
      the mockup shows it, but it is a separate diagnostic surface from sequencing
- [ ] Rotation-anomaly detection and the spin-rate sparkline: same reason
- [ ] Git state per capture file: the mockup's git column presumes a capture repo that does
      not exist yet

### Accepted residuals (no action planned)

- [ ] Sub-10 ms seams are fed through without frame-boundary alignment. At 20 Hz a 10 ms error
      is a fifth of a revolution; the resulting azimuth stitch is within the sensor's own
      inter-packet jitter and does not warrant per-packet re-ordering.
- [ ] Sessions are derived only from packet-time adjacency, not from sensor identity. A second
      sensor writing to the same root with interleaved timestamps would merge into one session.
      Multi-sensor capture is not a current field configuration.
