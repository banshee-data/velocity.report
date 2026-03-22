# Run List Labelling Rollup

- **Source plan:** `docs/plans/lidar-visualiser-run-list-labelling-rollup-icon-plan.md`

- **Status:** Implemented (March 2026) — macOS visualiser and backend complete. Web runs-list parity deferred.

## Visual Design

Segmented capsule icon next to each run in the run list:

| Segment     | Colour | Meaning                                                                      |
| ----------- | ------ | ---------------------------------------------------------------------------- |
| Classified  | Green  | Track has non-empty `user_label` with `label_source` empty or `human_manual` |
| Tagged only | Accent | Track has a label but from automated/carried-over source                     |
| Unlabelled  | Grey   | No label assigned                                                            |

Capsule proportions are computed from the ratio of tracks in each bucket.

## Bucket Classification Rules

- **Classified:** `label_source` is empty or `human_manual`, AND `user_label` is non-empty.
- **Tagged only:** `user_label` is non-empty but `label_source` is `auto_suggested` or `carried_over`.
- **Unlabelled:** `user_label` is empty.

Note: `carried_over` and `auto_suggested` sources are explicitly excluded from the "classified" bucket.

## Backend Rollup

Aggregated from `lidar_run_tracks` grouped by `run_id`. Returns three integer counts per run.

## Local State Update

On successful label write, the capsule updates locally from the write response without re-fetching the full run list. This keeps the UI responsive during labelling sessions.

## Deferred Work

Web runs-list parity UI: rendering the same capsule icon in the Svelte web frontend is tracked separately.
