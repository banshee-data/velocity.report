# Speed Percentile Aggregation Alignment Plan

**Status:** Proposed - design reset documented; implementation and contract rework pending
**Scope:** reserve percentiles for grouped/report aggregates only, back out the old single-track speed-label proto/API work, rename raw `peak` to `max`, define replacement track-level speed metrics, and keep one canonical aggregate percentile path
**Related:** [Track Description Language plan](data-track-description-language-plan.md), [LiDAR Visualiser Proto Contract Plan](lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md), [v0.5.0 Backward Compatibility Shim Removal Plan](v050-backward-compatibility-shim-removal-plan.md), [Metrics Registry and Observability Plan](metrics-registry-and-observability-plan.md), [Executive Decisions Register](../DECISIONS.md), [radar percentile queries](../../internal/db/queries_histograms.md)

## 1. Problem

The repo currently mixes the same words and measures across two different
concepts:

- **Single-track speed summaries** for one observed vehicle pass.
- **Population-level percentiles** for many vehicles over a report, group, or
  aggregation period.

That overlap is the design error. A single-track field that reuses aggregate
percentile labels sounds like the same thing as a grouped report percentile,
but it is not. The public model
should not ship both. Percentiles need to be reserved for grouped/report
aggregates, while tracks use distinct non-percentile speed metrics.

## 2. Metric Inventory

### 2.1 Percentile and max usage

| Metric         | Live aggregate/report use                                                      | Live track/internal use                                      | Historical/legacy use                                                    | Direction                                                              |
| -------------- | ------------------------------------------------------------------------------ | ------------------------------------------------------------ | ------------------------------------------------------------------------ | ---------------------------------------------------------------------- |
| `p50`          | Radar stats API, PDF/reporting, web charts                                     | Misapplied in some LiDAR single-track work today             | None                                                                     | Keep only for grouped/report aggregates                                |
| `p85`          | Radar stats API, PDF/reporting, web charts                                     | Misapplied in some LiDAR single-track work today             | None                                                                     | Keep only for grouped/report aggregates                                |
| `p95`          | None for live speed reporting                                                  | None intended in current live speed path                     | Original LiDAR track / analysis-run schema and older docs/devlog entries | Historical only for speed; do not revive                               |
| `p98`          | Current high-speed percentile in radar stats, PDF/reporting, and planning docs | Misapplied in some LiDAR single-track work today             | Replaced earlier `p95` usage                                             | Keep as the high-end aggregate percentile if the repo keeps one        |
| `p100` / `max` | Represented as `max_speed` in radar/reporting/TDL/transits                     | Represented today as raw `peak_speed_mps` on tracks/proto/UI | None                                                                     | Rename raw `peak` to `max`; reserve future `peak` for filtered measure |

### 2.2 Why `p95` exists

`p95` did not come from the current reporting stack. It came from the original
LiDAR track schema and early track-analysis work:

- `internal/db/migrations/000010_create_lidar_tracks.up.sql` created
  a single-track `p95` speed column on `lidar_tracks`.
- `internal/db/migrations/000011_create_lidar_analysis_runs.up.sql` created
  a single-track `p95` speed column on `lidar_run_tracks`.
- The December 1, 2025 entry in `docs/DEVLOG.md` explicitly says per-track
  percentile-style speed summaries were computed.

That is historical context, not a current live requirement. The speed path later
standardised on `p98`:

- `internal/db/migrations/000030_rename_p95_to_p98_speed.up.sql` says the
  codebase standardises on `p98` and renames the DB columns accordingly.
- Current live aggregate/report code uses `p50/p85/p98/max`, not `p95`.

Conclusion: for **speed**, `p95` should not stay as the canonical high-end
percentile unless we intentionally decide to reintroduce it. Right now the repo
already points to `p98` as the standard high-end aggregate percentile, and
`p95` mostly survives as historical migration residue plus non-speed domains
like `height_p95` or latency `p95`.

## 3. Current State Inventory

