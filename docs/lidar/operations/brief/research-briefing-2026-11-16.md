# Research briefing, November 9 to 16, 2026

- **Status:** Complete. No definition, benchmark kind, threshold or registered metric changed.
  Nothing adopted as a reference. One open question on Section 8.4 of the behaviour analytics
  plan, from Khalili, Moshfeghi, Jang and Smyth (arXiv 2611.13540, November 10), read in full.
- **Scope:** Roadside and infrastructure LiDAR perception, and the safety and behaviour literature
  the analytics cite, first listed or newly surfaced November 9 to 16, 2026. No third-party
  briefing was supplied. Nothing the October 10, 12, 19 and 26 or November 2 and 9 briefings
  evaluated is repeated here.
- **Related:**
  [behaviour analytics plan](../../../plans/lidar-behaviour-analytics-plan.md),
  [state estimation plan](../../../plans/lidar-state-estimation-plan.md),
  [label vocabulary](../../architecture/label-vocabulary.md),
  [previous briefing](research-briefing-2026-11-09.md)

Paper and research-report citation keys are mapped in the [reference audit](reference-audit.md);
full entries are in [references.bib](../../../../data/maths/references.bib).

All three discovery routes were run. The arXiv past-week listings answered (cs.RO 438 entries,
cs.CV 869, eess.SP 191, November 9 to 16, 1,418 unique after cross-lists) and were filtered by
title. None of them is a roadside LiDAR perception paper. One, a camera pipeline that extracts
conflict records from long fixed-camera traffic video, bears on the behaviour plan, and it is the
only source evaluated in full. The organisation watchlist found nothing new: Waymo, Valgo and
IIHS list the same items as on November 9, and no AAA Foundation, FHWA, FMCSA, NHTSA, SWOV or ITF
report was first published in the window. It was a quiet week, and its one useful finding is a
lesson in how not to compute TTC.

## Answer

Nothing changes a definition, threshold, benchmark kind or registered metric. AgenticConflictLens
writes each verified conflict as a record with an interval, a participant pair, a region,
trajectory evidence, an uncertainty and, for rejected candidates, a reason. The behaviour plan
already requires the same: the explainability record of Section 7.6, the closed suppression
vocabulary of Section 7.2 and the evidence surface of Section 8.12, which records an evasive
response "without implying fault". The paper reaches that record from video at 4 frames per
second through an approximate homography. A fixed LiDAR measures the same positions directly
and records no image of anyone, so the paper offers the project no method it lacks.

The paper's TTC is worth a sentence in the plan. It computes a centre-to-centre time to closest
approach, minus the dot product of relative position and relative velocity over the squared
relative speed, and evaluates it at the instant of minimum separation. At an interior minimum of
a smooth separation that numerator is zero by construction, so the quantity is zero or undefined
there. On sampled tracks it is a small number of either sign, and "TTC below 1.5 s" then says
little beyond "the pair was closing". Section 8.4 already defines `ttc` as gap over closing speed,
on a bumper-to-bumper gap, and takes `ttc_min` over the whole encounter, so the plan does not make
this mistake. Q1 asks whether it should say why in one sentence.

The rest of the week is vendor news and older work found again: a LiDAR toll-classification
launch with no published method, a roadside LiDAR wrong-way detection paper, an
infrastructure-LiDAR speed estimator, and an ADAS field dataset whose "car-following threshold"
is the range at which one vehicle's driver-assistance system detects a lead vehicle.

## Genuinely new this week

| Source                                                                                                                          | First listed | Status       |
| ------------------------------------------------------------------------------------------------------------------------------- | ------------ | ------------ |
| Khalili, Moshfeghi, Jang and Smyth, _AgenticConflictLens: From Long Traffic Videos to Verifiable Conflict Evidence_             | November 10  | Read         |
| Han et al., _Online Geometric Consistency Assessment of LiDAR Measurements via Recursive Bayesian Comparison_, arXiv 2611.10566 | November 7   | Read in part |
| Dong et al., _Ego2Flow_, closed-loop traffic-impact benchmark on CARLA maps from Argoverse 2, arXiv 2611.17720                  | November 13  | Read in part |
| Takken and You, _NightSteps_, night-time dashcam pedestrian crossing dataset, arXiv 2611.11583                                  | November 8   | Read in part |
| Seyond SIMPL-Tolling, LiDAR toll-gantry detection and classification (trade press, IBTTA, November 12 to 15)                    | November 9   | **Confirm**  |
| Indra deployment of Ouster Rev7 LiDAR for toll classification (trade press)                                                     | November     | **Confirm**  |
| IIHS speed camera communities page, November count of 392 communities                                                           | November     | **Confirm**  |

