# Facet annotation: select persistent parts without moving the body origin

This specification adds one to four selected vehicle features to the physical authoring design.
Each feature has a persistent identity and a frame-specific subset of the object's returns;
newly visible support may refine placement without repeatedly resetting the body reference.

- **Status:** Selection, active-facet pilot, compact/straight-edge horizontal registration and optional frozen pins built locally; runtime constraints, full contract and operator validation remain open
- **Scope:** Sparse facet proposals, review, body registration and bounded reanchor events
- **Canonical:** [point annotation tool](../lidar/operations/point-annotation-tool.md)
- **Related:** [facet registration experiment](lidar-facet-registration-experiment-plan.md), [authoring design](lidar-physical-authoring-comprehension-design.md), [physical references](lidar-physical-reference-review-plan.md), [shape descriptors](lidar-shape-descriptors-plan.md)

## Baseline and purpose

[PR #657](https://github.com/banshee-data/velocity.report/pull/657) supplies the offline facet
experiment, its A–D comparison arms and stop/go criteria. It is merged into main at `00d8b1ca3`.
This branch was rebased onto that main with the earlier authoring design preserved. The PR adds
a plan, not a facet annotation format or a native selection tool. This specification supplies
the operator/evidence increment for F0/F1, followed by the seeded F2 diagnostic.

The newer committed `dd/lidar/physical-pose-ui-929` foundation is now integrated into this branch.
It supplies the native physical editor and the recording-domain feature proposal writer. The
local increment adds exact lasso subsets, four active facets with retirement, and line/surface
support diagnostics. These proposals remain distinct from physical review and runtime anchors.
Compact features can now propose a horizontal metric offset under a pinned, reviewed
body/pose, with a named source return and conservative bound. Independent and tracker-assisted
seeds retain distinct proposal origins; neither establishes independent registration truth.
Straight edges can now propose a horizontal line relation from two direction-defining returns
and at least three supported points. The relation leaves tangent position unconstrained, retains
source pose/return bounds and refuses broad or degenerate support. Surface registration, full
3D constraints and the runtime event ledger remain engineering work. Optional frozen facet pins
now retain exact proposal revisions and assisted-origin counts; they are not scoring truth.

The [resource notes](lidar-facet-authoring-resource-notes.md) describe the implemented cache,
index payload and reproducible scalar fit timing. They do not qualify the runtime tracker.

An edge, a side patch, a corner or a wing mirror can be useful. Their visible returns change as
the sensor scans and the vehicle moves. The persistent thing is the feature on the object,
not a return index or the centroid of whichever points happened to strike it.

## One to four active features

Allow one to four active facets for each annotation object and experiment track lineage. Zero
is a valid starting or unsupported state: facet registration abstains. Four is a working-set
limit, not a claim that the vehicle has only four surfaces. Additional proposals may be saved
inactive, rejected or archived. Replacing a facet preserves its old identity and observations;
it never overwrites another physical feature under the same ID.

| Geometry type | Selected evidence                                              | Supported constraint                                                                    |
| ------------- | -------------------------------------------------------------- | --------------------------------------------------------------------------------------- |
| Edge segment  | Returns supporting a repeatable geometric line/edge            | Direction and displacement normal to the line; physical endpoints need separate support |
| Surface patch | A small, locally planar region                                 | Normal and normal displacement; in-plane motion may remain unknown                      |
| Local feature | A recognisable compact feature, such as a mirror tip or corner | Location only where its identity, local offset and uncertainty are supported            |

"Wing mirror" is an optional description of a local feature. It does not automatically supply
a plane, a rigid landmark, or a make/model label. A folding mirror or opened door must be marked
moving/articulated or excluded from rigid registration. A vehicle-part classifier is unnecessary
for the first labelled subsets.

## Minimum record: identity, observations and registration

Keep facet identity separate from its observations and its optional body registration. These
are proposed fields, not an implemented file layout.

| Record                  | Minimum content                                                                                                                                                                                |
| ----------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Document                | Schema/version, pack digest, coordinate/units contract, revision/concurrency token, pinned membership revision/digest and physical revision/digest when used                                   |
| Facet identity          | Persistent facet ID, annotation object ID, type, optional description, proposed/reviewed/rejected status, active state, rigid/moving/unknown attachment and author/source provenance           |
| Observation             | Facet ID, canonical sample ID, capture instant, sorted unique supporting point indices, uncertain indices separately, visibility, evidence status, review and membership pin                   |
| Geometry interpretation | Fit type/version/parameters, inlier subset, residual/spread/support diagnostics, direction/normal when supported, uncertainty and observable/weak directions; recomputable from frozen returns |
| Body registration       | Body revision, supported local position/direction or face relation/offset with bounds, support observations, independent/tracker-assisted origin and separate review                           |
| Reanchor event          | Track lineage/episode, instant, old/new registration revisions, reason, contributing facet IDs, observable correction/bounds, proposed/accepted/rejected outcome and applied-event count       |

A valid proposal needs a non-empty certain subset, object/source links, type and provenance.
Fit, body-local offset and uncertainty may remain unknown. Saving is not reviewing; reviewing
a subset is not reviewing its registration. Incomplete drafts remain visible and unscorable.

### Minimum support is specific to the geometry

- A line requires distinct returns with sufficient span. Three inliers is an initial pilot fit
  floor, not an accuracy guarantee. A mask boundary does not establish a physical edge.
- A plane requires at least three non-collinear inliers and sufficient two-dimensional spread.
  One straight scan strand cannot establish a confident surface normal.
- A compact feature may be proposed from one return. Registration requires repeatable physical
  identity/local geometry or an independent measurement, with bounds. One bright return alone
  remains diagnostic; it does not establish a stable wing mirror or body-local point.
- Pin span, rank, residual, overlap and temporal-repeatability thresholds on development data in
  F0/F1 before the comparison. Minimum point count alone cannot certify support.

### Preserve the point domain and its review

Reuse the pack's canonical indices, never GPU positions or filtered screen order. Certain facet
indices must be a subset of the object's certain mask in that sample. Uncertain indices are not
hard positives or registration inputs. Reject duplicates, wrong-frame/object/domain links and
out-of-range indices. Creating a facet must never expand the parent mask as a side effect.

Membership changes can make observations stale. Keep their history and require revalidation;
silently intersecting support must not retain review. Adjacent primitives may share corner
returns, but declare shared support and avoid counting it twice in a likelihood. The first tool
must warn and require an explicit shared-support choice or one estimator supplier.

Point indices have no identity across frames. Re-identifying a facet stores new indices under
the same facet ID. Occlusion has an explicit state and no fabricated support. No visible returns
is different from a frame that nobody reviewed.

## Turn a selected subset into a facet proposal

Add **Facets** to Guided body and a **Facet points** tool beside Register face. Dim the scene
outside the selected object. This tool edits a temporary subset; it cannot edit membership or
move the body accidentally.

1. Select the object/frame. Resolve unsaved membership or explicitly pin the saved mask being
   cited. Choose **Facet points**.
2. Lasso/brush a small feature in an orthographic view using the depth slab. Intersect the result
   with the parent's certain mask. Show "12 of 278 object points" plus excluded/uncertain counts.
   Cross-check an elevation to catch through-depth picks.
3. Choose **Make facet proposal**. Pick Edge segment, Surface patch or Local feature and optionally
   name it, for example "mirror tip". A fit preview may suggest a type, but must retain missing
   normals, edge ambiguity and weak support.
4. Save. The list shows facet ID, type, point count, visibility, review and registration readiness.
   A fifth activation offers **Replace an active facet** or **Keep as inactive proposal**.
5. At the next informative frame, choose **Add observation to this facet** and select its new
   returns. Propagation stays proposed; it never copies numeric indices into the next sample.
6. Review support and feature identity. Use **Register to body** only when local relations and
   bounds are supported, independently or with declared assistance. A reviewed facet may remain
   unregistered when the centre, size or an along-surface relation is unknown.

Use human labels such as Facet A–D with letters and line/patch markers, not colour alone. A list
click fits/highlights the feature; point inspection never places an anchor. **Suggest facets from
selected points** is a later extractor action after manual save/reopen works. Record its version
and source; acceptance cannot make a tracker-seeded proposal independent pose evidence.

## Keep the origin stable; allow two or three justified corrections

Maintain one body coordinate convention and shared size. A newly visible facet adds a relation
to that frame; it does not redefine the centre. Compatible measurements update pose smoothly
in supported directions. True turns, braking and lateral motion remain ordinary movement.

The requested flexibility becomes an explicit experimental policy:

- Record initial seed/birth separately. Target at most **two** accepted discrete reanchors and
  permit a maximum of **three after birth** over the logical track lifecycle. This is a ceiling.
- New independent support, such as an end becoming visible after a side-only view, may propose
  correction of a weak component. Adding/removing a facet alone consumes no event.
- Require geometry, overlap, uncertainty and new observability. Use past-frame confirmation and
  hysteresis pinned in F0/F1; future annotations cannot confirm an online correction. Prefer an
  ordinary bounded update where it satisfies the model.
- Accepted resets record old/new mappings and their effect. Rejected proposals/no-matches use
  no applied-event slot but remain in the diagnostic history.
- After the third accepted event, continue compatible updates. An incompatible reset proposal
  shows **Reanchor limit reached** and leaves affected components weak/suppressed until supported
  recovery without another reset. Do not retain an unsupported centre just to keep a smooth box.

Define a snap in the frozen protocol as an explicit registration reset/replacement or a
discontinuous correction beyond a predeclared uncertainty-scaled threshold. Log explicit events
and detected unlogged jumps. Thresholds account for acquisition interval and real manoeuvres;
counting events alone does not establish stable trails. A correction must not be disguised as
a normal update to evade the cap.

Track fragmentation or a new public ID must not reset the budget. Carry it through the physical
episode/lineage and resume after occlusion. An unresolved merge cannot share facets/budgets
between two vehicles. Unknown lineage means the lifecycle limit cannot be claimed. Reference
editing remains unlimited; the cap governs estimator behaviour, not human corrections.

Reference changes are not travelled distance or acceleration. Translate/revise affected history
explicitly where needed, retain the original causal version, and expose the correction in the
inspector. Never silently rewrite past online output with a later correction or score final
revised output as though it had been available online.

A side patch primarily constrains normal motion. Its visible centroid must not be registered to
the side midpoint, which would invent an along-face constraint. Two non-parallel supported faces
or an identifiable local feature may resolve more directions. Fixed size must not force straight
travel or suppress a true turn. A folded mirror invalidates its rigid relation rather than moving
the whole body to follow it.

## Engineering needed to start registering

Reuse [FrameMask](../../internal/lidar/annotation/sidecar.go) and
[selection](../../tools/visualiser-macos/VelocityVisualiser/Annotation/PointSelection.swift).
Do not overload masks, their optional Pose, physical-reference schema v1, or point colour: they
have no persistent facet identity/support contract.

Start with a separately versioned linked facet document and explicit client capability discovery.
Keep immutable pack data unchanged. The Go service owns writes with validated links, conflict
checks, history and retained origins, mirroring the physical writer. Pin membership and physical
revisions; extend the frozen bundle with facet revision/content digest before experiments consume
it. Older clients must not round-trip or erase documents they cannot edit.

| Increment                | Deliverable                                                              | Exit                                                                             |
| ------------------------ | ------------------------------------------------------------------------ | -------------------------------------------------------------------------------- |
| FA0: contract/writer     | Identities, sample subsets, visibility, separate reviews and source pins | Sparse proposal saves/reopens; invalid links refuse; stale membership is visible |
| FA1: native selection    | Facet points, Make proposal, Add observation and 1–4 active list         | Indices survive view/filter/zoom changes; parent masks do not change             |
| FA2: geometry audit      | Line/patch/feature interpretations and observable-direction diagnostics  | F1 repeatability/degeneracy checks; sparse support can abstain                   |
| FA3: body mapping/events | Local relations, bounded corrections and event ledger                    | Fixed dimensions/origin; side tangent stays weak; limit survives track splits    |
| FA4: seeded comparison   | Offline registration adapter and inspector                               | PR #657 F2 matched arms, causal boundaries, resource and accuracy scorecard      |

The first useful operator output is saved subsets across two or three informative frames.
FA0/FA1 need no full body dimensions. FA3 requires a supported body mapping or a declared uncertain
seed. Obtain acquisition time/scan topology from a compatible full observation profile for ring
experiments. If the point pack lacks them, restrict the audit to spatial subsets/patches and
re-extract richer evidence where needed.

Manual facets are a favourable diagnostic, like reviewed memberships. Keep evaluation-only poses
separate. A later annotated mirror is unavailable to an autonomous tracker before it is detected.
Seed-assisted, review-assisted and automatic results retain separate E2/E3 accounting.

## Minimum pilot and checks

Use two rigid vehicles: one changing side/end view and one turn with a compact feature if it is
resolvable. Select one to four facets with two or three observation frames each; include explicit
occlusion/missing support. Add a long featureless side as a negative case. Match source instants
and seed policy to the PR #657 comparator.

Storage/UI checks cover outside-object and uncertain points, a fifth activation, shared support,
stale membership, wrong sample/domain, conflicts, independent reviews, unknown geometry, retained
assisted provenance and visibility changes. Registration checks cover tangent ambiguity, parallel
edges, non-parallel support, a sampling boundary mistaken for an edge, a folding mirror, false
matches, real turns, three accepted events and a refused fourth, recovery without a reset, and
split/merge lineage. Measure size breathing, logged/unlogged jumps, manoeuvre preservation,
uncertainty coverage, endpoint error and supported opportunity.

Keep PR #657's A–D controls and physical gates. A UI contract is not an accuracy result or a reason
to delay the headway MVP. The native app now stores feature proposals with exact sample subsets
and membership pins;
it stores compact-feature and straight-edge horizontal body registration proposals but does not yet apply them to
the runtime tracker. Optional schema-4 frozen pins retain the proposal revision, exact-byte and
canonical-content digests, support/absence counts and assisted-registration counts. Inspection
is available through `BindFeatures` against the frozen membership. An experiment must call that
binding path before consuming the proposals and retain its seed policy; the existing physical
scorer does not consume them automatically.

The native **Project saved relation here · assisted** check projects a saved body_xy relation
through another exact reviewed pose without refitting. Compact spots retain their metric offsets;
straight edges retain their tangent weakness. The Top-only preview withholds incompatible bodies,
stale/unreviewed membership links, unsupported target poses and unconstrained line normals.
Current definite facet support may report cached absolute spot/normal distances, never an
accuracy score. Viewing is remembered as assistance for later proposals in that object/frame,
while saved evidence remains intact. This supports the manual two-to-three-frame pilot; it does
not implement the runtime accepted-event ledger or PR #657's controlled ablation.
