# Decision review: maths, October 2026

This packet collects the mathematical questions the repository still leaves to its owner, as of
the October 22, 2026 checkout, and prepares each for a short discussion. The scope is the maths
category of the decision-review procedure: `data/maths/`, the proposals and gap analysis under
it, the maths rows of the backlog, and the maths decisions that research briefings and experiment
reports have routed to the owner. Project, schema and research questions appear only where a
maths question depends on them. Seven questions are on the agenda; the inventory below records
what was found and why the rest is not.

- **Packet:** DR-MATHS-2026-10
- **Revision:** 2
- **Phase:** voice (held in text, October 22, 2026; all seven questions confirmed)
- **Branch:** `claude/peaceful-davinci-w8iuw8`
- **Related:** [MATHS.md](../../../data/maths/MATHS.md),
  [pipeline review](../../../data/maths/pipeline-review-open-questions.md),
  [gap analysis](../../../data/maths/paper-implementation-gap-analysis.md),
  [decision register](../../DECISIONS.md), [experiment review, October 18](../../lidar/operations/experiment-review-2026-10-18.md);
  checkpoint PR: none yet

## Voice brief

Maths only. Seven questions are prepared; none is a schema or project question, though Q-01 and
Q-02 feed the following report and Q-03 feeds the radar report. Nothing on the agenda blocks a
release this week. Two questions were routed here explicitly: the encounter-weighted following
view (B10, by D-28) and the following validity limits (the October 19 briefing's Q2, which the
owner explored and left undecided). The remaining five are decisions the gap analysis, the
percentile plan, the near-edge campaign and the experiment queue have each left to the owner
without naming a forum.

- Time budget: 20 minutes; selected question IDs in dependency order: Q-01, Q-02, Q-03, Q-04,
  Q-05, Q-06, Q-07.
- Carry forward: the pipeline review's eleven March questions (answered or superseded, see the
  inventory), the OBB and sign-anchor proposals' open sections (decided or deferred), and the
  beam-wise review's evidence questions (queued as experiments). No source was unavailable; one
  open PR exists and touches none of these files.

### Q-01: Following validity limits: a speed floor and a time ceiling

The following report cannot publish a `thw_below_X_rate` until valid following has a floor and a
ceiling, because the ceiling sets every rate. The October 19 briefing explored four options and
the owner said "a floor near 2 mph and a ceiling near 6 s, perhaps lower" without deciding.
Option A: one floor at 2 mph, a 6 s net ceiling, hysteresis. Option B: the same with a 4 s
ceiling converted to net, matching the roadside convention so rates compare with Michael and
Wasielewski. Option C: two floors (a measurement floor near 0.4 m/s and an opportunity floor at
2 mph), the time gap only above 10 mph and a spatial gap below it, with the 4 or 6 s ceiling
chosen on kirk0's reviewed pairs inside l8fol0004. Option D: report at 3, 4 and 6 s as a
sensitivity, which is not a definition. Recommendation: C with D's sensitivity in the report,
medium confidence; it keeps queue behaviour as a spatial measure and lets the ceiling be chosen
on reviewed pairs rather than by taste. Question: which option frames the following record?
About 3 minutes. No prerequisites. Card revision 1.

### Q-02: Encounter-weighted following view (B10)

D-28 deferred B10 until B1 measured whether both parties of an interaction are observed; B1 is
measured (October 20) and D-28 sends the call here. The time-weighted histogram gives each valid
second one vote; an encounter-weighted view gives each encounter one vote of its minimum or a low
quantile, answers "how many encounters reached below a band", and carries no interval. B1 found
following at almost every site, but half the track pairs last under half a second and the report
recommends counting by episode, not pair, which needs Q-01's hysteresis to define an episode.
Option A: publish both views, the encounter view as counts without an interval. Option B: keep
the time-weighted histogram only and record the episode count as a diagnostic in the following
record. Option C: defer again until Q-01's episode definition is pinned and l8fol0004 reports.
Recommendation: C, high confidence, because an encounter is undefined until Q-01 is settled and a
view published on track pairs would count one episode several times. Question: publish, record
as a diagnostic, or defer with Q-01 as the trigger? About 3 minutes. Prerequisite Q-01. Card
revision 1.

### Q-03: One aggregate percentile rule

