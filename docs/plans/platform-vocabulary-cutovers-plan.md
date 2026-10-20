# Vocabulary cutovers: clip, labels and sites, each in one release

- **Status:** Proposed. Expands option b of P4 in the [architecture review](platform-architecture-review-plan.md#p4-rename-by-alias-programme); the delivery plan for the vocabulary programme's naming work
- **Layers:** Cross-cutting (SQLite schema, Go stores and APIs, web, macOS, CLI, docs)
- **Target:** clip v0.5.9, labels v0.5.10, sites v0.6.6
- **Decisions:** D-20 (no compatibility shims) and the review's consumer test; ADR-2 of the review
- **Companion plans:** [platform-vocabulary-and-data-model-plan](platform-vocabulary-and-data-model-plan.md) (the vocabulary, decisions V1 to V36 and the ledger, which this plan delivers from), [platform-decision-keeping-plan](platform-decision-keeping-plan.md) (the compatibility register), [platform-binary-roles-plan](platform-binary-roles-plan.md) (the frame-model move that precedes the Go renames), [lidar-ui-workflow-plan](lidar-ui-workflow-plan.md)
- **Canonical:** [PLATFORM.md](../platform/PLATFORM.md)

The vocabulary programme is right about the problem and its nouns. This plan changes how its
names reach the code: three cutovers, each one PR series in one release, in which every caller
inside the release moves with the rename, and an old name survives only where D-20's consumer
test says something outside the release still needs it.

## Motivation

The programme planned eighteen items, each adding aliases, and a retirement release two minor
versions later. None has started. While it waited, migration 039 gave the table name
`lidar_scenes` to a second concept, and one concept now carries four names across four tiers:
the nav says Clips, the web route is `replay-cases`, the API is `/api/lidar/scenes`, the Go type
is `ReplayCase` and the table is `lidar_replay_cases`. D-20 already decided against temporary
dual formats. A rename that moves every caller at once is smaller than the alias programme, not
larger: the aliases, their tests and their retirement are the work it removes.

## Current state

| Concept | Names today                                                                                                                                                                                                         | Size of the rename                                                                                      |
| ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| Clip    | "Clips" (nav), `/lidar/replay-cases` (web route), `/api/lidar/scenes`, `ReplayCase`, `lidar_replay_cases`, `replay_case_id`; runs are `AnalysisRun` on `lidar_run_records`                                          | 1,102 `ReplayCase` or `replay_case` hits in 40 Go files; 305 in the web; 52 in Swift; 669 `AnalysisRun` |
| Label   | Track labels in `lidar_replay_annotations`, served as `/api/lidar/labels`, also in `lidar_run_tracks.user_label`; five write paths; an unused `LabelStore` beside 8 raw SQL calls in `internal/api/lidar_labels.go` | One table, one write path, three type names in three languages                                          |
| Site    | Thirteen identities, two tables that never join (`site` with an integer key and report fields, `lidar_sites` with an S2 token key), a public page slug and an archive slug                                          | One `sites` table and `deployments`, with a precedence rule (V14)                                       |

The macOS app, which ships as its own DMG, calls `/api/lidar/labels`, `/api/lidar/runs/*`,
`/api/lidar/vrlog/*`, `/api/lidar/pcap/stop` and nine `/api/annotations/*` routes, and reads
`replay_case_id` from run JSON. It is the only consumer of these surfaces outside the release.

## Design / approach

### What a cutover is

One PR series, merged in one release, that renames a concept everywhere the release carries it:

| Surface                                            | Treatment                                                                                                                          |
| -------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| Tables and columns                                 | A migration renames them; stored values, keys and row counts do not change                                                         |
| Go types, packages, files, handlers, error strings | Renamed in the same series, no type aliases                                                                                        |
| Web routes, API clients, copy                      | Renamed with the API; a moved web page redirects its old path for a release, since a person may have bookmarked it                 |
| CLI commands and flags                             | Renamed; the old spelling is a tombstone that prints the new one                                                                   |
| Routes and JSON keys the macOS app reads           | Renamed, with the old ones kept as registered aliases until the next DMG is the oldest supported; the app moves in the same series |
| Docs, skills, scripts, Make targets                | Renamed in the same series                                                                                                         |

Each cutover's register rows are added to the compatibility register, and the expiry test removes
them on time. No cutover adds feature work: folding selections into clips, merging site
identities beyond the precedence rule, or new survey relationships ride on their own items.

### Order

Clip first: it is the most-used noun, the frame-model move clears its Go path, and the run
browser's Clip column waits on it. Labels second: one write path fixes a live data hazard (five
writers, and deleting a clip cascades to reviewed labels). Sites last: it is the largest and is
tied to surveys and public publication in v0.6.6.

## Scope

### Cutover 1: clip (programme Item 2, and the clip part of Items 3 and 6)

**Summary:** clips are called clips in every layer, and runs are runs.

**Steps:**

1. Migration: `lidar_replay_cases` to `lidar_clips`, `lidar_replay_case_files` to `lidar_clip_captures`, `lidar_replay_evaluations` to `lidar_clip_evaluations`, `lidar_run_records` to `lidar_runs`, `replay_case_id` to `clip_id`; the `pcap_file` projection dropped once its readers use the clip's capture rows.
2. Go: `ReplayCase` to `Clip`, `AnalysisRun` to `Run`, the store and API files renamed; `/api/lidar/scenes` becomes `/api/lidar/clips`, with no alias, since only the web app calls it.
3. Wire value: the data source's `pcap` becomes `capture`; the macOS app's two files move, and `pcap` stays as a registered alias for the app until the next DMG.
4. Run JSON: `clip_id` beside `replay_case_id` for the app until the next DMG, registered.
5. Web: `/lidar/replay-cases` becomes `/lidar/clips`, with the old path redirecting for a release.
6. Docs, skills, scripts and the Make targets that name cases; the stop aliases move to `/replay/stop` in the same series.

