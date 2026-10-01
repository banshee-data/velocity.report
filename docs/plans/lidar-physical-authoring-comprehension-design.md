# Physical annotation: make the object, its pose and the evidence visible

The physical editor exposes a careful evidence model before it gives the operator a usable
authoring task. This design pass turns that model into a visible workflow: define one body,
place it at selected frames, register the surfaces that support the placement, then compare a
named estimate against a saved reference.

- **Status:** Source audit with initial comprehension fixes built locally; guided size workflow and operator validation remain open
- **Scope:** Physical and Compare modes, the current annotation pilot, and the next experiments
- **Canonical:** [point annotation tool](../lidar/operations/point-annotation-tool.md)
- **Related:** [physical reference review](lidar-physical-reference-review-plan.md), [0.5.2 sprint](lidar-052-mvp-sprint-plan.md), [state estimation](lidar-state-estimation-plan.md)

## Audit boundary and current readiness

The supplied screenshot and accessibility snapshot show the annotation pack from run
`ad8b9438-c1d4-40f0-bd0a-f1852c984569`, with car 5 selected at source frame 297. The snapshot
reports unsaved membership, an unsaved physical draft, no saved physical references, and a
validation error for a dimension declared known without its lower bound. These are observations
of the supplied snapshot, not a live inspection of the running application.

The original audit used `0b54067700abd1ce54af8fccd60938f8470349d8`. It has P0 physical storage,
P2 scoring, and the earlier point editor. The matching native Physical and Compare implementation
is in `/Users/david/code/velocity.report`, branch `dd/lidar/physical-pose-ui-929`, audited at
`ddea4159d`. The main checkout was clean when read. This audit did not modify that checkout or
the annotation pack, and does not establish which binary commit the running application uses.

| Tooling                                     | What exists in the audited code                                                                                                              | What is still needed                                                                                       |
| ------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| Membership and identity                     | Point selection, proposals, carrying, object review and revision history                                                                     | Finish the current operator run; record failures without losing corrections                                |
| Physical record and writer                  | Versioned body/keyframe records, bounds, evidence, origins, service validation and revision checks                                           | A comprehensible authoring workflow and successful operator round trip                                     |
| Native authoring on the newer branch        | Numeric fields, top-view anchor placement, elevation height placement, move/turn/length handles, copying a nearby keyframe, separate reviews | Visible incomplete drafts, width/height authoring, explicit tools, keyframe navigation and easier recovery |
| Compare on the newer branch                 | Read-only imported per-frame evaluation report, arm selection, source/revision checks and orthographic overlays                              | A guided report-generation path and a clear distinction from the main window's current tracker             |
| Split freeze on the newer branch            | Service-backed preview/freeze and physical revision/digest/review pins                                                                       | Complete the pilot, then verify a frozen bundle through evaluation and inspection                          |
| Reflectivity inspection on the newer branch | Raw colour/readout and distribution tools                                                                                                    | Establish per-frame data availability and repeatability before tracker experiments                         |

There is enough machinery to begin a bounded pilot. There is not yet operator evidence that
authoring, reviewing, freezing, scoring and reopening a report form a reliable working loop.
Storage and unit-test fixtures establish different facts from an operator completing that loop.
The older guide says no physical editor exists; the newer plan updates its status but retains
some earlier future-tense descriptions. Documentation needs a version-aware reconciliation.

## Explain the three things being edited

Use these labels in the product:

| Product label     | Meaning                                                   | Applies to                                            |
| ----------------- | --------------------------------------------------------- | ----------------------------------------------------- |
| Object points     | The returns that belong to this real object               | Membership in each frame                              |
| Object size       | The body's length, width and height, with their evidence  | The whole object episode                              |
| Pose at frame 297 | The body's location and direction at this capture instant | One explicit pose sample, currently called a keyframe |

A keyframe is a frame at which the operator explicitly records a pose. It is not a selectable
point in the cloud, a tracker track, or an animation command. The object moves between pose
samples while its dimensions remain shared. An explicit list must show which frames already
have poses, their review state and the evidence they establish.

