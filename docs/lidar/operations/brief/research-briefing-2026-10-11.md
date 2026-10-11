# Research briefing, October 4 to 11, 2026

- **Status:** Complete. No definition, benchmark kind or registered metric changed; nothing new
  adopted beyond two reference notes in the behaviour plan. Two papers the October 10 briefing
  could not reach were read in full this week, which settles step 0 of its L3 proposal for the
  paper and leaves the dataset licence open.
- **Scope:** Roadside and infrastructure LiDAR perception, and the safety and behaviour literature
  the analytics cite, first listed or newly surfaced October 4 to 11, 2026. No third-party
  briefing was supplied.
- **Related:** [Behaviour analytics plan](../../../plans/lidar-behaviour-analytics-plan.md),
  [crash-data integration plan](../../../plans/platform-crash-data-integration-plan.md),
  [posted speed limits plan](../../../plans/posted-speed-limits-plan.md),
  [background-grid maths](../../../../data/maths/background-grid-settling-maths.md),
  [previous briefing](research-briefing-2026-10-10.md)

Paper and research-report citation keys are mapped in the [reference audit](reference-audit.md);
full entries are in [references.bib](../../../../data/maths/references.bib).

The week was searched along all three routes. Nothing in scope was first published inside the
window that the searches could find: no Waymo, AAA Foundation, IIHS, SWOV or ITF release dated
October 4 to 11, and no roadside LiDAR preprint with a 2610 identifier. What changed is reach.
arxiv.org, aaafoundation.org and pypi.org answered from this session, so the beam-wise background
subtraction benchmark and PRISA, both **Confirm** on October 10, were read in full, and the AAA
Foundation's American Driving Survey for 2025 (September 2026), newly surfaced, was read in full.
The benchmark's figures reproduce from its own precision and recall; PRISA's do too, and its
time-to-collision formula, as printed, gives a finite value to an opening gap. Neither changes a
definition here, because the behaviour plan already defines time to collision on a signed closing
speed. Four journal papers surfaced that could not be fetched and are listed under "To fetch".

## Answer

Nothing changes a definition, a benchmark kind or a registered metric. The background
subtraction paper answers the October 10 proposal's step 0: its labels, splits and metrics are now
known, and its protocol trains a model for about a minute and then freezes it, which L3 never does.
That difference is the first thing a scoring run must decide, logged as Q1. PRISA's 1.5 s
operating point and its time-to-collision form are both already handled in the behaviour plan, and
its formula as printed is a reason to keep the plan's signed definition, not to change it. The
driving survey is a self-reported exposure estimate for drivers, not a vehicle-miles denominator,
and does not enter the crash-data plan's rate algebra.

## Genuinely new this week

Nothing in scope was found first published October 4 to 11. The nearest items by date, each from
search summaries only:

1. **Astoria, Oregon police acquire a handheld LiDAR speed unit (news, October 6).** An
   enforcement purchase, not a measurement claim. Recorded so it is not rediscovered.
2. **Valgo `humanbaselines`: no release since 0.4.0 of October 1.** Checked against the PyPI
   release list (read); the crash-data plan already records 0.4.0.

## Newly surfaced older work

| Source                                                                                     | First listed       | Why it surfaced this week                            |
| ------------------------------------------------------------------------------------------ | ------------------ | ---------------------------------------------------- |
| Baumann, Vosshans and Dang, beam-wise background subtraction, arXiv 2608.14868 (ITSC 2026) | August 14, 2026    | Primary newly reachable; read in full                |
| Bang et al., PRISA, arXiv 2607.16156 (ITSC 2026)                                           | July 17, 2026      | Primary newly reachable; read in full                |
| AAA Foundation, American Driving Survey: 2025 (Hungund and Steinbach)                      | September 2026     | Organisation watchlist; read in full                 |
| Matched case-control study of pre-crash trajectories, Accident Analysis and Prevention     | July 27, 2026 epub | Speed-change query; PubMed refused                   |
| Dean, Doerzaph and Riexinger, injury modification factors for speed limits, TRR            | 2026               | Speed-change query; SAGE refused                     |
| Occlusion-predictive network for roadside multi-vehicle tracking, Sensors 26(11):3529      | 2026               | Occlusion query; MDPI refused                        |
| AAA Foundation, 2026 Safe Mobility Conference proceedings                                  | September 2026     | Organisation watchlist; summary only                 |
| Agrawal et al., hardware-triggered LiDAR and camera synchronisation, arXiv 2607.15889      | July 17, 2026      | Headway query; abstract read; out of scope (Tenet 1) |
| European Commission ERSO thematic report, speed and speeding (with SWOV)                   | 2025 or 2026       | Organisation watchlist; summary only                 |