**Acceptance:** a copy of the development database migrates with identical row counts and
relationships; a fresh and a migrated database expose the same schema; outside migrations and
history, a grep for `replay_case`, `ReplayCase` and `replay-cases` finds only registered rows; the
current macOS app and the previous DMG both list runs and stop a replay against the new server.

**Milestone:** v0.5.9

### Cutover 2: labels (programme Item 7)

**Summary:** one track label table and one write path.

**Steps:**

1. Migration: `lidar_track_labels` from `lidar_replay_annotations` and the run-track label columns, with explicit row accounting; the `split` and `merge` tag labels become quality flags.
2. One write path through the run-track route and `LabelStore`; the raw SQL in `internal/api/lidar_labels.go` goes.
3. `/api/lidar/labels` writes and `/api/annotations/*` stay as registered aliases for the macOS app until the next DMG; the app moves in the same series.
4. `ObjectClass` and `QualityFlag` as the type names in Go, TypeScript and Swift.
5. Decide, before the migration, what deleting a clip does to its labels; today it cascades and deletes reviewed labels.

**Acceptance:** every label reads back identically through every path; the export lists each
label once; the clip-delete behaviour is the decided one and tested.

**Milestone:** v0.5.10

### Cutover 3: sites (programme Item 14)

**Summary:** one site and one deployment row per mounting, for both sensors.

**Steps:**

1. Migration: `sites` merged from `site` and `lidar_sites` under V14's precedence rule, coordinate conflicts set aside rather than averaged; `deployments` from `site_config_periods`.
2. `/api/sites` and `/api/deployments`; `/api/lidar/scene-map` and `/api/lidar/sites/{token}` folded in.
3. Public scene pages keep their URLs or redirect permanently; a redirect is not a shim.
4. Decide, before the migration, what deleting a site does to its deployments; today it cascades.

**Acceptance:** every former `site` and `lidar_sites` row maps to one `sites` row or a recorded
reject; a report's cosine correction and a run's calibration read from the same deployment.

**Milestone:** v0.6.6, with the survey work

## What changes in the programme

| Programme plan                                                           | After this plan                                                                                                                                                 |
| ------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [P1-A contract](platform-vocabulary-phase1-contract-plan.md)             | Superseded: the vocabulary is published in the programme plan; the decision it names is recorded under a new number, because D-28 went to the research briefing |
| [P1-B LiDAR terminology](platform-vocabulary-phase1-lidar-plan.md)       | Superseded by cutovers 1 and 2                                                                                                                                  |
| [P1-C radar terminology](platform-vocabulary-phase1-radar-plan.md)       | Parked: radar renames wait for a consumer                                                                                                                       |
| [P1-D interfaces](platform-vocabulary-phase1-interfaces-plan.md)         | Parked: CLI and flag renames ride on the cutover that needs them, as tombstones                                                                                 |
| [P2-A captures](platform-vocabulary-phase2-captures-plan.md)             | Unchanged: capture digests are feature work under D-27                                                                                                          |
| [P2-B clips and labels](platform-vocabulary-phase2-clips-labels-plan.md) | Label consolidation moves to cutover 2; selection folding and the candidates panel stay there as feature work after cutover 1                                   |
| [P2-C jobs](platform-vocabulary-phase2-jobs-plan.md)                     | Superseded by the one-queue revision of the [job queue plan](lidar-job-queue-implementation-plan.md)                                                            |
| [P2-D radar](platform-vocabulary-phase2-radar-plan.md)                   | Unchanged: feature work                                                                                                                                         |
| [P2-E VRLOG and pose](platform-vocabulary-phase2-vrlog-pose-plan.md)     | Unchanged: feature work                                                                                                                                         |
| [P2-F sites and surveys](platform-vocabulary-phase2-surveys-plan.md)     | Sites and deployments move to cutover 3; surveys, reports and publication stay there                                                                            |
| [P2-G retirement](platform-vocabulary-phase2-retirement-plan.md)         | Superseded by the compatibility register and its expiry test                                                                                                    |

## Dependencies

- The frame-model move in the binary roles plan precedes cutover 1's Go renames.
- The compatibility register precedes the first registered alias.
- Cutover 3 precedes the survey export and every map item; it waits for the surveyed coordinates the scene-publishing items already wait on.

## Risks

| Risk                                              | Likelihood | Impact | Mitigation                                                                         |
| ------------------------------------------------- | ---------- | ------ | ---------------------------------------------------------------------------------- |
| A large rename collides with perception branches  | High       | Medium | Land each cutover early in its release; mechanical renames; one series per concept |
| An older macOS app breaks against a newer server  | Medium     | Medium | Registered aliases and dual JSON keys until the next DMG is the oldest supported   |
| A migration loses or merges rows silently         | Low        | High   | Row-accounting tests against a copy of the development database before merge       |
| An operator's bookmark or script uses an old name | Medium     | Low    | Web redirects and CLI tombstones for one release                                   |

## Checklist

### Complete

- [x] Consumers outside the release identified: the macOS app's routes and JSON keys

### Outstanding

- [ ] Cutover 1: clip (`L`)
- [ ] Cutover 2: labels (`M`)
- [ ] Cutover 3: sites (`L`)
- [ ] Programme sub-plan statuses updated as above (`S`)

### Deferred

- [ ] Radar nouns, CLI and flag renames, VRLOG nouns: each waits for a consumer that needs the new name

### Accepted residuals (no action planned)

- [ ] "Scene" keeps its geometry sense in L3, L7 and the vector scene map