The current numeric form expresses conservative limits: minimum and maximum are plausible
bounds on a dimension, and the optional value must lie between them. A partial observed span is
a lower bound only. A class prior needs a named source. Unknown needs no invented number.
Translate this into "Size", "How certain?" and "What supports it?", with advanced raw fields
available when needed. A default or copied value remains a proposal with its actual provenance.

## Why the supplied frame feels inert

The following findings are verified in the newer branch's source:

1. `placePhysicalAnchor` writes X/Y on a top-view click. `PhysicalGeometry.derive` produces an
   anchor only when position has an eligible status, X/Y **and a bound**. The overlay and its
   move handle use that derived anchor. A newly placed point without a bound can therefore be
   invisible. Yaw also needs a value and bound before the turn handle is available.
2. The length handle requires a resolved front/rear axis, a full bounded length and derived
   ends. A left/right face anchor has no length handle. Width and height have no equivalent
   drag handles. The form asks the operator to complete the prerequisites before offering the
   interaction that appears intended to establish them.
3. Only the active orthographic editing view places an anchor. A click in another view first
   makes that view active. Dragging away from an existing handle pans. The top 3D view is for
   looking; it does not place a reference. These meanings need an explicit tool and cursor.
4. "Reload, discard draft" calls `PhysicalReferenceSession.load()`. It reads the service's
   saved physical document and replaces its draft. It does not create a box, import the main
   window's track, or discard membership edits. On an empty document it loads an empty document.
   A successful load clears errors but does not give a clear completion message.
5. The screenshot has a front/rear-ambiguous axis with a named left face. The current contract
   permits named faces only with a resolved axis. The picker allows this invalid combination
   and explains it afterwards. Missing dimension bounds further prevent saving.
6. Compare opens a JSON evaluation report generated with `-physical-reference`. It does not
   compare automatically with the recording open in the main window. The audited implementation
   refuses comparisons on held-out packs and records estimate exposure for later edits.

The large selected point count for car 5 is a reason to inspect membership before deriving a
size, but the supplied image alone does not establish that its mask is contaminated.

## Keep the current annotation run moving

Finish identity and point-membership work first. Their save/review controls remain in Points
mode. Physical references have a separate draft, save and review; one does not save the other.
Before trying Physical again, save or deliberately discard each pending draft through its own
controls. Keep a short issue log with object, source frame, mode, action and visible result.

For a minimal physical pilot in the current build:

1. Select one well-supported object. Enter Physical and choose **Add body (all unknown)** if
   needed. Leave dimensions unknown until there is evidence; do not use a plausible car size as
   observed truth. Empty fields under "Prior only" still require valid bounds and a named prior.
2. Choose an informative frame. **Add keyframe at this frame** creates a pose record, or click
   the active Top view to place its position. Supply a justified position bound to make the
   current preview marker available. An elevation click supplies optional Z after X/Y exists.
3. Keep **Body centre** only if the centre is supported. For a visible face, resolve the axis
   sufficiently to name that face, give its position, and declare the offset and offset bound
   only when supported. A face without an offset may locate the face while leaving the centre
   unavailable. Do not make up an offset to obtain a rectangle.
4. Enter yaw and its bound when supported; retain front/rear ambiguity when that is all the
   evidence establishes. Full supported length and width enable the footprint preview. Numeric
   entry is currently the practical route for width. Some incomplete references will properly
   show only an anchor or axis; the current editor fails to distinguish that from no placement.
5. Supply supporting frames and uncertainty assumptions. **Save proposal**, inspect it, then
   review the saved body and the saved keyframe separately. Check a second frame with a changed
   visible face. Copying a pose is a proposal, not evidence that the object stayed in place.
6. Freeze a tuning split after the pilot records are reviewed. Generate the physical evaluation
   report with the pinned references and two named estimate arms, then use **Compare → Open
   Report…**. The [per-frame evaluation guide](../lidar/operations/per-frame-evaluation.md) covers
   the command route. There is no current "compare with this main-window box" shortcut.

