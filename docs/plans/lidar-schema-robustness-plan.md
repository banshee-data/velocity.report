# LiDAR Schema Robustness Plan

- **Status:** Proposed
- **Layers:** Database, L5 Tracks, L8 Analytics, API, macOS visualiser
- **Related:** [LiDAR Tracks Table Consolidation Plan](lidar-tracks-table-consolidation-plan.md), [Pre-v0.5.0 Schema Simplification Migration 030 Plan](schema-simplification-migration-030-plan.md), [Track Labelling, Ground Truth Evaluation & Label-Aware Auto-Tuning](lidar-track-labelling-auto-aware-tuning-plan.md)

## Executive Summary

Two important questions drove this review:

1. What happens to `lidar_track_observations` and `lidar_track_annotations` when `lidar_tracks` changes?
2. Why is `lidar_track_annotations.replay_case_id` not a hard foreign key to `lidar_replay_cases`?

The answers are:

- **Normal track updates do not drop child rows.** `InsertTrack()` uses `INSERT ... ON CONFLICT DO UPDATE`, and `UpdateTrack()` uses plain `UPDATE`, so live-track updates do not delete and recreate the parent row.
- **Track deletion is inconsistent today.** The schema declares `ON DELETE CASCADE` from `lidar_tracks` to both observations and annotations, but the main DB open path currently leaves SQLite foreign-key enforcement disabled. On March 21, 2026 I verified locally that `NewDBWithMigrationCheck()` returns `PRAGMA foreign_keys = 0`.
- **In practice today:** observations are still removed during prune/clear because the code deletes them explicitly first, but annotations are not explicitly deleted in those same code paths, so they can be left orphaned.
- **`replay_case_id` is not a hard FK by design, not by accident.** Prior plan docs describe it as a nullable soft provenance reference: "which replay case was the human looking at when they authored this annotation?" rather than "this annotation is owned by that replay case."

That leaves us with a deeper design issue than one missing FK:

- `lidar_tracks` is a **transient live table**
- `lidar_run_tracks` is an **immutable run-scoped table with canonical track labels**
- `lidar_track_annotations` sits awkwardly between the two, because it currently points at a transient live track while also carrying replay-case provenance

The proposal below recommends fixing this by making ownership boundaries explicit instead of just adding one more FK to an already ambiguous table.

## Current Findings

### 1. Live-track updates do not delete observations or annotations

`InsertTrack()` uses `ON CONFLICT(track_id) DO UPDATE`, specifically to avoid the `INSERT OR REPLACE` delete-then-insert behavior. `UpdateTrack()` is an in-place `UPDATE`.

That means:

- updating a live track does **not** drop its observations
- updating a live track does **not** drop its annotations

### 2. Live-track deletes are only partially cleaned up today

The schema says:

- `lidar_track_annotations.track_id -> lidar_tracks(track_id) ON DELETE CASCADE`
- `lidar_track_observations.track_id -> lidar_tracks(track_id) ON DELETE CASCADE`

But the runtime open path does not currently execute `PRAGMA foreign_keys = ON`.

So today:

- `PruneDeletedTracks()` explicitly deletes from `lidar_track_observations`, then deletes from `lidar_tracks`
- `ClearTracks()` explicitly deletes from `lidar_track_observations`, then deletes from `lidar_tracks`
- neither path explicitly deletes from `lidar_track_annotations`

Result:

- observations are removed by application code
- annotations can survive as orphans when the parent live track row is deleted

### 3. `replay_case_id` is a soft reference because the table never had a clear owner

The historical intent appears to be:

- `/api/lidar/labels` is for **free-form event annotation**
- `/api/lidar/runs/{run_id}/tracks/{track_id}/label` is for **canonical run-track labelling**

That split is reasonable, but the current schema shape is not:

- canonical track labels already live on `lidar_run_tracks`
- free-form annotations still anchor to transient `lidar_tracks.track_id`
- `replay_case_id` was added later as context/provenance, not ownership

This is why a hard FK was never added: the table was never modeled as replay-case-owned data.

## Problems To Solve

### Problem 1: Declared referential integrity does not match actual runtime behavior

