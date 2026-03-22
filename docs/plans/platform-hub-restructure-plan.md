# Hub Restructure Plan

- **Status:** Active
- **Canonical:** [canonical-plan-graduation.md](../platform/architecture/canonical-plan-graduation.md)
- **Layers:** Cross-cutting (documentation)
- **Parent:** [platform-canonical-project-files-plan.md](platform-canonical-project-files-plan.md)

## Summary

Establish four hub directories under `docs/` as the long-term canonical homes for
all substantial project documentation. Collapse the previously proposed `docs/server/`
and `docs/engineering/` concepts into a single `docs/platform/` hub covering both
shared codebase structure and development methodology.

## Hub Structure

Five hubs total. Three existing, one new, one renamed.

| Hub              | Scope      | Sorting test                                                                                                                               |
| ---------------- | ---------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| `docs/lidar/`    | EXISTS     | LiDAR pipeline layer, algorithm, sensor-specific workflow, or LiDAR-specific storage                                                       |
| `docs/radar/`    | EXISTS     | Radar sensing, speed measurement, site configuration                                                                                       |
| `docs/ui/`       | EXISTS     | What a user sees or interacts with — web, macOS app chrome, homepage, design language                                                      |
| `docs/platform/` | **EXPAND** | Shared codebase structure (Go packages, DB schema, deployment, release) AND development methodology (docs rules, metrics, tooling, agents) |

### Mutual Exclusivity Test

Ask in order. First match wins.

1. Does the lasting knowledge describe a LiDAR pipeline layer, algorithm, sensor-specific
   workflow, or LiDAR-specific storage? → `docs/lidar/`
2. Does it describe radar sensing, speed measurement, or site configuration? → `docs/radar/`
3. Does it describe what a user sees or interacts with? → `docs/ui/`
4. Everything else → `docs/platform/`

## Canonical File Tree

`EXISTS` = already in the repo. `NEW` = needs creating. `MOVE` = relocating from `docs/server/`.

Plans that feed multiple canonical docs are marked with `†`.

### `docs/lidar/` — 38 plans

#### `docs/lidar/architecture/`

| Canonical file                                           | Status | Fed by plan(s)                                        | Notes                                 |
| -------------------------------------------------------- | ------ | ----------------------------------------------------- | ------------------------------------- |
| `lidar-data-layer-model.md`                              | EXISTS | `lidar-layer-dependency-hygiene-plan`                 | Layer ownership rules absorb          |
| `lidar-pipeline-reference.md`                            | EXISTS | `lidar-architecture-graph-plan`                       | Graph becomes asset of this doc       |
| `lidar-sidecar-overview.md`                              | EXISTS | —                                                     | Already canonical                     |
| `foreground-tracking.md`                                 | EXISTS | `lidar-bodies-in-motion-plan`                         | Kinematic prediction extends tracking |
| `vector-scene-map.md`                                    | EXISTS | `lidar-l7-scene-plan`                                 | L7 world model merges                 |
| `ground-plane-extraction.md`                             | EXISTS | —                                                     | Already canonical                     |
| `coordinate-flow-audit.md`                               | EXISTS | —                                                     | Already canonical                     |
| `lidar-background-grid-standards.md`                     | EXISTS | —                                                     | Already canonical                     |
| `network-configuration.md`                               | EXISTS | —                                                     | Already canonical                     |
| `gps-ethernet-parsing.md`                                | EXISTS | —                                                     | Already canonical                     |
| `ml-solver-expansion.md`                                 | EXISTS | `lidar-ml-classifier-training-plan`                   | Training plan merges                  |
| `multi-model-ingestion-and-configuration.md`             | EXISTS | —                                                     | Already canonical                     |
| `av-range-image-format-alignment.md`                     | EXISTS | `lidar-av-lidar-integration-plan`                     | AV integration merges                 |
| `arena-go-deprecation-and-layered-type-layout-design.md` | EXISTS | —                                                     | Already canonical                     |
| `math-foundations-audit.md`                              | EXISTS | —                                                     | Already canonical                     |
| `l8-l9-l10-migration-notes.md`                           | EXISTS | `lidar-l8-analytics-l9-endpoints-l10-clients-plan`    | Upper-layer refactor merges           |
| `lidar-logging-stream-split-and-rubric-design.md`        | EXISTS | —                                                     | Already canonical                     |
| `l2-dual-representation.md`                              | NEW    | `lidar-l2-dual-representation-plan`                   | Polar/Cartesian dual storage          |
| `velocity-foreground-extraction.md`                      | NEW    | `lidar-velocity-coherent-foreground-extraction-plan`  | Alternative FG algorithm              |
| `pluggable-algorithm-selection.md`                       | NEW    | `lidar-architecture-dynamic-algorithm-selection-plan` | Algorithm registry                    |
| `distributed-sweep.md`                                   | NEW    | `lidar-distributed-sweep-workers-plan`                | Multi-machine sweep                   |
| `track-storage-consolidation.md`                         | NEW    | `lidar-tracks-table-consolidation-plan`               | Live/analysis merge                   |
| `label-vocabulary.md`                                    | NEW    | `label-vocabulary-consolidation-plan`                 | Unified L6 classification             |

