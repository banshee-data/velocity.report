# Research briefing, October 5 to 12, 2026

- **Status:** Complete. No definition, benchmark kind or registered metric changed. Adopted: the
  primary SSAM user manual as the source for the 5 s PET screen, which answers the behaviour
  plan's question B8, and Waymo's ESV26-294 naturalistic and police-reported comparison as a
  cited source in the crash-data plan. Two open questions are logged.
- **Scope:** Roadside and infrastructure LiDAR perception, and the safety and behaviour literature
  the analytics cite, first listed or newly surfaced October 5 to 12, 2026. No third-party
  briefing was supplied. The window overlaps the October 11 briefing, which is still on its own
  branch (`claude/research-briefing-2026-10-11`) awaiting review; nothing it evaluated is
  repeated here.
- **Related:** [Behaviour analytics plan](../../../plans/lidar-behaviour-analytics-plan.md),
  [crash-data integration plan](../../../plans/platform-crash-data-integration-plan.md),
  [posted speed limits plan](../../../plans/posted-speed-limits-plan.md),
  [state estimation plan](../../../plans/lidar-state-estimation-plan.md),
  [physical-reference pilot](../physical-reference-pilot-kirk0-2026-10.md),
  [previous briefing on main](research-briefing-2026-10-10.md)

Paper and research-report citation keys are mapped in the [reference audit](reference-audit.md);
full entries are in [references.bib](../../../../data/maths/references.bib).

All three discovery routes were run, and the arXiv query API was searched directly for the first
time, because the search engines index no date. That route found what the others missed: four
preprints first listed in the window, of which two touch the project. A snow study shows that
LiDAR detection in heavy snow improves because objects return more points, so a comparison
across weather is confounded unless object point count is held. A third-party test programme
reports an open-source automated driving stack crashing about 1,000 times as often per mile as
Michigan's police-reported human rate, which reproduces from its numbers but is a simulated
rate. Among older work, the FHWA SSAM user manual answered from this session for the first time
and confirms the 5 s post-encroachment default the behaviour plan has carried as secondary since
it was written. A European Commission paper measures a commercial LiDAR tracker against RTK GNSS
on a motorway, and a Waymo conference paper finds a dash-camera naturalistic rate for pedestrian
collisions in New York City three times below the police-reported one, against its own
hypothesis.

## Answer

Nothing changes a definition, a threshold, a benchmark kind or a registered metric. One factual
question closes: the SSAM manual's Configuration tab shows a maximum TTC of 1.5 s and a maximum
PET of 5.0 s, and the manual describes those values as the defaults, so the behaviour plan's
Section 5.1 row now cites the primary and B8 is answered. The manual's map view also bins
conflicts at TTC of at most 0.5, 1.0 and 1.5 s, the same bands as `ttc_below_{1.5,1.0,0.5}`.

The Waymo comparison enters the crash-data plan's sources as evidence on the under-reporting
assumption, not as a multiplier. The motorway paper's tracker precision against RTK GNSS (0.83 m
position and 1.40 km/h speed, one standard deviation) is a reference point for a moving sensor,
not for this one, and it raises a question for the physical-reference work (Q1). The snow paper
raises another for any comparison across conditions (Q2). No other source changes anything.

## Genuinely new this week

| Source                                                                                           | First listed       | Status       |
| ------------------------------------------------------------------------------------------------ | ------------------ | ------------ |
| Yang and Bos, snow and LiDAR object detection, arXiv 2610.13891                                  | October 9          | Read         |
| Liu et al., Mcity behavioural safety assessment, Part II, arXiv 2610.08458                       | October 6          | Read in part |
| Liu, Zhou et al., percentile control for highway scenario generation, arXiv 2610.05003           | October 6 (listed) | Read in part |
| IIHS and HLDI, _Safety Solution: speed safety cameras_, states map "as of October 2026"          | October 2026       | Read         |
| AAA Foundation analysis of unbelted occupants in FARS, 2017 to 2022 (news coverage of October 8) | October 8          | **Confirm**  |

