# Model-assisted review: what a vision model and a screenshot API would add

An exploration of what the annotation flow gains if a multimodal model (vision and language) and
a programmatic renderer are put behind the proposals an operator grades. It builds on the
[review workflow plan](lidar-review-workflow-plan.md), which already reserves a place for a
language model as "pass 2", and asks the four questions that plan leaves open: how much operator
time it buys and where, what the records look like, how the borderline cases reach a person, and
what becomes of HINT.

- **Status:** Exploration. Nothing here is built; the review workflow's pass 1 and pass 2 are
  unbuilt too, and the only proposer that exists is the macOS `ObjectProposer`
- **Layers:** L5 Tracks, L6 Objects, L8 Analytics, L10 Clients (macOS visualiser), sweep, offline analysis
- **Related:** [Review workflow](lidar-review-workflow-plan.md), [Segment finder](lidar-annotation-segment-finder-plan.md), [HINT sweep mode](../lidar/operations/hint-sweep-mode.md), [Point annotation tool](../lidar/operations/point-annotation-tool.md), [Physical reference review](lidar-physical-reference-review-plan.md), [Priority review queue](lidar-visualiser-priority-review-queue-plan.md), [Track quality score](lidar-visualiser-track-quality-score-plan.md), [Classifier scorecard](lidar-ml-classifier-training-plan.md), [Deterministic scene capture](lidar-deterministic-scene-capture-plan.md), [Label-aware tuning](lidar-track-labelling-auto-aware-tuning-plan.md) (its deferred item 8.4 is this idea in one line)
- **Tenets touched:** 1 (privacy, local data), 3 (evidence over opinion), 4 (local-first)

## Assumptions

Two things in the question could not be pinned down from the repository and are read as follows.

- **"A model like jev"** is read as any general multimodal model that takes images and text and
  returns structured text. Nothing in the repository names such a model, and the design below is
  model-agnostic: the model sits behind one adapter, and a different model is a different
  proposer version.
- **"A screenshot API"** is read as a programmatic way to render one subject at one sample, in a
  chosen view, at a fixed metric scale, to an image. The shared conversation describing it could
  not be fetched from this session, so both ways of building one are weighed below.

## What the operator does today, and where the hour goes

The flow has four loops. Each is a place a model could enter, and they are not equally good bets.

### Loop A: track verdicts for HINT

