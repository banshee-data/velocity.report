# One-hour labelling: scene zones, carried bodies and exception review

label0001 asks for every foreground return of a 180 s held-out window labelled and every mask
reviewed, and its estimate is eight hours of one person's time. This plan aims at one hour. It
does so without an external model: the operator says once what the fixed parts of the scene are,
draws a road user's body at a few keyframes and lets the window carry it, and then reviews the
frames where the rules disagree rather than every frame. Code proposes, rules resolve, and a person
drives and validates. An optional local scorer ranks and suggests; it never labels.

- **Status:** Proposed, November 10, 2026, at the owner's direction: no external model calls, the
  person drives and validates, the target is label0001 in about an hour. For the held-out window
  it replaces the hosted-model direction of the
  [model-assisted review exploration](lidar-model-assisted-review-exploration.md). Nothing here is
  built
- **Canonical:** [point-annotation-tool.md](../lidar/operations/point-annotation-tool.md)
- **Layers:** L10 Clients (macOS annotation window), annotation storage, L3 ground, Gerardus
  measured geometry
- **Related:** [label0001](../../data/experiments/label/label0001.md),
  [label0003](../../data/experiments/label/label0003.md),
  [label0002](../../data/experiments/label/label0002.md),
  [labelling workspace](../lidar/operations/labelling-workspace.md),
  [labelling run guidance](lidar-labelling-run-guidance-plan.md),
  [annotation save latency](lidar-annotation-save-latency-plan.md),
  [fit to points](lidar-physical-fit-to-points-plan.md),
  [label-set team workflow](lidar-label-set-team-workflow-plan.md),
  [classifier training](lidar-ml-classifier-training-plan.md),
  [Gerardus alignment](../ui/visualiser/gerardus-alignment.md)

## Where the eight hours go

The only measured rate is six seconds per object-frame by hand, from a 200-frame pack. kirk0's 83 s
had 32 road users over 2,820 object-frames and 29 other objects over 1,785, and its pack took 1,968
saves over 29 hours. label0001's window is 180 s, 1,800 samples at embarcadero-folsom, whose road
users are not yet counted. At kirk0's rate it holds about 70 road users and 10,000 object-frames:
seventeen hours by hand, and the eight-hour estimate already assumes the proposer and the carry.

One hour cannot be had by making an object-frame faster. It needs the unit of work to change:

| Today                                                                                  | This plan                                                                       |
| -------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------- |
| A fixed thing is labelled frame by frame, or applied to every frame with no exceptions | A **zone** is said once and applies to every frame, unless a road user is in it |
| A road user is a mask per frame, carried one step at a time                            | A road user is a **body** set at a few keyframes; its masks follow from it      |
| Review is every frame, or "Review all frames…" on trust                                | Review is every **exception** frame, an audit sample, and a filmstrip           |

The work then scales with the number of things in the window rather than the number of
object-frames. At kirk0's rate that is about 130 things, half of them road users; most of the rest
are noise and fixed clutter, which zones and bulk marking take in groups. Seventy road users in
the forty minutes left after the scene and review steps is about thirty seconds each.

## What the window already does

Much of this exists, as the [hub](../lidar/operations/point-annotation-tool.md) describes:

- **Fixed classes** (ground, building, sign, vegetation) and **Apply to every frame**, which writes
  one world-fixed footprint into every frame. Nothing excludes another object's returns, so one
  shared return refuses the whole save.
- **`persistent_voxels` proposals**: 0.5 m voxels occupied in at least 80 % of frames, joined into
  patches, one proposal per patch over every frame. A car's returns inside a patch voxel go to the
  patch.
- **The column brush**: 0.5 m columns in eight 0.5 m voxels above one ground plane, estimated from
  the first sample.
- **The carry and Propagate**: a 0.25 m voxel footprint moved in XY only and rebuilt from each
  frame's returns, with stops for lost, grew, ambiguous, jumped and overlaps.
- **`ObjectProposer`**: chains of 0.5 m plan clusters across frames, from the pack's own points,
  with class guesses from size and travel. It covered 83 % of labelable foreground on a 200-frame
  pack, and one hand-labelled car was one proposal in all 19 of its frames.
- **Checks**: one frame, gap, speed, shared return, tiny mask, twin; each goes to its frame.
- **Physical keyframes**: a body's length, width and height, and a pose per keyframe, with the box
  fitted from reviewed masks. Nothing turns a box into membership, and nothing interpolates.

