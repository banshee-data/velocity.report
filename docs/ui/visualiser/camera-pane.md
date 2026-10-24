# Camera pane

The visualiser's camera controls: orbit the 3D view's camera round an ellipse, or fly it freely, with one speed curve shaping both; measure the ellipse from the camera, the scene or two clicks; and keep viewpoints to fly back to.

- **Code:** [CameraPane.swift](../../../tools/visualiser-macos/VelocityVisualiser/UI/CameraPane.swift) (the pane), [CameraControlSession.swift](../../../tools/visualiser-macos/VelocityVisualiser/App/CameraControlSession.swift) (its state), [CameraPaths.swift](../../../tools/visualiser-macos/VelocityVisualiser/Rendering/CameraPaths.swift) (the geometry), [CameraEasing.swift](../../../tools/visualiser-macos/VelocityVisualiser/Rendering/CameraEasing.swift) (the curves)
- **Tests:** [CameraPaneTests.swift](../../../tools/visualiser-macos/VelocityVisualiserTests/CameraPaneTests.swift)

## Opening it

Press **Camera** in the main toolbar, or choose View ▸ Camera Pane (⌥⌘C). The pane opens as a column beside the 3D view, and is there whether or not velocity.report is connected. Closing it stops an orbit (putting the camera back) and any flight.

The pane has two modes, **Orbit** and **Fly**, chosen at its top. The speed curve, the viewpoints and the settings note are shared by both.

## Orbit

The camera follows an ellipse round a centre, looking at a point above the centre.

| Control    | What it sets                                                                                                                       |
| ---------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| Centre     | The camera's target (the default), the selected object, or the site origin. Fixed at Play until Stop                               |
| A (along)  | The radius along the heading, metres: a field and a slider                                                                         |
| B (across) | The radius across it, at heading + 90°                                                                                             |
| Lock ratio | Editing one radius scales the other with it                                                                                        |
| A − B, A/B | The difference and the ratio, editable: either keeps A and moves B                                                                 |
| Heading    | A's direction, compass degrees clockwise from north                                                                                |
| Start      | Where Play begins on the ellipse, degrees from A's end towards B's                                                                 |
| Height     | **Height** above the centre, or **Elevation**, the angle up from the look-at point, so the camera rises where the ellipse is wider |
| Look at    | The height above the centre the camera looks at                                                                                    |
| Lap        | Seconds for one lap                                                                                                                |
| Direction  | Clockwise or anticlockwise, seen from above                                                                                        |
| Repeat     | **Loop** lap after lap, or **Once** round and stop                                                                                 |
| Show path  | Draws the ellipse in the 3D view as a thin amber line, with a white tick down from where Play begins                               |

North is the site's north, carried into the sensor frame the view is drawn in, once a sensor is placed in the [alignment pane](gerardus-alignment.md); before that it is the sensor's +y. With no object selected, Selected object goes round the camera's target; with no site placed, Site origin goes round the sensor. The pane says so under the picker.

**Play, Pause, Stop.** Play takes the camera and starts at phase 0; Pause holds it where it is; Play again resumes. Stop ends the orbit and puts back the camera from before Play. A single lap that has ended stays where it ended until Play (round again) or Stop. Any orbit, pan or zoom by hand in the 3D view, or a camera key, pauses playback: the view is the operator's again at once, and Play goes back to the path. Settings edited while paused move the camera on the path at once.

### Measuring the radii

| Button          | How                                                                                                                                                                                                                                                                                                                                                                                    |
| --------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| From camera     | The camera's horizontal offset from the centre is projected onto A's and B's axes, (pa, pb). Keeping the current A/B ratio k, the ellipse through that point has A = √(pa² + k² pb²) and B = A / k, and the start angle is set to where the camera lies on it. The height (or elevation) is the camera's too, so the orbit starts exactly where the camera is. Refused over the centre |
| From scene      | Fits A and B to the extent of what is drawn round the centre: the registered scans, or the placed reference's DataSF boxes and kerbs when it has no scans. Along each axis it takes the 99th percentile of the points' distances from the centre, so a few strays do not stretch it, and adds **Margin** (10% by default). Needs a placed reference                                    |
| Pick two points | The next click in the 3D view sets A's end (A's length and the heading), and the one after sets B's end (its distance across A's axis). Each click is unprojected through the camera onto the level ground at the centre's height, as the street painter's clicks land on its plan. Escape or **Cancel pick** stops                                                                    |

The measured values fill the fields. After an edit the pane shows the measured value beside the field, and by how much it was changed.

## Fly

Press **Fly** to take the camera; press it again to let go, leaving the camera where it flew to.

