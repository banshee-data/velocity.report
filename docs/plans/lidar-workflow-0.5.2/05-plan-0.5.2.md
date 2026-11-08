# 0.5.2 — implementation plans and backlog

Closes the gaps in `04-gaps-2026-11-08.md`. Six plans, each one shippable on its own and ordered so later plans only read what earlier ones wrote. Sizes: S ≤ ½ day, M 1–2 days, L 3–5 days. Items marked **0.5.3** are out of 0.5.2 and listed so nothing is lost.

Milestone goal, in one sentence: every run, window, pack and sweep knows its clip; packs have states both apps read; a view opens the same picture in web and VV; City lists what is left to do; a held-out pack can be drawn, reviewed in the web and frozen.

Repos: `banshee-data/velocity.report` (server + web), VelocityVisualiser (mac). Issue prefix suggestion: `wf-` plus the backlog id (`wf-A1`).

---

## Plan A — Schema and lineage (server) · ~4 days

Prerequisite for everything else. Spec: `01-model-and-api.md` §Data model, items 1–8.

| id  | Item                                                                                                                                                                                        | Size | Closes   |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---- | -------- |
| A1  | `runs.params_hash` (4 hex of sha256 of canonical params); backfill; expose on `GET /api/lidar/runs`                                                                                         | S    | G8       |
| A2  | `GET /api/lidar/runs?group=clip` → `{clip, recommended_hash, runs[{params_hash, params_diff[], label_fraction}]}`                                                                           | M    | G7, G8   |
| A3  | `clips.recommended_by_sweep_id`, `recommended_at`; only sweep-apply writes `recommended_params`; remove the web write path                                                                  | S    | G9       |
| A4  | `packs` table + `windows.pack_id`; states `candidate→queued→labelling→reviewed→frozen`; `role ∈ {tuning, evaluation, held-out}`; `POST/PATCH/GET /api/lidar/packs` with `?state=&assignee=` | M    | G13, G34 |
| A5  | `sweeps.clip_id, base_run_id, best_score, best_combination, progress_done, progress_total`; states add `queued, resumable, cancelled`                                                       | S    | G10      |
| A6  | Sweep scheduler: enqueue when one is running; on restart mark `resumable` and persist last combination; `POST /sweeps/:id/resume`, `POST /sweeps/:id/apply`                                 | M    | G11      |
| A7  | `sessions.site_id`, `clips.site_id` FK → sites; migrate free-text names where they match a site; API returns `site{id,name}` not `location`                                                 | M    | G3, G19  |
| A8  | Expose `runs.clip_id` + clip name in the endpoint VV reads for its runs sheet                                                                                                               | S    | G6       |

Acceptance: Runs API has no null `clip_id` on non-live runs; starting a sweep while one runs returns `queued`; killing the server mid-sweep leaves it `resumable`; `PATCH /packs/:id {state: frozen}` is refused unless current state is `reviewed`.

---

## Plan B — Vocabulary, Runs, Clips, Captures (web) · ~5 days

Spec: `02-web-screens.md` T1, T2 residue, T3, T4, T7, T8 counts, T11, T12.

| id  | Item                                                                                                                                                                                                    | Size | Closes   |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---- | -------- |
| B1  | Rename Segments → Windows, route `/app/lidar/windows` with redirect; nav order Status, Captures, Clips, Runs, Windows, Tracks, Sweeps, then Sensor group (Site viewer, Regions, Debug, Settings)        | S    | G1, G4   |
| B2  | Replace "location" copy with "site": Captures filter "sessions with static time and no site", Clips drop the Registered column once B6 lands                                                            | S    | G3       |
| B3  | Runs grouped by clip (one `tbody` per clip), params `p·hash` + diff chips vs recommended, fold identical consecutive runs, drop the Clip column (now the group header)                                  | M    | G7, G8   |
| B4  | Captures session detail: Site select from Sites (+ "New site from lat, lon"); period chips toggle; "Make clip from N periods" composer → `POST /clips {session_id, period_names, site_id, description}` | M    | G19, G20 |
| B5  | Clips header: replace "New Clip" / "Scan PCAP Folder" with "Make from periods →" (Captures) and keep scan under Captures' volume bar                                                                    | S    | G18      |
| B6  | Clips table: Clip, Site, Static, Captures (joined badge), Runs, Packs (state chips), Created; Lanes column moves to Site viewer keyed by site (B7)                                                      | M    | G14, G22 |
| B7  | Site viewer: show lane proposals for the chosen site with "Run lanes" there                                                                                                                             | S    | G22      |
| B8  | Clip side panel: description, site, period strip + chips, captured-at/sensor, read-only recommended params with sweep link, lineage (runs, packs, sweeps)                                               | L    | G9, G21  |
| B9  | Status page: tuning JSON becomes read-only "server defaults · hash"; link to the clip whose recommendation is loaded; edits go through sweep-apply or an explicit "experiment arm" file                 | S    | G9       |

