# One review workflow: proposals, grading and reference truth

- **Status:** Draft
- **Layers:** L4 Perception, L5 Tracks, L6 Objects, L8 Analytics, L9 Endpoints, L10 Clients (macOS visualiser first), offline analysis
- **Target:** v0.5.x, after the annotation toolset and the run-track finalisation fix
- **Companion plans:** [Background region overlays](lidar-background-region-overlay-plan.md), [Point annotation](lidar-point-annotation-and-object-dataset-plan.md), [Label-aware tuning](lidar-track-labelling-auto-aware-tuning-plan.md), [Track quality score](lidar-visualiser-track-quality-score-plan.md), [Priority review queue](lidar-visualiser-priority-review-queue-plan.md), [macOS local server](macos-local-server-plan.md), [Occupancy column grid](lidar-occupancy-column-grid-plan.md)
- **Canonical:** [track-labelling-ui-implementation.md](../lidar/operations/track-labelling-ui-implementation.md)

## Motivation

Three systems each hold part of what we know about a road user, and none of them talks to the
others.

- **Track labels** in `lidar_run_tracks` drive HINT and the ground-truth evaluator. They are
  keyed by a run's track IDs, so they die with the run, and HINT will not start a sweep until a
  person has labelled 90% of a round's tracks by hand.
- **Annotations** in a pack's sidecar record which returns belong to which physical object. They
  are exact and revision-safe, deliberately know nothing about tracks, and every one of a pack's
  200 samples starts from an empty canvas.
- **Evidence databases** hold immutable, content-addressed observations, state estimates and
  residuals, and a label-free scorecard is computed from them. Nothing a person decides is ever
  attached to them.

Ground truth today is 120 labelled tracks in 9 runs. One 34-minute replay of Columbus and
Broadway produces 4,188 track rows. The reference set cannot grow by hand, and adding a fourth
disconnected system for machine-made labels would make that worse.

This plan replaces the three with one workflow: something **proposes**, a person **grades**, and
the graded result is the only thing called truth. A proposal can come from code or from a
language model. The difference between what was proposed and what the person kept is the
measure of the proposer.

It also has one instrument. **The macOS visualiser is the primary tool for annotating, editing
and grading, and it is sufficient on its own.** An operator chooses a capture, replays it, sees
and edits traces, tracks and point clouds, grades, and runs a tuning round without opening a
browser. The web keeps what it has today, including its track label editing, and may gain views
for finding and identifying datasets. Nothing a grade, a reference label or a HINT round depends
on may exist only on the web.

## Current state

Verified on 2026-09-20 against the annotation branch, and `dd/docs/state-est` for the evidence
databases.

**The sidecar already is a proposal-and-grade model.** `internal/lidar/annotation/sidecar.go`
gives every `Object` and every `FrameMask` a `ReviewStatus` of `proposed`, `reviewed` or
`rejected`, and a `Provenance` whose `Algorithm` and `AlgorithmVersion` are "set only for
proposals". The comment on the status says why: a proposal that could quietly become reviewed
"would let the tracker grade its own homework". `ReviewedMasks` returns only records where both
the object and the mask are reviewed. `TrackCorrespondence` links an object to predicted tracks
and is "never an input to the estimator being judged". No code writes a proposal yet, and the
macOS tool never fills a correspondence.

**An object and its mask are graded separately.** `Object.Status` and `FrameMask.Status` are
independent. A person can confirm what something is without having traced its points. That is
the seam between quick track-level review and careful point-level annotation, and it already
exists.

**HINT waits on labels.** Each round runs a reference run, then sits in `awaiting_labels` until
`MinLabelThreshold` (default 0.9) of its tracks carry a label, then sweeps. `CarryOverLabels`
moves labels between rounds by temporal IoU and marks them `carried_over`. See
[HINT sweep mode](../lidar/operations/hint-sweep-mode.md).

**Label sources already separate people from machines.** `label_source` is `human_manual`,
`carried_over` or `auto_suggested`, and the store's manual predicate counts only an empty source
or `human_manual` as ground truth.

**Evidence identity is content-addressed.** `lidar_observations.observation_id` is a hash of
source, calibration, frame time and cluster. `lidar_track_estimates` and `lidar_track_residuals`
hang from it, carrying estimator and parameter identity, innovations, NIS, and an association
disposition with its reason. An observation keeps up to 1,024 of its cluster's points. The same
capture replayed with different tracker parameters produces the same observation IDs.

