# LiDAR workflow UI: the November audit and the four later changes

An audit of both apps against the October workflow design, taken from twelve screens captured on
November 8, 2026, and a plan for the four changes the design gained after the
[UI workflow plan](lidar-ui-workflow-plan.md) was written: a shared view record, Tracks as one
viewer, a city task feed, and held-out review. The twelve original changes keep their homes in
that plan and the backlog; this plan records what the audit found against them and where the
handoff's proposals differ from settled decisions.

- **Status:** Draft (D-32: not counted as active; the owner places it in a release slot). Written
  November 8, 2026 from a Claude Design handoff and the screen audit; nothing built
- **Decisions:** D-31 (renames are cutovers), D-32 (planning limits), D-37 (backlog format),
  V10, V18 and V19 in the [vocabulary plan](platform-vocabulary-and-data-model-plan.md) (a label
  is a track verdict, a clip is one window, a pack is immutable)
- **Layers:** L9 Endpoints, L10 Clients (web and macOS)
- **Target:** unscheduled. The handoff named the work "0.5.2"; that slot is the unattended
  experiment harness, and the twelve changes already sit in v0.5.9 to v0.5.12 and v0.7.4
- **Companion plans:** [UI workflow](lidar-ui-workflow-plan.md) (the twelve changes and their
  dispositions), [city view](lidar-city-view-plan.md) (the city map both clients draw),
  [lane review across the city](lidar-lane-review-citywide-plan.md) (review state on street
  segments), [segment finder](lidar-annotation-segment-finder-plan.md) (candidates, packs and the
  "Open in macOS" link), [review workflow](lidar-review-workflow-plan.md) (proposals and grading;
  parked), [label set team workflow](lidar-label-set-team-workflow-plan.md) (frozen splits),
  [P2-B clips and labels](platform-vocabulary-phase2-clips-labels-plan.md),
  [P2-C jobs](platform-vocabulary-phase2-jobs-plan.md)
- **Canonical:** [web-frontend-consolidation.md](../ui/web-frontend-consolidation.md)

## Motivation

The [UI workflow plan](lidar-ui-workflow-plan.md) reconciled the October design's twelve changes,
T1 to T12, with the vocabulary the repository had settled, and placed each in a release. Since
then the design gained four more changes, T13 to T16, and a Claude Design session produced an
implementation handoff: a data model, web screens, a macOS sheet, a PR order and acceptance checks,
then a gap list from the November 8 screens and six implementation plans of forty items.

The handoff is useful in two ways and wrong in one. Its screen audit is the first measurement of
how far the apps have moved since #707, and its four later changes fill real holes: the web and
macOS cannot show each other the same picture, the operator has no list of what is left to do
across sites, and a held-out window has no path from "drawn" to "frozen" that the web can see. But
it restates the data model the UI workflow plan already declined: a writable `packs` table with
`PATCH` from macOS, new `runs.clip_id` and `runs.params_hash` columns, `sessions.site_id` against
the radar `site` table, a Windows page, a sweep scheduler of its own and a `resumable` status.
Merged as written it would reopen those decisions in five files under a release name the backlog
uses for something else.

This plan keeps the audit and the four changes, maps every handoff item to the decision that
already covers it or to a new item here, and says which of it the next implementation PRs should
take first.

## Current state

### The November 8 audit

Twelve screens: the visualiser's City view and Analysis Runs sheet, and the web Status, Captures,
Clips, Site viewer, Segments, Labelling, Runs, Tracks, Sweeps and Regions pages. Status is what
the screen showed; "new" marks a gap the twelve changes never covered. "Owner" names the layer
that closes it. The last column names the item or disposition that already covers the gap.

#### Vocabulary and routes

