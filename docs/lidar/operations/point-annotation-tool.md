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

| Area                      | What it is                                                                                            |
| ------------------------- | ----------------------------------------------------------------------------------------------------- |
| Editing view (left)       | Orthographic. The only view that takes a selection. Its header says which object a stroke belongs to  |
| 3D view (top right)       | The main view's renderer on this frame. For looking, not selecting                                    |
| Check view (bottom right) | A second orthographic view on another axis. Review is gated on it                                     |
| Frame strip (bottom)      | One bar a frame: green agreed, amber in question. A yellow marker is a background update. Click to go |
| Pane (right)              | Status line, run and frame, progress, proposals, display, objects, tools, save and review             |

The status line at the top of the pane always says what went wrong or what to do next.

### Moving about

| Action                       | How                                                       |
| ---------------------------- | --------------------------------------------------------- |
| Zoom a selection view        | Scroll or pinch, about the cursor                         |
| Pan a selection view         | Right-drag, middle-drag, or control-drag                  |
| Orbit, pan, zoom the 3D view | Drag, shift-drag, scroll                                  |
| Frame the views              | **Fit**: Sample, Foreground, Selection; or click a sector |
| Step a frame                 | `,` and `.`, or the frame strip                           |
| Follow the main view         | **Follow the main view**, on by default                   |

The views hold their framing from frame to frame. Only a fit or your own pan and zoom moves
them. With **Follow the main view** on, stepping here seeks the main view to the same frame and
moving the main view steps here; the main view has to be replaying the same run.

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

Proposals are listed largest first. The small and short ones, which are mostly the speckle of
every frame, are behind a toggle.

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
| Column | Paints 0.5 m columns in the Top view, from the voxels switched on. Option subtracts                                                                                     |

The **depth slab** limits every tool along the view's depth axis. Set by hand, it is kept from
frame to frame until reset.

Stepping to the next frame lays the last frame's selection over it in cyan. Arrows nudge it
(shift for 0.5 m), Return accepts, Esc dismisses. For something that does not move, **Apply to
every frame** saves its mask into all of them.

⌘Z and ⇧⌘Z undo and redo selection edits.

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
- The web client has none of this. It keeps its existing track label CRUD.
