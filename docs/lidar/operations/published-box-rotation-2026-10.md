# The published box: why cars' boxes turn off their points, and a box that holds them

<!-- ignore-style-length -->

The macOS visualiser and the scene assets on velocity.report draw the same box for a track, the
one the tracker publishes in each frame. Cars' boxes rotate so that their returns fall outside
them, and turn in ways no car can. This report finds why, builds a box model that holds every return and turns
within what a vehicle can do, and scores it against the box drawn today on both five-minute
segments of 23 corpus sites, the two labelled kirk captures and kirk0's reviewed poses.

- **Status:** Complete, 2026-10-16. One unattended run; experiment [geom0005](../../../data/experiments/geom/geom0005.md)
- **Layers:** L5 (the published box), L9 (frame bundle, VRLOG, gRPC), scene export and the scene player, the macOS visualiser
- **Related:** [geometry convergence plan](../../plans/lidar-tracker-geometry-convergence-plan.md), [rectangle heading screen](solid-body-rectangle-heading-2026-10.md), [containment screen](solid-body-containment-corpus-2026-10.md), [criteria record](geometry-convergence-criteria.md)
- **Parameter set:** [box-model.json](../../../config/experiments/box-model.json)

## Summary

- **Why.** The box the visualiser and the scene assets draw is three estimates that need not
  agree: the tracked position (a medoid on the visible surface), a PCA heading smoothed at 8 % a
  frame behind three guards, and this frame's PCA extents measured along another axis. On 15
  held-out corpus sites 96 % of vehicle frames leave a return more than 10 cm outside the box
  drawn from them, the farthest 1.55 m out at p90, and the box's axis is 13.0° from the
  direction of travel at the median and 58° at p90. Moving that box onto the cluster's centre
  still leaves 63 %; the raw PCA box holds every return but turns faster than a vehicle can on
  13 % of frame pairs. The smoothing that stops the spinning is what lets the box leave its
  returns.
- **The box model** draws a heading filtered from each frame's rectangle fit and the course, its
  turn bounded by κ·v + ω₀ on a speed the velocity's own noise cannot explain, with extents and a
  centre that hold every return. Held out, no vehicle frame leaves a return outside it; its axis
  is 1.6° from the direction of travel at the median and 5.9° at p90; visible turn-rate
  violations fall from 3.7 % to 0.6 % of frame pairs, turning at rest from 19.8 % to 2.3 %, and
  one-frame turns over 30° from 1.29 % to 0.30 %. The second segment of 22 sites, read once after
  the defaults were fixed, agrees on every figure. On kirk0's 44 reviewed poses its axis is 2.1°
  from the reference at the median and 8.4° at p90, against 8.4° and 79° today.
- **The tracks are unchanged**: kirk0's tracking baseline is byte-identical with the block on.
  The box model writes only the published box.
- **Cost.** On the development Mac, `Tracker.Update` rises from 0.025 to 0.109 ms at the median
  and from 0.12 to 0.33 ms at p99 on kirk0, and the whole frame from 53.5 to 54.4 ms at p99. The
  Pi is not measured.
- **What it costs in drawing.** The box is slightly larger: vehicles' median length 4.29 m
  against 3.98 m and width 1.82 m against 1.72 m held out, with 4.7 % of frames wider than 2.8 m
  against 3.9 %, because a box that must hold a merged or noisy cluster is as wide as it is. It
  also wiggles a little more on a straight road (6.8° per 10 m at the median against 3.2°): it
  follows the measured axis, where the EMA is steady because it does not.
- **Not on by default.** The block is absent from the defaults, so the fingerprint and the perf
  baselines stand. [box-model.json](../../../config/experiments/box-model.json) turns it on for a
  server, a replay or a scene publication; making it the default waits on its timing on the Pi
  and the perf baselines its fingerprint will move.
- **Still wrong.** A car labelled before it moves faster than about 1 m/s can be drawn a quarter
  turn off (car 8 at 0.8 m/s, a 1.3 × 1.5 m view), and a truck whose cab never reached the
  tracker (truck 2) is drawn as its end face; neither is the box's to fix.

## Question

Why do the drawn boxes of cars turn so that their returns fall outside them, and turn in ways a
vehicle cannot? What pipeline or configuration change makes a drawn box hold its returns and turn
the way the vehicle under it does, and what does it cost?

## Why the drawn box leaves its points

The published box is assembled in `adaptTracks`
([adapter.go](../../../internal/lidar/l9endpoints/adapter.go)) from three estimates that need not
agree:

| Part    | What is drawn                 | What it is                                                                                                                                                                                                                                                  |
| ------- | ----------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Centre  | the tracked position `X`, `Y` | The Kalman state, measured at the cluster's medoid: a return on the visible surface, not the body's centre. The scene player's comment "Track position is the oriented box centre" was wrong.                                                               |
| Heading | `OBBHeadingRad`               | The cluster's PCA axis, disambiguated by velocity, behind three guards (too few points, a near-square aspect, a 60° to 120° jump), then smoothed by an EMA at α = 0.08 a frame ([tracking_update.go](../../../internal/lidar/l5tracks/tracking_update.go)). |
| Extents | `OBBLength`, `OBBWidth`       | This frame's PCA box, measured along this frame's PCA axis, not along the drawn heading; held from the last frame the guards let through.                                                                                                                   |

Each part fails in its own way, and together they explain both complaints:

- **The heading lags.** An EMA at 8 % a frame has a time constant of about 1.2 s at 10 Hz. Through
  a turn at 20°/s it trails the body by about 23°, so the box swings wide of the car and turns
  after it, not with it.
- **The extents are along another axis.** Whenever the smoothed heading and this frame's PCA axis
  differ, the frame's length and width are drawn turned by the difference, and the corners leave
  the returns.
- **PCA reads the wrong axis.** The principal axis of an L-shaped view is its diagonal and of an
  end-on view the face strip; the guards then reject the correction as a 90° jump until five
  rejections release it at once.
