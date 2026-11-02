# Research briefing, October 26 to November 2, 2026

- **Status:** Complete. No definition, benchmark kind or registered metric changed. Adopted: Jang
  (arXiv 2610.42121, October 30), read in full, as an evaluation-source row in the crash-data
  plan, with `Jang2026` in `references.bib` and the reference audit, and one open question on
  crash-type counts in the site crash-context record.
- **Scope:** Roadside and infrastructure LiDAR perception, and the safety and behaviour literature
  the analytics cite, first listed or newly surfaced October 26 to November 2, 2026. No
  third-party briefing was supplied. Nothing the October 10, 12, 19 or 26 briefings evaluated is
  repeated here.
- **Related:**
  [crash-data integration plan](../../../plans/platform-crash-data-integration-plan.md),
  [behaviour analytics plan](../../../plans/lidar-behaviour-analytics-plan.md),
  [attitudes plan](../../../plans/platform-speeding-attitudes-and-aggressive-driving-plan.md),
  [previous briefing](research-briefing-2026-10-26.md)

Paper and research-report citation keys are mapped in the [reference audit](reference-audit.md);
full entries are in [references.bib](../../../../data/maths/references.bib).

All three discovery routes were run. The arXiv past-week listings answered (cs.RO 487 entries,
cs.CV 914, eess.SP 179) and were filtered by title; none of the 1,580 is a roadside or
infrastructure LiDAR perception paper, and the four nearest are recorded below without
evaluation. The organisation watchlist found one release in the window, an IIHS study of Utah's
0.05% blood-alcohol limit, which is outside both bodies of literature. The week's one source that
bears on the analytics came from a topic query: a single-author preprint that compares the mix
of crash types in the federal automated-driving incident files with police-reported human
crashes, without any mileage. It is a composition study, not a rate, and it lands beside the
crash-data plan's area-rate family as a method and a set of cautions, not as a number.

## Answer

Nothing changes a definition, threshold, benchmark kind or registered metric. Jang's preprint
finds turn-across-path and single-vehicle crashes under-represented among driverless crashes
against police-reported human crashes, and the driverless vehicle rarely the striking party in
rear-end crashes. The figures it publishes reproduce, but the counts behind each finding are
small: one to eight turn-across-path crashes per specification, and, by inference from the width
of its interval, roughly six or seven striking rear-end crashes behind the 0.121 ratio.

For velocity.report the paper matters for what it says about shares. A share drawn from one
source compared with a share from another means something only when the two sources record the
same events under the same reporting rule, and only with the window denominator printed beside
it. The crash-data plan's site crash-context record prints counts by severity and mode and no
shares; Q1 asks whether it should also carry counts by crash type and, if it does, holds it to
those two rules. Nothing a survey measures checks any of the paper's findings: the sensor sees
passages, not crashes, and cannot tell whether a vehicle is under automated control.

## Genuinely new this week

| Source                                                                                                      | First listed | Status       |
| ----------------------------------------------------------------------------------------------------------- | ------------ | ------------ |
| Jang, _Where ADS Crashes Differ: A Harmonized Comparison Against Human Baselines_, arXiv 2610.42121 (cs.CY) | October 30   | Read         |
| Following-distance detection with fixed city cameras, _Sensors_ 26(21), 6815, Kocaeli University            | October 28   | **Confirm**  |
| IIHS, Utah's 0.05% per se limit and alcohol-involved crashes (news release)                                 | October 27   | **Confirm**  |
| Zheng, NIF: normal-intersection fields for oriented box refinement, arXiv 2610.39715                        | October 29   | Read in part |
| Rist, Triess, Enzweiler and Gavrila, LiDAR sensor-to-sensor domain generalisation, arXiv 2610.39516 (IV)    | October 29   | Read in part |
| Inoue et al., test-time track-query calibration for multi-object tracking, arXiv 2610.38494                 | October 28   | Read in part |

