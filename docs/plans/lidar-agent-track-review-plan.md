# Agent-assisted track review for ground truth

- **Status:** Draft
- **Layers:** L5 Tracks, L6 Objects, L8 Analytics, L9 Endpoints, offline analysis
- **Target:** v0.5.x, after the run-track finalisation fix; nothing here is sound on first-sighting rows
- **Companion plans:** [Background region overlays](lidar-background-region-overlay-plan.md), [Track quality score](lidar-visualiser-track-quality-score-plan.md), [Priority review queue](lidar-visualiser-priority-review-queue-plan.md), [Label-aware tuning](lidar-track-labelling-auto-aware-tuning-plan.md), [Point annotation](lidar-point-annotation-and-object-dataset-plan.md)
- **Canonical:** [track-labelling-ui-implementation.md](../lidar/operations/track-labelling-ui-implementation.md)

## Motivation

Ground truth for cars, pedestrians and noise rests on 120 hand-labelled tracks across 9 runs. A
single 34-minute replay of Columbus and Broadway produces 4,188 track rows. No person is going to
review that, and the 24-site corpus is two orders of magnitude beyond it. Tuning is therefore
steered by a reference set too small to notice most of what a parameter change does.

An AI agent can read a track's evidence and grade it against a written rubric far faster than a
person. The question this plan answers is what the agent should look at, where its grades go, and
how much they can be trusted. The consequence of getting it wrong is worse than having no labels:
a large, confident, subtly biased label set that every later experiment is tuned to agree with.

## The question asked: macOS annotation tool, or the web scene?

Neither is the right instrument for an agent, and they differ in what they are accurate about.

| Surface          | What it holds                                       | Fidelity                                                                                                                                   | Can an agent use it?                              |
| ---------------- | --------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------- |
| PCAP replay      | Every return                                        | Source of truth                                                                                                                            | Only by producing one of the rows below           |
| VRLOG            | Frames as recorded: points, clusters, tracks, debug | Exact float32 as recorded; usually foreground frames plus a background snapshot every 30 s                                                 | Yes, headless, through Go tooling                 |
| Annotation pack  | Up to 200 frozen samples cut from a VRLOG           | Exact, digest-bound point indices; no tracks, clusters or boxes by design                                                                  | Its files, yes; its GUI, no                       |
| Web scene export | A presentation of a VRLOG                           | Lossy by design: frame stride, positions rounded to 1 cm, angles to 0.001 rad, an optional `MaxPointsPerFrame` cap, a voxelised background | Viewable in a browser, but not evidence of record |

So, to the two direct questions. **The web scene is decimated**, in four separate ways, all in
`internal/scene/export.go`, and all appropriate to what it is for. **The macOS annotation tool is
the more accurate of the two**, because a pack keeps exact points, but it is the wrong shape for
this job twice over: it is a GUI, which an agent cannot drive, and it is a point-membership tool
for a few dozen reference objects, not a triage tool for thousands of tracks. It deliberately
carries no tracks at all.

An agent should review from the VRLOG, headless, through an evidence pack built for the purpose.
The web player remains useful to a person checking an agent's grade, and the annotation tool
remains the instrument for the thing an agent reviewing tracks can never supply, which is recall:
see "What this cannot establish" below.

## Current state

Verified on 2026-09-20.

- **Labels.** `lidar_run_tracks` carries `user_label`, `label_confidence`, `labeler_id`,
  `labeled_at`, `quality_label`, `label_source`, `linked_track_ids`. `label_source` is documented
  as `human_manual`, `carried_over` or `auto_suggested`, and the API defaults it to
  `human_manual`.
- **Ground truth already excludes non-human labels.** The store's `manualLabelSourcePredicate`
  counts a label as manual only when `label_source` is empty or `human_manual`. Agent grades
  written under any other source cannot enter a ground-truth query by accident.
- **Vocabulary.** Seven classes (noise, dynamic, pedestrian, cyclist, bird, bus, car) and eight
  quality flags (good, noisy, jitter velocity, jitter heading, merge, split, truncated,
  disconnected), as offered by the macOS track inspector.