## Baumann, Vosshans and Dang: beam-wise statistical background subtraction (arXiv 2608.14868)

1. **Measures.** Point-wise static or dynamic classification by a background model trained on
   unlabelled frames from a fixed sensor, scored as precision, recall and F-score on dynamic
   points. Four models: a density-aware coarse-fine triangle threshold, a quantile-constrained
   Gaussian (median and median absolute deviation over the far quantile of each beam), a per-beam
   Gaussian mixture with one to three components chosen by BIC, and OpenCV's MOG2 on a range image
   as baseline. Each is followed by a spatial consistency filter that keeps a foreground point only
   if enough neighbours are also foreground: a range-adaptive 3D radius, or a window on the angular
   grid. Two datasets. HighwayScene: a 40 km/h free-flowing highway construction site, 5,997
   frames at 10 Hz from an Ouster OS0, an Aeva Aeries II (FMCW) and a Blickfeld QB2 (5 Hz), split
   4,000 train, 1,000 validation, 997 test; labels are lane volumes for the Ouster and Blickfeld
   and radial velocity above 1 m/s for the Aeva. CoopScenes: four urban scenes from an
   infrastructure Ouster OS2, 76 hand-annotated frames with 2,455 objects of which 857 dynamic,
   mapped to point labels. Models train on 600 frames, about one minute; the paper says more frames
   gave no gain. MOG2's learning rate is fixed at zero after training, to match the others. It is
   not a measurement of detection, tracking, extent or speed.
2. **Maths.** Every F2 and F1 in Tables I and III was recomputed from its own precision and recall
   with F_beta = (1 + beta^2) P R / (beta^2 P + R); all reproduce to within 0.01 points. The
   paper's statement that the unimodal methods sit 0.18 to 3.19 F1 points below the mixture on
   HighwayScene after filtering also reproduces (Ouster 99.43 against 99.25, Aeva 97.59 against
   94.40). Two points the paper's own numbers make. First, the filter can only remove foreground,
   so recall cannot rise after it; that is why models are tuned on F2 and filters on F1. Second,
   the mixture does not dominate in the urban scenes: on CoopScenes Scene 1 its recall is 77.93%,
   the lowest of the five methods, and the density-aware threshold is the most consistent.
   The dominant uncertainty is label construction. Lane-volume labels call every return inside a
   lane dynamic, which holds only because no vehicle stops on that highway; velocity labels miss
   anything moving below 1 m/s.
3. **Sensor.** Fully within what a LiDAR sees; no identity. L3 differs from the benchmark's models
   in three ways that matter for any score. L3 is online: an EMA mean and spread per cell, a slow
   locked baseline, freezes and a deadlock breaker
   ([background-grid maths](../../../../data/maths/background-grid-settling-maths.md), Sections 6
   to 8), where the benchmark trains and freezes. L3's neighbour step asks whether a
   same-ring neighbour's background mean explains the observed range and, if enough do, calls the
   point background (Section 5); the paper's filter asks whether neighbouring points are also
   foreground. Both move points from foreground to background, so both can only raise precision,
   but they test different evidence, and the paper's best grid window spans 2 vertical and 24
   horizontal neighbours where L3 votes along the ring only (Section 11, limit 5). And L3 is
   unimodal per cell (limit 1), which the paper's highway results suggest costs little and its
   urban results do not settle. The paper names the same failure L3's freeze and relearning exist
   for: a vehicle present in training and absent at test left shadowed regions in Scene 3.