Han et al. detect registration drift in a moving platform's accumulated map by comparing each new
scan's voxel geometry with a Normal-Inverse-Wishart history; it is a SLAM integrity monitor, and
the project's sensor does not move. Ego2Flow scores driving policies in simulation; NightSteps is
dashcam video, which Tenet 1 excludes. All three are recorded and not evaluated.

The Seyond item specifies detection accuracy of "99.99% or greater" and classification accuracy
of "99.5% or greater" for toll gantries, and the Indra item reports "hundreds" of Ouster units in
tolling. Neither publishes a test population, a ground truth or a confusion matrix, so neither is
a measurement; both are search summaries only (traffictechnologytoday.com and automotiveworld.com
refused the proxy) and carry **Confirm**. The
[LiDAR market watch](../../../platform/hardware/lidar-market-watch.md) already lists Seyond,
with no spinning unit seen. The IIHS page is a tracking list updated monthly, not a study.

## Newly surfaced older work

| Source                                                                                                              | First listed     | Why it surfaced this week                                        |
| ------------------------------------------------------------------------------------------------------------------- | ---------------- | ---------------------------------------------------------------- |
| Li et al., _Benchmarking Tesla's Traffic Light and Stop Sign Control_, arXiv 2512.11802 and its journal version     | October 31, 2025 | Stop compliance topic query; journal version now listed          |
| _A Roadside LiDAR and HD-Map Framework for Early Wrong-Way Driving Detection_, _Sensors_ 26(21), 6770               | October 26, 2026 | Roadside LiDAR tracking query                                    |
| _Vehicle Speed Estimation Using Infrastructure-Mounted LiDAR via Rectangle Edge Matching_, _Appl. Sci._ 16(5), 2513 | 2026             | Extent and heading query                                         |
| Zhao, Wang, Zhuang and Yang, _Bench-RNR_, arXiv 2509.15583                                                          | September 2025   | Extent and heading query                                         |
| ERSO thematic report _Speed and speeding_, 2026 edition, SWOV among the authors                                     | 2026             | SWOV and ITF query; edition date not established                 |
| Yasir et al., Palermo speed reduction (_Frontiers in Future Transportation_, August 25, 2026)                       | August 2026      | Pedestrian impact-speed query; read in the October 19 briefing   |
| Scanlon et al. fatal-rate benchmarks; Jang (`Jang2026`); Salah, Li and Wang (`Salah2026`); NHTSA 813 823            | 2026             | Crash-rate and surrogate queries; all held                       |
| City of Gaithersburg stop-sign compliance collection, November 9 to 23                                              | November 2026    | Stop compliance query; carried from November 9, still unresolved |

The wrong-way paper reports, from search summaries, that staged tests detected all 14 daytime and
22 night-time annotated wrong-way events with no false alarm among 67 daytime and 88 night-time
normal passes (**Confirm**; mdpi.com refused the proxy and PubMed returned a cookie page). With
zero misses in 36 events, the exact one-sided 95% lower bound on the detection rate is
0.05^(1/36) = 0.920, and with zero false alarms in 155 normal passes the upper bound on the false
alarm rate is 1 − 0.05^(1/155) = 0.019, about the "rule of three" value 3/155. The staged events
are the paper's own, so these bounds describe that test, not traffic. The
[directional speed reporting plan](../../../plans/platform-directional-speed-reporting-plan.md)
already fixtures a wrong-way track; nothing more is needed until that plan reports direction
shares.

The rectangle edge matching paper models a vehicle's points as a rectangle and estimates speed
from edge displacement between frames. Bench-RNR (CC BY 4.0, abstract read) benchmarks roadside
vehicle localisation from repetitive and non-repetitive scanning LiDARs over 5,445 frames; search
summaries attribute heading errors of about 1.0° to 1.35° to its best baselines (**Confirm**).
Both bear on the state estimation plan's visible-surface bias only as comparisons, and neither
reports its error against an independent reference on public roads. They are listed under To
fetch for the extent work, not evaluated.