| #   | Gap                                                                                 | Owner      | Status      | Evidence                                                                           | Covered by                                                             |
| --- | ----------------------------------------------------------------------------------- | ---------- | ----------- | ---------------------------------------------------------------------------------- | ---------------------------------------------------------------------- |
| G1  | The page is still Segments at `/app/lidar/segments`, listed above Runs              | web        | not started | nav order Status, Captures, Clips, Site viewer, Segments, Labelling, Runs          | Vocabulary Item 6 folds Segments into Clips (v0.5.10); no Windows page |
| G2  | Route and column renamed to Clip                                                    | web, macOS | done        | `/app/lidar/clips`; the visualiser's sheet has a Clip column                       | T2, landed                                                             |
| G3  | "location" where the design says "site"                                             | web        | partial     | Captures filter "needs a location (51)"; Clips "Registered: unknown / not located" | T7 (v0.5.11); a session's site waits on deployments, Item 14 (v0.7.4)  |
| G4  | Five pages the design did not place: Status, Site viewer, Labelling, Regions, Debug | web        | new         | the design proposes a Sensor group for Site viewer, Regions, Debug and Settings    | Item 3 below                                                           |

#### Lineage: runs know their clip, sweeps know their run

| #   | Gap                                                                           | Owner         | Status      | Evidence                                                                | Covered by                                                                   |
| --- | ----------------------------------------------------------------------------- | ------------- | ----------- | ----------------------------------------------------------------------- | ---------------------------------------------------------------------------- |
| G5  | A run's clip shown on the web Runs page                                       | server        | done        | Runs shows "S2 archive: Ashbury at Downey" per row                      | T4, landed (matched by capture path meanwhile)                               |
| G6  | A run's clip not reaching the visualiser                                      | macOS, server | not started | Analysis Runs: Clip "-" on all 40 rows                                  | "Sweep runs record their clip" (v0.5.10); then the sheet reads it            |
| G7  | Runs not grouped by clip; repeats not folded                                  | web           | not started | 5 rows "Ashbury at Downey", 3 of them 362/362 tracks, same digest       | "Runs grouped by clip" (v0.5.10)                                             |
| G8  | Parameters column is a bare digest, no difference from the clip's recommended | web, server   | not started | `c927ea3a` on every row; no recommended digest anywhere                 | same item; the digest is `params_hash`, shown at eight hex digits            |
| G9  | Recommended parameters not surfaced; tuning JSON still hand-edited on Status  | web           | not started | Status "Tuning parameters: edit and apply"; no clip panel               | "Clip panel read-outs" (P2); Status stays the server defaults editor         |
| G10 | Sweeps carry no clip, base run, progress or best score                        | server, web   | not started | cards show id, type, capture, objective, error                          | "Sweep cards show the best score" (v0.5.12)                                  |
| G11 | A second sweep fails instead of queueing; a restart fails the running one     | server        | not started | "sweep already in progress" twice, "incomplete: server restarted" twice | "Checkpointed sweeps survive a restart" and "Sweeps queue as jobs" (v0.5.12) |
| G12 | Tracks' clip picker defaults to "None (Historical)"                           | web           | not started | header picker; 0 tracks visible                                         | "Tracks names the run's clip" (P2)                                           |

#### Packs and the web to macOS handoff

| #   | Gap                                                                          | Owner              | Status      | Evidence                                                            | Covered by                                                               |
| --- | ---------------------------------------------------------------------------- | ------------------ | ----------- | ------------------------------------------------------------------- | ------------------------------------------------------------------------ |
| G13 | Pack states beyond `candidate`                                               | server, web, macOS | partial     | Segments: every row "candidate"; no queue bar; no pack name         | Pack status is derived, not written: "Pack status on Clips" (v0.5.11)    |
| G14 | A Packs column on Clips; pack counts on the visualiser's groups              | web, macOS         | not started | Clips columns: Description, PCAP, Files, Registered, Lanes, Created | same item                                                                |
| G15 | Visualiser sheet grouped by clip, "Awaiting review", pack chip in Labels     | macOS              | not started | flat list by date; footer "Select a run to load for labelling"      | "macOS run browser grouped by clip" (v0.5.11); no assignee, one operator |
| G16 | Annotation window titled by clip, pack and window                            | macOS              | not started | not visible in this capture; assumed unchanged                      | same item                                                                |
| G17 | Segments offers "Make clip" per row, against clips from periods (T11)        | web                | partial     | Segments actions: Details, Preview in Tracks, Make clip, Hide       | Open question 3                                                          |
| G18 | Clips still offers "New Clip" and "Scan PCAP Folder"; no "Make from periods" | web                | not started | header buttons                                                      | "Captures makes clips from selected periods" (v0.5.10)                   |

