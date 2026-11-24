# Research briefing, November 16 to 23, 2026

- **Status:** Complete. No definition, benchmark kind, threshold or registered metric changed.
  Nothing adopted as a reference. Two open questions: one on the demand card plan's counting-line
  crossings, from GateTrace (He, Yu and Wong, arXiv 2611.26388, November 19), and one on the
  behaviour analytics plan's `braking_onset`, from Liu, Engström and Markkula (Waymo, arXiv
  2507.17943, newly listed in a journal this week). Both read in full.
- **Scope:** Roadside and infrastructure LiDAR perception, and the safety and behaviour literature
  the analytics cite, first listed or newly surfaced November 16 to 23, 2026. No third-party
  briefing was supplied. Nothing the October 10, 12, 19 and 26 or November 2, 9 and 16 briefings
  evaluated is repeated here.
- **Related:**
  [behaviour analytics plan](../../../plans/lidar-behaviour-analytics-plan.md),
  [SUMO demand card plan](../../../plans/lidar-sumo-demand-card-plan.md),
  [Columbus and Broadway counting-line headways](../sumo-columbus-broadway-headways-2026-10.md),
  [previous briefing](research-briefing-2026-11-16.md)

Paper and research-report citation keys are mapped in the [reference audit](reference-audit.md);
full entries are in [references.bib](../../../../data/maths/references.bib).

All three discovery routes were run. The arXiv past-week listings answered (cs.RO 511 entries,
cs.CV 1,156, eess.SP 144, announced November 17 to 23, 1,688 unique after cross-lists) and were
filtered by title. Three more UrbanTwin roadside LiDAR challenge reports appeared, and one camera
paper on counting crossings under occlusion bears directly on how the project counts passages at
a line. The organisation watchlist found one change: Waymo's research page lists 60 entries, one
more than on November 16, and the new one is a brake-onset method published in _Transportation
Research Interdisciplinary Perspectives_. Valgo, IIHS and the AAA Foundation list nothing new,
and no FHWA, FMCSA, NHTSA, SWOV or ITF report was first published in the window. The two
evaluated papers each find a measurement gap the project has not yet measured: missed crossings,
which the demand card's duplicate rule cannot see, and the lag a threshold rule puts into a
braking onset.

## Answer

Nothing changes a definition, threshold, benchmark kind or registered metric.

GateTrace counts crossings of a virtual line from pedestrian tracks in MOT20 video and asks which
annotated crossings the trackers miss, and where in the pipeline each is lost. Three trackers
sharing one detector recover 18.5% to 32.8% of crossings by mostly hidden people on the
confirmation sequences, against 92.7% to 96.6% of clearly visible ones, and most misses happen
before any counting filter runs: removing the filters recovers few and costs precision. The
project's demand card plan counts vehicle crossings at a line in the same way. It drops
split-track duplicates under 0.8 s and records the count dropped, which guards precision, but it
has no measure of missed crossings, and its own evidence report names kerb-lane vehicles hidden
behind inner-lane vehicles as unmeasured. Q1 asks whether the plan should state crossing recall
against a reviewed reference sample, by occlusion state, before a card publishes per-lane flows.

Liu, Engström and Markkula estimate brake onset as the knee of a two-piece linear fit to
longitudinal acceleration, and match a manual annotation within 0.5 s in 173 of 190 conflicts.
The behaviour plan's `braking_onset` (Section 8.2) is a threshold rule instead: the first time
deceleration exceeds a threshold for a minimum duration. On a ramp of constant jerk that rule
lags the knee by the threshold over the jerk, so gentle responses read as later than hard ones
by construction, and `response_latency` (Section 8.12) inherits the lag. Q2 asks whether the
plan should say so and name the knee as the onset `response_latency` measures to. Fitting the
knee on LiDAR tracks needs care that the paper does not: differencing positions twice at 10 Hz
gives acceleration noise of order 10 m/s², so the fit belongs on positions or on a filtered
state with a known covariance.

The rest of the week is older work found again and a resolved date: the ERSO _Speed and speeding_
thematic report carried from November 16 is dated November 2025 on its cover by search summary,
not 2026.

## Genuinely new this week

