# Deterministic scene capture and review harness

- **Status:** Planned
- **Layers:** L9 Endpoints, L10 Clients, offline evaluation tooling
- **Target:** v0.5.2-v0.6.x; staged delivery across milestones 1–5
- **Backlog:** [Release work items](../BACKLOG.md)
- **Related:** [Trail and uncertainty visualisation](lidar-visualiser-trails-and-uncertainty-visualisation-plan.md), [scene health metrics](lidar-visualiser-performance-and-scene-health-timeline-metrics-plan.md), [offline analysis tooling](lidar-offline-analysis-tooling-plan.md)

## Objective and boundaries

Give an agent a script that produces repeatable images of a recorded scene from
several supplied camera positions. The first useful result is three stills of the
same instant, with enough provenance to reproduce them. These images make trail,
point-cloud, and bounding-box alignment changes reviewable.

This document specifies future implementation. It does not deliver the harness,
change state estimation, or claim that an image difference establishes correctness.
An unchanged image can faithfully reproduce a wrong estimate.

Use the existing web scene renderer and existing scene exports first. Swift
capture, interactive target picking, and automated visual-review service selection
are outside the first milestones. Do not build a second renderer for the harness.

Before implementation, re-verify the current web scene reader/player/camera code on
the target branch and confirm the export format and existing trail behaviour.
Integrate with that work while preserving unrelated edits, rather than assuming
this plan’s working copy contains the required prerequisites.

## Delivery sequence

| Milestone | Release | Independently useful result                                                                                     |
| --------- | ------- | --------------------------------------------------------------------------------------------------------------- |
| 1         | v0.5.2  | Script accepting several coordinate-based views and producing stable stills, with deterministic optional trails |
| 2         | v0.5.3  | Bullet-time inspection around a frozen target                                                                   |
| 3         | v0.5.3  | Recorded-frame sequences, contact sheets, and optional animations                                               |
| 4         | v0.6.x  | Automatic VRLOG export and the 8081 Make web export workflow                                                    |
| 5         | v0.6.x  | CI artefact comparison and a visual-review integration decision                                                 |

Milestone 1 must be usable by a local or CI agent without waiting for hosted CI
integration, animation tools, or changes to recording workflows. Milestones 2 and
3 extend its capture interface. Milestone 5 follows practical use on a known
trail-alignment defect; it does not block the earlier releases.

## Milestone 1: agent-operated multi-angle stills

### Minimal input and output contracts

Accept a recipe file through a command-line script. Keep its documented structure
small; use the same capture preparation interface for the script and browser URL.

| Input          | Meaning                                                                                                          |
| -------------- | ---------------------------------------------------------------------------------------------------------------- |
| Source         | Existing local scene export, including its declared coordinate frame                                             |
| Frame selector | Exactly one timestamp offset or exported frame index                                                             |
| Views          | One to 50 uniquely named camera positions and look-at targets, each expressed as X, Y, Z; optional field of view |
| Layers         | Explicit LiDAR, boxes, and trails visibility; trail history defaults to five seconds                             |
| Viewport       | Width and height, default 1280 × 720; device-pixel ratio fixed to 1                                              |
| Output         | Destination directory and optional contact-sheet flag                                                            |

Coordinates use the export's scene frame before conversion into renderer
coordinates. Use the declared vertical axis for camera orientation. Reject a
camera coincident with its target, an undefined viewing orientation, non-finite
coordinates, or invalid field of view. Resolve a missing field of view from the
renderer once and record the result. Never automatically reframe individual views.

One recipe can contain three camera/target coordinate pairs to inspect the same
box from three sides. Every view resolves against the same frozen recorded frame.
Named camera presets may be conveniences later; explicit coordinates are the
first milestone's required interface.

The output is one named PNG per view and a manifest. Record source content hashes,
requested selector, resolved frame identity and capture timestamp, exact cameras,
layer settings, effective history duration, viewport, browser/rendering versions,
and code revision. Record working-tree dirt when present so a commit alone does
not falsely identify the renderer. Use UTC timestamps for machine metadata.

Offer a contact sheet with five columns, view names, and capture-time captions.
Images contain the scene viewport only; controls and the timeline stay outside
the image. Do not overwrite an existing capture set silently.

### Capture preparation and stability

The script serves the export locally and drives a pinned headless Chromium
version. Use a fixed rendering configuration and record it in the manifest.
Equivalent URL parameters must reproduce a single view through the same
preparation logic, rather than a separate URL-only render path.

The renderer-facing interface accepts the frame selector, exact camera, and
layers. Its completion result identifies the resolved frame and confirms that
requested assets and the final rendered state are ready. Fail on unavailable
requested LiDAR data instead of silently returning boxes without points.

Pause replay and disable flight, damping, transitions, and looping. A timestamp
selects the latest recorded frame at or before it. Reject out-of-range requests;
do not wrap. For equal timestamps, resolve consistently by export order and
report the selected index. Readiness waits have a bounded timeout and return
which stage failed.

After readiness, capture three PNGs 500 ms apart in wall time while capture time
remains frozen. Compare decoded pixels, not compressed PNG bytes. If all agree,
keep the first. Otherwise fail and retain all three with diagnostics. Stable
pixels are evidence of a frozen render, not proof of correct state estimation.

### Deterministic trail controls

Place a smaller **Trails** toggle below **Boxes** in the player. Trails default
on to preserve the existing presentation. Turning boxes off hides and disables
trails while retaining the preference for when boxes are enabled again.

Trail history defaults to five seconds of capture time, with a recipe/URL
override. Trails off requires no history pre-roll. When enabled, reconstruct
history from recorded samples rather than from whichever frames the browser
happened to render. Direct seeking, normal playback, and capture must share this
history rule. Preserve current trail placement initially; diagnosing or changing
the state estimate is separate work.