Li et al. instrumented one Tesla Model Y with a 10 Hz SparkFun GPS and a forward camera near
Madison, Wisconsin, and fitted the Full Velocity Difference Model to stopping, accelerating and
following segments. Read in part (Sections 1, 4 and 7). Its "car-following threshold (~90 m)" is
the range at which the driver-assistance system detected a lead vehicle and followed it through a
green light, bracketed between activation trials at about 60 m and 90 m; the authors call the
value "not exact due to the simplicity of the experimental setup". It describes one product's
sensing range, not a following state of traffic, and it does not bear on the leader-choice rules
of the behaviour plan's Section 8.3. Its text also fails the `aggress` stem of the wording guard
("less aggressive dynamics"). Not adopted.

Waymo's research page still lists 59 entries, the same as on October 26, November 2 and
November 9; Valgo's news page still carries only its June 19 announcement; the IIHS news index's
newest item is still the October 27 Utah release.

## Khalili, Moshfeghi, Jang and Smyth: AgenticConflictLens (arXiv 2611.13540, November 10, 2026)

1. **Measures.** Read in full from the arXiv PDF (Columbia University and Florida Atlantic
   University; a preprint, venue not stated). The system answers a natural-language query such as
   a turning conflict at a crosswalk over 30 minutes of fixed-camera video. A vision-language
   model (Qwen2.5-VL-7B-Instruct, frozen) directs a coarse scan, then a trajectory-based verifier
   confirms or rejects each candidate pair. Evaluation uses 96 hours: 60 from five public City of
   Bellevue intersections and 36 from three NYC DOT intersections whose recordings are restricted,
   twelve one-hour recordings per site. The paper's own definition: "A conflict is a temporally
   localized encounter where road users show converging, competing, or mutually dependent
   movement with a visible response, such as yielding, braking, stopping, directional adjustment,
   gap acceptance, or evasive motion. Proximity alone is not sufficient." The population is
   annotated video episodes, labelled by the authors against that definition; it is neither a
   surrogate-threshold count nor a crash series. Headline: conflict-existence F1 0.84 and mean
   temporal IoU 0.77 on Bellevue, 0.80 and 0.73 on NYC DOT, inspecting 8.7% and 11.9% of video.
2. **The maths.** The counts reproduce: 119 + 1,008 = 1,127 conflicts and 457 + 4,125 = 4,582
   queries over 60 + 36 = 96 hours. Conflict density is 119 / 60 = 1.98 per hour on Bellevue and
   1,008 / 36 = 28.0 per hour on NYC DOT, a factor of 14; the two datasets test different things,
   as the paper says. Inspecting 8.7% of a 30-minute window is 2.6 minutes of video per query.

   The TTC is the part to check. The verifier takes t\* as the time of minimum ground-plane
   separation between the two centres and evaluates, at t\*, TTC = −rᵀv / ‖v‖² when rᵀv < 0, with
   r the relative position and v the relative velocity under constant velocity; "an event is
   marked TTC-critical when TTC < 1.5 s". That expression is the time remaining until the two
   centres reach their closest approach, not the time until contact of two bodies with extent.
   The derivative of ‖r‖² is 2rᵀv, which is zero at an interior minimum, so at t\* the numerator
   vanishes and the true value is zero. With velocities from finite differences at 4 frames per
   second, the sampled rᵀv at the discrete minimum is small and of either sign: the result is
   "undefined" or a fraction of a second, below 1.5 s whenever it is defined. The flag is then
   close to a restatement of "closing at the sampled minimum". The only case where it carries
   more is a minimum at the end of the common observation, a pair still closing as one leaves
   view, where TTC measures time to an approach the sensor did not see. The paper uses TTC "as
   corroborating evidence, not as a stand-alone conflict decision", which limits the damage, but
   its TTC agreement also enters the stored uncertainty. Its code is not published, so this is
   the formula as printed, not a test of the implementation.

   The severity grades follow from Equation 10: critical, high or medium by the fraction of a
   context-adaptive separation threshold attained, with that threshold the product of a base
   value per class pair and bounded factors for closing speed, approach angle, local density and
   visibility. The base values, the factor shapes and γc and γh are not given, so no grade can be
   reproduced. The dominant uncertainty in the headline is the reference itself: F1 measures
   agreement with the authors' annotation of "visible response", and every main evaluation query
   "correspond[s] to annotated conflict events", so the main tables do not test a query whose
   correct answer is that nothing happened. The SSAM-style trajectory baseline (Pu and
   Joshi 2008) scores pair accuracy 0.86 on Bellevue against the system's 0.85; the system's
   gain is search cost and temporal localisation, not identifying the right pair.