| Surface                                                                     | Current state                                                                                                              | Status                                               |
| --------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------- |
| `internal/lidar/l6objects` + `internal/lidar/storage/sqlite/track_store.go` | Percentile-labelled single-track speed summaries are computed from track speed history and persisted today.                | Design debt to remove from the canonical track model |
| Visualiser proto/adapter                                                    | Branch-local work exposes percentile-labelled single-track speed summaries to gRPC/Swift clients.                          | Must be backed out before merge                      |
| `internal/lidar/monitor/track_api.go` per-track REST                        | Individual track JSON already exposes a percentile-labelled speed summary field.                                           | Must be backed out, not expanded                     |
| `internal/lidar/monitor/track_api.go` summary REST                          | Summary payloads contain a percentile-labelled placeholder field, but grouped percentile semantics are still incomplete.   | Needs redesign around aggregate-only percentiles     |
| `internal/api/server.go` + `internal/db/db.go` radar stats rollups          | Query-time `p50/p85/p98` are computed from raw speed rows in each bucket. This is the right aggregation level.             | Keep, but standardise algorithm/helper path          |
| `tools/pdf-generator/pdf_generator/cli/main.py` main report path            | Overall and daily summaries are fetched from API `group=all` / `group=24h`, so an aggregate-only path already exists.      | Keep                                                 |
| `tools/pdf-generator/pdf_generator/cli/main.py` fallback helpers            | `derive_overall_from_granular()` and `derive_daily_from_granular()` derive summaries from earlier bucket percentiles.      | Remove                                               |
| Planning/docs surface                                                       | Several active plans still say single-track speed surfaces should ship aggregate percentile labels in proto, REST, and UI. | Must be rewritten in this PR                         |

## 4. Decisions Already Settled

The high-level direction is now clear and should not be reopened:

1. **Percentiles are aggregate-only.**
   `p50/p85/p98` are reserved for report/group/aggregation-period outputs across
   a filtered population.
2. **For speed, the canonical high-end aggregate percentile is `p98`, not `p95`.**
   `p95` is historical LiDAR schema residue; current live aggregate/report code
   already standardises on `p98`.
3. **Raw maximum speed should be called `max`, not `peak`.**
   The current raw track maximum (`peak_speed_mps`) should be renamed to
   `max_speed_mps`; the name `peak` should be reserved for a future outlier-
   filtered or context-aware top-speed measure.
4. **Track-level speed summaries must use different terminology and different measures.**
   A single track should not expose aggregate percentile labels; it needs its own metrics such
   as a robust typical speed and a reliable peak speed.
5. **The unmerged single-track speed-label proto/API work should be backed out, not expanded.**
   This includes branch-local `Track` proto fields and any plan items that tell
   clients to adopt them.
6. **No percentile-of-percentile rollups.**
   A canonical aggregate summary cannot be derived from prior bucket
   `p50/p85/p98` outputs.

## 5. Decisions That Still Need Explicit Implementation Choices

| Decision                                                  | Why it matters                                                                                                                                | Recommended resolution                                                                                                                                                                                                                                      |
| --------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Names for replacement track metrics                       | Tracks still need public speed summaries, but aggregate percentile labels can no longer be reused for those names.                            | Use working names such as `typical_observed_speed_mps` and `reliable_peak_speed_mps` until the final naming review is done, then record the final ids/aliases in the [metrics registry and observability plan](metrics-registry-and-observability-plan.md). |
| Formula for track "typical" speed                         | A raw running mean is easy to compute but may not match "typical observed speed" if the track has startup/shutdown noise or occlusion spikes. | Define a robust central-speed measure over well-observed portions of the track, with explicit outlier rejection and smoothing rules.                                                                                                                        |
| Formula for track "reliable peak" speed                   | A single-frame max is too sensitive to noise, but the UI still needs a notion of top speed for one track.                                     | Define a context-aware peak metric that rejects implausible spikes and can use temporal/spatial consistency checks.                                                                                                                                         |
| Migration strategy for existing internal pXX calculations | Some internal code still computes/stores percentile-labelled single-track summaries today.                                                    | Stop public exposure first, keep any temporary internal use isolated, and remove the internal percentile calculations once the new track metrics land.                                                                                                      |
| Canonical aggregate percentile algorithm                  | Aggregate/report percentiles still need one documented implementation path.                                                                   | Standardise on one shared helper for dataset-level `p50/p85/p98`, document the indexing rule in code/tests, and migrate radar/reporting to it.                                                                                                              |
| `peak` -> `max` migration shape                           | We need to free the word `peak` for a future filtered metric without losing the current raw maximum.                                          | Rename the current raw field to `max_speed_mps` before merge where contracts are still unshipped; if a filtered `peak_speed_mps` is later added to proto/API, give it a new field number and clear docs.                                                    |