| Source                                                                                                                                         | First listed | Status       |
| ---------------------------------------------------------------------------------------------------------------------------------------------- | ------------ | ------------ |
| He, Yu and Wong, _GateTrace: Where Do Crowd Crossings Go Missing Under Occlusion?_, arXiv 2611.26388                                           | November 19  | Read         |
| Chen and Kuo, _Fixing the Twin, Not the Detector: Sim-to-Real Roadside LiDAR Detection from Digital-Twin Data_, arXiv 2611.26035               | November 20  | Read in part |
| Kumar and Soni, _UrbanTwin Sim2Real LiDAR Challenge 2026: Site-Calibrated Synthetic Data and Multi-Detector Fusion_, arXiv 2611.20431          | November 17  | Read in part |
| Deng et al., third-place UrbanTwin V2X-Real solution, arXiv 2611.18778                                                                         | November 17  | Read in part |
| Degenstein et al., _A Lightweight Method for Validating Simulated LiDAR Configurations Based on Point Density over Distance_, arXiv 2611.27379 | November 23  | Read in part |
| Gadzhiev and Morochko, _Learned versus Kinematic Trajectory Prediction under Reduced Observation Frequency_, arXiv 2611.23198                  | November 18  | Read in part |
| Vu, Quynh and Son, _Towards Deep Learning-Based Linear Assignment Problem Solvers for Multi-Object Tracking_, arXiv 2611.24107                 | November 19  | Read in part |
| Wang et al., _VTT: Reading Vehicle Size and Powertrain Type from Motion Trajectory_, arXiv 2611.22109                                          | November 18  | Read in part |
| Hahn, Shan and Perez, _Signs of Wear: Evaluating Intelligent Speed Assistance Under Synthetic Adverse Conditions_, arXiv 2611.20382            | November 17  | Read in part |

Chen and Kuo placed first on the LUMPI track of the UrbanTwin challenge. Read: abstract,
Sections I, VI and VII and the take-aways. Two findings touch the project. Their site priors use
the fixed pose: "false positives on poles, bushes and sign posts recur at the same place in
almost every frame, while road users move", and on LUMPI validation 60% of Person false positives
sat on 0.5 m cells holding a Person prediction in at least half of all frames, against 19% of
true detections. Down-weighting such cells gave +0.041 Person AP on test. That is the premise of
the project's L3 background model, which learns per-cell range statistics from unlabelled frames
and passes only departures from them to clustering; it already holds and needs no change. Their
second finding is a validation lesson: corrections of systematic sim-to-real differences "kept a
quarter to three quarters of their validation gain" on test, and choices fitted to a few
validation objects did not transfer. That is the reason the project's experiments write each
rule before their replays; it adds no method. Their remark that matching per-cell density
reaches the real-versus-real ceiling with no object modelled at all (0.947 against 0.949)
says pooled realism scores measure occupancy, the same caution the November 9 briefing drew from
the two Upganlawar reports.

Kumar and Soni (combined score 0.8947 on V2X-Real) report that per-scan ground levelling "more
than doubled the real-scan mAP"; Deng et al. (0.6512, with 0.6 × 0.5583 + 0.4 × 0.7905 = 0.6512
reproducing) document an implementation. Neither reports a tracking or extent error against an
independent reference. Degenstein et al. (accepted at IEEE ITSC 2026) score a simulated LiDAR by
point density on annotated objects in range bins, a bounded similarity built from the mean
absolute percentage error; datasheet-matched CARLA settings deviate most in the near field, and a
configuration with the sensor's non-uniform beam elevations scores best (0.559 to 0.709). The
project's synthetic mode feeds the visualiser and tests, not a detector, so the method has no
place yet.

Gadzhiev and Morochko downsample Argoverse 2 histories from 10 Hz to 5, 2 and 1 Hz with hypotheses
fixed before training; a learned predictor's displacement error grows 3.1%, 10.1% and 20.3%, and
for pedestrians constant velocity beats it. The scene export publishes tracks at 5 Hz, but the
project predicts nothing from them, so this is recorded only. Vu, Quynh and Son replace the
Hungarian assignment with neural solvers that reach "tracking accuracy close to the Hungarian
reference"; the L5 tracker solves the assignment exactly, so an approximation offers nothing.
VTT reads vehicle size classes from CAN-bus signals of instrumented cars, which a roadside sensor
does not have. Signs of Wear tests camera speed-limit sign reading, which Tenet 1 excludes; the
posted speed limits plan takes limits from records, not signs.

