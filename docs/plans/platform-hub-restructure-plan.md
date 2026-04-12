# Hub restructure plan

- **Status:** Active
- **Canonical:** [canonical-plan-graduation.md](../platform/architecture/canonical-plan-graduation.md)
- **Layers:** Cross-cutting (documentation)
- **Parent:** [platform-canonical-project-files-plan.md](platform-canonical-project-files-plan.md)

## Summary

Establish four hub directories under `docs/` as the long-term canonical homes for
all substantial project documentation. Collapse the previously proposed `docs/server/`
and `docs/engineering/` concepts into a single [docs/platform/](../platform) hub covering both
shared codebase structure and development methodology.

## Hub structure

Five hubs total. Three existing, one new, one renamed.

| Hub                           | Scope      | Sorting test                                                                                                                               |
| ----------------------------- | ---------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| [docs/lidar/](../lidar)       | EXISTS     | LiDAR pipeline layer, algorithm, sensor-specific workflow, or LiDAR-specific storage                                                       |
| [docs/radar/](../radar)       | EXISTS     | Radar sensing, speed measurement, site configuration                                                                                       |
| [docs/ui/](../ui)             | EXISTS     | What a user sees or interacts with: web, macOS app chrome, homepage, design language                                                       |
| [docs/platform/](../platform) | **EXPAND** | Shared codebase structure (Go packages, DB schema, deployment, release) AND development methodology (docs rules, metrics, tooling, agents) |

### Mutual exclusivity test

Ask in order. First match wins.

1. Does the lasting knowledge describe a LiDAR pipeline layer, algorithm, sensor-specific
   workflow, or LiDAR-specific storage? → [docs/lidar/](../lidar)
2. Does it describe radar sensing, speed measurement, or site configuration? → [docs/radar/](../radar)
3. Does it describe what a user sees or interacts with? → [docs/ui/](../ui)
4. Everything else → [docs/platform/](../platform)

## Canonical file tree

`EXISTS` = already in the repo. `NEW` = needs creating. `MOVE` = relocating from `docs/server/`.

Plans that feed multiple canonical docs are marked with `†`.

### [docs/lidar/](../lidar): 38 plans

#### [docs/lidar/architecture/](../lidar/architecture)

| Canonical file                                           | Status | Fed by plan(s)                                        | Notes                                 |
| -------------------------------------------------------- | ------ | ----------------------------------------------------- | ------------------------------------- |
| `LIDAR_ARCHITECTURE.md`                                  | EXISTS | `lidar-layer-dependency-hygiene-plan`                 | Layer ownership rules absorb          |
| `lidar-pipeline-reference.md`                            | EXISTS | `lidar-architecture-graph-plan`                       | Graph becomes asset of this doc       |
| `lidar-sidecar-overview.md`                              | EXISTS | -                                                     | Already canonical                     |
| `foreground-tracking.md`                                 | EXISTS | `lidar-bodies-in-motion-plan`                         | Kinematic prediction extends tracking |
| `vector-scene-map.md`                                    | EXISTS | `lidar-l7-scene-plan`                                 | L7 world model merges                 |
| `ground-plane-extraction.md`                             | EXISTS | -                                                     | Already canonical                     |
| `coordinate-flow-audit.md`                               | EXISTS | -                                                     | Already canonical                     |
| `lidar-background-grid-standards.md`                     | EXISTS | -                                                     | Already canonical                     |
| `network-configuration.md`                               | EXISTS | -                                                     | Already canonical                     |
| `gps-ethernet-parsing.md`                                | EXISTS | -                                                     | Already canonical                     |
| `ml-solver-expansion.md`                                 | EXISTS | `lidar-ml-classifier-training-plan`                   | Training plan merges                  |
| `multi-model-ingestion-and-configuration.md`             | EXISTS | -                                                     | Already canonical                     |
| `av-range-image-format-alignment.md`                     | EXISTS | `lidar-av-lidar-integration-plan`                     | AV integration merges                 |
| `arena-go-deprecation-and-layered-type-layout-design.md` | EXISTS | -                                                     | Already canonical                     |
| `math-foundations-audit.md`                              | EXISTS | -                                                     | Already canonical                     |
| `l8-l9-l10-migration-notes.md`                           | EXISTS | `lidar-l8-analytics-l9-endpoints-l10-clients-plan`    | Upper-layer refactor merges           |
| `lidar-logging-stream-split-and-rubric-design.md`        | EXISTS | -                                                     | Already canonical                     |
| `l2-dual-representation.md`                              | DONE   | `lidar-l2-dual-representation-plan`                   | Polar/Cartesian dual storage          |
| `velocity-foreground-extraction.md`                      | DONE   | `lidar-velocity-coherent-foreground-extraction-plan`  | Alternative FG algorithm              |
| `pluggable-algorithm-selection.md`                       | DONE   | `lidar-architecture-dynamic-algorithm-selection-plan` | Algorithm registry                    |
| `distributed-sweep.md`                                   | DONE   | `lidar-distributed-sweep-workers-plan`                | Multi-machine sweep                   |
| `track-storage-consolidation.md`                         | DONE   | `lidar-tracks-table-consolidation-plan`               | Live/analysis merge                   |
| `label-vocabulary.md`                                    | DONE   | `label-vocabulary-consolidation-plan`                 | Unified L6 classification             |

