# LiDAR workflow UI: the web structures the dataset, macOS annotates

The October 2026 workflow design gives every LiDAR screen a place in one chain, from capture to
sweep. This plan keeps its screens and intent and maps each change onto the vocabulary and schema
the repository already has, so the design can be built without reopening settled decisions.

- **Status:** Proposed. The copy, links and lineage it shares with #707 have landed; the rest is
  allocated to v0.5.8–v0.5.11 and v0.6.6 below
- **Layers:** L9 Endpoints, L10 Clients (web and macOS)
- **Target:** v0.5.9 to v0.5.11 for the screens; site and survey work with Item 14 in v0.6.6
- **Companion plans:** [platform vocabulary](platform-vocabulary-and-data-model-plan.md),
  [P1-B LiDAR terminology](platform-vocabulary-phase1-lidar-plan.md),
  [P2-B clips and labels](platform-vocabulary-phase2-clips-labels-plan.md),
  [P2-C jobs](platform-vocabulary-phase2-jobs-plan.md),
  [P2-F sites and surveys](platform-vocabulary-phase2-surveys-plan.md),
  [segment finder](lidar-annotation-segment-finder-plan.md),
  [review workflow](lidar-review-workflow-plan.md),
  [surface consolidation](web-frontend-consolidation-plan.md),
  [run-list labelling rollup](lidar-visualiser-run-list-labelling-rollup-icon-plan.md)
- **Canonical:** [web-frontend-consolidation.md](../ui/web-frontend-consolidation.md)

## Motivation

The design was made in Claude Design, in the project "PCAP to LIDAR sweep pipeline". It has three
parts:

- **LiDAR Workflow Review:** the chain as the screens show it, what breaks it, a proposed
  vocabulary and twelve targeted changes, T1 to T12.
- **LiDAR Workflow Mockups:** six web screens (Captures, Clips, Runs, Windows, Tracks, Sweeps)
  and two macOS windows (the run browser and the Annotation window), with each change marked.
- **An implementation handoff:** data model and API, web screens, and macOS with a PR order and
  acceptance checks.

It was drawn from screenshots of both apps, most taken on 7 October 2026 before #707's later commits.
The prototype loads React and Babel from a CDN, so it is not yet in `docs/ui/design/`; a
standalone export can join the Butterfly Net sketch there.

Its diagnosis holds. One thing had three names (Clip on the web, Case on macOS, `replay-cases` in
the route). "Segment" meant both a motion period and a ranked span of a run. Runs did not know
their clip, sweeps floated free of what they tuned, web Tracks overlapped the macOS editor, and the
handoff to macOS was a sentence of copy.

Taken literally, the handoff adds tables and states that contradict V1, V10, V14, V17, V18, V19
and V33, and duplicates columns that exist. This plan reconciles the two.

## Principle

The design's split restates the [surface rule](web-frontend-consolidation-plan.md#surface-ownership):
fidelity-bound interaction goes to macOS, workflow goes to the Svelte app.

| Surface   | Owns                                                                                                                                       | Never                                                                                                  |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------ |
| **Web**   | Index and probe captures, run motion passes, make clips from periods, launch runs and sweeps, rank candidates and queue packs, show status | Edit frames, labels, annotations or masks                                                              |
| **macOS** | Open a pack or a clip's run, label and annotate, accept, split or merge proposals, review, freeze splits, live and replay inspection       | Keep a second copy of the web's structuring screens; its Choose and Tune steps call the same endpoints |

#707 made web track labels read-only, which this principle requires. The review workflow plan's
earlier rule, that web label editing stays as it is, is amended to match.

## Vocabulary

The design's words, against the nouns the vocabulary plan settled.