If the present controls prevent a truthful partial reference, leave that component unknown and
record the obstacle. Membership review is useful even when physical geometry remains incomplete.

## Proposed authoring flow

Replace the modes' labels with **Label points**, **Place body**, and **Inspect comparison**.
At the top, always show object, source frame and an explicit work status. Keep the selected
object fitted in the authoring views, with scene context available separately. Elevations named
Front/Back today are sensor view directions; label those separately from the object's front/rear.

The recommended design uses a guided inspector beside an enlarged Top view and one useful
elevation. Other elevations remain available. A second design leads with supported surfaces and
a pose-frame list for operators who mostly register partial views.

### Start with a visible draft

Offer **Draw body**, **Start from object points**, **Start from tracker estimate**, and
**Enter measured size**. All lead to a visible, editable draft with a clearly named source.
Fitting points creates an observed-return envelope proposal; it does not establish full body
dimensions. Unknown faces are drawn with dashed edges and labelled "unseen".

Draft rendering must use a separate completeness model from scoring geometry. Show an X/Y
marker immediately, even when its uncertainty is missing. Show a yaw line when an angle exists,
even before its bound is supplied. Label missing uncertainty beside the mark and in the next-step
prompt. Keep save/review validation strict; seeing a draft must not promote its evidence.

### Define size once

The first panel says **Object size · shared across 141 frames**. Show length, width and height
with units and a small diagram. Allow top-view length/width and elevation height handles, plus
numeric entry. Provide separate **Move**, **Rotate**, **Resize** and **Register face** tools.
Move/Rotate hold size fixed. Resizing is a deliberate object-level edit, with a preview of which
pose reviews will need checking again before the revision is saved.

Use three simple evidence choices initially: **Measured/seen**, **Estimated from evidence**, and
**Unknown**. Offer **Use a named class prior** as a separate action. Preserve the existing exact
statuses in the record. Ask whether the complete extent is supported when the operator claims a
dimension; leave partial extents as lower bounds. Avoid a tiny full/partial picker with no context.

### Place the body in this frame

The second panel says **Pose at frame 297**. A visible direction arrow has choices **Front known**,
**Axis known, front uncertain**, and **Direction unknown**. Named front/rear/left/right faces must
not be selectable in an incompatible state. The body centre and selected face have different
markers and labels. Clicking a face selects the registration tool; dragging it registers that
face without resizing the body. Drawing a direction and placing an anchor work before their
uncertainty is complete.

The viewport prompt names the current action: "Click the visible side to place the face" or
"Drag the arrow to set direction". A first click must act in the clicked view, rather than
silently consuming the click only to activate it. Panning retains a distinct gesture.

### Show the evidence without making it a questionnaire

Place the next incomplete requirement beside the relevant field. Use **Cite this frame** by
default only for an action performed on this frame's raw evidence; imported or copied evidence
keeps its own source. Provide specific errors, for example:

> Length is marked known but has no size bounds. Enter a supported range, use a named prior,
> or set Length to Unknown.

Keep the raw method field, shared-error groups, nanosecond timestamp and sample ID in an advanced
section. Use source frame numbers consistently in ordinary controls. Uncertainty remains required
for scoring, but the interface must explain its unit and effect rather than expose the schema.

### Save, review and move through poses

Use separate visible badges for **Point labels**, **Object size**, and **Pose at this frame**.
Each has Draft, Saved proposal or Reviewed status. A summary action may orchestrate saves, but
must report which document succeeded and must not review automatically or discard the other draft.

Replace Reload with **Load saved physical reference**, showing what was loaded, whether any
draft was replaced, and whether the saved document is empty. Require a deliberate discard action
when physical work is dirty. Provide **Discard physical draft** and **Discard point edits** as
separate named actions with their own scope. A late service response must not erase newer edits.

