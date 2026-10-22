# Research briefing, October 12 to 19, 2026

- **Status:** Complete. No definition, benchmark kind or registered metric changed. Adopted: Chu,
  Gao, Xu and Liu (_Communications Engineering_, October 5) as the cited evidence for the
  behaviour plan's existing caveat that Waymo Open Motion is not a normative human-driving
  baseline, because traffic around an automated vehicle follows it differently. One open
  question for the owner. Revised October 19 after the owner supplied both journal full texts:
  Chu et al. and the Palermo study are now read in full, and no verdict changed. Corrected: the
  Waymo evidence in Chu et al. is weak, their headway includes a vehicle length, and the Palermo
  risks are averages that a few fast sites dominate. A second revision the same day read five
  follow-up papers the owner supplied (Jiao et al. 2024, Michael et al. 2000, Kröyer 2015, Rosén
  et al. 2011, Torrisi et al. 2026) and added the papers read in full to `references.bib`. A third
  revision read Punzo et al. 2011 and Ammourah et al. (TGSIM), explored Q2, logged Q3 as a
  candidate rename and applied Q4. Jiao et
  al. make the Chu et al. result contested, not settled, and three new open questions follow.
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
effect in Waymo data. Jiao et al. (2024), on the same Lyft data, attribute most of the headway
difference to how the data were collected rather than to drivers, so the citation now names both.
Their headway includes a vehicle length and this plan's is a net gap, so their numbers are not
comparable with a site's. The paper raises one question for reports at sites that
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
   before cleaning). TGSIM was recorded from a helicopter and from overpass and rooftop cameras,
   with each position at the front bumper (Ammourah et al. 2024, read below), so its headways
   are front to front.

   Headway is Eq. 4, THW = (X_L − X_F) / V_F, computed at each 0.1 s step. It is a **gross
   headway: the position difference includes a vehicle length**. TTC (Eq. 3) subtracts the
   leader length L from the same difference, which treats X as a front position. But Lyft
   positions are 3D box centroids (Jiao et al. 2024, Section 2.1), so Eq. 4 is centroid to
   centroid. That equals front to front only when the two vehicles are the same length.
   "Critical" means THW < 2 s, citing Michael, Leeming and Dwyer (2000), who present the 2 s
   figure as a driver-training rule, not a measured threshold. In the speed-based Lyft data, human followers behind matched automated leaders had a
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
     leader's rear-facing sensors. In MV–MV events both vehicles are sensed at a distance. The
     paper smooths the data (outlier replacement, then a Daubechies-6 wavelet with all detail
     coefficients dropped) but does not test whether sensing geometry alone could move headway.
     Jiao et al. (2024) show the asymmetry in the same Lyft data. The lengths detected for
     vehicles directly behind an automated vehicle cluster at 4.0 and 5.0 m instead of following
     the normal spread of passing vehicles. Their stationary length sums average 9.25 m behind
     an automated vehicle against 8.79 m between two manual vehicles.
   - **Matching leaves the cause open.** The paper uses TGSIM, whose cars look like any other, to
     argue that the effect is behavioural rather than visual. Its mediation analysis finds that the
     leader's speed and acceleration variability explain part of the headway difference, and
     the authors say plainly that variability "do[es] not fully account" for it. That leaves risk
     adaptation and drivers' attitudes to automated vehicles open. Their own literature disagrees:
     Wen et al. (2022) and Wang et al. (2025) found shorter headways behind Waymo vehicles; Hu et
     al. (2023) found no difference. Jiao et al. (2024), read in the follow-up section below,
     reach the opposite reading from the same Lyft fleet: observation bias plays a "remarkable
     (probably primary)" role. Chu et al. match on leader speed, which partly controls Jiao's
     first factor (the mix of following states). It does not control the second: the automated
     leaders are 20 cars running one algorithm, while the manual leaders are tens of thousands of
     different drivers. Chu et al. cite Jiao et al. only in passing. That is a contested result,
     not a settled one.
   - **The sample is dated.** Lyft data is from 2019–2020 at low speed (followers around 11 m/s),
     and the authors say it may not represent current automated driving.
