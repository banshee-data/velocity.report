# Research briefing, October 19 to 26, 2026

- **Status:** Complete. No definition, benchmark kind or registered metric changed. Adopted:
  Anderson (_Journal of Experimental Criminology_, October 5), read in full, as a cited source in
  the attitudes plan, with one open question beside the plan's radar-detector question: whether a
  deployment can itself be reported as enforcement on a navigation app, and what that would do to a
  site's speed distribution. One entry added to `references.bib`. Two open questions for the
  owner.
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
measuring position. It is a measurement of how a site's speed distribution moves when road users
believe they are being measured, which is a question this project has asked only of radar
detectors so far. San Francisco's 18-month speed camera report, released on October 15, was read
from the city's press release only.

## Answer

Nothing changes a definition, a threshold, a benchmark kind or a registered metric. Anderson
(2026) measured 9,052 spot speeds from a parked, unmarked car at 33 sites, split at the moment an
enforcement alert for that car appeared on Waze. Speed relative to the limit fell by 5.19 km/h on
the paper's mixed model, and fell at every one of the 33 sites. The figures reproduce from the
paper's own tables. The design has no control sites, the alert period always follows the baseline,
and traffic counts are not reported, so the study shows that speeds at the position fell after an
alert and does not separate the alert from time of day or from the car's visibility. Its claims
about deterrence across the wider network are theory and were not measured, and are not adopted.
What transfers is a measurement risk, not a figure: a site's speed distribution depends on whether
passers-by believe the site is enforced. The attitudes plan already asks this of radar detectors
and the OPS243-A's K-band signal. It now also asks it of navigation apps (Q1), and the plan
cites Anderson as the measured case.

The SFMTA's 18-month report says the share of vehicles 10 mph or more over the limit fell by close
to 84% at its 33 camera locations, with a SafeTREC preliminary report finding a decline at
comparable streets without cameras. The 10 mph edge is the attitudes plan's own reporting edge,
which is consistent with the plan and changes nothing in it. The report itself and SafeTREC's were
unreachable, so every figure is **Confirm**. Whether a site should record nearby automated
enforcement as context is Q2.

## Genuinely new this week

| Source                                                                                                          | First listed | Status       |
| --------------------------------------------------------------------------------------------------------------- | ------------ | ------------ |
| Xu, Sun, Tang and Sun, agentic discovery of car-following laws from multi-source trajectories, arXiv 2610.27836 | October 20   | Read in part |
| Aamir et al., WildDepth 2, RGB, LiDAR and 4D radar wildlife dataset, arXiv 2610.31123                           | October 22   | Read in part |
| Mayor of San Francisco, press release on the speed safety camera 18-month report                                | October 15   | Read         |
| SFMTA, _San Francisco Speed Safety Camera Program 18-Month Report_                                              | October 15   | **Confirm**  |

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
   - The t-tests treat 9,052 vehicles as independent. The mixed model fixes that for the mean but
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

3. **What the sensor can and cannot observe.** The sensor cannot see an app, an alert, or who
   uses either, and must not try: a lookup of a navigation app's enforcement reports would be a
   runtime network call to a third party, outside Tenet 4, about the site's own location. It can
   see what Anderson's design could not. A LiDAR track carries speed along the approach, so the
   pattern De Pauw et al. (2014) call the "kangaroo" effect, slowing towards a measured position
   and recovering after it, is a speed profile against range that a site can draw. A site that
   runs for weeks also has the reversal Anderson lacked: the hours after an installation against
   the same hours a month later. Neither says why a profile dips; mount geometry, cosine error
   and a junction near the sensor all shape it too.
4. **Where it lands.** The attitudes plan's open questions already ask whether a radar detector
   alerts on the OPS243-A and propose testing it with the LiDAR's approach speed profile, radar on
   against off. Anderson is the measured case of the same mechanism through a different channel: a
   road user's belief that a position is enforced moves the speeds measured there. The plan takes
   no position on enforcement, and the editorial role it cites calls the product "a tool for
   community empowerment, not for enforcement or surveillance"; Anderson's operational
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
   | A site's speed distribution depends on whether road users believe the site is enforced            | New and logged as Q1, beside the radar-detector question.                                                                                                                              |