**A label-free scorecard exists.** `l8analytics.ComputeTrackScorecard`, run by
`cmd/tools/lidar-track-scorecard`, reads one evidence database and reports lifetimes, how tracks
end, coast error and stratified innovations as canonical JSON with a digest. Its header states
that nothing in it depends on "the random track_id". It reports per run, not per track.

**What the macOS app can and cannot do today.** Its HTTP clients list runs and tracks, create,
update, delete and export labels, load and stop a VRLOG, return to live, and read playback status.
Over gRPC it already pauses, plays, seeks, steps and changes rate, so controlling a replay is not
the gap. Choosing one is: it can stop a PCAP replay and cannot start one. It has no client for
`pcap/files`, `pcap/start`, `capture/*`, replay cases, scenes, missed regions, sweeps, or any of
HINT. Every one of those endpoints exists on the server and is used by the web frontend. So an
operator is sent to the browser for the first step of any session, choosing what to play, and for
two steps that bear directly on validity: marking what the tracker missed, and starting,
continuing and stopping a HINT round. The gap is in the app, not in the server.

**What a recording lacks.** A VRLOG cluster carries a centroid and boxes, not its points. Frames
are usually foreground only, with a background snapshot every 30 s.

## Findings

| Area                                   | Current state                                        | Severity | Release view                           |
| -------------------------------------- | ---------------------------------------------------- | -------- | -------------------------------------- |
| Three label stores, no shared identity | Run-track IDs, pack point indices, observation IDs   | High     | The reason truth cannot accumulate     |
| Proposal model                         | Designed and stored, never written                   | High     | The workflow's spine is already there  |
| HINT's 90% gate                        | Filled entirely by hand, every round                 | High     | Caps how often HINT can be run         |
| Non-LLM evidence per track             | Computed per run only                                | Medium   | Pass 1 needs it per track              |
| Track context in a pack                | Absent by design                                     | Medium   | Right for truth, wrong for proposing   |
| Proposal lineage                       | No link from a reviewed record to what it came from  | Medium   | A proposer cannot be scored without it |
| Choosing what to replay                | Web only: captures, PCAP files, replay cases, scenes | High     | No session can begin in the macOS tool |
| Marking what the tracker missed        | Web only                                             | High     | A validity step outside the instrument |
| HINT control                           | Web only: start, continue, stop                      | High     | A grading loop outside the instrument  |
| Web label editing                      | Creates, reads, updates and deletes run-track labels | Low      | Left exactly as it is                  |

## Design / approach

### macOS is the instrument

One tool owns the loop, end to end:

1. **Choose.** Browse capture roots, PCAP files, replay cases and scenes, and past runs, from the
   server's existing endpoints. The server reads the capture, so the app lists what the server
   can see rather than opening files itself. That keeps the app inside its sandbox, and works
   the same when the server is a Raspberry Pi across the network.
2. **Replay.** Start a PCAP or a multi-file case over a chosen window, or load a run's VRLOG.
   Pause, seek, step and rate are already in the app.
3. **See.** Points, clusters, tracks, trails and boxes, with the layers below toggled at will.
4. **Propose.** Cut a review pack from the run and run pass 1, and pass 2 where a model is
   available, from the app.
5. **Grade and edit.** Accept, edit, reject or defer a proposal; change class and flags; add and
   remove returns; mark a subject nothing proposed.
6. **Score.** See how each proposer fared against what was graded.
7. **Tune.** Start, continue and stop a HINT round, grading its proposals in place.

Three other things exist, and none of them is a second instrument:

- **The Go server** is the backend the app talks to. "No web" means no browser, not no server.
  The [macOS local server](macos-local-server-plan.md) plan removes the terminal as well.
- **The command line** proposes, scores and exports for automation and for machines with no
  display. It never grades; a grade is always a person's.
- **The web** identifies datasets: the scene map, capture inventory, run lists, published
  scenes. Its existing track label create, read, update and delete stays exactly as it is:
  nothing is added to it and nothing is taken away. This workflow builds nothing else there.

