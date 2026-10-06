# Near-edge tracked-state campaign, October 2026

- **Status:** Complete. Retain production B0; no candidate or diagnostic correction is selected.
- **Scope:** Same-build comparisons at 23 prior-exposed core sites and six transfer recording sessions, with deterministic repeats, diagnostics, balanced timing, and retained-output analysis.
- **Related:** [Near-edge tracked-state plan](../../plans/lidar-near-edge-tracked-state-plan.md), [state-estimation plan](../../plans/lidar-state-estimation-plan.md), and [near-face evaluation](near-face-evaluation.md).

The campaign asked whether feeding near-edge geometry into tracked state improves steadiness
across recordings without degrading continuity or tracker cost. It also tested existing
components and inspected known regressions and transfer sessions.

A2 improves many label-free geometry summaries, but breaches the observed update-cost screen at
all 29 core/transfer cases and at least one geometry screen at nine. Its mean complete confirmed
duration decreases at 22 of 23 core sites; Claren loses about 43% of mean complete duration.
Diagnostic arms and tail review do not identify a universal correction. Keep B0 as the operational
reference while investigating cost, continuity, and specific geometry contexts separately.

This document preserves the experiment's methods, numerical findings, interpretation, and
limitations in Git. Raw replay outputs, recordings, row-level data, configurations, logs,
databases, and campaign state stay in the local evidence archive. They are not needed to read
this report and must not be committed as documentation attachments.

The planned window was October 2, 2026, 08:35:35 PDT to October 4, 2026, 08:35:35 PDT. The human
removed the hard stop, then requested immediate finalisation. Evidence reconciliation finished
at approximately 06:55 PDT on October 4, and the owned supervisor exited at approximately 06:57.
This completed report supersedes earlier checkpoints' pending work. No additional replay was
started to fill the scheduled window.

## Coverage and recovery

A successful invocation contains a first replay and its repeat. The 123 successful invocations
therefore contain 246 successful scored replay passes, excluding setup tests, coverage surveys,
and failed attempts. Repeats add reproducibility evidence on the same inputs, not independent
site or session coverage. Tracking baselines and exact execution configurations match byte for
byte across each repeat; timing measurements are expected to differ. Repeat observation identity
fields omitted by the exporter remain explicit missingness.

| Category                         | Completed coverage                                                                                       |
| -------------------------------- | -------------------------------------------------------------------------------------------------------- |
| Core                             | 69 invocations: B0, shadow, and A2 at 23 prior-exposed sites                                             |
| Diagnostics                      | 24 invocations: four variants at six selected prior-exposed sites                                        |
| Transfer                         | 18 invocations: B0, shadow, and A2 at six recording sessions; geographic independence is not established |
| Balanced timing                  | 12 invocations: two B0 and two A2 invocations at each of three predeclared sites                         |
| Reused controls                  | Two current-build core controls reused during supervisor recovery, already counted within core 69        |
| Historical baselines substituted | Zero                                                                                                     |
| Failed attempts                  | Two, each recovered with one successful retry; zero unresolved failures                                  |
| Preserved partials               | Both failed attempts retained locally with verified hashes; no active partial replay                     |
| Pending admitted work            | Zero replays, extraction jobs, or bootstrap/analysis jobs                                                |

The first failed attempt requested an unsupported uncertainty report on the tracked arm.
Recovery removed that unsupported combination and retried once with the frozen binary. Supported
B0/shadow reports remain evidence, but candidate pre-gate uncertainty calibration is not
established. The second failure was a confirmation coverage-path collision; one bounded retry
recovered it. Both failed attempts' source/log evidence was preserved. Completed comparisons were
not rerun during finalisation.

F8 was excluded from untouched confirmation because prior scoring and analysis require a
governed split and single-score audit. The sf-street-speeds exports overlap S2 and add no
independent coverage. Further Kirk/Soma candidates were excluded for tuning exposure, capture
overlap, uncertain or moving sensor placement, or short static windows. Reviewed annotation packs
exist, but eligible physical trajectory/pose references, alignment, and review governance remain
unresolved. Physical scoring was not admitted. The replay allowance was a ceiling, not a queue of
unfinished work.

## Arms and measurement definitions

| Arm        | Definition                                                                                                                                            |
| ---------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| B0         | Production medoid tracker, with experimental geometry disabled                                                                                        |
| Shadow     | `solid_body`, `solid_body_face_hysteresis` (T1), `solid_body_course_faces` (T3), and `solid_body_full_members`; geometry does not drive tracked state |
| A2         | Shadow options plus `near_edge_track`, feeding the near-edge measurement into tracked state                                                           |
| Without T1 | A2 without `solid_body_face_hysteresis`                                                                                                               |
| Without T3 | A2 without `solid_body_course_faces`                                                                                                                  |
| A1         | A2 plus `near_edge_track_a1`, using medoid-referenced association                                                                                     |
| T5         | A2 plus `solid_body_rank_one_medoid`                                                                                                                  |

Geometry compares A2 with same-build shadow. Update cost and complete confirmed duration compare
A2 with B0. B0's medoid residual represents a different estimator and population; it is not a
like-for-like body-geometry control.

The geometry measure is `five_point_time_xy_fit_v1`: lateral residual against a five-point
straight-line fit in capture time. The fit includes the centre point and future samples, so it
is an attenuated, non-causal anomaly measure. A physical manoeuvre can raise it. Moving-track
eligibility requires lifetime maximum speed of at least 6 m/s; fitted local speed must reach
2 m/s, and gaps above 0.3 seconds split the series. Percentiles use nearest rank. These are
label-free residuals, not errors against surveyed physical truth.

