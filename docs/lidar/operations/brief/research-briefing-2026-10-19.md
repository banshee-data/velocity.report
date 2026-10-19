# Research briefing, October 12 to 19, 2026

- **Status:** Complete. No definition, benchmark kind or registered metric changed. Adopted: Chu,
  Gao, Xu and Liu (_Communications Engineering_, October 5) as the cited evidence for the
  behaviour plan's existing caveat that Waymo Open Motion is not a normative human-driving
  baseline, because traffic around an automated vehicle follows it differently. One open
  question for the owner.
- **Scope:** Roadside and infrastructure LiDAR perception, and the safety and behaviour literature
  the analytics cite, first listed or newly surfaced October 12 to 19, 2026. No third-party
  briefing was supplied. The October 11 briefing is still on its own branch
  (`claude/research-briefing-2026-10-11`) awaiting review; nothing it or the October 12 briefing
  evaluated is repeated here.
- **Related:** [Behaviour analytics plan](../../../plans/lidar-behaviour-analytics-plan.md),
  [crash-data integration plan](../../../plans/platform-crash-data-integration-plan.md),
  [state estimation plan](../../../plans/lidar-state-estimation-plan.md),
  [physical-reference review plan](../../../plans/lidar-physical-reference-review-plan.md),
  [human crash baselines analysis](../../../platform/operations/human-crash-baselines-analysis-2026-10.md),
  [previous briefing](research-briefing-2026-10-12.md)

Paper and research-report citation keys are mapped in the [reference audit](reference-audit.md);
full entries are in [references.bib](../../../../data/maths/references.bib).

All three discovery routes were run. The arXiv query API refused this session ("Rate exceeded"),
so the arXiv route used the past-week listings of cs.RO, cs.CV and eess.SP instead, filtered by
title. It was a quiet week for primary work in scope: three preprints first listed in the window
touch roadside perception or tracking, and none measures what this sensor measures. The one
source that bears on the analytics is a journal paper published on October 5 and covered in the
press on October 16: matched car-following events from three datasets show human drivers keeping
shorter time headways behind automated leaders than behind manual ones. That is direct evidence
for a caveat the behaviour plan already carries. Most of the week's safety news was coverage of
the Waymo sober-baseline paper, which the repository already holds in full. No primary in the
safety literature could be read from this session; every figure taken from it is **Confirm**.

## Answer

Nothing changes a definition, a threshold, a benchmark kind or a registered metric. The behaviour
plan's Waymo Open Motion row in Section 12 already says the dataset is "collected around an
autonomous vehicle whose presence and behaviour influence the surrounding traffic". Chu et al.
now stand behind that sentence as its citation, from the abstract, with their figures marked
**Confirm**. The paper raises one question for reports at sites that automated fleets serve (Q1).

The roadside camera-and-radar paper validates its extent against templates that set the extent,
so its length errors measure the template, not the road user; it is not adopted, and it is
excluded as a method by Tenet 1. The sports tracker's finding that a hard Mahalanobis gate is
the binding constraint on association is not adopted: L5's gate is already a tuned parameter, and
the finding comes from a panning, zooming camera. A Palermo study applies impact-speed harm curves
to percentile travel speeds, which the crash-data plan already names as an upper bound; it is
logged and not adopted.

## Genuinely new this week

| Source                                                                                                       | First listed | Status       |
| ------------------------------------------------------------------------------------------------------------ | ------------ | ------------ |
| Agrawal, Bhanderi and Elger, roadside camera and radar 3D shape and orientation, arXiv 2610.18417 (SDF 2026) | October 13   | Read         |
| Adari, ARC-SORT, multi-object tracking with interacting multiple models, arXiv 2610.22306                    | October 15   | Read in part |
| Wang, Bai, Yang and Shen, InfraWorld, roadside 4D occupancy forecasting, arXiv 2610.14544                    | October 10   | Read in part |
| Waymo and MADD coverage of the sober-baseline paper, October 8 and 14                                        | October 8    | Superseded   |

InfraWorld was submitted on October 10 and first appeared in the listings during this window. It
forecasts future semantic occupancy from learned bird's-eye-view states on the InfraOcc and
Occ3D-nuScenes datasets; nothing here forecasts, so it is recorded and not evaluated further.

