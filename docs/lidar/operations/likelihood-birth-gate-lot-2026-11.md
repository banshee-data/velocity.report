# The likelihood cost with the fragment birth gate, on two placements drawn by lot

<!-- ignore-style-length -->

- **Status:** Measured on kirk0 and on two placements on 2026-11-10. `likelihood-birth` passes L1
  and fails L2 and L3. Nothing is named for label0003's spans, nothing is adopted and no default
  changes
- **Spec:** [l5id0024](../../../data/experiments/l5id/l5id0024.md). Its rule (L1 to L3) was
  committed at `39705e4`, before any replay
- **Tooling:** `lidar_figure_sensitivity.py` gains `--moving peak` and `--joins` (commits
  `70f7012` and `33fc658`). The default output is unchanged
- **Parameter set:** the [defaults](../../../config/tuning.defaults.json), fingerprint
  `ab0c4035780d`. The candidate differs only by replay experiments
- **Related:** l5id0023's [placements report](likelihood-birth-gate-placements-2026-11.md),
  l5id0021's [birth gate report](robust-birth-gate-kirk0-2026-11.md) and l5id0019's
  [November benchmark](tracking-geometry-fixes-benchmark-2026-11.md)

## Summary

- **L1 holds on kirk0, as in l5id0023.** The paired bootstrap against `a0` gives a HOTA 5th
  percentile of +0.0032 and an IDF1 5th percentile of +0.0055, switches 90 to 6. Both arms
  reproduce l5id0023's tracking baselines byte for byte (`79d9abff…` and `e6479076…`).
- **L2 fails.** The candidate costs noise on all four segments drawn by lot, where at least three
  must pass. Brief tracks rise by 4 and 5 at columbus-north-point and by 25 and 37 at
  lombard-broderick, against a bound of 3. Clutter rises by 6.4, 13.3 and 21.4 per hundred of
  `a0`'s moving tracks at three segments, against a bound of 5. At lombard-broderick's second
  segment the candidate also loses one lasting moving track by footprint: a 2.5 s vehicle at
  9 m/s that it confirms only for its last second.
- **L3 fails at lombard-broderick's second segment.** Under the peak test, after the joins are
  credited, the moving count moves by +2.7 %, +3.8 %, +4.6 % and **+8.1 %**. p85 moves by
  0.40 m/s at most, inside its bound. The credited count rises at lombard because the candidate
  carries 15 of `a0`'s moving vehicles in two moving tracks where `a0` used one: 7 split into
  sequential pieces and 8 doubled for 1 to 6 s.
- **The joins are mostly duplicates removed.** Across the four segments the candidate carries 94
  of `a0`'s moving tracks in 42 joins. 35 of the joins are concurrent (a duplicate in `a0`
  removed) and 7 sequential (pieces joined). On box-eval's scored window, duplicate pairs fall
  from 115 to 65 and pair-frames from 3,118 to 1,233.
- **The new segments are busier than l5id0023's.** `a0` carries 73 to 196 moving tracks a
  segment here, against 18 to 33 at ashbury-downey and haight-divisadero. The brief-track bound
  is absolute, so at lombard, with 76 brief tracks under `a0`, it is tight, but the rises (25 and 37) are a third to a half of `a0`'s brief tracks.

## Question and rule

l5id0023 ranked `likelihood-birth` first of its three forms and traced its one count failure, at
haight-divisadero's second segment, to the moving test and not to lost vehicles. This experiment
reads the same arm on two placements it had not seen, drawn by lot, with a moving test fixed
before they were read that a join cannot move.

The rule, in short ([spec](../../../data/experiments/l5id/l5id0024.md)):

- **L1, the anchor.** On kirk0 the paired bootstrap's 5th percentiles of the HOTA and IDF1 deltas
  against `a0` are above 0.
- **L2, noise.** On each segment under l5id0019's R4: brief tracks rise by 3 at most, clutter by 5
  per hundred of `a0`'s moving tracks at most (moving by the peak test), and duplicate pairs per
  hundred vehicle tracks do not rise. Pass on three of the four segments at least, and lose no
  lasting moving track by footprint on any.
- **L3, counts, with the peak test.** A track is moving when it lasts 2.0 s or more and its
  maximum speed (1.0 s rolling median) is 1.5 m/s or more. On each segment the moving count moves
  by 5 % at most after crediting every `a0` moving track the candidate carries in one track
  together with another (a join), and p85 moves by 0.5 m/s at most.
- **Outcome.** A pass on L1 to L3 names `likelihood-birth` for label0003's spans. A fail names its
  clause. Nothing is adopted.

## Arms

| Arm                | Replay experiments                     | Parameter hash |
| ------------------ | -------------------------------------- | -------------- |
| `a0`               | none                                   | `ab0c4035780d` |
| `likelihood-birth` | `likelihood_cost`, `robust_birth_gate` | `63e42b981b90` |