Acceptance: `grep -ri "segment\|replay.case\|location" web/src` returns only period-strip internals; Runs page shows no "-" clip cells and ≤1 row per identical parameter set unless expanded; making a clip from 3 periods shows those 3 highlighted in the panel.

---

## Plan C — Packs, Windows, Sweeps (web + mac) · ~5 days

Spec: `02-web-screens.md` T8, T5; `03-macos-order-acceptance.md` T10.

| id  | Item                                                                                                                                                                                                            | Size | Closes   |
| --- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---- | -------- |
| C1  | Windows page: run select `clip · run · p·hash`; pack-state strip; Clip/offset column; Pack column with chip + name; checkbox select → sticky bar "Queue for labelling" (POST packs); remove per-row "Make clip" | M    | G13, G17 |
| C2  | Sweeps cards: state pill, clip link, base run, combinations a/b, best, progress bar; actions Resume / Cancel / Set as recommended                                                                               | M    | G10, G11 |
| C3  | VV Analysis runs sheet: group by clip, pack counts on group header, Labels bar + pack chip, fold identical runs, segmented "All runs / Queued for me (n)", footer "Loaded: clip · run"                          | L    | G6, G15  |
| C4  | VV Annotation window: title `clip · pack · window i of n`; "This pack" block; "Open another…" menu (Queued for me / In progress / by run id); state writes labelling → reviewed → frozen via `PATCH /packs/:id` | M    | G16      |

Acceptance (from `03`): queuing windows on web appears under "Queued for me" in VV; opening it flips the web chip to `labelling`; Freeze pack writes `frozen` and the Clips Packs column updates on refresh.

---

## Plan D — Shared view surface and Tracks (T13, T14) · ~6 days

New. Replaces T9's one-way deep link.

### View record

```
views: id (v_ + 4 hex), run_id, clip_id, t_ns, frame, camera{mode: top|front|side|follow|free, az, el, dist, target[3] | track_id},
       layers{points, boxes, trails, grid, labels}, filters{class, state, min_obs}, selected_track_id,
       name, created_by, created_in (web|vv|pack), snapshot_asset_id (nullable), created_at
```

- `POST /api/lidar/views` (either app), `GET /api/lidar/views/:id`, `GET /api/lidar/runs/:id/views`.
- Pack windows get a view each at queue time (camera `follow` on the top-scoring pair, or `top` if none).
- Unsaved view in query form: `?run=&t=&cam=mode,az,el,dist,tx,ty,tz&layers=pbtgl&sel=`. Same keys in `velocityreport://view?…`; saved form `velocityreport://view/v_3f9c`.