Each HINT round replays a reference run, then waits until 90% of its tracks carry a class and a
quality label, assigned one at a time in the macOS visualiser. Labels carry between rounds by
temporal IoU at 0.5 and are marked `carried_over`; everything else is typed again. The reference
set is 120 labelled tracks across 9 runs, against a 34-minute replay that produces 4,188 track
rows ([review workflow, motivation](lidar-review-workflow-plan.md#motivation)). The 90% gate is
the reason HINT runs rarely.

### Loop B: point membership in the annotation window

An operator presses **Propose objects**, steps through each proposal's frames, and accepts,
splits, merges or dismisses it; then carries masks forward, fixes by hand what the carry got
wrong, and reviews every frame. Measured on a twenty-second pack: 43 clusters a frame, 8,600
object-frames, and six seconds each by hand
([point annotation tool](../lidar/operations/point-annotation-tool.md#what-it-is-for)).
The proposer and the carry remove most of the agreeing; the carry stops only where a person
would not have agreed either, with five named reasons (`lost`, `grew`, `ambiguous`, `jumped`,
`overlaps`, in `MaskPropagation.swift`). The class guess on a proposal comes from size and
distance travelled alone. An hour of labelling still covers tens of seconds of a busy street.

### Loop C: physical references

Once masks are reviewed, `fit:mask_v1` places a box and a pose at the frames that show the body
best. What is left to the person is the part the fit cannot do: whether an end the returns reach
is the vehicle's end or an occluder's edge, whether stray returns belong, and which keyframes a
following pair needs. The
[kirk0 pilot](../lidar/operations/physical-reference-pilot-kirk0-2026-10.md)
reviewed three vehicles and 22 poses, removed stray returns up to 9.5 m from a body, and refitted
one pose because a tracker-assisted declaration had persisted.

### Loop D: choosing what to look at

Finders and selectors rank windows (`following`, `leader_changes`, `lateral_jump`,
`split_flags`, `arm_disagreement`, `exposure`), a clip is cut, a pack is made, and proposals are
written as a layer. This loop is already code; the operator's choice is which window, not which
return.

### Where the time goes

In every loop the operator's time divides the same way:

1. **Agreeing** with what an algorithm already found. Loop B's carry has mostly removed this;
   loop A has not (every track is looked at), and loop C has only the fit.
2. **Deciding the stops**: the frames and tracks where the algorithm said it could not tell.
3. **Naming the class**, with a guess from size and travel as the starting point.
4. **Repairing identity**: a chain that ran from one car onto another, two halves of a car that
   went behind a bus, a mask that took in a trailer or a pedestrian beside the door.
5. **Judging evidence**: is this end seen or occluded, is this return the body or something
   in front of it.

A model that paints points well does not exist, and this flow does not need one: the carry
already paints. What the flow lacks is a cheap, reasonably reliable **semantic judgement** at
items 2 to 5, and a way to put the operator's eyes only where that judgement is weak.

## What a vision model can and cannot do here

The useful property of a multimodal model is that, shown a top view and a side view of a return
cloud **at a fixed metric scale**, with the trail drawn, it can say what the thing is and whether
two pictures are of one thing. The review workflow plan's reason for the fixed scale holds: at a
fixed scale a pedestrian and a car do not look alike, and scaled to fit they do.

What it is bad at, and must never be asked for:

- **Coordinates and indices.** It cannot reliably return point indices or metres. Every spatial
  output is a **choice among candidates code prepared**: a mask candidate, a fit alternative, an
  endpoint status, a sample at which to split.
- **Determinism.** Two calls can differ. A proposal layer therefore records the model, the rubric,
  the prompt digest and the dossier digest, and is immutable once written; re-running is a new
  layer version, never an edit.
- **Rare classes.** With seven pedestrians and one bus in the whole kirk0 pack, nothing can show
  the model is right about them. The plan's rule stands: rare classes stay human-first.
- **Recall.** It sees only what a proposer put in a dossier. A road user nothing proposed stays
  a from-scratch subject for a person.

So the model **refines, chooses and explains**. Code proposes, the model grades the proposal
before the person does, and the person grades last.

## Where it pays, loop by loop

| Loop | What the model is asked                                                                                                      | What it replaces                                      | Confidence that it helps                                          |
| ---- | ---------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------- | ----------------------------------------------------------------- |
| A    | Class and quality per subject, from a trail and keyframes; agreement with pass 1                                             | Looking at every track to label it                    | High: the bulk is cars and noise, which the picture settles       |
| B    | Class on each proposal; a resolution at each carry stop; merge and split proposals; whether stray returns belong             | The guess from size and travel; stepping to each stop | Medium to high for class and stops; medium for identity repair    |
| C    | Endpoint evidence status (seen, inferred, unknown) per keyframe; axis direction; which fit alternative; stray-return verdict | The operator's end-by-end judgement                   | Medium: a side view with the occluder drawn is what a person uses |
| D    | Nothing new; proposer disagreement becomes a finder                                                                          | Scrolling to find the hard cases                      | High, and it costs almost nothing                                 |

### Loop A: the largest, cleanest win

The HINT gate counts labelled tracks; the review workflow already proposes that it count graded
subjects instead, with pass 1 running when the reference run completes. Adding pass 2 means the
operator opens a round to a list of cards, each a picture with a proposed class, a quality flag,
a confidence and two lines of reason, and grades them. Where pass 1 and pass 2 agree with high
confidence, the card is in a "confirm in bulk" list that is sampled for audit; where they
disagree, or either is unsure, the card is in the queue and is opened in the visualiser.

On kirk0's whole pack, 29 of 61 objects were not road users and 24 of the 32 that were are cars
([annotation-scored tuning](../lidar/operations/annotation-scored-tuning.md#the-pack)). If the
two proposers agree on most of those, the operator's round is the pedestrians, the bus, and the
disagreements. The number that matters, the agreement rate per class, is exactly what
`review-score` is specified to report, and it does not exist yet; see
[what to measure first](#what-to-measure-first).

### Loop B: resolving stops, not painting

Each carry stop is already a named question. The model's job is to answer it in a form the carry
can act on:

| Stop        | Question put to the model                        | Answer, as a choice                                                          | What code does with it                                              |
| ----------- | ------------------------------------------------ | ---------------------------------------------------------------------------- | ------------------------------------------------------------------- |
| `lost`      | Is the object occluded, out of view, or gone?    | `partly_occluded` / `fully_occluded` / `outside_view` / `ended`, occluder id | Writes the mask's `Visibility`; resumes the carry past an occlusion |
| `grew`      | Did it meet another object, or just come closer? | `merged_with:<proposal>` / `closer` / `cannot_tell`                          | Offers a split at this sample, or lets the carry continue           |
| `ambiguous` | Which of the rival fits is the object?           | candidate index / `cannot_tell`                                              | Carries the chosen fit, marks the mask `partial` if unsure          |
| `jumped`    | Same object, or a new one where the old one was? | `same` / `new_object` / `cannot_tell`                                        | Continues, or ends this chain and seeds a new proposal              |
| `overlaps`  | Whose returns are these?                         | `this` / `other` / `both:split` / `cannot_tell`                              | Assigns, or marks `UncertainIndices`                                |

Every `cannot_tell` goes to the queue. Every other answer is a proposal: the mask stays
`proposed` and the person's review is still what makes it truth.

Identity repair is the same shape. The dossier for a proposal includes the trails of the
proposals that start where it ends or end where it starts, and the model may say "12 and 19 are
one car" or "7 runs from a car onto a van at sample 231". The window already has **Merge N
into** and **Accept up to here**; these become one-key confirmations of a proposed repair.

### Loop C: evidence status, with the tracker kept out

A fitted keyframe has, per component, an `EvidenceStatus` of observed, inferred, prior-only or
unknown, and a physical record is `independent` only if nothing from a tracker seeded it. A model
can propose the status of each end from a side view with the occluder drawn, flag returns that sit
outside any plausible body, and say which way the body faces from the trail. Two rules keep the
record honest:

- **The physical dossier carries no tracker layer.** Its provenance lists the layers rendered,
  so a reviewer can see that the proposal could not have been tracker-assisted. A dossier with a
  tracker layer makes a `tracker_assisted` record, by construction.
- **The model names itself in `PhysicalReview.Method`**, as `fit:mask_v1` does, never in
  `Provenance.Algorithm`, which an independent record may not carry.

### Loop D: disagreement as a finder

`arm_disagreement` already ranks windows where two estimate versions differ. A
`proposer_disagreement` finder ranks windows where pass 1 and pass 2 differ, or where either
proposed `cannot_tell`, and feeds the same Segments page and the same queue. This is the cheapest
piece of the whole design and the one that most directly puts eyes on the borderline.

## The screenshot API

The model needs a **dossier**: per subject, one image with the whole trail over the
region-coloured background, and keyframes of the subject's returns in top and side view at one
fixed scale, with the occluders drawn and the samples labelled. The operator needs the same
pictures as review cards. One renderer of stills already exists, and two more could be built:

| Property         | `tools/scene-capture` (exists)                                                                   | macOS visualiser render endpoint                                              | Headless Go renderer from the pack                                  |
| ---------------- | ------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------- | ------------------------------------------------------------------- |
| What it draws    | The web scene player over a scene export: clip points, tracks, boxes, trails; recipe-named views | Exactly what the operator sees: the four elevations, the 3D view              | Orthographic top and side projections of `points.bin`               |
| Determinism      | Checks the render is stable before trusting the pixels; repeatable, not byte-pinned              | GPU and window-size dependent; not byte-stable                                | Same pack, same bytes; the digest goes in the proposal's provenance |
| Where it runs    | Anywhere with Node and Chromium: the operator's Mac, CI                                          | A Mac with the app open                                                       | Anywhere `velocity` runs, including CI and the Pi                   |
| Fit to the model | Perspective cameras today; needs fixed-scale orthographic recipes and mask colouring             | Perspective and shading the model does not need                               | Fixed-scale orthographic, which the review workflow plan asks for   |
| Cost to build    | A pack-to-clip export, two orthographic recipes, mask colouring, a per-subject contact sheet     | A local HTTP listener or URL scheme in the app, plus an offscreen render path | New package: projection, colouring, legends, `image/png`            |
| Today            | Milestones 1 and 2 of the [scene capture plan](lidar-deterministic-scene-capture-plan.md) done   | No screenshot, snapshot or automation surface exists in the app               | No PNG writer exists in Go; charts are SVG                          |

The scene capture plan's rule, "do not build a second renderer", settles the order. The dossier
should come from `scene-capture` first: it already has recipes, a stability check, a contact
sheet and provenance, and the model runs where Chromium runs, on the operator's machine or in
CI, never on the Pi. What it lacks is a way in from a pack (a `KindClip` export of the pack's
samples with mask membership as colour), two orthographic fixed-scale recipes for top and side,
and the occluder and trail layers. The Go renderer is the fallback if byte-level determinism
turns out to matter for the anchoring run, or if the Chromium dependency is refused where
`velocity lidar propose` must run. The macOS endpoint is worth having for one reason only: the
grader should see **what the model saw**, and the window can show the dossier image beside its
own views. One picture, served to both, whichever renderer makes it.

What the renderer records, per image: pack digest, subject, samples, view, scale in pixels per
metre, layers drawn (points, background regions, trail, occluders, tracker context or not),
renderer version, and the image digest. That manifest is the `dossier` field in every proposal.

## Data structures

Everything below is additive to the sidecar and physical schemas in
`internal/lidar/annotation`. Nothing changes what a reviewed record is.

### Proposal layers

`proposals/<proposer>@<version>.json`, in the sidecar schema, every record `proposed`, as the
review workflow specifies. Two proposers are new:

| Proposer                     | `Provenance.Algorithm` | `Provenance.AlgorithmVersion`                       |
| ---------------------------- | ---------------------- | --------------------------------------------------- |
| Pass 1, rules on metrics     | `metric`               | rules version                                       |
| Pass 2, model on the dossier | `llm:<model id>`       | `<rubric version>+<prompt digest>+<dossier digest>` |

A proposal record gains fields a reviewed record never needs:

```go
// Proposed is the part of an Object, FrameMask or PhysicalKeyframe that only a
// proposal carries. A reviewed record keeps it only inside DerivedFrom.
type Proposed struct {
    Confidence   float64       `json:"confidence"`
    Alternatives []Alternative `json:"alternatives,omitempty"` // next-best classes, with confidence
    ReasonCodes  []string      `json:"reason_codes"`            // shared with the track quality score
    Rationale    string        `json:"rationale,omitempty"`     // two lines, citing what was seen
    Dossier      *DossierRef   `json:"dossier,omitempty"`       // what the model saw; nil for pass 1
    Anchored     *bool         `json:"anchored,omitempty"`      // pass 2: was pass 1's answer shown
}

type Alternative struct {
    Class      string  `json:"class"`
    Confidence float64 `json:"confidence"`
}

type DossierRef struct {
    RendererVersion string   `json:"renderer_version"`
    Layers          []string `json:"layers"`         // proves a physical dossier had no tracker layer
    ImageDigests    []string `json:"image_digests"`
    PixelsPerMetre  float64  `json:"pixels_per_metre"`
}
```

The margin between `Confidence` and the first alternative is the single most useful borderline
signal, so it is stored rather than recomputed.

### Choices, not geometry

A pass 2 mask is a choice among candidates pass 1 prepared, resolved to indices by code:

```go
type MaskChoice struct {
    ObjectID  string `json:"object_id"`
    SampleID  int    `json:"sample_id"`
    Candidate string `json:"candidate"` // as_observed | dilated | connected_component | height_limited | <candidate id>
    Reason    string `json:"reason,omitempty"`
}
```

Stops, identity repairs and endpoint statuses are the same pattern:

```go
type StopResolution struct {
    ObjectID   string  `json:"object_id"`
    SampleID   int     `json:"sample_id"`
    Stop       string  `json:"stop"`       // lost | grew | ambiguous | jumped | overlaps
    Resolution string  `json:"resolution"` // per the table above; cannot_tell always allowed
    OtherID    string  `json:"other_id,omitempty"` // the occluder, the rival, the merged object
    Confidence float64 `json:"confidence"`
}

type IdentityProposal struct {
    Kind       string   `json:"kind"`      // merge | split
    ObjectIDs  []string `json:"object_ids"`
    AtSample   *int     `json:"at_sample,omitempty"` // split only
    Confidence float64  `json:"confidence"`
    Rationale  string   `json:"rationale"`
}

// EndpointProposal proposes a PhysicalKeyframe's Front or Rear EvidenceStatus
// and, where observed, which fit alternative places it.
type EndpointProposal struct {
    ObjectID   string `json:"object_id"`
    SampleID   int    `json:"sample_id"`
    End        string `json:"end"`        // front | rear
    Status     string `json:"status"`     // observed | inferred | unknown
    OccluderID string `json:"occluder_id,omitempty"`
    FitChoice  string `json:"fit_choice,omitempty"` // with_stragglers | without | <alternative id>
    Confidence float64 `json:"confidence"`
}
```

A physical proposal layer, `proposals/physical/<proposer>@<version>.json`, uses the
`PhysicalReferenceSet` schema with `Review.Status = proposed`,
`Review.Method = "llm:<model>@<rubric>"`, and `Origin` set from the dossier's layers.

### Lineage and the grade

On a reviewed record, as the review workflow specifies, with the grade made explicit:

```go
type DerivedFrom struct {
    Layer       string   `json:"layer"`        // proposals/llm-...@...json
    LayerDigest string   `json:"layer_digest"`
    RecordID    string   `json:"record_id"`
    Grade       string   `json:"grade"`        // accept | accept_with_edits | reject | cannot_tell
    Proposed    Proposed `json:"proposed"`     // frozen copy, so the score survives layer pruning
}
```

`review-score` reads `DerivedFrom` and the reviewed record and reports, per proposer and version,
class agreement and the confusion matrix, mask IoU and edit cost, accept/edit/reject/cannot-tell
rates, and the anchoring delta, split by class, range and segment.

### The queue item

The [priority review queue](lidar-visualiser-priority-review-queue-plan.md) proposes
`lidar_review_queue_items` keyed by run and track. For packs the key is pack digest, object and
sample, and the components are the uncertainty signals:

```go
type ReviewQueueItem struct {
    PackDigest string `json:"pack_digest"`
    ObjectID   string `json:"object_id"`
    SampleID   *int   `json:"sample_id,omitempty"` // nil for an object-level item
    Kind       string `json:"kind"`     // class | stop | identity | endpoint | audit | from_scratch
    Priority   float64 `json:"priority"`
    Components struct {
        Margin        float64 `json:"margin"`          // top class minus first alternative
        Disagreement  bool    `json:"disagreement"`    // pass 1 and pass 2 differ
        CannotTell    bool    `json:"cannot_tell"`
        AnchoringDelta float64 `json:"anchoring_delta"` // shown vs hidden pass 2
        Stop          string  `json:"stop,omitempty"`
        Rarity        float64 `json:"rarity"`          // reviewed examples of this class, inverted
        Impact        float64 `json:"impact"`          // segment purpose: a following pair's shared sample scores high
    } `json:"components"`
    Status string `json:"status"` // open | claimed | resolved | skipped
}
```

The item is derived on read from the layers and the sidecar, as pack status is, and stored only
if operators need to share claims.

### HINT state

HINT's `LabelProgress` counts by class, and the sweep's `LabelProvenanceSummary` already counts
labels by source (`human_manual`, `carried_over`, `auto_suggested`). The gate counts graded
subjects, and the round state gains what the dashboard needs to show why a round is or is not
ready:

```go
type LabelProgress struct {
    Total, Labelled int
    Pct             float64
    ByClass         map[string]int
    BySource        map[string]int      // as LabelProvenanceSummary, per round
    Graded          int                 // subjects with a person's grade this round
    CarriedGrades   int                 // grades carried by observation identity
    Queued          int                 // items awaiting a person
    ProposerAgreement map[string]float64 // per class, from review-score on this round
}
```

## Surfacing the borderline

"Borderline" is defined by signals, most of which exist already, and the queue is where they
meet:

| Signal                                | Source                                                     | Exists today                                     |
| ------------------------------------- | ---------------------------------------------------------- | ------------------------------------------------ |
| Carry stop needing an operator        | `PropagationStop.needsOperator`                            | Yes, in the window; it halts the carry and waits |
| Proposer disagreement                 | pass 1 class versus pass 2 class                           | No proposers yet                                 |
| Low margin or `cannot_tell`           | `Proposed.Confidence` and `Alternatives`                   | No                                               |
| Anchoring delta                       | pass 2 shown versus hidden                                 | No                                               |
| Uncertain returns, unknown visibility | `FrameMask.UncertainIndices`, `Visibility == unknown`      | Yes, as fields                                   |
| Axis or endpoint unresolved           | `Pose.AxisAmbiguous`, `EndpointEvidence.Status == unknown` | Yes, as fields                                   |
| Two tracker chains on one footprint   | `tracker_chain` hint layer, "split here?"                  | Planned (segment finder, simplification 3)       |
| Rare class                            | reviewed examples per class, this split                    | Countable now                                    |
| Audit                                 | random sample of accepted proposals, re-graded blind       | Planned (review workflow, item 5)                |

How it reaches the person:

1. **The queue is the round.** A HINT round or a pack opens to its queue, ordered by priority,
   not to sample 0. Each item is a card: the dossier image, the proposal, the reason codes, the
   alternatives, and the four grades on single keys. Accept is never pre-selected.
2. **Confident agreement is confirmed in bulk**, off the queue, and sampled for audit. The
   operator sees the sample, not the whole list.
3. **Anything spatial opens the window at the sample.** A stop, an overlap, a split: one action
   from the card to the annotation window at that sample with the proposal ghosted over the
   points, and back. The review workflow's item 4 already specifies this move.
4. **"Nothing here was proposed" is one step away** on every surface, so recall is not lost
   behind a queue of things that were.
5. **The frame strip and the sector ring stay.** They show what is agreed, in question and
   unlabelled; the queue is a way through them, not a replacement.

What this removes: stepping to every stop, scrolling past the easy frames, and typing a class
that size and travel already made obvious. What it keeps: a person at every stop the model could
not resolve, every rare class, every from-scratch subject, and a blind sample of what was accepted.

## HINT: extended, not replaced

HINT's objective does not change. A sweep is still scored against truth, and truth is still a
person's grade. What changes is how the grades are produced and how many a round needs.

**Today.** Reference run; wait for 90% hand labels; sweep; carry by temporal IoU; repeat.

**With proposers.** Reference run; pass 1 and pass 2 run on completion; grades from the previous
round carry by **observation identity** where only tracker parameters changed, which is the
usual case, so most subjects arrive graded; the person grades the queue, which is the subjects
that are new, changed, disagreed on, or rare; the gate counts grades; sweep. The label window
becomes the model's run time plus a short pass over the queue. Proposer agreement per class is
shown on the round card, so an operator can see when a class is being trusted that has not
earned it.

**One seam must close first.** The HINT gate counts only manual labels, but
`EvaluateGroundTruth` takes any positive `user_label` as reference truth, carried-over labels
included. Today that is a mild inconsistency. Once graded and proposed objects project onto
run-track labels as `human_manual` and `auto_suggested`, as the review workflow specifies, the
evaluator would score sweeps against the model's proposals unless it applies the same manual
predicate the gate does. That change is small and goes in before any projection.

**Can the model replace the person in the loop?** Not as truth. A sweep tuned against the
model's labels is a tracker tuned to what the model finds plausible, which is the stale-reference
problem in a new coat, and the review workflow's rule that a proposal is never an input to the
estimator it describes exists for this. What is defensible is a **proxy round**: between human
rounds, narrow bounds against a model-scored objective the way auto-tune narrows against
`empty_box_ratio` and fragmentation today, and let the next human round confirm or reject the
narrowed region. Whether a model-scored proxy is better than the existing proxies is a
measurable question, and it should be measured on kirk0 before a proxy round is offered.

**The 90% gate** stays as a floor on graded coverage, and gains the per-class coverage gate HINT
already has (`MinClassCoverage`), so that a round whose pedestrians are all `cannot_tell` does
not proceed on cars alone.

## Constraints and risks

**Tenet 1 and Tenet 4 decide where the model runs.** Measurement data stays local, and a remote
request must not send vehicle data or raw sensor captures. A dossier is a rendering of returns
from vehicles. The case for a hosted model is that a LiDAR render has no PII by architecture and
that review packs already leave the Pi for the operator's Mac; the case against is the plain
reading of "vehicle data". This exploration does not decide it. It makes the adapter local by
default (a model running on the operator's machine, reached over localhost), keeps a hosted
backend behind an explicit operator flag that is off by default and never set on a Pi, and asks
for a decision record before any hosted backend ships. Either way the pipeline, the reports and
every production path never depend on it: a site with no model has pass 1.

**Anchoring.** A person shown a box tends to keep it, and a model shown pass 1's class tends to
agree with it. Pass 2 is calibrated shown and hidden, and the delta is a queue signal and a
reported figure.

**Rubber-stamping.** Bulk confirmation is the whole speedup and the whole risk. The blind audit
sample is not optional, the accept rate is reported beside agreement, and the queue never
pre-selects a grade.

**Truth shaped by the proposer.** From-scratch subjects on the active segments are the reference
no proposer touched, and the held-out packs are labelled without tracker overlays, as the segment
finder plan already requires. A pass 2 dossier for a held-out pack carries no tracker layer.

**Cost and latency.** A dossier is a few images per subject. A 34-minute run's 4,188 track rows
are not 4,188 subjects: pass 1 groups them into subjects and asks the model only where it is
unsure, which is the plan's "refines rather than starts again" made into a budget.

**Two renderers drifting.** If the macOS window and the Go renderer each draw a dossier, the
operator may grade a picture the model never saw. One renderer, served to both.

**Model churn.** A model update is a new proposer version with its own `review-score` row. No
grade is invalidated by it, because the grade is the person's.

## Expected gains, with the assumptions stated

The only measured figure is six seconds per object-frame by hand, and the carry already attacks
it. No measured time per object with `cluster_chain@1` exists; the segment finder plan's item 5
proposes to measure it. So the table below is an estimate under stated assumptions, to be replaced
by `review-score` and a timed pack.

| Loop | Unit today                                                       | Assumed split                                               | What remains human                                   | Estimated factor   |
| ---- | ---------------------------------------------------------------- | ----------------------------------------------------------- | ---------------------------------------------------- | ------------------ |
| A    | Every track opened and labelled; 90% of a run's tracks per round | 70 to 80% confident agreement on cars and noise             | The queue, the rare classes, the audit sample        | 3 to 5×            |
| B    | Every stop visited; every class chosen                           | Half the stops resolved; class right on most cars and noise | Unresolved stops, identity repairs, rare classes     | 1.5 to 2×          |
| C    | Every end judged; stray returns found by eye                     | Endpoint status proposed on most keyframes; strays flagged  | Every end still confirmed; every pair still reviewed | 1.2 to 1.5×        |
| D    | Hard cases found by scrolling or by `arm_disagreement`           | Disagreement finder ranks them                              | Choosing the window                                  | Not time; coverage |

The factors are the ratio of subjects a person must open, not of wall-clock, and they assume the
model's class agreement on cars and noise is high enough to be trusted after calibration. If it
is not, loop A's gain collapses to pass 1's alone, which is still a gain over typing.

## What to measure first

The design stands or falls on one number, and it can be had in an afternoon:

1. Render dossiers for kirk0's reviewed objects from the `ad8b9438` pack (30 reviewed objects
   in the frozen tuning split, 61 in the earlier pack with proposals accepted).
2. Run one model, shown and hidden, with the rubric.
3. Report the confusion matrix against the operator's classes, the road-user split in
   particular, and the anchoring delta.

If agreement on cars and noise is high and the model says `cannot_tell` rather than guessing on
the pedestrians, loop A is worth building. If not, the dossier renderer and the queue are still
worth building for the person, and pass 2 waits for a better model or a better rubric.

## Sequencing

Each step is useful without the next, and the model enters at step 3.

1. **Dossier renderer in Go**, deterministic, from a pack; review cards in the macOS window drawn
   from it. Helps the person at once, with no model.
2. **Pass 1 in Go**: per-track metrics, versioned rules, reason codes, `cannot_tell` below a
   floor; `review-score` against kirk0's reviewed objects. The deterministic baseline every model
   is compared with.
3. **Pass 2 adapter and rubric**: local backend first; the calibration run above; agreement per
   class on the round card.
4. **The queue**: carry stops, disagreement, margin, rarity and audit as items; cards with
   single-key grades; one action to the window and back; HINT counting grades and carrying by
   observation, with the evaluator's truth predicate aligned to the gate's before any proposal
   projects onto a run-track label.
5. **Physical pass**: endpoint status and stray-return proposals from a tracker-free dossier;
   F5's suggested frames for pairs.
6. **Proxy round** in HINT, only after step 3 has shown where the model may be leaned on.

## What this cannot establish

- Recall. A subject nothing proposed is found only by a person, from scratch. Today no UI can
  even record one: the web's missed-region marking is read-only since #707 and the macOS app has
  no missed-region client, so the review workflow's from-scratch subject is the first recall
  writer this flow would have.
- Whether a given model is good at these pictures. That is the calibration run's result, per
  class, and it is not known until it is run.
- Whether a hosted model is within the tenets. That is a decision, recorded as one.
- A wall-clock saving. The factors above count subjects opened, and the timed pack has not been
  run.

## Open questions

1. Does a hosted model backend get a decision record and an operator flag, or is the adapter
   local-only until a local model is shown to be good enough?
2. Should the macOS render endpoint exist at all, or should the window draw the Go dossier through
   the server and nothing else?
3. Is the proxy round worth a sweep mode of its own, or is it an objective option on auto-tune?
4. Which segment purposes set `Impact` in the queue, and who maintains the weights? The
   selectors file is the obvious home.