#### Captures and Clips structuring

| #   | Gap                                                               | Owner       | Status      | Evidence                                                     | Covered by                                                                 |
| --- | ----------------------------------------------------------------- | ----------- | ----------- | ------------------------------------------------------------ | -------------------------------------------------------------------------- |
| G19 | A site picked from Sites on the session row                       | server, web | partial     | the filter exists; no binding to a sites table               | T7: clips from `lidar_sites` (v0.5.11); sessions from deployments (v0.7.4) |
| G20 | Period chips toggle into a "Make clip from N periods" composer    | web         | unknown     | session detail was below the fold in the capture             | "Captures makes clips from selected periods" (v0.5.10)                     |
| G21 | Clip side panel: period strip, read-only parameters, lineage      | web         | not started | flat table, Run lanes and Delete                             | "Clip panel read-outs" (P2)                                                |
| G22 | Lanes and Registered columns on Clips have no place in the design | web         | new         | lane proposals are keyed by clip here, by site in the design | Not adopted: the lane plan keys them by street segment inside S2           |

#### Tracks and the shared view (T13, T14)

| #   | Gap                                                                              | Owner      | Status | Evidence                                               | Covered by                                                          |
| --- | -------------------------------------------------------------------------------- | ---------- | ------ | ------------------------------------------------------ | ------------------------------------------------------------------- |
| G23 | No view record (run, time, camera, layers, filters, selection)                   | server     | new    |                                                        | Item 1                                                              |
| G24 | Tracks has no run context in its header                                          | web        | new    | title "LiDAR Tracks · 0 tracks visible"                | Item 2                                                              |
| G25 | Two play bars with duplicated speed controls                                     | web        | new    | "Play 1x 0:00/0:00" and "Play Speed 0.5x 1x 2x 5x 10x" | Item 2                                                              |
| G26 | A filter panel (Class, State, Min observations) where a selected track should be | web        | new    | the right panel                                        | Item 2                                                              |
| G27 | No window or pack marks on the timeline; no keyboard                             | web        | new    |                                                        | Item 2                                                              |
| G28 | No "Open in visualiser" or "Copy view link"; no "Share view" in macOS            | web, macOS | new    |                                                        | Item 1, with "Open in visualiser" (v0.5.11)                         |
| G29 | The macOS URL scheme is not registered                                           | macOS      | new    | no `CFBundleURLTypes` in the app                       | "Open in visualiser" (v0.5.11); scheme decided: `velocityreport://` |

#### City navigator (T15)

| #   | Gap                                                                   | Owner  | Status | Evidence                                         | Covered by                                               |
| --- | --------------------------------------------------------------------- | ------ | ------ | ------------------------------------------------ | -------------------------------------------------------- |
| G30 | No derived task feed                                                  | server | new    | the Labelling page shows plan steps, not tasks   | Item 4                                                   |
| G31 | No web City page with a map and a To do list                          | web    | new    | the Site viewer shows one site at a time         | Item 5, on the [city view plan](lidar-city-view-plan.md) |
| G32 | The visualiser's "Lanes to review" is lanes only, computed in the app | macOS  | new    | City view panel: Site, High, Medium, Outstanding | Item 5                                                   |
| G33 | The Labelling page overlaps City and the review surface               | web    | new    | plans label0001 and label0002, "bind window"     | Open question 2                                          |

#### Held-out review (T16)