Body denotes body-centre frames; steady runs split at reference changes; face-stable runs also
split at face changes. Each arm has its own eligible tracks and windows. Changes in eligibility
can change a percentile without describing the same physical objects. The population counts
below preserve that distinction.

Geometry change is 100 × (A2 p99 / shadow p99 − 1). Above +10% breaches a component's observed
screen. Update cost above 1.5 times B0's Tracker.Update p99 breaches the observed cost screen.
Neither a value within a screen nor an aggregate improvement establishes a physical validation
pass. Governed physical identity, accuracy, and recall remain unknown for every case; Pi
performance and untouched confirmation remain unverified.

## Geometry and per-case gates

Scoring exposure is 30,689.52 seconds (511.49 minutes) for core and 3,499.81 seconds
(58.33 minutes) for transfer. Exposure weighting uses scoring seconds, not residual-window or
track counts. The means below average per-case p99 percentage changes; they are not percentiles
of pooled residuals or population estimates.

| Group    | Population | Equal-case mean % | Exposure-weighted mean % |
| -------- | ---------- | ----------------: | -----------------------: |
| core     | body       |            -18.82 |                   -19.53 |
| core     | steady     |            -22.13 |                   -22.67 |
| core     | face       |            -12.35 |                   -11.43 |
| transfer | body       |             -1.94 |                    -1.50 |
| transfer | steady     |             -3.01 |                    -2.63 |
| transfer | face       |            +15.69 |                   +15.03 |

Core body, steady, and face p99 improve at 20, 22, and 20 of 23 sites respectively. The individual
breaches remain: 3rd–Folsom and Ashbury–Downey for body, 3rd–Folsom for steady, and Pierce–Haight
for face. Transfer's face mean worsens despite lower body/steady means. Improvements elsewhere
cannot override a breached individual gate.

All rows below breach observed update cost. The first and repeat cost ratios are separate
measurements, not an average. Core/transfer arm ordering was unbalanced; balanced confirmation
is reported separately. Core Columbus/Haight timings retain census-I/O contention caveats.

### Core: 23 sites

| Case                  | Body % | Steady % | Face % | Geometry +10% breaches | Update p99 ratio, first / repeat |
| --------------------- | -----: | -------: | -----: | ---------------------- | -------------------------------- |
| 1st-mission           | -11.61 |   -13.49 | -22.66 | none observed          | 67.25x / 41.97x                  |
| 3rd-folsom            | +24.98 |   +22.91 | -10.05 | body, steady           | 18.95x / 18.60x                  |
| ashbury-downey        | +23.74 |   -11.00 |  -3.44 | body                   | 44.68x / 31.52x                  |
| broadway-gough        | -37.06 |   -39.77 | -32.77 | none observed          | 27.19x / 23.58x                  |
| bush-powell           |  +5.66 |    -7.97 |  +0.71 | none observed          | 103.51x / 52.67x                 |
| california-leon-baker |  -3.82 |    -3.24 | -12.37 | none observed          | 26.22x / 21.11x                  |
| columbus-broadway     |  -7.14 |    -7.14 | -10.08 | none observed          | 106.66x / 54.70x                 |
| columbus-north-point  | -14.90 |   -11.11 | -20.67 | none observed          | 30.64x / 21.15x                  |
| embarcadero-bay       | -29.02 |   -29.61 | -10.92 | none observed          | 90.30x / 49.20x                  |
| embarcadero-broadway  | -31.09 |   -36.11 | -22.14 | none observed          | 134.08x / 96.19x                 |
| embarcadero-bryant    | -19.41 |   -28.21 |  -7.89 | none observed          | 33.07x / 33.10x                  |
| franklin-mcallister   | -49.96 |   -47.49 | -19.54 | none observed          | 26.32x / 22.14x                  |
| fulton-divisadero     | -21.69 |   -23.19 |  +5.31 | none observed          | 6.80x / 5.63x                    |
| haight-divisadero     | -36.07 |   -36.07 | -43.50 | none observed          | 161.95x / 93.59x                 |
| howard-6th            | -23.09 |   -27.35 | -19.58 | none observed          | 53.94x / 38.38x                  |
| hyde-ofarrell         | -37.41 |   -32.85 | -22.64 | none observed          | 34.04x / 22.78x                  |
| laguna-eddy           | -17.02 |   -19.69 |  -2.00 | none observed          | 51.50x / 44.23x                  |
| lombard-broderick     | -18.27 |   -28.94 | -19.08 | none observed          | 40.45x / 29.01x                  |
| lombard-laguna        | -27.05 |   -35.34 | -37.10 | none observed          | 35.17x / 31.84x                  |
| marina-broderick      | -26.35 |   -33.98 | -11.96 | none observed          | 36.13x / 18.58x                  |
| marina-webster-beach  | -16.68 |   -10.83 | -14.25 | none observed          | 51.97x / 28.97x                  |
| pierce-haight         | -52.60 |   -46.29 | +80.20 | face                   | 16.91x / 14.39x                  |
| van-ness-sacramento   |  -6.99 |    -2.21 | -27.62 | none observed          | 132.45x / 63.82x                 |

### Transfer: six recording sessions

