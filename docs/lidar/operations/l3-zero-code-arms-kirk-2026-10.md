# Zero-code L3 arms on kirk0 and kirk1, October 2026

The first experiment the [beam-wise review](../../../data/maths/proposals/20261011-beamwise-background-subtraction-review.md)
asked for (Section 6, "Z0"): the five L3 arms that need no new code, run through the campaign's
offline harness on the two labelled captures, and scored with the review's Stage A metrics.

- **Status:** Measured, October 11, 2026. One machine, two captures, one site. Nothing shipped
  changes; no tuning default moves
- **Question:** does narrowing L3's acceptance window (region overrides off, then a narrower
  base window, then no neighbour confirmation) recover the road-user returns the shipped grid
  absorbs, and at what cost in false foreground, stationary tracks and time?
- **Answer:** the region overrides are the lever. Switching them off takes kirk0 road-user
  object-frame recall from 59% to 94% (pedestrians 28% to 91%) and covers every labelled road
  user on kirk1 (38 to 49 of 49), but multiplies unlabelled clusters by 39 and per-frame false
  positives by 12, and creates stationary tracks where there were none. The narrower base
  windows add no recall over that and only more speckle. No arm passes the review's Stage A
  rule; every one trips its M3 kill criterion. The next arm needs code: a range-adaptive support
  filter beside the override switch, and the dwell policy

## 1. Arms

All arms are the shipped `config/tuning.defaults.json` with the keys below changed in
`l3.ema_baseline_v1`, run through `lidar-state-estimation-baseline`, the campaign harness, which
can also switch the B8 region overrides off as the replay experiment `no_region_overrides`.

| Arm | Keys                                                      | Experiment            | Window at a settled cell                                                   |
| --- | --------------------------------------------------------- | --------------------- | -------------------------------------------------------------------------- |
| A0  | shipped                                                   | none                  | $3(s + 0.02r + 0.01) + 0.15$, then ×3 to ×8 on the range term after settle |
| A1  | shipped                                                   | `no_region_overrides` | $3(s + 0.02r + 0.01) + 0.15$                                               |
| A2  | `closeness_multiplier` 1.5                                | `no_region_overrides` | $1.5(s + 0.02r + 0.01) + 0.15$                                             |
| A3  | `noise_relative` 0.0005                                   | `no_region_overrides` | $3(s + 0.0005r + 0.01) + 0.15$: flat, about 0.2 m                          |
| A4  | `noise_relative` 0.0005, `neighbour_confirmation_count` 0 | `no_region_overrides` | as A3, no same-ring rescue                                                 |

A3 stands in for the review's E1 (a flat window) with existing keys: `noise_relative` 0 is
coerced to 0.01 at runtime, so the smallest positive value is used.

## 2. Method

**Captures.** kirk0 is the in-repo reference capture (`internal/lidar/perf/pcap/kirk0.pcapng`,
83 s, the same file as the labelled run). kirk1 was read from `/Volumes/lidar/lidar/pcaps/kirk/`
and trimmed to its first 490,000 UDP packets (272 s), which covers the 178 s labelled window,
so that the harness did not digest 4.9 GB per arm.

**Replay.** `-warmup 6 -sample-points 1024 -campaign-metrics`, with `-duration 0` on kirk0 and
`-duration 172` on kirk1. Evidence starts when L3 settles, at 5.9 s on both captures in every
arm. Each arm replays twice; the harness confirmed the repeat byte-equal every time. The harness
and scoring tools were built from this tree at `8a7f90447` with the version stamp set by hand
(`go build -tags=pcap`), since only the velocity binary goes through `make`.

**Scoring.** From each arm's `observations.db` (per-frame L4 clusters with up to 1,024 retained
points, and per-frame track estimates):