The rule that keeps it this way: a feature that creates, edits or grades truth is built in the
macOS tool first and need never be built anywhere else. A web view may show what the macOS tool
wrote. It may not be the only way to write it.

### The model: subject, proposal, grade

**Subject.** A physical road user over a stretch of a capture. This is the sidecar's `Object`.

**Proposal.** A claim about a subject: its class, quality flags, which returns are its in each
sample, and which evidence it corresponds to. Always `proposed`. Always carries the identity of
what made it. Never truth.

**Grade.** A person's act on a proposal, one of four:

| Grade             | Result                                                                      |
| ----------------- | --------------------------------------------------------------------------- |
| Accept            | A reviewed record identical to the proposal                                 |
| Accept with edits | A reviewed record whose class, flags or point membership the person changed |
| Reject            | The proposal marked `rejected` and kept, so it is not offered again         |
| Cannot tell       | Recorded as such; neither truth nor a mark against the proposer             |

Grading an object (what is it?) and grading its masks (which returns?) stay separate, as the
sidecar already has them. A quick pass grades objects and leaves masks `unreviewed`. A careful
pass edits masks. Both are the same workflow at different depth, and the first is what HINT
needs.

Only a graded record is truth. A proposal, however confident and whoever made it, is never
exported as reference, never counted by the ground-truth predicate, and never an input to the
estimator it describes.

### Three anchors, most stable first

A subject is tied to evidence by whichever of these is available, and keeps all it has:

1. **Pack point indices**, bound by the pack digest. Exact, and independent of every algorithm.
2. **Observation IDs.** Content-addressed, so stable across any rerun that leaves L3 and L4
   unchanged. This is what lets a grade survive a tracker change.
3. **Run and track IDs.** Least stable. Needed because HINT and the evaluator read them.

Carrying a grade forward uses the best anchor both sides share: identical observation sets where
only the tracker changed, point overlap where the pack is the same, and temporal IoU only when
nothing better exists.

### One unit of work: the review pack

Extend the annotation pack, rather than add a second container. A review pack is a pack cut
around a short segment, carrying alongside its immutable points:

- **Context layers**, clearly not truth and never digested into the point domain: the tracker's
  tracks and boxes per sample, the observation IDs and retained points that fall in each sample,
  and the background grid state from the [region overlay](lidar-background-region-overlay-plan.md)
- **Proposal layers**, one immutable file per proposer and version under `proposals/`, in the
  sidecar's own schema with every record `proposed`
- **The sidecar**, `annotations.json`, unchanged in role: the person's revisable document, and
  the only place a `reviewed` record lives

The plan's five to eight active segments are five to eight review packs. Every surface opens the
same directory.

Two additive changes to the sidecar schema make grading measurable: a reviewed object or mask
records `derived_from` (the proposal layer, its digest, and the record it came from), and the
grade itself is recorded. A reviewed record with no `derived_from` was made from scratch, which
remains allowed.

### Pass 1: proposals from code, no model, offline

`velocity lidar propose --pack <dir> --evidence <db>` writes `proposals/metric@<version>.json`.
It runs anywhere the binary runs, with no network, deterministically: the same pack and
evidence give the same bytes.

It is the label-free scorecard asked per track instead of per run, plus geometry:

- **Life:** lifetime, path length, net displacement, straightness, fraction coasting, how the
  track ended
- **Motion:** speed percentiles, heading stability, coast error, innovation and NIS consistency
- **Shape:** extent percentiles and their stability, height, points per observation, and the
  edge and plane primitives already fitted to each observation
- **Association:** dispositions and reasons from the residuals; contested and vanished shares
- **Background context:** the region, and the beyond-baseline state, of the cells under the path

From these it proposes a class with a confidence and reason codes (shared with the
[track quality score](lidar-visualiser-track-quality-score-plan.md)), quality flags, a track
correspondence, and a mask per sample. The mask comes from the observation's retained points
where an evidence database exists, and from the returns inside the tracker's box where it does
not; the layer records which.

Rules are written down and versioned. They are expected to be confidently right about most
noise and most cars, and honest about the rest: below a confidence floor it proposes
`cannot tell` rather than guessing.

### Pass 2: a language model, when one is available

`velocity lidar propose --pass llm` writes `proposals/llm-<model>@<rubric>.json`. It is
optional. Nothing downstream requires it, and a site with no model still has pass 1.