| Case         | Body % | Steady % | Face % | Geometry +10% breaches | Update p99 ratio, first / repeat |
| ------------ | -----: | -------: | -----: | ---------------------- | -------------------------------- |
| clar0        | -58.25 |   -58.25 | +38.07 | face                   | 40.04x / 29.07x                  |
| claren0      | +37.03 |   +37.03 |  -0.97 | body, steady           | 15.57x / 38.42x                  |
| kirk1        | -25.78 |   -39.51 | +27.51 | face                   | 67.22x / 40.17x                  |
| morg0        | +42.99 |   +42.99 |  +2.06 | body, steady           | 16.13x / 8.89x                   |
| soma1-static | -17.44 |   -16.24 | +38.78 | face                   | 41.92x / 28.78x                  |
| soma3-static |  +9.81 |   +15.93 | -11.29 | steady                 | 99.43x / 58.69x                  |

Nine cases breach at least one geometry component. All 29 breach update cost. The six transfer
names identify recording sessions, not six established independent geographic sites.

### Absolute geometry and population sizes

Each cell gives shadow / A2 p99 in metres, with eligible five-point window counts in parentheses.
These are each arm's own eligible populations. Window counts are correlated samples, not
independent physical objects. Small Claren populations and changed face-stable populations at
Pierce and Clar matter when interpreting their p99.

| Case                  | Body p99 m (windows)              | Steady p99 m (windows)            | Face p99 m (windows)              |
| --------------------- | --------------------------------- | --------------------------------- | --------------------------------- |
| 1st-mission           | 0.23420 (2,115) / 0.20700 (2,128) | 0.23406 (2,080) / 0.20248 (2,092) | 0.14583 (1,617) / 0.11279 (1,592) |
| 3rd-folsom            | 0.27751 (3,041) / 0.34683 (2,453) | 0.25617 (3,008) / 0.31484 (2,398) | 0.11729 (1,997) / 0.10550 (1,690) |
| ashbury-downey        | 0.23300 (1,587) / 0.28830 (1,454) | 0.22055 (1,559) / 0.19629 (1,421) | 0.08974 (952) / 0.08665 (802)     |
| broadway-gough        | 0.32096 (2,338) / 0.20202 (2,044) | 0.32035 (2,289) / 0.19293 (1,998) | 0.12158 (1,344) / 0.08174 (1,181) |
| bush-powell           | 0.19761 (1,928) / 0.20879 (1,778) | 0.19606 (1,901) / 0.18044 (1,734) | 0.11502 (1,259) / 0.11583 (1,136) |
| california-leon-baker | 0.31146 (1,805) / 0.29956 (1,488) | 0.30959 (1,784) / 0.29956 (1,451) | 0.14812 (870) / 0.12980 (696)     |
| columbus-broadway     | 0.27981 (6,809) / 0.25984 (6,154) | 0.27982 (6,735) / 0.25984 (6,037) | 0.14080 (4,011) / 0.12662 (3,469) |
| columbus-north-point  | 0.30891 (1,258) / 0.26288 (1,124) | 0.26943 (1,239) / 0.23951 (1,101) | 0.11397 (791) / 0.09041 (725)     |
| embarcadero-bay       | 0.35824 (4,112) / 0.25428 (3,465) | 0.33498 (4,066) / 0.23579 (3,411) | 0.13588 (2,438) / 0.12104 (2,041) |
| embarcadero-broadway  | 0.26774 (6,050) / 0.18450 (5,753) | 0.26692 (5,930) / 0.17053 (5,663) | 0.13182 (3,971) / 0.10263 (3,893) |
| embarcadero-bryant    | 0.20412 (8,395) / 0.16449 (7,558) | 0.19815 (8,357) / 0.14226 (7,476) | 0.10870 (5,876) / 0.10012 (5,301) |
| franklin-mcallister   | 0.46967 (1,609) / 0.23502 (1,476) | 0.44162 (1,602) / 0.23191 (1,467) | 0.14181 (1,148) / 0.11410 (1,025) |
| fulton-divisadero     | 0.44650 (1,418) / 0.34967 (1,090) | 0.45523 (1,385) / 0.34967 (1,068) | 0.21212 (770) / 0.22338 (585)     |
| haight-divisadero     | 0.21065 (844) / 0.13466 (861)     | 0.21065 (838) / 0.13466 (861)     | 0.17142 (492) / 0.09686 (506)     |
| howard-6th            | 0.22224 (2,921) / 0.17093 (2,665) | 0.21435 (2,837) / 0.15572 (2,559) | 0.10504 (1,611) / 0.08448 (1,463) |
| hyde-ofarrell         | 0.39139 (1,143) / 0.24497 (1,205) | 0.37169 (1,129) / 0.24957 (1,190) | 0.14336 (741) / 0.11090 (810)     |
| laguna-eddy           | 0.23560 (985) / 0.19551 (884)     | 0.23068 (970) / 0.18525 (860)     | 0.17099 (529) / 0.16757 (438)     |
| lombard-broderick     | 0.30124 (3,170) / 0.24620 (2,789) | 0.30117 (3,094) / 0.21401 (2,695) | 0.12606 (1,773) / 0.10200 (1,528) |
| lombard-laguna        | 0.37501 (5,159) / 0.27357 (4,618) | 0.37501 (5,074) / 0.24247 (4,488) | 0.14798 (3,073) / 0.09308 (2,711) |
| marina-broderick      | 0.19737 (4,028) / 0.14537 (4,013) | 0.19837 (3,977) / 0.13097 (3,937) | 0.10924 (2,494) / 0.09617 (2,456) |
| marina-webster-beach  | 0.15786 (5,586) / 0.13153 (5,093) | 0.13488 (5,551) / 0.12027 (5,022) | 0.06541 (3,771) / 0.05609 (3,435) |
| pierce-haight         | 0.51981 (501) / 0.24641 (482)     | 0.42758 (482) / 0.22965 (472)     | 0.10998 (214) / 0.19819 (233)     |
| van-ness-sacramento   | 0.25852 (3,524) / 0.24046 (3,023) | 0.23801 (3,482) / 0.23275 (2,973) | 0.17301 (1,865) / 0.12523 (1,661) |
| clar0                 | 0.50020 (267) / 0.20882 (216)     | 0.50020 (267) / 0.20882 (207)     | 0.06023 (175) / 0.08316 (144)     |
| claren0               | 0.15611 (135) / 0.21391 (111)     | 0.15611 (135) / 0.21391 (111)     | 0.13887 (58) / 0.13753 (51)       |
| kirk1                 | 0.27518 (1,042) / 0.20425 (956)   | 0.26829 (1,033) / 0.16228 (943)   | 0.08053 (670) / 0.10268 (643)     |
| morg0                 | 0.21293 (938) / 0.30446 (677)     | 0.21293 (929) / 0.30446 (662)     | 0.12584 (486) / 0.12843 (379)     |
| soma1-static          | 0.27269 (1,789) / 0.22513 (1,612) | 0.25813 (1,761) / 0.21621 (1,584) | 0.11355 (911) / 0.15757 (798)     |
| soma3-static          | 0.18752 (705) / 0.20592 (791)     | 0.17762 (696) / 0.20592 (774)     | 0.08268 (373) / 0.07334 (447)     |