#### [docs/lidar/operations/](../lidar/operations)

| Canonical file                             | Status | Fed by plan(s)                                      | Notes                            |
| ------------------------------------------ | ------ | --------------------------------------------------- | -------------------------------- |
| `auto-tuning.md`                           | EXISTS | `lidar-parameter-tuning-optimisation-plan`          | Parameter search merges          |
| `hint-sweep-mode.md`                       | EXISTS | `lidar-sweep-hint-mode-plan`                        | Already complete                 |
| `track-labelling-auto-aware-tuning.md`     | EXISTS | `lidar-track-labelling-auto-aware-tuning-plan`      | Labelling workflow merges        |
| `performance-regression-testing.md`        | EXISTS | `lidar-performance-measurement-harness-plan`        | Timing harness merges            |
| `pcap-analysis-mode.md`                    | EXISTS | `lidar-analysis-run-infrastructure-plan`            | Run infrastructure merges        |
| `settling-time-optimisation.md`            | EXISTS | -                                                   | Already canonical                |
| `foundations-fixit-progress.md`            | EXISTS | `lidar-architecture-foundations-fixit-plan`         | Fixit status merges              |
| `replay-case-management-implementation.md` | EXISTS | `lidar-replay-case-management-plan`                 | Replay case terminology + schema |
| `immutable-run-config.md`                  | DONE   | `lidar-immutable-run-config-asset-plan`             | Deterministic config + lineage   |
| `clustering-diagnostics.md`                | DONE   | `lidar-clustering-observability-and-benchmark-plan` | Per-frame diagnostics            |
| `test-corpus.md`                           | DONE   | `lidar-test-corpus-plan`                            | Five-PCAP validation dataset     |
| `observability-surfaces.md`                | DONE   | `hint-metric-observability-plan`                    | Scoring quality for labeller     |
| `static-pose-alignment.md`                 | DONE   | `lidar-static-pose-alignment-plan`                  | Hesai 7DOF (deferred)            |
| `motion-capture.md`                        | DONE   | `lidar-motion-capture-architecture-plan`            | Moving sensor (deferred)         |
| `data-completeness-remediation.md`         | DONE   | `unpopulated-data-structures-remediation-plan`      | Wire computed data to storage    |

#### `docs/lidar/operations/visualiser/` (new subdirectory)

Twelve visualiser QC plans describe lidar-specific QC features. Grouped in a
subdirectory to keep `operations/` navigable.

