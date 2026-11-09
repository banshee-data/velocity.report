# Research briefing, November 2 to 9, 2026

- **Status:** Complete. No definition, benchmark kind, threshold or registered metric changed.
  Adopted: Salah, Li and Wang (arXiv 2611.04988, November 3), read in full, as a reference in
  the behaviour analytics plan's weekly literature list, with `Salah2026` in `references.bib` and
  the reference audit, and two open questions on Section 5.1 and Section 8.4 of that plan.
- **Scope:** Roadside and infrastructure LiDAR perception, and the safety and behaviour literature
  the analytics cite, first listed or newly surfaced November 2 to 9, 2026. No third-party
  briefing was supplied. Nothing the October 10, 12, 19 and 26 or November 2 briefings evaluated
  is repeated here.
- **Related:**
  [behaviour analytics plan](../../../plans/lidar-behaviour-analytics-plan.md),
  [state estimation plan](../../../plans/lidar-state-estimation-plan.md),
  [crash-data integration plan](../../../plans/platform-crash-data-integration-plan.md),
  [previous briefing](research-briefing-2026-11-02.md)

Paper and research-report citation keys are mapped in the [reference audit](reference-audit.md);
full entries are in [references.bib](../../../../data/maths/references.bib).

All three discovery routes were run. The arXiv past-week listings answered (cs.RO 424 entries,
cs.CV 958, eess.SP 205, November 3 to 9) and were filtered by title. Five of the 1,587 concern a
roadside or infrastructure sensor, and one of the five, a structured review of surrogate safety
measures from roadside sensing accepted for the 2027 TRB Annual Meeting, bears directly on the
behaviour plan. The organisation watchlist found nothing new in the window: Waymo, Valgo and IIHS
list the same items as on November 2, and no FHWA, FMCSA, NHTSA, SWOV or ITF report was first
published. The week's main finding is therefore a review that agrees with the plan's existing
rules rather than one that moves them.

## Answer

