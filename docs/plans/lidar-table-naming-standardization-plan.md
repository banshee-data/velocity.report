# LiDAR Table Naming Standardization Plan

**Status:** Proposed
**Target:** v0.5.0
**Layers:** L3 Grid, L4 Perception, L5 Tracks, L8 Analytics, L9 Endpoints
**Related:** [Schema Simplification Migration 030](schema-simplification-migration-030-plan.md), [LiDAR Tracks Table Consolidation Plan](lidar-tracks-table-consolidation-plan.md), [LiDAR L8/L9/L10 Refactor Plan](lidar-l8-analytics-l9-endpoints-l10-clients-plan.md), [v0.5.0 Backward Compatibility Shim Removal Plan](v050-backward-compatibility-shim-removal-plan.md)

## Goal

Standardize the LiDAR SQLite table names into one coherent conceptual structure
before `v0.5.0`, with the smallest practical rename set.

This plan is explicitly **LiDAR-only**. Existing radar table names such as
`radar_data`, `radar_objects`, and `radar_data_transits` are left unchanged.

## Problem

The current LiDAR schema mixes:

- abbreviated table families: `lidar_bg_*`, `lidar_track_obs`
- live-track, run-track, replay-case, and tuning concepts without a clear
  owner model
- an L8 replay fixture table named `lidar_scenes`, even though "Scene" is
  already reserved by the layer model for future L7 canonical scene work
- a generic `lidar_labels` name for a table that is actually tied to track
  annotation spans

The result is that the schema reads as if it was assembled by feature rather
than by a stable naming model.

## Design Rules

1. Use full words by default, but allow entrenched short forms when they are
   already clear and low-risk. `bg` is an allowed exception.
2. Group tables by conceptual owner rather than one global prefix:
   - `lidar_bg_*` for L3 persisted background/grid state
   - `lidar_track_*` for live L5 track tables and direct children
   - `lidar_run_*` for artefacts owned by an executed analysis run
   - `lidar_replay_*` for saved replay fixtures and replay-scoped scores
   - `lidar_sweep*` or `lidar_tuning_*` for optimisation sessions
3. Reserve `scene` for future L7 canonical scene work.
4. Prefer plural entity names for tables.
5. Keep already-good anchor names when renaming them would create unnecessary
   FK and API churn before `v0.5.0`.

## Current LiDAR Schema Inventory

Current LiDAR tables in [`internal/db/schema.sql`](../../internal/db/schema.sql):

| Current table          | Current role                                       | Naming issue                                     |
| ---------------------- | -------------------------------------------------- | ------------------------------------------------ |
| `lidar_bg_regions`     | persisted L3 region state                          | acceptable `bg` exception                        |
| `lidar_bg_snapshot`    | persisted L3 grid snapshot                         | singular noun                                    |
| `lidar_clusters`       | L4 cluster persistence                             | acceptable                                       |
| `lidar_tracks`         | live L5 track buffer                               | acceptable anchor name                           |
| `lidar_track_obs`      | per-observation track state                        | abbreviated child noun                           |
| `lidar_labels`         | track-linked annotation spans                      | overly generic                                   |
| `lidar_analysis_runs`  | run metadata                                       | acceptable anchor name                           |
| `lidar_run_tracks`     | run-scoped track snapshots                         | acceptable run-owned child                       |
| `lidar_scenes`         | saved replay fixture                               | collides with future L7 "scene"                  |
| `lidar_evaluations`    | run-vs-run scores scoped to a saved replay fixture | missing replay owner                             |
| `lidar_sweeps`         | tuning/sweep metadata                              | broader than replay; owner wording could improve |
| `lidar_missed_regions` | run-scoped missed-detection evidence               | owner is implicit                                |

## Recommended Canonical Constructs

### 1. L3 background/grid

Keep the existing `bg` family. It is already established in code and schema,
and the meaning is unambiguous inside the LiDAR subsystem:

- `lidar_bg_regions`
- `lidar_bg_snapshot`

### 2. L4 perception

Keep `lidar_clusters` as the L4 persistence surface.

### 3. L5 live track surface