| id  | Item                                                                                                                                                                                                                                                    | Size | Closes   |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---- | -------- |
| D1  | `views` table + endpoints + query-form parser shared by web and VV (one spec doc, two parsers, one fixture set)                                                                                                                                         | M    | G23      |
| D2  | Web Tracks: context header `clip › run · p·hash · date · tracks`; Views menu; Copy view link; Open in visualiser (`velocityreport://view/:id`, falls back to query form for unsaved)                                                                    | M    | G24, G28 |
| D3  | Web Tracks: one play bar (play, speed, layers, camera presets Top/Front/Follow); remove the second bar; Clip picker defaults to the run's clip                                                                                                          | M    | G12, G25 |
| D4  | Web Tracks: window/pack marks on the timeline (click → Windows filtered to that window); lane click selects and follows; right panel is the selected track (class vs label, observed frames, speed, in windows) + labels read-only + view state readout | M    | G26, G27 |
| D5  | Web Tracks keyboard: space, ←/→ frame, ↑/↓ track, W/P next window/pack, F follow, 1/2/3 camera, L labels                                                                                                                                                | S    | G27      |
| D6  | VV: register `velocityreport://` (view and run forms); "Views ▾" and "Share view" in the toolbar; saving a view uploads a PNG snapshot (`snapshot_asset_id`)                                                                                            | M    | G28, G29 |
| D7  | Web Tracks shows the snapshot behind the viewer until frames stream, then fades it                                                                                                                                                                      | S    | —        |

Acceptance: open a view saved in VV from the web and the frame number, camera mode and selected track match; the reverse too; a view URL pasted into a fresh browser lands on the right clip and run with no picker interaction.

---

## Plan E — City navigator and task feed (T15) · ~5 days

New.

### Task feed

`GET /api/lidar/tasks?site=&kind=&surface=&owner=` → `[{id, site{id,name,lat,lon}, kind, label, priority, surface: web|mac, href, raised_at, closes_when}]`.
Derived on read (materialise later if slow), never stored by hand. Kinds and rules:

| kind   | raised when                                                                 | priority                        | surface | href                        |
| ------ | --------------------------------------------------------------------------- | ------------------------------- | ------- | --------------------------- |
| locate | session has static time and no `site_id`                                    | high if > 10 sessions, else low | web     | Captures filtered           |
| clip   | site has sessions with static periods and no clip                           | low                             | web     | Captures session            |
| run    | clip has no run whose `params_hash` = recommended (or no run at all)        | medium                          | web     | Runs group                  |
| rank   | run has no windows for the default selector                                 | medium if clip has ≥ 1 run      | web     | Windows                     |
| queue  | run has candidates and no pack in `queued+`                                 | low                             | web     | Windows                     |
| label  | pack in `queued` or `labelling`                                             | medium                          | mac     | `velocityreport://pack/:id` |
| review | held-out window with proposals and state ≠ frozen                           | high                            | web     | Review                      |
| freeze | pack `reviewed` not yet frozen; or plan step waiting on it                  | medium                          | web     | Review / Windows            |
| lanes  | VV lane flags High/Medium for the site (read from the existing lanes store) | high if any High                | mac     | VV City at site             |
| sweep  | sweep `resumable` or `queued`                                               | low                             | web     | Sweeps                      |

| id  | Item                                                                                                                                                                                                    | Size | Closes |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---- | ------ |
| E1  | Task feed endpoint with the ten rules above; one test per rule (raise + close)                                                                                                                          | L    | G30    |
| E2  | Web City page: Leaflet map (reuse Site viewer), dot per site sized by weighted open tasks, red when any high; site card on click; To do table with Kind / Surface / Owner filters; rows link via `href` | L    | G31    |
| E3  | Nav: City under Status; Labelling's plan progress rendered in the Review page header, Labelling page stays as the plan editor (bind window, notes) but loses its "Next" copy, which City now carries    | S    | G33    |
| E4  | VV City view: replace the lanes-only panel with the task feed filtered to `surface=mac` plus a "web" section; keep High/Medium/Outstanding columns for `lanes` rows                                     | M    | G32    |
| E5  | Owner: tasks of kind label/review carry `owner` from pack assignee; "Your name" field on Labelling becomes the identity used by `assignee=me`                                                           | S    | —      |

Acceptance: a session given a site drops its `locate` task on the next read; queuing a pack raises `label` in VV's panel within one refresh; the two apps show the same count for a site.

---

## Plan F — Held-out packs and web review (T16) · ~5 days

