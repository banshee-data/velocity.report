# Point annotation and temporal object datasets

This plan lets a person mark the returns belonging to one physical object and follow that
identity through a recording. It separates human evidence from tracker output so a split track
does not split the reference vehicle as well.

- **Status:** Proposed
- **Layers:** L4 Perception, L5 Tracks, L6 Objects, L9 Endpoints, L10 Clients, offline analysis
- **Related:** [Three-day demo](lidar-single-site-shape-demo-sprint-plan.md),
  [Shape descriptors](lidar-shape-descriptors-plan.md),
  [State estimation](lidar-state-estimation-plan.md), [Test corpus](lidar-test-corpus-plan.md),
  [Labelling and QC](lidar-visualiser-labelling-qc-enhancements-overview-plan.md)

## 1. Outcome and boundaries

The operator can pause a VRLOG-derived point cloud, select returns, assign a physical object
identity, correct the selection in another view, and save a reviewed annotation. The resulting
dataset supports point membership, temporal association, shape descriptors, and seeded
single-object tracking.

The first delivery is local and offline. It does not replace track labelling, change production
classification, or claim to recover surfaces the sensor never recorded. A selected point is an
observed return, not a permanent landmark on the vehicle: point identities do not persist
across scans, but the annotated object identity does.

## 2. Existing work and the missing contract

The current label APIs annotate run tracks and replay time spans. Migration
`000033_replay_annotations_and_eval_integrity.up.sql` provides replay annotations independent
of an optional run-track link, but contains no point-membership representation. Reuse that
distinction; do not overload `user_label` with point indices.

The macOS renderer already draws point clouds and selects track boxes. It has no inspected
point-painting workflow. The wire format carries point arrays and optional cluster samples, but
no stable per-point membership IDs. The shape-descriptors plan supplies bounded cluster
retention; annotation additionally needs an immutable source domain and context outside
predicted boxes.

Existing QC remains keyed by `run_id + track_id`. Point-level reference truth is keyed by
`dataset_id + object_id + sample_id`. A versioned mapping connects the two. One reference
object may map to several predicted tracks, and one merged predicted track may map to several
reference objects.

## 3. Immutable annotation source

### 3.1 A frozen excerpt pack

Export a bounded excerpt from a specified VRLOG into an annotation pack. The source recording
stays unchanged. The pack contains a manifest, canonical point arrays, and a frame lookup;
annotation revisions are separate sidecars. A pack can be recreated only as a new version if
its point domain changes. The three-day sprint loads this local pack into the existing
renderer; it does not require a new live annotation service.

| Field                                           | Meaning                                                                                          |
| ----------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| `dataset_id`, `source_digest`, `schema_version` | Immutable pack identity and content verification                                                 |
| Source provenance                               | VRLOG digest, source PCAP digest when available, run/config/build identities, and export version |
| `sample_id`                                     | Dense pack-local identifier, unrelated to tracker frame numbering                                |
| Source frame reference                          | Original record ordinal, frame ID, capture timestamp, sensor ID, and frame type                  |
| Coordinate contract                             | Metres, axis directions, handedness, origin convention, and versioned sensor-to-site transform   |
| Point domain                                    | Point count, array digest, attribute availability, and original array ordering                   |
| Point reference                                 | `sample_id + point_index`, valid only under this pack digest                                     |
| Capture coverage                                | Full, foreground-only, or decimated; recording and export filters recorded separately            |
| Export completeness                             | Requested/actual time bounds, missing frames, timestamp gaps, and dropped-point counts           |

Use canonical little-endian float32 arrays for coordinates and documented encodings for
attributes. Hash the stored bytes and their schema. Never reconstruct point identity by
approximate coordinate matching: coincident returns can still be different points. Index
validation rejects negative, duplicate, and out-of-range references. An identical membership
set has one canonical sorted encoding.

The exporter separates background snapshots from observation frames, verifies array lengths,
and retains original ordinals before constructing a chronological sample lookup. Duplicate
timestamps are legal if unambiguous sample references exist. Record ordering is not assumed to
be capture order.

