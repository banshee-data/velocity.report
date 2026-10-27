# Where the ground is: the surface read on 23 corpus sites

<!-- ignore-style-length -->

The tracker's state is planar in the sensor's frame, and L4's height band clips on absolute
sensor Z. Before the clip, the frame and the box are made to read a surface (ground0002 to
ground0004), this report measures the surface each site has: how far it tilts from the sensor's
plane and which way, how far it departs from one plane where the traffic goes, and how the tracked
bodies' lowest returns move against it. It answers [ground0001](../../../data/experiments/ground/ground0001.md).

- **Status:** Measurement complete, 2026-10-27. One unattended run on arrow-worker (exp-20261026);
  no default changed and no rule passes or fails
- **Layers:** L3 (the settled background the surface is fitted from), L4 (the clip), L5 (the
  planar state and the published box)
- **Related:** [state-estimation plan, P11](../../plans/lidar-state-estimation-plan.md#21-p11-is-a-correctness-problem-not-a-future-refinement),
  [Phase 0/1 corpus baseline](state-estimation-phase01-corpus-baseline.md#p11-ground-plane-fit-measured-per-site),
  [ground area](../../../data/experiments/ground/README.md), [l3bg0024](../../../data/experiments/l3bg/l3bg0024.md)

## Summary

- **Every site but four tilts by 1 % or more.** The global plane's gradient runs from 0.63 %
  (Embarcadero at Bay) to 5.53 % (California between Leon and Baker); 19 of the 23 first segments
  have 1 % or more, so ground0003 does not retire, and each of them is where ground0002 and
  ground0003 read first. The fixed band's error 30 m along the fall line reaches 1.66 m.
- **The planar speed does not care.** At the steepest site the planar speed is 0.15 % short of the
  speed along the plane (1 − cos(atan g)), and the steepest grade along a site's traffic is 5.16 %
  (0.13 %). The frame question is second order for speed, as the geometry said.
- **Much of the tilt is across the traffic, not along it.** At 14 sites the fall line lies within
  30° of square to the traffic's axis, with cross-slopes of 3.4 to 5.1 % at six of them. A road's
  crown is about 2 %, so a cross-slope of 4 % on a straight street is more likely the mount rolled
  towards or away from the road than the road itself: a pitched mount and a graded road are one
  number here, and this is the one the clip and the frame live with.
- **One plane is not enough where the traffic goes.** The cell planes depart from the global plane
  at the moving tracks' positions by more than the surface clip's 0.2 m floor at p90 on 22 of 23
  sites (p90 0.27 to 1.47 m), and by under 0.1 m on none. By the spec's reading, ground0002's
  one-plane control is expected to lose to its cell arms on every site but ashbury-downey.
- **The cells mostly earn it, not everywhere.** Read against the cells, a moving track's lowest
  return has a median slope of 0.0130 m/m against 0.0153 above the global plane and 0.0180 in the
  sensor frame; the cells flatten the bodies more than the plane at 17 sites, and 25 % of the
  moving tracks are explained by the cells against 11 % by the plane. At six sites (Haight at
  Divisadero worst) the plane does as well or better, so the cells are not right everywhere.
- **What the surface does not explain is mostly range, not kerbs.** The lowest return climbs with
  range, about 1.7 to 1.9 cm a metre: tracks moving away from the sensor rise above the surface
  (median +0.015 m/m above the cells) and tracks approaching fall (−0.013). The `both` class, 39 %
  of moving tracks against the cells, is dominated by this, which is l3bg0024's question, rather
  than by merges or kerbs.

## Method

**Replays.** `lidar-state-estimation-baseline -ground-report` on the first five-minute segment of
the 23 tuning and screen sites (embarcadero-folsom held out and unread), 200 s scored after a
70 s warm-up, the defaults' tuning, no evidence database. Each case replays twice; the repeat
runs without the report, and its baseline was byte-equal to the first at every site, so fitting
and reading the surface changed no track. On haight-divisadero the first run's baseline was also
byte-equal to a build of `main` without the report.

**The surface.** The P11 regional surface the clip and `box_ground` use
(`l3grid.FitRegionalGroundSurfaceFromBackground`, 10 m cells, global fallback), fitted once at the
first frame the background had settled, published and never applied
(`TrackingPipelineConfig.GroundSurfaceForReport`).

**The measures**, with the definitions and thresholds fixed in the spec before the replays:

1. _Grade and direction._ The global plane's gradient and the bearing of its fall line against
   the moving tracks' axis: the length-weighted axial mean of their net displacements, with its
   concentration (1 for one flow, near 0 for two equal crossing flows). The grade along and across
   is the plane's rise per metre along and across each track's course, median over the tracks.
2. _Departure from one plane._ At every sample of every moving track, one a track a frame, the
   cell's own plane less the global plane (0 where the global plane serves the cell): p50, p90,
   largest, and the signed median.
3. _Vertical motion._ For each moving track (10 samples or more, median speed 2 m/s or more, 5 m
   or more of travel), the least-squares slope of its associated cluster's lowest return against
   along-track distance in the sensor frame, above the blended cells and above the global plane
   alone. A slope rises or falls when it is 0.01 m/m or more, changes the return by 0.10 m or more
   over the track, and is two standard errors from zero; each track is `flat`, `explained` (the
   sensor frame only), `surface_only` or `both`.

The reading against the global plane alone was added after the instrument's check replay on
haight-divisadero, which showed the cells steeper than the bodies' paths there, and before the
corpus replays; no threshold changed.

## Results

| Measure (23 sites)                                       | Value                            |
| -------------------------------------------------------- | -------------------------------- |
| Sites with a grade of 1 % or more                        | 19                               |
| Steepest grade; along a site's traffic                   | 5.53 %; 5.16 % (median per site) |
| Planar speed's largest scale error                       | 0.15 %                           |
| Fixed band's largest error 30 m along the fall line      | 1.66 m                           |
| Fall line within 30° of square to the traffic            | 14 sites                         |
| Departure p90 over 0.2 m; under 0.1 m                    | 22 sites; none                   |
| Moving tracks read                                       | 2,087                            |
| Median \|slope\|: sensor frame, above cells, above plane | 0.0180, 0.0130, 0.0153           |
| Sites where the cells flatten more than the plane        | 17                               |

| Class against     | flat | explained | surface_only | both |
| ----------------- | ---: | --------: | -----------: | ---: |
| the blended cells | 22 % |      25 % |         14 % | 39 % |
| the global plane  | 28 % |      11 % |          8 % | 53 % |

Per site, steepest first. The angles are between undirected axes, 0 to 90°; the slopes are median
absolute metres per metre of travel.

| Site                  |  Grade | Fall line to course | Axis conc. | Grade along | Grade across | Departure p50 |    p90 |    Max | Moving tracks | \|b\| sensor | above cells | above plane |
| --------------------- | -----: | ------------------: | ---------: | ----------: | -----------: | ------------: | -----: | -----: | ------------: | -----------: | ----------: | ----------: |
| california-leon-baker | 5.53 % |                 21° |       1.00 |      5.16 % |       1.98 % |        0.54 m | 1.20 m | 1.86 m |            28 |        0.099 |       0.012 |       0.048 |
| ashbury-downey        | 5.49 % |                 67° |       0.98 |      2.17 % |       5.05 % |        0.04 m | 0.18 m | 2.08 m |            33 |        0.014 |       0.010 |       0.013 |
| laguna-eddy           | 4.97 % |                 17° |       0.26 |      4.71 % |       1.59 % |        0.67 m | 1.23 m | 3.50 m |            39 |        0.048 |       0.019 |       0.028 |
| pierce-haight         | 4.37 % |                 72° |       0.54 |      1.33 % |       4.16 % |        0.68 m | 1.28 m | 1.59 m |            18 |        0.046 |       0.018 |       0.040 |
| marina-webster-beach  | 4.35 % |                 83° |       0.99 |      0.52 % |       4.32 % |        0.16 m | 0.47 m | 2.68 m |            45 |        0.014 |       0.011 |       0.011 |
| haight-divisadero     | 4.25 % |                  0° |       1.00 |      4.25 % |       0.05 % |        0.58 m | 1.27 m | 2.37 m |            34 |        0.064 |       0.057 |       0.022 |
| marina-broderick      | 4.24 % |                 89° |       0.94 |      0.11 % |       4.23 % |        0.17 m | 0.41 m | 0.81 m |            89 |        0.011 |       0.009 |       0.012 |
| broadway-gough        | 4.01 % |                 76° |       0.47 |      1.17 % |       3.83 % |        0.70 m | 1.47 m | 2.88 m |           109 |        0.026 |       0.019 |       0.018 |
| howard-6th            | 3.43 % |                 88° |       0.84 |      0.25 % |       3.42 % |        0.31 m | 0.87 m | 1.25 m |            69 |        0.018 |       0.013 |       0.016 |
| columbus-north-point  | 3.41 % |                 86° |       0.77 |      0.38 % |       3.39 % |        0.33 m | 0.73 m | 1.96 m |            60 |        0.018 |       0.013 |       0.011 |
| franklin-mcallister   | 2.81 % |                 26° |       0.41 |      2.49 % |       1.30 % |        0.14 m | 0.49 m | 1.18 m |           123 |        0.017 |       0.019 |       0.014 |
| lombard-laguna        | 2.57 % |                 85° |       0.90 |      0.23 % |       2.56 % |        0.18 m | 0.49 m | 6.40 m |           153 |        0.013 |       0.010 |       0.012 |
| hyde-ofarrell         | 2.47 % |                 14° |       0.45 |      2.33 % |       0.83 % |        0.51 m | 1.20 m | 2.75 m |            52 |        0.042 |       0.018 |       0.020 |
| fulton-divisadero     | 1.94 % |                 72° |       0.45 |      0.64 % |       1.83 % |        0.49 m | 1.10 m | 2.43 m |            84 |        0.018 |       0.015 |       0.021 |
| bush-powell           | 1.90 % |                 14° |       0.95 |      1.84 % |       0.47 % |        0.41 m | 1.23 m | 3.93 m |            79 |        0.053 |       0.017 |       0.035 |
| 1st-mission           | 1.72 % |                 84° |       0.63 |      0.23 % |       1.70 % |        0.25 m | 0.61 m | 7.27 m |           141 |        0.021 |       0.023 |       0.017 |
| van-ness-sacramento   | 1.62 % |                 21° |       0.81 |      1.50 % |       0.60 % |        0.25 m | 0.78 m | 2.69 m |           133 |        0.034 |       0.018 |       0.027 |
| lombard-broderick     | 1.42 % |                 84° |       0.99 |      0.17 % |       1.41 % |        0.09 m | 0.27 m | 0.79 m |           202 |        0.014 |       0.009 |       0.013 |
| embarcadero-broadway  | 1.09 % |                 60° |       0.79 |      0.51 % |       0.96 % |        0.18 m | 0.32 m | 3.02 m |           166 |        0.018 |       0.012 |       0.015 |
| embarcadero-bryant    | 0.95 % |                 80° |       0.84 |      0.13 % |       0.94 % |        0.07 m | 0.27 m | 0.75 m |           105 |        0.017 |       0.011 |       0.016 |
| 3rd-folsom            | 0.93 % |                 55° |       0.85 |      0.52 % |       0.77 % |        0.06 m | 0.29 m | 1.62 m |            97 |        0.015 |       0.013 |       0.014 |
| columbus-broadway     | 0.88 % |                 68° |       0.70 |      0.49 % |       0.73 % |        0.15 m | 0.52 m | 2.09 m |           156 |        0.018 |       0.013 |       0.017 |
| embarcadero-bay       | 0.63 % |                 18° |       0.81 |      0.61 % |       0.16 % |        0.08 m | 0.33 m | 1.59 m |            72 |        0.010 |       0.005 |       0.008 |

**The tilt's direction.** Where one flow dominates (concentration 0.9 or more) the fall line is
along it at three sites (Haight at Divisadero 0°, Bush at Powell 14°, California 21°), nearly
square to it at four (Marina at Webster Beach 83°, Lombard at Broderick 84°, Lombard at Laguna 85°,
Marina at Broderick 89°), and between at Ashbury at Downey (67°). The four with the fall line
square to a single flow carry 1.4 to 4.3 % of cross-slope.

**The departure.** The largest departures (7.27 m at 1st at Mission, 6.40 m at Lombard at Laguna)
are single cells whose own plane is fitted to something other than the road and are read only as
an upper bound; p90 is the figure the clip meets. The signed medians are small (−0.63 to +0.55 m)
and of both signs: the cells do not sit consistently above or below the global plane.

**The bodies.** The sensor-frame slope follows the plane's grade along each course, with a gain of
1.68 (r 0.58 over 2,087 tracks): more than the grade alone. Split by direction against the
sensor, the 639 tracks whose range grows by half a metre or more per metre travelled have a median
slope of +0.0152 above the cells, and the 262 whose range shrinks as fast have −0.0129. Regressed
on the rate at which a track's range changes along it, the slope above the cells rises 0.019 m per
metre of range (r 0.27; 0.018 above the plane, 0.017 in the sensor frame): the lowest return
climbs with range by about 1.7 to 1.9 cm a metre on every surface read, which is what l3bg0024
measures by ring. It is why `both` is the largest class: a moving track's lowest return
climbs away from the surface as it recedes, whatever the surface.

## Reading, against the spec's

- **ground0003 does not retire.** 19 sites have a grade of 1 % or more. The planar speed's error
  there is at most 0.15 %, so its value is the trail's height and the box's base, not the speed.
- **ground0002 and ground0003 read first** at the 19 sites with 1 % or more; the steepest five are
  California between Leon and Baker, Ashbury at Downey, Laguna at Eddy, Pierce at Haight and
  Marina at Webster Beach.
- **One plane cannot clip correctly** at the 22 sites whose p90 departure exceeds 0.2 m; the mesh
  question closes at none. ground0002's cell arms are the expected winners, but the six sites where
  the plane flattens the bodies as well or better are where a cell arm may lose.
- **Listed tracks.** Each case's `ground_report.json` lists up to 25 `surface_only` and `both`
  tracks. Most are explained by range, not by a merge, a clip or a kerb; the listing is the place
  to look for the rest.

## What it does not establish

- **The ground's true grade.** The plane is fitted to the sensor's own returns, so a rolled or
  pitched mount and a graded road are one number. The cross-slopes suggest the mount; nothing here
  separates them.
- **Whether a surface clip is better.** This reads the surface and changes nothing; ground0002
  tests the clip.
- **The lower body's cause.** The climb with range is measured here only as a slope; which of
  absorption, occlusion and dropout takes the lower body is l3bg0024's.
- **Anything held out.** embarcadero-folsom and the second segments were not read.

## Provenance

| Item        | Value                                                                                                                                                                                                                        |
| ----------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Code        | `05ad73c4a` on `claude/ground0001-surface-read` (0.5.1-pre47), built with the Makefile's LDFLAGS; executable `sha256:95af4291…`; the instrument and definitions committed at `5f1c0b187` and `05ad73c4a`, before the replays |
| Tuning      | `config/tuning.defaults.json`, SHA-256 `aefcc10c…`; run parameters `sha256:a7713ff3…`, identical at every site                                                                                                               |
| Corpus      | the first-segment corpus and index of the October campaigns (`first-segment-corpus.json` SHA-256 `32e33814…`, `first-segment-site-index.json` `ccf180d3…`), 23 sites, 70 s warm-up, 200 s scored                             |
| Captures    | the NAS captures share read over NFS through a resolved root: ashbury-downey's `s2/s2-sf-0.pcapng` is an SMB symlink stub on the share, resolved to `s2-sf-0.pcapng` beside it                                               |
| Host        | arrow-worker (2 vCPU, 5.8 GB) beside its other services; 23 units in 1 h 38 min, 2 min 50 s to 5 min 41 s each                                                                                                               |
| Raw outputs | case summaries and `ground_report.json` per site under the run's scratch directory; the recordings archived to `velocity-campaign/exp-20261026/ground0001` on the results share; not in Git                                  |

## Recommendations

1. **Run ground0002 on the six sites where the plane holds its own as well as the steep ones.**
   Its cell arms are expected to win on 22 sites, but Haight at Divisadero, Broadway at Gough,
   1st at Mission, Columbus at North Point, Franklin at McAllister and Marina at Webster Beach are
   where a single plane does as well or better, and are the sites that test it.
2. **Measure the mount before reading a cross-slope as the road.** A per-site roll and pitch from
   the registered background (pose0001's 3DEP registration gives one) would separate the mount
   from the road; until then ground0003's frame arm should take the fitted plane as the frame.
3. **Read l3bg0024 against this table.** The lowest return's climb with range is about 1.7 to
   1.9 cm a metre on every surface; l3bg0024's ring classes say what takes the lower body, and
   ground0004's held base is the remedy if it is occlusion or dropout.