3. **Sensor.** The sensor sees passages, not who or what drives them. It cannot tell an automated
   leader from a manual one in the general case, and would not try: a vehicle's operator is not an
   observable this project records. What it can see is a leader's speed trace, which is the
   quantity the paper matched on. So the paper's lesson for a site is about population, not
   classification: where automated fleets operate, a share of observed following passages has an
   automated leader, and that share is unknown to the report. Their headway is not this plan's.
   The plan's `THW` is a net time gap ([Section 8.3](../../../plans/lidar-behaviour-analytics-plan.md#83-following-behaviour)),
   shorter than gross headway by about L/V_F. For a 4.5 m car at 11 m/s that is about
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

   | Claim                                                          | Verdict                                                                                                                                                                                                  |
   | -------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
   | Human followers keep shorter headways behind automated leaders | **New and adopted** as the citation for the existing Waymo Open Motion caveat, read in full; evidence mainly from Lyft, Waymo difference not significant; contested by Jiao et al. 2024, cited beside it |
   | Automated followers keep longer headways than human ones       | Already holds: the same row's reason for not treating the dataset as a human baseline                                                                                                                    |
   | Leader type as a stratum of a site's following distribution    | **Open question** Q1, as a limitations sentence only; the sensor cannot observe the stratum                                                                                                              |
   | THW < 2 s as "critical" or "dangerous"                         | Not adopted: the plan labels the 2.0 s band `no_established_threshold`, and the paper's headway is gross, not a net gap                                                                                  |
   | "Riskier" and "safer" as descriptions of followers             | Not adopted: verdict words; the plan describes headway distributions, never a driver's conduct                                                                                                           |

6. **Rights.** CC BY-NC-ND 4.0, confirmed on the article. Citation with credit and short
   quotation are within it; no figure is reproduced and no adapted material is made.
7. **What a survey could give back.** A site cannot test the paper's claim, because it cannot
   label leaders. It can test the weaker statement the claim implies for any mixed stream: that
   follower headway behind leaders with low speed variance differs from headway behind leaders
   with high variance, holding leader speed. The paper's mediation model is exactly that test,
   with the leader's speed and acceleration standard deviations as mediators. A site could copy
   its design, using net gap rather than gross headway. It stays a research-note
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
     estimates", not absolute probabilities.
   - **The curve is sound but dated.** Davis (2001) refits Ashton's 1970s Birmingham crashes,
     weighted to the UK's 1976 severity proportions. Rosén, Stigson and Sander (2011) count it
     among the few bias-adjusted curves (about 7% fatal at 50 km/h for ages 15–59) and call it
     "fairly reliable" for crashes of the 1960s and 70s. For crashes in the 2000s they point to
     Rosén and Sander (2009), which the plan already uses. Applying the 1976 fleet and medical
     care to Palermo in 2015–2022 is the paper's real weakness, and the plan does not list
     Davis (2001).
3. **Sensor.** A site measures travel-speed distributions at a point, with a real denominator over
   observed time. The probe percentiles here are a different instrument: their sample size is
   stated, but the share of the traffic it represents is not. Torrisi et al. (2026), which the
   paper cites and which is read below, measure that share at 14 radar sections in Catania: a
   median of about 3.5% of vehicles. Probe and radar P50 and P85 agreed within about 1 km/h on
   clean segments, but differed by up to 21 km/h where the probe segment ran through a junction.
   Pedestrian crashes cluster at junctions. The method is one a report could imitate, which is
   why the plan forbids it unqualified.
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

## Follow-up reading, October 19

The owner supplied seven papers from the first revision's to-fetch list, in two batches. Each
was read in full. Four bear on the following metric, three on the harm curves and speed data.

### Jiao, Li, Calvert, van Cranenburgh and van Lint: alternative explanations for shorter headways (_Transportation Research Part C_ 164, 104673, 2024)

