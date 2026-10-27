# Research briefing, October 19 to 26, 2026

This briefing reviews roadside sensing and speed research, separating observed changes from their
possible causes. It recommends a qualified setup warning and optional survey context without
changing measured speeds or registered metrics.

- **Status:** Complete. No definition, benchmark kind or registered metric changed. Adopted:
  Anderson (_Journal of Experimental Criminology_, October 5), read in full, as a cited source in
  the attitudes plan, with one open question beside the plan's radar-detector question: whether a
  deployment can itself be reported as enforcement on a navigation app, and what that would do to a
  site's speed distribution. Anderson and the verified SFMTA primary report are in
  `references.bib`. Q1 adds a qualified setup warning; Q2 recommends optional operator-provided
  enforcement context, with implementation deferred.
- **Scope:** Roadside and infrastructure LiDAR perception, and the safety and behaviour literature
  the analytics cite, first listed or newly surfaced October 19 to 26, 2026. No third-party
  briefing was supplied. Nothing the October 10, 12 or 19 briefings evaluated is repeated here.
- **Related:**
  [attitudes plan](../../../plans/platform-speeding-attitudes-and-aggressive-driving-plan.md),
  [crash-data integration plan](../../../plans/platform-crash-data-integration-plan.md),
  [behaviour analytics plan](../../../plans/lidar-behaviour-analytics-plan.md),
  [posted speed limits plan](../../../plans/posted-speed-limits-plan.md),
  [previous briefing](research-briefing-2026-10-19.md)

Paper and research-report citation keys are mapped in the [reference audit](reference-audit.md);
full entries are in [references.bib](../../../../data/maths/references.bib).

All three discovery routes were run. The arXiv past-week listings answered this week (cs.RO 459
entries, cs.CV 914, eess.SP 137) and were filtered by title; none of the 1,510 is a roadside or
infrastructure LiDAR perception paper. Two touch the analytics at a distance: a car-following law
fitted to highD, exiD and NGSIM, and a wildlife LiDAR and radar dataset with a measured speed
error. The organisation watchlist found nothing first published in the window by Waymo, Valgo,
the AAA Foundation, IIHS, NHTSA, FHWA, FMCSA, SWOV or ITF that the repository does not already
hold. The week's one source that bears on the analytics surfaced through press coverage and a
topic query: a field study, open access, that measured spot speeds with a handheld laser at 33
sites in Queensland before and after a navigation app showed an enforcement alert at the
measuring position. It records an association between an alert's appearance and lower spot
speeds, not a test of a visible LiDAR enclosure. The review update also checks San Francisco's
October 15 primary report directly, including its charts and limitations.

## Answer

Nothing changes a definition, threshold, benchmark kind, or registered metric. Anderson's
post-alert speeds were lower, but the study does not isolate a causal alert effect. The parked
car remained in place throughout: its presence was held constant, while elapsed time and
accumulating exposure were not. A visible pole-mounted enclosure is a different setting.

The transferable concern is potential measurement bias: people might react to a deployment.
Radar-detector alerts, visible hardware, and navigation-app alerts are separate mechanisms, each
requiring its own evidence. Q1 adds a qualified warning to the setup guide.

The SFMTA primary report is now verified; the figures and coverage limits appear below. Its
summary of preliminary SafeTREC work remains secondary evidence, pending the underlying study.
Comparison-site declines do not prove spillover. Q2 recommends optional operator-provided
enforcement metadata, kept separate from speed metrics and without causal attribution.

## Genuinely new this week

| Source                                                                                                          | First listed | Status                |
| --------------------------------------------------------------------------------------------------------------- | ------------ | --------------------- |
| Xu, Sun, Tang and Sun, agentic discovery of car-following laws from multi-source trajectories, arXiv 2610.27836 | October 20   | Read in part          |
| Aamir et al., WildDepth 2, RGB, LiDAR and 4D radar wildlife dataset, arXiv 2610.31123                           | October 22   | Read in part          |
| Mayor of San Francisco, press release on the speed safety camera 18-month report                                | October 15   | Read                  |
| SFMTA, _San Francisco Speed Safety Camera Program 18-Month Report_                                              | October 15   | Read (primary report) |