The _Sensors_ paper estimates speed, time headway, front-to-front spacing and time to collision
from a single fixed camera and tests eight criteria, one a distance of half the speed in km/h,
with a probabilistic flag for uncertain pairs (4.5% of pairs under a headway error model fitted
on the calibration video). All of that is from a search summary of the early-access page: mdpi.com
refused this session, so every figure is **Confirm**. The instrument is a camera, which Tenet 1
excludes from the product, and the title fails the `violat` stem of the wording guard. What it
could still give is an external accuracy figure for camera headway against a stated reference,
the kind of comparison the behaviour plan's headway rows lack. Listed under To fetch.

The IIHS release reports a lasting fall in the share of crashes involving alcohol in Utah after
the 0.05% limit took effect on December 30, 2018, against Arizona, Colorado and Idaho (12% to
21% relative, about 9% over four years; search summaries, **Confirm**; the iihs.org article
returned HTTP 500 to both `WebFetch` and `curl`). It concerns impairment policy. The crash-data
plan's sober cohort is a national exposure reconstruction that this does not touch, and a
roadside sensor cannot observe impairment. Recorded and not evaluated.

NIF refines oriented boxes in aerial images from signed distances at 20 boundary anchors with a
weighted least-squares rectangle fit; the fit is close in spirit to the L4 OBB refinement, but
the inputs are image features on DOTA and HRSC, not sparse returns. Rist et al. supervise LiDAR
semantic segmentation with an accumulated-scan occupancy target to transfer across sensors.
Inoue et al. calibrate appearance-query trackers on DanceTrack and SportsMOT. None measures a
roadside scene. Recorded and not evaluated.

## Newly surfaced older work

| Source                                                                                      | First listed | Why it surfaced this week                                        |
| ------------------------------------------------------------------------------------------- | ------------ | ---------------------------------------------------------------- |
| Safety+Health coverage of the AAA Foundation attitudes study (`Steinbach2026`)              | October 29   | Attitudes topic query; the study is held by the attitudes plan   |
| Zhao et al., road infrastructure perception survey, arXiv 2610.33758                        | October 24   | Listed in the window; vehicle-side lane, sign and map perception |
| Car-following behind automated leaders, _Communications Engineering_ (`Chu2026`), via press | October 2026 | AV topic query; read in the October 19 briefing                  |
| NHTSA safety research portfolio public meeting and ADS workshop, December 1 to 2, 2026      | October 7    | Agency watchlist; an event notice, not a report                  |
| Applied Sciences rectangle edge matching (`Hong2026`) and Schäfer et al. (`Schafer2026`)    | 2026         | Extent topic query; both held since the October 10 briefing      |

Waymo's research page still lists 59 entries, the same count as on October 26; Valgo's news page
still carries only its June 19 announcement. The AAA Foundation's research index returned 404,
and its newest release found by search is still the October 7 seat-belt study. Nothing from
FHWA, FMCSA, SWOV or ITF was first published in the window.

## Jang: where ADS crashes differ (arXiv 2610.42121, October 30, 2026)

1. **Measures.** Read in full from the arXiv PDF (12 pages, dated September 2026, single author,
   not peer reviewed). The question is composition: given a crash, does the mix of crash types
   differ between automated-driving-system (ADS) vehicles and human-driven ones? The ADS side is
   the NHTSA Standing General Order (SGO) incident files, both generations, to incident month
   August 2026: 3,773 filings reduced to 2,668 distinct incidents and 2,450 in scope, of which
   828 fall in 2021 to 2024, the years the human comparator covers. That comparator is the Crash
   Report Sampling System (CRSS), a nationally representative sample of police-reported crashes,
   at vehicle level and restricted to the ADS operating domain. The two populations differ by
   construction: SGO filings are written by the operators and compelled far below the police
   reporting threshold, so the paper brackets reportability with an objective severity floor
   (Tier A: injury B or worse, a tow, or an airbag) and a written rubric (Tier B), and claims a
   difference only where it survives both. A commercial large language model codes each
   operator-written narrative into the CRSS crash-type taxonomy, sampled at the API default
   temperature of 1.0, so a rerun will not reproduce every label. No exposure enters anywhere:
   the paper says plainly that it "says nothing about whether ADS vehicles crash more or less
   often than human drivers".
