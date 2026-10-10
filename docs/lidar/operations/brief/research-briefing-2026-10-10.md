# Research briefing, October 3 to 10, 2026

- **Status:** Complete; owner decisions applied October 10. No definition changed. Adopted: the
  Monfort and Mueller 2025 curve as a cited source in the crash-data plan. Parked: CAN-bus speed,
  noted in the motion-capture plan. Proposed: scoring L3 against point-wise static and dynamic
  labels. Every figure is **Confirm**: no primary could be read from the review environment.
- **Scope:** Roadside and infrastructure LiDAR perception, and the safety and behaviour literature
  the analytics cite, first listed or newly surfaced October 3 to 10, 2026. No third-party
  briefing was supplied.
- **Related:** [Behaviour analytics plan](../../../plans/lidar-behaviour-analytics-plan.md),
  [crash-data integration plan](../../../plans/platform-crash-data-integration-plan.md),
  [attitudes and aggressive-driving plan](../../../plans/platform-speeding-attitudes-and-aggressive-driving-plan.md),
  [human crash baselines analysis](../../../platform/operations/human-crash-baselines-analysis-2026-10.md),
  [LiDAR architecture, L3](../../architecture/LIDAR_ARCHITECTURE.md),
  [previous briefing](fixed-sensor-research-briefing-2026-10-09.md)

The week was searched along all three routes: organisation watchlists, venue watchlists and the
fourteen topic queries. Little primary work appeared in the window itself. Most of what surfaced is
older work that this week's searches newly returned: a statistical background-subtraction
benchmark for static roadside LiDAR, an infrastructure-LiDAR risk framework that scores time to
collision and predicted post-encroachment time, and a rectangle-fitting speed paper. Each lands on
something the repository already holds. The one candidate for new work is a published cross-sensor
protocol for L3 background evaluation, written up below as a proposal. Every host that carries
a primary (arxiv.org, waymo.com, aaafoundation.org, iihs.org, doi.org, mdpi.com) was unreachable,
so nothing below was read in full.

## Answer

Nothing published in the window changes a definition, a benchmark kind or a registered metric.
The Waymo sober-baseline material and the Traffic Injury Prevention fatal-rate paper are already
analysed in the [human crash baselines analysis](../../../platform/operations/human-crash-baselines-analysis-2026-10.md)
and folded into the crash-data plan. The AAA Foundation attitudes report (August 2026) is already
cited by the attitudes plan. The post-encroachment thresholds that surfaced (about 1 to 1.5 s for
serious proximity, 5 to 6 s for candidacy screening) match the rows the behaviour plan already
holds. The 85th-percentile material restates that the percentile is one input to an engineering
study, which the percentile aggregation semantics already say.

## Genuinely new this week

1. **Waymo blog post on sober-driver benchmarks, October 2026 (date Confirm).** The search summary
   gives the headline "Sober Drivers Still Face Nearly 4x Nighttime Risk" and a share of 8.2% of
   fatal cases involving strictly human behavioural factors alone. A second search could not
   reproduce the headline; it returned Waymo's July 2026 time-of-day post instead. Treat the
   headline and both figures as unverified. It is a blog summary of the sober-baseline preprint
   the repository has already read in full, so it adds no method.
2. **AAA Foundation release on unbuckled occupants, October 8, 2026.** Reported as 28% of drivers
   and passengers in fatal crashes in 2017 to 2022 being unbuckled (**Confirm**). Belt use is not
   observable from a roadside LiDAR passage. It is out of scope, and recorded so it is not
   rediscovered.

## Newly surfaced older work

| Source                                                                                                 | First listed  | Why it surfaced this week                                  |
| ------------------------------------------------------------------------------------------------------ | ------------- | ---------------------------------------------------------- |
| Baumann, Vosshans and Dang, beam-wise statistical background subtraction, arXiv 2608.14868 (ITSC 2026) | August 2026   | Returned by the roadside-tracking and extent queries       |
| Bang and co-authors, PRISA, arXiv 2607.16156                                                           | July 17, 2026 | Returned by the roadside-tracking and extent queries       |
| Rectangle edge matching for infrastructure LiDAR speed, Applied Sciences 16(5) 2513                    | 2026          | Returned by the extent-estimation query                    |
| Schaefer, lidar object tracking with edge sensor nodes, Advanced Intelligent Systems                   | 2026          | Returned by the roadside-tracking query                    |
| AAA Foundation, Attitudes Toward Speeding, technical report                                            | August 2026   | Returned by the attitudes query; already in the repository |
| IIHS and Journal of Safety Research, pedestrian injury risk by impact speed and front-end height       | 2025          | Returned by the harm-curve query                           |