## Confirmed duration and continuity

A confirmed interval starts at the first confirmed scored frame and ends at the last supported
observation. Coast time after the last support does not extend it. Means exclude intervals
marked left- or right-censored. Censoring is an attribute of an interval: an interval may have
both flags, so left and right counts must not be added to infer a separate population.

The table preserves B0 / A2 complete counts and left/right censor counts alongside each mean.
These are separately selected intervals, not matched physical-object lifetimes. Confirmation
counts and duration do not establish identity correctness or recall.

| Case                  | B0 mean s | A2 mean s | Change % | Complete B0 / A2 | Left B0 / A2 | Right B0 / A2 |
| --------------------- | --------: | --------: | -------: | ---------------- | ------------ | ------------- |
| 1st-mission           |    5.1919 |    5.1228 |    -1.33 | 1443 / 1471      | 10 / 11      | 10 / 10       |
| 3rd-folsom            |    3.1350 |    3.0107 |    -3.97 | 1363 / 1407      | 4 / 4        | 5 / 5         |
| ashbury-downey        |    6.1981 |    5.9174 |    -4.53 | 360 / 374        | 1 / 2        | 5 / 5         |
| broadway-gough        |    2.7579 |    2.7191 |    -1.41 | 1649 / 1681      | 8 / 7        | 3 / 4         |
| bush-powell           |    3.6669 |    3.6602 |    -0.18 | 1038 / 1041      | 12 / 12      | 7 / 7         |
| california-leon-baker |    2.4407 |    2.4109 |    -1.22 | 1161 / 1172      | 1 / 1        | 8 / 8         |
| columbus-broadway     |    3.5549 |    3.4612 |    -2.63 | 2451 / 2500      | 7 / 7        | 8 / 8         |
| columbus-north-point  |    3.6041 |    3.3120 |    -8.10 | 882 / 921        | 9 / 8        | 1 / 1         |
| embarcadero-bay       |    3.4068 |    3.1985 |    -6.11 | 779 / 807        | 4 / 5        | 7 / 7         |
| embarcadero-broadway  |    3.2486 |    3.0290 |    -6.76 | 1324 / 1379      | 9 / 9        | 6 / 6         |
| embarcadero-bryant    |    3.4023 |    3.3105 |    -2.70 | 1560 / 1581      | 11 / 11      | 1 / 1         |
| franklin-mcallister   |    4.1792 |    4.1134 |    -1.57 | 1315 / 1328      | 14 / 15      | 9 / 9         |
| fulton-divisadero     |    3.3702 |    3.2913 |    -2.34 | 1679 / 1709      | 5 / 5        | 5 / 5         |
| haight-divisadero     |    4.6448 |    4.5713 |    -1.58 | 374 / 381        | 5 / 5        | 9 / 9         |
| howard-6th            |    4.5897 |    4.3256 |    -5.75 | 609 / 621        | 2 / 2        | 3 / 3         |
| hyde-ofarrell         |    4.5724 |    4.5954 |    +0.50 | 693 / 694        | 3 / 3        | 1 / 1         |
| laguna-eddy           |    4.9674 |    4.8644 |    -2.07 | 1101 / 1111      | 2 / 2        | 2 / 2         |
| lombard-broderick     |    3.4843 |    3.4155 |    -1.98 | 1650 / 1687      | 7 / 7        | 7 / 7         |
| lombard-laguna        |    3.2020 |    3.0590 |    -4.47 | 2713 / 2798      | 5 / 4        | 6 / 4         |
| marina-broderick      |    5.4763 |    5.3334 |    -2.61 | 900 / 917        | 6 / 6        | 10 / 10       |
| marina-webster-beach  |    7.1702 |    6.8907 |    -3.90 | 517 / 538        | 11 / 11      | 2 / 2         |
| pierce-haight         |    4.3247 |    4.2849 |    -0.92 | 1434 / 1438      | 9 / 8        | 3 / 3         |
| van-ness-sacramento   |    3.3023 |    3.2068 |    -2.89 | 1589 / 1629      | 5 / 5        | 4 / 4         |
| clar0                 |    1.3122 |    1.3490 |    +2.81 | 205 / 206        | 3 / 3        | 1 / 1         |
| claren0               |    2.8042 |    1.5958 |   -43.09 | 47 / 48          | 1 / 1        | 0 / 0         |
| kirk1                 |    3.8507 |    3.6930 |    -4.09 | 240 / 248        | 2 / 3        | 3 / 3         |
| morg0                 |    2.7509 |    2.7289 |    -0.80 | 580 / 579        | 2 / 2        | 18 / 18       |
| soma1-static          |    1.7218 |    1.6616 |    -3.50 | 1165 / 1174      | 14 / 16      | 11 / 10       |
| soma3-static          |    3.2725 |    3.2160 |    -1.73 | 349 / 356        | 3 / 3        | 4 / 4         |