| Canonical file                        | Status | Fed by plan(s)                                                        |
| ------------------------------------- | ------ | --------------------------------------------------------------------- |
| `qc-enhancements-overview.md`         | DONE   | `lidar-visualiser-labelling-qc-enhancements-overview-plan`            |
| `track-quality-scoring.md`            | DONE   | `lidar-visualiser-track-quality-score-plan`                           |
| `track-event-timeline.md`             | DONE   | `lidar-visualiser-track-event-timeline-bar-plan`                      |
| `split-merge-repair.md`               | DONE   | `lidar-visualiser-split-merge-repair-workbench-plan`                  |
| `physics-checks.md`                   | DONE   | `lidar-visualiser-physics-checks-and-confirmation-gates-plan`         |
| `trails-and-uncertainty.md`           | DONE   | `lidar-visualiser-trails-and-uncertainty-visualisation-plan`          |
| `priority-review-queue.md`            | DONE   | `lidar-visualiser-priority-review-queue-plan`                         |
| `qc-dashboard-and-audit.md`           | DONE   | `lidar-visualiser-qc-dashboard-and-audit-export-plan`                 |
| `run-list-labelling-rollup.md`        | DONE   | `lidar-visualiser-run-list-labelling-rollup-icon-plan`                |
| `light-mode.md`                       | DONE   | `lidar-visualiser-light-mode-plan`                                    |
| `proto-contract.md`                   | DONE   | `lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan`        |
| `performance-and-timeline-metrics.md` | DONE   | `lidar-visualiser-performance-and-scene-health-timeline-metrics-plan` |

### [docs/radar/](../radar): 2 plans

| Canonical file                                       | Status | Fed by plan(s)                                  | Notes                            |
| ---------------------------------------------------- | ------ | ----------------------------------------------- | -------------------------------- |
| `architecture/time-partitioned-data-tables.md`       | EXISTS | -                                               | Already canonical                |
| `architecture/networking.md`                         | EXISTS | -                                               | Already canonical                |
| `architecture/serial-configuration-ui.md`            | EXISTS | -                                               | Already canonical                |
| `architecture/site-config-cosine-correction-spec.md` | EXISTS | -                                               | Already canonical                |
| `architecture/speed-limit-schedules.md`              | EXISTS | -                                               | Already canonical                |
| `architecture/transit-deduplication.md`              | EXISTS | -                                               | Already canonical                |
| `architecture/percentile-aggregation-semantics.md`   | DONE   | `speed-percentile-aggregation-alignment-plan` † | Percentile = aggregate not track |

### [docs/ui/](../ui): 6 plans

| Canonical file                          | Status | Fed by plan(s)                                | Notes                                |
| --------------------------------------- | ------ | --------------------------------------------- | ------------------------------------ |
| `DESIGN.md`                             | EXISTS | -                                             | Already canonical                    |
| `design-review-and-improvement.md`      | EXISTS | -                                             | Already canonical                    |
| `velocity-visualiser-architecture.md`   | EXISTS | `server-manager`                              | Connection lifecycle merges          |
| `velocity-visualiser-implementation.md` | EXISTS | `web-frontend-background-debug-surfaces-plan` | Background debug surfaces merge      |
| `velocity-visualiser-app/*`             | EXISTS | -                                             | Already canonical (5 files)          |
| `web-frontend-consolidation.md`         | DONE   | `web-frontend-consolidation-plan` †           | Port consolidation + SPA unification |
| `homepage.md`                           | DONE   | `homepage-responsive-gif-strategies`          | Static site responsive media         |
| `macos-menu-layout-design.md`           | DONE   | `wireshark-menu-alignment`                    | macOS menu layout design             |

### [docs/platform/](../platform): 22 plans (was engineering + platform)

#### [docs/platform/architecture/](../platform/architecture)

| Canonical file                  | Status | Fed by plan(s)                                                            | Notes                                   |
| ------------------------------- | ------ | ------------------------------------------------------------------------- | --------------------------------------- |
| `database-sql-boundary.md`      | DONE   | `data-database-alignment-plan`, `data-sqlite-client-standardisation-plan` | Two-package DB access model             |
| `track-description-language.md` | DONE   | `data-track-description-language-plan`                                    | Query language design                   |
| `go-package-structure.md`       | DONE   | `go-codebase-structural-hygiene-plan` †, `go-god-file-split-plan`         | Import boundaries + file size           |
| `structured-logging.md`         | DONE   | `go-structured-logging-plan`                                              | Three-stream logging model              |
| `typed-uuid-prefixes.md`        | DONE   | `platform-typed-uuid-prefixes-plan`                                       | Cross-package ID convention             |
| `tictactail-library.md`         | DONE   | `tictactail-platform-plan`                                                | Generic streaming aggregation           |
| `metrics-registry.md`           | DONE   | `metrics-registry-and-observability-plan` †                               | Naming rules + lifecycle                |
| `canonical-plan-graduation.md`  | DONE   | `platform-canonical-project-files-plan`                                   | This graduation model (self-graduating) |

