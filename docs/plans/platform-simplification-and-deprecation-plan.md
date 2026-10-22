# Simplification and deprecation plan

- **Status:** Active (approved in February; Phase 1 complete; image-surface items absorbed into the v0.5.1 consolidation plan; Make targets re-baselined October 19, 2026 as Project F)
- **Canonical:** [simplification-deprecation.md](../platform/operations/simplification-deprecation.md)
- **Related:** [deploy-single-binary-image-consolidation-plan.md](./deploy-single-binary-image-consolidation-plan.md) — owns the image apt-surface and `cmd/` consolidation in v0.5.1

- **Layers:** Cross-cutting (platform)

---

> **Sequencing update (2026-05):** the "Project B execution" item that the v0.6.0 backlog row
> pointed at is now overlapped by
> [single-binary image plan][single-binary-plan]
> work units A and E. The two plans agree on direction: one binary, fewer apt packages, and no
> shipped `internal/cmd/tune` sibling. The consolidation plan owns the v0.5.1 sequencing and image
> stage edits; this plan continues to own the Makefile, CLI-flag and metrics/stats trim items that
> survive into v0.5.11.

[single-binary-plan]: ./deploy-single-binary-image-consolidation-plan.md

## Goal

Create a single, prioritised plan to reduce non-core operational surface area and
clean up backward compatibility debt, focusing on:

1. Make targets
2. `cmd/` applications and tools
3. CLI flags
4. Consolidation of metrics/stats/frontend surfaces
5. Data model and API backward compatibility shims ([sub-plan](v050-backward-compatibility-shim-removal-plan.md))

This plan is scoped to capabilities that are not essential to the core query-serving path ([internal/cmd/server](../../internal/cmd/server) on `:8080` + SQLite-backed APIs).

## Baseline (2026-02-21)