## Baumann, Vosshans and Dang: beam-wise statistical background subtraction (arXiv 2608.14868)

1. **Measures.** Per-beam statistical background models for fixed LiDAR, scored point by point as
   static or dynamic. It adds a dataset, HighwayScene (an Ouster OS0, an Aeva Aeries II and a
   Blickfeld QB2 at a highway construction site), and point-wise labels for CoopScenes.
   It reports that per-beam models with spatial-consistency filtering raise precision at high
   recall in real time. No figures were read (**Confirm**). Not a measurement of detection,
   tracking or speed.
2. **Maths.** Not checked; the paper was not read.
3. **Sensor.** A per-beam model is what L3 holds: the range-image grid keeps a per-cell mean and
   variance over the sensor's own beams and azimuths. The question is not whether the paper's
   idea is new to the repository but whether its protocol and labels could score L3. None of this
   needs identity or a network call.
4. **Lands.** `grep` finds no mention of the paper or its datasets. L3b is documented in
   [background-grid-settling-maths.md](../../../../data/maths/background-grid-settling-maths.md) and
   the architecture table. The nearest repository document on scoring is the annotation-scored
   tuning note.
5. **Verdict.** New; the owner asked for the proposal to be explored. See
   [Proposal: score L3 against point-wise labels](#proposal-score-l3-against-point-wise-labels).
6. **Rights.** HighwayScene is listed as CC BY-NC-SA 4.0 on Voxel51 (**Confirm**). The project is
   non-commercial in the sense the licence asks only for research notes; any use beyond reading
   needs the licence read first, and share-alike would bind a derived label set.
7. **Give back.** A site can contribute nothing to a cross-sensor benchmark: it has one sensor
   model per deployment. It can label its own capture the same way, which is step 3 of the
   proposal.

## Proposal: score L3 against point-wise labels

**Why.** Nothing in the repository measures what L3 throws away. Annotation packs are exported
foreground only, with periodic settled-background snapshots
([point annotation tool](../point-annotation-tool.md)), and the
[annotation-scored tuning](../annotation-scored-tuning.md) sweep scores L4 clusters against the
labelled foreground. A return from a moving car that L3 absorbs into the background never reaches
a pack, so L3's dynamic recall is unmeasured, and its precision is seen only through what L4 does
with the speckle. A full-scene, point-wise static or dynamic label measures both directly.

**What L3 already shares with the method.** The grid keeps a per-cell range mean and spread at
(ring, azimuth bin), which is a per-beam model, and same-ring neighbour confirmation is a spatial
consistency step ([background-grid maths](../../../../data/maths/background-grid-settling-maths.md),
Sections 2 to 5). Section 11 of that note lists the known limits the benchmark would probe: a
unimodal cell, a heuristic confidence count, and neighbour votes along the ring only, never across
elevation. Whether the paper's filter is the same operation, and in which direction it moves the
decision, is unknown until the paper is read.

**Fit to the datasets.** The entry point is `ProcessFramePolarWithMask`, which takes points as
channel, azimuth and distance, so an adapter needs only per-point ring, azimuth and range.

| Sensor                    | Fit to L3                                                                                     |
| ------------------------- | --------------------------------------------------------------------------------------------- |
| Ouster OS0 (HighwayScene) | Rotating with fixed beams; maps to (ring, azimuth bin) once its beam count is set in the grid |
| Aeva Aeries II            | FMCW; L3 would ignore per-point velocity. Scan pattern unread (**Confirm**)                   |
| Blickfeld QB2             | Non-rotating; no ring and azimuth structure, so out of scope for the polar grid               |
| CoopScenes, added labels  | Sensors unread (**Confirm**)                                                                  |

**Steps.**

0. Read the paper, the HighwayScene card and licence, and the CoopScenes label format (`S`). This
   is gated on the "To fetch" list.
1. An offline Go tool beside `settling-eval` in `cmd/tools/` that reads a labelled sequence, maps
   each point to polar form, runs a local background manager with the shipped defaults, and reports
   per-point precision, recall and F1 for dynamic returns, after warm-up, per sensor and per range
   band (`M`). Outputs stay local; the written report goes in `docs/`.
2. A report beside the annotation-scored tuning note with the numbers, and the paper's own figures
   quoted only where the protocol matches.
3. Separately, and the evidence that matters for this product: label one Pandar40P capture full
   scene rather than foreground only, starting with kirk0, so that L3 is scored on the sensor and
   street the product ships on (`M`, the labelling is the cost).

**What it would not establish.** An Ouster OS0 at a highway construction site is not a Pandar40P
on a residential street: different beam count, range noise and traffic. A good or bad score there
says how the method behaves, not how the product does. Step 1 must not change the shipped
defaults; a tuning change moves the config fingerprint and needs the perf baselines recaptured in
the same change.

**Rights and tenets.** CC BY-NC-SA 4.0 (**Confirm**) permits non-commercial research use with
credit; derived labels or results would carry the share-alike terms. The dataset is never vendored
into the repository. Only the LiDAR streams are used; any camera stream in a dataset is left
unread, and nothing runs on the device or needs a network call at runtime.

**Decision needed.** Whether to run steps 0 and 1 as an experiment, and whether step 3 joins the
annotation backlog. No backlog item is added until then.

## Bang and co-authors: PRISA (arXiv 2607.16156)

1. **Measures.** A roadside-LiDAR perception layer plus a risk module that forecasts motion and
   scores time to collision for longitudinal conflicts and predicted post-encroachment time for
   crossings. Evaluated on R-LiViT and deployed on an edge GPU at a signalised intersection in
   Chattanooga. Reported: 194 ms end to end for the predicted post-encroachment assessment over a
   2.4 s horizon (**Confirm**). Predicted quantities, not observed ones.
2. **Maths.** Not checked.
3. **Sensor.** Observed post-encroachment time needs both road users in view of a shared conflict
   region, which the behaviour plan already scores "yes when both crossings are in view". A
   predicted value is a forecast from a learned model; it is not a measurement and would be
   printed apart from one. The edge-GPU deployment is outside the Raspberry Pi envelope.
4. **Lands.** The plan holds the 5 s candidacy and 1 to 1.5 s proximity rows already (behaviour
   plan, benchmark table). `published_model` is the proposed kind that a forecast would use.
5. **Verdict.** Already holds for thresholds; the forecasting approach is not adopted, because
   nothing in the roadmap forecasts. Not a rename of any registered metric.
6. **Rights.** arXiv licence not read (**Confirm**).
7. **Give back.** Observed minimum PET over a conflict region is a research-note candidate once
   v0.5.3 lands; a forecast is not testable from a point sensor.

## Rectangle edge matching for infrastructure LiDAR speed (Applied Sciences, 2026)

1. **Measures.** A two-dimensional rectangle fitted to each vehicle contour by Gauss-Newton, with
   speed taken from the fitted rectangles; reported mean absolute error 0.76 to 1.37 km/h
   against CAN-bus speed (**Confirm**; search summary).
2. **Maths.** Not checked. The comparison is to vehicle-reported speed, not to a radar, so it
   validates a speed estimate rather than a headway.
3. **Sensor.** Fully within what the sensor sees. Extent estimation from fitted rectangles is what
   the solid-body work already attempts.
4. **Lands.** The rectangle-fit lineage is in the
   [physical fit to points plan](../../../plans/lidar-physical-fit-to-points-plan.md) and the heading
   coherence plan. The paper is not cited in either.
5. **Verdict.** Already holds in method. The CAN-bus reference is parked: it suits moving-platform
   work (ego speed for ego-motion and SLAM), not the static work, which keeps the radar as its
   speed reference. Noted under the motion-capture plan's pose sources.
6. **Rights.** Open-access journal (**Confirm**).
7. **Give back.** None without CAN-bus ground truth.

## Attitudes, harm curves, and thresholds (already in the repository)

The AAA Foundation attitudes report (focus groups of 58 drivers and a national survey above
16,000, **Confirm**) is already cited by the attitudes plan. Its five-segment typology describes
people; the briefing records it only as survey prevalence and keeps it off any comparison with a
share of passages. Its reported 10 mph social threshold is a questionnaire finding, not an observed
share, and the plan's `no_established_threshold` kind is the right place for it.

The IIHS/Journal of Safety Research pedestrian curve (1% fatality at 20 mph, 19% at 35 mph, above
80% at 50 mph for a 202-crash sample, **Confirm**) differs sharply from the Tefft 2013 curve the
crash-data plan cites (50% at about 42 mph). It is Monfort and Mueller, "A modern injury risk
curve for pedestrian injury in the United States: the combined effects of impact speed and vehicle
front-end height", _Journal of Safety Research_ 94, 235 to 241, September 2025 (IIHS preprint,
December 2024), with MAIS 2+F, MAIS 3+F and fatal curves and formulas by hood leading-edge height
(**Confirm**). Adopted as a cited source: the crash-data plan now lists it beside Tefft 2013, adds
it to Item 1's reading, records it in its sources table as **Confirm from paper**, and notes it
under the deferred front-end geometry item, since hood height is the parameter the vehicle
encyclopedia would supply. The plan's rule stands: curves are stated side by side, never averaged,
and a curve applied to travel speeds is an upper bound.