1. **Measures.** The same Lyft Level 5 fleet as Chu et al. (20 vehicles, Palo Alto, October 2019
   to March 2020), through the car-following set of Li et al. (2023). The baseline difference in
   per-case minimum time headway, human behind automated against human behind human, is 0.61 s,
   and the spread is 0.48 s narrower. Three alternatives to a change in drivers are tested.
   - **Following states.** Each time step is labelled with one of seven regimes (cruising,
     following, accelerating, decelerating, free acceleration, free deceleration, stationary).
     Restricting to constant-speed following cuts the difference from 0.61 s to 0.33 s (1.46 s
     against 1.79 s). Free-following regimes are 20.0% of human–human minimums and 9.2% of
     human–automated ones.
   - **Leader homogeneity.** Calibrated Intelligent Driver Models of 1,227 human followers are
     made to follow one human leader or 1,227 different human leaders. One leader shortens the
     mean minimum headway by 0.275 s (Gipps' model: 0.31 s), with no automation involved.
   - **Leader characteristics.** An LSTM classifier picks out automated leaders that drive like
     humans. Followers keep 0.10 to 0.13 s longer headways behind those than behind automated
     leaders that drive like machines.

   Headway is centroid to centroid divided by follower speed, because lengths detected behind an
   automated vehicle are unreliable. Code is public. CC BY 4.0.

2. **Maths.** The three effects are not additive, and the authors say so. The simulation result
   is the strongest: it isolates leader homogeneity in human-only data. The paper does not rule
   out a change in drivers, and it cites a "Wizard of Oz" study (Soni et al. 2022) in which
   drivers told the leader was automated did not follow closer. Together with Chu et al.'s TGSIM
   argument, the evidence points to how the leader drives, not what drivers believe about it.
3. **Sensor.** Two lessons transfer directly. First, a headway distribution depends on the mix
   of following states in it, so a site's distribution is only comparable with another's if
   free following is excluded the same way. Second, a homogeneous leader population shortens and
   narrows follower headways on its own. A site whose leaders are mostly one kind (a bus route, a
   delivery fleet, a platoon behind a signal) will show this without any change in drivers.
4. **Lands.** The plan's `following_valid_time` is "time both parties were in view, same path,
   `F` behind `L`" ([Section 8.3](../../../plans/lidar-behaviour-analytics-plan.md#83-following-behaviour)).
   It has no constrained-following test, so free following is counted (Q2).
5. **Verdict.** **New and adopted** as the counter-citation beside Chu et al. in the Waymo Open
   Motion row. The following-state finding is raised as Q2.

### Michael, Leeming and Dwyer: headway on urban streets (_Transportation Research Part F_ 3, 55–64, 2000)

1. **Measures.** Roadside video of all lanes on three urban arterials in Memphis, autumn 1998, in
   free-flowing traffic only. Headway was coded by hand with mouse clicks, front of leader to
   front of follower past a point (the Evans 1991 definition). About 25,000 headways were
   recorded and 12,393 below 4 s kept. Following the "leader" definition of Wasielewski (1979),
   vehicles 4 s or more behind are treated as not following. Headways of 400 ms or less are
   stated to be inaccurate. 49.4% of followers kept 2 s or more, and 933 (7.5%) kept under 1 s.
   A hand-held "Help Prevent Crashes Please Don't Tailgate" sign raised the mean from 2.11 s to
   2.29 s (effect size 0.21). A sign without the word "crashes" had no effect.
2. **Maths.** Means and shares only. All differences by location, time of day and lane were
   "small" effect sizes. The 2 s figure is introduced as what driver-training programmes teach,
   not as a measured threshold.
3. **Sensor.** This is the closest prior art to the sensor's following measurement: a fixed
   roadside observer, free flow, all lanes, a following cut-off. It measures gross passage
   headway at a point, while the plan measures net gap over a path, so its numbers are not
   comparable either. It is the source Chu et al. cite for the 2 s "critical" threshold, and it
   supports the plan's line that the two-second rule "is driver education".
4. **Lands.** The plan's two-second paragraph and `no_established_threshold` label already hold.
   Its 4 s cut-off is one answer to Q2.
5. **Verdict.** Cited, not adopted as a threshold. Copyright Elsevier; citation only.

### Kröyer: injury severity and mean travel speed (_IATSS Research_ 39, 42–50, 2015)

1. **Measures.** Swedish STRADA pedestrian crashes, 2004–2008. Dataset 1 has 8,166 crashes
   (272 fatal). Dataset 2 is 79 crashes drawn from Scania by severity (31 minor, 29 severe, 19
   fatal), daytime, dry, run-over crashes excluded. At each dataset 2 site, the spot speeds of
   free-flowing vehicles on the crash vehicle's approach were measured (at least 59 per site).
   A weighted multinomial logit relates severity to site mean speed and age. Mean site speeds:
   36.6 km/h for minor, 40.5 for severe and 48.9 for fatal crashes. Fatal crashes were rare
   below a site mean of about 40 km/h (1 of 19). About 31% of severe injuries occurred at sites
   with mean speeds below 35 km/h.
2. **Maths.** Weighted for the outcome-based sampling. Mean speed separates fatal from minor
   crashes (p = 0.004) but not minor from severe (p = 0.114). The author attributes this partly
   to the crash vehicle's speed departing from the site mean. With 19 fatal crashes, the
   intervals are wide, and the speed effect per km/h (−0.086 in the logit for minor against
   fatal) is not a curve a report could print.
3. **Sensor.** Its independent variable is the one this sensor measures: mean free-flow speed
   at a fixed point on a site's approach. That makes it the most directly matched study in the
   literature read so far, and the weakest statistically. It also states the point the Palermo
   section makes: a site's speed distribution relates to the striking vehicle's speed only
   probabilistically.
4. **Lands.** The crash-data plan's harm curves are all impact-speed curves. Kröyer is not a
   replacement; it is the evidence for how travel speed relates to outcome, which the plan's
   upper-bound caveat currently states without a source (Q4).
5. **Verdict.** Cited as context, not adopted as a curve. Copyright IATSS; citation only.

### Rosén, Stigson and Sander: review of pedestrian fatality-risk curves (_Accident Analysis & Prevention_ 43, 25–33, 2011)

1. **Measures.** A review of every published curve of pedestrian fatality risk against car
   impact speed, judged on sampling and statistics. Curves fitted directly to samples biased
   toward severe crashes (Ashton 1980, Pasanen 1992, Anderson et al., Oh et al.) give 35–90%
   fatality at 50 km/h. Bias-adjusted ones (Davis 2001, Cuerden et al. 2007, Hannawald and Kauer
   2004, Rosén and Sander 2009) give about 10%.
2. **Maths.** Two cautions apply to any curve the plan prints. Random error in impact speed
   flattens a curve and systematic error shifts it. And every published risk is conditional on
   the pedestrian being injured at all.
3. **Sensor.** None directly. It confirms that the plan's chosen curves are the bias-adjusted
   ones and that Davis (2001) is reliable for a 1970s fleet only.
4. **Lands.** The crash-data plan already uses Rosén and Sander (2009) and Tefft (2013). Already
   holds; this review is the reason to keep Davis (2001) and Pasanen (1992) out.
5. **Verdict.** Cited in the Palermo section. Copyright Elsevier; citation only.

### Torrisi, Calabrò, Oliveira, Inturri, Cavalieri and Ignaccolo: floating-car data against fixed radar (_Research in Transportation Business & Management_ 64, 101565, 2026)

1. **Measures.** Fourteen MobilTraf300 radar sections on Catania's entry and exit roads (50 km/h
   limit), the whole of 2023, against TomTom Traffic Stats for the matching segments, aggregated
   to hours. Probe vehicles were a median 3.5% of radar counts (about 1% at 05:00, 4.7% at
   20:00), higher on lower-volume roads. Probe speeds ran lower: 34% of probe speeds exceeded
   50 km/h against 46% of radar speeds. The mean-speed bias was mostly −9 to +1 km/h (median
   about −5). P50 and P85 differences were within about 1 km/h. One section just past a signal
   showed −21 km/h, because the probe segment included the queue.
