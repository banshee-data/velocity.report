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

## Words

The window uses the main view's words where it means the same thing.

| Word        | Meaning                                                                                       |
| ----------- | --------------------------------------------------------------------------------------------- |
| Run         | The recording the pack was cut from                                                           |
| Frame       | One scan. Shown by the run's own frame number, which is the one the main view's timeline uses |
| Pack        | An immutable excerpt of a run: the points a label can cite, fixed by a digest                 |
| Object      | One real thing, named by class and number: "car 2"                                            |
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

The **settled background** in force is drawn behind each frame. It is context: no tool selects
from it. Stepping forward onto a new snapshot shows a banner, and returns that moved by half a
metre or more are drawn in white.

## The quick way through a pack

1. Enter your name under **Labelled by**. It is remembered.
2. **Propose objects.** Fixed clutter comes back as one proposal a patch; everything else as one
   proposal an object, followed through the frames.
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
3. **Save points** (⌘S).
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

**Grid angle** turns the columns so they follow the kerbs instead of the sensor's mounting. It
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
not review.

- **Review this frame** reviews the saved points on screen. **Review all frames…** reviews every
  saved frame of the object, which is how frames an algorithm filled in become agreed.
- Both need the second-view check ticked and a saved, unchanged frame.
- Changing a reviewed frame's points puts that frame back to proposed.
- A frame records how its points got there (`footprint_carry`, `cluster_chain`,
  `persistent_voxels`, or none for by hand), and keeps that through a review.

The **This frame** ring shows the frame's foreground in sixteen 22.5° sectors laid out as the top
view is: agreed, in question, and not labelled. Click a sector to go to what is left in it.

## Limits

- The point domain is what the recording kept: foreground. Background is shown and cannot be
  labelled.
- The proposer clusters the pack's points itself. It does not read the run's clusters or tracks:
  a recording keeps cluster boxes and not which returns were in them, and the tracker's
  identities would bring its fragmentation with them.
- Class guesses come from size and distance travelled only. The class you choose is what is
  saved.
- Every save writes a full snapshot to `annotation-revisions/`, and nothing prunes them.
- **The grid angle is not yet synced from the scene.** A pack records the sensor, the capture
  and the run, and nothing that says which junction it stood at, so the angle cannot be looked
  up; and no server reads `map-marks.json`. The channel that should carry it already exists —
  `CoordinateFrameInfo.rotation_deg` in the visualiser proto, "rotation of X-axis from East",
  which reaches the macOS client and is never populated. Filling that in, and recording the site
  in the pack, is what would close the loop.
- The web client has none of this. It keeps its existing track label CRUD.