## What changes

Nothing in code, registry, benchmark kinds or definitions. The edits in this change are this
briefing; a link from the behaviour plan's references; the Monfort and Mueller curve in the
crash-data plan (Section "What the evidence can and cannot establish", Item 1, sources checked and
the deferred geometry item); a parked CAN-bus note in the motion-capture plan; and a devlog
bullet.

## Open questions

| ID  | Question                                                                                         | Decision                                                                                                |
| --- | ------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------- |
| Q1  | Should L3 be scored against point-wise static and dynamic labels?                                | Go ahead: an `S` backlog item gated on the data release, no production change ([D-28][d28], October 10) |
| Q2  | Is CAN-bus-referenced speed a usable external check for fitted-extent speed?                     | Parked for motion and SLAM work                                                                         |
| Q3  | Should the harm-curve citations add the recent US curve with front-end height beside Tefft 2013? | Yes: added to the crash-data plan                                                                       |

## Sources checked

| Source                                                                                | Status                                                |
| ------------------------------------------------------------------------------------- | ----------------------------------------------------- |
| Waymo blog, sober-driver benchmarks, October 2026                                     | **Confirm** (search summary; headline not reproduced) |
| Waymo blog, time-of-day crash risk, July 2026                                         | **Confirm**                                           |
| Scanlon et al., sober baselines; Kusano et al., fatal rate (TIP 2026)                 | Superseded (read in full in the October 8 analysis)   |
| Valgo humanbaselines                                                                  | Superseded (October 8 analysis)                       |
| AAA Foundation, Attitudes Toward Speeding, August 2026                                | **Confirm**                                           |
| AAA Foundation, Traffic Safety Culture Index, June 2026; seat belt release, October 8 | **Confirm**                                           |
| arXiv 2608.14868, beam-wise background subtraction                                    | **Confirm**                                           |
| arXiv 2510.22390, interpretable background subtraction (ICVES 2025)                   | **Confirm**                                           |
| arXiv 2607.16156, PRISA                                                               | **Confirm**                                           |
| Applied Sciences 16(5) 2513, rectangle edge matching                                  | **Confirm**                                           |
| Schaefer, Advanced Intelligent Systems, doi 10.1002/aisy.202501102                    | **Confirm**                                           |
| IIHS pedestrian height release; J. Safety Research S0022437525001021                  | **Confirm**                                           |
| FHWA Speed Limit Setting Handbook; ITE setting speed limits page                      | **Confirm**                                           |
| Post-encroachment time threshold studies (Springer 2026, ScienceDirect midblock)      | **Confirm**                                           |

