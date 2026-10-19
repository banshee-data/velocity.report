# Research briefing, October 12 to 19, 2026

- **Status:** Complete. No definition, benchmark kind or registered metric changed. Adopted: Chu,
  Gao, Xu and Liu (_Communications Engineering_, October 5) as the cited evidence for the
  behaviour plan's existing caveat that Waymo Open Motion is not a normative human-driving
  baseline, because traffic around an automated vehicle follows it differently. One open
  question for the owner. Revised October 19 after the owner supplied both journal full texts:
  Chu et al. and the Palermo study are now read in full, and no verdict changed. Corrected: the
  Waymo evidence in Chu et al. is weak, their headway is front to front, and the Palermo risks
  are averages that a few fast sites dominate.
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
safety literature could be read from this session. The owner later supplied the Chu et al. and
Palermo full texts, and both sections below were revised against them.

## Answer

Nothing changes a definition, a threshold, a benchmark kind or a registered metric. The behaviour
plan's Waymo Open Motion row in Section 11 already says the dataset is "collected around an
autonomous vehicle whose presence and behaviour influence the surrounding traffic". Chu et al.
now stand behind that sentence as its citation, read in full. Their evidence comes almost
entirely from Lyft's Palo Alto fleet. Their Waymo sample is 50 matched pairs, and its headway
difference is not significant, so the citation supports the general claim and not a measured
effect in Waymo data. Their headway is front to front and this plan's is a net gap, so their
numbers are not comparable with a site's. The paper raises one question for reports at sites that
automated fleets serve (Q1).

The roadside camera-and-radar paper validates its extent against templates that set the extent,
so its length errors measure the template, not the road user; it is not adopted, and it is
excluded as a method by Tenet 1. The sports tracker's finding that a hard Mahalanobis gate is
the binding constraint on association is not adopted: L5's gate is already a tuned parameter, and
the finding comes from a panning, zooming camera. A Palermo study applies impact-speed harm curves
to percentile travel speeds, which the crash-data plan already names as an upper bound; it is
logged and not adopted. Read in full, it uses a 2001 curve the plan does not list, and it measures
no injury outcome: crash sites are used only to choose road segments, so its positive
relationship between speed and harm comes from the curve and was not tested on crashes.

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