Xu et al. fit a symbolic car-following law (their AERM) to NGSIM US-101, highD and exiD, and
evaluate it against seven classical models on Shanghai naturalistic data, NGSIM I-80 and Waymo
trajectories. It is a simulation model fitted to spacing and speed, not a measurement of
following, and nothing in the behaviour plan simulates; the datasets it uses are already named
there. Recorded and not evaluated further.

WildDepth 2 lifts animal tracks to metric 3D with LiDAR, radar or monocular depth, and reports
that radar Doppler transferred onto LiDAR estimates radial speed with r = 0.60 and about 50%
underestimation (abstract; **Confirm**). The scene is wildlife, the speeds are low and the
underestimate is radial Doppler against LiDAR track speed, so it says nothing about vehicle speed
error at a kerb. Recorded and not evaluated further.

The SFMTA report was released with the press release on October 15, before the window, and was
covered on October 19; it is listed here because the report itself is new to the repository.

## Newly surfaced older work

| Source                                                                                                                             | First listed     | Why it surfaced this week                                        |
| ---------------------------------------------------------------------------------------------------------------------------------- | ---------------- | ---------------------------------------------------------------- |
| Anderson, crowdsourced enforcement alerts and driver speed, _Journal of Experimental Criminology_ (doi:10.1007/s11292-026-09785-x) | October 5, 2026  | University release and press, October 9; speed-alert topic query |
| Truelove and Oviedo-Trespalacios, visibility, alerts and enforcement legitimacy, _Transportation Research Part F_ 119, 103592      | 2026             | Cited by Anderson; not reached                                   |
| Safety-oriented pedestrian trajectory prediction at urban intersections using time-to-collision, arXiv 2609.29706                  | September 2026   | Surrogate-measure topic query; forecasting, out of scope         |
| AAA Foundation seat-belt research, October 7 coverage                                                                              | October 7, 2026  | Organisation watchlist; out of scope                             |
| Ouster BlueCity and Gangneung demonstration, ITS World Congress, October 2026                                                      | October 16, 2026 | Vendor watchlist; a "99% stop bar detection accuracy" claim      |

The Ouster claim is a vendor figure with no stated method or ground truth; the skill admits vendor
notes only where they publish measurement claims, and this one publishes a number without a
measurement. Recorded as **Confirm** and not evaluated. Waymo's research page lists nothing new
since the October 12 briefing: the Campolettano, Scanlon and Kusano ESV paper on VRU benchmarks,
the Scanlon fatal-rate benchmarks, the sober-baseline paper and the time-of-day paper are all held
by the crash-data plan. NHTSA's newest early estimate is still the first quarter of 2026, NSC's
first-half estimate was seen on October 19, and the IIHS speed camera sheet was read on October 12.

## Anderson: crowdsourced enforcement alerts and driver speed (_Journal of Experimental Criminology_, October 5, 2026)

1. **Measures.** Read in full from the open-access PDF. A quasi-experimental pre-post design at
   33 sites in Southeast Queensland (Sunshine Coast), posted limits 40 to 100 km/h, on arterial,
   residential, commercial, active school zone, highway and roadworks sites. A Kustom Signals
   ProLaser 4 handheld laser (stated accuracy ±1 km/h, range to 1,000 m) was operated from an
   unmarked car parked at a fixed position. The baseline period runs from the start of measurement
   to the moment an enforcement alert for that position first appeared on Waze (Google Maps and
   Apple Maps were also watched; Waze was always first), and the post-alert period is of equal
   length. The dependent variable is "speed variation", recorded speed less the posted limit, in
   km/h, and "speeding" is any reading above the limit. The population is the vehicles the
   operator measured at one point, which the paper calls spot speeds: a continuous roadside
   observation, but of a sample whose selection rule is not stated, and with no traffic count
   reported. App use among passing drivers was not observed. Received June 3, 2026, accepted
   September 22, online October 5. Funded by Queensland's Motor Accident Insurance Commission.