- **The centre is a surface return.** A box centred on the medoid of an L is shifted by up to half
  a length and half a width from the returns it should hold.

Three diagnostics in the scorer swap one part of the drawn box for the cluster's own, on the same
tracks: `dx-centred` draws today's box at the centre of the cluster's PCA box; `dx-tight` keeps
today's heading and fits the extents and centre to the returns along it; `dx-pca` draws the
cluster's own PCA box, unsmoothed. Vehicles on the eight tuning sites:

| Variant      | Tracks |  Rows | Rows with a return outside | Returns outside, mean share | Farthest return, p90 | Farthest return, p99 |
| ------------ | -----: | ----: | -------------------------: | --------------------------: | -------------------: | -------------------: |
| `published`  |    197 | 6,007 |                     97.0 % |                      16.9 % |               2.00 m |               3.84 m |
| `dx-centred` |    197 | 6,007 |                     71.8 % |                      14.9 % |               1.06 m |               2.50 m |
| `dx-tight`   |    197 | 6,007 |                      0.0 % |                       0.0 % |               0.00 m |               0.00 m |
| `dx-pca`     |    197 | 6,007 |                      0.0 % |                       0.0 % |               0.00 m |               0.00 m |
| `km`         |    197 | 6,007 |                      0.0 % |                       0.0 % |               0.00 m |               0.00 m |

and how each turns:

| Variant      | Visible turn-rate violations | Turning at rest | Visible jumps over 30° | Heading jumps over 30° | Sideslip median | Sideslip p90 | Sideslip over 15° | Wiggle median (°/10 m) | Wiggle p90 |
| ------------ | ---------------------------: | --------------: | ---------------------: | ---------------------: | --------------: | -----------: | ----------------: | ---------------------: | ---------: |
| `published`  |                        6.1 % |          34.8 % |                 1.32 % |                 1.32 % |           15.4° |        56.3° |            48.5 % |                   6.7° |      26.6° |
| `dx-centred` |                        6.1 % |          34.8 % |                 1.32 % |                 1.32 % |           15.4° |        56.3° |            48.5 % |                   6.7° |      26.6° |
| `dx-tight`   |                        5.7 % |          34.8 % |                 0.03 % |                 1.32 % |           15.4° |        56.3° |            48.5 % |                   6.7° |      26.6° |
| `dx-pca`     |                       12.7 % |          19.1 % |                 4.00 % |                 4.08 % |           20.2° |        83.5° |            57.7 % |                  36.1° |      79.2° |
| `km`         |                        0.9 % |           3.9 % |                 0.35 % |                 1.15 % |            1.6° |         6.9° |             3.1 % |                   7.1° |      18.7° |

Read together:

- The centre costs a quarter of the misses and half the distance: today's box moved onto the
  cluster still leaves a return outside on 72 % of vehicle frames.
- The rest is the heading against the extents. Fitting the extents to the returns along today's
  heading holds them all (`dx-tight`), but the box is then as wide as a heading 15° off makes it
  (25.6 % of frames wider than 2.8 m, table below), and it still turns as today's does.
- The raw PCA box holds every return by construction, and turns faster than a vehicle can on
  12.7 % of pairs, jumps 30° in a frame on 4 %, and points 20° from the direction of travel at
  the median. Today's smoothing halves its turn-rate violations and is what parts the heading from
  the extents.

## The box model

`l5tracks.BoxEstimator` ([box_model.go](../../../internal/lidar/l5tracks/box_model.go)) keeps one
rectangle per track whose parts agree by construction. It reads the tracked state after each
update and writes only the published box: association, the Kalman state and identity are the
tracker's without it.

**Heading.** A two-state filter, heading and yaw rate, with white yaw acceleration
(σ = 0.4 rad/s²):

- Each frame the cluster's rectangle-fit axis observes the heading modulo 90°: the closeness
  criterion of `l4perception.FitRectangle` (the W1a fit), on 64 members drawn deterministically,
  at a 2° coarse step, with its own σ × 1.5. A fit beyond 3σ of the prediction is refused; five
  refusals in a row reopen the heading to the fit.