4. **Lands.** Already cited in the behaviour plan's roadside perception references and carried by
   the backlog item "Score L3 foreground extraction against point-wise labels (D-28)", designed in
   the
   [October 10 proposal](research-briefing-2026-10-10.md#proposal-score-l3-against-point-wise-labels).
   The proposal's sensor table can now be filled: the CoopScenes labels are from an Ouster OS2, a
   rotating sensor that maps to (ring, azimuth bin); the Aeva's labels come from its own velocity
   channel, which the paper excludes from every model.
5. **Verdict.**

   | Claim or recommendation                                      | Verdict                                                                                                     |
   | ------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------- |
   | Per-beam statistical models suit static roadside LiDAR       | Already holds: L3's (ring, azimuth bin) grid, background-grid maths Sections 1 to 3                         |
   | Spatial consistency filtering is essential for precision     | Already holds in kind (Section 5); the cross-elevation window is new, logged as Q2                          |
   | Multimodal per-beam mixtures                                 | Not adopted: the urban results do not show a consistent gain; limit 1 stays recorded                        |
   | Train-then-freeze protocol on about one minute of data       | New; it decides how L3 is scored, logged as Q1                                                              |
   | Released CoopScenes point labels as an external check for L3 | Already planned (D-28 backlog item); step 0 is done for the paper, the dataset licence is still **Confirm** |

6. **Rights.** The arXiv listing carries the arXiv non-exclusive distribution licence, and the PDF
   says: "© 2026 IEEE. Personal use of this material is permitted. Permission from IEEE must be
   obtained for all other uses". The briefing quotes figures with credit and reproduces no table.
   The HighwayScene project page (highwayscene.github.io) did not resolve, so the dataset licence,
   reported as CC BY-NC-SA 4.0 on October 10, remains **Confirm**.
7. **Give back.** A deployment cannot add a sensor to the benchmark, but a full-scene labelled
   Pandar40P capture would be the first residential-street, 40-beam entry, and the October 10
   proposal's step 3 already describes it.

## Bang et al.: PRISA (arXiv 2607.16156)

1. **Measures.** Two Ouster sensors 5 m up at opposite corners of a signalised intersection in
   Chattanooga, fused by the vendor's BlueCity backend at 10 Hz and downsampled to 5 Hz, plus the
   public R-LiViT intersections. A learned predictor (FlowChain) forecasts 2.4 s from 1.6 s of
   history. Following pairs (heading difference under 30 degrees, leader ahead, inside a corridor
   at least 3.66 m wide) are scored by time to collision from observed states; crossing, merging
   and vulnerable-road-user pairs are scored by predicted post-encroachment time from forecast
   boxes. Latency: 21 ms perception, 143 ms prediction, 30 ms scoring, 194 ms in all on an
   NVIDIA Jetson AGX Thor. "Conflict rate" is conflict instances divided by evaluation frames, so a
   pair that stays under threshold for one second counts five times.
2. **Maths.** Equation 6 defines the closing speed as the absolute difference of scalar speeds,
   |v_f - v_l|, and TTC as gap over that whenever it is positive. The text says a finite value
   means the follower is approaching, which holds only for the signed difference; as printed, a
   follower slower than its leader, with the gap opening, also gets a finite TTC and can be
   flagged. Whether the code does the same was not checked. The Table III rates reproduce from
   their counts against constant denominators: about 1,050 frames at R-LiViT Int. 2, about 1,200 at
   Int. 3 and about 3,000 (ten minutes at 5 Hz) at the field site, so a rate above 100% is several
   simultaneous pairs, as the paper says, not an error. Predicted post-encroachment counts are
   nearly flat from 0.5 s to 2.4 s (597 to 639 at Int. 2), and the paper notes that a pair that
   never clears the zone inside the horizon is assigned zero, so the low end of the distribution is
   partly an artefact of the horizon.
3. **Sensor.** Time to collision from observed positions and speeds is within reach for following
   pairs when both are in view; a predicted post-encroachment time is a forecast, not an
   observation. The vendor fusion and the edge GPU are outside the Raspberry Pi envelope, and
   nothing here needs identity.
4. **Lands.** The behaviour plan defines `ttc` as `gap / closing` for closing above zero only
   (Section 8.4), keeps the longitudinal formula to following pairs (the interaction-type table),
   and holds SSAM's 1.5 s as a `research_threshold` row. PRISA's 1.5 s is already noted in the
   plan's references as an operating point, not a severity finding.
5. **Verdict.**

   | Claim or recommendation                         | Verdict                                                                                               |
   | ----------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
   | TTC for following pairs, PET for crossings      | Already holds: behaviour plan, interaction-type table and Section 8.4                                 |
   | TTC on the absolute speed difference            | Not adopted: the plan's signed closing speed stands; noted beside the plan's reference to PRISA       |
   | 1.5 s operating point for TTC and predicted PET | Already holds: SSAM row for TTC; PRISA's own value stays out of Section 5.1                           |
   | Conflict rate as instances per frame            | Not adopted: duration below a band (`ttc_below_*`) and one minimum per encounter are the plan's forms |
   | Predicted PET from learned forecasts            | Not adopted: nothing in the roadmap forecasts                                                         |

6. **Rights.** arXiv non-exclusive distribution licence and the same IEEE notice as above.
7. **Give back.** Minimum observed time to collision per following encounter, from the radar's or
   the tracker's speeds, is testable once the following metrics land; a forecast is not.

## AAA Foundation: American Driving Survey, 2025 (Hungund and Steinbach, September 2026)

1. **Measures.** Self-reported travel on the day before an online or telephone interview, from a
   pre-recruited panel of about 5,100 respondents a year, interviews spread across the year,
   weighted to the US population aged 16 and over; response 19.9% of those invited in 2025. In
   2025: 95.0% drove at least occasionally, 2.40 trips, 59.3 minutes and 28.6 miles a day per
   driver, projected to 232 billion trips, 95 billion hours and 2.76 trillion miles. These are
   miles driven by people, as they report them; not vehicle-miles counted on roads.
2. **Maths.** The projection reproduces: 278.6 million residents aged 16 and over times 0.950 is
   264.7 million drivers; times 365 times 28.6 miles is 2.763 trillion; times 365 times 2.40 is
   231.9 billion trips; times 365 times 59.3 minutes is 95.5 billion hours. 28.6 times 365 is
   10,439 miles a year, the figure the brief prints. The dominant uncertainty is recall and
   coverage of a one-day diary, not sampling.
3. **Sensor.** A roadside sensor counts passages on one segment over observed time. A national
   per-driver mileage is never a check on a site's passage count, and the two are not printed
   together.
4. **Lands.** `grep` finds no mention in the repository. The crash-data plan's rates use FHWA
   vehicle-miles (HM-71 and VM-4) and hosted baselines that carry their own vehicle-miles.
5. **Verdict.** Not adopted: it is a person-reported exposure measure for drivers, not the
   vehicle-miles denominator the plan's rates divide by, and mixing them would change the unit of
   a rate. No plan edit. If a report ever wants national context on how much people drive, this is
   the citable source, as a questionnaire.
6. **Rights.** "© 2026 AAA Foundation for Traffic Safety" on every page; no reuse terms were read.
7. **Give back.** Nothing a site can test.

## Journal papers seen only as summaries

Each is **Confirm**; no figure below was read in a primary.

- **Matched case-control study of pre-crash trajectories, Accident Analysis and Prevention, epub
  July 27, 2026 (PubMed 42508180).** Summary: root-mean-square deceleration is the strongest
  predictor, with crash odds more than four times higher; jerk and yaw rate also associate; higher
  mean speed associates with lower odds. The data source is unknown. If it is vehicle telemetry
  from crash-involved and matched vehicles, it is a different population from roadside passages,
  and an observational association, not a speed-change crash model. Lands next to the crash-data
  plan's speed-change models; logged as Q3 until read.
- **Dean, Doerzaph and Riexinger, injury modification factors for Safe System speed-limit setting,
  Transportation Research Record, 2026 (doi 10.1177/03611981251327216).** Summary only: injury
  modification factors developed for setting limits. Lands on the posted speed limits plan and on
  the harm-curve edition in the crash-data plan, where curves are cited side by side and never
  averaged. Logged as Q3.
- **Occlusion-predictive network for roadside multi-vehicle tracking, Sensors 26(11):3529, 2026.**
  A learned occlusion predictor for track continuity; the behaviour plan already cites the
  time-space discontinuity correction in Sensors 26(12):3755 for the same problem. Not adopted
  unread.
- **ERSO thematic report on speed and speeding, with SWOV.** Summary: 10 to 15% of all crashes and
  30% of fatal crashes attributed to speeding or inappropriate speed. An attribution share, like
  NHTSA's, which the attitudes plan already reads as officer attribution and never a share of
  passages.

The AAA Foundation's Safe Mobility Conference proceedings (September 2026) were seen as a listing
only. The hardware synchronisation paper is about aligning cameras to a LiDAR; Tenet 1 puts it out
of scope.

## What changes

Nothing in code, the registry, benchmark kinds or definitions. In this change: this briefing; in
the behaviour plan, a link under "Weekly literature review" and a sentence beside the PRISA
reference recording its absolute-difference closing speed against Section 8.4; and a devlog
bullet.

## Open questions

| ID  | Question                                                                                                                                                                                                                                                      |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Q1  | The D-28 L3 scoring item: score L3 as it runs (online, from a cold start, scored after warm-up), as the benchmark does (warm up on the training split, then freeze), or both? The first is what ships; only the second is comparable with the paper's tables. |
| Q2  | Is a cross-elevation neighbour test worth an experiment arm once the point-label scores exist, given that L3's same-ring vote and the paper's foreground-support window test different evidence? Gated on Q1's numbers; no change to L3 is proposed.          |
| Q3  | Read the case-control trajectory paper and Dean et al. before either is placed: is the first an association or a model, and do the second's injury modification factors belong in the safety-reference edition beside the harm curves?                        |

## Sources checked

| Source                                                                                | Status                                                 |
| ------------------------------------------------------------------------------------- | ------------------------------------------------------ |
| arXiv 2608.14868, beam-wise background subtraction                                    | Read                                                   |
| arXiv 2607.16156, PRISA                                                               | Read                                                   |
| AAA Foundation, American Driving Survey: 2025, research brief PDF                     | Read                                                   |
| PyPI `humanbaselines` release list                                                    | Read                                                   |
| arXiv 2607.15889, hardware-triggered synchronisation                                  | Read in part (abstract)                                |
| HighwayScene project page and licence                                                 | **Confirm** (host did not resolve)                     |
| Accident Analysis and Prevention, pre-crash trajectory case-control (PubMed 42508180) | **Confirm**                                            |
| Dean, Doerzaph and Riexinger, TRR 2026                                                | **Confirm**                                            |
| Sensors 26(11):3529, occlusion-predictive tracking                                    | **Confirm**                                            |
| ERSO thematic report, speed and speeding                                              | **Confirm**                                            |
| AAA Foundation, 2026 Safe Mobility Conference proceedings                             | **Confirm**                                            |
| NHTSA, Speeding: 2024 Data                                                            | Superseded (read in full by the attitudes plan)        |
| Teoh, Kidd and Riexinger, IIHS preprint                                               | Superseded (read in full by the October 2026 briefing) |
| Waymo safety research page                                                            | **Confirm** (no October 2026 entry found)              |

## Provenance

Searched October 11, 2026, extended mode unless noted: roadside LiDAR tracking October 2026; Waymo
safety research publication October 2026; AAA Foundation for Traffic Safety research report October
2026; infrastructure LiDAR vehicle detection arXiv preprint October 2026; NHTSA FHWA IIHS speeding
research new report October 2026; human crash baseline automated driving comparison 2026 preprint;
following distance headway enforcement LiDAR 2026; speed change crash risk model 2026 study;
aggressive driving survey 2026; post-encroachment time time-to-collision LiDAR trajectory 2026
Accident Analysis and Prevention; FMCSA headway following too closely research project 2026; stop
sign compliance LiDAR 2026; 85th percentile speed practice speed limit setting 2026 preprint; Valgo
human baselines crash rate news 2026; new vehicle trajectory dataset drone LiDAR 2026; roadside
LiDAR preprint arXiv tracking clustering occlusion; SWOV or ITF OECD speed safety report September
October 2026; impact speed pedestrian fatality risk curve 2026 study; sober driving baseline crash
rate preprint 2026; LiDAR headway time gap measurement preprint 2026; and, in standard mode, FHWA
Traffic Volume Trends 2025 annual vehicle-miles. Not run as separate queries: the `preprint`
variants of the crash-involvement and speeding-attitudes topics.

Read with `curl` through the proxy into a scratch directory and converted with `pdftotext`:
arxiv.org PDFs for 2608.14868 and 2607.16156, the arXiv API record for 2607.15889, the AAA
Foundation PDF, and the PyPI JSON for `humanbaselines`. Refused or unreachable:
pubmed.ncbi.nlm.nih.gov (cookie wall), mdpi.com and doi.org for SAGE (403), api.crossref.org and ebi.ac.uk (egress policy),
highwayscene.github.io and eng.auburn.edu (did not resolve). No stop-sign compliance or headway
LiDAR primary was found in the window.

## To fetch

- https://pubmed.ncbi.nlm.nih.gov/42508180/ (the case-control trajectory paper, full text)
- https://doi.org/10.1177/03611981251327216 (Dean, Doerzaph and Riexinger, TRR 2026)
- https://doi.org/10.3390/s26113529 (occlusion-predictive roadside tracking)
- https://highwayscene.github.io/ (dataset licence text and download)
- https://road-safety.transport.ec.europa.eu/document/download/9826c063-bc55-423e-84a3-24200dca3547_en?filename=ERSO-TR-speed_2026.pdf
- https://aaafoundation.org/research/2026-safe-mobility-conference-proceedings/
- https://eng.auburn.edu/news/2026/09/lidar-pedestrian-sight-distance-zhou-aldot.html (seen as a
  search result only; scope unknown)