3. **What the sensor can and cannot observe.** A fixed LiDAR measures positions, extents and
   headings in metres directly, at 10 Hz, without the homography "from scene reference points and
   known road-width cues" on which every distance in the paper rests. It cannot see the "visible
   response" cues that the paper reads from video beyond kinematics, and it needs none: the
   paper's Table 5 response signatures (braking as a speed drop over 30%, directional adjustment
   as a heading change over 20°) are kinematic rules any trajectory source can apply. Nothing in
   the method needs a camera once trajectories exist, and the project's trajectories come from
   a sensor that records no image of anyone, by Tenet 1.
4. **Where it lands.** Checked by grep against the behaviour plan, the
   [label vocabulary](../../architecture/label-vocabulary.md#suppression-reasons),
   `internal/lidar/l8behaviour/` and the wording guard.
   - A structured record per verified event, with uncertainty and provenance: already holds,
     Section 7.6, the explainability record, carried as fields in Section 10.2.
   - Rejected candidates kept with a reason and counted: already holds, Section 7.2 ("a
     suppression reason is a first-class result: it is stored, queryable and reportable") and its
     closed vocabulary. The paper's reasons map onto it: "insufficient track quality" to
     `insufficient_observation` and `trajectory_uncertainty_too_high`, "ambiguous spatial
     support" to `interaction_type_uncertain`. "Non-closing motion" is already a status in
     `internal/lidar/l8behaviour/drac.go` (`non_closing`, `uncertain_closing_direction`).
     "Ordinary co-presence" is a classifier outcome, not a suppression, and is not adopted.
   - Response without implied fault: already holds, Section 8.12, `InducedResponse`, which
     records actor, responder, latency and magnitudes and "does not assert a cause".
   - TTC as gap over closing speed, defined only when closing, minimum over the encounter:
     already holds, Section 8.4 (`ttc`, `ttc_min`) with the SSAM 1.5 s band reported as such. The
     plan does not say why the closest-approach form is not used. Q1.
   - Minimum separation: already holds as `min_separation`, "surface-to-surface", Section 8.6;
     the paper's is centre-to-centre on an approximate ground plane.
   - Severity grades and a "severity-weighted risk field" of hotspots: not adopted. A graded
     label is a verdict on an encounter and a smoothed field of them is a score; both fail the
     wording guard's intent, and the field's title fails its `risk` stem.
   - Wording: the paper's text uses "unsafe turns" (`unsafe`) and "surrogate-risk measures"
     (`risk`). The briefing cites it in the docs only.
5. **Verdicts.**

   | Claim or recommendation                                                                     | Verdict                                                                                                                                      |
   | ------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
   | Write each verified conflict as a record with interval, pair, region, evidence, uncertainty | Already holds: behaviour plan Sections 7.6 and 10.2.                                                                                         |
   | Keep rejected candidates with explicit reasons, countable through an audit trail            | Already holds: Section 7.2 and the label vocabulary's suppression reasons; `non_closing` in `drac.go`.                                       |
   | A conflict needs a response, not proximity alone                                            | Already holds as an evidence surface, Section 8.12, without a causal claim. Not adopted as a definition of a counted conflict.               |
   | TTC = −rᵀv / ‖v‖² at minimum separation, TTC-critical below 1.5 s                           | Not adopted: degenerate at the closest-approach instant, centre to centre, no extents. Section 8.4 already differs; Q1.                      |
   | Context-adaptive separation threshold with critical, high and medium grades                 | Not adopted: unreproducible parameters, and a grade is a verdict word on an encounter.                                                       |
   | Severity-weighted hotspot field                                                             | Not adopted: a score, which the wording guard forbids on a page.                                                                             |
   | Query-conditioned search of long recordings to cut inspection cost                          | Not adopted: the project computes every passage offline from trajectories, which the paper's SSAM-style baseline shows is accurate on pairs. |

6. **Rights.** The arXiv listing carries the arXiv non-exclusive licence to distribute 1.0, which
   grants others nothing beyond reading. The briefing paraphrases and quotes short definitions;
   no table, figure or equation image is reproduced. The NYC DOT recordings are restricted; the
   Bellevue videos are public through the City of Bellevue's dataset.
7. **What a velocity.report survey could give back.** A property of the formulas on observed
   passages, testable on any site where the pairing code already emits closing pairs: compute the
   closest-approach expression at the sampled minimum-separation frame and `ttc_min` from Section
   8.4 for the same pairs, and report the share of pairs whose closest-approach value falls below
   1.5 s against the share whose `ttc_min` does. If the first share is close to the share of
   pairs closing at the sampled minimum, the degeneracy in item 2 holds on real tracks. It is a
   note about two formulas, not a finding about anyone, and it needs no new sensing.

## What changes

In this change:

- [Behaviour analytics plan](../../../plans/lidar-behaviour-analytics-plan.md): a November 16
  entry in the weekly literature review list naming this briefing and Q1.
- One devlog fragment.

Nothing in code, `references.bib`, the metrics registry, benchmark kinds, Section 5.1 or
definitions. AgenticConflictLens is not added to `references.bib`: the plan cites it for nothing
it holds, and Q1 can be answered without a citation.

## Open questions

### Q1: say why Section 8.4 does not use the closest-approach form

Section 8.4 defines `ttc` as gap over closing speed on a same-path, bumper-to-bumper gap, and
`ttc_min` as the minimum over the encounter. A common alternative in the conflict literature, used
this week by AgenticConflictLens, is the centre-to-centre time to closest approach, −rᵀv / ‖v‖²,
which some pipelines evaluate at the minimum-separation instant where it is zero by construction.
Should Section 8.4 add one sentence saying that the closest-approach expression is not TTC (it
ignores extent and measures time to the nearest approach of two points, not to contact), and that
no TTC is evaluated at the closest-approach instant? The case for is that the plan already lists
"conflating" short headway and low TTC as "a common error", and this is a second one of the same
kind that a future crossing-TTC implementation could make. The case against is that the plan
already restricts longitudinal TTC to following pairs and leaves crossing TTC to the
two-dimensional form, so the sentence guards a door the plan has not opened. It would add text,
never a threshold or a metric. The November 9 briefing's Q1 and Q2, the November 2 briefing's Q1,
the October 26 briefing's Q1 and Q2 and the October 19 briefing's Q1 remain open and are not
repeated.

## Sources checked

| Source                                                                             | Status                                                                  |
| ---------------------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| Khalili, Moshfeghi, Jang and Smyth, arXiv 2611.13540                               | Read (arXiv PDF, with appendices)                                       |
| Li et al., arXiv 2512.11802                                                        | Read in part (arXiv PDF, Sections 1, 4 and 7)                           |
| arXiv 2611.10566, 2611.17720, 2611.11583, 2509.15583                               | Read in part (abstracts)                                                |
| arXiv past-week listings, cs.RO, cs.CV, eess.SP                                    | Read (1,418 titles filtered)                                            |
| Waymo safety research page                                                         | Read (59 entries; unchanged since October 26)                           |
| Valgo news page                                                                    | Read (one post, June 19, 2026)                                          |
| IIHS news index                                                                    | Read (ten most recent items, to October 27)                             |
| Seyond SIMPL-Tolling, Traffic Technology Today                                     | **Confirm** (proxy refused; search summary only)                        |
| Indra and Ouster Rev7 tolling, Automotive World                                    | **Confirm** (proxy refused; search summary only)                        |
| IIHS speed camera communities page                                                 | **Confirm** (search summary only)                                       |
| Roadside LiDAR and HD-map wrong-way detection, _Sensors_ 26(21), 6770              | **Confirm** (mdpi.com refused; PubMed returned a cookie page)           |
| Rectangle edge matching speed estimation, _Applied Sciences_ 16(5), 2513           | **Confirm** (search summary only; mdpi.com refused)                     |
| ERSO _Speed and speeding_ thematic report, 2026                                    | **Confirm** (search summary only; not fetched)                          |
| City of Gaithersburg stop-sign compliance release                                  | **Confirm** (gaithersburgmd.gov refused the proxy; search summary only) |
| Yasir et al., _Frontiers in Future Transportation_, 2026                           | Superseded (read in the October 19 briefing)                            |
| Scanlon et al. fatal-rate benchmarks, `Jang2026`, `Salah2026`, `NHTSASpeeding2026` | Superseded (held since earlier briefings)                               |

## Provenance

Searched November 16, 2026, extended mode: roadside LiDAR tracking November 2026; AAA Foundation
for Traffic Safety new research report November 2026; Waymo safety research new paper November
2026 crash rate human baseline; IIHS study speeding speed cameras November 2026; following
distance tailgating headway study November 2026; crash involvement rate per vehicle mile exposure
study November 2026; pedestrian impact speed fatality risk curve new study 2026; 85th percentile
speed limit setting research NCHRP FHWA November 2026; post-encroachment time threshold surrogate
safety LiDAR 2026 preprint; stop sign compliance observational study LiDAR 2026; the title of
Li et al. (arXiv 2512.11802) as written; speeding aggressive driving survey results released
November 2026; new vehicle trajectory dataset released November
2026 LiDAR drone intersection; NHTSA FHWA FMCSA speeding research report November 2026; speed
change crash model power model Elvik new study 2026; sober driving baseline automated driving
comparison human crash baseline November 2026; LiDAR headway measurement following distance
enforcement 2026; infrastructure LiDAR vehicle extent heading estimation 2026 IEEE T-ITS ITSC;
SWOV ITF OECD road safety speed report published November 2026; FMCSA research project following
distance headway 2026 update; Accident Analysis and Prevention November 2026 speeding free-flow
speed roadside sensor; Frontiers in Future Transportation 2026 Palermo speed reduction. As in
earlier weeks, the `preprint` variants of the topics were folded into the arXiv listing route
rather than run as separate web queries. Two search results pointed back at this repository's own
pull requests; they were not used as sources.

arXiv: the past-week listings for cs.RO, cs.CV and eess.SP were fetched from arxiv.org with
`curl` on November 16 and filtered by title for LiDAR, roadside, infrastructure, headway,
car-following, crash, surrogate, V2X, intersection, traffic, tracking, pedestrian, occlusion,
point cloud, radar and collision terms. The export API returned a rate-limit reply, so metadata
and licences came from the abstract pages. The two PDFs were read with `curl` and
`pdftotext -layout`; the arithmetic in the AgenticConflictLens section is from its Sections 4.1
and 4.4, Tables 1 to 4 and Appendices 1.1.3 and 2.2. `WebFetch` read the Waymo research page, the
Valgo news page and the IIHS news index; www.frontiersin.org, mdpi.com,
traffictechnologytoday.com, automotiveworld.com and gaithersburgmd.gov refused the proxy, and
PubMed returned a cookie page. The PDFs are not committed.

## To fetch

- _Sensors_ 26(21), 6770, https://doi.org/10.3390/s26216770: the wrong-way detection paper's
  sensor, sites and staged-event protocol, only when the directional speed reporting plan reports
  direction shares.
- _Applied Sciences_ 16(5), 2513, https://www.mdpi.com/2076-3417/16/5/2513: the rectangle edge
  matching speed estimator and its reference, for comparison with the state estimation plan's
  extent and speed work.
- Bench-RNR, https://arxiv.org/abs/2509.15583: the heading-error tables behind the 1.0° to 1.35°
  figures, for the same comparison.
- ERSO _Speed and speeding_ 2026,
  https://road-safety.transport.ec.europa.eu/document/download/9826c063-bc55-423e-84a3-24200dca3547_en?filename=ERSO-TR-speed_2026.pdf:
  whether the 2026 edition changes any figure the posted speed limits or attitudes plans cite.
- City of Gaithersburg release, https://www.gaithersburgmd.gov/Home/Components/News/News/10247:
  the device type, only if it turns out to be a roadside range sensor; collection ends November 23.

Carried from November 9 and still unread: Singh et al. 2024 and Li et al. 2024 from the
bibliography of `Salah2026`, SIT-PET and _PLOS ONE_ e0356394; and from November 2, October 26 and
October 19, the items listed in those briefings' To fetch sections.