| #   | Gap                                                                                       | Owner         | Status | Evidence                                                      | Covered by                                                    |
| --- | ----------------------------------------------------------------------------------------- | ------------- | ------ | ------------------------------------------------------------- | ------------------------------------------------------------- |
| G34 | A held-out role; the selector locked to `random` with a recorded seed                     | server, web   | new    | Segments roles: Tuning, Evaluation                            | Item 6; Item 6 of the vocabulary plan holds the held-out rule |
| G35 | A web Review page: accept, reject, re-class, needs box edit, freeze window                | web, server   | new    | label0001: "Labels are edited in the macOS annotation window" | Not adopted: review stays in macOS                            |
| G36 | Sealing: frozen held-out packs excluded from tuning; held-out scores only from evaluation | server        | new    |                                                               | Item 6                                                        |
| G37 | Proposals exported from macOS per window                                                  | macOS, server | new    | macOS already makes proposals                                 | Not adopted with G35; the web reads status, not proposals     |

#### Summary

Of T1 to T12, one is done (T2), five are partial (T3, T4, T7, T8, T11) and six are not started
(T1, T5, T6, T9, T10, T12). T9's one-way deep link is now the run form of Item 1's view link. T13
to T16 have no implementation. The handoff's "largest single lever", a `packs` table with writable
states, is the one piece this plan does not take: the status those states would carry is derived
on read from sidecars and jobs, and the five gaps it would close (G14, G15, G16, G30, G34) read the
derived status instead.

### What the handoff proposed, and what stands

The handoff's forty items, grouped as it grouped them, against the decision that covers each.
"Backlog" means the item already exists under the UI workflow plan; "here" means an item below.