Mean complete duration decreases at 22 of 23 core sites; Hyde–O'Farrell is the exception.
Claren decreases 43.09%, from 2.8042 to 1.5958 seconds, with 47 / 48 complete intervals and one
additional left-censored interval per arm. There are no right-censored Claren intervals. Its
sparse update gaps and clamped predictions remain evidence requiring a continuity investigation.
Zero backward timestamps does not establish gap-free tracking.

## Diagnostic comparisons

The six sites were selected for diagnostic use and were already exposed. Every percentage below
compares the named variant with full A2, using the same body, steady, and face residual
populations. Negative means a lower p99. This is a descriptive ablation screen, not an untouched
selection set or physical score.

| Case               | Variant vs A2 |  Body % | Steady % | Face % |
| ------------------ | ------------- | ------: | -------: | -----: |
| 3rd-folsom         | Without T1    |   -0.02 |    +6.37 | +13.53 |
| 3rd-folsom         | Without T3    |  -38.22 |   -52.50 |  -6.05 |
| 3rd-folsom         | A1            |  -18.78 |   -17.09 |  -8.72 |
| 3rd-folsom         | T5            |  -28.70 |   -27.60 |  +2.76 |
| ashbury-downey     | Without T1    |  -12.24 |   +24.03 | +48.13 |
| ashbury-downey     | Without T3    |   +3.34 |   -24.23 | +32.83 |
| ashbury-downey     | A1            |   +7.71 |   +58.19 |  +7.43 |
| ashbury-downey     | T5            |   -0.89 |    -3.80 | +19.26 |
| bush-powell        | Without T1    |  +28.26 |   +25.37 |  +6.87 |
| bush-powell        | Without T3    |   -5.00 |    +8.90 | +44.77 |
| bush-powell        | A1            |  +12.52 |   +30.21 |  +2.31 |
| bush-powell        | T5            |   +7.93 |   +18.37 |  +1.80 |
| pierce-haight      | Without T1    |  +31.34 |   +14.63 |  -4.68 |
| pierce-haight      | Without T3    |  -19.35 |   -24.31 | -53.15 |
| pierce-haight      | A1            | +128.13 |  +145.21 |  +3.93 |
| pierce-haight      | T5            |  +21.95 |    +1.54 |  +0.00 |
| fulton-divisadero  | Without T1    |  +12.36 |    +9.95 | -39.70 |
| fulton-divisadero  | Without T3    |   +5.48 |   -20.44 | -51.91 |
| fulton-divisadero  | A1            |  +43.58 |   +38.14 | -34.46 |
| fulton-divisadero  | T5            |  +26.28 |   +25.49 |  +0.00 |
| embarcadero-bryant | Without T1    |  +18.81 |   +31.42 | +10.73 |
| embarcadero-bryant | Without T3    |  +30.24 |   +47.37 | +13.98 |
| embarcadero-bryant | A1            |  +36.31 |   +42.83 | +12.19 |
| embarcadero-bryant | T5            |   -9.36 |    -4.96 |  +6.30 |

Removing T1 worsens steady p99 at all six sites; face p99 improves at two and worsens at four.
Removing T3 improves all three components at 3rd–Folsom, but gives mixed six-site results: body
improves at three, steady at four, and face at three. A1 improves body/steady only at 3rd–Folsom;
face improves at two sites. T5 improves no face p99: four worsen and two are unchanged. None of
these variants supplies a general correction or a selected replacement for A2.

## Balanced update timing

The three sites were predeclared, prior-exposed diagnostics. Each arm has two invocations, each
with a first replay and repeat: four p99 measurements per arm and site. The combined ratio divides
the median of A2's four measurements by the median of B0's four. It is neither a median of paired
ratios nor a p99 pooled across frames. First and repeat differ in persistence, so the separate
first-only and repeat-only ratios are retained.

| Case               | B0 median ms | A2 median ms | Combined ratio | First ratio | Repeat ratio | B0 range ms | A2 range ms |
| ------------------ | -----------: | -----------: | -------------- | ----------- | ------------ | ----------- | ----------- |
| 3rd-folsom         |        0.167 |        3.541 | 21.20x         | 24.14x      | 18.77x       | 0.140–0.196 | 3.418–3.612 |
| pierce-haight      |        0.384 |        6.047 | 15.74x         | 17.16x      | 14.34x       | 0.343–0.438 | 6.020–6.092 |
| embarcadero-bryant |        0.227 |        8.013 | 35.36x         | 38.81x      | 30.03x       | 0.203–0.287 | 7.811–8.070 |

All twelve invocation summaries follow. Order is within each site's declared sequence; samples
count timed Tracker.Update calls per pass. Idle is the invocation's admission sample, not a
continuous guarantee. Values are rendered to six decimal places in milliseconds.