2. **The maths.** The headline figures reproduce. Speeding shares are 2,255 of 4,115 before
   (54.80%) and 1,276 of 4,937 after (25.85%). The mean difference is 1.23 − (−3.92) = 5.15 km/h.
   With the pooled variance, s² = (4,114 × 8.34² + 4,936 × 7.77²) / 9,050 = 64.55, the standard
   error is √(64.55 × (1/4,115 + 1/4,937)) = 0.1696 and t = 30.37 (paper 30.376); Cohen's d =
   5.15 / √64.55 = 0.641 (paper .641). The 100 km/h row gives 0.72 − (−9.26) = 9.98 km/h, as the
   text says. Four things limit what the numbers mean:
   - The t-tests treat 9,052 vehicles as independent. The mixed model accounts for site clustering but
     has a random intercept only: the effect itself varies by site from 0.79 to 22.42 km/h, and
     with no random slope the 0.15 km/h standard error describes the average over these 33 sites,
     not the uncertainty of the effect at a new one. The direction does not depend on the model:
     all 33 sites fell, and a two-sided sign test gives p = 2 × 0.5³³ = 2.3 × 10⁻¹⁰.
   - The post-alert period always follows the baseline, so time of day, traffic build-up and the
     car's growing exposure to repeat passers are all on the alert's side. There are no control
     sites, no reversal after the alert cleared, and no traffic count, so the dominant uncertainty
     is confounding, not sampling.
   - The two periods are of equal length, yet the post-alert period has 20% more readings (4,937
     against 4,115). Over a total baseline of about 33 × 13.8 = 455 minutes, that is roughly 9.0
     and 10.8 readings a minute. More traffic later, denser and slower traffic that is easier to
     target, or an operator measuring more readily once the alert is up would each produce it,
     and the paper reports none of the three. A selective sample can move a mean and a share.
   - The four 40 km/h sites are the four active school zones (Tables 2 and 3 carry identical
     rows), so "the smallest effect at 40 km/h" and "the smallest effect in school zones" are one
     finding, not two. "Speeding" counts a reading 1 km/h over the limit, inside the instrument's
     stated error, so the share is sensitive to rounding at the limit.

   The crash-risk statements in the Discussion come from the Nilsson and Elvik power model, not
   from any crash measured here.

3. **What the sensor can and cannot observe.** A speed track cannot reveal whether someone saw
   an enclosure, received a detector warning, or used an app. The three candidate mechanisms are:
   - **Radar-detector alerts:** a response to emitted radio-frequency energy. First test whether
     representative detectors alert on the OPS243-A; a LiDAR enclosure is not evidence of this.
   - **Visible hardware:** a response to the enclosure or installation activity. Anderson did not
     vary the roadside car's presence, so it does not estimate this effect for a fixed sensor.
   - **Navigation-app alerts:** a response to a location being marked as enforcement. Anderson
     motivates this hypothesis, but the study did not observe app use by individual motorists.

   LiDAR can support a speed profile against distance along a mapped lane, separately by travel
   direction. Raw sensor range is not road position: approach and departure can share a range,
   and lanes, curvature, occlusion, and tracking quality can create apparent dips. A profile alone
   cannot identify a mechanism or establish that an enclosure was mistaken for enforcement.
   Do not add a runtime lookup of third-party enforcement alerts: the research proposals use
   local measurements and operator-provided context.