### 3.2 What old recordings can support

An existing point-bearing VRLOG can be annotated without reconstructing its DBSCAN membership.
Human selection is authoritative; the current box is only a suggestion. A foreground-only
recording supports masks over recorded foreground, not complete raw-scene segmentation. A
decimated recording supports labels on its retained sample, not on discarded returns.

For full-scene datasets, regenerate an annotation capture from PCAP with an explicitly recorded
point-retention policy. Keep background, ground, and neighbouring-object context where
available. Never export only the target's hand-labelled points as the input to a tracking
benchmark.

Changing clustering, decimation, calibration, or export ordering invalidates old point
references. Import fails closed on digest mismatch. Reattachment to regenerated data is a
separate reviewed transfer operation, not a silent nearest-neighbour lookup.

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

Unselected means unlabelled, not background. A frame can be marked exhaustively reviewed inside
a specific ROI; only then can its unassigned returns be treated as local negatives. A point
cannot be reviewed as belonging to two objects simultaneously. Conflicting edits require
resolution.

Keep semantic class and research subtype separate. Follow the existing seven selectable labels;
for example, `car` with subtype `box_truck`, or `bus` with subtype `school_bus`. Do not enable
reserved production enum values through a dataset export. A Waymo-labelled exemplar needs a
verified platform subtype; its operator label is not a geometric feature or proof of identity.

Point masks describe visible returns. Physical boxes include inferred unseen extent and
therefore carry a separate confidence and annotation source. Do not derive a supposedly exact
full vehicle box from the minimum and maximum of a partial mask.

Optional part masks can label an observed body side, cab, cargo body, or roof within the
object. They inherit the same point references and review rules. Body-front/rear labels remain
ambiguous unless evidence resolves them. Parts are not required for the initial dataset, and no
absent surface receives a fabricated mask.

## 5. Selection and paint interface

### 5.1 Minimum reliable interaction

1. Enter annotation mode on a paused immutable sample. Show source coverage and point count.
2. Create or select a reference object independently of predicted tracks.
3. Draw an orthographic lasso or rectangle; add/subtract membership with a modifier key.
4. Restrict selection by a visible height/depth slab. Show the candidate count before
   acceptance.
5. Inspect the selection from a second view, correct contamination, and save the revision.
6. Step to the next keyframe while retaining object identity, not the previous frame's point
   IDs.

Selection operates on the pack's canonical arrays, not GPU buffer positions after rendering
filters. Any display decimation must preserve the mapping to canonical indices. In the sprint's
clipped orthographic view, select all canonical points inside both polygon and slab; explicitly
label this as through-slab selection. It is not hidden-surface picking. Perspective
front-surface painting needs a depth/ID buffer and is outside the minimum delivery.

Navigation, playback, source switching, or a filter change must not retarget an unfinished
stroke. Finish or cancel it first. Undo/redo operates on membership edits and survives a
save/reload through the revision log. A dirty-session warning prevents accidental loss.

### 5.2 Paint and temporal assistance

The full interface adds a radius brush/eraser, depth-aware point picking, optional
normal-consistent region growth, and a timeline showing reviewed, suggested, and missing masks.
Region growth stops at depth discontinuities and never silently selects an entire predicted
box.

Propagation uses a selected seed mask/pose to suggest membership in subsequent scans. It writes
new per-frame point references, not copied indices. Suggested masks have a different colour and
cannot enter reference truth without confirmation. Crossing objects, loss of overlap, or
ambiguous yaw stop propagation and request review. An operator may accept, modify, reject, or
start a new object.

Support temporal object-link corrections without mutating the original tracker. A merge of
human identities preserves both histories and invalidates affected exports. Geometry
accumulation must be recomputable after a mistaken frame or association is removed.

## 6. Storage, export, and safety

