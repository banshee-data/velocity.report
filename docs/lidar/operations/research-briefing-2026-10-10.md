# Research briefing, October 3 to 10, 2026

- **Status:** Complete. No definition changed; nothing adopted. Three items are logged as open
  questions (Q1 to Q3). Every figure is **Confirm**: no primary
  document could be read from the review environment.
- **Scope:** Roadside and infrastructure LiDAR perception, and the safety and behaviour literature
  the analytics cite, first listed or newly surfaced October 3 to 10, 2026. No third-party
  briefing was supplied.
- **Related:** [Behaviour analytics plan](../../plans/lidar-behaviour-analytics-plan.md),
  [crash-data integration plan](../../plans/platform-crash-data-integration-plan.md),
  [attitudes and aggressive-driving plan](../../plans/platform-speeding-attitudes-and-aggressive-driving-plan.md),
  [human crash baselines analysis](../../platform/operations/human-crash-baselines-analysis-2026-10.md),
  [LiDAR architecture, L3](../architecture/LIDAR_ARCHITECTURE.md),
  [previous briefing](fixed-sensor-research-briefing-2026-10-09.md)

The week was searched along all three routes: organisation watchlists, venue watchlists and the
fourteen topic queries. Little primary work appeared in the window itself. Most of what surfaced is
older work that this week's searches newly returned: a statistical background-subtraction
benchmark for static roadside LiDAR, an infrastructure-LiDAR risk framework that scores time to
collision and predicted post-encroachment time, and a rectangle-fitting speed paper. Each lands on
something the repository already holds. The one candidate for new work is a published cross-sensor
protocol for L3 background evaluation, logged as a question. Every host that carries a primary
(arxiv.org, waymo.com, aaafoundation.org, iihs.org, doi.org, mdpi.com) was unreachable, so
nothing below was read in full.

## Answer

Nothing published in the window changes a definition, a benchmark kind or a registered metric.
The Waymo sober-baseline material and the Traffic Injury Prevention fatal-rate paper are already
analysed in the [human crash baselines analysis](../../platform/operations/human-crash-baselines-analysis-2026-10.md)
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
   [background-grid-settling-maths.md](../../../data/maths/background-grid-settling-maths.md) and
   the architecture table. The nearest repository document on scoring is the annotation-scored
   tuning note. Logged as Q1.
5. **Verdict.** Not adopted. New and logged as an open question.
6. **Rights.** HighwayScene is listed as CC BY-NC-SA 4.0 on Voxel51 (**Confirm**). The project is
   non-commercial in the sense the licence asks only for research notes; any use beyond reading
   needs the licence read first, and share-alike would bind a derived label set.
7. **Give back.** A site can contribute nothing to a cross-sensor benchmark: it has one sensor
   model per deployment.

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
   [physical fit to points plan](../../plans/lidar-physical-fit-to-points-plan.md) and the heading
   coherence plan. The paper is not cited in either.
5. **Verdict.** Already holds in method; the CAN-bus reference is a candidate external check on
   speed accuracy. Logged as Q2.
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
crash-data plan cites (50% at about 42 mph). The plan already says a curve applied to measured
speeds is an upper bound and that front-end geometry is absent. No curve is adopted; a recent
US curve with vehicle height is a candidate second citation, logged as Q3.

## What changes

Nothing in code, registry, benchmark kinds or definitions. The edits in this change are this
briefing, a link from the behaviour plan's references, and a devlog bullet.

## Open questions

| ID  | Question                                                                                                                                                             |
| --- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Q1  | Should L3 background quality be scored against the beam-wise benchmark protocol and its point-wise static/dynamic labels, and does the CC BY-NC-SA licence allow it? |
| Q2  | Is CAN-bus-referenced speed (rectangle-fit papers) a usable external check for the fitted-extent speed, beside the radar reference?                                  |
| Q3  | Should the crash-data plan's harm-curve citations add the recent US curve with front-end height beside Tefft 2013 and Rosen and Sander 2009?                         |

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
