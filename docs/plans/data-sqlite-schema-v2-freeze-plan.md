# SQLite schema v2 freeze-and-cleanup plan

- **Status:** Proposed

The database schema has grown across capture, replay, scene, sweep, and site lifecycle concerns without a single clean ownership model. This plan freezes the current schema as the v1 contract, does a one-time v2 cleanup that separates those domains explicitly, and limits migration churn to a finite cutover window so future evolution remains additive instead of recursive.

## Goal

Create a schema boundary that is stable enough to support the current product while making the long-term lifecycle model legible to engineers, data consumers, and reviewers. The aim is not a bigger schema; it is a smaller set of durable ownership boundaries that reflect the actual domains the system already uses.

## Architecture decision record

- **Decision:** Freeze the current SQLite schema as a v1 compatibility layer, then rebuild the operational model as an explicit v2 schema with clean domain boundaries and a short cutover migration.
- **Status:** Proposed for review.

### Context

The current schema in [internal/db/schema.sql](../../internal/db/schema.sql) and the migration history under [internal/db/migrations](../../internal/db/migrations) show overlapping life cycles rather than one coherent model:

- capture inventory and file/session provenance,
- replay and benchmark cases,
- run records and per-run tracks,
- scene publication and asset metadata,
- sweep/optimization requests and results,
- site and configuration periods.

These domains are adjacent, but they are not identical. The schema currently mixes them in adjacent tables and shared IDs, which makes long-lived evolution harder than it needs to be. The risk is semantic drift rather than a single catastrophic bug: the DB is working, but its ownership model is ambiguous.

### Alternatives considered

| Option         | Approach                                                                                 | Verdict                                                                            |
| -------------- | ---------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------- |
| **1 (chosen)** | Freeze v1, create v2 with explicit domain tables, migrate once, keep compatibility views | Best long-term safety; one-time cleanup cost, but bounded and reviewable           |
| **2**          | Keep evolving the current schema in place                                                | Lowest immediate effort, but cumulative drift and semantic ambiguity will continue |
| **3**          | Split every subdomain into a fully independent database                                  | Too expensive and operationally heavy for a local SQLite deployment                |

## System boundary diagram

```text
+--------------------------------------------+
| Product surfaces                            |
|--------------------------------------------|
| radar ingest / web / API / PDFs / visualiser|
+-------------------------+----------------------+
                          |
                          v
+------------------------------------------------------+
| v1 compatibility layer (frozen)                     |
| - existing schema.sql / migration chain             |
| - read-compat view layer for legacy app queries     |
| - no new feature tables added to v1                 |
+-------------------------+----------------------------+
                          |
                          v
+------------------------------------------------------+
| v2 domain schema                                    |
|--------------------------------------------+---------+
| Capture domain | Replay domain | Run domain      |
|-------------- |---------------|-----------------|
| capture_roots | replay_cases  | run_records     |
| capture_files | replay_eval   | run_tracks      |
| capture_sessions | annotations | missed_regions  |
| motion_periods | benchmarks    | track_obs       |
+-------------------------+----------------------------+
| Scene domain | Sweep domain | Site domain         |
|--------------|--------------|----------------------|
| scenes       | sweep_runs   | site                |
| publications | sweep_results| config_periods      |
+------------------------------------------------------+
                          |
                          v
+------------------------------------------------------+
| Operational rules                                   |
| - one owner per lifecycle                           |
| - delete rules explicit                             |
| - compatibility views for old callers               |
| - no new v1 semantics after freeze                   |
+------------------------------------------------------+
```

## Proposed model

Freeze the schema contract as a compatibility layer; then introduce a clean v2 model with explicit domain ownership. The current product can keep reading through compatibility views while all new writes target v2 tables. This keeps the migration bounded and reviewable.

### v2 ownership boundaries

1. **Capture domain**
   - `capture_roots`
   - `capture_files`
   - `capture_sessions`
   - `capture_motion_periods`
   - `capture_jobs`
   - Purpose: inventory, provenance, and file/session lifecycle.

2. **Replay benchmark domain**
   - `replay_cases`
   - `replay_case_files`
   - `replay_evaluations`
   - `replay_annotations`
   - Purpose: benchmark definitions and manual/derived comparison metadata.

3. **Processing run domain**
   - `run_records`
   - `run_tracks`
   - `run_track_observations`
   - `run_missed_regions`
   - Purpose: the concrete run produced from a sensor capture or replay.

4. **Current-state domain**
   - `active_tracks`
   - `current_objects`
   - `track_observations_current`
   - Purpose: live operational state, not historical benchmark storage.

5. **Scene publication domain**
   - `scenes`
   - `scene_assets`
   - `scene_publications`
   - Purpose: user-facing publication and asset state.

6. **Sweep/optimization domain**
   - `sweep_runs`
   - `sweep_rounds`
   - `sweep_results`
   - `sweep_recommendations`
   - Purpose: optimization and auto-tuning outcomes.

7. **Site/config domain**
   - `site`
   - `site_config_periods`
   - `site_reports`
   - Purpose: geographic and operational configuration.

### Core design principles

- One lifecycle, one owner. A row should clearly belong to one durable domain and not be implicitly owned by a transient table.
- Stable semantics over historical compatibility. Once a domain is frozen, new code must use the v2 names and relationships.
- Compatibility views are transitional, not permanent architecture.
- Delete semantics must be explicit in the schema and reviewed by the code that writes them.

## Proposed schema shape

The v2 schema does not require a single giant table or custom JSON blob. It should prefer narrow, explicit tables with stable foreign keys and durable keys. The most important changes are conceptual rather than SQL-heavy:

- `capture_*` tables should not be used as a de facto storage for replay benchmark metadata.
- `lidar_replay_cases` should own benchmark metadata, not be a thin alias of a scene or capture session.
- `run_*` tables should own the processed results of a specific execution; they should not be overloaded with scene publication or sweep metadata.
- `scenes` should be publication assets, not a catch-all bucket for every benchmark case.
- `sweep_*` data should be separate from both replay case definitions and run results.

A practical v2 naming pattern is:

```sql
capture_roots
capture_files
capture_sessions
capture_motion_periods

replay_cases
replay_case_files
replay_evaluations
replay_annotations

run_records
run_tracks
run_track_observations
run_missed_regions

scenes
scene_assets
scene_publications

sweep_runs
sweep_rounds
sweep_results
sweep_recommendations

site
site_config_periods
site_reports
```

This keeps the domain names explicit and makes the schema easier to review in code and in the generated snapshot.

## Migration approach

### Phase 0: freeze v1

- Keep the current schema as the compatibility baseline.
- Mark it as the v1 contract in [internal/db/schema.sql](../../internal/db/schema.sql) and migration chain documentation.
- No new feature work adds columns or tables to v1 after the freeze point.
- Document the freeze in this plan and update any schema-ownership review notes.

### Phase 1: create v2 shadow schema

- Build a new set of v2 tables beside the v1 schema.
- Keep v1 tables intact so reads continue to work during the cutover.
- Add domain-level indexes and constraints once; do not rebuild the v1 chain again.
- Add compatibility views or translation adapters for the most common old query shapes.

### Phase 2: cutover window

- Update the read layer to prefer v2 tables.
- Use adapter queries or views to map old application callers to the new schema.
- Add a migration guard or startup check that fails fast if the v2 schema is missing or partially applied.
- Restrict writes to v2 for any newly introduced feature paths.

### Phase 3: remove v1 after validation

- Once reads and writes have stabilised, delete the obsolete v1 tables or archive them behind a final, explicit migration.
- Keep a final compatibility snapshot only for disaster recovery, not for general runtime use.

## Migration cost profile

This is the key tradeoff: a one-time v2 rebuild is expensive in migration work, but it avoids repeated incremental churn forever.

| Area                         | Expected cost | Notes                                                        |
| ---------------------------- | ------------- | ------------------------------------------------------------ |
| Schema design review         | Low           | A few architecture review cycles and one clear ownership map |
| Dual-schema implementation   | Medium        | Shadow v2 tables + compatibility layer                       |
| Data backfill and validation | Medium        | Requires explicit row mapping and integrity checks           |
| Compatibility view cleanup   | Medium        | Read-path translation and query regression checks            |
| Final v1 removal             | Low           | One clean delete migration after stabilization               |
| Long-term maintenance        | Very low      | Added work is bounded; future schema changes are additive    |

This is a finite migration cost. The alternative is continuous migration churn across overlapping domains, and that cost is harder to measure because it appears in every future change.

## Compatibility strategy

Compatibility should be a short-lived bridge, not a permanent architecture.

### Read compatibility

Keep a set of read-only views that expose the old table names or common query shapes to legacy callers while the application moves to v2. This reduces risk during the cutover, but the views should be explicitly marked as temporary.

### Write compatibility

Only allow write compatibility for a brief transition period. New code must not write to v1 tables once the v2 path is available. Enforce this in code review and via tests.

### Validation gates

Before v1 removal, run checks for:

- orphaned foreign keys,
- ambiguous ownership rows,
- duplicate benchmark keys,
- stale `site_id` or capture references,
- run-to-scene references that should have been separated,
- sweep rows that incorrectly point at run records instead of sweep tables.

## Failure registry

| Failure mode                    | User-visible effect                           | Recovery / guardrail                                                           |
| ------------------------------- | --------------------------------------------- | ------------------------------------------------------------------------------ |
| Partial v2 migration            | App reads from half-migrated domain tables    | Fail-fast startup check; migration status table or explicit verification step  |
| Legacy code still writes v1     | Data gets split across schemas                | Short write-compat window; CI enforcement against legacy write paths           |
| Ambiguous row ownership remains | Queries drift or records disappear            | Domain review gate before merge; ownership map embedded in schema names        |
| Backfill rejects rows           | cutover stalls                                | Quarantine invalid rows in a `*_legacy_*` table and audit before final cleanup |
| Delete semantics mismatch       | orphan rows or accidental cascade loss        | Make cascade targets explicit in v2; test delete paths before v1 removal       |
| Views become permanent          | architecture never converges                  | Put an expiry comment in the migration and a ticket for removal                |
| Schema snapshot drift           | generated schema and migration chain disagree | Keep the freeze point and migration validation checks in place for v2          |

## Review checklist

This plan is intentionally narrow and should be evaluated by a reviewer asking a simple question: does the schema reflect the real lifecycle boundaries of the system?

Reviewers should confirm:

- capture inventory is separate from replay benchmark definitions,
- run results are separate from scene publication,
- sweep and tuning remain separate from both,
- all references use durable keys,
- the migration can be executed once and then retired.

## Related materials

- [internal/db/schema.sql](../../internal/db/schema.sql)
- [internal/db/db.go](../../internal/db/db.go)
- [internal/db/migrations](../../internal/db/migrations)
- [docs/plans/lidar-schema-robustness-plan.md](lidar-schema-robustness-plan.md)
- [docs/plans/data-sqlite-client-standardisation-plan.md](data-sqlite-client-standardisation-plan.md)

## Conclusion

The right move is not to keep endlessly patching the current schema. The right move is to freeze the existing contract, make the lifecycle model explicit, and complete a single, reviewable v2 migration. That gives the team a stable database boundary and a clear route to future additive changes without reintroducing the same ambiguity.
