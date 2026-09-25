# Point annotation and temporal object datasets

This plan lets a person mark the returns belonging to one physical object and follow that
identity through a recording. It separates human evidence from tracker output so a split track
does not split the reference vehicle as well.

- **Status:** Revision-safe backend and macOS annotation client implemented, with proposal, propagation and review; reviewed dataset and its acceptance open
- **Canonical:** [point-annotation-tool.md](../lidar/operations/point-annotation-tool.md)
- **Layers:** L4 Perception, L5 Tracks, L6 Objects, L9 Endpoints, L10 Clients, offline analysis
- **Related:** [Shape descriptors](lidar-shape-descriptors-plan.md), [Test corpus](lidar-test-corpus-plan.md), [Labelling and QC](lidar-visualiser-labelling-qc-enhancements-overview-plan.md)

## 1. Outcome and boundaries

**Current delivery:** The v0.5.2 pilot supplies independent evidence for temporal body extent and
identity, then the held-out references that sprints 0.5.2.2 and 0.5.2.4 validate occlusion
continuity and the tailgating report against. Review complete
physical-object episodes, not isolated tracker IDs. Record observable yaw and front/rear extent
bounds separately from point membership; unknown or prior-only bumpers remain labelled as such.
Include vehicle, pedestrian and cyclist occlusions in the broader corpus; the rigid-vehicle demo
alone does not validate those classes. Use separate reference pairs for gap acceptance.

The operator can pause a VRLOG-derived point cloud, select returns, assign a physical
object identity, correct the selection in another view, and save a reviewed annotation.
The resulting dataset supports point membership, temporal association, shape
descriptors, and seeded single-object tracking.

The first delivery is local and offline. It does not replace track labelling, change production
classification, or claim to recover surfaces the sensor never recorded. A selected point is an
observed return, not a permanent landmark on the vehicle: point identities do not persist across
scans, but the annotated object identity does.

Keep membership evidence, uncertain seed dimensions, and physical pose reference labels separate.
A mask does not certify unseen dimensions or cross-frame point correspondence. Future reviewed masks
must not enter a causal evaluation; report assisted corrections separately from unassisted predictions.

The [visibility-aware review][visibility-research]
defines the estimator contract. Keep membership evidence, uncertain seed dimensions, and
physical pose reference labels separate. A mask does not certify unseen dimensions or
cross-frame point correspondence. Future reviewed masks must not enter a causal evaluation;
report assisted corrections separately from unassisted predictions.

[visibility-research]: ../../data/maths/proposals/20260905-visibility-aware-object-tracking-research.md

## 2. Existing work and the missing contract

The root contains pack/export, digest, point-index, reference-object, and sidecar groundwork.
Revision safety landed in `a8481872d`, with a final overflow guard and API documentation in this
recovery increment. Saves reject stale revisions and changed source bytes, serialise local writers,
archive exact prior bytes, and restore history as a new revision. Failed saves preserve the
caller's
dirty state. See the [storage contract](../../internal/lidar/annotation/README.md).

The operator workflow is now delivered in the macOS visualiser
(`tools/visualiser-macos/VelocityVisualiser/Annotation`): orthographic lasso and rectangle
selection over a depth slab against canonical pack indices, add/subtract, stroke-level undo and
redo, a second view for the contamination check, operator provenance collection, and
dirty-navigation protection. Selection is through-slab, not hidden-surface picking; a radius
brush, depth-aware picking and reviewed propagation (section 5.2) remain future work.

What remains is operator work rather than engineering: reviewed masks across a site's keyframes,
and the frozen object-disjoint dataset splits derived from them. Requirements below are acceptance
contracts unless explicitly covered by tested implementation; automatic proposals remain
unreviewed after restore.

The current label APIs annotate run tracks and replay time spans. Migration
`000033_replay_annotations_and_eval_integrity.up.sql` provides replay annotations independent of an
optional run-track link, but contains no point-membership representation. Reuse that distinction;
do not overload `user_label` with point indices.

The macOS renderer already draws point clouds and selects track boxes. It has no inspected
point-painting workflow. The wire format carries point arrays and optional cluster samples, but no
stable per-point membership IDs. The shape-descriptors plan supplies bounded cluster retention;
annotation additionally needs an immutable source domain and context outside predicted boxes.