New. Depends on A4 (`role`), D1 (views, for crops), and macOS proposals export (F1).

| id  | Item                                                                                                                                                                                                                                                                             | Size     | Closes                                                                                                              |
| --- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- | ------------------------------------------------------------------------------------------------------------------- |
| F1  | macOS: export proposals per window to the server when a pack is opened: `POST /packs/:id/windows/:i/proposals [{id, class, confidence, points, frames, box_frames[]}]`; also top/front/side crops per object as small PNGs (or server renders from the frame store if available) | M        | G37                                                                                                                 |
| F2  | Server: `random` selector (seeded; records `seed` and `draw_count` on the pack); `role=held-out` forces it; `decisions` table `{proposal_id, decision: accepted                                                                                                                  | rejected | geometry, class, by, at}`; `POST /packs/:id/windows/:i/freeze`requires every proposal decided and no`geometry` open | M   | G34, G36 |
| F3  | Server: sealing rule — sweeps and auto-tune exclude packs with `role=held-out`; `POST /sweeps/:id/evaluate?pack=` returns a held-out score, stored separately as `evaluations`                                                                                                   | S        | G36                                                                                                                 |
| F4  | Web Windows: Pack role select gains Held-out; with it the selector shows "Random · n windows · seed"; Plan select binds to a labelling plan; held-out table with state, progress, "Review in web →", "Draw n more", "Freeze pack"                                                | M        | G34                                                                                                                 |
| F5  | Web Review page: header (clip, pack, window i of n, run, plan step, progress bar); crops top/front/side with frame stepper; proposals table with Accept / class select / Reject / needs box edit; rules panel; Freeze window (disabled until all decided); pack window list      | L        | G35                                                                                                                 |
| F6  | `needs box edit` raises a `label` task for macOS on that window (via E1) and the window stays open; VV Annotation shows "returned from web review: k objects"                                                                                                                    | S        | G35                                                                                                                 |
| F7  | Labelling plan steps read state from the pack: select = pack exists, propose = proposals present on all windows, review = all windows frozen, freeze = pack frozen                                                                                                               | S        | G33                                                                                                                 |

Acceptance: a held-out pack cannot be used as a sweep objective (API refuses); a window with one `geometry` decision cannot be frozen; freezing all four windows moves label0001 to 4 of 4; the frozen split's score appears only under an evaluation, never in a sweep card's "best".

---

## Order and parallelism

```
week 1   A1–A8 (server)                         B1, B2 (web, no server dependency)
week 2   B3–B9 (needs A)                        C2 (needs A5/A6)         D1 (server, independent of A except runs.clip_id)
week 3   C1, C3, C4 (needs A4)                  D2–D5 (needs D1)
week 4   E1 (needs A4, A7)  →  E2–E5            D6, D7 (mac)
week 5   F1 (mac) → F2, F3 → F4, F5 → F6, F7    buffer, acceptance pass from 03 + the four above
```

Web and mac can run in parallel from week 2. If one person does everything, A → B → C → D → E → F in order is the safe path and E/F slip to 0.5.3.

## 0.5.3 (deferred, keep visible)

- VV snapshot fade and web pre-render for every pack window (D7 extension).
- Materialised task feed with webhooks to VV instead of poll-on-refresh.
- Lanes: move proposal generation to be site-keyed end to end (today keyed by clip).
- Retire the Labelling page once plans are fully derived (F7 covers reading; writing plans still needs the page).
- Full annotation in the web (box editing). Not planned; review-only is the agreed split.
- Clip from a window (Segments' current "Make clip"): decide whether a "moment clip" is a real need; if so, specify it as a clip with `source=window` and keep periods as the normal path.

## Backlog index (issue-ready)

A1 A2 A3 A4 A5 A6 A7 A8 · B1 B2 B3 B4 B5 B6 B7 B8 B9 · C1 C2 C3 C4 · D1 D2 D3 D4 D5 D6 D7 · E1 E2 E3 E4 E5 · F1 F2 F3 F4 F5 F6 F7 — 40 items, ~30 working days across server, web and mac.