## Newly surfaced older work

| Source                                                                                                                      | First listed  | Why it surfaced this week                                    |
| --------------------------------------------------------------------------------------------------------------------------- | ------------- | ------------------------------------------------------------ |
| Liu, Engström and Markkula, _Automated Brake Onset Detection in Naturalistic Driving Data_, arXiv 2507.17943                | July 2025     | New on the Waymo research page; journal version, TRIP 2026   |
| Wang and Zhou, _Driver Takeovers and Longitudinal Responses Analysis from Naturalistic Driving_ (ADAS-TO), arXiv 2611.08798 | November 5    | Brake-onset query; cs.HC, outside the listings filtered      |
| ERSO thematic report _Speed and speeding_, carried from November 16                                                         | November 2025 | SWOV and ITF query; the cover date is now reported           |
| Young novice driver speed and headway at signalised intersections, New South Wales naturalistic study, _IATSS Research_     | 2026          | Following distance query                                     |
| NHTSA information collection on Intelligent Speed Assistance testing, Federal Register 2026-21054                           | October 2026  | Federal research query                                       |
| VRUD drone dataset inventory page; FLUID (_Scientific Data_, March 30, 2026)                                                | 2026          | Trajectory dataset query                                     |
| Dong et al. occlusion trajectory correction; Kim et al. wrong-way detection; PRISA; `Salah2026`                             | 2026          | Roadside LiDAR and PET queries; all held or recorded earlier |

ADAS-TO is 6,206 reviewed takeovers from Level 2 vehicles with pedal inputs, outside the window
by five days and in a category the routine does not list. Read: abstract and Section 1. Its
headline is that 78.5% of 2,031 still-closing events had a lower minimum TTC in the five seconds
after disengagement than before; it measures the ego vehicle's own controls, which a roadside
sensor cannot see, and it is recorded only.

The ERSO report's cover reads "Version November 2025" by search summary (**Confirm**;
road-safety.transport.ec.europa.eu does not resolve from this session). The 2026 in its filename
is a download name, so the November 16 briefing's "edition date not established" is answered as
far as a summary can answer it, and nothing the posted speed limits or attitudes plans cite
changes. Its figures of 10% to 15% of all crashes and 30% of fatal crashes "the direct result of
speeding or inappropriate speed" are attributions from crash coding, not observations of
passages, and are not adopted.

The New South Wales study, by search summary only (**Confirm**; ScienceDirect was not tried
again after refusing in earlier weeks), uses a 1.5 s headway as its following threshold and
reports 370 of 901 trajectories under it, 41.1%, with a mean approach headway of 1.64 s. It is a
naturalistic study of novice drivers' own vehicles, so its share is of instrumented drivers'
approaches, not of passages at a site, and the two are not set side by side. The behaviour plan
already reports headway shares under bands it cites; a band from a novice cohort is not one of
them. The Federal Register notice announces simulator and closed-track ISA testing and says NHTSA
treats such results as "complementary to rather than a substitute for" naturalistic data
(**Confirm**). VRUD's page says it "does not imply that a new raw-data release has already been
uploaded" (**Confirm**).

Valgo's news page still carries only its June 19 announcement; the IIHS news index's newest item
is still the October 27 Utah release; the AAA Foundation research index returned 404 and its
newsroom search found nothing after the September American Driving Survey brief.

## He, Yu and Wong: GateTrace (arXiv 2611.26388, November 19, 2026)

1. **Measures.** Read in full from the arXiv PDF, main text (University of Florida; a preprint,
   venue not stated; supplements referenced but not in the main PDF). The population is
   pedestrians in four MOT20 sequences of crowded scenes, recorded by fixed cameras, with
   annotated full-body boxes, identities kept across frames and a visibility value per box. A
   reference crossing is derived, not observed: each sequence gets six virtual lines, each line
   two regions of depth equal to the predicted body height there, and a transition counts when an
   annotated bottom-centre point "reaches at least 0.2 times its annotated box height into the
   destination region" within 50 frames. Crossing visibility is the mean annotated visibility
   within ±25 frames (±1 s), binned at 0.25, 0.5 and 0.75. A system report matches a reference
   when "person and direction agree, their times differ by at most 1 s, and their points are no
   farther apart" than one body height. Recall is matched over reference per visibility bin;
   precision is matched over all final reports. Hypotheses for the two confirmation sequences
   were "fixed in advance". The motivation was a stadium study where a tracker matched 103 of 129
   clearly visible tunnel crossings but 40 of 138 mostly obscured ones.