Existing QC remains keyed by `run_id + track_id`. Point-level reference truth is
keyed by `dataset_id + object_id + sample_id`. A versioned mapping connects the two.
One reference object may map to several predicted tracks, and one merged predicted
track may map to several reference objects.

## 3. Immutable annotation source

### 3.1 A frozen excerpt pack

Export a bounded excerpt from a specified VRLOG into an annotation pack. The source recording
stays unchanged. The pack contains a manifest, canonical point arrays, and a frame lookup;
annotation revisions are separate sidecars. A pack can be recreated only as a new version if its
point domain changes. The three-day sprint loads this local pack into the existing renderer; it
does not require a new live annotation service.

| Field                                           | Meaning                                                                                           |
| ----------------------------------------------- | ------------------------------------------------------------------------------------------------- |
| `dataset_id`, `source_digest`, `schema_version` | Immutable pack identity and content verification                                                  |
| Source provenance                               | VRLOG digest, source PCAP digest when available, run/config/build identities, and export version  |
| `sample_id`                                     | Dense pack-local identifier, unrelated to tracker frame numbering                                 |
| Source frame reference                          | Original record ordinal, frame ID, capture timestamp, sensor ID, and frame type                   |
| Coordinate contract                             | Metres, axis directions, handedness, origin convention, and versioned sensor-to-site transform    |
| Point domain                                    | Point count, array digest, attribute availability, and original array ordering                    |
| Point reference                                 | `sample_id + point_index`, valid only under this pack digest                                      |
| Capture coverage                                | Full, foreground-only, or decimated; recording and export filters recorded separately             |
| Export completeness                             | Requested/actual time bounds, missing frames, timestamp gaps, and dropped-point counts            |
| Height band                                     | The run's L4 floor, ceiling and switch, so a client can show which points the clusterer never saw |
| Settled background                              | The snapshots in force over the excerpt, as context with digests of their own                     |

Use canonical little-endian float32 arrays for coordinates and documented encodings for
attributes. Hash the stored bytes and their schema. Never reconstruct point identity by
approximate coordinate matching: coincident returns can still be different points. Index
validation rejects negative, duplicate, and out-of-range references. An identical
membership set has one canonical sorted encoding.

The exporter separates background snapshots from observation frames, verifies array
lengths, and retains original ordinals before constructing a chronological sample
lookup. Duplicate timestamps are legal if unambiguous sample references exist. Record
ordering is not assumed to be capture order.

Background snapshots are carried in `background.bin` and `backgrounds.json`. They are context
for the operator and no mask can cite them, so their digests sit in the manifest beside the
pack digest rather than inside it, and a re-export that adds them keeps the pack's identity.
The snapshot in force at a sample is the last recorded at or before it, by record ordinal. Not
by timestamp: a replay settled ahead of time opens with a snapshot stamped after every frame
that follows it, which also used to end any export given an end time at frame 0. Nor by
sequence number, which only moves on a grid reset.

The exporter refuses `full` coverage when every classed point is foreground. Coverage is stated
because a sparse full scene cannot be told from a foreground-only one by counting points, but a
full scene always has background in it, and the recorder's own class bytes say when it has none.

### 3.2 What old recordings can support

An existing point-bearing VRLOG can be annotated without reconstructing its DBSCAN membership.
Human selection is authoritative; the current box is only a suggestion. A foreground-only recording
supports masks over recorded foreground, not complete raw-scene segmentation. A decimated recording
supports labels on its retained sample, not on discarded returns.

Recordings made today keep per-frame foreground returns and a settled-background snapshot about
every fifteen seconds. Checked on one 300 s run: 3,000 foreground frames of about 5,400 returns
and 20 snapshots of about 67,000. Their packs are therefore `foreground_only` with a background
behind them, and the labelable domain is the foreground.

For full-scene datasets, regenerate an annotation capture from PCAP with an explicitly recorded
point-retention policy. Keep background, ground, and neighbouring-object context where available.
Never export only the target's hand-labelled points as the input to a tracking benchmark.

Changing clustering, decimation, calibration, or export ordering invalidates old point references.
Import fails closed on digest mismatch. Reattachment to regenerated data is a separate reviewed
transfer operation, not a silent nearest-neighbour lookup.

## 4. Annotation records

