# LiDAR workflow changes: implementation spec (part 1 of 3)

Source of truth for intent: `LiDAR Workflow Review.html`. Visual reference: `LiDAR Workflow Mockups.html` (T1–T12 markers show where each change goes).
Repo: `banshee-data/velocity.report`. Web app: Svelte + svelte-ux + Tailwind under `/app/*`. macOS: SwiftUI VelocityVisualiser.

## Principle

- The web app structures the dataset and shows status. It never edits frames or labels.
- macOS is the only place frames and labels change.

## Vocabulary (apply to UI copy, routes, API field names where cheap)

| Term    | Meaning                                                         | Replaces                               |
| ------- | --------------------------------------------------------------- | -------------------------------------- |
| Capture | one pcap file                                                   | —                                      |
| Session | captures with contiguous packet clocks                          | —                                      |
| Period  | static/motion stretch from motion pass (`static-0`, `motion-0`) | "segment" at capture stage             |
| Clip    | one or more periods kept as replay material (= replay case)     | "Case" (macOS), `replay-cases` (route) |
| Run     | one pipeline pass over a clip with one parameter set            | —                                      |
| Window  | selector-ranked span of a run                                   | "Segment" (web page)                   |
| Pack    | windows queued together for labelling                           | implicit                               |
| Sweep   | parameter search scored against a clip's labels                 | —                                      |

## Data model changes (backend first)

1. `runs.clip_id` (nullable FK → clips). Null only for live-data runs. Backfill: match existing runs to clips by source capture path(s); leave null where no match.
2. `runs.params_hash` — short stable hash (first 4 hex of sha256 of canonical sorted JSON params). Expose as `p·7f3a`.
3. `clips.recommended_params` (JSON, exists) + `clips.recommended_by_sweep_id` + `clips.recommended_at`. Write only from sweep completion or the "Set as recommended" action; the web edit form no longer writes it.
4. `clips.periods` — ordered list of `{session_id, period_name, start_ns, end_ns, cls}`. Replaces the free-text captures list as the authoring input; derive the capture list server-side (still validate adjacency like today).
5. `clips.site_id` FK → sites, and `sessions.site_id` FK → sites. Replace the free-text "name this site" on sessions.
6. `packs` table: `id, run_id, name, role (tuning|evaluation), state, created_at, updated_at, updated_by`. `windows.pack_id` nullable FK.
   - States, in order: `candidate → queued → labelling → reviewed → frozen`.
   - Web may set `queued` (and back to `candidate` by removing). macOS sets `labelling`, `reviewed`, `frozen`.
   - Window state with no pack = `candidate`.
7. `sweeps.clip_id`, `sweeps.base_run_id`, `sweeps.best_score`, `sweeps.best_combination`, `sweeps.progress_done/progress_total`.
   - New states: `queued`, `running`, `resumable`, `completed`, `cancelled`, `failed`.
8. Sweep scheduler: when a sweep is already running, enqueue (`queued`) instead of failing with "sweep already in progress". On server restart, mark in-flight sweeps `resumable` (not `failed`) and persist the last completed combination index.

## API additions

- `GET /api/lidar/clips` — include `site`, `static_seconds`, `capture_count`, `run_count`, `pack_counts{state:n}`, `sweep_count`.
- `GET /api/lidar/runs?group=clip` — groups `{clip, recommended_hash, runs[]}`; each run includes `params_hash`, `params_diff[]` vs clip recommended (`{key, from, to}`), `label_fraction`.
- `POST /api/lidar/clips` body `{session_id, period_names[], site_id, description}`.
- `POST /api/lidar/packs` `{run_id, name, role, window_ids[]}` → state `queued`.
- `PATCH /api/lidar/packs/:id` `{state}` (macOS uses this).
- `GET /api/lidar/packs?state=queued&assignee=me`.
- `POST /api/lidar/sweeps/:id/resume`, `POST /api/lidar/sweeps/:id/apply` (sets clip recommended params).