2. **The maths.** The headline ranges reproduce from Tables 1 and 2.
   - Development low visibility: ByteTrack 83/180 = 46.1%, OC-SORT 70/180 = 38.9%, BoT-SORT
     84/180 = 46.7%, the stated 38.9% to 46.7%. High: 206/210 = 98.1% and 208/210 = 99.0%.
   - Confirmation combined: low ByteTrack (56 + 84)/(136 + 291) = 140/427 = 32.8%, OC-SORT 79/427
     = 18.5%, BoT-SORT 136/427 = 31.9%, composite 184/427 = 43.1%; high from 1,128/1,217 = 92.7%
     to the composite's 1,176/1,217 = 96.6%. All as stated.
   - Misses: 180 − 83 = 97, 180 − 70 = 110 and 180 − 84 = 96 on development, and 287, 348 and 291
     on confirmation, match Table 3's totals. The "47% to 79%" with no linked output or no possible
     crossing is BoT-SORT development (18 + 27)/96 = 46.9% and OC-SORT confirmation
     (165 + 111)/348 = 79.3%. The depth check's "11% to 27%" is 11/97 and 25/92.
   - Precision: ByteTrack 580/644 = 90.1%; BASE 629/659 = 95.4% to 632/659 = 95.9%.

   Sampling does not explain the gap. Wilson 95% intervals for ByteTrack on development are
   0.390 to 0.534 at low visibility and 0.952 to 0.993 at high; on confirmation 0.285 to 0.374
   against 0.938 to 0.962. Crossings in one crowded scene are not independent, as the authors
   say, so the true intervals are wider; a design effect of four would roughly double each
   half-width and still leave them far apart. The human audit is small: ByteTrack missed 8 of
   18 low-visibility crossings both annotators supported (recall 0.56, Wilson 0.34 to 0.75) and
   none of 18 at high visibility (lower bound 0.82). The dominant uncertainty is the reference:
   full-body boxes include hidden body parts, so a reference crossing "can occur while the
   person's feet are hidden", and the authors keep this distinct from physical occurrence. A
   second is BASE's undocumented coordinate origin, reported as two readings.

   The filter result is the one that transfers. Removing every check after possible crossings
   form lifts BASE's low-visibility matches only from 114 to 115 up to 127 to 128 of 180, while
   precision falls from about 96% to 77%; for ByteTrack 83 to 95 and 90.1% to 65.9%. Most misses
   are lost before the filters, in detection and association, so filters can trade precision for
   recall but cannot recover what tracking never produced.

3. **What the sensor can and cannot observe.** The paper's evidence is camera video of people
   with identities, which Tenet 1 excludes; faces in its figures are blurred. The method needs
   none of that: a reference list of crossings derived by fixed rules from reviewed trajectories,
   a matching rule by direction, time and position, and a per-miss record of where the crossing
   was lost. A fixed LiDAR's reviewed tracks can supply the reference, and the project's
   labelling workflow already produces reviewed tracks. What a LiDAR cannot supply is the
   paper's visibility value from a human annotation of a camera frame; the nearest observable is
   geometric, such as whether the line of sight to a passage crosses another tracked object, or
   the point count on the object against its range. Crossings are passages of vehicles, counted
   per period; nothing is about a person.
4. **Where it lands.** Checked by grep against the demand card plan, its evidence report, the
   behaviour plan, the state estimation plan and the velocity-coherent foreground plan.
   - Crossings at counting lines with split-track duplicates under 0.8 s dropped and the count
     dropped recorded: already holds, demand card plan Item 2, step 1. That rule guards
     precision; nothing in the plan measures recall.
   - Missed crossings behind other vehicles: named, not measured. The evidence report says
     "Inner-lane vehicles stand between the sensor and the kerb lane and can hide kerb-lane
     vehicles" and that crossings "come from published tracks, not annotated references"; the
     plan's risk table mitigates with "Report per lane; flag lanes the sensor sees through
     another". GateTrace shows the size such a gap can reach in crowds and that a flag is not a
     number. Q1.
   - Fragmentation as a metric: planned elsewhere, as a track-level rate (velocity-coherent
     foreground plan; `Fragmentation` not implemented in the ground-truth evaluator, per the
     September parameter campaign). A crossing-level recall is a different quantity: it counts
     passages lost at the line, which is what a card publishes.
   - Matching by direction, time within 1 s and position within one object length: a rule for an
     evaluation, not a metric. Not adopted until Q1 is answered.
   - Wording: the paper's text is neutral and passes the wording guard's stems.