Both arms run from `config/tuning.defaults.json` (fingerprint `ab0c4035780d`). The parameter hash
folds the replay experiments into the tuning hash. Both hashes equal l5id0023's.

## Method

- **The lot.** `random.Random(20261110).sample(pool, 2)` over the 21 corpus sites other than
  ashbury-downey, haight-divisadero and embarcadero-folsom, in the order of
  `tools/s2-archive/site-index.json`, gives columbus-north-point and lombard-broderick, as the
  spec names. The spec does not name the list's order. The phase 1 corpus file
  (`state-estimation-phase01-corpus.json`) orders the same 21 sites differently and gives
  lombard-laguna and broadway-franklin; a sorted list gives embarcadero-bryant and
  columbus-broadway. The site index is the order that reproduces the spec's names, so it is the
  order recorded here. embarcadero-folsom was not read.
- **The segments.** Each placement's capture came from the Hugging Face dataset
  (`raw/lidar/20260902T1153_80858-0e4_columbus-north-point.pcapng` and
  `raw/lidar/20260902T0938_80858-0d4_lombard-broderick.pcapng`), one at a time, and was cut into
  its first two five-minute segments (0 to 300 s and 300 to 600 s from the first packet) by the
  run's `pcapng_window.py`, l5id0019's method. Each capture was deleted once cut.
- **The join table.** `lidar_figure_sensitivity.py --moving peak --joins` (commit `70f7012`,
  before any replay) pairs each of `a0`'s moving tracks with the candidate track of best
  single-track footprint coverage (`lidar_flip_cost.py --match footprint`), when that share is 0.5
  or more. A join is a candidate track paired so with two or more. Run on the first segment, `a0`
  against itself gave three joins where there must be none: a duplicate wholly inside a longer
  track's footprint is covered fully by both, and the tie went to the longer track. Commit
  `33fc658` sends a tie to the candidate track whose own rows the `a0` track covers most, and
  credits a member only when no other moving candidate track also carries it 0.5 or more, since
  such a member still counts in the candidate. That fix was made after the first segment's
  figures had been seen and before any other segment's scripts were run. Both credits are
  reported; L3's verdict is the same under either (below).
- **kirk0 replays.** `lidar-state-estimation-baseline` on kirk0 whole as a one-case corpus, with
  `-warmup 6 -require-settled=false -sample-points 1024 -campaign-metrics`, evidence per case,
  the committed cohort manifest and a scratch use ledger at migration 71. Each arm's repeat pass
  reproduced its first pass's tracking baseline byte for byte.
- **kirk0 boxes.** `lidar-box-eval -variants none -rows -require-settled=false -warmup 6
-duration 0`, for duplicate pairs over the scored frames.
- **Identity.** `lidar-ground-truth-eval perframe` on the reconstructed pack, `kirk0-tuning` split
  with `-allow-tuning-split`, `obj_788622b5` ignored, online estimates as declared baselines, the
  candidate against `a0` with `-bootstrap 2000 -bootstrap-seed 20261108`.
- **The placements.** Each arm ran through `lidar-box-eval -warmup 40 -duration 120 -variants none
-rows -require-settled=false -sample-points 1024` with an evidence database. On each arm's
  database: `lidar_noise_screen.py` and `lidar_clutter_origin.py` on their own;
  `lidar_flip_cost.py --match footprint` and `lidar_figure_sensitivity.py` (the old median test,
  plain and with `--joins`, and `--moving peak --joins`) against `a0`'s. Duplicates come from
  `lidar-box-eval`'s summary. Lost, shortened and added under the peak test come from
  `lidar_flip_cost.py`'s own coverage applied to the peak test's moving tracks.
- **Which window decides.** As in l5id0019 and l5id0023, the tables read the evidence, which spans
  about 154 s of each segment, warm-up included. The same scripts on the rows, which span only the
  scored 120 s, are reported, not decided on.
- **The tracks behind the count.** A scratch script on the two modules paired each moving
  candidate track that carries no `a0` moving track of its own with the `a0` track it covers most,
  and with that track's carrier, to say whether it is a piece or a double.
- **Order.** The two kirk0 replays ran first, then the eight segment replays in two lanes,
  `GOMAXPROCS=2`, `nice -n 5`, on a host shared with other experiments (load average 12 to 14 on
  4 cores). L1 was read before any segment's figures were read.

**The anchor.** `a0` equals the l5id README's anchor in every figure checked: tracking baseline
`79d9abff…`, HOTA 0.4284, IDF1 0.4728, 90 switches, 46 fragmentations, 265 false negatives, 79
tracks; the bootstrap's labelled-object HOTA 0.4798 [0.4111, 0.5415]; 44 vehicle tracks, 10
duplicate pairs, 452 pair-frames.

