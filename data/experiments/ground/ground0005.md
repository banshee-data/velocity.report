---
id: ground0005
title: "What the published scene draws: the trail and the box on the surface, not on each frame's lowest return"
status: proposed
owner: david
created: 2026-10-18
asks:
  - docs/plans/lidar-web-scene-export-plan.md
  - docs/BACKLOG.md
depends_on: [ground0004]
report: docs/lidar/operations/published-scene-vertical-2026-10.md
requires:
  capabilities: [pcap:s2, pcap-build]
  disk_gb: 40
arms:
  - name: a0
    tuning: config/tuning.defaults.json
  - name: surface
    tuning: config/tuning.defaults.json
    replay: [surface_ground]
  - name: box-ground
    tuning: config/tuning.defaults.json
    replay: [surface_ground, box_ground]
cases:
  corpus: d2-first-segment
  count: 3
  sites: [columbus-broadway, marina-webster-beach, embarcadero-bay]
steps:
  - id: player
    kind: manual:implement
    estimate: { human: 2h }
    note: >-
      scene-player.js draws the box with its base at the frame's z, which is the cluster's lowest
      return (OBB.CenterZ is the minimum z), instead of centred on it, and the trail at that base
      rather than h/2 below it; the export documents z as the base; a reader in tools/s2-archive
      reports, per track of an exported scene, the standard deviation of the trail's height and
      the box's height above the surface, and the offset between the trail and the box.
  - id: scenes
    kind: scene_variants
    over: [arms, cases]
    estimate: { each: 25m, worker: m1pro }
    params: { settle_before_recording: false, start_frame: 400, stride: 2 }
    note: >-
      the three windows exported as web scenes under each arm, as perf0002's driver does; the
      reader's figures from the exported frames, which are what the site draws
  - id: write
    kind: manual:write
    estimate: { human: 1h }
---

# ground0005: what the published scene draws

**Question.** The published scenes move in z. On Columbus–Broadway (0.5.1-pre45) the longest
tracks' published `z` has a standard deviation of 0.3 to 0.8 m and a range of a metre or two,
and the trail the player draws at `z − h/2` is wider still, 0.5 to 1.25 m, because the box's
height moves with it. Two things make this, and they are separate:

1. _The data._ The exported `z` is the associated cluster's lowest return in each frame
   (`OBB.CenterZ` is the minimum z, carried as `LatestZ` and recorded as the track's z), and
   the lowest foreground return of a body is wherever the clip, the background and the view left
   it: a bumper one frame, a wheel arch the next, the roof when the lower body is occluded, the
   road when a ground return is admitted. On Columbus the median published z sits a metre above
   the road. Nothing tracks it. ground0004 replaces it with the surface's height under the box
   and a held height belief.
2. _The drawing._ The player positions a centred unit box at `z` and scales it by `h`, so a box
   whose data says "base at z" is drawn sunk by half its height, and the trail, placed just
   above the drawn base, sits `h/2` below the data's base and moves whenever `h` does, even on a
   frame whose lowest return did not. The macOS renderer's unit box runs from z 0 to 1, so the
   visualiser draws the same record with its base at `z`; the two viewers disagree by `h/2`.

Fixing the second is engineering, not an experiment, and the `player` step does it before any
scene is exported. The experiment is the first: with the player right, how steady is the trail
the site draws under each ground model, and what does the surface add to a fixed base?

**Measures.** From the exported frames, which are what the site draws, per confirmed vehicle
track and pooled per site: the standard deviation of the trail's height and of the box's base
above the surface; the share of vehicle frames whose base is more than 0.3 m above the surface
or below it; the trail's horizontal offset from the box centre, which this spec does not change
and geom0012 decides; and, for the three sites, the vertical range the longest ten tracks show,
the number the survey page's reader sees.

**Rule, written before any export (2026-10-18).** The `player` step alone, read on `a0`, must
take the trail's height deviation on every track to the deviation of `z` itself: the `h/2` term
gone, 0.53 m to 0.33 m on Columbus's track `2hu`, nothing else moved. `box-ground` passes when
the trail's height deviation on confirmed vehicle tracks is under 0.05 m at every site and 95 %
of vehicle frames stand within 0.3 m of the surface; `surface` alone is the control that says
how much of that the clip gave and how much the held base did. Horizontal rows are identical to
`surface`'s, as ground0004 requires. A site where `box-ground` passes and `a0` shows a metre of
range is the site to republish first.

**What could mislead.** A box held on the surface hides a real vertical event, a vehicle on a
kerb or a lifted load; the held height belief, not the base, is where such a thing would show,
and the report lists frames whose cluster's lowest return is more than 0.5 m above the held
base. The reader measures the exported frames at the export's stride, not every tracker frame.

**What it does not establish.** Which horizontal centre the trail should follow (geom0012).
Anything about the sites not exported. The ground's truth: the surface is the sensor's own
returns.
