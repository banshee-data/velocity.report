# VRLOG recording contract review

This review settles the vocabulary and ownership needed before VRLOG can become the durable L4
observation boundary. It is a companion to the [asynchronous tracking plan][async-plan], which owns
estimator, queue, storage, and hardware sizing. No wire format or runtime behaviour changes here.

- **Status:** Proposed contract review; names and wire fields are not implemented
- **Layers:** L1–L4 evidence, L5–L8 results, L9–L10 replay and exports
- **Canonical:** [VRLOG wire format](../../data/structures/VRLOG_FORMAT.md) describes the
  implemented v0.5 format
- **Related:** [shared VRLOG plan](lidar-vrlog-observation-format-plan.md),
  [VRLOG analysis report](../../data/structures/VRLOG_ANALYSIS_FORMAT.md),
  [web scene export](lidar-web-scene-export-plan.md),
  [pipeline profiles](lidar-pipeline-profiles-plan.md)
- **Evidence baseline:** Repository commit `9b2014b90`; source inspection, no new measurements

## Decision to carry forward

Use one processed-observation recording, VRLOG, in the capture package. Give a future recording an
explicit **evidence contract** saying which L4 observations and supporting point fields it actually
retains. Keep L5+ results and human annotations in the package catalogue, keyed to an immutable
capture identity and a separate run identity. A web scene is a derived export, not a smaller VRLOG.
PCAP remains an optional, earlier L1 source when L2–L4 must be rerun.

Do not introduce one `profile` enum for every choice. Six independent questions are currently being
called profiles in plans or code; conflating them would make a name such as `compact` silently
stand for evidence loss, compression, upload behaviour, and retention at once.

| Question                            | Owner and proposed term                                                       | Contract consequence                                                                          |
| ----------------------------------- | ----------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| How is the directory decoded?       | VRLOG **format version** and required capabilities                            | Version reader, index, frame schema, committed extents, and validation together               |
| How far did the live pipeline run?  | Existing **pipeline depth** (`l3-only`, `detect`, `full`)                     | Derived from engine selectors; it is not the recording's evidence contract                    |
| Which measurements survived?        | Proposed **evidence class** plus machine-readable field and loss declarations | Decides whether L5 replay, L4 repair, or only display is supportable                          |
| What extra internals were captured? | **Diagnostic attachments**                                                    | Optional, bounded, versioned, and never needed to decode authoritative evidence               |
| What is shown to a consumer?        | **Projection** or **export kind**                                             | UI filtering and the existing `tracks`, `clip`, `background` scene kinds do not alter capture |
| How are bytes moved and kept?       | **Transport policy** and **retention policy**                                 | Chunk size, codec, batching, destination, ACK, and deletion may vary independently            |

The term _evidence class_ is provisional. If a future API retains `recording_profile`, its values
must answer only the third question and include exact field-presence, precision, filtering, and
completeness declarations. Unknown required capabilities fail closed for inference. A familiar name
is a shorthand, never permission to infer more data than the manifest declares.

## What the current project actually has

| Producer or consumer                                                                                                                          | Implemented behaviour                                                                                                                   | Consequence                                                                                      |
| --------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| [Recorder](../../internal/lidar/l9endpoints/recorder/recorder.go)                                                                             | v0.5 directory with `header.json`, `index.bin`, length-prefixed protobuf `FrameBundle` chunks; chunks rotate at 1,000 frames or 150 MiB | A container and frame schema exist, but no recording-profile field or committed live frontier    |
| [Frame adapter](../../internal/lidar/l9endpoints/adapter.go)                                                                                  | Converts L2–L5 state to presentation bundles; skips clusters already matched to tracks                                                  | Existing cluster lists are incomplete as L4 evidence                                             |
| [Publisher](../../internal/lidar/l9endpoints/publisher.go)                                                                                    | Removes background points for split streaming, records the bundle, logs recording errors and carries on                                 | Capture cannot currently promise lossless observation storage                                    |
| [Replayer](../../internal/lidar/l9endpoints/recorder/recorder.go) and [publisher replay](../../internal/lidar/l9endpoints/publisher_vrlog.go) | Read closed index/chunks, including legacy JSON payloads, and stream recorded bundles                                                   | Old tracks are fixed snapshots; a future run selector needs observation/result composition       |
| [Swift run browser](../../tools/visualiser-macos/VelocityVisualiser/App/RunBrowserState.swift)                                                | Loads a run's VRLOG through the server and plays the resulting stream                                                                   | Preserve seek and source-mode semantics while making selected-run overlays explicit              |
| [Analysis tool](../../cmd/tools/vrlog-analyse/main.go)                                                                                        | Reads recorded tracks and writes `analysis.json` inside the VRLOG directory                                                             | Move future reports into run results; old reports remain readable as legacy sidecars             |
| [Inspection tool](../../cmd/tools/vrlog-check/main.go)                                                                                        | Scans for first background and empty frames                                                                                             | Extend to evidence completeness and damaged-tail checks; do not imply it already validates those |
| [Scene export](../../internal/scene/types.go)                                                                                                 | Produces separate `tracks`, `clip`, and `background` JSON artefacts                                                                     | These are presentation/export kinds, not VRLOG recording variants                                |