#### `docs/lidar/operations/`

| Canonical file                         | Status | Fed by plan(s)                                      | Notes                          |
| -------------------------------------- | ------ | --------------------------------------------------- | ------------------------------ |
| `auto-tuning.md`                       | EXISTS | `lidar-parameter-tuning-optimisation-plan`          | Parameter search merges        |
| `hint-sweep-mode.md`                   | EXISTS | `lidar-sweep-hint-mode-plan`                        | Already complete               |
| `track-labelling-auto-aware-tuning.md` | EXISTS | `lidar-track-labelling-auto-aware-tuning-plan`      | Labelling workflow merges      |
| `performance-regression-testing.md`    | EXISTS | `lidar-performance-measurement-harness-plan`        | Timing harness merges          |
| `pcap-analysis-mode.md`                | EXISTS | `lidar-analysis-run-infrastructure-plan`            | Run infrastructure merges      |
| `settling-time-optimisation.md`        | EXISTS | —                                                   | Already canonical              |
| `foundations-fixit-progress.md`        | EXISTS | `lidar-architecture-foundations-fixit-plan`         | Fixit status merges            |
| `scene-management-implementation.md`   | EXISTS | —                                                   | Already canonical              |
| `immutable-run-config.md`              | NEW    | `lidar-immutable-run-config-asset-plan`             | Deterministic config + lineage |
| `clustering-diagnostics.md`            | NEW    | `lidar-clustering-observability-and-benchmark-plan` | Per-frame diagnostics          |
| `test-corpus.md`                       | NEW    | `lidar-test-corpus-plan`                            | Five-PCAP validation dataset   |
| `observability-surfaces.md`            | NEW    | `hint-metric-observability-plan`                    | Scoring quality for labeller   |
| `static-pose-alignment.md`             | NEW    | `lidar-static-pose-alignment-plan`                  | Hesai 7DOF (deferred)          |
| `motion-capture.md`                    | NEW    | `lidar-motion-capture-architecture-plan`            | Moving sensor (deferred)       |
| `data-completeness-remediation.md`     | NEW    | `unpopulated-data-structures-remediation-plan`      | Wire computed data to storage  |

#### `docs/lidar/operations/visualiser/` (new subdirectory)

Twelve visualiser QC plans describe lidar-specific QC features. Grouped in a
subdirectory to keep `operations/` navigable.

| Canonical file                        | Status | Fed by plan(s)                                                        |
| ------------------------------------- | ------ | --------------------------------------------------------------------- |
| `qc-enhancements-overview.md`         | NEW    | `lidar-visualiser-labelling-qc-enhancements-overview-plan`            |
| `track-quality-scoring.md`            | NEW    | `lidar-visualiser-track-quality-score-plan`                           |
| `track-event-timeline.md`             | NEW    | `lidar-visualiser-track-event-timeline-bar-plan`                      |
| `split-merge-repair.md`               | NEW    | `lidar-visualiser-split-merge-repair-workbench-plan`                  |
| `physics-checks.md`                   | NEW    | `lidar-visualiser-physics-checks-and-confirmation-gates-plan`         |
| `trails-and-uncertainty.md`           | NEW    | `lidar-visualiser-trails-and-uncertainty-visualisation-plan`          |
| `priority-review-queue.md`            | NEW    | `lidar-visualiser-priority-review-queue-plan`                         |
| `qc-dashboard-and-audit.md`           | NEW    | `lidar-visualiser-qc-dashboard-and-audit-export-plan`                 |
| `run-list-labelling-rollup.md`        | NEW    | `lidar-visualiser-run-list-labelling-rollup-icon-plan`                |
| `light-mode.md`                       | NEW    | `lidar-visualiser-light-mode-plan`                                    |
| `proto-contract.md`                   | NEW    | `lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan`        |
| `performance-and-timeline-metrics.md` | NEW    | `lidar-visualiser-performance-and-scene-health-timeline-metrics-plan` |

