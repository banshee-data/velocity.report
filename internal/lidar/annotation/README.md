# Immutable point packs and reference annotations

This package separates human reference objects from predicted track identities. Packs retain
exact point indices; the [label set](labelset/README.md) a pack links holds reviewed, proposed,
and rejected membership independently of tracker splits and reruns, and this package reads it
projected onto the pack's points. The selection client is the macOS visualiser's annotation pane
(`tools/visualiser-macos/VelocityVisualiser/Annotation`), which reads these packs and saves to the
same label sets; it is not supplied by this package.

## What a pack carries

| File                                 | What it is                                                                | In the pack digest |
| ------------------------------------ | ------------------------------------------------------------------------- | ------------------ |
| `points.bin`, `samples.json`         | The point domain: every frame's returns, by the index a mask cites        | Yes                |
| `manifest.json`                      | Identity, coverage, coordinate contract, and the run's L4 height band     | It holds it        |
| `background.bin`, `backgrounds.json` | The settled-background snapshots in force over the excerpt, when recorded | No                 |

A pack also carries its returns: `packets.bin` (the packet table, keyed by a digest of each UDP
payload), `returns.bin` (one return number per point), and each sample's frame range. Both files
are inside the pack digest. They are what lets a [label set](labelset/README.md) made against one
run's pack apply to another run's pack of the same capture. Packs are schema version 2, the only
version this package reads or writes. `OpenPack` refuses a version 1 pack, which had no returns:
those were migrated to label sets
([migration report](../../../docs/lidar/operations/annotation-label-set-migration.md)), and the
git tag `annotation-v1-reader` keeps the last build that reads one.

| Writer                                                               | Contract                                                                                                                                 |
| -------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| `WritePack(dir, m, samples, blocks, returns)`                        | A version 2 pack, staged and renamed into place; refuses a point without its return                                                      |
| `WritePackWithBackground(…, returns, backgrounds, backgroundBlocks)` | The same, with the settled-background snapshots in force over the excerpt                                                                |
| `FixtureReturns(samples)`                                            | Fabricated returns for tests and synthetic fixtures: packets of their own per sample, deterministic keys. Sets each sample's frame range |

`Export` writes version 2. The VRLOG's point frames name their returns: each frame's packet table,
one return number per point and the frame's full return range (see
[`visualiser.proto`](../../../proto/velocity_visualiser/v1/visualiser.proto) `PointCloudFrame`).
`Export` joins the frames' tables into one in recording order. A packet that the rotation split
between two consecutive frames is listed once; any other repetition is refused, because its
returns could not be told apart. A recording made before frames carried identity, or from a source
without packets, is refused with `ErrNoReturnIdentity`: replay its PCAP with a current build to
record it again, then export that. The HTTP export answers such a run with 422.

The background is context for whoever is labelling: it shows what the run had already decided
was static, and when that decision changed. No mask can cite a background point, so it has
digests of its own in the manifest and stays out of the pack digest. The snapshot in force at a
frame is the last recorded at or before it, by position in the recording. Neither its timestamp
nor its sequence number orders it: a replay settled ahead of time opens with a snapshot stamped
after every frame that follows, and the sequence number only moves on a grid reset.

The exporter refuses `full` coverage when every classed point is foreground. Today's recordings
keep foreground returns and periodic background snapshots, so their packs are `foreground_only`
with a background behind them.

### Intensity presence

Every point block is a fixed size, so a frame that carried no intensity column is written with
zero bytes where measurements would be. A zero byte cannot say which it is, and nothing may infer
presence from the bytes. Presence is recorded instead, at two levels:

| Field                                       | Meaning                                                                                             |
| ------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| `manifest.json` `has_intensity`             | Some sample's intensity bytes are measurements. It cannot say which                                 |
| `samples.json` `has_intensity` (per sample) | This sample's bytes are measurements. Written `true` or `false` on every sample the exporter writes |

Read presence through `Pack.SampleHasIntensity`: the sample's own flag when recorded, else the
manifest's, which is all a sample cut before the field existed has. The migrated packs are such:
their samples came from version 1 packs, and the pack digest covers them as they are. Check
`Pack.IntensityPresenceIsPerSample` first when every byte read must be a measurement rather than
some sample's; it is false for a pack whose samples record no flag, or record it on some only.
When every sample records its flag, the manifest must equal their disjunction. `WritePack` refuses
to write the contradiction and `OpenPack` refuses to read it, since the manifest sits outside the
pack digest and an edit to it passes every digest check.

The exporter reads the `FrameBundle` point cloud, which has no column-presence bit: a
frame is present when its column holds one byte per point and absent when it holds none. A column
of any other length is refused, because zero-filling its tail under a `true` flag would be a lie
and dropping it under `false` would discard readings. Samples in packs written before this field
carry no key, so their bytes and digests are unchanged.

## Membership

A pack's membership (its objects, masks, track correspondences and dismissed proposals) lives in
the label set named by `label-set-link.json` in the pack. A pack with no link has none yet.