- Make targets: 118 (`Makefile`)
- Top-level `cmd/` applications: `radar`, `deploy`, `sweep`, `transit-backfill`, `tools/*`
- [internal/cmd/server](../../internal/cmd/server) CLI flags: 32 ([internal/cmd/server/radar.go](../../internal/cmd/server/radar.go))
- Existing strategic dependencies:
  - Raspberry Pi image pipeline: [#210](../BACKLOG.md)
  - Frontend consolidation: [#252](../BACKLOG.md)

## What is core vs non-core

### Core to serving queries

- [internal/cmd/server](../../internal/cmd/server) binary and its API serving path (`--listen`, DB path, units/timezone, radar/LiDAR runtime enablement)
- Database migration, schema, and query paths used by HTTP APIs
- Web app surface on `:8080` used by operators

### Non-core or simplification candidates

- Legacy deployment wrappers and duplicated deploy pathways
- One-off migration/backfill binaries kept as permanent surface
- Local plotting helper targets duplicated by modern frontend/dashboard surfaces
- CLI flags tied to transitional debug/prototype pathways

## Deprecation candidate inventory

### 1) Make target candidates

The February candidates are done or carried below: the deploy targets (`setup-radar`,
`deploy-*`, `build-deploy*`) were removed in v0.5.1, and none remains. The plot, stats and API
wrappers are still present, and the Makefile has grown around them.

#### Re-baseline (October 19, 2026)

| Measure                                                                      | Value                                                                   |
| ---------------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| Targets                                                                      | 231 (118 in February), in 2,857 lines                                   |
| Listed by the hand-written `help` block                                      | 176; 55 unlisted; two sentence fragments echoed as if they were targets |
| Targets with a `##` comment                                                  | 19                                                                      |
| Targets called by CI                                                         | 36                                                                      |
| Targets nothing calls (no CI step, skill, doc, script, code or other target) | 68                                                                      |
| CI steps that run a script or `pnpm` directly instead of a Make target       | 27                                                                      |

Every target was traced to its callers in `.github/workflows`, `.claude` and `.github` skills,
agents and knowledge, the Markdown docs, `scripts/`, `tools/`, `image/` and the other Make targets.

#### Dispositions

| Group                           | Targets                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           | Count | Callers today                                                                                                                                                                                                                                                                                              | Disposition                                                                                                                                                                                                                                                                                            |
| ------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----: | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| A. Aliases                      | `build-radar-linux`, `build-radar-mac`, `build-radar-mac-intel`, `build-ctl`, `build-ctl-linux`, `build-tools`, `build-velocity-linux-embed`, `build-radar-linux-embed`, `build-radar-local`, `codex-setup`, `lint-docs-offline`, `config-order-check`, `config-order-sync`, `readme-maths-check`, `readme-maths-check-strict`, `render-scene-map-mac`, `render-scene-map-linux`                                                                                                                                                                                                                                                  |    17 | `build-radar-local`: four skills (release-prep, review-pr, run-experiments, ship-change), AGENTS.md, the Copilot instructions, the build-and-test knowledge module and more docs; `build-radar-linux`: two skills; `build-ctl`: release-prep. The release-prep skill must move before the next cut uses it | Delete. Callers move to the canonical name in the same PR. One tombstone rule fails `build-radar-*` with the new name for one release. `build-radar-local` is `build-velocity` writing `velocity-report-local`; dropping it leaves one dev binary, `velocity`, which the scene publisher already needs |
| B. API curl wrappers            | `api-grid-status`, `api-grid-reset`, `api-grid-heatmap`, `api-snapshot`, `api-snapshots`, `api-acceptance`, `api-acceptance-reset`, `api-params`, `api-params-set`, `api-persist`, `api-export-snapshot`, `api-export-next-frame`, `api-status`, `api-start-pcap`, `api-stop-pcap`, `api-switch-data-source`                                                                                                                                                                                                                                                                                                                      |    16 | none; all default to :8081                                                                                                                                                                                                                                                                                 | Delete the Make wrappers. The scripts stay in `scripts/api/lidar` with their README, and their default port moves to :8080 with the :8081 retirement                                                                                                                                                   |
| C. Migration wrappers           | `migrate-up`, `migrate-down`, `migrate-status`, `migrate-detect`, `migrate-version`, `migrate-force`, `migrate-baseline`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |     7 | none                                                                                                                                                                                                                                                                                                       | Delete. `velocity data migrate <command>` is the interface; COMMANDS.md documents it                                                                                                                                                                                                                   |
| D. Python plot and stats        | `plot-noise-sweep`, `plot-multisweep`, `plot-noise-buckets`, `stats-live`, `stats-pcap` (`stats-live` and `stats-pcap` deleted with `tools/grid-heatmap` in v0.5.2)                                                                                                                                                                                                                                                                                                                                                                                                                                                               |     5 | docs only                                                                                                                                                                                                                                                                                                  | Delete. The `data/explore` scripts stay runnable where they are; `stats-live`, `stats-pcap` and `tools/grid-heatmap` go once the Go-served grid view replaces it (D-17, amended)                                                                                                                       |
| E. Wrappers of `velocity lidar` | `build-pcap-split`, `run-pcap-split`, `run-settling-eval`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |     3 | one doc                                                                                                                                                                                                                                                                                                    | Delete with `cmd/tools/pcap-split` and `cmd/tools/settling-eval`; `velocity lidar pcap-split` and `velocity lidar settling-eval` remain                                                                                                                                                                |
| F. Conveniences nothing calls   | `print-version`, `version-exact`, `git-fs`, `git-files`, `dev-vis-server`, `record-sample`, `vrlog-analyse`, `run-pcap-stats`, `run-pcap-stats-10s`, `render`, `cache-basemap`, `build-embedded-assets`, `release-radar-remote`, `test-go-coverage-summary`, `report-backtick-paths`, `schema-erd-dot`, `schema-erd-from-dot`, `debug-mac-cycles`, `profile-macos-lidar`                                                                                                                                                                                                                                                          |    19 | none                                                                                                                                                                                                                                                                                                       | Delete. A target someone runs by hand moves to `scripts/` with a usage line instead; `release-radar-remote` is the SSH install path D-14 retired, beside `velocity device upgrade --binary`                                                                                                            |
| G. Variants that fold           | `log-go-cat`, `log-go-tail-all` into `log-go-tail`; `dev-go-lidar-trace` into `dev-go-lidar` with a log level; `dev-go-kill-server`, `dev-docs-kill`, `dev-docs-offline-kill` into the dev targets that start the servers; `test-go-coverage-gate` into `test-go-cov-pcap`; `clean-web`, `clean-mac`, `clean-docs-offline` into one `clean`; `install-docs`, `install-docs-offline`, `install-scene-capture`, `install-s2-hilbert` into one install for a pnpm workspace; `install-transcribe` with the podcast tool's move out; `build-docs-public-html`, `verify-docs-public-html-build` into the site build with a path prefix |    17 | the image build calls `build-docs-public-html`; one skill calls `install-transcribe`                                                                                                                                                                                                                       | Fold. The image build and the skill change in the same PR                                                                                                                                                                                                                                              |
| H. Names that mislead           | `build-radar-static`, `build-radar-static-amd64`, `build-radar-static-arm64`, `build-radar-linux-docker`, `test-radar-static-arm64-vm`, `test-radar-static-golden-pcap`, `release-build-darwin-radar`, `release-package-linux-radar`, `build-docs`, `dev-docs`, `install-web`                                                                                                                                                                                                                                                                                                                                                     |    11 | CI calls `build-radar-static` twice, `build-docs` once and the two release targets once each                                                                                                                                                                                                               | Rename: `velocity` in place of `radar`, no `-radar` suffix on release targets, `site` for the public site that `build-docs` builds today, and one JavaScript install. Callers move in the same PR; the tombstone rule covers the old names for one release                                             |

The other 136 targets stay as they are: the quality gate, the builds, the release and macOS
signing chain, protobuf, the dev servers, the test and coverage targets, perf and evidence runs,
the gated checks, the experiment targets, and the scene publishing targets until the survey
site leaves the repository.

**Result.** 231 targets less the 84 deleted or folded, plus one new `clean`: 148, against a
budget of 150.

#### Keeping the count down

1. **Generated help.** Each public target carries a `##` comment and `make help` prints them by section; a target without one is internal and hidden. The 226-line echo block goes, and with it the 55 targets it forgot.
2. **A Make check in lint.** It fails on an alias recipe (a single `$(MAKE)` call or a single prerequisite with nothing else), fails on a public target without a `##` comment, and reports the count against the budget. Advisory for one release, then gated.
3. **CI calls Make.** The 27 direct steps move to targets, most of which already exist: `check-md-links` for the two link checkers, `lint-web`, `test-web` and `build-web` for the web steps, the static VM test for the image workflow, and a `check-lfs` target for the four LFS validations.
4. **Includes after the trim.** Splitting the Makefile into one include per concern under `mk/` is worth doing once the file is smaller; doing it first would turn every deletion into a cross-file conflict.

Under D-20, Make targets sit inside the release unit: the docs, skills and workflows that call
them change in the same commit. They never need an alias, only a tombstone for the person who
types the old name.

### 2) `cmd/` app and tool candidates

#### A. `cmd/deploy` (replaced by `velocity device …` in v0.5.1): ✅ complete

- Replaced by [internal/cmd/device/](../../internal/cmd/device): purpose-built on-device management binary.
- Reduction: one binary + 3,678 LOC + SSH surface + associated Make targets + duplicated deployment docs.
- See [deploy-rpi-imager-fork-plan.md § 8](deploy-rpi-imager-fork-plan.md#8-deploy-tool-replacement-velocity-device) for the superseded intermediate design.

#### B. `cmd/transit-backfill` (high priority): ✅ complete

- Removed `cmd/transit-backfill/` and `cmd/tools/scan_transits.go`.
- `velocity-report transits rebuild` is the supported replacement.

#### C. [internal/cmd/tune](../../internal/cmd/tune) and ad hoc `cmd/tools/*` utilities (medium priority)

- [internal/cmd/tune](../../internal/cmd/tune) remains useful during transition, but should be reviewed after frontend sweep migration in [#252](../BACKLOG.md).
- `cmd/tools/scan_transits.go`: ✅ removed alongside `transit-backfill`.
- Other narrow-scope helper tools should be either:
  - promoted and maintained as supported tooling, or
  - explicitly marked deprecated and removed.

### 3) CLI flag candidates ([internal/cmd/server](../../internal/cmd/server))

#### A. transitional/debug LiDAR forwarding flags (high priority)

- `--lidar-foreground-forward`
- `--lidar-foreground-forward-port`
- `--lidar-foreground-forward-addr`

Rationale: niche forwarding path; high cognitive load for low general usage.

#### B. port-split and monitor-era flags (medium priority, dependency on #252)

- `--lidar-listen` (port `:8081`)

Rationale: candidate for deprecation when monitor/frontend consolidation retires the split-surface model. The split went in v0.5.2: the `:8081` listener is retired and the flag is accepted and ignored with a warning until the v0.5.3 flag tombstone helper registers it.

#### C. consolidation candidates (medium priority)

- Legacy PDF flow flags should be removed rather than consolidated into a
  continuing operator-facing mode selector.
- Transit worker tuning flags can remain but should be grouped and documented as advanced/runtime tuning.

## Consolidation options (metrics, stats, frontend)

### Option 1: consolidate on Svelte surface (recommended) ✅

- Use [frontend consolidation](web-frontend-consolidation-plan.md) as the canonical migration path.
- Retire duplicated stats/debug HTML surfaces after parity is reached.
- Move “stats/metrics first look” workflows into one route hierarchy and one API surface.

### ~~Option 2: keep dual surfaces but reduce documented surface~~

- Keep existing monitor pages and scripts, but mark them internal-only.
- Lower migration risk, but retains duplicated maintenance burden.

### ~~Option 3: CLI-first metrics workflow~~

- Standardise on API + CLI scripts and minimise UI migration.
- Lowest web effort, but weakest operator UX and discoverability.

## Prioritised deprecation targets

1. **`cmd/deploy` replaced by `velocity device …`** (✅ v0.5.1)
2. **Deployment Make target cleanup** (`setup-radar`, `deploy-*`, `build-deploy*`): replaced by the on-device `velocity device …` lifecycle (✅ v0.5.1)
3. **Data model and API compat-shim removal** ([sub-plan](v050-backward-compatibility-shim-removal-plan.md)): v0.5.0 breaking changes
4. **`cmd/transit-backfill` and unowned tools cleanup**
5. **LiDAR forwarding flag simplification**
6. **Make target trim to a budget of 150** (Project F): stats, plot and API-shortcut wrappers no longer wait for #252 parity

## Migration guidance: deploy tool → `velocity device`

The `cmd/deploy` tool and its associated Make targets (`setup-radar`, `deploy-install`, `deploy-upgrade`, `deploy-status`, `deploy-health`, `deploy-install-latex`, `deploy-install-latex-minimal`, `deploy-update-deps`) are **deleted in v0.5.1**. The replacement is the `velocity device …` namespace in the multi-call binary; the earlier `velocity-ctl` intermediate design is superseded.

### Current workflow (deleted in v0.5.1)

1. Cross-compile binary: `make build-radar-linux`
2. Build deploy tool: `make build-deploy`
3. Copy binary and deploy tool to Pi or use SSH: `make deploy-install`
4. Install LaTeX remotely: `make deploy-install-latex TARGET=<host>`
5. Upgrade via SSH: `make deploy-upgrade`

### New workflow (v0.5.1)

1. Build a complete Raspberry Pi image: `make build-image`
2. Flash the image to an SD card using Raspberry Pi Imager or `dd`
3. Boot the Pi: the service starts automatically with all dependencies pre-installed
4. Upgrade in-place: `sudo velocity device upgrade` checks GitHub Releases,
   downloads the latest binary, and applies the upgrade: preserving the
   sensor database and all collected data. See
   [deploy-rpi-imager-fork-plan.md § 4.2.2a](deploy-rpi-imager-fork-plan.md#422a-update-mechanism)
   for the full workflow specification.

### Transition period

`cmd/deploy/` is deleted in v0.5.1. No transition period: the image has not
shipped to end users yet, so there are no existing deploy-tool users to migrate.

## Deploy retirement: complete

`cmd/deploy/` was deleted in v0.5.1 and replaced by [internal/cmd/device/](../../internal/cmd/device). The
four-gate retirement plan below is superseded: retained for historical
reference only.

<details>
<summary>Original four-gate retirement plan (superseded)</summary>

## Active usage assumptions

### `cmd/transit-backfill`

- **Current status:** One-off batch tool for backfilling `radar_data_transits` from historical `radar_data` events.
- **Active production need:** None confirmed. The built-in hourly transit worker (`--enable-transit-worker`) and the `velocity-report transits rebuild` subcommand now cover the same use case.
- **Recommendation:** Deprecate after v0.5.0. The `transits rebuild` subcommand in [internal/cmd/server](../../internal/cmd/server) is the supported replacement.

### `cmd/tools/scan_transits.go`

- **Current status:** Scans for hourly periods with `radar_data` but no corresponding transit records and optionally backfills.
- **Active production need:** None confirmed. Duplicates `cmd/transit-backfill` capability at a different granularity.
- **Recommendation:** Deprecate alongside `cmd/transit-backfill`.

### [internal/cmd/tune](../../internal/cmd/tune)

- **Current status:** Parameter sweep utility for LiDAR tuning. Actively used for iterative sensor calibration.
- **Active production need:** Yes; required until frontend sweep migration ([#252](../BACKLOG.md)) provides equivalent capability.
- **Recommendation:** Retain until #252 parity, then review.

### [cmd/tools/backfill_ring_elevations](../../cmd/tools/backfill_ring_elevations)

- **Current status:** Backfills ring elevation data for LiDAR background grid.
- **Active production need:** Low. Used during initial LiDAR setup, not ongoing operations.
- **Recommendation:** Retain as maintenance tool; review when LiDAR foundations fix-it completes.

## Deploy retirement gate

Removal of `cmd/deploy`, its associated Make targets, and legacy deployment documentation is gated on **all** of the following conditions being met:

1. **#210 image pipeline operational:** A working `make build-image` target produces a bootable Raspberry Pi image with the static multi-call `velocity` binary, systemd service, database directory, and embedded Go + Typst PDF path. The image must boot on a Raspberry Pi 4 and pass an integration check: service starts, API responds on `:80`, and database is accessible.
2. **Packaging path confirmed:** At least one successful end-to-end deployment has been performed using the image pipeline (flash → boot → service running → API responding).
3. **Migration period elapsed:** v0.5.0 has shipped with both the image pipeline and the deprecated deploy tool available, giving users time to migrate. Removal is planned for v0.5.1.
4. **No active deploy-tool users:** No known deployments rely exclusively on `cmd/deploy` for upgrades (confirmed via release notes or user communication).

Once all four conditions are met, the following will be removed:

- `cmd/deploy/` directory and binary
- `internal/deploy/` package
- Makefile targets: `setup-radar`, `deploy-install`, `deploy-upgrade`, `deploy-status`, `deploy-health`, `build-deploy`, `build-deploy-linux`, `deploy-install-latex`, `deploy-install-latex-minimal`, `deploy-update-deps`; all replaced by the image build and on-device `velocity device …` lifecycle
- `scripts/setup-radar-host.sh`
- Deployment section from [README.md](../../README.md) (replaced by image pipeline instructions)

</details>

## v0.5.0 Breaking changes plan

The following breaking changes are planned for the v0.5.0 release. They are
documented here so that downstream consumers can prepare.

The full inventory of data model and API compat-shim removals is tracked in the
sub-plan:
[v0.5.0 Backward Compatibility Shim Removal Plan](v050-backward-compatibility-shim-removal-plan.md).

### 1. Visualiser proto: percentile-style additions withdrawn from track contract

- **What:** The proposed branch-local percentile-style track speed additions are not the direction to ship. Percentiles are reserved for grouped/report aggregates only. The stable track speed contract remains `avg_speed_mps` (field 24) plus the raw maximum, which should be renamed from `peak_speed_mps` (field 25 today) to `max_speed_mps` before merge if the contract is still unshipped.
- **Impact:** gRPC and REST clients should not adopt branch-local percentile-labelled track speed fields as a stable contract. Existing aggregate/report percentile work is unaffected.
- **Migration:** Treat `avg_speed_mps` and the raw maximum (`max_speed_mps` after rename; `peak_speed_mps` only as a temporary branch-local name) as the only stable per-track speed summary fields for now. The branch-local percentile-style additions should be backed out before merge.
- **Design docs:** [speed-percentile-aggregation-alignment-plan.md](speed-percentile-aggregation-alignment-plan.md), [lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md](lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md), [shim removal overview](v050-backward-compatibility-shim-removal-plan.md)

### 2. Deployment surface replaced

- **What:** `cmd/deploy`, `setup-radar`, and all `deploy-*` Make targets are **deleted** in v0.5.1. Replaced by `velocity device …` ([internal/cmd/device/](../../internal/cmd/device)).
- **Impact:** `velocity-deploy` binary no longer exists. `velocity-update` wrapper script no longer exists. Users run `sudo velocity device upgrade` instead.
- **Migration:** See [distribution-packaging.md](../platform/operations/distribution-packaging.md) for the current shipped command surface.
- **Migration:** Begin planning migration to the image pipeline (#210) when available.

### 3. `cmd/transit-backfill` soft-deprecated

- **What:** `cmd/transit-backfill` is soft-deprecated. It continues to work but is no longer the recommended approach.
- **Impact:** None in v0.5.0. Removal planned for a future release after confirmation of zero active usage.
- **Migration:** Use `velocity-report transits rebuild` instead.

### 4. Sweep API: legacy request/result field removal (pending on `main`)

- **What:** `v0.5.0` is still intended to remove legacy sweep request fields (`noise_values`, `closeness_values`, `neighbour_values`, per-variable range fields, fixed-value fields) and legacy result fields (`noise`, `closeness`, `neighbour` at top level of `ComboResult`). The `computeCombinations()` legacy code path should be deleted as part of that cleanup.
- **Impact:** Once this lands, any client sending sweep requests in the old per-variable format will receive errors. Sweep results will no longer include top-level `noise`/`closeness`/`neighbour` keys.
- **Migration:** For sweep requests, use the `params: []SweepParam` request shape; for sweep results, use the `param_values` map on `ComboResult`. See the [shim removal overview](v050-backward-compatibility-shim-removal-plan.md).

### 5. Report download: query-parameter endpoint removed

- **What:** The legacy `/api/reports/{id}/download?file_type=pdf` endpoint is removed.
- **Impact:** Callers using the query-parameter format will receive 404s.
- **Migration:** Use the path-based format: `/api/reports/{id}/download/{filename}.pdf`. See the [shim removal overview](v050-backward-compatibility-shim-removal-plan.md).

### 6. Stats API: bare-array response format removed

- **What:** The stats API (`/api/radar/stats`) no longer returns a bare JSON array. It always returns `{ "metrics": [...], "histogram": {...} }`.
- **Impact:** Any consumer expecting a bare `[...]` array will break. The Python PDF generator legacy format branch is also removed.
- **Migration:** Parse the response as an object with a `metrics` key. See the [shim removal overview](v050-backward-compatibility-shim-removal-plan.md).

### 7. Sweep handler: malformed JSON now returns 400

- **What:** The sweep handler previously silently ignored malformed JSON request bodies. It now returns `400 Bad Request`.
- **Impact:** Callers sending invalid JSON will receive errors instead of silent acceptance.
- **Migration:** Ensure sweep requests are valid JSON. See the [shim removal overview](v050-backward-compatibility-shim-removal-plan.md).

### Unchanged in v0.5.0

- No CLI flags are removed in v0.5.0.
- Privacy model is unchanged: local-only storage, no PII.

## Delivery plan (task lists)

### Project a (P1): deprecation readiness and signalling

- [x] Add deprecation notices to `setup-radar`, deploy targets, and `cmd/deploy` docs
- [x] Publish migration guidance: “deploy tool → image pipeline”
- [x] Freeze new feature work in `cmd/deploy` except critical fixes
- [x] Record active usage assumptions for `cmd/transit-backfill` and ad hoc tools

### Project b (P1): deploy retirement gate

- [x] Define explicit removal gate: #210 image pipeline operational + packaging path confirmed
- [x] Remove legacy deploy targets once the gate is met (none remain, checked October 19, 2026)
- [x] Remove `cmd/deploy` binary once migration period closes (`cmd/` holds only `velocity` and `tools`)
- [ ] Update setup/deployment docs to image-first workflow

### Project c (P2): metrics/stats/frontend consolidation

- [ ] Complete #252 status/regions/sweep migration: implemented on branch `claude/echarts-svelte-migration-83fbd5`, #836
- [ ] Retire duplicated stats/plot targets: moved to Project F, which deletes them without waiting for parity
- [ ] Collapse or demote `api-*` Make shortcuts to internal developer scripts: moved to Project F
- [ ] Deprecate `--lidar-listen` when port split is removed: the split is removed on the same branch and the flag is ignored with a warning; the tombstone waits for the v0.5.3 helper

### Project d (P2): CLI simplification pass

- [ ] Deprecate low-use LiDAR foreground-forward flags
- [ ] Group and document advanced transit worker flags
- [ ] Simplify PDF mode flags for operators while keeping backward compatibility for one release

### Project f (P1): Make target trim (October 2026 re-baseline)

- [ ] Delete groups A to F of the [dispositions](#dispositions) and fold group G, moving every caller in the same PR (`M`)
- [ ] Rename group H with one tombstone rule for the old names, removed after one release (`S`)
- [ ] Generate `make help` from `##` comments and delete the echo block (`S`)
- [ ] Add the Make check to lint, advisory for one release and then gated (`S`)
- [ ] Move the 27 direct CI steps onto Make targets (`S`)
- [ ] Split the Makefile into includes under `mk/` after the trim (`S`)

### Project e (P1): data model and API compat-shim removal; retired to backlog

Sub-plan: [v0.5.0 Backward Compatibility Shim Removal Plan](v050-backward-compatibility-shim-removal-plan.md)

**Retired:** Task tracking for Project E has moved to [BACKLOG.md](../BACKLOG.md)
v0.5.0 under three items: "v0.5.0 backward compatibility shim removal"
(web/Python/macOS fallback removal), "Visualiser track proto parity" (proto
rename, percentile back-out, binding regeneration), and "v0.5.0 breaking
changes" (`transit-backfill` soft-deprecation, breaking-changes release notes). The sub-plan retains the
full implementation detail.

Intersections with other projects:

- Sweep legacy field removal (shim §2, §14) shares scope with Project C (#252 sweep migration). The shim removal covers server-side and frontend field-name changes; Project C covers full sweep UI migration.
- Deploy executor compat methods (shim §5) deferred to Project B (deploy retirement gate).

## Decision notes

- This plan intentionally prioritises deprecation signalling first, then removal.
- No privacy model changes are proposed: local-only storage and no PII remain unchanged.
- Removal milestones are dependency-gated to avoid breaking existing deployments.
- Phase 1 (Project A signalling + Project B gate definition) completed in v0.5.0.
- Actual removal of deprecated deployment surfaces is planned for v0.5.1.
- v0.5.0 also ships data model and API compat-shim removals (Project E). These are breaking changes coordinated as a single batch to avoid prolonged dual-format maintenance.
