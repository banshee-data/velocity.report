# LiDAR workflow changes: implementation spec (part 3 of 3): macOS, order, acceptance

Reference: `LiDAR Workflow Mockups.html`, "macOS · annotating" surface.

## T10 — VelocityVisualiser: Analysis runs sheet

- Title "Analysis runs". Segmented control top-right: **All runs | Queued for me (n)** (badge count from `GET packs?state=queued&assignee=me`).
- Rows grouped by clip: group header = clip name (bold), site, pack state counts on the right (wrap if needed).
- Columns: Run (short id), Date, Duration, Tracks, **Labels** (progress bar filled from label fraction + pack state chip, or `—`). Rename the old **Case** column → drop it (clip is now the group).
- Repeat runs with identical params fold into "+ N more, identical parameters".
- "Queued for me" shows only runs with a queued pack.
- Footer: "Loaded: <clip> · <run id>", Stop replay, **Open** (primary).
- No horizontal scroll; last column `minmax(0,1fr)`.

## T10 — Annotation window

- Window title: `Annotation — <clip name> · <pack name> · window i of n`. Fall back to run id only for packless/live runs.
- Sidebar "Run" block: clip name, `<run id> · p·<hash>`, frame position. New "This pack" block: state chip, "k of n windows reviewed".
- **Open another…** menu sections: "Queued for me" (clip bold, `pack · N windows`, queued chip), "In progress" (labelling), separator, "Open by run id…".
- State writes (`PATCH /api/lidar/packs/:id`): opening a queued pack → `labelling`; all windows reviewed → `reviewed`; **Freeze pack** button → `frozen` (rename existing "Freeze S…" button).
- Register URL scheme `velocityreport://run/<id>?frame=<n>` to open the run at that frame (used by web Tracks).

## Shared state chip colours (web + macOS)

candidate: outline grey · queued: slate tint · labelling: sky tint · reviewed: emerald tint · frozen: solid emerald-700 with emerald-50 text.

## Suggested PR order

1. Migrations + backfill (runs.clip_id, params_hash, packs, sweeps fields, site_id).
2. Vocabulary + routes (T1, T2) with redirects.
3. Runs grouping (T4) and Tracks clip default (T6).
4. Captures site picker + make-clip-from-periods (T7, T11, T3 copy).
5. Clips panel read-outs + lineage (T12, T8 counts).
6. Packs on Windows (T8) + pack API.
7. Sweeps queue/resume/apply (T5).
8. Tracks preview + deep link (T9).
9. macOS sheet, annotation title, Open another, pack state writes, URL scheme (T10).

## Acceptance checks

- No user-facing "segment" or "case" strings remain in web or macOS (grep).
- Old routes `/lidar/segments`, `/lidar/replay-cases` redirect.
- Every non-live run shows a clip; Runs page has no "-" clip cells.
- Starting a sweep while one runs yields `queued`, never `failed`. Restarting the server yields `resumable`.
- Clip edit has no free-text captures list or params JSON.
- Making a clip from 3 selected periods on Captures creates a clip whose panel shows those 3 periods highlighted.
- Queuing windows on web shows up under "Queued for me" in macOS; opening it flips web state to `labelling`.
- "Open in visualiser" from web Tracks opens macOS at the same run and frame.