5. **Verdicts.**

   | Claim or recommendation                                                              | Verdict                                                                                                                     |
   | ------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------- |
   | Evaluate crossing recall by visibility, not only total counts or frame metrics       | New; logged as Q1 for the demand card plan. Not adopted: the plan is parked and no reviewed reference crossings exist yet.  |
   | Derive references from annotated trajectories under fixed rules, then match reports  | Logged with Q1 as the method, if Q1 is answered yes.                                                                        |
   | Record for each miss the stage where it was lost                                     | Logged with Q1. Already holds in spirit for suppressions (behaviour plan Section 7.2); crossings have no equivalent record. |
   | Relaxing later filters recovers few misses and lowers precision                      | Already holds as practice: Item 2 drops duplicates and records the count; nothing proposes relaxing it.                     |
   | Frame metrics and crossing recall rank trackers the same way in 53 of 56 comparisons | Not adopted: a camera tracker result, and the project ranks tracker changes on its own gates.                               |

6. **Rights.** The arXiv listing carries the arXiv non-exclusive licence to distribute 1.0, which
   grants others nothing beyond reading. The briefing paraphrases, quotes short definitions and
   re-derives figures from the tables; no table or figure is reproduced. The authors say "We will
   release the code and per-crossing records"; neither is public yet. MOT20 is used under its own
   terms, which this project does not need.
7. **What a velocity.report survey could give back.** A recall figure for counting-line
   crossings, by lane, at the junction the evidence report already measured: review the
   Columbus and Broadway tracks for a fixed window, derive reference crossings by a rule fixed
   before scoring, match the exported crossings to them, and report recall per lane with the
   kerb lane's passages split by whether an inner-lane object stood on the line of sight. If kerb
   recall is lower, the kerb-lane headways in the card are long for a reason the card can state.
   It needs no new sensing, and it is a statement about a counting method, not about anyone.

## Liu, Engström and Markkula: Automated brake onset detection (Waymo; arXiv 2507.17943, July 2025; _Transportation Research Interdisciplinary Perspectives_, 2026)

1. **Measures.** Read in full from the arXiv PDF (Waymo and the University of Leeds). The
   population is 219 conflicts involving a Waymo rider-only vehicle, "randomly selected from all
   the trajectory conflicts observed in Waymo logs that are either a crash or a near-crash"
   (7 crashes, 212 near-crashes; 201 passenger-car responders), on dense urban surface streets,
   and "not intended to be representative of the real-world distributions". The responder is
   either the Waymo vehicle or another road user observed by its perception. The quantity is the
   brake onset, the knee tB of a two-piece linear model of longitudinal acceleration: a constant
   a0, then a ramp of jerk jB from tB. The fit window runs from one second before a manually
   annotated stimulus onset T1 to the minimum acceleration within four seconds after it (or 0.2 s
   before a crash), and a grid search picks the parameters with the highest R². The reference is
   a manual annotation of the same time series, with event video for context. The journal DOI
   given on the Waymo page is 10.1016/j.trip.2026.102301 (**Confirm**; the journal page was not
   fetched).