2. **The maths.** The funnel and the tier shares reproduce from the paper's own numbers.
   - 2,668 − 218 = 2,450 in scope, and 2,450 − 828 = 1,622 outside the window, as Table 1
     states. Under the looser driverless definition, 576 × 0.790 = 455 Waymo incidents and 121
     others, as Table 2 states.
   - Every share in Table 3 reproduces to a tenth of a point: 1,052 / 2,450 = 42.9%,
     243 / 828 = 29.3%, 119 / 828 = 14.4%, 230 / 828 = 27.8%, 295 / 828 = 35.6%, and the rest.
   - Occupancy: the raw any-injury gap is 0.656 − 0.281 = 0.375 and the occupied-only gap
     0.656 − 0.438 = 0.218, so (0.375 − 0.218) / 0.375 = 0.419, the "about 42%" of Table 5.
   - The single-vehicle correction multiplies each ratio by precision over recall, 0.843 / 1.000:
     0.19 → 0.160, 0.42 → 0.354 and 0.16 → 0.135 reproduce the printed 0.16, 0.35 and 0.14; the
     upper no-onboard bound 0.56 → 0.472 against a printed 0.48, which holds if the unrounded
     ratio is at least 0.564. Correction moves every ratio further from parity, as the paper
     says.
   - Table 7's percentages reproduce: 18 / 241 = 7.5%, 20 / 112 = 17.9%, 237 / 512 = 46.3%.

   The rear-end quasi-induced-exposure ratio, 0.121 (95% CI 0.05 to 0.23), cannot be reproduced:
   the striking and struck counts are not printed. Its width bounds them. On a log scale the
   interval runs 0.884 below and 0.642 above the estimate, a mean half-width of 0.763 and a
   standard error near 0.39; if the ADS striking count dominates the variance, as a small Poisson
   count does, that count is about 1 / 0.39² ≈ 6.6. The ratio therefore rests on roughly six or
   seven crashes (an inference from the interval, **Confirm** against the author's counts). The
   sign of the stated bias checks: the ratio divides the striking share by the struck share, so a
   vehicle struck more often than its exposure implies inflates the denominator, lowers the
   ratio, and overstates the ADS-human difference, as the paper says.

   The dominant uncertainty is not sampling on the human side but the ADS side's small counts and
   its coding. Turn-across-path rests on one to eight ADS crashes per cell, and its extraction-free
   upper bound passes parity (1.24 and 1.35) once the human side is entropy-balanced onto the ADS
   domain, so in that arm the finding depends on the language model's coding. The model's
   striking-role recall on the minority class is 27% (7 of 26), and its rubric changed label under
   edits it should have ignored in about 18% of cases. The paper runs the rear-end ratio on the
   structured key for that reason.

3. **What the sensor can and cannot observe.** A velocity.report survey records passages and
   their kinematics. It records no crash as a crash, has no access to a vehicle's automation
   state, and by Tenet 1 identifies no vehicle or person. No finding in the paper is a quantity
   the sensor measures, and no survey result can check one.
4. **Where it lands.** The crash-data plan's
   [evidence table][cd-evidence]
   has four source families; the fourth, exposure-normalised area involvement rates, is where the
   ADS benchmark papers sit. Jang is a fifth kind the plan does not use: composition without a
   denominator. Its two transferable cautions already hold in part. The plan prints no site rate
   and states each count's window, and the October answer on residential windows prints the count,
   the window and the interval. What the plan does not yet say is what happens to a share
   compared across sources with different reporting thresholds; the site crash-context record
   carries counts by severity and mode only, so the question arises only if crash type is added
   (Q1). The paper corroborates two figures the plan's `Teoh2026` row relies on: its rubric gives
   26.8% reportable against IIHS's 21.6% on IIHS's frame, and it finds 52.1% of driverless
   crashes unoccupied against the IIHS estimate of about 48%. The repository holds no reference
   to the SGO files, CRSS or quasi-induced exposure (Stamatiadis and Deacon 1997), checked by
   grep. The title passes the wording guard in `internal/report/headway/wording.go`; the
   abstract's "striking propensity" fails the `propensity` stem, so the plan cites it in the docs
   and never on a page.
5. **Verdicts.**

   | Claim or recommendation                                                                           | Verdict                                                                                                                                                                |
   | ------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
   | Crash-type composition can be compared without an exposure denominator                            | New and adopted as an evaluation-source row in the crash-data plan, read in full. It names a method, not a value: no figure enters the edition, a benchmark or a page. |
   | Turn-across-path and single-vehicle crashes are under-represented among ADS crashes               | Logged as context here. Small counts, and the turn-across-path finding depends on the coding in the balanced arm; the sensor cannot observe either.                    |
   | The ADS vehicle is the striking party in rear-end crashes at 0.121 of the human ratio             | Logged as context. About six or seven crashes by inference from the interval; **Confirm** the counts.                                                                  |
   | Every share from the federal ADS files should state its comparison-window denominator             | Already holds in spirit: the crash-data plan states every count's window. Extended to crash-type shares only if Q1 adds them.                                          |
   | A tow criterion is confounded by fleet recovery; occupancy explains about 42% of the severity gap | Not adopted: the plan uses neither SGO nor a tow criterion. Recorded so that any later use of SGO reads it first.                                                      |
   | A language model can code operator narratives into the CRSS taxonomy at 0.744 crash-type accuracy | Not adopted: no part of the plan codes free text, and a sampled, non-reproducible coding would fail the plan's rebuildable-provenance rule for the context record.     |

6. **Rights.** The arXiv listing carries the arXiv non-exclusive licence to distribute 1.0, which
   grants arXiv the right to distribute and grants others nothing beyond reading. The briefing
   cites and paraphrases; no table or figure is reproduced. The paper's inputs, the SGO files and
   CRSS, are NHTSA publications.
7. **What a velocity.report survey could give back.** Nothing testable. The paper is about
   crashes and automation state; a survey sees neither, and adding either would need a camera or
   identity, which Tenet 1 rules out.

[cd-evidence]: ../../../plans/platform-crash-data-integration-plan.md#what-the-evidence-can-and-cannot-establish

## What changes

In this change:

- [Crash-data integration plan](../../../plans/platform-crash-data-integration-plan.md): a
  `Jang2026` row in Sources checked, Q1 under Open questions, and this briefing in Related.
- [`references.bib`](../../../../data/maths/references.bib): `Jang2026`, mapped in the
  [reference audit](reference-audit.md).
- One devlog bullet under November 2, 2026.

Nothing in code, the metrics registry, benchmark kinds or definitions.

## Open questions

### Q1: crash type in the site crash-context record

The site crash-context record carries counts by severity class and road-user mode. Jurisdiction
crash records also code the manner of collision (rear end, angle, sideswipe, and so on), and a
reader of a site report may ask what kinds of crash the street's history holds. Should the record
add counts by crash type as the source codes it?

If it does, Jang's cautions set two rules. A crash-type count is printed with the source's window
and its reporting threshold, as the severity counts already are. And a crash-type share is never
set beside a share from another source, an area or a national sample unless both record the same
events under the same reporting rule; DataSF injury crashes, SWITRS and CRSS each have their own.
The record would stay a count, never a rate and never a comparison. The decision is the owner's;
nothing is built until it is made. The October 26 briefing's Q1 and Q2 and the October 19
briefing's Q1 remain open and are not repeated.

## Sources checked

| Source                                                                                   | Status                                                                        |
| ---------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------- |
| Jang, arXiv 2610.42121 (`Jang2026`)                                                      | Read (arXiv PDF, 12 pages, with Appendix A)                                   |
| _Sensors_ 26(21), 6815, following-distance detection with fixed city cameras             | **Confirm** (mdpi.com and doi.org returned 403; search summary only)          |
| IIHS, Utah 0.05% limit and alcohol-involved crashes, October 27, 2026                    | **Confirm** (iihs.org article returned 500; search summaries only)            |
| arXiv 2610.39715, 2610.39516, 2610.38494, 2610.33758                                     | Read in part (abstracts)                                                      |
| arXiv past-week listings, cs.RO, cs.CV, eess.SP                                          | Read (1,580 titles filtered)                                                  |
| Waymo safety research page                                                               | Read (59 entries; unchanged since October 26)                                 |
| Valgo news page                                                                          | Read (one post, June 19, 2026)                                                |
| IIHS news index                                                                          | Read (ten most recent items, to October 27)                                   |
| AAA Foundation research index                                                            | **Confirm** (404 from this session; newest release found by search October 7) |
| Safety+Health, AAA Foundation attitudes coverage, October 29                             | Superseded (`Steinbach2026` is held by the attitudes plan)                    |
| _Communications Engineering_ car-following behind automated leaders, press coverage      | Superseded (`Chu2026`, read in the October 19 briefing)                       |
| NHTSA Federal Register notice, safety research portfolio public meeting, December 1 to 2 | **Confirm** (search summary; an event, not a report)                          |
| `Hong2026`, `Schafer2026`                                                                | Superseded (held since the October 10 briefing)                               |

## Provenance

Searched November 2, 2026, extended mode unless stated: roadside LiDAR vehicle tracking
infrastructure point cloud paper November 2026; Waymo safety research new paper October 2026;
AAA Foundation for Traffic Safety new research report late October 2026; human crash baseline
automated driving Valgo humanbaselines 2026; IIHS speeding study October November 2026; following
distance tailgating headway study 2026 FMCSA; speeding aggressive driving national survey results
published October 2026; pedestrian impact speed fatality risk curve new study 2026; 85th
percentile speed limit setting research 2026 FHWA NCHRP report; post-encroachment time
time-to-collision LiDAR surrogate safety study 2026 preprint; stop sign compliance full stop
rolling stop observational study 2026; crash involvement rate per vehicle mile exposure
denominator study 2026 Traffic Injury Prevention; autonomous vehicles nearby human drivers take
more risks Communications Engineering October 2026; speed change crash model power model Nilsson
Elvik new study 2026; new vehicle trajectory dataset released October 2026 drone LiDAR
intersection highD inD; LiDAR headway measurement following distance enforcement 2026; SWOV ITF
OECD speed road safety report published October 2026; sober driving baseline automated driving
comparison 2026 preprint; IIHS Utah .05 BAC law linked to drop in alcohol-related crashes October
2026 study; the _Sensors_ paper's title; infrastructure LiDAR vehicle length extent heading
estimation 2026 IEEE ITSC T-ITS; NHTSA FHWA FMCSA speeding research report released October
2026; and, in standard mode, a DOI for a Frontiers post-encroachment-time paper that one search
summary named. The `preprint` variants of the topics were folded into the arXiv listing route
rather than run as separate web queries.

That Frontiers DOI, 10.3389/ffutr.2026.1893114, could not be resolved: a second search found no
page for it and www.frontiersin.org does not resolve from this session. It is not listed as a
source, and no claim about it is made.

arXiv: the past-week listings for cs.RO, cs.CV and eess.SP were fetched from arxiv.org with
`curl` on November 2 and filtered by title for LiDAR, point cloud, roadside, infrastructure,
traffic, headway, car-following, crash, collision, pedestrian, intersection, trajectory,
occlusion, tracking and radar terms; abstract pages for the five papers above. Jang's PDF was read
with `curl` and `pdftotext -layout`; the arithmetic in the Jang section is from its Tables 1 to 7.
`WebFetch` read the Waymo research page, the Valgo news page and the IIHS news index. The PDFs are
not committed.

## To fetch

- _Sensors_ 26(21), 6815, https://www.mdpi.com/1424-8220/26/21/6815 (doi:10.3390/s26216815):
  the camera headway and spacing error against its reference, the eight criteria and the
  uncertainty model, if the behaviour plan wants an external headway accuracy figure.
- IIHS Utah 0.05% study, https://www.iihs.org/news/detail/utah-law-linked-to-drop-in-alcohol-related-crashes,
  and the underlying paper it links, only if the crash-data plan's sober cohort is revisited.
- Jang's striking and struck rear-end counts behind the 0.121 ratio: not in the paper; the
  author's address is on its title page, and the counts are only needed if Q1 grows into a
  comparison, which it is not meant to.
- Stamatiadis and Deacon 1997, _Accident Analysis & Prevention_ 29(1), 37 to 52,
  https://doi.org/10.1016/S0001-4575(96)00060-7: the quasi-induced-exposure method, if any later
  plan uses it.

Carried from October 26 and still unread: the SafeTREC San Francisco evaluation, Truelove and
Oviedo-Trespalacios 2026, and De Pauw et al. 2014; and from October 19, the items listed in that
briefing's To fetch section.
