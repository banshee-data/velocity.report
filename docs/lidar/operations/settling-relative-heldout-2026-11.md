# A convergence criterion relative to the scene's spread, held out on 22 second segments

<!-- ignore-style-length -->

l3bg0022 found that the absolute spread delta measures traffic rather than convergence, and read a
relative criterion off its first-segment trajectories: the spread delta judged against 0.075 of
the observed cells' mean spread. This report replays the second five-minute segment of the 22
sites of corpus `d2-second-segment`, which no settling measure had read, under the honest first
check with and without that ratio, and applies the rule the November 5 review fixed before any of
them was read. It answers [l3bg0030](../../../data/experiments/l3bg/l3bg0030.md).

- **Status:** Measured, 2026-11-10. One unattended cloud run (4 shared cores), 06:30 to 09:20 UTC;
  no default changed
- **Layers:** L3 (settling: the warm-up ceiling and the convergence criterion)
- **Related:** [settling convergence on the first segments](settling-convergence-corpus-2026-11.md)
  (l3bg0022), [the kirk0 cloud run](cloud-run-kirk0-labelled-2026-11.md#l3bg0030-on-kirk0),
  [the November 5 review](experiment-review-2026-11-05.md#d-what-counts-as-noise)

## Answer

**The ratio fails its held-out clause, so it is not proposed as the default.** Under
`settling_relative_0075`, 17 of the 22 second segments converge before the 30 s ceiling (the rule
needs 18), and two of them, howard-6th and pierce-haight, converge at the first measured check
(the rule needs none). The 17 converge at a median of 17.9 s. On the first segments l3bg0022 read,
the same ratio gave 19 of 23 and none at the first check, so the figure chosen after looking did
not hold out, though it missed by one site and two early stops rather than by a wide margin.

- **The ratio's floor sits close to 0.075 everywhere.** Each site's lowest ratio before 30 s runs
  from 0.048 to 0.081, median 0.064; five sites never reach 0.075 (floors 0.076 to 0.081) and
  three more reach it only once or twice. The ratio swings by 0.02 to 0.13 within a site before
  30 s as traffic passes (0.30 at columbus-broadway, with one check at 0.207), so a fixed ratio settles a site on whichever check traffic happens to leave
  quiet.
- **kirk0 (clause 2) holds, kirk1 was not read.** kirk0's ratio never falls under 0.075 before the
  ceiling (lowest 0.076, median 0.104), so `ratio` settles at 30 s as `ceiling` does and scores the
  same: HOTA 0.3921, IDF1 0.4363, 91 identity switches under both, on the reconstructed pack.
  kirk1 is not in this container, so its half of the clause is unread.
- **Downstream (clause 3, reported, not gating): 6 of 22 sites are named.** At 3rd-folsom,
  broadway-franklin, california-leon-baker, embarcadero-broadway, fulton-divisadero and howard-6th
  the spurious clusters a frame in the 30 s after the ratio arm settles run 1.5 to 3.3 times the
  same site's 60 to 120 s. The median site runs 1.00 times. Under `ceiling`, which settles every
  site at 30 s, the same read also names 6 sites (median 1.16), so the read does not single the
  ratio out: an early window that runs high is traffic as often as an unsettled background.

## The rule

Fixed by the [November 5 review](experiment-review-2026-11-05.md) before any held-out segment was
read, and quoted from the spec: `ratio` is proposed as the default criterion when all of these
hold.

1. 18 or more of the 22 held-out sites converge before the 30 s ceiling under the ratio, and none
   at the first measured check (l3bg0022's reading of the 50-frame clause);
2. on kirk0, HOTA under `ratio` is no more than 0.01 below the `ceiling` arm's, and kirk1 covers no
   fewer of its 49 labelled road users;
3. the downstream cost is reported and does not gate: at each held-out site, spurious clusters a
   frame in the 30 s after the criterion settles, against the same site's 60 to 120 s after it. A
   site whose first 30 s run 1.5 × or more above its steady state is named.

A pass is a recommendation only; the owner decides the default.

## Method

**Arms.** Both run [`config/tuning.defaults.json`](../../../config/tuning.defaults.json)
(fingerprint `ab0c4035780d`) unchanged; the options are Go-only replay experiments, so neither arm
moves the fingerprint.

| Arm       | Tuning file                   | Fingerprint    | Replay experiments                          | Replay parameter hash  |
| --------- | ----------------------------- | -------------- | ------------------------------------------- | ---------------------- |
| `ceiling` | `config/tuning.defaults.json` | `ab0c4035780d` | `settling_honest`                           | `sha256:8c9fb1a1…1900` |
| `ratio`   | `config/tuning.defaults.json` | `ab0c4035780d` | `settling_honest`, `settling_relative_0075` | `sha256:d989ab6a…6e0c` |

`settling_relative_0075` (built for the kirk0 cloud run) replaces the absolute spread-delta
threshold with 0.075 times the check's mean spread over observed cells; the other three criteria
(coverage 0.8, region stability 0.95, mean confidence 10) and the 30 s ceiling stay as the
defaults set them.

**New in this branch.** The background records when its warm-up ended and whether convergence or
the ceiling ended it (`BackgroundManager.SettledAfter`), and
`lidar-state-estimation-baseline -spurious-clusters` writes, per tracker update of the first run,
the clusters the tracker was given and those no lasting track took. A lasting track is one
confirmed at any frame; a spurious cluster is unassociated or taken only by a track that was never
confirmed. A cluster that births a lasting track is unassociated on its birth frame, so each
lasting track's first cluster counts once. The tool sums the spurious clusters a frame from the
settle to 30 s after it and from 60 to 120 s after it, and fails the case if the repeat settled at
another time. Both read state and change no estimate.

**Held-out replays.** Each second segment (300 to 600 s from the placement's first packet, cut with
the run's `pcapng_window.py` from the `banshee-data/sf-street-speeds` placement, one at a time,
each placement deleted once cut) was replayed under each arm with
`lidar-state-estimation-baseline -settling-trajectory -spurious-clusters`, 120 s warm-up and 120 s
scored, `-require-settled=false`, first run and repeat, both repeat-verified. haight-divisadero's
second segment was the run's shared copy. The ceiling arm's trajectory is every convergence check
to 30 s; under `ratio` the trajectory ends at its settle. Up to its settle each `ratio` trajectory
equals the `ceiling` one check for check at every site, and the settle the ceiling trajectory
predicts for the ratio (the first measured check meeting all four criteria with the relative
threshold) equals the time the `ratio` replay settled at every site, so the ratio's decision was
read from the replay that made it, not inferred.

**Reading clause 1.** As l3bg0022 read it: a site converges before the ceiling when the `ratio`
replay settles on convergence before 30 s; checks run every ten frames from the 50-frame minimum
(5.9 s, unmeasured under `settling_honest`), so the first measured check is at 6.9 s at every site,
and a site settling there fails the second half of the clause.

**kirk0.** kirk0 whole at 6 s warm-up under each arm, with an evidence database, scored by
`lidar-ground-truth-eval perframe` against the reconstructed pack ad8b9438 (digest
`sha256:f0ed3414…15dc`), split `kirk0-tuning`, ignoring the parked car `obj_788622b5`, `ratio`
against `ceiling`. kirk1 is on a LiDAR volume, not on the dataset, and was not read.

**Order and coverage.** Sites ran whole, both arms in parallel, smallest placement first after
haight-divisadero, so partial coverage would still have given whole sites. All 22 were reached; no
site failed or was skipped.

## Results

### Per site

"Settles at" is when each arm's warm-up ended; `ceiling` never converges under the absolute 0.001,
so it settles at the 30 s ceiling at every site. The ratio is the spread delta over the mean
spread at each measured check before 30 s, from the `ceiling` trajectory. The spurious columns are
the `ratio` arm's, in the windows after its own settle.

| Site                  | `ceiling` settles at | `ratio` settles at          | Ratio at the first measured check | Ratio floor before 30 s (at) | Median ratio, 7–30 s | Spurious a frame, first 30 s | Spurious a frame, 60–120 s | First 30 s against steady |
| --------------------- | -------------------- | --------------------------- | --------------------------------: | ---------------------------: | -------------------: | ---------------------------: | -------------------------: | ------------------------: |
| 1st-mission           | 30.0 s (ceiling)     | 13.9 s                      |                             0.133 |               0.073 (13.9 s) |                0.087 |                        1.163 |                      1.227 |                      0.95 |
| 3rd-folsom            | 30.1 s (ceiling)     | 18.9 s                      |                             0.105 |               0.060 (24.9 s) |                0.075 |                        1.146 |                      0.760 |          1.51 **(named)** |
| broadway-franklin     | 30.0 s (ceiling)     | 7.9 s                       |                             0.083 |               0.048 (24.9 s) |                0.063 |                        0.643 |                      0.215 |          2.99 **(named)** |
| bush-powell           | 30.1 s (ceiling)     | 30.1 s (ceiling)            |                             0.084 |               0.076 (22.9 s) |                0.086 |                        0.631 |                      0.848 |                      0.74 |
| california-leon-baker | 30.0 s (ceiling)     | 17.9 s                      |                             0.110 |               0.054 (24.9 s) |                0.076 |                        0.233 |                      0.072 |          3.26 **(named)** |
| columbus-broadway     | 30.0 s (ceiling)     | 26.9 s                      |                             0.110 |               0.073 (26.9 s) |                0.090 |                        1.003 |                      0.756 |                      1.33 |
| columbus-north-point  | 30.1 s (ceiling)     | 20.9 s                      |                             0.092 |               0.059 (29.9 s) |                0.080 |                        0.439 |                      0.418 |                      1.05 |
| embarcadero-bay       | 30.0 s (ceiling)     | 30.0 s (ceiling)            |                             0.128 |               0.081 (29.9 s) |                0.112 |                        0.645 |                      0.719 |                      0.90 |
| embarcadero-broadway  | 30.0 s (ceiling)     | 30.0 s (ceiling)            |                             0.104 |               0.076 (29.9 s) |                0.090 |                        1.607 |                      0.730 |          2.20 **(named)** |
| embarcadero-bryant    | 30.1 s (ceiling)     | 17.9 s                      |                             0.116 |               0.059 (23.9 s) |                0.072 |                        1.262 |                      1.665 |                      0.76 |
| franklin-mcallister   | 30.0 s (ceiling)     | 20.9 s                      |                             0.123 |               0.066 (21.9 s) |                0.084 |                        0.734 |                      0.797 |                      0.92 |
| fulton-divisadero     | 30.1 s (ceiling)     | 19.9 s                      |                             0.099 |               0.072 (19.9 s) |                0.085 |                        0.817 |                      0.367 |          2.23 **(named)** |
| haight-divisadero     | 30.1 s (ceiling)     | 7.9 s                       |                             0.076 |               0.061 (29.9 s) |                0.080 |                        0.216 |                      0.305 |                      0.71 |
| howard-6th            | 30.0 s (ceiling)     | 6.9 s, first measured check |                             0.064 |                0.054 (7.9 s) |                0.102 |                        0.773 |                      0.403 |          1.92 **(named)** |
| hyde-ofarrell         | 30.0 s (ceiling)     | 27.9 s                      |                             0.116 |               0.066 (29.9 s) |                0.087 |                        0.277 |                      0.215 |                      1.29 |
| laguna-eddy           | 30.0 s (ceiling)     | 17.9 s                      |                             0.109 |               0.061 (28.9 s) |                0.089 |                        0.153 |                      0.303 |                      0.51 |
| lombard-broderick     | 30.0 s (ceiling)     | 15.9 s                      |                             0.148 |               0.059 (17.9 s) |                0.091 |                        1.013 |                      1.285 |                      0.79 |
| lombard-laguna        | 30.1 s (ceiling)     | 30.1 s (ceiling)            |                             0.114 |               0.076 (12.9 s) |                0.090 |                        0.930 |                      1.138 |                      0.82 |
| marina-broderick      | 30.0 s (ceiling)     | 27.9 s                      |                             0.113 |               0.060 (28.9 s) |                0.078 |                        0.313 |                      0.222 |                      1.41 |
| marina-webster-beach  | 30.1 s (ceiling)     | 20.9 s                      |                             0.101 |               0.052 (27.9 s) |                0.061 |                        0.273 |                      0.335 |                      0.82 |
| pierce-haight         | 30.1 s (ceiling)     | 6.9 s, first measured check |                             0.072 |                0.072 (6.9 s) |                0.080 |                        0.177 |                      0.283 |                      0.62 |
| van-ness-sacramento   | 30.0 s (ceiling)     | 30.0 s (ceiling)            |                             0.111 |               0.077 (20.9 s) |                0.095 |                        0.920 |                      0.800 |                      1.15 |

Every window was complete: each replay ran 240 s, and the latest steady window ends 150 s in.
Coverage decided two settles: marina-webster-beach's ratio was at or under 0.075 from 10.9 s and
marina-broderick's from 18.9 s, but both sat just under coverage 0.8 (0.797, 0.796) until 20.9 s
and 27.9 s. Mean confidence was 42 or more at every measured check, against a criterion of 10.

### The ratio's trajectory

The ratio at every third check, and how many of the 24 measured checks before 30 s are at or under
0.075. The floor in the table above is the lowest of all 24.

| Site                  | 6.9 s | 9.9 s | 12.9 s | 15.9 s | 18.9 s | 21.9 s | 24.9 s | 27.9 s | 29.9 s | Checks at or under 0.075 |
| --------------------- | ----: | ----: | -----: | -----: | -----: | -----: | -----: | -----: | -----: | -----------------------: |
| 1st-mission           | 0.133 | 0.121 |  0.079 |  0.086 |  0.084 |  0.089 |  0.095 |  0.080 |  0.080 |                  1 of 24 |
| 3rd-folsom            | 0.105 | 0.079 |  0.078 |  0.079 |  0.074 |  0.067 |  0.060 |  0.067 |  0.063 |                 12 of 24 |
| broadway-franklin     | 0.083 | 0.072 |  0.066 |  0.064 |  0.058 |  0.051 |  0.048 |  0.067 |  0.093 |                 20 of 24 |
| bush-powell           | 0.084 | 0.076 |  0.090 |  0.093 |  0.084 |  0.078 |  0.084 |  0.088 |  0.090 |                  0 of 24 |
| california-leon-baker | 0.110 | 0.083 |  0.079 |  0.082 |  0.062 |  0.055 |  0.054 |  0.073 |  0.073 |                 12 of 24 |
| columbus-broadway     | 0.110 | 0.207 |  0.090 |  0.090 |  0.094 |  0.090 |  0.078 |  0.078 |  0.074 |                  2 of 24 |
| columbus-north-point  | 0.092 | 0.097 |  0.082 |  0.090 |  0.082 |  0.077 |  0.077 |  0.067 |  0.059 |                  8 of 24 |
| embarcadero-bay       | 0.128 | 0.147 |  0.121 |  0.131 |  0.109 |  0.083 |  0.081 |  0.083 |  0.081 |                  0 of 24 |
| embarcadero-broadway  | 0.104 | 0.091 |  0.087 |  0.083 |  0.081 |  0.102 |  0.126 |  0.086 |  0.076 |                  0 of 24 |
| embarcadero-bryant    | 0.116 | 0.097 |  0.092 |  0.086 |  0.071 |  0.065 |  0.060 |  0.064 |  0.064 |                 13 of 24 |
| franklin-mcallister   | 0.123 | 0.101 |  0.090 |  0.095 |  0.090 |  0.066 |  0.069 |  0.074 |  0.080 |                  8 of 24 |
| fulton-divisadero     | 0.099 | 0.086 |  0.114 |  0.119 |  0.080 |  0.081 |  0.079 |  0.074 |  0.094 |                  5 of 24 |
| haight-divisadero     | 0.076 | 0.077 |  0.082 |  0.090 |  0.080 |  0.076 |  0.086 |  0.068 |  0.061 |                  6 of 24 |
| howard-6th            | 0.064 | 0.074 |  0.096 |  0.104 |  0.102 |  0.100 |  0.115 |  0.116 |  0.081 |                  4 of 24 |
| hyde-ofarrell         | 0.116 | 0.118 |  0.085 |  0.090 |  0.099 |  0.093 |  0.078 |  0.073 |  0.066 |                  3 of 24 |
| laguna-eddy           | 0.109 | 0.090 |  0.090 |  0.084 |  0.065 |  0.108 |  0.089 |  0.067 |  0.065 |                  6 of 24 |
| lombard-broderick     | 0.148 | 0.118 |  0.086 |  0.062 |  0.063 |  0.074 |  0.092 |  0.155 |  0.106 |                  7 of 24 |
| lombard-laguna        | 0.114 | 0.091 |  0.076 |  0.083 |  0.085 |  0.115 |  0.096 |  0.088 |  0.101 |                  0 of 24 |
| marina-broderick      | 0.113 | 0.110 |  0.102 |  0.084 |  0.073 |  0.063 |  0.060 |  0.064 |  0.063 |                 11 of 24 |
| marina-webster-beach  | 0.101 | 0.075 |  0.063 |  0.060 |  0.060 |  0.062 |  0.057 |  0.052 |  0.053 |                 20 of 24 |
| pierce-haight         | 0.072 | 0.079 |  0.089 |  0.089 |  0.079 |  0.075 |  0.078 |  0.078 |  0.082 |                  2 of 24 |
| van-ness-sacramento   | 0.111 | 0.097 |  0.089 |  0.109 |  0.119 |  0.088 |  0.089 |  0.101 |  0.092 |                  0 of 24 |

Site medians run from 0.061 (marina-webster-beach) to 0.112 (embarcadero-bay), against 0.047 to
0.084 on the first segments: the ratio is narrower than the absolute delta, as l3bg0022 found,
but it is not a property of the scene alone. It does not fall steadily either: broadway-franklin,
lombard-broderick, howard-6th and lombard-laguna end 0.025 to 0.047 above their floor, and at
howard-6th the floor comes at 7.9 s and the ratio then rises to 0.116.

### kirk0

| Arm       | Settles at       | Ratio floor before 30 s |   HOTA |   IDF1 | ID switches | Fragmentations | False negatives | False positives |
| --------- | ---------------- | ----------------------: | -----: | -----: | ----------: | -------------: | --------------: | --------------: |
| `ceiling` | 30.1 s (ceiling) |                   0.076 | 0.3921 | 0.4363 |          91 |             29 |             855 |            2773 |
| `ratio`   | 30.1 s (ceiling) |                   0.076 | 0.3921 | 0.4363 |          91 |             29 |             855 |            2773 |

All episodes of split `kirk0-tuning` (a tuning split, not a held-out result). The two arms' tracking
baselines are byte-identical (`cff77f61…`), and the figures equal the
[kirk0 cloud run's](cloud-run-kirk0-labelled-2026-11.md#l3bg0030-on-kirk0) exactly. kirk0's replay
ends 83 s in, so its steady window (90 to 150 s) does not exist; its first 30 s after the settle
run 0.42 spurious clusters a frame.

## Against the rule

| Clause                                          | Need                           | Measured                                                             | Holds                   |
| ----------------------------------------------- | ------------------------------ | -------------------------------------------------------------------- | ----------------------- |
| 1. Before the ceiling                           | 18 or more of 22               | 17 of 22 (median 17.9 s)                                             | No                      |
| 1. At the first measured check                  | none                           | 2 (howard-6th, pierce-haight, at 6.9 s)                              | No                      |
| 2. kirk0 HOTA against `ceiling`                 | no more than 0.01 below        | +0.0000 (0.3921 both; `ratio` never converges on kirk0)              | Yes                     |
| 2. kirk1 covered of 49                          | no fewer than `ceiling`        | not read: kirk1 is not in this container                             | Unread                  |
| 3. Spurious clusters, first 30 s against steady | reported, names sites at 1.5 × | 6 of 22 named; median site 1.00 × (under `ceiling`: 6 of 22, 1.16 ×) | Reported, does not gate |

Clause 1 fails on both counts, so the ratio is not proposed. Clause 2 would hold on kirk0 alone,
but kirk0 never meets the ratio, so the arms are the same replay there and the clause tests
nothing; with kirk1 unread it cannot be read in full in any case. Nothing changes in the defaults.

## Interpretation

The relative ratio is a better-scaled measure than the absolute delta, but a threshold on it
inherits the delta's problem at a smaller scale. Within a site the ratio moves by more than the
spread between the threshold and most sites' typical level, so whether a site converges before
30 s depends on whether a quiet check falls in the window, and a site whose first measured check
happens to be quiet (howard-6th at 0.064, pierce-haight at 0.072) stops there. The held-out
site medians (0.061 to 0.112) run somewhat higher than the first segments' (0.047 to 0.084), and
the miss is one site and two early stops. That is the size of effect a threshold chosen on 23
trajectories should be expected to lose on 22 others.

The downstream read says the same from the other side. Where the ratio stopped early, the 30 s
after it ran 1.9 to 3.0 times the steady state at howard-6th and broadway-franklin (settled at
6.9 s and 7.9 s), but haight-divisadero and pierce-haight, also settled by 7.9 s, ran under their
steady state (0.71 and 0.62). Under `ceiling`, settled at 30 s everywhere, six sites also ran 1.5 ×
or more, with california-leon-baker at 4.4 ×. The early-to-steady comparison measures how traffic
in the first 30 s compares with the next 90 s as much as it measures the background.

## Limitations

- **Cloud host.** Four cores shared with two other experiment agents; no timing is read, and the
  replays' wall time (about 7 minutes a site an arm, first run and repeat) says nothing about the
  pipeline's speed.
- **kirk1 unread.** It is on a LiDAR volume, not on the dataset, so clause 2's second half is not
  measured.
- **kirk0's pack is reconstructed.** The pack's packet table and returns were rebuilt from the
  capture and the label set's text files and match the published digests; its points were decoded
  by the current parser and their digest differs from the published pack's
  ([setup](cloud-run-kirk0-labelled-2026-11.md)).
- **Held out for settling, not for the scene.** Each second segment follows its first in time, so
  the traffic resembles it; this tests whether the threshold holds on unread data from the same
  placements, not on a scene the sensor has never seen.
- **The spurious read is label-free.** It counts fragments of movers and brief tracks as well as
  background paint, and a lasting track's birth cluster once. It is a replay measurement of what
  the pipeline drew, not physical validation of the background.
- **One replay per arm per site** (repeat-verified); the windows are one draw of each site's
  traffic.

## Recommendations

1. **Do not adopt `settling_relative_0075`.** It fails clause 1 on both counts. The defaults keep
   the current first check and the 0.001 absolute threshold.
2. **Do not tune the ratio again on these segments.** A ratio of 0.08 would let more sites converge
   but these segments have now been read; any new value needs another held-out set (a third
   segment of the same placements, or the Lombard–Laguna long capture).
3. **If a convergence criterion is wanted, judge it over a window, not a check.** The ratio's
   within-site swing is the obstacle: a spec could require the ratio under a threshold at k
   consecutive checks, or its median over the last N checks, and read it on the first segments with
   these second segments' trajectories as a second tuning set and a fresh segment held out. That is
   a new spec with its own rule, not a change made here.
4. **Keep `-spurious-clusters` for later settling work.** It reads the downstream cost of any
   settle from the same replay; the `ceiling` arm's figures here are the reference to compare
   against.

## Provenance

| Item                    | Value                                                                                                                                                                                                                                              |
| ----------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Branch and commit       | `exp/l3bg0030`, tools built at `2f3e2707149c` (the settle record and `-spurious-clusters`) on `099709e` (the approved spec)                                                                                                                        |
| Tools                   | `lidar-state-estimation-baseline` SHA-256 `c6eec6b6…f080`; `lidar-ground-truth-eval` `639a5487…1dad`; both `Version=0.5.1-pre52`, `GitSHA=2f3e2707149c`, `tags=pcap`, built by `make` with the Makefile's LDFLAGS against the run's static libpcap |
| Parameter set           | `config/tuning.defaults.json` SHA-256 `9ad7377f…1675`, fingerprint `ab0c4035780d`, both arms                                                                                                                                                       |
| Replay parameter hashes | `ceiling` `sha256:8c9fb1a18e8051f80d6515ae0e649d6f58e1952de511eed62901780aff861900`; `ratio` `sha256:d989ab6a7d31bf011994e5942a8c63525bcbaba67ec292ae94871c9820996e0c`                                                                             |
| Corpus                  | `d2-second-segment`: the second five-minute segment (300 to 600 s from the first packet) of the 22 sites less ashbury-downey; embarcadero-folsom not read; broadway-franklin is the `broadway-gough` placement                                     |
| Replay settings         | `-warmup 120 -duration 120 -require-settled=false -settling-trajectory -spurious-clusters`, first run and repeat, `GOMAXPROCS=2`                                                                                                                   |
| kirk0                   | capture `2864ebde…8b9a`; pack ad8b9438 version 2 reconstructed (`sha256:f0ed3414…15dc`), split `kirk0-tuning` (`7194bb61…`), reference digest `a784e69d…`, `-warmup 6`, `obj_788622b5` ignored                                                     |
| Host                    | Claude Code cloud container, 4 cores shared with two other experiment agents, 15 GB                                                                                                                                                                |
| Raw output              | scratch only (trajectories, spurious records, manifests, kirk0 evidence); not kept                                                                                                                                                                 |

Segment SHA-256 (first 16 hex digits) and the placement each was cut from (placement SHA-256, first
16 hex digits):

| Site                  | Placement                                       | Placement SHA-256  | Segment SHA-256    |
| --------------------- | ----------------------------------------------- | ------------------ | ------------------ |
| 1st-mission           | `20260903T1410_80858-064_1st-mission`           | `cef1cf1ba7f97415` | `e2341d3b7a985a25` |
| 3rd-folsom            | `20260903T1520_80858-07c_3rd-folsom`            | `1c991d5b5c53f9cf` | `6a696d2ee4597188` |
| broadway-franklin     | `20260901T1510_80858-0c4_broadway-gough`        | `8169d57498379e2d` | `8f39f1d69f978891` |
| bush-powell           | `20260903T1135_80858-08c_bush-powell`           | `c79dc65658735269` | `22df7d525902a5c4` |
| california-leon-baker | `20260901T1433_80858-0cc_california-leon-baker` | `c21e469ffe6ccb00` | `bf45bc27a8426fdc` |
| columbus-broadway     | `20260902T1320_80858-0f4_columbus-broadway`     | `aa69875be4dd4509` | `9f60b5dcdc97e8f9` |
| columbus-north-point  | `20260902T1153_80858-0e4_columbus-north-point`  | `f6f4a4993a2a83a1` | `7dc184577c8b2867` |
| embarcadero-bay       | `20260902T1235_80858-0fc_embarcadero-bay`       | `50ccf70b60d820b1` | `506fba0594500bd0` |
| embarcadero-broadway  | `20260902T1522_80858-05c_embarcadero-broadway`  | `1dc7d4daf663fa88` | `36ced09e5e85f432` |
| embarcadero-bryant    | `20260903T1440_80858-074_embarcadero-bryant`    | `03e10c86b718f068` | `a9601871255d5423` |
| franklin-mcallister   | `20260903T1035_80858-09c_franklin-mcallister`   | `e66015254762b458` | `8296eb12979a54db` |
| fulton-divisadero     | `20260901T1359_80858-0b4_fulton-divisadero`     | `ae6c6f0660c77558` | `eaf7b74ea90d5cde` |
| haight-divisadero     | the run's shared second segment                 | –                  | `5b4d6b8eb6f958d0` |
| howard-6th            | `20260903T1550_80858-084_howard-6th`            | `ab3c24890bc5f0cd` | `522e0d05ebb063bc` |
| hyde-ofarrell         | `20260903T1105_80858-094_hyde-ofarrell`         | `beadc6f7ca2c5848` | `ad7948a43d4d8209` |
| laguna-eddy           | `20260901T1546_80858-0bc_laguna-eddy`           | `9d98d8873d40a56b` | `ac24fc4d1c7c0800` |
| lombard-broderick     | `20260902T0938_80858-0d4_lombard-broderick`     | `1eb08d9e6ced4e20` | `667e235853dfa873` |
| lombard-laguna        | `20260902T1058_80858-0dc_lombard-laguna`        | `11e9b37a471ca04c` | `2133fa1a680ab569` |
| marina-broderick      | `20260902T1003_80858-12c_marina-broderick`      | `1d7d876b99ca3ea9` | `0a289b67dca7254c` |
| marina-webster-beach  | `20260902T1028_80858-124_marina-webster-beach`  | `c7eee599c4f88a0a` | `9f0a01d61af14967` |
| pierce-haight         | `20260901T1327_80858-0a4_pierce-haight`         | `9771bf57bfa19dc7` | `5c582efec40e4d53` |
| van-ness-sacramento   | `20260902T1427_80858-0ec_van-ness-sacramento`   | `771d456b6f1eb9fc` | `cda2b7307f49adc7` |