For the sprint, the pack and versioned JSON annotation sidecar are the source of truth. Save
through a temporary file followed by atomic replacement, preserving prior revisions. Validate
schema, digests, finite coordinates, counts, and membership bounds before committing a write. A
damaged or incompatible sidecar opens read-only with a useful error. Use one writer; reject
stale revisions.

Large point arrays remain ordinary dataset files, not JSON model parameters and not database
rows. The first exporter may use compact binary arrays with a JSON manifest. Keep a documented
loader and round-trip test. Production database indexing and server-connected multi-user writes
are later work.

All import paths are relative to the pack, cannot escape it, and have explicit size limits. Raw
captures and annotated road-user instances stay local by default. Do not upload them or commit
them to ordinary Git. The repository receives schemas, small synthetic fixtures, and compact
parameter models only. Any distribution of real captures needs a separate licence/privacy
decision.

## 7. Dataset splits and evaluation boundaries

Split by physical object/trajectory before fitting models. All frames, propagated masks,
revised identities, and augmented views of that object remain in the same partition. Adjacent
frames are not independent examples. If a repeated vehicle cannot be identified reliably, group
conservatively by capture block and disclose the limitation. One-site results demonstrate that
site only.

Export distinct products:

- **Reference masks:** reviewed point memberships, uncertainty/coverage flags, and optional
  poses.
- **Tracker inputs:** surrounding scene points, sensor transforms, timestamps, and initial seed
  only.
- **Descriptor examples:** mask-derived features with formula versions and valid-support
  counts.
- **Predictions:** estimated poses, membership, descriptors, and diagnostics, never reference
  fields.

Keep two labelled modes. In unassisted SOT, only the initial seed is available to the tracker;
all later masks are evaluation-only. In assisted annotation, later corrections may reseed
tracking, but every intervention and resulting segment is counted. Do not report the latter as
autonomous tracking.

For classification experiments, report oracle-mask results separately from predicted-mask
results. Otherwise good human segmentation can conceal a failing tracker. Missing reference
poses mean pose error and box IoU are unavailable, not zero. Point membership remains
measurable without them.

## 8. Compatibility with SOTracker and D-04

SOTracker starts from a supplied box and scene point clouds, then estimates motion and
accumulates shape. It is not a semantic classifier. Its registration, shape, and motion terms
make it a useful offline comparator without neural checkpoints. Our JSON class scorer is a
separate component, not an input SOTracker requires.
[Paper](https://arxiv.org/html/2103.06028v2)

The upstream loader expects `pc` and `ego`; its seed box order is `[x, y, z, yaw, l, w, h]`. An
adapter must verify centre/base-Z conventions, radians, handedness, timestamps, and whether
points are already world-aligned. Use an identity transform only for points already expressed
in the declared world frame. Review the pinned revision's dependencies and reuse licence before
adopting or distributing code. [Upstream API](https://github.com/tusen-ai/LiDAR_SOT)

D-04 consumes a derived object-local geometry belief. Preserve raw masks and points so face
interpretations can change without rewriting evidence. Store per-surface support and viewpoint
coverage; repeated observations from one aspect do not establish an unseen dimension. Keep
shape, pose, semantic class, and their uncertainty separate. The current heading/fragment
guards remain replaceable and must not define annotation truth.

## 9. Delivery gates

- Round-trip membership is exact, including coincident points, empty masks, and boundary
  indices.
- Source mismatch, missing frames, duplicate timestamps, and malformed arrays fail explicitly.
- Lasso plus depth slab cannot select outside the advertised domain; a second view verifies
  this.
- Add, subtract, undo, redo, save, reload, and cancellation preserve the intended object
  identity.
- Automatic proposals cannot become reviewed labels through export or reload.
- Reference identities survive a predicted split, merge, and fresh pipeline run.
- Test-set masks and future poses cannot reach unassisted tracking or model fitting.
- Sparse/foreground-only sources retain their limitations through every export.

The implementation slice, effort budget, and demo gates are in the
[three-day sprint](lidar-single-site-shape-demo-sprint-plan.md).
