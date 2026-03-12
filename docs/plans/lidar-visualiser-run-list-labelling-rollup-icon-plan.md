# Design: Run-List Labelling Rollup Icon

**Status:** In implementation (March 2026)
**Layers:** SQLite store, L9 Endpoints, macOS visualiser, web contract

## Objective

Add a compact run-list icon that shows the percentage of tracks with human-applied labelling state, without forcing the client to re-fetch run data after every successful label write.

The icon is a segmented rollup:

- green: manually classified tracks
- accent colour: manually tagged tracks with no class label
- grey: tracks with no human-applied label state

## Scope

This feature is specifically for the macOS Swift run browser list.

It requires:

- backend rollup computation so the client does not perform N+1 progress fetches
- a stable API contract on run summaries
- local client state updates after successful `2xx` label writes

## Product Semantics

The run-list icon represents the current human labelling state for tracks in a run.

Bucket rules:

- `classified`:
  - `label_source` is empty or `human_manual`
  - `user_label` is non-empty
  - `user_label` is not `split` or `merge`
- `tagged_only`:
  - no manual classification is present
  - but some human-applied tag state exists:
    - `quality_label` non-empty, or
    - legacy `user_label` of `split` / `merge`, or
    - split/merge candidate flags, or
    - linked track ids
- `unlabelled`:
  - none of the above

Explicitly excluded from green/accent colour:

- `carried_over`
- `auto_suggested`

Rationale:

- the icon should communicate human review progress, not model carry-over state
- the rollup should cover all manual tags, not just split/merge

## Backend Design

### Store Contract

Extend `AnalysisRun` with:

- `label_rollup`

Add a derived rollup model:

- `total`
- `classified`
- `tagged_only`
- `unlabelled`

Add store helpers in `internal/lidar/storage/sqlite/analysis_run.go`:

- `GetRunLabelRollup(runID string)`
- grouped rollup population for `ListRuns(limit)`

Implementation requirements:

- compute from `lidar_run_tracks`, not parent run counters
- use one grouped query for run lists
- avoid per-run follow-up queries

### API Contract

Expose `label_rollup` on:

- `GET /api/lidar/runs`
- `GET /api/lidar/runs/{run_id}`
- `GET /api/lidar/runs/{run_id}/labelling-progress`

`labelling-progress.labelled` should mean:

- `classified + tagged_only`

This keeps run-browser progress and labelling-progress semantics aligned.

### Notes

The implementation should tolerate environments where `lidar_run_tracks` is absent, so tests that build partial schemas do not fail while reading run rows.

## macOS Client Design

### Model Contract

Extend `RunTrackLabelAPIClient.swift` models with:

- `RunLabelRollup`
- `AnalysisRun.labelRollup`
- `LabellingProgress.labelRollup`
- additional `RunTrack` fields needed to recompute local rollups:
  - `label_source`
  - `linked_track_ids`
  - split/merge flags

### UI

Add a new `Labels` column in `RunBrowserView.swift`.

Render a segmented capsule:

- green segment width = `classified / total`
- accent-colour segment width = `tagged_only / total`
- grey segment width = `unlabelled / total`

Tooltip/help text should show exact counts and percentages.

### Local State Update Rules

The browser icon must update immediately after a successful label write.

Requirements:

- do not fetch run summaries again after a successful label update
- do not perform an extra validation trip after a `2xx` response
- update local rollup state only after the server returns success

Implementation shape:

- keep `RunBrowserState` as shared app state, not sheet-local throwaway state
- prime per-run track snapshots when a run is loaded for replay
- recompute the run rollup in memory when:
  - `assignLabel(...)` succeeds
  - `assignQuality(...)` succeeds
  - bulk label writes succeed

## Web Contract

Even though the icon is macOS-only in this slice, the shared API contract should be typed on the web side.

Add optional `label_rollup` typing to:

- `AnalysisRun`
- `LabellingProgress`

No web rendering changes are required for this phase.

## Current Diff Coverage

The current branch diff covers:

- backend rollup model and grouped query
- `labelling-progress` alignment with the new rollup
- Swift decoding for `label_rollup`
- macOS run-browser segmented icon
- shared run-browser state
- immediate in-memory icon refresh after successful label writes
- Go and Swift tests for the contract and local update behaviour

## Follow-Up Work

Outstanding follow-through after the initial diff:

- decide whether `/flags` mutations should also update the local rollup path in Swift
- add the same icon or a derivative summary to the web runs list if product wants parity
- document operator meaning of green/accent-colour/grey in visualiser user docs

## Task Checklist

### Backend

- [x] Add `label_rollup` to analysis run responses
- [x] Compute run-list rollups from `lidar_run_tracks`
- [x] Align `labelling-progress` with the rollup contract
- [x] Add tests for classified / tagged-only / unlabelled counts

### macOS

- [x] Add rollup decoding models
- [x] Add segmented run-list icon in `RunBrowserView`
- [x] Share run-browser state through `AppState`
- [x] Update the icon locally after successful label writes
- [x] Add focused Swift tests for decode and local rollup updates

### Web

- [x] Add optional TypeScript contract for `label_rollup`
- [ ] Add web runs-list parity UI if required

### Documentation

- [x] Write this design document
- [ ] Add operator-facing docs if the icon ships as user-visible workflow guidance

## Acceptance Criteria

- `GET /api/lidar/runs` returns `label_rollup` for each run without client-side N+1 requests.
- The macOS run browser shows a segmented green/accent-colour/grey icon for each run.
- A successful label write updates the displayed icon immediately, without another server validation trip.
- Carried-over and auto-suggested labels do not count as human-labelled progress.

## Open Questions

- Should `/api/lidar/runs/{run_id}/tracks/{track_id}/flags` be folded into the same optimistic update path as `updateLabel(...)`?
- Does product want web runs-list parity, or is this intentionally macOS-only?