2. **Maths.** The radar speeds were available only as hourly 10 km/h histograms, and percentiles
   were reconstructed from bin midpoints. Percentile agreement within 1 km/h is finer than that
   resolution, so it shows the distributions are aligned, not that either instrument is accurate
   to 1 km/h. The radar is treated as ground truth without its own validation, and speeds above
   110 km/h were dropped as unreliable.
3. **Sensor.** This is the comparison a site report would face: a point instrument against probe
   percentiles. On clean segments the medians and 85th percentiles agree. Shares above the limit
   and means do not, and junction segments do not at all. A report should never set its share
   above the limit against a probe figure.
4. **Lands.** No plan compares site speeds with probe data. Recorded for when one does.
5. **Verdict.** Recorded, not adopted. CC BY-NC-ND 4.0; citation only.

### Punzo, Borzacchiello and Ciuffo: checking trajectory accuracy (_Transportation Research Part C_ 19, 1243–1262, 2011)

1. **Measures.** A method for checking the accuracy of vehicle trajectory data without ground
   truth, applied to every NGSIM dataset (I-80, US-101, Lankershim, Peachtree). It has three
   tests:
   - **Jerk.** The share of samples above 15 m/s³ (beyond what a vehicle can do), and the share
     of 1 s windows with more than one change of sign in jerk.
   - **Platoon consistency.** Each vehicle's trajectory is integrated from its speeds and the
     inter-vehicle spacing this implies is compared with the observed one. Spacing below 5 m,
     front to front, means the trajectories have the cars colliding.
   - **Spectrum.** Energy above about 1 Hz in speed and acceleration, where human and vehicle
     responses do not reach, is treated as noise.

   NGSIM failed all three: 6.5–17.4% of jerk values above 15 m/s³, and 74.6–94.3% of 1 s windows
   with more than one sign change.

2. **Maths.** Differentiation amplifies noise, so any speed or acceleration taken from positions
   needs filtering that keeps the integrated positions consistent with the measured spacing.
   Internal consistency of one vehicle (speed integrates back to position) is not enough; the
   pair must agree too. Chu et al. took their 15 m/s³ and sign-inversion thresholds from this
   paper.
3. **Sensor.** The plan already refuses raw finite-difference jerk (Section 8.2.1: about
   28 m/s³ of noise at 0.2 s spacing). At 5 Hz a 1 s window holds five samples, so the
   sign-inversion test needs adapting. The transferable test is platoon consistency. The
   following metric's gap comes from two tracks' positions, and a check that it agrees with the
   gap implied by integrating both tracks' speeds would catch a biased pair before it reaches a
   rate. The plan's `non_positive_gap` reason is the crude form of that check.
4. **Lands.** Section 8.2.1 (jerk) and the following fixtures in Section 8.3. Nothing changes
   now; this is a candidate quality test for L5 trajectories and following pairs.
