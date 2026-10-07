# Swift physical-pose authoring and inspection plan

Extend the macOS annotation window so an operator can record what is known about a car's physical
body, review that evidence separately from point membership, and compare a frozen reference with
named tracker estimates. It also adds raw intensity inspection and experimental distribution
summaries; the supervised three-site pilot establishes a workflow, not tracker accuracy.

- **Status:** Increments A–D delivered (#656): authoring, Compare, frozen-split physical pins with `verify-bundle`, and a native Freeze action previewed through the service. The evaluator refuses drifted reviews, intensity presence is per frame, and the length handle, following instants, elevations in Compare and the proposal population are in. Not yet exercised by the operator pilot. E not started.
- **Target:** Native macOS annotation pilot, followed by complete P1 and the P2 inspector
- **Layers:** L10 Clients, annotation storage, and offline evaluation
- **Canonical:** [Point annotation tool](../lidar/operations/point-annotation-tool.md)
- **Related:** [Physical reference review](lidar-physical-reference-review-plan.md), [MVP sprint](lidar-052-mvp-sprint-plan.md), [Annotation datasets](lidar-point-annotation-and-object-dataset-plan.md)

## 1. Outcome and working boundary

An operator should be able to select an already identified car, enter persistent dimensions,
place its physical centre or a supported face at a keyframe, set its body axis, declare bounds
and evidence, and save the result. A separate review approves the saved body or keyframe. An
inspection mode then shows the reviewed reference beside a named estimate at the same instant.

The user confirmed **macOS Swift**. The earlier mention of Svelte was a transcription error.
Svelte remains the existing segment-selection surface; this work does not add a second authoring
application there. The native editing views use SwiftUI Canvas and `OrthoViewport`; the adjacent
3D view uses Metal. Reuse that split so drawing and interaction share one projection.

This document began in the repository root and now lives in `docs/plans`. It owns the Swift
execution sequence. The [recording-domain extension plan](lidar-vrlog-observation-format-plan.md#73-persistent-features-and-revisioned-shape-beliefs-proposed)
owns the proposed shared feature contract; Swift and backend experiments consume that one model.
The physical-reference plan continues to own independent body/pose evidence.

Continue in the root checkout on `dd/lidar/physical-pose-ui-929`, now inspected at `ddea4159d`.
The original planning baseline was `0b5406770`; subsequent commits delivered the pose workflow.
Use this checkout directly and leave the preserved BuildInfo stash untouched. This update is
now accompanied by a user-authorised, approximately two-hour local implementation sprint.
Do not commit, push, merge, move checkouts, or change the stash as part of that sprint.

## 2. What exists and what remains

The committed baseline was inspected on September 30, 2026 at `ddea4159d`. The local feature
sprint adds the bounded proposal path described in section 11. Desktop/unit evidence is separate
from the still outstanding operator pilot, multi-site accuracy and device qualification.

| Area               | Current branch evidence                                                                          | Remaining prerequisite                                                                                |
| ------------------ | ------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------- |
| Membership         | Local pack/sidecar, selections, uncertain points, identity and review                            | Preserve all existing labels and exact point ordering                                                 |
| Physical authoring | `PhysicalReferenceSession`, controls, overlays, and Go physical-reference API exist              | Run the actual pack save/reopen smoke test; feature anchors remain unresolved                         |
| Compare/intensity  | Report inspector, intensity display/distribution, per-frame intensity availability               | Operator validation; retain the explicit zero-versus-missing requirement below                        |
| Freeze             | Native Freeze sheet, physical pins, `verify-bundle`, and `BindPhysical` exist                    | Verify current source/revision behaviour on the pilot packs                                           |
| Feature candidates | Local `features.proto`, Go revision store/API, sphere authoring and translation proposal preview | Complete validation and operator smoke test; body anchors and rotation registration remain follow-ons |

The earlier split-loader and physical-pin gaps were closed by `93a733d0d` and related commits.
Physical scoring now uses the shared split binding and physical pins. Sections describing those
increments below remain their design/acceptance criteria, not a claim that they are still absent.
The existing pose editor is therefore an available prerequisite on this branch, but its operator
pilot remains unverified. A checkout lacking those commits cannot deliver the full feature pilot
in three days without separately restoring/building that prerequisite.

Physical evaluation also deliberately refuses held-out splits. No persisted contract yet freezes
the physical error limits, reference precision, and coverage requirements learned on tuning data.
Preserve that refusal until the separate qualification work in section 10 is complete.

The facet research plan merged as `00d8b1ca3` (#657) on `origin/main`, after this branch's base.
Its document is brought into this checkout for the coordinated plan update, without merging code
or changing branches. Recheck integration conflicts before a later merge.

## 3. Three-site review scope

The pilot retains the three recordings selected for the September 30, 2026 review:

| Site               | Recording scope                         | Role     | Preparation still needed                                                    |
| ------------------ | --------------------------------------- | -------- | --------------------------------------------------------------------------- |
| Kirk Zero          | Whole capture, approximately 83 seconds | Tuning   | Resolve the exact capture and pack, verify duration and source identity     |
| Columbus–Broadway  | Approximately two minutes               | Tuning   | Select and record the exact window, pack, and frame range                   |
| Embarcadero–Folsom | Approximately two minutes               | Held out | Select and record the exact window without inspecting estimator performance |

The recordings total roughly 5.5 minutes. Recording duration is not an annotation budget. Reviewing
every car and every frame is a different task from checking a handful of informative keyframes.

### 3.1 Pack preparation

Before review, produce a small queue manifest or operator worksheet containing:

- Site and corpus case identity, source capture identity/digest, pack directory and pack digest.
- Exact start/end capture times, source frame ordinals, pack sample IDs, and selection rationale.
- Coverage domain, available point classes, sensor/calibration identity, and coordinate contract.
- Tuning or held-out role, assigned before looking at estimator results.
- Target objects, intended keyframes, review state, and reasons for omitted or unsupported facts.

Verify that each pack contains the intended interval. Check export sample caps and any settling
exclusion: selecting the whole Kirk Zero capture must not silently mean that only a capped subset
was exported. Record any unavailable beginning or end explicitly. Preserve capture timestamps,
sample ordering, and recording gaps; never renumber time to conceal missing samples.

Choose the Embarcadero window from traffic coverage or a predetermined/random interval. Do not
choose it from the candidate tracker's failures. All tuning, UI rehearsals against estimates,
and threshold decisions use Kirk Zero and Columbus. Independent human annotation of Embarcadero
is allowed, but its masks and references stay outside tracker inputs and parameter fitting.

### 3.2 Minimum pilot queue

For a first workflow trial, budget one focal car and approximately four to six informative
keyframes per site. Expand only after measuring the first object's review time. Choose frames
that expose changing visible faces, an oblique or partial end, a turn or lateral manoeuvre,
sparse support, and occlusion where those conditions actually occur.

These categories do not authorise replacing the selected sites. If the recordings lack a required
condition, record that coverage gap. The full acceptance suite can use synthetic fixtures for
software behaviour; those fixtures do not establish real-world applicability.

Keep a queue for point membership and identity beside the physical queue. Unreviewed masks remain
unreviewed; pose work must not make them appear complete. A keyframe references only its sample,
not the interval until the next keyframe.

## 4. Evidence rules the interface must preserve

The version 1 Go contract is the authority. Swift mirrors its representation and gives prompt
feedback, while Go decides whether a document can be committed.

| Fact              | Authoring rule                                                                     | Visible consequence                                                                     |
| ----------------- | ---------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------- |
| Object identity   | Reuse the sidecar's physical-object ID across the episode                          | A tracker ID or changing view never creates a new reference identity                    |
| Dimensions        | One persistent body belief with length, width, and height bounds                   | Changing face does not shrink the car; revising dimensions is an explicit body revision |
| Position          | Body centre or named supported face, with a horizontal bound                       | A mask centre is never silently labelled a physical centre                              |
| Face offset       | Optional distance and bound from face to centre, consistent with the body's bounds | A face without an offset locates only the face                                          |
| Height placement  | Optional position Z is unbounded in version 1                                      | Do not describe the reference as fully bounded in 3D                                    |
| Yaw               | Body-front direction in the pack frame; resolved, front/rear ambiguous, or unknown | An ambiguous axis has no signed front/rear claim                                        |
| Endpoints         | Derived from anchor, yaw, and length, with separate evidence support               | Endpoint coordinates cannot drift away from the body they describe                      |
| Support           | Per component: observed, inferred, prior-only, or unknown                          | Review status and evidence status are different controls                                |
| Partial dimension | A lower bound on a visible span                                                    | No invented upper bound or complete-box score                                           |
| Uncertainty       | Conservative bounds with stated assumptions and shared evidence                    | Reference bounds are not confidence scores or prediction sigma                          |
| Provenance        | Independent or tracker-assisted, with immutable origin history                     | A review click cannot turn a tracker proposal into independent truth                    |

Observed claims need supporting frames with the object's returns. An observed keyframe position,
axis, or bumper must cite its own sample. Inferred facts need named frames or external evidence;
class priors must name their source. Unknown components carry no value. The UI should explain a
refused claim in these terms, rather than invite the operator to increase a confidence slider.

Version 1 includes a 0.5 m evidence slack for applicable observed span/bumper checks. That is a
validation rule, not a measurement precision to prefill into uncertainty fields.

Drawing a box supplies a proposal. Its unseen surfaces become known only when evidence supports
them. Do not prefill a typical car's dimensions as observed or inferred. An optional later prior
picker must declare `prior_only` and remain excluded from independent physical scoring.

## 5. Operator workflow and Swift controls

### 5.1 Modes and layout

Keep the current Annotation window, object list, frame controls, and views. Add explicit modes:
**Points**, **Physical reference**, and **Compare**. Mode changes retain object/frame selection
but apply the appropriate draft guard. Point-selection gestures and pose gestures cannot both
consume the same drag.

In Physical reference mode, the editing column contains:

1. Body dimensions and their review state, labelled as applying to the whole object.
2. This keyframe's anchor, position, yaw, and endpoint support.
3. Evidence sources, uncertainty assumptions, and shared-error notes.
4. Save and separate review actions, revision status, and any stale-link problems.

The object column shows separate membership, body, and keyframe progress. A single green
"reviewed" badge would hide the distinction this feature exists to establish.

### 5.2 Persistent body editor

Each dimension has evidence status, full/partial span where applicable, lower/upper bounds, an
optional chosen value, and frame/external support. Show metres explicitly. Unknown clears the
stored value; partial span accepts only the lower bound. Validate finite, ordered bounds and
ensure an optional chosen value lies inside its interval.

Saving a changed body belief creates a new body ID and preserves the previous version in history.
There is one current body per object, not a different size for each frame. The editor must show
that change's episode-wide effect before commit.

Version 1 keyframes do not pin a separate body ID. For the first UI, conservatively reset review
on the object's dependent keyframes and related following records when body geometry changes.
Use an explicit dependency policy in the Go editing service, tested against the affected records.
Do not claim that P0 already performs this review invalidation automatically. A later optimisation
may preserve demonstrably unaffected reviews with evidence and tests.

### 5.3 Keyframe pose editor

Create a proposed keyframe at the current pack sample. Display source ordinal and capture time,
while persisting the exact pack sample ID and integer nanosecond timestamp. Duplicate capture
timestamps must remain distinct samples.

The minimum controls are click-to-place position, numeric X/Y, horizontal bound, yaw and yaw
bound, axis state, and optional Z. Display degrees to the operator; convert to radians at the
model boundary. Use the pack's coordinate convention and handle angle wrap explicitly. View or
grid rotation must not rotate the stored physical yaw.

Draw the available centre/face marker, axis, body outline, and supported ends in both orthographic
views. Draw only geometry that can be derived. A position with unknown length can still be useful;
it does not need a plausible rectangle to fill the screen. Optional elevation placement must not
acquire an implicit ground plane or a vertical uncertainty claim.

Named-face selection requires the axis state the Go contract permits. An offset can locate the
body centre only when its direction and bound are supported. The editor should expose that
distinction rather than translating every selected face to an assumed centre.

### 5.4 Endpoint interaction

Front and rear controls set evidence and support. Their positions are derived, never independently
stored XY values. In the first pilot, numeric body/anchor/yaw changes update endpoint overlays.

Later drag handles must name their constraint: translate the body while holding size, or revise
the persistent length while holding a selected face. Resizing length invokes the body review
invalidation above. An unresolved front/rear axis shows unsigned ends, where derivable, and
disables signed face/bumper claims. The nearer end is not necessarily the front.

### 5.5 Save and review

The visible sequence is **Edit → Save proposal → Inspect in both views → Review saved record**.
Review is permitted for honest unknown/partial records; reviewed does not mean every component
is scorable. The operator must supply author, method, supporting evidence, and uncertainty
assumptions for bounded facts. Retain provenance for authorship and later review using the
existing record/history mechanisms; do not silently erase the original author.

Provide separate Review body and Review keyframe actions. Neither changes mask review. Reviewing
a mask does not review physical geometry. Do not add a blanket "review all physical frames"
button for the pilot.

Any material edit to a reviewed record, including bounds, support, or anchor semantics, returns
it to proposed. A body change also invalidates dependent reviews. Copies to other frames retain
their actual origin and become proposals with corrected target sample/time. New observed claims
must supply support at the target instant. The pilot does not interpolate reference truth.

### 5.6 Undo, navigation, and failures

Maintain separate membership and physical dirty states. Undo/redo during editing operates on the
physical draft without changing saved history. Restoring saved history is a new revision through
the existing Go restore protocol; it never overwrites an old revision or clears assisted origin.
History browsing and restore controls may follow the pilot, but unsaved undo is required.

Guard frame/object changes, pack replacement, window closure, and playback-driven navigation.
Offer Save, Keep editing, or Discard for a dirty draft. If a main-view seek would move a dirty
annotation session, hold the annotation frame and show the sync pause rather than dropping work.

Save/review errors leave the draft intact. A conflict offers reload and deliberate reconciliation,
not an automatic last-writer-wins retry. A connection failure cannot display "Saved". A response
lost after a possible commit triggers a read/reconciliation before another write.

### 5.7 Blind authoring and comparison exposure

Physical authoring defaults to point evidence and independently supplied measurements. Suppress
tracker boxes, trails, and estimate readouts in the authoring views and any linked main view that
could expose the same object. If that suppression cannot be guaranteed, pause the link and make
the required blind-view arrangement explicit.

Compare mode is read-only. Viewing an estimate does not rewrite a previously saved independent
reference, but editing it against that estimate creates a tracker-assisted proposal with the
estimate identity. Returning to authoring must not clear the exposure state. Reject an attempt to
claim independent origin for such an edit. Preserve the original independent revision.

The application can enforce its own transitions, not erase what a person has seen elsewhere.
Independent provenance remains an operator declaration backed by the workflow. Embarcadero's
pilot queue disables estimate comparison until the held-out evaluation contract permits it.

### 5.8 Reflectivity inspection: raw intensity and experimental distributions

Add reflectivity inspection to the native annotation UI. Label the measured channel **Intensity
(raw 0–255)**: a sensor's intensity code is not a calibrated material reflectance measurement.
This feature helps the operator inspect returns without changing masks, identity, or physical
review. Derive its statistics from existing raw points for the pilot; do not persist descriptors.

**Per-point readout.** Hover or select a point to display its exact integer intensity, pack sample,
and point index beside its position. Use the same projection and depth/visibility rules as the
annotation view. For overlapping returns, identify which point was picked and provide a way to
select another; never report the mean of nearby points as the hovered point's measurement. A
selected point keeps a readable inspector value when the pointer moves away.

**Explicit colour toggle.** Add an on/off **Reflectivity colour** control, off by default. Off
preserves the normal annotation/object palette with no intensity-driven colour or brightness.
On gives **measured zero its own fixed colour**, maps **1–255** through the adjustable spectrum,
and uses a third unavailable style when intensity was not measured. Keep this independent of the
Object Points/Feature Candidates/Physical/Compare workflow modes and of histogram visibility. Preserve object
selection and review cues through outlines, labels, or other non-colour marks.

The current [Metal shader](../../tools/visualiser-macos/VelocityVisualiser/Rendering/Shaders/PointCloud.metal)
uses intensity for subtle foreground, ground, and background colour modulation, while annotation
palette entries ignore it. Explicit off therefore needs a defined renderer path that bypasses
that modulation for the annotation views, not an assumption that it was absent. Scope the new
toggle to annotation views, including their companion 3D view; do not change unrelated live-view
defaults. SwiftUI Canvas and Metal must honour the same mode, ramp, and missing-value behaviour.

**Ramp controls and legend.** Start the gradient at 1 and end at 255; zero is a separate swatch. Provide lower/upper raw-code limits,
a ramp/palette selector, a contrast or gamma control, and brightness control for the dark
background, plus Reset to the full 1–255 gradient/default ramp. Keep numeric bounds ordered and in range.
Explore a bright, starburst-like spectrum as a design direction, not a prescribed palette or
physical lighting effect. Check visibility of low values and selection marks on the actual dark
background. Do not silently rescale between frames.

The legend shows the active ramp, raw-code endpoints, intermediate labels reflecting any nonlinear
mapping, and out-of-range clipping. Clamp display colours at the selected limits and identify
clipped nonzero values; retain their exact raw readout. Zero must not be folded into the lower
gradient endpoint or reported as a clipped nonzero measurement. Range, gamma, and brightness are display transforms
only: they do not change histogram populations, bin edges, peak calculations, source bytes, or
review state. Missing intensity uses a distinct neutral style and an unavailable label, separate
from the valid colour assigned to zero. Toggle off restores the normal palette exactly; remembered
ramp settings may be reused on the next toggle on.

**Histogram scope.** Show a configurable 16- or 32-bin histogram for the selected object's returns
at the current sample, or an explicitly selected cluster/point set. Name the population and its
provenance: saved object mask, unsaved selection, or algorithmic cluster proposal. Retained cluster
boxes alone do not establish point membership. If exact members are unavailable, disable the
cluster summary or explicitly offer a labelled box-containment subset; never call that exact
cluster membership. Default to the selected saved mask's definite members, excluding uncertain
points, and disclose the exclusions.

Use fixed bin edges over the 256 integer codes: 16 bins of width 16, or 32 bins of width 8. The
last bin includes 255. Show count and fraction, measured-point count, excluded/unavailable count,
sample identity, and the selected bin setting. An empty selection differs from a selection whose
intensity is unavailable. Do not silently pool frames or weight repeated returns as independent
evidence. Camera clipping should not change the selected mask's histogram; any explicit
visible-only subset needs its own label and denominator.

**Up to four peaks.** Add a plainly marked Experimental summary with zero to four derived peaks.
For each, show centre in raw intensity units, width, fraction of all measured selected points,
and integer support count. Use a deterministic, tested extraction rule, not an unexplained fitted
mixture: identify histogram local maxima, collapse plateaux consistently, partition supported
basins at valleys, and rank by support with a deterministic tie rule. Compute each reported
centre and width from its assigned raw values; define width as population standard deviation in
raw codes, not a confidence interval or physical uncertainty. Zero width is valid for identical
codes. Expose the minimum-support rule and algorithm version; fix defaults using tuning data.

The pilot implementation must settle and test edge/plateau/valley rules before presenting the
summary. Do not force four peaks, or claim a binning-independent model. Show remaining/unassigned
support when fewer than all basins are reported; fractions use the full measured population so
the four displayed fractions need not sum to one. Let the operator compare 16 versus 32 bins
without implying that a peak is a particular vehicle surface or material.

**Availability and interpretation.** Use explicit source/pack presence, never the numeric value,
to decide availability. With known presence, intensity 0 is measured data and contributes to the
first bin. When absent, show Unavailable and disable intensity-derived summaries rather than
producing a zero-valued histogram. Malformed/out-of-range source arrays are a data error, not
values to truncate or wrap into `UInt8`.

Disclose foreground-only or decimated coverage and any selected subset next to the histogram.
The distribution describes retained selected returns, not every surface of the vehicle. Show the
sensor identity, known native-to-stored mapping, and calibration status. If mapping or calibration
is unspecified, say so; do not invent a response curve or compare codes across sensors as equal
reflectance. Range, incidence, and sensor response can change intensity without a material change.

**Evidence boundary.** Histogram/peak settings are inspection state. Hovering, changing colour or
bin count, and reading peaks do not save annotations, mark a pose reviewed, alter geometry, or
promote evidence status. A statistic from an estimator-selected cluster retains that source; it
cannot become independent physical evidence through a review action. Blind authoring may inspect
raw returns or the independent membership mask. Estimator-derived selections follow the existing
comparison exposure rules and stay out of Embarcadero's blind pilot workflow.

### 5.9 Existing intensity storage and the VRLOG boundary

**No VRLOG format change is required to display existing raw intensity.** Source inspection finds:

| Existing layer                 | Intensity representation                                                                                                                                                                                                                                           | Work for this feature                                                                    |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------- |
| Legacy FrameBundle point cloud | [PointCloudFrame](../../proto/velocity_visualiser/v1/visualiser.proto) has packed repeated `uint32 intensity`, documented as 0–255                                                                                                                                 | Preserve point indexing, presence, and the native integer values                         |
| VRLOG 1.x observations         | [RetainedPoints](../../proto/velocity_recording/v1/recording.proto) has one intensity byte per point and explicit column presence                                                                                                                                  | Respect presence and retained-point domain                                               |
| Annotation packs               | [Manifest and point blocks](../../internal/lidar/annotation/pack.go) carry `has_intensity` and byte intensity alongside coordinates                                                                                                                                | Read availability and expose the stored byte without renormalising it                    |
| Native client                  | [Swift point models](../../tools/visualiser-macos/VelocityVisualiser/Models/Models.swift) and pack reader ingest bytes; [MetalRenderer](../../tools/visualiser-macos/VelocityVisualiser/Rendering/MetalRenderer.swift) uses intensity divided by 255 for rendering | Keep the raw integer for readout/statistics; rendering normalisation must not replace it |

There is a presence caveat in the existing annotation export: `has_intensity` becomes true if any
exported sample supplies intensity, while the fixed-size point encoding can zero-fill absent
values. A pack-wide true flag alone does not prove every sample has measurements. Verify the
pilot sources have complete intensity arrays at each inspected sample. If presence is mixed or
cannot be established, use source-derived per-sample availability in memory or mark affected
samples unavailable; do not infer presence from zero or nonzero bytes. Handle that case before
calling the absent/zero acceptance check complete.

This caveat concerns annotation export/availability, not a need to add raw intensity to VRLOG.
Durable per-sample availability for mixed packs may require a separate annotation-pack metadata
extension; do not quietly widen the pose reference schema to solve it. Persisting derived
histograms/peaks or additional sensor-response metadata would likewise need a separate contract
decision about ownership, provenance, versioning, and reproducibility. None is required for this
pilot's on-demand summaries of verifiably present raw values.

## 6. Swift architecture and integration seams

Keep persistence and evidence rules out of SwiftUI view bodies. Use a dedicated physical session
associated with the active annotation pack/object/sample, with observable UI state on the main
actor and file/network work off it.

| Existing seam                                                                                                                | Proposed change                                                                                                  |
| ---------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| [AnnotationSession.swift](../../tools/visualiser-macos/VelocityVisualiser/Annotation/AnnotationSession.swift)                | Coordinate active object/sample, membership save notifications, navigation guards, and physical-session lifetime |
| [AnnotationSidecar.swift](../../tools/visualiser-macos/VelocityVisualiser/Annotation/AnnotationSidecar.swift)                | Keep membership ownership and file format; never repurpose optional `AnnotationPose` as the new reference        |
| [AnnotationWindow.swift](../../tools/visualiser-macos/VelocityVisualiser/UI/AnnotationWindow.swift)                          | Compose modes, retain drafts during navigation, and apply blind-view rules                                       |
| [AnnotationPane.swift](../../tools/visualiser-macos/VelocityVisualiser/UI/AnnotationPane.swift)                              | Add physical controls and separate progress/review actions, preferably in extracted views                        |
| [AnnotationViewports.swift](../../tools/visualiser-macos/VelocityVisualiser/UI/AnnotationViewports.swift)                    | Draw reference/proposal/comparison layers using the existing coordinate mapping                                  |
| [OrthoViewport.swift](../../tools/visualiser-macos/VelocityVisualiser/Annotation/OrthoViewport.swift)                        | Reuse projection for hit testing and drawing; test rotation, zoom, pan, and elevation                            |
| [AnnotationFrameSync.swift](../../tools/visualiser-macos/VelocityVisualiser/Annotation/AnnotationFrameSync.swift)            | Hold dirty drafts and reject mismatched frame updates                                                            |
| [PackFolderAccess.swift](../../tools/visualiser-macos/VelocityVisualiser/Annotation/PackFolderAccess.swift)                  | Preserve user-granted local pack access; do not assume a remote server path is locally readable                  |
| [AnnotationExportAPIClient.swift](../../tools/visualiser-macos/VelocityVisualiser/Labelling/AnnotationExportAPIClient.swift) | Follow its local API/error conventions in a separate physical-reference client                                   |

Proposed new responsibilities, with final filenames chosen during implementation:

- `PhysicalReferenceModels`: explicit Codable mappings, enums, and immutable revision identity.
- `PhysicalReferenceSession`: draft, undo, selected record, review/exposure state, and validation.
- `PhysicalReferenceAPIClient`: load, validation, save, review, and structured failures.
- `PhysicalReferenceGeometry`: pure display projection/derivation with Go parity fixtures.
- `PhysicalReferencePane` and overlay views: controls, uncertainty, and evidence labels.
- `PhysicalComparisonReport` and inspector state: decode and index Go output, select estimate arm,
  and bind the report to its reference revision and pack.
- `IntensityInspectionState` and pure distribution helpers: resolve the selected population and
  explicit availability, preserve raw-value hover/selection, and compute bins and peak summaries.
  Keep this display state separate from physical drafts/review. Cache by pack/sample, selected
  point indices, presence, bin count, and algorithm settings; discard stale asynchronous results.
- A shared intensity display specification for Canvas and Metal: explicit on/off state, raw-code
  range, ramp, contrast/gamma, brightness, legend, clipping, and missing-value style. Do not reuse
  object-classification palette indices as synthetic measured intensity.

Do not recompute scoring, matching, or statistical outcomes in Swift. Limited geometry needed for
an immediate draft preview must agree with Go through shared fixtures. Preserve integer capture
timestamps and explicit coding keys; do not round nanoseconds through floating-point JSON helpers.
Unsupported schema versions open read-only or fail clearly, never decode and save a lossy subset.

Use distinct colour and line style for proposed reference, reviewed reference, and prediction.
Label layers directly; colour alone is insufficient. Numeric controls, focus order, and keyboard
navigation must provide a usable alternative to dragging. Loading/validation should not block
the view thread. Measure interaction responsiveness on the pilot machine before setting a limit.

## 7. Go integration: one authoritative writer

Use a thin local HTTP API around the existing annotation package. This is the recommended first
implementation because a second Swift writer would need to reproduce evidence validation,
origin history, canonicalisation, and concurrency semantics. No database migration or live tracker
change is needed for the authoring contract.

The pilot requires the local Go service to access the same pack as Swift. Packs opened outside
its configured annotation root need deliberate local registration or relocation before physical
saving. The first milestone must test this arrangement with a real pilot pack. A disconnected
native writer, bundled helper, and remote pack transfer are later alternatives, not hidden
dependencies of the initial UI.

### 7.1 Proposed service operations

Endpoint spelling is an implementation detail; these behaviours are required. Resolve an opaque
pack handle beneath the configured root and verify its dataset/pack identity. Do not accept an
arbitrary client filesystem path as write authority. Follow the server's existing access controls.

| Operation           | Request/response contract                                                                                                                                                                       |
| ------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Load                | Return current or requested physical revision, exact-byte and content digests, sidecar revision/digest, source identity, and stale-record diagnostics; represent an empty set distinctly        |
| Validate draft      | Apply the same proposed edit/dependency rules and validation as save, without writing; return record/component diagnostics and affected reviews                                                 |
| Save edit           | Require base physical revision/digest and membership revision/digest, author/session, and a document edit; canonicalise, reset affected reviews, validate links/evidence, and commit through P0 |
| Review saved record | Name body/keyframe and the exact saved revision; require no unsaved replacement payload; record reviewer provenance and commit a new revision                                                   |
| Restore, later UI   | Restore a retained document through P0 as a new revision, retaining origin history and validating current links                                                                                 |

The Go store's concurrency token is private. Do not trust a Swift-supplied token as a replacement
for loading the document. Load the current Go document, compare the client's expected identity,
apply the edit, and retain the store's own token so a racing physical save is still rejected.
Check the expected membership revision/digest under the same annotation lock as the final write;
a check only in the HTTP handler would leave a race. This requires a small store/service extension.

Retain the shared `.annotations.lock`, exact-byte archives, atomic replacement, and origin ledger.
Reject unknown fields, malformed/trailing data, non-finite values, excessive request sizes, wrong
sources, stale tokens, and invalid evidence. Return structured conflicts/busy/validation errors
that the client can explain without parsing prose. Never silently repair evidence by weakening
or strengthening its declared status.

### 7.2 Membership changes and dependent references

Membership saves continue to work independently. Do not claim a global transaction across the
Swift sidecar writer and the physical API. Existing P0 loads report stale links, and physical
saves refuse unresolved invalid records.

After a membership save, merge, split, rejection, or reload, refresh physical-link diagnostics.
An object merge must not silently merge physical bodies or transfer support to another identity.
Show records needing repair and require deliberate reassignment/review through Go. Also refresh
on focus/reload and before save to detect writes by another process.

If points change while a link remains structurally valid, the previous physical review may still
have relied on the old returns. Track the membership revision used by the editing/review session;
require revalidation and renewed review for affected support. Define the durable provenance needed
for this in the service contract. Do not mistake P0's invalid-link check for proof that all valid
links still support the same human judgement.

## 8. Inspector: reuse the evaluator's per-instant record

Start with opening an existing Go per-frame evaluation JSON report. Live evaluation scheduling
and SQL browsing are unnecessary for the pilot. Decode the outer comparison envelope and its
physical section; do not assume the nested physical result is a standalone report file.

Index records by pack, episode, object, sample, exact capture time, reference revision/digest, and
estimate identity. Read matching decisions from Go. The current scorer uses a one-to-one nearest
match within its gate at each sample; it does not prove a persistent identity association. Show
the matched track and ambiguity/missing reasons so a nearby car cannot hide behind an aggregate.

The inspector displays:

- Reference position/anchor, axis, dimensions, supported ends, bounds, and review/provenance.
- Prediction geometry and its declared reference point, estimate version, stage, and uncertainty.
- Go's component errors, reference bounds, and unavailable/suppression reasons.
- Counts of expected, scored, unknown, unmatched, and missing-prediction instants.
- Available consecutive-centre comparisons, with actual reference motion alongside prediction
  motion so a real turn or lane change is not presented as unwanted jitter.

The report's scored geometry is not a complete authoring record. Read evidence sources and review
details from the matching retained physical revision; verify its digest first. If that revision
is unavailable, keep the report usable for its recorded comparison and label provenance details
unavailable. Never fill them from the current head by assumption.

A medoid or visible OBB centre remains labelled as such. Its distance from a body centre is a
diagnostic, not a scored physical-centre prediction. A complete box is shown/scored only where
supported. A front/rear ambiguity allows axial or unsigned-end comparisons, not signed errors.

On pack/frame/object/report/arm/stage changes, clear the previous overlay immediately and cancel
or invalidate pending loads. Use a request-generation identity so a late response cannot repopulate
the wrong view. Preserve the displayed report revision even if the authoring head advances; label
the difference and offer an explicit new evaluation. Never splice new references into old errors.

## 9. Freeze and reproducibility

### 9.1 Delivered compatibility prerequisite

The branch now routes physical scoring through the shared validated split binding and
`BindPhysical`. Retain regression coverage for legacy version 1 and frozen membership inputs,
exact per-pack pins, history after head changes, and source drift. The earlier split-loader
mismatch is resolved in source; actual pilot-pack reproducibility still needs its own evidence.

### 9.2 Supervised pilot freeze

Once selected membership is reviewed, freeze its split using the existing CLI. Select an explicit
physical revision for each pack, validate it against the pinned membership, and retain a pilot
verification bundle. Run each pack with its own explicit physical revision; one revision number
does not identify the same content across different packs.

The bundle includes exact physical-reference bytes, revision and content digests, frozen split
bytes/digest, source/manifest/selection identities, estimate/config/build identities, the command
and scoring options, and the JSON/Markdown outputs. Archive a current physical head into the bundle
even if the store has not yet moved it into history. A repeat run must verify all pins and refuse
any changed input. Supply a small verification runner if the product freeze extension is deferred;
an unchecked list of hashes is not enforcement.

The current native Freeze path and versioned split can retain physical pins. The bundle remains
useful audit evidence alongside that product path. Do not infer successful pilot reproduction
merely from the existence of the controls.

### 9.3 Complete freeze integration

Extend the frozen-split draft and output with per-pack physical revision, exact-byte digest,
content digest, and separate physical-review/coverage summaries. Use an explicit versioned format
change; retain readers for supported legacy inputs and test their behaviour. Do not reinterpret
the existing optional-pose geometry summary as physical review.

Bind validates the physical document against the pinned sidecar, rather than substituting the
current membership head. Subsequent membership changes must not invalidate a correctly retained
historical evaluation merely because the current editing state is different. Tampered retained
bytes, wrong source identity, or unsupported historical links must refuse the run.

The evaluator reads physical pins from the frozen split and refuses a conflicting explicit
revision flag. Pin review outcomes and missing-evidence populations; freezing honest unknowns is
allowed, while a qualification gate may reject insufficient coverage. Saving a new reference
creates a new revision; using it for evaluation requires a new freeze and result.

Preserve object/capture disjointness and the tuning lineage across successor splits. Excluding an
old tuning pack from a later file must never make its objects unseen again. Keep tracking seeds
separate from evaluation-only keyframes; later masks/poses must not reach unassisted tracking.

Add a native Freeze action after the backend contract and CLI tests work. It shows what will be
pinned, separate review coverage, unresolved evidence, and the output identity. A UI button must
not be the only place these constraints are enforced.

## 10. Held-out qualification and following remain explicit follow-on work

Embarcadero–Folsom stays held out. The pilot can author and freeze its references blindly. It must
not score them by temporarily changing their role to tuning or bypassing `ErrPhysicalHeldOut`.

Before allowing held-out physical evaluation, persist and freeze the tuning-derived acceptance
contract: component error limits, maximum reference uncertainty, required coverage and encounter
counts, strata, missing-opportunity limits, estimator/config version, and scoring rules. Decide
these using Kirk Zero and Columbus. Add backend checks and tests before opening Embarcadero
results.

A later following editor must author leader/no-leader/ambiguous decisions over explicit intervals
and gap bounds only at supported instants. Preserve shared endpoint errors, leader changes,
unresolved ordering, standstill, and gaps in recording. The current gap definition is a straight
chord projected along the follower axis. It is not the along-path headway arc on a bend.

P2 does not yet score leader-choice correctness, and along-path gaps need persisted paths. Neither
capability is implied by drawing bumpers. Full P3 requires independent pair/manoeuvre references
and a broader reviewed queue; the three-clip authoring pilot alone does not close it.

## 11. Next three engineering days: Swift feature authoring

This is an annotation and inspection slice over the existing physical editor. It is separate from
F0–F5 in the [facet research plan](lidar-facet-registration-experiment-plan.md#6-work-packages-and-effort).
That plan budgets **15–25 engineer-days** to a research decision and **35–60 engineer-days** through
conditional integration and robustness. Its upper bound is 60 engineer-days; it is not a claim
that basic manual feature authoring needs three months. This three-day slice does not close H1–H5.

### 11.1 Two-hour local sprint: a bounded proposal loop

The authorised sprint prioritises a usable, reviewable path:

1. Save whole-object membership in **Object Points** mode. Switch visibly to **Feature Candidates**;
   the sphere now selects a subset of the saved, definite active-object domain. Other-object and
   uncertain claims are excluded. A feature gesture cannot add, subtract, or review object points.
2. Seed an adjustable sphere at a real return. Default **radius is 0.20 m**, diameter 0.40 m;
   the controls label both. Inspect orange draft support in top and elevation views. Store a
   persistent feature ID, distinct from the fresh per-sample return indices.
3. Enter a geometric type (unknown, edge, corner, patch, protrusion) and an optional semantic hint.
   A corner is geometry; “headlight” is a tentative meaning. Overlapping features are permitted.
   These labels do not run a detector or validate a geometric template.
4. Accept and save, reject, mark missing/occluded, or cancel. Acceptance saves a **proposal**,
   not independent physical truth. Go validates the shared protobuf under the annotation lock,
   checks feature and membership tokens, archives exact prior bytes, and atomically replaces the
   feature head. Whole-object membership and physical review remain unchanged.
5. Reopen the same local pack. The service must identify the same folder, not merely a copy with
   matching bytes. Without the local service, a saved feature file can be inspected read-only.
   This workflow needs no Internet service, but authoritative writes require the local Go process.
6. Preview the next consecutive source frame. Use whole-object centroid translation and a bounded
   local footprint search, constrained to that same object's saved target domain. Recompute fresh
   target indices. Preview does not write; each frame requires an explicit decision. Stop on gaps,
   missing/sparse support, implausible movement, competing fits, changed membership, an already
   annotated next frame, or a navigation guard. Cancel stops the sequence without a write.

The current footprint has a 0.25 m voxel pitch and searches horizontal residual translation
around the three-axis centroid displacement. That is a coarse proposal aid for a 0.20 m sphere,
not a full local 6DOF registration result or a claim that the same physical surface was measured.
Partial visibility can move the object's observed centroid. The operator must correct or reject
such a preview. No automatic mask propagation is invoked and no frame is silently skipped.

The first shared document is `velocity.report/feature-proposals` version 1, stored as
`feature-proposals.pb`, with exact-byte SHA-256 tokens and `feature-proposal-revisions` history.
The pilot explicitly uses `legacy_pack` point identity; it invents no retained 1.x point IDs.
Part `body` has an **unknown** relation and the metric anchor remains absent. The reserved anchor
message is refused by the pilot writer until transform and physical-revision validation exist.
Provenance distinguishes human proposals from assisted proposals; neither claims reviewed truth.

Save errors retain the draft. An uncertain commit forces deliberate cancel/reload before another
write, because a missing response is not evidence that the server did nothing. Historical support
keeps its original membership revision; changed observations require the current membership pin.

### 11.2 Three-day delivery sequence

| Day | Focus                                                                                                                                           | Concrete exit evidence                                                                                                                                                     |
| --- | ----------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | Complete the sprint loop, wire-contract fixtures, failure handling, visible mode separation, operator guide                                     | Seed/edit/save/reopen on a real tuning pack; cancelled and rejected proposals never modify masks; source and revision conflicts fail closed                                |
| 2   | Manual linking across samples, explicit stable body/part-frame anchor authoring under usable physical poses, uncertainty and stale-link display | Same persistent feature ID with fresh point support; known transform/revision pinned; unresolved axes or parts stay unresolved; extended bounds do not move metric anchors |
| 3   | Inspection polish, three-site queue, reproducible annotation export and backend consumption fixture                                             | Export pins source maps, membership, physical and feature revisions; another reader interprets the same records; operator records failures, missing intervals and timings  |

One implementer and existing physical authoring are assumed. The three days are a planning
allowance, conditional on usable packs and body poses, not a guaranteed deadline. Do not spend
them on a second tracker, automatic semantic detection, general articulation, a mesh, or model
classification. If body poses are unavailable, ship observed feature support with unresolved
anchors and name the missing dependency. Preserve the current held-out scoring refusal.

### 11.3 Eight-hour follow-on: matching and body anchoring

“Identification” here means following an operator-seeded persistent feature across adjacent
frames, not identifying a vehicle make/model or recognising arbitrary mirrors. Local registration
and placement along the body are distinct tasks. A useful eight-hour attempt can cover a narrow
rotation-aware matching experiment **and** a body-anchor inspection path if the current proposal
loop is validated and suitable physical body/keyframe poses already exist. It cannot promise
production six-degree-of-freedom tracking for arbitrary geometry.

| Allowance      | Work and dependency                                                                                                                         | Acceptance or stop condition                                                                                                                                          |
| -------------- | ------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1 hour         | Pin examples, coordinate conventions, source/target domains and current baseline; agree a observable corner/patch example                   | Known translation and three-axis rotation fixtures; clear no-match and ambiguous examples                                                                             |
| 2–3 hours      | Bounded local rigid-registration proposal, initialised by existing motion; full three-axis rotation and translation where data support them | Recover declared fixture transforms within separate positional/angular tolerances; show residual and support; retain alternatives and refuse underconstrained results |
| 1–2 hours      | Feature-to-body anchor controls and readouts under a pinned usable body/pose revision                                                       | Longitudinal/lateral/height metric coordinates; optional derived percentages; unknown front/rear explicit; changing dimensions does not move stored anchors           |
| 1–2 hours      | Review UI, serialisation and failure fixtures, operator smoke test on tuning examples                                                       | Accept/edit/reject each frame, save/reopen consistency, no mask edits, honest occlusion and unresolved anchors                                                        |
| Remaining time | Fix findings and record evidence                                                                                                            | Reduce the delivered slice rather than remove rejection or persistence checks                                                                                         |

These ranges consume roughly 5–8 hours before unexpected repairs. If the pose model cannot
provide the required orientation or stable part frame, the anchor work needs additional design;
ship a labelled preview or unresolved anchor, not guessed coordinates. A directed longitudinal
axis is needed to say “from the front”; an undirected axis can only report an explicit axis-relative
coordinate. Height needs a supported vertical origin. Full 6DOF fitting needs non-degenerate
geometry: an edge can slide or rotate, a plane leaves tangent motion unresolved, and a sparse
corner may lack enough support. Report data observability separately from motion-prior prediction.

Potential presets are later proposals: box-truck right-angle edge, approximately 90/90/90
three-plane corner, headlight corner, mirror protrusion/concavity, and wheel-well curved recess.
Current controls provide only the five geometric labels and free-text semantic hint. Define
size tolerance (possibly an initial ±10%), angular tolerance in degrees, fit residual in metres,
minimum support and alternative-match separation independently. A universal “10% match” has no
meaning across angles, size, visibility and sensor noise; translation/rotation invariance does
not establish semantic identity. Curved or concave examples may require a different model.

### 11.4 Operator and research time remain separate

Allow initially **2–3 operator hours** for a small focal-car/keyframe queue whose membership is
mostly reviewed, then re-estimate after the first car. Full membership, identity, pose and feature
annotation across the three clips can take substantially longer. Kirk Zero is the approximately
83-second tuning clip; Columbus–Broadway and Embarcadero–Folsom still require exact approximately
two-minute window/pack selection. Embarcadero remains blind held-out evidence. Feature proposals
are not frozen independent references merely because they can be reopened.

## 12. Acceptance criteria and verification

### 12.1 Automated contract and regression tests

| Area                    | Required cases                                                                                                                                                                                                                      |
| ----------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Cross-language contract | Go fixtures decode in Swift; Swift edits pass Go validation; explicit coding keys; optional/unknown values; unsupported versions; exact Int64 timestamps; repeated timestamps with distinct samples                                 |
| Geometry                | Metres and degrees/radians; angle wrap; ambiguous axis; face offsets; partial/unknown dimensions; optional unbounded Z; conservative endpoint bounds; shared errors; Go/Swift preview parity                                        |
| Interaction             | Top/elevation projections under pan/zoom/grid rotation; mode-exclusive gestures; numeric input; undo/redo; object/frame/pack change; dirty close; playback seek while editing                                                       |
| Review                  | Saved proposal versus reviewed record; independent mask/body/keyframe review; edit resets review; body edits invalidate dependencies; copies remain proposals; unknowns may be reviewed without becoming scorable                   |
| Provenance              | Tracker-assisted copy/edit cannot become independent, including deletion/recreation and restore; compare-to-edit exposure persists; later evaluation references never reach tracker inputs                                          |
| Store/API               | Wrong source or pack; unknown/trailing fields; request limits; stale physical and membership tokens; lock contention; membership race; atomic-write failure; lost response; draft retained; exact history; damaged head/history     |
| Membership links        | Removed support, changed points, rejected object, merge/split, external writer, stale versus historically valid pinned membership; no automatic transfer of physical identity                                                       |
| Inspector               | Report envelope decoding; pack/revision/digest mismatch; unavailable historical source; arm/stage switch; duplicate timestamps; late response after seek; missing prediction; ambiguous axis; medoid semantics; complete accounting |
| Freeze                  | Legacy version 1 input; current frozen membership input with physical scoring; per-pack revision pins; tampered bytes; changed head with valid historical pin; conflicting override; successor tuning lineage; held-out refusal     |

Reuse the existing physical validation, geometry, review, store, and evaluator fixtures. Add tests
at new boundaries rather than copying the implementation into expectations. Exercise failures
and races in the Go service, and pure Swift session/projection behaviour without requiring a live
renderer for every assertion. Keep a small integration test that crosses the actual HTTP boundary.

Intensity-specific acceptance is required in addition to the pose matrix:

- Round-trip known raw values including 0, 1, 127, 128, 254, and 255 through recording/export/pack
  decoding. Hover and selected-point readout must equal the source integer exactly, including
  after colour normalisation, ramp edits, and view rotation. Refuse malformed legacy values
  outside 0–255 rather than wrapping them during conversion.
- Distinguish present all-zero intensity, absent intensity, zero selected points, and mixed
  per-sample presence. Valid zeros count in bin zero; absent or padded values do not. Exercise
  `has_intensity = false` with zero-filled storage and a mixed-presence export with the flag true.
- Toggle colour on/off repeatedly in Canvas and the annotation 3D view. Off must bypass existing
  intensity modulation and restore the object palette. On must match the legend for identical
  codes in both renderers. Zero needs its own invariant swatch, distinct from both code 1 and
  unavailable. Check the 1–255 default gradient, narrowed range, invalid limits, clipping,
  gamma/brightness, reset, missing style, and readable points on the dark background.
- Prove all display controls leave point bytes, membership, sidecar/physical revisions, draft
  dirtiness, and review states unchanged. Changing the display range must not filter histogram
  input or change peak results. Histogram/peak controls likewise never commit reference edits.
- Test 16/32 bins with values on every edge, especially 0 and 255; counts must sum to measured
  support and fractions use that same denominator. Test selection changes, uncertain-point
  exclusions, duplicate indices, missing exact cluster membership, and frame changes.
- Test peak extraction with empty/constant, one-, two-, and four-mode samples, more than four
  modes, plateaux, equal-support ties, sparse noise, and end bins. Assert deterministic centres,
  population-standard-deviation widths, fractions, support, and remaining/unassigned accounting.
- Verify asynchronous histogram results cannot reappear after changing pack/frame/selection or
  bin count. Show coverage, source population, sensor mapping, and calibration caveats. Confirm
  an estimator-derived selection remains assisted and is blocked in the blind held-out workflow.

### 12.2 Build and repository checks

- Run focused Go annotation/server/per-frame evaluator/CLI tests during implementation; include
  race tests for the shared-lock and request lifecycle changes.
- Run `make test-mac` and `make build-mac`, plus applicable Swift formatting/lint checks. Verify the
  newly built app, not a bundle left by an earlier run.
- Run the repository-required Go checks and `make test-go-changed-coverage` for new production Go
  files, with its real `pcap` configuration and required coverage threshold.
- Before an authorised commit, complete required repository lint/test gates and document any
  unrelated baseline/environment failures separately. Do not describe targeted tests as full CI.
- Mac build/test targets generate `App/BuildInfo.swift`. Inspect that diff explicitly; do not pop
  or alter the preserved stash, or sweep unrelated generated changes into the feature.

### 12.3 Manual acceptance on the actual clips

1. Open the verified Kirk Zero pack. Review membership/identity for the focal car, then author
   body bounds and several poses without seeing tracker output.
2. Save, close, reopen, and separately review body/keyframes in both views. Record elapsed time.
3. Change a reviewed bound and confirm dependent review is cleared; undo/discard an unsaved edit
   and prove existing saved data remain intact.
4. Exercise a partial or ambiguous case on Columbus. Preserve unknown evidence rather than
   forcing a complete box. Include a turn/lateral change if present.
5. Load tuning reports. Step rapidly, switch arms, and inspect unknown/missing outcomes. Confirm
   the displayed reference and estimate share their recorded instant and identity.
6. Freeze and rerun the tuning reference revision. Save a later edit, then prove the earlier
   result remains reproducible from its retained inputs.
7. Author Embarcadero references blind and preserve its held-out role. Confirm physical scoring
   refuses until the qualification contract is delivered.
8. Record per-site objects, reviewed keyframes, missing intervals, component support, unresolved
   identity/geometry, interventions, failures, and operator time. Do not substitute aggregate
   completeness for those counts.
9. Inspect known-intensity points and an absent-intensity fixture. Toggle reflectivity colour,
   adjust the ramp/range on the dark background, and verify the raw readout and unchanged object
   palette when off. For a focal car, compare 16/32-bin summaries and check that experimental
   peaks, support, and coverage labels are clear without implying physical reference truth.

## 13. Risks and decisions to settle during implementation

| Risk or decision                                                 | Required handling                                                                                     |
| ---------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| Exact Columbus/Embarcadero windows are unspecified               | Resolve them as a preparation task; retain selection rationale and roles                              |
| Pack is outside local server's writable root                     | Establish supported pack registration/location in increment A; do not add arbitrary-path writes       |
| Independent view exposes tracker output through main-window sync | Suppress it or pause the link before authoring; test the complete window arrangement                  |
| Body edits affect every pose under version 1                     | Use conservative dependent-review invalidation; defer selective preservation until proved             |
| Membership changes keep valid links but alter evidence           | Record/recheck support revision and require affected review; structural link validity is insufficient |
| Cross-language geometry differs                                  | Use shared fixtures; keep scoring in Go and compare preview derivations                               |
| Frozen physical evaluation is assumed to exist                   | Retain delivered split binding; verify current pins and pilot bundle on real inputs                   |
| One day is insufficient                                          | Deliver the proven increment, preserve the queue, and state the missing gate                          |
| Held-out pressure encourages a bypass                            | Retain refusal and report insufficient evidence until the tuning contract is frozen                   |

Documentation shipped with the implementation should update the canonical physical-reference
plan, the point-annotation operator guide, and the macOS component guide. Record completed work
and limitations in the backlog/devlog as appropriate, preserving historical entries. The local feature
slice must record automated and operator evidence separately; a passing build does not close it.

The authorised local sprint is bounded by section 11.1. The three-day and eight-hour extensions
remain estimates and scope recommendations. None establishes physical estimator accuracy, the
held-out following gate, multi-site transfer, or device qualification.
