# Point annotation tool

How to label the returns that belong to one physical object and follow it through a recording,
in the macOS visualiser's Annotation window.

- **Status:** Implemented on macOS; the labelling itself is open work
- **Layers:** L9 Endpoints, L10 Clients, offline analysis
- **Canonical:** [lidar-point-annotation-and-object-dataset-plan.md](../../plans/lidar-point-annotation-and-object-dataset-plan.md)
- **Related:** [annotation package README](../../../internal/lidar/annotation/README.md), [Track labelling](track-labelling-ui-implementation.md)

## What it is for

The main view labels tracks, which are the tracker's account of the scene. This tool labels
points, which are the sensor's. An **object** here is one real thing followed through the
frames: this car, that wall. It is your answer and is kept apart from the tracker's tracks on
purpose, so that a track the tracker split in two cannot split the reference it is judged
against.

The unit of work is an object, not an object in each frame. Measured on a real twenty-second
pack there are about 43 clusters a frame and 8,600 object-frames, and labelling one at a time
took about six seconds each. So the tool proposes, carries and lets you review in bulk, and
painting by hand is for fixing what those get wrong.

### What review records today

Open a pack, correct which returns belong to each object, separate chains that join different
cars, and join fragments of the same car across frames. Review the object and its masks. These
actions establish membership, class, and identity independently of the tracker's account.

The current window has no physical-box editor: it cannot record an independent expected body
centre, yaw, full dimensions, or bumper endpoints. Its rectangle selects points; it does not
declare a car's physical box. Optional pose fields in the sidecar do not supply that interface,
and the present reference scorer uses a visible-mask position rather than a physical centre.

