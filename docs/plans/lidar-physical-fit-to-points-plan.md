# Physical references by fitting to reviewed points

Once an object's points are reviewed, fit its physical box to them instead of asking the
operator to type it. The operator keeps the judgements only a person can make: which returns
belong to the object, and whether an end the returns reach is the vehicle's real end or the edge
of something hiding it. The fit does the arithmetic, states every bound it uses, and writes
proposals that go through the same validation and review as anything typed by hand.

- **Status:** F1 and F2 built: the Go fit, its command, its endpoint, and the window's Fit section with the evidence pickers removed. F4 in part: the whole box is drawn in the 3D view and the elevations, and a fitted pose turns about its centre; the one frame label is open. F3 (errors at the field) and F5 open
- **Target:** v0.5.2, Sprint 0.5.2.0; the P3 operator pilot of the [physical reference plan](lidar-physical-reference-review-plan.md)
- **Layers:** L10 Clients, annotation and offline evaluation
- **Canonical:** [point annotation tool](../lidar/operations/point-annotation-tool.md)
- **Related:** [physical reference review](lidar-physical-reference-review-plan.md), [authoring comprehension design](lidar-physical-authoring-comprehension-design.md) (its authoring flow is replaced by this one), [F0 report](../lidar/operations/facet-f0-attribution-kirk0-report.md), [kirk0 pilot](../lidar/operations/physical-reference-pilot-kirk0-2026-10.md), [mounting](../../.github/knowledge/hardware.md#mounting)

## Why

The first operator attempt at a physical reference, on kirk0's truck 1 (`obj_35c661db`, sample 471) on 2026-10-08, stalled twice after following written steps. The first stop was an empty
"uncertainty assumptions" field in another section; the second was an empty `±` after a
position, a field that displays only "m". Between them the pane offers about thirty controls
before a box counts:

| Problem seen                     | What the operator met                                                                                                                                    |
| -------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Too many decisions               | Four evidence states, full or partial, value and `±`, exact min and max and cited frames per dimension; axis, five anchors, offset and two ends per pose |
| Bounds with no stated meaning    | Nothing says whether the box includes mirrors, whether a face is the outermost return or the surface, or what `±` must cover                             |
| Silent required fields           | A free-text field and a tiny `±` block saving; the error appears at the top and bottom of the pane, not at the field                                     |
| One name, two meanings           | "Height" is the body's height in section 1 and the anchor's vertical coordinate in section 2                                                             |
| The box is not where you look    | Nothing is drawn in the 3D view; the elevations show vertical lines until a height is known                                                              |
| Three frame numbers              | The window shows a source ordinal ("Frame 475"), the record and tools use the sample ("sample 471"), and the pack also stores a source frame ID          |
| Numbers the mask already implies | Every value starts unknown and is typed or dragged, although the reviewed mask fixes the outermost returns exactly                                       |

The record model is right: evidence status, conservative bounds, independence from the tracker,
a separate review. What is wrong is asking a person to compute numbers the reviewed points
already determine, in a form that does not say what they mean.

## The flow

1. **Review the points.** Unchanged. Fitting is offered only for a reviewed object whose every
   mask is reviewed, so a fit never rests on proposed membership.
2. **Fit the object.** One action. The fit measures the body's size from the frames that show
   it best and places a pose at each of those frames, because a size is checkable only at frames
   with a pose (an observed length is measured along a pose's heading).
3. **Fit a pose here.** At any other frame, place the fitted size on that frame's returns.
4. **Check the ends.** For each pose, the fit says which ends and sides it saw and why. The
   operator's one judgement per end is whether it is the real end or cut off by something in
   front of it. Everything else is accepted, adjusted, or discarded.
5. **Save and review.** Unchanged: saving never reviews, the size and each pose are reviewed
   separately, and only reviewed independent records are scored.

The manual controls stay, behind an "Adjust" disclosure, for the cases a fit cannot handle.

## What a fitted box is

- **The plan-view envelope of the vehicle as its reviewed returns show it.** Anything attached
  is included: mirrors, racks, a tow bar. Passing clearance needs the outermost point, and a
  bumper-to-bumper gap is unaffected by mirrors. Returns that should not count belong out of the
  mask, not inside a box drawn smaller than it.
- **A face sits at the outermost reviewed return along its normal**, ignoring the outermost
  0.5 % of the object's returns in that frame, at most five, so one stray return does not move a
  face. A face seen edge-on is a thin column of returns, and a percentage trim alone would eat it
  on a dense object: 2 % of a truck's 15,000 returns at 4 m is 300 points.
- **The body axis points to the front**, which is the direction of travel when the object
  moves. A stationary object has a front/rear-ambiguous axis and no named front or rear.
- **Height is a lower bound**: the vertical span of the reviewed returns. At the 2.3 m mount a
  tall vehicle's roof is above the top ring near the sensor, and gap scoring does not use height.

## What `±` means

Every bound is a conservative half-width, as the record defines it, not a standard deviation.
The fit adds named terms and reports each one, so the operator can see what a bound covers:

| Term     | Covers                                                                    | Size                                               |
| -------- | ------------------------------------------------------------------------- | -------------------------------------------------- |
| Sampling | A face seen edge-on can lie up to one azimuth step beyond its last return | Range × 0.2°: 3.5 mm per metre, 0.14 m at 40 m     |
| Range    | A face seen face-on is placed by measured range                           | 0.03 m                                             |
| Trim     | The returns ignored as stragglers                                         | The distance from the outermost return to the face |
| Heading  | A heading bound rotates the projection that places a face                 | Half the perpendicular extent × the heading bound  |

A dimension adds the bounds of its two faces. A position combines its along and across bounds
as a radius. An end that is not seen has no bound: the dimension becomes a lower bound and the
end is not observed. The generated uncertainty-assumptions text names these terms and their
values, so the free-text field is filled from what was actually done.

## Seen ends and sides

In each frame the fit decides, for each end and side, whether the returns reach a real face:

- **Facing.** Its outward normal points toward the sensor, so the face itself returns light; or,
  for an end, the side facing the sensor runs to it.
- **Not cut.** No return of another object, or unlabelled foreground, lies nearer the sensor
  within 1° of azimuth beyond the end. A nearer return there means the end the mask reaches may
  be the edge of an occluder.

These are proposals. The operator confirms or overrides each end, and the validator still
requires an observed end to be reached by that frame's returns to within 0.5 m.

## Fitting the size

Over every reviewed frame of the object with at least 20 returns:

- **Heading per frame.** The course from the object's mask centroids over about one second
  either side, when it moved at least 2 m in that window. It is refined to the nearest side of
  the minimum-area rectangle around the returns when they span at least 2 m along it. A
  stationary object uses the rectangle alone.
- **Length** from the frames that see both ends, using the better-bounded half of them. A frame
  sees at most the whole body, so every span is a lower bound on the length and their median
  understates it: on kirk0's truck 1 the spans of well-bounded close frames run from 10.37 to
  10.69 m. The value is the 90th percentile of the spans, and the half-width the median of those
  frames' bounds. Fewer than three such frames make it a lower bound from the longest span seen.
- **Width** from the frames that see an end face square-on, within 30° of end-on, measured across
  that end face's own returns so a heading error tilts only a thin slice; or across a roof seen
  from above, below the sensor and within 20 m, when its returns span at least 1 m. The same
  estimator applies.
- **Supporting frames** are the most tightly bounded frames at or above the median span, and the
  fit places a pose at each, because an observed dimension is checked along a pose's heading. A
  cited frame that cannot take a pose is dropped; with none left, a full span is stated as
  inferred and a lower bound withdrawn.
- **Detached returns.** A few returns separated from the rest along the axis, by more than 0.5 m
  among the outermost, are reported in runs of frames with the worst one named. On kirk0 this
  found frames where a truck's mask holds returns 9.5 m ahead of it: membership to fix, not a
  longer truck.

## Fitting a pose at a frame

With the size known:

- **Heading** from this frame's returns when a side is seen over at least 2 m (observed),
  otherwise from the course (inferred), with its bound.
- **Anchor on what is seen.** Both ends: the body centre. One end: that end's face, offset half
  the length from the centre. No end seen: the body centre, anywhere the fitted length still
  covers the returns. When the width is only a lower bound, the centre across is unknown, so the
  pose locates the side it sees instead, at its middle, with no offset. Nothing usable: no pose,
  and the fit says why.
- **Ends.** Seen ends are observed. An end that is not seen is inferred from the body length
  when the length is full, and unknown otherwise.
- **Review fields.** Proposed, independent, method `fit:mask_v1`, the generated assumptions,
  and the membership revision the fit read.

## Independence

The fit reads the reviewed membership and the pack's points, and nothing from the tracker. Its
records are independent proposals and become reference truth only when a person reviews them,
like any proposal. A fit pins the membership revision and digest it used; a save against changed
membership is refused as stale, as it is today.

## Service and tools

- `velocity lidar annotation-reference fit --pack DIR --object ID [--sample N ...] --out FILE`
  writes an import file of proposals, checked exactly as `import` would check it, and prints
  what was seen at each frame and every bound term. It never overwrites a file and stores
  nothing.
- `POST /api/annotations/physical/fit` returns the same proposal, with the per-frame diagnostics,
  for the macOS client to place in the draft.

## F1 on kirk0

The fit ran read-only on kirk0's reviewed pack (sidecar revision 1981); every import it wrote
passes `validate --file`:

| Object                | Length         | Width         | Height   | Poses |
| --------------------- | -------------- | ------------- | -------- | ----: |
| `obj_35c661db`, truck | 10.70 ± 0.24 m | 2.45 ± 0.27 m | ≥ 3.08 m |     6 |
| `obj_8e829a06`, truck | 9.63 ± 0.35 m  | 2.84 ± 0.20 m | ≥ 3.13 m |     6 |
| `obj_14ed7858`, car   | 4.07 ± 0.26 m  | 1.74 ± 0.20 m | ≥ 1.15 m |     6 |
| `obj_8529ec5b`, car   | 5.14 ± 0.23 m  | 1.87 ± 0.21 m | ≥ 1.52 m |     5 |

Truck 1's hand-measured 10.43 m at sample 471 lies just below the fitted interval: that frame's
returns span 10.43 m, and one frame's span is a lower bound, which is why the fit takes a high
percentile over many frames. Truck 2's width is measured across its front, mirrors included, as
the convention says. These are fitted
proposals on a tuning pack, not physical validation: nothing here was measured independently.

## F2 as built

`POST /api/annotations/physical/fit` returns a fit for the membership the window is looking at,
by its digest, and stores nothing. In the window:

- **Fit to reviewed points** comes first. **Fit object** takes the fitted size and every fitted
  pose; **Fit pose here** takes this frame's pose and keeps the draft's size if it has one. Either
  is one undo step. The section says why it cannot run (an unreviewed object, proposed frames,
  unsaved point changes) and shows the fitted size, the notes, and this frame's faces with their
  bound terms.
- **No evidence pickers.** A dimension is Full, At least or Unknown. Set by hand, a full value is
  inferred from the frame it was read at and a lower bound is observed there; a typed position
  is observed at its frame. Each end is one checkbox: a real end in this frame, or placed by the
  length.
- **Adjust by hand** holds the axis, heading, anchor and offset, position, the renamed **Anchor
  height (optional)**, shared errors, and method and assumptions. A record whose bounds are set by
  hand states its assumptions itself, so no free-text field blocks a save; a missing `±` is
  flagged beside its value.

That takes part of F3 early. What F3 still needs is the service's refusals shown beside the field
that causes them, rather than at the top and bottom of the column.

## The pane

- A **Fit** section first: "Fit object" and "Fit pose here", the fitted values read-only with
  their source frames, and one "real end / cut off" choice per end.
- The manual controls under **Adjust**, unchanged, for exceptions.
- Errors beside the field that causes them.
- The pose "Height" control renamed "Anchor height (optional)" and moved under Adjust.
- The box drawn in the 3D view and every elevation, from the lowest to the highest return when
  the height is a lower bound.
- One frame label everywhere, "Frame 475 · sample 471", and `--frame` accepted where tools take
  `--sample`.

## Increments

| Increment | Delivers                                                                                                             | Done when                                                                                                                           |
| --------- | -------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| F1        | The Go fit (size and poses), the `fit` command writing an import file, unit tests on synthetic boxes                 | Synthetic boxes are recovered within their stated bounds; kirk0's two slow trucks fit, and the import file passes `validate --file` |
| F2        | The service endpoint; "Fit object" and "Fit pose here" in the window, filling the draft; per-frame diagnostics       | An operator fits, saves and reviews truck 1 without typing a number                                                                 |
| F3        | The simplified pane: fitted view first, manual controls under Adjust, errors at the field, the renamed anchor height | The pilot's two stops cannot recur: no silent required field remains in the fitted path                                             |
| F4        | The box in 3D and every elevation; one frame label                                                                   | A fitted box is visible in every view; window and tools name frames the same way                                                    |
| F5        | Suggested frames: face changes and the shared frames a following pair needs; fitting several frames at once          | kirk0's truck pair gets poses at shared frames that `draft-following` turns into gaps                                               |

## What this does not change

The record schema, the validator, review, independence and scoring are untouched: a fitted
record is an ordinary proposal. A fit is no truer than its mask, and whether an end is real stays
a human judgement. A fit does not replace an independent measurement, such as a survey or a
manufacturer's dimensions, which still arrives through `import`.

## Open questions

- Whether the envelope should exclude mirrors by default, for a body-shell width that matches a
  catalogue. The default here includes them; passing clearance is the reason.
- Whether five stragglers is the right cap at the densest ranges.
- Whether the fit should ever mark an end observed without the operator's confirmation.