- At 1 m/s or more the course (the Kalman velocity's direction) observes the heading too, with the
  velocity covariance's spread across the course plus 4° of sideslip, while that spread is under
  15°. A course more than 45° from the heading is a manoeuvre and is left out.
- The axis does not say which side is the length, or which end the front. An unlabelled box takes
  the longer visible side as its length. The first course whose spread is under 25° labels it,
  turning the box by a quarter at once if the course says the length runs the other way: the
  turn corrects a belief, so it is not drawn as a rotation, and the extents swap with it. A
  steering-grade course more than 60° from the labelled axis for five frames relabels it.
- **The turn-rate bound.** Each frame the yaw rate and the drawn turn are limited to κ·v + ω₀,
  with κ = 0.2 /m (a 5 m turning radius) and ω₀ = 3°/s. The speed is the Kalman speed less twice
  its own standard deviation, never below zero: a body turns only by moving, and a stopped car's
  filtered speed is noise. A box in its first second, or whose longer side is under 2 m (a person
  may turn on the spot), has ω₀ = 45°/s. A change of front or a label is not a turn.
- While the track coasts the yaw rate halves each frame and the box moves with the track.

**Extents.** The larger of a belief and this frame's span of the members along the heading:

- The belief is the third largest of the last 50 admitted spans along the fit's own axes,
  trimmed of the outermost hundredth, admitted only when the fit agrees with the heading and the
  tracker has not flagged the cluster as a probable merge. A window, unlike a whole-life maximum,
  lets the box return to the body once a merge ends.
- The believed width is capped at 2.6 m, the widest road vehicle allowed; the drawn width is still
  as wide as the frame's returns.
- A labelled box of a vehicle's believed width (1.5 m or more) is drawn at least 3.8 m long, the
  unseen part away from the sensor: an end-on car is not a 0.6 m box.

**Centre.** Placed so that the box holds every member: inside the interval of centres that
contain this frame's span along each axis, as near as it allows to the centre carried from the
last frame, moved 30 % of the way toward the placement that puts the unseen remainder on the side
away from the sensor.

**Publishing.** The tracker fills `BoxX`, `BoxY`, `BoxHeadingRad`, `BoxLength` and `BoxWidth`.
`adaptTracks` puts them in the frame's existing box fields and the box centre in three new track
fields, `bbox_center_x`, `bbox_center_y` and `bbox_center_valid` (protobuf fields 36 to 38), which
the VRLOG and gRPC codecs carry. The scene export writes them as `bx` and `by`; the scene player
and the macOS visualiser draw the box there, and at the track when they are absent. A recording
made with the model off is drawn exactly as before.

**Configuration.** The optional `l5.cv_kf_v1.box_model` block, absent from the defaults, so the
shipped fingerprint is unchanged ([CONFIG.md](../../../config/CONFIG.md#l5-box-model)). Its
`rectangle_fit` axis source keeps every cluster member for the frame, in memory only; `pca` keeps
none and reads the cluster's PCA axis. The server applies the block at start-up, from a tuning
file at replay start, and as a runtime patch; switched off at run time, every published box is
dropped rather than frozen.

**Tracks unchanged.** kirk0 replayed through the Phase 0 harness under the defaults and under
[box-model.json](../../../config/experiments/box-model.json) gives byte-identical
`tracking_baseline.json` and `confirmed_duration.json`.

## Method

**The scorer.** `lidar-box-eval` ([main.go](../../../cmd/tools/lidar-box-eval/main.go)) replays
a capture through `replayeval.Run` with a tap after every tracker update. For every confirmed track
it scores the drawn box (`published`) and, on the same clusters and the same tracked states, every
box-model variant named on its command line, so one replay scores them all. A replay under a
tuning file with the box model on reproduces the matching variant under `published` exactly
(checked on kirk0, every figure equal). The evaluation bounds are the scorer's own, independent of
any variant:

| Measure                    | Definition                                                                                                                                                                                  |
| -------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Outside                    | Share of rows (a confirmed track's associated frame) with any cluster member more than 0.10 m outside the drawn box; also the mean share of members outside                                 |
| Turn-rate violation        | Share of consecutive frame pairs whose drawn turn, modulo 180°, is faster than 0.25/m × speed + 10°/s                                                                                       |
| Turning at rest            | Share of pairs below 0.5 m/s whose drawn box turned more than 1°                                                                                                                            |
| Jump                       | Share of pairs whose drawn box turned more than 30° in one frame                                                                                                                            |
| Visible                    | The same three for the drawn rectangle: a turn of more than 45° whose extents swap draws the same rectangle turned by the remainder, and counts as that                                     |
| Sideslip                   | Angle between the drawn axis and the direction of travel, modulo 180°, at 3 m/s or more                                                                                                     |
| Minimum-area axis and area | Per row with 30 or more members, the drawn axis against the members' minimum-area rectangle's, and the ratio of that rectangle's area to the members' bounding area along the drawn heading |
| Wiggle                     | Per track over 10 m, (Σ\|Δψ\| − \|ΣΔψ\|) per 10 m travelled: turning back and forth that the net turn does not explain                                                                      |
| Size                       | Median drawn length and width per track; share of rows wider than 2.8 m                                                                                                                     |

Groups follow each track's final L6 class: vehicles are cars, trucks and buses.

**References.** kirk0's 44 reviewed keyframes with an observed yaw (three vehicles: truck 1 at
10.6 m, car 8 at 4.3 m and truck 2 at 9.6 m; [pilot](physical-reference-pilot-kirk0-2026-10.md)),
from the frozen bundle at revision 76. Each keyframe is matched to the drawn box nearest its
observed point at the nearest frame time, within 2 m, ties to the nearest centre; the axis error
is modulo 180°. The references are fits to the same reviewed returns the tracker reads, within
their own bound of 1.6° to 7°, so this compares a fit with a fit.

**Captures.** Tuning: the first five-minute segment of eight corpus sites (Haight at Divisadero,
California at Leon Baker, Lombard at Laguna, Columbus at Broadway, Van Ness at Sacramento, Hyde at
O'Farrell, 1st at Mission, Embarcadero at Bryant), kirk0 and kirk1's first 270 s, with 70 s of
warm-up on the corpus and 20 s on kirk. Held out: the first segment of the other fifteen tuning and
screen sites, scored only once the defaults were fixed (three times as the scorer was corrected;
see the provenance). Then the second segment of 22 sites, once. Embarcadero at Folsom, the
declared held-out site, is in no figure here: an earlier held-out lane read it before the
exclusion was applied, and that output was set aside unread.

**Variants.** All share the defaults below and change one thing. They were scored side by side in
nine passes on the tuning sites; each pass's build is in the provenance section.

| Variant       | Change                                                  |
| ------------- | ------------------------------------------------------- |
| `km`          | The box model as configured in box-model.json           |
| `km-pca`      | Axis from the cluster's PCA box; no members kept        |
| `km-nocourse` | No course: neither steering nor labels                  |
| `km-free`     | No turn-rate bound                                      |
| `km-k15`      | κ = 0.15 /m                                             |
| `km-k25`      | κ = 0.25 /m                                             |
| `km-sig1`     | Axis σ × 1                                              |
| `km-yaw02`    | Yaw acceleration σ = 0.2 rad/s²                         |
| `km-floor0`   | No vehicle length floor                                 |
| `km-full`     | Fit on 256 members at a 1° coarse step (the W1a search) |
| `km-fit128`   | Fit on 128 members at a 2° step                         |
| `km-pull0`    | Centre carried only                                     |
| `km-pull1`    | Centre anchored every frame                             |

## Findings

### The box model against the box drawn today

On the tuning sites, the box model holds every return of every vehicle frame (the first table
above) and turns as a vehicle does: visible turn-rate violations 0.9 % of pairs against 6.1 %,
turning at rest 3.9 % against 34.8 %, visible one-frame turns over 30° 0.35 % against 1.32 %, and
an axis 1.6° from the direction of travel at the median (15.4° today). Its heading turns by more
than 30° in a frame on 1.15 % of pairs, nearly all of them the labelling instant's quarter turn
with the extents swapped, which draws the same rectangle.

**Held out**, on the first segment of the fifteen sites the variants were not chosen on:

| Variant      | Tracks |  Rows | Rows with a return outside | Farthest return, p90 | Visible turn-rate violations | Turning at rest | Visible jumps over 30° | Sideslip median | Sideslip p90 | Sideslip over 15° | Wiggle median | Wiggle p90 |
| ------------ | -----: | ----: | -------------------------: | -------------------: | ---------------------------: | --------------: | ---------------------: | --------------: | -----------: | ----------------: | ------------: | ---------: |
| `published`  |    237 | 7,892 |                     96.1 % |               1.55 m |                        3.7 % |          19.8 % |                 1.29 % |           13.0° |        58.4° |            41.2 % |          3.2° |      21.0° |
| `dx-centred` |    237 | 7,892 |                     63.2 % |               0.92 m |                        3.7 % |          19.8 % |                 1.29 % |           13.0° |        58.4° |            41.2 % |          3.2° |      21.0° |
| `dx-pca`     |    237 | 7,892 |                      0.0 % |               0.00 m |                       13.4 % |          15.1 % |                 4.84 % |           19.6° |        82.9° |            55.0 % |         37.7° |      91.6° |
| `km`         |    237 | 7,892 |                      0.0 % |               0.00 m |                        0.6 % |           2.3 % |                 0.30 % |            1.6° |         5.9° |             2.8 % |          6.8° |      22.0° |

Every figure holds or improves out of sample except the wiggle, which is the box following its
measured axis: its median is 6.8° per 10 m against 3.2° for today's box, and its p90 22.0°
against 21.0°. Today's box is steady on a straight road because the EMA does not follow anything
quickly; the same EMA is why it is 13° off the direction of travel at the median.

**Second segments.** After the defaults were fixed and the report's figures above were in, the
second five-minute segment of 22 sites (Embarcadero at Folsom excluded), none of them read before
in this run, scored once with the published box, the box model, its PCA-axis variant and yaw
acceleration 0.2 rad/s². Vehicles:

| Variant     | Tracks |   Rows | Rows with a return outside | Farthest return, p90 | Visible turn-rate violations | Turning at rest | Visible jumps over 30° | Sideslip median | Sideslip p90 | Sideslip over 15° | Wiggle median | Wiggle p90 | Length |  Width | Wider than 2.8 m |
| ----------- | -----: | -----: | -------------------------: | -------------------: | ---------------------------: | --------------: | ---------------------: | --------------: | -----------: | ----------------: | ------------: | ---------: | -----: | -----: | ---------------: |
| `published` |    360 | 12,595 |                     96.1 % |               1.90 m |                        4.9 % |          14.2 % |                 1.15 % |           13.6° |        60.3° |            45.1 % |          4.2° |      25.1° | 4.14 m | 1.69 m |            6.3 % |
| `km`        |    360 | 12,595 |                      0.0 % |               0.00 m |                        0.9 % |           1.9 % |                 0.24 % |            1.5° |         6.4° |             2.2 % |          7.2° |      17.7° | 4.31 m | 1.88 m |            8.5 % |
| `km-pca`    |    360 | 12,595 |                      0.0 % |               0.00 m |                        1.1 % |           3.1 % |                 0.19 % |            1.7° |         6.1° |             2.6 % |          8.3° |      18.4° | 4.54 m | 2.14 m |            9.9 % |
| `km-yaw02`  |    360 | 12,595 |                      0.0 % |               0.00 m |                        0.8 % |           1.9 % |                 0.23 % |            1.7° |         6.7° |             2.3 % |          6.3° |      15.9° | 4.31 m | 1.88 m |            8.7 % |

All tracks:

| Variant     | Tracks |    Rows | Rows with a return outside | Visible turn-rate violations | Turning at rest | Visible jumps over 30° | Sideslip median | Sideslip p90 |
| ----------- | -----: | ------: | -------------------------: | ---------------------------: | --------------: | ---------------------: | --------------: | -----------: |
| `published` |  4,767 | 142,955 |                     72.1 % |                       13.0 % |          22.3 % |                 0.78 % |           17.3° |        67.0° |
| `km`        |  4,767 | 142,955 |                      0.0 % |                        6.6 % |          13.6 % |                 0.14 % |            2.4° |        11.1° |

The second segments agree with both first-segment sets on every figure: no return outside,
turn-rate violations and turning at rest cut by four fifths or more, the axis within 1.5° of the
direction of travel at the median. The wiggle splits as before (the median rises, the p90 falls),
and the box is wider than 2.8 m on 8.5 % of vehicle frames against 6.3 %. Three of these
segments span an unrecorded knock to the sensor
([pose sweep](l3-corpus-pose-sweep-2026-10.md)); it moves the background, not the box.

**Size and axis.** Tuning sites, vehicles:

| Variant     | Length, median | Width, median | Length CV | Wider than 2.8 m | Area efficiency p10 | Area efficiency median | Minimum-area axis error median |   p90 |
| ----------- | -------------: | ------------: | --------: | ---------------: | ------------------: | ---------------------: | -----------------------------: | ----: |
| `published` |         3.86 m |        1.70 m |      0.36 |           14.3 % |                0.58 |                   0.86 |                          14.0° | 36.0° |
| `dx-tight`  |         3.41 m |        2.13 m |      0.39 |           25.6 % |                0.58 |                   0.86 |                          14.0° | 36.0° |
| `dx-pca`    |         3.81 m |        1.72 m |      0.35 |           14.5 % |                0.81 |                   0.94 |                           6.5° | 18.6° |
| `km`        |         4.14 m |        1.84 m |      0.16 |           15.7 % |                0.80 |                   0.96 |                           4.6° | 25.4° |

Held out:

| Variant     | Length, median | Width, median | Length CV | Wider than 2.8 m | Area efficiency p10 | Area efficiency median | Minimum-area axis error median |   p90 |
| ----------- | -------------: | ------------: | --------: | ---------------: | ------------------: | ---------------------: | -----------------------------: | ----: |
| `published` |         3.98 m |        1.72 m |      0.35 |            3.9 % |                0.59 |                   0.89 |                          10.6° | 34.7° |
| `dx-tight`  |         3.62 m |        1.90 m |      0.41 |           12.8 % |                0.59 |                   0.89 |                          10.6° | 34.7° |
| `dx-pca`    |         4.09 m |        1.75 m |      0.36 |            4.3 % |                0.84 |                   0.95 |                           5.7° | 17.0° |
| `km`        |         4.29 m |        1.82 m |      0.16 |            4.7 % |                0.77 |                   0.96 |                           4.6° | 28.5° |

The box model draws a steadier length (coefficient of variation 0.16 against 0.36) at a car's
length, 4.1 to 4.3 m. It is wider than 2.8 m on slightly more frames than today's box (4.7 %
against 3.9 % held out); those are clusters whose returns are that wide across the heading, a
merge or a bus with mirrors, which a box that must hold them cannot draw narrower. The
minimum-area rectangle of each frame's returns is an independent check on the axis, and it
favours per-frame fits: the raw PCA box agrees with it better at p90 than the box model does. It
is a coarse check, not truth: a moving car's returns are sheared across one sweep of a spinning
sensor, so its minimum-area rectangle is a parallelogram's (at 8 m/s, a car the box model holds
1.7 m wide is 17° off its minimum-area axis), and a partial view has no rectangle to find.
Against kirk0's reviewed poses, where the yaw was reviewed, the box model is the closer of the
two by a wide margin (below).

**All tracks.** Pedestrians, cyclists and unclassified objects are in these too:

| Variant     | Tracks |   Rows | Rows with a return outside | Visible turn-rate violations | Turning at rest | Visible jumps over 30° | Sideslip median | Sideslip p90 |
| ----------- | -----: | -----: | -------------------------: | ---------------------------: | --------------: | ---------------------: | --------------: | -----------: |
| `published` |  1,886 | 58,688 |                     67.5 % |                       12.3 % |          19.2 % |                 0.85 % |           19.0° |        70.0° |
| `dx-pca`    |  1,886 | 58,688 |                      0.0 % |                       33.6 % |          43.2 % |                 5.28 % |           24.0° |        83.0° |
| `km`        |  1,886 | 58,688 |                      0.0 % |                        6.3 % |          12.4 % |                 0.16 % |            2.3° |        11.9° |

Held out:

| Variant     | Tracks |    Rows | Rows with a return outside | Visible turn-rate violations | Turning at rest | Visible jumps over 30° | Sideslip median | Sideslip p90 |
| ----------- | -----: | ------: | -------------------------: | ---------------------------: | --------------: | ---------------------: | --------------: | -----------: |
| `published` |  3,172 | 114,517 |                     67.8 % |                       14.4 % |          24.9 % |                 0.70 % |           17.4° |        66.3° |
| `dx-pca`    |  3,172 | 114,517 |                      0.0 % |                       35.4 % |          46.9 % |                 6.07 % |           25.0° |        81.2° |
| `km`        |  3,172 | 114,517 |                      0.0 % |                        5.8 % |          11.4 % |                 0.15 % |            2.4° |        12.6° |

Pedestrians alone, tuning then held out:

| Variant     | Tracks |  Rows | Rows with a return outside | Visible turn-rate violations | Turning at rest | Visible jumps over 30° | Length |  Width |
| ----------- | -----: | ----: | -------------------------: | ---------------------------: | --------------: | ---------------------: | -----: | -----: |
| `published` |     83 | 8,452 |                     73.9 % |                       17.6 % |          20.6 % |                 0.94 % | 0.73 m | 0.28 m |
| `km`        |     83 | 8,452 |                      0.0 % |                        8.9 % |          12.5 % |                 0.05 % | 0.71 m | 0.44 m |

| Variant     | Tracks |   Rows | Rows with a return outside | Visible turn-rate violations | Turning at rest | Visible jumps over 30° | Length |  Width |
| ----------- | -----: | -----: | -------------------------: | ---------------------------: | --------------: | ---------------------: | -----: | -----: |
| `published` |    183 | 16,453 |                     61.0 % |                       21.5 % |          26.6 % |                 0.69 % | 1.14 m | 0.33 m |
| `km`        |    183 | 16,453 |                      0.0 % |                        5.9 % |           7.7 % |                 0.04 % | 1.08 m | 0.54 m |

A pedestrian's box is wider under the model (0.44 to 0.54 m against 0.28 to 0.33 m): it holds
the returns today's box, turned off its own extents, leaves outside.

### Against kirk0's reviewed poses

kirk0's 44 keyframes with a reviewed yaw, matched to the drawn box nearest the reference's
observed point (39 to 42 matched; a truncated truck's box is sometimes more than 2 m from its
reference point):

| Variant       | Matched | Axis error, median |   p90 | Over 10° | Over 45° | Centre error, median |    p90 |
| ------------- | ------: | -----------------: | ----: | -------: | -------: | -------------------: | -----: |
| `published`   |      42 |               8.4° | 79.3° |       19 |       15 |               3.26 m | 4.71 m |
| `dx-pca`      |      39 |              12.5° | 65.2° |       22 |        7 |               2.61 m | 3.59 m |
| `km`          |      39 |               2.1° |  8.4° |        4 |        2 |               2.14 m | 4.16 m |
| `km-nocourse` |      39 |              88.4° | 89.6° |       32 |       31 |               2.41 m | 4.26 m |
| `km-pca`      |      39 |               6.2° | 16.0° |       10 |        2 |               2.08 m | 4.23 m |
| `km-pull1`    |      39 |               1.9° | 14.0° |        5 |        2 |               2.04 m | 3.19 m |

The two keyframes over 45° under the box model are the known label failures: car 8 at 0.8 m/s
on a young track, seen 1.3 × 1.5 m, and a fragment of truck 2 at 1.8 m/s whose 2.2 m rear face
is drawn as its length. The centre errors are dominated by the two
trucks, which reach the tracker at 60 % of their length (truck 2's cab never does), so every
variant draws them short; the reviewed lengths are 10.6 and 9.6 m. Without the course
(`km-nocourse`) the axis is right and the label wrong: the trucks are seen end on, and the
longer visible side is their width.

On kirk0 and kirk1 together, vehicles:

| Variant     | Tracks | Rows | Rows with a return outside | Visible turn-rate | At rest | Visible jumps | Sideslip med |   p90 |
| ----------- | -----: | ---: | -------------------------: | ----------------: | ------: | ------------: | -----------: | ----: |
| `published` |     13 |  466 |                     96.6 % |             5.2 % |  33.3 % |        1.37 % |        28.5° | 64.2° |
| `dx-pca`    |     13 |  466 |                      0.0 % |            14.4 % |  50.0 % |        2.88 % |        39.8° | 84.2° |
| `km`        |     13 |  466 |                      0.0 % |             1.2 % |  16.7 % |        0.41 % |         2.3° |  8.5° |

### What each piece does

Each variant changes one setting of the box model. Tuning sites, vehicles (the update cost is
per track update on a Mac running three replays at once, so a relative figure):

| Variant       | Visible turn-rate | At rest | Visible jumps | Sideslip med |   p90 | Min-area axis | Wiggle med |   p90 | Length |  Width | Wider than 2.8 m | Centre slip p90 | µs a track update |
| ------------- | ----------------: | ------: | ------------: | -----------: | ----: | ------------: | ---------: | ----: | -----: | -----: | ---------------: | --------------: | ----------------: |
| `km`          |             0.9 % |   3.9 % |        0.35 % |         1.6° |  6.9° |          4.6° |       7.1° | 18.7° | 4.14 m | 1.84 m |           15.7 % |          0.33 m |             43 µs |
| `km-pca`      |             1.1 % |   2.1 % |        0.31 % |         1.9° |  6.9° |          6.8° |       7.9° | 23.7° | 4.55 m | 2.23 m |           18.3 % |          0.32 m |             23 µs |
| `km-nocourse` |             1.8 % |   3.9 % |        0.00 % |        81.7° | 89.2° |          4.3° |       6.9° | 11.1° | 2.64 m | 2.95 m |           14.5 % |          0.36 m |             26 µs |
| `km-free`     |             5.3 % |  54.9 % |        0.40 % |         1.5° |  6.3° |          4.5° |       6.6° | 20.2° | 4.14 m | 1.84 m |           15.2 % |          0.33 m |             32 µs |
| `km-k15`      |             0.9 % |   3.9 % |        0.33 % |         1.6° |  6.9° |          4.7° |       6.9° | 17.6° | 4.15 m | 1.85 m |           16.1 % |          0.33 m |             34 µs |
| `km-k25`      |             0.8 % |   3.9 % |        0.32 % |         1.6° |  6.7° |          4.5° |       7.1° | 19.9° | 4.14 m | 1.84 m |           15.7 % |          0.33 m |             32 µs |
| `km-sig1`     |             0.8 % |   3.9 % |        0.32 % |         2.1° |  8.5° |          4.5° |       7.3° | 21.5° | 4.14 m | 1.84 m |           15.1 % |          0.33 m |             31 µs |
| `km-yaw02`    |             0.8 % |   3.7 % |        0.33 % |         1.6° |  7.2° |          4.6° |       5.8° | 17.0° | 4.14 m | 1.84 m |           15.8 % |          0.34 m |             31 µs |
| `km-floor0`   |             0.8 % |   3.9 % |        0.26 % |         1.6° |  6.9° |          4.6° |       7.1° | 18.7° | 4.14 m | 1.84 m |           15.5 % |          0.35 m |             30 µs |
| `km-full`     |             0.8 % |   3.9 % |        0.32 % |         1.5° |  6.6° |          4.4° |       6.6° | 18.1° | 4.17 m | 1.88 m |           15.6 % |          0.34 m |            126 µs |
| `km-fit128`   |             0.8 % |   3.6 % |        0.33 % |         1.6° |  6.4° |          5.0° |       6.1° | 19.2° | 4.18 m | 1.85 m |           15.6 % |          0.34 m |             53 µs |
| `km-pull0`    |             0.9 % |   3.9 % |        0.35 % |         1.6° |  6.9° |          4.6° |       7.1° | 18.7° | 4.14 m | 1.84 m |           15.7 % |          0.31 m |             32 µs |
| `km-pull1`    |             0.9 % |   3.9 % |        0.35 % |         1.6° |  6.9° |          4.6° |       7.1° | 18.7° | 4.14 m | 1.84 m |           15.7 % |          0.39 m |             38 µs |

Held out:

| Variant       | Visible turn-rate | At rest | Visible jumps | Sideslip med |   p90 | Min-area axis | Wiggle med |   p90 | Length |  Width | Wider than 2.8 m | Centre slip p90 |
| ------------- | ----------------: | ------: | ------------: | -----------: | ----: | ------------: | ---------: | ----: | -----: | -----: | ---------------: | --------------: |
| `km`          |             0.6 % |   2.3 % |        0.30 % |         1.6° |  5.9° |          4.6° |       6.8° | 22.0° | 4.29 m | 1.82 m |            4.7 % |          0.32 m |
| `km-pca`      |             0.7 % |   8.7 % |        0.24 % |         1.8° |  6.0° |          5.9° |       6.6° | 18.4° | 4.58 m | 2.04 m |            6.0 % |          0.30 m |
| `km-nocourse` |             1.7 % |   2.3 % |        0.00 % |        82.3° | 89.1° |          4.7° |       8.6° | 16.6° | 2.08 m | 3.23 m |            4.4 % |          0.35 m |
| `km-free`     |             1.8 % |   7.0 % |        0.28 % |         1.5° |  5.6° |          3.3° |       6.5° | 17.9° | 4.35 m | 1.82 m |            3.1 % |          0.31 m |
| `km-k15`      |             0.7 % |   2.3 % |        0.31 % |         1.6° |  6.1° |          4.8° |       6.8° | 20.2° | 4.29 m | 1.82 m |            5.0 % |          0.32 m |
| `km-k25`      |             0.6 % |   2.3 % |        0.30 % |         1.6° |  5.9° |          4.4° |       6.8° | 21.3° | 4.29 m | 1.82 m |            4.5 % |          0.31 m |
| `km-sig1`     |             0.5 % |   1.7 % |        0.27 % |         1.8° |  7.8° |          4.4° |       7.7° | 23.2° | 4.29 m | 1.82 m |            4.6 % |          0.32 m |
| `km-yaw02`    |             0.6 % |   2.3 % |        0.30 % |         1.6° |  6.0° |          5.0° |       5.4° | 19.0° | 4.29 m | 1.82 m |            4.8 % |          0.32 m |
| `km-floor0`   |             0.6 % |   2.3 % |        0.24 % |         1.6° |  5.9° |          4.6° |       6.8° | 22.0° | 4.29 m | 1.82 m |            4.0 % |          0.32 m |
| `km-full`     |             0.6 % |   2.3 % |        0.27 % |         1.6° |  6.0° |          3.8° |       6.5° | 20.6° | 4.29 m | 1.83 m |            4.9 % |          0.32 m |
| `km-fit128`   |             0.6 % |   2.9 % |        0.27 % |         1.5° |  6.3° |          4.1° |       7.1° | 21.5° | 4.27 m | 1.82 m |            4.9 % |          0.32 m |
| `km-pull0`    |             0.6 % |   2.3 % |        0.30 % |         1.6° |  5.9° |          4.6° |       6.8° | 22.0° | 4.29 m | 1.82 m |            4.7 % |          0.29 m |
| `km-pull1`    |             0.6 % |   2.3 % |        0.30 % |         1.6° |  5.9° |          4.6° |       6.8° | 22.0° | 4.29 m | 1.82 m |            4.7 % |          0.40 m |

- **The course** labels and steers the box. Without it the axis is still right (4.3° from the
  minimum-area axis) but the label is wrong on most vehicles: 82° from the direction of travel at
  the median, a box drawn across the car.
- **The turn-rate bound** is what stops the box turning at rest: without it 54.9 % of
  at-rest pairs turn more than 1° on the tuning sites (7.0 % held out) and 5.3 % of pairs break
  the evaluation's turn-rate bound. κ between 0.15 and 0.25 per metre changes nothing visible:
  the bound acts at low speed, where ω₀ and the speed's own uncertainty decide it.
- **The rectangle fit** against the PCA axis: with the PCA axis the box is wider (2.04 to 2.23 m
  against 1.82 to 1.84 m, as the diagonal of an L), turns at rest less on the tuning sites (2.1 %
  against 3.9 %) and more held out (8.7 % against 2.3 %), and is 6.2° off kirk0's references at
  the median against 2.1°. It keeps no members and
  costs half as much; it is the fallback for a pipeline that cannot keep them.
- **The fit's size.** The full W1a search (256 members, 1° steps) and 128 members at 2° score
  within noise of 64 members at 2°, at three times and a quarter more the cost.
- **The axis σ scale** of 1 against 1.5 is as close to the minimum-area axis, closer to kirk0's
  references at the median (1.6° against 2.1°) and further from the direction of travel (8.5°
  against 6.9° at p90 on the tuning sites). 1.5 is kept, as the W1b screen chose for the solid
  body.
- **Yaw acceleration** 0.2 rad/s² against 0.4 smooths the wiggle (5.8° against 7.1° per 10 m at
  the median on the tuning sites, 5.4° against 6.8° held out) at no visible cost elsewhere. It was
  not chosen before the held-out score was read, so 0.4 stays; 0.2 is the first candidate for a
  smoother box.
- **The vehicle length floor** draws end-on cars at 3.8 m instead of their face. It moves the
  matched centre on kirk0 by 0.1 m and adds a few visible quarter turns at labelling (0.35 %
  against 0.26 %), where the box grows along its other axis.
- **The centre pull** of 0.3 sits between carried only (the smoothest centre, 0.31 m of slip at
  p90) and anchored every frame (0.39 m, and the closest centre to kirk0's references). No
  containment or heading figure depends on it.

### Three tracks

Plotted from the scorer's `-trace` output for this report (not in Git): a car turning right at
Van Ness and Sacramento, whose box model turns with the direction of travel from 0° to 95°
while today's box stops 18° short of it and leaves a return outside on 163 of 170 frames (the
model on none); a car following the Embarcadero curve, where today's box trails the turn by 20°
to 30° for three seconds; and a car held at Columbus and Broadway for 30 s, whose box today saws
between 60° and 115° as the EMA drifts and the lock releases every few seconds, with a return
outside on all 282 frames, while the box model holds 100°.

### Cost

kirk0 through the Phase 0 harness on the final model, three runs of each arm alternated on the
development Mac with nothing else of this run's on it (medians of three):

| Arm            | `Tracker.Update` p50 |      p99 | Frame p50 |     p95 |     p99 |
| -------------- | -------------------: | -------: | --------: | ------: | ------: |
| Defaults       |             0.025 ms | 0.120 ms |    4.4 ms | 34.2 ms | 53.5 ms |
| box-model.json |             0.109 ms | 0.325 ms |    4.5 ms | 33.7 ms | 54.4 ms |

The box adds about 0.1 ms to the median update and 0.2 ms to its tail. The frame's median moves
0.1 ms and its p99 0.9 ms, within the defaults' own spread across the three runs (53.2 to
54.1 ms). Per track the update costs about 30 µs on a busy Mac, so a
frame with 50 tracks pays about 1.5 ms here. Scene assets with the box centre are 29 % larger
(kirk0's track export, 90 against 70 KB).

## Limitations

- **Label-free scoring.** Outside kirk0 nothing here is ground truth. The direction of travel is
  the Kalman velocity's, which lags a turn and is noisy below 2 m/s; the box model is steered by
  it, so its sideslip is partly a statement about its own input. The minimum-area check is
  independent of both the course and the fit but is distorted by the sensor's sweep at speed.
- **The turn-rate bound is enforced by construction**, so the box model's low violation shares
  show the bound working, not that the bound is the vehicle's. The evaluation's bound (0.25/m and
  10°/s) is looser than the model's and fixed independently of it.
- **The references are three vehicles**, two of them end-on trucks the tracker sees in part, and
  are fits to the same reviewed returns the box is fitted to.
- **Classes are L6's.** A vehicle is a track L6 called a car, truck or bus at its end; misclassified
  tracks fall in other groups, and the all-tracks tables carry them.
- **The Pi is not timed**, and the live pipeline has never kept cluster members by default; the
  `pca` axis source is the option if keeping them is too dear.
- **The web app's live view** draws boxes from the persisted track observations, which record the
  legacy box: the box model reaches the visualiser and the scene assets, not that view.
- **The macOS visualiser** builds and passes its 1,289 tests with the box centre, but was not
  watched against a live stream in this run.
- **Five-minute segments.** Each corpus case is one segment after 70 s of warm-up; long stops,
  sensor knocks (three of the second segments carry one) and rare manoeuvres are under-sampled.

## Provenance

| Item                      | Value                                                                                                                                                                                                                                                                             |
| ------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Box model as scored       | build `12e96c331` onward (code unchanged after it); scorer `039557b73` for the final tuning, held-out and kirk passes                                                                                                                                                             |
| Parameter set             | `config/experiments/box-model.json`, SHA-256 `f742cc37224b871e2262d70f672467cebd56fff9556f384e83eb52e07eb0e300`                                                                                                                                                                   |
| Defaults                  | `config/tuning.defaults.json`, SHA-256 `1844fd390a88db0c25bf8aa18510d37ad2a91fca96727144830af92371a75ca6`                                                                                                                                                                         |
| Corpus                    | first five-minute segment of each site, from the D2 first-segment site index; 70 s warm-up, the rest scored                                                                                                                                                                       |
| kirk                      | `internal/lidar/perf/pcap/kirk0.pcapng` and kirk1's first 270 s, 20 s warm-up                                                                                                                                                                                                     |
| References                | physical-reference bundle revision 76 (pack `ds_95be6fb22498b44a`), 44 keyframes with an observed yaw                                                                                                                                                                             |
| Identity check and timing | `lidar-state-estimation-baseline` on kirk0 with `-warmup 20 -campaign-metrics`: the identity check at `f125ec74d` and again at `33acd141a`, the timing at `33acd141a`                                                                                                             |
| Tuning passes             | nine passes on the eight tuning sites, with kirk0, at `c4c4fdbda`, `88a82a5fd`, `b21e07138`, `2da081947`, `6a8f44d55`, `d4ce8ee75`, `f125ec74d`, `33acd141a` and `039557b73`                                                                                                      |
| Held-out scores           | three: at `f125ec74d`, before the box model drew the fit's members itself for cost; at `33acd141a` and `039557b73` on the final model, as the scorer's visible-turn rule was corrected. No setting of the model changed after the first was read; the figures above are the third |
| Second segments           | the D2 second-segment site index, 22 sites; scorer `e1625bed4` (the final model), 70 s warm-up, no per-frame rows kept                                                                                                                                                            |
| Raw outputs               | summaries, per-frame rows and traces under the run's scratch directory on the development Mac, not in Git                                                                                                                                                                         |

## Recommendations

1. **Turn the box model on for publishing now.** Scene assets for velocity.report can be
   republished from replays run under box-model.json (the server's `--config`, or `tuning_config`
   on each replay start); nothing else about the tracks moves. The site must be built from this
   change, whose scene player draws the box centre; an older player would draw the new box at
   the tracked position.
2. **Make it the default once the Pi has timed it.** Time a replay on the Pi 5 under live load
   with box-model.json against the defaults (the `pi` step of geom0005), choose the `pca` axis
   source if keeping members is too dear there, then add the block to the defaults and recapture
   the perf baselines in the same change.
3. **Label slow vehicles from more than the course.** A displacement over a second or two labels
   a car the Kalman velocity cannot at 0.8 m/s; a class prior labels a parked car seen square on.
4. **Persist the published box** in the track observations, so the web app's live view draws it
   too.
5. **Try yaw acceleration 0.2 rad/s²** on a fresh capture: it smoothed the box on both corpus
   halves at no visible cost.
6. **Upstream.** The truncated trucks and the merged clusters are L3 and L4 problems (the
   [return fate report](w3-return-fate-kirk0-2026-10.md)); a box that holds its returns draws
   them faithfully, as wide or as short as the evidence is.
