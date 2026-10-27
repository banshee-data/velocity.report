# The subsampled frames: DBSCAN's threshold rescaled to the kept fraction

<!-- ignore-style-length -->

Above 8,000 foreground points a frame, DBSCAN clusters a uniform subsample while `MinPts` stays at
5, so the busiest frames cluster at an effective threshold of 5/f, where f is the kept fraction
(gap analysis D6). The rescaled threshold (`density_cap`) was built and never measured. This report
counts how many corpus frames exceed the cap and asks whether, on the sites that do, rescaling
recovers what removing the cap recovers without the frame-time tail removing it costs. It answers
[l4clus0004](../../../data/experiments/l4clus/l4clus0004.md).

- **Status:** Measured, 2026-10-27. One unattended run on arrow-worker (exp-20261026). No
  candidate: the defaults are unchanged and `density_cap` stays off
- **Layers:** L4 (clustering), read through L5's confirmed tracks
- **Related:** [l4clus0002, the input cap on kirk0](l4-dbscan-input-cap-2026-10.md#recommendations),
  [maths decision review E-06](../../platform/operations/decision-review-maths-2026-10.md#e-06-q-06),
  [gap analysis D6](../../../data/maths/paper-implementation-gap-analysis.md#gaps)

## Summary

- **Ten of 23 sites are capped.** Under the defaults (`a0`) 10 sites have 1 % or more of their
  scored frames over the 8,000-point cap, up to 7.85 % (Embarcadero at Broadway). The kept
  fraction among those frames has a median of 0.49 to 0.92 by site and falls to 0.26 (Van Ness at
  Sacramento, whose busiest frame holds 30,292 foreground points).
- **The cap is not immaterial (clause 1).** Removing it (`nocap`) moves the confirmed vehicle
  tracks beyond the run-to-run band at 5 of the 10 capped sites, but in both directions: +3, +4 and
  +2 at 1st at Mission and the two Embarcadero sites, −4 at Franklin at McAllister and −15 at Van
  Ness at Sacramento. Its median change over the capped sites is 0.
- **The rescaled threshold is not a candidate (clause 2).** Its median change is +1 vehicle track,
  which is at least half of `nocap`'s 0 as the rule is written, but it raises spurious clusters a
  frame beyond 1.1 × `a0`'s at Van Ness at Sacramento (4.22 against 3.72, 1.13 ×), and its frame
  p99 beyond 1.1 × at Embarcadero at Bryant and Franklin at McAllister. Either bound alone fails
  it; the spurious-cluster bound does not depend on the host's timing, and the p99 overshoot at
  Embarcadero at Bryant did not replicate on a rerun with the host idle.
- **Nothing for a voxel stage to recover on this measure (clause 3 not met).** `nocap` does not
  gain vehicle tracks, so the corpus does not show that the dropped points are lost tracks. It
  costs: the frame p99 rises 2.3 to 2.7 × at seven of the capped sites (195 to 481 ms at 1st at
  Mission, 217 to 512 ms at Columbus at Broadway on this host).
- **So D6 stays open at the shipped cap, and nothing ships.** The cap changes which tracks exist on
  the busiest sites without a net gain in count; whether the tracks it changes are better or worse
  is a labelled question this corpus cannot answer.

## Method

**Arms.** `a0` the defaults; `density` the defaults with the replay experiment `density_cap`
(`l4perception.DBSCANParams.ScaleMinPtsWhenSubsampled`: `MinPts` rescaled to round(5·f), floored
at 2, in a frame over the cap); `nocap` [`l4-cap100k.json`](../../../config/experiments/l4-cap100k.json),
the defaults with `l4.dbscan_xy_v1.foreground_max_input_points` at 100,000, above any corpus
frame's foreground.

**Replays.** `lidar-state-estimation-baseline -campaign-metrics` on the first five-minute segment
of the 23 tuning and screen sites (embarcadero-folsom held out and unread), 200 s scored after a
70 s warm-up, 256 sample points, an evidence database for the screen. A site's three arms ran back
to back, one replay at a time; each case replays twice, and every repeat was byte-equal to its
first run.

**The counts.** The campaign metrics' new `dbscan_input.json` (this PR): each scored frame's DBSCAN
input, which the pipeline hands the tracker's `RecordFrameStats` before any subsample, against the
cap, with the share of frames over it and the median and smallest kept fraction among them. The
frame times are the repeat run's `frame_timing.json`, which writes no evidence.

**The screen.** l3bg0023's (`screen_consol.py`, with `score_free.py` beside it) on each evidence
database, and the readings fixed in the spec before the replays: confirmed vehicle tracks are the
screen's vehicle-sized tracks (lasting 2 s or more, the p90 of their clusters' longer side 3.0 m
or more); spurious clusters a frame are the clusters no lasting track ever takes as its
measurement, over the frames with observations; the band is one vehicle track or 1 % of `a0`'s
count at the site, whichever is larger, since deterministic replays have no run-to-run spread.

## Results

Per site, with `a0`'s share of scored frames over the cap. The changes are against `a0`; the frame
p99 is on arrow-worker, not the Pi. The ten capped sites are those at 1.00 % or more.

| Site                  | Over the cap | Kept f, median | Kept f, least | Largest input | Vehicle tracks `a0` | `density` | `nocap` | Band | Spurious a frame `a0` | `density` | `nocap` | Frame p99 `a0` | `density` | `nocap` |
| --------------------- | -----------: | -------------: | ------------: | ------------: | ------------------: | --------: | ------: | ---: | --------------------: | --------: | ------: | -------------: | --------: | ------: |
| embarcadero-broadway  |       7.85 % |           0.78 |          0.30 |        26,463 |                 181 |        +1 |      +4 |  1.8 |                  3.54 |      3.66 |    3.60 |         132 ms |    139 ms |  301 ms |
| columbus-broadway     |       5.75 % |           0.54 |          0.27 |        29,248 |                 241 |        +9 |      +0 |  2.4 |                  4.73 |      5.11 |    4.85 |         217 ms |    183 ms |  512 ms |
| van-ness-sacramento   |       5.55 % |           0.68 |          0.26 |        30,292 |                 241 |        +0 |     −15 |  2.4 |                  3.72 |      4.22 |    3.90 |         114 ms |    118 ms |  285 ms |
| 1st-mission           |       4.60 % |           0.73 |          0.40 |        19,988 |                 198 |        −3 |      +3 |  2.0 |                  5.13 |      5.25 |    6.05 |         195 ms |    198 ms |  481 ms |
| lombard-laguna        |       4.45 % |           0.59 |          0.31 |        25,861 |                 183 |        +4 |      +0 |  1.8 |                  2.71 |      2.84 |    2.81 |         167 ms |    168 ms |  445 ms |
| columbus-north-point  |       2.25 % |           0.55 |          0.38 |        21,081 |                  75 |        +1 |      +0 |  1.0 |                  1.63 |      1.66 |    1.64 |         115 ms |    120 ms |  291 ms |
| 3rd-folsom            |       2.10 % |           0.49 |          0.39 |        20,398 |                 111 |        +0 |      −1 |  1.1 |                  2.44 |      2.63 |    2.54 |         122 ms |    120 ms |  282 ms |
| laguna-eddy           |       1.85 % |           0.92 |          0.86 |         9,250 |                  71 |        +4 |      +0 |  1.0 |                  1.17 |      1.18 |    1.19 |         219 ms |    217 ms |  196 ms |
| franklin-mcallister   |       1.75 % |           0.92 |          0.36 |        22,080 |                 207 |        −4 |      −4 |  2.1 |                  2.40 |      2.50 |    2.43 |          98 ms |    107 ms |  103 ms |
| embarcadero-bryant    |       1.55 % |           0.67 |          0.40 |        20,196 |                 120 |        +1 |      +2 |  1.2 |                  3.10 |      3.12 |    3.09 |         130 ms |    145 ms |  172 ms |
| california-leon-baker |       0.95 % |           0.73 |          0.62 |        12,839 |                  37 |        +2 |      +0 |  1.0 |                  0.59 |      0.59 |    0.61 |          82 ms |     87 ms |   96 ms |
| lombard-broderick     |       0.70 % |           0.90 |          0.85 |         9,407 |                 272 |        −1 |      +4 |  2.7 |                  3.92 |      4.54 |    4.29 |         120 ms |    118 ms |  120 ms |
| bush-powell           |       0.60 % |           0.85 |          0.68 |        11,708 |                 117 |        +1 |      −5 |  1.2 |                  1.89 |      1.84 |    1.83 |          90 ms |     86 ms |   88 ms |
| pierce-haight         |       0.55 % |           0.87 |          0.80 |         9,991 |                  66 |        +0 |      −6 |  1.0 |                  2.19 |      2.23 |    2.34 |          99 ms |     94 ms |   96 ms |
| haight-divisadero     |       0.45 % |           0.89 |          0.84 |         9,519 |                  50 |        +4 |      +1 |  1.0 |                  0.84 |      0.86 |    0.84 |         102 ms |    101 ms |  100 ms |
| marina-webster-beach  |       0.40 % |           0.67 |          0.55 |        14,511 |                  39 |        +1 |      +1 |  1.0 |                  0.85 |      0.85 |    0.85 |          66 ms |     64 ms |   60 ms |
| ashbury-downey        |       0.00 % |              – |             – |         7,902 |                  36 |        +0 |      +0 |  1.0 |                  0.60 |      0.60 |    0.60 |          78 ms |     79 ms |   77 ms |
| broadway-gough        |       0.00 % |              – |             – |         5,714 |                 114 |        +1 |      +2 |  1.1 |                  1.49 |      1.50 |    1.51 |          69 ms |     69 ms |   73 ms |
| embarcadero-bay       |       0.00 % |              – |             – |         6,449 |                  95 |        +0 |      +0 |  1.0 |                  1.33 |      1.33 |    1.33 |          76 ms |     79 ms |   76 ms |
| fulton-divisadero     |       0.00 % |              – |             – |         3,957 |                  88 |        +0 |      +0 |  1.0 |                  1.53 |      1.53 |    1.53 |          66 ms |     66 ms |   67 ms |
| howard-6th            |       0.00 % |              – |             – |         3,281 |                  77 |        +0 |      +0 |  1.0 |                  1.66 |      1.66 |    1.66 |          59 ms |     58 ms |   58 ms |
| hyde-ofarrell         |       0.00 % |              – |             – |         4,446 |                 113 |        +1 |      +2 |  1.1 |                  1.76 |      1.76 |    1.74 |          63 ms |     62 ms |   62 ms |
| marina-broderick      |       0.00 % |              – |             – |         5,141 |                  96 |        +0 |      +0 |  1.0 |                  1.11 |      1.11 |    1.11 |          62 ms |     56 ms |   56 ms |

| Capped sites (10)                         | `density`              | `nocap`                  |
| ----------------------------------------- | ---------------------- | ------------------------ |
| Median change in vehicle tracks           | +1                     | 0                        |
| Sites beyond the band                     | 5 (+9, +4, +4, −3, −4) | 5 (+3, +4, +2, −4, −15)  |
| Spurious clusters a frame over 1.1 × `a0` | 1 (Van Ness, 1.13)     | 1 (1st at Mission, 1.18) |
| Frame p99 over 1.1 × `a0`                 | 2 (1.12, 1.10)         | 8 (1.32 to 2.66)         |

**The sites under the cap differ too.** At the seven sites with no scored frame over the cap the
arms still differ by up to two vehicle tracks (Broadway at Gough and Hyde at O'Farrell). No scored
frame there is subsampled under any arm, so the difference enters from the warm-up, whose frames
the counter does not read. The likely source is the frames clustered before the background
settles, when most of a frame is foreground; the tracks they leave change the background that
follows through the track guard's holds. This is inferred, not counted. The rule's sites are those
capped in the scored window.

**The host's timing.** arrow-worker has two virtual cores and runs other services. At the seven
uncapped sites, where the arms subsample no scored frame, the frame p99 still differs between arms
by up to 11 % (Marina at Broderick, 62 against 56 ms): the 1.1 × bound is within this host's
noise. The run's own commands overlapped timed runs at Columbus at Broadway (`a0`, `density`) and
Embarcadero at Bryant (`density`, `nocap`), so all three arms at those two sites were replayed
again with the host otherwise idle, as the run's notes fixed before the last sites were read:

| Frame p99, lane against rerun | `a0`         | `density`    | `nocap`      |
| ----------------------------- | ------------ | ------------ | ------------ |
| Columbus at Broadway          | 217 / 178 ms | 183 / 177 ms | 512 / 495 ms |
| Embarcadero at Bryant         | 130 / 144 ms | 145 / 135 ms | 172 / 172 ms |

On the reruns `density`'s p99 is 0.99 × `a0`'s at Columbus at Broadway and 0.94 × at Embarcadero
at Bryant, so the p99 failure there does not replicate, while `nocap`'s cost does. The rule is
applied to the lane's own runs, and `density` fails it on spurious clusters at Van Ness at
Sacramento in any case.

## Reading, against the rule

1. **Immaterial at the shipped cap:** not met. Ten sites have 1 % or more of their frames over the
   cap, and at five of them `nocap` moves vehicle tracks beyond the band.
2. **`density` a candidate:** not met. Its median change (+1) is at least half of `nocap`'s (0),
   but its spurious clusters exceed 1.1 × `a0`'s at Van Ness at Sacramento and its frame p99 at
   Embarcadero at Bryant and Franklin at McAllister. kirk0's per-frame HOTA, the clause's last
   condition, cannot change that and was not read.
3. **`nocap` gains and `density` does not:** not met; `nocap`'s median change is 0.

None of the three clauses closes the question: the cap matters at the busiest sites, and neither
lever gains on the corpus's label-free count.

## What it does not establish

- **Whether the changed tracks are better.** A vehicle-track count moves both ways under `nocap`;
  only labels say whether a gained track is a real vehicle or a split, or a lost one a merge
  undone. kirk0 and kirk1 never reach the cap often enough to say (l4clus0002).
- **Recall of distant objects by range band**, which the corpus has no labels for.
- **Any Pi frame time.** The frame times are arrow-worker's.
- **Whether a different cap would serve better.**

## Provenance

| Item        | Value                                                                                                                                                                                                                                 |
| ----------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Code        | `648a61c74` on `claude/l4clus0004-capped-frames` (0.5.1-pre47), built with the Makefile's LDFLAGS; executable `sha256:473532af…`; the counter and the spec's readings committed at `648a61c74`, before any replay                     |
| Tuning      | `config/tuning.defaults.json` SHA-256 `aefcc10c…` (`a0`, `density` with `density_cap`); `config/experiments/l4-cap100k.json` `990b50c4…` (`nocap`)                                                                                    |
| Corpus      | the first-segment corpus and index of the October campaigns (`32e33814…`, `ccf180d3…`), 23 sites, 70 s warm-up, 200 s scored; captures read over NFS through a resolved root (ashbury-downey's capture is an SMB symlink stub)        |
| Screen      | `screen_l4clus0004.py` `6df9c7d2…` over `screen_consol_arrow.py` `1758f7a7…` (l3bg0023's `screen_consol.py` with only its `score_free.py` path changed) and `score_free.py` `706458a0…`                                               |
| Host        | arrow-worker (2 vCPU, 5.8 GB) beside its other services; 69 units in 7 h 50 min, one at a time, and the six timing reruns in 29 min                                                                                                   |
| Raw outputs | screens, case summaries and campaign metrics under the run's scratch directory; the recordings archived to `velocity-campaign/exp-20261026/l4clus0004` on the results share; the evidence databases deleted once screened; not in Git |

## Recommendations

1. **Keep the cap and leave `density_cap` off.** It fails the rule on a bound that does not depend
   on timing, and removing the cap costs two to three times the frame tail where it bites.
2. **Read the capped frames with labels, not counts.** The held-out window label0001 freezes at
   embarcadero-folsom is on the corridor whose sites are the most capped here (7.85 % at
   Embarcadero at Broadway); scoring `a0` against `nocap` there, by range band, is what would say
   whether the dropped points cost recall. Until then D6 is open but not urgent.
3. **Do not build a voxel stage for D6 yet.** Clause 3 is the spec's trigger for one, and it is
   not met.