5. **Verdict.** Cited, not adopted. Copyright Elsevier; citation only.

### Ammourah et al.: how TGSIM was collected (_Transportation Research Record_ 2679, 1768–1784, 2025; online 2024)

1. **Measures.** Six trajectory datasets of SAE Level 1–3 vehicles among human traffic, made by
   three methods:
   - **Helicopter.** Hovering or following the automated vehicles at 700–1,000 ft, 8K video at
     30 fps, on I-90/I-94 and I-294 near Chicago, including a three-car ACC platoon.
   - **Overpass cameras.** On I-395 in Washington, D.C., 1080p at 30 fps over 0.34 mi.
   - **Rooftop cameras.** Twelve cameras on five buildings over four blocks of Foggy Bottom,
     Washington, D.C., with a Level 3 vehicle, pedestrians, cyclists and scooters, three hours
     of evening peak.

   Detection uses a pre-trained CNN detector and tracker on stabilised frames. Each vehicle's
   position is its front bumper, a Kalman filter smooths it, and camera hand-offs are matched by
   hand. Every trajectory was inspected, and zero-speed, short and wrong-way ones were removed.
   The automated vehicles carried GPS and IMUs as ground truth, but the paper reports no
   comparison of video against GPS and no accuracy figure. A run yields 400–500 trajectories.
   The ACC platoon drove near the limit, slower than the traffic around it, so its immediate
   follower kept changing. That fits Chu et al.'s 29 human-behind-automated events.

2. **Maths.** None beyond the pipeline: no error statistics, which is the gap Punzo et al. (2011)
   say every trajectory dataset should close before use.
3. **Sensor.** The Foggy Bottom set is the closest public dataset read so far to what this
   sensor sees: fixed, infrastructure-mounted, urban intersections, with vulnerable road users.
   It is camera-based, and its accuracy is unquantified. The GPS on the automated vehicles is a
   physical reference that could be used to quantify it, if the GPS logs are released.
4. **Lands.** The behaviour plan's Section 11 evaluation datasets. It is a candidate for
   geometry and interaction structure, with the same caveat as Waymo Open Motion: automated
   vehicles in the stream, and accuracy to be measured before use. Open via the U.S. DOT ITS
   DataHub.
5. **Verdict.** Recorded as a Section 11 candidate, not added to the plan until its accuracy is
   checked. SAGE copyright with author reuse terms; citation only.

## Q2 explored: a speed floor and a time ceiling for valid following

The owner's starting point (October 19) is a speed floor of about 2 mph and an upper time bound
near 6 s, perhaps lower, with the options still to be explored. This section sets out what each
choice would change. It decides nothing; the limits belong in the following record that
[l8fol0004](../../../../data/experiments/l8fol/l8fol0004.md) pins before any held-out window is
read.

**What the code does now.** `FollowingParams.SpeedFloorMps` suppresses net time gap below a
follower speed (`below_speed_floor`). It has no default; the fixtures use 0.5 m/s, and the plan
places the measurement noise floor near 0.3 m/s at 5 Hz (Section 8.11). Standstill already does
not count as following opportunity, but its spatial gap stays valid. Leader choice is bounded in
space, not time: `PairingParams.MaxLeaderRangeM`, 60 m centre to centre in the fixtures. There
is no time ceiling, and `following_valid_time` has no test that the follower is constrained by
the leader. Both live in `internal/lidar/l8behaviour` (`following.go`, `pairing.go`).

**What the literature read here used.**

| Source              | Floor                                               | Upper bound                            | Headway kind |
| ------------------- | --------------------------------------------------- | -------------------------------------- | ------------ |
| Chu et al. 2026     | Follower above 1 m/s (2.2 mph) at the event's start | Gap under 85 m; event of at least 15 s | Gross        |
| Jiao et al. 2024    | Stationary below 0.1 m/s is its own regime          | Regime test (Newell time gap, DTW lag) | Gross        |
| Michael et al. 2000 | Free-flowing traffic only                           | 4 s, after Wasielewski (1979)          | Gross        |
| Plan, as built      | 0.5 m/s in fixtures (noise near 0.3 m/s)            | 60 m centre to centre                  | Net          |

**Repercussions of a speed floor near 2 mph (0.89 m/s).**

1. **It is a behavioural floor, not a measurement one.** At 5 Hz, speed uncertainty near the
   floor is about 0.2 to 0.35 m/s (Section 8.11). That is 22–39% of 0.89 m/s, and the net time
   gap inherits the whole relative error. At 4.5 mph (2 m/s) it is 10–17%; at 10 mph, 4–8%. So
   2 mph removes creep and standstill, but the time gaps just above it are still too noisy to
   publish as values.