The schema claims cascades; the runtime does not reliably enforce them.

### Problem 2: Annotation ownership is ambiguous

`lidar_track_annotations` currently mixes three concepts:

- live-track identity (`track_id`)
- replay context (`replay_case_id`)
- free-form human annotation metadata (`notes`, `source_file`, timestamps)

Those concepts do not share the same lifecycle.

### Problem 3: Canonical labelling already has a better home

`lidar_run_tracks` already stores run-scoped track labels and quality flags. That is the right home for durable ground-truth track labelling. Keeping a second track-linked annotation table tied to live tracks increases confusion.

### Problem 4: A one-off replay-case FK would harden the wrong table shape

Adding `FOREIGN KEY (replay_case_id) REFERENCES lidar_replay_cases(...)` to the current table would improve provenance validation, but it would not solve the more important problem: the table is still attached to transient live tracks.

## Recommended End State

Use one owner per lifecycle:

- **Live transient data**
  - `lidar_tracks`
  - `lidar_track_observations`
- **Run-scoped durable track truth**
  - `lidar_run_records`
  - `lidar_run_tracks`
- **Replay-scoped durable annotation context**
  - `lidar_replay_cases`
  - `lidar_replay_evaluations`
  - `lidar_replay_annotations` (new or renamed/rekeyed table)

### Recommended table contract for free-form annotations

If `/api/lidar/labels` remains a free-form annotation API, its storage should be replay-owned, not live-track-owned.

Recommended direction:

```sql
CREATE TABLE lidar_replay_annotations (
    annotation_id TEXT PRIMARY KEY,
    replay_case_id TEXT NOT NULL,
    run_id TEXT,
    track_id TEXT,
    class_label TEXT NOT NULL,
    start_timestamp_ns INTEGER NOT NULL,
    end_timestamp_ns INTEGER,
    confidence REAL,
    created_by TEXT,
    created_at_ns INTEGER NOT NULL,
    updated_at_ns INTEGER,
    notes TEXT,
    source_file TEXT,
    FOREIGN KEY (replay_case_id) REFERENCES lidar_replay_cases(replay_case_id) ON DELETE CASCADE,
    FOREIGN KEY (run_id, track_id) REFERENCES lidar_run_tracks(run_id, track_id) ON DELETE SET NULL
);
```

Why this shape is better:

- replay case becomes the durable owner
- optional run/track linkage is durable and versioned
- annotations no longer disappear just because a live track buffer is pruned
- canonical run-track labels stay on `lidar_run_tracks`

### Alternative if replay ownership is too strong

If the team wants annotations to survive replay-case deletion, keep `replay_case_id` nullable and make it:

- `FOREIGN KEY ... ON DELETE SET NULL`

But even in that softer model, the annotation table should still stop pointing at live `lidar_tracks`.

## What We Should Not Do

- Do **not** merge live and run track tables as part of this work. The existing consolidation plan correctly notes that live and run tracks have different lifecycles and FK requirements.
- Do **not** simply add a hard replay-case FK to the current `lidar_track_annotations` table and call the problem solved.
- Do **not** enable `PRAGMA foreign_keys = ON` globally without first deciding what should happen to existing annotations when live tracks are pruned.

## Proposed Phases

### Phase 0: Integrity Audit And Contract Decision

- **Target window:** March 24, 2026 to March 31, 2026
- **Goal:** make the current behavior explicit before changing it

Deliverables:

- add an automated DB test that checks the production open path foreign-key setting
- add one audit query/report for orphaned:
  - `lidar_track_observations`
  - `lidar_track_annotations`
  - `replay_case_id` values that do not resolve
- write down the intended delete behavior for annotations:
  - should they be deleted with live tracks?
  - or should they survive as replay/run-owned assets?

Exit criteria:

- the team makes an explicit contract decision for annotation ownership

### Phase 1: Runtime Integrity Baseline

- **Target window:** April 1, 2026 to April 11, 2026
- **Goal:** make DB behavior deterministic across environments

Deliverables:

- enable `PRAGMA foreign_keys = ON` in the main DB open path
- add regression tests for:
  - track update
  - track prune
  - track clear
  - replay case delete
  - run delete