The repository computes aggregate percentiles four ways: the radar report uses nearest rank
(ceiling of p times n), the LiDAR analysis report and the replay evaluation truncate p times
(n minus 1) with no interpolation, and the per-track classifier uses the floor of p times n. The
percentile plan asks for one shared helper and the pipeline review recommended linear
interpolation between order statistics (NumPy's default). On a report-sized sample the rules
differ by one order statistic at most, which can move a published p85 or p98 by one speed bin.
Option A: linear interpolation (Hyndman and Fan type 7) everywhere. Option B: nearest rank
everywhere, which the radar report already uses and which always returns an observed speed.
Option C: leave the four as they are. Recommendation: B, medium confidence: an aggregate p85 that
is an observed vehicle speed matches the project's definition of percentiles over a population
of vehicle maxima, and it changes the public radar report least. A would be right if the report
ever publishes sub-bin precision. Question: nearest rank, interpolated, or leave it? About 3
minutes. No prerequisites. Card revision 1.

### Q-04: The near-edge cost screen: the tracker update, or the frame

The October campaign's cost screen fails the near-edge arm when its Tracker.Update p99 is over
1.5 times the baseline's. The baseline's update is about half a percent of a frame, so the same
arm passes a frame-level screen at all three timing sites by a wide margin: it moves frame p99 by
2 to 7 percent, under 58 ms against a 98 ms budget. The frame-cost record measured this and left
the choice open. Option A: keep the update screen (strict; protects the tracker's own cost).
Option B: move the screen to the frame, keep the update ratio as a regression signal. Option C:
both must pass. Recommendation: B, medium confidence; a live sensor pays per frame, and the
update ratio stays reported. The Pi has not been measured, so B should name a Pi confirmation
before any promotion. Question: update, frame, or both? About 2 minutes. No prerequisites. Card
revision 1.

### Q-05: Association cost (S3): what to test next

The gap analysis's top row says a coasting track outbids a fresh one for the same cluster because
the cost is bare Mahalanobis distance and coasting covariance is inflated on purpose. Two
remedies are built default-off: a likelihood cost (d squared plus the log determinant) and a
cascade, plus the gated tentative priority. On kirk0 the likelihood arm was the only one to halve
duplicate pairs, and it lost 8.7 percent of vehicle frames and HOTA; tentative priority met every
reviewed clause and missed the target. Option A: a new l5id spec that scores the likelihood cost
on held-out identity with the containment share as an input, as the backlog row asks. Option B:
carry tentative priority forward as the S2 remedy and park S3 until W2's containment is scored.
Option C: test retiring the occlusion inflation first, since it is the term that deepens the
discount. Recommendation: B now and A when W2 reports, medium confidence; the one measured
likelihood run traded identity for frames, and the backlog already makes containment its input.
Question: which arm gets the next spec? About 3 minutes. No prerequisites. Card revision 1.

### Q-06: Subsampling density (D6): measure the built remedy or move to voxels

When a frame exceeds the 8,000-point cap, uniform subsampling thins every neighbourhood while
MinPts stays put, so the busiest frames cluster at an effective MinPts of 7.5 to 10. A remedy
that rescales MinPts by the kept fraction is built default-off; voxel downsampling, which bounds
density instead of scaling it, is specified but not wired. Option A: measure the built rescaling
on the frames over the cap across the corpus, as one cheap spec. Option B: skip to voxel
downsampling. Option C: accept the loss on about five percent of frames. Recommendation: A, high
confidence, because it exists, costs an hour of machine time, and tells us whether D6 is worth a
new L4 stage. Question: measure the built option first? About 2 minutes. No prerequisites. Card
revision 1.

### Q-07: Which maths experiments to approve now

Nine maths specs await approval. Three are cheap and unblock others: l8fol0004 (the following
limits, Q-01's venue, 6 minutes of machine time and 12 hours of person time), ground0001 (where
the ground is per site, about an hour of machine time) and l3bg0024 (where a vehicle's lower
body goes by range, which tests the beam-wise review's absorbed-band claim with a kill criterion).
The rest wait on labels or on these. Option A: approve those three. Option B: approve all nine.
Option C: approve l8fol0004 only. Recommendation: A, high confidence. Question: which set?
About 3 minutes. Prerequisite Q-01 for l8fol0004's framing. Card revision 1.

## Question index

| ID   | Category | Source ID and link                                                                                                                                                                                                                                                                                              | State                 | Depends on | Current decision entry |
| ---- | -------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------- | ---------- | ---------------------- |
| Q-01 | maths    | [Briefing 2026-10-19, Q2](../../lidar/operations/brief/research-briefing-2026-10-19.md#open-questions); [l8fol0004](../../../data/experiments/l8fol/l8fol0004.md)                                                                                                                                               | ready for integration | none       | [E-01](#e-01-q-01)     |
| Q-02 | maths    | [Behaviour plan B10](../../plans/lidar-behaviour-analytics-plan.md#13-open-research-questions); [D-28](../../DECISIONS.md#d-28--research-briefing-decisions-october-2026)                                                                                                                                       | ready for integration | Q-01       | [E-02](#e-02-q-02)     |
| Q-03 | maths    | [Percentile plan §5, canonical aggregate percentile algorithm](../../plans/speed-percentile-aggregation-alignment-plan.md#5-decisions-that-still-need-explicit-implementation-choices); [pipeline review §5.2](../../../data/maths/pipeline-review-open-questions.md#52-speed-percentile-aggregation-alignment) | ready for integration | none       | [E-03](#e-03-q-03)     |
| Q-04 | maths    | [Near-edge frame cost](../../lidar/operations/near-edge-frame-cost-2026-10.md#interpretation); [backlog, solid-body remaining work](../../BACKLOG.md)                                                                                                                                                           | ready for integration | none       | [E-04](#e-04-q-04)     |
| Q-05 | maths    | [Gap analysis S3](../../../data/maths/paper-implementation-gap-analysis.md#computed-s3); [backlog, association cost](../../BACKLOG.md); [l5id0001](../../../data/experiments/l5id/l5id0001.md)                                                                                                                  | ready for integration | none       | [E-05](#e-05-q-05)     |
| Q-06 | maths    | [Gap analysis D6](../../../data/maths/paper-implementation-gap-analysis.md#gaps); [remediation plan, P2 D6](../../../data/maths/paper-implementation-gap-analysis.md#phase-1-tests-for-known-gaps-no-paper-downloads-needed)                                                                                    | ready for integration | none       | [E-06](#e-06-q-06)     |
| Q-07 | maths    | [Experiment queue](../../../data/experiments/DEPENDENCIES.md); specs l8fol0004, ground0001, ground0002, l3bg0022, l3bg0024, l5est0001, l5est0002, geom0002, geom0012                                                                                                                                            | ready for integration | Q-01       | [E-07](#e-07-q-07)     |

## Candidate inventory

| Source question                                                                                                                            | Disposition                               | Reason or resolution                                                                                                                                                                                                                | Agenda ID or reopening trigger                                               |
| ------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------- |
| Briefing 2026-10-19 Q2, valid-following floor and ceiling                                                                                  | discuss                                   | Owner explored on October 19 and left it open; no rate publishes until it is pinned                                                                                                                                                 | Q-01                                                                         |
| Behaviour plan B10, encounter-weighted view                                                                                                | discuss                                   | D-28's deferral lapsed when B1 was measured; D-28 routes the call to `/decision-review`                                                                                                                                             | Q-02                                                                         |
| Percentile plan §5, canonical aggregate percentile algorithm                                                                               | discuss                                   | Four rules in code; the plan names one helper as the resolution but not which rule                                                                                                                                                  | Q-03                                                                         |
| Percentile plan §5, the other five rows (names, typical speed, reliable peak, migration, `peak` to `max`)                                  | already decided or implementation pending | D-18, D-19 and D-24 settle names and semantics; the formulas for typical and reliable peak speed are plan work with no maths evidence yet                                                                                           | Reopen when a track speed summary is added to a public contract              |
| Near-edge cost screen scope (backlog, solid-body remaining work)                                                                           | discuss                                   | The frame-cost record measured both screens and left the choice to the owner                                                                                                                                                        | Q-04                                                                         |
| Gap analysis S3, association cost                                                                                                          | discuss                                   | The remedies are built; one kirk0 run measured a trade; the backlog asks for a selection on held-out identity                                                                                                                       | Q-05                                                                         |
| Gap analysis D6, subsampling density                                                                                                       | discuss                                   | The rescaling remedy is built default-off and unmeasured; voxel downsampling is specified                                                                                                                                           | Q-06                                                                         |
| Nine proposed maths specs awaiting approval                                                                                                | discuss                                   | Approval is the owner's; three are cheap and unblock the rest                                                                                                                                                                       | Q-07                                                                         |
| Pipeline review Q1 (tile planes and the vector scene), Q2 and Q9 (OSM priors, GPS as offset)                                               | already decided                           | Answered in the review; the ground work is now the `ground` experiment area (ground0001 to 0005) and P3 is scheduled under D-05                                                                                                     | D-05; the ground specs                                                       |
| Pipeline review Q3 (intensity pose anchors) and the sign-anchor proposal §15 (is the cache worth the layering cost)                        | deferred                                  | The proposal's strict base case keeps anchors in L7/L8; the back-edge needs a measured false-reset reduction of at least 50 percent, which needs the anchors built first                                                            | Reopen when a sign-anchor diagnostic exists and reports false resets         |
| Pipeline review Q4 (shared settlement core)                                                                                                | already decided                           | P4 is sequenced by D-05 (after P2); the readiness-signal design is recorded in the review                                                                                                                                           | D-05                                                                         |
| Pipeline review Q5 (CV fragmentation, future-forward work)                                                                                 | already decided, partly superseded        | The state-estimation plan's Phase 4 (G-EST-1) now owns CA and IMM: demonstrate an acceleration-specific CV failure before any production change. The review's "avoid CTRV" stands                                                   | Backlog Phase 4 row                                                          |
| Pipeline review Q6 (benchmarks before replacing DBSCAN) and Q8 (geometry-coherent gain)                                                    | already decided                           | D-04 and the held-out geometry gate G-GEO-1; the review itself retracted its quantified gain table                                                                                                                                  | D-04                                                                         |
| Pipeline review Q7 (provisional defaults) and Q11 (kirk0 overfit)                                                                          | implementation pending                    | The September campaign and l3bg0020 to 0025 moved the L3 defaults on 24 sites; the corpus exists; evidence levels live in `config/CONFIG.md`                                                                                        | Reopen if `config/CONFIG.md`'s evidence table is found stale at release prep |
| Pipeline review Q10 (edge budget)                                                                                                          | implementation pending                    | The perf gate and baselines exist; the Pi frame-time spec is perf0001                                                                                                                                                               | perf0001                                                                     |
| Pipeline review §5.1 (classification thresholds to config), §5.3 (quality-score weights sum to 1), §5.4 (per-class comparison, speed RMSE) | implementation pending                    | Backlog and plan work; no undecided mathematics                                                                                                                                                                                     | Maths coherence plan Item 2; per-frame evaluator (M5)                        |
| OBB heading review §9, items 1 to 5                                                                                                        | already decided                           | D-06 keeps the guards as maintenance, keeps 0.25 and retires the guards under W1b; the 60 to 120 degree band and per-frame dimensions go with them                                                                                  | D-06                                                                         |
| Gap analysis K1, K2 (process noise coupling, Joseph form)                                                                                  | already decided                           | Measured immaterial; built as Go options with the default unchanged                                                                                                                                                                 | Reopen if the state vector grows (CA or IMM)                                 |
| Gap analysis K10 (gate width 36)                                                                                                           | already decided                           | The analysis says tightening is not recommended because moving-track innovations are heavier-tailed than chi-squared; the campaign saw no change from 9 to 100                                                                      | Reopen with K9's anisotropic R (G-UNC-1)                                     |
| Gap analysis B1 (MAD versus sigma), B2, B7                                                                                                 | implementation pending                    | Documentation rows in the remediation plan's Phase 2; B7's window is the beam-wise E1 arm, queued as l3bg0009                                                                                                                       | l3bg0009                                                                     |
| Beam-wise comparison §10 and review §9, questions 1 to 5                                                                                   | needs evidence                            | Return mode is resolved for kirk0 (single, last return); the arrival effect, the variance tercile, the count rate and the OS0 beam count are measurements, and E1 and E2 are queued (l3bg0001 and l3bg0008 answered, l3bg0009 open) | l3bg0009; label0001                                                          |
| Facet registration review and the attribution gate                                                                                         | deferred                                  | The backlog restates the gate as a measure to predeclare once W2's floored box is scored                                                                                                                                            | W2 scored                                                                    |
| Near-edge span-search reduction, sensor geometry for the held-out case, F8                                                                 | needs evidence                            | Tuning changes needing the corpus screen and a governed split                                                                                                                                                                       | label0001                                                                    |
| Maths coherence plan (citation loop, L6 confidence equation, L8 note)                                                                      | implementation pending                    | Documentation substrate; no undecided choice                                                                                                                                                                                        | Plan checklist                                                               |

## Evidence and options

### Evidence Q-01: Following validity limits

- Card revision: 1.
- Current behaviour and decision boundary: `following_valid_time` and every `thw_below_X_rate`
  are defined in the behaviour plan but no rate is published; the following record drafts carry
  `leader`, `no_leader` and `ambiguous` and no floor, ceiling, margin or dwell. The decision is
  the definition of valid following; the values of a 4 or 6 s ceiling are chosen inside
  l8fol0004 on kirk0's reviewed pairs, not here.
- Sources: [briefing 2026-10-19, Q2 section and options](../../lidar/operations/brief/research-briefing-2026-10-19.md);
  [behaviour plan §13](../../plans/lidar-behaviour-analytics-plan.md#13-open-research-questions);
  [l8fol0004](../../../data/experiments/l8fol/l8fol0004.md), whose `decide` step pins the record
  as a D-NN; `Michael2000`, `Jiao2024`, `Chu2026` in `references.bib`.
- Evidence quality: derived. The briefing's cap table is arithmetic on a 60 m leader range and a
  4.5 m car: a 6 s ceiling binds only below about 21 mph and a 4 s ceiling below about 31 mph.
  Jiao et al. is a source result on a different population (freeway, automated leaders). No
  repository measurement of the ceiling's effect exists yet; B1 measured pair durations only.
- Technical check: with ceiling C the share below band X is F(X) over F(C) for the time-gap
  distribution F, so lowering C raises every rate with no driver changing; a rate is comparable
  only at the same floor and ceiling. Net and gross differ by L over v: about 0.4 s at 25 mph and
  1 s at 10 mph, so a 4 s gross convention is about 3.6 s net at 25 mph. A per-instant ceiling
  flickers; hysteresis (leave after C plus about 0.5 s for about 1 s) is the usual remedy. A time
  gap under C is a proxy for constrained following, not a test of it.
- Prerequisites and cycles: none. Q-02 and l8fol0004's framing depend on it.

| Option | Result and tradeoff                                                                                                                   | Effort and reversibility                                                        | Evidence and uncertainty                                                   |
| ------ | ------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| A      | Floor 2 mph, ceiling 6 s net, hysteresis. Wide; binds only in slow traffic; noisy gaps just above the floor still count               | Small (record fields and the finder); reversible until a rate is published      | Derived cap table; close to Chu's 1 m/s floor                              |
| B      | Floor 2 mph, ceiling 4 s converted to net, hysteresis. Rates compare with the roadside convention; drops some free-flow following     | Small; reversible until published                                               | `Michael2000`, Wasielewski; the conversion is arithmetic                   |
| C      | Two floors (measurement about 0.4 m/s, opportunity 2 mph), time gap above 10 mph only, spatial gap below, ceiling 4 or 6 s from kirk0 | Medium (two reporting modes, a band edge to choose); reversible until published | Keeps queue behaviour as a spatial measure, where the time gap is unstable |
| D      | Report at 3, 4 and 6 s as a sensitivity                                                                                               | Small, additive to any of A to C                                                | Not a definition; a headline still needs one value                         |

- Recommendation: C with D's sensitivity, medium confidence. It would change if kirk0's reviewed
  pairs show almost no following below 10 mph, in which case B's single convention is simpler
  and comparable.
- Decision requested: which option frames the following record that l8fol0004 pins?

### Impact Q-01

| Option | Target path and symbol                                                                                                | Trace evidence                                      | Required change                                                                       | Prerequisite | Validation               | Integration disposition |
| ------ | --------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------- | ------------------------------------------------------------------------------------- | ------------ | ------------------------ | ----------------------- |
| A/B/C  | [l8fol0004](../../../data/experiments/l8fol/l8fol0004.md) `decide` step note                                          | Its note lists the record's fields                  | Add floor, ceiling, margin and dwell (and C's band edge) to the fields the D-NN pins  | none         | `make check-experiments` | pending                 |
| A/B/C  | [behaviour plan §13 B10 row and the `following_valid_time` definition](../../plans/lidar-behaviour-analytics-plan.md) | Plan defines the metric and lists Q2 as explored    | Record the chosen framing; mark the ceiling value as l8fol0004's                      | none         | `make lint-docs`         | pending                 |
| A/B/C  | [briefing 2026-10-19 Q2 row](../../lidar/operations/brief/research-briefing-2026-10-19.md#open-questions)             | Row says explored, not decided                      | Link the decision entry                                                               | none         | link check               | pending                 |
| A/B/C  | Following record draft fields and parameter hash (`draft-following`, the physical scorer)                             | l8fol0004 `implement` step; briefing recommendation | Implementation under l8fol0004, not here: a recorded decision, implementation pending | Q-01         | l8fol0004's `score` step | deferred to l8fol0004   |
| C      | The 10 mph band edge                                                                                                  | Option C's definition                               | Chosen on kirk0's pairs with the ceiling                                              | Q-01         | l8fol0004 report         | deferred to l8fol0004   |
| D      | Following report limitations text                                                                                     | Briefing recommendation                             | Report 3, 4 and 6 s beside the headline                                               | Q-01         | report review            | pending                 |

### Evidence Q-02: Encounter-weighted following view (B10)

- Card revision: 1.
- Current behaviour and decision boundary: the scene distribution publishes a time-weighted
  histogram only. B10 asks whether a view with one vote per encounter publishes beside it. D-28
  deferred it until B1 measured; B1 is measured and D-28 names this forum. The decision is
  publish, record as a diagnostic, or defer with a trigger.
- Sources: [behaviour plan B10](../../plans/lidar-behaviour-analytics-plan.md#13-open-research-questions);
  [D-28](../../DECISIONS.md#d-28--research-briefing-decisions-october-2026);
  [B1 report](../../lidar/operations/following-opportunity-corpus-2026-10.md), answer,
  limits and recommendation 2; [October 9 briefing](../../lidar/operations/brief/fixed-sensor-research-briefing-2026-10-09.md).
- Evidence quality: observed, label-free. Over 23 sites and 100 minutes of tracks: 2,264 track
  pairs, median following 0.5 s, 29 percent lasting a second or more, median closest gap 18 m.
  The counts are an upper bound on pairs; fragmentation makes several pairs of one episode. The
  gate's class requirement holds for under a third of reviewed kirk0 vehicles.
- Technical check: the per-encounter Monte Carlo draws do not combine, so the encounter view
  has no interval; it is a count. An encounter needs a start and end rule, which is Q-01's
  hysteresis and dwell; without it the denominator is track pairs, and B1 shows that overcounts.
- Prerequisites and cycles: Q-01.

| Option | Result and tradeoff                                                                 | Effort and reversibility                                            | Evidence and uncertainty                                                    |
| ------ | ----------------------------------------------------------------------------------- | ------------------------------------------------------------------- | --------------------------------------------------------------------------- |
| A      | Publish both views; the encounter view as counts with no interval                   | Medium (registry entry, scene export, display); public once shipped | Counts by pair overcount episodes until Q-01 and one track per vehicle land |
| B      | Time-weighted histogram only; episode count as a diagnostic in the following record | Small; reversible                                                   | Loses the "how many encounters reached below a band" reading                |
| C      | Defer until Q-01 is pinned and l8fol0004 reports; reopen then                       | None now                                                            | The deferral has a concrete trigger this time                               |

- Recommendation: C, high confidence; B is the fallback if the owner wants the question closed
  now. It would change if l8fol0004's reviewed pairs show episodes are long enough that pair and
  episode counts agree.
- Decision requested: publish, diagnostic only, or defer with Q-01 and l8fol0004 as the trigger?

### Impact Q-02

| Option | Target path and symbol                                                                             | Trace evidence                          | Required change                                                       | Prerequisite | Validation       | Integration disposition |
| ------ | -------------------------------------------------------------------------------------------------- | --------------------------------------- | --------------------------------------------------------------------- | ------------ | ---------------- | ----------------------- |
| A      | Metrics registry entry and definition for the encounter view; scene export; display                | Behaviour plan B10; D-28 rename clauses | New metric id, interval marked none, export field, display            | Q-01         | registry tests   | pending                 |
| A/B    | [behaviour plan B10 row](../../plans/lidar-behaviour-analytics-plan.md#13-open-research-questions) | Row cites D-28's deferral               | Record the outcome                                                    | none         | `make lint-docs` | pending                 |
| A/B/C  | [D-28](../../DECISIONS.md#d-28--research-briefing-decisions-october-2026) and the audit row        | D-28 names this forum                   | Record the call (a sentence in D-28 or a new entry), update the audit | none         | register review  | pending                 |
| A/B/C  | [backlog row, encounter-weighted following view](../../BACKLOG.md)                                 | Row carries B10 at 0.5.2.4              | Reschedule, close, or restate with the trigger                        | none         | backlog lint     | pending                 |
| B      | Following record fields                                                                            | B1 recommendation 2                     | Add an episode count                                                  | Q-01         | l8fol0004        | deferred to l8fol0004   |

### Evidence Q-03: One aggregate percentile rule

- Card revision: 1.
- Current behaviour and decision boundary: four rules compute percentiles. The radar report path
  uses nearest rank, index ceiling(p n) minus 1, in `internal/db/db_radar.go`
  (`empiricalQuantileSorted`). The LiDAR analysis report (`internal/lidar/analysis/report.go`,
  `percentileSorted`) and the replay evaluation (`internal/lidar/replayeval/solid_body_summary.go`,
  `percentile`) truncate p (n minus 1) without interpolating. The per-track classifier
  (`internal/lidar/l6objects/classification.go`, `ComputeSpeedPercentiles`) uses floor(p n); D-18
  makes those per-track values historical, so it is out of scope except for removal. The
  decision is the rule for aggregate p50, p85 and p98; the helper's home is implementation.
- Sources: [percentile plan §5](../../plans/speed-percentile-aggregation-alignment-plan.md#5-decisions-that-still-need-explicit-implementation-choices);
  [pipeline review §5.2](../../../data/maths/pipeline-review-open-questions.md#52-speed-percentile-aggregation-alignment);
  [D-18](../../DECISIONS.md#d-18--speed-percentile-aggregation-semantics);
  [radar histogram queries](../../../internal/db/queries_histograms.md), which document nearest rank.
- Evidence quality: repository observation (read from the code at the preparation snapshot).
  No measurement of how often the rules disagree on real reports; the bound below is derived.
- Technical check: for sorted x of size n and p in (0, 1), nearest rank returns x at index
  ceiling(p n) minus 1; the truncated rule returns x at index floor(p (n minus 1)); type 7
  returns x at floor(h) plus the fraction of h times the gap to the next value, h equal to
  p (n minus 1). The three agree when p n is an integer and otherwise differ by at most one order
  statistic, so the published value can move by one speed bin on a small bin. Nearest rank and
  the truncated rule always return an observed speed; type 7 does not. The project's definition
  (percentiles over a population of vehicle maxima, not per-track observations) is independent
  of the rule. Percentiles must not be merged across time bins under any rule.
- Prerequisites and cycles: none.

| Option | Result and tradeoff                                                                                      | Effort and reversibility                                                      | Evidence and uncertainty                                              |
| ------ | -------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------- | --------------------------------------------------------------------- |
| A      | Type 7 linear interpolation everywhere; matches NumPy and most statistical software; values not observed | Medium: one helper, three call sites, report fixtures and tests move by a bin | Standard; changes the public radar report's p85 and p98 on small bins |
| B      | Nearest rank everywhere; values are observed speeds; the radar report is unchanged                       | Small: the two LiDAR helpers change by one index; tests move                  | Already documented for radar in `queries_histograms.md`               |
| C      | Leave the four as they are                                                                               | None                                                                          | Reports from different paths can disagree by one order statistic      |

- Recommendation: B, medium confidence. It would change if a report ever needs sub-bin
  precision or comparability with an external tool that interpolates.
- Decision requested: nearest rank, type 7 interpolation, or leave as is?

### Impact Q-03

| Option | Target path and symbol                                                                                                                                                                          | Trace evidence                                          | Required change                                                   | Prerequisite | Validation                                | Integration disposition |
| ------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------- | ----------------------------------------------------------------- | ------------ | ----------------------------------------- | ----------------------- |
| A/B    | `internal/db/db_radar.go` `empiricalQuantileSorted` and callers                                                                                                                                 | `grep` at the snapshot                                  | A: interpolate; B: no change beyond moving into the shared helper | none         | `go test ./internal/db ./internal/report` | pending                 |
| A/B    | `internal/lidar/analysis/report.go` `percentileSorted`                                                                                                                                          | `grep` at the snapshot                                  | Replace with the shared helper                                    | none         | `go test ./internal/lidar/analysis`       | pending                 |
| A/B    | `internal/lidar/replayeval/solid_body_summary.go` `percentile`                                                                                                                                  | `grep` at the snapshot                                  | Replace with the shared helper                                    | none         | `go test ./internal/lidar/replayeval`     | pending                 |
| A/B    | `internal/lidar/l6objects/classification.go` `ComputeSpeedPercentiles`                                                                                                                          | D-18: per-track percentiles are historical              | No change here; removal is D-18's work                            | none         | none                                      | no change               |
| A/B    | [percentile plan §5 row](../../plans/speed-percentile-aggregation-alignment-plan.md); [queries_histograms.md](../../../internal/db/queries_histograms.md); `tracking-maths.md` or a new L8 note | Plan names the row; the coherence plan wants an L8 note | Record the rule and its indexing formula                          | none         | `make lint-docs`                          | pending                 |
| A      | Report fixtures and golden tests under `internal/report`                                                                                                                                        | Report tests pin percentile values                      | Refresh expected values                                           | none         | `make test-go`                            | pending                 |
| C      | none                                                                                                                                                                                            |                                                         | Record the choice in the plan row                                 | none         | `make lint-docs`                          | pending                 |

### Evidence Q-04: The near-edge cost screen

- Card revision: 1.
- Current behaviour and decision boundary: the October campaign's screen compares Tracker.Update
  p99 alone and fails A2 when over 1.5 times B0's. The frame-cost record measured the frame and
  did not choose. The decision is the screen's scope; the 1.5 ratio and the Pi confirmation are
  parameters of it.
- Sources: [near-edge frame cost](../../lidar/operations/near-edge-frame-cost-2026-10.md);
  [update cost profile](../../lidar/operations/near-edge-update-cost-profile-2026-10.md);
  [near-edge plan](../../plans/lidar-near-edge-tracked-state-plan.md); backlog row "Solid-body
  estimate, remaining tracked-state work (S2)".
- Evidence quality: observed on one Mac, three sites, 300 s windows, four runs per arm, balanced
  order. Update p99 ratios of 9 to 21; frame p99 moved 2 to 7 percent, p95 5 to 10 percent;
  every frame p99 under 58 ms against 98 ms. Not measured on the Pi; the frame includes replay
  recording; replay back-pressure delays rather than drops frames.
- Technical check: B0's update is about 0.5 percent of a frame, so a ratio to it is a ratio of
  small numbers and says little about what a live sensor pays. A frame screen reads the budget
  the sensor has. An update screen catches regressions in the tracker alone.
- Prerequisites and cycles: none.

| Option | Result and tradeoff                                                                    | Effort and reversibility       | Evidence and uncertainty                             |
| ------ | -------------------------------------------------------------------------------------- | ------------------------------ | ---------------------------------------------------- |
| A      | Keep the update screen; A2 fails at all three sites                                    | None                           | Strict; disconnected from the frame budget           |
| B      | Frame screen (p99 under budget, ratio to B0 at 1.5), update ratio reported as a signal | Small (plan and campaign text) | A2 passes by a wide margin on the Mac; Pi unmeasured |
| C      | Both must pass                                                                         | Small                          | Equivalent to A in outcome today                     |

- Recommendation: B, with a Pi frame measurement named as a condition of any promotion, medium
  confidence. It would change if the Pi frame p99 under A2 approaches the budget.
- Decision requested: update, frame, or both?

### Impact Q-04

| Option | Target path and symbol                                                                      | Trace evidence                 | Required change                                  | Prerequisite | Validation               | Integration disposition |
| ------ | ------------------------------------------------------------------------------------------- | ------------------------------ | ------------------------------------------------ | ------------ | ------------------------ | ----------------------- |
| A/B/C  | [near-edge plan](../../plans/lidar-near-edge-tracked-state-plan.md), cost screen definition | Plan states the update screen  | Record the screen's scope and parameters         | none         | `make lint-docs`         | pending                 |
| A/B/C  | [backlog row, solid-body remaining work](../../BACKLOG.md)                                  | Row lists the choice as open   | Remove the open clause; keep the Pi confirmation | none         | backlog lint             | pending                 |
| B      | [frame cost record](../../lidar/operations/near-edge-frame-cost-2026-10.md) status line     | Record says it does not choose | Link the decision                                | none         | link check               | pending                 |
| B      | A Pi timing step (perf area) for A2's frame                                                 | Record's limitation            | New or amended perf spec                         | Q-04         | `make check-experiments` | pending                 |

### Evidence Q-05: Association cost (S3)

- Card revision: 1.
- Current behaviour and decision boundary: the shipped cost is d squared with the gate at 36 and
  `occlusion_cov_inflation` widening a coasting track's S. `LikelihoodAssociationCost`,
  `CascadedAssociation` and `TentativePriority` are Go-level options, default off, in
  `internal/lidar/l5tracks/tracking_config.go`. The decision is which arm the next identity spec
  scores, and on what evidence the inflation is retired.
- Sources: [gap analysis S3 and its computed section](../../../data/maths/paper-implementation-gap-analysis.md#computed-s3);
  [l5id0001](../../../data/experiments/l5id/l5id0001.md) and [its report](../../lidar/operations/l5-duplicate-tracks-2026-10.md);
  backlog row "Association cost (S3), before longer coasting"; `Wojke2017` in `references.bib`.
- Evidence quality: observed on kirk0 and the corpus's label-free screen. Duplicate pairs per
  hundred vehicle tracks: shipped 15.5, likelihood cost with contested rejoins 6.9 (the only arm
  to meet the target) at a loss of 8.7 percent of vehicle frames and HOTA 0.376 to 0.358;
  tentative priority 13.4, HOTA 0.385, IDF1 0.390, 117 identity switches against 124. The
  held-out split was not read.
- Technical check: the likelihood cost d squared plus ln det S charges a vague track for its
  vagueness; the gate stays on d squared. Moving tracks coast on 34 to 57 percent of frames in
  the corpus, so S3 is the common case. The one measured likelihood run traded frames for
  duplicates, consistent with the cost refusing clusters that a coasting track had been
  claiming; whether those were its own clusters or another body's is what containment (W2)
  would say.
- Prerequisites and cycles: none for the choice; option A's spec takes W2's containment share as
  an input once scored.

| Option | Result and tradeoff                                                                                             | Effort and reversibility                       | Evidence and uncertainty                                         |
| ------ | --------------------------------------------------------------------------------------------------------------- | ---------------------------------------------- | ---------------------------------------------------------------- |
| A      | New l5id spec: likelihood cost on held-out identity with containment as an input; retire inflation if it passes | Medium (spec, held-out read, 2 to 3 h machine) | One kirk0 run lost HOTA; containment not yet scored              |
| B      | Carry tentative priority as the S2 remedy; park S3 until W2 reports, then run A                                 | Small now                                      | Tentative priority met every clause; missed the duplicate target |
| C      | Test retiring `occlusion_cov_inflation` first, alone                                                            | Small spec                                     | Removes the amplifier, not the discount; affects coasting gates  |

- Recommendation: B now, A when W2 is scored, medium confidence. It would change if W2 scores
  soon enough that A can start this week.
- Decision requested: which arm gets the next spec, and does the inflation's retirement wait for it?

### Impact Q-05

| Option | Target path and symbol                                                                         | Trace evidence                          | Required change                                            | Prerequisite | Validation                                  | Integration disposition |
| ------ | ---------------------------------------------------------------------------------------------- | --------------------------------------- | ---------------------------------------------------------- | ------------ | ------------------------------------------- | ----------------------- |
| A      | New `data/experiments/l5id/l5id0011.md`                                                        | Backlog row's wording                   | Spec with arms a0, likelihood, likelihood plus containment | W2 scored    | `make check-experiments`                    | pending                 |
| A/B/C  | [backlog row, association cost (S3)](../../BACKLOG.md)                                         | Row asks for the selection              | Restate with the chosen sequence                           | none         | backlog lint                                | pending                 |
| A/B/C  | [gap analysis S3 row and Phase 3 P1](../../../data/maths/paper-implementation-gap-analysis.md) | Row names the remedies                  | Record the decision and l5id0001's measured trade          | none         | `make lint-docs`                            | pending                 |
| B      | [l5id0006](../../../data/experiments/l5id/l5id0006.md)                                         | Spec carries tentative priority forward | No change; approval is Q-07's                              | none         | none                                        | no change               |
| C      | New l5id spec on `occlusion_cov_inflation`; `config/experiments/` parameter set                | Tuning key, moves the fingerprint       | Spec, parameter set, README row                            | none         | `go test ./internal/config -run Experiment` | pending                 |

### Evidence Q-06: Subsampling density (D6)

- Card revision: 1.
- Current behaviour and decision boundary: `foreground_max_input_points` is 8,000; over it,
  `uniformSubsample` thins the frame and MinPts stays 5, so the effective threshold is MinPts
  over the kept fraction (7.5 at 12,000 points, 10 at 16,000). `ScaleMinPtsWhenSubsampled`
  (`internal/lidar/l4perception/cluster.go`, `effectiveMinPts`) is built, default off, unmeasured.
  Voxel downsampling is specified in `clustering-maths.md` §3 and not wired before DBSCAN. The
  decision is whether to measure the built remedy before anything else.
- Sources: [gap analysis D6](../../../data/maths/paper-implementation-gap-analysis.md#gaps),
  [remediation plan Phase 1 P2 and Phase 3 P3](../../../data/maths/paper-implementation-gap-analysis.md#prioritised-remediation-plan);
  [clustering maths §3](../../../data/maths/clustering-maths.md); the subsampling determinism
  note in `cluster.go` (about 5 percent of frames on a busy street exceed the cap).
- Evidence quality: derived (the threshold arithmetic) and repository observation (the code
  comment's 5 percent). No measurement of recall on frames over the cap. The September campaign's
  `foreground_min_cluster_points` sweep bounds the direction (8 on every frame took kirk1 from 10
  to 6 of 49 labelled tracks) but not the size.
- Technical check: thinning N points to M keeps each with probability f equal to M over N; a
  neighbourhood's expected count falls by f; round(MinPts f), floored at 2, restores the
  threshold in expectation but not its variance. Voxel downsampling bounds density rather than
  scaling it, so it changes clustering on every frame, not only the capped ones, and moves the
  L4 fingerprint and the perf baselines.
- Prerequisites and cycles: none.

| Option | Result and tradeoff                                                                     | Effort and reversibility                                        | Evidence and uncertainty                                  |
| ------ | --------------------------------------------------------------------------------------- | --------------------------------------------------------------- | --------------------------------------------------------- |
| A      | One spec: the built rescaling on the corpus frames over the cap, labelled score on kirk | Small (about an hour machine time); a Go option, no fingerprint | Tells whether D6 costs anything measurable                |
| B      | Wire voxel downsampling before DBSCAN                                                   | Medium; moves the L4 fingerprint and perf baselines             | Changes every frame; needs the corpus screen              |
| C      | Accept the loss on the capped frames                                                    | None                                                            | The capped frames are the busiest; recall loss unmeasured |

- Recommendation: A, high confidence. It would change if the corpus shows almost no frames over
  the cap, in which case C is free.
- Decision requested: measure the built option first?

### Impact Q-06

| Option | Target path and symbol                                                                   | Trace evidence                        | Required change                                                           | Prerequisite  | Validation                              | Integration disposition |
| ------ | ---------------------------------------------------------------------------------------- | ------------------------------------- | ------------------------------------------------------------------------- | ------------- | --------------------------------------- | ----------------------- |
| A      | New `data/experiments/l4clus/l4clus0004.md` (next free slug to be confirmed)             | Remediation plan P2 D6                | Spec: a0 against `ScaleMinPtsWhenSubsampled` via a replay experiment flag | none          | `make check-experiments`                | pending                 |
| A      | A replay experiment name wiring `ScaleMinPtsWhenSubsampled`                              | Options are Go-level, not tuning keys | Add the experiment switch if none exists                                  | none          | `go test ./internal/lidar/l4perception` | pending                 |
| A/B/C  | [gap analysis D6 row](../../../data/maths/paper-implementation-gap-analysis.md#gaps)     | Row names the remedies                | Record the decision                                                       | none          | `make lint-docs`                        | pending                 |
| B      | `internal/lidar/l4perception` voxel stage; `config/tuning.defaults.json`; perf baselines | Clustering maths §3                   | New stage, key, baselines recaptured                                      | corpus screen | `make test-perf-all`                    | deferred                |

### Evidence Q-07: Which maths experiments to approve now

- Card revision: 1.
- Current behaviour and decision boundary: `experiments next` lists nine proposed maths specs
  awaiting approval (l8fol0004, ground0001, ground0002, l3bg0022, l3bg0024, l5est0001, l5est0002,
  geom0002, geom0012) and several waiting on dependencies. Approval is the owner's. The decision
  is which to approve this week.
- Sources: [DEPENDENCIES.md](../../../data/experiments/DEPENDENCIES.md); each spec's header;
  [experiment review, October 18](../../lidar/operations/experiment-review-2026-10-18.md), which
  ranked l8fol0004 fifth and named its decision step.
- Evidence quality: the specs' own estimates. l8fol0004: 6 minutes machine, 12 hours person;
  ground0001: 1 h 9 m machine, 4 h person; l3bg0024: 2 h 21 m machine, 6 h person; ground0002:
  6 h 20 m machine; l5est0002: 10 h 44 m machine and 60 GB.
- Technical check: ground0001 is a read, not a change; l3bg0024 tests the beam-wise review's C2
  (an absorbed band of about 0.15 m as configured, 0.35 m or more with region overrides) with the
  review's kill criterion; l8fol0004 is the venue for Q-01 and the prerequisite of l8fol0005.
- Prerequisites and cycles: Q-01 frames l8fol0004's `decide` step.

| Option | Result and tradeoff                     | Effort and reversibility        | Evidence and uncertainty                |
| ------ | --------------------------------------- | ------------------------------- | --------------------------------------- |
| A      | Approve l8fol0004, ground0001, l3bg0024 | About 4 h machine, 22 h person  | Each unblocks a chain                   |
| B      | Approve all nine                        | About 33 h machine, 56 h person | Person time is the binding constraint   |
| C      | Approve l8fol0004 only                  | Minimal                         | Leaves the ground and L3 chains waiting |

- Recommendation: A, high confidence. It would change if labelling hours this week are already
  committed to label0001, in which case C.
- Decision requested: which set?

### Impact Q-07

| Option | Target path and symbol                                       | Trace evidence               | Required change          | Prerequisite       | Validation                                         | Integration disposition |
| ------ | ------------------------------------------------------------ | ---------------------------- | ------------------------ | ------------------ | -------------------------------------------------- | ----------------------- |
| A/B/C  | The chosen specs' `status` fields                            | `EXPERIMENTS.md` field table | `proposed` to `approved` | Q-01 for l8fol0004 | `make check-experiments`; `make experiments-graph` | pending                 |
| A/B/C  | [DEPENDENCIES.md](../../../data/experiments/DEPENDENCIES.md) | Generated                    | Regenerate               | none               | `make check-experiments`                           | pending                 |

## Decision log

Seven entries, recorded October 22, 2026 from a text discussion of the revision 1 cards. Each is
the owner's explicit choice; integration review has not yet run on any of them.

### E-01: Q-01

- State: ready for integration.
- Disposition: adopt.
- Choice and constraints: option C with option D's sensitivity. Valid following has two floors
  (a measurement floor near 0.4 m/s and an opportunity floor at 2 mph), the time gap is used
  only above 10 mph and a spatial gap below it, the ceiling is a net-gap ceiling of 4 or 6 s
  chosen on kirk0's reviewed pairs inside l8fol0004, with hysteresis (leave after the ceiling
  plus a margin for a minimum dwell); the report shows 3, 4 and 6 s as a sensitivity beside the
  headline. The floor, ceiling, margin, dwell and band edge go in the following record and the
  parameter hash. No rate publishes before the record is pinned.
- Rationale: keeps queue behaviour as a spatial measure where the time gap is unstable, and lets
  the ceiling be chosen on reviewed pairs rather than by convention alone.
- Confirmation: explicit selection of the option in a text decision session; the question was put with the options and the recommendation marked, and the owner chose the option named above. No ambiguity remained.
- Provenance: Claude Code session `session_01CCGg6RfzL9uoYZhqKsEqK9`, text modality, 2026-10-22T01:08:56Z.
- Reviewed card: DR-MATHS-2026-10 revision 1, evidence card Q-01 revision 1.
- Supersedes: none.
- Follow-ups: the 4 or 6 s value and the 10 mph band edge are l8fol0004's `decide` step (F-05).

### E-02: Q-02

- State: ready for integration.
- Disposition: defer.
- Choice and constraints: option C. The encounter-weighted view (B10) does not publish now. It
  reopens when E-01's following record is pinned and l8fol0004 reports; the question then is
  whether episodes under that record are long enough that an encounter view adds a reading the
  histogram lacks. Until then the time-weighted histogram is the only published view.
- Rationale: an encounter is undefined until the episode rule exists, and B1 shows a view on
  track pairs would count one episode several times.
- Confirmation: explicit selection of the option in a text decision session; the question was put with the options and the recommendation marked, and the owner chose the option named above. No ambiguity remained.
- Provenance: Claude Code session `session_01CCGg6RfzL9uoYZhqKsEqK9`, text modality, 2026-10-22T01:08:56Z.
- Reviewed card: DR-MATHS-2026-10 revision 1, evidence card Q-02 revision 1.
- Supersedes: none.
- Follow-ups: reopening trigger as stated; D-28's forward reference to this forum is satisfied by this entry (F-06).

### E-03: Q-03

- State: ready for integration.
- Disposition: adopt.
- Choice and constraints: option B. Aggregate p50, p85 and p98 use nearest rank everywhere: on
  sorted x of size n the value at index ceiling(p n) minus 1, clamped to the slice. One shared
  helper replaces the radar, analysis-report and replay-evaluation helpers; the per-track
  classifier percentiles are out of scope and go with D-18. The indexing rule is documented
  beside the helper and in the maths notes. Percentiles are never merged across time bins.
- Rationale: an aggregate percentile that is an observed vehicle speed matches the project's
  definition; the public radar report does not change.
- Confirmation: explicit selection of the option in a text decision session; the question was put with the options and the recommendation marked, and the owner chose the option named above. No ambiguity remained.
- Provenance: Claude Code session `session_01CCGg6RfzL9uoYZhqKsEqK9`, text modality, 2026-10-22T01:08:56Z.
- Reviewed card: DR-MATHS-2026-10 revision 1, evidence card Q-03 revision 1.
- Supersedes: none.
- Follow-ups: F-03.

### E-04: Q-04

- State: ready for integration.
- Disposition: adopt.
- Choice and constraints: option B. The near-edge cost screen is a frame-level screen: a
  candidate arm's frame p99 stays under the frame budget and within 1.5 times the baseline's at
  each timing site. The Tracker.Update p99 ratio is still reported as a regression signal for the
  tracker, not as a pass or fail. A Pi frame-time measurement of the arm is a condition of any
  promotion.
- Rationale: a live sensor pays per frame; the baseline update is about half a percent of one.
- Confirmation: explicit selection of the option in a text decision session; the question was put with the options and the recommendation marked, and the owner chose the option named above. No ambiguity remained.
- Provenance: Claude Code session `session_01CCGg6RfzL9uoYZhqKsEqK9`, text modality, 2026-10-22T01:08:56Z.
- Reviewed card: DR-MATHS-2026-10 revision 1, evidence card Q-04 revision 1.
- Supersedes: none.
- Follow-ups: F-04.

### E-05: Q-05

- State: ready for integration.
- Disposition: adopt (sequence).
- Choice and constraints: option B. Tentative priority is carried forward as the S2 remedy under
  its existing spec (l5id0006). S3 is parked until W2's containment share is scored; the
  likelihood-cost spec then runs on held-out identity with containment as an input.
  `occlusion_cov_inflation` is retired only after that comparison, not before.
- Rationale: the one measured likelihood run traded identity for duplicates, and the backlog
  already makes containment the input that would explain which clusters it refused.
- Confirmation: explicit selection of the option in a text decision session; the question was put with the options and the recommendation marked, and the owner chose the option named above. No ambiguity remained.
- Provenance: Claude Code session `session_01CCGg6RfzL9uoYZhqKsEqK9`, text modality, 2026-10-22T01:08:56Z.
- Reviewed card: DR-MATHS-2026-10 revision 1, evidence card Q-05 revision 1.
- Supersedes: none.
- Follow-ups: F-07 (the likelihood spec, blocked on W2).

### E-06: Q-06

- State: ready for integration.
- Disposition: adopt.
- Choice and constraints: option A. One l4clus spec measures the built `ScaleMinPtsWhenSubsampled`
  option against the shipped behaviour on the corpus frames over the 8,000-point cap, with the
  labelled score on kirk. Voxel downsampling is not wired until that spec reports.
- Rationale: the remedy exists, costs about an hour of machine time and no fingerprint change,
  and says whether D6 is worth a new L4 stage.
- Confirmation: explicit selection of the option in a text decision session; the question was put with the options and the recommendation marked, and the owner chose the option named above. No ambiguity remained.
- Provenance: Claude Code session `session_01CCGg6RfzL9uoYZhqKsEqK9`, text modality, 2026-10-22T01:08:56Z.
- Reviewed card: DR-MATHS-2026-10 revision 1, evidence card Q-06 revision 1.
- Supersedes: none.
- Follow-ups: F-08 (the spec).

### E-07: Q-07

- State: ready for integration.
- Disposition: adopt.
- Choice and constraints: option A. Approve l8fol0004, ground0001 and l3bg0024 this week; the
  other six proposed maths specs stay proposed. l8fol0004's `decide` step pins E-01's record.
- Rationale: each unblocks a chain at about four hours of machine time and 22 of person time.
- Confirmation: explicit selection of the option in a text decision session; the question was put with the options and the recommendation marked, and the owner chose the option named above. No ambiguity remained.
- Provenance: Claude Code session `session_01CCGg6RfzL9uoYZhqKsEqK9`, text modality, 2026-10-22T01:08:56Z.
- Reviewed card: DR-MATHS-2026-10 revision 1, evidence card Q-07 revision 1.
- Supersedes: none.
- Follow-ups: F-09 (status edits and graph regeneration).

## Follow-ups

| ID   | Question or decision entry | Action and reason                                                                                                                                                                                          | Owner      | Blocks | Acceptance condition                                      | Disposition and durable destination |
| ---- | -------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------- | ------ | --------------------------------------------------------- | ----------------------------------- |
| F-01 | inventory                  | The pipeline review still says "Active review (March 2026)" while its Q5 and Q8 are superseded by the state-estimation plan and D-04; mark it a historical review and point each question at its successor | unassigned | none   | Status line and §3 headers link the successors            | pending; integration phase          |
| F-02 | inventory                  | AGENTS.md's L3 row says "EMA + Welford variance"; the spread is an EMA of absolute deviation (gap analysis B6, beam-wise review errata); correct the row                                                   | unassigned | none   | AGENTS.md row matches `background-grid-settling-maths.md` | pending; integration phase          |
| F-03 | Q-03                       | After the rule is chosen, one shared percentile helper replaces the three aggregate helpers; the per-track one goes with D-18                                                                              | unassigned | none   | One helper, three call sites, tests pass                  | pending; integration phase          |
| F-04 | Q-04                       | A Pi frame-time measurement of A2 before any promotion, whichever screen is chosen                                                                                                                         | unassigned | none   | A perf spec names it                                      | pending; perf area                  |
| F-05 | E-01                       | Choose the 4 or 6 s net ceiling and the 10 mph band edge on kirk0's reviewed pairs; pin floor, ceiling, margin, dwell and band edge in the following record as a D-NN                                      | owner      | E-02   | l8fol0004 `decide` step closed with a D-NN                | pending; l8fol0004                  |
| F-06 | E-02                       | Record B10's deferral and its trigger in the behaviour plan's B10 row, D-28 and the backlog row                                                                                                            | unassigned | none   | The three rows cite E-02 and the trigger                  | pending; integration phase          |
| F-07 | E-05                       | Write the likelihood-cost spec (held-out identity, containment share as an input, inflation retirement as its consequence) once W2 is scored                                                               | unassigned | none   | Spec validates and depends on the W2 spec                 | pending; l5id area, blocked on W2   |
| F-08 | E-06                       | Write the l4clus spec for `ScaleMinPtsWhenSubsampled` on capped frames, adding a replay experiment switch if none exists                                                                                   | unassigned | none   | Spec validates; the switch has a test                     | pending; integration phase          |
| F-09 | E-07                       | Set l8fol0004, ground0001 and l3bg0024 to `approved`; regenerate DEPENDENCIES.md                                                                                                                           | unassigned | none   | `make check-experiments` passes                           | pending; integration phase          |

## Integration review

Not yet run.

| Entry | Technical verdict | Dependency closure | Checks and limitations | Outcome | Return question |
| ----- | ----------------- | ------------------ | ---------------------- | ------- | --------------- |

- PR disposition: preparation checkpoint.
- Merge event: pending.

## Provenance and coverage

- Preparation snapshot: commit `60c1e748f386c9bc24e27ec4c8486046c3c27cf9` on branch
  `claude/peaceful-davinci-w8iuw8`, equal to `origin/main` after a fetch on October 22, 2026;
  clean worktree.
- Source inventory (blob IDs at the snapshot): `pipeline-review-open-questions.md` `dfbdf141e764`;
  `MATHS.md` `780da0418a43`; `paper-implementation-gap-analysis.md` `47a858a66487`;
  `20261011-beamwise-background-subtraction-review.md` `539839499432`; `DECISIONS.md`
  `53d96ca865a7`; `BACKLOG.md` `782d551248fd`; `lidar-behaviour-analytics-plan.md`
  `293ce7260046`; `research-briefing-2026-10-19.md` `724e454e7bd1`;
  `speed-percentile-aggregation-alignment-plan.md` `1e0b9f3a11f7`; `l8fol0004.md` `d776bb4d2a5c`;
  `following-opportunity-corpus-2026-10.md` `5847d4ab346b`; `near-edge-frame-cost-2026-10.md`
  `ee8181701c2e`; `l4perception/cluster.go` `74367320a4df`; `l6objects/classification.go`
  `fd67b5e8eef0`; `analysis/report.go` `5ab3de955908`; `l5tracks/tracking_config.go`
  `8e350dc0ac43`. Open PRs: one, [banshee-data/velocity.report#503](https://github.com/banshee-data/velocity.report/pull/503)
  (Tailscale access control, head `4efc5586`), which touches none of these sources.
- Search coverage: the maths category only. Read in full: `MATHS.md`, the pipeline review, the
  gap analysis's summary, gaps and remediation plan, the beam-wise review's rule and open
  sections, the OBB review §9, the sign-anchor proposal §15, `DECISIONS.md`, the decision audit,
  the maths rows of `BACKLOG.md`, the percentile plan §5, the October 19 briefing's Q2 and open
  questions, the B1 report, the frame-cost record, the October 18 experiment review, and the
  headers of every proposed experiment spec. Searched with `rg` for "open question",
  "undecided", "decision", "{math}" and the gap IDs. Excluded: schema, project and research
  questions with no maths dependency; the facet registration and lane stitching plans beyond
  their backlog rows. No source was unavailable.
- Phase runs: prepare, revision 1, requested tier high; actual model `claude-opus-5-5` as the
  session reports it, session `session_01CCGg6RfzL9uoYZhqKsEqK9`, 2026-10-22T00:00:00Z (date
  known; time not recorded by the host).
- Phase runs (continued): voice, revision 2, requested tier light, actual model as the session
  reports it (`claude-fable-5-1`, text, no voice), session
  `session_01CCGg6RfzL9uoYZhqKsEqK9`, 2026-10-22T01:08:56Z.
- Validation: question IDs and source links checked by hand; states are all `ready for
integration` after revision 2; each entry records its confirmation; dependency rows cover each option; follow-ups have destinations;
  `make format-docs` and `make lint-docs` run before commit (results in the commit message).

## Resume handoff

- Next phase and requested tier: integrate/high.
- Packet and revision: `docs/platform/operations/decision-review-maths-2026-10.md`,
  DR-MATHS-2026-10, revision 2.
- Branch and PR: `claude/peaceful-davinci-w8iuw8`; committed; the draft PR link is in the
  Related line once opened.
- Read first: Decision log E-01 to E-07, Follow-ups F-03 to F-09, the Impact tables of the
  chosen options, then this section.
- Resume at: integration of E-01 to E-07. Confirmed: Q-01 to Q-07. Returned: none. Blockers:
  F-05 and F-07 are owner and W2 work, not integration blockers. Overflow: none.
- Invocation: `/decision-review-integrate docs/platform/operations/decision-review-maths-2026-10.md`
- Handoff prompt: Re-check the source blobs against the snapshot, then apply the chosen options'
  impact rows in dependency order: E-01 and E-02 (plan, briefing, D-28, backlog rows, l8fol0004's
  decide-step fields), E-03 (one nearest-rank helper replacing three, with tests and the
  documented rule), E-04 (near-edge plan screen definition, backlog clause, frame-cost record
  link, Pi perf step), E-05 (backlog S3 row and gap analysis), E-06 (l4clus spec and switch),
  E-07 (three specs approved, graph regenerated), plus F-01 and F-02. Record each row as
  changed, no change or deferred; run `make format-docs`, `make lint-docs`,
  `make check-experiments`, `make check-no-results` and the Go tests of touched packages; set
  `integrated` only where every row is accounted for; return a material conflict to voice with
  one question.