| Record               | Required content                                                                   |
| -------------------- | ---------------------------------------------------------------------------------- |
| Object               | Stable object ID, human class, optional subtype, confidence, and review status     |
| Frame mask           | Object/sample IDs, sorted point indices, membership completeness, and revision     |
| Uncertain points     | Explicit uncertain membership; excluded from hard positive/negative scores         |
| Object visibility    | Present, partly occluded, fully occluded, outside view, or unknown                 |
| Pose annotation      | Optional box/pose, body-frame origin, ambiguity, uncertainty, and provenance       |
| Track correspondence | Optional predicted run/track references with temporal bounds                       |
| Revision             | Parent revision, operation, author/session, UTC timestamp, and source digest       |
| Proposal provenance  | Algorithm/config version, seed revision, and suggested rather than reviewed status |

Unselected means unlabelled, not background. A frame can be marked exhaustively reviewed inside a
specific ROI; only then can its unassigned returns be treated as local negatives. A point cannot be
reviewed as belonging to two objects simultaneously. Conflicting edits require resolution.

Keep semantic class and research subtype separate. Follow the existing seven selectable labels; for
example, `car` with subtype `box_truck`, or `bus` with subtype `school_bus`. Do not enable reserved
production enum values through a dataset export. A Waymo-labelled exemplar needs a verified
platform subtype; its operator label is not a geometric feature or proof of identity.

Point masks describe visible returns. Physical boxes include inferred unseen extent and therefore
carry a separate confidence and annotation source. Do not derive a supposedly exact full vehicle
box from the minimum and maximum of a partial mask.

Optional part masks can label an observed body side, cab, cargo body, or roof within the
object. They inherit the same point references and review rules. Body-front/rear labels remain
ambiguous unless evidence resolves them. Parts are not required for the initial dataset, and no
absent surface receives a fabricated mask.

## 5. Selection and paint interface

### 5.1 Minimum reliable interaction

1. Enter annotation mode on a paused immutable sample. Show source coverage and point count.
2. Create or select a reference object independently of predicted tracks.
3. Draw an orthographic lasso or rectangle; add/subtract membership with a modifier key.
4. Restrict selection by a visible height/depth slab. Show the candidate count before acceptance.
5. Inspect the selection from a second view, correct contamination, and save the revision.
6. Step to the next keyframe while retaining object identity, not the previous frame's point IDs.

Selection operates on the pack's canonical arrays, not GPU buffer positions after rendering
filters. Any display decimation must preserve the mapping to canonical indices. In the sprint's
clipped orthographic view, select all canonical points inside both polygon and slab; explicitly
label this as through-slab selection. It is not hidden-surface picking. Perspective front-surface
painting needs a depth/ID buffer and is outside the minimum delivery.

Navigation, playback, source switching, or a filter change must not retarget an unfinished stroke.
Finish or cancel it first. Undo/redo operates on membership edits and survives a save/reload
through the revision log. A dirty-session warning prevents accidental loss.

### 5.2 Paint and temporal assistance

The full interface adds a radius brush/eraser, depth-aware point picking, optional
normal-consistent region growth, and a timeline showing reviewed, suggested, and missing masks.
Region growth stops at depth discontinuities and never silently selects an entire predicted box.

Propagation uses a selected seed mask/pose to suggest membership in subsequent scans. It writes new
per-frame point references, not copied indices. Suggested masks have a different colour and cannot
enter reference truth without confirmation. Crossing objects, loss of overlap, or ambiguous yaw
stop propagation and request review. An operator may accept, modify, reject, or start a new object.

What is built differs from the paragraph above in three ways. The brush is a sphere that paints
and a column brush on a local 0.5 m lattice; there is no normal-consistent region growth.
Propagation carries a voxelised footprint of the mask, refitted to each frame, rather than a
pose; it stops on too few returns (scaled for range), too many, a rival fit that is a separate
peak, a fit further than the object could have moved, or another object's returns, and there is
no yaw test. And proposals do not start from a predicted box at all: see §10.

Support temporal object-link corrections without mutating the original tracker. A merge of human
identities preserves both histories and invalidates affected exports. Geometry accumulation must be
recomputable after a mistaken frame or association is removed.

## 6. Storage, export, and safety

For the sprint, the pack and versioned JSON annotation sidecar are the source of truth.
Save through a temporary file followed by atomic replacement, preserving prior revisions.
Validate schema, digests, finite coordinates, counts, and membership bounds before
committing a write. A damaged or incompatible sidecar opens read-only with a useful
error. Use one writer; reject stale revisions.

