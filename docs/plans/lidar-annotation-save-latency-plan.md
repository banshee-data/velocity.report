# Annotation saves: batched, off the main thread, and cheaper to keep

Saving one frame in the Annotation window freezes the app for seconds, and every save keeps a
whole copy of the document for good. This plan batches edits into fewer saves, moves the save off
the main thread, and makes the copies that remain smaller.

- **Status:** Draft. The prune tool for existing packs is delivered; everything else is planned
- **Layers:** L10 Clients (the macOS annotation window), annotation storage (Swift and Go writers)
- **Target:** v0.5.2.x, because labelling hours are the constraint on every reviewed-data gate
- **Companion plans:** [point annotation and object dataset](lidar-point-annotation-and-object-dataset-plan.md), [physical reference review](lidar-physical-reference-review-plan.md)
- **Canonical:** [point annotation tool](../lidar/operations/point-annotation-tool.md)

## Motivation

One labelled kirk0 pack (23 objects, 3,683 masks, 83 s) took 1,968 saves over 29 hours. At the end
the window stopped for several seconds on every frame-to-frame save, a beach ball each time, and
the pack's history had grown to 37.9 GB on the USB volume. Both come from one design: a save
rewrites the whole document, and keeps the whole previous one. Labelling is the constraint on every
gate that waits for reviewed data, so seconds per frame are hours per pack.

## Current state

Measured from the summary of the labelled kirk0 pack (run `ad8b9438`) and read from
the code.

| Fact                                                                                                                   | Where                                                                 |
| ---------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------- |
| `AnnotationSession` is `@MainActor`, and `save()` calls `SidecarStore.save` synchronously                              | `AnnotationSession.swift` `save()`                                    |
| A save reads the head from disk and hashes it, encodes the whole document, writes a full archive copy, writes the head | `AnnotationSidecar.swift` `save`, `archive`, `atomicReplace`          |
| Each write is temp file, sync, rename, directory sync; the archive and the head are two of them, 25 MB each            | `atomicReplace`                                                       |
| The encoder is `JSONEncoder` with `.prettyPrinted, .sortedKeys`: one point index to a line, about 17 bytes each        | `SidecarStore.encoder`; the Go writer matches it with `MarshalIndent` |
| The head grew from 0.38 MB (52 masks) to 14.9 MB (1,738) within 31 minutes of accepting proposals, then to 25.5 MB     | revision sizes in the summary                                         |
| 1,968 revisions, 37.9 GB: the sum of revision sizes, so history is saves times document size                           | `annotation-revisions/`                                               |
| 77 % of the bytes are the last 1,200 saves, when the document was already 21 to 25 MB                                  | estimated from the sampled revisions                                  |
| Nothing in the app reads history: `retainedRevisions`, `loadRevision` and `restore` have no caller outside the store   | `grep` over `tools/visualiser-macos`                                  |
| A frozen split pins one revision by the digest of its exact bytes; `.annotations.lock` guards the writer               | `split_frozen.go`; `lockAnnotations`                                  |

By construction, then, each save is two 25 MB writes with two fsyncs, a 25 MB read and hash, and
the encoding of about 1.5 million integers, all on the main thread and on a USB volume. Which of
those dominates is a guess until it is measured (Phase 0).

## Design

Four layers, each shippable alone, in this order. The first two answer the beach ball; the last two
answer the 38 GB.

### A. Batch the edits and write behind

The window keeps the document in memory as the truth for the screen. Saving stops being a step the
operator waits on:

- **Triggers.** A batch is written after N unsaved edits or M seconds since the first of them,
  whichever comes first. Suggested defaults 8 edits and 10 seconds, both settings.
- **Flush points.** Anything that makes an unsaved edit visible to another reader or lose it flushes
  first: **S** (flush now, without waiting), **X** (queue the save, then go to the next frame at
  once), an object review or accepted proposal (they change what counts as truth), closing the
  window, opening another pack, **Generate from Run**, **Freeze Split**, export, and quit. Only the
  last few wait, and only when something is pending, with a short "Saving" sheet. `QuitGuard`
  already stands at quit.
- **Latest wins.** Every snapshot is a whole document, so while a write runs only the newest
  pending snapshot matters: a burst of edits is one revision. That alone cuts the revision count
  by about the batch size.
- **State, always visible.** Saved, saving, "N unsaved edits", or "Not saved: reason", beside the
  frame strip. A failure never leaves the operator guessing.

### Threading

Yes, and it is the point. `SidecarStore.save` is Foundation file I/O and encoding; it touches no UI
and no main-actor state. Move it onto a writer that owns the store and the concurrency token (the
loaded digest and revision): an `actor SidecarWriter`, or a serial queue at utility QoS, one per
pack folder.

- **Hand-off.** The main actor builds the next `Sidecar` value (copy on write: the arrays are
  shared and only the changed mask is copied, so it is cheap), tags it with an edit sequence
  number, and enqueues it. The writer encodes, archives, writes the head, and returns the new
  token or an error to the main actor.
- **What stays on main.** Mutating `sidecar`, `document` and `dirtySamples`, and
  `refreshFrameProgress`. When a write returns, only the samples whose edits were in that snapshot
  are marked clean; an edit made after the snapshot stays dirty.
- **Hazards to design for.** The token has one owner, so there is no race on it; restore, reload and
  propagation call the store from the main actor today and must go through the writer or flush it
  first; `Sidecar` must be `Sendable` for strict concurrency (it is a struct of value types); two
  writers in one process would contend on the `flock` of separate descriptors, so there is exactly
  one per pack.