It reads a **dossier** rendered from the review pack: per subject, the measurements pass 1
computed, and one image with the whole trail over the region-coloured background plus keyframes
of the subject's returns in top and side view at one fixed metric scale. At a fixed scale a
pedestrian and a car do not look alike. Scaled to fit, they do.

It refines rather than starts again. Its input includes pass 1's proposal and reason codes, and
its output is structured: class, flags, a confidence, a short rationale citing what it saw, and
for each mask a choice among candidates pass 1 prepared (as observed, dilated, connected
component, height-limited) rather than a list of point indices it cannot reliably produce. It
may also propose that two subjects are one, or one is two.

Pass 2 runs against a written, versioned rubric. It is run twice during calibration, with pass
1's class shown and hidden, and the difference is reported as a measure of anchoring.

### Grading, in the macOS tool

| Where             | Depth             | What the person does                                                                                                                                                         |
| ----------------- | ----------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Main visualiser   | Objects           | Grades a subject from its track during replay: accept, reject, cannot tell, change class or flags, without leaving the 3D view                                               |
| Annotation window | Objects and masks | Sees a proposal ghosted over the points; accepts, rejects, changes class, or edits membership with the lasso, rectangle, sphere and column tools, each adding or subtracting |
| Either            | From scratch      | Marks a subject nothing proposed. This is how recall is measured, and it records what was missed and which returns, where a missed region records only where                 |
| HINT panel        | Objects           | `awaiting_labels` presents the round's proposals to grade in the main visualiser, then continues the round                                                                   |

Moving between the two depths is one action: from a track in the main visualiser, open its
subject in the annotation window at the same moment in time, and back.

Layers are toggled independently, each with a legend and a shortcut, off by default and
remembered per user: pass 1 proposals, pass 2 proposals, reviewed truth, tracker context, and
the background regions. The region layer is the same data and the same toggle in the main
visualiser and the annotation window.

An edit is the grade. When a person adds or removes returns from a proposed mask, the reviewed
mask is saved with `derived_from`, and the difference is the proposer's score for that sample.
Nothing extra is asked of the person.

### Scoring the proposers

`velocity lidar review-score --pack <dir>...` compares every proposal layer with the sidecar:

- class agreement and the confusion matrix, per class
- mask agreement as IoU, and edit cost as returns added and removed
- accept, edit, reject and cannot-tell rates
- the same, split by range, by class and by segment

Reported per proposer and version, with confidence intervals, and never pooled across
proposers. This is how a change to pass 1's rules or pass 2's rubric is judged, and it is the
calibration: a proposer's grades are only leaned on, class by class, where its measured
agreement earns it. A random audit sample of accepted proposals is re-graded blind so that
acceptance does not drift into rubber-stamping.

### How the existing systems join

- **Run-track labels.** A graded object projects onto its corresponding run tracks: `reviewed`
  becomes `user_label` with `human_manual` and the grader's identity; `proposed` becomes
  `auto_suggested`. No new label source is needed, and the manual predicate already keeps
  proposals out of ground truth.
- **HINT.** Controlled from the macOS tool through the server's existing HINT endpoints. Pass 1
  runs when a reference run completes. The 90% threshold counts graded subjects only. Carry-over
  between rounds uses observation identity where the round changed only tracker parameters, which
  is the common case, and falls back to temporal IoU otherwise.
- **Evidence databases.** Read-only here. Proposals and grades cite observation IDs; nothing is
  written into an evidence database, which stays immutable.
- **The evaluator.** Reports against human-graded truth, against each proposer, and against
  human-confirmed proposals, separately.
- **The web.** Unchanged. Its track label editing keeps writing `human_manual` run-track labels
  as it does today. Those are ordinary labels: when a review pack is cut from a run, any label
  already on a track arrives as an object-level grade with its labeller's identity, whichever
  tool wrote it. No web code changes for that, and no web feature is added or removed.

## What this cannot establish

Pass 1 and pass 2 propose from the tracks and observations that exist. A road user the pipeline
never detected has no observation and no proposal. **Grading proposals measures precision, class
and membership. It does not measure recall.** Recall comes from a person marking a subject from
scratch in the macOS tool, with no `derived_from`, on the active segments. A from-scratch subject
says what was missed and which returns were its, where the web's missed-region marking, which
stays as it is, says only where. The grading UI keeps a "nothing here was proposed" action one
step away, and the score report counts from-scratch subjects as misses of every proposer.