## L1: identity on kirk0

Full-comparison figures, then the paired bootstrap (19 scored objects, 2,000 draws, seed
20261108).

| Arm                |   HOTA |   IDF1 | Switches | Frag |  FN | Tracks | Δ HOTA, 5th / 50th / 95th   | Δ IDF1, 5th / 50th / 95th   | Δ switches      | Δ frag         | L1   |
| ------------------ | -----: | -----: | -------: | ---: | --: | -----: | --------------------------- | --------------------------- | --------------- | -------------- | ---- |
| `a0`               | 0.4284 | 0.4728 |       90 |   46 | 265 |     79 |                             |                             |                 |                |      |
| `likelihood-birth` | 0.4369 | 0.4839 |        6 |   30 | 247 |     78 | +0.0032 / +0.0126 / +0.0228 | +0.0055 / +0.0183 / +0.0337 | −226 / −80 / −4 | −42 / −16 / −2 | Pass |

The candidate's bootstrap labelled-object HOTA is 0.4927. On kirk0's box-eval it has 43 vehicle
tracks, 8 duplicate pairs and 246 pair-frames (`a0` 44, 10, 452), and it absorbs 43 births
(`fragment_births_absorbed`, a count of frames). Every figure equals l5id0023's.

## L2 and L3: the four segments

Each segment's `a0` row, then the candidate. Brief, near-static, clutter, spurious, unowned,
moving and speeds read the evidence; duplicates read `lidar-box-eval`'s scored window. "Moving"
gives the count under the peak test, then lost, shortened and added against `a0` by footprint
under the same test. Speeds are the peak test's p85 and p98.

| Case       | Arm                | Tracks | Brief | Near-static | Clutter (art / stat / frag / uncl) | Spurious a frame | Unowned | Moving, peak (lost / short / added) | p85, p98 (m/s) | Vehicle tracks | Dup pairs (per 100) | Pair-frames |
| ---------- | ------------------ | -----: | ----: | ----------: | ---------------------------------- | ---------------: | ------: | ----------------------------------- | -------------- | -------------: | ------------------- | ----------: |
| columbus 1 | `a0`               |    191 |    32 |          11 | 43 (14 / 5 / 7 / 17)               |            0.886 |   1,366 | 73                                  | 8.75, 11.08    |             66 | 28 (42.4)           |         791 |
|            | `likelihood-birth` |    192 |    36 |          10 | 46 (15 / 5 / 9 / 17)               |            0.906 |   1,396 | 66 (0 / 1 / 0)                      | 8.75, 11.08    |             62 | 14 (22.6)           |         333 |
| columbus 2 | `a0`               |    179 |    36 |          10 | 46 (20 / 4 / 2 / 20)               |            0.927 |   1,425 | 78                                  | 8.61, 11.59    |             81 | 24 (29.6)           |         488 |
|            | `likelihood-birth` |    178 |    41 |          10 | 51 (22 / 3 / 6 / 20)               |            0.904 |   1,388 | 72 (0 / 2 / 2)                      | 9.01, 11.59    |             74 | 12 (16.2)           |         227 |
| lombard 1  | `a0`               |    356 |    76 |           5 | 81 (16 / 3 / 4 / 58)               |            1.866 |   2,880 | 196                                 | 11.52, 13.28   |            241 | 44 (18.3)           |       1,101 |
|            | `likelihood-birth` |    379 |   101 |           6 | 107 (20 / 3 / 7 / 77)              |            1.938 |   2,993 | 188 (0 / 8 / 3)                     | 11.67, 13.94   |            256 | 24 (9.4)            |         407 |
| lombard 2  | `a0`               |    317 |    77 |           2 | 79 (14 / 0 / 7 / 58)               |            1.750 |   2,693 | 173                                 | 12.53, 14.56   |            167 | 19 (11.4)           |         738 |
|            | `likelihood-birth` |    342 |   114 |           2 | 116 (18 / 0 / 10 / 88)             |            1.724 |   2,657 | 175 (**1** / 0 / 1)                 | 12.54, 14.60   |            177 | 15 (8.5)            |         266 |

Under the old median test the candidate loses no moving track on the first three segments and one
at lombard 2, the same vehicle; it shortens 1, 3, 10 and 2.

**L2 on each segment** (candidate less `a0`): brief Δ, clutter Δ per hundred of `a0`'s moving
tracks (peak test), duplicate pairs per hundred vehicle tracks `a0` → candidate, lasting movers
lost. Bold marks what costs.