The October 8 and 14 coverage of Waymo's sober-baseline research is coverage of Scanlon, Kusano,
McMurry, Campolettano and Victor, accepted at the _Journal of Safety Research_, which the
[human crash baselines analysis](../../../platform/operations/human-crash-baselines-analysis-2026-10.md)
read in full from the preprint and the crash-data plan lists as **Confirm on publication**. The
coverage repeats the 23% average fatal reduction and the 3.1 to 3.9 night-to-day ratio, both
already reproduced there. One summary attributes "up to 31%" to the paper; the analysis gives
Houston as 32%, the largest per-area reduction, so the press figure is a rounding of a per-area
value read from a figure, not a national one. Nothing changes.

## Newly surfaced older work

| Source                                                                                                                   | First listed    | Why it surfaced this week                                               |
| ------------------------------------------------------------------------------------------------------------------------ | --------------- | ----------------------------------------------------------------------- |
| Chu, Gao, Xu and Liu, automated leaders and human followers, _Communications Engineering_ (s44172-026-00793-3)           | October 5, 2026 | Press coverage of October 16; headway topic query                       |
| Palermo vehicle-pedestrian crash severity and speed reduction, _Frontiers in Future Transportation_ (ffutr.2026.1805850) | August 25, 2026 | Harm-curve topic query; not in the repository                           |
| Waymo safety hub update to 270 million rider-only miles through June 2026                                                | October 2026    | Organisation watchlist; an automated-driving rate, not a human baseline |

The Waymo hub update reports the automated fleet's own crash rates; the crash-data plan uses
human baselines, so it is recorded and not evaluated. AAA Foundation, IIHS, NHTSA, FHWA, FMCSA,
SWOV and ITF searches found nothing first published in the window: the newest items were the
August 2026 attitudes report, the October 2026 IIHS speed-camera sheet and NHTSA's first-quarter
2026 early estimate, all already held or out of scope.

## Chu, Gao, Xu and Liu: automated vehicles and the human drivers behind them (_Communications Engineering_, October 5, 2026)

1. **Measures.** From the abstract as quoted by search results: car-following events from three
   real-world datasets, Lyft and Waymo driverless vehicles and vehicles under adaptive cruise
   control, "align[ed] by similarities in lead-vehicle speed profiles" in quasi-experiments. The
   quantities are time headway and time to collision, for the automated vehicle as follower and
   for a human driver following an automated leader. The finding: "Lyft AVs exhibit smoother and
   safer car-following, with longer time-headways and higher time-to-collision than human drivers
   when following aligned manual vehicles", while "human drivers behave more riskily, with shorter
   time-headways and smaller time-to-collision when following aligned Lyft AVs versus manual
   vehicles, partly due to AV leaders' greater stability", with "similar changes" in the other two
   datasets. These are naturalistic trajectories recorded by the automated vehicles' own sensors
   (the Lyft and Waymo sets) or by instrumented platoons, not roadside observations, and the
   headway definition (front to front or rear to front) was not readable. Authors from Chang'an
   University and Zhejiang University; the paper itself was unreachable (**Confirm**).
2. **Maths.** Nothing could be re-derived: no number in the paper was readable. The design matters
   more than any figure here. Matching on the leader's speed profile controls the stimulus but not
   the follower population, which the abstract does not describe, and "partly due to" stability
   says the authors did not fully separate the leader's smoothness from any response to the
   leader being automated. The direction agrees with earlier work cited in the same results (Wen
   et al.; Rahmati et al., instrumented automated leader), which is consistency, not confirmation.
3. **Sensor.** The sensor sees passages, not who or what drives them. It cannot tell an automated
   leader from a manual one in the general case, and would not try: a vehicle's operator is not an
   observable this project records. What it can see is a leader's speed trace, which is the
   quantity the paper matched on. So the paper's lesson for a site is about population, not
   classification: where automated fleets operate, a share of observed following passages has an
   automated leader, and that share is unknown to the report.
4. **Lands.** The behaviour plan's Section 12 table already says of Waymo Open Motion: "Not a
   normative human-driving baseline: it is collected around an autonomous vehicle whose presence
   and behaviour influence the surrounding traffic". That row had no citation for the influence.
   `grep` finds no other mention of automated leaders. The registry's
   `interaction.following_net_time_gap_s` (alias `time_headway`) is untouched; this is a
   population note, not a definition.