## Provenance

Searched October 10, 2026 with extended mode unless noted: roadside LiDAR tracking 2026;
infrastructure LiDAR vehicle extent estimation preprint; Waymo safety research publication October
2026; AAA Foundation for Traffic Safety new report October 2026; human crash baseline ADS sober
driving baseline 2026; following distance headway measurement LiDAR 2026 preprint; speeding
attitudes national survey 2026; impact speed pedestrian fatality curve 2026; post-encroachment
time threshold 2026; stop sign compliance LiDAR 2026; 85th percentile speed practice speed limit
setting 2026 FHWA NHTSA IIHS; crash involvement rate per vehicle mile baseline preprint 2026; and,
in standard mode, three title lookups for the background-subtraction benchmark, PRISA and the
Waymo headline. Not run as separate queries: speed change crash model, aggressive driving survey,
roadside LiDAR tracking preprint, sober driving baseline preprint. Fetches of the Waymo post,
the three arXiv papers, doi.org, IIHS, AAA Foundation and PMC failed to resolve in both the fetch
tool and `curl` through the proxy. Stop-sign compliance and headway queries found no LiDAR
primary in the window.

## To fetch

- https://waymo.com/blog/2026/10/sober-driving-benchmarks/
- https://arxiv.org/abs/2608.14868 and https://arxiv.org/abs/2607.16156
- https://doi.org/10.3390/app16052513
- https://aaafoundation.org/wp-content/uploads/2026/08/202608_AAAFTS-Attitudes-towards-Speeding.pdf
- https://www.iihs.org/news/detail/vehicle-height-compounds-dangers-of-speed-for-pedestrians
- https://huggingface.co/datasets/iis-esslingen/HighwayScene (licence text)
  [d28]: ../../../DECISIONS.md#d-28--research-briefing-decisions-october-2026