| Case               | Order | Arm | First p99 ms | Repeat p99 ms | Timed calls first / repeat | Observed idle % |
| ------------------ | ----: | --- | -----------: | ------------: | -------------------------- | --------------: |
| 3rd-folsom         |     1 | B0  |     0.139583 |      0.187542 | 10904 / 10904              |           89.51 |
| 3rd-folsom         |     2 | A2  |     3.418417 |      3.594250 | 10904 / 10904              |           89.79 |
| 3rd-folsom         |     3 | A2  |     3.487083 |      3.611667 | 10904 / 10904              |           82.16 |
| 3rd-folsom         |     4 | B0  |     0.146458 |      0.196458 | 10904 / 10904              |           80.20 |
| pierce-haight      |     1 | A2  |     6.019792 |      6.050917 | 15935 / 15935              |           83.69 |
| pierce-haight      |     2 | B0  |     0.343459 |      0.438042 | 15935 / 15935              |           90.52 |
| pierce-haight      |     3 | B0  |     0.359541 |      0.408667 | 15935 / 15935              |           88.00 |
| pierce-haight      |     4 | A2  |     6.042125 |      6.091958 | 15935 / 15935              |           81.72 |
| embarcadero-bryant |     1 | B0  |     0.203833 |      0.286917 | 12894 / 12894              |           87.89 |
| embarcadero-bryant |     2 | A2  |     7.987292 |      8.070459 | 12894 / 12894              |           80.92 |
| embarcadero-bryant |     3 | A2  |     7.811084 |      8.039084 | 12894 / 12894              |           86.16 |
| embarcadero-bryant |     4 | B0  |     0.203250 |      0.249459 | 12894 / 12894              |           81.80 |

These are Mac Tracker.Update wall-time measurements excluding parsing, clustering, persistence,
and diagnostic collection. They do not certify Pi throughput or end-to-end latency. OS activity
can change after admission, and timing alone does not identify the expensive function. The large
balanced ratios remain cost-screen breaches despite these limitations. Future stage/CPU
profiling must use a separately declared build and compare B0, shadow, and A2 before attributing
the cost to a particular mechanism.

## Retained-output robustness

Separate offline extraction decoded 87 retained first-pass recordings and produced 311,513
posterior XY proxy windows without changing the measured replay binary. The proxy uses confirmed
tracker positions with zero misses and the same five-point fit. It differs from observation-anchor
geometry and must not be substituted for a geometry score.

The analysis declaration was post hoc and descriptive. It used 60-second primary non-overlapping
capture-time blocks and 30/120-second sensitivities, starting at the frozen scoring start. A
five-point window belongs to its centre timestamp's block; a duration interval belongs to its
confirmation-time block. Full-track eligibility is retained before assignment, without fitting
across synthetic concatenations. Each case resamples the same time-block indices across arms.
There are 2,000 bootstrap replicates, base seed 20261003, and 2.5/97.5 percentile endpoints.
Separate whole-track resampling occurs within each arm and assumes no matched physical identities.

The following primary intervals compare A2 with B0. The point estimate is posterior XY proxy p99
change; the duration interval concerns mean complete confirmed duration. An interval crossing
zero remains inconclusive within this conditional resampling analysis. These intervals do not
establish independent-frame inference, physical accuracy, or population generalisation.

| Case                  | Posterior XY p99 change % | 60 s proxy interval % | 60 s duration interval % |
| --------------------- | ------------------------: | --------------------- | ------------------------ |
| 1st-mission           |                    +23.34 | [+7.87, +64.77]       | [-15.21, +17.48]         |
| 3rd-folsom            |                    +57.43 | [+24.25, +85.95]      | [-7.77, +0.10]           |
| ashbury-downey        |                    +53.82 | [+29.13, +99.34]      | [-7.53, -1.29]           |
| broadway-gough        |                     -4.28 | [-10.45, +23.60]      | [-12.73, +10.17]         |
| bush-powell           |                    +42.59 | [+9.04, +100.77]      | [-3.20, +2.81]           |
| california-leon-baker |                    +15.91 | [+5.43, +39.66]       | [-3.85, +1.55]           |
| columbus-broadway     |                    +59.16 | [+30.63, +85.44]      | [-5.92, +0.97]           |
| columbus-north-point  |                    +44.34 | [+3.35, +70.10]       | [-14.33, -1.94]          |
| embarcadero-bay       |                    +78.78 | [+53.83, +104.54]     | [-9.67, -2.35]           |
| embarcadero-broadway  |                    +76.89 | [+47.76, +108.27]     | [-9.08, -4.60]           |
| embarcadero-bryant    |                    +30.25 | [+16.82, +49.21]      | [-6.05, +0.35]           |
| franklin-mcallister   |                    +37.45 | [-15.18, +95.72]      | [-6.18, +2.98]           |
| fulton-divisadero     |                    +26.29 | [-1.73, +70.35]       | [-5.04, -0.17]           |
| haight-divisadero     |                    +79.38 | [+22.13, +116.43]     | [-5.07, +2.51]           |
| howard-6th            |                    +31.53 | [+7.16, +60.55]       | [-12.89, +0.84]          |
| hyde-ofarrell         |                    +48.89 | [+23.20, +88.88]      | [-4.87, +8.94]           |
| laguna-eddy           |                    +26.52 | [+5.85, +95.17]       | [-6.24, +1.92]           |
| lombard-broderick     |                    +59.15 | [+33.16, +79.68]      | [-10.53, +6.92]          |
| lombard-laguna        |                    +31.31 | [+4.23, +61.16]       | [-7.81, -1.21]           |
| marina-broderick      |                    +45.42 | [+23.19, +65.98]      | [-5.32, +0.30]           |
| marina-webster-beach  |                    +59.18 | [+38.87, +88.98]      | [-34.80, +42.26]         |
| pierce-haight         |                     +4.29 | [-28.97, +75.87]      | [-3.34, +1.86]           |
| van-ness-sacramento   |                    +35.88 | [+19.14, +57.65]      | [-20.30, +17.84]         |
| clar0                 |                    +40.25 | [-15.43, +137.85]     | [-1.54, +7.22]           |
| claren0               |                    +62.73 | [-16.53, +127.15]     | [-61.26, -13.06]         |
| kirk1                 |                    +89.83 | [+27.40, +165.33]     | [-11.38, +2.73]          |
| morg0                 |                    +56.76 | [+1.24, +128.62]      | [-4.97, +2.81]           |
| soma1-static          |                    +81.42 | [+30.61, +185.70]     | [-6.60, -0.35]           |
| soma3-static          |                    +53.83 | [+5.82, +127.47]      | [-4.18, +0.63]           |

