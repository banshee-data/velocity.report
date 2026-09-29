# Physical reference review: independent body geometry for 0.5.2

This plan adds the reference authoring and scoring that the tailgating sprint needs to judge a
physical body estimate. Point masks establish which returns belong to a car; a separate reference
must establish what is known about the car's pose, dimensions, and physical endpoints.

- **Status:** Planned; optional pose storage exists, but the macOS authoring and physical scoring workflow is not delivered
- **Target:** v0.5.2, Sprint 0.5.2.0; S0 of the [MVP sprint plan](lidar-052-mvp-sprint-plan.md)
- **Layers:** L5 Tracks, L8 Analytics, L10 Clients, annotation and offline evaluation
- **Canonical:** [point annotation tool](../lidar/operations/point-annotation-tool.md)
- **Related:** [annotation datasets](lidar-point-annotation-and-object-dataset-plan.md), [state estimation](lidar-state-estimation-plan.md), [behaviour analytics](lidar-behaviour-analytics-plan.md), [visibility-aware geometry](../../data/maths/proposals/20260905-visibility-aware-object-tracking-research.md)

## Why this belongs in the first sprint

The current Annotation window reviews point membership, class, and physical-object identity across
frames. It does not let the operator set an independent expected physical box. The Go and Swift
sidecars already have an optional pose record, but storage is only part of the workflow.
`annotation.BuildReference` derives its evaluation position from the visible mask's footprint
centre or point mean; neither is the physical body centre, and it does not consume that pose.

The annotation queue therefore contains two kinds of work. Reviewing masks is operator work that
can begin now. Authoring physical references and connecting them to the geometry and following
scorers requires engineering first. Completing the mask queue cannot close the physical gates.

This is a dependency of the qualified 0.5.2 result, including the reviewed portions of G-GEO-1,
G-UNC-1, G-SMO-1, and the following metric gate. Tracker development, label-free comparisons, and
the visibly provisional MVP can continue while the workflow and references are prepared. A
validated import of independent physical references may satisfy the same contract; the native
editor is the intended local route, not the definition of truth.

## Reference contract

Use one physical-object identity across its episode. Keep a persistent body geometry belief apart
from each keyframe's pose and visible returns. A change from a side view to an end view changes
the evidence, not the object's identity or the definition of its centre. Real turns and braking
remain changes in pose and motion; stable geometry must not force a straight trajectory.

| Reference fact        | Required meaning                                                                                                             |
| --------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| Source and time       | Pack digest, object ID, sample ID, capture timestamp, coordinate frame, units, and applicable sensor/calibration identity    |
| Reference point       | Explicit physical body centre or a named supported surface with a declared offset; never an implicit mask centre             |
| Body geometry         | Length, width, and height bounds, body-axis convention, and supporting observations or independent measurements              |
| Keyframe pose         | Position and body yaw with stated uncertainty; retain axis and front/rear ambiguity where evidence cannot resolve them       |
| Evidence status       | Per component: observed, inferred, prior-only, or unknown; identify the supporting frames or external reference              |
| Review and provenance | Author, source/method, review status, revision, and uncertainty assumptions, separate from the membership review             |
| Following reference   | Independently reviewed leader, no-leader, or ambiguous decision, with supported endpoint/gap bounds over the stated interval |

The existing pose's confidence scalar is not a positional or dimensional error bound. Define a
versioned extension or linked reference record for component bounds, evidence status, and source
before implementation. Keep current masks and sidecars readable and preserve existing revision
safety. Record shared reference errors or a conservative joint bound when a centre, dimension,
and endpoint depend on the same observation; separate fields do not make their errors independent.

A partial span may support a lower bound. It cannot establish an unseen full width or bumper by
itself. A person drawing a plausible box does not make those surfaces measured. Keep unsupported
components unknown, or obtain independent evidence. Repeated views of the same face must not
manufacture confidence through frame count alone.

## Operator workflow

1. Choose a source-pinned pack with the delivered Segments flow. Fix its tuning or held-out role
   before choosing the window; use traffic or random selection for held-out references.
2. Review membership and identity with the existing tools: correct points, separate different
   objects, join fragments of the same object, and review the masks and object independently.
3. In a new physical-reference mode, choose informative keyframes around changes of visible face,
   occlusion, turning, or braking. Start from raw evidence or an independent measured seed.
4. Place, rotate, and size the reference box in the top and elevation views, with numeric controls
   for position, yaw, and dimensions. Declare bounds and which components are supported, inferred,
   or unknown. Keep object-level dimensions consistent across the episode unless evidence revises
   them, with that revision recorded.