Large point arrays remain ordinary dataset files, not JSON model parameters and not
database rows. The first exporter may use compact binary arrays with a JSON manifest.
Keep a documented loader and round-trip test. Production database indexing and
server-connected multi-user writes are later work.

All import paths are relative to the pack, cannot escape it, and have explicit size limits. Raw
captures and annotated road-user instances stay local by default. Do not upload them or commit them
to ordinary Git. The repository receives schemas, small synthetic fixtures, and compact parameter
models only. Any distribution of real captures needs a separate licence/privacy decision.

## 7. Dataset splits and evaluation boundaries

Split by physical object/trajectory before fitting models. All frames, propagated masks, revised
identities, and augmented views of that object remain in the same partition. Adjacent frames are
not independent examples. If a repeated vehicle cannot be identified reliably, group conservatively
by capture block and disclose the limitation. One-site results demonstrate that site only.

Export distinct products:

- **Reference masks:** reviewed point memberships, uncertainty/coverage flags, and optional poses.
- **Tracker inputs:** surrounding scene points, sensor
  transforms, timestamps, and initial seed only.
- **Descriptor examples:** mask-derived features with formula versions and valid-support counts.
- **Predictions:** estimated poses, membership, descriptors, and
  diagnostics, never reference fields.

Keep two labelled modes. In unassisted SOT, only the initial seed is available to the
tracker; all later masks are evaluation-only. In assisted annotation, later corrections
may reseed tracking, but every intervention and resulting segment is counted. Do not
report the latter as autonomous tracking.

For classification experiments, report oracle-mask results separately from predicted-mask results.
Otherwise good human segmentation can conceal a failing tracker. Missing reference poses mean pose
error and box IoU are unavailable, not zero. Point membership remains measurable without them.