## Scope

### Item 1: the macOS tool closes the loop

**Summary:** Choose a source and start replaying it from the app, against endpoints that already exist.

**Steps:**

1. HTTP clients for `capture/roots`, `capture/files`, `capture/sessions`, `pcap/files`, replay
   cases and scenes; no server change expected, and any that proves necessary is listed here.
2. A source browser: captures, cases, scenes and runs in one place, with search and the site a
   capture belongs to.
3. Start a PCAP or multi-file case over a chosen window, handing over to the playback controls
   the app already has.
4. Clear states for the failures seen in practice: a recording outside the server's allowed
   directories, a run whose VRLOG is missing, a recording with frames absent.

**Milestone:** v0.5.x, first, because every other item is reached through it

### Item 2: review pack and proposal layers

**Summary:** Context layers, proposal layers and `derived_from`, with the sidecar's guarantees kept.

**Steps:**

1. Context layers in the pack, outside the point digest; export from a VRLOG plus an optional
   evidence database.
2. Proposal layer format: the sidecar schema, all `proposed`, immutable, bound to the pack digest.
3. `derived_from` and the grade on reviewed records, additive, with the Swift and Go stores in
   step and their shared fixture updated.
4. A test that no path turns a proposal into a reviewed record without a grade.

**Milestone:** v0.5.x

### Item 3: pass 1, proposals from code

**Summary:** Deterministic per-track metrics, rules, reason codes and mask proposals.

**Steps:**

1. Per-track form of the scorecard metrics in `l8analytics`.
2. Rules and reason codes, versioned; `cannot tell` below a confidence floor.
3. Mask proposals from retained points, else from the box; candidates prepared for pass 2.
4. Determinism test: two runs, identical bytes.
5. Run from the app as well as the command line.

**Milestone:** v0.5.x

### Item 4: grading and editing in the macOS tool

**Summary:** Ghosted proposals, four grades, edit-as-grade, from-scratch subjects, independent layer toggles.

**Steps:**

1. Layer toggles with legends, including regions, in the main visualiser and annotation window.
2. Accept, edit, reject, cannot tell; class and flag changes; membership edits saved with lineage.
3. Sphere and column selection beside the lasso and rectangle, for selecting and deselecting,
   as specified in the [occupancy column grid](lidar-occupancy-column-grid-plan.md) plan.
4. Object-level grading from a track in the main visualiser.
5. One action between a track in replay and its subject in the annotation window, and back.
6. A from-scratch subject. The web's missed-region marking stays as it is; missed regions already
   recorded for a run are read into its pack as subjects with no mask.

**Milestone:** v0.5.x

### Item 5: scoring and the audit sample

**Summary:** Measure every proposer against graded truth, per class, with intervals, and show it in the app.

**Steps:**

1. `review-score` and its report; a summary view in the app.
2. Blind re-grade sampling.
3. First report on the active segments; decide class by class what pass 1 may be trusted with.

**Milestone:** v0.5.x

### Item 6: HINT from the macOS tool

**Summary:** Start, continue and stop a round in the app; proposals at `awaiting_labels`; carry-over by observation.

**Steps:**

1. HTTP client and a HINT panel over the existing `hint` and sweep endpoints.
2. Pass 1 on reference-run completion; the round's proposals graded in the main visualiser.
3. Projection of graded and proposed objects onto run-track labels.
4. Carry-over by observation identity, temporal IoU as the fallback.

**Milestone:** v0.5.x

### Item 7: pass 2 and the dossier

**Summary:** Fixed-scale dossiers, a versioned rubric, structured output, anchoring measured.

**Steps:**

1. Dossier renderer from a review pack; deterministic.
2. Rubric, reviewed by the person who made the existing labels.
3. Structured output into a proposal layer; shown-and-hidden anchoring run.

**Milestone:** v0.5.x, after item 5 has reported on pass 1

### Item 8: web views for identifying datasets

**Summary:** Optional read-only views of what exists and what has been graded. Not part of any grading or validity path. Existing web functionality, track label editing included, is neither extended nor removed.

**Steps:**

1. Graded coverage per site and segment on the scene map and run lists.