| Entry point                   | Contract                                                                                                                |
| ----------------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| `LoadSidecar`                 | The label set's head projected onto this pack's samples and point indices; an unlinked pack starts empty                |
| `LoadSidecarRevision`         | The same at a retained revision; refused for an unlinked pack, and for a revision older than the history the copy holds |
| `Sidecar.Digest`              | The membership token: the label set's state digest, which physical references, features and frozen splits pin           |
| `ReviewedMasks`               | Only masks whose object and mask are both reviewed; a complete object mask does not certify background negatives        |
| `labelset.SaveMembership`     | Fold an edited projection back into the label set, creating and linking one beside the pack on the first save           |
| `RegisterMembershipProjector` | How package `labelset`, which imports this one, supplies the projection; importing `labelset` registers it              |

A projection is a view: it shows the returns this pack's run separated as foreground. A save
through it replaces what it shows and keeps what it does not, so a mask's members in another
run's foreground survive an edit made through this one; deleting a mask that still holds them is
refused. History is the label set's journal, kept locally and never published; see
[label sets](../../../docs/lidar/operations/label-sets.md) for the files, locking and recovery.

Physical references and feature proposals stay in the pack, each with its own revision store:
the current document, prior revisions under a directory of its own, and `.annotations.lock`. The
writer takes a non-blocking kernel lock, writes unique temporary files, syncs, renames atomically
and syncs the directory. `ErrSidecarBusy` means another writer holds the lock; `ErrSidecarConflict`
means the base changed, and the caller reloads and reconciles. This is not a network-filesystem
locking protocol. Each document is limited to 64 MiB, and reads and writes stay inside the pack
directory through `os.Root`.

## Physical references

Body geometry, keyframe poses and following gaps live in `physical-references.json`, with a
separate review from the masks. The record is documented in the
[point annotation tool guide](../../../docs/lidar/operations/point-annotation-tool.md#physical-references).

| Entry point                                                         | Contract                                                                                  |
| ------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| `LoadPhysicalReferences`, `SavePhysicalReferences`                  | The pack's revision store and lock; a save refuses any record whose links do not hold     |
| `PhysicalReferenceSet.Stale`, `StaleAgainst`                        | The records a later membership edit invalidated, found by the load, and the revision      |
| `LoadPhysicalReferenceRevision`, `RestorePhysicalReferenceRevision` | Read a retained revision; restore one as a new revision                                   |
| `PhysicalReferenceSet.Validate`                                     | Pack, source, bounds, evidence, provenance, gap bumpers and conflicting following records |
| `ValidateLinks`, `LinkProblems`                                     | Objects declared, cited frames holding them, observed claims borne out by the returns     |
| `PhysicalReferenceSet.ContentDigest`                                | SHA-256 over the references alone, unchanged by revision metadata                         |
| `PhysicalObject.Geometry`, `PhysicalReferenceSet.Geometries`        | A keyframe's centre, bumpers and box with conservative bounds, or why each is unavailable |
| `ParsePhysicalImport`, `PreparePhysicalImport`                      | An independent import merged with the stored references and checked as a save; no write   |
| `ImportPhysicalReferences`                                          | The prepared import, saved as a new revision                                              |

A record's origin is fixed when it is created. The store carries every record ID's origin forward
in `record_origins`, so a tracker-assisted record cannot become independent by review, by editing,
or by being deleted and added back under its old ID.

Membership saves do not read the references, and the macOS client makes them without this
package, so a later edit of the label set can invalidate a saved reference. Loads find those records and list them
in `Stale`; saves refuse them. No file is rewritten to mark them.

## Scoring against the annotations

| Entry point                     | Contract                                                                                                        |
| ------------------------------- | --------------------------------------------------------------------------------------------------------------- |
| `BuildReference`                | One reference point per mask under a recorded policy: position rule, review status, road-user classes, partials |
| `LoadSplitManifest`             | Read a hand-written version 1 split; refuse shared objects and episodes that score another partition's objects  |
| `SplitManifest.ValidateAgainst` | Bind it to one pack digest, dataset, pinned revision and the objects that revision still carries                |
| `SplitManifest.SelectEpisodes`  | Return a split's episodes; with held-out scoring requested, refuse a tuning split with `ErrNotHeldOut`          |
| `FreezeSplit`                   | Freeze a draft over packs; refuse unfinished review, one capture in two roles, and holding out anything tuned   |
| `LoadAnySplit`                  | Read either version; a version 2 file whose content no longer matches its `split_digest` is refused             |
| `FrozenSplit.Bind`              | Re-check a frozen split's pins against a pack, derive again what it copies, and return its version 1 form       |
| `FrozenSplit.CaseRoles`         | A corpus case's role; a held-out case replays only as a held-out score, which takes no other                    |
| `FrozenSplit.CheckCaseCaptures` | Refuse a replay of a case unless each capture is the case's: by SHA-256 where declared, else by basename        |

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
object-disjoint dataset splits derived from them (`velocity lidar annotation-split freeze`). The
[annotation plan](../../../docs/plans/lidar-point-annotation-and-object-dataset-plan.md) owns
that; the three-day demo's descriptor model and seeded tracker remain separate follow-ons.