Sensitivity and whole-track intervals were retained for all 29 cases. The table counts intervals
wholly above zero (proxy regression), wholly below zero (proxy improvement), and crossing zero.
Counts describe this corpus and the chosen resampling scheme; they are not a pooled gate.

| Resampling unit | Group    | Wholly above zero | Wholly below zero | Crosses zero |
| --------------- | -------- | ----------------: | ----------------: | -----------: |
| 30 s blocks     | core     |                19 |                 0 |            4 |
| 30 s blocks     | transfer |                 3 |                 0 |            3 |
| 60 s blocks     | core     |                19 |                 0 |            4 |
| 60 s blocks     | transfer |                 4 |                 0 |            2 |
| 120 s blocks    | core     |                20 |                 0 |            3 |
| 120 s blocks    | transfer |                 5 |                 0 |            1 |
| Whole tracks    | core     |                15 |                 0 |            8 |
| Whole tracks    | transfer |                 3 |                 0 |            3 |

Posterior XY proxy p99 is higher for A2 than B0 at 28 of 29 cases; Broadway–Gough is the one
lower point estimate. The primary interval is wholly above zero at 19 core sites and four transfer
sessions, with no interval wholly below zero. Several proxy regressions therefore coexist with
improved observation-geometry summaries. The measures use different positions and eligibility
populations; this observation cannot identify a physical error or its cause.

Raw observation-geometry rows are absent from the retained FrameBundle, and successful replay
databases were discarded. Observation-geometry block intervals cannot be recovered from this
compact evidence. Posterior proxy intervals cannot fill that gap. Retaining geometry rows must
be part of a future declared replay before database disposal.

## Tail anatomy and interpretation

Review covered shadow and A2 transitions, heading/range strata, and the ten largest posterior
proxy records per arm at all 29 cases. Transition groups overlap, and each arm's own p95 defines
its transition-tail threshold. Omitting a group's windows changes the population; it does not
intervene on the estimator. Face-stable range and heading strata form another selected population.
Sparse bins remain sparse. The following observations distinguish associations from hypotheses.

| Case               | Observation                                                                                                                                                                                                                                                                                                           | Interpretation to test                                                                                                                                                                                                |
| ------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 3rd-folsom         | A2 steady p99 0.31484 m versus shadow 0.25617 m; transition-stable subsets 0.06736 m/1,147 windows versus 0.08954 m/1,355. A2 lateral-face entry has 63 tail windows among 178 (35.39%); omitting these windows gives 0.14317 m. Shadow also has 69/192 (35.94%).                                                     | Face-entry/extent-transition contexts deserve inspection; a higher entry-tail fraction alone does not explain the A2 regression, since shadow's fraction is similar.                                                  |
| ashbury-downey     | Body regresses 23.74% while steady improves 11.00%. A2 transition-stable steady p99 rises from 0.06586 m/687 windows to 0.08736 m/587. A2 face-stable range 20–40 m has 0.07654 m/145 windows versus 0.02648 m/139.                                                                                                   | The body/steady population distinction matters. A blanket transition exclusion would miss a stable-subset/range association. Objects are not paired by identity.                                                      |
| pierce-haight      | Face-stable p99 regresses 80.20%, while full steady p99 improves 46.29%. A2 stable steady subset is 0.21247 m/187 windows versus 0.10735 m/177. Near-range face-stable p99 is 0.19819 m/215 versus 0.05725 m/175. Every heading bin has fewer than 100 windows per arm.                                               | A face-stability selection or near-range association is plausible; small-bin p99 and changing populations prevent causal attribution.                                                                                 |
| claren0            | Steady/body p99 regress 37.03%, yet stable steady subset falls from 0.08922 m/39 windows to 0.04311 m/30. The full steady populations have only 135/111 windows; face-stable populations have 58/51. Mean complete duration falls from 2.8042 s/47 intervals to 1.5958 s/48, with one left-censored interval per arm. | Sparse support, subset changes and tracker update gaps require joint inspection. Zero backward timestamps does not establish gap-free tracking; the duration loss is retained independently of a geometry hypothesis. |
| morg0              | Steady p99 rises from 0.21293 m to 0.30446 m. A2 lateral-face entry has 18 tail windows among 59 (30.51%); omission gives 0.13277 m. Stable subset p99 also rises from 0.10909 m/287 to 0.11640 m/233.                                                                                                                | Transition association is strong descriptively, but does not account for every stable-subset change.                                                                                                                  |
| embarcadero-bryant | Improving comparator: steady p99 falls from 0.19815 m to 0.14226 m; stable subset from 0.09425 m/4,379 to 0.08643 m/3,980. Lateral-entry tail fractions are still 38.05% shadow and 33.13% A2.                                                                                                                        | Face-entry tails also occur where A2 improves; their presence is not a sufficient failure explanation.                                                                                                                |