Add a pose list and distinct timeline markers. **Add pose here**, **Previous pose**, **Next pose**
and **Copy pose as draft** make the existing keyframe operations discoverable. Show where size
is shared and where placement is per-frame. Interpolation is a proposed preview with an explicit
interval and method; it is not silently written as reviewed evidence between keyframes.

## Tracker estimates are useful seeds with lasting provenance

The requested shortcut should exist on tuning packs: **Start from tracker estimate**. It must
select a specific source instant, estimate version and stage, then let the operator choose the
corresponding real object. Track identity may split or merge while the annotation object remains
one object, so the mapping needs inspection. Refuse or explain frame/coordinate/source mismatch.

Show the original estimate as a ghost outline and the editable body as a separate outline.
Copy available position, orientation and extent as proposals, preserving anchor semantics:
an observed OBB or medoid cannot be relabelled as a supported physical centre. The imported
estimate may have only a visible envelope; dimensions must retain that meaning. Missing bounds
remain visibly incomplete rather than becoming arbitrary precision.

Keep `tracker_assisted` provenance through editing, review, copying and reopening. Such records
are useful for diagnosis, surface registration, and explicitly assisted initialisation experiments.
They cannot establish independent absolute accuracy of the tracker that supplied them. An
independently authored record can be compared after it is pinned; subsequent assisted edits form
a new revision while the earlier independent revision remains immutable.

The running app cannot detect that the operator looked at tracker output in another window.
Offer an explicit **I used the main-window estimate** declaration immediately. This is relevant
to the supplied session: viewing that box must not be represented as blind authorship merely
because the Annotation window paused its main-view link. Keep held-out authoring blind and use
a separate tuning episode for assisted work. Current physical Compare refuses held-out packs;
changing that policy is a separate evaluation-workflow decision, not a label change in this pass.

## Register returns to a body surface without making the body bounce

The [minimum facet specification](lidar-facet-annotation-minimum-spec.md) extends this flow with
one to four active edges, patches or compact features, each recorded as a subset of the object's
returns. It defines proposals, separate observation/registration review, and the requested
two-to-three correction ceiling without treating ordinary vehicle motion as a reset.

The proposed body coordinate frame has fixed dimensions and named faces. At each pose frame,
record which visible face supports translation/rotation and its relation to the centre. Show
the selected returns and their distance to the face, with unsupported directions visibly weak.
Multiple views can strengthen a dimension only when they supply new geometric information.

A changing point centroid on a long visible side must not move the body centre along that side.
A face patch primarily constrains displacement normal to its surface. It may supply little
information about translation along the face. Registering the patch centroid to the face's
midpoint would invent that missing constraint and reproduce the jumping-box problem.

Therefore the later surface tool needs a named body face, its normal and supported extent, a
declared local feature if one is recognisable, and uncertainty in the constrained directions.
Use two non-parallel faces or a persistent local feature to resolve more degrees of freedom.
The current scalar anchor radius and named-face position do not fully express a partial surface
constraint; assess an explicit schema extension rather than hiding one inside a full X/Y point.
An unresolved axis can use an unsigned side/plane constraint in a future extension; the present
schema's named left/right faces require a resolved front direction.

Fixed dimensions reduce shape breathing. Pose must still follow real turns, lateral motion and
braking. No face support, heavy occlusion or an ambiguous correspondence should produce a
visible missing/weak placement, not a confident box snapped to whichever returns remain.

Reflectivity can later help recognise a persistent patch on this body frame. It should not
control size or turn raw brightness into a confidence bound. First establish reliable geometry,
membership and source timing; then evaluate whether intensity adds a repeatable correspondence
signal on the same paired episodes.

## Issue list for after the current annotation run

Evidence tags distinguish screenshot observations, source-confirmed behaviour and proposed work.
These are local issues, not submitted GitHub tickets.