4. **Where it lands.** The attitudes plan keeps these hypotheses separate. Radar on/off tests
   concern emissions; enclosure presence/absence concerns visibility; neither identifies an app
   effect without separate exposure evidence. The plan takes no position on enforcement, and the
   editorial role it cites calls the product "a tool for community empowerment, not for enforcement
   or surveillance"; Anderson's operational
   recommendations (reposition on alert, use alerts as a force multiplier) are therefore out of
   scope. The crash-data plan's speed-change row requires a change in the mean speed of the same
   traffic on the same road; a local, transient dip at one position is not that, so Anderson gives
   the crash-data plan nothing. The paper's vocabulary ("offending", "offenders", "punishment
   avoidance") fails the `offend` stem of the wording guard in
   `internal/report/headway/wording.go`, and its title fails `driver`, as both AAA Foundation
   titles do; the plan cites it in the docs and never on a page.
5. **Verdicts.**

   | Claim or recommendation                                                                           | Verdict                                                                                                                                                                                |
   | ------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
   | Spot speeds at the position fell after an enforcement alert appeared (−5.19 km/h, 33 of 33 sites) | New and adopted, as a cited source in the attitudes plan's Sources checked and in Q1. No figure enters a page or a benchmark.                                                          |
   | The fall is largest at 100 km/h and smallest in school zones                                      | Logged as context in this briefing. Seven sites carry the two extremes (three at 100 km/h, four at 40) and the post period is confounded, so no ranking by road type is drawn from it. |
   | Alerts erode deterrence and embolden speeding across the wider network over time                  | Not adopted: the study measured one position for under an hour at a time and observed nothing upstream, downstream or later. The paper says so in its limitations.                     |
   | Police should reposition on an alert and use alerts as a force multiplier                         | Not adopted: the attitudes plan takes no position on enforcement, and the plan's open questions propose recording that in `DECISIONS.md`.                                              |
   | The speed reduction implies a reduction in crash risk                                             | Already holds as a caution: the crash-data plan's speed-change row admits only a change in mean speed of the same traffic on the same road, which a transient dip at one point is not. |
   | A deployment might influence measured speeds through perceived enforcement                        | Hypothesis logged as Q1; no measured effect size for velocity.report hardware.                                                                                                         |

6. **Rights.** "This article is licensed under a Creative Commons Attribution 4.0 International
   License, which permits use, sharing, adaptation, distribution and reproduction in any medium or
   format, as long as you give appropriate credit to the original author(s) and the source,
   provide a link to the Creative Commons licence, and indicate if changes were made." No
   non-commercial condition; figures may be reproduced with credit and a link.
7. **What a velocity.report survey could give back.** These are deferred research designs, not
   completed experiments or report features. Fix the measurement positions, populations, analysis,
   and exclusion rules before collecting data:
   - For visibility, collect a genuine pre-installation baseline with an independent, minimally
     conspicuous instrument that remains unchanged through installation. Include contemporaneous
     matched control sites, multiple before/after periods, and comparable weekdays and hours.
     Record traffic volume, weather, roadworks, school schedules, and enforcement changes; assess
     pre-trends and possible control-site exposure before interpreting a difference-in-differences.
   - Where safe and permitted, randomise or counterbalance repeated enclosure-present/absent
     periods while independent measurement continues. A reversal requires actual removal and
     observation of the return condition, with allowance for persistent awareness. Switching
     radar emissions on/off while its enclosure stays fixed tests a different mechanism; keep
     LiDAR measurement unchanged, check interference, and balance those periods too.
   - Analyse speeds at fixed road positions upstream, alongside, and downstream, separately by
     lane and direction, within validated coverage. Match track eligibility and traffic conditions
     across positions; check calibration, pose, occlusion, sampling, and nearby junctions.
     Deceleration followed by recovery is descriptive evidence of a local pattern, not proof of
     perceived enforcement. Do not infer behaviour outside the observed road segment.

   Week 1 versus week 5 is only a post-installation comparison. It is neither a pre-installation
   baseline nor a reversal, and any difference could reflect traffic mix, seasonality, or other
   changes. Repeated measurements help characterise change; they do not identify its cause alone.