2. **It should be a second parameter, not a raised `SpeedFloorMps`.** Two roles are in play: a
   measurement floor, where net time gap is undefined (keep near the noise, about 0.3–0.5 m/s);
   and an opportunity floor, below which time is queue or creep and does not count toward
   `following_valid_time`. The spatial gap stays valid there, which is how the code treats
   standstill already. Merging the two would hide which one suppressed an instant.
3. **Low-speed following is mostly queue discharge.** Between the floor and about 10 mph, gaps
   of 2–8 m give time gaps of 1–4 s that describe a queue, not a driver's chosen margin. One
   option is to report the spatial gap below a band edge (say 10 mph) and the time gap above it.
   Another is to keep both, with every distribution stratified by follower speed band; the plan
   already stratifies the observation budget by speed (Section 3).
4. **It moves the denominator.** Every instant below the floor leaves `following_valid_time`, so
   `thw_below_X_rate` changes even when no gap does. The floor belongs in the parameter hash
   and beside every published rate.

**Repercussions of an upper time bound.**

1. **The 60 m leader range already caps the time gap, but only at speed.** The implied ceiling
   is about (60 − 4.5) m / v:

   | Follower speed | Implied cap from the 60 m range | L/v (gross − net, 4.5 m car) |
   | -------------- | ------------------------------- | ---------------------------- |
   | 2 mph          | 62 s                            | 5.0 s                        |
   | 10 mph         | 12.4 s                          | 1.0 s                        |
   | 20 mph         | 6.2 s                           | 0.5 s                        |
   | 25 mph         | 5.0 s                           | 0.4 s                        |
   | 30 mph         | 4.1 s                           | 0.34 s                       |
   | 35 mph         | 3.5 s                           | 0.29 s                       |

   So a 6 s ceiling binds only below about 21 mph, a 4 s ceiling below about 31 mph, and a 3 s
   ceiling below about 41 mph. At typical urban speeds the choice between 4 and 6 s matters
   mainly in slower traffic. Above about 25 mph, the observed path (51–60 m median, Section 3)
   is the real limit, because both parties must be in view.

2. **The ceiling sets the rates.** For a ceiling C, the share below a band X is
   F(X) / F(C), where F is the time-gap distribution. Lowering C from 6 s to 4 s raises every
   `thw_below_X_rate` without any driver changing. This is the mechanism Jiao et al. measured:
   the mix of free and constrained following moved their comparison from 0.61 s to 0.33 s. A
   rate is only comparable with another at the same floor and ceiling, and both are printed
   with it.
3. **A per-instant ceiling flickers.** A pair hovering near C would enter and leave valid
   following every few frames, fragmenting the "maximal contiguous runs" the plan reports
   (Section 10). The usual remedy is hysteresis: enter below C, leave only after the time gap
   stays above C plus a margin (say 0.5 s) for a minimum dwell (say 1 s, five samples at 5 Hz).
   The same applies to the speed floor in stop-and-go traffic.
4. **Net against gross.** The cut-offs in the literature are on gross headway. A net-gap ceiling
   equal to a gross one admits more distant pairs by L/v: 0.4 s at 25 mph, but 1 s at 10 mph. A
   4 s gross convention is about 3.6 s net at 25 mph. If the ceiling is chosen to match Michael
   et al. or Wasielewski, say so and convert; otherwise call it a net-gap ceiling of our own.
5. **It is not a following test.** A time gap under C does not prove the follower is responding
   to the leader. In 10-second passages at 5 Hz, a response test like Jiao's (lagged speed
   correlation) has little to work with. A ceiling is a practical proxy, and the report should
   call it one.

**Options.**

| Option                                                                                                   | For                                                                          | Against                                                                      |
| -------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------- | ---------------------------------------------------------------------------- |
| A. Floor 2 mph, ceiling 6 s, hysteresis                                                                  | Wide; drops creep and plainly free following; close to Chu's 1 m/s           | Binds only below ~21 mph; noisy time gaps just above the floor still counted |
| B. Floor 2 mph, ceiling 4 s (converted to net), hysteresis                                               | Matches the roadside convention (Michael, Wasielewski), so rates compare     | Drops some genuine following in free flow; rates rise against option A       |
| C. Two floors (measurement ~0.4 m/s, opportunity 2 mph), ceiling 4 or 6 s, spatial gap only below 10 mph | Keeps queue behaviour as a spatial measure; time gap only where it is stable | Two reporting modes; more to explain; needs the band edge chosen             |
| D. Report at several ceilings (3, 4, 6 s) as a sensitivity                                               | Shows how much the answer depends on the choice                              | Not a definition: a report still needs one headline value                    |