| ID    | Priority | Evidence and problem                                                                               | Desired result                                                                                |
| ----- | -------- | -------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| PA-01 | P1       | Source: newly placed anchors and yaw handles disappear until bounds are supplied                   | Incomplete draft geometry draws immediately; missing bounds stay visible and unscorable       |
| PA-02 | P1       | Snapshot/source: Reload feels inert; physical reload leaves membership dirty                       | Explicit document scope, loading/completion/empty feedback and deliberate discard             |
| PA-03 | P1       | Source/snapshot: named face can be chosen with incompatible ambiguous axis                         | Gate choices before selection; offer a supported centre or explain the unsigned-surface route |
| PA-04 | P1       | Snapshot: known dimensions have empty required bounds; error reports internal IDs                  | Inline human-readable correction at the dimension; unknown remains an easy valid choice       |
| PA-05 | P1       | Source: point and physical drafts have separate saves; global unsaved notice hides the distinction | Separate dirty badges and recovery for each document                                          |
| PA-06 | P1       | Source: editor cannot draw a useful box until many numeric prerequisites are complete              | Visible draw/fit/import proposal with guided next steps and exact provenance                  |
| PA-07 | P2       | Source: inactive-view click activates instead of placing; 3D click cannot author                   | Explicit tool state, immediate click feedback and clear 3D inspection affordance              |
| PA-08 | P2       | Source: move/turn/length only; small handles; side-anchor length editing absent                    | Larger effective hit targets, width/height tools and intentional resize with face constraints |
| PA-09 | P2       | Snapshot/source: keyframe concept and frame/sample numbering are opaque                            | Pose-at-frame naming, pose list, timeline marks and copy-as-draft preview                     |
| PA-10 | P2       | Source: Compare requires an external report, with no guided creation flow                          | Report wizard with pinned references, named arms, prerequisites and reopen action             |
| PA-11 | P1       | Snapshot/source: external tracker viewing cannot be detected by the blind-authoring flag           | Explicit assisted declaration and source-pinned seed import; lasting origin                   |
| PA-12 | P2       | Source: centre placement and face registration are mixed in one position form                      | Visual anchor/centre distinction and stable body-frame constraints                            |
| PA-13 | P2       | Docs: older worktree says editor absent; newer plan mixes delivered and future wording             | Version-aware guide with one worked example and current limitations                           |
| PA-14 | P2       | Snapshot: 2,211 points selected for one car; contamination is unconfirmed                          | Inspect selected membership and counts before fitting; warn about implausible envelopes       |
| PA-15 | P2       | Snapshot/source: raw intensity presence is pack-wide and calibration is absent                     | Preserve missing per-frame intensity separately from valid zeros before experiments           |
| PA-16 | P2       | Source: shared-size edits invalidate pose review across the episode                                | Show affected frames before save and a targeted re-review queue afterwards                    |

For each issue encountered during the run, record source frame, object name/ID, selected mode,
active view/tool, point/physical dirty states, action, expected result and visible result. Preserve
the saved pack and revision history; a screenshot alone is not a reproducible state record.

## What comes next for experimentation

1. **Complete membership and identity.** Clean the selected object's mask and its episode. Keep
   the current full-run labelling work, but use a small tuning episode for the physical pilot.
2. **Close one authoring loop.** On two rigid vehicles, author a straight pass with a face change
   and a turning/partial-view pass. Save/reload, review body and poses separately, freeze, score,
   open Compare, and revisit the exact instants. Record missing components explicitly.
3. **Fix comprehension blockers.** PA-01 through PA-06 and PA-11 are prerequisites for broader
   physical annotation. Preserve existing labels; these changes should not require relabelling.
4. **Compare geometry on pinned evidence.** Use paired estimator arms and the same source/frames.
   Measure identity switches, fragmentation, face-transition position excursions, independently
   supported yaw/endpoint error, uncertainty coverage, and unavailable/missing populations.
   Keep mask-position scores separate from physical-centre scores.
5. **Try fixed-body surface registration.** Start with explicit proposals and partial constraints.
   Compare against centroid/medoid behaviour while retaining true turns and lateral manoeuvres.
   A smoother trail without endpoint evidence is a stability result, not an accuracy result.