1. **Measures.** Read in full from the Article in Press. Car-following events in three datasets,
   with each event labelled follower–leader (AV–MV is an automated follower behind a manual
   leader). Lyft Level 5 is the main dataset: 20 automated vehicles on a fixed route in Palo Alto,
   in daytime, October 2019 to March 2020, in 25 s scenes at 10 Hz. After cleaning it gives 9,301
   AV–MV, 58,885 MV–AV and 35,566 MV–MV events in the position-based set, and 11,539, 59,536 and
   44,937 in the speed-based set. An event is the same lane, has no turn or lane change, lasts at
   least 15 s, has a gap under 85 m and starts with the follower above 1 m/s. Leaders are matched by
   the mean absolute difference of their speed traces over a sliding window (cut-off 0.30 m/s;
   0.20 m/s gives the same conclusions), and the results are compared by ANOVA and ANCOVA. The
   Waymo Open Dataset (the Perception set, reused from Hu et al. 2022, not Waymo Open Motion)
   produced one matching pair at the cut-off, so the 50 closest pairs were used instead. TGSIM
   supplies 28 AV–MV and 29 MV–AV events with cars under adaptive cruise control (SAE Level 1/2,
   driver on board). The Lyft and Waymo trajectories come from the automated vehicles' own
   sensors. For a manual vehicle these are far noisier than for the automated vehicle itself
   (position RMSE 1.84 m against 0.14 m; 48% of manual-vehicle jerk values exceeded 15 m/s³
   before cleaning). The paper does not say how TGSIM was collected.

   Headway is Eq. 4, THW = (X_L − X_F) / V_F. TTC (Eq. 3) subtracts the leader length L from the
   same position difference, so X is a front position and **THW is front-to-front headway**,
   computed at each 0.1 s step. "Critical" means THW < 2 s, citing Michael, Leeming and Dwyer
   (2000). In the speed-based Lyft data, human followers behind matched automated leaders had a
   mean THW of 1.959 s against 2.698 s behind matched manual leaders (Cohen's d = −0.67), with
   65.3% against 39.3% of time below 2 s. The position-based set agrees. Mean TTC and minimum TTC
   were smaller only in the speed-based set (d ≈ −0.1). Waymo, 50 pairs: 2.476 s against
   2.728 s, p = 0.236, which is not significant; only mean TTC reached p = 0.041. TGSIM: 3.819 s
   against 7.107 s, d = −0.93, with no TTC difference. Authors from Zhejiang University and
   Chang'an University. Code and the extracted events are on Zenodo
   (doi:10.5281/zenodo.21783161).

2. **Maths.** The direction holds in Lyft at both cut-offs and under robust, paired and ANCOVA
   variants. Four limits matter more than the effect size:
   - **The Waymo evidence is weak.** One of three datasets contributes 50 hand-relaxed pairs and a
     headway difference that is not significant. The abstract's "similar changes appear in the
     other two datasets" holds for TGSIM's headway and not for Waymo's.
   - **Measurement is not symmetric.** In MV–AV events the human follower is sensed by the
     leader's rear-facing sensors. In MV–MV events both vehicles are sensed at a distance. Noise
     in the manual-vehicle trajectories differs between the groups. The paper smooths the data
     (outlier replacement, then a Daubechies-6 wavelet with all detail coefficients dropped)
     but does not test whether sensing geometry alone could move headway.
   - **Matching leaves the cause open.** The paper uses TGSIM, whose cars look like any other, to
     argue that the effect is behavioural rather than visual. Its mediation analysis finds that the
     leader's speed and acceleration variability explain part of the headway difference, and
     the authors say plainly that variability "do[es] not fully account" for it. That leaves risk
     adaptation and drivers' attitudes to automated vehicles open. Their own literature disagrees:
     Wen et al. (2022) and Wang et al. (2025) found shorter headways behind Waymo vehicles; Hu et
     al. (2023) found no difference; and Jiao et al. (2024) give alternative explanations. That
     is a contested result, not a settled one.
   - **The sample is dated.** Lyft data is from 2019–2020 at low speed (followers around 11 m/s),
     and the authors say it may not represent current automated driving.
3. **Sensor.** The sensor sees passages, not who or what drives them. It cannot tell an automated
   leader from a manual one in the general case, and would not try: a vehicle's operator is not an
   observable this project records. What it can see is a leader's speed trace, which is the
   quantity the paper matched on. So the paper's lesson for a site is about population, not
   classification: where automated fleets operate, a share of observed following passages has an
   automated leader, and that share is unknown to the report. Their headway is not this plan's.
   The plan's `THW` is a net time gap ([Section 8.3](../../../plans/lidar-behaviour-analytics-plan.md#83-following-behaviour)),
   shorter than front-to-front headway by about L/V_F. For a 4.5 m car at 11 m/s that is about
   0.4 s, which is the same size as the effect. A site's net-gap share below 2 s must never be
   set beside the paper's 39% or 65%.
4. **Lands.** The behaviour plan's Section 11 table already says of Waymo Open Motion: "Not a
   normative human-driving baseline: it is collected around an autonomous vehicle whose presence
   and behaviour influence the surrounding traffic". That row had no citation for the influence.
   `grep` finds no other mention of automated leaders. The registry's
   `interaction.following_net_time_gap_s` (alias `time_headway`) is untouched; this is a
   population note, not a definition. The paper's "critical THW < 2 s" is the two-second rule
   used as a threshold. The plan already says that rule is driver education and that the 2.0 s
   band is labelled `no_established_threshold`; that still holds.
5. **Verdict.**

   | Claim                                                          | Verdict                                                                                                                                                  |
   | -------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
   | Human followers keep shorter headways behind automated leaders | **New and adopted** as the citation for the existing Waymo Open Motion caveat, read in full; evidence mainly from Lyft, Waymo difference not significant |
   | Automated followers keep longer headways than human ones       | Already holds: the same row's reason for not treating the dataset as a human baseline                                                                    |
   | Leader type as a stratum of a site's following distribution    | **Open question** Q1, as a limitations sentence only; the sensor cannot observe the stratum                                                              |
   | THW < 2 s as "critical" or "dangerous"                         | Not adopted: the plan labels the 2.0 s band `no_established_threshold`, and the paper's headway is front to front, not a net gap                         |
   | "Riskier" and "safer" as descriptions of followers             | Not adopted: verdict words; the plan describes headway distributions, never a driver's conduct                                                           |

6. **Rights.** CC BY-NC-ND 4.0, confirmed on the article. Citation with credit and short
   quotation are within it; no figure is reproduced and no adapted material is made.
7. **What a survey could give back.** A site cannot test the paper's claim, because it cannot
   label leaders. It can test the weaker statement the claim implies for any mixed stream: that
   follower headway behind leaders with low speed variance differs from headway behind leaders
   with high variance, holding leader speed. The paper's mediation model is exactly that test,
   with the leader's speed and acceleration standard deviations as mediators. A site could copy
   its design, using net gap rather than front-to-front headway. It stays a research-note
   hypothesis over observed pairs, after the behaviour plan's B1 gate is met; it is not proposed
   as work.

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

1. **Measures.** Read in full (Yasir, Logan, Migliore, D'Orso, Pervez and Koppel). The study uses
   141 police-reported pedestrian crashes in Palermo from 2015 to 2022: 54 fatal and 87 serious.
   Each crash location is linked to TomTom Move probe speeds for its road segment. For each crash,
   the speeds come from the one-hour interval of the crash time, pooled over one month of all
   days. That is 29,208 route observations at fatal sites (median 358 per site) and 41,938 at
   serious sites. The 15th, 50th and 85th percentile speeds are used "as proxies for impact speed"
   in Davis (2001), an ordered-logit pedestrian injury model, with the 15–59 age-group
   parameters (a2 = 8.866, a1 = 4.970, b = 0.127; x in km/h). The counterfactuals shift every
   percentile down by 3 or 5 km/h. The paper reports a mean fatal risk of 0.7%, 2.6% and 8.2%
   at the three percentiles, and a mean serious-injury risk of 13.0%, 26.8% and 43.6%.
   Shifting speeds down by 5 km/h lowers mean fatal risk by 46%, 40% and 33%.
2. **Maths.** Re-derived from Eq. 1 and 2, three findings matter:
   - **The averages are driven by a few fast sites.** The fatal curve evaluated at the mean
     speeds (20.1, 30.5 and 41.5 km/h) gives 0.18%, 0.67% and 2.67%. The reported mean risks are
     about three to four times higher because a few fast sites dominate them. The paper's
     Viale Ercole site has an 85th percentile of 92 km/h, where the curve gives 94%. The
     serious-injury curve turns over at 40 km/h (50.5% at the mean speed against a 43.6% mean).
     So the headline percentages say more about a few roads than about a typical one.
   - **No crash outcome is tested.** The recorded injury severity is used only to split the
     sites into two groups. Neither group is compared with each other or with sites without
     crashes. So "a strong positive relationship between percentile speeds and FSI risk" is the
     monotonic risk curve itself, not an empirical finding.
   - **Upper bound, with a qualification.** Putting a travel speed into an impact-speed curve
     assumes no braking, and the authors acknowledge this. The plan's upper-bound rule holds for
     a vehicle whose travel speed was measured. A percentile of segment speeds is not the
     striking vehicle's speed (Kröyer 2015, which the paper cites), so here the bound is on a
     hypothetical vehicle at that percentile. The authors call their risks "comparative
     estimates", not absolute probabilities. Davis (2001) was fitted to older crash data, the
     kind Rosén, Stigson and Sander (2011) find biased toward severe outcomes, and the plan does
     not list it.
3. **Sensor.** A site measures travel-speed distributions at a point, with a real denominator over
   observed time. The probe percentiles here are a different instrument: their sample size is
   stated, but the share of the traffic it represents is not. The paper cites Torrisi et al.
   (2026), which compared TomTom floating-car data with fixed radar in an Italian city and found
   uneven probe coverage and some underestimation of speed. That is direct evidence on how a
   roadside instrument compares with probe percentiles; it is in the to-fetch list. The method is
   one a report could imitate, which is why the plan forbids it unqualified.
4. **Lands.** The crash-data plan's harm-curve row already says the curves give harm at
   **impact** speed, and that "applied to measured speeds the curve is an upper bound, and the
   report says so". Already holds. The plan's curves are Tefft 2013, Rosén and Sander 2009,
   Hussain et al. 2019 and Monfort and Mueller 2025. This paper adds no reason to add Davis 2001.
   A report built the same way should also say how much of a mean risk the fastest sites supply.
   That is a presentation note for the harm-paragraph backlog item, not a new rule.
5. **Verdict.** Not adopted, and not added to the plan's sources: it applies an older curve the
   plan does not use to one city's probe percentiles, and it tests no outcome.
6. **Rights.** CC BY 4.0, confirmed on the article; citation with credit.
7. **Survey.** A site could reproduce the calculation, labelled as an upper bound, from its own
   percentiles; the plan already specifies that and nothing new is proposed.

## What changes

In this change:

- [Behaviour analytics plan](../../../plans/lidar-behaviour-analytics-plan.md): the Waymo Open
  Motion row in Section 11 cites Chu et al. for the influence of automated vehicles on the traffic
  around them; this briefing is linked under "Weekly literature review".
- A devlog bullet.

Nothing in code, the metrics registry, benchmark kinds or definitions.

## Open questions

| ID  | Question                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| --- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Q1  | Should the headway report's limitations text say, for a site within an automated fleet's service area, that an unknown share of observed following passages may have an automated leader, and that published evidence (Chu et al. 2026) finds followers keep shorter headways behind such leaders? The sensor cannot observe the share and must not try; the sentence would be a population note, not a stratum. If adopted, it cites the direction only. Chu et al. measure front-to-front headway on Lyft data from 2019–2020, so no figure of theirs belongs beside a site's net gap. |

The October 11 briefing's three questions remain open on its own branch and are not repeated.

## Sources checked

| Source                                                                             | Status                                                                           |
| ---------------------------------------------------------------------------------- | -------------------------------------------------------------------------------- |
| arXiv 2610.18417, Agrawal, Bhanderi and Elger                                      | Read                                                                             |
| arXiv 2610.22306, Adari, ARC-SORT                                                  | Read in part (abstract, introduction, Figure 2, tables)                          |
| arXiv 2610.14544, InfraWorld                                                       | Read in part (abstract)                                                          |
| Chu, Gao, Xu and Liu, _Communications Engineering_, doi:10.1038/s44172-026-00793-3 | Read (full text supplied by the owner, Article in Press; supplementary not read) |
| Yasir et al., Palermo study, doi:10.3389/ffutr.2026.1805850                        | Read (full text supplied by the owner)                                           |
| Waymo and MADD coverage of the sober-baseline paper, October 8 and 14              | Superseded (the preprint is read in full in the analysis)                        |
| Waymo safety hub, 270 million miles through June 2026                              | **Confirm** (search summary; automated-driving rate, out of scope)               |
| AAA Foundation, _Attitudes Toward Speeding_, August 2026                           | Superseded (held by the attitudes plan)                                          |
| IIHS and HLDI, speed safety cameras sheet, October 2026                            | Superseded (read in the October 12 briefing)                                     |
| NHTSA early estimate, first quarter of 2026; NSC first half of 2026                | **Confirm** (search summaries; FARS remains the plan's fatal source)             |
| arXiv past-week listings, cs.RO, cs.CV, eess.SP                                    | Read (titles filtered; abstracts read for the three above)                       |

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

Revision, October 19: the owner downloaded both journal PDFs into the paper intake folder on the
lidar volume (`ffutr-7-1805850.pdf`, `s44172-026-00793-3_reference.pdf`). They were read with
`pdftotext -layout`, and the Davis (2001) risks at the reported mean speeds were recomputed from
the paper's Eq. 1 and 2. The PDFs are not committed.

## To fetch

Ordered by what each would settle. None is held in `references.bib` unless marked.

Following and headway (Chu et al. follow-ups):

- Jiao, Li, Calvert, van Cranenburgh and van Lint 2024, _Transportation Research Part C_ 164,
  104673: alternative explanations for shorter headways behind automated vehicles. This is the
  main rival reading of the Chu et al. result and the first to read before Q1 is decided.
- Michael, Leeming and Dwyer 2000, _Transportation Research Part F_ 3, 55–64: observed headway on
  urban streets. It is the source of Chu et al.'s 2 s "critical" cut-off and a roadside
  observational headway study, the closest prior art to this sensor's following measurement.
  Check its headway definition against the plan's net gap.
- Chu et al. supplementary information and the Zenodo deposit
  (https://doi.org/10.5281/zenodo.21783161): Suppl. Tables 15–21 (Waymo and TGSIM) and the
  extracted events, from which a net-gap version of their headway could be recomputed with the
  leader length.
- Hu, Zheng, Chen, Zhang and Sun 2022, _Transportation Research Part C_ 134, 103490: processing
  and quality of Waymo Open Dataset trajectories. It bears on the plan's Section 11 use of Waymo
  data, and it recommends against Waymo's speed-based trajectories.
- Punzo, Borzacchiello and Ciuffo 2011, _Transportation Research Part C_ 19, 1243–1262: trajectory
  accuracy checks (jerk magnitude and sign-inversion tests). Compare with the plan's Section 8.2.1
  on jerk before L5 trajectories are quality-scored the same way.
- Ammourah et al. 2025, _Transportation Research Record_ 2679, 1768–1784: how TGSIM was
  collected. If it is an infrastructure or overhead dataset, it is a candidate evaluation set for
  Section 11.
- Wen, Cui and Jian 2022, _Accident Analysis & Prevention_ 172, 106689, and Hu, Zheng, Chen and
  Sun 2023, _IEEE T-ITS_ 24, 6711–6724: the earlier Waymo results that disagree with each other.

Harm curves and speed data (Palermo follow-ups):

- Kröyer 2015, _IATSS Research_ 39, 42–50: pedestrian injury severity against **travel** speed, not
  impact speed. This sensor measures travel speed, so this may be the most directly applicable
  curve for the crash-data plan's harm paragraph.
- Rosén, Stigson and Sander 2011, _Accident Analysis & Prevention_ 43, 25–33: review of fatality-
  risk curves and the sampling bias in older ones. It decides whether a curve like Davis (2001)
  should ever be printed. The repository holds Rosén and Sander 2009, not this.
- Torrisi et al. 2026, _Research in Transportation Business & Management_ 64, 101565
  (doi:10.1016/j.rtbm.2025.101565): TomTom floating-car data against fixed radar in an Italian
  city. Evidence on how probe percentiles compare with a roadside instrument.
- Davis 2001, _Transportation Research Record_ 1773, 108–113: the curve itself and the data it was
  fitted to, only if the harm-paragraph work needs to rule it out explicitly.

Carried from October 12 and still unread: https://doi.org/10.1016/j.aap.2026.108696 (Wu et al.)
and https://doi.org/10.1177/03611981251327216 (Dean et al.)