#### [docs/platform/operations/](../platform/operations)

| Canonical file                  | Status | Fed by plan(s)                                                                   | Notes                             |
| ------------------------------- | ------ | -------------------------------------------------------------------------------- | --------------------------------- |
| `distribution-packaging.md`     | DONE   | `deploy-distribution-packaging-plan`                                             | Single-binary subcommand model    |
| `rpi-imager.md`                 | DONE   | `deploy-rpi-imager-fork-plan`                                                    | Image building + flashing         |
| `schema-migration-030.md`       | DONE   | `schema-simplification-migration-030-plan`                                       | Column/table renaming (transient) |
| `v050-release-migration.md`     | DONE   | `v050-backward-compatibility-shim-removal-plan`, `v050-tech-debt-removal-plan`   | Both v0.5.0 plans merge           |
| `documentation-standards.md`    | DONE   | `platform-documentation-standardisation-plan`, `line-width-standardisation-plan` | Doc metadata + line width merge   |
| `quality-coverage.md`           | DONE   | `platform-quality-coverage-improvement-plan`                                     | Coverage thresholds               |
| `data-science-methodology.md`   | DONE   | `platform-data-science-metrics-first-plan`                                       | Reproducibility principles        |
| `python-venv.md`                | DONE   | `tooling-python-venv-consolidation-plan`                                         | Shared root venv model            |
| `pdf-reporting.md`              | DONE   | `pdf-go-chart-migration-plan` †, `pdf-latex-precompiled-format-plan`             | Chart generation + TeX vendoring  |
| `agent-preparedness.md`         | DONE   | `agent-claude-preparedness-review-plan`                                          | AI agent knowledge architecture   |
| `simplification-deprecation.md` | DONE   | `platform-simplification-and-deprecation-plan`                                   | Tech debt reduction register      |

## Plans that split across hubs

| Plan                                          | Concept A → Hub                                                                 | Concept B → Hub                                                     |
| --------------------------------------------- | ------------------------------------------------------------------------------- | ------------------------------------------------------------------- |
| `go-codebase-structural-hygiene-plan`         | SQL boundary → `platform/architecture/database-sql-boundary.md`                 | Package structure → `platform/architecture/go-package-structure.md` |
| `speed-percentile-aggregation-alignment-plan` | Percentile semantics → `radar/architecture/percentile-aggregation-semantics.md` | Metric naming → `platform/architecture/metrics-registry.md`         |
| `pdf-go-chart-migration-plan`                 | All concepts stay in `platform/operations/pdf-reporting.md`                     | (phases, not separate hubs)                                         |
| `web-frontend-consolidation-plan`             | All concepts stay in `ui/web-frontend-consolidation.md`                         | Port consolidation noted in platform                                |

## Immediate actions

All completed on branch `dd/docs/merge-canonical`:

1. [x] Rename `docs/server/` → moved contents to [docs/platform/](../platform)
2. [x] Update [docs/platform/README.md](../platform/PLATFORM.md): expanded scope
3. [x] Update `Canonical:` links in 3 data-\* plans from `../server/` → `../platform/` <!-- link-ignore -->
4. [x] Update `ALLOWED_HUB_PREFIXES`: removed `docs/server/`
5. [x] Verified `make report-plan-hygiene`: 0 gate violations, 6 advisory notes

## Counts

| Hub                           | Existing docs | New canonical docs | Plans absorbed | % of plans |
| ----------------------------- | ------------- | ------------------ | -------------- | ---------- |
| [docs/lidar/](../lidar)       | ~42           | ~18                | 38             | 56%        |
| [docs/radar/](../radar)       | 8             | 1                  | 2              | 3%         |
| [docs/ui/](../ui)             | 9             | 3                  | 6              | 9%         |
| [docs/platform/](../platform) | 1             | 19                 | 22             | 32%        |

## Implementation sequence

1. [x] Land this plan and the immediate actions (section above)
2. [x] Phase 2 of parent plan: `Canonical` metadata added to all 69 plans
3. [x] 46 NEW canonical doc stubs created across 4 hubs
4. [ ] Merge durable content from plans into canonical docs
5. [ ] Graduate superseded plans to symlinks
6. [ ] Hard-fail CI (Phase 3 of parent plan)
