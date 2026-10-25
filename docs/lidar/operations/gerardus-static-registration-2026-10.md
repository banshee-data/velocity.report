# Static registration of the 24 archive sites against 3DEP: the outcomes table and the search settings

Only one of the 24 archive captures has a registered sensor pose. This report registers all 24
settled backgrounds against USGS 3DEP through Gerardus from their eye-read positions, under
thirteen arms that vary the search settings nobody had varied, and under backgrounds made with
and without the L3 track guard. It answers [pose0001](../../../data/experiments/pose/pose0001.md).

- **Status:** Complete; run 2026-10-24 to 2026-10-25, unattended. No pose was accepted: a
  CANDIDATE is a pose every gate passed, and `registered` needs an operator's acceptance in the
  pane
- **Layers:** L3 (the settled background and its export), Gerardus `register` (coarse search,
  point-to-plane ICP, the six gates)
- **Related:** [Gerardus alignment plan](../../plans/lidar-visualiser-gerardus-alignment-plan.md),
  [L3 track guard](l3-track-guard-kirk-2026-10.md), Gerardus `docs/REGISTRATION-STATUS.md` and
  `docs/plans/MVP-REGISTER-808581.md` (MVP step 5), the review
  `docs/reviews/2026-10-24-vector-map-and-slam-review.md` and the maths note
  `docs/research/2026-10-24-registration-and-slam-maths.md` in the Gerardus repository
- **Parameter sets:** the [defaults](../../../config/tuning.defaults.json) (the track guard on) and
  [track-guard-off.json](../../../config/experiments/track-guard-off.json) for the backgrounds;
  the registration arms are Gerardus settings, listed below

## Summary

- **CANDIDATE at 6 of 24 sites** from the eye-read positions with the defaults: ashbury-downey,
  lombard-laguna, columbus-broadway, van-ness-sacramento, franklin-mcallister and
  embarcadero-bryant, each 3.8–6.6 m from the eye-read point with the hint's heading within 5.4°.
- **Hypothesis separation fails at all 18 others**, alone at nine. Thirteen of the 18 carry the
  hint's heading within 6.4°; five land on a wrong peak 36–173° off.
- **No search setting helps under the rule.** Heading, seeds and random seed reproduce the defaults
  exactly; a 0.5 m correspondence, 0.10 m voxels, 80 m range and a 15 m search each lose one to
  three passing sites; DataSF boxes move a passing pose by 0.46 m. Nothing becomes a default.
- **The L3 track guard helps**: off, four candidates are lost and two gained, with separation
  falling at sites where vehicles dwell. **The 300 s background is not better than the 83 s one**:
  R1's backgrounds pass at both sites with tonight's code, including embarcadero-folsom, where
  every 300 s background fails.
- **The separation gate compares the best pose with neighbours in its own basin** at several
  sites; a definition of "distinct" outside the seed basin is the one Gerardus change this argues
  for. The 1.25 threshold is not moved.
- Run time: 24 backgrounds in 55–87 s each, 314 registrations at 12–85 s each; about four hours
  unattended on the M1.

## Question

MVP 0 step 5 asks for a registration record for every archive site. R1 registered three sites
from the command line in October: two CANDIDATE, one FAILED under the Transit Center. The other
21 had never been tried. Three questions:

1. How many of the 24 register from their eye-read position with the defaults, and which gate
   stops each of the rest?
2. Does any search setting move a site across a gate without moving a pose that already passed?
   The settings: the hint's heading, the seed count and random seed, the correspondence distance,
   the voxel size, the range, the search radius, and DataSF box walls as extra reference.
3. Does the L3 track guard, which keeps moving vehicles out of the background, change a pose or a
   gate?

The rule for reading the answers was written before any run, in the spec.

## Method

**Backgrounds.** Each site's raw capture from the S2 corpus (`sf-street-speeds/raw/lidar/`)
replayed on a scratch server (port 18080, its own clone of the sensor database) in analysis mode
for the first 300 s, with the full pipeline (L1–L6). The preserved grid was persisted
(`POST /api/lidar/persist`) and exported as ASC with a new tool, `cmd/tools/bgasc`, which picks
the snapshot a run produced by its time window and writes the `X Y Z Intensity AverageRange
TimesSeen` file Gerardus reads. Two arms: the defaults (the track guard on) and
`track-guard-off.json`. A replay of 300 s took 55–80 s; the exports hold 59–72 k points, the
same order as R1's whole-run exports (68–71 k).