| Segment    | Brief Δ | Clutter Δ per 100 | Dup pairs per 100 | Movers lost | Costs noise |
| ---------- | ------: | ----------------: | ----------------- | ----------: | ----------- |
| columbus 1 |  **+4** |              +4.1 | 42.4 → 22.6       |           0 | Yes         |
| columbus 2 |  **+5** |          **+6.4** | 29.6 → 16.2       |           0 | Yes         |
| lombard 1  | **+25** |         **+13.3** | 18.3 → 9.4        |           0 | Yes         |
| lombard 2  | **+37** |         **+21.4** | 11.4 → 8.5        |       **1** | Yes         |

The candidate costs noise on four of four segments, and loses a lasting moving track at lombard 2,
so **L2 fails** on both its conditions. The duplicate condition holds everywhere.

**Where the brief tracks come from.** `lidar_flip_cost.py`'s brief-origin classes for the
candidate (`a0` in brackets): re-births 1, 0, 0, 1 (0, 0, 0, 0); splits on a lasting track's
body 7, 7, 2, 6 (6, 4, 0, 4); other 28, 34, 99, 107 (26, 32, 76, 73). At lombard the rise is
almost all "other": brief tracks away from any lasting track's footprint, which the birth gate
does not reach. The clutter classes say the same: the rise at lombard is unclassed clutter, 58 to
77 and 58 to 88.

**The mover lost at lombard 2.** `a0` carries a vehicle from 102.7 to 104.9 s, 21.4 m at a median
9.2 m/s, some 56 to 77 m from the sensor. The candidate's only track on it starts at 104.2 s and
lasts 1.0 s, so it covers 35 % of `a0`'s rows. The candidate confirmed the vehicle 1.5 s later
than `a0`.

### L3: the counts, peak test beside the old median test

Count changes against `a0`, raw and after the join credit. "Every member" is the credit without
`33fc658`'s exception for a member another moving candidate track also carries.

| Segment    | Peak: `a0` → cand | Raw    | Joins (members) | Credit | Credited   | Every member | p85 Δ | Median: `a0` → cand | Raw   | Credit | Credited | p85 Δ |
| ---------- | ----------------- | ------ | --------------- | -----: | ---------- | ------------ | ----: | ------------------- | ----- | -----: | -------- | ----: |
| columbus 1 | 73 → 66           | −9.6 % | 7 (20)          |      9 | +2.7 %     | +8.2 %       | +0.00 | 83 → 77             | −7.2% |     11 | +6.0 %   | +0.03 |
| columbus 2 | 78 → 72           | −7.7 % | 9 (19)          |      9 | +3.8 %     | +5.1 %       | +0.40 | 63 → 60             | −4.8% |      4 | +1.6 %   | +0.02 |
| lombard 1  | 196 → 188         | −4.1 % | 15 (32)         |     17 | +4.6 %     | +4.6 %       | +0.15 | 182 → 171           | −6.0% |     16 | +2.7 %   | +0.09 |
| lombard 2  | 173 → 175         | +1.2 % | 11 (23)         |     12 | **+8.1 %** | **+8.1 %**   | +0.01 | 158 → 156           | −1.3% |      9 | +4.4 %   | +0.06 |

Under the peak test with the credit, the count moves by more than 5 % at lombard 2, so **L3
fails**. It fails there under either credit, and under the every-member credit it also fails at
columbus 1 and 2. p85 holds everywhere (0.40 m/s at most). Read raw, without the credit, the peak
count moves by more than 5 % at both columbus segments. Under the old median test with the
credit, the count moves by more than 5 % at columbus 1 alone (+6.0 %).

**Why the credited count rises.** The credit restores a count the candidate gives up by carrying
two of `a0`'s moving tracks in one. At lombard the candidate also does the reverse: some of
`a0`'s moving vehicles are carried in two moving candidate tracks where `a0` used one. These
are the moving candidate tracks that are not the carrier of any `a0` moving track:

| Segment    | Moving candidate tracks with no `a0` track of their own | Of which a piece (sequential) | A double (concurrent, 1 to 6 s) | Over an `a0` track not moving |
| ---------- | ------------------------------------------------------: | ----------------------------: | ------------------------------: | ----------------------------: |
| columbus 1 |                                                      12 |                             5 |                               6 |                             1 |
| columbus 2 |                                                       8 |                             5 |                               3 |                             0 |
| lombard 1  |                                                      28 |                             6 |                              17 |                             0 |
| lombard 2  |                                                      17 |                             7 |                               8 |                             2 |

Five of lombard 1's 28 had no carrier to compare against and are counted in the first column
only. At lombard 2, for example, `a0` carries one vehicle from 70.3 to 81.6 s (115.6 m at a median
10.4 m/s); the candidate carries it from 70.3 to 72.1 s in one track and from 74.6 to 81.6 s in
another. The 8 doubles there are short second tracks born on a vehicle the candidate already
carries. Inferred: box-eval's duplicate test, which reads the scored window's vehicle class and
overlap, does not count most of them, which is how its duplicate pairs still fall.