## 6. Execution Plan

### Phase 0 - Documentation and Decision Reset

- [x] Inventory current percentile producers/consumers and the unresolved aggregation boundaries.
- [x] Record the governing semantic decision in [../DECISIONS.md](../DECISIONS.md).
- [x] Update the backlog item so it reflects track metric redesign plus aggregate-only percentiles.
- [x] Clean up current planning docs that still say `p95` where the repo now standardises on `p98`.
- [x] Mark earlier proto/API expansion plans for single-track aggregate-percentile labels as superseded.

### Phase 1 - Back Out The Wrong Public Contract

- [ ] Remove branch-local aggregate-percentile label additions from the `Track` proto, generated bindings, visualiser model/UI, and any new REST contract work tied to them.
- [ ] Stop expanding per-track REST payloads with percentile fields; if a branch-local percentile field exists, remove it before merge.
- [ ] Rename the current raw public `peak_speed_mps` field to `max_speed_mps` before merge where the contract is still unshipped.
- [ ] Update tests and docs so aggregate percentile labels appear only on grouped/report surfaces.

### Phase 2 - Track Metric Redesign

- [ ] Define the replacement public track metrics and their names.
- [ ] Reserve `peak_speed_mps` for the future filtered/context-aware top-speed measure, not the raw maximum.
- [ ] Specify how those metrics reject outliers and use expected temporal/spatial behaviour.
- [ ] Decide which track-level speed metrics remain public API and which stay internal to classification/evaluation.

### Phase 3 - Remove Per-Track Percentile Calculations

- [ ] Replace branch-local/internal dependencies on legacy single-track speed-summary outputs with the new track metrics or other internal descriptors.
- [ ] Stop treating track-level aggregate-percentile labels as canonical stored fields in new APIs and schemas.
- [ ] Migrate public/raw `peak_speed_mps` references to `max_speed_mps` so "peak" is available for the future filtered metric.
- [ ] Plan migration for historical storage surfaces (`lidar_tracks`, analysis runs, tests) that still carry legacy single-track speed-summary columns today.

### Phase 4 - Aggregate-Only Percentile Path

- [ ] Add a shared Go helper for dataset-level `p50/p85/p98` from a scalar speed slice, with one documented indexing rule.
- [ ] Switch `internal/db.RadarObjectRollupRange` and report consumers to the shared helper so aggregate surfaces use the same algorithm.
- [ ] Remove or fence off `derive_overall_from_granular()` and `derive_daily_from_granular()` as non-canonical fallbacks.
- [ ] Add per-transit `max_speed_mph` to the fused transit layer/query path.
- [ ] Serve TDL `speed summary` from filtered transit max speeds using the shared helper.
- [ ] Move report consumers to fused transit summaries when they can replace source-specific radar/LiDAR rollups.

## 7. Acceptance Criteria

- No track-level public field, proto property, or UI label reuses aggregate percentile labels.
- No raw maximum field is publicly named `peak`; raw maxima are named `max`.
- Track-level speed summaries use distinct non-percentile metrics.
- One documented percentile algorithm exists across radar, LiDAR, and reporting.
- No report summary is derived from prior percentile buckets.
- API and docs clearly reserve percentiles for grouped/report aggregates only.
