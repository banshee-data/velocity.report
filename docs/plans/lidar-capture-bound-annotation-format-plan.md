# Capture-bound annotations: labels that outlive the run, history kept as deltas

A label made today belongs to one run's export of a capture, not to the capture. Replay the same
PCAP with other settings, or even with the same settings under a new run, and the labels no longer
apply. Each save also writes the whole document twice, so one kirk0 pack carries 42 GB of history
for a 26 MB head. This plan binds labels to the returns in the capture, keeps history as deltas,
and moves the existing kirk0 work across by an exact, proven mapping or not at all. It has to land
before any annotation is published to the
[Hugging Face dataset](../../tools/s2-archive/huggingface-dataset-layout.md).

- **Status:** Draft. The packet key and the annotation domain are decided (§8); Phase 0 is partly measured; nothing is built
- **Layers:** L1 Packets, L2 Frames, L3 Grid, L9 Endpoints (VRLOG, protobuf), L10 Clients (macOS annotation window), annotation storage, offline analysis
- **Target:** before the first annotation is published; the format and the salvage first, the pipeline locator next
- **Companion plans:** [VRLOG observation format](lidar-vrlog-observation-format-plan.md), [annotation save latency](lidar-annotation-save-latency-plan.md), [point annotation and object dataset](lidar-point-annotation-and-object-dataset-plan.md), [physical reference review](lidar-physical-reference-review-plan.md), [platform vocabulary](platform-vocabulary-and-data-model-plan.md)
- **Canonical:** [point annotation tool](../lidar/operations/point-annotation-tool.md)

## 1. Why the format has to change

### 1.1 A label belongs to one run's export

A sidecar binds to a `pack_digest`, and a mask cites `sample_id + point_index` inside that pack
([sidecar.go](../../internal/lidar/annotation/sidecar.go)). The pack digest covers `points.bin` and
`samples.json`. `samples.json` records the run's frame numbering, so the digest changes whenever
the run does, even when no point does.

The two kirk0 packs show it. `8e422582-…-20260922` and `ad8b9438-…-20260929` hold byte-identical
`points.bin`, cut from two runs of `kirk0.pcapng`. Their samples number the same frames from 2497
and from 833, so their pack digests differ and neither pack can load the other's labels. Change the
L3 settings and the point domain itself changes, because a pack holds only the run's foreground.

### 1.2 The foreground a label was made on cannot be followed into another run

Annotation is downstream of foreground separation: an operator labels what an extraction kept, and
that stays the design. What fails today is that the labelled points are known only by their place
in one pack, so another run's foreground cannot be compared with them. A run with other L3
settings cannot be scored against the labels at all, although the question that matters, which
labelled returns its L3 kept, has an exact answer once labels cite returns. kirk0 was labelled on a
deliberately generous foreground (1,833,561 points against 985,224 at the defaults). The
[annotation-scored tuning](../lidar/operations/annotation-scored-tuning.md) found that the defaults
lose pedestrians in L3, and return-keyed labels turn that loss into a direct count.

### 1.3 Nothing ties a pack to its PCAP

The manifest records `pcap_basename: kirk0.pcapng` and the digests of a VRLOG that will not be
published. No PCAP digest is stored, so nobody holding the published capture can check that a pack
came from it.

### 1.4 Every save keeps the whole document