- **Reference labels.** 120 labelled tracks in 9 runs; the most-labelled runs hold 64, 22 and 14.
  Four of the labelled runs still have their VRLOG. The run holding 22, the kirk0 reference, does
  not.
- **Row quality.** Until the run-track finalisation fix, every row was written at a track's first
  sighting: no row has 50 observations, mean lifetime is 0.4 to 0.9 s, and about 80% have no
  class. Track counts such as 4,188 are therefore counts of track births, and say little about
  how many road users there were.
- **Existing designs.** A per-track quality score with reason codes and a priority review queue
  are both proposed and unbuilt. This plan consumes them; it does not replace them.
- **Evaluation.** `internal/lidar/adapters/ground_truth.go` matches candidate to reference tracks
  by temporal IoU above 0.3.

## Findings

| Area                            | Current state                                   | Severity | Release view                                |
| ------------------------------- | ----------------------------------------------- | -------- | ------------------------------------------- |
| Reference set size              | 120 tracks, 9 runs, class-imbalanced            | High     | Too small to steer a 24-site corpus         |
| Evidence an agent could read    | None packaged; a VRLOG needs Go tooling to open | High     | The blocking gap                            |
| Web scene as evidence           | Strided, quantised, capped, voxelised           | Medium   | Fine to look at, unsafe to grade from       |
| Annotation tool for triage      | Carries no tracks by design                     | Medium   | Right tool for recall, wrong tool for this  |
| Label provenance for non-humans | One free-text `label_source` column             | Medium   | Enough to separate, not enough to reproduce |
| Row measurements                | First-sighting snapshots until the fix lands    | High     | Rule-based triage is unsound before it      |

## Design / approach

### Principle

An agent grades what it is shown, so the work is in what it is shown. Build one evidence pack per
track, from the VRLOG at full precision, that a person and an agent can both read, and that is
identical every time it is built from the same recording.

### 1. The track dossier

`velocity lidar track-dossier --vrlog <dir> [--run <id>] [--track <id>...] --out <dir>` writes,
per track, a directory of two files.

**`dossier.json`**: the track's life as series and summaries.

- identity: run, track ID, source capture, time span in capture time, dossier schema version
- series per observation: time, position, speed, heading, box length, width and height, point
  count, mean intensity, range and azimuth from the sensor, matched or coasting
- summaries: lifetime, path length, net displacement, straightness, speed percentiles, extent
  percentiles, points-per-observation percentiles, fraction coasting, where it started and ended
  (inside the scene, at its edge, at a known occluder)
- context: nearest other tracks over its life, tracks that begin where it ends and end where it
  begins (split and merge candidates), the classifier's class history with confidence, the
  quality score and reason codes once that design is built
- background context: the region and beyond-baseline state of the cells under its path, from the
  [region overlay](lidar-background-region-overlay-plan.md); a "track" that never leaves cells
  the background model has wrong is most of a noise verdict on its own

**`sheet.png`**: one contact sheet, laid out the same way every time.

- a top-down view of the whole trail over the background points, coloured by region, with the
  sensor, a north arrow and a scale bar
- six to eight keyframes spread over the track's life, each as a top view and a side view of the
  track's own points **at one fixed metric scale**, with its box and a scale bar
- sparklines of speed, heading, extent and point count against time

The fixed metric scale is the detail that matters. A pedestrian is about half a metre square and
under two metres tall; a car is about four and a half metres long. Rendered to fit the frame they
look the same. Rendered at a fixed scale they do not, and neither an agent nor a person needs the
numbers to tell them apart.

Dossiers are deterministic: the same recording and the same tool version give the same bytes. The
JSON records a digest of the sheet, and a grade records the digest of the dossier it was made
from.

### 2. The rubric

A written rubric, versioned in the repository, defining each of the seven classes and eight flags
in terms an observer can check against a dossier: size ranges, speed ranges, gait-like speed
variation for pedestrians, the elevation and erratic path of birds, the short life, small
displacement and low point count of noise, what makes a track truncated as against disconnected.
Every class has counter-examples. `cannot tell` is a permitted answer and is expected to be
common at long range.