Human state has its own split: [run-track labels][run-track-labels] are edited on analysis rows,
while [replay annotations][replay-annotations] can attach to a case and optionally a run/track.
Neither is part of a VRLOG `FrameBundle`.

`FrameTypeFull`, `Foreground`, `Background`, `Delta`, and `Empty` say what a particular
`FrameBundle` contains. They are not evidence classes. `source_type` in `header.json` records live,
PCAP, or synthetic provenance; it is neither a worker placement nor a recording class. The
implemented [pipeline depth labels](lidar-pipeline-profiles-plan.md#implementation-notes) describe
which engines ran. A `detect` run is a promising L4 boundary, but does not by itself record all
clusters.

The current header and index are written at `Close()`. An open or power-interrupted directory
cannot therefore be treated as a committed stream by the existing replayer. The future design needs
append-safe committed extents and crash recovery before a local or remote worker consumes live
VRLOG. The project should not call current v0.5 recordings complete L4 evidence: matched clusters
are absent, point arrays lack per-point timing and ring, and capture losses are not a durable part
of the record.

## Evidence contracts worth testing

These are **candidates**, not existing named VRLOG profiles or final product tiers. Each row must
carry the same capture identity, frame cadence, explicit gaps, timestamps, coordinate frame,
configuration provenance, and committed-range integrity. Every source rotation needs a record or
declared gap; a quiet street must not become an invisible interval.

| Candidate                   | Required payload                                                                                                                                         | Supported later work                             | Irrecoverable omission                                        |
| --------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------ | ------------------------------------------------------------- |
| L4 summaries                | All clusters, features, uncertainty/quality, frame and crop/filter status                                                                                | L5 association, smoothing, cheap worker transfer | Geometric split/merge repair and L4 retuning                  |
| L4 with foreground evidence | Above plus retained foreground points with acquisition timing, ring, intensity, cluster linkage or reconstructible membership, and declared quantisation | L5 work and selected L4 reinterpretation         | Full L3/background reprocessing; discarded foreground returns |
| PCAP-associated package     | Either class above with separately hashed raw PCAP segments                                                                                              | Rerun L2–L4 when algorithms change               | Nothing beyond what PCAP itself failed to capture             |
| v0.5 historical recording   | Whatever its stored `FrameBundle` contains, with an inferred legacy capability report                                                                    | Playback and analysis of that historical result  | A complete original L4 stream cannot be inferred              |

The first two rows may become actual `recording_profile` values after measurement. Avoid making
`PCAP-associated` another VRLOG profile: optional L1 retention is a linked package asset with its
own size and policy. Keep background snapshots as pipeline state with declared cadence and version;
specify whether they suffice for faithful visual replay, and never claim they permit a fresh L3
pass. An empty detection frame, a skipped L4 frame, a missing sensor rotation, and an uncommitted
tail are distinct states.

The metadata needs to state precision and transformations, not just field names: frame time bounds,
point time domain and ring, coordinates and calibration, crop/region/filter decisions, subsampling
and quantisation, cluster algorithm/configuration, point-to-cluster membership or unassigned
status, packet/frame loss, and a reason for every missing observation. Record diagnostics
separately with their own schema and resource budget; debug overlays are not a substitute for the
measurements.

## Identity, authority, and compatibility

The capture identity must cover the exact committed payload bytes, not only a filename or
`header.json` and `index.bin`. The present scene exporter fingerprints just those two metadata
files, so that hash is useful historical provenance but not a sufficient content-addressed identity
for worker input. Define a manifest containing ordered segment hashes, capture range, format and
evidence contract, plus a final manifest hash. Use immutable segment identities and explicit
committed frontiers so an in-progress recording can gain segments without its earlier content
changing. Results name the input segment range and hashes they used.

Preserve v0.5 and legacy JSON readers as compatibility paths. Do not rewrite an old VRLOG to
suggest it contains missing clusters. A versioned adapter can expose its recorded track snapshots
as a fixed historical run, with completeness marked unknown or presentation-only. If a matching
PCAP exists, generate a **new** observation capture with separate provenance; do not silently
replace the old one. Once the new format is agreed, update the canonical wire spec and reader
compatibility table together.

Reports, annotations, and revised tracks belong outside the immutable observation directory in the
session catalogue or versioned export. This changes the `vrlog-analyse` sidecar convention for new
captures; migration should still discover old `analysis.json` files without treating them as source
evidence. A scene export must bind both capture identity and selected result version. For a new
observation-only VRLOG, the current scene exporter will otherwise emit no tracks. The protobuf
`LabelEvent` and `LabelSet` are transport messages, not embedded VRLOG labels. Preserve old
annotation IDs and their weaker time/case anchors; do not manufacture point anchors for old
captures or reattach a person's label by a changed track ID without review.

## Local and remote use of the same record

The capture owner writes and verifies the observation log, advances its committed frontier, and
keeps the catalogue authoritative. A local worker reads committed ranges from the same disk. A
remote worker receives the same immutable ranges by verified, resumable transfer; its receipt does
not move catalogue authority. Both use the same run ID, input range/hash, lease epoch, checkpoint,
and idempotent result-commit contract described in the [asynchronous tracking plan][worker-move].
Only the owner publishes accepted results to the UI. Switching workers fences the old lease and
replays sufficient overlap at a finalised boundary. Fully offline backpack capture is different:
after close and verification, import the package and explicitly transfer catalogue authority to one
workstation. No open SQLite WAL file is moved as a handoff mechanism.

Transport can package 10-second/64-MiB proposed segments differently for LAN and cellular, without
changing their evidence class. Keep durable stored ACK separate from processed ACK; spool locally
through outages. Compression must be declared at the container/segment layer and decoded by
versioned readers; renaming a v0.5 `.pb` chunk to `.gz` is not compatible with existing tools.
Retention decides how long local and remote copies, PCAP, and derived runs survive. It cannot make
an incomplete capture complete. The capture owner must report buffer pressure and a hard
evidence-loss state if its promised durability cannot be sustained.

## Migration sequence and review gates

1. Measure real v0.5 captures and representative quiet, busy, occluded, and damaged sessions: field
   presence, point/cluster counts, loss, codec sizes, write and read latency. Publish a capability
   report for old recordings before choosing names or default evidence class.
2. Specify the new frame/manifest schema in the canonical format document: required vs optional
   fields, gaps, point membership, time domains, segment hashes, committed extents, and version
   negotiation. State precisely which old readers can read which new records.
3. Record at L4 before presentation adaptation. Round-trip every cluster and selected point,
   compare the recorded boundary with the in-memory one, and prove power-loss recovery and
   concurrent reader behaviour. Make recorder failure a visible capture-health failure.
4. Run the existing causal tracker from the recorded boundary first. Then add independently
   versioned L5+ results, annotation projection, and a selected-run replay compositor. Update
   Swift, report, inspection, and scene-export consumers against the compatibility table.
5. Exercise identical run inputs locally, over gigabit LAN, over intermittent cellular, and after
   offline import. Verify hashes, ACK and retry idempotence, lease fencing, finality, and that
   moving a worker does not alter the capture or result identity.

The acceptance test is semantic as well as byte-level: given one declared evidence class and
committed input range, a qualified worker must either produce a traceable result or say exactly
which missing capability, gap, corruption, or version prevents it. A visually plausible track is
not evidence that the recording contract was met.

[async-plan]: lidar-cluster-observation-log-and-async-tracking-plan.md
[run-track-labels]: ../../internal/lidar/server/run_track_api.go
[replay-annotations]: ../../internal/db/migrations/000033_replay_annotations_and_eval_integrity.up.sql
[worker-move]: lidar-cluster-observation-log-and-async-tracking-plan.md#moving-the-same-analysis-between-workers
