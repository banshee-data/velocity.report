# LiDAR workflow changes: implementation spec (part 2 of 3): web screens

Reference: `LiDAR Workflow Mockups.html`, "Web · structuring" surface. Keep existing styling (emerald AppBar, surface sidebar, emerald-tinted table header/stripes, status pills).

## T1 — Nav: Segments → Windows, reorder

- Sidebar LiDAR group order: **Captures, Clips, Runs, Windows, Tracks, Sweeps, Settings**.
- Rename page/route `/app/lidar/segments` → `/app/lidar/windows` (keep redirect). Page title "Windows".
- Subtitle: "Spans of a run ranked by a selector. Queue the ones worth labelling as a pack; the macOS visualiser picks packs up from the queue."

## T2 — Clip everywhere

- Route `/app/lidar/replay-cases` → `/app/lidar/clips` (301/redirect from old).
- Grep and replace user-facing "case"/"replay case" with "clip" across web + API docs.

## T3 + T7 + T11 — Captures, expanded session

- Session table gains a **Site** column (between Session and Window). Unset shows amber "no site".
- Expanded session (below the motion strip and legend):
  - **Site select** populated from Sites, plus a last option "+ New site from <lat, lon>" that creates a site. Hint: "From Sites. The clip and any radar report share this record." Remove the free-text "name this site" + "Save name".
  - Heading **"Periods"** (keep the word; never "segment"). Chips `static-N m:ss` / `motion-N m:ss` become toggleable (checkmark when selected). Clicking a strip block toggles the same period. Unselected blocks dim to ~30% when any is selected.
  - **New clip composer** (bordered emerald-tinted box): read-only prefilled description `"<Site as 'A at B'> (<m:ss> static across N periods)"`, hint line `N periods · m:ss · [warn: includes K motion periods ·] <capture count> · <file>`, primary button **"Make clip from N periods"** (disabled at 0). On success replace with "✓ Clip created · Open in Clips →" linking to the new clip selected.

## T8 + T12 — Clips list + edit panel

- Table columns: Clip, Site, Static (m:ss), Captures (`N joined` badge if >1, nowrap), Runs, **Packs** (state count chips, `—` if none), Created. New clips get a small "new" badge.
- Right panel (360px):
  - Header "Clip" + short id; link "Open latest run in Tracks →".
  - Description (textarea, editable), Site (select from Sites).
  - **Periods**: session strip with the clip's periods highlighted, chips of chosen periods (read-only), hint `<session id> · N captures · change periods in Captures` (link). **Remove the captures textarea.**
  - Captured at / Sensor read-outs.
  - **Recommended parameters**: read-only key/value list; hint "Set by sweep <id link> on <date> · hash p·xxxx · read-only". Empty state: "None yet. A completed sweep on this clip sets them." **Remove the JSON textarea.**
  - Lineage block: Runs (count, top 3 with status pill, id, p·hash, tracks; "all →" to Runs filtered), Packs (state chips; "windows →"), Sweeps (count, top 2 with pill + best score).
  - Footer: Save changes (saves description + site only).

## T4 — Runs grouped by clip

- One `<tbody>` per clip; group header row: **clip name**, site, `recommended p·xxxx`, `N runs`, "clip →". Final group "No clip · live data".
- Columns: Status, Run (id · duration), **Parameters** (`p·hash` then either "= recommended" or amber diff chips `key from→to`), Tracks, Labels (progress bar + %; `—` if 0), Created, Actions (Tracks, Windows, Delete).
- Consecutive runs with identical hash, no diff and same status fold into the first row plus a fold row "+ N more, identical p·xxxx" (nowrap), click to expand as indented sub-rows.
- Drop the always-empty "Clip" column.

## T8 — Windows

- Run select labels: `<clip> · <run id> · p·<hash>`.
- Pack-state strip under the description: `candidate → queued → labelling → reviewed → frozen` chips + hint "Web sets candidate and queued. macOS sets labelling, reviewed, frozen."
- Table: checkbox (disabled if window already in a pack), Window UTC, **Clip / offset** (clip name `@ 45.3 s`, not capture filename), Score, Finder detail, **Pack** (state chip + pack name, or `candidate`), Actions (Details, Preview, Hide). Remove "Make clip" action (clips come from periods now).
- Sticky bottom bar when ≥1 selected: "N windows selected", Pack select (New: <auto name> | existing candidate/queued packs), primary **Queue for labelling** → POST packs.

## T6 + T9 — Tracks as preview

- Header Clip select defaults to the run's clip (hint "from this run"); "None (live data)" only for runs with null clip_id.
- Keep 3D view, playback bar, track timeline lanes unchanged.
- Replace right-hand filter panel (Class/State/Label/Min obs/Sort) with:
  - "Labels · from macOS, read-only": progress bar, points agreed `98% · a of b`, objects reviewed, pack state chip + pack name.
  - Primary **Open in visualiser** button → deep link `velocityreport://run/<run_id>?frame=<current_frame>`, shown in mono below.
  - Hint "Opens the same run and frame in the macOS visualiser. Labels are only edited there."

## T5 — Sweeps

- Card rows: (1) state pill, type, id, timestamp; (2) `clip <link>`, `base run <id>`, `combinations a/b`, `best <score>` (each label/value nowrap, row wraps), duration right; (3) progress bar for `running` on its own row; (4) note; (5) actions.
- Notes by state: queued "Queued behind <id>. Starts when it finishes." · resumable "Interrupted by server restart at combination n of N." + **Resume from n** · cancelled "Cancelled in round r. Round … results kept." · completed applied "Set as recommended parameters for <clip>." · completed not applied, show compare to current + **Set as recommended** · failed keeps error text in red.
- Pill colours: completed green, failed red, running sky, queued slate, resumable amber, cancelled grey.