6. **Rights.** "This article is licensed under a Creative Commons Attribution 4.0 International
   License, which permits use, sharing, adaptation, distribution and reproduction in any medium or
   format, as long as you give appropriate credit to the original author(s) and the source,
   provide a link to the Creative Commons licence, and indicate if changes were made." No
   non-commercial condition; figures may be reproduced with credit and a link.
7. **What a velocity.report survey could give back.** Two research-note hypotheses, neither a
   report feature. First, at a site with LiDAR, the median speed against range along the approach
   has no dip at the sensor's position beyond what the mount geometry predicts; a dip that appears
   or deepens after installation would be evidence that the deployment is read as enforcement.
   Second, at any site, the hourly pace (p50 less the limit) in the first week after installation
   matches the same hours four weeks later; a step that decays would say the same. Both tests use
   only the site's own data and say nothing about any passage.

## SFMTA and SafeTREC: San Francisco speed safety camera program at 18 months (October 15, 2026)

1. **Measures.** Read from the Mayor's press release in full; the SFMTA report and SafeTREC's
   preliminary report were not reached. 56 cameras at 33 locations, many on the 2024 High Injury
   Network. The share of vehicles travelling 10 mph or more over the limit is said to have fallen
   by close to 84% at camera locations against pre-installation levels, "an estimated 42,000
   fewer instances each day", and by up to 94% near schools. SafeTREC is said to find that
   speeding fell during the warning period before fines began, stayed lower 50 weeks after
   activation, and also declined on comparable streets without cameras. The instrument, the
   counting periods and the denominator behind 42,000 a day are not in the release (**Confirm**).
   The citation figures (63% of those cited were not cited again) are about people and out of
   scope.
2. **The maths.** Nothing to re-derive: a relative change in a share, with neither share given.
   An 84% fall from a 10% share and from a 40% share are different streets.
3. **What the sensor can and cannot observe.** A site measures the same quantity, a share of
   transits 10 mph or more over the limit, and the attitudes plan's limit-relative shares already
   print it. A site on or near a camera corridor measures speeds inside whatever halo the camera
   casts, which the site does not record.
4. **Where it lands.** The 10 mph edge matches the behaviour plan's `speeding_exposure_10` edge and
   the attitudes plan's limit-relative share edges; consistent, nothing to change. The October 12
   briefing read the IIHS camera sheet and gave the same verdict: before-and-after enforcement
   effects are not cited by any plan, and the primaries would be the sources if the posted speed
   limits plan ever needs an enforcement context paragraph.
5. **Verdicts.** The 84% figure: not adopted, unread primary and an enforcement evaluation. The
   spillover finding: logged as Q2, because it bears on what a site's speeds mean.
6. **Rights.** The release is a public government page; no reuse terms were stated. The report's
   terms are unread.
7. **What a velocity.report survey could give back.** Nothing a site can test about cameras
   without a before period. A site already running when a camera is installed nearby would have
   one, and its limit-relative shares before and after are the measurement; that is a note for the
   owner, not a feature.

## What changes

In this change:

- [Attitudes plan](../../../plans/platform-speeding-attitudes-and-aggressive-driving-plan.md):
  a Sources-checked row for Anderson (2026), and an open question beside the radar-detector
  question on whether a deployment can be reported as enforcement on a navigation app (Q1),
  linking this briefing.
- [`references.bib`](../../../../data/maths/references.bib): `Anderson2026`, read in full, mapped
  in the [reference audit](reference-audit.md).
- A devlog bullet.

Nothing in code, the metrics registry, benchmark kinds or definitions.

## Open questions

| ID  | Question                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| --- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Q1  | Can a deployment be reported as enforcement on a navigation app, and should the docs and the setup guide say so? Anderson (2026) measured a 5.19 km/h fall in speed relative to the limit at all 33 sites after an alert for a parked car appeared. A sensor on a pole is not a parked car with an operator, and no one has measured whether passers-by report one. If adopted, the setup guide says that a visible enclosure can be read as enforcement and that this would lower the speeds it measures, and the two research-note tests in the Anderson section become an item in the attitudes plan's deferred research notes. Logged in the attitudes plan's open questions. |
| Q2  | Should a site record operator-stated enforcement context beside its posted limit: an automated camera on the same corridor, with its installation date? SafeTREC's preliminary finding that speeding fell on comparable streets without cameras (**Confirm**) means a site near a camera may sit in its halo. It would be context printed as operator-stated, not a stratum and not a covariate, and it belongs with the posted speed limits plan's site attributes if adopted.                                                                                                                                                                                                   |