Physical geometry review is the
[0.5.2.0 reference workflow](../../plans/lidar-physical-reference-review-plan.md). Its storage and
independent import exist ([Physical references](#physical-references) below); the macOS editor for
placing a box at a keyframe does not yet. Until references are authored or imported and reviewed,
completing this point-review flow cannot close the physical box or bumper-gap accuracy gates. An
unseen bumper remains unknown unless independent evidence supports it.

## Words

The window uses the main view's words where it means the same thing.

| Word        | Meaning                                                                                       |
| ----------- | --------------------------------------------------------------------------------------------- |
| Run         | The recording the pack was cut from                                                           |
| Frame       | One scan. Shown by the run's own frame number, which is the one the main view's timeline uses |
| Pack        | An immutable excerpt of a run: the points a label can cite, fixed by a digest                 |
| Object      | One real thing, named by class and number: "car 2". Clicking one goes to its first frame      |
| Labelled by | Your name. Saved with every label. Not the name of an object or a track                       |
| Proposed    | Made or suggested by an algorithm, or saved and not yet reviewed                              |
| Reviewed    | You have checked it. Only a reviewed frame of a reviewed object is reference truth            |

## Getting a pack

1. Start the server with `make dev-go-lidar` and replay or record the run you want.
2. In the visualiser, **Annotation → Open Annotation Window** (⇧⌘A), then **Generate from Run…**
3. Choose the run and state its coverage. Recordings keep foreground returns and periodic
   settled-background snapshots, so choose **foreground only**. The exporter refuses **full**
   when every point is classed foreground, because a full scene always has background in it.
4. The first time, grant the packs folder when asked. It is remembered.

A pack carries the run's L4 height band and the settled background in force over the excerpt.
Packs cut before those were added have neither: generate them again. A re-export of the same
frames has the same pack digest, so its `annotations.json` and `annotation-revisions/` apply to
it unchanged.

### Choosing what to cut

An hour of labelling covers tens of seconds of a busy street, so the window matters more than the
speed. Open **LiDAR → Segments** at `/app/lidar/segments`, choose a run and a selector, then inspect
the score strip and ranked windows. **Preview in scene player** shows a tuning window with the
run's tracks. **Make case** fixes the capture and offset; **Queue clip** replays it with points and
exports the annotation pack. The page reads review counts from `annotations.json`. Labels and
review decisions still belong in the macOS window.

A selector is one way of choosing: following, close following, moving traffic, a random draw, or
one of the tracker-failure hunts. The list is grouped by category and each selector says what it
ranks by and what it requires. The selectors are defined in
[segment-selectors.defaults.json](../../../config/segment-selectors.defaults.json), which the
server reads when it starts; [SELECTORS.md](../../../config/SELECTORS.md) says how to add one.
Every window you make a case from records the selector that chose it, and so does its pack.

A run is ranked from wherever it kept its tracks. A live run stores track observations. A replayed
run does not, because a replay must not add to the live track store, so it is ranked from its own
recording. A run with neither has nothing to rank, and only the random finder offers it a window.

A chosen window outlives the run it was chosen from. Deleting the run clears the link and keeps
the window, its replay case and its pack, with the run's name still recorded beside them. Deleting
the replay case is what removes the choice.

A clip that fails or is cancelled leaves nothing in the annotation packs directory. If the server
stops while a clip is being cut, the job runs again when the server starts: it keeps the pack if
the pack was finished, and cuts it again if not. The job remembers its pack by a path inside the
packs directory and by the pack's digest, so the directory can be moved without losing the link.

For an evidence database, the Go finder produces a JSON report. Give it the capture path
when selecting at random, so an empty stretch remains eligible even if the tracker saw nothing:

```bash
velocity lidar segments --db "$LIDAR_EVIDENCE_DIR/<run>/observations/observations.db" \
  --source "$SOURCE_ID" --selector following --capture "$CAPTURE" --top 1 > "$SEGMENTS_JSON"
velocity lidar annotation-clip --pcap "$CAPTURE" \
  --start-seconds "$(jq -r '.windows[0].offset_seconds' "$SEGMENTS_JSON")" \
  --duration-seconds 10 --selection "$SEGMENTS_JSON" --output "$PACK_OUTPUT"
```

`--list-selectors` prints the selectors. The report records each 10 s window's score, capture
offset, finder version and parameters, and the selector that ranked it. Pass
`--segment-id` as well when the report contains several windows. The clip command uses the
report's selection and replay manifest to write `vrlog/`, `pack/` and `pack/segment.json` in one
new output directory. It refuses a selected window whose role, capture, offset or duration does
not match the command. The old `scripts/lidar-following-windows.py` remains a useful reference
for comparing following scores.

Decide a pack's role before choosing its window. Tuning packs may be chosen for tracker failure.
Held-out packs, Embarcadero's among them, use the standard following, moving-traffic or random
selectors at their own parameters, never one that hunts tracker failures; the page offers only
those, and a held-out request that sets parameters is refused. Choose a random window from each capture first. The Segments page
enforces that order before making a held-out traffic case. Blind review hides tracker previews.
The warm-up precedes the clip and is not exported; random windows start at least 35 seconds into
a capture with measured packet times so that this prefix is available.

When the macOS annotation window first opens a pack, it saves its moving and fixed suggestions
as proposal layers. They load again on the next open, while dismissals live in the sidecar. The
pack opens near the selected segment's peak sample. Proposals are suggestions, not reference
truth; only reviewed objects and masks count as such.

## The window

| Area                  | What it is                                                                               |
| --------------------- | ---------------------------------------------------------------------------------------- |
| Left sidebar          | What is being labelled: run and frame, this frame's progress, proposals, objects         |
| 3D view (top, large)  | The main view's renderer on this frame. For looking, not selecting                       |
| Top view (below left) | Large, because this is where a selection is usually made                                 |
| Four elevations       | Stacked to its right: Front and Back along Y, Side and Far side along X                  |
| Frame strip (bottom)  | One bar a frame: green agreed, amber in question. A yellow marker is a background update |
| Right sidebar         | How: display, tools, depth slab, carrying, saving and review                             |

Each elevation is the sensor looking outward and **shows only the half of the scene in front of
it**: 180° of azimuth each, the pairs opposed. Without that the half behind the sensor lands on
top of the half in front and two views along one axis are the same picture mirrored. Every
return is in front of exactly two elevations, one from each pair, so between them every side of
an object is on screen without orbiting anything. Back and Far side are turned around rather
than mirrored: a car driving right in Front drives left in Back. The axes are the sensor's own
— a pack records no site transform — so they are not compass directions.

The top view frames the whole sample. An elevation frames its **height** and is panned along,
because a street is a hundred metres wide and four high and fitting its width would leave a car
too small to see.

Only the editing view takes strokes, so there is always another view to check a selection in.
Changing the editing view drops a depth slab that was set along the old view's depth axis.

The status line across the top of the views always says what went wrong or what to do next.

### Moving about

| Action                       | How                                                       |
| ---------------------------- | --------------------------------------------------------- |
| Zoom a selection view        | Scroll or pinch, about the cursor                         |
| Pan a selection view         | Right-drag, middle-drag, or control-drag                  |
| Orbit, pan, zoom the 3D view | Drag, shift-drag, scroll                                  |
| Frame the views              | **Fit**: Sample, Foreground, Selection; or click a sector |
| Step a frame                 | `←` and `→`, `,` and `.`, or the frame strip              |
| Follow the main view         | **Follow the main view**, on by default                   |

The two bars across the top say how far through you are: this frame, and every frame in the
pack, as labelled, agreed and in question. The ring in the left sidebar says where in the frame
what is left is.

The views hold their framing from frame to frame. Only a fit or your own pan and zoom moves
them. With **Follow the main view** on, stepping here seeks the main view to the same frame and
moving the main view steps here; the main view has to be replaying the same run.

**Loop this pack's frames**, on by default, keeps a playing main view inside the pack: when it
runs past the pack's last frame, or off the end of the recording, it is sent back to the first.
Space plays and pauses the main view. A paused main view is left wherever you put it. While the
main view plays, this window follows as many frames as leave the 3D view responsive, and skips
the rest; paused, it follows every frame.

With the Annotation window in front, the Playback menu's bare keys mean this window: `,` and
`.` step its frames, and `[` and `]` size its brush. In the main window they are unchanged.

### What is drawn

| Colour                    | Meaning                                                       |
| ------------------------- | ------------------------------------------------------------- |
| Green, tan, grey          | Foreground, ground, background, as the main view colours them |
| The object's class colour | Saved points of an object, with its name above them           |
| Orange                    | Selected and not saved                                        |
| Red ring                  | Saved, and taken out since                                    |
| Cyan                      | Proposed: a carried selection, or a proposal being looked at  |
| Yellow                    | What the stroke or the brush under the cursor would take      |
| White                     | Settled background that the latest snapshot added or moved    |

**Ground** is exactly what the run's height band removed before clustering: below its floor or
above its ceiling, by the filter's own strict comparisons. Hiding it leaves what the tracker had
to work with. A pack that does not record its band uses the pipeline default and says
"assumed". A hidden class cannot be selected.

Display also switches the three states the progress bars count: **agreed**, **in question** and
**unlabelled**. Turning the settled ones off leaves the work still to do on its own — the saved
masks go with them — and since a hidden return cannot be selected, a lasso thrown over what is
left cannot take back what is already agreed. Agreed is reviewed or labelled by hand; in
question is saved but still the algorithm's word for it.

Hiding a _class_ is different: a mask is still drawn through it, because switching the
background off must not hide what a mask claims about it.

The **settled background** in force is drawn behind each frame. It is context: no tool selects
from it. Stepping forward onto a new snapshot shows a banner, and returns that moved by half a
metre or more are drawn in white.

## The quick way through a pack

1. Enter your name under **Labelled by**. It is remembered.
2. **Propose objects.** Fixed clutter comes back as one proposal a patch; everything else as one
   proposal an object, followed through the frames. Clicking one goes to its first frame.
   **Propose more** adds to the list: it finds only what nothing yet covers, never clears what is
   already there, and does not hand back anything you dismissed.
3. Click a proposal and step through its frames. Then:
   - **Accept** as a class, or **Noise** if it is not an object.
   - **Accept up to here** or **From here** to split a chain that ran from one car onto another.
     The rest stays proposed.
   - **Add to car 1** to merge it into the object being edited, which is how the two halves of
     a car that went behind a bus are joined. Frames the object already has are left alone.
   - **Dismiss** to drop it from the list.
4. Fix what is wrong by hand (below), then **Review all frames…** for the object.

Proposals can be listed by most frames, most points, furthest moved, steadiest return count
or earliest, and filtered by proposed type. Steadiest is the least lurching count from frame to
frame, which is the chain least likely to have hopped between things. The small and short ones,
mostly the speckle of every frame, are behind a toggle.

## Labelling by hand

1. Select the object's points in the editing view.
2. Choose a class and press **New object from selection**. Selecting first and naming second is
   the expected order.
3. **Save points** (**S** or ⌘S), or **Save and next** (**X**) to save and step to the next frame.
4. **Forward ▶▶** (⌘]) or **◀◀ Back** (⌘[) carries the mask through the frames on its own and
   saves them at once. It stops, and leaves you on that frame with the refused fit to nudge,
   where the object is lost, has doubled, could be in two places, is further than it could have
   moved, or takes in another object's points.

### Tools

| Tool   | Use                                                                                                                                                                     |
| ------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Lasso  | Drag an outline. Command-drag for a rectangle. Shift adds, option subtracts                                                                                             |
| Sphere | A paint brush. Click or drag. `[` `]` size it, shift-scroll moves it in depth, option subtracts. Before the button goes down it shows where it would mark in every view |
| Column | Paints 0.5 m columns in the Top view, from the voxels switched on. Option subtracts. `0`–`7` toggle a voxel, `↑` `↓` move the ground plane                              |

The **depth slab** limits every tool along the view's depth axis. Set by hand, it is kept from
frame to frame until reset.

Voxel 0 is the ground, so excluding the road while keeping the bottom of a car standing on it is
one press of `0`. Where that leaves kerb or tarmac in, or the wheels out, the ground plane is in
the wrong place rather than the filter being too blunt: `↑` and `↓` move it a twentieth of a
metre, or a quarter with shift. **Estimate** puts it back where a twentieth of this frame's
returns lie below it.

**Grid angle** turns the views and the columns together so they follow the kerbs instead of the
sensor's mounting: the top view's axes become the lattice's axes, so the grid sits square on
screen, and the four elevations cut along the street. Changing it re-frames the views, because a
framing kept from before is a position in the old view plane. It
is the scene's `grid_azimuth_deg`, and **this window is not where that value lives**: it belongs
to `tools/s2-archive/map-marks.json`, one per site, because it is a property of the street and
is shared by every site on the same grid. What the window keeps is a working value, remembered
for this pack on this machine. When you have it right, type the site id and press **Copy**: that
gives you the one line to paste into map-marks.json, the same way the scene viewer's own angle
panel does. Nothing reads the angle back from the scene yet — see Limits.

Stepping to the next frame lays the last frame's selection over it in cyan. Arrows nudge it
(shift for 0.5 m), Return accepts, Esc dismisses. While it is on screen it claims all four
arrows, so the frame cannot step out from under it. For something that does not move, **Apply to
every frame** saves its mask into all of them.

⌘Z and ⇧⌘Z undo and redo selection edits.

### Two objects that are one

Click the object to keep, then tick the merge arrow on each of the others and press
**Merge N into …**. Every frame of them becomes a frame of the first, and they are gone. Where
both have the same frame the returns are unioned: two masks over one thing are two accounts of
the same returns. Several at a time, because a chain that broke twice comes back as three.

This is the repair for a chain that came back as two objects because it went behind a bus, and
the undo for a split that should not have happened. Merged frames go back to proposed even where
both sides were reviewed — what was checked was two objects, and nobody has yet looked at the
one — so step through them before reviewing.

### One object that is two

1. Make the object the one being edited, on a frame where the two can be told apart.
2. Take the second one's points out of it (option-drag with a brush). They are ringed in red.
3. Choose its class and press **Split off as new object**.

That frame is divided as you divided it. The division is then carried both ways through the
frames the object is labelled in: the pair is followed together and each return goes to
whichever of the two it is nearer. It stops where the second object cannot be found as itself,
which is where the label only ever covered the first. Every frame it changes goes back to
proposed, so step through them before reviewing.

## What counts

Reference truth is a **reviewed** frame of a **reviewed** object, and nothing else. Saving does
not review. [Per-frame evaluation](per-frame-evaluation.md#the-reference) is where that rule is
applied: a mask it does not certify becomes an ignore point, neither found nor missed.

- **Review this frame** reviews the saved points on screen. **Review all frames…** reviews every
  saved frame of the object, which is how frames an algorithm filled in become agreed.
- Both need the second-view check ticked and a saved, unchanged frame.
- Changing a reviewed frame's points puts that frame back to proposed.
- A frame records how its points got there (`footprint_carry`, `cluster_chain`,
  `persistent_voxels`, or none for by hand), and keeps that through a review.

A held-out number needs a frozen split as well
([freezing a split](per-frame-evaluation.md#freezing-a-split)). Freezing refuses an object with a
frame still proposed, or a reviewed frame whose completeness was never stated, and pins the
revision it froze: review every frame of an object before putting it in a split.

The **This frame** ring shows the frame's foreground in sixteen 22.5° sectors laid out as the top
view is: agreed, in question, and not labelled. Click a sector to go to what is left in it.

## Physical references

A mask says which returns belong to an object. A physical reference says what is known about its
body: where its centre or a named face was, which way it faced, how long, wide and high it is, and
how each of those is known. The two have separate files and separate reviews. Reviewing a mask
never reviews a pose, and the sidecar's optional `pose` is not read as a physical reference.

References live beside the sidecar, in `physical-references.json`, with every earlier revision
kept byte for byte in `physical-reference-revisions/`. The store follows the sidecar's revision
protocol and takes the same `.annotations.lock`, so a physical save does not interleave with a
membership save by a writer that takes it. A save is refused if the base revision changed, and it
checks every link against the sidecar as it stands at that commit. Each revision has two digests:
one over its exact bytes, and a **content digest** over the references alone. Two revisions with
the same content share a content digest; that is what an evaluation cites.

A later membership edit can still invalidate a saved reference: rejecting its object, or removing
a mask a reference cites. Membership saves are not refused for that, because the macOS client
saves the sidecar without reading the references. Instead every load checks the links again and
lists what no longer holds, `validate` reports it, and the next save of references refuses until
those records are repaired or removed. Nothing is rewritten to mark them, so no file's bytes
change.

### The record

The document is `velocity.report/physical-reference`, schema version 1. Unknown fields are refused.

| Field                               | Meaning                                                                                   |
| ----------------------------------- | ----------------------------------------------------------------------------------------- |
| `pack_digest`, `dataset_id`         | The pack. Must match                                                                      |
| `source`                            | Sensor, VRLOG header and frame digests, coordinate and reference frame, transform, units  |
| `source.calibration_id`             | Optional calibration identity; evaluation holds each prediction to it                     |
| `objects[].object_id`               | An object the sidecar declares and has not rejected                                       |
| `objects[].body`                    | The body belief for the whole episode: `length`, `width`, `height`, axis convention       |
| `objects[].keyframes[]`             | One reviewed instant: sample, capture time, anchor, position, yaw, front and rear bumpers |
| `following[]`                       | A leader, `no_leader` or `ambiguous` decision over an interval, with gaps at instants     |
| `record_origins`                    | Every record ID ever held, with the origin it was created under. The store keeps it       |
| `revision`, `updated_utc`, `change` | Set by the store                                                                          |

Every component has a **status**: `observed` (the sensor saw it, in named frames), `inferred`
(from named frames or an external reference), `prior_only` (a named class prior) or `unknown` (no
value at all). Observed needs supporting frames; a prior must name itself. Bounds are conservative:
a dimension is `lower_m`/`upper_m` with an optional `value_m`, a position has a horizontal
`bound_m`, a yaw a `bound_rad`, a gap `lower_m`/`upper_m`. They say what the record's
`uncertainty_assumptions` state.

Observed claims are held to the returns. Every frame a record cites must hold returns of the
object in its mask (for a gap, of the parties it cites). An observed position, yaw or bumper at a
keyframe, and an observed gap or gap bumper, must cite its own sample. A claim the returns cannot
test is refused as observed; state it as inferred.

- **Span.** A dimension is `full` or `partial`. A partial span is an observation of part of the
  body and carries only `lower_m`. An observed dimension is measured in its cited frames: height
  from the returns, length and width along the axis of a keyframe at that frame. It is refused if
  no cited frame can measure it, or if the returns fall short of its lower bound by more than
  0.5 m.
- **Bumpers.** An observed bumper must be reached, to within 0.5 m, by the keyframe's own returns
  along its axis: at the anchor when the anchor is that face, and otherwise half the body's lower
  length from the centre. With neither a face nor a centre and a length, it cannot be observed.
- **Anchor.** A keyframe's position is the `body_centre`, or the centre of the `front_face`,
  `rear_face`, `left_face` or `right_face`. A face may declare `offset_m` and `offset_bound_m`, the
  distance from it to the centre; that offset must agree with the body's own bounds. A face with no
  offset locates the face, not the centre. The anchor is never a mask's centre.
- **Axis.** Yaw is the direction of the body's front (`x_front_y_left_z_up`). The axis is
  `resolved`, `front_rear_ambiguous` (known modulo half a turn) or `unknown`. Only a resolved axis
  may name a bumper or a face.
- **Shared errors.** `shared_errors` name an observation that several components rest on, such as
  one outline fit that places the centre and bounds the length. Anything derived from several
  components is bounded by adding their bounds, which holds however their errors are correlated.
- **Following.** A gap is `along_follower_axis`: the leader's rear extreme minus the follower's
  front extreme, projected onto the follower's body axis. For aligned cars these are the bumpers.
  It is a straight chord, not the along-path headway arc. A gap is no better known than the weaker
  of its two bumpers, and a bumper no better known than its party's keyframe at that sample says:
  a party whose axis is unresolved there has no named bumper, and an observed bumper needs the
  party's keyframe there. Two reviewed records for one follower that overlap must agree on the
  decision and leader, and cannot both give a gap at one sample; a proposal may disagree with a
  reviewed record.
- **Review.** Each body, keyframe and following reference has its own `review`: status, origin,
  method, author and uncertainty assumptions. A `tracker_assisted` record names the tracker output
  it came from, and never becomes `independent`: not by review, not by editing its origin, not by
  deleting it and adding it back under its old ID. Only a reviewed, independent record is truth.

A keyframe covers its own sample and nothing else. Frames between keyframes have no reference
until someone reviews one there.

### Authoring in the window

Physical references are authored in the annotation window, in the **Physical reference** mode at
the top of the right-hand column. The window never writes `physical-references.json` itself: it
sends each edit to the local server, which applies the rules below, and shows what the server
answers. The server must hold the same pack folder the window opened, under its
`--lidar-annotation-dir`. A pack opened from anywhere else can be viewed, but not saved.

Entering the mode pauses the link with the main view. The main view draws the tracker's boxes, and
a reference is independent only if its author has not seen them, so keep the main window's boxes
out of sight while authoring.

1. Choose the object in the list. Membership and identity come first, in Points mode.
2. **1 · Object size · all frames** holds one length, width and height for the whole episode. A new body
   is unknown in every dimension; nothing is prefilled. Each dimension has a status (observed,
   inferred, prior only, unknown), a full or partial span, bounds in metres, and the frames or
   external reference it rests on. Enter a whole span as **value ± tolerance**: the tolerance
   states how far either way it could be wrong. **Exact min/max (advanced)** preserves
   asymmetric bounds. A partial span uses **at least**, a lower bound only. A value without
   bounds can draw an incomplete sketch; it is not supported geometry for scoring.
3. **2 · Pose at this frame** holds this frame's pose. Click in the Top view to place its
   position, or type X and Y. Then state the horizontal bound yourself: placing a point does not
   say how well it is known. Set the axis state, then the yaw and its bound in degrees. A named
   face or bumper needs a resolved axis. A click in an elevation sets only the optional height,
   and only after the position exists.
4. Give each record its method and uncertainty assumptions. The server checks the draft as you
   edit and says what it would refuse, and which reviews a save would reset.
5. **Save proposal** (**S**) saves the draft as a new revision; **Save and next** (**X**) saves it
   and steps to the next frame. A save never reviews anything. A changed
   body dimension is a new body under a new ID, and every keyframe of that object returns to
   proposed.
6. Inspect the saved record in both views, then **Review body** and **Review saved pose**,
   separately. Review is refused while the draft has unsaved changes, because it confirms the
   saved record.

The object list shows three separate progress lines: membership frames, the body's review, and
keyframes saved and reviewed. A pose is a keyframe: one marked instant, with no interpolation
between it and another. Use **Poses: N marked frames** to return to a recorded pose after saving or
discarding current edits. The overlay immediately draws an editable marker after placement, even
before its bound is entered; **bounds not set** marks that sketch as incomplete. A missing bound
is never written as zero or used for scoring. The supported geometry summary still requires
bounds. The overlay draws a marker and its bound for a supported position, an arrow for a
resolved axis, an unsigned line for an ambiguous one, and a
box only when centre, yaw, length and width are all known. It labels each reference as unsaved,
proposed or reviewed, and uses a different line style for each.

Unsaved physical work guards stepping, switching objects, opening another pack and closing the
window, as unsaved membership does. Undo and redo apply to the draft only. After a conflict,
**Reload, keep draft** rereads the stored references and keeps your draft to reconcile against
them. If a save gets no answer, the window will not save again until it has reloaded, because the
first save may have committed. **Reload** reads the saved physical document and reports the
revision or explains that none exists. It does not create a body, import a tracker box, or discard
unsaved object membership.

More in the Physical column:

- **Drag handles** in the Top view: drag the square to move the anchor with the size and yaw held,
  the dot to turn it with the position held (the first turn creates a front/rear-ambiguous axis),
  and the diamond at the far end of the body to revise its length with the anchor held. The
  outlined square labelled **width** revises width about the supported centre, keeping position,
  heading and length fixed. A side-face anchor has no width handle: its inward offset would need
  an explicit coupled edit. Resizing carries the dimension's stated interval with its value and
  is a body change: saving it makes a new body and returns every keyframe of the object to
  proposed. A drag is one undo step.
- **Copy keyframe from sample N** starts this frame's keyframe from the nearest one. The copy is
  a proposal: whatever the source observed becomes inferred from the source's frames, and its
  origin stays the source's. Check it against this frame's returns before claiming anything
  observed.
- **Shared errors** name one observation several components rest on, such as one rear-face fit
  that placed the anchor and bounded the length.
- **Stale records** after a membership change (a rejected object, a removed mask) can be removed,
  or moved to another object after a merge. Moving refuses when the target already has a body or
  a keyframe at the same sample.
- **History** lists every saved revision. **Restore** makes an old one current as a new revision;
  the current one stays in history.

A review records the membership revision it was made against. If membership later changes in a
frame the record rests on (its own frame, or any frame it cites), the column lists it under
**Reviewed, but membership changed since**, and it can be reviewed again. The link checks alone
would miss this, because every link can still hold while the points a person judged are
different.

Membership saves refuse to make one return belong to two objects in a frame, because Go's tools,
the physical service included, refuse a pack where one does. A pack that already has such returns
lists them at the top of the object column, with a button to go to each frame, and can still be
saved so they can be repaired.

### Comparing with an estimate

**Compare** mode opens a per-frame evaluation report run with `-physical-reference` (see
[per-frame evaluation](per-frame-evaluation.md#physical-references)) and shows, at each frame, the
reference the report scored beside the chosen arm's estimate. The window writes nothing while in
this mode.

- The report must be for this pack. The column shows its split, the membership revision and the
  reference revision it scored, and whether that revision is still kept with the same bytes. If
  the references have moved on since, the report is still shown as it was scored. It is never
  mixed with the current references; run a new evaluation to compare those.
- For each object at this frame it shows the matched track, the centre, yaw, dimension and end
  errors against the reference bounds, and why any component was not scored. It also compares
  the reference's movement since the previous frame with the estimate's, so a real turn is not
  read as jitter. A medoid or visible-return centre is labelled as that, never as a body centre.
  Following instants at the frame show the reference gap, the estimate's gap and the error. In
  the elevations each layer is a vertical at its own planar position: the report records no
  heights.
- A reviewed record whose membership changed after its review is shown by the evaluator and not
  scored, with its own reason, and the report's caveats name it. Review it again against the
  scored membership and run a new evaluation.
- **Seeing is recorded.** Once an estimate for an object has been shown, any later edit of that
  object's reference is saved as tracker-assisted, naming the estimate, in any mode. A record
  saved as independent is not relabelled. The edit becomes a new record, and the independent one
  stays in history. This is kept per pack across launches, and cannot be cleared from the window.
- A pack whose `segment.json` role is `held_out`, or a report of a held-out split, is not
  compared.

### Freezing from the window

**Freeze Split…** (beneath the editing column) freezes a reviewed split through the service. The
window does not author the split: choose a draft file in the CLI's format
(`velocity.report/split-draft`), naming each pack by its folder beneath the service's annotation
folder, as the physical-reference service names packs. **Preview** shows what the service would
pin: for each pack its membership revision and digest, its physical revision and digests (or that
it has no physical references, in which case the split pins membership only), each object's
partition, each object's body and keyframe review, the components that reviewed keyframes leave
unavailable, and every problem that stops the freeze. **Freeze** is enabled only when the preview
is of the chosen draft and says it would freeze; it writes the split once, under the name you
give, into `splits/` beneath the annotation folder, and refuses to overwrite. A new revision of an
existing split names it under **Supersedes**. The same rules refuse a freeze on the command line;
the button is not where they are enforced.

### Importing and validating

An independent reference measured elsewhere comes in through an import file,
`velocity.report/physical-reference-import` version 1. It holds `pack_digest`, `dataset_id`,
`source`, `objects` and `following` exactly as the record does, without the fields the store sets.
It is merged into the current references, and the merged document passes every check a save
makes, including the stored origin ledger and the links, before it is saved as a new revision. It
is never checked on its own, since it may add a keyframe to a body already stored:

```bash
velocity lidar annotation-reference validate --pack "$PACK" --file references.json
velocity lidar annotation-reference import --pack "$PACK" --file references.json --author "$NAME"
velocity lidar annotation-reference validate --pack "$PACK"
```

`validate --file` is a dry run of that import, `--replace` included, and writes nothing: what it
passes, the import passes against the same stored state. An import that would replace a body, a
keyframe at the same sample or a following reference is refused unless `--replace` is given.
`validate` on its own checks the stored references against the current annotation and names every
record that no longer holds; on a pack with none it says so. `validate --revision N` checks a
retained revision. Each command prints the revision, the digests and the counts, including how
many keyframes are reviewed and independent.

### Scoring against physical references

The [per-frame evaluator](per-frame-evaluation.md) scores two estimate versions against the
reviewed masks at a visible-mask position. That comparison is for identity; it is not a body-centre
accuracy score and is not relabelled as one. `-physical-reference` adds a separate physical score
for each arm against these references:

```bash
bin/lidar-ground-truth-eval perframe -pack "$PACK" -split-manifest "$SPLIT" -split tuning \
  -allow-tuning-split -physical-reference \
  -a-label near_edge -a-db "$DB" -a-param-hash "$PARAMS" -a-stage online -a-declared-baseline -a-solid-body \
  -b-label medoid -b-db "$DB" -b-param-hash "$PARAMS" -b-stage online -b-declared-baseline \
  -json physical.json -markdown physical.md
```

| Flag                             | Meaning                                                                        |
| -------------------------------- | ------------------------------------------------------------------------------ |
| `-physical-reference`            | Score each arm's centre, yaw, dimensions, bumpers, box and following gap       |
| `-physical-reference-revision N` | Score a retained revision; the default is the current one. Both are recorded   |
| `-physical-gate-metres M`        | How far a prediction's point may be from the reference centre, or anchor (3 m) |

A prediction matches a reference by source (every row's sensor, and one calibration: the
reference's, if it names one), by capture instant (the frame tolerance), in the pack's frame, and
by object: at each sample, reviewed keyframes and predictions pair one to one, nearest first,
within the gate. Objects the split manifest puts in another split take no part, so another split's
references never change this one's outcomes; a prediction of such an object, like one of an
untracked neighbour, may then match a scored object inside the gate.

A component is scored only where the reference has it, reviewed and independent, with its bound.
A derived bound adds its inputs' bounds and the chord a yaw bound sweeps with its lever arm. Each
error is reported beside that bound. Only a body-centre prediction scores the centre: a medoid or
a visible-box point is counted as a missing prediction, and its distance from the body centre is
reported only by prediction reference. A signed front or rear needs a scorable yaw, and an
ambiguous axis gives an axis error and an unsigned comparison of both ends, never a signed front or
rear error. A box overlap needs a complete box on both sides. The gap is the behaviour layer's
projected footprint gap, along the reference follower's axis. Where that keyframe has no resolved
axis, the gap is counted as unknown geometry (`follower_axis_unavailable`) rather than measured
along the prediction's own heading; so is a gap whose leader the episode does not score
(`leader_outside_episode`) or has no keyframe for (`no_leader_keyframe`).

Every object of every episode is expected at every sample where it has a mask or a keyframe, and
every follower once at each sample its following records cover, whichever record speaks for it. Each
expected instant is **scored**, or counted, per component, as **unknown geometry** (no keyframe
there, not reviewed, tracker-assisted, or the component unknown, a prior, a lower bound only),
**unmatched** or **missing prediction**, with its reason. A reviewed `no_leader` or `ambiguous`
decision is counted as **not following**. Each per-instant record keeps the reference layer, the
prediction layer and the comparison apart, and names the reference revision and the estimate
version, so an inspector showing it cannot pair one instant's reference with another's estimate.
Consecutive scored centres also report how far each layer moved: a step at a face change that the
reference does not make shows up there.

Physical scoring refuses a held-out split. Its error limits, reference precision and coverage have
to be pinned on tuning data first, and no record of them exists yet. Leader choice is not scored,
and the along-path gap waits for persisted paths.

## Feature candidates

Choose **Feature Candidates** in the editing-mode menu after saving the active object's membership
in **Object Points**. Choose **Sphere** to click a return, or **Lasso subset** to draw an exact
subset through the current depth slab. Shift adds to the draft subset; Option removes from it.
Both tools select only that object's saved, definite returns. It cannot add or subtract object membership. Orange rings mark an unsaved
feature preview; cyan marks saved support. The mode, feature ID, method and proposal origin remain
visible. The companion 3D view supplies object context; feature rings are in the orthographic views.

1. Enter **Labelled by**, choose the object, and select **New facet**. Click a return in either
   orthographic view. Dragging pans rather than painting object points.
2. For a sphere, adjust its radius; it starts at **0.20 m**, a **0.40 m diameter**. Check top and elevation
   views. The selected returns must be saved definite members of this object, with conflicting
   or uncertain object claims excluded. A lasso retains exact selected indices; its enclosing
   sphere is a selection envelope, and resizing does not broaden it. Different features may
   overlap, so their support must not later be counted as independent evidence.
3. Name the feature and choose unknown, edge, corner, patch or protrusion. A semantic hint such as
   “wing mirror” is optional. These are descriptions, not recognition presets: the tool does not
   fit a right-angle corner, recognise a headlight or detect a wheel-well recess.
4. **Save facet proposal** retains a proposal under a persistent feature ID. **Reject**, **Missing**
   and **Occluded** retain decisions without measured support. Missing/occluded can be recorded for
   an existing feature even when this frame has no return to click. **Cancel / stop** drops only
   the unsaved proposal. No feature action reviews a physical pose or changes an object mask.
5. **Preview next frame** moves to one consecutive source frame and proposes fresh return indices
   inside that same object's saved domain. Inspect, edit the sphere, accept, reject or stop before
   continuing. It does not save automatically or jump across gaps. A weak/ambiguous fit, changed
   membership or already annotated next frame stops it; seed that frame manually if appropriate.
6. To revise saved support, select the feature and use **Edit this frame**, then click or resize
   and save. Names/types/hints are saved with that frame edit. Reopen the pack and select the same
   ID to inspect its saved decisions. **Edit this frame** preserves a lasso's exact support.
   Historical revisions retain the earlier interpretation. At most four facets can be active for
   one object. **Retire facet, keep evidence** frees a slot without deleting observations; a fifth
   activation is refused. Zero active facets is a supported abstention.

The proposal is deliberately limited: it uses observed-object centroid translation and a small
local footprint search, with 0.25 m voxels. It does not estimate rotation or establish that every
selected return belongs to the same physical surface. Nearby competing fits are checked at the
feature scale. Changing visibility can shift the observed centroid; reject an implausible result.
A missing or hidden feature is not a new measurement. General feature recognition, directed
front/rear placement, part motion and full rigid registration remain unfinished. The compact
feature path below can store a horizontal body registration proposal; unresolved relations stay
explicitly unresolved.

**Geometry check · proposal only** reports an initial line or surface fit, its span, transverse
spread and residual. A line needs at least three returns and sufficient span; a surface needs
non-collinear support in two directions. The initial sampling floors are 0.10 m span and 0.05 m
surface spread, with a covariance ratio at most 0.10; they are experimental support checks, not
calibrated sensor uncertainty. A scan boundary can still look like an edge. A surface leaves its
two tangent directions weak, and its visible centre must not become a fixed body anchor. Compact
features require a repeatability check across frames. These diagnostics neither review a facet
nor establish a rigid relation.

### Registering a compact feature to the body

For a repeatable **Corner** or **Protrusion**, first save accepted feature support in two frames.
Review an independent body and a resolved physical pose at the source frame against the same
saved membership. In **Body registration · proposal**, select the exact physical spot with
**Fixed return**. Alternatively, enable **Inspect returns**, hover that spot, press **M**, then
choose **Use pinned return**. The chosen return is yellow in the orthographic views. Name what
makes it repeatable, such as the outer rigid mirror tip, and enter a positive return uncertainty
in metres before choosing **Register to saved body**.

The stored coordinates are metres forward and left from the pinned body centre. They are
horizontal only; height is unresolved, because physical schema v1 has no vertical uncertainty.
The conservative bound adds the source position bound, the yaw lever-arm displacement and the
stated return bound. The Go writer rechecks the coordinates, definite point support, membership,
body/keyframe IDs and exact physical revision digest. A missing or stale review, ambiguous axis,
absent source point or understated bound is refused.

Changing body dimensions does not rescale the stored offset. A later physical revision is shown
as needing a recheck, while the mapping retains its original body-frame pin. **Detach
registration, keep facet** removes the current mapping while preserving its revision history;
do this before changing the registration's source support. These are annotation proposals seeded
from references, never independent physical truth or online tracker resets. A folding mirror
must not be treated as rigid support. Edge and surface point anchors are refused: those need a
constraint that preserves weak tangent directions, rather than a moving visible centroid.

The local Go annotation service owns feature saves, using the same confined pack root as physical
references. It must serve the same folder the window opened, not another copy with matching bytes.
No Internet connection is required. Without the local service, a supported `feature-proposals.pb`
can be inspected read-only. Authoritative saves go through the shared recording-domain protobuf,
revision tokens, annotation lock and exact-byte history in `feature-proposal-revisions`.

If a save is not confirmed, the draft remains and further writes are disabled. Cancel it and
**Reload** to inspect what the service actually committed before retrying. Do not assume a lost
response means nothing reached disk. Changed membership stops propagation; reconcile and reseed
against its new revision. Old support keeps its original membership pin and is not silently
reinterpreted. Proposals are not independent reviewed references, and the held-out scoring gate
is unchanged.

## Raw intensity

**Intensity (raw 0–255)** in the right-hand column is independent of the modes. It writes nothing,
and marks nothing unsaved.

- **Reflectivity colour** is off by default. On, it colours the returns in the orthographic views
  and the 3D view from one 256-entry table, so a code has the same colour in both. Measured zero
  has a fixed swatch; the gradient covers 1–255, and unavailable intensity has its own colour.
  The range, ramp,
  contrast and brightness only change how returns are drawn. A nonzero code outside the range is drawn in
  the end colour, and the column counts how many were clamped in this frame. Off restores the
  class colours, without the intensity brightening the main view uses.
- **Distribution (experimental)** shows a 16- or 32-bin histogram of the chosen object's saved
  mask at this frame (definite members; uncertain points are counted as excluded), of the unsaved
  selection, or of the chosen proposal. A proposal is the proposer's own clustering of the pack's
  points, with every member recorded, so it is exact membership; it is an algorithm's suggestion
  and not a review, and not the tracker's output. It also shows up to four peaks by a fixed rule, `intensity-peaks/v1`: each
  with a centre and width (the mean and spread of the raw codes in it), a support count and a
  fraction of all measured returns. A peak is not a surface or a material, and changing the colour
  range does not change it.
- **Inspect returns** reads out the return under the cursor: its stored byte, point index, sample
  and position. **M** pins it; **N** steps to the next return under the same place. The readout
  is the stored byte, whatever the colour settings.

A pack declares intensity for the whole pack, and packs cut since presence was recorded also say
it per frame (`has_intensity` on each sample). If a frame's source carried no intensity column,
its stored bytes are zero-fill: the readout says unavailable and every return is drawn in a
distinct "not measured" colour, whatever the pack-wide flag says. A pack cut before that was
recorded has only the pack-wide flag, and the column says that a frame the source did not measure
would read as zeros. The code is the sensor's raw byte, not a calibrated reflectance.

## Revision history

Every save archives the exact bytes it replaced as a full snapshot, so the history grows by one
whole document per save, however small the edit. A pack of 3,700 masks writes about 25 MB each
time: one labelled kirk0 pack reached 1,968 revisions and 38 GB in a day and a half. The history is
the recovery path, and a frozen split pins one revision by the digest of its exact bytes, but the
window never reads it back, so most of it is dead weight.

`lidar-annotation-prune` plans first and writes nothing:

```bash
go run ./cmd/tools/lidar-annotation-prune -pack "$PACK" -keep-last 200 -keep-every 1h \
  -split splits/kirk0-split-r1.json
```

It keeps the newest `-keep-last` revisions, the oldest, one revision per `-keep-every` interval of
what is older (by when each was archived), and every revision a `-split` file or `-protect N`
names. Splits are read for this pack's pinned revision only. To prune, add `-apply` and a
`-backup` path:

```bash
go run ./cmd/tools/lidar-annotation-prune -pack "$PACK" -keep-last 200 -keep-every 1h \
  -split splits/kirk0-split-r1.json -apply -backup /Volumes/lidar/backups/kirk0-revisions.tar.gz
```

Nothing is removed until the removed revisions and the current snapshot are in a compressed tar
with a SHA-256 manifest, and the tar has been read back and every member's digest matches the one
taken from the pack. The removal then runs under the pack's writer lock, so the Annotation window
can stay open: its saves archive revisions newer than any being removed. A backup that does not
verify, a busy lock, a full volume or an archive that changed since the plan leaves that file, or
the whole pack, as it was. To restore, `tar -xzf BACKUP -C "$PACK"`; the current snapshot comes out
under `head/` and does not replace the live one.

A pruned revision cannot be loaded or restored by number, and the store says so by name; the head
and every kept revision are unaffected. Prune after freezing a split, and pass the split, so the
revision it pins stays.

## Limits

- The point domain is what the recording kept: foreground. Background is shown and cannot be
  labelled.
- The proposer clusters the pack's points itself. It does not read the run's clusters or tracks:
  a recording keeps cluster boxes and not which returns were in them, and the tracker's
  identities would bring its fragmentation with them.
- Class guesses come from size and distance travelled only. The class you choose is what is
  saved.
- Every save writes a full snapshot to `annotation-revisions/`.
  [`lidar-annotation-prune`](#revision-history) thins them; nothing does so on its own.
- **The grid angle is not yet synced from the scene.** A pack records the sensor, the capture
  and the run, and nothing that says which junction it stood at, so the angle cannot be looked
  up; and no server reads `map-marks.json`. The channel that should carry it already exists —
  `CoordinateFrameInfo.rotation_deg` in the visualiser proto, "rotation of X-axis from East",
  which reaches the macOS client and is never populated. Filling that in, and recording the site
  in the pack, is what would close the loop.
- The web client has none of this. It keeps its existing track label CRUD.
- Following references are read and kept, but not authored, in the window. Width and height are
  set numerically; only length has a handle.
- Compare opens a saved report; it does not start an evaluation.