**Recommendation for the l8fol0004 decision.** Option C with a 4 s or 6 s net ceiling chosen on
kirk0's reviewed pairs, with option D's sensitivity in the report. The draft following records
(`leader`, `no_leader`, `ambiguous`) are where the ceiling's effect can first be seen: count how
many reviewed `leader` records each candidate ceiling would drop. Record the floor, ceiling,
margin and dwell in the following record and the parameter hash. Until that decision, no rate is
published.

## What changes

In this change:

- [Behaviour analytics plan](../../../plans/lidar-behaviour-analytics-plan.md): the Waymo Open
  Motion row in Section 11 cites Chu et al. for the influence of automated vehicles on the traffic
  around them, with Jiao et al. beside it as the contested reading; this briefing is linked under
  "Weekly literature review".
- [Crash-data integration plan](../../../plans/platform-crash-data-integration-plan.md): the harm-curve
  row cites Kröyer (2015) for its travel-speed caveat, with a sources-table row (Q4).
- [SCHEMA_CHANGES.md](../../../SCHEMA_CHANGES.md): a new log of pending renames and schema changes
  across the repository, starting with the headway names (Q3); listed in
  [DOCS.md](../../../DOCS.md).
- [`references.bib`](../../../../data/maths/references.bib): nine entries for the papers read in
  full here (`Ammourah2025`, `Chu2026`, `Jiao2024`, `Kroyer2015`, `Michael2000`, `Punzo2011`,
  `Rosen2011`, `Torrisi2026`, `Yasir2026`), mapped in the [reference audit](reference-audit.md).
- A devlog bullet.

Nothing in code, the metrics registry, benchmark kinds or definitions.

## Open questions