- **M1, labelled road users covered.** The labelled reference tracks are the campaign's, in
  `lidar_run_tracks` of the deployment database: kirk0 run `dd98c68e`, 16 road users; kirk1 run
  `60a4774c`, 49. Their spans are first-sighting snapshots (gap M5, pre-#584), a mean of 0.4 to
  1.0 s, so the evaluator's temporal-IoU rule finds 2 of 16 for the shipped arm against the
  campaign's 7 and cannot be used. The usable count is "covered": a candidate track overlaps at
  least 30% of the reference's span. Both are reported.
- **M2, how low the clusters reach.** kirk0 only, against the annotation pack `8e422582`'s
  ground truth (proposals included, 2,584 road-user object-frames from sample 60): for each
  labelled object-frame the clusters claiming it (centroid within 1 m plus half the footprint
  diagonal), and the lowest retained point inside the labelled footprint with a 0.2 m margin,
  minus the labelled bottom $z - l_z/2$. Positive means the cluster starts above the label's
  bottom; negative means it reaches below it. The label's bottom is the pack's, recorded by a
  run with a loosened L3, so it is not the road.
- **Object-frame recall and unlabelled clusters** on the same ground truth, as in the
  annotation-scored tuning scorer; and the reviewed pack `ad8b9438` through
  `lidar-ground-truth-eval perframe` with the `kirk0-ad8b9438-tuning-r1` split (every reviewed
  object; a tuning split, not held out; online stage, declared baseline) for MOTA, HOTA, IDF1
  and the per-frame counts.
- **M3, stationary tracks.** Tracks with more than 20 s (and 60 s) of estimates whose p90 speed
  is under 0.3 m/s.
- **M4,** seconds from capture start to the first observation. **M5,** the harness's frame
  timing for the whole L3 to L6 callback on an M1 Pro: not a Pi figure.

## 3. Results

### 3.1 kirk0

| Arm | M1 covered | Clusters/frame | FG points/frame | Object-frame recall |    Car / ped / bus | Unlabelled clusters/frame | Bottom in footprint p10 / median / p90 (m) | M3 stationary >20 s / >60 s | M5 p50 / p95 (ms) |
| --- | ---------: | -------------: | --------------: | ------------------: | -----------------: | ------------------------: | -----------------------------------------: | --------------------------: | ----------------: |
| A0  |   11 of 16 |           3.43 |           1,172 |               0.587 | 0.63 / 0.28 / 0.53 |                      0.31 |                      −0.01 / +0.26 / +0.91 |                       0 / 0 |        5.0 / 46.8 |
| A1  |   12 of 16 |          16.70 |           3,343 |               0.938 | 0.95 / 0.91 / 0.37 |                     12.05 |                      −1.03 / −0.32 / +0.01 |                       5 / 3 |       12.8 / 81.4 |
| A2  |   12 of 16 |          24.34 |           4,241 |               0.953 | 0.98 / 0.88 / 0.37 |                     20.19 |                      −1.62 / −0.54 / −0.11 |                       8 / 1 |      24.7 / 163.6 |
| A3  |   12 of 16 |          49.26 |           5,099 |               0.944 | 0.97 / 0.82 / 0.35 |                     45.04 |                      −1.76 / −0.63 / −0.18 |                      16 / 2 |      21.9 / 125.3 |
| A4  |   12 of 16 |          61.64 |           5,453 |               0.944 | 0.97 / 0.82 / 0.35 |                     57.43 |                      −1.76 / −0.63 / −0.18 |                      22 / 5 |       12.2 / 33.0 |

Per-frame evaluator, reviewed pack, all episodes (3,709 reference points):

| Arm |   MOTA |  HOTA |  DetA |  AssA |  IDF1 |   IDR |   IDP |    FN |     FP | IDSW |  FM |
| --- | -----: | ----: | ----: | ----: | ----: | ----: | ----: | ----: | -----: | ---: | --: |
| A0  |  0.148 | 0.376 | 0.234 | 0.679 | 0.350 | 0.275 | 0.482 | 2,315 |    722 |  124 |  48 |
| A1  | −1.623 | 0.309 | 0.148 | 0.720 | 0.317 | 0.656 | 0.209 |   690 |  8,609 |  430 |  66 |
| A2  | −2.849 | 0.258 | 0.099 | 0.729 | 0.249 | 0.675 | 0.153 |   720 | 13,414 |  142 | 153 |
| A3  | −6.445 | 0.204 | 0.057 | 0.814 | 0.158 | 0.716 | 0.089 |   651 | 26,821 |  142 |  68 |
| A4  | −8.315 | 0.190 | 0.047 | 0.838 | 0.142 | 0.778 | 0.078 |   617 | 33,821 |  113 |  47 |

Where A1's unlabelled clusters are, per frame: 0.37 within 10 m, 3.08 at 10 to 20 m, 5.41 at
20 to 40 m, 3.18 beyond 40 m. Of the 12.05: 1.45 have every point within 25 cm of a road 2.25 m
below the sensor (road surface), 4.71 have fewer than 8 points, and 1.19 sit at one 0.5 m
position in at least half the frames. For A3 the same figures are 45.04, with 2.97 road surface,
18.26 under 8 points and 5.60 recurring; for A4, 57.43, 3.05, 20.57 and 12.82.

### 3.2 kirk1

| Arm | M1 covered | Candidate tracks | Clusters/frame | FG points/frame | Road-surface clusters/frame | M3 stationary >20 s / >60 s | M5 p50 / p95 (ms) |
| --- | ---------: | ---------------: | -------------: | --------------: | --------------------------: | --------------------------: | ----------------: |
| A0  |   38 of 49 |               88 |           2.33 |             372 |                        0.31 |                       1 / 0 |        6.9 / 35.7 |
| A1  |   49 of 49 |              227 |          12.76 |           1,373 |                        1.81 |                      10 / 1 |        7.3 / 26.0 |
| A2  |   49 of 49 |              258 |          18.66 |           1,795 |                        3.26 |                      20 / 3 |        9.1 / 25.0 |
| A3  |   49 of 49 |              332 |          33.99 |           2,032 |                        3.44 |                      26 / 8 |        6.5 / 15.8 |
| A4  |   49 of 49 |              325 |          44.10 |           2,457 |                        3.56 |                     36 / 15 |        8.6 / 25.9 |

M4 was 5.9 s on both captures in every arm.

## 4. Interpretation

1. **The region overrides are the largest single lever on recall, and B8 is confirmed on
   labels.** The shipped grid misses 41% of kirk0's road-user object-frames and 72% of its
   pedestrian frames; with the overrides held off it misses 6% and 9%, and on kirk1 it covers
   every labelled road user instead of 38 of 49. Nothing else in the arms moved recall: A2 adds
   1.5 points, A3 and A4 add none.
2. **The cost is a flood of mid-range speckle, not road leakage.** A1 reports 12 unlabelled
   clusters a frame against 0.31; 70% of them lie at 10 to 40 m, 39% have fewer than 8 points,
   12% are road surface, and 10% recur at one position, which is static structure flickering
   in and out of the foreground. This is the review's C4 with its mechanism corrected: the ×8
   "volatile" override was suppressing far-field flicker, and the ×3 and ×4 overrides on the
   quiet terciles were absorbing the lower part of near-field road users at the same time. The
   two effects come from one switch, which is why a targeted replacement (a range-adaptive
   support filter, E4) is the right next arm and the overrides as they stand are the wrong tool
   for either job.
3. **The base window's shape is not the lever.** The flat window (A3) recovers no more than A1
   and quadruples the speckle; at 40 m it is 0.2 m wide against A1's 2.6 m, and at that range
   oblique surfaces and vegetation move more than 0.2 m. The review's C1 stands in its original
   form ("the window is decisive") and its sharpened form ("flat beats proportional") is
   falsified on these captures. E1 as written, a flat window with a specification floor and no
   learned term wide enough for range, should not proceed.
4. **Neighbour confirmation costs no recall here and suppresses recurring flicker.** A4 against
   A3: recall identical, unlabelled clusters up by a quarter, recurring positions up from 5.6
   to 12.8 a frame, and the frame p50 nearly halved. Its second job from the review's Section
   5.2 is visible in the numbers; its recall cost, with the overrides off, is not.
5. **The lower band is recovered, and ground comes with it.** The shipped clusters' lowest
   in-footprint points sit a median 0.26 m above the labelled bottoms (p10 at the bottom, p90
   0.91 m above), which is the direction and order of the review's C2 band once the label's own
   offset from the road is allowed for. With the overrides off they reach a median 0.32 m
   below the labelled bottoms: road returns beside the vehicle inside a 0.2 m margin, attached
   by DBSCAN's 0.8 m eps. The bus's object-frame recall falling from 0.53 to 0.37 is the same
   effect seen through the centroid gate. L4 has no ground test at road height (its band floor
   is 2.8 m below the sensor at a 2.3 m mount), so once L3 stops absorbing the road, nothing
   downstream removes it.
6. **Stationary tracks appear under every narrowed arm**: 0 to 5, 8, 16 and 22 on kirk0 over
   20 s, 1 to 10, 20, 26 and 36 on kirk1. With the overrides off this is the review's Section
   5.1 arrival regime, in which a newly stationary object is never absorbed, plus tracks on
   recurring flicker. The review's M3 kill criterion is tripped by every arm.
7. **Time.** Frame p50 on the Mac rises 2.5 to 5× under A1 to A3 from the clusters and tracks
   the extra foreground feeds to L4 and L5, and A4's p50 is lower than A3's because the
   neighbour tests are gone. None of this is a Pi measurement.

**Against the review's Stage A rule.** M1 and M2 move the right way under A1; M3 and precision
move the wrong way, and the rule adopts nothing. That is the result the rule was written to
give: narrowing alone is not shippable, and it says which code to write next.

## 5. What this does not establish

- One site, two captures, 83 and 172 scored seconds, one machine. The per-frame scores use a
  tuning split, not a held-out one, and their false positives count hypotheses on returns nobody
  labelled, so they are an upper bound; the deltas between arms are sounder than any arm's
  count.
- The kirk0 object-frame ground truth includes accepted proposals, and its bottoms come from a
  pack recorded by a loosened L3. The in-footprint measure removes road beside the object but
  not the label's own offset from the road, which is unknown.
- M1's reference spans are first-sighting snapshots. "Covered" is robust to that; the IoU rule
  is not, and the campaign's 7 of 16 is not reproduced by it for that reason.
- A3 approximates a flat window with existing keys; it has no learned-spread term wide enough
  to track range, which is what E1 would add. The result says the base window is not the lever
  on these captures, not that a learned window could not help at range.
- No Pi timing, no long capture, so the arrival regime's dependence on uptime (review Section
  5.1) is inferred from the stationary-track counts, not measured.