2. **The maths.** The counts reproduce: 219 events, 3 removed for incomplete series, and missing
   values in the rest leave 190. 91.1% within 0.5 s is 173 of 190 (91.05%), and 84.2% within
   0.3 s is 160 of 190 (84.21%). That leaves 17 beyond 0.5 s, as stated. The paper accounts for
   10 of them as complex braking, one as braking outside the window and five as noisy signals:
   16, one short of 17, and the text does not say where the seventeenth falls. The largest
   deviation is −4.4 s at R² 0.56; the distribution leans early.

   The comparison that matters for the project is with a threshold rule. Write the ramp as
   a(t) = a0 + jB (t − tB) for t ≥ tB, with jB < 0. A rule "first time a < −a_th" fires at
   t = tB + (a0 + a_th) / |jB|. From a constant speed (a0 = 0), with a threshold of 1 m/s², a hard
   response at jB = −10 m/s³ is marked 0.1 s after its knee and a gentle one at −2 m/s³ 0.5 s
   after. A sustain condition does not change the instant reported. A latency measured to a
   threshold onset therefore grows as the response gets gentler, by an amount set by the
   threshold, and that bias is in the direction of making mild responses look slow. A knee
   estimate has no such term. Its own error is the fit's, which the paper bounds empirically.

   The paper's fit runs on "unfiltered acceleration data" with jerk by finite difference over two
   samples. On a LiDAR track that input does not exist with useful noise. The behaviour plan's
   Section 8.11 already derives speed noise of 0.2 to 0.35 m/s at 5 Hz from a 0.05 m position
   error. A second difference of positions has standard deviation σx √6 / Δt²: 3.1 m/s² at 5 Hz
   and 12 m/s² at 10 Hz for σx = 0.05 m, larger than most braking. The same knee can be fitted
   without differencing: speed under the model is v0 + a0 (t − tB) before tB and
   v0 + a0 (t − tB) + jB (t − tB)² / 2 after it, and position is its integral, so the fit can run
   on positions or speeds with their known errors, or on a filtered acceleration whose covariance
   the tracker reports. This is a proposal for when the metric is built, not a test of it.

   The dominant uncertainty is the stimulus anchor. The window opens at T1 − 1 s with T1 annotated
   by hand, and the authors say alternatives "will be addressed in the Discussion". The plan's
   `response_latency` anchors on the `candidate_manoeuvre`, which the pipeline would detect, not
   annotate, so the window would move with that detection.

3. **What the sensor can and cannot observe.** The method needs only a longitudinal time series
   of the responder, which is what a fixed LiDAR track gives for every vehicle in view; it needs
   no pedal signal, brake light or video, and the paper's use of video for context in annotation
   is not part of the algorithm. A roadside sensor sees fewer conflicts than a fleet and sees each
   from outside; it cannot see the stimulus a driver saw. It can mark the onset of a deceleration
   and the actor manoeuvre before it, which is all Section 8.12 records, "without implying fault".
4. **Where it lands.** Checked by grep against the behaviour plan, the metrics registry,
   `internal/lidar/l8behaviour/` and `references.bib`.
   - `braking_onset`: defined in Section 8.2 as "First time `accel_long < −a_thresh` sustained
     for ≥ `t_min`", experimental, `no_established_threshold`, not implemented. The threshold
     lag in item 2 applies to it. Q2.
   - `response_latency`: Section 8.12, "Time from candidate manoeuvre to response onset", with
     response onset undefined. Q2.
   - `distance_at_braking_onset`, `time_to_conflict_at_onset` (Section 8.10) and
     `braking_onset_distance` (Section 8.11): inherit whichever onset Section 8.2 defines. Q2.
   - Markkula et al. 2016, the three-piece model this paper simplifies: not in `references.bib`.
     Not added; Q2 can be answered without it.
   - Nothing in `internal/lidar/l8behaviour/` computes an onset today, so no code is affected.
5. **Verdicts.**

   | Claim or recommendation                                               | Verdict                                                                                                          |
   | --------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
   | Brake onset as the knee of a piecewise linear acceleration model      | New; logged as Q2 for Sections 8.2 and 8.12. Not adopted: the metrics are unbuilt and the choice is the owner's. |
   | R² of the fit as a confidence measure                                 | Logged with Q2; the plan's confidence fields would carry it.                                                     |
   | Fit on unfiltered acceleration with two-sample finite-difference jerk | Not adopted: on LiDAR positions the noise exceeds the signal. Fit on positions or a filtered state instead.      |
   | Window anchored on a manually annotated stimulus onset                | Not adopted: the project anchors on a detected candidate manoeuvre; the window rule would need its own test.     |
   | 91.1% within 0.5 s of manual annotation                               | Recorded as the paper's result on its own sample; not a threshold or benchmark for the project.                  |