### `docs/radar/` — 2 plans

| Canonical file                                       | Status | Fed by plan(s)                                  | Notes                            |
| ---------------------------------------------------- | ------ | ----------------------------------------------- | -------------------------------- |
| `architecture/time-partitioned-data-tables.md`       | EXISTS | —                                               | Already canonical                |
| `architecture/networking.md`                         | EXISTS | —                                               | Already canonical                |
| `architecture/serial-configuration-ui.md`            | EXISTS | —                                               | Already canonical                |
| `architecture/site-config-cosine-correction-spec.md` | EXISTS | —                                               | Already canonical                |
| `architecture/speed-limit-schedules.md`              | EXISTS | —                                               | Already canonical                |
| `architecture/transit-deduplication.md`              | EXISTS | —                                               | Already canonical                |
| `architecture/percentile-aggregation-semantics.md`   | NEW    | `speed-percentile-aggregation-alignment-plan` † | Percentile = aggregate not track |

### `docs/ui/` — 6 plans

| Canonical file                          | Status | Fed by plan(s)                                | Notes                                |
| --------------------------------------- | ------ | --------------------------------------------- | ------------------------------------ |
| `DESIGN.md`                             | EXISTS | —                                             | Already canonical                    |
| `design-review-and-improvement.md`      | EXISTS | —                                             | Already canonical                    |
| `velocity-visualiser-architecture.md`   | EXISTS | `server-manager`                              | Connection lifecycle merges          |
| `velocity-visualiser-implementation.md` | EXISTS | `web-frontend-background-debug-surfaces-plan` | Background debug surfaces merge      |
| `velocity-visualiser-app/*`             | EXISTS | —                                             | Already canonical (5 files)          |
| `web-frontend-consolidation.md`         | NEW    | `web-frontend-consolidation-plan` †           | Port consolidation + SPA unification |
| `homepage.md`                           | NEW    | `homepage-responsive-gif-strategies`          | Static site responsive media         |
| `macos-menu-layout-design.md`           | NEW    | `wireshark-menu-alignment`                    | macOS menu layout design             |

### `docs/platform/` — 22 plans (was engineering + platform)

#### `docs/platform/architecture/`

| Canonical file                  | Status                   | Fed by plan(s)                                                            | Notes                                   |
| ------------------------------- | ------------------------ | ------------------------------------------------------------------------- | --------------------------------------- |
| `database-sql-boundary.md`      | MOVE from `docs/server/` | `data-database-alignment-plan`, `data-sqlite-client-standardisation-plan` | Two-package DB access model             |
| `track-description-language.md` | MOVE from `docs/server/` | `data-track-description-language-plan`                                    | Query language design                   |
| `go-package-structure.md`       | NEW                      | `go-codebase-structural-hygiene-plan` †, `go-god-file-split-plan`         | Import boundaries + file size           |
| `structured-logging.md`         | NEW                      | `go-structured-logging-plan`                                              | Three-stream logging model              |
| `typed-uuid-prefixes.md`        | NEW                      | `platform-typed-uuid-prefixes-plan`                                       | Cross-package ID convention             |
| `tictactail-library.md`         | NEW                      | `tictactail-platform-plan`                                                | Generic streaming aggregation           |
| `metrics-registry.md`           | NEW                      | `metrics-registry-and-observability-plan` †                               | Naming rules + lifecycle                |
| `canonical-plan-graduation.md`  | NEW                      | `platform-canonical-project-files-plan`                                   | This graduation model (self-graduating) |

#### `docs/platform/operations/`