## 6. Recommendations

1. **Do not ship any of these arms.** The B8 switch alone is not a fix.
2. **Next arms need code, as replay experiments** (no fingerprint cost): A1 plus a
   range-adaptive support filter in index space (E4, replacing neighbour confirmation only if it
   also suppresses recurring flicker); A1 plus region overrides restricted to beams beyond a
   range or above the horizon, which keeps their flicker suppression and drops their near-field
   cost; and either with an L4 ground test at road height, since the recovered band brings
   road returns with it.
3. **The dwell policy** (review Section 6) is a prerequisite for any arm with the overrides off:
   the stationary-track counts show the arrival regime on an 83 s capture.
4. **Re-score with a held-out split and a full-scene kirk0 label** before any adoption decision;
   the Stage B of the review's rule.

## 7. Provenance

- Tree `8a7f90447` (branch `claude/roadside-lidar-eval-9ce17d`), velocity binary
  `v0.5.1-pre44`; `lidar-state-estimation-baseline`, `lidar-ground-truth-eval` and
  `lidar-track-scorecard` built from the same tree with `-tags=pcap`.
- kirk0: `internal/lidar/perf/pcap/kirk0.pcapng`; harness corpus and index from
  `cmd/tools/lidar-state-estimation-baseline/testdata/`. kirk1:
  `/Volumes/lidar/lidar/pcaps/kirk/kirk1.pcapng`, first 490,000 UDP packets, last packet at
  272.2 s; a one-case corpus and index written for it outside the repository.
- Labels: deployment database runs `dd98c68e-4e51-4155-a1ee-1d5bc12535ae` (kirk0) and
  `60a4774c-db3e-4008-9b7e-d1059ec27319` (kirk1), `user_label` not noise; annotation pack
  `8e422582…-20260922-043329` ground truth (`kirk0-tuning/ground-truth.json`, revision 204);
  reviewed pack `ad8b9438…-20260929-184718` with split `kirk0-ad8b9438-tuning-r1.json`
  (frozen split `sha256:b58ee590…`, revision 1).
- Harness flags as in Section 2; `-source-manifest` written per arm. Every arm's repeat replay
  was byte-equal to its first.
- Scoring scripts (about 300 lines of Python over `observations.db` and the ground-truth JSON)
  and the per-arm score rows are in the session's scratch directory, not the repository; the
  evidence databases (0.1 to 0.9 GB each) were deleted after scoring. The method above is
  complete enough to rewrite them.
