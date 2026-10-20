# Experiment review, October 18, 2026: the following-distance metric's next seven days

<!-- ignore-style-length -->

The October experiments (geom0001 to geom0010, l3bg0001 to l3bg0018, l4clus0001 and
l4clus0002, l5id0001 to l5id0005) answered thirty-eight questions in twelve days. This review
reads what they established for one product measure, the following distance between two
vehicles, ranks what to run next, says which of it a machine can do overnight and which needs a
person at the annotation window, and names the engineering each step waits on. The specs it
proposes are under `data/experiments/` with their rules written now, before any replay; the
owner approves them.

- **Status:** Review, 2026-10-18; no measurement was run and no default changed
- **Goal:** the bumper-to-bumper gap and net time gap of
  [Sprint 0.5.2.3](../../BACKLOG.md#v052---following-distance-evidence-052) and
  [0.5.2.4](../../BACKLOG.md#v052---following-distance-evidence-052)
- **Inputs:** the [dependency graph](../../../data/experiments/DEPENDENCIES.md), the
  Recommendations of every October report, the
  [0.5.2 review](0.5.2-sprint-review.md), the
  [MVP sprint plan](../../plans/lidar-052-mvp-sprint-plan.md), the
  [behaviour analytics plan](../../plans/lidar-behaviour-analytics-plan.md), the
  [physical-reference plan](../../plans/lidar-physical-reference-review-plan.md), the three
  October briefings and [D-28](../../DECISIONS.md#d-28--research-briefing-decisions-october-2026)
  and [D-29](../../DECISIONS.md#d-29--research-briefing-decisions-october-12-2026)
- **Method:** the [experiment-review skill](../../../.claude/skills/experiment-review/SKILL.md),
  written from this review
- **Related:** [experiment queue plan](../../plans/platform-experiment-queue-plan.md),
  [run-experiments skill](../../../.claude/skills/run-experiments/SKILL.md)

## Answer

The following metric's machinery runs end to end and finds nothing: on kirk0 the field run fits
7 of 60 follower paths and 3 encounters with no valid time, because the only physical endpoint
source is the near-edge solid body, which needs a face fix, and because the class gate admits 3
of 60 tracks. The October experiments moved the three things upstream of it. A quarter of
vehicle tracks were duplicates and now need not be (l5id0004 held out and replicated, off until
labelled). The box every viewer draws now holds its returns with its axis on the direction of
travel (geom0005, the default). The L3 retune keeps 87 % of reviewed returns against 49 % (CMR,
Stage B waiting). None of the three is adopted where it matters, and all three wait on the same
thing: one labelled window nobody tuned on, label0001, which has been proposed since October 13
and has not started. That window is the week's critical path. One operator pass on it, extended
with fitted bodies and following records (label0002), serves the identity decision, the L3
decision, the geometry line's W7 and the following gate at once.

The machine's week is five experiments that need no person: whether both parties of a pair are
ever observed (B1, never measured), whether the published box can stand in for the solid body
as the endpoint source (the largest single lever on the field value), what class evidence the
gate actually has, the recommended refinements of the overlap end, and the W9 decomposition
that says whether the gap's error is the body's length. Two zero-code L3 and L4 reads on kirk
fill the gaps. Together they are about six hours of replays and twelve of agent implementation,
two overnight runs on the Mac, and they produce the site ranking and the tuning-side limits the
labelled window is scored with. The papers add two measures and no new direction; nothing in
them should be read this week beyond one specification document.

## What the October experiments established

| Line                             | Experiments                                | What held                                                                                                                                                                                                                                                                                                                                                                                             | What it means for following                                                                                                                                                                                                            |
| -------------------------------- | ------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| One vehicle, one track           | l5id0001 to l5id0005                       | Under the defaults 15.5 pairs of vehicle tracks per hundred overlap for five frames or more; the overlap end on the body's returns, keeping the older track, takes that to 1.2 to 1.4 per hundred on three sets of segments with observed vehicle frames at 100 to 102 % of the defaults' ([report](l5-duplicate-tracks-2026-10.md))                                                                  | A duplicate reads as a follower under a metre behind, so every following count on the corpus carries a sixth of false pairs until this ships. It ships only after a labelled held-out window scores it                                 |
| The published box                | geom0005 to geom0010                       | The box model holds every vehicle frame's returns, its axis 1.6° from the direction of travel at the median held out, and is the default; the trucks are drawn at 0.62 of their length because a growing fragment trips the merge guard and a truck's other fragments are other clusters ([report](published-box-refinement-2026-10.md))                                                              | A heading and extents exist on every observation of every vehicle track, which the solid body has for 7 of 60; the box is the candidate endpoint source. A truck's rear will be short by what its box is short                         |
| Physical references              | the kirk0 pilot                            | Three vehicles fitted to their reviewed returns and two following pairs drafted and frozen; the near-edge arm's gaps within 0.12 to 0.46 m where scored, the control's 7.9 m off once; truck 2's gaps 1.6 to 2.2 m long because its footprint ends short of its front ([report](physical-reference-pilot-kirk0-2026-10.md))                                                                           | The workflow works and the gap error follows the length error. The reference gaps carry half-widths of 1.5 to 2.0 m, wider than any limit worth pinning, so reference precision, not the tracker, bounds what a held-out score can say |
| The gate                         | the R6 review and v2                       | The held-out gap scorer pins its plan and reference digest and fails on unmatched shares; it still keys encounters to track ids, tests bounds on point estimates and cannot check the leader ([review](0.5.2-sprint-review.md#r6-the-held-out-gap-scorer-is-a-component-not-a-complete-promotion-gate))                                                                                               | Leader truth now exists as reviewed following records (#714); the scorer does not read it. The following limits are unpinned, so physical scoring refuses every held-out split                                                         |
| L3 retune and the dwell policy   | l3bg0001 to l3bg0018                       | CMR keeps 87 % of reviewed returns against the shipped 49 %, pedestrians 0.83 against 0.17, moving tracks up 60 % at 44 of 45 site-segments; the dwell policy clears long stationary tracks at 20 s without costing a moving road user; Stage B waits on labels and the Pi ([corpus](l3-candidates-corpus-dwell-2026-10.md), [labelled cost](l3-dwell-policy-labelled-cost-2026-10.md))               | Longer, whole tracks are what the class gate and the leader choice need; nothing adopts until label0001 and perf0001                                                                                                                   |
| Retention and fragmentation (W3) | geom0003, geom0004, l4clus0001, l4clus0002 | Half of labelled returns reach a cluster under the shipped L3; a third of the instants that do are split, most at 20 to 50 m and on trucks; the fate measure names the stage that loses each return; lifting the input cap recovers returns and joins no fragment ([W3](w3-retention-fragmentation-kirk0-2026-10.md), [fate](w3-return-fate-kirk0-2026-10.md), [cap](l4-dbscan-input-cap-2026-10.md)) | Fragments are duplicates, short trucks and missing class evidence at once; the fix is v0.5.6 work, and two zero-code reads this week say which of the Stage B arms touches the mid-range gaps                                          |
| Geometry convergence             | geom0001, geom0002 (open)                  | The rectangle heading held out; the lateral margin shrank to a third; W1b's remaining pieces (the low-speed label, the ambiguous row) are a 24-hour implementation ([report](geometry-convergence-heldout-2026-10.md))                                                                                                                                                                                | The box model supplies the heading the gap projects along; W1b refines the solid body's, which the gap no longer depends on first. Deprioritised this week                                                                             |

## Where the metric stands

The chain from a capture to a following figure, each link with its measured state:

1. **Tracks.** A quarter duplicated under the defaults; one track per vehicle held out but off.
   Fragmentation at 20 to 50 m is a third of instants and is upstream of everything below.
2. **Bodies.** The solid body places a body centre on a fifth of samples; the box model places a
   centre, axis and extents on every observation but records no face, so every box endpoint is
   an inference, and trucks are short.
3. **Class.** Following is a rigid-vehicle metric on both sides. The gate admitted 3 of 60
   kirk0 tracks; nothing measures class recall against the reviewed pack.
4. **Pairs.** The local path, leader choice and encounter accounting are built and tested on
   fixtures; on kirk0 7 of 60 follower paths fit. How often both parties are observed at all
   (B1) has never been measured on the corpus.
5. **The gate.** The physical scorer scores gaps against reviewed references (G9: 3 of 7 within
   bound on the pilot) but refuses held-out splits until the following limits are pinned; the
   leader is not scored; the along-path gap waits on persisted paths (R5).
6. **The field value.** `velocity report headway --db --solid-bodies` renders a provisional
   report with every page labelled; on kirk0 it has no valid second to show.

Nothing below link 4 can be read until links 1 to 3 give it tracks with bodies and a class.
Nothing in links 1 to 3 can be adopted until a labelled held-out window exists. That is why the
ranking below puts the window first for the person and the endpoint source first for the machine.

## The next seven days

Ranked by the defect measured, what an answer unblocks, whether it runs without a person, and
cost, as the run-experiments skill ranks. Person time is the operator's; machine time is on the
M1 Pro; an agent closes `manual:implement` steps overnight, as the October runs did in 15 to 35
minutes against 4 to 16 hour person estimates.

| Rank | Spec                                                                                  | Question                                                                                                                             | Runs without a person         | Machine | Agent implement | Unblocks                                                                                                   |
| ---- | ------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------- | ------- | --------------- | ---------------------------------------------------------------------------------------------------------- |
| 1    | [label0001](../../../data/experiments/label/label0001.md) (proposed since October 13) | A lot-drawn window at embarcadero-folsom, labelled, reviewed and frozen                                                              | No: 8h15m operator            | none    | none            | l5id0004's default, l3bg0009, the geometry line's W7, l8fol0005                                            |
| 2    | [label0002](../../../data/experiments/label/label0002.md) (new)                       | The same window's vehicles fitted and keyframed, and a following record for every follower interval                                  | No: 5h15m operator            | none    | none            | l8fol0005, W7's held-out score, the duplicate count checked against bodies                                 |
| 3    | [l8fol0003](../../../data/experiments/l8fol/l8fol0003.md) (new)                       | The published box as the endpoint source: G9 on kirk0's 11 reference gaps, and how many paths and encounters the field run then fits | Yes                           | 55m     | 4h              | A first non-empty provisional field value; the reader l8fol0005 scores                                     |
| 4    | [l8fol0001](../../../data/experiments/l8fol/l8fol0001.md) (new)                       | B1: pair-seconds, pairs, leader changes and closest gaps on the 23 first segments, under the defaults and one track per vehicle      | Yes                           | 1h24m   | none            | B10 (deferred on B1 by D-28); which sites deserve following references; the R5 denominators' expected size |
| 5    | [l8fol0004](../../../data/experiments/l8fol/l8fol0004.md) (new)                       | The following limits pinned on kirk0's pairs, with the scorer reading leader truth, counting bodies and giving bootstrap verdicts    | Partly: 3h pairs, 1h decision | 6m      | 6h              | Physical scoring admitted to a held-out split; l8fol0005                                                   |
| 6    | [l8fol0002](../../../data/experiments/l8fol/l8fol0002.md) (new)                       | Class evidence on the two labelled kirk captures: which reviewed vehicles end classed rigid, when, under four arms                   | Yes                           | 24m     | 3h              | Whether the class gate or the geometry hides following; the classifier plan's acceptance figure            |
| 7    | [l5id0006](../../../data/experiments/l5id/l5id0006.md) (new)                          | The surviving track's box inheriting the ended track's beliefs; tentative priority with the overlap end; the 50 m count error        | Yes                           | 2h24m   | 3h              | The box's axis p90 back from 13.4° to 8.4° with one track per vehicle; a survey-facing identity figure     |
| 8    | [geom0011](../../../data/experiments/geom/geom0011.md) (new)                          | W9: how much of the reference centre error is the length's, per arm including the box                                                | Yes                           | 9m      | 2h              | Whether the gap's next lever is upstream (fragments, merge spans) or the centre                            |
| 9    | [l3bg0019](../../../data/experiments/l3bg/l3bg0019.md) (new)                          | Return fate under the Stage B arms on kirk0: do the dwell policy or the cell tercile touch the 20 to 35 m gaps                       | Yes, zero code                | 16m     | none            | The W3 fix's design; whether any Stage B arm changes fragments                                             |
| 10   | [l4clus0003](../../../data/experiments/l4clus/l4clus0003.md) (new)                    | CMR's minimum cluster size of 8 against the default, on W3 fragmentation at 20 to 35 m                                               | Yes, zero code                | 20m     | none            | Whether l3bg0009 gains a `cr` arm; the fragments that remain are L3's                                      |
| 11   | [l8fol0005](../../../data/experiments/l8fol/l8fol0005.md) (new)                       | The first held-out following score on the labelled window                                                                            | Yes, once 1, 2 and 5 are done | 18m     | none            | The metric gate's evidence                                                                                 |
| 12   | [perf0001](../../../data/experiments/perf/perf0001.md) (proposed)                     | CMR's frame-time tails on the Pi; the box model's Pi timing is owed under the same step                                              | No: a Pi 5                    | 20m     | none            | l3bg0009; the box model's claim on the deployment target                                                   |
| 13   | [geom0002](../../../data/experiments/geom/geom0002.md) (proposed)                     | W1b's low-speed label and ambiguous row                                                                                              | Yes, 24h agent                | 1h      | 24h             | The solid body's young-track axis; not on the following path this week                                     |

**Two overnight runs.** The first night runs what needs no code: l3bg0019, l4clus0003 and
l8fol0001 (zero code), and geom0011 (a two-hour measure). The second runs the three with
implementation steps: l8fol0003, l8fol0002 and l5id0006, each rule committed with its spec before
its replays. Both fit a twelve-hour `/run-experiments` window on the Mac with the NAS mounted.
In the cloud only the kirk0 half runs (l8fol0002 on kirk0 alone, geom0011, l3bg0019, l8fol0003's
score step); the corpus screens need the LiDAR volume or the NAS, or one placement at a time from
the Hugging Face dataset.

**What is read first.** l8fol0001's site ranking says where following references are worth the
operator's time beyond the lot-drawn window; l8fol0003's result says whether the box or the solid
body is the source l8fol0005 scores; l8fol0002's says whether the class gate is the binding
constraint. Follow a result where it leads: if the box fails G9 on the cars as well as the
trucks, the next question is the centre (geom0011's remainder), not more sites.

**Held-out ledger.** Segments read for a measure are no longer held out for it. Identity has
read the second (l5id0004), third (l5id0005) and fifth (l5id0003) segments; the box's labels the
fourth (geom0006); the geometry candidate the second (geom0001). l5id0006 reads the fourth for
identity. l8fol0001 reads only the first segments, so the second stay held out for following
opportunity. embarcadero-folsom has never been read by any screen and is the lot for label0001.

## Labelling and annotation priorities

The operator's hours are the constraint every gate names, and three plans ask for three
different captures. They are one window. The W7 spike's rule (one five-minute segment the corpus
never used, from a site with end-on geometry, by lot, reviewed whole with operator-keyframed yaw
and extents) and label0001's draw (a window of embarcadero-folsom, by lot, not by what is in it)
name the same segment, and the following gate's references are fits and records on the same
reviewed objects.

| Order | Task                                                                                                                                                                                                                                                                                           | Person time | Tooling that exists                                                                                                                                                                       | Gates it serves                                                                       |
| ----- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------- |
| 1     | **label0001, select and cut.** Draw the window by lot with `velocity lidar segments --finder random --role held_out` on embarcadero-folsom, record the draw, cut the pack with `annotation-clip`, open it and run Propose objects. An agent can do everything but the window's proposals today | 1h          | [segments finder](../../plans/lidar-annotation-segment-finder-plan.md), [one-step clip](point-annotation-tool.md#choosing-what-to-cut)                                                    | All of them                                                                           |
| 2     | **label0001, label and review.** Road users and the full-scene static and dynamic labels, then a second pass that grades every proposal; freeze with role `held_out`                                                                                                                           | 7h15m       | [label sets](label-sets.md), [checks as warnings](point-annotation-tool.md#checks) (#751)                                                                                                 | l5id0004's default, l3bg0009 Stage B, W7                                              |
| 3     | **label0002, bodies.** `annotation-reference fit` for every vehicle with reviewed masks, each pose reviewed; long bodies keyframed by hand for yaw and extents, end on or not                                                                                                                  | 3h          | [fit to reviewed points](point-annotation-tool.md#fitting-to-reviewed-points) (F1, F2, F4); F5's shared-sample suggestions are not built, so the operator picks the samples a pair shares | W7's held-out score, geom0011's successor, the duplicate proxy checked against bodies |
| 4     | **label0002, following records.** `draft-following` for every follower interval, leader, `no_leader` or `ambiguous`, imported reviewed; freeze the physical revision in the split                                                                                                              | 2h15m       | [drafting a following reference](point-annotation-tool.md#drafting-a-following-reference) (#714)                                                                                          | l8fol0005                                                                             |
| 5     | **l8fol0004, kirk0's other pairs.** The pilot's two pairs are two slow trucks and the car between them; fit and review kirk0's other followed vehicles at shared samples, draft their records, refreeze the tuning split with non-overlapping episodes                                         | 3h          | The same; the pilot's [recommendations](physical-reference-pilot-kirk0-2026-10.md#recommendations)                                                                                        | The limits pinned on tuning data; nothing held out is read until then                 |
| 6     | **l8fol0004, the decision.** Pin the following record (gap error limit, reference precision, coverage, minimum independent encounters, missing opportunity, wrong-leader tolerance) as a D-NN, against the display resolution and the references' own half-widths                              | 1h          | The W0 [criteria record](geometry-convergence-criteria.md) as the model                                                                                                                   | l8fol0005                                                                             |
| 7     | **W7 tuning windows** (1st-mission track 775, 3rd-folsom track 730), reviewed and frozen as tuning: an end-on truck that is not kirk0's for W1b and the box's length                                                                                                                           | 3h          | The [spike](end-on-truck-window-spike-2026-10.md#the-capture-request)                                                                                                                     | The geometry line only; after 1 to 6                                                  |

About seventeen and a half hours for items 1 to 6, which is two and a half hours a day over the
seven days. Not this week: the facet annotation pilot, the Columbus and Embarcadero pedestrian
and cyclist windows (following is rigid-vehicle only), and any second site's following pairs
before l8fol0001 says which site holds pairs.

Two rules hold throughout. A held-out window is chosen by lot or by traffic, never for a
failure, and a window read once for a measure is tuning for it afterwards. A tracker-assisted
pose stays assisted whatever is done to it later; the pilot lost a sample to that and the app
setting that caused it is cleared.

## Superseded and revised work

Backlog items whose remaining work this review changes, applied in the same change under the
backlog's governance (split, consolidate, complete; nothing deleted).

| Item                                                             | Change                                                                                                                                                                                                                      | Why                                                                                                                                                  |
| ---------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| Second capture with an end-on truck (W7), 0.5.2.1                | Superseded as a capture: the held-out segment is label0001's lot, reviewed whole under label0002; the tuning windows stay                                                                                                   | The spike's rule and label0001's draw are one segment; a third capture request would spend operator hours twice                                      |
| Reviewed reference pilot, operator pass, 0.5.2.0                 | The kirk0 pilot is delivered; what remains is kirk0's other pairs (l8fol0004) and the held-out window's bodies and records (label0002)                                                                                      | The [pilot report](physical-reference-pilot-kirk0-2026-10.md) closed the workflow question; the item now names the two specs instead of "more pairs" |
| Following gate completeness, 0.5.2.3                             | Independent leader truth is no longer missing: reviewed following records carry leader, `no_leader` and `ambiguous` (#714). What remains is the scorer reading them, bodies by object id, the bootstrap verdict (l8fol0004) | R6's list predates #714                                                                                                                              |
| Provisional headway field value, 0.5.2.4                         | Split: the class evidence is l8fol0002's measure, and the endpoint source is a new item, the published box read as the body (l8fol0003); the item keeps the pipeline producer and the provisional label                     | "Supply class evidence" had no measure; "7 of 60 paths" has a named cause with a cheaper source than the solid body                                  |
| Headway field validation and promotion, 0.5.2.4                  | Names l8fol0005 as the first held-out score, after l8fol0004 and label0002                                                                                                                                                  | The promotion item had no path from today to a held-out figure                                                                                       |
| Encounter-weighted following view (B10), 0.5.2.4                 | B1 is l8fol0001; the deferral stands until it reports                                                                                                                                                                       | D-28 deferred B10 on a measurement nobody had scheduled                                                                                              |
| Reacquisition and identity acceptance, 0.5.2.2                   | Gains l5id0006 (the box inheriting beliefs; tentative priority with the overlap end)                                                                                                                                        | The duplicate tracks report's recommendations 3 and 4                                                                                                |
| Physical scoring criteria and held-out acceptance, 0.5.2.0       | The following limits are l8fol0004's; the geometry levels (W0) are pinned already                                                                                                                                           | The item owned both and named neither spec                                                                                                           |
| Physical references by fitting to reviewed points, 0.5.2.0       | F5 (shared-sample suggestions for a pair) is what label0002 and l8fol0004's pairs need; named as the next increment                                                                                                         | Without it the operator picks shared samples by hand, which the pilot found costly                                                                   |
| Foreground retention and cluster fragmentation (W3, fix), v0.5.6 | l3bg0019 and l4clus0003 read where the mid-range fragments' returns go under the Stage B arms before the fix is designed                                                                                                    | The fix is three rules; two cheap reads say which rule the Stage B arms already touch                                                                |

Nothing is retired. geom0002, l3bg0009 and perf0001 stand as proposed; this review changes their
order, not their content.

## Engineering work needed

What a spec's `manual:implement` step or a labelling task waits on, by size, with the spec that
reads it. Agents close the `S` and `M` items overnight under `/run-experiments`; the `L` items
are a person's sprint work.

| Work                                                                                                                                                                                                           | Size         | Reads it                         | Why it matters for following                                                                                                                                 |
| -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------ | -------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `TrajectoriesFromBoxes` and `velocity report headway --db --boxes`: the published box (migration 000062) read as the body, endpoints `temporally_inferred`, class from the track's label                       | M            | l8fol0003, l8fol0005             | Every confirmed vehicle track has a box; the solid body gives 7 of 60 paths. The largest lever on a non-empty provisional value                              |
| The following gate on the physical scorer: cite a predeclared following record, score leader choice against reviewed following records, count bodies and body pairs, bootstrap verdicts, admit held-out splits | M {math}     | l8fol0004, l8fol0005             | R6's remainder; without it no held-out following figure can be read, whatever the tracker does                                                               |
| A class measure in `lidar-ground-truth-eval perframe`: matched track class per frame and at its end against the reviewed label                                                                                 | S            | l8fol0002                        | The gate's binding constraint is unmeasured                                                                                                                  |
| `BoxInherit` on the overlap end, and a 50 m zone count in `lidar-box-eval`                                                                                                                                     | S            | l5id0006                         | Gives back the axis p90 the overlap end costs; the count error is the figure the fragment-stitching papers report                                            |
| The W9 decomposition in the physical scorer                                                                                                                                                                    | S            | geom0011                         | Says whether the gap's error is the length's                                                                                                                 |
| Fit-to-points F5: suggested shared samples for a following pair, fitting several frames at once                                                                                                                | S            | label0002, l8fol0004             | Halves the operator's time per pair                                                                                                                          |
| Object proposer in Go, wired into the window and run by the clip job                                                                                                                                           | L            | label0001 and every later window | Moves the proposals (4 of label0001's 8 hours are proposing and correcting) off the operator's machine and makes an agent able to prepare a window overnight |
| Persisted paths and follower decisions, including free flow and refused paths (R5)                                                                                                                             | M            | l8fol0005's successor            | The along-path gap and the unique opportunity denominator; the scorer reads the gap along the reference axis until then                                      |
| Foreground retention and fragmentation fix (W3)                                                                                                                                                                | L {math}     | after l3bg0019                   | Fragments are the duplicates, the short trucks and the missing class at once; v0.5.6 as scheduled, designed from the fate reads                              |
| Pi timing for CMR and the box model (perf0001)                                                                                                                                                                 | hardware     | l3bg0009                         | Needs a Pi 5 for twenty minutes; the Mac's timings are not evidence for the target                                                                           |
| The following distance debug layer (S6)                                                                                                                                                                        | L {frontend} | none this week                   | Product, not evidence; after a field value exists to draw                                                                                                    |

## What the papers say to do, and what to leave

The three briefings and D-28 and D-29 already judged the October sources; this reads them for
the following metric only and adds no new paper.

- **Schäfer, Alrifaee and Hashemi 2026** gives the centre-versus-extent decomposition (W9, geom0011).
  The pilot's reading, that the along-path error is mostly an extent too short, becomes a number
  per arm, including the box.
- **Dong et al. 2026 and Luo et al. 2025** report count error on a 50 m zone before and after
  fragment stitching (14.5 % to 2.6 %). The identity link is the only part adopted, and l5id0006
  reports the same figure on kirk0 so a survey can quote it; interpolation across a gap stays
  refused, so a stitched interval contributes no valid following time (Section 9.2).
- **The FMCSA handheld programme** validates an intervention, not a sensor, and has published no
  report. The one document worth fetching this week is NHTSA's DOT HS 809 811, the specification
  the handheld instruments meet, because its stated tolerance (±0.3 m at 90 m, **Confirm**) is a
  defensible display resolution for l8fol0004's limits to be chosen against. Nothing else.
- **Cellina et al.** (an RTK probe vehicle) is the external reference the kirk0 fits cannot be;
  D-29 adopted it as a proposal with fix availability measured first. It is fieldwork, not this
  week.
- **R-LiViT** is the published analogue of B1 for pedestrians and cyclists at pole height;
  l8fol0001 measures the vehicle case on our own corpus before any comparison.
- **Baumann et al.'s point-wise labels** and the **Applied Sciences rectangle fit** stay where
  D-28 left them: gated on a data release and a reading.

## Decisions for the owner

| Decision                                                                                                                                                         | Recommendation                                                                                                                                                              |
| ---------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Approve label0001 and schedule its hours; approve label0002 on the same window                                                                                   | Yes, first. It is the critical path for three adoptions and the gate; nothing an agent runs this week replaces it                                                           |
| Approve the `l8fol` area and the nine proposed specs, with `corpus_screen` carrying the finder and field-run reads until a `following_screen` kind is registered | Yes; register the kind when the hub runs it                                                                                                                                 |
| Adopt l5id0004's overlap end on the label-free held-out evidence alone, or wait for label0001                                                                    | Wait, as the report says, and let l8fol0001 and l8fol0003 run both arms so the cost of waiting is measured: every following count carries a sixth of false pairs until then |
| Twenty minutes on a Pi 5 for perf0001 and the box model's timing                                                                                                 | Schedule it; it is the only step of l3bg0009 no agent can close                                                                                                             |
| Whether a following limit looser than the display resolution may count toward a pass                                                                             | No: record it as insufficient reference precision and let label0002's keyframed references narrow it                                                                        |
| Whether the W7 tuning windows (two end-on trucks) are reviewed this week                                                                                         | After items 1 to 6; the geometry line can wait a week, the following gate cannot                                                                                            |

## What this review does not establish

It ran nothing. Every figure is a report's, read at the commit below, and the ranking is a
judgement about what each answer unblocks, not a measurement. The person-time estimates are the
specs' and have not been calibrated against an operator pass of this shape; the pilot's hours
were not recorded. The machine estimates use October's actuals, where machine steps ran from a
fiftieth of their estimate to seven times it. A spec proposed here can be wrong about what a
tool can read; the validator checks its shape, not its feasibility, and the first overnight run
is where that is found.

## Provenance

- Repository at `972b23bf` (main, 2026-10-18), with [#777](https://github.com/banshee-data/velocity.report/pull/777)
  and [#503](https://github.com/banshee-data/velocity.report/pull/503) open and neither on this
  path.
- `go run ./cmd/tools/experiments next` at that commit: geom0002, l3bg0009, label0001 and
  perf0001 open, 1h47m machine and 39h15m person estimated; longest open chain geom0002 at 27h2m.
- Reports read in full or by their Summary, Limitations and Recommendations: the duplicate tracks,
  published box rotation and refinement, physical-reference pilot, geometry held-out, W3,
  return fate, input cap, L3 attribution, region levers, dwell policy (arrival, labelled cost,
  clock, drift), corpus candidates, pose step and corpus sweep reports; the 0.5.2 sprint review;
  the end-on truck spike; the headway report oracle's provisional field run.
- Plans: the MVP sprint plan, the behaviour analytics plan (Sections 8.3, 10.4, 11, 12 and 13),
  the physical-reference review plan, the geometry convergence plan (Sections 5 and 6), the
  experiment queue plan, the label-set team workflow plan, the fit-to-points plan.
- Briefings: October 9 (fixed-sensor), October 10, October 12 and the January 2025 to October
  2026 window; D-28 and D-29.
- No raw output was produced or read; nothing under a `results` directory was touched.
