# The occupancy profile anchored to the body and admitted on the box's heading, on kirk0 and two placements

<!-- ignore-style-length -->

geom0019's occupancy profile failed all four clauses of its rule, and its report traced both
causes to things other than keeping free space: a grid whose origin moved with the extent state,
and an admission that read the solid body's own heading. This report reads a default-off shadow,
`solid_body_extent_profile_anchored`, which is the same profile with those two things changed,
against a rule committed before the option existed. It answers
[geom0020](../../../data/experiments/geom/geom0020.md).

- **Status:** Measured on kirk0 and on the first five-minute segments of ashbury-downey and
  haight-divisadero, 2026-11-10. P0 holds; P1, P2, P3 and P4 fail. Nothing adopted, no default
  changed
- **Spec:** [geom0020](../../../data/experiments/geom/geom0020.md); rule at `39705e4`, before the
  option existed; option at `fb8a764`, before any replay
- **Layers:** L5, the shadow solid body and its profile block; scored against L10 physical
  references on kirk0 and label-free on the placements
- **Parameter sets:** the [defaults](../../../config/tuning.defaults.json), fingerprint
  `ab0c4035780d`. Every option is a replay experiment, not a tuning key
- **Related:** [the occupancy profile (geom0019)](extent-occupancy-profile-2026-11.md),
  [shared-orientation duplicates](shared-orientation-duplicates-2026-11.md),
  [geom0015's bias baseline](d2-frozen-split-bias-baseline-2026-11.md),
  [state plan §9.2.4](../../plans/lidar-state-estimation-plan.md#924-extents-in-the-state-and-censored-extent-evidence-delivered-default-off)

## Answer

- **The anchored profile is not a candidate.** P0 holds and P1 to P4 fail. Nothing is adopted.
- **Admission is no longer the limit.** On the box's heading the profile reports an extent on all
  22 reviewed poses, where geom0019's had one. It admits 54 % of kirk0's associated frames (4.8 %
  before), 58 % on ashbury-downey (31 %) and 42 % on haight-divisadero (6.8 %).
- **Anchoring holds truck 2 across the extent state's jump.** At 77.4 s the extent state takes its
  first bound and its length goes from 4.28 to 8.97 m. geom0019's profile went from 9.9 to 12.3 m
  in the next frames. The anchored profile reads 9.6 to 9.9 m from 75.6 s to 77.4 s and 9.7 to
  10.1 m from 77.4 s to 80 s, against the reference's 9.63 m.
- **P1 fails on truck 2 because its first four poses see one end.** The profile reads 4.2 to 4.3 m
  there with its rear end censored, then 9.2 and 9.6 m on poses 504 and 509. The median error of
  the six is 5.33 m against the bar of 2.54 m. On truck 1 it reads 10.9 to 11.1 m against 10.63 m,
  a median error of 0.47 m against the bar of 0.37 m.
- **P2 fails on length and holds on width.** Car 8 reads 4.7 to 4.8 m against 4.26 m (median error
  0.54 m against 0.365 m) and 1.9 m wide (under 2.0 m).
- **Label-free, the profile grows at speed.** P3: its within-track length spread is 1.47 m against
  the box's 0.82 m on ashbury-downey, and 0.74 m against 0.25 m on haight-divisadero. 10 of 18 and
  10 of 19 tracks shorten by more than 0.5 m after lock-on. At haight-divisadero cars at 8 to 10 m/s
  end at 7 to 9 m, the excess at the front end. P4: it holds more of the withheld returns than the
  solid body's own extent (93.1 % against 84.4 %, 90.6 % against 74.5 %), with 1.25 and 1.48 times
  its area at the median, over the 1.15 bound.

## Question and rule

The rule is in the [spec](../../../data/experiments/geom/geom0020.md), written before the option
existed. It is geom0019's, with P0 added and P4 restated:

- **P0, admitted at all.** The anchored profile reports an extent on at least 4 of truck 2's six
  poses, 5 of truck 1's nine and 4 of car 8's seven. Below that, P1 and P2 are not read.
- **P1, long bodies.** Truck 2's median absolute length error is at least 0.5 m under the
  published box's 3.04 m. Truck 1's is no more than 0.25 m over the box's 0.12 m.
- **P2, cars.** Car 8's median absolute length error is at most 0.365 m and its median width under
  2.0 m.
- **P3, steadiness, label-free.** On each placement segment, over confirmed vehicle tracks of 3 s or
  more, the median within-track spread of the profile's length after its first second (p90 less
  p10) is no more than the box's, and under 5 % of tracks shorten by more than 0.5 m after lock-on.
- **P4, withheld frames, label-free, restated.** Every fifth admitted frame is withheld. The share
  of its returns inside the profile's extent at its pose, grown by 0.25 m, is at least the solid
  body's own extent's share on the same frames, grown the same, and the profile's area is no more
  than 1.15 times the solid body's at the median.
- **Verdict.** A candidate when P0 to P4 hold. A fail names its clause.

The measurement details are geom0019's: P3 and P4 are read per segment and must hold on both;
P3's spread is over the frames after a track's first scored second on which the profile is known,
with the box's on the same frames; P4 reads all confirmed tracks' withheld frames with the profile
known, and each share is the mean over frames, the returns-weighted share reported beside it. The
solid body's extent is its reported length and width at its centre and orientation.

## What the option does

`SolidBodyOptions.ExtentProfileAnchored`, replay name `solid_body_extent_profile_anchored`
([`solid_body_extent_profile_anchored.go`](../../../internal/lidar/l5tracks/solid_body_extent_profile_anchored.go)),
is geom0019's profile with its constants unchanged and two changes. The grids, the height band,
the occupied and free evidence, the lock-on, the end marks and the withheld frames are the
profile's, applied by the same code (`extentProfile.advance`). It writes only its own grids and the
reading's `Profile` block. No track, no solid-body state and no published box changes.

- **The grid is held to the body.** The grid's origin is the solid body's centre plus an offset in
  the body frame, along and across the profile's axis. The offset starts at zero on the first
  admitted frame. On every later associated frame of the track, admitted or refused, it takes back
  the part of the centre's change that the extent state made. A change of the half-extents, `Δh`,
  moves the centre by the joint state's regression of position on the half-extents,
  `P_ch · P_hh⁻¹ · Δh`, read from the frame's posterior covariance. That is exactly the move a span
  bound's estimate projection makes, since the projection moves the mean by that regression and
  leaves `P` alone. It is also the part of a face update's move that goes with the half-extents.
  On a frame where the reference changes, between the medoid and the centre, the origin follows the
  solid body's velocity over the interval and the rest of the jump goes to the offset. The rest of
  the centre's change, the prediction's `v·dt` and the innovation the half-extents do not carry, is
  the body's tracked motion, and the origin follows it.
- **The axis is the published box's heading.** The believed axis is the box model's heading
  filter, read as `solid_body_shared_orientation` reads it: its `ψ` and `P_ψψ`, on a frame the box
  updated, once the box is labelled. A frame is admitted when that standard deviation is 5 degrees
  or less, with the body referenced to its centre and past initialising as before. There is no
  course fallback. The profile runs after step 3d, where the box holds this frame's posterior, and
  reads the solid body as step 3c left it.
- **The reading's pose** is the anchored origin and the box's heading, so the extents placed there
  are where the profile infers the body to be.
- **The rows.** `lidar-box-eval -rows` writes the same columns as for geom0019's profile. On a
  withheld frame it now also writes the share of the frame's returns inside the solid body's own
  extent at the solid body's pose, grown by 0.25 m, and the profile's area over that extent's
  (`wh_sb_share`, `wh_sb_area_ratio`), for the restated P4. The two columns are filled for
  geom0019's profile too.

**Tests, in the same commit.** The through-truck pass (9.6 by 2.5 m at 7 m/s, 4 m from the sensor)
is driven through both profiles with the solid body's centre at the true centre. At the midpoint,
abeam, the extent state shortens the truck by 3 m with its rear face held: its half-length falls
1.5 m and its centre moves 1.5 m back, through a cross-covariance that makes that the joint
regression. geom0019's profile then reads up to 11.1 m. The anchored profile's extents equal, frame
for frame, those of the same pass without the change, and its length stays at 9.7 to 9.8 m, within
0.2 m of the truck's (0.1 m cells). A car whose box heading has a standard deviation of 30 degrees
is refused, as a heading refusal, while its solid body's own orientation is settled; at 1 degree it
is admitted although the body's own orientation is not. An unlabelled box, a box not updated on the
frame and a medoid-referenced body are refused. Without a change of the extent state the two
profiles give the same reading on every frame. The replay name is refused without `solid_body`,
beside `solid_body_extent_profile`, and without the box model.

**Off-pin digests.** The four solid-body digests geom0019 pinned, taken at `ab19e51` before either
profile existed, are unchanged with the option off and with it on. A new digest of geom0019's own
profile readings on two through-truck scenarios, taken at `39705e4` before this change, is
unchanged, so `solid_body_extent_profile` reads as it did. The defaults' fingerprint is unchanged.

## kirk0: the existing arms reproduce geom0019

All four arms ran `lidar-state-estimation-baseline` on kirk0 whole, repeat-verified, with tools
built at `fb8a764`. Every arm's tracking baseline is `79d9abff…`, first and repeat, so neither
profile changes a track. The solid-body rows are `perframe -b-solid-body` on each arm's baseline
evidence and on its `lidar-box-eval` evidence, gate 3 m, on geom0015's 22 poses; both give the
same rows.

| Arm          | Matched | Median centre | Truck 1: L, \|err\|, W | Car 8: L, \|err\|, W | Truck 2: L, \|err\|, W | geom0019                   |
| ------------ | ------: | ------------: | ---------------------- | -------------------- | ---------------------- | -------------------------- |
| `a0` box     | 22 / 22 |       0.190 m | 10.60, 0.12, 2.94      | 4.40, 0.14, 1.87     | 6.59, 3.04, 2.84       | 0.190 m; the same rows     |
| `face-state` | 22 / 22 |       1.028 m | 9.73, 0.90, 3.82       | 3.90, 0.35, 2.45     | 4.60, 5.03, 1.90       | 1.028 m; the same rows     |
| `profile`    | 22 / 22 |       1.028 m | 9.73, 0.90, 3.82       | 3.90, 0.35, 2.45     | 4.60, 5.03, 1.90       | 1.028 m; the same rows     |
| `anchored`   | 22 / 22 |       1.028 m | 9.73, 0.90, 3.82       | 3.90, 0.35, 2.45     | 4.60, 5.03, 1.90       | (new; `face-state`'s body) |

The profile arm's own figures are geom0019's to the digit: truck 2's profile on pose 509 only,
9.50 by 3.00 m with its centre 0.28 m from the reference, 5 frames admitted and 111 refused (81 by
the heading) by then. Over kirk0 it admits 329 frames and refuses 6,469 (4,114 by the heading), 11
of 79 confirmed tracks lock, and its label-free measures read a spread of 0.61 m against the box's
0.63 m and a withheld share of 97.5 % with an area ratio of 1.17 to the box. On the placements its
rows give geom0019's table: 944 and 339 frames admitted, spreads 0.60 and 0.20 m, falls 7 of 15 and
0 of 17, withheld shares 97.1 % and 94.9 %.

## kirk0: the anchored profile on the reviewed poses

The profile is read from the anchored arm's `lidar-box-eval -rows`, joined to each pose's
solid-body row by frame time and position (to 0.01 m), as in geom0019. Its centre is its extents'
midpoint placed at its pose.

| Vehicle | Poses | With a profile | Admitted by the last pose | Refused by then (heading) | Locked at | Profile L, W (median) | \|L err\| median | Centre median | Ends          | Box L, \|err\| |
| ------- | ----: | -------------: | ------------------------: | ------------------------: | --------- | --------------------- | ---------------: | ------------: | ------------- | -------------- |
| Truck 1 |     9 |              9 |                       277 |                    23 (8) | 46.3 s    | 11.10, 3.70           |           0.47 m |        0.32 m | all four      | 10.60, 0.12    |
| Car 8   |     7 |              7 |                       108 |                    12 (0) | 65.0 s    | 4.80, 1.90            |           0.54 m |        0.08 m | all four      | 4.40, 0.14     |
| Truck 2 |     6 |              6 |                        83 |                    33 (3) | 60.1 s    | 4.30, 3.50            |           5.33 m |        2.85 m | rear censored | 6.59, 3.04     |

Times are the capture's timestamps less 1,764,973,000 s, as in geom0019's report. Truck 1 reads
10.9 to 11.1 m on every pose against the reference's 10.63 m, and car 8 4.7 to 4.8 m against
4.26 m; both are longer than the reference and than the box, with every end observed. Over the 22
poses the profile's centre sits a median 0.30 m from the reference, against the extent state's
1.03 m and the box's 0.19 m.

Truck 2's six poses, in metres:

| Pose | Time   | Box L (centre error) | Extent state L (centre error) | Anchored profile L, W (centre error) | Ends (rear, front, right, left)        |
| ---: | ------ | -------------------- | ----------------------------- | ------------------------------------ | -------------------------------------- |
|  491 | 73.8 s | 4.10 (2.95)          | 4.84 (2.66)                   | 4.2, 3.5 (2.96)                      | censored, observed, observed, observed |
|  495 | 74.2 s | 2.78 (2.83)          | 4.80 (2.67)                   | 4.3, 3.5 (2.85)                      | the same                               |
|  500 | 74.7 s | 3.59 (2.50)          | 4.64 (2.63)                   | 4.3, 3.5 (2.85)                      | the same                               |
|  502 | 74.9 s | 9.42 (0.30)          | 4.56 (2.65)                   | 4.3, 3.5 (2.87)                      | the same                               |
|  504 | 75.1 s | 9.09 (0.49)          | 4.50 (2.70)                   | 9.2, 3.5 (0.47)                      | the same                               |
|  509 | 75.6 s | 9.49 (0.21)          | 4.38 (2.66)                   | 9.6, 3.5 (0.38)                      | the same                               |

On the first four poses the profile and the box both read the end of the truck they see, and the
profile marks its rear end censored: it reports a lower bound, as it should. Its length errors are
5.43, 5.33, 5.33, 5.33, 0.43 and 0.03 m. The box reaches the truck's length two frames earlier, at
pose 502.

## Truck 2 from 70 to 80 s

From the anchored arm's rows (label-free after 75.6 s), with geom0019's profile beside it:

| Interval    | Frames | Anchored profile L | Its width  | Ends                                            | geom0019's profile L     | Extent state L |
| ----------- | -----: | ------------------ | ---------- | ----------------------------------------------- | ------------------------ | -------------- |
| 70.0–73.0 s |     25 | 3.3 to 4.1         | 3.1 to 3.2 | rear censored                                   | none (not admitted)      | 4.2            |
| 73.0–75.1 s |     21 | 4.0 to 9.1         | 3.2 to 3.5 | rear censored                                   | none                     | 4.4 to 4.8     |
| 75.1–75.6 s |      5 | 9.2 to 9.5         | 3.5        | rear censored                                   | 9.5 from 75.3 s          | 4.4 to 4.5     |
| 75.6–77.4 s |     18 | 9.6 to 9.9         | 3.0 to 3.5 | rear censored; left censored from 76.0 s        | 9.7 to 9.9               | 4.3            |
| 77.4–80.0 s |     26 | 9.7 to 10.1        | 3.1 to 3.7 | every end but the left observed, 77.7 to 79.6 s | 12.3 to 12.5 from 77.5 s | 8.7 to 9.0     |

At 77.43 s the extent state's length goes from 4.28 to 8.96 m and the solid body's centre moves
back along the body: its x goes from −7.23 to −8.01 m on that frame, where the truck advances about
0.3 m in x a frame. The anchored origin carries on at the truck's pace, from −6.98 to −6.66 m, and
the profile's
extent does not move: 9.9 m, along −8.3 to +1.6 m, before and after. geom0019's profile, whose
origin is that centre, read 12.3 m on the next frame. This is Recommendation 2's test, and the
anchored profile holds near 9.6 m across 77.4 s. It reads up to 10.1 m by 78.4 s, as its width
grows from 3.1 to 3.7 m.

## The placements

`lidar-box-eval -variants none -rows -require-settled=false -sample-points 1024` with an evidence
database, 40 s warm-up and 120 s scored, `profile` and `anchored`. The published boxes of both arms
are the box model's own, which neither profile changes.

| Segment              | Arm      | Admitted / refused (heading) | Tracks locked | Vehicle tracks ≥ 3 s (scored) | Profile spread | Box spread | Falls > 0.5 m | Withheld frames | Profile share (pooled) | Solid body share (pooled) | Area ratio to the solid body | Box share |
| -------------------- | -------- | ---------------------------- | ------------: | ----------------------------: | -------------: | ---------: | ------------- | --------------: | ---------------------- | ------------------------- | ---------------------------: | --------: |
| ashbury-downey-s1    | profile  | 944 / 2,108 (948)            |       16 / 41 |                       18 (15) |         0.60 m |     0.95 m | 7 / 15, 47 %  |             183 | 97.1 % (98.1 %)        | 94.5 % (95.6 %)           |                         1.17 |   100.0 % |
| ashbury-downey-s1    | anchored | 1,781 / 1,271 (111)          |       22 / 41 |                       18 (18) |         1.47 m |     0.82 m | 10 / 18, 56 % |             348 | 93.1 % (97.0 %)        | 84.4 % (85.7 %)           |                         1.25 |   100.0 % |
| haight-divisadero-s1 | profile  | 339 / 4,678 (2,791)          |       21 / 57 |                       19 (17) |         0.20 m |     0.00 m | 0 / 17, 0 %   |              59 | 94.9 % (93.5 %)        | 95.1 % (85.5 %)           |                         1.08 |   100.0 % |
| haight-divisadero-s1 | anchored | 2,085 / 2,932 (1,045)        |       33 / 57 |                       19 (19) |         0.74 m |     0.25 m | 10 / 19, 53 % |             316 | 90.6 % (91.3 %)        | 74.5 % (52.3 %)           |                         1.48 |   100.0 % |

The share columns
are the mean over withheld frames, with the returns-weighted share in brackets, and each segment's
solid-body share is on the same frames as the profile's. The profile rows read geom0019's profile
against the restated bar, which was not its rule.

Over the frames where the anchored profile is known, the end marks read (observed, censored): on
ashbury-downey rear 1,496, 634; front 1,861, 269; right 1,944, 186; left 1,863, 267. On
haight-divisadero rear 2,084, 592; front 2,236, 440; right 2,180, 496; left 2,356, 320. On kirk0
the anchored arm admits 3,655 frames and refuses 3,143 (788 by the heading), and 49 of 79 confirmed
tracks lock.

**Where the length goes.** At each vehicle track's last known frame the anchored profile's ends
were compared with the published box's, along the profile's axis. The rear ends agree to a median
0.10 m on both segments. The front reaches a median 1.04 m beyond the box's front on
ashbury-downey and 3.14 m on haight-divisadero, where cars at 8 to 10 m/s read 7 to 9 m against the
box's 3.9 to 6.0 m. Over each track the anchored origin moves a median 0.6 m relative to the box's centre,
forward on ashbury-downey and back on haight-divisadero, which is less than the profile's median
growth from lock-on, 3.1 and 4.8 m.

## Against the rule

| Clause                | Figure                                                                                                          | Result |
| --------------------- | --------------------------------------------------------------------------------------------------------------- | ------ |
| P0, admitted          | an extent on 6 of 6 truck 2 poses (4 needed), 9 of 9 truck 1 (5) and 7 of 7 car 8 (4)                           | holds  |
| P1, truck 2           | median \|L err\| 5.33 m against the bar of 2.54 m (box 3.04 m less 0.5 m)                                       | fails  |
| P1, truck 1           | 0.47 m against the bar of 0.37 m (box 0.12 m plus 0.25 m)                                                       | fails  |
| P2, car 8             | median \|L err\| 0.54 m against at most 0.365 m fails; median width 1.90 m against under 2.0 m holds            | fails  |
| P3, ashbury-downey    | spread 1.47 m against the box's 0.82 m fails; falls 56 % against under 5 % fails                                | fails  |
| P3, haight-divisadero | spread 0.74 m against 0.25 m fails; falls 53 % fails                                                            | fails  |
| P4, ashbury-downey    | share 93.1 % against the solid body's 84.4 % holds; area 1.25 times the solid body's against at most 1.15 fails | fails  |
| P4, haight-divisadero | share 90.6 % against 74.5 % holds; area 1.48 times fails                                                        | fails  |
| Verdict               | not a candidate: P1, P2, P3 and P4 fail; admission does not limit it                                            |        |

The pooled shares give the same results (97.0 % against 85.7 %, 91.3 % against 52.3 %). P1 fails
on truck 2 however its censored poses are read: on the two poses where its rear end is in view
(504 and 509) the errors are 0.43 and 0.03 m, but the rule asks for the median of six.

## Interpretation

Each reading is marked measured or inferred.

**The box's heading admits the profile.** Measured: on the box's heading the profile is admitted
on 54 % of kirk0's associated frames and on every reviewed pose, against 4.8 % and one pose on the
solid body's believed axis. Heading refusals fall from 4,114 to 788 on kirk0. Most of what is
still refused is a body not yet referenced to its centre or still initialising. Inferred: the box
filter's `P_ψψ` at or under 5 degrees is the usual state of a labelled box on these captures, so
geom0019's Recommendation 3, a length-dependent bound, is not needed to admit frames.

**Anchoring removes the smear the extent state wrote.** Measured: truck 2's anchored profile does
not move when the extent state's length doubles at 77.4 s, and the synthetic through-truck test
gives the same extents with and without a 3 m change of the extent state. Inferred: the 12.5 m that
geom0019 read was the extent state's projection, which the regression takes back exactly.

**Truck 2's P1 is observation, not the estimator.** Measured: on its first four poses the profile
and the box both read 2.8 to 4.3 m, and the profile marks the rear censored; on the two poses with
the side in view it reads 9.2 and 9.6 m against 9.63 m. Inferred: the rule's median of six asks
for a length the sensor had not yet seen. The censored mark says so; a scorer that read a censored
end as a lower bound would not count those poses as errors.

**The profile grows at speed, at the front.** Measured: on the placements the anchored profile's
lengths grow a median 3.1 and 4.8 m from lock-on, half the tracks shorten by more than 0.5 m
somewhere after, the excess sits at the front end while the rear agrees with the box's, and the
origin moves less than a metre relative to the box's centre. On kirk0, at 1 to 5 m/s, truck 1 and
car 8 read about 0.5 m longer than their references. Inferred, and not separated here: once the profile is admitted on half the
frames of fast vehicles, the remaining motion of the solid body's centre relative to the body
reaches the grid. A face update moves the centre and the half-extents together, and the regression
takes back only the part that goes with the half-extents. Locked cells inside the body's own
bearings are freed only by a return beyond them, and a receding vehicle's front is hidden, so a
front end written ahead of the body stays. The 0.1 m cells and the lock-on at two frames of
occupancy make any return ahead of the body lengthen it.

**P4 holds on containment and fails on area.** Measured: the profile holds 9 and 16 points more of
the withheld returns than the solid body's own extent, because the extent state is short, but at
1.25 to 1.48 times its area. Inferred: containment here is bought with the same length the P3
growth shows, which is what the area clause guards against.

## Recommendations

1. Keep `solid_body_extent_profile_anchored` default-off, as a shadow. It does not pass its rule,
   and the two-dimensional successor the verdict names is not specified.
2. Keep the box's heading as the profile's axis in any successor. It is what admits the profile,
   and the box sits 1.2 degrees from reviewed yaw at the median (geom0015).
3. Before another estimator, measure what the origin follows. A diagnostic that places the grid on
   the box's own centre, or on the observed near end, beside this anchor, on the same placements,
   would say whether the growth at speed is the origin or the grid's ratchet. It needs its own
   spec, and it decides P3, not P1.
4. Score a censored end as a lower bound. A P1 restated for a successor should count a pose whose
   end is censored as an error only when the reported extent exceeds the reference, so that a body
   seen from one end is not read as the estimator's failure.
5. Give the replay manifest a code identity for experiment options before a column is added for
   the profile, as geom0018's and geom0019's reports recommend.

## What this does not establish

- **Anything beyond kirk0's three reviewed vehicles and two five-minute segments.** All of it is
  tuning data that geom0019 had already read, and the two changes were chosen after reading its
  failure.
- **Physical accuracy of the placements' figures.** P3 and P4 are label-free. They measure what
  the pipeline drew against its own returns, not where the vehicles were.
- **Which of the two changes causes the growth at speed.** No arm runs either change alone.
- **The box's heading through a turn.** The box's heading filter is smoothed, and none of the
  reviewed poses is in a turn.
- **The free-space model's errors**, as in geom0019: it does not see static background, and a
  parked car shadowing an end reads as free.
- **Cost.** The replays ran beside other agents' jobs on a four-core host (load average 10 to 13),
  so per-frame timings are not read.

## Provenance

| Item        | Value                                                                                                                                                                                                                                                                                                                              |
| ----------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Base        | `39705e4` (the geom0020 and l5id0024 specs)                                                                                                                                                                                                                                                                                        |
| Commit      | option and tests `fb8a764`                                                                                                                                                                                                                                                                                                         |
| Tools       | built at `fb8a764` by `make` with the Makefile's LDFLAGS and the `pcap` tag, `0.5.1-pre52`, `GitSHA` `fb8a76453adc0710b76a00ccc254d0c83e53b95c`: `lidar-state-estimation-baseline` `9db954d0…`, `lidar-box-eval` `751fa599…`, `lidar-ground-truth-eval` `0f80eb38…`. Every replay header stamps the sha                            |
| Replay      | `lidar-state-estimation-baseline`, kirk0 whole, `-warmup 6 -require-settled=false -sample-points 1024 -campaign-metrics`, evidence per case, every arm repeat-verified; the kirk0 corpus, index and cohort manifest; a scratch use ledger at migration 71 per arm                                                                  |
| Box replays | `lidar-box-eval -variants none -rows -require-settled=false -sample-points 1024` with an evidence database: kirk0 `-warmup 6 -duration 0`, every arm; placements `-warmup 40 -duration 120`, `profile` and `anchored`                                                                                                              |
| Arm hashes  | `a0` `ab0c4035780d`; `face-state` `80e04c6ea08e`; `profile` `43c2e3673c92`; `anchored` `79eaf41c3116`. Tuning `config/tuning.defaults.json`, fingerprint `ab0c4035780d`, file SHA-256 `9ad7377f…`                                                                                                                                  |
| Tracks      | tracking baseline `79d9abff…` for every arm, first and repeat; source manifest file `d7e33e0d…`                                                                                                                                                                                                                                    |
| Capture     | kirk0, `sha256:2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a`                                                                                                                                                                                                                                                   |
| Segments    | first five minutes, cut by the scratch pcapng window filter (l5id0019's method): ashbury-downey `971e093e…`, haight-divisadero `2afd5be4…`                                                                                                                                                                                         |
| Labels      | the reconstructed pack `ds_67842cb1b22b70e7` (`sha256:f0ed3414…`) and physical references revision 77 (`sha256:189ecf65…`, re-pinned copy `fef9f97f…`); split `kirk0-tuning` drafted (`7194bb61…`), reference digest `a784e69d…`, `obj_788622b5` ignored                                                                           |
| Scoring     | `perframe -physical-reference -physical-reference-revision 77 -physical-gate-metres 3`, online estimates as declared baselines, `-a-boxes -b-solid-body`; the profile joined from the box-eval rows by frame time and solid-body position; P3 and P4 by geom0019's scratch script over the rows, extended for the solid-body share |
| Host        | Claude Code cloud container, 4 cores shared with other agents; raw output in the session scratch, not kept                                                                                                                                                                                                                         |