Lateral-face entry associates with large tails at 3rd–Folsom and Morg, but similar entry-tail
fractions occur in the improving Embarcadero–Bryant comparator. Ashbury's body/steady distinction
and Pierce's stable/range subsets prevent a universal transition explanation. At Claren,
stable-subset improvement coexists with full geometry regression and duration loss. Those
populations and update gaps need joint inspection before selecting a correction.

The largest ten A2 posterior proxy records span ten tracker UUIDs at 3rd–Folsom, five at Ashbury,
and six at Pierce. Their maxima are 1.15223, 1.65659, and 0.95060 metres respectively. They are
not a single UUID's repeated tail, but UUIDs do not establish distinct physical objects. Exact
UUIDs and capture timestamps stay in the local navigation evidence, not in this report. A proxy
record cannot reconstruct discarded observation geometry or visually establish a manoeuvre.

## What remains unverified

| Question                                | Evidence boundary                                                                                                                                                             |
| --------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Physical accuracy, identity, and recall | Reviewed packs exist; eligible physical references, alignment, and governance are unresolved. Counts and label-free residuals cannot answer these questions.                  |
| Untouched evaluation                    | Core and diagnostic sites are prior-exposed. Transfer prior exposure is unknown; F8 needs governed prior-use/split handling. Overlapping exports add no independent coverage. |
| Observation-geometry uncertainty        | Raw geometry rows were not retained; posterior XY proxy intervals are a different measure.                                                                                    |
| Pi performance                          | Mac Tracker.Update-only timing excludes other pipeline work; Pi and end-to-end performance are unverified.                                                                    |
| Transfer sensor pose                    | Static metadata is classification evidence; `warmup_static_verified` is false. Identity sensor transforms and unavailable measured compass extrinsics limit interpretation.   |
| Continuity                              | Update gaps and clamped predictions remain. Monotonic timestamps do not prove gap-free tracking or physical identity preservation.                                            |
| Candidate uncertainty calibration       | The tracked-arm uncertainty-report combination is unsupported; no pre-gate calibration claim follows.                                                                         |
| Repeat observation identity             | Omitted source/config/calibration/scoring observation fields remain missing, despite matching baseline/config bytes.                                                          |

These are limits of the completed campaign, not unfinished admitted jobs. Another same-build
replay cannot create governed references or establish a causal intervention.

## Frozen provenance and verification

The fetched main commit, measured source, and replay binary were frozen before comparison.
The binary was not rebuilt or refetched during measurement or finalisation. A separate decoder
performed offline extraction; its identity is recorded separately from the measured replay tool.

| Identity                        | Frozen value                                                       |
| ------------------------------- | ------------------------------------------------------------------ |
| Fetched main                    | `b687e89d5c8014e7fdf73a4d24d69906d86ee946`                         |
| Measured source                 | `a966ac476eb532fe980a6c9a2a226a08f5c37f87`                         |
| Measured replay binary SHA256   | `6aa1a03053ad2fe055eb71055b0ecb161b4dbd81bfffd18c8a9999e0f074a58f` |
| Separate offline decoder SHA256 | `950ac58d0d587de11c676ef963f7bdaaef66acada9725077efe879d6fbd4c92d` |
| Offline analysis source SHA256  | `9b635ab724fcf47430c6892e99b98815613af36f7a9d3828653ddb48bc0463f7` |

Core/transfer source, configuration, calibration, build, and scoring identities were reconciled;
all six transfer source hashes were frozen before scoring, and exact filtered case manifests were
validated before warm-up. Final verification checked 1,862 completed-artifact hashes, 36
analysis/report hashes, all 87 compressed trajectory hashes, both recovery partials, and the
frozen replay binary. All 123 baseline/config byte comparisons and confirmed-interval repeat
comparisons passed. All four admitted phase plans have completion markers.

The reusable change is the default-off `-campaign-metrics` option in the offline
`lidar-state-estimation-baseline` tool. Confirmed interval and Tracker.Update timing exports are
separate from deterministic tracking baselines. The warmed-PCAP equivalence test and serial PCAP
preflight passed before measurement; campaign unit tests passed again during report review.
Production tracking defaults remain unchanged.

After completion, the publication branch incorporated newer main commits for documentation and
repository safeguards. Those commits are not the measured source above. Original campaign
history and byte-exact evidence remain in the local archive; the Git report preserves the
findings without publishing raw outputs. Approximately 37.67 GiB was free at final verification,
above the 25 GiB reserve. No unrelated owners or source captures were modified. The owned
supervisor released its exclusive lock and the campaign monitor is paused.

## Ranked follow-up

1. Declare a separate tracker stage/CPU profiling build comparing B0, shadow, and A2 before
   choosing an optimisation.
2. Retain raw geometry rows and synchronised evidence in a governed paired replay. Declare object
   matching, eligibility, and block definitions before outputs. Inspect 3rd face-entry/extent
   contexts, Ashbury stable/range contexts, and Pierce near-range face-stable contexts alongside
   improving Embarcadero–Bryant before correction.
3. Investigate Claren continuity separately, including sparse update gaps, complete/censored
   intervals, and physical identity references.
4. Resolve physical references, alignment, review governance, and prior-use/split handling before
   physical scoring.
5. Evaluate any future frozen candidate across the retained regressions and an eligible independent
   evaluation set. Aggregate improvements and selected diagnostic arms cannot supply a passing
   population or untouched-confirmation claim.

The completed evidence supports keeping B0 and separating these future investigations. It does
not support promoting A2, selecting a diagnostic toggle, or correcting the estimator speculatively.