Use `lidar_tracks` as the anchor and make direct child tables explicit:

- `lidar_tracks`
- `lidar_track_observations`
- `lidar_track_annotations`

`lidar_track_annotations` is preferred over `lidar_track_labels` because the
track-level ground-truth labels already live on `lidar_run_tracks.user_label`.
Using "annotations" here keeps those concepts distinct.

### 4. L8 executed run surface

Use `run` for artefacts owned by a concrete executed run, regardless of whether
that run came from live capture or PCAP replay:

- `lidar_analysis_runs`
- `lidar_run_tracks`
- `lidar_run_missed_regions`

### 5. L8 replay fixtures

Use `replay_case` for a saved replayable PCAP slice plus its reference run and
best-known launch params:

- `lidar_replay_cases`
- `lidar_replay_evaluations`

This is the right construct for the current `lidar_scenes` table. It is more
concrete than "analysis context" and more future-safe than "scene".

### 6. L8 tuning sessions

`lidar_sweeps` is broader than replay. It stores plain sweeps, auto-tune, and
HINT sessions, with replay-case linkage carried inside the request payload when
applicable.

So for the pre-`v0.5.0` minimal pass:

- keep `lidar_sweeps` unchanged
- treat it conceptually as the tuning-session table
- revisit a possible rename such as `lidar_tuning_sessions` only if we want a
  larger L8 terminology cleanup later

## Minimal Rename Set

This is the recommended pre-`v0.5.0` rename set.

### Keep unchanged

These names are already good enough and act as stable anchors:

| Keep                  | Why                                                                  |
| --------------------- | -------------------------------------------------------------------- |
| `lidar_bg_regions`    | `bg` is an allowed exception and already established                 |
| `lidar_bg_snapshot`   | paired naturally with `lidar_bg_regions`; churn not justified        |
| `lidar_tracks`        | core live-track anchor; renaming it would force unnecessary FK churn |
| `lidar_clusters`      | already clear and short; no conflicting family                       |
| `lidar_analysis_runs` | clear anchor for executed runs that may be `live` or `pcap`          |
| `lidar_run_tracks`    | already reads correctly as a run-owned child table                   |
| `lidar_sweeps`        | broad tuning-session table, not purely replay-owned                  |

### Rename

| Current                | Proposed                   | Why                                                                   |
| ---------------------- | -------------------------- | --------------------------------------------------------------------- |
| `lidar_track_obs`      | `lidar_track_observations` | remove abbreviation                                                   |
| `lidar_labels`         | `lidar_track_annotations`  | make track ownership explicit without colliding with run-track labels |
| `lidar_scenes`         | `lidar_replay_cases`       | make replay-fixture role explicit; avoid L7 scene collision           |
| `lidar_evaluations`    | `lidar_replay_evaluations` | scores are persisted against a replay case                            |
| `lidar_missed_regions` | `lidar_run_missed_regions` | make run ownership explicit                                           |

## Resulting Conceptual Structure

After the rename, the LiDAR schema reads as one system rather than a set of
feature-era leftovers:

```text
lidar_bg_regions
lidar_bg_snapshot

lidar_clusters

lidar_tracks
lidar_track_observations
lidar_track_annotations

lidar_analysis_runs
lidar_run_tracks
lidar_run_missed_regions

lidar_replay_cases
lidar_replay_evaluations

lidar_sweeps
```

## Column and API Counterpart Renames

The replay-case cleanup should move with the obvious column/type/API terms.

### Replay case rename

If `lidar_scenes` becomes `lidar_replay_cases`, then:

- `scene_id` becomes `replay_case_id` where it refers to the replay-case table
- `SceneStore` becomes `ReplayCaseStore`
- `Scene` becomes `ReplayCase`
- `/api/lidar/scenes/...` becomes `/api/lidar/replay-cases/...`

Affected analysis surfaces include:

- current `lidar_evaluations.scene_id`
- current `lidar_labels.scene_id` when it refers to a replay case
- any Go/TS/Swift types named `SceneID` that actually mean an L8 replay fixture

### What should not move into the replay family