- add explicit delete behavior where needed during the transition so cleanup is not connection-state-dependent
- add basic `CHECK` constraints where low-risk:
  - `end_timestamp_ns IS NULL OR end_timestamp_ns >= start_timestamp_ns`
  - `confidence IS NULL OR (confidence >= 0 AND confidence <= 1)`

Important note:

- if Phase 0 concludes that annotations should survive live-track pruning, then this phase must **not** leave `lidar_track_annotations -> lidar_tracks` intact when FK enforcement is enabled

Exit criteria:

- the database behaves the same way in local, test, and production opens

### Phase 2: Re-key Free-Form Annotations

- **Target window:** April 14, 2026 to May 9, 2026
- **Goal:** move annotation ownership onto the correct lifecycle boundary

Deliverables:

- create `lidar_replay_annotations` or migrate `lidar_track_annotations` into the replay-owned shape
- backfill existing rows:
  - preserve rows with valid replay-case provenance
  - map track references to `(run_id, track_id)` where possible
  - quarantine unresolved rows into an audit table or export file instead of silently dropping them
- update `/api/lidar/labels` to use the new storage contract
- validate `replay_case_id` on create/update

Exit criteria:

- free-form annotations are no longer anchored to transient live tracks

### Phase 3: Simplify Label Ownership

- **Target window:** May 12, 2026 to June 6, 2026
- **Goal:** make each labelling path have one obvious home

Deliverables:

- treat `lidar_run_tracks` as the only canonical home for run-ground-truth labelling
- treat replay annotations as the only home for free-form replay/event annotation
- deprecate the old live-track-linked annotation table after one release
- move raw annotation SQL out of `internal/api/lidar_labels.go` into a store/repository layer
- update docs and client code to reflect the final split

Exit criteria:

- there is no longer any ambiguity between:
  - run-track labels
  - replay annotations
  - live transient track data

### Phase 4: Hardening And Visibility

- **Target window:** June 2026+
- **Goal:** keep the schema healthy over time

Deliverables:

- add an admin/integrity endpoint or report for orphan counts and FK health
- add migration-time integrity checks before destructive schema changes
- optionally add generated schema documentation / ERD verification in CI

## Decision Matrix

| Question                                      | Option A                        | Option B                                                  | Recommendation                          |
| --------------------------------------------- | ------------------------------- | --------------------------------------------------------- | --------------------------------------- |
| Should `replay_case_id` become a hard FK?     | No, keep soft reference forever | Yes, but only after annotation ownership is replay-scoped | **Option B**                            |
| What should free-form annotations attach to?  | Live `lidar_tracks`             | Replay case plus optional run track                       | **Replay case plus optional run track** |
| Where should durable track ground truth live? | `lidar_track_annotations`       | `lidar_run_tracks`                                        | **`lidar_run_tracks`**                  |
| When should FK enforcement be turned on?      | Never                           | After delete semantics are made explicit                  | **After semantics are explicit**        |

## Concrete Answers To The Original Questions

### What happens when `lidar_tracks` records are updated?

- **On update/upsert:** child observation and annotation rows are not dropped.
- **On delete/prune in the schema as written:** they are supposed to cascade.
- **On delete/prune in the runtime as it behaves today:** observations are deleted explicitly by code; annotations can remain orphaned because foreign-key enforcement is currently off.

### Why is `replay_case_id` on annotations not a foreign key today?

- because it was introduced as a soft provenance field, not as the owner of the row
- the current table shape mixes live-track ownership with replay-case context
- adding a hard FK without re-keying the table would improve one edge while leaving the deeper lifecycle mismatch intact

## Recommendation

Treat this as an ownership-boundary cleanup, not a single-column FK patch.

The right near-term sequence is:

1. decide whether annotations are ephemeral or durable
2. turn foreign-key enforcement on deliberately
3. move free-form annotations onto replay/run-owned tables
4. keep canonical track labelling on `lidar_run_tracks`

That gives us a schema that is both simpler and more robust: every durable table has one clear owner, and every delete path becomes predictable.