What is missing is what this plan adds: precedence between labels, a ground surface, a rigid body
carried through time, membership from that body, and review by exception.

## The scene in layers

Every return in a frame is resolved by the first layer that claims it. The layers are authored
once per window; the masks follow from them, and the masks remain the record.

| Order | Layer               | Says                                                                          | Authored                                        |
| ----- | ------------------- | ----------------------------------------------------------------------------- | ----------------------------------------------- |
| 1     | **Carried bodies**  | The returns inside this road user's body at this frame are this road user's   | Keyframes per road user, carried between them   |
| 2     | **Fixed zones**     | The returns in this footprint and height range are a tree, pole, sign or wall | Once per window, from suggestions or drawn      |
| 3     | **Ground band**     | The returns within this distance of the ground surface are ground             | Once per window: the surface and the band       |
| 4     | **Road-user space** | The returns in this space are a road user not yet named                       | Once per window: carriageway and sidewalk areas |
| 5     | **Unresolved**      | Nothing claims these                                                          | The queue                                       |

The order is what makes the zones safe. A car under a tree takes the returns inside its body, and
the canopy above it stays tree. A car over the road takes the returns that were ground before it
arrived, because it outranks the ground band. A fixed zone never needs to know when a road user
passes through it.

Layer 4 never labels. A return in the road-user space that no body claims is a missed road user
until someone says otherwise, and it is the queue's first kind of item. Clusters of such returns,
grouped as the proposer groups them, are what the operator turns into new bodies or marks as noise.

Precedence also settles the save rule that one return belongs to one object per frame: the
resolver assigns each return once before anything is saved, so a zone and a body can no longer
refuse each other's save.

### Fixed zones

A zone is a footprint in the top view, a height range above the ground surface, a class and its
source. "This area is always a tree" is one zone; a pole is a small one with a tall range. A zone's
masks are written to every frame in one save, minus whatever a body claims.

The window suggests zones; the operator accepts, edits or draws them:

- `persistent_voxels` patches, which already find what is there in most frames.
- Gerardus's trees, street furniture and building footprints for the site, placed by its accepted
  pose. These are measured and inferred geometry with an evidence status, and the suggestion
  carries it.

A swaying canopy or a flag is why zones matter: it is foreground in most frames and has nothing to
do with traffic, and today it is either labelled frame by frame or left unlabelled.

### The ground surface and band

The ground is a surface, not the one plane the column brush estimates from the first sample. Two
tracker-free sources exist for this window: Gerardus's ground mesh for the site (1 m cells from
3DEP heights refined by registered scans, about 83 % of cells within 60 m covered), and a fit to
the pack's own background snapshots, as the L3 regional plane fit does in replay. The band is a
thickness either side of it, 0.2 m above as the opt-in surface clip uses. "Ground, but it might
become a car" is layer 3 under layer 1: ground until a body is over it.

### Road-user space

The space road users occupy is an area times a height range: the carriageway and the sidewalks,
from 0.2 m to 4.5 m above the ground surface, the range the surface clip already uses. The areas
come from DataSF kerbs and centrelines through Gerardus, or from a few polygons the operator draws
in the street painter, which Gerardus stores as `street_use` surfaces. Carriageway and sidewalk
also give a class prior: a body on the sidewalk is more likely a pedestrian or cyclist.

Lanes proposed from tracks are not used: they are the tracker's output, and the held-out window is
labelled blind to it.

### Carried bodies

A road user is a box: length, width, height, and a position and yaw at each keyframe. The operator
sets a keyframe where the body is clear, usually its first and last good frames; the window
interpolates the pose between keyframes at constant velocity and yaw rate, then snaps each frame by
a small search (0.5 m in XY, 10° in yaw) to the pose that holds the most returns. The body's mask
at a frame is every return inside the box, with a margin, above the ground band.

This is the physical keyframe turned the other way. Today a box is fitted from reviewed masks;
here the masks follow from the box. It is what "carry the same physical shape into other frames"
means: a rigid body moved through the world, not a footprint rebuilt from whatever returns it last
took, which is why today's carry creeps.

The window seeds a body for each proposer chain by fitting a box to the chain's returns, so most
road users start with a body and keyframes the operator only checks. A snap that fails is an
exception, judged by the same stops the carry uses today: lost, grew, jumped and overlaps.
Returns near the box that it does not take, and returns claimed by two bodies, are exceptions too.

Long vehicles and side-by-side pairs get more keyframes; they are where a rigid box fits worst and
where the November 10 review asks label0001 to start.