Some tables participate in replay workflows but are not themselves replay-owned:

- `lidar_analysis_runs` can represent either `live` or `pcap` runs
- `lidar_run_tracks` are snapshots owned by a specific run, not by a replay
  case directly
- `lidar_sweeps` stores tuning sessions across plain sweeps, auto-tune, and
  HINT, so a replay prefix would overfit one mode

### Existing adjacent naming cleanups

This plan assumes the already-identified adjacent cleanups still stand:

- `world_frame` -> `frame_id` from the migration-030 plan
- `peak_speed_mps` -> `max_speed_mps` where still pending

Those are complementary and should be batched with this work when practical.

## Why Keep `lidar_tracks`

The conceptual live-track table could be named `lidar_track_states`, but doing
that before `v0.5.0` buys little and costs a lot:

- `lidar_track_obs` and `lidar_labels` currently FK to `lidar_tracks`
- many docs already use `lidar_tracks` as the anchor name
- the main inconsistency is not the word `tracks`; it is the surrounding family
  names (`run_tracks`, `track_obs`, generic `labels`)

So the minimal-change answer is: keep `lidar_tracks`, rename the surrounding
tables into a consistent family.

## Why Keep Separate Physical Track Tables

This plan is naming-only. It does **not** recommend merging live and analysis
track tables.

That question is handled separately in
[LiDAR Tracks Table Consolidation Plan](lidar-tracks-table-consolidation-plan.md),
whose current recommendation remains correct: one logical track measurement
model, but separate physical tables for live/transient vs immutable analysis
snapshots.

Scene replay strengthens that separation:

- analysis replay intentionally avoids writing to the live `lidar_tracks` hot table
- replayed runs need immutable run-scoped track snapshots
- deterministic replay can reuse the same `track_id` values across runs, which
  fits `lidar_run_tracks` better than a single mixed-lifecycle table

## Migration Strategy

### Phase 1: Lock the naming contract

- Approve the construct model in this plan
- update adjacent design docs to use the proposed names
- update `docs/data/SCHEMA.svg` after schema changes land

### Phase 2: Ship the SQL renames as one coordinated break

- rename tables
- rename affected FKs and indexes
- rename Go stores/types and endpoint paths in the same release window

### Phase 3: Remove old names rather than keeping long-lived aliases

Because `v0.5.0` is already the coordinated breaking-change release, prefer one
clean cut over indefinite dual naming.

## Proposed Checklist

- [ ] Approve the LiDAR constructs: `bg`, `track`, `run`, `replay_case`, `tuning`
- [ ] Rename `lidar_track_obs` -> `lidar_track_observations`
- [ ] Rename `lidar_labels` -> `lidar_track_annotations`
- [ ] Rename `lidar_scenes` -> `lidar_replay_cases`
- [ ] Rename `lidar_evaluations` -> `lidar_replay_evaluations`
- [ ] Rename `lidar_missed_regions` -> `lidar_run_missed_regions`
- [ ] Rename `scene_id` -> `replay_case_id` where it refers to replay cases
- [ ] Rename `SceneStore`/`Scene`/scene API paths to replay-case names
- [ ] Keep `lidar_bg_regions`, `lidar_bg_snapshot`, `lidar_tracks`, `lidar_clusters`, `lidar_analysis_runs`, `lidar_run_tracks`, and `lidar_sweeps` unchanged
- [ ] Keep radar table names unchanged

## Recommendation

Before `v0.5.0`, standardize the LiDAR schema around:

- `lidar_bg_*` as the L3 persisted background/grid family
- `lidar_tracks` plus `lidar_track_*` as the live L5 surface
- `lidar_analysis_runs` plus `lidar_run_*` as the executed-run surface
- `lidar_replay_cases` plus `lidar_replay_evaluations` as the replay-fixture surface
- `lidar_sweeps` as the tuning-session table for now

That gives us one unifying conceptual structure with minimal churn, avoids a
future L7 `scene` naming collision, keeps `bg` as a deliberate local
abbreviation, and avoids incorrectly forcing live/run/tuning tables under a
single replay prefix they do not really own.
