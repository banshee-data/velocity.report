# Immutable point packs and reference annotations

This package separates human reference objects from predicted track identities. Packs retain
exact point indices; sidecars hold reviewed, proposed, and rejected membership independently of
tracker splits and reruns. The selection client is the macOS visualiser's annotation pane
(`tools/visualiser-macos/VelocityVisualiser/Annotation`), which reads these packs and writes
these sidecars through the same revision protocol; it is not supplied by this package.

## What a pack carries

| File                                 | What it is                                                                | In the pack digest |
| ------------------------------------ | ------------------------------------------------------------------------- | ------------------ |
| `points.bin`, `samples.json`         | The point domain: every frame's returns, by the index a mask cites        | Yes                |
| `manifest.json`                      | Identity, coverage, coordinate contract, and the run's L4 height band     | It holds it        |
| `background.bin`, `backgrounds.json` | The settled-background snapshots in force over the excerpt, when recorded | No                 |

The background is context for whoever is labelling: it shows what the run had already decided
was static, and when that decision changed. No mask can cite a background point, so it has
digests of its own in the manifest and stays out of the pack digest. The snapshot in force at a
frame is the last recorded at or before it, by position in the recording. Neither its timestamp
nor its sequence number orders it: a replay settled ahead of time opens with a snapshot stamped
after every frame that follows, and the sequence number only moves on a grid reset.

The exporter refuses `full` coverage when every classed point is foreground. Today's recordings
keep foreground returns and periodic background snapshots, so their packs are `foreground_only`
with a background behind them.

## Revision-safe storage

| Entry point              | Contract                                                                                                                |
| ------------------------ | ----------------------------------------------------------------------------------------------------------------------- |
| `LoadSidecar`            | Read the current bounded, validated snapshot and its private concurrency token; an untouched pack starts empty          |
| `SaveSidecar`            | Save an edit of that loaded snapshot; do not increment its revision manually                                            |
| `LoadSidecarRevision`    | Read a retained revision without making it current                                                                      |
| `RestoreSidecarRevision` | Restore a snapshot as a new revision, retaining its original membership and review status                               |
| `ReviewedMasks`          | Return only masks whose object and mask are both reviewed; a complete object mask does not certify background negatives |

Set `Change.Author`, `Change.Session`, and `Change.Operation` for an operator action. The store
supplies parent/current revision numbers and a UTC timestamp. It never invents a human author.
Anonymous legacy data remain readable; the future client must collect operator provenance.

Load a document, edit its membership, then save that same document. The revision and a digest of
the exact loaded bytes both have to match the current file. A fresh document cannot overwrite an
existing one, and editing its public revision cannot bypass the private token. Keep separate
documents for separate sessions; concurrent mutation of one Go object is not supported.

`ErrSidecarBusy` means another writer owns the transaction. Retry after it completes.
`ErrSidecarConflict` means the base changed: reload and explicitly reconcile the unsaved edit.
Neither error discards or canonicalises the caller's dirty data. Do not resolve a conflict by
blindly copying the old mask over the newer revision.

## Files and failure behaviour

The immutable manifest and point arrays are unchanged. Annotation storage uses:

- `annotations.json`: current snapshot, replaced atomically.
- `annotation-revisions/0000000001.json`: exact prior snapshots, retained before advancing the
  current file. The latest snapshot is read through the same revision API.
- `.annotations.lock`: persistent lock inode. The kernel releases its lock when the descriptor
  closes or the process exits. Do not delete this file while a writer might be running.

The local macOS/Linux writer uses a non-blocking kernel lock, unique temporary files, file sync,
atomic rename, and directory sync. Failed archive writes leave the current revision unchanged.
An error after replacement may represent an uncertain commit: reload before retrying. This is
not a distributed or network-filesystem locking protocol; do not mix older, unguarded writers
with this writer.

Each JSON snapshot is limited to 64 MiB. Reads and writes stay inside the pack directory through
`os.Root`; non-regular sidecars and linked archive destinations are refused. Historical files
are not a tamper-proof audit service or an off-device backup. Back up the whole pack with its
annotation history before moving or distributing it.

Legacy schema-v1 snapshots acquire history on their next save without rewriting their original
bytes. A corrupt current file opens with an error, never as an empty dataset. If history exists
but the current file is missing, new saves are refused. Retained snapshots can still be read
by revision for explicit recovery; damaged-head replacement is not automated.

Undo and redo use the same restore operation. Restoring revision 1 over revision 2 produces
revision 3; restoring revision 2 then produces revision 4. Neither action changes earlier files
or converts an algorithm's proposal into a reviewed label. Ordinary saves clear the restore
marker. This is revision-level recovery, not the future client's unsaved-stroke undo stack.

## Scoring against the annotations

| Entry point                     | Contract                                                                                                        |
| ------------------------------- | --------------------------------------------------------------------------------------------------------------- |
| `BuildReference`                | One reference point per mask under a recorded policy: position rule, review status, road-user classes, partials |
| `LoadSplitManifest`             | Read a frozen split; refuse shared objects and episodes that score another partition's objects                  |
| `SplitManifest.ValidateAgainst` | Bind it to one pack digest, dataset, pinned revision and the objects that revision still carries                |
| `SplitManifest.SelectEpisodes`  | Return a split's episodes; with held-out scoring requested, refuse a tuning split with `ErrNotHeldOut`          |

Masks a person did not certify become ignore points rather than truth: unreviewed, not a road user,
not visible, only uncertain returns, or completeness never stated. Rejected and empty masks are
dropped. The per-frame evaluator that consumes these, and the manifest's field table, are
documented in [per-frame evaluation](../../../docs/lidar/operations/per-frame-evaluation.md).

## Remaining reference-loop work

Canonical-index lasso/slab selection, sphere and column brushes, second-view inspection,
dirty-session navigation protection and operator provenance are delivered in the macOS client.
So is what an operator needs in order to find the object: views that pan, zoom and hold their
framing from sample to sample, a 3D view, class filters for background, foreground and ground,
and frame sync with the main view's replay of the same recording. So is what keeps the work to an object at a time rather than an object in each frame: the
client proposes objects from a pack (fixed clutter by persistence, the rest as clusters followed
through the frames), carries an accepted mask forwards and back until the fit is one a person
would not have accepted, and lets a review reach the masks. Masks made that way record the
algorithm that made them and stay proposed until a person reviews them. What remains is operator
work rather than engineering: reviewed masks across a site's keyframes, and the frozen
object-disjoint dataset splits derived from them. The
[annotation plan](../../../docs/plans/lidar-point-annotation-and-object-dataset-plan.md) owns
that; the three-day demo's descriptor model and seeded tracker remain separate follow-ons.
