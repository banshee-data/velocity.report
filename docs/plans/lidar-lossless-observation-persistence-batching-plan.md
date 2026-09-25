# Lossless LiDAR observation persistence batching

- **Status:** Offline frame-batch writer implemented on `dd/docs/state-est`; full corpus oracle and G-PER-1 acceptance remain open
- **Canonical:** [Tracking maths](../../data/maths/tracking-maths.md)
- **Layers:** L4 Perception, L5 Tracks, SQLite offline replay storage
- **Target:** Sprint 0.5.2.0
- **Backlog:** [State-estimation evidence sprint](../BACKLOG.md#sprint-0520-trustworthy-replay-evidence)
- **Related:** [State estimation](lidar-state-estimation-plan.md), [test corpus](lidar-test-corpus-plan.md), [performance measurement harness](lidar-performance-measurement-harness-plan.md), [shared VRLOG storage](lidar-vrlog-observation-format-plan.md)

The sections below preserve the original SQLite performance and equivalence design. The branch now
has an offline `FrameEvidenceStore` that commits observations with linked online estimates and
residuals per frame. It remains a reduced-profile replay/oracle path: its configured point cap and
combined L4-plus-L5 transaction do not meet the independent, complete observation-capture
contract. Sprint 0.5.2.2 reuses its source manifests, tests and semantic comparisons while making
the shared VRLOG stream the new capture authority. Do not extend this writer as a competing live
recording path.

## Objective

Make full-fidelity Phase 1 observation replays practical without changing what
they record. The current first pass writes every immutable L4 observation and
every linked online state record one row at a time. It is correct, but it turns
a replay into a long conversation with SQLite, one sentence per commit.

The replacement batches writes at a frame boundary. It must retain the same
evidence, precision, provenance, ordering contract, and fail-closed behaviour.
It is a persistence-performance change, not a new measurement model and not a
licence to discard points.

## Why this work exists

The Phase 0/1 corpus tool deliberately enables the observation database only
for its first pass. That pass does all of the following:

1. freezes every L4 cluster before L5 association, including clusters later
   rejected by the tracker;
2. retains up to the configured sample-point cap for each cluster;
3. serialises each immutable observation into `lidar_observations`, while
   maintaining its source-time and calibration-time indexes; and
4. writes a derived estimate and residual for every accepted association.

The repeat pass deliberately disables that database. It can therefore prove
baseline determinism quickly, but it cannot prove that a full evidence corpus
is affordable to build. A long first pass is useful evidence about the current
path. It is not evidence that the data are more precise because each row had a
private transaction.

## Source-corpus identity

Every evidence run must begin by freezing a SHA-256 manifest of the PCAPs it
will read. The manifest lives on the LiDAR volume with the corpus artefacts,
not in Git: it names each capture in declared replay order, its path relative
to the configured LiDAR root, byte size, SHA-256, corpus case, and source ID.
The run report records the manifest's own SHA-256.

For the completed Phase 0/1 corpus, the manifest belongs in a sibling
`/Volumes/lidar/lidar/manifests/` location, rather than inside the immutable
`state-estimation-phase01-rebased-20260913` result directory. Future runs
write their source manifest before opening the output database. A changed,
missing, reordered, or substituted PCAP is a different corpus and must fail
the comparison rather than quietly inherit a previous result.

Git keeps the declaration, the manifest checksum, and the result report. The
manifest and the PCAPs remain together on the LiDAR volume and travel with the
external evidence corpus or its Hugging Face copy.

## Non-negotiable data contract

The batched path must be logically indistinguishable from the current path.

| Property         | Required behaviour                                                                                                                                          |
| ---------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Cluster coverage | Persist every L4 cluster before profile gating and association, including rejected and untracked clusters.                                                  |
| Retained points  | Preserve the configured cap and the exact retained-point sequence, coordinates, intensity, timestamps, and all other fields already frozen into the record. |
| Provenance       | Preserve source ID, calibration ID, sensor ID, frame ID, capture times, cluster ID, schema version, extractor identity, and parameter hash.                 |
| Derived records  | Persist each eligible estimate and residual with the same observation linkage, model identity, covariance, measurement source, and disposition.             |
| Immutability     | A duplicate observation ID remains an error. Batching must not turn a duplicate into an update, skip, or partial success.                                   |
| Failure          | A failed frame batch commits none of that frame's observations, estimates, or residuals. The replay fails and reports the cause.                            |
| Precision        | Do not round, quantise, downsample, recompute, or compress values as part of this change. Storage encoding remains the existing canonical JSON record.      |

`inserted_at_ns` is operational bookkeeping, not sensor evidence. Equivalence
checks may ignore that field, but no measurement or provenance field may be
ignored.

## Design

### Frame-owned write batch

Build one immutable write batch after L4 has produced its clusters and L5 has
finished the frame. The batch contains the frozen L4 observations plus any
derived L5 estimate/residual pairs that refer to those observations. The batch
does not cross a frame boundary: a frame is the smallest useful unit for both
capture-time ordering and crash recovery.

Use one SQLite transaction for the batch and reuse prepared insert statements
within it. The transaction commits only after every observation and linked
derived row has been accepted. This removes repeated transaction setup and
durability waits while keeping memory bounded by one frame.

The tracker must still see the same in-memory clusters in the same order. The
batch writer sits beside persistence, not in the measurement, association, or
subsampling paths. Changing those paths would make this a precision experiment,
which it is not.

### Error and restart behaviour

A transaction error rolls back the whole frame. The replay reports the source
case, frame time, and failing record identity, then stops. It must not retry a
partial frame by silently changing IDs or timestamps.

After a stopped run, reopening the same immutable source must keep the current
duplicate-ID rule. Recovery is an explicit operator decision: use a new output
database or remove the incomplete offline artefact after inspection. Automatic
resume belongs to a later design because it needs a durable, reviewed checkpoint
contract.

### What stays out of scope

This work does not change the sample-point cap, choose a smaller payload,
introduce lossy compression, defer evidence to a background queue, or relax the
write-once contract. Those may be separately measured later, but none should
hide inside a performance patch.

It also does not turn a deterministic tracking baseline into a physical
accuracy result. The Phase 0 and Phase 1 gates still need their residual,
reference, Pi, surface, and replay evidence.

## Delivery sequence

| Step | Delivery                                                                                                                                                                                                                                                              | Exit                                                                                                                                                                                               |
| ---- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1    | Freeze the source-PCAP SHA-256 manifest on `/Volumes/lidar/lidar` and measure the current full-fidelity corpus run: wall time, transaction count, rows by table, database size, peak memory, and per-layer time.                                                      | Published workload identity names the exact ordered inputs, manifest digest, tuning, and machine class.                                                                                            |
| 2    | Add the lossless internal frame-batch writer: prepared inserts and one transaction per frame. In the same change, add the external-corpus golden test for the PCAP set with recorded frame-by-frame evidence. Keep the existing single-row writer only as the oracle. | Unit tests cover success, duplicate IDs, invalid records, rollback, and database errors. The corpus test verifies its source manifest before replay and passes against the frozen semantic oracle. |
| 3    | Compare legacy and batched recordings using the frozen corpus oracle, first for a representative capture and then all 16 Phase 0/1 captures.                                                                                                                          | Per-frame and whole-table canonical checksums, counts, ordering, payloads, and links match; tracking baselines remain byte-identical.                                                              |
| 4    | Replay the full Phase 0/1 corpus with full evidence retention using batching.                                                                                                                                                                                         | Measured wall time and storage reported; no data-loss, provenance, baseline, or memory regression.                                                                                                 |
| 5    | Make batching the only production replay path once the evidence review passes.                                                                                                                                                                                        | G-PER-1 inputs are available without an impractical offline collection run.                                                                                                                        |

### External-corpus golden test

The Phase 0/1 PCAPs and their existing frame-by-frame database records are an
external integration fixture, not a multi-gigabyte Git fixture. The test is
explicitly opt-in on a host with the LiDAR volume mounted; normal unit tests
remain self-contained. It must fail closed when the source manifest is absent
or any PCAP path, byte size, SHA-256, declaration order, source ID, replay
configuration, or expected corpus case differs.

The oracle is a compact semantic-checksum manifest derived from the legacy
recording, plus the retained external SQLite database for audit. It records
the following for each capture and frame, in canonical capture and frame-time
order:

- row counts for `lidar_observations`, `lidar_track_estimates`, and
  `lidar_track_residuals`;
- an ordered row checksum for every table, using stable primary and temporal
  keys before the canonical record payload;
- a per-frame checksum and count for observations, estimates, and residuals;
- exact hashes of each canonical observation payload, including the ordered
  retained-point sequence, and each estimate/residual payload; and
- semantic invariants: unique immutable IDs; one valid observation link per
  estimate; one valid estimate and observation link per residual; matching
  source, calibration, and frame identities across every link; and no missing
  or duplicate derived record.

The canonical export ignores only `inserted_at_ns`. SQLite page layout, WAL
state, and file checksum are deliberately not comparison targets because a
different transaction strategy legitimately changes them. Row order remains a
first-class semantic check: exports are explicitly sorted by source, frame
time, record type, and stable record ID, so a batching change cannot hide a
reorder behind an unordered SQL query. The test also compares the existing
first/repeat tracking-baseline SHA-256 values.

## Acceptance and performance gates

The benchmark compares identical PCAP files, warm-up, tuning, measurement mode,
sample-point cap, source/calibration identities, and machine class. A quicker
run produced from less evidence is a different experiment and fails this one.

Promotion requires all of the following:

- the batched and reference recordings have identical logical evidence after
  excluding only `inserted_at_ns`;
- the source-PCAP SHA manifest, frame-by-frame semantic oracle, table-wide
  checksums, row counts, and explicit ordering checks all pass;
- both produce byte-identical tracking baselines for each repeatable case;
- every retained-point payload and estimate/residual linkage is present once;
- failure injection proves that a failed batch leaves no partial frame; and
- the report shows a material reduction in transaction count and wall time on
  the full corpus, with disk and peak-memory figures beside it.

There is intentionally no invented percentage target. The first full-fidelity
corpus run establishes the baseline. The measured reduction must be large
enough to make Phase 1 collection operationally usable, not merely flatter a
benchmark chart.

## Risks and decisions

| Risk                                                      | Control                                                                                                 |
| --------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| A bulk API accidentally drops rejected clusters           | Compare source-time ordered observation IDs and payload hashes, not only accepted track records.        |
| A transaction groups unrelated frames                     | Keep one frame per batch and make frame time part of failure reports.                                   |
| Faster storage obscures an upstream perception regression | Require the same tracking baseline and separate per-layer timing from persistence timing.               |
| Database size remains high                                | Report it honestly. Full evidence has a real storage cost; batching reduces mechanics, not information. |
| Later compression is smuggled into this work              | Treat encoding or retention changes as a separate proposal with its own fidelity comparison.            |

## Sprint relationship

This is the first Phase 1 item in Sprint 0.5.2.0 because every later
state-estimation gate relies on evidence that can be collected, reopened, and
replayed without changing its meaning. It unblocks practical corpus collection;
it does not itself close G-PER-1, G-GEO-1, G-UNC-1, G-EST-1, or G-SMO-1.