## SFMTA and SafeTREC: San Francisco speed safety camera program at 18 months (October 15, 2026)

1. **Verified primary figures.** [SFMTA2026][sfmta-report], pp. 17–19, uses pneumatic-tube
   surveys from winter 2024–winter 2025 (pre-installation) and May 2026. The share travelling at
   least 10 mph over the limit fell from 10.0% to 1.6%: 8.4 percentage points, or 84% relatively.
   Figure 2's daily counts fall from 49,346 to 7,099: 42,247 fewer, about 85.6%. Share and count
   reductions differ; rounded shares do not recover exact traffic denominators. These are
   survey-based speeding counts, not citations.
2. **Coverage.** Figure 3 covers 31 of 33 camera locations. Tube collection at two was limited by
   construction or Muni light rail operations; Table 4 instead uses Engineering and Traffic Surveys
   for them. The chart does not establish identical coverage for every result.
3. **SafeTREC status.** Page 22 summarises preliminary findings: speeding declined at comparison
   sites too, with faster and larger declines at camera sites. It cites the SafeTREC
   manuscript as submitted for publication. The underlying study remains unread (**Confirm**).
4. **Interpretation.** Comparison-site declines could reflect spillover, broader trends, or other
   interventions. They do not prove spillover. Verify control selection, pre-trends, exposure, and
   adjustment methods before adopting causal conclusions.

The matching 10 mph threshold does not make pneumatic-tube observations, LiDAR trajectories, and
transit maximum-speed shares interchangeable. Preserve each population and measurement method.
The verified descriptive findings belong in this briefing, not in a metric correction or a crash
risk calculation. No report graphics are reproduced; reuse terms have not been established.

For a future local evaluation, record camera installation, warnings, and citations separately.
A survey running before citations begin may already be exposed to visible equipment, publicity,
or warnings. Its baseline status must state which change it precedes. Use the controlled designs
above; proximity alone supplies neither exposure nor a causal estimate.

[sfmta-report]: https://www.sfmta.com/media/50300/download?inline=

## What changes

In this change:

- [Attitudes plan](../../../plans/platform-speeding-attitudes-and-aggressive-driving-plan.md):
  a qualified Anderson (2026) source row, separate hypotheses for radar detectors, visible
  hardware, and app alerts, and corrected research proposals linked to this briefing.
- [`references.bib`](../../../../data/maths/references.bib): `Anderson2026` and `SFMTA2026`, mapped
  in the [reference audit](reference-audit.md), with the Anderson row ordered after Ambros2026.
- [Setup guide](../../../../public_html/src/guides/setup.md): a qualified warning about possible
  measurement bias from perceived enforcement.
- An appended devlog review update, preserving the original entry.

Nothing in code, the metrics registry, benchmark kinds or definitions.

## Open questions

The review resolves Q1's documentation change and recommends Q2's scope; the research and metadata
implementation remain deferred.

### Q1: warn about potential measurement bias

The setup guide now warns that a visible radar or LiDAR enclosure might be mistaken for speed
enforcement, or reported on a navigation app, and could influence measured speeds. This is a
potential source of bias, not an established effect of this hardware, and the direction, size,
and duration are unknown. Anderson does not supply a correction factor for velocity.report.
Record deployment changes and avoid treating the first installed week as an untreated baseline.
The attitudes plan retains the separate research questions above.

### Q2: keep optional enforcement context beside the survey

Recommend optional, operator-provided contextual metadata, linked to a survey or time-bounded
site record. This is a documentation proposal; no schema, interface, automatic lookup, or metric
change is implemented here. Suggested fields are:

| Context                       | What the operator records                                                                                                             |
| ----------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| Location                      | Camera or enforcement location, with the precision and uncertainty available                                                          |
| Route and direction relevance | Same route or another road; enforced direction; upstream or downstream for each surveyed direction; unknown where unverified          |
| Distance                      | Approximate distance along the relevant route where known; label straight-line distance explicitly                                    |
| Installation                  | Date the equipment became visible, or unknown                                                                                         |
| Warning phase                 | Warning start/end dates, or unknown                                                                                                   |
| Citation phase and status     | Citation start date and known operational status during the survey, including interruptions                                           |
| Provenance                    | Agency document/URL or operator observation, date checked, and any uncertainty                                                        |
| Baseline status               | Whether installation, warnings, or citations were already present during the reference survey; dates and unknowns retained separately |

For example: “Operator-provided context: camera approximately 300 metres east along this road,
monitoring westbound traffic; citation phase reported active during this survey. Dates and source
recorded in the survey notes.” This preserves context without saying the camera caused any speed
change. A nearby camera on another road may be irrelevant; distance alone is not exposure.

Keep these notes separate from measured speeds, percentiles, and limit-relative shares. Do not
automatically adjust metrics, infer a halo, or turn the metadata into a causal explanation.
Unknown context remains unknown. Any future analytical use requires a separately reviewed design.

The October 19 briefing's Q1 remains open and is not repeated.

## Sources checked

