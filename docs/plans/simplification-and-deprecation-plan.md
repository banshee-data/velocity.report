# Simplification and Deprecation Plan

## Status: Draft

## Goal

Create a single, prioritised plan to reduce non-core operational surface area, focusing on:

1. Make targets
2. `cmd/` applications and tools
3. CLI flags
4. Consolidation of metrics/stats/frontend surfaces

This plan is scoped to capabilities that are not essential to the core query-serving path (`cmd/radar` on `:8080` + SQLite-backed APIs).

## Baseline (2026-02-21)

- Make targets: 118 (`Makefile`)
- Top-level `cmd/` applications: `radar`, `deploy`, `sweep`, `transit-backfill`, `tools/*`
- `cmd/radar` CLI flags: 32 (`cmd/radar/radar.go`)
- Existing strategic dependencies:
  - Raspberry Pi image pipeline: [#210](../../BACKLOG.md)
  - Frontend consolidation: [#252](../../BACKLOG.md)

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

Rationale: these are superseded by the image-builder direction once [#210](../../BACKLOG.md) lands.

#### B. Data visualisation wrappers (medium priority)

- `plot-noise-sweep`, `plot-multisweep`, `plot-noise-buckets`
- `stats-live`, `stats-pcap`

Rationale: these duplicate visibility goals already being migrated under frontend consolidation [#252](../../BACKLOG.md).

#### C. API shortcut wrappers (medium priority)

- `api-*` shortcut targets that wrap HTTP endpoints (e.g. `api-grid-status`, `api-params`, `api-start-pcap`)

Rationale: useful for development, but not required as first-class public workflow once UI and API docs are consolidated.

### 2) `cmd/` app and tool candidates

#### A. `cmd/deploy` (conditional deprecation, highest impact)

- Candidate for staged deprecation once image builds and flashing flow are available.
- Expected reduction: one binary + associated Make targets + duplicated deployment docs and pathways.
- Dependency: [Raspberry Pi imager pipeline](rpi-imager-fork-design.md) and packaging roadmap.

#### B. `cmd/transit-backfill` (high priority)

- One-off operational backfill utility that can move behind documented maintenance procedures.
- Candidate to deprecate after confirming no active production need.

#### C. `cmd/sweep` and ad hoc `cmd/tools/*` utilities (medium priority)

- `cmd/sweep` remains useful during transition, but should be reviewed after frontend sweep migration in [#252](../../BACKLOG.md).
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

### Option 1 — Consolidate on Svelte surface (recommended)

- Use [frontend consolidation](frontend-consolidation.md) as the canonical migration path.
- Retire duplicated stats/debug HTML surfaces after parity is reached.
- Move “stats/metrics first look” workflows into one route hierarchy and one API surface.

### Option 2 — Keep dual surfaces but reduce documented surface

- Keep existing monitor pages and scripts, but mark them internal-only.
- Lower migration risk, but retains duplicated maintenance burden.

### Option 3 — CLI-first metrics workflow

- Standardise on API + CLI scripts and minimise UI migration.
- Lowest web effort, but weakest operator UX and discoverability.

## Prioritised Deprecation Targets

1. **`cmd/deploy` deprecation path** (start now; remove after #210 milestones)
2. **Deployment Make target cleanup** (`setup-radar`, `deploy-*`, `build-deploy*`)
3. **`cmd/transit-backfill` and unowned tools cleanup**
4. **LiDAR forwarding flag simplification**
5. **Stats/plot/API-shortcut target consolidation after #252 parity**

## Delivery Plan (Task Lists)

### Project A (P1): Deprecation readiness and signalling

- [ ] Add deprecation notices to `setup-radar`, deploy targets, and `cmd/deploy` docs
- [ ] Publish migration guidance: “deploy tool → image pipeline”
- [ ] Freeze new feature work in `cmd/deploy` except critical fixes
- [ ] Record active usage assumptions for `cmd/transit-backfill` and ad hoc tools

### Project B (P1): Deploy retirement gate

- [ ] Define explicit removal gate: #210 image pipeline operational + packaging path confirmed
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

## Decision Notes

- This plan intentionally prioritises deprecation signalling first, then removal.
- No privacy model changes are proposed: local-only storage and no PII remain unchanged.
- Removal milestones are dependency-gated to avoid breaking existing deployments.