### The joins

Every join under the peak test, from the evidence. Times are seconds from the first evidence row
of either arm. A starred member is also carried 0.5 or more by another moving candidate track, so
it earns no credit.

| Case       | Candidate track (s)       | Moving? | `a0` moving tracks carried (s, coverage)                                                                       | Kind       | Credit |
| ---------- | ------------------------- | ------- | -------------------------------------------------------------------------------------------------------------- | ---------- | -----: |
| columbus 1 | `16ba4049` 12.5 to 28.9   | yes     | 12.5 to 28.9 (1.00); 23.6 to 28.1 (1.00)                                                                       | concurrent |      1 |
| columbus 1 | `d8630f38` 49.4 to 127.4  | yes     | 43.9 to 87.1 (0.83); 49.4 to 54.1 (1.00); 54.3 to 60.5 (1.00)\*; 84.7 to 127.4 (1.00); 111.1 to 124.0 (0.60)\* | concurrent |      2 |
| columbus 1 | `f6c8d2ae` 77.9 to 110.2  | yes     | 77.9 to 110.2 (1.00); 94.9 to 107.9 (0.83)                                                                     | concurrent |      1 |
| columbus 1 | `2dd90d52` 106.0 to 126.5 | yes     | 104.6 to 108.1 (0.55); 106.0 to 126.5 (1.00)                                                                   | concurrent |      1 |
| columbus 1 | `abd23951` 108.9 to 135.4 | yes     | 108.9 to 135.4 (1.00); 109.6 to 116.0 (0.92)\*; 114.3 to 116.0 (1.00)                                          | concurrent |      1 |
| columbus 1 | `ac96b2ac` 124.7 to 133.2 | yes     | 125.9 to 129.4 (0.95); 126.7 to 133.2 (0.94)                                                                   | concurrent |      1 |
| columbus 1 | `388a81f1` 132.0 to 153.7 | yes     | 132.0 to 144.9 (1.00); 140.6 to 143.8 (1.00)\*; 143.6 to 153.7 (1.00); 143.9 to 145.8 (1.00)                   | concurrent |      2 |
| columbus 2 | `044e49d6` 23.5 to 38.2   | yes     | 23.5 to 26.1 (1.00); 25.4 to 38.2 (1.00)                                                                       | concurrent |      1 |
| columbus 2 | `c4ec12dc` 27.9 to 45.7   | yes     | 26.5 to 41.7 (0.91); 41.8 to 45.7 (1.00)                                                                       | sequential |      1 |
| columbus 2 | `eaa29736` 31.5 to 77.3   | yes     | 31.5 to 65.0 (1.00); 65.4 to 77.3 (1.00)                                                                       | sequential |      1 |
| columbus 2 | `96942e91` 31.3 to 36.7   | yes     | 31.8 to 36.5 (1.00); 34.4 to 36.7 (1.00)                                                                       | concurrent |      1 |
| columbus 2 | `d91eb7cf` 52.1 to 92.2   | yes     | 52.1 to 79.1 (1.00); 77.6 to 92.2 (1.00)                                                                       | concurrent |      1 |
| columbus 2 | `59461dce` 70.4 to 97.2   | yes     | 70.4 to 91.7 (1.00); 91.7 to 102.0 (0.54)                                                                      | sequential |      1 |
| columbus 2 | `ce72fbdc` 70.6 to 111.0  | yes     | 70.6 to 96.2 (1.00); 95.9 to 111.0 (1.00)                                                                      | concurrent |      1 |
| columbus 2 | `d95f2dbd` 106.5 to 149.6 | yes     | 106.5 to 149.6 (1.00); 114.9 to 118.5 (1.00); 146.7 to 151.2 (0.78)\*                                          | concurrent |      1 |
| columbus 2 | `d0f89ce6` 108.5 to 115.4 | yes     | 108.5 to 113.7 (1.00); 114.1 to 115.4 (1.00)                                                                   | sequential |      1 |
| lombard 1  | `fc491eb3` 0.0 to 49.9    | yes     | 0.0 to 41.7 (0.81); 31.2 to 49.9 (0.99)                                                                        | concurrent |      1 |
| lombard 1  | `8f4da850` 13.7 to 24.5   | yes     | 13.7 to 17.4 (0.86); 17.0 to 24.5 (1.00)                                                                       | concurrent |      1 |
| lombard 1  | `499c8ede` 14.5 to 24.4   | yes     | 14.5 to 23.2 (1.00); 18.4 to 24.4 (1.00)                                                                       | concurrent |      1 |
| lombard 1  | `c65bd158` 43.9 to 51.2   | yes     | 42.5 to 51.2 (0.82); 43.9 to 46.5 (0.57)                                                                       | concurrent |      1 |
| lombard 1  | `c3c55b27` 45.1 to 56.6   | yes     | 45.1 to 56.6 (1.00); 48.7 to 53.0 (0.58)                                                                       | concurrent |      1 |
| lombard 1  | `653b62f2` 51.5 to 62.7   | yes     | 51.5 to 62.6 (0.83); 52.1 to 62.7 (0.73)                                                                       | concurrent |      1 |
| lombard 1  | `16d49f71` 68.1 to 81.7   | yes     | 65.2 to 81.7 (0.90); 71.6 to 75.1 (0.50)                                                                       | concurrent |      1 |
| lombard 1  | `48832d18` 69.7 to 78.5   | yes     | 69.7 to 74.2 (0.62); 70.2 to 82.7 (0.71)                                                                       | concurrent |      1 |
| lombard 1  | `95e77943` 74.9 to 89.8   | yes     | 74.9 to 89.0 (0.52); 76.7 to 89.8 (0.94)                                                                       | concurrent |      1 |
| lombard 1  | `8d428333` 76.0 to 88.3   | yes     | 76.0 to 88.3 (0.99); 79.8 to 86.6 (0.50)                                                                       | concurrent |      1 |
| lombard 1  | `9cbfd0a1` 80.1 to 95.9   | yes     | 80.1 to 95.6 (0.98); 82.2 to 95.9 (0.85)                                                                       | concurrent |      1 |
| lombard 1  | `cc74a287` 81.6 to 89.5   | yes     | 80.9 to 89.5 (0.69); 81.6 to 83.3 (0.93); 83.4 to 87.7 (1.00)                                                  | concurrent |      2 |
| lombard 1  | `2232ec46` 135.4 to 145.5 | yes     | 135.4 to 137.4 (0.75); 137.8 to 145.6 (0.99)                                                                   | sequential |      1 |
| lombard 1  | `b46b9294` 143.1 to 153.1 | yes     | 139.2 to 153.1 (0.81); 143.1 to 145.1 (0.50); 143.9 to 147.2 (0.83)                                            | concurrent |      2 |
| lombard 1  | `06f360d9` 141.4 to 147.9 | yes     | 141.4 to 145.9 (0.68); 144.2 to 147.9 (0.93)                                                                   | concurrent |      1 |
| lombard 2  | `089aff6f` 0.3 to 11.6    | yes     | 0.0 to 11.6 (0.75); 0.3 to 2.6 (0.88); 1.2 to 11.5 (0.83)                                                      | concurrent |      2 |
| lombard 2  | `a2f17e06` 19.0 to 26.2   | yes     | 20.7 to 23.8 (0.77); 24.2 to 26.2 (1.00)                                                                       | sequential |      1 |
| lombard 2  | `86a0502b` 21.5 to 65.1   | yes     | 22.4 to 65.1 (0.88); 47.5 to 65.1 (1.00)                                                                       | concurrent |      1 |
| lombard 2  | `370efc19` 72.1 to 83.9   | yes     | 71.2 to 83.9 (0.64); 72.1 to 78.2 (1.00)                                                                       | concurrent |      1 |
| lombard 2  | `72fb24cd` 81.1 to 93.0   | yes     | 81.1 to 89.8 (1.00); 91.2 to 93.0 (1.00)                                                                       | sequential |      1 |
| lombard 2  | `876fc506` 83.7 to 91.8   | yes     | 82.3 to 85.3 (0.67); 83.7 to 91.8 (0.99)                                                                       | concurrent |      1 |
| lombard 2  | `ba8e986b` 88.0 to 102.0  | yes     | 86.7 to 95.6 (0.78); 90.7 to 102.0 (0.75)                                                                      | concurrent |      1 |
| lombard 2  | `c57f9d99` 108.3 to 115.6 | yes     | 103.2 to 113.7 (0.60); 111.8 to 115.6 (1.00)                                                                   | concurrent |      1 |
| lombard 2  | `fe050340` 127.0 to 153.7 | yes     | 126.9 to 142.4 (0.99); 127.0 to 153.7 (1.00)                                                                   | concurrent |      1 |
| lombard 2  | `5e130c2e` 130.7 to 153.7 | yes     | 130.3 to 153.6 (0.87); 130.7 to 153.7 (1.00)                                                                   | concurrent |      1 |
| lombard 2  | `14e80a77` 130.5 to 153.7 | yes     | 130.5 to 139.5 (0.99); 136.4 to 153.7 (0.99)                                                                   | concurrent |      1 |

