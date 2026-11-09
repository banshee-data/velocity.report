# Two-point initialisation's speed defect: reflections, not arithmetic, and the guards that remove it

<!-- ignore-style-length -->

The November benchmark (l5id0019, on the open PR #994) found that the default-off
`two_point_velocity_init` raised the scored-window p98 speed by 5 to 18 m/s at every case. On
ashbury-downey's first segment, six tracks carried 191 rows over 15 m/s, up to 28.9 m/s, where the
defaults' fastest is 11.8 m/s. This run traces that defect to its cause, guards the option against
it, and scores the guarded option against the defaults. It answers
[l5est0006](../../../data/experiments/l5est/l5est0006.md).

- **Status:** Measured, 2026-11-09. The rule passes on all three clauses. The option stays
  default-off; no default changed
- **Spec:** [l5est0006](../../../data/experiments/l5est/l5est0006.md), rule committed at
  `c1486176` before any replay of the guarded option
- **Layers:** L5 initialisation (`initialiseTwoPoint`), read through the published box rows and
  kirk0's per-frame identity
- **Parameter sets:** the [defaults](../../../config/tuning.defaults.json) for both arms,
  fingerprint `ab0c4035780d`. The option is the replay experiment `two_point_velocity_init`, not a
  tuning key
- **Related:** [tracking maths §2.3](../../../data/maths/tracking-maths.md#two-point-velocity-initialisation),
  [state-estimation plan](../../plans/lidar-state-estimation-plan.md)

## Summary

- **The pair arithmetic did not invent the speeds. The clusters carried them.** Every published
  track over 15 m/s under the unguarded option, two on kirk0 and six on the ashbury segment, is a
  thin, sparse cluster. Each lies on a real car's bearing from the sensor, farther out. Its range
  ratio to the car matches its speed ratio: 2.0 to 2.4 on kirk0, 1.6 to 3.0 on ashbury. Measured:
  the bearings, ratios and courses below. Inferred: these are reflections of the cars. kirk0's
  reviewed labels hold a noise object in the windows of one of them.
- **The shipped tracker hides reflections by accident.** It cannot hold anything born above
  about 14 m/s, so a fast reflection is born again every few frames and never confirms. The
  unguarded option initialises the reflection's velocity exactly, as it was built to. The track
  confirms after four frames, loses the flickering cluster, and coasts at the reflection's speed.
  172 of ashbury's 191 fast rows are coasting rows.
- **A second cause is in the pairs, not in the rows.** Pairs whose second cluster is the whole body
  after a fragment imply 18 to 27 m/s from a reference point that slides along the body. On these
  two cases none of those tracks reached a published row.
- **The guards.** A pair whose second measurement lies in a confirmed track's shadow, as the
  sensor sees it, is declined. The velocity covariance is widened by how far the reference point
  can slide with the visible extent, and the pair is declined when that cannot be represented. A
  declined pair takes the shipped update.
- **Against the rule: passes.** Scored-row p98 change −0.001 m/s on kirk0 and +0.405 m/s on
  ashbury (limit +0.5). kirk0's switch change is 0 at its 95th percentile (limit +10). The 20 m/s
  noise-free vehicle is one track. No row over 15 m/s remains on either case.
- **What it costs.** The guarded option is now close to a no-op on kirk0's identity: switches 90
  against 90, fragmentations 45 against 46. The unguarded option's 15 fewer fragmentations went
  with its 30 extra switches. The shadow test declines 66 % of kirk0's pairs and 53 % of
  ashbury's. It has not been shown to help anywhere.

## Question and rule

The question, arms and rule are in the [spec](../../../data/experiments/l5est/l5est0006.md),
committed at `c1486176` before the first replay of the guarded option. The guarded arm passes when:

1. **(a)** the scored-window rows' p98 change from `lidar_figure_sensitivity.py`, guarded minus
   `a0`, is at most +0.5 m/s on kirk0 and on the ashbury segment;
2. **(b)** the 95th percentile of kirk0's paired-bootstrap change in switches is at most +10;
3. **(c)** `TestTwoPointVelocityInitHoldsAFastVehicleFromBirth` passes at the built commit.

The root-cause replays below came first. They ran the unguarded option, the benchmark's arm, and
were diagnostic. The guards were designed from them, and the rule tests the guards on the same two
cases. That is a limitation, recorded under [what this does not establish](#what-this-does-not-establish).

## Root cause

**Method.** `lidar-box-eval` was built at main `704b8e46` with a temporary stderr log in
`initialiseTwoPoint` and the update path, never committed. It ran the unguarded option on kirk0
whole (6 s warm-up, the rest scored) and on ashbury-downey's first five minutes (40 s warm-up,
120 s scored), with l5id0019's flags. The runs reproduce the benchmark's two-point rows: kirk0's
moving-track counts, p98 and fast tracks match, and ashbury's 191 rows over 15 m/s on six tracks
match. Only the track UUIDs differ. Each log line gave a pair's two measurements, interval,
implied velocity and the two clusters' extents, and the young track's next updates.

**The intervals are not short.** On kirk0, 122 pairs initialised, at intervals of 0.0975 to
0.3006 s, median 0.1000 s. On ashbury, 103 pairs were seen, median 0.1000 s. One pair 2.2 ms apart
implied 536 m/s and was already declined by the speed limit.

**Where the fast rows sit.** Each fast track has one to six observed rows, and the rest coast.

| Case    | Fast tracks | Rows over 15 m/s | Of them observed | Rows per track | Observed per track |
| ------- | ----------: | ---------------: | ---------------: | -------------: | -----------------: |
| kirk0   |           2 |               30 |                2 |       25 to 39 |             1 to 6 |
| ashbury |           6 |              191 |               19 |       25 to 40 |             1 to 5 |

**The clusters move fast.** On kirk0, the track that published 15.5 m/s had four consecutive
clusters 1.2 to 1.9 m apart at 0.1 s intervals: 12 to 19 m/s from the clusters themselves. Later
observations, 1.7 s on, still moved at about 14 m/s. They were 0.6 to 1.9 m long, 0.14 to 0.54 m
wide and held 10 to 22 points. Under the defaults no track exists there. A car, `a0`'s
`trk_fb7bf15f` (7.95 to 5.70 m/s), was passing nearer the sensor on the same course.

**A nearer car on the same bearing, at the matching ratio.** The replay tracks in the sensor
frame, so the sensor is at the origin. For each observed fast row, the scratch script took the
observed row of another track at the same instant with the closest bearing, and a smaller range.
It counted a pair where the bearing differed by under 6° and the course by under 30°. Each fast
track's best such pair:

| Case    | Fast track (max) | Partner, class | Bearing difference | Range ratio | Speed ratio | Course difference |
| ------- | ---------------- | -------------- | -----------------: | ----------: | ----------: | ----------------: |
| kirk0   | 15.5 m/s         | car            |               2.5° |        2.38 |        2.05 |              5.0° |
| kirk0   | 15.2 m/s         | car            |               4.5° |        2.16 |        2.16 |               n/a |
| kirk0   | 14.8 m/s (below) | dynamic        |               4.8° |        2.01 |        2.14 |              6.0° |
| ashbury | 28.9 m/s         | car            |              11.4° |        2.88 |        2.83 |               n/a |
| ashbury | 23.3 m/s         | car            |              14.6° |        2.81 |        2.71 |              2.0° |
| ashbury | 23.5 m/s         | car            |               3.7° |        1.75 |        2.18 |              2.5° |
| ashbury | 19.0 m/s         | car            |               4.9° |        1.63 |        1.71 |              1.9° |
| ashbury | 16.5 m/s         | car            |               5.1° |        1.91 |        1.71 |              6.3° |
| ashbury | 15.8 m/s         | car            |               3.6° |        1.79 |        1.53 |              3.5° |

The two ashbury rows at 11° and 15° are outside the 6° rule. Their partners were 6.5 to 7 m from
the sensor, where a 5 m car spans about 40° of bearing, so the cluster lies within the car's
angular extent. A point at k times a body point's range on its bearing moves at about k times its
speed, which is what the ratios show.

What is measured: every fast track has such a partner, and every partner is a published car or
moving road user. What is inferred: the fast clusters are reflections of those cars, a returned
pulse that travelled farther than the line of sight. The data do not show which path. kirk0's
reviewed labels hold `obj_6075854b`, class `noise`, in windows 118 to 129, inside the 15.5 m/s
track's life of about windows 114 to 131. Its masks were not decoded to positions, so that match
is by time only.

**Why the shipped tracker never confirms them.** At birth it has zero velocity with unit variance.
After its second update the velocity is still near zero, and a cluster moving 1.5 to 3 m a frame
leaves the gate within a frame or two. The cluster is born again and never reaches four hits. That
is the same cold-start limit the option was built to fix
(`TestTwoPointVelocityInitHoldsAFastVehicleFromBirth`: one 20 m/s vehicle, born 10 times in 3 s).
The pair initialises a reflection's velocity exactly. The reflection holds for four frames and
confirms. It then flickers out, and the confirmed track coasts at its speed for up to 25 frames.

**The second cause: the reference point slides.** On kirk0, 14 pairs implied over 15 m/s. Among
them, the second cluster was much longer than the first:

- 0.2 to 4.4 m, implying 18.8 m/s;
- 0.6 to 4.2 m, implying 23.8 m/s;
- 1.1 to 3.9 m, implying 25.9 m/s;
- 0.7 to 4.3 m, implying 25.7 m/s;
- 1.8 to 12.5 m (a truck), implying 23.1 m/s.

A body first seen as a fragment and then whole moves its cluster mean by up to half the extent
change without moving. None of these tracks published a row. The benchmark's 30 extra switches on
kirk0 were not traced to them. The [kirk0 identity](#kirk0-identity) table shows only that they
went away with the guards.

**A guard read offline.** Each initialisation's second measurement was tested against the other
confirmed tracks' published boxes at the nearest instant, with a 1° margin. All 8 fast tracks lay
in a nearer box's shadow. So did 12 of 25 tracks that looked real on ashbury (30 rows or more,
half observed) and 19 of 45 on kirk0, often behind a parked car or a large static object. A
decline only returns a pair to the shipped update. A false positive therefore costs nothing
against the defaults, but it removes the option from that pair.

## The fix

Commit `0f0316b8`, in `initialiseTwoPoint` and
[two_point_init.go](../../../internal/lidar/l5tracks/two_point_init.go). The option is
default-off, keeps its replay name, and declines in two new cases besides the existing ones:

- **Shadow.** The second measurement lies on a bearing between another confirmed track's box
  corners, as the sensor sees them, and farther than its nearest corner. The sensor is at
  `NoiseSensorX`, `NoiseSensorY`, which the adaptive noise model already reads. The pipeline
  clusters without a pose, so zero is correct today. A tentative track casts no shadow, and
  neither does a track without a positive, finite box.
- **Reference slack.** σ_g is half the change between the two clusters' extents along the unit
  displacement u, read from each cluster's OBB. Without a heading it is half the change in the
  larger dimension. The velocity block becomes (2R + σ_g² u uᵀ)/Δt². The position and cross
  blocks are unchanged. A diagonal above `MaxCovarianceDiag` declines, as a short Δt already did.
  At the shipped R, cap and Δt = 0.1 s, that is σ_g above about 0.95 m, an extent change of about
  1.9 m.

To make the slack readable, `initTrack` keeps the founding cluster's footprint. No other code
reads it.

**Tests,** in the same commit:

- `TestTwoPointVelocityInitOffKeepsTheShippedArithmetic` pins seven scenes, association records
  included, with the option off. Its digests were taken on `704b8e46` before the change, and none
  moved. With the option on, the digests of the association and hygiene scenes did not move; the
  guards do not reach them. The digests of the reflection scenes, the default pass and the end-on
  truck did move, by design. The test lists the unguarded values beside the new ones. Disabling
  each guard in turn showed the slack alone moves the truck, and both guards reach the default pass.
- `TestTwoPointVelocityInitDoesNotHoldAReflection` is the defect as a scene: a car at 8 m/s and a
  thin cluster at 2.2 times its range for ten frames, beside a vehicle born from a rear fragment.
  The unguarded option confirmed the reflection at 17.6 m/s and published the 6 m/s vehicle at
  26 m/s. The guarded option confirms the two vehicles at their own speeds, as the shipped tracker
  does.
- Unit tests of the shadow test and of the slack, including the decline that leaves the shipped
  update bit for bit and the widened covariance staying positive definite.
- The existing two-point tests pass unchanged, including the 20 m/s vehicle held as one track.
  `TestHygieneExperimentsOnKirk0` (`pcap` tag) passes: the option still changes kirk0's recording.

## Results against the rule

| Clause                | Read                                                         | `a0`    | Guarded | Change     | Limit      | Holds |
| --------------------- | ------------------------------------------------------------ | ------- | ------- | ---------- | ---------- | ----- |
| (a) kirk0             | rows p98 of moving tracks' maximum speeds                    | 8.154   | 8.153   | −0.001 m/s | +0.5       | Yes   |
| (a) ashbury           | the same                                                     | 11.2045 | 11.6095 | +0.405 m/s | +0.5       | Yes   |
| (b) kirk0             | paired bootstrap Δ switches, 5th / 50th / 95th (2,000 draws) |         |         | 0 / 0 / 0  | 95th ≤ +10 | Yes   |
| (c) synthetic, 20 m/s | tracks created for one vehicle over 3 s                      | 10      | 1       |            | 1          | Yes   |

`a0` reproduces l5id0019's figures: kirk0 rows p98 8.154, ashbury 11.2045, and kirk0's tracking
baseline digest `79d9abff…`. For reference, the unguarded option's p98 changes were +7.00 and
+17.74 m/s.

Ashbury's +0.405 m/s is within the limit by 0.095 m/s. Both p98 values, 11.61 and 11.20 m/s, are
below the segment's fastest row, 11.80 m/s, which is the same in both arms. The change comes with
two fewer moving tracks, which moves the nearest-rank p98. It is not a faster track.

## Reported, not decided

**Rows over 15 m/s.** None in either arm on either case. The benchmark's falsifier ("still writes
rows over 15 m/s where `a0` writes none") does not apply.

| Case    | Arm                  | Moving tracks |    p85 |     p98 | Fastest row | Rows over 15 m/s |
| ------- | -------------------- | ------------: | -----: | ------: | ----------: | ---------------: |
| kirk0   | `a0`                 |            51 |   6.98 |   8.154 |        9.04 |                0 |
| kirk0   | guarded              |            51 |   6.98 |   8.153 |        9.34 |                0 |
| kirk0   | unguarded (l5id0019) |            57 |  7.442 |  15.155 |       15.51 |               30 |
| ashbury | `a0`                 |            24 | 10.724 | 11.2045 |       11.80 |                0 |
| ashbury | guarded              |            22 | 11.072 | 11.6095 |       11.80 |                0 |
| ashbury | unguarded (l5id0019) |            29 |   16.5 |  28.947 |       28.95 |              191 |

Moving tracks, p85 and p98 are `lidar_figure_sensitivity.py`'s, on rolling one-second medians.
The fastest row is the largest published speed of any row.

**The ashbury moving count, 24 to 22 (−8.3 %), and p85 +0.35 m/s.** Every `a0` moving track
passes within 2 m of a guarded moving track at a shared instant, so none is simply lost. Which two
`a0` tracks the guarded arm holds as one was not established. In a queue of vehicles the 2 m match
is ambiguous. Duplicate pairs stay at one, with 15 pair-frames against 86.

<a id="kirk0-identity"></a>

**kirk0 identity**, full comparison, then the paired bootstrap against `a0` (19 scored objects).

| Arm                  |   HOTA |   IDF1 | Switches | Frag |  FN | Δ HOTA 5th / 50th / 95th    | Δ IDF1 5th / 50th / 95th    | Δ frag 5th / 50th / 95th |
| -------------------- | -----: | -----: | -------: | ---: | --: | --------------------------- | --------------------------- | ------------------------ |
| `a0`                 | 0.4284 | 0.4728 |       90 |   46 | 265 |                             |                             |                          |
| guarded              | 0.4281 | 0.4725 |       90 |   45 | 266 | −0.0009 / −0.0002 / +0.0003 | −0.0009 / −0.0003 / 0.0000  | −3 / −1 / 0              |
| unguarded (l5id0019) | 0.4238 | 0.4689 |      120 |   31 | 248 | −0.0123 / −0.0043 / +0.0047 | −0.0182 / −0.0019 / +0.0144 | −41 / −15 / 0            |

No interval lies wholly on one side of zero. kirk0's split is tuning data, not held out.

**What the guards decided.** These counts come from a scratch build of `c1486176` with one stderr
line per decision, never committed. Its rows equal the guarded run's once track IDs are dropped.

| Case    | Pairs | Initialised (all widened) | Shadow | Slack over the cap | Speed limit | Shadow, implied over 15 m/s |
| ------- | ----: | ------------------------: | -----: | -----------------: | ----------: | --------------------------: |
| kirk0   |   123 |                        42 |     81 |                  0 |           0 |                          13 |
| ashbury |   108 |                        48 |     57 |                  2 |           1 |                          21 |

No initialised pair implied more than 15 m/s. Every initialised pair carried some slack, so none
used the unwidened covariance.

## Interpretation

**The defect was in what the option let through, not in its formula.** The pair formula is exact
for a pair of one body's measurements. On these captures the pairs it got wrong were not one
body's measurements seen directly. The shipped tracker's zero-velocity start never let a fast
reflection confirm, which hid this from every replay before the option existed. The defaults have
no reflection filter. They have a cold-start limit that acts as one on streets where traffic is
slower than about 14 m/s.

**The guards remove the defect, and most of the option's reach with it.** On kirk0 the guarded
option and the defaults agree to within one fragmentation. The unguarded option's fragmentation
gain did not survive. Nothing here shows whether it came from real vehicles held earlier or from
reflections and fragments counted as continuous tracks. The option's stated purpose is to hold a
real vehicle faster than about 14 m/s from birth. These two cases do not test that. No track in
either arm publishes a row above 11.8 m/s that is not a reflection. The defaults could not show a
faster vehicle if one were there, so the evidence for one would have to come from elsewhere: the
radar, labels, or a faster street.

**The shadow test is a perception question asked at L5.** It declines half the pairs on these
cases, mostly real ones behind a nearer confirmed body, because a near car's box spans tens of
degrees of bearing. It is safe as a fallback. As a reflection filter it is coarse. A reflection
that is not in a confirmed track's shadow, or that confirms under the defaults on a faster street,
is not removed by it. The same reflections still reach L4 and the tentative tracks under every
arm.

## What this does not establish

- **Held-out evidence.** The guards were designed after reading kirk0 and the ashbury segment, and
  the rule was scored on the same two cases. kirk0 is a tuning capture.
- **That the fast clusters are reflections.** Measured: bearing, range ratio, speed ratio and
  course against a nearer car. The physical path is inferred. The labels carry no reflection
  class, and kirk0's noise object was matched by time, not position.
- **Any benefit of the option.** No real vehicle above 14 m/s appears in either case, so the
  option's purpose is untested on real data. Its gain is shown only on the synthetic 20 m/s
  vehicle.
- **The source of the unguarded option's 30 extra kirk0 switches.** They are gone under the
  guards; which pairs caused them was not traced.
- **Haight-divisadero and ashbury's second segment.** The benchmark read them; this run did not.
- **Pi cost.** The shadow test visits every live track at each tentative track's second frame. It
  was not timed. The replays ran on a shared cloud container, 4 cores, beside other work.

## Recommendations

1. **Keep `two_point_velocity_init` off.** The guarded option passes its rule, but it is not
   shown to help. Read it next on a capture with road users above 14 m/s, where the shipped cold
   start fails and the option has something to fix.
2. **Treat reflections as an L4 question.** A cluster in a nearer body's shadow, at a multiple of
   its range and speed, is identifiable before it reaches the tracker. Labelling a few such
   clusters on kirk0 and ashbury would give a reflection class to measure a filter against. The
   L5 shadow test should not become the place reflections are handled.
3. **Before any read of the robust or combined arms with two-point,** rerun them at `0f0316b8` or
   later. From that commit the experiment name runs the guarded option, and the replay header's
   `build_git_sha` says which ran. The parameter hash does not: `3184a2e6c676` names both builds.

## Provenance

| Item              | Value                                                                                                                                                                                                                                                                                                                                                                                                    |
| ----------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Base              | main `704b8e46` (`704b8e46bf24e505f1e7bfd794e3617547e6bbb6`)                                                                                                                                                                                                                                                                                                                                             |
| Fix               | `0f0316b8` (`0f0316b86e8ff1f7122c43cf6ae86aedadb137a7`); spec `c1486176`, which changes no tool source                                                                                                                                                                                                                                                                                                   |
| Tools             | Built at `c1486176` (`c148617657758a4fb534164b407321d1555a0c3d`) with the Makefile's LDFLAGS (`version.Version` 0.5.1-pre52) and the `pcap` tag; summaries record that build. `lidar-box-eval` `sha256:1e25428d…`, `lidar-state-estimation-baseline` `f79f1686…`, `lidar-ground-truth-eval` `f59c260a…`                                                                                                  |
| Diagnostic builds | the root cause at `704b8e46` with a temporary log; the decision counts at `c1486176` with a temporary log; neither committed                                                                                                                                                                                                                                                                             |
| Parameter set     | `config/tuning.defaults.json`, fingerprint `ab0c4035780d`, file SHA-256 `9ad7377f…`; arm parameter hashes `a0` `ab0c4035780d`, guarded `3184a2e6c676`                                                                                                                                                                                                                                                    |
| Replays           | `lidar-state-estimation-baseline`, kirk0 whole, l5id0019's corpus, index and cohort manifest, `-warmup 6 -require-settled=false -sample-points 1024 -campaign-metrics`, evidence per case, repeat-verified, a scratch copy of l5id0019's use ledger; `lidar-box-eval -variants none -rows -require-settled=false -sample-points 1024`, kirk0 `-warmup 6 -duration 0`, ashbury `-warmup 40 -duration 120` |
| Identity          | `perframe` on the online-stage estimates as declared baselines, `-bootstrap 2000 -bootstrap-seed 20261108`; pack `ds_67842cb1b22b70e7` as reconstructed for l5id0019 (pack digest `sha256:f0ed3414…`), split `kirk0-tuning` (manifest `7194bb61…`, reference digest `sha256:a784e69d…`), `obj_788622b5` ignored                                                                                          |
| Captures          | kirk0 `sha256:2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a`; `20260901T1225_80858-0ac_ashbury-downey` from `banshee-data/sf-street-speeds`, `sha256:0f6811cd96c8a3ae…`, 2,960,318,212 bytes, deleted once cut; its first 300 s by l5id0019's pcapng window filter, `sha256:971e093e…`, 540,018 UDP 2369 packets, equal to l5id0019's segment                                         |
| Host              | Claude Code cloud container, 4 cores shared with other sessions; raw output in the session scratch, deleted once summarised                                                                                                                                                                                                                                                                              |