| Input             | Moves                                         |
| ----------------- | --------------------------------------------- |
| W, ↑ / S, ↓       | Forward and back along the view               |
| A, ← / D, →       | Left and right, level                         |
| Q / E             | Down and up                                   |
| ⌥ and mouse       | Looks round: yaw and pitch, short of vertical |
| Drag, scroll, R … | As outside flight: orbit, pan, zoom, reset    |

The keys reach the 3D view when it has the keyboard; pressing Fly gives it the keyboard. Keys held with ⌘ or ⌃ are left to the menus.

**Top speed** is metres a second with the keys held. **Ramp** is the seconds from standing to top speed after a key goes down, and from top speed to standing after the last one comes up; the speed curve shapes it, so speed = top speed × f(throttle), the throttle climbing the ramp while a key is held and falling while none is. Curves that dip below zero (back in-out) do not fly backwards.

### Viewpoints and Fly to

**Save view** keeps the camera's position, target, up and field of view under a name, up to eight per site. **Fly to** moves the camera from where it is to a viewpoint over **Fly to** seconds (zero jumps), in a straight line eased by the speed curve; an overshooting curve overshoots the viewpoint and settles on it. Hand input stops a Fly to under way.

## Speed curves

One dropdown for both modes. Every curve is a pure function of t in [0, 1] with f(0) = 0 and f(1) = 1.

| Curve              | f(t)                                                                      |
| ------------------ | ------------------------------------------------------------------------- |
| Linear             | t                                                                         |
| Ease in (cubic)    | t³                                                                        |
| Ease out (cubic)   | 1 − (1 − t)³                                                              |
| Ease in-out        | 4t³, then 1 − (2 − 2t)³ / 2                                               |
| Exponential in     | (2^10t − 1) / 1023, normalised so that it meets 0 and 1 exactly           |
| Exponential out    | 1 − exponential in (1 − t)                                                |
| Exponential in-out | The two halves of exponential in, mirrored                                |
| Sine in-out        | (1 − cos πt) / 2                                                          |
| Bounce out         | Penner's four parabolic bounces, within [0, 1]                            |
| Elastic out        | 2^−10t sin((10t − 0.75) · 2π/3) + 1: overshoots to about 1.37             |
| Back in-out        | Penner's, with c = 1.70158 × 1.525: dips to about −0.10 and rises to 1.10 |
| Custom Bézier      | A CSS-style cubic Bézier from (0, 0) to (1, 1) through (x1, y1), (x2, y2) |

The pane draws the curve chosen in a small preview, scaled to show any overshoot. For the custom Bézier it shows four fields; x1 and x2 are held to [0, 1] so that every t has one value.

**In Orbit** the curve shapes progress round each lap: the angle is start + 2π (laps + f(fraction of the lap)), so the camera meets the start of each lap at the same place. **In Fly** it shapes the speed's ramp up and down, and Fly to's progress.

**The loop seam.** Looping, speed carries on from one lap into the next only when the curve leaves t = 0 at the slope it arrives at t = 1. Linear and the in-out curves do (the in-out ones by slowing to a stop at the seam); ease in, ease out, exponential in and out, bounce, elastic and most Béziers do not. The pane warns, under the curve, when such a curve is set to loop.

## With paint mode

The camera pane takes the 3D view's camera the same way the street painter's plan view does: the renderer gives its camera to one holder at a time (plan, orbit or fly), keeps the camera the holder replaced, and puts it back, or leaves it, when the holder lets go. So:

- while an orbit plays, is paused or has finished, or while flying or flying to a viewpoint, **Paint streets** is disabled, its tooltip saying why;
- while painting, Play, Fly and Fly to are disabled, and the pane says to leave paint mode first.

## Where settings are kept

The pane's settings are JSON in UserDefaults: under `cameraPane.settings.<L16 token>` while a site is loaded in the alignment pane, and otherwise under `cameraPane.settings`, the global default. A site without settings of its own starts from the global default; **Use these as the default** makes the current site's settings the global default. Viewpoints are kept the same way under `cameraPane.viewpoints`. Choosing another site stops the orbit and flight, then loads that site's settings and viewpoints. Settings saved by a later version that this one cannot read fall back to the defaults.

## Known limits

- The pane's mouse and keyboard handling (flight's keys, Option-look, picking, and the pause on hand input) is covered by the session's tests, not by driving the app: the app cannot be run headless.
- Measure from scene uses the reference's points only, not the live point cloud; with no site placed it has nothing to fit.
- The orbit's up is the sensor frame's z, as the rest of the view's is, so a tilted sensor tilts the ellipse with it.