Every joining track is itself moving under the peak test. Under the old median test, columbus 1's
`d8630f38` is not: it lives 78 s with a median speed of 0.34 m/s and a maximum of 8.2 m/s, the
stop-and-go case the peak test was written for.

### Pooled over the four segments

| Arm                | Brief | Clutter | Moving, peak | Moving, median | Dup pairs | Pair-frames |
| ------------------ | ----: | ------: | -----------: | -------------: | --------: | ----------: |
| `a0`               |   221 |     249 |          520 |            486 |       115 |       3,118 |
| `likelihood-birth` |   292 |     320 |          501 |            464 |        65 |       1,233 |

### The rows read (reported, not decided)

On the rows, which span only the scored 120 s, the peak count moves by −8.2 %, −5.6 %, +8.7 % and
+10.7 % raw, and by +9.4 %, +4.5 %, +12.0 % and +14.1 % after the credit (columbus 1, columbus 2,
lombard 1, lombard 2). `lidar_flip_cost.py` on the rows counts 5, 3, 15 and 12 lasting moving
tracks lost against `a0` under the median test. A row track's duration is its span inside the
window, so tracks cut by the window's edges move in and out of "lasting", and these losses were
not traced. The rows read is worse for the candidate than the evidence read on every count.

## Against the rule