5. Review each supplied reference. Copies, temporal propagation, and interpolated poses remain
   proposals until reviewed; unreviewed frames are not silently filled with reference truth.
6. Review leader/no-leader and physical endpoint evidence for the selected following episodes.
   Retain ambiguity, low-speed suppression, and missing truth as explicit outcomes.
7. Freeze object-disjoint references and split digests. Run the selected estimator version and
   inspect a separate prediction overlay and the scored differences. A reference changed after
   scoring creates a new revision and evaluation result.
   `velocity lidar annotation-split freeze` freezes membership review today and records geometry
   review from the optional pose beside it; P0's reference record should be pinned there too.

Reference authoring defaults to a blind view. Tracker boxes may be shown for diagnosis after the
reference is recorded, or copied as an explicitly tracker-assisted proposal; they never become
independent truth through a review click alone. Keep initial seeds supplied to a tracking
experiment separate from later evaluation-only references. Future masks and poses must not enter
an unassisted tracker or its parameter fitting.

The first operator queue covers changing faces, a partial front/rear view, an oblique body, a
bend, nearby lane distractors, a leader switch, ambiguous ordering, standstill, and a recording
gap. Include genuine braking and lateral manoeuvres for the geometry and refinement gates.
Only claim applicability for the classes, views, ranges, and support represented by the references.

## Scoring and inspection

Add an explicit physical-reference path to the evaluation tools. Keep the current mask-position
policy named and available for identity comparisons; it must not silently become a body-centre
accuracy score. Score physical centre, yaw, dimensions, endpoints, and following gap only where
the corresponding independent reference and uncertainty are available. Box overlap is unavailable
when the reference cannot establish a complete box.

Match by source, capture instant, reference frame, and physical-object correspondence. Report
reference uncertainty alongside prediction error, and distinguish true motion from an apparent
step at a face transition. An ambiguous yaw axis cannot support a signed front/rear error.

The inspector shows reviewed references and predictions as separate layers, with the estimator
version, reference revision, physical anchor, component bounds, and suppression reasons. Seeking,
changing runs, or selecting another estimate stage must not compare different instants or retain
a previous version's box.

Account for every expected reference interval, including missing predictions, unknown geometry,
and unmatched objects. Keyframes alone do not justify reference coverage of the intervening
frames. Publish the scored and unscored populations by the required strata. Pin error limits,
reference precision, nominal coverage, encounter counts, and missing-opportunity limits on tuning
data before held-out scoring. Missing truth or a missing criterion is insufficient evidence.

## Delivery order and acceptance

| Increment                           | Completion evidence                                                                                                                                                                                                           |
| ----------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| P0: reference storage and import    | Component bounds, ambiguity, support, source, and revisions round-trip without changing masks; an independent import passes the same validation; invalid frames and source mismatches refuse                                  |
| P1: native authoring                | An operator can place and revise a physical reference at keyframes, save/reload it, and distinguish geometry review from mask review; unknown components and proposals retain their status                                    |
| P2: evaluator and inspector         | A saved reference produces the expected centre/axis/endpoint comparison against a named estimate version; missing geometry yields unavailable scores and complete accounting; reference and prediction layers remain separate |
| P3: reviewed pilot and frozen split | An operator completes a bounded episode queue, records independent physical/pair evidence, and freezes object-disjoint tuning and held-out revisions with digests                                                             |

Acceptance must exercise a straight pass with changing visible faces, a real turn and lane
change, partial and sparse support, an unresolved axis, an occlusion, and a contaminated mask.
Check undo, reload, source/version changes, and reference revision during inspection. Prove that
reviewing a mask does not review its pose, that tracker-assisted suggestions cannot acquire
independent provenance, and that references withheld for evaluation never reach tracker inputs.

P0–P2 deliver the missing engineering workflow. P3 is the operator pass that follows it. Passing
the workflow checks is not a pass of the physical estimator or following gates: those still need
their existing evidence and thresholds. The week of live observations and hardware/power-loss
qualification remain separately tracked under the persistence and deployment plans.

## Checklist

- [ ] P0: versioned physical-reference contract, independent import, and revision-safe round trip
- [ ] P1: macOS pose/extent authoring with component bounds, ambiguity, and separate review
- [ ] P2: physical-reference scoring, prediction comparison, and complete missing-evidence accounting
- [ ] P3: operator pilot, independent pair/manoeuvre references, and frozen object-disjoint splits
- [ ] Physical gates scored against the frozen references; insufficient evidence remains visible