The scenario-generation paper uses minimum post-encroachment time on highD as the risk scale for
a diffusion model that generates simulated futures; it measures nothing on a road and nothing
here forecasts, so it is recorded and not evaluated further. The seat-belt analysis concerns
occupant restraint, which no roadside sensor observes; it is out of scope and was seen only
through news coverage.

## Newly surfaced older work

| Source                                                                                           | First listed        | Why it surfaced this week                                              |
| ------------------------------------------------------------------------------------------------ | ------------------- | ---------------------------------------------------------------------- |
| Cellina et al., public-road testing of assisted lane change with LiDAR, arXiv 2608.26669         | August 2026         | Headway topic query; read in full                                      |
| FHWA, _SSAM: Software User Manual_, FHWA-HRT-08-050                                              | 2008                | Primary newly reachable; the behaviour plan's B8                       |
| Campolettano, Scanlon and Kusano, VRU benchmarks from naturalistic and police data, ESV26-294    | 2026 (ESV, Toronto) | Waymo research page; read in full                                      |
| AAA Foundation, 2026 Safe Mobility Conference proceedings                                        | September 2026      | **Confirm** on October 11; read in part this week                      |
| Post-encroachment thresholds at unsignalised intersections, Patna (Springer, s44465-026-00015-2) | May 2026            | PET topic query; Springer served a script challenge                    |
| Gao and Luo, TACTIC, roadside attacks on a vehicle's LiDAR, arXiv 2609.39969                     | September 30, 2026  | arXiv API; abstract read; the attacked sensor is a vehicle's, not ours |