Nothing changes a definition, threshold, benchmark kind or registered metric. Salah, Li and Wang
organise surrogate safety measures by conflict geometry and by when each value becomes known,
and argue that a perception pipeline can score well on detection benchmarks and still produce
trajectories unfit for conflict inference. The behaviour plan already holds both points: it
classifies interaction type before any pairwise formula (Section 8, "Do not apply the
longitudinal TTC formula to arbitrary crossing trajectories") and gates PET on both crossings
being in view with crossing-time uncertainty below the reported precision. The review's threshold
values are all secondary: a 1.5 s TTC it attributes to Li et al. 2024 (the plan cites FHWA SSAM
for the same value), a PET screen "of up to 6 s" and a k-means TTC cut of 1.15 s, both from Singh
et al. 2024. None enters Section 5.1 from a review; Q1 asks whether Singh et al. should be read
for a row. The review also passes on, from Li et al. 2024, that one-dimensional TTC departs from
the two-dimensional form by about 20% even below 1.5 s. That would put a number on the plan's
interaction-type rule if the primary bears it out (Q2).

The two UrbanTwin challenge reports from the same week are useful mostly as confirmation. One
finds that about 31% of an Ouster OS1-64 roadside cloud is null returns written as the point
(0, 0, 0); the L1 Hesai decoder already drops zero-distance returns. The other measures a car's
visible-surface centroid at roughly 0.8 m from its box centre, which the state estimation plan
already treats as a viewpoint-dependent bias of up to half the vehicle's extent. Nothing a survey
measures checks the review's control-side claims: velocity.report reports, offline, on passages
and does not feed a controller.

## Genuinely new this week

| Source                                                                                                                               | First listed | Status       |
| ------------------------------------------------------------------------------------------------------------------------------------ | ------------ | ------------ |
| Salah, Li and Wang, _From Roadside Sensing to Risk-Informed Safety Control: Surrogate Safety Measures as a Cyber-Physical Interface_ | November 3   | Read         |
| Upganlawar, _Geometric Refinement and Distributional Alignment for Sim2Real Roadside LiDAR Detection_, arXiv 2611.03936              | November 2   | Read in part |
| Upganlawar, _Coordinate Frame Recovery, Per Class NMS and Null Return Modelling for Sim2Real V2X LiDAR Detection_, arXiv 2611.03940  | November 2   | Read in part |
| Sun, Huang and Cao, CoMET: shared pose error of roadside messages, arXiv 2611.09121                                                  | November 6   | Read in part |
| Alsakabi et al., roadside 4D radar vehicle classification by point-cloud dispersion, arXiv 2611.02147                                | November 1   | Read in part |
| Roadside Technologies VECTA, LiDAR and AI roadworks hazard detection (Highways Today, November 7)                                    | November 7   | **Confirm**  |
| City of Gaithersburg, Maryland, stop-sign compliance study in school zones, November 9 to 23                                         | November     | **Confirm**  |
| AAA Foundation project listing, _Assessment of Different Technologies in Meeting Requirements of the HALT Act_                       | November     | **Confirm**  |

CoMET estimates the translation error shared by every box in a roadside message from objects both
the vehicle and the roadside unit see, and carries it forward with a Kalman filter per sender
when no pair matches; on V2X-Seq it finds the benchmark's offset is a zero placeholder in 28% of
validation messages. It is cooperative vehicle-infrastructure detection and has no bearing on a
single fixed sensor, though its finding that a benchmark's calibration labels can be placeholders
is the same caution the LUMPI report makes about its own validation harness. Alsakabi et al.
classify cars against motorcycles from the temporally smoothed spread of 4D radar clusters, error
52.1% to 42.7%, against a stereo camera reference; the project's radar is a one-dimensional
Doppler unit that cannot form such clusters. Both are recorded and not evaluated.

The VECTA item is a vendor deployment that pairs LiDAR with CCTV for roadworks alerts, with no
published measurement. The Gaithersburg release describes unspecified "data collection devices"
at school-zone stop signs from November 9 to 23, not for enforcement; gaithersburgmd.gov did not
resolve from this session, so the sensor type and method are unknown. The AAA Foundation entry
is a project title under Vulnerable Road Users with no report. All three are search summaries
only and carry **Confirm**.

## Newly surfaced older work

| Source                                                                                            | First listed       | Why it surfaced this week                                    |
| ------------------------------------------------------------------------------------------------- | ------------------ | ------------------------------------------------------------ |
| Roadside LiDAR-camera target-free wide-area calibration, _PLOS ONE_, e0356394                     | 2026               | Roadside LiDAR topic query; date not established             |
| Zhong and Dutta, stop-sign compliance prediction on a closed campus, SSRN 6321442                 | Not established    | Stop compliance topic query; not a LiDAR study               |
| SIT-PET, multimodal trajectories with PET interaction events, _Data_ 11(4), 68                    | March 2026         | PET topic query; video or LiDAR tracking at one intersection |
| PRISA (`Bang2026b`) and the review _Roadside lidar-based scene understanding_ (`Zhang2026b`)      | 2026               | Topic queries; both held since the October 10 briefing       |
| Schäfer et al. (`Schafer2026`), Hong et al. (`Hong2026`)                                          | 2026               | Tracking and extent queries; both held                       |
| AAA Foundation _2025 Traffic Safety Culture Index_ (`Zhang2026a`), Colorado driving questionnaire | June and July 2026 | Attitudes queries; the index is held by the attitudes plan   |

The _PLOS ONE_ calibration paper reports a mean rotation error of 0.185° and translation error of
2.36 cm (search summary, **Confirm**; journals.plos.org did not resolve). SIT-PET defines PET for
eight predefined conflict scenarios at one signalised intersection; mdpi.com was not tried again
after refusing every request in the October and November 2 runs, and it is listed under To fetch
for the behaviour plan's PET work.

Waymo's research page still lists 59 entries, the same count as on October 26 and November 2;
Valgo's news page still carries only its June 19 announcement; the IIHS news index's newest item
is still the October 27 Utah release.

## Salah, Li and Wang: surrogate safety measures from roadside sensing (arXiv 2611.04988, November 3, 2026)

1. **Measures.** Read in full from the arXiv PDF (21 pages, City College of New York, "Accepted
   by 2027 TRB Annual Meeting", not yet in proceedings). It is a structured narrative synthesis,
   not a systematic review: Scopus, Web of Science, TRID, IEEE Xplore, Google Scholar, arXiv and
   SSRN, January 2015 to August 1, 2026, three term blocks (sensing, safety measures, control)
   combined pairwise, 56 sources retained, "no pre-registered protocol, and no formal quality
   appraisal". It measures nothing itself. Its contributions are a comparison of camera, roadside
   LiDAR and drone sensing for safety-grade trajectories; a table of surrogate measures (TTC,
   modified TTC, two-dimensional TTC, PET, initially attempted PET, proportion of stopping
   distance, gap time) by conflict geometry and by "temporal availability", anticipatory against
   event-completion only; and a four-part "risk state" of exposure, imminence, potential severity
   and persistence offered as "an organizing schema rather than a single mandated
   implementation" for feeding signal control and automated vehicles.
2. **The maths.** The review derives nothing and reproduces no figure; there is no equation
   beyond the schematic risk-state vector. What can be checked is attribution. Its Table 2 gives
   "a common threshold used is 1.5 seconds" for TTC and cites Li et al. 2024; the behaviour plan
   gives the same 1.5 s from FHWA SSAM (FHWA-HRT-08-049), read in the October 12 briefing, and
   the value is older than either. The review's own text then reports Singh et al. 2024's k-means
   cut at 1.15 s, "lower than the 1.5 s threshold in Table 2, underscoring that thresholds are
   site- and study-specific", which is the plan's position. Table 2 also gives a "generalized
   threshold of up to 6 s" for PET from Singh et al. 2024, with a turning conflict area of
   3.5 m by 3.5 m, against SSAM's 5 s candidacy default in the plan. Those three values are the
   review's reading of other papers; this briefing has read none of the cited primaries, so each
   is **Confirm** and none is a candidate for Section 5.1 on this evidence. The one quantitative
   claim with a bearing on a registered metric is that one-dimensional TTC values "can deviate
   substantially from higher-dimensional ones, by roughly 20% even for critical TTC values below
   1.5 s" (from Li et al. 2024; **Confirm**). The review gives no sign for the deviation, so the
   direction of the bias cannot be taken from it.

   The dominant uncertainty is selection. With no protocol and no quality appraisal, the review
   shows what 56 chosen sources say, not what the literature says, and its sensing comparison
   (Table 1) is a qualitative summary rather than a measured one.

3. **What the sensor can and cannot observe.** A velocity.report survey can supply the review's
   inputs for passages in view: positions, extents, headings and speeds, from which TTC, PET and
   gap measures follow under the plan's gates. It does not supply the review's outputs. The risk
   state's "potential severity" leans on road-user type and a vulnerable road user's presence,
   which the sensor infers only as a class with stated confidence; "imminence" for in-event
   warnings is a real-time control use the project does not have, and Tenet 4 rules out the
   runtime network call that feeding a controller would need. The review's privacy argument for
   LiDAR over cameras agrees with Tenet 1 but is stated, not tested: an anonymous point cloud is
   the review's assumption, and the project's own guarantee is architectural.
4. **Where it lands.** Checked by grep against the behaviour plan, the metrics registry and
   `internal/lidar/l8behaviour/metrics.go`.
   - Interaction type before formula: already holds, Section 8 and its interaction-type table,
     which limits `THW` and longitudinal `TTC` to following and assigns `PET` to crossing, merging
     and turning.
   - Perception-valid against safety-valid output: already holds as the PET gate ("both
     crossings fully inside the field of view; crossing-time uncertainty below the reported
     precision") and the TTC uncertainty row ("As closing speed approaches zero, TTC diverges").
   - PET as a measured fact about two crossing times, needing no motion model: already holds,
     Section 8.6. The review's "event-completion only" label is the same property seen from the
     control side; for a report that is computed offline it changes nothing.
   - The interaction, conflict and near-miss hierarchy (Hayward 1972; Archer 2005; Laureshyn et
     al. 2010): `Laureshyn2010` is in `references.bib` and the plan lists it as "Not yet
     consulted directly". The review cites it the same way PRISA does, so it does not close
     that note.
   - Thresholds: Section 5.1 holds TTC ≤ 1.5 s (SSAM), PET ≤ 5 s (SSAM default), PET about 1 to
     1.5 s and DRAC; no 6 s PET and no 1.15 s TTC. Q1.
   - Two-dimensional TTC: the plan names it as the alternative for crossing trajectories and
     gives no quantity for the error of the longitudinal form; Li et al. 2024 is not in
     `references.bib`. Q2.
   - Extreme-value crash-risk estimation (Anis et al. 2025, 2026): not in the repository, and a
     crash-probability output would be a `published_model` at best and a verdict word on a page
     at worst. Not adopted.
   - Wording: the title fails the `risk` stem of `internal/report/headway/wording.go`, and the
     introduction's "aggressive right-turns-on-red" fails `aggress`. The plan cites the paper in
     the docs and never on a page.
5. **Verdicts.**

   | Claim or recommendation                                                                     | Verdict                                                                                                                                                |
   | ------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
   | Match each surrogate measure to conflict geometry; longitudinal TTC only for following      | Already holds: behaviour plan Section 8, interaction-type rule. Adopted as a supporting reference in the plan's weekly literature list.                |
   | A pipeline can be benchmark-valid and still unfit for conflict inference                    | Already holds: the PET and TTC gates in the behaviour plan's uncertainty and gating tables.                                                            |
   | Classify measures by temporal availability, anticipatory against event-completion only      | Not adopted: the distinction matters for real-time control, which the project does not do; PET's offline status is already stated.                     |
   | TTC threshold 1.5 s (attributed to Li et al. 2024)                                          | Already holds from the primary the plan cites (SSAM); the review's attribution is not used.                                                            |
   | PET screen up to 6 s and TTC cut 1.15 s (Singh et al. 2024)                                 | New and logged as Q1. Secondary, **Confirm**; nothing enters Section 5.1 from a review.                                                                |
   | One-dimensional TTC departs from the two-dimensional form by about 20% below 1.5 s          | New and logged as Q2. Secondary, **Confirm**; no direction given.                                                                                      |
   | Compose a four-part risk state (exposure, imminence, severity, persistence) for controllers | Not adopted: a composite of that kind is a score, which the wording guard forbids on a page, and its control use is outside the project.               |
   | Extreme-value models turn surrogate measures into short-term crash probability              | Not adopted: a modelled crash probability for a site would print a forecast the sensor cannot check, and the plan has no `published_model` row for it. |

6. **Rights.** The arXiv listing carries the arXiv non-exclusive licence to distribute 1.0, which
   grants others nothing beyond reading. The briefing cites and paraphrases, with short quotations
   for definitions; no table or figure is reproduced.
7. **What a velocity.report survey could give back.** One testable research note, conditional on
   Q2: on a site where following and crossing passages are both in view, compute longitudinal and
   two-dimensional TTC for the same following pairs with the plan's uncertainty propagation, and
   report the distribution of their ratio by closing speed. That is a property of the formulas on
   observed passages, not a finding about anyone, and it would say whether the 20% figure holds
   for a fixed roadside sensor's trajectories.

## Upganlawar: two UrbanTwin Sim2Real roadside LiDAR reports (arXiv 2611.03936 and 2611.03940, November 2, 2026)

1. **Measures.** Two single-author, eight-page challenge reports, first place in Tracks 6
   (LUMPI, a five-sensor roadside array in Hannover) and 7 (V2X-Real, an Ouster OS1-64 roadside
   unit) of the DriveX UrbanTwin challenge at an ECCV 2026 workshop. A PointPillars detector is
   trained only on synthetic twins of each site and scored on 50 hidden real frames, alongside a
   realism score for the synthetic clouds. Read: LUMPI Sections 1 to 5 and V2X-Real Section 5.1
   and its provenance section. `Shahbaz2026`, the UrbanTwin dataset paper, is already held.
2. **The maths.** The figures reproduce from the reports' own numbers.
   - Combined score, 0.6 × (mAP / 0.7) + 0.4 × realism: LUMPI 0.6 × 0.1333 / 0.7 + 0.4 × 0.9126 =
     0.4793; V2X-Real 0.6 × 0.4382 / 0.7 + 0.4 × 0.9251 = 0.7456, printed as 0.7457 from
     unrounded inputs.
   - Relative gains: Person AP 0.0346 to 0.0645 is +86%; Van 0.0073 to 0.0317 is +334% (printed
     335%) and 0.0310 to 0.1713 on the held-out slice +453%; Person size scaling 0.1298 to 0.1491
     is +14.9%.
   - The pedestrian localisation argument: a 0.15625 m cell at stride 2 quantises centres to
     0.3125 m; a 0.76 m pedestrian spans 2.43 cells and a 2.0 m car 6.4, so the same centre error
     costs the pedestrian 6.4 / 2.43 = 2.63 times the IoU, the "roughly 2.6" printed.
3. **What the sensor can and cannot observe.** Both reports are about a learned detector's
   transfer from simulation, which the project does not use: L4 clusters foreground points and
   fits boxes geometrically. Two measured properties of roadside clouds transfer regardless of
   the detector.
4. **Where it lands.**
   - Null returns: about 31% of the OS1-64 cloud as delivered is points at (0, 0, 0), "a property
     of the OS1-64 output convention as delivered in this dataset". Already holds for the
     project's sensor: `internal/lidar/l1packets/parse/extract.go` skips a channel whose raw
     distance is 0 ("0 = no return" in its field comment).
   - Visible-surface centroid: "a car is observed only on its sensor facing surfaces, so its
     visible centroid sits roughly 0.8 m from its true centre", which is why replacing a car's
     box centre with its point centroid cost 62.9 points of AP. Already holds: the
     [state estimation plan](../../../plans/lidar-state-estimation-plan.md) treats the
     measurement as "a visible-surface point, not an object centre", with a bias of up to half the
     vehicle's width or length (P1).
   - Matched-pair size calibration (median ratio over confident matched pairs only) and the
     audit that found a validation harness with van boxes of pedestrian size: methods for a
     learned detector's dimension bias. Not adopted; the project's extent work is scored against
     its own labels in the state estimation plan.
5. **Verdicts.** Null returns: already holds (L1 decoder). Visible-surface bias: already holds
   (state estimation plan, P1). Pedestrian centroid refinement: not adopted, since L4 boxes are
   fitted to the points already. Multiplexed per-class submissions: not adopted, a leaderboard
   method with no counterpart in the project's replay evaluation.
6. **Rights.** Both are CC BY 4.0 on arXiv, so figures may be reproduced with credit; none is.
7. **What a velocity.report survey could give back.** Nothing new: the visible-surface offset is
   already measured on the project's own captures under the state estimation plan.

## What changes

In this change:

- [Behaviour analytics plan](../../../plans/lidar-behaviour-analytics-plan.md): a November 9 entry
  in the weekly literature review list naming `Salah2026`, Q1 and Q2.
- [`references.bib`](../../../../data/maths/references.bib): `Salah2026`, mapped in the
  [reference audit](reference-audit.md).
- One devlog bullet under November 9, 2026.

Nothing in code, the metrics registry, benchmark kinds, Section 5.1 or definitions.

## Open questions

### Q1: Singh et al. 2024 PET and TTC values for Section 5.1

The review reports a PET screen "of up to 6 s" and a k-means TTC cut of 1.15 s, both from Singh
and colleagues (2024). Section 5.1 admits a value only as the kind its source justifies, read in
the primary. Should that paper be fetched and, if its 6 s is a candidacy screen as SSAM's 5 s is,
recorded as a second `research_threshold` candidacy row beside SSAM, or left out as one more
study-specific value? The case for a row is that two screens a second apart make the plan's point
that a screen is not a severity line; the case against is that Section 5.1 is a list of values the
plan may print, and a second screen adds none the report needs. Nothing changes until the primary
is read.

### Q2: a quantity for the longitudinal TTC rule

Section 8 forbids the longitudinal TTC formula on crossing trajectories and gives no measure of
how wrong it would be. Li et al. 2024, as the review reports it, puts the departure at about 20%
even below 1.5 s, direction unstated. Should the plan read Li et al. 2024 and, if the figure
holds, cite it as the quantified reason for the rule, and should the research note in item 7 of
the Salah section be logged with the PET work? This would add a citation and possibly a
research note, never a threshold. The November 2 briefing's Q1, the October 26 briefing's Q1 and
Q2 and the October 19 briefing's Q1 remain open and are not repeated.

## Sources checked

| Source                                                                            | Status                                                                |
| --------------------------------------------------------------------------------- | --------------------------------------------------------------------- |
| Salah, Li and Wang, arXiv 2611.04988 (`Salah2026`)                                | Read (arXiv PDF, 21 pages)                                            |
| Upganlawar, arXiv 2611.03936 (LUMPI track)                                        | Read in part (arXiv PDF, Sections 1 to 5)                             |
| Upganlawar, arXiv 2611.03940 (V2X-Real track)                                     | Read in part (arXiv PDF, Section 5.1 and Section 9)                   |
| arXiv 2611.09121, 2611.02147, 2611.04397, 2611.04871                              | Read in part (abstracts)                                              |
| arXiv past-week listings, cs.RO, cs.CV, eess.SP                                   | Read (1,587 titles filtered)                                          |
| Waymo safety research page                                                        | Read (59 entries; unchanged since October 26)                         |
| Valgo news page                                                                   | Read (one post, June 19, 2026)                                        |
| IIHS news index                                                                   | Read (ten most recent items, to October 27)                           |
| Highways Today, VECTA roadworks hazard detection, November 7                      | **Confirm** (search summary only)                                     |
| City of Gaithersburg stop-sign compliance study release                           | **Confirm** (gaithersburgmd.gov did not resolve; search summary only) |
| AAA Foundation, HALT Act technologies project listing                             | **Confirm** (search summary of the Our Work page)                     |
| _PLOS ONE_ e0356394, roadside LiDAR-camera target-free calibration                | **Confirm** (journals.plos.org did not resolve; search summary only)  |
| SIT-PET, _Data_ 11(4), 68                                                         | **Confirm** (search summary only)                                     |
| Zhong and Dutta, SSRN 6321442                                                     | **Confirm** (search summary only)                                     |
| Singh et al. 2024; Li et al. 2024, as cited in `Salah2026`                        | **Confirm** (not fetched; cited values only)                          |
| `Bang2026b`, `Zhang2026b`, `Schafer2026`, `Hong2026`, `Zhang2026a`, `Shahbaz2026` | Superseded (held since earlier briefings)                             |

## Provenance

Searched November 9, 2026, extended mode: roadside LiDAR tracking November 2026; Waymo safety
research new paper November 2026; AAA Foundation for Traffic Safety new research report November
2026; IIHS study speeding crash November 2026; human crash baseline automated driving study 2026
preprint Valgo; following distance tailgating headway study November 2026; pedestrian impact
speed fatality risk curve study 2026; 85th percentile speed limit setting research November 2026
FHWA NCHRP; post-encroachment time threshold surrogate safety LiDAR 2026 preprint; stop sign
compliance observational study LiDAR 2026; crash involvement rate per vehicle mile exposure study
November 2026; speeding aggressive driving survey results released November 2026; new vehicle
trajectory dataset released November 2026 LiDAR drone intersection; NHTSA FHWA FMCSA speeding
research report November 2026; speed change crash model Nilsson power model Elvik 2026 new study;
LiDAR headway measurement following distance enforcement 2026; SWOV ITF OECD road safety speed
report published November 2026; infrastructure LiDAR vehicle extent heading estimation 2026 IEEE
ITSC T-ITS; sober driving baseline automated driving comparison November 2026. As in earlier
weeks, the `preprint` variants of the topics were folded into the arXiv listing route rather than
run as separate web queries.

arXiv: the past-week listings for cs.RO, cs.CV and eess.SP were fetched from arxiv.org with
`curl` on November 9 and filtered by title for LiDAR, point cloud, roadside, infrastructure,
traffic, headway, car-following, crash, collision, pedestrian, intersection, trajectory,
occlusion, tracking and radar terms; metadata for the six papers above came from the arXiv
export API, and licences from their abstract pages. The three PDFs were read with `curl` and
`pdftotext -layout`; the arithmetic in the Upganlawar section is from the LUMPI report's
Sections 1 to 3 and the V2X-Real abstract. `WebFetch` read the Waymo research page, the Valgo news
page and the IIHS news index; journals.plos.org and www.gaithersburgmd.gov did not resolve. The
PDFs are not committed.

## To fetch

- Singh et al. 2024, the source of the 6 s PET screen and 1.15 s TTC cut: the full reference is in
  the bibliography of https://arxiv.org/abs/2611.04988 and was not fetched. Needed only for Q1.
- Li et al. 2024, the generalised two-dimensional TTC framework and its 20% figure, also in that
  bibliography. Needed for Q2.
- SIT-PET, https://www.mdpi.com/2306-5729/11/4/68: the PET event definitions and scenario
  geometry, for comparison with the plan's Section 8.6 when PET work starts.
- _PLOS ONE_ e0356394, https://journals.plos.org/plosone/article?id=10.1371/journal.pone.0356394:
  the calibration reference behind the 0.185° and 2.36 cm figures, if the extrinsic calibration
  work wants an external target-free figure.
- City of Gaithersburg release, https://www.gaithersburgmd.gov/Home/Components/News/News/10247:
  the device type, only if it turns out to be a roadside range sensor.

Carried from November 2 and still unread: the _Sensors_ 26(21), 6815 camera headway paper, the
IIHS Utah study, Jang's rear-end counts and Stamatiadis and Deacon 1997; and from October 26 and
19, the items listed in those briefings' To fetch sections.