| Design word  | Design meaning                                           | Platform noun                                                                                         | Decision                                                                                                                          |
| ------------ | -------------------------------------------------------- | ----------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| Capture      | One pcap file                                            | capture (V1)                                                                                          | Adopt                                                                                                                             |
| Session      | Captures with contiguous packet clocks                   | capture sequence (V1, V7)                                                                             | "Session" stays in copy until P1-B renames the sequence                                                                           |
| Period       | A static or motion stretch from the motion pass          | capture period (V1, V20)                                                                              | Adopt; already the Captures copy                                                                                                  |
| Clip         | One or more periods kept as replay material              | clip (V18): one window, with selection provenance and recommended parameters                          | Adopt; a clip stays one contiguous window ([Clips from periods](#clips-from-periods))                                             |
| Run          | One pipeline pass over a clip with one parameter set     | run (V6)                                                                                              | Adopt                                                                                                                             |
| Window       | A selector-ranked span of a run, and the page listing it | candidate (V18, Item 6)                                                                               | Reject as a page and a noun: ranked spans are candidates on Clips. "Window" stays the plain word, since V18 defines a clip as one |
| Pack         | Windows queued together, with a writable state           | pack (V19): an immutable excerpt identified by its digest; review status lives on annotations (V10)   | Keep the word; derive its status ([Pack status](#pack-status))                                                                    |
| (named pack) | Several windows under one name, "frozen" at the end      | split: a reviewed, object-disjoint set over packs, frozen by `velocity lidar annotation-split freeze` | Call it a split                                                                                                                   |
| Sweep        | A parameter search scored against a clip's labels        | sweep (V6)                                                                                            | Adopt; name the objective, since only `ground_truth` scores against labels                                                        |
| Labels       | Points agreed and objects reviewed                       | label is a track verdict; annotation is a point mask (V10)                                            | "Labels" for track verdicts, "Annotations" for mask review                                                                        |
| p·7f3a       | A four-digit parameter hash                              | the param set's `params_hash`; the tuning config's fingerprint (V33)                                  | Show the existing digest at eight hex digits; four would collide within one sweep                                                 |
| Site         | A picker on sessions and clips, from radar Sites         | site (V14), deployment (V17); a clip's coordinates move to its survey (V18)                           | Clips pick from `lidar_sites` now; sessions take a site from deployments in v0.6.6                                                |

## The twelve changes

Each change, what this plan does with it, and where it lands. "Landed" means #707.

| #   | Design change                                                          | Disposition                                                                                                                                                      | Lands                          |
| --- | ---------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------ |
| T1  | Rename Segments to Windows; nav Captures, Clips, Runs, Windows, Sweeps | Adapted. Nav order and LiDAR group landed. Segments folds into Clips as the candidates panel instead of being renamed                                            | Item 6, v0.5.9                 |
| T2  | Clip everywhere; `/lidar/clips` with a redirect                        | Adopted. Web copy landed; the macOS column reads Source until runs record their clip                                                                             | Item 2, v0.5.8                 |
| T3  | Keep "periods"; drop "segment" at the capture stage                    | Adopted. The copy already says periods; types follow P1-B                                                                                                        | Item 3, v0.5.8                 |
| T4  | Every run carries a clip; parameter hash and diff; fold repeats        | Adapted. Clip match, digest and the recommended mark landed. Runs record their clip at write, group by clip, and fold a sweep's runs under the sweep             | v0.5.9                         |
| T5  | Sweeps show clip, base run, best score; queue; resumable               | Adapted. Clip and objective landed. Best score on the card; "base run" only for HINT, which has a reference run; queueing through jobs; resumable is `suspended` | v0.5.11                        |
| T6  | Tracks' clip picker defaults to the run's clip                         | Landed as specified. Target: the clip as a link, with a picker over that clip's runs                                                                             | v0.5.9                         |
| T7  | Pick the site from Sites instead of free text                          | Adapted. Clips pick from `lidar_sites` now; a session's site comes from its deployment                                                                           | v0.5.10; v0.6.6                |
| T8  | Packs get a visible state; counts on Clips                             | Adapted. The Segments page already derives the status; it moves to the candidates panel and Clips counts it. No writable state                                   | Item 6, v0.5.9; counts v0.5.10 |
| T9  | Web Tracks becomes a preview with "Open in visualiser"                 | Adopted. Read-only playback landed; the deep link needs a macOS URL scheme                                                                                       | v0.5.10                        |
| T10 | macOS run browser by clip, "Queued for me", clip in the window title   | Adapted. Group by clip and title by clip; "Awaiting review" from pack status instead of an assignee                                                              | v0.5.10                        |
| T11 | "Make clip" from selected periods, description prefilled               | Adapted. One clip per selected static period, or per adjacent run with no motion inside                                                                          | v0.5.9                         |
| T12 | Clip panel read-outs instead of a captures textarea and params JSON    | Adopted. Periods drawn on the session strip by time overlap; recommended parameters read-only, naming the sweep that set them                                    | v0.5.9                         |

## Design notes

### Clips from periods

A clip is one window over ordered captures: a start offset and a duration. Three static periods
with motion between them cannot be one clip without replaying the motion. The mockup's "27:45
static across 3 periods" replays 28:04, 19 s of it with the sensor moving, which is what the
motion pass exists to exclude. Selecting several periods therefore makes one clip per period, or
one per run of adjacent static periods; the composer says how many clips it will make. A clip
that spans several windows is a schema change for P2-B, not this plan.

A clip keeps its window, not period names. `static-0` is an ordinal: a fresh motion pass renumbers
periods, and a session a rescan changes loses its periods. The Clips panel draws the clip's
periods on the session strip by time overlap, so it survives either.

### Pack status

The status is derived on read from what already exists, and nothing writes it:

| Status    | Shown as  | Source today                                                     |
| --------- | --------- | ---------------------------------------------------------------- |
| candidate | candidate | a ranked span not yet chosen                                     |
| case      | clip made | a clip made from it                                              |
| clipping  | packing   | its pack job queued or running                                   |
| packed    | packed    | the pack's manifest on disk                                      |
| proposed  | proposed  | proposal layers in the pack                                      |
| reviewed  | reviewed  | at least one reviewed object with a reviewed mask in the sidecar |
| frozen    | frozen    | planned: the pack is in a frozen split                           |

The first six are what the Segments page shows today. The design separates "labelling" from
"reviewed", and so should the status: "reviewed" is set at the first reviewed mask, so it reads as
finished when review has only started. It becomes "in review" while some objects remain proposed,
and "reviewed" when every object in the sidecar is. macOS moves a pack along by reviewing
annotations and freezing splits, which it already does; nothing needs a `PATCH` on a pack.

### Status colours

The design colours in-progress states sky blue. [DESIGN.md §3.2](../ui/DESIGN.md) makes
in-progress amber on every surface, success green, failure red and inactive grey; the chips follow
it.

### Open in visualiser

The web Tracks page links to the same run and frame in the macOS app through a URL scheme the app
registers. The [segment finder](lidar-annotation-segment-finder-plan.md#api-and-svelte-surface)
already plans an "Open in macOS" link naming a pack; both use the one scheme, with a run form and
a pack form.

## Data the screens need

| Need                            | Exists                                                                   | Gap                                                                       |
| ------------------------------- | ------------------------------------------------------------------------ | ------------------------------------------------------------------------- |
| A run's clip                    | `lidar_run_records.replay_case_id`; clip replays set it                  | The sweep runner does not; the web matches capture paths meanwhile (#707) |
| A run's parameters              | `requested_param_set_id`, `param_set_id`; `lidar_param_sets.params_hash` | None                                                                      |
| Difference from the recommended | both param sets' `params_json`                                           | Computed on read                                                          |
| Which sweep recommended them    | `recommended_param_set_id` on the clip                                   | `recommended_by_sweep_id` and `recommended_at`, additive                  |
| A run's labelling share         | `label_rollup` on the runs list                                          | None; landed on Runs                                                      |
| Pack status and counts per clip | segment status, pack sidecars, `lidar_segment_clip_jobs`                 | Counts grouped by clip, on read                                           |
| A sweep's clip and objective    | the stored request, `objective_name`                                     | None; landed on Sweeps                                                    |
| A sweep's best score            | `results`, `recommendation`                                              | Read into the list summary                                                |
| Queued sweeps                   | the job queue                                                            | Sweeps become a job kind with Item 11                                     |
| Resuming after a restart        | `suspended` with checkpoint columns, for auto-tune                       | Restart recovery fails them instead (backlog, v0.5.11)                    |
| A session's site                | none; a free-text label                                                  | Deployments, Item 14                                                      |
| Opening macOS from the web      | none                                                                     | A URL scheme and its handler                                              |
| "Queued for me"                 | none; an operator name in the macOS session                              | Not planned: there is one operator                                        |

## Not adopted

- **A `packs` table with writable states, and `PATCH` from macOS.** A pack is immutable (V19)
  and review status is on annotations (V10); the derived status above already covers it.
- **"Freeze pack".** The macOS "Freeze Split…" freezes a split over several packs. Renaming it
  would describe a different act.
- **`sessions.site_id` and `clips.site_id` against the radar `site` table.** One sites table is
  Item 14 (V14); a sensor at a site over time is a deployment (V17); a clip's coordinates move to
  its survey (V18).
- **New `runs.clip_id` and `runs.params_hash` columns.** Both exist, as `replay_case_id` (renamed
  by Item 2) and the param set's digest.
- **`clips.periods` by period name.** See [Clips from periods](#clips-from-periods).
- **A Windows page.** See T1.
- **A sweep scheduler of its own.** Item 11 makes the job queue the one queue.
- **A `resumable` status.** `suspended` already means it.

## Delivery

Each line is one backlog item, linked to this plan unless it is a vocabulary item.

| Release | Item                                                                                               | Changes |
| ------- | -------------------------------------------------------------------------------------------------- | ------- |
| v0.5.8  | Vocabulary Item 2: routes, `/lidar/clips`, the macOS column                                        | T2      |
| v0.5.9  | Sweep runs record their clip                                                                       | T4      |
| v0.5.9  | Runs grouped by clip, a sweep's runs folded under it, parameters compared with the recommended     | T4      |
| v0.5.9  | Captures makes clips from selected periods                                                         | T3, T11 |
| v0.5.9  | The Clips panel reads out periods and recommended parameters                                       | T12     |
| v0.5.9  | Tracks names the run's clip and offers that clip's runs                                            | T6      |
| v0.5.9  | Vocabulary Item 6: the candidates panel on Clips, with the derived status                          | T1, T8  |
| v0.5.10 | Pack status counts on Clips and "in review"                                                        | T8      |
| v0.5.10 | Clips pick their site from `lidar_sites`                                                           | T7      |
| v0.5.10 | macOS run browser grouped by clip; clip and pack in the Annotation window title; "Awaiting review" | T10     |
| v0.5.10 | "Open in visualiser": the macOS URL scheme and the web Tracks link                                 | T9      |
| v0.5.11 | Sweep cards show the best score; HINT names its reference run                                      | T5      |
| v0.5.11 | Checkpointed sweeps survive a restart                                                              | T5      |
| v0.5.11 | A sweep requested while one runs is queued as a job                                                | T5      |
| v0.6.6  | Vocabulary Item 14: a session's site from its deployment                                           | T7      |

## Acceptance

- [ ] Every run started from a clip records it, the sweep runner's included; the Runs page names a
      clip for every run whose capture a clip covers.
- [ ] Making clips from selected periods never puts a motion period inside a clip.
- [ ] The clip panel has no captures textarea or parameters JSON; recommended parameters name the
      sweep that set them.
- [ ] A pack's status on the web matches what macOS shows, derived from the same sidecars; the web
      writes no pack, label or annotation state.
- [ ] After Item 6 there is no Segments page; candidates appear on Clips.
- [ ] A sweep requested while one runs is queued, not refused; a restart suspends a checkpointed
      auto-tune rather than failing it.
- [ ] "Open in visualiser" on web Tracks opens the macOS app at the same run and frame.
- [ ] Status chips use the §3.2 colours on both surfaces.

## Open questions

1. **Several windows per clip.** Is a clip of several static periods worth the schema change in
   P2-B, or are one-per-period clips enough for replay and annotation?
2. **The URL scheme's name.** The mockup shows `velocity.report://`, the handoff
   `velocityreport://`. Both are valid; pick one before the macOS item.
3. **The prototype in the docs.** Export a standalone copy from Claude Design and add it to
   `docs/ui/design/` beside the Butterfly Net sketch, or keep it in the design project only.