| Clause  | `likelihood-birth`                                                                           |
| ------- | -------------------------------------------------------------------------------------------- |
| L1      | Pass: HOTA 5th +0.0032, IDF1 5th +0.0055                                                     |
| L2      | **Fail**: noise costs on 4 of 4 segments (brief +4, +5, +25, +37); a mover lost at lombard 2 |
| L3      | **Fail**: lombard 2's credited peak count +8.1 % (bound 5 %); p85 within 0.40 m/s            |
| Outcome | Not named for label0003's spans. Nothing adopted                                             |

## Interpretation

Each reading is marked measured or inferred.

- **On a busy arterial the likelihood cost makes brief tracks the birth gate cannot hold.**
  Measured: at lombard-broderick brief tracks rise by a third and a half, and nearly all the new
  ones lie away from any lasting track's body. Inferred: the gate absorbs fragments born inside a
  confirmed vehicle's box. These are born elsewhere, as at haight-divisadero's second segment in
  l5id0023, where the extra clutter was artefacts away from movers. The `ln det S` term changes
  which scraps win the contests that confirm a track, and more traffic means more contests.
- **The candidate also splits and doubles vehicles that the defaults keep whole.** Measured: at
  lombard 2 it carries 15 of `a0`'s moving vehicles in two moving tracks, 7 as sequential pieces
  and 8 as short concurrent doubles, and confirms one vehicle 1.5 s late. Inferred: this is the
  same cost in another form. A young track that wins a contest on a busy road is as likely to be
  a second track on a real vehicle as a scrap of clutter. It is why the credited count rises:
  the credit gives back what the joins took, and the splits stay.
- **The identity gain holds off kirk0.** Measured: duplicate pairs fall at every segment (pooled
  115 to 65) and pair-frames by 60 %. 35 of the 42 joins are an `a0` duplicate removed. The gain
  and the cost come together.
- **The peak test does what it was written for, and it does not decide here.** Measured: under
  the old median test with the credit, the count moves by more than 5 % at columbus 1 alone,
  where a 78 s stop-and-go track falls under the median threshold. Under the peak test that
  track is moving and columbus 1 passes. Lombard 2 fails under the peak test because of splits,
  not joins. Neither measure would have named the candidate: L2 fails without L3.
- **The moving test changes the count, not the verdict.** Measured: the peak test counts 34 more
  of `a0`'s tracks as moving pooled (520 against 486) and 10 fewer at columbus 1. Tracks that are
  slow for most of their lives but reach 1.5 m/s count; tracks that move steadily at 1.0 to
  1.5 m/s do not.

## Recommendations

1. **Do not carry `likelihood-birth` to label0003's spans.** It passes the anchor and fails both
   placement clauses on segments drawn by lot. The noise cost is in brief tracks away from any
   vehicle's body, at a busy site, which is where a deployment's counts are read.
2. **Read the likelihood cost's new births at lombard-broderick before another birth rule.** A
   tap like l5id0021's on lombard's first segment would say what the new brief tracks and the
   concurrent doubles are born from: range, a tentative track's age at the contest it won, or a
   cluster split by the ground. l5id0023 recommended the same at haight 2; lombard has about ten
   times the rise, so it is the clearer place to read it.
3. **Keep the peak test and the join table for the next count clause.** The join credit is
   honest only beside a count of the splits the candidate adds. A later rule should credit joins
   and charge splits in the same table: each `a0` moving track's carriers, one, two or none.
4. **Scale the brief-track bound to the segment.** An absolute rise of 3 is a sixth to a quarter of
   `a0`'s brief tracks at haight-divisadero and 4 % at lombard. It did not decide here (the rises are 25
   and 37), but a bound per hundred moving tracks, like clutter's, would read busy and quiet
   segments alike.
5. **Keep everything default-off.** Nothing here is held out: kirk0 is in sample, and the two
   placements have been read label-free for identity and noise by earlier screens.