## Exception review

label0001's second pass grades every mask. Over 10,000 object-frames that is only possible if the
grade is given per object, over a filmstrip, with every frame that needs a look shown to the person
individually. The plan makes that explicit rather than relying on "Review all frames…", which today
records only the operator's statement.

An object's review shows:

1. **The filmstrip**: the body or zone over its whole life, small, at a fixed scale.
2. **Every exception frame**, one by one: a stop, a failed snap, a shared or nearby return, a
   check from the Checks panel, a frame where the mask's count or extent jumps.
3. **An audit sample**: three frames drawn at random from the rest.

The grade is then given for all its masks at once, and the review records what was seen: the
exception frames, the audit frames, and the rules that produced the exceptions, so a report can
say how truth was graded. A mask the person edits is reviewed individually.

Whether this is enough is measured, not assumed: on kirk0, how many of the frames where a reviewed
mask was corrected would the exception rules have shown? See [Proving it on kirk0](#proving-it-on-kirk0).

## The optional local scorer

A scorer helps the person decide faster; it does not decide. It suggests a class for each body and
orders the queue so the doubtful come first. It runs on the operator's machine, offline, and calls
nothing outside it.

The first scorer follows the [classifier plan](lidar-ml-classifier-training-plan.md)'s transparent
tiers, which add no dependencies: rules, fitted thresholds, logistic regression or a shallow tree,
over the per-cluster features L6 already computes (point count, extents, height p95, intensity,
elongation, compactness, vertical spread), the body's speed from its keyframes, and whether it is
on the carriageway or the sidewalk. It is fitted on kirk0's reviewed masks, which D-40 puts in the
development cohort, and frozen before it runs on the held-out window.

A local vision model through Ollama is a comparator, not the plan: the classifier plan's Tier X,
offline only and never wired in. It is worth running only if the transparent tiers are weak on
classes the person then spends time on, and it is measured on kirk0 the same way.

## Inputs and blindness

The window's inputs are the pack, the declared priors, and what the operator draws. The priors are
the Gerardus record for the site, pinned by id (the shared Gerardus holds two accepted records for
Embarcadero at Folsom), its layers and their evidence status, and the ground surface. Each is
recorded with the label set's provenance by id and digest. Prior files stay outside Git, beside the
pack, as D-39 requires for site geometry.

No track, no L5 output and no lane derived from tracks is an input. Two indirect influences remain
and are stated rather than removed: the pack's own foreground comes from an L3 whose track guard
reads L5 boxes, and Gerardus's registered scans drop points inside tracked boxes. Neither tells the
operator where a road user was; city-sourced layers (DataSF, 3DEP, the tree inventory) are
preferred where both exist. The annotation window should refuse to follow the main view on a
held-out pack in every mode, and record that it did.

## The hour

An estimate for label0001, to be replaced by the kirk0 trial's measured minutes:

| Step       |     Person | What the person does                                                      | What code did beforehand                                                            |
| ---------- | ---------: | ------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| Scene      |     10 min | Accept or draw about 20 zones, the ground band and the road-user space    | Fetched the pinned priors, fitted the ground, proposed patches and zone suggestions |
| Road users |     35 min | About 70 bodies: class, keyframes checked, exceptions fixed, pairs joined | Ran the proposer, fitted a body per chain, carried and snapped it, scored it        |
| Unresolved |      5 min | New bodies for missed road users; the rest marked noise in bulk           | Grouped the returns no layer claimed                                                |
| Review     |     10 min | Every object's filmstrip, exception frames and audit frames               | Listed the exceptions per object and drew the audit sample                          |
| **Total**  | **60 min** |                                                                           |                                                                                     |

The bodies also give label0003's spans: a body's first and last frames are its span, and joined
chains are one vehicle. label0003 is not held for this; it is labelled by hand now, as the
November 10 review decided.

## What to build

| #   | Piece                   | Where                     | What it adds                                                                                                                                                          | Size |
| --- | ----------------------- | ------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---- |
| 1   | Timing                  | Labelling run guidance R2 | Active time from the saves, per step, so the hour is measured                                                                                                         | `S`  |
| 2   | Ground surface          | Pack sidecar and window   | The Gerardus mesh or a fit to the background snapshots, used by the band and the column brush in place of one plane                                                   | `S`  |
| 3   | Priors                  | Pack sidecar and window   | The pinned Gerardus record's trees, furniture, footprints, kerbs and centrelines, placed by the pose and drawn as context                                             | `M`  |
| 4   | Zones and the resolver  | Window and label set      | Zone records (footprint, height range, class, source); the precedence resolver; one save per zone over every frame, minus bodies; suggestions from patches and priors | `M`  |
| 5   | Road-user space         | Window                    | Carriageway and sidewalk areas from priors or drawn; unresolved returns grouped into the queue                                                                        | `S`  |
| 6   | Carried bodies          | Window and label set      | Body records with keyframes; interpolation and snap; membership by box; a body per proposer chain; stops as exceptions                                                | `L`  |
| 7   | Exception review        | Window                    | The per-object review above, recording exceptions and audit frames with the grade                                                                                     | `M`  |
| 8   | Bulk save               | Label set                 | A zone or body writes up to 1,800 masks in one save; the [save latency plan](lidar-annotation-save-latency-plan.md)'s cost per save matters here                      | `M`  |
| 9   | Local scorer (optional) | Offline tool, then Go     | Tier 1 to 3 scorer fitted on kirk0; class suggestions and queue order                                                                                                 | `S`  |

Zone and body records are authoring records: the label set keeps them so a keyframe can be moved
and the masks regenerated, but the masks stay the truth the evaluators read. Generated masks are
`proposed`, with algorithms `zone` and `carried_body`, until the review grades them;
`annotation-split freeze` already refuses anything still `proposed`.

## Proving it on kirk0

Before the held-out window, a timed trial on kirk0's pack, in the development cohort, filed as a
spec with its rule written first. kirk0 is 83 s with 61 objects and a reviewed reference of 4,102
masks over 30 objects, and nobody timed it. The rule, for the owner to set, starting from:

- **Time:** the whole pack in 30 minutes or less, by the timing in piece 1; kirk0 has about half of
  label0001's object count at the same rate.
- **Agreement:** per reviewed object, median point IoU against the reference of at least 0.9 and no
  object below 0.7.
- **Exceptions:** the exception rules show at least 95 % of the frames where the reference's masks
  differ from what the layers produced.

If the time is met but the exceptions miss more than that, the review step shows every frame for
the objects that missed, and the hour grows; the report says by how much.

## Risks

- **A zone that swallows a road user.** A pedestrian standing under a tree for a minute, or a
  parked car inside a `persistent_voxels` patch. Bodies outrank zones, so the fix is a body; the
  risk is not noticing. The unresolved queue and the patch suggestions' class guesses are where it
  shows.
- **A rigid box on a body that is not rigid.** An articulated bus, a truck with a trailer, a person
  with a bicycle. These get more keyframes, and a failed snap is an exception.
- **A low road user in the ground band.** A child, a dog, a wheel. The band is thin and bodies
  outrank it; the snap test for lost catches a body that keeps losing its lowest returns.
- **Registration error.** Gerardus's pose for the site passed its placement check, but on walls
  within 30 m it scores below the moved pose on only 12 samples. Priors are drawn as context and
  suggestions; nothing labels from them without a person accepting the zone.
- **Rubber-stamping.** The grade per object is the whole saving and the whole risk. The exception
  rules are measured on kirk0, the audit sample is not optional, and the review records what was
  seen.
- **label0002 and fits from boxes.** A body's keyframes are the operator's, but snapped frames are
  fits to the returns. label0002 needs references that are not fits to the returns the tracker
  reads, so only keyframes the operator set by hand can serve as its references.

## What this does not establish

- The hour. It is an estimate from kirk0's counts until the trial times it.
- The window's road-user count. It is unknown until label0003 counts it.
- That exception review is as good as frame-by-frame review. The kirk0 trial measures one capture.

## Sequence

1. **Now:** label0003's spans by hand, as decided. Build timing (piece 1) so every later step is
   measured.
2. The ground surface, priors, zones and road-user space (pieces 2 to 5): the scene pass alone
   removes the static clutter from every frame.
3. Carried bodies and the bulk save (pieces 6 and 8).
4. Exception review (piece 7).
5. The kirk0 trial; the scorer (piece 9) only if class choices dominate its time.
6. label0001 on the held-out window, then label0002 from the operator's keyframes.

## Open questions

1. Does the resolver live only in the window, or also in Go so an agent can prepare a window
   overnight? The Go proposer is already a P2 row.
2. Are zones per window, or per site and reused by the next window at the same site?
3. Does the exception review satisfy label0001's `reviewed_fraction: 1.0` as the spec intends, or
   does the spec need to say how a mask is graded?
