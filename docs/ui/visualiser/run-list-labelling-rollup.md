# Run List Labelling Rollup

- **Status:** Implemented — macOS visualiser and backend complete. Web runs-list parity deferred.

Compact run-list icon in the visualiser's run list showing human review progress for each analysis run at a glance. The icon communicates human review progress — not model carry-over state.

## Visual Design

Segmented capsule icon next to each run in the run list:

| Segment     | Colour | Meaning                                                                      |
|-------------|--------|------------------------------------------------------------------------------|
| Classified  | Green  | Track has non-empty `user_label` with `label_source` empty or `human_manual` |
| Tagged only | Accent | Human-applied tag state exists but no manual classification                  |
| Unlabelled  | Grey   | No human-applied label state                                                 |

Capsule proportions are computed from the ratio of tracks in each bucket.

## Bucket Classification Rules

- **Classified:** `label_source` is empty or `human_manual`, AND `user_label` is non-empty, AND `user_label` is not `split` or `merge`.
- **Tagged only:** No manual classification present, but some human-applied tag state exists: `quality_label` non-empty, legacy `user_label` of `split`/`merge`, split/merge candidate flags, or linked track IDs.
- **Unlabelled:** None of the above.

Explicitly excluded from green/accent: `carried_over`, `auto_suggested`.

## Backend Design

### Store Contract

`AnalysisRun` includes a `label_rollup` field with:

| Field         | Type | Description             |
|---------------|------|-------------------------|
| `total`       | int  | Total tracks in the run |
| `classified`  | int  | Human-classified tracks |
| `tagged_only` | int  | Tagged but unclassified |
| `unlabelled`  | int  | No human label state    |

Rollups are computed from `lidar_run_tracks` using one grouped query for
run lists (no per-run follow-up queries). Store helpers live in
`internal/lidar/storage/sqlite/analysis_run.go`.

### API Contract

`label_rollup` is exposed on:

- `GET /api/lidar/runs` — list view with rollup per run
- `GET /api/lidar/runs/{run_id}` — single run detail
- `GET /api/lidar/runs/{run_id}/labelling-progress` — dedicated progress endpoint

`labelling-progress.labelled` = `classified + tagged_only`, keeping
run-browser progress and labelling-progress semantics aligned.

The implementation tolerates environments where `lidar_run_tracks` is
absent, so partial-schema tests do not fail while reading run rows.

## macOS Client Design

### Model

`RunTrackLabelAPIClient.swift` models include `RunLabelRollup`,
`AnalysisRun.labelRollup`, and additional `RunTrack` fields for local
recomputation: `label_source`, `linked_track_ids`, split/merge flags.

### UI

`RunBrowserView.swift` renders a `Labels` column with the segmented
capsule. Tooltip shows exact counts and percentages.

### Local State Update Rules

The icon updates immediately after a successful label write — no
re-fetch of run summaries:

- `RunBrowserState` is shared app state, not sheet-local
- Per-run track snapshots are primed when a run is loaded for replay
- Rollup is recomputed in memory when `assignLabel(...)`,
  `assignQuality(...)`, or bulk label writes succeed

- No extra validation trip after a `2xx` response

## Web Contract

The shared API contract is typed on the web side (optional `label_rollup`
on `AnalysisRun` and `LabellingProgress`). No web rendering in this
phase.

## Deferred Work

- Web runs-list parity UI: rendering the same capsule icon in the Svelte web frontend
- Decide whether `/flags` mutations should update the local rollup path in Swift
- Operator-facing docs for the icon's workflow meaning