| ID  | Question                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| --- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Q1  | Should the headway report's limitations text say, for a site within an automated fleet's service area, that an unknown share of observed following passages may have an automated leader, and that published evidence (Chu et al. 2026) finds followers keep shorter headways behind such leaders? The sensor cannot observe the share and must not try; the sentence would be a population note, not a stratum. If adopted, it cites the direction only. Chu et al. measure gross headway on Lyft data from 2019–2020, so no figure of theirs belongs beside a site's net gap, and Jiao et al. (2024) attribute much of the difference to observation bias, so the sentence should say the evidence is contested.                                                                                                                                                                                    |
| Q2  | **Decided** (owner, October 22, in the [maths decision review](../../../platform/operations/decision-review-maths-2026-10.md#e-01-q-01), E-01): option C with option D's sensitivity, as this section recommends; the 4 or 6 s ceiling and the 10 mph band edge are chosen on kirk0's reviewed pairs in l8fol0004. Explored October 19 (a floor near 2 mph and a ceiling near 6 s, perhaps lower). Should `following_valid_time` require constrained following? Jiao et al. (2024) show the mix of free and constrained following alone moved a headway comparison from 0.61 s to 0.33 s. The [Q2 section](#q2-explored-a-speed-floor-and-a-time-ceiling-for-valid-following) sets out the repercussions and four options, and recommends two floors and a 4 or 6 s net ceiling with hysteresis, decided on kirk0's reviewed pairs in l8fol0004's following record. No rate is published before then. |
| Q3  | **Decided: no rename yet** (owner, October 19). The `time_headway` alias of `interaction.following_net_time_gap_s` and the plan's `distance_headway` and `thw_*` names describe a net gap, while the literature uses headway for a gross measure about L/v longer. The candidate is logged in [SCHEMA_CHANGES.md](../../../SCHEMA_CHANGES.md#headway-names-on-a-net-gap), started here with the repository's other pending renames, to be considered before a report publishes these names.                                                                                                                                                                                                                                                                                                                                                                                                           |
| Q4  | **Decided yes** (owner, October 19). The [crash-data plan](../../../plans/platform-crash-data-integration-plan.md)'s harm-curve row now cites Kröyer (2015) for its travel-speed caveat, with a row in its sources table. It is context, not a curve to print.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |

The October 11 briefing's three questions remain open on its own branch and are not repeated.

## Sources checked

| Source                                                                                                | Status                                                                           |
| ----------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------- |
| arXiv 2610.18417, Agrawal, Bhanderi and Elger                                                         | Read                                                                             |
| arXiv 2610.22306, Adari, ARC-SORT                                                                     | Read in part (abstract, introduction, Figure 2, tables)                          |
| arXiv 2610.14544, InfraWorld                                                                          | Read in part (abstract)                                                          |
| Chu, Gao, Xu and Liu, _Communications Engineering_, doi:10.1038/s44172-026-00793-3                    | Read (full text supplied by the owner, Article in Press; supplementary not read) |
| Yasir et al., Palermo study, doi:10.3389/ffutr.2026.1805850                                           | Read (full text supplied by the owner)                                           |
| Jiao et al., _Transportation Research Part C_ 164, doi:10.1016/j.trc.2024.104673                      | Read (full text supplied by the owner)                                           |
| Michael, Leeming and Dwyer, _Transportation Research Part F_ 3, doi:10.1016/S1369-8478(00)00015-2     | Read (full text supplied by the owner)                                           |
| Kröyer, _IATSS Research_ 39, doi:10.1016/j.iatssr.2014.08.001                                         | Read (full text supplied by the owner)                                           |
| Rosén, Stigson and Sander, _Accident Analysis & Prevention_ 43, doi:10.1016/j.aap.2010.04.003         | Read (full text supplied by the owner)                                           |
| Torrisi et al., _Research in Transportation Business & Management_ 64, doi:10.1016/j.rtbm.2025.101565 | Read (full text supplied by the owner)                                           |
| Punzo, Borzacchiello and Ciuffo, _Transportation Research Part C_ 19, doi:10.1016/j.trc.2010.12.007   | Read (full text supplied by the owner)                                           |
| Ammourah et al., _Transportation Research Record_ 2679, doi:10.1177/03611981241257257                 | Read (full text supplied by the owner)                                           |
| Waymo and MADD coverage of the sober-baseline paper, October 8 and 14                                 | Superseded (the preprint is read in full in the analysis)                        |
| Waymo safety hub, 270 million miles through June 2026                                                 | **Confirm** (search summary; automated-driving rate, out of scope)               |
| AAA Foundation, _Attitudes Toward Speeding_, August 2026                                              | Superseded (held by the attitudes plan)                                          |
| IIHS and HLDI, speed safety cameras sheet, October 2026                                               | Superseded (read in the October 12 briefing)                                     |
| NHTSA early estimate, first quarter of 2026; NSC first half of 2026                                   | **Confirm** (search summaries; FARS remains the plan's fatal source)             |
| arXiv past-week listings, cs.RO, cs.CV, eess.SP                                                       | Read (titles filtered; abstracts read for the three above)                       |

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

Second revision, October 19: the owner supplied five more PDFs to the same folder
(`1-s2.0-S0968090X24001943-main.pdf`, `michael2000.pdf`, `10.1016@j.iatssr.2014.08.001.pdf`,
`10.1016@j.aap.2010.04.003.pdf`, `1-s2.0-S2210539525002822-main.pdf`), read with `pdftotext`.
Every DOI in this briefing and its to-fetch list was checked against Crossref on October 19.

Third revision, October 19: `punzo2011.pdf` and
`ammourah-et-al-2024-introduction-to-the-third-generation-simulation-dataset-data-collection-and-trajectory-extraction.pdf`
from the same folder, read with `pdftotext`. The Q2 figures (implied ceilings from the 60 m leader
range, relative speed error at the floor, L/v) are arithmetic from the stated inputs, not
measurements.

## To fetch

Every DOI below was checked against Crossref on October 19. None is in `references.bib`.

Following and headway:

- Chu et al. supplementary information (Suppl. Tables 15–21, Waymo and TGSIM) and the Zenodo
  deposit, https://doi.org/10.5281/zenodo.21783161. The extracted events would let their headway
  be recomputed as a net gap.
- Hu, Zheng, Chen, Zhang and Sun 2022, _Transportation Research Part C_ 134, 103490,
  https://doi.org/10.1016/j.trc.2021.103490: processing and quality of Waymo Open Dataset
  trajectories, the source of Chu et al.'s Waymo events. Relevant to the plan's Section 11 use of
  Waymo data.
- Soni, Reddy, Tsapi, van Arem and Farah 2022, _Transportation Research Part F_ 86, 48–64,
  https://doi.org/10.1016/j.trf.2022.02.002: the "Wizard of Oz" study in which drivers told the
  leader was automated did not follow closer.
- Wen, Cui and Jian 2022, _Accident Analysis & Prevention_ 172, 106689,
  https://doi.org/10.1016/j.aap.2022.106689, and Hu, Zheng, Chen and Sun 2023, _IEEE
  Transactions on Intelligent Transportation Systems_ 24, 6711–6724,
  https://doi.org/10.1109/TITS.2023.3258145: the earlier Waymo results that disagree.

Harm curves:

- Davis 2001, _Transportation Research Record_ 1773, 108–113, https://doi.org/10.3141/1773-13:
  only if the harm-paragraph work needs to rule the curve out explicitly; Rosén et al. (2011)
  already give the reason.

Carried from October 12 and still unread:

- Wu et al. 2026, _Accident Analysis & Prevention_ 236, 108696,
  https://doi.org/10.1016/j.aap.2026.108696: crash risk from pre-crash trajectories, matched
  case-control.
- Dean, Doerzaph and Riexinger 2025, _Transportation Research Record_ 2680, 29–42,
  https://doi.org/10.1177/03611981251327216: Safe System speed limits through injury modification
  factors.