6. **Rights.** The arXiv version is under CC BY-NC-ND 4.0 ("Attribution-NonCommercial-
   NoDerivatives"). The briefing quotes short passages with credit and re-derives figures; it
   reproduces no figure or table, and the project's use is non-commercial documentation. The
   algorithm is described in the paper, not released as code ("implemented in Google Colaboratory
   using Python").
7. **What a velocity.report survey could give back.** A property of two onset rules on observed
   decelerations: on any site's tracks, for passages with a sustained deceleration, compute the
   threshold onset and the knee onset fitted on positions, and report the distribution of their
   difference against the fitted jerk. If the difference falls as |jB| rises, at about a_th / |jB|,
   the bias in item 2 holds on real tracks. It is a note about two definitions, not a finding
   about anyone, and it needs no new sensing.

## What changes

In this change:

- [Behaviour analytics plan](../../../plans/lidar-behaviour-analytics-plan.md): a November 23
  entry in the weekly literature review list naming this briefing, Q1 and Q2.
- One devlog fragment.

Nothing in code, `references.bib`, the metrics registry, benchmark kinds, Section 5.1 or
definitions. The demand card plan is parked under D-32 and is not edited; Q1 is addressed to it
here and in the behaviour plan's entry. Neither paper is added to `references.bib`: the plans
cite them for nothing they hold.

## Open questions

### Q1: should counting-line crossings state their recall before a card publishes flows?

The demand card plan's Item 2 writes crossings per line and lane and drops split-track duplicates
under 0.8 s, recording the count dropped. That is a precision guard. GateTrace shows, for camera
trackers in crowds, that the larger loss is crossings never reported, that most are lost before
any counting filter, and that the loss grows steeply with occlusion. The evidence report already
names kerb-lane vehicles hidden behind inner-lane vehicles as unmeasured. Should the plan add a
step: before a card publishes per-lane flows or headways, score the exported crossings against a
reviewed reference sample under a rule fixed in advance, and state recall per lane, with the
kerb lane split by whether its line of sight crossed another object? The case for is that a card
is the project's most reusable output and a flow with unknown recall is read as a count. The case
against is that the plan is parked, a reviewed reference costs annotation time, and the one site
measured so far matched demand within its interval. It would add a step and a stated figure,
never a threshold or a metric.

### Q2: should `braking_onset` be the knee of a fit rather than a threshold crossing?

Section 8.2 defines `braking_onset` as the first time deceleration exceeds `a_thresh` for at
least `t_min`, and Section 8.12's `response_latency` runs to an undefined "response onset". On a
ramp of constant jerk a threshold rule fires (a0 + a_th) / |jB| after the deceleration begins, so
gentle responses are marked later than hard ones by construction. Should Section 8.2 define the
onset as the knee of a two-piece model fitted on positions or filtered state, with the fit's R²
as its confidence, and Section 8.12 say that `response_latency` runs to that onset? The case for
is that a latency is meant to measure time, not severity, and the dependent metrics in Sections
8.10 and 8.11 inherit the choice. The case against is that a threshold is simpler to explain and to
test, the knee needs a window anchor that the pipeline would have to detect, and none of these
metrics is built. It would change a definition in an experimental row, never a threshold.

The November 16 briefing's Q1, the November 9 briefing's Q1 and Q2, the November 2 briefing's Q1,
the October 26 briefing's Q1 and Q2 and the October 19 briefing's Q1 remain open and are not
repeated.

## Sources checked

| Source                                                                                   | Status                                                                     |
| ---------------------------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| He, Yu and Wong, arXiv 2611.26388                                                        | Read (arXiv PDF, main text; supplements not in the PDF)                    |
| Liu, Engström and Markkula, arXiv 2507.17943                                             | Read (arXiv PDF)                                                           |
| Chen and Kuo, arXiv 2611.26035                                                           | Read in part (arXiv PDF, Sections I, VI and VII and take-aways)            |
| Wang and Zhou, arXiv 2611.08798                                                          | Read in part (arXiv PDF, abstract and Section 1)                           |
| arXiv 2611.20431, 2611.18778, 2611.27379, 2611.23198, 2611.24107, 2611.22109, 2611.20382 | Read in part (abstracts)                                                   |
| arXiv past-week listings, cs.RO, cs.CV, eess.SP                                          | Read (1,688 titles filtered)                                               |
| Waymo safety research page                                                               | Read (60 entries; one new since November 16)                               |
| Valgo news page                                                                          | Read (one post, June 19, 2026)                                             |
| IIHS news index                                                                          | Read (ten most recent items, to October 27)                                |
| AAA Foundation research index                                                            | **Confirm** (404 from WebFetch; newsroom by search summary)                |
| Journal version of Liu et al., DOI 10.1016/j.trip.2026.102301                            | **Confirm** (DOI from the Waymo page by search summary; not fetched)       |
| ERSO _Speed and speeding_ thematic report                                                | **Confirm** (host does not resolve from this session; search summary only) |
| New South Wales novice driver headway study, _IATSS Research_                            | **Confirm** (search summary only)                                          |
| Federal Register 2026-21054, NHTSA ISA testing                                           | **Confirm** (search summary only)                                          |
| VRUD inventory page; FLUID                                                               | **Confirm** (search summary only)                                          |
| Dong et al. (`s26123755`), Kim et al. wrong-way, PRISA, `Salah2026`                      | Superseded (held or recorded in earlier briefings)                         |
| UrbanTwin dataset (`Shahbaz2026`), Upganlawar reports                                    | Superseded (held; read in the November 9 briefing)                         |

## Provenance

Searched November 23, 2026, extended mode: AAA Foundation for Traffic Safety new research report
November 2026; Waymo safety research paper crash rate human benchmark November 2026; IIHS study
speeding November 2026; roadside LiDAR tracking 2026 vehicle trajectory infrastructure; NHTSA
FHWA FMCSA speeding following distance research report November 2026; pedestrian impact speed
fatality risk new study 2026; following distance tailgating headway study November 2026; 85th
percentile speed limit setting research NCHRP November 2026; post-encroachment time threshold
surrogate safety LiDAR 2026 preprint; speeding aggressive driving survey results released
November 2026; crash involvement rate per vehicle mile human baseline automated driving November
2026 sober; SWOV ITF OECD road safety speed report November 2026; the title of Liu, Engström and
Markkula as written; stop sign compliance observational study LiDAR November 2026; LiDAR headway
measurement following distance enforcement 2026 FMCSA; new vehicle trajectory dataset released
November 2026 drone LiDAR intersection speed change crash model. As in earlier weeks, the
`preprint` variants of the topics were folded into the arXiv listing route rather than run as
separate web queries, and the speed-change crash model topic was folded into the dataset query.
One search result pointed back at this repository's own pull request; it was not used as a
source.

arXiv: the past-week listings for cs.RO, cs.CV and eess.SP were fetched from arxiv.org with
`curl` on November 23 and filtered by title for LiDAR, roadside, infrastructure, headway,
car-following, crash, surrogate, V2X, intersection, traffic, tracking, pedestrian, occlusion,
collision, conflict, speed, trajectory and point cloud terms. Metadata and licences came from the
abstract pages. The four PDFs were read with `curl` and `pdftotext -layout`; the arithmetic in
the GateTrace section is from its Sections 3 to 5 and Tables 1 to 3, and in the brake-onset
section from its Methods and Results. `WebFetch` read the Waymo research page, the Valgo news
page and the IIHS news index; the AAA Foundation research index returned 404, and
road-safety.transport.ec.europa.eu did not resolve. The PDFs are not committed.

## To fetch

- ERSO _Speed and speeding_,
  https://road-safety.transport.ec.europa.eu/document/download/9826c063-bc55-423e-84a3-24200dca3547_en?filename=ERSO-TR-speed_2026.pdf:
  the cover's version date, to close the question carried from November 16.
- GateTrace supplements and per-crossing records, https://arxiv.org/abs/2611.26388: the matching
  tolerance sweep (Supplement B.3) and the human-review protocol (Supplement C), only if Q1 is
  answered yes.
- Liu, Engström and Markkula, journal version, https://doi.org/10.1016/j.trip.2026.102301: whether
  the journal text accounts for the seventeenth large deviation and changes the window rule.
- Markkula et al. 2016, the three-piece braking model the paper simplifies, only if Q2 is
  answered yes.
- New South Wales novice driver headway study,
  https://www.sciencedirect.com/science/article/pii/S038611122600049X: its headway definition
  and site type, only if the behaviour plan considers a novice cohort band.

Carried from November 16 and still unread: the wrong-way detection paper, the rectangle edge
matching estimator, the Bench-RNR tables and the City of Gaithersburg release; and from earlier
weeks, the items listed in those briefings' To fetch sections.