A grade is structured: class, class confidence, flags, one or two sentences of evidence citing
what was seen in the dossier, the rubric version, the agent and model identity, and the dossier
digest.

### 3. Triage, so that volume falls before an agent is involved

| Tier | Who    | What                                                                                                                                               | Expected share                                                       |
| ---- | ------ | -------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------- |
| 0    | Rules  | Tracks that are noise by measurement alone: below a lifetime, displacement and point-count floor                                                   | Most rows, if the first-sighting rows are any guide once re-measured |
| 1    | Agent  | Everything tier 0 does not settle, in priority-queue order                                                                                         | The bulk of the grading work                                         |
| 2    | Person | Agent `cannot tell` and low confidence; agent and classifier disagreement; a random audit sample from every class and every tier, including tier 0 | Small and bounded by the audit rate                                  |

Tier 0 thresholds are set from re-measured rows, not guessed, and tier 0 is audited like the rest:
a rule that discards real pedestrians at range is exactly the bias this plan exists to avoid.

### 4. Where grades go

Agent grades never become ground truth by being written. They are proposals.

- First delivery is file-based: `agent-reviews/<run_id>.jsonl`, one grade per line, beside the
  dossiers. No schema change, nothing written to a production database, trivially re-runnable
  and diffable.
- When the flow has earned it, grades are written to `lidar_run_tracks` under a new
  `label_source` of `agent_reviewed`. The manual predicate already excludes it.
- A person confirming a grade in the review UI rewrites the row as `human_manual` with their own
  `labeler_id`, keeping the agent's grade in the review file as history. Confirmation is an act,
  not a default: a bulk "accept all" is out of scope on purpose.
- Evaluation reports three numbers separately: against human labels, against agent grades, and
  against human-confirmed agent grades. They are never pooled.

### 5. Calibration before trust

Before any agent grade is used for anything:

1. Build dossiers for the labelled tracks whose VRLOG survives and have the agent grade them
   blind.
2. Report per-class precision and recall, the confusion matrix, and Cohen's kappa against the
   human labels, with confidence intervals. With roughly a hundred tracks and few cyclists or
   buses, those intervals will be wide, and the report says so.
3. Proceed only where the agent is demonstrably good, class by class. Object against noise and
   car against pedestrian are the two separations tuning depends on; rare classes may stay
   human-only for a long time.
4. The tier 2 audit sample keeps measuring agreement as the set grows, so drift in the agent, the
   rubric or the tracker shows up as a number.

### 6. How an agent reaches it

A Claude Code session reads `sheet.png` and `dossier.json` with its ordinary file tools and
appends to the review file. Nothing else is needed for the first delivery. An HTTP or MCP surface
over the same dossiers can follow if review moves off the machine that holds the recordings.

Cost is per track and dominated by the image: on the order of a few thousand tokens each. That
is affordable for a labelled active set and for audit samples, and expensive across the whole
corpus, which is the practical argument for tier 0 and the priority queue.

## What this cannot establish

An agent reviewing tracks grades the tracks that exist. A road user the tracker missed has no
track, so no dossier, so no grade. **This flow measures precision and class, never recall.**
Recall needs a reference made independently of the tracker, which is what the point annotation
tool and its reference objects are for. The two are complementary and are kept apart: agent
grades at scale on tracks, human reference objects on a handful of short segments.

There is a quieter circularity too. A dossier shows the tracker's box, trail and class. An agent
shown a confident "car" will lean towards it. Calibration is therefore run with the classifier's
class hidden, and the disagreement rate with it shown is reported as a bias measurement.

## Scope

### Item 1: dossier generator

**Summary:** Deterministic per-track evidence from a VRLOG, as JSON and a fixed-scale sheet.

**Steps:**

1. Dossier schema, versioned; series and summaries from the recorded track set.
2. Sheet renderer: top-down trail, fixed-scale keyframe crops, sparklines.
3. Determinism test: two builds from one recording are byte-identical.
4. Background context fields, empty until the region overlay records them.