| Handoff group                        | Items                                                                                                                                                                     | Disposition                                                                                                                                                                                                                                                                                                                        |
| ------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| A: schema and lineage                | `runs.params_hash`, grouped runs API, `recommended_by_sweep_id`, `packs` table, sweep columns, sweep scheduler, `site_id` columns, a run's clip to the visualiser         | `params_hash` and a run's clip exist; `recommended_by_sweep_id` and `recommended_at` are the one additive change (backlog, P2). `packs`, the scheduler, `resumable` and radar `site_id` are [not adopted](lidar-ui-workflow-plan.md#not-adopted). A run's clip reaches the visualiser once sweep runs record it (backlog, v0.5.10) |
| B: vocabulary, Runs, Clips, Captures | Segments to Windows, "location" to "site", runs grouped, the clip composer, the Clips header, the Clips table, lanes on the Site viewer, the clip panel, Status read-only | Backlog, v0.5.10 to v0.5.11, except: no Windows page (Item 6 folds Segments into Clips); lanes stay keyed as the [lane review plan](lidar-lane-review-citywide-plan.md) keys them; Status keeps editing the server defaults, since experiment arms are files in `config/experiments/`                                              |
| C: packs, Windows, Sweeps            | Windows page with a queue bar, sweep cards, the visualiser's sheet, the Annotation window's pack writes                                                                   | Queueing a pack is the segment finder's pack job, already built; the sheet and the title are backlog (v0.5.11); sweep cards are backlog (v0.5.12); no `PATCH` on a pack                                                                                                                                                            |
| D: shared view and Tracks            | a `views` table and endpoints, Tracks header, one play bar, timeline marks, keyboard, the URL scheme, snapshots                                                           | Adopted as Items 1 and 2 below, with the snapshot deferred                                                                                                                                                                                                                                                                         |
| E: City and the task feed            | a derived task feed, a web City page, nav changes, the visualiser's City panel, an owner field                                                                            | Adopted as Items 3 to 5 below, without the owner: there is one operator                                                                                                                                                                                                                                                            |
| F: held-out packs and web review     | proposals exported from macOS, a seeded random selector, a decisions table, a sealing rule, a Review page, a return path, plan steps read from the pack                   | The selector and the sealing rule are Item 6 below; review in the web is not adopted (the owner keeps macOS as the one annotation and grading instrument); plan steps reading pack state is Item 4's `freeze` rule                                                                                                                 |

## Design

### Item 1: a view record both apps can open

A view is what one person is looking at: a run, an instant, a camera, which layers are drawn,
which filters apply and which track is selected. Today the web and macOS each hold this in memory
and neither can hand it to the other. T9's "Open in visualiser" link carried a run and a frame;
the view record carries the rest, and the deep link names a view instead.

| Field                | Meaning                                                                                          |
| -------------------- | ------------------------------------------------------------------------------------------------ |
| `view_id`            | Typed id, a four-letter prefix such as `view` (the platform's typed-UUID rule)                   |
| `run_id`, `clip_id`  | The run shown and, when it has one, its clip                                                     |
| `t_ns`, `frame`      | The instant, as recording time and frame index                                                   |
| `camera`             | Mode (top, front, side, follow, free), azimuth, elevation, distance, and a target point or track |
| `layers`             | Points, boxes, trails, grid, labels: each on or off                                              |
| `filters`            | Class, state, minimum observations                                                               |
| `selected_track_id`  | The selected track, if any                                                                       |
| `name`, `created_in` | An optional name; which app saved it (web, macOS, or a pack job)                                 |

Endpoints: create a view from either app, read one by id, list a run's views. An unsaved view
travels as query parameters on the Tracks route and on the URL scheme, with the same keys; a saved
one as its id. Each pack window gets a view when the pack job writes it, following the top-scoring
pair, or the top camera when there is none. One parser specification serves both apps, with one
fixture set the Go, TypeScript and Swift parsers all pass.

The handoff's snapshot (a PNG saved with the view, shown behind the web viewer until frames
stream) is deferred: it needs an asset store the server does not have, and the view itself is the
part both apps need.

### Item 2: Tracks as one viewer

The web Tracks page keeps its 3D view, playback and timeline lanes and loses what duplicated them
or belonged to macOS:

- A context header naming the clip, the run, its parameter digest, the date and the track count,
  with a Views menu, Copy view link, and Open in visualiser.
- One play bar: play, speed, layers and the camera presets. The second bar goes.
- Window and pack marks on the timeline; clicking one opens Clips filtered to that candidate.
- The right panel shows the selected track (class against label, observed frames, speed, the
  windows it is in) and the read-only labels. The class, state and minimum-observation filters
  move into the play bar's layers menu.
- Keyboard: space, left and right for a frame, up and down for a track, W and P for the next
  window or pack, F to follow, 1 to 3 for the camera, L for labels.

"Tracks names the run's clip" (backlog, P2) lands with this item.

### Item 3: the navigation

The web sidebar's LiDAR group gains the pages the design did not place. Proposed order: Status,
City, Captures, Clips, Runs, Tracks, Sweeps; then a Sensor group of Site viewer, Regions, Debug
and Settings. There is no Windows page (Item 6 of the vocabulary plan) and no Review page (see
[not adopted](#not-adopted)). Where the Labelling page goes is Open question 2.

### Item 4: a derived task feed

One endpoint lists what is left to do, derived on read from the records that already exist and
never stored by hand. Each task names a site, a kind, a priority, which app closes it and a link.
The rules:

| Kind   | Raised when                                                                        | Priority                         | Closed in                    |
| ------ | ---------------------------------------------------------------------------------- | -------------------------------- | ---------------------------- |
| locate | a capture session has static time and no site                                      | high over ten sessions, else low | web, Captures                |
| clip   | a site has sessions with static periods and no clip                                | low                              | web, Captures                |
| run    | a clip has no run whose parameters match its recommended set, or no run at all     | medium                           | web, Runs                    |
| rank   | a run has no candidates for the default selector                                   | medium when the clip has a run   | web, Clips                   |
| queue  | a run has candidates and no pack job                                               | low                              | web, Clips                   |
| label  | a pack is packed or proposed and not yet in review                                 | medium                           | macOS, the pack              |
| freeze | a pack is reviewed and not in a frozen split, or a labelling plan step waits on it | medium                           | macOS, the split             |
| lanes  | the lane snapshot flags a street segment High or Medium at the site                | high when any is High            | macOS, City view at the site |
| sweep  | a sweep is suspended or queued                                                     | low                              | web, Sweeps                  |

The handoff's `review` kind (a held-out window with proposals, reviewed in the web) folds into
`label`, since review happens in macOS. Pack kinds read the derived pack status of the UI workflow
plan; `lanes` reads the snapshot the [lane review plan](lidar-lane-review-citywide-plan.md) pulls
from Gerardus. One test per rule, raising and closing it. Materialising the feed is deferred until
reading it is slow.

### Item 5: City lists what is left to do

The [city view plan](lidar-city-view-plan.md) draws every site with data on one map in both
clients from static tiles. This item adds the task feed to it: a dot per site sized by its open
tasks and red when any is high, a site card on click, and a To do table filtered by kind and app.
In the visualiser, the "Lanes to review" panel becomes the feed filtered to the tasks macOS closes,
with the lane rows keeping their High, Medium and Outstanding columns. The web page is the city
view plan's web surface and lands with it; this item adds the feed, not the map.

### Item 6: held-out windows the apps can see

The vocabulary plan's Item 6 already carries the held-out rule in a clip's selection: a held-out
candidate is drawn by the `random` finder with a recorded seed and draw count, and nothing else may
draw one. This item makes the rule visible and enforced:

- The candidates panel on Clips offers the held-out role; choosing it fixes the finder to
  `random` and shows the seed and the count drawn, with "Draw n more".
- Sweeps and the auto-tuner refuse a held-out pack as an objective. A held-out score comes only
  from an explicit evaluation, stored as an evaluation and never shown as a sweep's best.
- A labelling plan's steps (label0001 to label0003) read their state from the pack: selected when
  the pack exists, proposed when every window has proposals, reviewed when every object is, frozen
  when the pack is in a frozen split. The plan files stop recording progress by hand.

Review itself, accepting or rejecting proposals and freezing the split, stays in macOS.

## Priorities for the next implementation PRs

Reviewed against the backlog, the gaps' dependencies and the owner's decisions. Each line is one
PR, smallest and least contested first.

1. **Sweep runs record their clip** (server, `S`, v0.5.10). Every lineage gap downstream (G6, G7,
   G8, G15, G16) reads a run's clip, and none of the latest 500 development runs records one. No
   design question is open.
2. **A restart suspends a checkpointed sweep instead of failing it** (server, `S`, v0.5.12). Two
   sweeps on the audit's Sweeps page had died this way. The `suspended` status and the
   checkpoint columns exist; only recovery changes.
3. **Runs grouped by clip, repeats folded, parameters against the recommended set** (web, `M`,
   v0.5.10). The audit's Runs page shows five identical rows for one clip; this is the page an
   operator reads most. Depends on 1.
4. **Captures makes clips from selected periods** (web and server, `S`, v0.5.10). Closes G18 and
   G20 and retires the Clips header's "New Clip"; one clip per static period, never spanning
   motion, as the UI workflow plan decided.
5. **The URL scheme and "Open in visualiser"** (macOS and web, `M`, v0.5.11). Pick the scheme's
   name first (decided: `velocityreport://`, see Design decisions); the run-and-frame form ships now and Item 1 adds the view form
   later without a second scheme.
6. **The visualiser's run browser grouped by clip, the Annotation window titled by clip and pack**
   (macOS, `M`, v0.5.11). Depends on 1; the audit's sheet shows forty rows with no clip.

Pack status on Clips and the clip panel read-outs follow once Item 6 of the vocabulary plan lands,
since both read the selection it adds. Items 1 to 6 of this plan wait for the owner to place them;
of them, the task feed (Item 4) is the cheapest to start because it is read-only over records that
exist, and the view record (Item 1) is the one that needs a specification both apps sign before
either writes a parser.

## Not adopted

- **Review in the web** (handoff F5, F6, G35, G37). The owner's decision is that macOS is the one
  annotation and grading instrument and the web is left as it is. The web shows a window's state
  and links to macOS; it accepts no verdict, and macOS exports no proposals to the server for the
  web to show.
- **A `packs` table with writable states, and `PATCH` from macOS.** A pack is immutable (V19) and
  review status is on annotations (V10); the derived status covers every reader. Restated from the
  [UI workflow plan](lidar-ui-workflow-plan.md#not-adopted) with its other declined columns.
- **An assignee and "Queued for me".** There is one operator. The visualiser filters by pack status
  ("Awaiting review"), not by a name.
- **Lanes keyed by site on the Site viewer.** The lane review plan keys proposals and authored
  lanes by city street segment and direction inside S2; the Clips columns show per-clip job
  progress, which is a different fact.
- **A four-digit parameter hash.** Four hex digits collide within one sweep; the existing digest is
  shown at eight.
- **A new release name.** The handoff's "0.5.2" is the harness slot; this work has no slot until
  the owner places it.

## Dependencies

- Items 1 and 2 need nothing new on the server but the view record; Item 2's clip link needs
  priority 1 above.
- Items 4 and 5 read the derived pack status (UI workflow plan, v0.5.11), the lane snapshot (lane
  review plan) and the city tiles (city view plan); Item 5's web page is the city view plan's.
- Item 6 needs Item 6 of the vocabulary plan (the selection on the clip) and the segment finder's
  `random` finder.

## Risks

| Risk                                                                | Likelihood | Impact | Mitigation                                                                                |
| ------------------------------------------------------------------- | ---------- | ------ | ----------------------------------------------------------------------------------------- |
| Two parsers for the view's query form drift                         | Medium     | Medium | One specification and one fixture set; the Go, TypeScript and Swift tests all read it     |
| The task feed is slow over many sites                               | Low        | Low    | Derived on read first; materialise only when measured                                     |
| The held-out rule is enforced in the UI but not in the sweep runner | Medium     | High   | The refusal lives in the server where sweeps start; the UI only reflects it               |
| The audit's "partial" and "unknown" rows age before the work starts | High       | Low    | Each PR re-reads its screen before closing a gap; the table records the date it was taken |

## Design decisions

- The twelve changes stay in the [UI workflow plan](lidar-ui-workflow-plan.md); this plan adds no
  second home for them. Implemented facts: `params_hash` on param sets, `replay_case_id` on run
  records, the derived pack status on Segments, the pack job, `suspended` with checkpoint columns.
- The handoff's five files are replaced by this plan; the design project in Claude Design remains
  the visual reference, as the UI workflow plan's Open question 2 records.
- **The URL scheme is `velocityreport://`**, chosen by the owner on November 7, 2026 over the
  mockup's `velocity.report://`. The run form is `velocityreport://run/<run_id>?frame=<n>`, where
  `n` is the 0-based index of a record in the run's VRLOG, background snapshots included, as the
  visualiser counts; without `frame` the run opens at its start. The view and pack forms use the
  same scheme when they are specified.

## Open questions

1. **Where a view lives.** A table of its own, or a JSON column on the pack window plus a table for
   saved views only. The unsaved query form needs neither; decide when Item 1 is placed.
2. **The Labelling page.** Keep it as the editor for a labelling plan's binding and notes with its
   progress read from the pack (Item 6), or retire it once the task feed carries its "next" copy.
3. **A clip from a window.** Segments' "Make clip" makes a clip from one ranked span. Clips from
   periods is the normal path; decide whether a "moment clip" is a real need before removing it.

## Checklist

### Complete

- [x] The November 8 audit recorded, G1 to G37, with each gap mapped to the item or decision that
      covers it
- [x] The handoff's forty items dispositioned against the UI workflow plan

### Outstanding

- [ ] Item 1: the view record, endpoints, the shared query form and its fixtures (`M`)
- [ ] Item 2: Tracks as one viewer: header, one play bar, timeline marks, selected-track panel,
      keyboard (`M`)
- [ ] Item 3: the navigation groups (`S`)
- [ ] Item 4: the task feed, one test per rule (`L`)
- [ ] Item 5: the feed on the City page and in the visualiser's City panel (`M`)
- [ ] Item 6: the held-out role in the candidates panel, the sweep refusal, plan steps read from
      the pack (`M`)

### Deferred

- [ ] A view's snapshot and the web pre-render behind the viewer: needs an asset store
- [ ] A materialised task feed with push to the visualiser: only when reading it is slow
- [ ] A clip of several static periods: a schema change for P2-B, as the UI workflow plan records

### Accepted residuals (no action planned)

- [ ] Full annotation in the web: review-only is not adopted either; macOS is the instrument
