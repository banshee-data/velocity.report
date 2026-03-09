# Simplification and Deprecation Plan

## Status: Approved (Phase 1 complete)

**Layers:** Cross-cutting (platform)

## Goal

Create a single, prioritised plan to reduce non-core operational surface area and
clean up backward compatibility debt, focusing on:

1. Make targets
2. `cmd/` applications and tools
3. CLI flags
4. Consolidation of metrics/stats/frontend surfaces
5. Data model and API backward compatibility shims ([sub-plan](v050-backward-compatibility-shim-removal-plan.md))

This plan is scoped to capabilities that are not essential to the core query-serving path (`cmd/radar` on `:8080` + SQLite-backed APIs).

## Baseline (2026-02-21)

- Make targets: 118 (`Makefile`)
- Top-level `cmd/` applications: `radar`, `deploy`, `sweep`, `transit-backfill`, `tools/*`
- `cmd/radar` CLI flags: 32 (`cmd/radar/radar.go`)
- Existing strategic dependencies:
  - Raspberry Pi image pipeline: [#210](../BACKLOG.md)
  - Frontend consolidation: [#252](../BACKLOG.md)

## What is Core vs Non-Core

### Core to serving queries

- `cmd/radar` binary and its API serving path (`--listen`, DB path, units/timezone, radar/LiDAR runtime enablement)
- Database migration, schema, and query paths used by HTTP APIs
- Web app surface on `:8080` used by operators

### Non-core or simplification candidates

- Legacy deployment wrappers and duplicated deploy pathways
- One-off migration/backfill binaries kept as permanent surface
- Local plotting helper targets duplicated by modern frontend/dashboard surfaces
- CLI flags tied to transitional debug/prototype pathways

## Deprecation Candidate Inventory

### 1) Make target candidates

#### A. Deployment legacy surface (high priority)

- `setup-radar` (already labelled legacy in Make help)
- `deploy-install`, `deploy-upgrade`, `deploy-status`, `deploy-health`
- `build-deploy`, `build-deploy-linux`
- `deploy-install-latex`, `deploy-install-latex-minimal`, `deploy-update-deps`

Rationale: these are superseded by the image-builder direction once [#210](../BACKLOG.md) lands.

#### B. Data visualisation wrappers (medium priority)

- `plot-noise-sweep`, `plot-multisweep`, `plot-noise-buckets`
- `stats-live`, `stats-pcap`

Rationale: these duplicate visibility goals already being migrated under frontend consolidation [#252](../BACKLOG.md).

#### C. API shortcut wrappers (medium priority)

- `api-*` shortcut targets that wrap HTTP endpoints (e.g. `api-grid-status`, `api-params`, `api-start-pcap`)

Rationale: useful for development, but not required as first-class public workflow once UI and API docs are consolidated.

### 2) `cmd/` app and tool candidates

#### A. `cmd/deploy` (conditional deprecation, highest impact)

- Candidate for staged deprecation once image builds and flashing flow are available.
- Expected reduction: one binary + associated Make targets + duplicated deployment docs and pathways.
- Dependency: [Raspberry Pi imager pipeline](deploy-rpi-imager-fork-plan.md) and packaging roadmap.

#### B. `cmd/transit-backfill` (high priority)

- One-off operational backfill utility that can move behind documented maintenance procedures.
- Candidate to deprecate after confirming no active production need.

#### C. `cmd/sweep` and ad hoc `cmd/tools/*` utilities (medium priority)

- `cmd/sweep` remains useful during transition, but should be reviewed after frontend sweep migration in [#252](../BACKLOG.md).
- `cmd/tools/scan_transits.go` and narrow-scope helper tools should be either:
  - promoted and maintained as supported tooling, or
  - explicitly marked deprecated and removed.

### 3) CLI flag candidates (`cmd/radar`)

#### A. Transitional/debug LiDAR forwarding flags (high priority)

- `--lidar-foreground-forward`
- `--lidar-foreground-forward-port`
- `--lidar-foreground-forward-addr`

Rationale: niche forwarding path; high cognitive load for low general usage.

#### B. Port-split and monitor-era flags (medium priority, dependency on #252)

- `--lidar-listen` (port `:8081`)

Rationale: candidate for deprecation when monitor/frontend consolidation retires the split-surface model.

#### C. Consolidation candidates (medium priority)

- PDF flow flags (`--pdf-latex-flow`, `--pdf-tex-root`) should be assessed for simplification into a single operator-facing mode selector.
- Transit worker tuning flags can remain but should be grouped and documented as advanced/runtime tuning.

## Consolidation Options (Metrics, Stats, Frontend)

### Option 1 — Consolidate on Svelte surface (recommended) ✅

- Use [frontend consolidation](web-frontend-consolidation-plan.md) as the canonical migration path.
- Retire duplicated stats/debug HTML surfaces after parity is reached.
- Move “stats/metrics first look” workflows into one route hierarchy and one API surface.

### ~~Option 2 — Keep dual surfaces but reduce documented surface~~

- Keep existing monitor pages and scripts, but mark them internal-only.
- Lower migration risk, but retains duplicated maintenance burden.

### ~~Option 3 — CLI-first metrics workflow~~

- Standardise on API + CLI scripts and minimise UI migration.
- Lowest web effort, but weakest operator UX and discoverability.

## Prioritised Deprecation Targets

1. **`cmd/deploy` deprecation path** (start now; remove after #210 milestones)
2. **Deployment Make target cleanup** (`setup-radar`, `deploy-*`, `build-deploy*`)
3. **Data model and API compat-shim removal** ([sub-plan](v050-backward-compatibility-shim-removal-plan.md)) — v0.5.0 breaking changes
4. **`cmd/transit-backfill` and unowned tools cleanup**
5. **LiDAR forwarding flag simplification**
6. **Stats/plot/API-shortcut target consolidation after #252 parity**

## Migration Guidance: Deploy Tool → Image Pipeline

The `cmd/deploy` tool and its associated Make targets (`setup-radar`, `deploy-install`, `deploy-upgrade`, `deploy-status`, `deploy-health`, `deploy-install-latex`, `deploy-install-latex-minimal`, `deploy-update-deps`) are deprecated. The replacement workflow is the Raspberry Pi image pipeline ([#210](../BACKLOG.md), [design doc](deploy-rpi-imager-fork-plan.md)).

### Current workflow (deprecated)

1. Cross-compile binary: `make build-radar-linux`
2. Build deploy tool: `make build-deploy`
3. Copy binary and deploy tool to Pi or use SSH: `make deploy-install`
4. Install LaTeX remotely: `make deploy-install-latex TARGET=<host>`
5. Upgrade via SSH: `make deploy-upgrade`

### Future workflow (image pipeline, #210)

1. Build a complete Raspberry Pi image: `make build-image` (planned)
2. Flash the image to an SD card using Raspberry Pi Imager or `dd`
3. Boot the Pi — the service starts automatically with all dependencies pre-installed
4. Upgrade by re-flashing a new image or using an over-the-air update mechanism (TBD)

### Transition period

- Both workflows are available until the removal gate (below) is met.
- No new features will be added to `cmd/deploy` or the deprecated Make targets.
- Critical bug fixes remain accepted during the transition.

## Active Usage Assumptions

### `cmd/transit-backfill`

- **Current status:** One-off batch tool for backfilling `radar_data_transits` from historical `radar_data` events.
- **Active production need:** None confirmed. The built-in hourly transit worker (`--enable-transit-worker`) and the `velocity-report transits rebuild` subcommand now cover the same use case.
- **Recommendation:** Deprecate after v0.5.0. The `transits rebuild` subcommand in `cmd/radar` is the supported replacement.

### `cmd/tools/scan_transits.go`

- **Current status:** Scans for hourly periods with `radar_data` but no corresponding transit records and optionally backfills.
- **Active production need:** None confirmed. Duplicates `cmd/transit-backfill` capability at a different granularity.
- **Recommendation:** Deprecate alongside `cmd/transit-backfill`.

### `cmd/sweep`

- **Current status:** Parameter sweep utility for LiDAR tuning. Actively used for iterative sensor calibration.
- **Active production need:** Yes — required until frontend sweep migration ([#252](../BACKLOG.md)) provides equivalent capability.
- **Recommendation:** Retain until #252 parity, then review.

### `cmd/tools/backfill_ring_elevations`

- **Current status:** Backfills ring elevation data for LiDAR background grid.
- **Active production need:** Low. Used during initial LiDAR setup, not ongoing operations.
- **Recommendation:** Retain as maintenance tool; review when LiDAR foundations fix-it completes.

## Deploy Retirement Gate

Removal of `cmd/deploy`, its associated Make targets, and legacy deployment documentation is gated on **all** of the following conditions being met:

1. **#210 image pipeline operational:** A working `make build-image` (or equivalent) target produces a bootable Raspberry Pi image with `velocity-report` binary, systemd service, database, and LaTeX pre-installed. The image must boot on a Raspberry Pi 4 and pass an integration check: service starts, API responds on `:8080`, and database is accessible.
2. **Packaging path confirmed:** At least one successful end-to-end deployment has been performed using the image pipeline (flash → boot → service running → API responding).
3. **Migration period elapsed:** At least one minor release (e.g. v0.7.0) has shipped with both the image pipeline and the deprecated deploy tool available, giving users time to migrate.
4. **No active deploy-tool users:** No known deployments rely exclusively on `cmd/deploy` for upgrades (confirmed via release notes or user communication).

Once all four conditions are met, the following will be removed:

- `cmd/deploy/` directory and binary
- `internal/deploy/` package
- Makefile targets: `setup-radar`, `deploy-install`, `deploy-upgrade`, `deploy-status`, `deploy-health`, `build-deploy`, `build-deploy-linux`, `deploy-install-latex`, `deploy-install-latex-minimal`, `deploy-update-deps`
- `scripts/setup-radar-host.sh`
- Deployment section from `README.md` (replaced by image pipeline instructions)

## v0.5.0 Breaking Changes Plan

The following breaking changes are planned for the v0.5.0 release. They are
documented here so that downstream consumers can prepare.

The full inventory of data model and API compat-shim removals is tracked in the
sub-plan:
[v0.5.0 Backward Compatibility Shim Removal Plan](v050-backward-compatibility-shim-removal-plan.md).

### 1. Visualiser proto: percentile-style additions withdrawn from track contract

- **What:** The proposed branch-local percentile-style track speed additions are not the direction to ship. Percentiles are reserved for grouped/report aggregates only. The stable track speed contract remains `avg_speed_mps` (field 24) plus the raw maximum, which should be renamed from `peak_speed_mps` (field 25 today) to `max_speed_mps` before merge if the contract is still unshipped.
- **Impact:** gRPC and REST clients should not adopt branch-local percentile-labelled track speed fields as a stable contract. Existing aggregate/report percentile work is unaffected.
- **Migration:** Treat `avg_speed_mps` and the raw maximum (`max_speed_mps` after rename; `peak_speed_mps` only as a temporary branch-local name) as the only stable per-track speed summary fields for now. The branch-local percentile-style additions should be backed out before merge.
- **Design docs:** [speed-percentile-aggregation-alignment-plan.md](speed-percentile-aggregation-alignment-plan.md), [lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md](lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md), [shim removal §1](v050-backward-compatibility-shim-removal-plan.md#1-go-server--track-speed-contract-reset)

### 2. Deployment surface deprecated

- **What:** `cmd/deploy`, `setup-radar`, and all `deploy-*` Make targets now print deprecation warnings. No functionality is removed in v0.5.0 but users should plan for removal in v0.7.0 or later, once the retirement gate is satisfied.
- **Impact:** Operators who rely on `make deploy-install` or `velocity-deploy` will see deprecation warnings on stderr. Scripts that parse stderr, or that treat any output as machine-readable, may need to be updated to ignore or handle these warning lines.
- **Migration:** Begin planning migration to the image pipeline (#210) when available.

### 3. `cmd/transit-backfill` soft-deprecated

- **What:** `cmd/transit-backfill` is soft-deprecated. It continues to work but is no longer the recommended approach.
- **Impact:** None in v0.5.0. Removal planned for a future release after confirmation of zero active usage.
- **Migration:** Use `velocity-report transits rebuild` instead.

### 4. Sweep API: legacy request/result fields removed

- **What:** Legacy sweep request fields (`noise_values`, `closeness_values`, `neighbour_values`, per-variable range fields, fixed-value fields) and legacy result fields (`noise`, `closeness`, `neighbour` at top level of `ComboResult`) are removed. The `computeCombinations()` legacy code path is deleted.
- **Impact:** Any client sending sweep requests in the old per-variable format will receive errors. Sweep results no longer include top-level `noise`/`closeness`/`neighbour` keys.
- **Migration:** Use the `param_values` map format for requests and results. See [shim removal §2](v050-backward-compatibility-shim-removal-plan.md#2-go-server--sweep-legacy-request-format).

### 5. Report download: query-parameter endpoint removed

- **What:** The legacy `/api/reports/{id}/download?file_type=pdf` endpoint is removed.
- **Impact:** Callers using the query-parameter format will receive 404s.
- **Migration:** Use the path-based format: `/api/reports/{id}/download/{filename}.pdf`. See [shim removal §3](v050-backward-compatibility-shim-removal-plan.md#3-go-server--legacy-download-endpoint-format).

### 6. Stats API: bare-array response format removed

- **What:** The stats API (`/api/radar/stats`) no longer returns a bare JSON array. It always returns `{ "metrics": [...], "histogram": {...} }`.
- **Impact:** Any consumer expecting a bare `[...]` array will break. The Python PDF generator legacy format branch is also removed.
- **Migration:** Parse the response as an object with a `metrics` key. See [shim removal §9, §13](v050-backward-compatibility-shim-removal-plan.md#9-python--legacy-api-response-format-handling).

### 7. Sweep handler: malformed JSON now returns 400

- **What:** The sweep handler previously silently ignored malformed JSON request bodies. It now returns `400 Bad Request`.
- **Impact:** Callers sending invalid JSON will receive errors instead of silent acceptance.
- **Migration:** Ensure sweep requests are valid JSON. See [shim removal §4](v050-backward-compatibility-shim-removal-plan.md#4-go-server--lenient-json-parsing-in-sweep-handler).

### Unchanged in v0.5.0

- No CLI flags are removed in v0.5.0.
- Privacy model is unchanged: local-only storage, no PII.

## Delivery Plan (Task Lists)

### Project A (P1): Deprecation readiness and signalling

- [x] Add deprecation notices to `setup-radar`, deploy targets, and `cmd/deploy` docs
- [x] Publish migration guidance: “deploy tool → image pipeline”
- [x] Freeze new feature work in `cmd/deploy` except critical fixes
- [x] Record active usage assumptions for `cmd/transit-backfill` and ad hoc tools

### Project B (P1): Deploy retirement gate

- [x] Define explicit removal gate: #210 image pipeline operational + packaging path confirmed
- [ ] Remove legacy deploy targets once the gate is met
- [ ] Remove `cmd/deploy` binary once migration period closes
- [ ] Update setup/deployment docs to image-first workflow

### Project C (P2): Metrics/stats/frontend consolidation

- [ ] Complete #252 status/regions/sweep migration
- [ ] Retire duplicated stats/plot targets replaced by UI parity
- [ ] Collapse or demote `api-*` Make shortcuts to internal developer scripts
- [ ] Deprecate `--lidar-listen` when port split is removed

### Project D (P2): CLI simplification pass

- [ ] Deprecate low-use LiDAR foreground-forward flags
- [ ] Group and document advanced transit worker flags
- [ ] Simplify PDF mode flags for operators while keeping backward compatibility for one release

### Project E (P1): Data model and API compat-shim removal

Sub-plan: [v0.5.0 Backward Compatibility Shim Removal Plan](v050-backward-compatibility-shim-removal-plan.md)

- [ ] Remove `AvgSpeedMps` from visualiser model, proto field 24 rename, p85/p98 fields (PR #336)
- [ ] Remove `AvgSpeedMps` from REST API, TrackFeatures, track store, DB columns, pcap-analyse
- [ ] Remove Svelte/web compat shims (BackgroundCell legacy fields, dual-format cache, sweep legacy field names)
- [ ] Remove Python compat shims (legacy stats format, config dict helpers, pylatex stubs)
- [ ] Remove macOS compat shims (regenerate Swift proto, legacy point buffer, playback defaults)
- [ ] Validation pass: lint, test, build across all platforms

Intersections with other projects:

- Sweep legacy field removal (shim §2, §14) shares scope with Project C (#252 sweep migration). The shim removal covers server-side and frontend field-name changes; Project C covers full sweep UI migration.
- Deploy executor compat methods (shim §5) deferred to Project B (deploy retirement gate).

## Decision Notes

- This plan intentionally prioritises deprecation signalling first, then removal.
- No privacy model changes are proposed: local-only storage and no PII remain unchanged.
- Removal milestones are dependency-gated to avoid breaking existing deployments.
- Phase 1 (Project A signalling + Project B gate definition) completed in v0.5.0.
- Actual removal of deprecated deployment surfaces is deferred to v0.7.0 after the retirement gate is satisfied.
- v0.5.0 also ships data model and API compat-shim removals (Project E). These are breaking changes coordinated as a single batch to avoid prolonged dual-format maintenance.