| Source                                                                              | Status                                                                             |
| ----------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------- |
| Anderson, _Journal of Experimental Criminology_, doi:10.1007/s11292-026-09785-x     | Read (open-access PDF, 31 pages, with references and Appendix 1)                   |
| Mayor of San Francisco, press release, October 15, 2026, sf.gov                     | Read                                                                               |
| SFMTA, _San Francisco Speed Safety Camera Program 18-Month Report_ (`SFMTA2026`)    | Read: primary PDF, methods and performance pp. 16–22; Figures 2–3 visually checked |
| SafeTREC, preliminary evaluation of the San Francisco speed camera pilot            | **Confirm**: underlying manuscript unread; summary in SFMTA p. 22 only             |
| arXiv 2610.27836, Xu, Sun, Tang and Sun                                             | Read in part (abstract)                                                            |
| arXiv 2610.31123, Aamir et al., WildDepth 2                                         | Read in part (abstract)                                                            |
| arXiv 2610.30340, NVIDIA, Foundation4D                                              | Read in part (abstract); camera-only driving perception, out of scope              |
| arXiv past-week listings, cs.RO, cs.CV, eess.SP                                     | Read (1,510 titles filtered; abstracts read for the three above)                   |
| Waymo safety research page                                                          | Read (59 entries; nothing new since October 12)                                    |
| Truelove and Oviedo-Trespalacios 2026, _Transportation Research Part F_ 119, 103592 | **Confirm** (cited by Anderson; a questionnaire study, not reached)                |
| University of the Sunshine Coast release and press coverage of Anderson, October 9  | Superseded (the paper is read in full)                                             |
| Ouster BlueCity, "99% stop bar detection accuracy", October 2026                    | **Confirm** (trade coverage; vendor claim with no method)                          |
| AAA Foundation seat-belt research, October 7 coverage                               | **Confirm** (search summary; out of scope)                                         |
| NHTSA early estimate, first quarter of 2026; NSC first half of 2026                 | Superseded (seen on October 19; FARS remains the plan's fatal source)              |
| IIHS and HLDI speed safety cameras sheet, October 2026                              | Superseded (read in the October 12 briefing)                                       |
| Campolettano, Scanlon and Kusano, ESV26-294                                         | Superseded (read in full in the October 12 briefing)                               |

## Provenance

Searched October 26, 2026, extended mode unless stated: Waymo safety research new paper October
2026; AAA Foundation for Traffic Safety new research report October 2026; roadside LiDAR vehicle
tracking infrastructure point cloud paper October 2026; human crash baseline automated driving
Valgo humanbaselines October 2026; IIHS speeding study October 2026; NHTSA traffic fatalities
early estimate first half 2026 speeding; following distance tailgating headway study October
2026; pedestrian impact speed fatality risk curve study 2026 Accident Analysis and Prevention;
85th percentile speed limit setting study 2026 FHWA SWOV ITF speed report October;
post-encroachment time time-to-collision surrogate safety LiDAR 2026 preprint; new vehicle
trajectory dataset released October 2026 drone LiDAR intersection; FMCSA research tailgating
following distance handheld LiDAR 2026; speeding aggressive driving national survey results
October 2026; crash involvement rate per vehicle mile traveled exposure study 2026 Traffic Injury
Prevention; stop sign compliance full stop rolling stop observational study 2026; speed change
crash model power model Nilsson Elvik 2026 new study; roadside LiDAR arXiv October 2026
infrastructure-side 3D detection tracking; the ESV26-294 title; SWOV or ITF OECD speed safety
report published October 2026; Transportation Research Part F October 2026 speeding headway
driver behaviour observational study; Hesai or Ouster roadside intelligent transportation lidar
announcement October 2026; NHTSA Countermeasures That Work 2026 edition or FHWA safety research
report speed management October 2026; speed camera warning app alert driver speed study 2026
navigation app enforcement; and, in standard mode, the SFMTA 18-month report's 84% figure and
date. The `preprint` variants of the topics were folded into the arXiv listing route rather than
run as separate web queries.

arXiv: the past-week listings for cs.RO, cs.CV and eess.SP were fetched from arxiv.org with
`curl` on October 26 and filtered by title for LiDAR, point cloud, tracking, traffic, vehicle,
radar, pedestrian, roadside, infrastructure, crash, collision, occupancy, car-following,
headway, trajectory and intersection terms; abstract pages for 2610.27836, 2610.31123 and
2610.30340.

Read with `curl` and `pdftotext -layout`: the Anderson article page and PDF from
link.springer.com (the article page through `WebFetch` redirected to a login and was not
followed), ESV26-294 from static.nhtsa.gov to confirm it is the paper the October 12 briefing
read, and the sf.gov press release. `WebFetch` read the Waymo research page. The arithmetic in
the Anderson section is from the paper's Tables 1 to 3 and Appendix 1. The PDFs are not
committed.

The original search could not reach www.sfmta.com, localnewsmatters.org, or api.crossref.org.
During the PR review update, SFMTA access succeeded: the report landing page links the primary
PDF above. Its text was extracted and Figures 2–3 were rendered and checked visually; the
Anderson publisher page was also rechecked. The original SFMTA access failure is superseded.
Downloaded sources and page images remain outside Git.

## To fetch

- SafeTREC's underlying San Francisco evaluation, cited on SFMTA p. 22 as Lee, Raifman, and
  Griswold, _Evaluating the Effectiveness of Automated Speed Enforcement in San Francisco,
  California: A Quasi-Experimental Approach Using Telematics Data_ (manuscript submitted for
  publication): control selection, pre-trends, exposure, adjustments, and uncertainty behind the
  comparison-site results (Q2). Publication status has not been independently confirmed.
- Truelove and Oviedo-Trespalacios 2026, _Transportation Research Part F_ 119, 103592,
  https://doi.org/10.1016/j.trf.2026.103592: the questionnaire on alerts and enforcement
  legitimacy that Anderson cites, if later research needs the self-reported side.
- De Pauw, Daniels, Brijs, Hermans and Wets 2014, _Accident Analysis & Prevention_ 73, 132–140:
  the "kangaroo" speed profile around fixed cameras, the method behind the first research-note
  test in the Anderson section. DOI not checked against Crossref from this session.

Carried from October 19 and still unread: the Chu et al. supplementary information and Zenodo
deposit; Hu et al. 2022; Soni et al. 2022; Wen, Cui and Jian 2022 and Hu et al. 2023; Davis 2001;
Wu et al. 2026; Dean, Doerzaph and Riexinger 2025. Their URLs are in the
[October 19 briefing](research-briefing-2026-10-19.md#to-fetch).