The October 19 briefing's Q1 remains open and is not repeated.

## Sources checked

| Source                                                                              | Status                                                                |
| ----------------------------------------------------------------------------------- | --------------------------------------------------------------------- |
| Anderson, _Journal of Experimental Criminology_, doi:10.1007/s11292-026-09785-x     | Read (open-access PDF, 31 pages, with references and Appendix 1)      |
| Mayor of San Francisco, press release, October 15, 2026, sf.gov                     | Read                                                                  |
| SFMTA, _San Francisco Speed Safety Camera Program 18-Month Report_                  | **Confirm** (sfmta.com refused; figures from the press release)       |
| SafeTREC, preliminary evaluation of the San Francisco speed camera pilot            | **Confirm** (not located; figures from the press release)             |
| arXiv 2610.27836, Xu, Sun, Tang and Sun                                             | Read in part (abstract)                                               |
| arXiv 2610.31123, Aamir et al., WildDepth 2                                         | Read in part (abstract)                                               |
| arXiv 2610.30340, NVIDIA, Foundation4D                                              | Read in part (abstract); camera-only driving perception, out of scope |
| arXiv past-week listings, cs.RO, cs.CV, eess.SP                                     | Read (1,510 titles filtered; abstracts read for the three above)      |
| Waymo safety research page                                                          | Read (59 entries; nothing new since October 12)                       |
| Truelove and Oviedo-Trespalacios 2026, _Transportation Research Part F_ 119, 103592 | **Confirm** (cited by Anderson; a questionnaire study, not reached)   |
| University of the Sunshine Coast release and press coverage of Anderson, October 9  | Superseded (the paper is read in full)                                |
| Ouster BlueCity, "99% stop bar detection accuracy", October 2026                    | **Confirm** (trade coverage; vendor claim with no method)             |
| AAA Foundation seat-belt research, October 7 coverage                               | **Confirm** (search summary; out of scope)                            |
| NHTSA early estimate, first quarter of 2026; NSC first half of 2026                 | Superseded (seen on October 19; FARS remains the plan's fatal source) |
| IIHS and HLDI speed safety cameras sheet, October 2026                              | Superseded (read in the October 12 briefing)                          |
| Campolettano, Scanlon and Kusano, ESV26-294                                         | Superseded (read in full in the October 12 briefing)                  |

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

Refused from this session: www.sfmta.com (`WebFetch` could not resolve the host; `curl` proxy
403), localnewsmatters.org and api.crossref.org (proxy 403).

## To fetch

- SFMTA, _San Francisco Speed Safety Camera Program 18-Month Report_,
  https://www.sfmta.com/reports/san-francisco-speed-safety-camera-program-18-month-report: the
  instrument, the counting periods, the before and after shares behind the 84% and the
  denominator behind 42,000 a day.
- SafeTREC's preliminary evaluation of the San Francisco pilot, linked from the SFMTA report: the
  control-street selection behind the spillover finding (Q2).
- Truelove and Oviedo-Trespalacios 2026, _Transportation Research Part F_ 119, 103592,
  https://doi.org/10.1016/j.trf.2026.103592: the questionnaire on alerts and enforcement
  legitimacy that Anderson cites, if Q1 is adopted and the guide needs the self-reported side.
- De Pauw, Daniels, Brijs, Hermans and Wets 2014, _Accident Analysis & Prevention_ 73, 132–140:
  the "kangaroo" speed profile around fixed cameras, the method behind the first research-note
  test in the Anderson section. DOI not checked against Crossref from this session.

Carried from October 19 and still unread: the Chu et al. supplementary information and Zenodo
deposit; Hu et al. 2022; Soni et al. 2022; Wen, Cui and Jian 2022 and Hu et al. 2023; Davis 2001;
Wu et al. 2026; Dean, Doerzaph and Riexinger 2025. Their URLs are in the
[October 19 briefing](research-briefing-2026-10-19.md#to-fetch).
