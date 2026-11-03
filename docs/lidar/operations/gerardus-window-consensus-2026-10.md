# Registration by agreement across four background windows

<!-- ignore-style-length -->

[pose0002](gerardus-background-window-2026-10.md) found that at the wrong-peak sites the coarse
peak that wins depends on the background's window, and that ground p95 at a fixed pose moves 5 to
8 cm between windows at two passing sites. One background is one draw. This report registers four
windows of every archive capture and accepts a pose only when the windows agree. It answers
[pose0003](../../../data/experiments/pose/pose0003.md), whose rule was committed (`9da4853c`)
before any registration.

- **Status:** Complete, 2026-11-03. Not adopted: clause 3c fails. No pose accepted and nothing written to the shared Gerardus server
- **Layers:** L3 (the settled background, by window), Gerardus `register` with its defaults
- **Related:** [static registration of the 24 sites](gerardus-static-registration-2026-10.md),
  [the background's length](gerardus-background-window-2026-10.md),
  [Broadway at Franklin correction](broadway-franklin-location-correction.md)

## Summary

- **The procedure is not adopted.** None of pose0002's five wrong-peak sites is registered by
  agreement, and clause 3c needed two. Clauses 3a and 3b hold.
- **Six sites are registered by agreement.** They are the six pose0001 CANDIDATEs: ashbury-downey,
  lombard-laguna, columbus-broadway, van-ness-sacramento, franklin-mcallister and
  embarcadero-bryant. Each consensus pose matches its pose0001 distance from the hint and heading
  to the report's 0.1 m and 0.1°, and lies within 0.12 m and 0.2° of the owner's accepted pose.
- **The windows agree far more often than they pass.** 17 of 24 sites have a consensus pose, 16
  of them from all four windows. Eleven of the 17 still fail a gate: hypothesis separation at all
  eleven, and facade p75 at five. At those sites every window lands on the same pose, within
  0.10 m and 0.5°. Separation measures how the surroundings score a runner-up, not whether the
  pose repeats.
- **The consensus poses match the owner's accepted poses.** At 14 of the 17 sites with
  agreement, the consensus is within 0.12 m and 0.2° of the accepted pose. The exceptions are
  marina-broderick (0.89 m, 0.8°), embarcadero-bay (0.47 m) and 1st-mission (2.85 m), all three
  hand-placed.
- **The wrong-peak sites still flip.** pierce-haight, fulton-divisadero, california-leon-baker,
  broadway-franklin, laguna-eddy, hyde-ofarrell and bush-powell have no three windows that agree.
  embarcadero-folsom agrees at 60, 90 and 180 s on its accepted pose (0.03 m, 0.1°), but fails
  facade p75 there (0.207 to 0.248 m against 0.20). Its 300 s window lands 23 m away on a reversed
  heading, as in pose0002.

## Question

Is the pose that most of four windows agree on a better answer than any single window's? And
does accepting a site only when the windows agree keep every pose pose0001 passed?

## Method

**Backgrounds.** Each of the 24 placements' first 60, 90, 180 and 300 s was replayed in analysis
mode on a scratch server (`velocity-report-local` 0.5.1-pre50 at `84492f9e`, built with
`make build-radar-local`), with the defaults, from the whole-placement captures on the NAS. The
preserved grid was persisted after each replay and exported with `cmd/tools/bgasc`. All four
windows were made in this run. pose0001's 300 s exports were not reused, because the defaults' L3
has changed since (l3bg0023, the foreground refinement). The 300 s backgrounds are the ones
[ground0006](../../../data/experiments/ground/ground0006.md) read.

**Registration.** The installed Gerardus `0.2.0+0c2bf37d68e7` ran `register` with its defaults on
each of the 96 backgrounds: 20 seeds, ±5 m and ±30°, a 1.0 m correspondence, 0.15 m voxels, a
60 m range and a 30 m search. It started from the eye-read position in
`tools/s2-archive/site-index.json`, without a heading, and on the L16 cell from Gerardus's
`REGISTRATION-STATUS`, with broadway-franklin at its corrected cell `808580c27`. It ran on a private
copy of the server's data directory: the cells and maps from the nightly snapshot of
2026-11-02 12:21Z, and each site's current bundle. The shared server was not written to, and no
pose was accepted. One registration ran at a time, beside another lane's replays.

**The rule**, with its reading fixed at approval:

1. _Agreement._ Four windows agree when at least three best poses (rank 1: site-frame x, y and
   heading) lie pairwise within 0.10 m horizontally and 0.5°. The median of the largest such set
   is the _consensus pose_.
2. _Registered by agreement._ At least three of the agreeing windows are CANDIDATE. The CLI scores
   gates only at a run's own best pose, so they are read there. That pose is within 0.10 m and 0.5°
   of every other agreeing pose.
3. _Adopted_ for MVP step 5 when (a) every pose0001 CANDIDATE is registered by agreement within
   0.10 m and 0.5° of its pose0001 pose, (b) no site registered by agreement has a consensus
   heading more than 6.4° from the hint's (180° − `north_azimuth_deg`), and (c) at least two of
   pose0002's five wrong-peak sites are registered by agreement.

pose0001's raw poses are not on this host. Its report gives each CANDIDATE's distance from the hint
and heading to 0.1 m and 0.1°, so clause 3a compares those two numbers. That is a necessary
condition for 0.10 m and 0.5°, not a sufficient one.

**Beyond the rule.** Since pose0001, the owner has accepted a pose at every site on the shared
server: six registered, 18 placed by hand. Each consensus pose is compared with the accepted one.
That comparison does not enter the rule.

## Results

Each window's state (C = CANDIDATE, F = FAILED) and best heading. Consensus poses are in Gerardus's
site frame for the site's L16 cell. "Δ hint" is the consensus heading less 180° −
`north_azimuth_deg`. The accepted pose is the owner's record on the shared server, read from the
snapshot. 96 registrations took 43 to 134 s each, 72 s on average; the 96 backgrounds took about
3 h 50 min.

| Site                  | 60 s     | 90 s     | 180 s    | 300 s    | Agreeing         | Consensus x, y (m) | Heading | Δ hint | Passing of agreeing | By agreement | Accepted pose   | Δ accepted (m, °) |
| --------------------- | -------- | -------- | -------- | -------- | ---------------- | ------------------ | ------: | -----: | ------------------: | ------------ | --------------- | ----------------- |
| ashbury-downey        | C 171.0° | C 171.0° | C 171.0° | C 171.0° | 60, 90, 180, 300 | -48.55, -83.01     |   171.0 |    1.0 |                   4 | **yes**      | registered      | 0.01, 0.0         |
| haight-divisadero     | F 79.8°  | F 79.8°  | C 79.8°  | F 79.8°  | 60, 90, 180, 300 | 13.73, 53.53       |    79.8 |    0.8 |                   1 | no           | manually_placed | 0.01, 0.0         |
| pierce-haight         | F 172.7° | F 350.9° | F 171.2° | F 171.1° | –                | –                  |       – |      – |                   – | no           | manually_placed | –                 |
| fulton-divisadero     | F 85.5°  | F 242.3° | F 237.4° | C 80.8°  | –                | –                  |       – |      – |                   – | no           | manually_placed | –                 |
| california-leon-baker | F 259.1° | F 259.8° | F 261.5° | F 259.7° | –                | –                  |       – |      – |                   – | no           | manually_placed | –                 |
| broadway-franklin     | F 81.1°  | F 81.0°  | F 81.1°  | F 81.1°  | –                | –                  |       – |      – |                   – | no           | manually_placed | –                 |
| laguna-eddy           | F 216.4° | C 170.3° | C 170.3° | F 180.8° | –                | –                  |       – |      – |                   – | no           | registered      | –                 |
| lombard-broderick     | F 85.9°  | F 86.0°  | F 86.0°  | F 86.0°  | 60, 90, 180, 300 | 59.32, 72.25       |    86.0 |    3.0 |                   0 | no           | manually_placed | 0.02, 0.1         |
| marina-broderick      | F 264.4° | F 264.4° | F 264.4° | F 264.4° | 60, 90, 180, 300 | 46.82, 34.78       |   264.4 |    6.4 |                   0 | no           | manually_placed | 0.89, 0.8         |
| marina-webster-beach  | F 312.3° | F 312.3° | F 312.3° | F 312.2° | 60, 90, 180, 300 | -33.52, -40.53     |   312.3 |    3.3 |                   0 | no           | manually_placed | 0.01, 0.0         |
| lombard-laguna        | C 265.8° | C 265.8° | C 265.8° | C 265.8° | 60, 90, 180, 300 | 50.50, 57.92       |   265.8 |    0.8 |                   4 | **yes**      | registered      | 0.03, 0.1         |
| columbus-north-point  | C 133.0° | F 133.0° | C 133.0° | F 133.0° | 60, 90, 180, 300 | -33.16, -78.81     |   133.0 |    1.0 |                   2 | no           | registered      | 0.02, 0.1         |
| embarcadero-bay       | F 290.3° | F 290.3° | F 290.3° | F 290.3° | 60, 90, 180, 300 | 31.02, 24.34       |   290.3 |    3.3 |                   0 | no           | manually_placed | 0.47, 0.1         |
| columbus-broadway     | C 143.6° | F 143.6° | C 143.6° | C 143.6° | 60, 90, 180, 300 | -14.11, -41.89     |   143.6 |    1.4 |                   3 | **yes**      | registered      | 0.00, 0.1         |
| van-ness-sacramento   | C 175.2° | C 175.2° | C 175.4° | F 175.4° | 60, 90, 180, 300 | -48.86, 29.04      |   175.3 |    5.3 |                   3 | **yes**      | manually_placed | 0.12, 0.2         |
| embarcadero-broadway  | F 78.2°  | C 78.2°  | C 78.2°  | F 78.2°  | 60, 90, 180, 300 | 23.58, -55.61      |    78.2 |    0.8 |                   2 | no           | manually_placed | 0.03, 0.0         |
| franklin-mcallister   | C 347.5° | C 347.5° | C 347.5° | C 347.5° | 60, 90, 180, 300 | 13.25, -57.24      |   347.5 |    0.5 |                   4 | **yes**      | manually_placed | 0.02, 0.0         |
| hyde-ofarrell         | F 347.9° | F 347.9° | F 347.9° | F 347.9° | –                | –                  |       – |      – |                   – | no           | manually_placed | –                 |
| bush-powell           | F 113.7° | F 113.4° | F 112.5° | F 117.5° | –                | –                  |       – |      – |                   – | no           | manually_placed | –                 |
| embarcadero-folsom    | F 49.4°  | F 49.4°  | F 49.4°  | F 237.0° | 60, 90, 180      | -42.33, -56.24     |    49.4 |    0.4 |                   0 | no           | registered      | 0.03, 0.1         |
| 1st-mission           | F 129.9° | F 129.9° | F 129.9° | F 129.9° | 60, 90, 180, 300 | 44.76, -40.40      |   129.9 |    0.9 |                   0 | no           | manually_placed | 2.85, 0.3         |
| embarcadero-bryant    | C 186.7° | C 186.7° | C 186.7° | C 186.6° | 60, 90, 180, 300 | -10.38, -32.00     |   186.7 |    0.3 |                   4 | **yes**      | manually_placed | 0.07, 0.0         |
| 3rd-folsom            | F 317.5° | F 317.5° | F 317.5° | F 317.5° | 60, 90, 180, 300 | 64.36, -6.37       |   317.5 |    0.5 |                   0 | no           | manually_placed | 0.01, 0.0         |
| howard-6th            | F 228.2° | F 228.2° | F 228.2° | F 228.2° | 60, 90, 180, 300 | -39.66, -41.48     |   228.2 |    0.2 |                   0 | no           | manually_placed | 0.01, 0.0         |

### Against the rule

| Clause                                                                       | Required       | Measured                                                                      | Result |
| ---------------------------------------------------------------------------- | -------------- | ----------------------------------------------------------------------------- | ------ |
| 3a. Every pose0001 CANDIDATE registered by agreement, within 0.10 m and 0.5° | 6 of 6         | 6 of 6; distance from the hint equal to 0.05 m and heading to 0.1° at all six | holds  |
| 3b. No site registered by agreement with heading > 6.4° from the hint        | none           | largest 5.3° (van-ness-sacramento)                                            | holds  |
| 3c. Wrong-peak sites registered by agreement                                 | 2 or more of 5 | 0: four have no agreement, and embarcadero-folsom agrees but fails facade p75 | fails  |

Gates that failed, counted over the four windows: separation at all 11 sites that agree but are
not registered, and at 23 of the 28 runs at the seven sites without agreement. Facade p75 fails in at least one
window at haight-divisadero, lombard-broderick, embarcadero-folsom, 1st-mission and 3rd-folsom
among the sites that agree.

## Interpretation

1. **The verdict stands: single-window registration stays for MVP step 5.** Agreement keeps
   every pose pose0001 passed, and adds no wrong heading. It does not recover a wrong-peak site,
   which clause 3c asked of it.
2. **Agreement measures a different property from the gates.** At 17 sites four backgrounds of
   one capture land on one pose to a few centimetres, including the 11 that fail separation. That
   repeatability is what pose0002 said a single window lacked. It is not independence: the windows
   share the sensor, the start and much of the data.
3. **Where the windows agree, they agree with the operator.** At 14 of 17 sites the consensus is
   within 0.12 m and 0.2° of the accepted pose, though nine of those poses were placed by hand in
   the pane. The three that differ are worth the owner's look: marina-broderick, embarcadero-bay
   and 1st-mission.
4. **The wrong-peak sites need a different start or a second capture, not more windows.** Seven
   sites still flip between peaks from window to window. At embarcadero-folsom the short windows
   find the right pose, and the facade gate stops it there, as in pose0002.

## Sites registered by agreement

These six would be registered under this procedure. Their consensus poses (site frame) are in the
table above. All six already carry an owner-accepted pose: three registered and three placed by
hand. This report accepts nothing.

## What it does not establish

- No pose is checked against the world.
- Agreement across windows of one capture is not independence.
- Clause 3a compares the report-rounded distance from the hint and heading, not full poses.
  pose0001's raw outputs are not on this host.
- The gates are read at each agreeing window's own pose, not at the median.
- Poses are parsed from the CLI's text at 0.01 m and 0.1°.

## Provenance

| Item        | Value                                                                                                                                                                                                       |
| ----------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Rule        | `9da4853c` on `exp/pose0003-window-consensus`, from `main` `84492f9e`                                                                                                                                       |
| Backgrounds | `velocity-report-local` 0.5.1-pre50 at `84492f9e` (SHA-256 `cbf534c5…`), tuning fingerprint `ab0c4035780d`; analysis mode, 0 to 60, 90, 180 and 300 s of each placement; `bgasc` from the ground0006 branch |
| Captures    | `sf-street-speeds/raw/lidar/` on the NAS: 24 whole placements, franklin-mcallister's re-export                                                                                                              |
| Gerardus    | `0.2.0+0c2bf37d68e7`, defaults; data copy from the nightly snapshot `20261102T122145Z` plus the 24 current bundles; the shared server untouched                                                             |
| Host        | arrow-worker, 4 vCPU, one job at a time beside another lane                                                                                                                                                 |
| Raw outputs | backgrounds, CLI outputs and `pose0003.json` under the run's scratch and `velocity-campaign/exp-20261103/laneB` on the results share; not in Git                                                            |

## Recommendations

1. **Keep single-window registration.** Use agreement as a reviewer's aid in the pane: show how
   many windows land on the pose.
2. **Revisit the separation gate with this evidence.** At 11 sites four windows land on one pose
   and the gate fails on all four. pose0001 already proposed defining "distinct" outside the seed
   basin; this run gives that change its test set.
3. **Check three hand-placed poses**: marina-broderick, embarcadero-bay and 1st-mission, where
   the agreeing windows sit 0.47 to 2.85 m from the placement.