## What this does not establish

- **Physical truth on the placements.** The segments have no labels. Brief tracks, splits, joins
  and counts are what the pipeline drew, not where road users were. That a join or a split is one
  vehicle is read from footprints and times.
- **That the lost vehicle at lombard 2 is a vehicle.** It moves at 9 m/s for 2.5 s at 56 to 77 m.
  Nothing here classifies it.
- **A held-out read.** label0003's spans were not read and do not exist yet.
- **A reading of other sites.** Two placements drawn by lot from 21 are a thin screen. The busy one
  decided; a quiet one might not have.
- **The cause of the new brief tracks.** It is inferred from where they lie, not traced to the
  contests that confirmed them.
- **Pi cost.** No timing is read; the host ran at a load average of 12 to 14 on 4 cores.
- **The rows-read losses.** They were reported, not traced.

## Provenance

| Item          | Value                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| ------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Base          | `exp/l5id0024` from `39705e4` (spec approved, rule committed before any replay)                                                                                                                                                                                                                                                                                                                                                                                                              |
| Tooling       | `70f7012` (`--moving peak`, `--joins`, committed before any replay) and `33fc658` (mutual tie-break and the credit's exception, after the first segment's figures); `pytest scripts/test_lidar_figure_sensitivity.py scripts/test_lidar_flip_cost.py` pass                                                                                                                                                                                                                                   |
| Tools         | Built at `39705e4` (`39705e47eee89238633d25fdec204619cb00dc62`) by the Makefile's LDFLAGS (`version.Version` 0.5.1-pre52, `GitSHA`, `BuildTime`) with the `pcap` tag, against the run's static libpcap (submodule `a999701d`). Every replay header and box-eval summary reads `build_version` 0.5.1-pre52 and `build_git_sha` `39705e47…`. `lidar-state-estimation-baseline` `sha256:bcd5743f2b35effa…`, `lidar-box-eval` `7a4d26557d9c36e9…`, `lidar-ground-truth-eval` `7b33b6cabe1e1738…` |
| Parameter set | `config/tuning.defaults.json`, fingerprint `ab0c4035780d`, file SHA-256 `9ad7377f…`                                                                                                                                                                                                                                                                                                                                                                                                          |
| Arm hashes    | `a0` `ab0c4035780d`; `likelihood-birth` `63e42b981b90`                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| Baselines     | Tracking baseline digests, first and repeat equal: `a0` `79d9abff…`, `likelihood-birth` `e6479076…`; source manifest `sha256:d7e33e0d…`                                                                                                                                                                                                                                                                                                                                                      |
| Capture       | kirk0, `sha256:2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a`                                                                                                                                                                                                                                                                                                                                                                                                             |
| Lot           | `random.Random(20261110).sample` over the 21 sites of `tools/s2-archive/site-index.json` in file order, less ashbury-downey, haight-divisadero and embarcadero-folsom: columbus-north-point, lombard-broderick                                                                                                                                                                                                                                                                               |
| Placements    | Hugging Face `banshee-data/sf-street-speeds`: columbus-north-point `20260902T1153_80858-0e4`, `sha256:f6f4a4993a2a83a1…8389`; lombard-broderick `20260902T0938_80858-0d4`, `sha256:1eb08d9e6ced4e20…4643`                                                                                                                                                                                                                                                                                    |
| Segments      | Cut by `pcapng_window.py` at 0 and 300 s for 300 s: columbus 1 `ceeb92abc1c45467…e1e0`, 2 `7dc184577c8b2867…7b98`; lombard 1 `a38d974e77fc69a7…77aa`, 2 `667e235853dfa873…33c3`                                                                                                                                                                                                                                                                                                              |
| Labels        | The pack RECONSTRUCTED as in l5id0019 and l5id0023: label set `kirk0-reference` revision 2233, pack `ds_67842cb1b22b70e7` rebuilt by `lidar-pack-rebuild` (pack digest `sha256:f0ed3414…`); split `kirk0-tuning` manifest `7194bb61…`, reference digest `sha256:a784e69d…`; `obj_788622b5` ignored                                                                                                                                                                                           |
| Eligibility   | Committed cohort manifest; a scratch use ledger at migration 71 (the run's shared ledger for the candidate's kirk0 replay, a copy of it for `a0`'s after the shared one was locked); actor `appius-l5id0024`                                                                                                                                                                                                                                                                                 |
| Host          | Claude Code cloud container, 4 cores, shared with other experiments; two lanes, `GOMAXPROCS=2`, `nice -n 5`                                                                                                                                                                                                                                                                                                                                                                                  |
| Raw output    | Scratch only. Captures were deleted once cut and replayed; evidence databases, rows and summaries were deleted once these tables were written                                                                                                                                                                                                                                                                                                                                                |