The proceedings were read for the sessions in scope. One abstract, Campolettano's on speed-limit
compliance, is the paper the crash-data plan already cites
([Campolettano, Kusano and Victor 2025](../../../plans/platform-crash-data-integration-plan.md#sources-checked)),
and its figures match the plan's row. Two abstracts describe speed-camera and probe-data
evaluations in San Francisco, Detroit and Bellevue that report shares of vehicles over the limit
and the 85th percentile speed; neither has published results to evaluate.

The Waymo research page also lists four 2026 entries the repository does not hold and this week
did not evaluate: Scanlon et al., _Building scientific consensus on the crash safety performance
of automated driving systems_ (ESV26-252); Scanlon et al., SAE 2026-01-0520, on collision
avoidance against a human reference model in reconstructed fatal collisions; and two
interaction-modelling preprints, arXiv 2604.19838 and 2604.27168. None measures a roadside
quantity; ESV26-252 is the one a later crash-data review should read.

## Yang and Bos: snow and LiDAR object detection (arXiv 2610.13891)

1. **Measures.** Average precision of five learned 3D detectors (SECOND, Voxel R-CNN,
   PointPillars, PV-RCNN, Part-A2) for cars on two snowy datasets: WADS-3D, 4,100 labelled frames
   from 2,050 scans by a vehicle-mounted Ouster OS1-64 in Michigan's Upper Peninsula, released with
   the paper; and CADC, 7,000 frames from a Velodyne VLP-32C. Frames are binned by snowfall
   intensity, measured as the count of points a two-stage filter (DSOR, then LIOR) calls snow, with
   quantile bins of equal car counts. The detectors were trained on clear-weather KITTI and, for
   CADC, fine-tuned on a 50:50 split. The finding is that AP rises with snowfall intensity, and
   that the rise follows object point count, not the snow itself. It is not a measurement of
   tracking, extent or speed, and nothing in it is roadside.
2. **Maths.** The Pearson correlations between snowfall bin and mean object point count (r = 0.820,
   p = 3.70 × 10^-3 on WADS-3D; r = 0.733, p = 1.86 × 10^-3 on CADC) reproduce from the t
   statistic r √(n − 2) / √(1 − r²) only with n = 10 and n = 15: the unit is the bin, not the
   object. They are correlations across ten and fifteen bin means, an ecological association that
   says nothing about how point count and detection relate for one car. The paper's mechanism,
   accumulated snow enlarging the body and turning a specular surface diffuse, is stated as
   hypotheses it has not tested. The dominant uncertainty is that confound itself: the paper shows
   that cars in heavy-snow bins differ (more points, farther away, lower intensity) and cannot
   separate why.
3. **Sensor.** The lesson transfers; the numbers do not. A static Pandar40P sees falling snow as
   transient returns, which L3 must keep out of the foreground, and sees a snow-covered car as a
   larger, more diffuse body, which changes the face and extent evidence the solid-body work
   relies on. Neither was measured here: the detectors are learned and vehicle-mounted.
4. **Lands.** `grep` finds snow only as a cause of sensor blockage in the
   [L7 scene plan](../../../plans/lidar-l7-scene-plan.md). The state estimation plan records
   "dry conditions" as a limit of its captures. No plan compares L3 or L4 across weather.
5. **Verdict.**

   | Claim or recommendation                                               | Verdict                                                                                 |
   | --------------------------------------------------------------------- | --------------------------------------------------------------------------------------- |
   | Detection improves with snowfall because objects return more points   | Not adopted: an ecological correlation over bins, mechanism untested, learned detectors |
   | Weather bins without object-level covariates mislead about robustness | New; logged as Q2 for any cross-condition comparison of L3 or L4                        |
   | WADS-3D as an evaluation set                                          | Not adopted: vehicle-mounted, forward-view crops, no roadside geometry                  |

6. **Rights.** arXiv non-exclusive distribution licence. The WADS-3D licence was not read
   (**Confirm**).
7. **Give back.** A site that records through a snowfall could test the hypothesis on its own
   terms: point count per passage and L3 foreground fraction against a snow-return count, with
   the car's extent estimate as the outcome. No capture with snow exists in the corpus.

## Cellina et al.: public-road testing of assisted lane change using LiDAR (arXiv 2608.26669)

1. **Measures.** Whether a production car's assisted lane change respects UNECE Regulation No. 79's
   critical distance on a French motorway (A31, March 2026): 27 manoeuvres at 100 to 130 km/h,
   triggered at 20 to 60 m, 18 completed and 9 suppressed. The test vehicles carried a commercial
   LiDAR tracking system (Ouster OS1-128, 10 Hz output of position, velocity, heading, width and
   length) and RTK GNSS receivers as ground truth.
   Lane-crossing time was annotated by hand from a camera at one-second resolution. It measures one
   vehicle's automation, not drivers and not traffic.
2. **Maths.** The regulation's critical distance is
   S_crit = (v_app − v_ALC) t_B + (v_app − v_ALC)² / (2a) + v_ALC t_G, with a = 3 m/s², t_B = 0.4 s
   and t_G = 1 s. At 100 km/h it gives 27.8 m with no speed difference, 30.2 m at 10 km/h and
   42.7 m at 30 km/h; at 130 km/h, 36.1 m to 51.0 m. Its slope in the speed difference is t_B +
   Δv/a, at least 0.4 s, so the curve is flattest, not flat, at small Δv, where the paper's six
   overshoots sit. There the time-gap term dominates, and the 1 s lane-crossing annotation moves
   the measured gap by Δv times the timing error; at a few metres per second that is the same order as
   the tracker's 0.83 m. The paper claims "3-sigma or 99% confidence" for three cases: three
   sigma is 99.7%, so the two statements differ, and the case list names 16d in the discussion and
   16c in the results. The precision figures rest on the 2.3% of the record where both vehicles
   held an RTK integer fix at once: that small, selected subset is the dominant uncertainty.
3. **Sensor.** A roadside sensor cannot see whether a lane change was automated and has no use for
   the regulation's critical distance as a band: it is a type-approval requirement on a system,
   not a rule for drivers, and the behaviour plan already keeps time-headway bands
   `no_established_threshold`. What transfers is the method: a tracker scored against RTK GNSS on a
   moving test vehicle.
4. **Lands.** The state estimation plan says plainly that there is "no instrumented probe vehicle,
   no survey, no external reference trajectory"; the physical-reference pilot scores fitted
   references drawn from the sensor's own returns. An RTK-equipped probe vehicle is the external
   reference neither has. For a static sensor only the probe needs a fix, since the sensor is
   surveyed once, which avoids the simultaneous-fix problem that cut this paper's ground truth to
   2.3%.
5. **Verdict.**

   | Claim or recommendation                                    | Verdict                                                                                  |
   | ---------------------------------------------------------- | ---------------------------------------------------------------------------------------- |
   | LiDAR tracking is precise enough for regulatory gap checks | Not adopted: one vehicle, a moving sensor, 2.3% fix availability; no roadside claim      |
   | R79's 1 s time gap as a following band                     | Not adopted: a requirement on an automated system; time-headway bands stay unbenchmarked |
   | RTK GNSS on a probe vehicle as an external reference       | New; logged as Q1 for the physical-reference work                                        |

6. **Rights.** The arXiv listing carries CC BY 4.0, so figures may be reproduced with credit.
7. **Give back.** A probe vehicle with an RTK receiver driven past a site at known speeds and gaps
   would give the first external check on L5 speed and the following gap; the pilot's fitted
   references would then have something to be scored against.

## FHWA: SSAM software user manual (FHWA-HRT-08-050)

1. **Measures.** The manual for the Surrogate Safety Assessment Model, which screens simulated
   trajectory files for conflicts. Its appendix defines PET as "the time between when the first
   vehicle last occupied a position and the second vehicle subsequently arrived at the same
   position", zero being a collision, and TTC as the minimum time to collision "based on the
   current location, speed, and trajectory of two vehicles at a given instant".
2. **Maths.** None to re-derive. The values are the point: the Configuration tab "configured with
   Maximum TTC value to be 1.5 and Maximum post-encroachment to be 5.0" (Figure 20), the text
   says the example leaves "default maximum TTC and maximum PET values" unchanged (Figure 24), and
   the map view groups conflicts at TTC of at most 0.5, 1.0 and 1.5 s. The values come from the
   figures' descriptions and the text; the screenshots were not viewed, which is why the plan
   says "as the manual describes it".
3. **Sensor.** Unchanged from the plan: a screen built for simulated trajectories, extended to
   measured ones with that said wherever an SSAM number appears.
4. **Lands.** The behaviour plan's Section 5.1 cited the 5 s PET default "as reported in the
   secondary literature", its references listed the manual as "not yet consulted directly", and
   question B8 asked whether the value appears in it.
5. **Verdict.**

   | Claim                                      | Verdict                                                                    |
   | ------------------------------------------ | -------------------------------------------------------------------------- |
   | Maximum PET defaults to 5.0 s              | Adopted as the primary citation; Section 5.1, its note and B8 updated      |
   | Maximum TTC defaults to 1.5 s              | Already holds: Section 5.1 cites the TechBrief and validation report       |
   | Map-view bands at TTC ≤ 0.5, 1.0 and 1.5 s | Already holds: `ttc_below_{1.5,1.0,0.5}`; the manual added as their source |

6. **Rights.** A US government publication.
7. **Give back.** Nothing new; `pet` and `pet_min` remain continuous measurements.

## Campolettano, Scanlon and Kusano: VRU collision benchmarks from naturalistic and police data (ESV26-294)

1. **Measures.** Vehicle-level collision rates per million miles with pedestrians, cyclists and
   motorcyclists on New York City surface streets, from two sources. One is Nexar dash-camera
   fleet data: 674 flagged events, 592 verified contacts and 509 with injury potential from July
   2020 to October 2024, of which 104 contacts (96 with injury potential) fall in the two years
   with mileage, against 92.9 million surface-street miles, reweighted by borough to FHWA shares.
   The other is New York State police reports for 2022, all passenger vehicles in transport,
   against 9,494.6 million surface-street miles. Collision speed is reconstructed from video and GPS, with
   side contacts scaled down for partial engagement.
2. **Maths.** The police-reported rates reproduce: 8,501.58 / 9,494.58 = 0.895, 4,239.95 / 9,494.58
   = 0.447 and 1,610.95 / 9,494.58 = 0.170 per million miles, and Byar's approximation on those
   counts returns the published intervals to the third decimal. The counts are fractional, so they
   are weighted; Byar's method treats them as Poisson counts all the same. The naturalistic
   intervals do not reproduce from the method the paper names: 0.27 per million miles over 92.9
   million miles implies about 25 events, whose Byar interval is about 0.17 to 0.40, where the
   paper prints 0.08 to 0.53; cyclists and motorcyclists widen the same way. The paper does not say
   what widens them, plausibly the borough reweighting with factors up to 5.35. The scale factors
   in Table 4 reproduce from Table 3. The text's "nearly 1.4 billion miles" of 2022 driving
   matches no figure in Table 3 (18,398.3 million in all, 9,494.6 million on surface streets).
   The dominant uncertainty is who the fleet is: rideshare, fleet and volunteer cameras, with
   Queens over-represented in its mileage and Staten Island at 2%.
3. **Sensor.** None of it is observable from the roadside. It bears on the crash-data plan's
   rates, whose police-reported baselines this paper tests against the assumption that a
   naturalistic source recovers the missing low-severity events.
4. **Lands.** The crash-data plan cites Valgo's white paper for Blincoe under-reporting
   multipliers and the Waymo papers for police-reported baselines; `grep` finds no naturalistic
   VRU rate and no test of the under-reporting assumption.
5. **Verdict.**

   | Claim                                                   | Verdict                                                                            |
   | ------------------------------------------------------- | ---------------------------------------------------------------------------------- |
   | Police-reported VRU rates, NYC 2022, with intervals     | Adopted as a cited source row in the crash-data plan; not a baseline the plan uses |
   | Naturalistic rates exceed police-reported ones for VRUs | Not supported by the paper itself: lower for pedestrians, inconclusive otherwise   |
   | A 10 mph collision-speed threshold as a severity cut    | Not adopted: a postural-stability heuristic, not an injury outcome                 |

6. **Rights.** No statement was found in the PDF; the plan quotes figures with credit only.
7. **Give back.** Nothing a site can test: the sensor records no collisions.

## Liu et al.: Mcity behavioural safety assessment, Part II (arXiv 2610.08458)

Read for its rate and method. An open-source automated driving stack (Autoware.Universe) was run
in a simulated naturalistic and adversarial environment of the Mcity test facility, which
amplifies safety-critical events and reweights them, and on the physical track. Its estimated
3.01 × 10^-3 crashes per mile against Michigan's police-reported 3.00 × 10^-6 is a ratio of 1,003,
which is the paper's "approximately 1,000 times". The environment's own simulated human rate,
2.93 × 10^-6, is the paper's evidence of fidelity. The miles are simulated, the human denominator
is a state's police-reported crashes per vehicle-mile without an under-reporting adjustment, and
the stack is not a deployed fleet. Not adopted: it is a test-programme rate, not a road exposure
rate, and the crash-data plan's sources compare only rates on public roads. Rights: arXiv
non-exclusive distribution licence.

## IIHS and HLDI: Safety Solution sheet on speed safety cameras (October 2026)

Read in full. A two-page summary for policy makers, its states map dated "as of October 2026":
26 states and the District of Columbia, more than 350 communities. It restates published figures
with references: 29% of 2024 traffic deaths in speeding-related crashes (NHTSA, already read in
full by the attitudes plan); reductions of 70% or more in the share of vehicles more than 10 mph
over the limit after cameras on an Arizona freeway, Maryland residential roads and District of
Columbia streets (Retting et al. 2008 and others); 39% lower odds of incapacitating or fatal
injury in Montgomery County. The share-of-vehicles measure is the same kind of quantity as the
behaviour plan's `speeding_exposure_10`, but in mph and from spot speed studies, and the plan
does not cite before-and-after effects of enforcement. Not adopted: a secondary summary of older
studies; the primaries it cites would be the sources if the posted speed limits plan ever needs
an enforcement context paragraph. Rights: no reuse terms stated.

## Sources seen only as summaries

Each is **Confirm**; no figure below was read in a primary.

- **Post-encroachment thresholds at six unsignalised intersections in Patna, India (Springer,
  May 2026).** Search summary: a 6 s PET threshold for critical conflicts in heterogeneous
  traffic, citing earlier values of 3 to 6 s. The behaviour plan's Section 5.1 already says PET
  thresholds "vary substantially by site type and road user". Not adopted unread.
- **NSC Injury Facts, preliminary motor-vehicle deaths, first half of 2026.** Search summary:
  1.12 deaths per 100 million vehicle-miles. A preliminary estimate the crash-data plan has no use
  for while FARS is its fatal source; the host refused the proxy.
- **Nationwide insurer questionnaire, 2026.** Perceptions of other road users' conduct; a
  questionnaire, already listed in the January 2025 to October 2026 briefing.

## What changes

In this change:

- [Behaviour analytics plan](../../../plans/lidar-behaviour-analytics-plan.md): the Section 5.1
  PET row cites the SSAM user manual directly; the note below the table no longer calls the value
  unconfirmed; B8 is marked answered; the manual's reference entry records what was read and the
  TTC map-view bands; this briefing is linked under "Weekly literature review".
- [Crash-data plan](../../../plans/platform-crash-data-integration-plan.md): one row in "Sources
  checked" for ESV26-294, with its link.
- A devlog bullet.

Nothing in code, the metrics registry, benchmark kinds or definitions.

## Open questions

| ID  | Question                                                                                                                                                                                                                                                                                                                            |
| --- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Q1  | Should the physical-reference work add an RTK-equipped probe vehicle, driven past a surveyed static sensor at known speeds and gaps, as the external reference the state estimation plan says is missing? Cellina et al. had both vehicles fixed at once for 2.3% of a motorway record; a static sensor needs only the probe's fix. |
| Q2  | Before any comparison of L3 foreground or L4 detection across weather or season, should object point count per passage be recorded as a covariate, so that a change in returns is not read as a change in conditions? No capture in the corpus has snow; the question gates nothing today.                                          |

The October 11 briefing's three questions remain open on its own branch and are not repeated.

## Sources checked

| Source                                                                   | Status                                                          |
| ------------------------------------------------------------------------ | --------------------------------------------------------------- |
| arXiv 2610.13891, Yang and Bos                                           | Read                                                            |
| arXiv 2608.26669, Cellina et al.                                         | Read                                                            |
| FHWA-HRT-08-050, SSAM software user manual (HTML)                        | Read in part (Configuration, Filter, Map and appendix)          |
| ESV26-294, Campolettano, Scanlon and Kusano                              | Read                                                            |
| arXiv 2610.08458, Liu et al., Part II                                    | Read in part (abstract, method and results)                     |
| IIHS and HLDI, _Safety Solution: speed safety cameras_                   | Read                                                            |
| AAA Foundation, 2026 Safe Mobility Conference proceedings                | Read in part (surrogate-measure and speed-management sessions)  |
| Waymo safety research page                                               | Read; ESV26-294 taken up; four other 2026 entries not evaluated |
| PyPI `humanbaselines` release list                                       | Read; 0.4.0 of October 1 is still the latest                    |
| arXiv 2610.05003, percentile control for scenario generation             | Read in part (abstract)                                         |
| arXiv 2609.39969, TACTIC                                                 | Read in part (abstract)                                         |
| Springer s44465-026-00015-2, Patna PET thresholds                        | **Confirm** (script challenge)                                  |
| NSC Injury Facts, preliminary estimates, first half of 2026              | **Confirm** (refused by the proxy)                              |
| AAA Foundation unbelted-occupant analysis, October 2026                  | **Confirm** (news coverage only; host refused)                  |
| WADS-3D dataset licence                                                  | **Confirm**                                                     |
| PubMed 42508180, pre-crash trajectory case-control (October 11 To fetch) | **Confirm** (cookie wall again)                                 |
| HighwayScene project page (October 11 To fetch)                          | **Confirm** (refused by the proxy)                              |
| ERSO thematic report on speed, 2026 PDF (October 11 To fetch)            | **Confirm** (refused by the proxy)                              |
| Sensors 26(11):3529 (October 11 To fetch)                                | **Confirm** (403)                                               |

## Provenance

Searched October 12, 2026, extended mode unless noted: roadside LiDAR tracking arXiv October 2026;
Waymo safety research publication October 2026; AAA Foundation for Traffic Safety new report
October 2026; IIHS speeding study October 2026; human crash baseline automated driving 2026
preprint; following distance headway LiDAR measurement 2026; infrastructure LiDAR vehicle
detection preprint arXiv 2610; post-encroachment time threshold study 2026; 85th percentile speed
limit setting practice 2026 study; pedestrian impact speed fatality risk curve 2026; stop sign
compliance LiDAR 2026; speeding attitudes national survey 2026 aggressive driving; FMCSA following
too closely headway research project 2026; vehicle trajectory dataset released 2026 drone LiDAR
new; speed change crash risk model Accident Analysis and Prevention 2026; SWOV ITF OECD road
safety speed report October 2026; crash involvement rate per vehicle mile baseline 2026 preprint;
arXiv roadside LiDAR preprint October 2026 multi-object tracking infrastructure point cloud; and,
in standard mode, AAA Foundation seat belt unbelted fatalities FARS 2017-2022 report October 2026.
The `preprint` variants of the topics were folded into the arXiv API route below rather than run as
separate web queries.

arXiv query API (export.arxiv.org), sorted by submission date, results from September 25 on:
"roadside lidar", "infrastructure lidar", lidar and headway, "time to collision" and lidar,
"post-encroachment", "crash rate" and "automated driving", speeding and "speed limit". Five hits
in or near the window; two relevant.

Read with `curl` through the proxy into scratch directories and converted with `pdftotext`: arXiv
PDFs for 2608.26669, 2610.13891 and 2610.08458; the arXiv API records for 2610.05003 and
2609.39969; arXiv abstract pages for licences; the IIHS sheet; the NHTSA-hosted ESV26-294 PDF; the
AAA Foundation proceedings page and PDF; the FHWA SSAM manual page; the Waymo research page; the
PyPI JSON for `humanbaselines`. Refused: link.springer.com (script challenge), injuryfacts.nsc.org,
highwayscene.github.io, road-safety.transport.ec.europa.eu and contracosta.news (proxy 403),
mdpi.com (403), pubmed.ncbi.nlm.nih.gov (cookie wall).

## To fetch

- https://link.springer.com/article/10.1007/s44465-026-00015-2 (Patna PET thresholds)
- https://digitalcommons.mtu.edu/wads-3d/ (WADS-3D licence)
- https://injuryfacts.nsc.org/motor-vehicle/overview/preliminary-estimates/
- https://www.fhwa.dot.gov/publications/research/safety/08050/ figures 20 and 24 as images, to see
  the default thresholds rather than their descriptions
- From the October 11 list, still unread: https://pubmed.ncbi.nlm.nih.gov/42508180/,
  https://doi.org/10.1177/03611981251327216, https://doi.org/10.3390/s26113529,
  https://highwayscene.github.io/ and
  https://road-safety.transport.ec.europa.eu/document/download/9826c063-bc55-423e-84a3-24200dca3547_en?filename=ERSO-TR-speed_2026.pdf