| Canonical file                  | Status | Fed by plan(s)                                                                   | Notes                             |
| ------------------------------- | ------ | -------------------------------------------------------------------------------- | --------------------------------- |
| `distribution-packaging.md`     | NEW    | `deploy-distribution-packaging-plan`                                             | Single-binary subcommand model    |
| `rpi-imager.md`                 | NEW    | `deploy-rpi-imager-fork-plan`                                                    | Image building + flashing         |
| `schema-migration-030.md`       | NEW    | `schema-simplification-migration-030-plan`                                       | Column/table renaming (transient) |
| `v050-release-migration.md`     | NEW    | `v050-backward-compatibility-shim-removal-plan`, `v050-tech-debt-removal-plan`   | Both v0.5.0 plans merge           |
| `documentation-standards.md`    | NEW    | `platform-documentation-standardisation-plan`, `line-width-standardisation-plan` | Doc metadata + line width merge   |
| `quality-coverage.md`           | NEW    | `platform-quality-coverage-improvement-plan`                                     | Coverage thresholds               |
| `data-science-methodology.md`   | NEW    | `platform-data-science-metrics-first-plan`                                       | Reproducibility principles        |
| `python-venv.md`                | NEW    | `tooling-python-venv-consolidation-plan`                                         | Shared root venv model            |
| `pdf-reporting.md`              | NEW    | `pdf-go-chart-migration-plan` †, `pdf-latex-precompiled-format-plan`             | Chart generation + TeX vendoring  |
| `agent-preparedness.md`         | NEW    | `agent-claude-preparedness-review-plan`                                          | AI agent knowledge architecture   |
| `simplification-deprecation.md` | NEW    | `platform-simplification-and-deprecation-plan`                                   | Tech debt reduction register      |

## Plans That Split Across Hubs

| Plan                                          | Concept A → Hub                                                                 | Concept B → Hub                                                     |
| --------------------------------------------- | ------------------------------------------------------------------------------- | ------------------------------------------------------------------- |
| `go-codebase-structural-hygiene-plan`         | SQL boundary → `platform/architecture/database-sql-boundary.md`                 | Package structure → `platform/architecture/go-package-structure.md` |
| `speed-percentile-aggregation-alignment-plan` | Percentile semantics → `radar/architecture/percentile-aggregation-semantics.md` | Metric naming → `platform/architecture/metrics-registry.md`         |
| `pdf-go-chart-migration-plan`                 | All concepts stay in `platform/operations/pdf-reporting.md`                     | (phases, not separate hubs)                                         |
| `web-frontend-consolidation-plan`             | All concepts stay in `ui/web-frontend-consolidation.md`                         | Port consolidation noted in platform                                |

## Immediate Actions

These changes apply to work already done on branch `dd/docs/merge-canonical`:

1. Rename `docs/server/` → move contents to `docs/platform/`
   - `docs/server/architecture/database-sql-boundary.md` → `docs/platform/architecture/database-sql-boundary.md`
   - `docs/server/architecture/track-description-language.md` → `docs/platform/architecture/track-description-language.md`
   - Delete `docs/server/README.md` and `docs/server/`
2. Update `docs/platform/README.md` — expand scope to cover both codebase structure and methodology
3. Update `Canonical:` links in the 3 data-\* plans from `../server/` → `../platform/`
4. Update `ALLOWED_HUB_PREFIXES` in `scripts/check-plan-canonical-links.py` — remove `docs/server/`
5. Verify `make report-plan-hygiene` still shows 65 G1 + 1 G4

## Counts

| Hub              | Existing docs | New canonical docs | Plans absorbed | % of plans |
| ---------------- | ------------- | ------------------ | -------------- | ---------- |
| `docs/lidar/`    | ~42           | ~18                | 38             | 56%        |
| `docs/radar/`    | 8             | 1                  | 2              | 3%         |
| `docs/ui/`       | 9             | 3                  | 6              | 9%         |
| `docs/platform/` | 1             | 19                 | 22             | 32%        |

## Implementation Sequence

1. Land this plan and the immediate actions (section above)
2. Continue Phase 2 of the parent plan: add `Canonical` metadata to all 68 plans
3. Create NEW canonical docs as stubs when adding `Canonical` links
4. Merge durable content from plans into canonical docs
5. Graduate superseded plans to symlinks
6. Hard-fail CI (Phase 3 of parent plan)