`SaveSidecar` copies the current `annotations.json` to `annotation-revisions/N.json`, then writes the
new head ([README](../../internal/lidar/annotation/README.md#files-and-failure-behaviour)). A
typical save changes one mask, while the head is a pretty-printed document with one integer per line.

## 2. Measurements

Taken from the packs and captures on the lidar volume. Phase 0 completes the rest.

| Observation                                                                                                                                                                                   | Source                                                               |
| --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------- |
| `ad8b9438` head: revision 2,232; 30 objects and 4,102 masks, all `reviewed`; all `partial` and `present`; 1,756,903 member indices; 832 samples                                               | `annotations.json`                                                   |
| Of those masks, 2,574 were drawn by hand, 1,422 accepted from `cluster_chain` and 106 from `footprint_carry`                                                                                  | mask `provenance.algorithm`                                          |
| History: 2,231 snapshots, 42 GB. The head is 25.9 MB pretty-printed and 9.4 MB minified                                                                                                       | `annotation-revisions/`                                              |
| Twelve sampled consecutive saves each changed or added one mask, by 1 to 159 indices, and each wrote a 0.3 to 25.9 MB snapshot                                                                | revision pairs 3, 51, 201 … 2231                                     |
| `8e422582-…-20260922` (the sweep's ground truth): revision 402; 67 objects (6 reviewed); 4,661 masks (442 reviewed); same `points.bin`                                                        | its `annotations.json`, `manifest.json`                              |
| `aadfe835` (4 packs) labels `s2_sf_3_20260902132037_00002.pcap`, a source roll of the published columbus-broadway capture. The `-175735` pack's head: 13 objects, 1,054 masks, all `proposed` | its `annotations.json`                                               |
| kirk0 packets are 1,266 bytes (UDP sequence on), in Last-return mode (0x38)                                                                                                                   | first 59,984 LiDAR packets                                           |
| The sensor clock reads 2017-09-06 (not synchronised) but advances: sensor time is unique for 59,984 of 59,984 packets                                                                         | packet tail                                                          |
| Record (capture) time repeats: 59,980 unique of 59,984                                                                                                                                        | PCAP record headers                                                  |
| SHA-256 of the UDP payload is unique for 59,984 of 59,984 packets                                                                                                                             | payload bytes                                                        |
| Clipping preserves packets: 10 of 10 sampled packets of the columbus-broadway roll appear in the published PCAPNG with identical payload, sensor time, UDP sequence and record time           | roll part `00002` against `20260902T1320_…_columbus-broadway.pcapng` |
| No commit has touched `l1packets/parse` or `l2frames` since `0b540677`, the build that made `ad8b9438`                                                                                        | `git log`                                                            |

## 3. Decisions (proposed)

### D1. Labels cite returns in the capture, not points in a pack

A return is identified by `(packet key, block, channel)`. The **packet key** is the first 64 bits
of SHA-256 over the UDP payload: the bytes the sensor sent, which never change once captured,
without Ethernet, IP or UDP headers or the capture record. It survives clipping, merging,
PCAP-to-PCAPNG conversion and re-stamping, because none of those touch the payload. Decided (§8).

| Candidate                | Why not                                                                 |
| ------------------------ | ----------------------------------------------------------------------- |
| Pack `sample + index`    | Run-bound (§1.1)                                                        |
| Record time              | Repeats: 4 in 59,984 kirk0 packets                                      |
| Packet ordinal in a file | Changes when rolls are clipped or merged into a published capture       |
| UDP sequence             | Optional in the sensor config, resets on power-up, 32-bit wrap          |
| Sensor time              | Unsynchronised on these captures (2017 on kirk0) and resets on power-up |
| Coordinates              | Forbidden: coincident returns are distinct points, and decoders change  |

Within a label set, the packets it covers are listed once in capture order (the **packet table**).
A return then has a dense local **return number**:
`packet_index × blocks × channels + block × channels + (channel − 1)`, which is 400 per packet on a
Pandar40P. The numbering belongs to the label set's own table, not to any run.

### D2. A label set binds to packets, and to files only by verification

`bind` reads a PCAP, computes packet keys and records the result in `labels.json` as an
`applies_to` entry: the file's SHA-256, packets found, missing and duplicated. A label set applies
to any file that contains its packets. A clipped file that holds only some of them gives coverage
for the windows it holds, and the rest are unavailable rather than empty. A key that appears twice
in one file refuses the bind.

### D3. Windows replace samples

A **window** is the half-open range of return numbers the operator saw as one frame. Completeness
and visibility are claimed per window. Another run's frames need not align with it, because scoring
compares returns, not frames.

### D4. Every mask is made on a named foreground

Annotation is downstream of foreground separation. Every mask records as its `domain` the
foreground extraction it was made on: run ID, config hash, params hash and build SHA. There is no
all-returns domain, and no label claims to have seen a return its extraction discarded. Decided
(§8).

Scoring another run of the same capture compares returns:

| Return                    | Labelling extraction kept it | Scored run kept it | Counts as                                                       |
| ------------------------- | ---------------------------- | ------------------ | --------------------------------------------------------------- |
| Member of a mask          | Yes                          | Yes                | Membership, scored as today                                     |
| Member of a mask          | Yes                          | No                 | Lost by the scored run's L3                                     |
| In the domain, in no mask | Yes                          | Yes                | Treated as the per-frame evaluator treats unlabelled foreground |
| Outside the domain        | No                           | Yes                | Unlabelled and ignored, never a negative                        |

The domain bounds what a label set can measure, so label on a generous foreground, as kirk0 was.
Salvaged kirk0 masks carry that run's foreground as their domain.

### D5. History is a journal of deltas; the head is a checkpoint

Each save appends one line to `journal.ndjson`. The line holds the parent revision and parent state
digest, author, session, operation and UTC time, the per-mask additions and removals as return
ranges, and the field changes, then the resulting state digest. A pinned revision is
`(label_set_id, revision, state_digest)`, rebuilt by replaying the journal and checked against the
digest. Restoring an old revision appends a delta that recreates it.

The lock and the optimistic concurrency check stay: a save names the base revision and base state
digest. The macOS client appends and syncs one line per edit (the save-latency plan's recovery
journal becomes the real one) and rewrites the head only in batches (that plan's part A). No
snapshot is written per save.

### D6. One mask per line, in one canonical encoding

`masks.ndjson` holds one mask per line, sorted by window then object, with membership as sorted
`[start, length]` ranges of return numbers. An edit to one mask changes one line, so Git and Xet
store it as a small change rather than a new 26 MB file.

### D7. Packs become views

A version 2 pack is a version 1 pack plus `returns.bin` (one return number per point, parallel to
`points.bin`) and the digest of the packet table it was built against. The window displays a view
and translates selections into returns. Edits are deltas, so returns a view does not show are kept
rather than dropped. Version 1 packs and sidecars stay readable, as the VRLOG plan's
[§7](lidar-vrlog-observation-format-plan.md#71-one-selection-manifest-the-same-point-codec)
requires.

### D8. Legacy labels move only by an exact, proven mapping

The [annotation plan](lidar-point-annotation-and-object-dataset-plan.md#31-a-frozen-excerpt-pack)
forbids rebuilding point identity by approximate coordinate matching, and the VRLOG plan allows a
label transfer only "after proving an exact, digest-bound mapping". The migration therefore:

1. Decodes the PCAP with the same L1/L2 decoder, in the pack's coordinate frame.
2. Matches each pack point to a return with bit-identical float32 `x, y, z` among the returns of
   that sample's frame.
3. Accepts one-to-one matches only. A coincident pair becomes uncertain on that mask.
4. Refuses the label set if any member has no match, reports it, and leaves the version 1 files as
   they are.
5. Records a mapping digest over the pack digest, the PCAP SHA-256, the decoder build and the
   mapping table.

## 4. Format version 2

```text
<label-set>/
  labels.json               identity, sensor layout, bindings, domains, head revision and digest
  packets.bin               packet keys, uint64 little-endian, capture order
  windows.json              per window: return range, sensor and record time bounds
  objects.json              objects (version 1 fields, unchanged)
  masks.ndjson              one mask per line (D6)
  physical-references.json  version 1 record, keyframes rebound from sample to window
  journal.ndjson            deltas (D5); local, optional when published
  .lock                     local only, never published
```

| `labels.json` field | Meaning                                                                                      |
| ------------------- | -------------------------------------------------------------------------------------------- |
| `schema_version`    | 2                                                                                            |
| `label_set_id`      | Random UUID for this annotation effort; stable across revisions and files                    |
| `sensor`            | Model, return mode, `blocks` and `channels`: the return-number layout                        |
| `packet_key`        | `sha256-64:udp-payload`, and the packet table's digest and count                             |
| `applies_to`        | Verified files: SHA-256, bytes, name, found, missing and duplicated packets, verifying build |
| `domains`           | The foreground extractions masks were made on (D4)                                           |
| `head`              | Revision, state digest, journal length and digest of its last line                           |
| `migrated_from`     | For salvaged sets: pack digest, sidecar revision, mapping digest                             |

Mask fields keep their version 1 meaning (`object_id`, `completeness`, `visibility`, `status`,
`pose`, `provenance`), with `window_id` in place of `sample_id`, return ranges in place of indices,
and `domain` added. The state digest is SHA-256 over the canonical encoding of `objects.json`,
`windows.json` and `masks.ndjson`.

For kirk0 the packet table is about 150,000 packets (83 s at about 1,800 packets per second), or
1.2 MB. Mask and journal sizes are measured in Phase 3. A typical save is one mask by under 200
indices, so the whole journal should fall from 42 GB to tens of megabytes at most.

## 5. What we can salvage

| Version 1 source                                                            | Version 2 result                                                                                                 | Condition                                                                                                               |
| --------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| `ad8b9438` revision 2,232: 30 objects, 4,102 masks, all reviewed            | Label set `kirk0-reference`, reviewed, its domain the loosened run's foreground                                  | All 1,756,903 members map one to one (D8)                                                                               |
| Its 2,231 revision snapshots (42 GB)                                        | `journal.ndjson`, keeping each save's author, session, operation and time                                        | Replaying the journal reproduces every revision's state digest. Archiving the snapshots is then the operator's decision |
| Its physical references and their revisions                                 | Same records, keyframes rebound from sample to window; geometry unchanged                                        | Their links still validate                                                                                              |
| Its feature proposals, proposal layer and dismissed proposals               | Carried where they cite indices that map; otherwise regenerated, since proposals are reproducible                | Inventoried in Phase 0                                                                                                  |
| `8e422582` revision 402, the sweep's ground truth                           | Separate label set `kirk0-sweep-gt` through the same mapping, plus an agreement report against `kirk0-reference` | Sample-to-frame alignment re-derived, because its frame numbering differs                                               |
| `aadfe835-…-20260921-175735` revision 30: 13 objects, 1,054 masks, proposed | Proposed label set on the columbus-broadway roll, then bound to the published PCAPNG                             | The first proof that one label set applies to two files                                                                 |
| `splits/kirk0-ad8b9438-tuning-r1.json`                                      | Re-frozen against `kirk0-reference` revision and state digest                                                    | Same object partition                                                                                                   |
| kirk0-tuning `ground-truth.json` and its 11,861 scored configs              | Ground truth regenerated from `kirk0-sweep-gt`                                                                   | A sample of configs scores identically: same memberships in the same domain                                             |

The domain costs nothing in the salvage. The kirk0 labels were made on a foreground, which is what
every version 2 label is, so they are published as they stand, `partial` masks included, with
their domain stated. They are not re-reviewed against returns the extraction discarded.

## 6. Phases

| Phase | Work                                                                                                                                                                                                                                                                                                                                                                                         | Gate                                                                                                                |
| ----- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------- |
| P0    | Finish measuring: packet-key uniqueness over every capture under `LIDAR_PCAP_DIR`; a dry-run exact match of the kirk0 pack (matched, coincident, unmatched); an inventory of every record that cites pack indices in Go, Swift, scripts and frozen splits                                                                                                                                    | No duplicate key within any capture, and every reviewed kirk0 member matches one to one, or D1 and D8 are revisited |
| P1    | Specification and Go library: codec, validation, state digest, journal store (lock, append, replay, restore), `bind` over a PCAP. Swift parity fixtures in the style of `swift_fixture_test.go`. Package README and operations guide                                                                                                                                                         | Round-trip, replay and restore tests; byte-identical canonical encoding from Go and the fixtures                    |
| P2    | Return locator through the pipeline: packet key computed in L1, carried on L2 points and through L3 foreground, written to `FrameBundle` and the VRLOG. Propose the packet key as the "packet/block/channel/return locator" of the VRLOG plan's [§3.1](lidar-vrlog-observation-format-plan.md#31-session-frame-and-observation-identities). Pack export writes `returns.bin` (pack schema 2) | Perf gate within its committed bounds; every exported point carries a return number                                 |
| P3    | Migration tool and salvage of §5, with a report in `docs/` recording match rates, sizes and the mapping digests                                                                                                                                                                                                                                                                              | The conditions in §5                                                                                                |
| P4    | macOS window reads and writes label sets through views: journal append per edit, batched head, delta edits that keep members the view does not show. Takes in the save-latency plan's threading (part A) and replaces its recovery journal and snapshot slimming                                                                                                                             | Undo, restore and conflict behaviour match the version 1 store; no per-save snapshot                                |
| P5    | Server and scoring: label-set endpoints; the per-frame evaluator, `lidar-ground-truth-eval`, split freezing and the held-out guard read version 2 and bind by capture SHA-256                                                                                                                                                                                                                | kirk0 scores reproduce from version 2                                                                               |
| P6    | Publication: `labels/<capture-stem>/<label-set>/` in the dataset, `.gitattributes` entries (`labels/** -text`, LFS for anything over 10 MB), the manifest join from capture to label set, and the README                                                                                                                                                                                     | `bind` succeeds against the published PCAPNG for every published label set                                          |

P3 does not wait for P2, because the migration decodes the PCAP itself. New labelling in version 2
needs P2 and P4. P6 needs P3.

## 7. Risks

| Risk                                                          | Handling                                                                                                                                                      |
| ------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| A decoder change moves coordinates                            | Packet keys are raw bytes and do not move; views are rebuilt. Only the version 1 migration depends on the decoder, and it records the build it proved against |
| Another sensor model has a different block and channel layout | The layout is declared in `labels.json`; a second model adds its own                                                                                          |
| Dual-return captures                                          | The block identifies the return; coincident returns stay separate returns                                                                                     |
| Long captures make large packet tables                        | 20 minutes is about 2.2 million packets, or 17 MB. A label set lists only its windows' packets                                                                |
| The protobuf change touches the Swift stubs and the VRLOG     | Lands with the VRLOG plan's point locator, not separately                                                                                                     |
| The same packets appear in two published files                | Intended: one label set binds to both. Duplicates within one file refuse the bind                                                                             |

## 8. Decisions and open questions

Decided on October 12, 2026:

1. **Packet key:** a hash of the packet's bytes, which never change once captured (D1). Not UDP
   sequence plus sensor time.
2. **Domain:** foreground only. Annotation is downstream of foreground separation (D4). The
   salvaged kirk0 labels are published as foreground-domain labels, not re-reviewed against all
   returns.

Open:

1. Publish journals with label sets, or heads only, with Git history as the published history?
   Proposed: heads only; journals stay local.
2. After P2, does the exporter still write version 1 packs for anything, or only version 2 views?

## 9. Not in scope

- Dataset release versioning, multi-dataset and multi-database import.
- Changes to object semantics, review statuses or the proposal workflow.
- Object identity across captures.