The frozen form of a partition is a split manifest (`velocity.report/annotation-split` v1, read by
`annotation.LoadSplitManifest`): the pack digest, object-disjoint splits with a `tuning` or
`held_out` role, and episodes as frame intervals plus the objects each scores. The per-frame
evaluator refuses to score a tuning split as held out, and ignores any other object that shares an
episode's frames. Field table and refusals:
[per-frame evaluation](../lidar/operations/per-frame-evaluation.md#held-out-episodes-the-split-manifest).

## 8. Compatibility with SOTracker and D-04

SOTracker starts from a supplied box and scene point clouds, then estimates motion and accumulates
shape. It is not a semantic classifier. Its registration, shape, and motion terms make it a useful
offline comparator without neural checkpoints. Our JSON class scorer is a separate component, not
an input SOTracker requires. [Paper](https://arxiv.org/html/2103.06028v2)

The upstream loader expects `pc` and `ego`; its seed box order is `[x, y, z, yaw, l, w, h]`. An
adapter must verify centre/base-Z conventions, radians, handedness, timestamps, and whether points
are already world-aligned. Use an identity transform only for points already expressed in the
declared world frame. Review the pinned revision's dependencies and reuse licence before adopting
or distributing code. [Upstream API](https://github.com/tusen-ai/LiDAR_SOT)

D-04 consumes a derived object-local geometry belief. Preserve raw masks and points so face
interpretations can change without rewriting evidence. Store per-surface support and viewpoint
coverage; repeated observations from one aspect do not establish an unseen dimension. Keep shape,
pose, semantic class, and their uncertainty separate. The current heading/fragment guards remain
replaceable and must not define annotation truth.

## 9. Delivery gates

- Round-trip membership is exact, including coincident points, empty masks, and boundary indices.
- Source mismatch, missing frames, duplicate timestamps, and malformed arrays fail explicitly.
- Lasso plus depth slab cannot select outside the advertised domain; a second view verifies this.
- Add, subtract, undo, redo, save, reload, and cancellation preserve the intended object identity.
- Automatic proposals cannot become reviewed labels through export or reload.
- Reference identities survive a predicted split, merge, and fresh pipeline run.
- Test-set masks and future poses cannot reach unassisted tracking or model fitting.
- Sparse/foreground-only sources retain their limitations through every export.

## 10. Delivery record

Delivered in the macOS client and the Go exporter. The operator's guide is
[point-annotation-tool.md](../lidar/operations/point-annotation-tool.md).

| Area                | Delivered                                                                                                   |
| ------------------- | ----------------------------------------------------------------------------------------------------------- |
| Pack                | Export from a run over HTTP and CLI; height band and settled background carried; false `full` refused       |
| Views               | Two orthographic views that pan, zoom and hold their framing; a 3D view; frame sync with the main view      |
| Display             | Class toggles with counts; ground as what the height band removed; background updates signalled and shown   |
| Selection           | Lasso, rectangle, depth slab, sphere paint brush with hover preview, column brush with voxel toggles, undo  |
| Objects             | Named by class and number; class and edit-state colours in every view; other objects shown while labelling  |
| Temporal assistance | Carry to the next frame with nudge; unattended propagation with stop rules; apply to every frame for fixed  |
| Proposals           | Fixed clutter by persistence; moving objects as clusters chained by footprint; accept, split, merge, reject |
| Review              | Per frame and per object; reaches the masks; a changed frame returns to proposed; provenance kept           |
| Progress            | Sixteen-sector ring for the frame; a bar a frame for the pack                                               |
| Correction          | Split an object that is two, carried through its frames; merge a proposal into an object                    |

Measured on one real pack of 200 frames, against the operator's own labels. A hand-labelled car
was matched by one proposal in all 19 of its frames at a median overlap of 1.00 (least 0.97), and
that proposal ran on to 165 frames. Propagation carried the same car from 19 frames to 167. The
422 moving and 23 fixed proposals covered 83% of the labelable foreground. Labelling one object
in one frame by hand took a median of six seconds, which for the 1,954 larger object-frames in
that pack is three to six hours: the reason the unit of work became the object.

Not delivered: region growth, depth-aware picking, reattachment of labels to regenerated points,
frozen dataset splits (§7; the manifest format, its reader and the evaluator that enforces it are
delivered, the splits themselves are operator work), and pruning of retained revisions, which grow
by a full snapshot a save.
The proposer clusters the pack's points itself instead of reading the run's clusters, because a
recording keeps cluster boxes and not their membership, and the tracker's identities would bring
its fragmentation with them.

## 11. Carrying labels across runs of the same capture

Labels are bound to a pack digest, and a pack is cut from one run. Every new run of the same
PCAP, which is what tuning produces, would have to be labelled again. §3.2 leaves reattachment
as "a separate reviewed transfer operation". This section records what makes that transfer
exact rather than approximate, and proposes it.

**Measured.** Recordings of the same PCAP were compared frame by frame, matching returns by the
bit patterns of their coordinates within frames of equal timestamp.

| Pair                                          | Frames sharing a timestamp | Returns bit-identical in both |
| --------------------------------------------- | -------------------------- | ----------------------------- |
| Same parameters, different builds (`s2_sf_3`) | 600 of 600                 | 100% of 2,318,167             |
| Different parameters (`kirk0`, three pairs)   | 291 to 600 of 521 to 600   | 83% to 99% of either side     |

No frame in any recording held two returns with the same coordinates, so within a frame a
return's coordinates identify it. A return is computed from range, azimuth and elevation before
L3 sees it, so the same packet gives the same three floats in every run. What differs between
runs is which returns L3 kept as foreground, not what any of them is.

**So a label's durable form** is (capture identity, frame timestamp, coordinate bits), not
(pack digest, sample, index). The second is derived from the first for any one pack.

**Proposed transfer**, as a Go command and an action in the tool, from a labelled pack to
another pack of the same PCAP:

1. Refuse unless both manifests name the same PCAP and sensor. A capture digest belongs in the
   manifest for this; the basename is what is there today.
2. For each frame timestamp in both, map each labelled return to the index of the bit-identical
   return in the new pack. These are exact: the same return, labelled by the same person.
3. Returns in the new frame that the old run did not keep cannot be matched. Those inside the
   object's footprint for that frame are added as proposed, recorded as geometric.
4. Write the result as proposed masks with `algorithm: return_identity_transfer`, the source
   pack digest, and per mask the counts matched exactly, added by footprint, and lost. A reviewed
   source mask becomes a proposed one: the operator reviewed different membership.

**Open before building.** Two `kirk0` pairs shared only half their frame timestamps, which
means frame boundaries moved between those builds; matching should then look in the neighbouring
frames, and how often that arises on current builds is not yet measured. Classification and
height band differ between runs by design and are not transferred. The proposer could use
transferred labels as seeds, so that a new run starts from the last run's objects.

This plan is the implementation slice and acceptance record for the annotation pilot. Any later
demo or state-estimation work must retain these evidence boundaries rather than treating a reviewed
mask as a direct observation of hidden geometry.