**Milestone:** Unscheduled. Nothing above depends on it.

## Acceptance

With no browser open at any point, an operator using only the macOS tool can:

1. find the Columbus and Broadway capture, and start replaying a chosen five minutes of it;
2. pause on a vehicle, see its track, trail, box and returns, and switch the region layer on and
   off;
3. cut a review pack from that run and run pass 1 on it;
4. accept one proposal, reject one, change the class of one, and add and remove returns from one;
5. mark a pedestrian that nothing proposed;
6. see how pass 1 scored against those grades;
7. start a HINT round, grade its proposals, and continue it to the sweep.

The same walk-through is repeated with the server on another machine.

## Dependencies

- **The annotation toolset** (PR #579): the pack, the sidecar and the macOS window.
- **Existing server endpoints** for captures, PCAP replay, playback, scenes, replay cases and
  HINT. They are already there and already used by the web; item 1 is client work.
- **[macOS local server](macos-local-server-plan.md)**, so that the app starts its own backend.
  Not blocking: the loop closes against a server started by hand.
- **The evidence databases** (PR #559): observation identity, residuals, the scorecard. Pass 1
  degrades to box-derived masks and run-track measurements without them.
- **The run-track finalisation fix:** pass 1's life and motion metrics, and temporal IoU, are
  unsound on first-sighting rows.
- **Region overlays:** for the background context layer. Not blocking.
- **Full-retention recordings** of the active segments, where a mask must include returns the
  foreground filter dropped.

## Risks

- **Rubber-stamping.** Accepting is one keystroke and editing is work. Mitigated by the blind
  audit sample, by reporting the accept rate beside agreement, and by never pre-selecting accept.
- **Truth shaped by the proposer.** A person shown a box tends to keep it. Edits are measured, the
  anchoring run is reported, and from-scratch subjects on the active segments provide a reference
  no proposer touched.
- **Rules fitted to their own grades.** Pass 1's rules and pass 2's rubric are versioned, and
  agreement is always reported against a stated version.
- **A second instrument growing back.** The web already edits labels, and it is the quicker place
  to add a button. It keeps what it has and gains nothing; the rule above and the browser-free
  acceptance test are what hold the line.
- **One platform.** An operator without a Mac cannot grade. Accepted: the command line still
  proposes and scores anywhere, and one good instrument is worth more than two partial ones.
- **Two stores drifting.** Run-track labels can be written by the web as well as projected from
  graded objects. They are reconciled in one direction at one moment: a pack takes the labels a
  run has when it is cut, and a graded object projects back onto its tracks. A label changed on
  the web after a pack was cut is seen the next time one is cut, and the score report lists
  tracks whose label and grade disagree rather than choosing between them.
- **Pack size.** Context layers and several proposal layers enlarge a pack. They sit outside the
  point digest and can be dropped and regenerated.

## Checklist

### Complete

- [x] Sphere and column selection, on the annotation branch in `305bc1bd3`

### Outstanding

- [ ] macOS source browser; start a PCAP or case replay over a window (`M`)
- [ ] Context layers and proposal layer format (`M`)
- [ ] `derived_from` and grade in both sidecar stores, fixture updated (`M`)
- [ ] Per-track scorecard metrics (`M`)
- [ ] Pass 1 rules, reason codes and mask proposals, deterministic, runnable from the app (`L`)
- [ ] macOS layer toggles, ghosted proposals, four grades, edit-as-grade (`L`)
- [ ] From-scratch subjects in the macOS tool; existing missed regions read into packs (`M`)
- [ ] `review-score`, audit sampling, first report, in-app summary (`M`)
- [ ] HINT panel in the macOS tool, label projection, carry-over by observation (`L`)
- [ ] Dossier, rubric, pass 2, anchoring run (`L`)
- [ ] Browser-free acceptance walk-through, local and remote server (`S`)

### Deferred

- [ ] Temporal propagation of a graded mask to later samples, as a third proposer
- [ ] An HTTP or MCP surface over review packs, until review leaves the recording host
- [ ] Web views of graded coverage, optional

### Accepted residuals (no action planned)

- [ ] Recall is measured only on from-scratch subjects in the active segments
- [ ] Rare classes may stay from-scratch and human-only indefinitely
- [ ] Grading requires a Mac
