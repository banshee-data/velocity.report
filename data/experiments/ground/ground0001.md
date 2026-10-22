---
id: ground0001
title: "Where the ground is: each site's surface against the sensor's plane, and how far the tracks leave it"
status: approved
owner: david
created: 2026-10-18
asks:
  - docs/BACKLOG.md
  - docs/plans/lidar-state-estimation-plan.md#21-p11-is-a-correctness-problem-not-a-future-refinement
  - data/maths/paper-implementation-gap-analysis.md#8-patchwork-lim-et-al-2022
  - docs/lidar/operations/state-estimation-phase01-corpus-baseline.md#p11-ground-plane-fit-measured-per-site
report: docs/lidar/operations/ground-surface-corpus-read-2026-10.md
requires:
  capabilities: [pcap:s2, pcap-build]
  disk_gb: 20
arms:
  - name: a0
    tuning: config/tuning.defaults.json
cases:
  corpus: d2-first-segment
  count: 23
steps:
  - id: implement
    kind: manual:implement
    estimate: { human: 2h }
    note: >-
      lidar-state-estimation-baseline fits and reports the regional surface without clipping by
      it (ground_report): the global plane's gradient and the bearing of its fall line against
      the confirmed moving tracks' course; the cell planes' height against the global plane at
      the positions tracks cross, p50, p90 and max; and for each confirmed track at 2 m/s or more
      the slope of its cluster's lowest return against along-track distance, in the sensor frame
      and as height above the surface, pooled per site into the summary.
  - id: replay
    kind: state_estimation_baseline
    over: [arms, cases]
    estimate: { each: 3m, worker: m1pro }
    params: { ground_report: true }
    note: the defaults' tracks, with the surface fitted beside them and read, never applied
  - id: write
    kind: manual:write
    estimate: { human: 2h }
---

# ground0001: where the ground is

**Question.** The tracker's state is planar in the sensor's frame, and the sensor's horizontal
plane is the road's only where the mount is level and the road flat. The band that removes ground
is on absolute sensor Z, so on a grade it sits at a different height above the road at every
range. Before the clip, the frame and the box are changed to read a surface (ground0002 to
ground0004), measure the surface each site has: how far it tilts from the sensor's plane, in
which direction, how far it departs from one plane, and how much vertical motion the tracked
bodies show in the sensor frame as a result. Three sites have a global gradient (Marina 4.35 %,
Columbus 0.88 %, Embarcadero-Folsom 0.14 %); the other twenty, the direction, and the departure
from a plane have never been read.

**Measures.** All from the P11 regional surface fitted from the settled background
(`l3grid.FitRegionalGroundSurfaceFromBackground`, 10 m cells, global fallback), read and never
applied, so the tracks are the defaults'.

1. _Grade and its direction._ The global plane's gradient `g` (metres per metre) and the bearing
   of its fall line against the course of the site's confirmed moving tracks: a grade along the
   road moves the clip with range, a cross-slope moves it across lanes. The two numbers the
   geometry predicts from it are reported beside it: the planar speed's scale against the speed
   along the surface, `1 − cos(atan g)`, and the fixed band's height error 30 m along the fall
   line, `30 g`.
2. _Departure from one plane._ At every position a confirmed track crosses, the cell plane's
   height less the global plane's: p50, p90 and the largest, per site. This is the number that
   decides whether a single plane is enough or the cell model, the mesh in waiting, is needed.
3. _Vertical motion of the bodies._ For each confirmed track at 2 m/s or more, the least-squares
   slope of its cluster's lowest return against along-track distance, in the sensor frame, and
   the same slope of its height above the surface. The first is what the tracker lives with
   today; the second is what remains once the surface is read.

**Reading, written before the replays (2026-10-18).** A measurement, no rule. What it decides:

- If no first segment has `g` of 0.01 or more, the planar speed's scale error is under 0.005 %
  everywhere and ground0003 retires as moot for these mounts, with this table as its reason; the
  clip question stands, because the band's floor sits 0.5 m below the road on level ground.
- Any site with `g` of 0.01 or more is where ground0002 and ground0003 read first.
- Where the p90 departure from the global plane exceeds the surface clip's 0.2 m floor, one
  plane cannot clip correctly and ground0002's one-plane control is expected to lose to its cell
  arms there; where it is under 0.1 m everywhere tracks go, the cell model has nothing to earn on
  the clip and the mesh question closes for that site.
- A body whose lowest return rises or falls with distance in the sensor frame but not above the
  surface is a body the surface explains; one that does neither, or both, is a merge, a clip
  artefact or a kerb, and is listed.

**What it does not establish.** The ground's true grade: the plane is fitted to the sensor's own
returns, so a pitched mount and a graded road are one number, which is the number the tracker
needs and not the one a survey would give. Nothing held out: embarcadero-folsom stays unread.
Kerbs, driveways and the far side of a crown appear only as cell departures, not as surfaces of
their own.