6. **Screen reflectivity.** On already reviewed memberships, compare within-object short-term
   repeatability with between-object separation by range, viewpoint and support. Test a bounded
   association cue before a dense object map. Report processing time, allocations and storage.

Do not use later authored pose frames as inputs to an allegedly unassisted online experiment.
If a reviewed seed or full-episode dimensions are supplied, name the experiment as assisted and
declare exactly which information was available and when. Keep evaluation references separate.

## Acceptance for implementing this design

- A first-time operator can explain object size versus pose-at-frame, place a visible draft,
  save/reload it and review it without consulting the schema.
- Missing bounds leave a visible incomplete draft; they never generate scorable certainty.
- Translation/rotation hold size fixed. Resize exposes its whole-object review consequences.
- A front/rear-ambiguous pose cannot acquire named signed faces or bumper accuracy by accident.
- Saving or discarding physical work cannot silently save or discard point membership.
- A partial visible side cannot impose an unsupported along-face body-centre constraint.
- Copied, interpolated and tracker-seeded placements remain proposals with retained provenance.
- A pinned independent revision remains reopenable after later assisted edits.
- Compare identifies source instant, estimate arm/stage, physical revision and unavailable reasons.
- The operator pilot exercises empty reload, service failure/conflict, dirty navigation, sparse
  support, a true turn, a face change, an occlusion and a membership change after physical review.

This pass was checked against source and the supplied UI snapshot. It does not reproduce the
running app's failures, run the physical pilot, or certify estimator accuracy. The accompanying
interactive sketches use illustrative geometry and show proposed interactions only.

## First implementation increment

The newer native pose and feature foundation is integrated on `codex/facet-authoring-design`.
Placement now draws an editable marker before an uncertainty bound exists, and its turn handle
can explicitly author an initially ambiguous axis. The displayed sketch is separated from
supported/scored geometry; the document keeps a missing bound absent. Reload reports the saved
revision or an empty document, invalid named-face choices are refused while the axis is
unresolved, and a pose-frame menu makes existing keyframes navigable.

The panel names shared object size and pose at this frame, explains the fixed-point constraint,
and accepts a placement on the first click when activating another orthographic view. Feature
mode adds exact lasso subsets within saved definite membership and the depth slab, four active
facets with retained retirement, and diagnostic line/surface fits. This closes part of PA-02,
PA-03, PA-04 and PA-06; it is not the completed guided workflow or an operator acceptance result.
A compact feature can also save a horizontal metric body registration proposal under a pinned,
reviewed body/pose, retaining whether either source is tracker-assisted. The operator selects a
named physical return and states its
return bound; changing dimensions does not rescale this mapping. Whole spans now use value and
explicit tolerance fields, with asymmetric min/max under an advanced disclosure. Missing bounds
remain missing in the saved model; a value-only or prior-only outline is labelled as a sketch.
A width handle complements the length handle without changing pose or the other dimension.
Assisted authoring can be declared explicitly, including for outstanding edits; undo keeps the
exposure ledger while preserving unchanged saved independent history. A source-pinned report can
seed an assisted sketch at an exact, uniquely matched instant when it states a physical body
centre. The original estimate stays a separate ghost outline, and imported statistical sigmas
do not become reference bounds. Existing size and poses are retained. Direct main-window import
remains open: its track stream does not declare the physical position's meaning. Weak-direction
edge/surface registration and the runtime snap ledger also remain open. The earlier audit above
remains a record of the supplied UI.

The current increment also wires the exposure ledger to the production session's preference
store, so reopening the pack retains the disclosure. Face anchors are labelled as face centres:
an arbitrary surface return cannot acquire an unsupported along-face centre relation. Invalid
draft values stay out of the editable overlay. Facet observation navigation keeps identity and
guards outstanding work; the histogram can inspect the facet's definite subset. Geometry fits
are cached for one immutable frame/subset/type so return hover does not repeatedly rebuild them.