**Registration.** Gerardus at `claude/register-experiments` (`f8e3ea0` on `29d0e32`, #19), on a
copy of the data directory taken from the NAS the same evening. Each run:
`gerardus register --asc <site>.asc --at <lat>,<lon>[,<heading>] --l16 <token>` with the eye-read
position from `tools/s2-archive/site-index.json` and the L16 token from REGISTRATION-STATUS. Six
sites still had flat bundles without footprints, streets or kerbs; their bundles were rebuilt
from the caches with the published 3DEP cut reused, which the branch makes possible. The arms:

| Arm                                         | Backgrounds | What changes from the defaults (20 seeds, ±5 m, ±30°, 1.0 m correspondence, 0.15 m voxels, 60 m range, 30 m search)  |
| ------------------------------------------- | ----------- | -------------------------------------------------------------------------------------------------------------------- |
| `base`                                      | guard on    | nothing                                                                                                              |
| `heading`                                   | guard on    | the hint carries 180° − `north_azimuth_deg`; expected to repeat `base`, since the coarse search covers every heading |
| `seeds40`                                   | guard on    | 40 seeds                                                                                                             |
| `rseed2`                                    | guard on    | random seed 2                                                                                                        |
| `corr05`                                    | guard on    | 0.5 m correspondence                                                                                                 |
| `vox10`                                     | guard on    | 0.10 m scan and reference voxels                                                                                     |
| `range80`                                   | guard on    | points used out to 80 m                                                                                              |
| `radius15`                                  | guard on    | 15 m coarse search                                                                                                   |
| `boxes`                                     | guard on    | DataSF box walls, corrected by measured wall planes where there are any, as extra reference                          |
| `perturb-e15`, `perturb-n15`, `perturb-e25` | guard on    | the hint moved 15 m east, 15 m north, 25 m east                                                                      |
| `base-guardoff`                             | guard off   | nothing                                                                                                              |

A run took about 20 s once the bundle was prepared. Every run appended a registration to the
copy's alignment log; none was reviewed.

## Results

### The outcomes table, `base`

Every site from its eye-read position, the defaults, no heading. "Moved" is the best hypothesis's
distance from the hint; "Δ from hint heading" is the best hypothesis's heading of +Y less
180° − `north_azimuth_deg`, which R1 found to hold within 1.5° at three sites. A `nan` facade is a
site where fewer than 100 held-out wall points met the planar, near-vertical selection, so the
facade gates did not apply.

| #   | Site                  | State     | Moved from hint (m) | Heading of +Y | Δ from hint heading | Ground p95 | Facade p75 |   Slope | Rank | Separation | Seeds | Failing gates                                                                     | Time (s) |
| --- | --------------------- | --------- | ------------------: | ------------: | ------------------: | ---------: | ---------: | ------: | ---: | ---------: | ----: | --------------------------------------------------------------------------------- | -------: |
| 01  | ashbury-downey        | CANDIDATE |                 3.8 |         171.0 |                -1.0 |      0.083 |      0.110 |  0.0009 |    6 |       1.37 |  1.00 | –                                                                                 |       81 |
| 02  | haight-divisadero     | FAILED    |                16.4 |          79.8 |                +0.8 |      0.089 |      0.155 | -0.0006 |    6 |       1.22 |  1.00 | hypothesis_separation                                                             |       41 |
| 03  | pierce-haight         | FAILED    |                23.4 |         171.1 |                +2.1 |      0.139 |      0.309 |  0.0023 |    6 |       1.05 |  0.00 | facade_p75_m,hypothesis_separation,seed_agreement                                 |       80 |
| 04  | fulton-divisadero     | FAILED    |                18.0 |         237.8 |              +158.8 |      0.216 |        nan |     nan |    6 |       1.00 |  0.25 | ground_p95_m,hypothesis_separation,seed_agreement                                 |       69 |
| 05  | california-leon-baker | FAILED    |                31.2 |          81.4 |                +2.4 |      0.127 |      0.268 | -0.0028 |    6 |       1.20 |  1.00 | facade_p75_m,hypothesis_separation                                                |       68 |
| 06  | broadway-gough        | FAILED    |                30.2 |         348.4 |               -92.6 |      0.221 |      0.391 |  0.0097 |    6 |       1.05 |  0.25 | ground_p95_m,facade_p75_m,facade_range_slope,hypothesis_separation,seed_agreement |       46 |
| 07  | laguna-eddy           | FAILED    |                21.8 |         211.1 |               +40.1 |      0.211 |        nan |     nan |    6 |       1.00 |  0.15 | ground_p95_m,hypothesis_separation,seed_agreement                                 |       53 |
| 08  | lombard-broderick     | FAILED    |                 7.7 |          86.0 |                +3.0 |      0.225 |      0.233 |  0.0024 |    6 |       1.06 |  1.00 | ground_p95_m,facade_p75_m,hypothesis_separation                                   |       45 |
| 09  | marina-broderick      | FAILED    |                 1.8 |         264.4 |                +6.4 |      0.099 |        nan |     nan |    6 |       1.18 |  1.00 | hypothesis_separation                                                             |       40 |
| 10  | marina-webster-beach  | FAILED    |                 4.8 |         312.2 |                +3.2 |      0.094 |        nan |     nan |    6 |       1.16 |  1.00 | hypothesis_separation                                                             |       48 |
| 11  | lombard-laguna        | CANDIDATE |                 4.8 |         265.8 |                +0.8 |      0.122 |      0.078 |  0.0000 |    6 |       1.26 |  1.00 | –                                                                                 |       57 |
| 12  | columbus-north-point  | FAILED    |                 4.2 |         133.0 |                +1.0 |      0.095 |      0.113 | -0.0002 |    6 |       1.22 |  1.00 | hypothesis_separation                                                             |       60 |
| 13  | embarcadero-bay       | FAILED    |                 5.2 |         290.3 |                +3.3 |      0.100 |      0.124 |  0.0010 |    6 |       1.21 |  1.00 | hypothesis_separation                                                             |       51 |
| 14  | columbus-broadway     | CANDIDATE |                 5.8 |         143.6 |                -1.4 |      0.185 |      0.174 | -0.0001 |    6 |       1.46 |  1.00 | –                                                                                 |       25 |
| 15  | van-ness-sacramento   | CANDIDATE |                 4.7 |         175.4 |                +5.4 |      0.146 |      0.173 |  0.0010 |    6 |       1.58 |  1.00 | –                                                                                 |       59 |
| 16  | embarcadero-broadway  | FAILED    |                 4.5 |          78.2 |                -0.8 |      0.096 |      0.109 |  0.0009 |    6 |       1.21 |  1.00 | hypothesis_separation                                                             |       53 |
| 17  | franklin-mcallister   | CANDIDATE |                 6.6 |         347.5 |                -0.5 |      0.174 |      0.130 |  0.0003 |    6 |       1.46 |  1.00 | –                                                                                 |       67 |
| 18  | hyde-ofarrell         | FAILED    |                12.7 |          76.9 |               -89.1 |      0.168 |      0.533 | -0.0076 |    6 |       1.01 |  0.00 | facade_p75_m,hypothesis_separation,seed_agreement                                 |       56 |
| 19  | bush-powell           | FAILED    |                 8.7 |         113.8 |               +35.8 |      0.218 |        nan |     nan |    6 |       1.03 |  0.00 | ground_p95_m,hypothesis_separation,seed_agreement                                 |       70 |
| 20  | embarcadero-folsom    | FAILED    |                25.2 |         235.8 |              -173.2 |      0.173 |        nan |     nan |    6 |       1.00 |  0.00 | hypothesis_separation,seed_agreement                                              |       85 |
| 21  | 1st-mission           | FAILED    |                 2.9 |         129.9 |                +0.9 |      0.150 |      0.247 | -0.0001 |    6 |       1.12 |  1.00 | facade_p75_m,hypothesis_separation                                                |       75 |
| 22  | embarcadero-bryant    | CANDIDATE |                 4.6 |         186.6 |                -0.4 |      0.123 |        nan |     nan |    6 |       1.29 |  1.00 | –                                                                                 |       77 |
| 23  | 3rd-folsom            | FAILED    |                12.2 |         317.5 |                +0.5 |      0.152 |      0.159 |  0.0010 |    6 |       1.23 |  0.35 | hypothesis_separation,seed_agreement                                              |       69 |
| 24  | howard-6th            | FAILED    |                13.0 |         228.2 |                +0.2 |      0.144 |      0.113 |  0.0000 |    6 |       1.22 |  1.00 | hypothesis_separation                                                             |       82 |

**CANDIDATE at 6 of 24.** Hypothesis separation fails at all 18 others; it is the only failing gate at 9 of them.

Reading the heading column against the failures:

- **13 of the 18 failures have the hint's heading within 6.4°**: haight-divisadero,
  pierce-haight, california-leon-baker, lombard-broderick, marina-broderick,
  marina-webster-beach, columbus-north-point, embarcadero-bay, embarcadero-broadway, 1st-mission,
  3rd-folsom, howard-6th and bush-powell's runner-up aside. Nine of them fail on separation alone
  with ground p95 under 0.13 m and facade p75 under 0.16 m, at separations of 1.16 to 1.23 against
  the gate's 1.25. These look like the right pose that the aerial survey cannot vouch for along
  the street: the maths note's prediction that separation is about 1 + 2.6 g, with g the share of
  points an along-street shift moves, puts them at g of 6 to 9 % against the gate's 10 %.
- **5 failures have a heading 36° to 173° from the hint**: fulton-divisadero, broadway-gough,
  laguna-eddy, hyde-ofarrell and embarcadero-folsom, with bush-powell at 36°. These are wrong
  peaks: the coarse search's best candidate is not the sensor's place, the seeds disagree (0.00 to
  0.25) and the ground p95 is at or over the gate. embarcadero-folsom was a CANDIDATE in R1 at
  heading 49.4°; tonight's best is 235.8° and 25 m away. R1's background came from an 83 s replay
  on 2026-10-14, before the track guard; tonight's from 300 s with it. Which of the two explains
  the change is the `r1bg` check below.
- **The six CANDIDATEs** moved 3.8 to 6.6 m from the eye-read point, as R1's did, with headings
  within 1.4° of the hint except van-ness-sacramento at 5.4°.

### The arms against `base`

Each arm over the 24 sites. "New" and "lost" are sites whose state changed against `base`; the
pose-shift columns are over the six `base` CANDIDATEs, the rule's test that an arm moves no pose
that already passed. `r1bg` registers the two R1 backgrounds exported from the 83 s replays of
2026-10-14 (before the track guard) with tonight's code.

| Arm           | Runs | CANDIDATE | New candidates (site)                      | Lost candidates (site)                                                       | Pose moved > 0.5 m or 1° at a base candidate | Max pose shift at base candidates (m, °) | Median time (s) |
| ------------- | ---: | --------: | ------------------------------------------ | ---------------------------------------------------------------------------- | -------------------------------------------- | ---------------------------------------- | --------------: |
| base-guardoff |   24 |         4 | marina-broderick, marina-webster-beach     | embarcadero-bryant, franklin-mcallister, lombard-laguna, van-ness-sacramento | van-ness-sacramento (10.46 m, +96.1°)        | 10.46, 96.10 (van-ness-sacramento)       |              14 |
| boxes         |   24 |         4 | –                                          | franklin-mcallister, van-ness-sacramento                                     | none                                         | 0.46, 0.60 (franklin-mcallister)         |              25 |
| corr05        |   24 |         3 | –                                          | ashbury-downey, embarcadero-bryant, lombard-laguna                           | none                                         | 0.04, 0.00 (embarcadero-bryant)          |              14 |
| heading       |   24 |         6 | –                                          | –                                                                            | none                                         | 0.00, 0.00 (van-ness-sacramento)         |              27 |
| perturb-e15   |   24 |         6 | columbus-north-point                       | embarcadero-bryant                                                           | none                                         | 0.00, 0.00 (van-ness-sacramento)         |              14 |
| perturb-e25   |   24 |         7 | columbus-north-point, marina-webster-beach | lombard-laguna                                                               | none                                         | 0.00, 0.00 (van-ness-sacramento)         |              13 |
| perturb-n15   |   24 |         6 | –                                          | –                                                                            | none                                         | 0.00, 0.00 (van-ness-sacramento)         |              13 |
| r1bg          |    2 |         2 | embarcadero-folsom                         | –                                                                            | none                                         | 0.01, 0.10 (columbus-broadway)           |              14 |
| radius15      |   24 |         4 | pierce-haight                              | columbus-broadway, embarcadero-bryant, lombard-laguna                        | none                                         | 0.00, 0.00 (van-ness-sacramento)         |              12 |
| range80       |   24 |         5 | –                                          | lombard-laguna                                                               | none                                         | 0.02, 0.00 (embarcadero-bryant)          |              15 |
| rseed2        |   24 |         6 | –                                          | –                                                                            | none                                         | 0.00, 0.00 (van-ness-sacramento)         |              14 |
| seeds40       |   24 |         6 | –                                          | –                                                                            | none                                         | 0.00, 0.00 (van-ness-sacramento)         |              25 |
| vox10         |   24 |         5 | –                                          | lombard-laguna                                                               | none                                         | 0.02, 0.00 (columbus-broadway)           |              17 |

- **Determinism.** `heading`, `rseed2`, `seeds40` and `perturb-n15` reproduce `base`'s poses to
  the millimetre at every CANDIDATE, and `heading` reproduces every state. The coarse search
  covers every heading and the hint's heading enters only the summary, as the maths note
  predicted; the seeds' agreement was already 1.00 wherever a pose passed.
- **No arm helps under rule 2.** The only new CANDIDATEs come from `radius15` (pierce-haight) and
  the perturbed starts (columbus-north-point, marina-webster-beach), and each of those arms loses
  one to three sites that `base` passed. `corr05` loses three, as predicted: a tighter final gate
  drops the recess points the fit needs. `range80` and `vox10` each lose lombard-laguna, whose
  separation of 1.26 had 0.01 to spare. `boxes` loses two and is the only setting that moves a
  passing pose (franklin-mcallister by 0.46 m and 0.6°, the eave inset's bias at a site with no
  measured wall planes) and its facade p75 rises to 0.68 m there.
- **Separation is set by which runner-up falls in the window.** columbus-north-point's best pose is
  the same from every start, at heading 133.0°; its separation is 1.22 from the eye-read point
  (runner-up 1.8 m away at 135.1°), 1.27 from 15 m east and 1.40 from 25 m east. columbus-broadway
  passes at 1.46 from a 30 m search and fails at exactly 1.250 from a 15 m one, against a
  runner-up 4.9 m along the street at the same heading. lombard-laguna's radius15 runner-up is
  2.5 m away at rank 5. So the gate is often comparing the best pose with a neighbour inside the
  same basin, which the seeds (spread ±5 m) already collapse onto the best, rather than with a
  distinct alternative a lot away. A "distinct" alternative should be defined as one the seed
  basin does not reach, before the 1.25 threshold is read as a verdict on the pose.

### Perturbed starts

Convergence is the best pose landing within 0.20 m and 0.5° of `base`'s best.

| Arm         | Converged | Did not | Sites that did not (shift m)                                                                                                                                                                  |
| ----------- | --------: | ------: | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| perturb-e15 |        16 |       8 | broadway-gough (11.1), bush-powell (0.3), california-leon-baker (26.1), embarcadero-folsom (23.2), fulton-divisadero (3.9), haight-divisadero (26.0), laguna-eddy (2.1), pierce-haight (56.4) |
| perturb-e25 |        18 |       6 | broadway-gough (4.6), california-leon-baker (60.3), embarcadero-folsom (23.2), haight-divisadero (28.0), hyde-ofarrell (58.0), pierce-haight (73.1)                                           |
| perturb-n15 |        20 |       4 | broadway-gough (63.9), california-leon-baker (26.1), embarcadero-folsom (23.2), laguna-eddy (32.7)                                                                                            |

The right-looking poses converge from every start: the six CANDIDATEs and the nine separation-only
failures all return to the same pose from 15 m north, and all but california-leon-baker and
haight-divisadero from 15 m east. The sites that do not converge are the five wrong peaks, whose
`base` pose was itself one of several, and the two Haight sites, where a 15–25 m eastward start
finds another block's facade line. The basin of the coarse search is therefore about the search
radius around a right peak, and the wrong-peak sites are wrong from any start.

### The track guard, and the background's length

`base-guardoff` registers backgrounds made with `track-guard-off.json`, L3 as it ran before #854.

| Site                  | On        | Off       | Pose shift (m, °) | Ground p95 on / off | Facade p75 on / off | Separation on / off |
| --------------------- | --------- | --------- | ----------------- | ------------------- | ------------------- | ------------------- |
| ashbury-downey        | CANDIDATE | CANDIDATE | 0.00, 0.00        | 0.083 / 0.084       | 0.110 / 0.103       | 1.37 / 1.37         |
| haight-divisadero     | FAILED    | FAILED    | 0.00, 0.00        | 0.089 / 0.094       | 0.155 / 0.148       | 1.22 / 1.23         |
| pierce-haight         | FAILED    | FAILED    | 0.10, 0.10        | 0.139 / 0.149       | 0.309 / 0.376       | 1.05 / 1.05         |
| fulton-divisadero     | FAILED    | FAILED    | 6.83, 6.50        | 0.216 / 0.221       | nan / 0.586         | 1.00 / 1.00         |
| california-leon-baker | FAILED    | FAILED    | 0.00, 0.00        | 0.127 / 0.107       | 0.268 / 0.237       | 1.20 / 1.20         |
| broadway-gough        | FAILED    | FAILED    | 0.01, 0.10        | 0.221 / 0.227       | 0.391 / 0.328       | 1.05 / 1.04         |
| laguna-eddy           | FAILED    | FAILED    | 8.63, 12.30       | 0.211 / 0.236       | nan / nan           | 1.00 / 1.00         |
| lombard-broderick     | FAILED    | FAILED    | 0.04, 0.00        | 0.225 / 0.216       | 0.233 / 0.234       | 1.06 / 1.05         |
| marina-broderick      | FAILED    | CANDIDATE | 0.00, 0.10        | 0.099 / 0.098       | nan / nan           | 1.18 / 1.27         |
| marina-webster-beach  | FAILED    | CANDIDATE | 0.01, -0.10       | 0.094 / 0.108       | nan / nan           | 1.16 / 1.25         |
| lombard-laguna        | CANDIDATE | FAILED    | 0.03, 0.00        | 0.122 / 0.196       | 0.078 / 0.093       | 1.26 / 1.21         |
| columbus-north-point  | FAILED    | FAILED    | 0.01, 0.00        | 0.095 / 0.118       | 0.113 / 0.115       | 1.22 / 1.22         |
| embarcadero-bay       | FAILED    | FAILED    | 0.01, 0.00        | 0.100 / 0.103       | 0.124 / 0.129       | 1.21 / 1.16         |
| columbus-broadway     | CANDIDATE | CANDIDATE | 0.01, 0.00        | 0.185 / 0.200       | 0.174 / 0.171       | 1.46 / 1.44         |
| van-ness-sacramento   | CANDIDATE | FAILED    | 10.46, 96.10      | 0.146 / 0.248       | 0.173 / nan         | 1.58 / 1.00         |
| embarcadero-broadway  | FAILED    | FAILED    | 0.00, 0.00        | 0.096 / 0.100       | 0.109 / 0.111       | 1.21 / 1.19         |
| franklin-mcallister   | CANDIDATE | FAILED    | 0.00, 0.00        | 0.174 / 0.248       | 0.130 / 0.132       | 1.46 / 1.19         |
| hyde-ofarrell         | FAILED    | FAILED    | 0.64, -0.30       | 0.168 / 0.199       | 0.533 / 0.577       | 1.01 / 1.00         |
| bush-powell           | FAILED    | FAILED    | 0.46, -0.10       | 0.218 / 0.228       | nan / 0.587         | 1.03 / 1.02         |
| embarcadero-folsom    | FAILED    | FAILED    | 2.24, -0.80       | 0.173 / 0.169       | nan / nan           | 1.00 / 1.01         |
| 1st-mission           | FAILED    | FAILED    | 0.01, 0.00        | 0.150 / 0.172       | 0.247 / 0.264       | 1.12 / 1.12         |
| embarcadero-bryant    | CANDIDATE | FAILED    | 0.03, 0.00        | 0.123 / 0.134       | nan / nan           | 1.29 / 1.13         |
| 3rd-folsom            | FAILED    | FAILED    | 0.01, 0.00        | 0.152 / 0.165       | 0.159 / 0.207       | 1.23 / 1.22         |
| howard-6th            | FAILED    | FAILED    | 0.01, -0.10       | 0.144 / 0.157       | 0.113 / 0.128       | 1.22 / 1.20         |

- **The guard helps.** With it off, four CANDIDATEs are lost (lombard-laguna, van-ness-sacramento,
  franklin-mcallister, embarcadero-bryant) and two gained (the two Marina sites, at separations of
  1.25 and 1.27). At franklin-mcallister and embarcadero-bryant the pose does not move but the
  separation falls from 1.46 to 1.19 and from 1.29 to 1.13, and the ground p95 rises: vehicles
  learned into the background add points that match the aerial survey nowhere, and the maths
  note's reading (anything raising the unmatched share compresses separation) fits. At
  van-ness-sacramento the guard-off background lands on a wrong peak 10 m away and 96° off.
- **The 300 s background is not better than the 83 s one.** `r1bg` makes both R1 backgrounds
  CANDIDATE with tonight's code, embarcadero-folsom at the R1 heading of 49.4° (separation 1.47).
  Tonight's 300 s backgrounds of the same capture fail there whether the guard is on or off,
  landing 25 m away at 235°. The guard is not the cause; the extra 217 s of background is. The
  Embarcadero has dwelling buses and trucks that stop longer than the guard's cool-down, and far
  cells settle later; which of the two the longer window adds is not measured here. Either way the
  background's window is a registration setting, and 60–90 s from the start of a capture is the
  one with evidence behind it.

## Interpretation

1. **The outcomes table stands: 6 of 24 pass every gate from their eye-read positions**, and 13 of
   the 18 that fail do so on separation with the hint's heading, nine of them on separation alone.
   Those nine are where an operator's eye in the pane, with the residual colouring, should decide:
   the gate was a filter for a reviewer, and tonight it filters out poses that every other measure
   calls right. Five sites land on a wrong peak and need a different start, a second scan or a tie
   point.
2. **No search setting is adopted.** Nothing passed rule 2 at two sites without harming one, and
   the one setting that moves a passing pose (`boxes`) does so for a known reason. The defaults
   stay. The two settings that matter are not Gerardus's: the L3 track guard, which should stay on,
   and the background's window, which should be short.
3. **The separation gate needs a definition of "distinct".** Hypotheses a few metres apart along
   the street at the same heading are one basin, not two; the seeds already prove it. Counting only
   alternatives outside the seed basin would pass columbus-north-point, lombard-laguna and probably
   the other separation-only failures without moving a pose, and would stop a 15 m search from
   failing columbus-broadway at 1.250. This is the one change this run argues for in Gerardus, and
   it must be confirmed on kirkham-9th's reviewed poses before it changes a default.
4. **The guard's effect on registration is the first held-out evidence for it.** The track-guard
   report asked for "Gerardus re-registering from guarded replays"; here the guard gains four
   candidates and loses two, moves no passing pose, and raises separation where vehicles dwell.
5. **Background length is an open question with one data point.** The 83 s backgrounds register
   where the 300 s ones do not at embarcadero-folsom. A window sweep (60, 90, 180, 300 s) at the
   five wrong-peak sites is the next cheap experiment.

   **Answered by [pose0002](../../../data/experiments/pose/pose0002.md) (2026-10-25):** no window
   from 60 to 300 s is better under its rule. The wrong-peak sites alternate between peaks from
   window to window, so the length is not a dial for them:
   [report](gerardus-background-window-2026-10.md).

## Limitations

- No pose is checked against a surveyed position. The gates measure consistency with the aerial
  survey, and the maths note shows which direction they cannot see: the along-street translation
  on a straight street rests only on surfaces with along-street normals.
- Five minutes of background rather than the whole capture. Point counts match R1's, but cells
  seen rarely may be thinner.
- The 2026-09 eye-read positions are the only start; a site whose eye-read error exceeds the
  search radius cannot register whatever the settings.
- One night, one machine, one random seed per arm but `rseed2`. Nothing here is a default yet.

## Provenance

- velocity.report `claude/geraldus-vector-slam-388337` on `main` `d8a3aeb56` (#858); the server
  binary `v0.5.1-pre47`, git sha `feb0652cb` (the `w4-w7-layers` branch, whose Go code is main's).
- Gerardus `claude/register-experiments` `f8e3ea0`.
- Captures: the S2 corpus raw files on `/Volumes/lidar/lidar/pcaps/sf-street-speeds/raw/lidar/`.
- Gerardus data: a copy of `/mnt/results/gerardus/data` (arrow-worker) taken 2026-10-24 about
  20:00 PDT; the copy's alignment log holds the runs and is not the shared log.
- Tuning fingerprint of the defaults: `e44f577dc1df`.
- Raw outputs (backgrounds, per-run logs, the results table) stay under
  `/Volumes/lidar/gerardus-exp-20261024/`, outside the repository.