**Milestone:** v0.5.x

### Item 2: rubric and grade schema

**Summary:** Written class and flag definitions an observer can check, and a structured grade.

**Steps:**

1. Draft from the existing label vocabulary and the labelled tracks.
2. Grade schema with rubric version, agent identity and dossier digest.
3. Review by the person who made the existing labels; their disagreements with the draft are the
   most valuable input it will get.

**Milestone:** v0.5.x

### Item 3: calibration run

**Summary:** Measure the agent against the human labels before using a single grade.

**Steps:**

1. Repair or re-measure the labelled runs' rows (see Dependencies).
2. Blind grading with the classifier's class hidden; then again with it shown.
3. Agreement report with intervals, per class; decide class by class.

**Milestone:** v0.5.x

### Item 4: triage and the review loop

**Summary:** Rules, agent, person, with audit sampling across all three.

**Steps:**

1. Tier 0 thresholds from re-measured rows; audit rate set.
2. Agent grading in priority order into review files.
3. A person's queue of `cannot tell`, disagreements and the audit sample, in the existing
   labelling UI.

**Milestone:** v0.5.x

### Item 5: grades in the database

**Summary:** `agent_reviewed` as a label source, and confirmation as an explicit act.

**Steps:**

1. Accept and store the new source; confirm the manual predicate still excludes it, with a test.
2. Confirmation path that rewrites to `human_manual` with the confirmer's identity.
3. Evaluation reports the three agreement numbers separately.

**Milestone:** v0.5.x, after calibration has passed for at least object against noise

## Dependencies

- **Run-track finalisation fix.** Branch `dd/go/run-track-finalise`. Tier 0 rules, summaries and
  the evaluator's temporal IoU are all unsound on first-sighting rows.
- **Repair of the labelled reference runs.** Their rows predate the fix. Where the VRLOG
  survives, measurements can be rebuilt from it per track ID. The kirk0 reference has lost its
  VRLOG and needs re-labelling on a fresh run.
- **Deterministic track identity.** Without it a label dies with its run. Agent grades are cheap
  to redo; human labels are not.
- **Region overlays.** For background context in the dossier. Not blocking.
- **Full-retention recordings** for the active set, where a question needs the whole scene and
  not only foreground.

## Risks

- **Confident, biased labels at scale.** The central risk. Mitigated by keeping agent grades out
  of ground truth, calibrating per class, auditing every tier, and reporting agreement as a
  number over time.
- **Anchoring on the tracker's own output.** Measured by grading with and without the
  classifier's class shown.
- **Rubric drift.** A rubric edited to raise agreement is a rubric fitted to its test set. Rubric
  changes are versioned, and agreement is always reported against a stated version.
- **Tier 0 discarding real road users.** Distant pedestrians are short-lived, small and sparse,
  and so is noise. Tier 0 is audited at the same rate as everything else.
- **Cost.** Bounded by tiering and the priority queue, and measured on the calibration run
  before any corpus-wide pass is attempted.

## Checklist

### Outstanding

- [ ] Dossier schema and generator with a determinism test (`L`)
- [ ] Fixed-scale sheet renderer (`M`)
- [ ] Rubric and grade schema, reviewed by the existing labeller (`M`)
- [ ] Reference-run row repair from VRLOG (`M`)
- [ ] Calibration run and per-class agreement report (`M`)
- [ ] Tier 0 thresholds from re-measured rows (`S`)
- [ ] Review-file loop and the person's queue (`M`)
- [ ] `agent_reviewed` source, confirmation path, three-way evaluation report (`M`)

### Deferred

- [ ] HTTP or MCP surface over dossiers, until review leaves the recording host
- [ ] Agent-proposed point masks in the annotation tool, tracked by
      [point annotation](lidar-point-annotation-and-object-dataset-plan.md)

### Accepted residuals (no action planned)

- [ ] Recall is not measurable by this flow; it stays with human reference objects
- [ ] Rare classes may remain human-only indefinitely