Scope track identity to its recording part. Do not connect trails across part
boundaries. At recording starts use available history and report its duration;
do not wait five seconds of wall time or invent missing samples.

### Acceptance

An agent supplies three named views and obtains three stable PNGs of one frame,
a manifest, and an optional contact sheet. Repeated runs match in the pinned
environment. A direct seek and sequential playback produce identical trail
samples at the same frame. CI service credentials are unnecessary.

## Milestone 2: frozen-scene camera paths

Generate camera views and pass them through milestone 1's capture interface.
Freeze the recorded frame, background, point cloud, boxes, and trail history;
only the camera changes. Select a target by explicit scene coordinates or an
exported object ID. Resolve its centre once; missing or ambiguous IDs fail.
Cluster IDs are accepted only when the export actually supplies them.

Use a fixed target, radius, and field of view. Derive radius from the chosen view
unless supplied explicitly. The default arc spans 180°, from −90° to +90°
relative to the chosen view's azimuth, centred on that view.

Vary elevation above the target's horizontal plane through **20°, 30°, 20°, and
10°** at normalised arc positions **0, ⅓, ⅔, and 1**. These are positive
elevations, not angles from vertical. Convert to the renderer's polar convention
internally. Advance azimuth uniformly and interpolate elevation with smoothstep
between keyframes without overshoot.

Default to 25 images including endpoints. Allow image count, radius, arc offsets,
and elevation keyframes to be overridden; enforce a maximum of 50 images and a
minimum of two for a path. Sample positions directly by index rather than by
browser timing. Keep the target centred without camera roll. Do not refit the
view, remove occluding geometry, close the partial arc, or reverse it automatically.

Record each exact camera pose, azimuth, and elevation with the unchanged scene
timestamp. Contact-sheet captions identify angle and frame. Reuse the readiness
and stability checks for each sample.

**Acceptance:** Scene geometry and timestamps remain identical, radius and target
stay fixed, and camera positions follow the requested path reproducibly.

## Milestone 3: temporal sequences and animation outputs

Hold the resolved camera fixed while advancing through consecutive exported
recorded frames, without interpolation. Start at a frame index or resolved
timestamp. Default to ten frames and cap at 50; reject sequences extending beyond
available frames. Reconstruct each frame's trail history as for an independent
still. Exported frames may already be subsampled: do not call them original sensor
frames unless the export provenance establishes that.

Always produce numbered PNGs and a manifest. Independent flags request a contact
sheet, MP4, GIF, or both animation formats. Keep five contact-sheet columns, with
timestamp captions for temporal sequences and angle captions for bullet time.

Temporal animations preserve recorded spacing, with an optional positive
playback-speed multiplier. Use the preceding positive interval for the final
frame, or 100 ms when none exists. Preserve equal-timestamp frames in the PNG
sequence; use the format's minimum representable duration for animation and
record the adjustment. Bullet-time animation defaults to ten images per second,
with an override. Document output-format timing quantisation. Require FFmpeg only
when an animation is requested; missing FFmpeg must not prevent still capture.

Save displayed track positions, box geometry, and trail samples beside the images
for agent inspection. These describe rendered evidence, not ground truth.

**Acceptance:** Image counts, captions, manifests, geometry records, and animation
order agree. A sequence frame matches an independent still of the same state.

## Milestone 4: automatic export and recording workflow

Accept VRLOG input and invoke the existing scene exporter before capture. Use
stride 1 for diagnostic exports, requested foreground point data, and sufficient
preceding trail history. Cache only when source hashes and export settings match.
Keep generated exports and captures local; generation does not publish a scene.

Add an unchecked **Make web export** checkbox to the 8081 PCAP replay/recording
workflow. It generates a web scene from the resulting VRLOG after successful
recording finalisation; it does not modify the source PCAP. Reuse the harness's
export path and settings rather than adding another conversion implementation.

Show export progress, output location, and a local-viewer link. Keep recording
and export outcomes separate. Export failure preserves the successful VRLOG and
allows retry of export alone. Do not start export against an unfinished recording.

**Acceptance:** Automatic and explicit export produce equivalent capture inputs;
export failure or retry does not damage or repeat the original recording.

## Milestone 5: CI review integration

Establish small versioned fixtures and recipes for stills, temporal sequences,
and bullet-time inspection. Initially publish before/after images, contact sheets,
manifests, and numeric geometry differences as review artefacts.

Evaluate Happo or an equivalent service before choosing hosted visual review.
Service selection, credentials, baseline approval policy, and associated workflow
configuration remain a separate milestone decision. Do not introduce those
dependencies into the local script merely to reserve the option.

Exact comparison verifies frozen rendering in the pinned environment. Differences
between code revisions need review; cross-platform pixel equality is not required.
Reproduce a known trail-alignment defect before using this harness to demonstrate
a fix. Use geometry/alignment evidence alongside images to judge improvement.

## Validation across milestones

- Repeated captures in the pinned environment produce identical decoded pixels.
- Direct seek, sequential playback, and independent capture agree on trail samples.
- Trails off skips trail-history loading; boxes off suppresses trails.
- Recording starts, chunk boundaries, equal timestamps, and part transitions resolve consistently.
- Delayed or failed asset loads, unavailable layers, invalid cameras/targets, and unstable rendering fail with useful diagnostics.
- Bullet-time keeps scene data fixed while changing only the camera along the specified path.
- Output counts, names, captions, timestamps, and animation order agree; the 50-image cap is enforced.
- Existing renderer work and unrelated checkout edits remain intact during integration.

Mark each milestone complete only with its own capture evidence and relevant
checks. Completing this document does not complete any implementation milestone.
