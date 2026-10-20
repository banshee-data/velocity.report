# Labelling runs: guidance in the window, progress against the step, and time measured from saves

A labelling spec (label0001, label0002) estimates operator hours that no one has measured, and the
operator follows its steps from prose spread across three guides. This plan makes the macOS
annotation window say, at every moment, which step of the run it is on, what closes that step,
how far through it the pack is, and where in the window to go next, using controls the window
already has rather than a tutorial or an overlay. It measures active time from the saves the
operator already makes, so the next review's person-hour estimates are measured, not guessed.

- **Status:** Parked (D-32, October 20, 2026): its rows sit in v0.5.14 (facets) or v0.5.15 (label sets), or it has none; revisit when that slot is next, or a held-out window needs it. Was: Proposed, 2026-10-18
- **Layers:** L10 Clients (macOS annotation window), annotation storage, experiment queue
- **Target:** v0.5.2, Sprint 0.5.2.0, before label0001's operator pass
- **Companion plans:** [fit to reviewed points](lidar-physical-fit-to-points-plan.md),
  [label-set team workflow](lidar-label-set-team-workflow-plan.md),
  [physical authoring comprehension](lidar-physical-authoring-comprehension-design.md),
  [experiment queue](platform-experiment-queue-plan.md)
- **Canonical:** [point annotation tool](../lidar/operations/point-annotation-tool.md)
- **Related:** [experiment review 2026-10-18](../lidar/operations/experiment-review-2026-10-18.md#labelling-and-annotation-priorities),
  [label sets](../lidar/operations/label-sets.md)

## 1. Why

- label0001 (8h15m) and label0002 (5h15m) are the week's critical path, and both estimates are
  uncalibrated: the kirk0 pilot's hours were not recorded.
- An operator resumes by memory. Nothing in the window says which step a run is on, what closes
  it, or how much remains.
- `manual:*` units close only by hand on the command line, so the queue does not know a step is
  done.
- The first physical pilot stalled on silent required fields (PA-04, PA-06 in the
  [comprehension design](lidar-physical-authoring-comprehension-design.md#issue-list-for-after-the-current-annotation-run)),
  and lost a pose to a tracker-assisted declaration nobody could see (PA-11). Both are guidance
  failures, not missing features.

## 2. What the window already has

Measured on `main` at `0504248c`, so the plan extends these rather than building beside them.

| Surface                                                  | What it does today                                                                                                                  | What it lacks for a run                                                                                     |
| -------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------- |
| `AnnotationStatusStrip` and `AnnotationSession.nextStep` | One line, always on screen: an error, else the next thing that blocks a save (enter your name, select points, choose a class, save) | It answers "what blocks this save", never "what step of the run is this and what closes it"                 |
| `AnnotationProgressHeader`                               | Two bars: this frame's and the pack's labelled points, agreed against in question                                                   | Points are the wrong unit for review, bodies or following records; the spec's gates count objects and pairs |
| `AnnotationFrameStrip`                                   | Per-frame agreed and in-question colouring across the pack                                                                          | One colouring for every mode; Physical has no "frames with a reviewed pose" view                            |
| `AnnotationWorkMode` picker                              | Object Points, Physical, Facets, Compare, as a menu                                                                                 | No count of open work per mode, so switching mode is a guess                                                |
| Checks panel (`LabelChecks`)                             | Warnings that never block a save                                                                                                    | No order by what blocks the current step's gate                                                             |
| `FreezeSplitSheet`                                       | Previews what a freeze pins, including review coverage, then freezes                                                                | Readiness is visible only after opening the sheet with a draft                                              |
| Label-set journal                                        | One line per save with `change.created_utc`, `author`, `session` and `operation` (`internal/lidar/annotation/labelset/journal.go`)  | Nothing reads it for time                                                                                   |
| Physical references                                      | Each revision carries `updated_utc` and a change record (`physical_store.go`)                                                       | Same                                                                                                        |

## 3. Review of the first draft

The first draft (R1 to R8, below as revised) had the right goal. What changed, and why:

1. **Open question 1 is answered: every save is already timed.** Journal lines carry
   `created_utc`, and physical-reference revisions carry `updated_utc`. R2 needs no format change
   and shrinks from `M` to `S`, and it can run on kirk0's 2,231-revision journal today. Two
   cautions belong in its method: a propagation writes many masks in one save, so attribution
   reads the delta's operation and size, not one item per save; and the `session` field already
   separates sittings, which is a better boundary than the idle cap alone.
2. **One guidance line, not a second panel.** The window already spends its top strip on "what to
   do next". A separate run panel beside it would give the operator two places to look and two
   answers that can disagree. The run's step belongs in the same strip, one level up from
   `nextStep`: the strip says the step and its progress, `nextStep` still says what blocks the
   save, and a single button takes the operator to the next item. A full checklist is a popover
   from that line, not a column.
3. **Progress counts the gate's units.** label0001's review gate is `reviewed_fraction: 1.0` over
   proposals, not a share of points. The header's point bars stay for labelling; each step shows
   the count its gate reads (objects reviewed of total, vehicles with a reviewed body, follower
   intervals with a record).
4. **The mapping from step to window is code, not prose.** Per-spec runbooks (R1) would repeat
   the same four steps for label0001 and label0002 and drift from the window. The mapping from a
   step kind to a work mode, a queue and a "done when" check belongs in one Go registry beside
   the experiment kinds, read by the CLI and served to the window; the guide gets one table
   generated from it. R1 becomes that table plus the pilot's traps.
5. **The manifest is generated on demand (open question 3).** The spec is the source; a
   committed manifest is a derived copy that can disagree with it. The window asks the server for
   the manifest of the spec its pack serves.
6. **The pack must know its run.** Nothing links a pack to a spec today. The clip's
   `segment.json` already records the pack's role and why it was chosen; it gains the spec slug
   and step when a clip is cut for a labelling spec, and an existing pack can be linked once by
   command. Without this, the window cannot show a run at all.
7. **Closing a step from the window is a server endpoint (R6).** The window reaches the
   experiment state directory only through the server, so "Mark step done" needs an endpoint
   that runs the CLI's own check. That is `M`, not `S`.
8. **Active time is about a person.** It measures the operator, not road users, but it is still
   data about a named person. It stays in the local state directory, is never published with a
   label set, and reviews report per-item rates, not per-person timelines.
9. **Order.** R3 (progress counts) first, because it needs no new state and every later item
   reads it. R2 alongside it, since it is a read of existing files. Then the guidance line and
   queue (G1, G2), which is where the operator's time is saved. F5's shared-sample suggestions
   move ahead of R7: label0002's following records need the shared samples more than they need a
   new editor, and the CLI path for drafting works.

## 4. Guidance without a tutorial

The principle: the window derives what to do next from the pack's state and the run's gate, and
shows it where the operator already looks, with the control that does it. It stores no wizard
position, so work done out of order, by two people, or after a restart is read correctly, and an
operator who ignores the guidance loses nothing.

| Item | What                                                                                                                                                                                                                                                                                                                                                                                     | Size |
| ---- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---- |
| G1   | **The step in the status strip.** One line above today's `nextStep`: "Step 2 of 4 · Review · 31 of 67 objects reviewed · Next: Car 12 at sample 412", with a **Go** button that switches mode, selects the object and seeks to the sample. When the step's gate is met the line says so and names the next step and its mode. `nextStep` keeps its place as what blocks the current save | M    |
| G2   | **Next and previous in the step's queue.** `n` and Shift-`n` (free today; the window binds `[`, `]`, `s` and their Command forms) move through the current step's open items: the next proposed object, the next vehicle without a reviewed body, the next follower interval without a record, the next frame with an unlabelled sector. The queue is R4's                               | S    |
| G3   | **Open work on the mode picker.** Each mode's menu item carries its count: "Object Points · 36 to review", "Physical · 12 bodies to fit", "Facets", "Compare". A mode with nothing open for the run says so                                                                                                                                                                              | S    |
| G4   | **The object list sorted by what needs attention.** A status glyph per object (proposed, masks reviewed but object not, body missing, review drifted after an edit), and a sort that puts the current step's open items first                                                                                                                                                            | S    |
| G5   | **The frame strip in the mode's terms.** In Physical, frames shaded by reviewed poses; when drafting following records, the samples where both parties have a bumper-placing reviewed pose, which are F5's shared samples                                                                                                                                                                | S    |
| G6   | **Every unavailable control says why.** Each disabled or inert control names its precondition in place, through the existing `.inert(_:hint:)`, never a silent grey. An audit of the Physical pane against PA-04 and PA-06 first, then the rest                                                                                                                                          | S    |
| G7   | **Resume where the last save was.** On opening a pack, the strip offers "Resume at Truck 1, sample 495", read from the journal's and the physical store's last change by this author                                                                                                                                                                                                     | S    |
| G8   | **Assisted state always visible.** While the estimate disclosure is non-empty, a badge on the strip says that edits are tracker-assisted, with the control to clear it beside it (PA-11's pilot trap)                                                                                                                                                                                    | S    |
| G9   | **Freeze readiness in the strip.** The freeze preview's coverage computed continuously for the run's split: "Freeze: blocked by 4 unreviewed objects", with Go taking the operator to the first of them                                                                                                                                                                                  | S    |
| G10  | **Checks ordered by the step.** Warnings that block the current gate (an unreviewed mask before a freeze, a body fitted over proposed masks) above those that never block (tiny masks)                                                                                                                                                                                                   | S    |

What is deliberately left out: coach marks, a first-run tour, modal step dialogs, and any
guidance that hides a control until a step is reached. Each costs an experienced operator time
on every pass to save a new one time once.

## 5. Work

The first draft's items, revised by Section 3.

| Item | What                                                                                                                                                                                                                                                                                                 | Size |
| ---- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---- |
| R1   | **Step registry and guide table.** A Go registry mapping each labelling step kind (`manual:capture`, `manual:annotate`, `manual:review`, `manual:freeze`) and its spec note to a work mode, a queue, a gate check and the pilot's traps; one generated table in the point annotation guide           | S    |
| R2   | **Active time from saves.** `velocity lidar labels time` reads journal `created_utc` and physical-reference `updated_utc`, splits at `session` changes, sums gaps capped at an idle limit (5 min, with the gap distribution printed), and attributes time to steps by each save's operation and size | S    |
| R3   | **Progress counts from pack state**, in Go and the window: objects accepted of proposed, objects and masks reviewed of total, vehicles with a reviewed body, follower intervals with a record, each as its gate counts it                                                                            | M    |
| R4   | **The step's queue**, in Go and the window: the ordered open items G1, G2 and G4 read; it shares F5's suggestions for following pairs                                                                                                                                                                | M    |
| R5   | **Run link and manifest.** `segment.json` gains the spec slug and step for packs cut for a labelling spec, and `labels link-run` sets them on an existing pack; the server serves the manifest of that spec, generated from it, with steps, gates and estimates                                      | M    |
| R6   | **Close a step from the window.** A server endpoint runs the CLI's check and records the artefact as `experiments mark … done -artefact`; the window's button is enabled only when the check would pass, and says why when not                                                                       | M    |
| R7   | **Following records authored in the window**, replacing the `draft-following` round trip, after F5                                                                                                                                                                                                   | L    |
| R8   | **Rates fed back.** An answered labelling spec's `actual` is its measured active time; per-item rates recorded in the review that ran it                                                                                                                                                             | S    |

## 6. Order

1. R3 and R2: no UI, no new state. R2 runs on kirk0's journal now and gives the first measured
   rates before label0001 starts.
2. G1, G2 with R4, then G6 and G8: the guidance line, the queue keys, every disabled control
   explaining itself, and the assisted badge. This is the slice that saves operator time on
   label0001.
3. R1 and R5: the registry and the run link, so the window knows its spec.
4. G3, G4, G5, G7, G9, G10: each small, in any order.
5. R6, then F5 (the fit plan's), then R7.
6. R8 after the first labelling run R2 timed.

## 7. Rules

- Guidance is derived from the pack's state and the spec's gates, never from a stored step
  position.
- A gate shown in the window is the same check the CLI runs; the window never closes a step the
  CLI would refuse.
- Progress counts read the pack's state, so they hold whoever did the work and survive restarts.
- Time and progress are runtime data: session records live in the experiment state directory,
  never in Git, and never in a published label set. A registry entry, a guide table and a spec
  are hand-authored and belong in Git.
- Active time measures the operator's working pattern, not anything about road users; reviews
  report per-item rates, not a person's timeline.
- No control is hidden to enforce an order; an unavailable control says what it waits for.

## 8. Open questions

1. The idle cap: 5 minutes as the default with the gap distribution printed, or a cap per step
   kind chosen from the first timed run.
2. Whether a pack serves one step or several: label0001's pack serves select, propose, review
   and freeze; label0002 reuses the same pack for fit, following and freeze. One pack, many
   steps, appears to be the case, which the run link must allow.
3. Whether the queue keys move between objects or between items within an object (a body's poses
   at shared samples), or both with a modifier.

## Checklist

- [ ] R3 progress counts
- [ ] R2 `labels time` on kirk0's journal
- [ ] G1 the step in the status strip, with Go
- [ ] G2 and R4 the step's queue and its keys
- [ ] G6 every unavailable control says why
- [ ] G8 assisted state always visible
- [ ] R1 step registry and guide table
- [ ] R5 run link and manifest
- [ ] G3, G4, G5, G7, G9, G10
- [ ] R6 close a step from the window
- [ ] R7 following records in the window (after F5)
- [ ] R8 rates fed back into specs