5. **Verdict.**

   | Claim                                                          | Verdict                                                                                                           |
   | -------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
   | Human followers keep shorter headways behind automated leaders | **New and adopted** as the citation for the existing Waymo Open Motion caveat, abstract only, figures **Confirm** |
   | Automated followers keep longer headways than human ones       | Already holds: the same row's reason for not treating the dataset as a human baseline                             |
   | Leader type as a stratum of a site's following distribution    | **Open question** Q1, as a limitations sentence only; the sensor cannot observe the stratum                       |
   | "Riskier" and "safer" as descriptions of followers             | Not adopted: verdict words; the plan describes headway distributions, never a driver's conduct                    |

6. **Rights.** Semantic Scholar lists the paper as gold open access under CC BY-NC-ND
   (**Confirm** on the publisher's page). Citation with credit and short quotation are within it;
   no figure is reproduced.
7. **What a survey could give back.** A site cannot test the paper's claim, because it cannot
   label leaders. It can test the weaker statement the claim implies for any mixed stream: that
   follower headway behind leaders with low speed variance differs from headway behind leaders
   with high variance, holding leader speed. That is a research-note hypothesis over observed
   pairs, after the behaviour plan's B1 gate is met; it is not proposed as work.

## Agrawal, Bhanderi and Elger: roadside camera and radar 3D shape and orientation (arXiv 2610.18417)

1. **Measures.** A two-stage roadside pipeline: an off-the-shelf 2D camera detector and the
   associated 3D automotive radar points give each road user a position (radar centroid), a height
   (2D box corners projected using radar depth) and a Doppler velocity; length and width come from
   per-class templates, scaled for pedestrians and clamped for bicycles and cars; a small network
   regresses yaw for bicycles and cars. Validation is against hand-labelled 3D boxes in a LiDAR
   point cloud on 500 frames at one curved road with a junction (451 persons, 360 bicycles, 299
   cars, 0 to 50 m). It is not a LiDAR method, and the camera is the first stage.
2. **Maths.** The ablation tables show what the extent numbers measure. For cars, raising the
   length floor from 3.8 m to 4.4 m (ceiling 5.5 m) cuts the mean length error from 0.64 m to
   0.21 m (Table III, rows 1 to 4), and narrowing the ceiling to 4.9 m leaves it at 0.21 m with a
   standard deviation of 0.05 m. A floor that does that work means the radar spread was short of
   the true length on most cars, and a 0.05 m spread in error across 299 cars whose true lengths
   vary by more than that is what a clamp produces. Width behaves the same way (floor 1.5 m to
   1.9 m: 0.42 m to 0.08 m), and the y-centroid error rises as width improves (0.52 m to 0.58 m),
   because the box is shifted to the sensor-facing edge after the template is fitted. The
   reported extent accuracy is the accuracy of the template against the labelled population at
   this site. The yaw error for cars, 3.68° (standard deviation 3.65°), is a measured quantity and
   the only one in the paper that is.
3. **Sensor.** None of it transfers as a method: a camera is excluded by Tenet 1, and a Pandar40P
   measures extent from returns rather than assuming it. The transferable lesson is the validation
   one: an extent error reported for a pipeline with size priors must say how much of the extent
   the prior supplied.
4. **Lands.** The state estimation plan already holds that lesson: class priors are used only
   before a body has accepted its first frame, are "marked" as such (Section 5.5 onwards), and
   "remain fallible and revisable"; and the physical-reference review plan exists because
   labelled boxes are not a physical reference. Already holds.
5. **Verdict.** Not adopted: a camera method, and an extent validation that measures its own
   templates. The LiDAR-labelled ground truth is labels, not a physical reference.
6. **Rights.** arXiv non-exclusive distribution licence; citation only.
7. **Survey.** Nothing to give back.

## Adari: ARC-SORT (arXiv 2610.22306)

1. **Measures.** Higher-order tracking accuracy (HOTA) of a training-free tracking-by-detection
   tracker on SportsMOT and DanceTrack: broadcast camera video of athletes and dancers, with
   camera pan and zoom. An interacting multiple model Kalman filter's predictive covariance sets a
   per-track expanded matching region, close assignment decisions at crossings are kept under
   review, and one corrective identity swap is allowed on later appearance evidence.
2. **Maths.** The headline for this project is a sensitivity, not an equation: "the same
   covariance used as a hard Mahalanobis gate hurts at every threshold we swept". The paper's
   Figure 2 gives pooled SportsMOT validation HOTA rising from 79.19 with the default motion-gate
   tolerance to 84.03 with the gate removed, in the offline re-identification merge. The paper's
   own explanation is domain-specific: for players who leave a panning frame for seconds, motion
   extrapolation carries no information.
3. **Sensor.** A static LiDAR in a fixed world frame has no camera motion to compensate, and its
   bodies obey vehicle kinematics; the case where the gate binds in the paper is the case the
   sensor least resembles. Appearance re-identification is not available here and would not be
   wanted.
4. **Lands.** L5 associates by squared Mahalanobis distance with `gating_distance_squared`
   (default 36) as a hard gate (`internal/lidar/l5tracks/tracking_association.go`), and the
   [auto-tuning guide](../auto-tuning.md) already sweeps it from 4 to 100. Duplicate
   and replacement tracks have their own work
   ([L5 duplicate tracks](../l5-duplicate-tracks-2026-10.md)).
   Already holds as a tuned parameter.
5. **Verdict.** Not adopted: the finding is measured in a domain whose failure mode, unpredictable
   motion in a moving frame, the sensor does not share.
6. **Rights.** arXiv non-exclusive distribution licence; citation only.
7. **Survey.** Nothing to give back.

## Palermo: speed reduction and vehicle-pedestrian crash severity (_Frontiers in Future Transportation_, August 25, 2026)

1. **Measures.** From search excerpts only (**Confirm**): police-reported vehicle-pedestrian
   crash locations in Palermo, linked to TomTom probe speeds, with the 15th, 50th and 85th
   percentile travel speeds used "as proxies for impact speed" in published fatal and serious
   injury risk functions, and counterfactual reductions of 3 and 5 km/h across the distribution.
   The excerpts give estimated fatal risk at the three percentiles as 0.7%, 2.6% and 8.2%.
2. **Maths.** Not re-derivable without the risk functions. The structure is the point: a travel
   speed put into an impact-speed curve assumes no braking, so every risk is an upper bound, and
   the counterfactual inherits the bound.
3. **Sensor.** A site measures travel-speed distributions at a point, with a real denominator over
   observed time; the probe percentiles here are a different instrument with an unstated
   denominator. The method is one a report could imitate, which is why the plan forbids it
   unqualified.
4. **Lands.** The crash-data plan's harm-curve row already says the curves give harm at
   **impact** speed, and that "applied to measured speeds the curve is an upper bound, and the
   report says so". Already holds.
5. **Verdict.** Not adopted, and not added to the plan's sources: it adds one city's application of
   curves the plan already lists, read only in excerpt.
6. **Rights.** **Confirm**; Frontiers articles are usually CC BY, not read here.
7. **Survey.** A site could reproduce the calculation, labelled as an upper bound, from its own
   percentiles; the plan already specifies that and nothing new is proposed.

## What changes

In this change:

- [Behaviour analytics plan](../../../plans/lidar-behaviour-analytics-plan.md): the Waymo Open
  Motion row in Section 12 cites Chu et al. for the influence of automated vehicles on the traffic
  around them; this briefing is linked under "Weekly literature review".
- A devlog bullet.

Nothing in code, the metrics registry, benchmark kinds or definitions.

## Open questions

| ID  | Question                                                                                                                                                                                                                                                                                                                                                                                                         |
| --- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Q1  | Should the headway report's limitations text say, for a site within an automated fleet's service area, that an unknown share of observed following passages may have an automated leader, and that published evidence (Chu et al. 2026) finds followers keep shorter headways behind such leaders? The sensor cannot observe the share and must not try; the sentence would be a population note, not a stratum. |

The October 11 briefing's three questions remain open on its own branch and are not repeated.

## Sources checked

| Source                                                                             | Status                                                                          |
| ---------------------------------------------------------------------------------- | ------------------------------------------------------------------------------- |
| arXiv 2610.18417, Agrawal, Bhanderi and Elger                                      | Read                                                                            |
| arXiv 2610.22306, Adari, ARC-SORT                                                  | Read in part (abstract, introduction, Figure 2, tables)                         |
| arXiv 2610.14544, InfraWorld                                                       | Read in part (abstract)                                                         |
| Chu, Gao, Xu and Liu, _Communications Engineering_, doi:10.1038/s44172-026-00793-3 | **Confirm** (abstract through search quotation; metadata from Semantic Scholar) |
| Palermo study, doi:10.3389/ffutr.2026.1805850                                      | **Confirm** (search excerpts only)                                              |
| Waymo and MADD coverage of the sober-baseline paper, October 8 and 14              | Superseded (the preprint is read in full in the analysis)                       |
| Waymo safety hub, 270 million miles through June 2026                              | **Confirm** (search summary; automated-driving rate, out of scope)              |
| AAA Foundation, _Attitudes Toward Speeding_, August 2026                           | Superseded (held by the attitudes plan)                                         |
| IIHS and HLDI, speed safety cameras sheet, October 2026                            | Superseded (read in the October 12 briefing)                                    |
| NHTSA early estimate, first quarter of 2026; NSC first half of 2026                | **Confirm** (search summaries; FARS remains the plan's fatal source)            |
| arXiv past-week listings, cs.RO, cs.CV, eess.SP                                    | Read (titles filtered; abstracts read for the three above)                      |

## Provenance

Searched October 19, 2026, extended mode: Waymo safety research new paper October 2026; AAA
Foundation for Traffic Safety new research report October 2026; IIHS speeding study October 2026;
NHTSA traffic fatalities early estimate first half 2026 speeding; roadside LiDAR vehicle tracking
2026 paper infrastructure point cloud; human crash baseline automated driving system 2026 preprint
Valgo humanbaselines; Waymo Mothers Against Drunk Driving impaired drivers fatal crashes 31
percent study October 2026; study autonomous vehicles nearby human drivers take risks October 2026
headway following; following distance headway measurement LiDAR enforcement 2026 FMCSA tailgating
research; post-encroachment time threshold or time to collision surrogate safety roadside lidar
2026 study; pedestrian impact speed fatality risk curve or speed change crash model 2026 Accident
Analysis and Prevention; 85th percentile speed limit setting practice 2026 study or SWOV or ITF
speed report October 2026; the Chu et al. title with its datasets; stop sign compliance roadside
sensor study 2026 or new vehicle trajectory dataset released October 2026 drone LiDAR; speeding
attitudes national survey 2026 aggressive driving survey results October; the Palermo study's
journal and counterfactual; crash involvement rate per vehicle mile exposure denominator FARS 2026
paper Traffic Injury Prevention October; FHWA safety research October 2026 report speed or TRB
TRID 2026 LiDAR traffic monitoring report. The `preprint` variants of the topics were folded into
the arXiv listing route rather than run as separate web queries.

arXiv: the query API at export.arxiv.org answered "Rate exceeded" to every query, so the past-week
listings for cs.RO (661 entries), cs.CV (1,066) and eess.SP (200) were fetched from arxiv.org and
filtered by title for roadside, infrastructure, LiDAR, traffic, headway, crash, pedestrian, speed,
vehicle and safety terms. PDFs for 2610.18417 and 2610.22306 were read with `curl` and
`pdftotext`; abstract pages for all three.

Refused from this session: nature.com and doi.org (proxy 403; `WebFetch` could not resolve the
host), frontiersin.org and its public-pages mirror, api.crossref.org, api.unpaywall.org,
ebi.ac.uk, bioengineer.org, ntd.com and zerohedge.com (proxy 403). Semantic Scholar answered once
with the Chu et al. metadata and then rate-limited (429).

## To fetch

- https://www.nature.com/articles/s44172-026-00793-3 (Chu et al., full text: headway definition,
  datasets, event counts and effect sizes)
- https://www.frontiersin.org/journals/future-transportation/articles/10.3389/ffutr.2026.1805850/full
  (Palermo study, risk functions and sample sizes)
- Carried from October 12 and still unread: https://doi.org/10.1016/j.aap.2026.108696 (Wu et al.)
  and https://doi.org/10.1177/03611981251327216 (Dean et al.)