- **Failure.** Busy retries with backoff and then says so. A conflict stops further writes and
  keeps the edits in memory for the reconcile flow that exists. Unwritable (a full or missing
  volume) keeps trying at the next trigger and blocks quit with a warning.

### B. A private recovery journal

Batching trades durability: a crash loses up to N edits or M seconds. Close that without making
saves expensive. Each edit appends one JSON line to `annotation-journal/<session>.ndjson` and syncs
that tiny file. On open, a journal newer than the head offers to recover its edits; once they are in
a saved revision the journal is removed.

The journal is private to the editor. The Go commands, the evaluator and a frozen split read only
the published head and revisions, and those are still written only at batch boundaries, so no other
reader sees an unsaved edit, which is how it is today, and no digest or pin changes.

### C. Cheaper saves with the format unchanged

Each is small, and independent of A and B:

1. **Link the archive instead of copying it.** Before replacing the head, hard-link it to
   `annotation-revisions/N.json`, then write the new head and rename it over the old: the old inode
   lives on as the archive. One 25 MB write and fsync disappear. Fall back to a copy where the
   volume has no hard links (exFAT and FAT have none; check the pack's volume with `diskutil
info`).
2. **Do not re-read the head to check it.** Under the lock the store reads 25 MB and hashes it to
   prove nobody else saved. When the head's inode, size and modification time are those the store
   last wrote, skip the read; any difference falls back to the full digest.
3. **Encode compactly.** Drop the indentation. Nobody reads these files by eye, and the index
   arrays are the document. About a third of the size, and both decoders are untouched. Revisions
   written from then on have different bytes; a frozen split pins the bytes of the revision it
   froze, which are already on disk.

### D. Retention

- **Existing packs.** `lidar-annotation-prune` ([PR 658](https://github.com/banshee-data/velocity.report/pull/658))
  thins history behind a verified backup, keeping the newest N, the oldest, one per interval and
  every revision a split pins.
- **New saves.** Thin at archive time by the same rule, so history stops growing without a person
  running a tool. With batching, hard links and compact encoding the same 1,968 saves would be
  about 250 revisions of 8 MB: under 2 GB before any pruning.
- **If that is not enough.** Compress archives (`.json.gz`, readers try both), about six to ten
  times; or store point indices as runs or deltas (a v3 format), which makes a save cost what
  changed instead of the whole document. That is a protocol change for both stores and the digests
  frozen splits rely on, so it waits for evidence that A to D fall short.

## Phase 0: measure before choosing

Add per-phase timings to `SidecarStore.save` (read, digest, encode, archive, head, directory sync)
and `os_signpost` intervals around `AnnotationSession.save`, and record one session on the user's
volume. The plan assumes encoding and the two fsynced writes dominate. If the read and hash do, C2
moves first; if the main-thread work after the write (`refreshFrameProgress` over 3,700 masks)
shows, it joins A.

## Coordination

The functions this plan changes are `AnnotationSession.save` and `SidecarStore`, which the
physical-reference editor branch (`dd/lidar/physical-pose-ui-929`) also changes, along with
`QuitGuard`. Neither is edited from this plan. A to C land after that branch merges, or on it by
its author. The Go writer (`sidecar_store.go`) mirrors the Swift one so a snapshot round-trips
through either with the same shape; C1 to C3 reach it after the Swift side proves them. The prune
tool is new files only.

## Scope

### Item 1: measure

**Summary:** Per-phase timings and signposts around one real labelling session.

**Milestone:** v0.5.2.x, before anything else here.

### Item 2: write-behind and the writer actor

**Summary:** Layer A and the threading above, with the state indicator and the flush points.

**Steps:**

1. The writer actor and the hand-off, behind a setting that defaults to the current behaviour.
2. N edits or M seconds batching, latest wins.
3. Flush points, the state indicator, quit and close handling.
4. Failure paths: busy, conflict, unwritable.

**Milestone:** v0.5.2.x.

### Item 3: journal, then cheaper saves

**Summary:** Layers B and C.

**Milestone:** v0.5.2.x; C1 to C3 first if Phase 0 says the writes dominate.

### Item 4: automatic retention

**Summary:** Layer D for new saves.

**Milestone:** v0.5.3.

## Acceptance

- The main thread spends under 16 ms per edit in the save path (one frame), measured on the
  kirk0 pack on the USB volume.
- A burst of ten edits writes one revision; **S** and **X** do not wait for the disk.
- After `kill -9` mid-labelling, opening the pack recovers every edit but the one being written.
- A frozen split's `verify` passes after a batched save, and `velocity lidar` reads the head after
  every flush.
- The same labelling session leaves under a tenth of the history it leaves today.

## Risks

- **Operator trust.** A save that does not block has to say what is saved. The indicator is not
  decoration: it is the contract.
- **Two writers.** The window and a Go command can both write; the lock and digest check stay as
  they are, and a conflict is surfaced, never resolved silently.
- **Format drift between the Swift and Go stores.** Land each change in both, with the existing
  round-trip fixtures, or the shapes diverge.
- **Hard links across volumes and formats.** Detect, fall back to a copy, and say once.

## Checklist

- [x] `lidar-annotation-prune` for existing packs ([PR 658](https://github.com/banshee-data/velocity.report/pull/658))
- [ ] Phase 0: per-phase timings and signposts
- [ ] A: writer actor, N edits or M seconds, flush points, state indicator
- [ ] B: private recovery journal
- [ ] C1: hard-link archive; C2: head cache; C3: compact encoding (Swift and Go)
- [ ] D: retention at archive time for new saves
